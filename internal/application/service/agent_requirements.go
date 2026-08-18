package service

import (
	"roche.local/knowledge-agent-platform/internal/agent/tools"
	"roche.local/knowledge-agent-platform/internal/types"
)

// agentRequiresRerankModel reports whether the configured tool set includes
// knowledge_search. The caller separately checks the user's effective
// knowledge scope; agent KB selection no longer controls data access.
func agentRequiresRerankModel(agent *types.CustomAgent) bool {
	if agent == nil {
		return false
	}
	allowed := agent.Config.AllowedTools
	if len(allowed) == 0 {
		allowed = tools.DefaultAllowedTools()
	}
	for _, toolName := range allowed {
		if toolName == tools.ToolKnowledgeSearch {
			return true
		}
	}
	return false
}
