package types

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"time"
)

// DefaultMaxContextTokens is the default context window budget for agent conversations (200k).
const DefaultMaxContextTokens = 200000

// AgentConfig is the runtime configuration for one agent execution.
type AgentConfig struct {
	MaxIterations         int           `json:"max_iterations"`                   // Maximum number of ReAct iterations
	AllowedTools          []string      `json:"allowed_tools"`                    // List of allowed tool names
	Temperature           float64       `json:"temperature"`                      // LLM temperature for agent
	KnowledgeBases        []string      `json:"knowledge_bases"`                  // Effective KB IDs derived from the current user's grants
	KnowledgeIDs          []string      `json:"knowledge_ids"`                    // Effective document IDs derived from the current user's grants
	SystemPrompt          string        `json:"system_prompt,omitempty"`          // Unified system prompt (uses web_search_status placeholder for dynamic behavior)
	UseCustomSystemPrompt bool          `json:"use_custom_system_prompt"`         // Whether to use custom system prompt instead of default
	WebSearchEnabled      bool          `json:"web_search_enabled"`               // Whether web search tool is enabled
	WebSearchMaxResults   int           `json:"web_search_max_results"`           // Maximum number of web search results (default: 5)
	WebSearchProviderID   string        `json:"web_search_provider_id,omitempty"` // WebSearchProviderEntity ID (resolved from agent config)
	MultiTurnEnabled      bool          `json:"multi_turn_enabled"`               // Whether multi-turn conversation is enabled
	HistoryTurns          int           `json:"history_turns"`                    // Number of history turns to keep in context
	SearchTargets         SearchTargets `json:"-"`                                // Pre-computed unified search targets (runtime only)
	// MCP service selection
	MCPSelectionMode string   `json:"mcp_selection_mode"` // MCP selection mode: "all", "selected", "none"
	MCPServices      []string `json:"mcp_services"`       // Selected MCP service IDs (when mode is "selected")
	// MCPAuthWaitTimeout is how many seconds an agent run waits for
	// in-conversation OAuth authorization before skipping. <=0 falls back to
	// the gate's configured timeout. The wait is always bounded (no leak).
	MCPAuthWaitTimeout int `json:"mcp_auth_wait_timeout,omitempty"`
	// Whether to enable thinking mode (for models that support extended thinking)
	Thinking *bool `json:"thinking"`
	// Whether to retain retrieval history across turns (default: false)
	RetainRetrievalHistory bool `json:"retain_retrieval_history"`

	// Skills configuration (Progressive Disclosure pattern)
	SkillsEnabled bool     `json:"skills_enabled"` // Whether skills are enabled (default: false)
	SkillDirs     []string `json:"skill_dirs"`     // Directories to search for skills
	AllowedSkills []string `json:"allowed_skills"` // Skill names whitelist (empty = allow all)

	// Runtime-only fields (not persisted)
	VLMModelID string `json:"-"` // VLM model ID for tool result image analysis (set from CustomAgent config)
	// Per-request @mention pins (runtime only; injected as <must_use> in the user message).
	PinnedMCPServiceIDs []string `json:"-"`
	PinnedSkillNames    []string `json:"-"`
	// LLM call timeout in seconds (default: 120). Controls the maximum time for a single LLM call.
	LLMCallTimeout int `json:"llm_call_timeout,omitempty"`

	// Maximum character length for tool output (default: 16000).
	// Outputs exceeding this limit are truncated with head + tail preservation.
	MaxToolOutputChars int `json:"max_tool_output_chars,omitempty"`

	// Maximum context window tokens for the agent (default: 200000).
	// The agent compresses older messages to stay within this limit,
	// preserving tool_call/tool_result pairs.
	MaxContextTokens int `json:"max_context_tokens,omitempty"`

	// Whether to execute independent tool calls in parallel (default: false).
	// When enabled and the LLM returns multiple tool calls, they run concurrently via errgroup.
	ParallelToolCalls bool `json:"parallel_tool_calls,omitempty"`
}

// Value implements driver.Valuer interface for AgentConfig
func (c AgentConfig) Value() (driver.Value, error) {
	return json.Marshal(c)
}

// Scan implements sql.Scanner interface for AgentConfig
func (c *AgentConfig) Scan(value interface{}) error {
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
		return nil
	}
	return json.Unmarshal(b, c)
}

