package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTextCounterTool_Execute(t *testing.T) {
	tests := []struct {
		name               string
		text               string
		characterCount     int
		nonWhitespaceCount int
		wordCount          int
		lineCount          int
	}{
		{
			name: "empty text",
		},
		{
			name:               "unicode multiline text",
			text:               "Hello 世界\nGo",
			characterCount:     11,
			nonWhitespaceCount: 9,
			wordCount:          3,
			lineCount:          2,
		},
	}

	tool := NewTextCounterTool()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, err := json.Marshal(TextCounterInput{Text: tt.text})
			require.NoError(t, err)

			result, err := tool.Execute(context.Background(), args)
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.Success)
			assert.Equal(t, tt.characterCount, result.Data["character_count"])
			assert.Equal(t, tt.nonWhitespaceCount, result.Data["non_whitespace_count"])
			assert.Equal(t, tt.wordCount, result.Data["word_count"])
			assert.Equal(t, tt.lineCount, result.Data["line_count"])
		})
	}
}

func TestTextCounterTool_InvalidInput(t *testing.T) {
	result, err := NewTextCounterTool().Execute(context.Background(), json.RawMessage(`{"text":`))

	require.Error(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Success)
	assert.NotEmpty(t, result.Error)
}
