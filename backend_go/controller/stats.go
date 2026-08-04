package controller

import (
	"api/models"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"events"

	"github.com/gorilla/mux"
)

// statusNever marks a user who has never had a calculation requested, which is
// distinct from one whose calculation genuinely produced zero characters.
const statusNever = "never"

// sseHeartbeatInterval keeps idle SSE connections from being closed by proxies.
const sseHeartbeatInterval = 20 * time.Second

const selectStatsSQL = `
	SELECT users_id, total_characters, posts_count, status,
	       COALESCE(request_id, ''), COALESCE(error_message, ''), calculated_at
	FROM user_post_stats
	WHERE users_id = $1`

const selectAllStatsSQL = `
	SELECT users_id, total_characters, posts_count, status,
	       COALESCE(request_id, ''), COALESCE(error_message, ''), calculated_at
	FROM user_post_stats`

// markPendingSQL flags a recalculation as in flight. Previous totals are deliberately
// left in place so the UI can keep showing the last known value while it recalculates.
const markPendingSQL = `
	INSERT INTO user_post_stats (users_id, status, request_id, requested_at)
	VALUES ($1, 'pending', $2, NOW())
	ON CONFLICT (users_id) DO UPDATE SET
		status        = 'pending',
		request_id    = EXCLUDED.request_id,
		requested_at  = EXCLUDED.requested_at,
		error_message = NULL`

const markFailedSQL = `
	UPDATE user_post_stats
	SET status = 'failed', error_message = $2
	WHERE users_id = $1`

// requestStats publishes a recalculation request to Kafka and returns immediately.
// The answer arrives later on the SSE stream; this handler never waits for it.
func requestStats(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := parseUserID(w, r)
		if !ok {
			return
		}

		var existingUserID int
		err := db.QueryRow("SELECT id FROM users WHERE id = $1", userID).Scan(&existingUserID)
		if err == sql.ErrNoRows {
			writeJSONError(w, http.StatusNotFound, "user not found")
			return
		}
		if err != nil {
			log.Printf("Error checking user %d before requesting stats: %v", userID, err)
			writeJSONError(w, http.StatusInternalServerError, "failed to request calculation")
			return
		}

		// Mark pending before publishing: once the message is out, stats_service may
		// finish at any moment, and a later 'pending' write would clobber its result.
		requestID := newRequestID()
		if _, err := db.Exec(markPendingSQL, userID, requestID); err != nil {
			log.Printf("Error marking stats pending for user %d: %v", userID, err)
			writeJSONError(w, http.StatusInternalServerError, "failed to request calculation")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		request, err := publishStatsRequest(ctx, userID, requestID)
		if err != nil {
			log.Printf("Error publishing stats request for user %d: %v", userID, err)
			if _, updateErr := db.Exec(markFailedSQL, userID, "failed to queue calculation"); updateErr != nil {
				log.Printf("Error marking stats failed for user %d: %v", userID, updateErr)
			}
			writeJSONError(w, http.StatusInternalServerError, "failed to queue calculation")
			return
		}

		log.Printf("Queued stats calculation for user %d (request %s)", userID, request.RequestID)

		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(models.UserPostStats{
			UserId:    userID,
			Status:    events.StatusPending,
			RequestId: request.RequestID,
		})
	}
}

// getUserStats returns the last known stats for one user.
func getUserStats(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := parseUserID(w, r)
		if !ok {
			return
		}

		stats, err := loadUserStats(db, userID)
		if err != nil {
			log.Printf("Error loading stats for user %d: %v", userID, err)
			writeJSONError(w, http.StatusInternalServerError, "failed to load stats")
			return
		}

		json.NewEncoder(w).Encode(stats)
	}
}

