package unifiedqa

import (
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strings"
)

type rmbCHFRate struct {
	RMBAmount string
	CHFAmount string
}

var defaultRMBCHFRate = rmbCHFRate{RMBAmount: "6", CHFAmount: "1"}

type answerBusinessPolicy struct {
	ScenarioCount                 int                 `json:"scenario_count"`
	RequiresScenarioSelection     bool                `json:"requires_scenario_selection"`
	RequiresScenarioClarification bool                `json:"requires_scenario_clarification"`
	DefaultCurrency               string              `json:"default_currency,omitempty"`
	CurrencyConversion            *currencyConversion `json:"currency_conversion,omitempty"`
}

type currencyConversion struct {
	Amount       string `json:"amount"`
	FromCurrency string `json:"from_currency"`
	ToCurrency   string `json:"to_currency"`
	Result       string `json:"result"`
	Rate         string `json:"rate"`
}

var (
	currencyAmountPattern = regexp.MustCompile(`(?i)([0-9]+(?:\.[0-9]+)?)\s*(CHF|RMB|CNY|瑞士法郎|人民币|元)`)
	currencyPrefixPattern = regexp.MustCompile(`(?i)(CHF|RMB|CNY|瑞士法郎|人民币)\s*([0-9]+(?:\.[0-9]+)?)`)
	bareAmountPattern     = regexp.MustCompile(`(?:^|[^0-9])([0-9]+(?:\.[0-9]+)?)(?:[^0-9]|$)`)
)

func buildAnswerBusinessPolicy(question string, facts []ObservedFact, requiresScenarioSelection bool, configuredRates ...rmbCHFRate) answerBusinessPolicy {
	rate := defaultRMBCHFRate
	if len(configuredRates) > 0 && configuredRates[0].valid() {
		rate = configuredRates[0]
	}
	policy := answerBusinessPolicy{
		ScenarioCount:             countFactScenarios(facts),
		RequiresScenarioSelection: requiresScenarioSelection && !questionRequestsScenarioEnumeration(question),
	}
	policy.RequiresScenarioClarification = policy.RequiresScenarioSelection && policy.ScenarioCount >= 3
	policy.CurrencyConversion = parseCurrencyConversion(question, rate)
	if policy.CurrencyConversion == nil && questionUsesUnspecifiedFinancialAmount(question) {
		policy.DefaultCurrency = "RMB"
	}
	return policy
}

