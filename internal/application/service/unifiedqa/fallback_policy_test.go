package unifiedqa

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuildTopicAnswerPolicyCombinesPartialFallbackAndSpecificDisclaimer(t *testing.T) {
	policy := buildTopicAnswerPolicy(
		mustTestTopicCatalog(t),
		AggregatedObservation{MatchedTopics: []string{"doa"}, FallbackTopics: []string{"travel_expense"}},
		"DoA 和 T&E 分别有什么要求？",
		answerLanguageChinese,
		nil,
	)
	if got, want := policy.TailResponsePolicyCodes(), []string{
		"topic.travel_expense.no_match",
		"topic.doa.answer_disclaimer",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("response policy codes = %v, want %v", got, want)
	}
	tail := strings.Join(policy.TailSections(), "\n\n")
	if !strings.Contains(tail, "未能在差旅报销政策中找到") ||
		!strings.Contains(tail, "以上信息基于现行授权手册（DoA）") ||
		strings.Contains(tail, "本回答基于当前可检索") {
		t.Fatalf("tail = %q", tail)
	}
}

func TestBuildTopicAnswerPolicyConcatenatesAllTopicFallbacksInConfiguredOrder(t *testing.T) {
	policy := buildTopicAnswerPolicy(
		mustTestTopicCatalog(t),
		AggregatedObservation{FallbackTopics: []string{"travel_expense", "doa", "compliance"}},
		"DoA、T&E 和 Compliance",
		answerLanguageChinese,
		nil,
	)
	if got, want := policy.NoKnowledgeResponsePolicyCodes(), []string{
		"topic.doa.no_match",
		"topic.travel_expense.no_match",
		"topic.compliance.no_match",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("response policy codes = %v, want %v", got, want)
	}
	answer := policy.NoKnowledgeResponse()
	doa := strings.Index(answer, "授权手册（DoA）")
	travel := strings.Index(answer, "差旅报销政策")
	compliance := strings.Index(answer, "合规团队")
	if doa < 0 || travel <= doa || compliance <= travel {
		t.Fatalf("fallback order is incorrect: %q", answer)
	}
}

func TestBuildTopicAnswerPolicyReportsConcurAddendum(t *testing.T) {
	policy := buildTopicAnswerPolicy(
		mustTestTopicCatalog(t),
		AggregatedObservation{MatchedTopics: []string{"travel_expense"}, Facts: []ObservedFact{{Statement: "fact"}}},
		"How do I submit this in Concur?",
		answerLanguageChinese,
		nil,
	)
	if got, want := policy.TailResponsePolicyCodes(), []string{
		"topic.travel_expense.answer_disclaimer",
		"topic.travel_expense.addendum.concur",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("response policy codes = %v, want %v", got, want)
	}
}

func TestNoKnowledgePolicyCodesFollowRenderedResponsePrecedence(t *testing.T) {
	policy := buildTopicAnswerPolicy(
		mustTestTopicCatalog(t),
		AggregatedObservation{FallbackTopics: []string{"doa"}},
		"What does ABC mean?",
		answerLanguageEnglish,
		[]string{"ABC"},
	)
	if got, want := policy.NoKnowledgeResponsePolicyCodes(), []string{"global.term_unrecognized"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("response policy codes = %v, want %v", got, want)
	}

	empty := TopicAnswerPolicy{}
	if got, want := empty.NoKnowledgeResponsePolicyCodes(), []string{"global.no_knowledge"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("empty response policy codes = %v, want %v", got, want)
	}
}
