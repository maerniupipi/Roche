package types

// ContextKey defines a type for context keys to avoid string collision
type ContextKey string

const (
	// KnowledgeDomainIDContextKey is the context key for the target knowledgeDomain.
	KnowledgeDomainIDContextKey ContextKey = "KnowledgeDomainID"
	// KnowledgeDomainInfoContextKey is the context key for knowledge-domain information.
	KnowledgeDomainInfoContextKey ContextKey = "KnowledgeDomainInfo"
	// RequestIDContextKey is the context key for request ID
	RequestIDContextKey ContextKey = "RequestID"
	// LoggerContextKey is the context key for logger
	LoggerContextKey ContextKey = "Logger"
	// UserContextKey is the context key for user information
	UserContextKey ContextKey = "User"
	// UserIDContextKey is the context key for user ID
	UserIDContextKey ContextKey = "UserID"
	// PrincipalContextKey is the context key for the terminal caller principal.
	PrincipalContextKey ContextKey = "Principal"
	// EmbedQueryContextKey is the context key for embedding query text
	EmbedQueryContextKey ContextKey = "EmbedQuery"
	// LanguageContextKey is the context key for user language preference (e.g. "zh-CN", "en-US")
	LanguageContextKey ContextKey = "Language"
	// LangfuseTraceContextKey carries the active Langfuse *Trace across the
	// request lifecycle. Defined here (not inside the langfuse package) so
	// that logger.CloneContext can preserve it without importing langfuse.
	LangfuseTraceContextKey ContextKey = "LangfuseTrace"
	// SystemAdminContextKey is the context key indicating whether the user is a system administrator
	SystemAdminContextKey ContextKey = "SystemAdmin"
	// KnowledgeOfficerContextKey is the context key indicating whether the user has knowledge officer role
	KnowledgeOfficerContextKey ContextKey = "KnowledgeOfficer"
	// MCPOAuthNonInteractiveContextKey marks a request whose channel cannot
	// resolve an in-conversation MCP OAuth prompt (e.g. an IM bot: there is no
	// live client to click "Authorize" and call the resolve endpoint). When set,
	// the agent emits a one-shot authorization notice and continues instead of
	// blocking until the OAuth wait times out. See IsMCPOAuthNonInteractive.
	MCPOAuthNonInteractiveContextKey ContextKey = "MCPOAuthNonInteractive"
	// ClientIPContextKey carries the 操作人发起请求的 IP for the current
	// request. Injected by the RequestID middleware (from c.ClientIP()) so
	// every audit write path can stamp the actor's IP without each call site
	// reaching into gin. Read via ClientIPFromContext.
	ClientIPContextKey ContextKey = "ClientIP"
)

// String returns the string representation of the context key
func (c ContextKey) String() string {
	return string(c)
}
