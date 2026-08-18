package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"roche.local/knowledge-agent-platform/internal/agent/skills"
	"roche.local/knowledge-agent-platform/internal/types"
)

func newReadSkillTestManager(t *testing.T, skillContent string, extraFiles map[string]string) *skills.Manager {
	t.Helper()

	root := t.TempDir()
	skillDir := filepath.Join(root, "text-tools")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, skills.SkillFileName), []byte(skillContent), 0o644))
	for name, content := range extraFiles {
		path := filepath.Join(skillDir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}

	manager := skills.NewManager(&skills.ManagerConfig{
		SkillDirs: []string{root},
		Enabled:   true,
	}, nil)
	require.NoError(t, manager.Initialize(context.Background()))
	return manager
}

func executeReadSkill(t *testing.T, tool *ReadSkillTool, input ReadSkillInput) *types.ToolResult {
	t.Helper()
	args, err := json.Marshal(input)
	require.NoError(t, err)
	result, err := tool.Execute(context.Background(), args)
	require.NoError(t, err)
	require.NotNil(t, result)
	return result
}

func TestReadSkillTool_ActivatesDeclaredTools(t *testing.T) {
	manager := newReadSkillTestManager(t, `---
name: text-tools
description: Text helpers.
tools:
  - hidden_tool
---
# Text Tools

Use hidden_tool for this task.
`, nil)
	registry := NewToolRegistry()
	require.NoError(t, registry.RegisterDormantTool(&mockTool{name: "hidden_tool", description: "hidden"}))

	tool := NewReadSkillTool(manager, registry)
	result := executeReadSkill(t, tool, ReadSkillInput{SkillName: "text-tools"})

	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "## Activated Tools")
	assert.Contains(t, result.Output, "hidden_tool")
	assert.True(t, registry.IsToolActive("hidden_tool"))
	assert.Equal(t, []string{"hidden_tool"}, definitionNames(registry.GetFunctionDefinitions()))
}

func TestReadSkillTool_AuxiliaryFileDoesNotActivateTools(t *testing.T) {
	manager := newReadSkillTestManager(t, `---
name: text-tools
description: Text helpers.
tools:
  - hidden_tool
---
# Text Tools
`, map[string]string{"REFERENCE.md": "reference content"})
	registry := NewToolRegistry()
	require.NoError(t, registry.RegisterDormantTool(&mockTool{name: "hidden_tool", description: "hidden"}))

	tool := NewReadSkillTool(manager, registry)
	result := executeReadSkill(t, tool, ReadSkillInput{SkillName: "text-tools", FilePath: "REFERENCE.md"})

	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "reference content")
	assert.False(t, registry.IsToolActive("hidden_tool"))
}

func TestReadSkillTool_MissingDeclaredToolFails(t *testing.T) {
	manager := newReadSkillTestManager(t, `---
name: text-tools
description: Text helpers.
tools:
  - missing_tool
---
# Text Tools
`, nil)
	registry := NewToolRegistry()

	tool := NewReadSkillTool(manager, registry)
	result := executeReadSkill(t, tool, ReadSkillInput{SkillName: "text-tools"})

	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "missing_tool")
}

func TestReadSkillTool_InstructionOnlySkillRemainsSupported(t *testing.T) {
	manager := newReadSkillTestManager(t, `---
name: text-tools
description: Instruction-only text guidance.
---
# Text Tools

Follow these instructions.
`, nil)
	registry := NewToolRegistry()

	tool := NewReadSkillTool(manager, registry)
	result := executeReadSkill(t, tool, ReadSkillInput{SkillName: "text-tools"})

	assert.True(t, result.Success)
	assert.NotContains(t, result.Output, "## Activated Tools")
}

func TestReadSkillTool_ActivatesTextCounterTool(t *testing.T) {
	manager := newReadSkillTestManager(t, `---
name: text-counter
description: Count characters, words, and lines in text.
tools:
  - text_counter
---
# Text Counter

Call text_counter with the user's text.
`, nil)
	registry := NewToolRegistry()
	require.NoError(t, registry.RegisterDormantTool(NewTextCounterTool()))

	readSkillTool := NewReadSkillTool(manager, registry)
	result := executeReadSkill(t, readSkillTool, ReadSkillInput{SkillName: "text-counter"})

	assert.True(t, result.Success)
	assert.True(t, registry.IsToolActive(ToolTextCounter))

	args, err := json.Marshal(TextCounterInput{Text: "测试 skill\n第二行"})
	require.NoError(t, err)
	countResult, err := registry.ExecuteTool(context.Background(), ToolTextCounter, args)
	require.NoError(t, err)
	assert.True(t, countResult.Success)
	assert.Equal(t, 12, countResult.Data["character_count"])
	assert.Equal(t, 2, countResult.Data["line_count"])
}
