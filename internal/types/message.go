// Package types defines data structures and types used throughout the system
package types

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// History represents a conversation history entry
// Contains query-answer pairs and associated knowledge references
// Used for tracking conversation context and history
type History struct {
	Query               string     // User query text
	Answer              string     // System response text
	CreateAt            time.Time  // When this history entry was created
	KnowledgeReferences References // Knowledge references used in the answer
}

// MentionedItem represents a mentioned knowledge base or file
type MentionedItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`       // "kb", "file", "tag", "mcp", "skill"
	KBType    string `json:"kb_type"`    // "document" or "faq" (only for kb type)
	KBID      string `json:"kb_id"`      // Parent knowledge base for file/tag mentions
	KBName    string `json:"kb_name"`    // Display name for parent KB
	ServiceID string `json:"service_id"` // Parent MCP service for MCP tool mentions
	SkillName string `json:"skill_name"` // Preloaded agent skill name
}

// MessageImage represents an image attached to a chat message
type MessageImage struct {
	URL     string `json:"url"`
	Caption string `json:"caption,omitempty"`
}

// MessageImages is a slice of MessageImage for database storage
type MessageImages []MessageImage

// Value implements the driver.Valuer interface for database serialization
func (m MessageImages) Value() (driver.Value, error) {
	if m == nil {
		return json.Marshal([]MessageImage{})
	}
	return json.Marshal(m)
}

// Scan implements the sql.Scanner interface for database deserialization
func (m *MessageImages) Scan(value interface{}) error {
	if value == nil {
		*m = make(MessageImages, 0)
		return nil
	}
	var b []byte
	switch v := value.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		*m = make(MessageImages, 0)
		return nil
	}
	return json.Unmarshal(b, m)
}

// MessageAttachment represents a file attachment in a chat message
type MessageAttachment struct {
	URL         string `json:"url"`                    // Storage URL (provider://path)
	FileName    string `json:"file_name"`              // Original filename
	FileType    string `json:"file_type"`              // File extension (e.g., ".pdf", ".docx")
	FileSize    int64  `json:"file_size"`              // File size in bytes
	Content     string `json:"content,omitempty"`      // Extracted text content (for small text files)
	IsTruncated bool   `json:"is_truncated,omitempty"` // Whether content was truncated
	LineCount   int    `json:"line_count,omitempty"`   // Total line count (for text files)
}

// MessageAttachments is a slice of MessageAttachment for database storage
type MessageAttachments []MessageAttachment

// BuildPrompt returns a formatted prompt section for all attachments,
// injecting file metadata and extracted content into the LLM context.
func (attachments MessageAttachments) BuildPrompt() string {
	if len(attachments) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n\n<attachments>\n")

	for i, att := range attachments {
		sb.WriteString(fmt.Sprintf("<attachment index=\"%d\" name=\"%s\">\n", i+1, att.FileName))
		sb.WriteString("<metadata>\n")
		sb.WriteString(fmt.Sprintf("<type>%s</type>\n", att.FileType))
		sb.WriteString(fmt.Sprintf("<size_kb>%.2f</size_kb>\n", float64(att.FileSize)/1024))
		sb.WriteString("</metadata>\n")

		if att.Content != "" {
			sb.WriteString("<content>\n")
			sb.WriteString(att.Content)
			sb.WriteString("\n</content>\n")

			if att.IsTruncated {
				sb.WriteString(fmt.Sprintf("<note>This file has a total of %d lines, truncated to show only the first 500 lines.</note>\n",
					att.LineCount))
			}
		} else {
			sb.WriteString("<note>File content extraction failed or is unsupported.</note>\n")
		}
		sb.WriteString("</attachment>\n")
	}
	sb.WriteString("</attachments>\n\n")

	return sb.String()
}

// Value implements the driver.Valuer interface for database serialization
func (m MessageAttachments) Value() (driver.Value, error) {
	if m == nil {
		return json.Marshal([]MessageAttachment{})
	}
	return json.Marshal(m)
}

// Scan implements the sql.Scanner interface for database deserialization
func (m *MessageAttachments) Scan(value interface{}) error {
	if value == nil {
		*m = make(MessageAttachments, 0)
		return nil
	}
	var b []byte
	switch v := value.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		*m = make(MessageAttachments, 0)
		return nil
	}
	return json.Unmarshal(b, m)
}

// MentionedItems is a slice of MentionedItem for database storage
type MentionedItems []MentionedItem

// Value implements the driver.Valuer interface for database serialization
func (m MentionedItems) Value() (driver.Value, error) {
	if m == nil {
		return json.Marshal([]MentionedItem{})
	}
	return json.Marshal(m)
}

// Scan implements the sql.Scanner interface for database deserialization
func (m *MentionedItems) Scan(value interface{}) error {
	if value == nil {
		*m = make(MentionedItems, 0)
		return nil
	}
	var b []byte
	switch v := value.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		*m = make(MentionedItems, 0)
		return nil
	}
	return json.Unmarshal(b, m)
}

// FeedbackRating is the user's assessment of an assistant message.
type FeedbackRating string

const (
	FeedbackRatingLike    FeedbackRating = "like"
	FeedbackRatingDislike FeedbackRating = "dislike"
)

// FeedbackReason is the stable code submitted for a dislike. The code is
// persisted; localized labels are API-only fields derived from this catalog.
type FeedbackReason string

const (
	FeedbackReasonFactualError   FeedbackReason = "factual_error"
	FeedbackReasonLogicConfusion FeedbackReason = "logic_confusion"
	FeedbackReasonOutdated       FeedbackReason = "outdated"
	FeedbackReasonFormatError    FeedbackReason = "format_error"
	FeedbackReasonTooLong        FeedbackReason = "too_long"
	FeedbackReasonRepetitive     FeedbackReason = "repetitive"
	FeedbackReasonOther          FeedbackReason = "other"
)

// FeedbackReasonLabels contains the two labels returned to clients for a
// stable dislike reason code.
type FeedbackReasonLabels struct {
	Zh string
	En string
}

var feedbackReasonCatalog = map[FeedbackReason]FeedbackReasonLabels{
	FeedbackReasonFactualError:   {Zh: "事实性错误", En: "Factual error"},
	FeedbackReasonLogicConfusion: {Zh: "逻辑混乱", En: "Confused logic"},
	FeedbackReasonOutdated:       {Zh: "时效性差", En: "Outdated information"},
	FeedbackReasonFormatError:    {Zh: "格式错误", En: "Format error"},
	FeedbackReasonTooLong:        {Zh: "回复过长", En: "Response too long"},
	FeedbackReasonRepetitive:     {Zh: "内容重复", En: "Repetitive content"},
	FeedbackReasonOther:          {Zh: "其他", En: "Other"},
}

// LookupFeedbackReason returns localized labels for a dislike reason code.
func LookupFeedbackReason(reason string) (FeedbackReasonLabels, bool) {
	labels, ok := feedbackReasonCatalog[FeedbackReason(reason)]
	return labels, ok
}

// MessageFeedback stores the current feedback for one assistant message.
// A message has at most one feedback row; updating the rating overwrites the
// previous choice while preserving the original creation time.
type MessageFeedback struct {
	ID        string         `json:"id" gorm:"type:varchar(36);primaryKey"`
	MessageID string         `json:"message_id" gorm:"type:varchar(36);uniqueIndex"`
	SessionID string         `json:"session_id" gorm:"type:varchar(36);index"`
	UserID    string         `json:"-" gorm:"type:varchar(512);index"`
	Rating    FeedbackRating `json:"rating" gorm:"type:varchar(16);not null"`
	Reason    string         `json:"reason,omitempty" gorm:"type:varchar(32);not null;default:''"`
	ReasonZh  string         `json:"reason_zh,omitempty" gorm:"-"`
	ReasonEn  string         `json:"reason_en,omitempty" gorm:"-"`
	Comment   string         `json:"comment,omitempty" gorm:"type:text;not null;default:''"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// BeforeCreate assigns an ID when feedback is inserted through GORM.
