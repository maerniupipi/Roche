package unifiedqa

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"roche.local/knowledge-agent-platform/internal/config"
)

var terminologyTokenPattern = regexp.MustCompile(`[A-Za-z0-9][A-Za-z0-9&./+_-]*`)

type TerminologyCatalog struct {
	version  string
	accepted map[string]struct{}
}

func NewTerminologyCatalog(cfg *config.UnifiedQATermsConfig) (*TerminologyCatalog, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate terminology catalog: %w", err)
	}
	catalog := &TerminologyCatalog{version: cfg.Version, accepted: make(map[string]struct{}, len(cfg.AcceptedTerms))}
	for _, term := range cfg.AcceptedTerms {
		catalog.accepted[normalizeTerm(term)] = struct{}{}
		for _, token := range terminologyTokenPattern.FindAllString(term, -1) {
			catalog.accepted[normalizeTerm(token)] = struct{}{}
		}
	}
	return catalog, nil
}

func LoadTerminologyCatalog() (*TerminologyCatalog, error) {
	cfg, err := config.LoadUnifiedQATermsFile(filepath.Join(config.ConfigDir(), "unified_qa_terms.yaml"))
	if err != nil {
		return nil, err
	}
	return NewTerminologyCatalog(cfg)
}

func (c *TerminologyCatalog) Version() string {
	if c == nil {
		return ""
	}
	return c.version
}

func (c *TerminologyCatalog) UnknownTerms(query string) []string {
	if c == nil || len(c.accepted) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	unknown := make([]string, 0)
	for _, token := range terminologyTokenPattern.FindAllString(query, -1) {
		normalized := normalizeTerm(token)
		if normalized == "" {
			continue
		}
		if _, known := c.accepted[normalized]; known {
			continue
		}
		if c.isKnownComposite(token) {
			continue
		}
		if !looksLikeProprietaryTerm(token) {
			continue
		}
		if _, duplicate := seen[normalized]; duplicate {
			continue
		}
		seen[normalized] = struct{}{}
		unknown = append(unknown, token)
	}
	slices.SortFunc(unknown, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
	return unknown
}

func (c *TerminologyCatalog) isKnownComposite(token string) bool {
	parts := strings.FieldsFunc(token, func(current rune) bool {
		return strings.ContainsRune("_./+-", current)
	})
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		if strings.Trim(part, "0123456789") == "" {
			continue
		}
		if _, known := c.accepted[normalizeTerm(part)]; !known {
			return false
		}
	}
	return true
}

func normalizeTerm(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.ReplaceAll(value, "＆", "&")
	return strings.Join(strings.Fields(value), " ")
}

func looksLikeProprietaryTerm(token string) bool {
	if len([]rune(token)) < 2 || len([]rune(token)) > 24 {
		return false
	}
	hasLetter, hasDigit, hasSpecial := false, false, false
	upper, lower := 0, 0
	for _, current := range token {
		switch {
		case unicode.IsLetter(current):
			hasLetter = true
			if unicode.IsUpper(current) {
				upper++
			} else if unicode.IsLower(current) {
				lower++
			}
		case unicode.IsDigit(current):
			hasDigit = true
		case strings.ContainsRune("&./+_-", current):
			hasSpecial = true
		}
	}
	if !hasLetter {
		return false
	}
	return hasDigit || hasSpecial || (upper >= 2 && lower == 0) || (upper >= 2 && lower > 0)
}
