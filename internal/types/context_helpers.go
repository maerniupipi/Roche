package types

import (
	"context"
	"os"
	"strings"
)

// EnvLanguage returns the ROCHE_KAP_LANGUAGE environment variable value, or empty string if unset.
func EnvLanguage() string {
	return strings.TrimSpace(os.Getenv("ROCHE_KAP_LANGUAGE"))
}

// DefaultLanguage returns the configured default language locale.
// It reads the ROCHE_KAP_LANGUAGE environment variable; if unset, falls back to "zh-CN".
func DefaultLanguage() string {
	if lang := EnvLanguage(); lang != "" {
		return lang
	}
	return "zh-CN"
}

// KnowledgeDomainIDFromContext extracts the knowledge-domain ID from ctx.
// Returns (0, false) when the key is absent or the value is not uint64.
func KnowledgeDomainIDFromContext(ctx context.Context) (uint64, bool) {
	v, ok := ctx.Value(KnowledgeDomainIDContextKey).(uint64)
	return v, ok
}

// MustKnowledgeDomainIDFromContext extracts the knowledge-domain ID from ctx, panicking if missing.
func MustKnowledgeDomainIDFromContext(ctx context.Context) uint64 {
	v, ok := KnowledgeDomainIDFromContext(ctx)
	if !ok {
		panic("types.KnowledgeDomainIDContextKey not set in context")
	}
	return v
}

// KnowledgeDomainInfoFromContext extracts the *KnowledgeDomain from ctx.
func KnowledgeDomainInfoFromContext(ctx context.Context) (*KnowledgeDomain, bool) {
	v, ok := ctx.Value(KnowledgeDomainInfoContextKey).(*KnowledgeDomain)
	return v, ok && v != nil
}

// RequestIDFromContext extracts the request ID string from ctx.
func RequestIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(RequestIDContextKey).(string)
	return v, ok && v != ""
}

// UserIDFromContext extracts the user ID string from ctx.
func UserIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(UserIDContextKey).(string)
	return v, ok && v != ""
}

// UserFromContext extracts the full *User from ctx. Returns (nil, false)
// when the key is absent — callers that only need the ID should prefer
// UserIDFromContext to stay independent of hydration depth.
func UserFromContext(ctx context.Context) (*User, bool) {
	v, ok := ctx.Value(UserContextKey).(*User)
	return v, ok && v != nil
}

// ActorNameFromContext extracts the actor's username from ctx (the
// *types.User stored by Auth / InternalServiceAuth). Returns "" when no
// hydrated user is present so audit writers can default the 操作人 name
// column without nil checks at each call site.
func ActorNameFromContext(ctx context.Context) string {
	if user, ok := UserFromContext(ctx); ok {
		return user.Username
	}
	return ""
}

// WithClientIP returns a ctx carrying the 操作人发起请求的 IP. Injected by
// the RequestID middleware (from gin's c.ClientIP()) so audit write paths
// can stamp the actor IP without each call site reaching into gin.
func WithClientIP(ctx context.Context, ip string) context.Context {
	if ip == "" {
		return ctx
	}
	return context.WithValue(ctx, ClientIPContextKey, ip)
}

// ClientIPFromContext extracts the 操作人发起请求的 IP from ctx. Returns ""
// when the key is absent or empty — audit writers treat "" as "no IP
// available" (e.g. background jobs, tests).
func ClientIPFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, ok := ctx.Value(ClientIPContextKey).(string)
	if !ok || v == "" {
		return ""
	}
	return v
}

// IsSystemAdminFromContext extracts the system admin flag from ctx.
// Returns false (fail-closed) when the key is absent.
func IsSystemAdminFromContext(ctx context.Context) bool {
	v, ok := ctx.Value(SystemAdminContextKey).(bool)
	if !ok {
		return false
	}
	return v
}

// IsKnowledgeOfficerFromContext extracts the knowledge officer role flag from ctx.
// Returns false (fail-closed) when the key is absent or not int.
func IsKnowledgeOfficerFromContext(ctx context.Context) bool {
	v, ok := ctx.Value(KnowledgeOfficerContextKey).(int)
	if !ok {
		return false
	}
	return v == RoleFlagTrue
}

// WithMCPOAuthNonInteractive marks ctx as originating from a channel that cannot
// complete an in-conversation MCP OAuth prompt (e.g. an IM bot). The agent uses
// this to emit a one-shot authorization notice instead of blocking on the OAuth
// wait until it times out. See MCPOAuthNonInteractiveContextKey.
func WithMCPOAuthNonInteractive(ctx context.Context) context.Context {
	return context.WithValue(ctx, MCPOAuthNonInteractiveContextKey, true)
}

// IsMCPOAuthNonInteractive reports whether ctx was marked non-interactive for
// MCP OAuth (see WithMCPOAuthNonInteractive).
func IsMCPOAuthNonInteractive(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(MCPOAuthNonInteractiveContextKey).(bool)
	return v
}

// LanguageFromContext extracts the language locale string from ctx (e.g. "zh-CN", "en-US").
// Returns ("zh-CN", false) when the key is absent.
func LanguageFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(LanguageContextKey).(string)
	return v, ok && v != ""
}

// LanguageNameFromContext returns the human-readable language name for use in prompts.
// e.g. "zh-CN" -> "Chinese (Simplified)", "en-US" -> "English", "ko-KR" -> "Korean"
// Falls back to DefaultLanguage() (ROCHE_KAP_LANGUAGE env, then "zh-CN").
func LanguageNameFromContext(ctx context.Context) string {
	lang, ok := LanguageFromContext(ctx)
	if !ok {
		lang = DefaultLanguage()
	}
	return LanguageLocaleName(lang)
}

// LanguageLocaleName maps a locale code to a human-readable language name for LLM prompts.
func LanguageLocaleName(locale string) string {
	switch locale {
	case "zh-CN", "zh", "zh-Hans":
		return "Chinese (Simplified)"
	case "zh-TW", "zh-HK", "zh-Hant":
		return "Chinese (Traditional)"
	case "en-US", "en", "en-GB":
		return "English"
	case "ko-KR", "ko":
		return "Korean"
	case "ja-JP", "ja":
		return "Japanese"
	case "ru-RU", "ru":
		return "Russian"
	case "fr-FR", "fr":
		return "French"
	case "de-DE", "de":
		return "German"
	case "es-ES", "es":
		return "Spanish"
	case "pt-BR", "pt":
		return "Portuguese"
	default:
		// For unknown locales, return the locale itself
		return locale
	}
}
