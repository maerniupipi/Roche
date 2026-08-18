package unifiedqa

import (
	"reflect"
	"testing"
)

func TestObservationAggregatorMergesFactsAndPreservesContributors(t *testing.T) {
	aggregator := NewObservationAggregator()
	result := aggregator.Aggregate([]AgentObservation{
		{AgentID: FinanceAgentID, Status: EvidenceStatusSufficient, Facts: []ObservedFact{{Statement: "The limit is 100.", Citations: []EvidenceCitation{{OpaqueID: "e_1"}}}}},
		{AgentID: ComplianceAgentID, Status: EvidenceStatusSufficient, Facts: []ObservedFact{{Statement: "  the LIMIT is 100. ", Citations: []EvidenceCitation{{OpaqueID: "e_2"}}}}, Conflicts: []string{"Policy scope differs"}},
	})
	if result.Coverage != CoverageComplete || len(result.Facts) != 1 {
		t.Fatalf("result = %+v", result)
	}
	if got, want := result.Facts[0].ContributingAgents, []string{FinanceAgentID, ComplianceAgentID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("contributors = %v, want %v", got, want)
	}
	if len(result.Facts[0].Citations) != 2 || len(result.Conflicts) != 1 {
		t.Fatalf("result = %+v", result)
	}
}

func TestObservationAggregatorComputesPartialAndInsufficientCoverage(t *testing.T) {
	aggregator := NewObservationAggregator()
	partial := aggregator.Aggregate([]AgentObservation{
		{AgentID: FinanceAgentID, Status: EvidenceStatusSufficient, Facts: []ObservedFact{{Statement: "Fact", Citations: []EvidenceCitation{{OpaqueID: "e"}}}}},
		{AgentID: ComplianceAgentID, Status: EvidenceStatusInsufficient, MissingRequirements: []string{"applicable policy"}},
	})
	if partial.Coverage != CoveragePartial || len(partial.MissingRequirements) != 1 {
		t.Fatalf("partial = %+v", partial)
	}
	insufficient := aggregator.Aggregate([]AgentObservation{
		{AgentID: FinanceAgentID, Status: EvidenceStatusInsufficient, MissingRequirements: []string{"amount"}},
		{AgentID: ComplianceAgentID, Status: "failed", MissingRequirements: []string{"review unavailable"}},
	})
	if insufficient.Coverage != CoverageInsufficient {
		t.Fatalf("insufficient = %+v", insufficient)
	}
}

func TestObservationAggregatorTracksTopicLevelPartialCoverage(t *testing.T) {
	result := NewObservationAggregator().Aggregate([]AgentObservation{
		{AgentID: FinanceAgentID, TopicID: "doa", Status: EvidenceStatusSufficient, Facts: []ObservedFact{{Statement: "Fact", Citations: []EvidenceCitation{{OpaqueID: "e"}}}}},
		{AgentID: FinanceAgentID, TopicID: "travel_expense", Status: EvidenceStatusInsufficient},
	})
	if result.Coverage != CoveragePartial || !reflect.DeepEqual(result.MatchedTopics, []string{"doa"}) || !reflect.DeepEqual(result.FallbackTopics, []string{"travel_expense"}) {
		t.Fatalf("result = %+v", result)
	}
	if result.AgentStatuses[FinanceAgentID] != EvidenceStatusInsufficient {
		t.Fatalf("finance status = %q", result.AgentStatuses[FinanceAgentID])
	}
}

func TestObservationAggregatorPreservesScenarioSelectionRequirement(t *testing.T) {
	result := NewObservationAggregator().Aggregate([]AgentObservation{
		{AgentID: FinanceAgentID, Status: EvidenceStatusSufficient},
		{AgentID: ComplianceAgentID, Status: EvidenceStatusSufficient, RequiresScenarioSelection: true},
	})
	if !result.RequiresScenarioSelection {
		t.Fatalf("result = %+v", result)
	}
}