// Tool defines the interface that all agent tools must implement
type Tool interface {
	// Name returns the unique identifier for this tool
	Name() string

	// Description returns a human-readable description of what the tool does
	Description() string

	// Parameters returns the JSON Schema for the tool's parameters
	Parameters() json.RawMessage

	// Execute runs the tool with the given arguments
	Execute(ctx context.Context, args json.RawMessage) (*ToolResult, error)
}

// Cleanable is an optional interface that tools can implement to release resources.
// Tools implementing this interface will have their Cleanup method called during
// registry cleanup (e.g., at the end of an agent session).
type Cleanable interface {
	Cleanup(ctx context.Context)
}

// ToolResult represents the result of a tool execution
type ToolResult struct {
	Success bool                   `json:"success"`          // Whether the tool executed successfully
	Output  string                 `json:"output"`           // Human-readable output
	Data    map[string]interface{} `json:"data,omitempty"`   // Structured data for programmatic use
	Error   string                 `json:"error,omitempty"`  // Error message if execution failed
	Images  []string               `json:"images,omitempty"` // Base64 data URIs from tool (e.g. MCP image content)
}

// ToolCall represents a single tool invocation within an agent step
type ToolCall struct {
	ID               string                 `json:"id"`                          // Function call ID from LLM
	Name             string                 `json:"name"`                        // Tool name
	Args             map[string]interface{} `json:"args"`                        // Tool arguments
	Result           *ToolResult            `json:"result"`                      // Execution result (contains Output)
	Reflection       string                 `json:"reflection,omitempty"`        // Agent's reflection on this tool call result (if enabled)
	Duration         int64                  `json:"duration"`                    // Execution time in milliseconds
	ProviderMetadata ToolCallMetadata       `json:"provider_metadata,omitempty"` // Provider-specific tool-call state for replay
}

// AgentStep represents one iteration of the ReAct loop
type AgentStep struct {
	Iteration int    `json:"iteration"` // Iteration number (0-indexed)
	Thought   string `json:"thought"`   // LLM's reasoning/thinking (Think phase)
	// Progress fields describe sanitized workflow progress from unified QA.
	// They intentionally contain no model chain-of-thought and live inside the
	// existing agent_steps JSON column, so no schema migration is required.
	RunID                string       `json:"run_id,omitempty"`
	ProgressStepID       string       `json:"progress_step_id,omitempty"`
	ProgressResponseType ResponseType `json:"progress_response_type,omitempty"`
	ProgressStage        string       `json:"progress_stage,omitempty"`
	ProgressAgentID      string       `json:"progress_agent_id,omitempty"`
	ProgressStatus       string       `json:"progress_status,omitempty"`
	ProgressResultCount  int          `json:"progress_result_count,omitempty"`
	ProgressToolCalls    int          `json:"progress_tool_calls,omitempty"`
	ProgressModelCalls   int          `json:"progress_model_calls,omitempty"`
	Duration             int64        `json:"duration,omitempty"`
	// ReasoningContent stores the OpenAI-protocol reasoning_content emitted by the
	// model in this round. Persisted on AgentStep so cross-turn replay can put it
	// back on the assistant message — required by MiMo / DeepSeek V3.2+ thinking
	// mode, ignored by providers that don't recognize the field.
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls"` // Tools called in this step (Act phase)
	Timestamp        time.Time  `json:"timestamp"`  // When this step occurred
}

// GetObservations returns observations from all tool calls in this step
// This is a convenience method to maintain backward compatibility
func (s *AgentStep) GetObservations() []string {
	observations := make([]string, 0, len(s.ToolCalls))
	for _, tc := range s.ToolCalls {
		if tc.Result != nil && tc.Result.Output != "" {
			observations = append(observations, tc.Result.Output)
		}
		if tc.Reflection != "" {
			observations = append(observations, "Reflection: "+tc.Reflection)
		}
	}
	return observations
}

// AgentState tracks the execution state of an agent across iterations
type AgentState struct {
	CurrentRound  int             `json:"current_round"`  // Current round number
	RoundSteps    []AgentStep     `json:"round_steps"`    // All steps taken so far in the current round
	IsComplete    bool            `json:"is_complete"`    // Whether agent has finished
	FinalAnswer   string          `json:"final_answer"`   // The final answer to the query
	KnowledgeRefs []*SearchResult `json:"knowledge_refs"` // Collected knowledge references
}

// FunctionDefinition represents a function definition for LLM function calling
type FunctionDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}
