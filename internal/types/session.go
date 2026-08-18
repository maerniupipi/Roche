package types

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// FallbackStrategy represents the fallback strategy type
type FallbackStrategy string

const (
	FallbackStrategyFixed FallbackStrategy = "fixed" // Fixed response
	FallbackStrategyModel FallbackStrategy = "model" // Model fallback response
)

// SummaryConfig represents the summary configuration for a session
type SummaryConfig struct {
	// Max tokens
	MaxTokens int `json:"max_tokens"`
	// Repeat penalty
	RepeatPenalty float64 `json:"repeat_penalty"`
	// TopK
	TopK int `json:"top_k"`
	// TopP
	TopP float64 `json:"top_p"`
	// Frequency penalty
	FrequencyPenalty float64 `json:"frequency_penalty"`
	// Presence penalty
	PresencePenalty float64 `json:"presence_penalty"`
	// Prompt
	Prompt string `json:"prompt"`
	// Context template
	ContextTemplate string `json:"context_template"`
	// No match prefix
	NoMatchPrefix string `json:"no_match_prefix"`
	// Temperature
	Temperature float64 `json:"temperature"`
	// Seed
	Seed int `json:"seed"`
	// Max completion tokens
	MaxCompletionTokens int `json:"max_completion_tokens"`
	// Thinking - whether to enable thinking mode
	Thinking *bool `json:"thinking"`
}

// ContextCompressionStrategy represents the strategy for context compression
type ContextCompressionStrategy string

const (
	// ContextCompressionSlidingWindow keeps the most recent N messages
	ContextCompressionSlidingWindow ContextCompressionStrategy = "sliding_window"
	// ContextCompressionSmart uses LLM to summarize old messages
	ContextCompressionSmart ContextCompressionStrategy = "smart"
)

// ContextConfig configures LLM context management
// This is separate from message storage and manages token limits
type ContextConfig struct {
	// Maximum tokens allowed in LLM context
	MaxTokens int `json:"max_tokens"`
	// Compression strategy: "sliding_window" or "smart"
	CompressionStrategy ContextCompressionStrategy `json:"compression_strategy"`
	// For sliding_window: number of messages to keep
	// For smart: number of recent messages to keep uncompressed
	RecentMessageCount int `json:"recent_message_count"`
	// Summarize threshold: number of messages before summarization
	SummarizeThreshold int `json:"summarize_threshold"`
}

// Session represents the session
type Session struct {
	// ID
	ID string `json:"id"          gorm:"type:varchar(36);primaryKey"`
	// Title
	Title string `json:"title"`
	// Description
	Description string `json:"description"`
	// UserID is the owner scope for this session. Local users and external API
	// principals both use this column.
	UserID string `json:"user_id,omitempty" gorm:"type:varchar(512);index"`
	// IsPinned indicates whether the session is pinned in the list.
	IsPinned bool `json:"is_pinned" gorm:"default:false"`
	// PinnedAt records when the session was pinned; nil when not pinned.
	PinnedAt *time.Time `json:"pinned_at,omitempty"`

	// LastRequestState records the input-bar state used by the most recent
	// question so reopening a conversation can restore the same UI choices.
	LastRequestState *SessionLastRequestState `json:"last_request_state,omitempty" gorm:"column:last_request_state;type:jsonb"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Association relationship, not stored in the database
	Messages []Message `json:"-" gorm:"foreignKey:SessionID"`
}

func (s *Session) BeforeCreate(tx *gorm.DB) (err error) {
	s.ID = uuid.New().String()
	return nil
}

// SessionListQuery bundles the parameters for listing sessions.
// Keyword matches title ILIKE '%keyword%'.
// Source is "web" for browser sessions and may be set by API clients.
type SessionListQuery struct {
	UserID   string
	Keyword  string
	Page     int
	PageSize int
}

// SessionListItem is the paginated session response row.
type SessionListItem struct {
	Session
}

// StringArray represents a list of strings
type StringArray []string

// Value implements the driver.Valuer interface, used to convert StringArray to database value
func (c StringArray) Value() (driver.Value, error) {
	return json.Marshal(c)
}

// Scan implements the sql.Scanner interface, used to convert database value to StringArray
func (c *StringArray) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(b, c)
}

// Value implements the driver.Valuer interface, used to convert SummaryConfig to database value
func (c *SummaryConfig) Value() (driver.Value, error) {
	return json.Marshal(c)
}

// Scan implements the sql.Scanner interface, used to convert database value to SummaryConfig
func (c *SummaryConfig) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(b, c)
}

// SessionLastRequestState captures the user-facing input-bar state at the
// time of the most recent QA request on a session. It is purely a UI memory
// aid — none of the fields here drive backend behaviour. They are echoed back
// to the frontend by GetSession so the chat input can restore the same agent,
// model, KB scope, etc. the user had selected last time.
type SessionLastRequestState struct {
	AgentID          string         `json:"agent_id,omitempty"`
	AgentEnabled     bool           `json:"agent_enabled"`
	ModelID          string         `json:"model_id,omitempty"`
	KnowledgeBaseIDs []string       `json:"knowledge_base_ids,omitempty"`
	KnowledgeIDs     []string       `json:"knowledge_ids,omitempty"`
	TagIDs           []string       `json:"tag_ids,omitempty"`
	MCPServiceIDs    []string       `json:"mcp_service_ids,omitempty"`
	SkillNames       []string       `json:"skill_names,omitempty"`
	MentionedItems   MentionedItems `json:"mentioned_items,omitempty"`
	WebSearchEnabled bool           `json:"web_search_enabled"`
}

// Value implements driver.Valuer for SessionLastRequestState (JSONB).
func (s *SessionLastRequestState) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	return json.Marshal(s)
}

// Scan implements sql.Scanner for SessionLastRequestState (JSONB).
func (s *SessionLastRequestState) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	var b []byte
	switch v := value.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return fmt.Errorf("unsupported SessionLastRequestState value type %T", value)
	}
	if len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, s)
}

// Value implements the driver.Valuer interface, used to convert ContextConfig to database value
func (c *ContextConfig) Value() (driver.Value, error) {
	return json.Marshal(c)
}

// Scan implements the sql.Scanner interface, used to convert database value to ContextConfig
func (c *ContextConfig) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(b, c)
}
