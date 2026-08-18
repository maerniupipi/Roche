package types

import "time"

// DashboardKnowledgeBaseStats holds document lifecycle counters for a knowledge base.
type DashboardKnowledgeBaseStats struct {
	PublishedCount        int64 `json:"published_count"`
	UploadSuccessCount    int64 `json:"upload_success_count"`
	UploadFailedCount     int64 `json:"upload_failed_count"`
	ScheduledPublishCount int64 `json:"scheduled_publish_count"`
	UnpublishedCount      int64 `json:"unpublished_count"`
	ArchivedCount         int64 `json:"archived_count"`
}

// DashboardDailyChatStats is a single day's conversation metrics.
type DashboardDailyChatStats struct {
	Date            string  `json:"date"`
	QuestionCount   int64   `json:"question_count"`
	UniqueUsers     int64   `json:"unique_users"`
	SatisfactionPct float64 `json:"satisfaction_pct"`
}

// DashboardChatStats groups time-range chat metrics.
type DashboardChatStats struct {
	AvgFirstResponseSec float64                   `json:"avg_first_response_sec"`
	AvgCompleteSec      float64                   `json:"avg_complete_sec"`
	Daily               []DashboardDailyChatStats `json:"daily"`
}

// DashboardDomainDistributionItem is one slice of the domain pie chart.
type DashboardDomainDistributionItem struct {
	Name  string `json:"name"`
	Value int64  `json:"value"`
}

// DashboardTopDocumentItem is a row in the hot documents list.
type DashboardTopDocumentItem struct {
	Rank     int    `json:"rank"`
	Title    string `json:"title"`
	KbName   string `json:"kb_name"`
	HitCount int64  `json:"hit_count"`
}

// DashboardFeedbackItem is a row in product feedback chart.
type DashboardFeedbackItem struct {
	Category string `json:"category"`
	Count    int64  `json:"count"`
}

// DashboardTopUserItem is a row in the top askers list.
type DashboardTopUserItem struct {
	Rank        int    `json:"rank"`
	UserName    string `json:"user_name"`
	QuestionCnt int64  `json:"question_count"`
}

// DashboardFallbackQuestionItem is a row in fallback utterance list.
type DashboardFallbackQuestionItem struct {
	Rank    int    `json:"rank"`
	Content string `json:"content"`
}

// DashboardOverview holds aggregated cross-domain analytics.
type DashboardOverview struct {
	DomainDistribution  []DashboardDomainDistributionItem `json:"domain_distribution"`
	CrossDomainSingle   int64                             `json:"cross_domain_single"`
	CrossDomainMulti    int64                             `json:"cross_domain_multi"`
	TopDocuments        []DashboardTopDocumentItem        `json:"top_documents"`
	ProductFeedback     []DashboardFeedbackItem           `json:"product_feedback"`
	TopUsers            []DashboardTopUserItem            `json:"top_users"`
	ValidAnswerCount    int64                             `json:"valid_answer_count"`
	FallbackAnswerCount int64                             `json:"fallback_answer_count"`
	FallbackQuestions   []DashboardFallbackQuestionItem   `json:"fallback_questions"`
}

