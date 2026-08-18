package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/utils"
)

var textCounterTool = BaseTool{
	name:        ToolTextCounter,
	description: "Count Unicode characters, non-whitespace characters, whitespace-delimited words, and lines in the provided text.",
	schema:      utils.GenerateSchema[TextCounterInput](),
}

// TextCounterInput is the input accepted by the text_counter tool.
type TextCounterInput struct {
	Text string `json:"text" jsonschema:"the complete text to count"`
}

// TextCounterTool performs deterministic, dependency-free text counting.
type TextCounterTool struct {
	BaseTool
}

// NewTextCounterTool creates a text_counter tool.
func NewTextCounterTool() *TextCounterTool {
	return &TextCounterTool{BaseTool: textCounterTool}
}

// Execute counts text using Unicode code points. Words are separated by
// whitespace, and an empty string has zero lines.
func (t *TextCounterTool) Execute(_ context.Context, args json.RawMessage) (*types.ToolResult, error) {
	var input TextCounterInput
	if err := json.Unmarshal(args, &input); err != nil {
		return &types.ToolResult{
			Success: false,
			Error:   fmt.Sprintf("failed to parse input args: %v", err),
		}, err
	}

	characterCount := utf8.RuneCountInString(input.Text)
	nonWhitespaceCount := 0
	for _, r := range input.Text {
		if !unicode.IsSpace(r) {
			nonWhitespaceCount++
		}
	}

	lineCount := 0
	if input.Text != "" {
		lineCount = strings.Count(input.Text, "\n") + 1
	}
	wordCount := len(strings.Fields(input.Text))

	output := fmt.Sprintf(
		"字符数（含空白）：%d\n非空白字符数：%d\n单词数（按空白分隔）：%d\n行数：%d",
		characterCount,
		nonWhitespaceCount,
		wordCount,
		lineCount,
	)

	return &types.ToolResult{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			"character_count":      characterCount,
			"non_whitespace_count": nonWhitespaceCount,
			"word_count":           wordCount,
			"line_count":           lineCount,
		},
	}, nil
}
