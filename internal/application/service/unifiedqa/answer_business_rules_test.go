package unifiedqa

import (
	"strings"
	"testing"
)

func TestBuildAnswerBusinessPolicyRequiresClarificationForThreeScenarios(t *testing.T) {
	policy := buildAnswerBusinessPolicy("Can this request be approved?", []ObservedFact{
		{Scenario: "employee travel"}, {Scenario: "HCP travel"}, {Scenario: "international travel"},
	}, true)
	if policy.ScenarioCount != 3 || !policy.RequiresScenarioClarification {
		t.Fatalf("policy = %+v", policy)
	}
}

func TestBuildAnswerBusinessPolicyListsThreeScenariosWhenSelectionIsNotRequired(t *testing.T) {
	policy := buildAnswerBusinessPolicy("List all applicable rules", []ObservedFact{
		{Scenario: "gifts"}, {Scenario: "educational items"}, {Scenario: "business meals"},
	}, false)
	if policy.ScenarioCount != 3 || policy.RequiresScenarioSelection || policy.RequiresScenarioClarification {
		t.Fatalf("policy = %+v", policy)
	}
}

func TestBuildAnswerBusinessPolicyListsScenariosForExplicitEnumerationQuestions(t *testing.T) {
	facts := []ObservedFact{{Scenario: "one"}, {Scenario: "two"}, {Scenario: "three"}}
	questions := []string{
		"出差期间的餐费是否可以报销？需要哪些凭证和审批？",
		"邀请 HCP 参加外地会议并报销交通住宿，需要哪些财务审批和合规条件？",
		"部门负责人可以审批多大金额的采购合同？",
		"员工垫付客户会议费用后应该如何申请报销？",
		"赞助医疗卫生组织举办会议时，付款和合规审批分别有什么要求？",
		"What approvals are required for reimbursing an HCP's travel expenses?",
	}
	for _, question := range questions {
		policy := buildAnswerBusinessPolicy(question, facts, true)
		if policy.RequiresScenarioSelection || policy.RequiresScenarioClarification {
			t.Fatalf("question=%q policy=%+v", question, policy)
		}
	}
}

func TestBuildAnswerBusinessPolicyStillClarifiesUnderspecifiedExpense(t *testing.T) {
	policy := buildAnswerBusinessPolicy("这笔费用能不能批？", []ObservedFact{
		{Scenario: "travel"}, {Scenario: "entertainment"}, {Scenario: "procurement"},
	}, true)
	if !policy.RequiresScenarioSelection || !policy.RequiresScenarioClarification {
		t.Fatalf("policy = %+v", policy)
	}
}

func TestScenarioClarificationDoesNotExposeInternalScenarioCount(t *testing.T) {
	answer := renderScenarioClarification(answerLanguageChinese)
	if !strings.Contains(answer, "信息不足") || strings.Contains(answer, "3 个") || strings.Contains(answer, "检索到") {
		t.Fatalf("answer = %q", answer)
	}
}

func TestSortFactsByDocumentPriority(t *testing.T) {
	facts := sortFactsByDocumentPriority([]ObservedFact{
		{Statement: "industry", DocumentLevel: "industry_guideline"},
		{Statement: "policy", DocumentLevel: "formal_policy"},
		{Statement: "sop", DocumentLevel: "internal_sop"},
	})
	if facts[0].Statement != "sop" || facts[1].Statement != "policy" || facts[2].Statement != "industry" {
		t.Fatalf("facts = %+v", facts)
	}
}

func TestBuildAnswerBusinessPolicyCalculatesCHFAndRMB(t *testing.T) {
	tests := []struct {
		question string
		want     string
		to       string
	}{
		{question: "100 CHF 等于多少人民币？", want: "600", to: "RMB"},
		{question: "CHF 100 等于多少人民币？", want: "600", to: "RMB"},
		{question: "600 RMB 等于多少瑞士法郎？", want: "100", to: "CHF"},
	}
	for _, tt := range tests {
		policy := buildAnswerBusinessPolicy(tt.question, nil, false)
		if policy.CurrencyConversion == nil || policy.CurrencyConversion.Result != tt.want || policy.CurrencyConversion.ToCurrency != tt.to {
			t.Fatalf("question=%q policy=%+v", tt.question, policy)
		}
	}
}

func TestBuildAnswerBusinessPolicyUsesConfiguredRMBCHFRate(t *testing.T) {
	rate := rmbCHFRate{RMBAmount: "6.25", CHFAmount: "1"}
	policy := buildAnswerBusinessPolicy("100 CHF 等于多少人民币？", nil, false, rate)
	if policy.CurrencyConversion == nil || policy.CurrencyConversion.Result != "625" ||
		policy.CurrencyConversion.Rate != "1 CHF = 6.25 RMB" {
		t.Fatalf("policy = %+v", policy)
	}

	reverse := buildAnswerBusinessPolicy("625 RMB 等于多少瑞士法郎？", nil, false, rate)
	if reverse.CurrencyConversion == nil || reverse.CurrencyConversion.Result != "100" {
		t.Fatalf("reverse policy = %+v", reverse)
	}
}

func TestBuildAnswerBusinessPolicyFallsBackForInvalidConfiguredRate(t *testing.T) {
	policy := buildAnswerBusinessPolicy("100 CHF 等于多少人民币？", nil, false, rmbCHFRate{})
	if policy.CurrencyConversion == nil || policy.CurrencyConversion.Result != "600" ||
		policy.CurrencyConversion.Rate != "1 CHF = 6 RMB" {
		t.Fatalf("policy = %+v", policy)
	}
}

func TestBuildAnswerBusinessPolicyDefaultsUnspecifiedFinancialAmountToRMB(t *testing.T) {
	if policy := buildAnswerBusinessPolicy("这笔报销金额 100 可以吗？", nil, false); policy.DefaultCurrency != "RMB" {
		t.Fatalf("policy = %+v", policy)
	}
	if policy := buildAnswerBusinessPolicy("2025 年的报销政策是什么？", nil, false); policy.DefaultCurrency != "" {
		t.Fatalf("year must not be treated as a financial amount: %+v", policy)
	}
}
