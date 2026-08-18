package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var unifiedQATopicIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

type UnifiedQALocalizedText map[string]string

type UnifiedQAFallbacksConfig struct {
	Version string                            `yaml:"version" json:"version"`
	Global  map[string]UnifiedQALocalizedText `yaml:"global" json:"global"`
	Topics  []UnifiedQATopicConfig            `yaml:"topics" json:"topics"`
}

type UnifiedQATopicConfig struct {
	ID                        string                         `yaml:"id" json:"id"`
	AgentID                   string                         `yaml:"agent_id" json:"agent_id"`
	KnowledgeBaseNameContains []string                       `yaml:"knowledge_base_name_contains" json:"knowledge_base_name_contains"`
	RouteKeywords             []string                       `yaml:"route_keywords" json:"route_keywords"`
	NoMatchResponse           UnifiedQALocalizedText         `yaml:"no_match_response" json:"no_match_response"`
	AnswerDisclaimer          UnifiedQALocalizedText         `yaml:"answer_disclaimer" json:"answer_disclaimer"`
	Addenda                   []UnifiedQATopicAddendumConfig `yaml:"addenda" json:"addenda"`
}

type UnifiedQATopicAddendumConfig struct {
	ID              string                 `yaml:"id" json:"id"`
	TriggerKeywords []string               `yaml:"trigger_keywords" json:"trigger_keywords"`
	Response        UnifiedQALocalizedText `yaml:"response" json:"response"`
}

func LoadUnifiedQAFallbacksFile(path string) (*UnifiedQAFallbacksConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read unified QA fallbacks config: %w", err)
	}
	var cfg UnifiedQAFallbacksConfig
	if err := decodeStrictUnifiedQAYAML(data, &cfg); err != nil {
		return nil, fmt.Errorf("decode unified QA fallbacks config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate unified QA fallbacks config: %w", err)
	}
	return &cfg, nil
}

func (c *UnifiedQAFallbacksConfig) Validate() error {
	if c == nil {
		return fmt.Errorf("config is nil")
	}
	if strings.TrimSpace(c.Version) == "" {
		return fmt.Errorf("version is required")
	}
	for _, key := range []string{"out_of_service", "term_unrecognized", "out_of_coverage", "route_failed"} {
		text, ok := c.Global[key]
		if !ok || strings.TrimSpace(text["zh-CN"]) == "" {
			return fmt.Errorf("global.%s.zh-CN is required", key)
		}
	}
	seenTopics := make(map[string]struct{}, len(c.Topics))
	for i := range c.Topics {
		topic := &c.Topics[i]
		if !unifiedQATopicIDPattern.MatchString(topic.ID) {
			return fmt.Errorf("topics[%d].id %q must match %s", i, topic.ID, unifiedQATopicIDPattern.String())
		}
		if _, duplicate := seenTopics[topic.ID]; duplicate {
			return fmt.Errorf("duplicate topic id %q", topic.ID)
		}
		seenTopics[topic.ID] = struct{}{}
		if !unifiedQAAgentIDPattern.MatchString(topic.AgentID) {
			return fmt.Errorf("topic %q has invalid agent_id %q", topic.ID, topic.AgentID)
		}
		if err := validateTopicStrings(topic.ID, "knowledge_base_name_contains", topic.KnowledgeBaseNameContains, true); err != nil {
			return err
		}
		if err := validateTopicStrings(topic.ID, "route_keywords", topic.RouteKeywords, true); err != nil {
			return err
		}
		if strings.TrimSpace(topic.NoMatchResponse["zh-CN"]) == "" {
			return fmt.Errorf("topic %q no_match_response.zh-CN is required", topic.ID)
		}
		if strings.TrimSpace(topic.AnswerDisclaimer["zh-CN"]) == "" {
			return fmt.Errorf("topic %q answer_disclaimer.zh-CN is required", topic.ID)
		}
		seenAddenda := make(map[string]struct{}, len(topic.Addenda))
		for j := range topic.Addenda {
			addendum := &topic.Addenda[j]
			if !unifiedQATopicIDPattern.MatchString(addendum.ID) {
				return fmt.Errorf("topic %q addenda[%d].id %q is invalid", topic.ID, j, addendum.ID)
			}
			if _, duplicate := seenAddenda[addendum.ID]; duplicate {
				return fmt.Errorf("topic %q contains duplicate addendum %q", topic.ID, addendum.ID)
			}
			seenAddenda[addendum.ID] = struct{}{}
			if err := validateTopicStrings(topic.ID, "addenda.trigger_keywords", addendum.TriggerKeywords, true); err != nil {
				return err
			}
			if strings.TrimSpace(addendum.Response["zh-CN"]) == "" {
				return fmt.Errorf("topic %q addendum %q response.zh-CN is required", topic.ID, addendum.ID)
			}
		}
	}
	return nil
}

func validateTopicStrings(topicID, field string, values []string, required bool) error {
	if required && len(values) == 0 {
		return fmt.Errorf("topic %q %s must not be empty", topicID, field)
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return fmt.Errorf("topic %q %s contains an empty value", topicID, field)
		}
		key := strings.ToLower(value)
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("topic %q %s contains duplicate value %q", topicID, field, value)
		}
		seen[key] = struct{}{}
	}
	return nil
}
