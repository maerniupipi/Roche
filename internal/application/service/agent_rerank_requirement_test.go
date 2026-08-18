package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"roche.local/knowledge-agent-platform/internal/agent/tools"
	"roche.local/knowledge-agent-platform/internal/types"
)

func TestAgentRequiresRerankModel(t *testing.T) {
	tests := []struct {
		name  string
		agent *types.CustomAgent
		want  bool
	}{
		{
			name: "knowledge search requires reranker",
			agent: &types.CustomAgent{Config: types.CustomAgentConfig{
				AllowedTools: []string{tools.ToolKnowledgeSearch},
			}},
			want: true,
		},
		{
			name:  "default tools include retrieval",
			agent: &types.CustomAgent{Config: types.CustomAgentConfig{}},
			want:  true,
		},
		{
			name: "non-retrieval tools do not use reranker",
			agent: &types.CustomAgent{Config: types.CustomAgentConfig{
				AllowedTools: []string{"thinking", "todo_write"},
			}},
			want: false,
		},
		{
			name:  "nil agent",
			agent: nil,
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, agentRequiresRerankModel(tt.agent))
		})
	}
}
