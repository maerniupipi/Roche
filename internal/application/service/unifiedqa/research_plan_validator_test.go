package unifiedqa

import "testing"

func TestResearchPlanValidatorAcceptsBoundedAllowedPlan(t *testing.T) {
	validator := NewResearchPlanValidator(5)
	profile, _ := mustTestCatalog(t).Get(FinanceAgentID)
	profile.AllowedResearchTools = append(profile.AllowedResearchTools, "grep_chunks")
	plan := ResearchPlan{Calls: []ResearchCall{
		{Tool: "knowledge_search", Query: "expense policy"},
		{Tool: "grep_chunks", Query: "limit|threshold"},
	}}
	if err := validator.Validate(profile, plan); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestResearchPlanValidatorRejectsUnapprovedOrExcessCalls(t *testing.T) {
	validator := NewResearchPlanValidator(1)
	profile, _ := mustTestCatalog(t).Get(FinanceAgentID)
	for _, plan := range []ResearchPlan{
		{Calls: []ResearchCall{{Tool: "web_search", Query: "q"}}},
		{Calls: []ResearchCall{{Tool: "knowledge_search", Query: "q1"}, {Tool: "grep_chunks", Query: "q2"}}},
		{Calls: []ResearchCall{{Tool: "knowledge_search", Query: ""}}},
	} {
		if err := validator.Validate(profile, plan); err == nil {
			t.Fatalf("Validate(%+v) error = nil", plan)
		}
	}
}
