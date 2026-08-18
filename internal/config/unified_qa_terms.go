package config

import (
	"fmt"
	"os"
	"strings"
)

type UnifiedQATermsConfig struct {
	Version       string   `yaml:"version" json:"version"`
	AcceptedTerms []string `yaml:"accepted_terms" json:"accepted_terms"`
}

func LoadUnifiedQATermsFile(path string) (*UnifiedQATermsConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read unified QA terms config: %w", err)
	}
	var cfg UnifiedQATermsConfig
	if err := decodeStrictUnifiedQAYAML(data, &cfg); err != nil {
		return nil, fmt.Errorf("decode unified QA terms config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate unified QA terms config: %w", err)
	}
	return &cfg, nil
}

func (c *UnifiedQATermsConfig) Validate() error {
	if c == nil {
		return fmt.Errorf("config is nil")
	}
	if strings.TrimSpace(c.Version) == "" {
		return fmt.Errorf("version is required")
	}
	if len(c.AcceptedTerms) == 0 {
		return fmt.Errorf("accepted_terms must not be empty")
	}
	seen := make(map[string]struct{}, len(c.AcceptedTerms))
	for _, term := range c.AcceptedTerms {
		term = strings.TrimSpace(term)
		if term == "" {
			return fmt.Errorf("accepted_terms contains an empty value")
		}
		key := strings.ToLower(term)
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("accepted_terms contains duplicate value %q", term)
		}
		seen[key] = struct{}{}
	}
	return nil
}
