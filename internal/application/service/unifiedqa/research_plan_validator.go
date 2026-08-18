package unifiedqa

import (
	"fmt"
	"slices"
	"strings"
)

type ResearchCall struct {
	Tool  string   `json:"tool"`
	Query string   `json:"query"`
	Terms []string `json:"terms,omitempty"`
}

type ResearchPlan struct {
	Calls []ResearchCall `json:"calls"`
}

type ResearchPlanValidator struct{ maxCalls int }

func NewResearchPlanValidator(maxCalls int) *ResearchPlanValidator {
	if maxCalls <= 0 {
		maxCalls = 5
	}
	return &ResearchPlanValidator{maxCalls: maxCalls}
}

// Validate is the code-side safety boundary for model-proposed research. The
// plan has no KB, user, ACL, web, MCP, or write-operation fields by design.
func (v *ResearchPlanValidator) Validate(profile DomainAgentProfile, plan ResearchPlan) error {
	if v == nil {
		return fmt.Errorf("research plan validator is required")
	}
	if len(plan.Calls) == 0 || len(plan.Calls) > v.maxCalls {
		return fmt.Errorf("research plan must contain between one and %d calls", v.maxCalls)
	}
	for i, call := range plan.Calls {
		if !slices.Contains(profile.AllowedResearchTools, call.Tool) {
			return fmt.Errorf("research call %d tool %q is not allowed for agent %q", i, call.Tool, profile.ID)
		}
		if strings.TrimSpace(call.Query) == "" {
			return fmt.Errorf("research call %d query is required", i)
		}
		if len(call.Terms) > 10 {
			return fmt.Errorf("research call %d has too many exact terms", i)
		}
		for _, term := range call.Terms {
			if strings.TrimSpace(term) == "" {
				return fmt.Errorf("research call %d contains an empty exact term", i)
			}
		}
	}
	return nil
}
