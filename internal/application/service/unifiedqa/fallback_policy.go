package unifiedqa

import (
	"fmt"
	"slices"
	"strings"
)

type TopicAnswerPolicy struct {
	NoMatchNotices               []string
	NoMatchPolicyCodes           []string
	SuccessDisclaimers           []string
	SuccessDisclaimerPolicyCodes []string
	Addenda                      []string
	AddendumPolicyCodes          []string
	UnknownTermNotice            string
	MatchedTopics                []string
	FallbackTopics               []string
	FailedTopics                 []string
}

func buildTopicAnswerPolicy(
	catalog *AgentCatalog,
	aggregated AggregatedObservation,
	question string,
	language answerLanguage,
	unknownTerms []string,
) TopicAnswerPolicy {
	policy := TopicAnswerPolicy{
		MatchedTopics:  slices.Clone(aggregated.MatchedTopics),
		FallbackTopics: slices.Clone(aggregated.FallbackTopics),
		FailedTopics:   slices.Clone(aggregated.FailedTopics),
	}
	if catalog == nil {
		return policy
	}
	matched := stringSet(aggregated.MatchedTopics)
	fallback := stringSet(aggregated.FallbackTopics)
	normalizedQuestion := strings.ToLower(question)
	for _, agent := range catalog.Agents() {
		for _, topic := range agent.Topics {
			if _, ok := fallback[topic.ID]; ok {
				if text := localizedTopicText(topic.NoMatchResponse, language); text != "" {
					policy.NoMatchNotices = append(policy.NoMatchNotices, text)
					policy.NoMatchPolicyCodes = append(policy.NoMatchPolicyCodes, topicNoMatchPolicyCode(topic.ID))
				}
			}
			if _, ok := matched[topic.ID]; !ok {
				continue
			}
			if text := localizedTopicText(topic.AnswerDisclaimer, language); text != "" {
				policy.SuccessDisclaimers = append(policy.SuccessDisclaimers, text)
				policy.SuccessDisclaimerPolicyCodes = append(policy.SuccessDisclaimerPolicyCodes, topicDisclaimerPolicyCode(topic.ID))
			}
			for _, addendum := range topic.Addenda {
				if containsAnyKeyword(normalizedQuestion, addendum.TriggerKeywords) {
					if text := localizedTopicText(addendum.Response, language); text != "" {
						policy.Addenda = append(policy.Addenda, text)
						policy.AddendumPolicyCodes = append(policy.AddendumPolicyCodes, topicAddendumPolicyCode(topic.ID, addendum.ID))
					}
				}
			}
		}
	}
	if len(unknownTerms) > 0 {
		policy.UnknownTermNotice = renderCatalogFallback(catalog, "term_unrecognized", language, unknownTerms)
	}
	return policy
}

func (p TopicAnswerPolicy) TailSections() []string {
	sections := make([]string, 0, len(p.NoMatchNotices)+len(p.SuccessDisclaimers)+len(p.Addenda)+1)
	sections = append(sections, p.NoMatchNotices...)
	if strings.TrimSpace(p.UnknownTermNotice) != "" {
		sections = append(sections, p.UnknownTermNotice)
	}
	sections = append(sections, p.SuccessDisclaimers...)
	sections = append(sections, p.Addenda...)
	return sections
}

func (p TopicAnswerPolicy) NoKnowledgeResponse() string {
	if strings.TrimSpace(p.UnknownTermNotice) != "" {
		return p.UnknownTermNotice
	}
	return strings.Join(p.NoMatchNotices, "\n\n")
}

// TailResponsePolicyCodes mirrors TailSections and only reports deterministic
// policy text that is actually appended to a successful answer.
func (p TopicAnswerPolicy) TailResponsePolicyCodes() []string {
	codes := make([]string, 0, len(p.NoMatchPolicyCodes)+len(p.SuccessDisclaimerPolicyCodes)+len(p.AddendumPolicyCodes)+1)
	codes = append(codes, p.NoMatchPolicyCodes...)
	if strings.TrimSpace(p.UnknownTermNotice) != "" {
		codes = append(codes, globalResponsePolicyCode("term_unrecognized"))
	}
	codes = append(codes, p.SuccessDisclaimerPolicyCodes...)
	codes = append(codes, p.AddendumPolicyCodes...)
	return codes
}

// NoKnowledgeResponsePolicyCodes follows the same precedence as
// NoKnowledgeResponse: an unknown-term notice replaces topic no-match notices.
func (p TopicAnswerPolicy) NoKnowledgeResponsePolicyCodes() []string {
	if strings.TrimSpace(p.UnknownTermNotice) != "" {
		return []string{globalResponsePolicyCode("term_unrecognized")}
	}
	if len(p.NoMatchNotices) > 0 {
		return slices.Clone(p.NoMatchPolicyCodes)
	}
	return []string{globalResponsePolicyCode("no_knowledge")}
}

func globalResponsePolicyCode(kind string) string {
	return "global." + strings.TrimSpace(kind)
}

func topicNoMatchPolicyCode(topicID string) string {
	return "topic." + strings.TrimSpace(topicID) + ".no_match"
}

func topicDisclaimerPolicyCode(topicID string) string {
	return "topic." + strings.TrimSpace(topicID) + ".answer_disclaimer"
}

func topicAddendumPolicyCode(topicID, addendumID string) string {
	return "topic." + strings.TrimSpace(topicID) + ".addendum." + strings.TrimSpace(addendumID)
}

func renderCatalogFallback(catalog *AgentCatalog, kind string, language answerLanguage, unknownTerms []string) string {
	if catalog == nil {
		return ""
	}
	text := strings.TrimSpace(catalog.GlobalFallback(kind, language.locale()))
	if text == "" || kind != "term_unrecognized" || len(unknownTerms) == 0 {
		return text
	}
	if language == answerLanguageEnglish {
		return fmt.Sprintf("%s\n\nUnrecognized terms: %s", text, strings.Join(unknownTerms, ", "))
	}
	return fmt.Sprintf("%s\n\n未识别术语：%s", text, strings.Join(unknownTerms, "、"))
}

func localizedTopicText(values map[string]string, language answerLanguage) string {
	if values == nil {
		return ""
	}
	return strings.TrimSpace(values[language.locale()])
}

func (language answerLanguage) locale() string {
	if language == answerLanguageEnglish {
		return "en-US"
	}
	return "zh-CN"
}

func stringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func containsAnyKeyword(normalizedValue string, keywords []string) bool {
	for _, keyword := range keywords {
		if strings.Contains(normalizedValue, strings.ToLower(strings.TrimSpace(keyword))) {
			return true
		}
	}
	return false
}
