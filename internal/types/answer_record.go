package types

import "time"

// AdminAnswerRecordQuery is the system-admin filter set for the flattened
// question/answer list. Feedback accepts like, dislike, none or empty (all).
type AdminAnswerRecordQuery struct {
	Channel    string
	Username   string
	Feedback   string
	IsFallback *bool
	StartTime  *time.Time
	EndTime    *time.Time
	Page       int
	PageSize   int
}

// AdminAnswerRecord is one user question paired with its assistant response.
// KnowledgeBases intentionally contains names only, as required by the admin UI.
type AdminAnswerRecord struct {
	ID               string           `json:"id"`
	SessionID        string           `json:"session_id"`
	RequestID        string           `json:"request_id"`
	Channel          string           `json:"channel"`
	UserID           string           `json:"user_id"`
	Username         string           `json:"username"`
	SessionTitle     string           `json:"session_title"`
	Question         string           `json:"question"`
	Answer           string           `json:"answer"`
	KnowledgeBases   []string         `json:"knowledge_bases"`
	Feedback         *MessageFeedback `json:"feedback"`
	QuestionedAt     time.Time        `json:"questioned_at"`
	AnswerFinishedAt time.Time        `json:"answer_finished_at"`
}
