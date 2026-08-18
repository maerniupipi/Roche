package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"roche.local/knowledge-agent-platform/internal/common"
	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/types"
)

// toolErrorHint is appended to tool error messages to guide the LLM to retry with a different approach.
const toolErrorHint = "\n\n[Analyze the error above and try a different approach.]"

// ToolRegistry manages the registration and retrieval of tools
type ToolRegistry struct {
	tools             map[string]types.Tool
	activeTools       map[string]struct{}
	skillBindings     map[string][]string
	maxToolOutputSize int // maximum chars for tool output (0 = use DefaultMaxToolOutput)
	mu                sync.RWMutex
}

// NewToolRegistry creates a new tool registry
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools:         make(map[string]types.Tool),
		activeTools:   make(map[string]struct{}),
		skillBindings: make(map[string][]string),
	}
}

// SetMaxToolOutputSize sets the maximum character length for tool output.
// Values <= 0 will use DefaultMaxToolOutput.
func (r *ToolRegistry) SetMaxToolOutputSize(maxChars int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.maxToolOutputSize = maxChars
}

// getMaxToolOutput returns the effective max tool output size.
func (r *ToolRegistry) getMaxToolOutput() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.maxToolOutputSize > 0 {
		return r.maxToolOutputSize
	}
	return DefaultMaxToolOutput
}

// RegisterTool adds a tool to the registry.
// If a tool with the same name is already registered, the existing one is kept
// (first-wins) to prevent tool execution hijacking via name collision (GHSA-67q9-58vj-32qx).
func (r *ToolRegistry) RegisterTool(tool types.Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := tool.Name()
	if _, exists := r.tools[name]; exists {
		logger.Warnf(context.Background(),
			"[ToolRegistry] Duplicate tool registration rejected: %s (first-wins policy)", name)
		return
	}
	r.tools[name] = tool
	r.activeTools[name] = struct{}{}
}

// RegisterDormantTool registers an executable tool without exposing it to the
// LLM. It becomes active only after a bound skill is activated.
func (r *ToolRegistry) RegisterDormantTool(tool types.Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := tool.Name()
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool already registered: %s", name)
	}
	r.tools[name] = tool
	return nil
}

// BindSkillTools records a skill's tool names and hides any of those tools
// that are currently registered. Unknown names remain bound so activation can
// return a useful unavailable/unauthorized error for the current agent.
func (r *ToolRegistry) BindSkillTools(skillName string, toolNames []string) error {
	if skillName == "" {
		return fmt.Errorf("skill name cannot be empty")
	}

	unique := make(map[string]struct{}, len(toolNames))
	for _, name := range toolNames {
		if name == "" {
			return fmt.Errorf("skill %s contains an empty tool name", skillName)
		}
		unique[name] = struct{}{}
	}
	names := make([]string, 0, len(unique))
	for name := range unique {
		names = append(names, name)
	}
	sort.Strings(names)

	r.mu.Lock()
	defer r.mu.Unlock()
	r.skillBindings[skillName] = names
	for _, name := range names {
		delete(r.activeTools, name)
	}
	return nil
}

// ActivateSkillTools exposes all tools bound to a skill. Activation is atomic:
// if any declared tool is unavailable, none of them are activated.
func (r *ToolRegistry) ActivateSkillTools(skillName string) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	names, exists := r.skillBindings[skillName]
	if !exists {
		return nil, fmt.Errorf("skill has no registered tool binding: %s", skillName)
	}
	for _, name := range names {
		if _, exists := r.tools[name]; !exists {
			return nil, fmt.Errorf("skill %s declares unavailable tool %s", skillName, name)
		}
	}
	for _, name := range names {
		r.activeTools[name] = struct{}{}
	}
	return append([]string(nil), names...), nil
}

// IsToolActive reports whether a registered tool is currently exposed and
// executable for this request-scoped registry.
func (r *ToolRegistry) IsToolActive(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, active := r.activeTools[name]
	return active
}

// GetTool retrieves a tool by name
func (r *ToolRegistry) GetTool(name string) (types.Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, exists := r.tools[name]
	if !exists {
		return nil, fmt.Errorf("tool not found: %s", name)
	}
	return tool, nil
}