func (f *MessageFeedback) BeforeCreate(tx *gorm.DB) error {
	if f.ID == "" {
		f.ID = uuid.NewString()
	}
	return nil
}

// EnrichReasonLabels derives localized API labels without persisting duplicate
// text in the database. Unknown/empty legacy reason codes return no labels.
func (f *MessageFeedback) EnrichReasonLabels() {
	if f == nil {
		return
	}
	labels, ok := LookupFeedbackReason(f.Reason)
	if !ok {
		f.ReasonZh = ""
		f.ReasonEn = ""
		return
	}
	f.ReasonZh = labels.Zh
	f.ReasonEn = labels.En
}

// AfterFind ensures feedback loaded directly or through Message.Feedback has
// the localized labels required by API history responses.
func (f *MessageFeedback) AfterFind(tx *gorm.DB) error {
	f.EnrichReasonLabels()
	return nil
}

// Message represents a conversation message
// Each message belongs to a conversation session and can be from either user or system
// Messages can contain references to knowledge chunks used to generate responses
type Message struct {
	// Unique identifier for the message
	ID string `json:"id"                    gorm:"type:varchar(36);primaryKey"`
	// ID of the session this message belongs to
	SessionID string `json:"session_id"`
	// Request identifier for tracking API requests
	RequestID string `json:"request_id"`
	// Message text content
	Content string `json:"content"`
	// Message role: "user", "assistant", "system"
	Role string `json:"role"`
	// References to knowledge chunks used in the response
	KnowledgeReferences References `json:"knowledge_references"  gorm:"type:json,column:knowledge_references"`
	// Agent execution steps (only for assistant messages generated by agent)
	// This contains the detailed reasoning process and tool calls made by the agent
	// Stored for user history display, but NOT included in LLM context to avoid redundancy
	AgentSteps AgentSteps `json:"agent_steps,omitempty" gorm:"type:jsonb,column:agent_steps"`
	// Mentioned knowledge bases and files (for user messages)
	// Stores the @mentioned items when user sends a message
	MentionedItems MentionedItems `json:"mentioned_items,omitempty" gorm:"type:jsonb,column:mentioned_items"`
	// Attached images with OCR/Caption text (for user messages)
	Images MessageImages `json:"images,omitempty" gorm:"type:jsonb;column:images"`
	// Attached files (documents, audio, etc., for user messages)
	Attachments MessageAttachments `json:"attachments,omitempty" gorm:"type:jsonb;column:attachments"`
	// Whether message generation is complete
	IsCompleted bool `json:"is_completed"`
	// Whether this response is a fallback (no knowledge base match found)
	IsFallback bool `json:"is_fallback,omitempty"`
	// Agent total execution duration in milliseconds (from query start to answer start)
	AgentDurationMs int64 `json:"agent_duration_ms,omitempty" gorm:"column:agent_duration_ms;default:0"`
	// Feedback is loaded for assistant messages so the UI can restore the
	// like/dislike state after the conversation is reopened.
	Feedback *MessageFeedback `json:"feedback,omitempty" gorm:"foreignKey:MessageID;references:ID"`
	// RenderedContent stores the full RAG-augmented user message (with retrieved context)
	// sent to the LLM. Used to preserve retrieval context across conversation turns.
	// Empty for non-retrieval intents or assistant messages.
	RenderedContent string `json:"-" gorm:"type:text;column:rendered_content;default:''"`
	// Channel indicates the client channel of this message ("web" or "app").
	Channel string `json:"channel,omitempty" gorm:"type:varchar(50);default:''"`
	// Message creation timestamp
	CreatedAt time.Time `json:"created_at"`
	// Last update timestamp
	UpdatedAt time.Time `json:"updated_at"`
	// Soft delete timestamp
	DeletedAt gorm.DeletedAt `json:"deleted_at"            gorm:"index"`
}