// getAllUserStats returns every stored row in one call, so the user list can render
// existing labels on page load without firing one request per user.
func getAllUserStats(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query(selectAllStatsSQL)
		if err != nil {
			log.Printf("Error loading all stats: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to load stats")
			return
		}
		defer rows.Close()

		stats := []models.UserPostStats{}
		for rows.Next() {
			entry, err := scanStats(rows)
			if err != nil {
				log.Printf("Error scanning stats row: %v", err)
				writeJSONError(w, http.StatusInternalServerError, "failed to load stats")
				return
			}
			stats = append(stats, entry)
		}

		if err := rows.Err(); err != nil {
			log.Printf("Error iterating stats rows: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to load stats")
			return
		}

		json.NewEncoder(w).Encode(stats)
	}
}

// streamUserStats holds an SSE connection open and pushes every result event for one
// user. It sends the current row immediately on connect, so a client that subscribes
// after the calculation already finished still sees the answer.
func streamUserStats(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := parseUserID(w, r)
		if !ok {
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			writeJSONError(w, http.StatusInternalServerError, "streaming unsupported")
			return
		}

		// Overrides the application/json set by jsonContentTypeMiddleware.
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		// Subscribe before reading the snapshot, otherwise a result landing between
		// the two would be missed by both.
		updates := statsEvents.subscribe(userID)
		defer statsEvents.unsubscribe(userID, updates)

		// Sent under a separate event name: this is the last known value, not the
		// answer to the request the client is currently waiting on. The client uses
		// it to render a label immediately without treating the stream as finished.
		if snapshot, err := loadUserStats(db, userID); err == nil {
			if !writeSSE(w, flusher, "snapshot", snapshot) {
				return
			}
		} else {
			log.Printf("Error loading stats snapshot for user %d: %v", userID, err)
		}

		heartbeat := time.NewTicker(sseHeartbeatInterval)
		defer heartbeat.Stop()

		for {
			select {
			case <-r.Context().Done():
				return

			case result := <-updates:
				if !writeSSE(w, flusher, "stats", statsFromEvent(result)) {
					return
				}

			case <-heartbeat.C:
				// A comment line is a valid no-op SSE frame.
				if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	}
}

func loadUserStats(db *sql.DB, userID int) (models.UserPostStats, error) {
	row := db.QueryRow(selectStatsSQL, userID)

	stats, err := scanStats(row)
	if err == sql.ErrNoRows {
		// No row yet: report "never" rather than a misleading zero.
		return models.UserPostStats{UserId: userID, Status: statusNever}, nil
	}
	if err != nil {
		return models.UserPostStats{}, err
	}

	return stats, nil
}

// rowScanner lets scanStats work for both a single QueryRow and a Query result set.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanStats(row rowScanner) (models.UserPostStats, error) {
	var stats models.UserPostStats
	var calculatedAt sql.NullTime

	err := row.Scan(
		&stats.UserId,
		&stats.TotalCharacters,
		&stats.PostsCount,
		&stats.Status,
		&stats.RequestId,
		&stats.ErrorMessage,
		&calculatedAt,
	)
	if err != nil {
		return stats, err
	}

	if calculatedAt.Valid {
		stats.CalculatedAt = calculatedAt.Time.UTC().Format(time.RFC3339)
	}

	return stats, nil
}

func statsFromEvent(result events.StatsResult) models.UserPostStats {
	return models.UserPostStats{
		UserId:          result.UserID,
		TotalCharacters: result.TotalCharacters,
		PostsCount:      result.PostsCount,
		Status:          result.Status,
		RequestId:       result.RequestID,
		ErrorMessage:    result.ErrorMessage,
		CalculatedAt:    result.CalculatedAt,
	}
}

// writeSSE emits one event frame. It reports false once the client has gone away,
// which is the signal for the handler to stop.
func writeSSE(w http.ResponseWriter, flusher http.Flusher, event string, payload any) bool {
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error encoding SSE payload: %v", err)
		return false
	}

	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
		return false
	}

	flusher.Flush()
	return true
}

func parseUserID(w http.ResponseWriter, r *http.Request) (int, bool) {
	raw := mux.Vars(r)["id"]

	userID, err := strconv.Atoi(raw)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid user id")
		return 0, false
	}

	return userID, true
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