// questionRequestsScenarioEnumeration identifies questions that explicitly ask
// for the applicable rules, requirements, limits, or process to be listed. In
// that case the product requirement is to present every applicable scenario,
// even when the evidence reviewer discovered three or more scenarios. The
// review model's requires_scenario_selection flag remains authoritative for
// genuinely underspecified decisions such as "Can this expense be approved?".
func questionRequestsScenarioEnumeration(question string) bool {
	normalized := strings.ToLower(strings.TrimSpace(question))
	if normalized == "" {
		return false
	}
	for _, underspecified := range []string{
		"这笔费用", "这项费用", "这个费用", "该笔费用", "该项费用",
		"this expense", "that expense", "this payment", "that payment", "this request", "that request",
	} {
		if strings.Contains(normalized, underspecified) {
			return false
		}
	}
	for _, marker := range []string{
		"哪些", "有什么", "是什么", "如何", "怎么", "多少", "多大", "分别", "列出", "说明", "总结", "汇总",
		"要求", "限制", "规则", "规定", "政策", "流程", "标准",
		"what ", "what?", "which ", "how ", "list ", "summarize", "summary",
		"requirement", "restriction", "rule", "policy", "process", "standard", "limit",
	} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func countFactScenarios(facts []ObservedFact) int {
	seen := make(map[string]struct{})
	for _, fact := range facts {
		if scenario := normalizeEvidenceText(fact.Scenario); scenario != "" {
			seen[scenario] = struct{}{}
		}
	}
	return len(seen)
}

func sortFactsByDocumentPriority(facts []ObservedFact) []ObservedFact {
	ordered := append([]ObservedFact(nil), facts...)
	sort.SliceStable(ordered, func(i, j int) bool {
		left, right := documentLevelRank(ordered[i].DocumentLevel), documentLevelRank(ordered[j].DocumentLevel)
		if left != right {
			return left < right
		}
		return normalizeEvidenceText(ordered[i].Scenario) < normalizeEvidenceText(ordered[j].Scenario)
	})
	return ordered
}

func renderScenarioClarification(language answerLanguage) string {
	if language == answerLanguageEnglish {
		return "There is not enough information to determine whether this request can be approved. Please provide the expense or request type, amount and currency, people involved, location, business purpose, and the applicant/approver role; I will then apply the relevant policy and approval scenario."
	}
	return "目前信息不足，暂时无法判断这笔费用是否可以批准。请补充费用/事项类型、金额和币种、涉及人员、地点、业务目的，以及申请人或审批人的角色；我会再按对应政策和审批场景给出结论与依据。"
}

func renderCurrencyPolicyAddendum(policy answerBusinessPolicy, language answerLanguage) string {
	if policy.CurrencyConversion != nil {
		conversion := policy.CurrencyConversion
		if language == answerLanguageEnglish {
			return fmt.Sprintf("Conversion result: %s %s = %s %s (using %s).", conversion.Amount, conversion.FromCurrency, conversion.Result, conversion.ToCurrency, conversion.Rate)
		}
		return fmt.Sprintf("换算结果：%s %s = %s %s（按 %s）。", conversion.Amount, conversion.FromCurrency, conversion.Result, conversion.ToCurrency, conversion.Rate)
	}
	if policy.DefaultCurrency == "RMB" {
		if language == answerLanguageEnglish {
			return "Currency note: The amount in the question has no specified currency and is interpreted as RMB."
		}
		return "币种说明：问题中的金额未注明币种，按人民币（RMB）理解。"
	}
	return ""
}

func answerContainsCurrencyCalculation(answer string) bool {
	upper := strings.ToUpper(answer)
	mentionsCurrency := strings.Contains(upper, "CHF") || strings.Contains(upper, "RMB") ||
		strings.Contains(answer, "人民币") || strings.Contains(answer, "瑞士法郎")
	if !mentionsCurrency {
		return false
	}
	return strings.Contains(answer, "=") || strings.Contains(answer, "换算") ||
		strings.Contains(answer, "汇率") || strings.Contains(answer, "折合")
}

func parseCurrencyConversion(question string, rate rmbCHFRate) *currencyConversion {
	matches := currencyAmountPattern.FindAllStringSubmatch(question, -1)
	amount, currency := "", ""
	if len(matches) == 1 {
		amount, currency = matches[0][1], matches[0][2]
	} else if prefixMatches := currencyPrefixPattern.FindAllStringSubmatch(question, -1); len(prefixMatches) == 1 {
		currency, amount = prefixMatches[0][1], prefixMatches[0][2]
	} else {
		return nil
	}
	from := normalizeCurrencyToken(currency)
	upper := strings.ToUpper(question)
	wantsCHF := strings.Contains(upper, "CHF") || strings.Contains(question, "瑞士法郎")
	wantsRMB := strings.Contains(upper, "RMB") || strings.Contains(upper, "CNY") || strings.Contains(question, "人民币")
	to := ""
	switch {
	case from == "CHF" && wantsRMB:
		to = "RMB"
	case from == "RMB" && wantsCHF:
		to = "CHF"
	default:
		return nil
	}
	value, ok := new(big.Rat).SetString(amount)
	if !ok {
		return nil
	}
	rmbAmount, rmbOK := new(big.Rat).SetString(rate.RMBAmount)
	chfAmount, chfOK := new(big.Rat).SetString(rate.CHFAmount)
	if !rmbOK || !chfOK || rmbAmount.Sign() <= 0 || chfAmount.Sign() <= 0 {
		return nil
	}
	if from == "CHF" {
		value.Mul(value, new(big.Rat).Quo(rmbAmount, chfAmount))
	} else {
		value.Mul(value, new(big.Rat).Quo(chfAmount, rmbAmount))
	}
	return &currencyConversion{
		Amount: amount, FromCurrency: from, ToCurrency: to,
		Result: formatCurrencyRat(value),
		Rate:   fmt.Sprintf("%s CHF = %s RMB", trimDecimal(rate.CHFAmount), trimDecimal(rate.RMBAmount)),
	}
}

func (r rmbCHFRate) valid() bool {
	rmb, rmbOK := new(big.Rat).SetString(r.RMBAmount)
	chf, chfOK := new(big.Rat).SetString(r.CHFAmount)
	return rmbOK && chfOK && rmb.Sign() > 0 && chf.Sign() > 0
}

func trimDecimal(value string) string {
	if strings.Contains(value, ".") {
		value = strings.TrimRight(strings.TrimRight(value, "0"), ".")
	}
	return value
}

func normalizeCurrencyToken(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "CHF", "瑞士法郎":
		return "CHF"
	default:
		return "RMB"
	}
}

func formatCurrencyRat(value *big.Rat) string {
	formatted := value.FloatString(2)
	formatted = strings.TrimRight(strings.TrimRight(formatted, "0"), ".")
	return formatted
}

func questionUsesUnspecifiedFinancialAmount(question string) bool {
	upper := strings.ToUpper(question)
	for _, token := range []string{"CHF", "RMB", "CNY", "人民币", "瑞士法郎", "美元", "USD", "欧元", "EUR"} {
		if strings.Contains(upper, strings.ToUpper(token)) {
			return false
		}
	}
	amounts := bareAmountPattern.FindAllStringSubmatchIndex(question, -1)
	if len(amounts) == 0 {
		return false
	}
	markers := []string{"金额", "费用", "报销", "限额", "预算", "价格", "付款", "支付", "成本", "amount", "expense", "cost", "price", "budget", "payment"}
	lower := strings.ToLower(question)
	for _, indexes := range amounts {
		if len(indexes) < 4 || indexes[2] < 0 || indexes[3] < 0 {
			continue
		}
		amountEnd := indexes[3]
		if amountHasNonCurrencyUnit(question[amountEnd:]) {
			continue
		}
		for _, marker := range markers {
			if strings.Contains(lower, marker) {
				return true
			}
		}
	}
	return false
}

func amountHasNonCurrencyUnit(suffix string) bool {
	suffix = strings.TrimSpace(suffix)
	for _, unit := range []string{"年", "月", "日", "天", "次", "人", "%", "％"} {
		if strings.HasPrefix(suffix, unit) {
			return true
		}
	}
	return false
}