// ListTools returns all registered tool names sorted alphabetically.
// Sorting keeps the order stable across calls — Go map iteration is
// intentionally randomized.
func (r *ToolRegistry) ListTools() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// GetFunctionDefinitions returns function definitions for all registered tools.
// The slice is sorted by tool name so the serialized payload sent to the LLM
// is byte-identical across requests. Providers that key prompt caching on a
// byte-level prefix match (e.g. Qwen explicit caching) require this — map
// iteration order would otherwise reshuffle the tools block and break cache
// hits.
func (r *ToolRegistry) GetFunctionDefinitions() []types.FunctionDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.activeTools))
	for name := range r.activeTools {
		names = append(names, name)
	}
	sort.Strings(names)

	definitions := make([]types.FunctionDefinition, 0, len(names))
	for _, name := range names {
		tool := r.tools[name]
		definitions = append(definitions, types.FunctionDefinition{
			Name:        tool.Name(),
			Description: tool.Description(),
			Parameters:  tool.Parameters(),
		})
	}
	return definitions
}

// ExecuteTool executes a tool by name with the given arguments
func (r *ToolRegistry) ExecuteTool(
	ctx context.Context,
	name string,
	args json.RawMessage,
) (*types.ToolResult, error) {
	common.PipelineInfo(ctx, "AgentTool", "execute_start", map[string]interface{}{
		"tool": name,
		"args": args,
	})
	r.mu.RLock()
	tool, exists := r.tools[name]
	_, active := r.activeTools[name]
	r.mu.RUnlock()
	if !exists {
		err := fmt.Errorf("tool not found: %s", name)
		common.PipelineError(ctx, "AgentTool", "execute_failed", map[string]interface{}{
			"tool":  name,
			"error": err.Error(),
		})
		return &types.ToolResult{
			Success: false,
			Error:   err.Error() + toolErrorHint,
		}, err
	}
	if !active {
		err := fmt.Errorf("tool is not active: %s", name)
		common.PipelineError(ctx, "AgentTool", "execute_failed", map[string]interface{}{
			"tool":  name,
			"error": err.Error(),
		})
		return &types.ToolResult{
			Success: false,
			Error:   err.Error() + toolErrorHint,
		}, err
	}

	// Cast parameters to match expected schema types before execution.
	// This handles common LLM quirks like returning "true" instead of true.
	args = CastParams(args, tool.Parameters())

	// Validate parameters against the tool's JSON Schema before execution.
	// This catches invalid arguments early, avoiding a wasted tool execution + LLM round.
	if validationErrs := ValidateParams(args, tool.Parameters()); len(validationErrs) > 0 {
		errMsg := FormatValidationErrors(validationErrs) + toolErrorHint
		common.PipelineWarn(ctx, "AgentTool", "validation_failed", map[string]interface{}{
			"tool":   name,
			"errors": errMsg,
		})
		return &types.ToolResult{
			Success: false,
			Error:   errMsg,
		}, nil
	}

	result, execErr := tool.Execute(ctx, args)

	// Truncate large tool outputs to prevent context window poisoning.
	maxOutput := r.getMaxToolOutput()
	if result != nil && len(result.Output) > maxOutput {
		result.Output = TruncateToolOutput(result.Output, maxOutput)
	}

	fields := map[string]interface{}{
		"tool": name,
		"args": args,
	}
	if result != nil {
		fields["success"] = result.Success
		if result.Error != "" {
			fields["error"] = result.Error
		}
	}
	if execErr != nil {
		fields["error"] = execErr.Error()
		common.PipelineError(ctx, "AgentTool", "execute_done", fields)
	} else if result != nil && !result.Success {
		// Append error hint to guide LLM to retry with a different approach
		if result.Error != "" {
			result.Error = result.Error + toolErrorHint
		}
		common.PipelineWarn(ctx, "AgentTool", "execute_done", fields)
	} else {
		common.PipelineInfo(ctx, "AgentTool", "execute_done", fields)
	}

	return result, execErr
}

// Cleanup cleans up all registered tools that implement the types.Cleanable interface.
// This is called at the end of agent sessions to release tool-specific resources.
func (r *ToolRegistry) Cleanup(ctx context.Context) {
	r.mu.RLock()
	tools := make(map[string]types.Tool, len(r.tools))
	for name, tool := range r.tools {
		tools[name] = tool
	}
	r.mu.RUnlock()

	for name, tool := range tools {
		if cleanable, ok := tool.(types.Cleanable); ok {
			logger.Infof(ctx, "[ToolRegistry] Cleaning up tool: %s", name)
			cleanable.Cleanup(ctx)
		}
	}
}
