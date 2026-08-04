// Package events defines the Kafka message contract shared between the REST API
// (backend_go, which produces requests and consumes results) and stats_service
// (which consumes requests and produces results).
//
// It is its own module so both services compile against one definition of the wire
// format; each imports it through a replace directive pointing at ../events.
package events

// Kafka topic names.
const (
	// TopicStatsRequests carries StatsRequest messages: "please recalculate user N".
	TopicStatsRequests = "post-stats-requests"
	// TopicStatsResults carries StatsResult messages: "user N is calculated".
	TopicStatsResults = "post-stats-results"
)

// Calculation states, mirrored in the user_post_stats.status column.
const (
	StatusPending = "pending"
	StatusDone    = "done"
	StatusFailed  = "failed"
)

// StatsRequest is produced by the REST API when a user presses "Count characters".
// The message key is the user id, so all requests for one user land on the same
// partition and are therefore processed in order.
type StatsRequest struct {
	RequestID   string `json:"request_id"`
	UserID      int    `json:"user_id"`
	RequestedAt string `json:"requested_at"`
}

// StatsResult is produced by stats_service once the calculation is finished (or failed).
// The REST API consumes it and pushes it to the browser over SSE.
type StatsResult struct {
	RequestID       string `json:"request_id"`
	UserID          int    `json:"user_id"`
	TotalCharacters int64  `json:"total_characters"`
	PostsCount      int    `json:"posts_count"`
	Status          string `json:"status"`
	ErrorMessage    string `json:"error_message,omitempty"`
	CalculatedAt    string `json:"calculated_at"`
}