// DashboardFeedbackRecord is the persisted user feedback record.
type DashboardFeedbackRecord struct {
	ID                uint64    `json:"id" gorm:"primaryKey"`
	KnowledgeDomainID uint64    `json:"knowledge_domain_id"`
	SessionID         string    `json:"session_id"`
	MessageID         string    `json:"message_id"`
	Category          string    `json:"category"`
	Comment           string    `json:"comment"`
	Satisfaction      int       `json:"satisfaction"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// DashboardDailyStat is one row of the pre-aggregated dashboard table.
//
// The scheduled job (DashboardStatsService) computes yesterday's numbers into
// this table once a day; the three dashboard endpoints read from it instead of
// scanning the raw messages/sessions/knowledges tables per request.
//
// Row granularity is (stat_date, knowledge_domain_id, knowledge_base_id):
//   - (d, 0, '')          global row: all knowledge domains + all knowledge bases
//   - (d, domain_id, '')  knowledge-domain scoped row
//   - (d, 0, kb_id)       knowledge-base row (document counters only)
//
// List-type overview metrics are stored as per-day detail JSON arrays so the
// endpoints can merge them across a date range.
type DashboardDailyStat struct {
	ID uint64 `json:"id" gorm:"primaryKey"`
	// StatDate is the UTC date this row aggregates (YYYY-MM-DD).
	StatDate time.Time `json:"stat_date" gorm:"type:date;not null"`
	// KnowledgeDomainID is 0 for the global row.
	KnowledgeDomainID uint64 `json:"knowledge_domain_id" gorm:"not null;default:0"`
	// KnowledgeBaseID is "" for global/domain rows.
	KnowledgeBaseID string `json:"knowledge_base_id" gorm:"type:varchar(36);not null;default:''"`

	// Knowledge base document counters (knowledge-base-stats endpoint).
	PublishedCount        int64 `json:"published_count"`
	UploadSuccessCount    int64 `json:"upload_success_count"`
	UploadFailedCount     int64 `json:"upload_failed_count"`
	ScheduledPublishCount int64 `json:"scheduled_publish_count"`
	UnpublishedCount      int64 `json:"unpublished_count"`
	ArchivedCount         int64 `json:"archived_count"`

	// Chat stats (chat-stats endpoint).
	QuestionCount        int64   `json:"question_count"`
	UniqueUsers          int64   `json:"unique_users"`
	SatisfactionPct      float64 `json:"satisfaction_pct"`
	AnswerCount          int64   `json:"answer_count"`
	TotalAgentDurationMs int64   `json:"total_agent_duration_ms"`

	// Overview scalar metrics (overview endpoint).
	ValidAnswerCount        int64 `json:"valid_answer_count"`
	FallbackAnswerCount     int64 `json:"fallback_answer_count"`
	CrossDomainSingleCount  int64 `json:"cross_domain_single_count"`
	CrossDomainMultiCount   int64 `json:"cross_domain_multi_count"`

	// Overview list metrics, stored as per-day detail arrays (overview endpoint).
	DomainDistribution JSON `json:"domain_distribution"`
	TopDocuments       JSON `json:"top_documents"`
	ProductFeedback    JSON `json:"product_feedback"`
	TopUsers           JSON `json:"top_users"`
	FallbackQuestions  JSON `json:"fallback_questions"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (DashboardDailyStat) TableName() string { return "dashboard_daily_stats" }

// JSON detail payloads stored in the DashboardDailyStat list columns.
type DashboardDomainDistributionDetail struct {
	KnowledgeDomainID uint64 `json:"knowledge_domain_id"`
	Name              string `json:"name"`
	Value             int64  `json:"value"`
}

type DashboardTopDocumentDetail struct {
	KnowledgeID string `json:"knowledge_id"`
	Title       string `json:"title"`
	KbName      string `json:"kb_name"`
	HitCount    int64  `json:"hit_count"`
}

type DashboardTopUserDetail struct {
	UserName      string `json:"user_name"`
	QuestionCount int64  `json:"question_count"`
}

type DashboardFallbackQuestionDetail struct {
	Content string `json:"content"`
	Count   int64  `json:"count"`
}

// DashboardKnowledgeBaseFilter is the query parameter for KB stats.
type DashboardKnowledgeBaseFilter struct {
	KnowledgeBaseID string `json:"knowledge_base_id" form:"knowledge_base_id"`
}

// DashboardDateRangeFilter is the date range query parameter.
type DashboardDateRangeFilter struct {
	KnowledgeDomainID uint64 `json:"knowledge_domain_id" form:"knowledge_domain_id"`
	StartDate         string `json:"start_date" form:"start_date"`
	EndDate           string `json:"end_date" form:"end_date"`
}
