// Package interfaces defines the interface contracts for custom agent management
package interfaces

import (
	"context"

	"roche.local/knowledge-agent-platform/internal/types"
)

// CustomAgentService defines the custom agent service interface
// Provides high-level operations for agent creation, querying, updating, and deletion
type CustomAgentService interface {
	// CreateAgent creates a new custom agent
	// Parameters:
	//   - ctx: Context information, carrying request tracking, user identity, etc.
	//   - agent: Agent object containing basic information and configuration
	// Returns:
	//   - Created agent object (including automatically generated ID)
	//   - Possible errors such as insufficient permissions, validation errors, etc.
	CreateAgent(ctx context.Context, agent *types.CustomAgent) (*types.CustomAgent, error)

	// GetAgentByID retrieves agent information by ID (uses knowledgeDomain from context)
	// Parameters:
	//   - ctx: Context information
	//   - id: Unique identifier of the agent
	// Returns:
	//   - Agent object, if found (including built-in agents)
	//   - Possible errors such as not existing, insufficient permissions, etc.
	GetAgentByID(ctx context.Context, id string) (*types.CustomAgent, error)

	// ListAgents lists platform agents visible to the caller, including built-in agents.
	// Parameters:
	//   - ctx: Context information, containing knowledgeDomain information
	// Returns:
	//   - List of agent objects (built-in agents first, then custom agents sorted by creation time)
	//   - Possible errors such as insufficient permissions, etc.
	ListAgents(ctx context.Context) ([]*types.CustomAgent, error)

	// UpdateAgent updates agent information
	// Parameters:
	//   - ctx: Context information
	//   - agent: Agent object containing update information
	// Returns:
	//   - Updated agent object
	//   - Possible errors such as not existing, insufficient permissions, cannot modify built-in, etc.
	UpdateAgent(ctx context.Context, agent *types.CustomAgent) (*types.CustomAgent, error)

	// DeleteAgent deletes an agent
	// Parameters:
	//   - ctx: Context information
	//   - id: Unique identifier of the agent
	// Returns:
	//   - Possible errors such as not existing, insufficient permissions, cannot delete built-in, etc.
	DeleteAgent(ctx context.Context, id string) error

	// CopyAgent creates a copy of an existing agent
	// Parameters:
	//   - ctx: Context information
	//   - id: Unique identifier of the agent to copy
	// Returns:
	//   - The newly created agent copy
	//   - Possible errors such as not existing, insufficient permissions, etc.
	CopyAgent(ctx context.Context, id string) (*types.CustomAgent, error)

	// GetSuggestedQuestions returns suggested questions from the current user's
	// effective knowledge grants. Optional request IDs may narrow that scope.
	// Parameters:
	//   - ctx: Context information
	//   - agentID: Agent ID
	//   - kbIDs: Optional knowledge base IDs to narrow the request
	//   - knowledgeIDs: Optional knowledge item IDs to further filter
	//   - tagIDs: Optional knowledge tag IDs; resolved to knowledge item IDs (OR semantics)
	//   - limit: Maximum number of questions to return
	// Returns:
	//   - List of suggested questions
	//   - Possible errors
	GetSuggestedQuestions(ctx context.Context, agentID string, kbIDs []string, knowledgeIDs []string, tagIDs []string, limit int) ([]types.SuggestedQuestion, error)
}

// CustomAgentRepository defines the custom agent repository interface
// Responsible for agent data persistence and retrieval
type CustomAgentRepository interface {
	// CreateAgent creates an agent record
	// Parameters:
	//   - ctx: Context information
	//   - agent: Agent object
	// Returns:
	//   - Possible errors such as database connection failure, unique constraint conflicts, etc.
	CreateAgent(ctx context.Context, agent *types.CustomAgent) error

	// GetAgentByID queries an agent by ID.
	// Parameters:
	//   - ctx: Context information
	//   - id: Agent ID
	// Returns:
	//   - Agent object, if found
	//   - Possible errors such as record not existing, database errors, etc.
	GetAgentByID(ctx context.Context, id string) (*types.CustomAgent, error)

	// ListAgents lists all platform agents.
	// Parameters:
	//   - ctx: Context information
	// Returns:
	//   - List of agent objects
	//   - Possible errors such as database errors, etc.
	ListAgents(ctx context.Context) ([]*types.CustomAgent, error)

	// UpdateAgent updates an agent record
	// Parameters:
	//   - ctx: Context information
	//   - agent: Agent object containing update information
	// Returns:
	//   - Possible errors such as record not existing, database errors, etc.
	UpdateAgent(ctx context.Context, agent *types.CustomAgent) error

	// DeleteAgent deletes an agent record
	// Parameters:
	//   - ctx: Context information
	//   - id: Agent ID
	// Returns:
	//   - Possible errors such as record not existing, database errors, etc.
	DeleteAgent(ctx context.Context, id string) error

	// CountByModelID counts active agents whose config references
	// the given model ID (chat, rerank, VLM, ASR, query-understand, etc.).
	CountByModelID(ctx context.Context, modelID string) (int64, error)
}
