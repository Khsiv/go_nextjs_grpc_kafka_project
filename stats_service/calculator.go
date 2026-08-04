package main

import (
	"context"
	"database/sql"
	"log"
	"time"

	"events"
)

// countCharactersSQL sums the character length of every post belonging to one user.
// Postgres LENGTH() on a text column counts characters, not bytes, so multi-byte
// content (emoji, Cyrillic, …) is counted the way a reader would count it.
const countCharactersSQL = `
	SELECT COALESCE(SUM(LENGTH(COALESCE(post_text, ''))), 0), COUNT(*)
	FROM posts
	WHERE users_id = $1`

const upsertStatsSQL = `
	INSERT INTO user_post_stats
		(users_id, total_characters, posts_count, status, request_id, error_message, calculated_at)
	VALUES ($1, $2, $3, $4, $5, $6, NOW())
	ON CONFLICT (users_id) DO UPDATE SET
		total_characters = EXCLUDED.total_characters,
		posts_count      = EXCLUDED.posts_count,
		status           = EXCLUDED.status,
		request_id       = EXCLUDED.request_id,
		error_message    = EXCLUDED.error_message,
		calculated_at    = EXCLUDED.calculated_at
	RETURNING calculated_at`

// calculate counts the characters for one user and persists the outcome.
// A failure is persisted too (status=failed) so the frontend can stop spinning and
// show why, rather than waiting forever on a result that will never arrive.
func calculate(ctx context.Context, db *sql.DB, req events.StatsRequest) events.StatsResult {
	result := events.StatsResult{
		RequestID: req.RequestID,
		UserID:    req.UserID,
		Status:    events.StatusDone,
	}

	var totalCharacters int64
	var postsCount int

	err := db.QueryRowContext(ctx, countCharactersSQL, req.UserID).Scan(&totalCharacters, &postsCount)
	if err != nil {
		log.Printf("counting posts for user %d: %v", req.UserID, err)
		result.Status = events.StatusFailed
		result.ErrorMessage = "failed to count posts"
	} else {
		result.TotalCharacters = totalCharacters
		result.PostsCount = postsCount
	}

	var calculatedAt time.Time
	err = db.QueryRowContext(ctx, upsertStatsSQL,
		req.UserID,
		result.TotalCharacters,
		result.PostsCount,
		result.Status,
		req.RequestID,
		nullableString(result.ErrorMessage),
	).Scan(&calculatedAt)
	if err != nil {
		// Most likely cause: the user was deleted between pressing the button and
		// this message being consumed, so the foreign key rejects the row.
		log.Printf("storing stats for user %d: %v", req.UserID, err)
		result.Status = events.StatusFailed
		result.ErrorMessage = "failed to store stats"
		calculatedAt = time.Now()
	}

	result.CalculatedAt = calculatedAt.UTC().Format(time.RFC3339)
	return result
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
