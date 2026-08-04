// stats_service consumes "recalculate this user" requests from Kafka, counts the
// characters across all of that user's posts, stores the result in user_post_stats,
// and publishes a result event that the REST API turns into an SSE push.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"events"

	_ "github.com/lib/pq"
	"github.com/segmentio/kafka-go"
)

const (
	defaultDatabaseURL = "postgres://postgres:postgres@localhost:5433/postgres?sslmode=disable"
	defaultBrokers     = "localhost:29092"
	defaultGroupID     = "stats-service"
)

func main() {
	log.SetPrefix("[stats_service] ")

	brokers := splitBrokers(envOrDefault("KAFKA_BROKERS", defaultBrokers))
	groupID := envOrDefault("STATS_CONSUMER_GROUP", defaultGroupID)

	// Optional artificial delay, off by default. Set it to e.g. 2000 to make the
	// "Calculating..." state on the frontend visible while demoing the async flow.
	calcDelay := time.Duration(envInt("STATS_CALC_DELAY_MS", 0)) * time.Millisecond

	db, err := openDatabase()
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	// Root context cancelled on SIGINT/SIGTERM so the reader/writer close cleanly
	// and the consumer group offset is committed before we exit.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := ensureTopics(ctx, brokers); err != nil {
		log.Fatalf("kafka topics: %v", err)
	}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: brokers,
		GroupID: groupID,
		Topic:   events.TopicStatsRequests,
		// Offsets are committed manually after the row is written, so a crash
		// mid-calculation replays the request instead of losing it.
		MaxWait: 500 * time.Millisecond,
	})
	defer reader.Close()

	writer := &kafka.Writer{
		Addr:                   kafka.TCP(brokers...),
		Topic:                  events.TopicStatsResults,
		Balancer:               &kafka.Hash{},
		AllowAutoTopicCreation: true,
	}
	defer writer.Close()

	log.Printf("listening on topic %q (brokers=%v, group=%q)", events.TopicStatsRequests, brokers, groupID)

	for {
		msg, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				log.Println("shutting down")
				return
			}
			log.Printf("fetch message: %v", err)
			continue
		}

		handleMessage(ctx, db, writer, msg, calcDelay)

		if err := reader.CommitMessages(ctx, msg); err != nil && ctx.Err() == nil {
			log.Printf("commit offset: %v", err)
		}
	}
}

// handleMessage runs one calculation and publishes its outcome. It never returns an
// error: a bad message must not stall the partition, so failures are recorded on the
// row (status=failed) and reported to the frontend instead of being retried forever.
func handleMessage(ctx context.Context, db *sql.DB, writer *kafka.Writer, msg kafka.Message, calcDelay time.Duration) {
	var req events.StatsRequest
	if err := json.Unmarshal(msg.Value, &req); err != nil {
		log.Printf("skipping malformed message at offset %d: %v", msg.Offset, err)
		return
	}

	log.Printf("calculating stats for user %d (request %s)", req.UserID, req.RequestID)

	if calcDelay > 0 {
		select {
		case <-time.After(calcDelay):
		case <-ctx.Done():
			return
		}
	}

	result := calculate(ctx, db, req)

	if err := publishResult(ctx, writer, result); err != nil {
		// The row is already written, so the data is not lost — only the live SSE
		// push is. The frontend's snapshot-on-connect will still show the result.
		log.Printf("publish result for user %d: %v", req.UserID, err)
		return
	}

	log.Printf("user %d: %d characters across %d posts (%s)",
		result.UserID, result.TotalCharacters, result.PostsCount, result.Status)
}

func publishResult(ctx context.Context, writer *kafka.Writer, result events.StatsResult) error {
	payload, err := json.Marshal(result)
	if err != nil {
		return err
	}

	return writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(strconv.Itoa(result.UserID)),
		Value: payload,
	})
}

func openDatabase() (*sql.DB, error) {
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		connStr = defaultDatabaseURL
		log.Println("DATABASE_URL not set, using default local connection:", connStr)
	}

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

// ensureTopics creates the request/result topics up front so neither service races
// against auto-creation. Kafka usually needs a few seconds after the container starts
// before it accepts admin requests, hence the retry loop.
func ensureTopics(ctx context.Context, brokers []string) error {
	client := &kafka.Client{Addr: kafka.TCP(brokers...), Timeout: 10 * time.Second}

	topics := []kafka.TopicConfig{
		{Topic: events.TopicStatsRequests, NumPartitions: 3, ReplicationFactor: 1},
		{Topic: events.TopicStatsResults, NumPartitions: 3, ReplicationFactor: 1},
	}

	var lastErr error
	for attempt := 1; attempt <= 15; attempt++ {
		resp, err := client.CreateTopics(ctx, &kafka.CreateTopicsRequest{Topics: topics})
		if err == nil {
			for topic, topicErr := range resp.Errors {
				if topicErr != nil && !errors.Is(topicErr, kafka.TopicAlreadyExists) {
					return topicErr
				}
				log.Printf("topic ready: %s", topic)
			}
			return nil
		}

		lastErr = err
		log.Printf("kafka not ready yet (attempt %d/15): %v", attempt, err)

		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return lastErr
}

func splitBrokers(raw string) []string {
	parts := strings.Split(raw, ",")
	brokers := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			brokers = append(brokers, trimmed)
		}
	}
	return brokers
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(key))
	if err != nil {
		return fallback
	}
	return value
}
