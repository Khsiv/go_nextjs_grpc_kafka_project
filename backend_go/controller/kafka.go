package controller

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"events"

	"github.com/segmentio/kafka-go"
)

const defaultKafkaBrokers = "localhost:29092"

var (
	statsWriter *kafka.Writer
	statsEvents = newStatsHub()
)

// statsHub fans a single Kafka result message out to every browser currently
// watching that user over SSE. Subscribers are keyed by user id so a result only
// wakes the connections that care about it.
type statsHub struct {
	mu          sync.RWMutex
	subscribers map[int]map[chan events.StatsResult]struct{}
}

func newStatsHub() *statsHub {
	return &statsHub{subscribers: make(map[int]map[chan events.StatsResult]struct{})}
}

func (h *statsHub) subscribe(userID int) chan events.StatsResult {
	// Buffered so publish never blocks on a subscriber that is mid-write.
	ch := make(chan events.StatsResult, 4)

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.subscribers[userID] == nil {
		h.subscribers[userID] = make(map[chan events.StatsResult]struct{})
	}
	h.subscribers[userID][ch] = struct{}{}

	return ch
}

func (h *statsHub) unsubscribe(userID int, ch chan events.StatsResult) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if subs, ok := h.subscribers[userID]; ok {
		delete(subs, ch)
		if len(subs) == 0 {
			delete(h.subscribers, userID)
		}
	}
}

func (h *statsHub) publish(result events.StatsResult) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for ch := range h.subscribers[result.UserID] {
		select {
		case ch <- result:
		default:
			// Subscriber is not keeping up; drop rather than stall the Kafka consumer.
			// The client still converges via the snapshot it gets on reconnect.
			log.Printf("dropping stats event for user %d: subscriber buffer full", result.UserID)
		}
	}
}

// InitKafka opens the producer used by the recalculate endpoint and starts the
// background consumer that turns result events into SSE pushes. The returned
// function closes the producer.
func InitKafka() func() {
	brokers := kafkaBrokers()
	log.Printf("kafka brokers: %v", brokers)

	statsWriter = &kafka.Writer{
		Addr:                   kafka.TCP(brokers...),
		Topic:                  events.TopicStatsRequests,
		Balancer:               &kafka.Hash{},
		AllowAutoTopicCreation: true,
		// Wait for the broker to acknowledge, so a failed publish surfaces as a
		// 500 instead of the user watching a spinner that will never resolve.
		RequiredAcks: kafka.RequireAll,
	}

	go consumeStatsResults(brokers)

	return func() {
		if err := statsWriter.Close(); err != nil {
			log.Printf("closing kafka writer: %v", err)
		}
	}
}

// consumeStatsResults reads finished calculations and hands them to the SSE hub.
//
// The consumer group is unique per process on purpose: every API instance must see
// every result, because it does not know which instance is holding the browser's SSE
// connection. A shared group would give each message to only one instance.
func consumeStatsResults(brokers []string) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: brokers,
		GroupID: fmt.Sprintf("api-sse-%s-%d", hostname, os.Getpid()),
		Topic:   events.TopicStatsResults,
		// Only care about results produced from now on; old ones are already in the DB.
		StartOffset: kafka.LastOffset,
		MaxWait:     500 * time.Millisecond,
	})
	defer reader.Close()

	log.Printf("consuming topic %q for SSE fan-out", events.TopicStatsResults)

	for {
		msg, err := reader.ReadMessage(context.Background())
		if err != nil {
			log.Printf("reading stats result: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}

		var result events.StatsResult
		if err := json.Unmarshal(msg.Value, &result); err != nil {
			log.Printf("skipping malformed stats result at offset %d: %v", msg.Offset, err)
			continue
		}

		log.Printf("stats result for user %d: %d characters (%s)",
			result.UserID, result.TotalCharacters, result.Status)

		statsEvents.publish(result)
	}
}

// publishStatsRequest asks stats_service to recalculate one user. The user id is the
// message key so all requests for a user share a partition and are handled in order.
// The caller supplies requestID because it is written to the database first.
func publishStatsRequest(ctx context.Context, userID int, requestID string) (events.StatsRequest, error) {
	request := events.StatsRequest{
		RequestID:   requestID,
		UserID:      userID,
		RequestedAt: time.Now().UTC().Format(time.RFC3339),
	}

	payload, err := json.Marshal(request)
	if err != nil {
		return request, err
	}

	err = statsWriter.WriteMessages(ctx, kafka.Message{
		Key:   []byte(strconv.Itoa(userID)),
		Value: payload,
	})

	return request, err
}

func newRequestID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		// Fall back to a timestamp; the id is only used for correlation in logs.
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(buf)
}

func kafkaBrokers() []string {
	raw := os.Getenv("KAFKA_BROKERS")
	if raw == "" {
		raw = defaultKafkaBrokers
		log.Println("KAFKA_BROKERS not set, using default local brokers:", raw)
	}

	parts := strings.Split(raw, ",")
	brokers := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			brokers = append(brokers, trimmed)
		}
	}
	return brokers
}
