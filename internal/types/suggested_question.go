package types

import "time"

type SuggestedQuestionAnswerMode string

const (
	SuggestedQuestionAnswerGenerated SuggestedQuestionAnswerMode = "generated"
	SuggestedQuestionAnswerCustom    SuggestedQuestionAnswerMode = "custom"
)

// HomepageSuggestedQuestion is one of the three global questions configured for the
// empty chat page. It is deliberately independent of knowledge domains, KBs,
// documents, FAQ entries and user knowledge permissions.
type HomepageSuggestedQuestion struct {
	ID           string                      `json:"id" gorm:"type:varchar(36);primaryKey"`
	Question     string                      `json:"question" gorm:"type:text;not null"`
	AnswerMode   SuggestedQuestionAnswerMode `json:"answer_mode" gorm:"type:varchar(16);not null"`
	CustomAnswer string                      `json:"custom_answer" gorm:"type:text;not null;default:''"`
	SortOrder    int                         `json:"sort_order" gorm:"not null;uniqueIndex"`
	CreatedAt    time.Time                   `json:"created_at"`
	UpdatedAt    time.Time                   `json:"updated_at"`
}

func (HomepageSuggestedQuestion) TableName() string { return "suggested_questions" }

// SuggestedQuestionConfigItem is one row in the full three-question
// replacement payload. ID is optional when a question is first created.
type SuggestedQuestionConfigItem struct {
	ID           string                      `json:"id,omitempty"`
	Question     string                      `json:"question"`
	AnswerMode   SuggestedQuestionAnswerMode `json:"answer_mode"`
	CustomAnswer string                      `json:"custom_answer,omitempty"`
	SortOrder    int                         `json:"sort_order"`
}