// AgentSteps represents a collection of agent execution steps
// Used for storing agent reasoning process in database
type AgentSteps []AgentStep

// Value implements the driver.Valuer interface for database serialization
func (a AgentSteps) Value() (driver.Value, error) {
	if a == nil {
		return json.Marshal([]AgentStep{})
	}
	return json.Marshal(a)
}

// Scan implements the sql.Scanner interface for database deserialization
func (a *AgentSteps) Scan(value interface{}) error {
	if value == nil {
		*a = make(AgentSteps, 0)
		return nil
	}
	var b []byte
	switch v := value.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		*a = make(AgentSteps, 0)
		return nil
	}
	return json.Unmarshal(b, a)
}

// BeforeCreate is a GORM hook that runs before creating a new message record
// Automatically generates a UUID for new messages and initializes knowledge references
// Parameters:
//   - tx: GORM database transaction
//
// Returns:
//   - error: Any error encountered during the hook execution
func (m *Message) BeforeCreate(tx *gorm.DB) (err error) {
	m.ID = uuid.New().String()
	if m.KnowledgeReferences == nil {
		m.KnowledgeReferences = make(References, 0)
	}
	if m.AgentSteps == nil {
		m.AgentSteps = make(AgentSteps, 0)
	}
	if m.MentionedItems == nil {
		m.MentionedItems = make(MentionedItems, 0)
	}
	if m.Images == nil {
		m.Images = make(MessageImages, 0)
	}
	if m.Attachments == nil {
		m.Attachments = make(MessageAttachments, 0)
	}
	return nil
}
