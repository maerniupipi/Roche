package unifiedqa

import (
	"slices"
	"sort"
	"strings"
)

const (
	CoverageComplete     = "complete"
	CoveragePartial      = "partial"
	CoverageInsufficient = "insufficient"
)

type AggregatedObservation struct {
	Coverage                  string            `json:"coverage"`
	Facts                     []ObservedFact    `json:"facts"`
	RequiresScenarioSelection bool              `json:"requires_scenario_selection"`
	Conflicts                 []string          `json:"conflicts,omitempty"`
	MissingRequirements       []string          `json:"missing_requirements,omitempty"`
	AgentStatuses             map[string]string `json:"agent_statuses"`
	TopicStatuses             map[string]string `json:"topic_statuses,omitempty"`
	MatchedTopics             []string          `json:"matched_topics,omitempty"`
	FallbackTopics            []string          `json:"fallback_topics,omitempty"`
	FailedTopics              []string          `json:"failed_topics,omitempty"`
}

type ObservationAggregator struct{}

func NewObservationAggregator() *ObservationAggregator { return &ObservationAggregator{} }

func (*ObservationAggregator) Aggregate(observations []AgentObservation) AggregatedObservation {
	result := AggregatedObservation{
		AgentStatuses: make(map[string]string, len(observations)),
		TopicStatuses: make(map[string]string, len(observations)),
	}
	factsByKey := make(map[string]*ObservedFact)
	allSufficient := len(observations) > 0
	for _, observation := range observations {
		result.RequiresScenarioSelection = result.RequiresScenarioSelection || observation.RequiresScenarioSelection
		if current := result.AgentStatuses[observation.AgentID]; current == "" || (current == EvidenceStatusSufficient && observation.Status != EvidenceStatusSufficient) {
			result.AgentStatuses[observation.AgentID] = observation.Status
		}
		if observation.TopicID != "" {
			result.TopicStatuses[observation.TopicID] = observation.Status
			switch {
			case len(observation.Facts) > 0:
				appendUniqueStrings(&result.MatchedTopics, observation.TopicID)
			case observation.Status == EvidenceStatusInsufficient:
				appendUniqueStrings(&result.FallbackTopics, observation.TopicID)
			default:
				appendUniqueStrings(&result.FailedTopics, observation.TopicID)
			}
		}
		if observation.Status != EvidenceStatusSufficient {
			allSufficient = false
		}
		appendUniqueStrings(&result.Conflicts, observation.Conflicts...)
		appendUniqueStrings(&result.MissingRequirements, observation.MissingRequirements...)
		for _, fact := range observation.Facts {
			key := strings.ToLower(strings.Join(strings.Fields(fact.Statement), " "))
			if key == "" {
				continue
			}
			existing, ok := factsByKey[key]
			if !ok {
				copy := fact
				copy.Citations = slices.Clone(fact.Citations)
				copy.ContributingAgents = nil
				appendUniqueStrings(&copy.ContributingAgents, observation.AgentID)
				factsByKey[key] = &copy
				continue
			}
			appendUniqueStrings(&existing.ContributingAgents, observation.AgentID)
			for _, citation := range fact.Citations {
				found := false
				for _, current := range existing.Citations {
					if current.OpaqueID == citation.OpaqueID {
						found = true
						break
					}
				}
				if !found {
					existing.Citations = append(existing.Citations, citation)
				}
			}
			mergeObservedFactMetadata(existing, fact)
		}
	}
	keys := make([]string, 0, len(factsByKey))
	for key := range factsByKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		result.Facts = append(result.Facts, *factsByKey[key])
	}
	sort.Strings(result.Conflicts)
	sort.Strings(result.MissingRequirements)
	sort.Strings(result.MatchedTopics)
	sort.Strings(result.FallbackTopics)
	sort.Strings(result.FailedTopics)
	switch {
	case len(result.Facts) == 0:
		result.Coverage = CoverageInsufficient
	case allSufficient && len(result.MissingRequirements) == 0:
		result.Coverage = CoverageComplete
	default:
		result.Coverage = CoveragePartial
	}
	return result
}

func mergeObservedFactMetadata(target *ObservedFact, incoming ObservedFact) {
	if target.Quote == "" {
		target.Quote = incoming.Quote
	}
	target.IsAmbiguous = target.IsAmbiguous || incoming.IsAmbiguous
	if target.Scenario == "" {
		target.Scenario = incoming.Scenario
	}
	if documentLevelRank(incoming.DocumentLevel) < documentLevelRank(target.DocumentLevel) {
		target.DocumentLevel = incoming.DocumentLevel
	}
	if target.Currency == "" || target.Currency == "unspecified" {
		target.Currency = incoming.Currency
	} else if incoming.Currency != "" && incoming.Currency != "unspecified" && incoming.Currency != target.Currency {
		target.Currency = "mixed"
	}
}

func documentLevelRank(value string) int {
	switch normalizeDocumentLevel(value) {
	case "internal_sop":
		return 0
	case "formal_policy":
		return 1
	case "industry_guideline":
		return 2
	case "other":
		return 3
	default:
		return 4
	}
}

func appendUniqueStrings(target *[]string, values ...string) {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !slices.Contains(*target, value) {
			*target = append(*target, value)
		}
	}
}
