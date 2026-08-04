package models

// UserPostStats is the JSON shape returned by the stats endpoints and pushed over SSE.
// It mirrors a row of the user_post_stats table.
type UserPostStats struct {
	UserId          int    `json:"user_id"`
	TotalCharacters int64  `json:"total_characters"`
	PostsCount      int    `json:"posts_count"`
	// pending | done | failed | never (never = no calculation has been requested yet)
	Status       string `json:"status"`
	RequestId    string `json:"request_id,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
	CalculatedAt string `json:"calculated_at,omitempty"`
}
