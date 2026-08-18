package unifiedqa

import (
	"context"
	"regexp"
	"strings"
	"unicode"

	"roche.local/knowledge-agent-platform/internal/types"
)

type answerLanguage string

const (
	answerLanguageChinese answerLanguage = "zh-CN"
	answerLanguageEnglish answerLanguage = "en-US"
)

var disclaimerPattern = regexp.MustCompile(`(?i)免责声明|仅供参考|以正式文件.*为准|\bdisclaimer\b|\bfor reference only\b|official documents?.*prevail`)

func detectAnswerLanguage(ctx context.Context, question string) answerLanguage {
	var han, latin int
	for _, current := range question {
		switch {
		case unicode.Is(unicode.Han, current):
			han++
		case unicode.Is(unicode.Latin, current):
			latin++
		}
	}
	if han > latin {
		return answerLanguageChinese
	}
	if latin > han {
		return answerLanguageEnglish
	}
	if locale, ok := types.LanguageFromContext(ctx); ok && strings.HasPrefix(strings.ToLower(locale), "en") {
		return answerLanguageEnglish
	}
	return answerLanguageChinese
}

func (language answerLanguage) promptName() string {
	if language == answerLanguageEnglish {
		return "English"
	}
	return "Chinese (Simplified)"
}

func renderNoKnowledgeFallback(language answerLanguage) string {
	if language == answerLanguageEnglish {
		return "No relevant knowledge was found to support an answer. The question may be outside the service scope, contain an unrecognized term, or fall outside the currently covered materials. Please provide more context or consult the responsible department."
	}
	return "未检索到可支持回答的相关知识。该问题可能超出服务范围、包含未识别术语，或超出当前资料覆盖范围。请补充更多背景信息，或咨询相关负责部门。"
}

func renderAnswerDisclaimer(language answerLanguage) string {
	if language == answerLanguageEnglish {
		return "Disclaimer: This response is based on currently available knowledge materials and is for reference only. Where it differs from the latest official policy or approval requirements, the official documents and confirmation from the responsible department shall prevail."
	}
	return "免责声明：本回答基于当前可检索的知识资料生成，仅供参考；如与最新正式政策或审批要求不一致，请以正式文件及相关部门确认结果为准。"
}

func answerContainsDisclaimer(answer string) bool {
	return disclaimerPattern.MatchString(answer)
}

func validateAnswerLanguage(answer string, language answerLanguage, facts []answerFact) error {
	prose := answerCitationTagPattern.ReplaceAllString(answer, "")
	for _, fact := range facts {
		if quote := strings.TrimSpace(fact.Quote); quote != "" {
			prose = strings.ReplaceAll(prose, quote, "")
		}
	}
	var han, latin int
	for _, current := range prose {
		switch {
		case unicode.Is(unicode.Han, current):
			han++
		case unicode.Is(unicode.Latin, current):
			latin++
		}
	}
	if han == 0 && latin == 0 {
		return nil
	}
	if language == answerLanguageEnglish && han > latin {
		return &answerPolicyError{message: "final answer does not follow the English input language"}
	}
	if language == answerLanguageChinese && latin > han*3 && han < 4 {
		return &answerPolicyError{message: "final answer does not follow the Chinese input language"}
	}
	return nil
}

func textMatchesAnswerLanguage(value string, language answerLanguage) bool {
	var han, latin int
	for _, current := range value {
		switch {
		case unicode.Is(unicode.Han, current):
			han++
		case unicode.Is(unicode.Latin, current):
			latin++
		}
	}
	if han == 0 || latin == 0 {
		if language == answerLanguageEnglish {
			return latin >= han
		}
		return han >= latin
	}
	if language == answerLanguageEnglish {
		return latin >= han
	}
	return han > latin
}

type answerPolicyError struct{ message string }

func (e *answerPolicyError) Error() string { return e.message }
