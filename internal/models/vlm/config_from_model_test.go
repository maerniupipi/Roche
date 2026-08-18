package vlm

import (
	"testing"

	"roche.local/knowledge-agent-platform/internal/types"
)

func TestConfigFromModel_RemoteDefaultsToOpenAI(t *testing.T) {
	m := &types.Model{
		ID:     "v1",
		Name:   "gpt-4o",
		Source: types.ModelSourceRemote,
		Parameters: types.ModelParameters{
			BaseURL:       "https://api.example.com/v1",
			APIKey:        "sk",
			Provider:      "openai",
			ExtraConfig:   map[string]string{"x": "y"},
			CustomHeaders: map[string]string{"H": "v"},
		},
	}
	cfg := ConfigFromModel(m)
	if cfg.InterfaceType != "openai" {
		t.Errorf("expected openai default for remote, got %q", cfg.InterfaceType)
	}
	if cfg.CustomHeaders["H"] != "v" {
		t.Errorf("CustomHeaders not propagated: %+v", cfg.CustomHeaders)
	}
	if cfg.Extra["x"] != "y" {
		t.Errorf("ExtraConfig not propagated as Extra: %+v", cfg.Extra)
	}
}
