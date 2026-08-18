package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	"roche.local/knowledge-agent-platform/internal/types"
)

// Config 应用程序总配置
type Config struct {
	Conversation    *ConversationConfig    `yaml:"conversation"     json:"conversation"`
	Server          *ServerConfig          `yaml:"server"           json:"server"`
	KnowledgeBase   *KnowledgeBaseConfig   `yaml:"knowledge_base"   json:"knowledge_base"`
	Audit           *AuditConfig           `yaml:"audit"            json:"audit"`
	LocalAuth       *LocalAuthConfig       `yaml:"local_auth"       json:"local_auth"`
	Registration    *RegistrationConfig    `yaml:"registration"     json:"registration"`
	OIDCAuth        *OIDCAuthConfig        `yaml:"oidc_auth"        json:"oidc_auth"`
	SAMLAuth        *SAMLAuthConfig        `yaml:"saml_auth"        json:"saml_auth"`
	Workday         *WorkdayConfig         `yaml:"workday"          json:"workday"`
	InternalService *InternalServiceConfig `yaml:"internal_service" json:"internal_service"`
	Models          []ModelConfig          `yaml:"models"           json:"models"`
	VectorDatabase  *VectorDatabaseConfig  `yaml:"vector_database"  json:"vector_database"`
	DocReader       *DocReaderConfig       `yaml:"docreader"        json:"docreader"`
	StreamManager   *StreamManagerConfig   `yaml:"stream_manager"   json:"stream_manager"`
	ExtractManager  *ExtractManagerConfig  `yaml:"extract"          json:"extract"`
	WebSearch       *WebSearchConfig       `yaml:"web_search"       json:"web_search"`
	PromptTemplates *PromptTemplatesConfig `yaml:"prompt_templates" json:"prompt_templates"`
	Agent           *AgentConfig           `yaml:"agent"            json:"agent"`
	UnifiedQA       *UnifiedQAConfig       `yaml:"unified_qa"       json:"unified_qa"`
	Features        *FeatureConfig         `yaml:"features"         json:"features"`
}

// UnifiedQAConfig contains model-selection overrides for the unified Q&A
// workflow. The route model may be a small OpenAI-compatible model configured
// directly here; the summary model remains an ID from the models table.
type UnifiedQAConfig struct {
	RouteModel     *UnifiedQARouteModelConfig `yaml:"route_model" json:"route_model"`
	SummaryModelID string                     `yaml:"summary_model_id" json:"summary_model_id"`
}

// UnifiedQARouteModelConfig describes the small, OpenAI-compatible model used
// when the models table does not designate a dedicated unified-QA router.
type UnifiedQARouteModelConfig struct {
	ID           string `yaml:"id" json:"id"`
	ModelName    string `yaml:"model_name" json:"model_name"`
	BaseURL      string `yaml:"base_url" json:"base_url"`
	APIKey       string `yaml:"api_key" json:"api_key"`
	Provider     string `yaml:"provider" json:"provider"`
	OutputSchema string `yaml:"output_schema" json:"output_schema"`
}

// FeatureConfig controls optional product modules. Enterprise builds default
// these modules off; an operator must explicitly opt in.
type FeatureConfig struct {
	MCP    bool `yaml:"mcp"    json:"mcp"`
	Skills bool `yaml:"skills" json:"skills"`
}

func (c *Config) IsMCPEnabled() bool {
	return c != nil && c.Features != nil && c.Features.MCP
}

func (c *Config) AreSkillsEnabled() bool {
	return c != nil && c.Features != nil && c.Features.Skills
}

// AgentConfig represents the global agent settings.
type AgentConfig struct {
	// LLMCallTimeout is the default timeout for a single LLM call in seconds.
	// Default: 120 (standard agents) or 300 (can be overridden by Env).
	LLMCallTimeout int `yaml:"llm_call_timeout" json:"llm_call_timeout"`
	// ToolApprovalTimeoutSeconds is how long the agent waits for human approval on a flagged MCP tool.
	// 0 means default 600 (10 minutes).
	ToolApprovalTimeoutSeconds int `yaml:"tool_approval_timeout_seconds" json:"tool_approval_timeout_seconds"`
}

// DocReaderConfig configures the document parser client (gRPC or HTTP).
type DocReaderConfig struct {
	// Addr: for gRPC it is the server address (e.g. "localhost:50051"); for HTTP it is the base URL (e.g. "http://localhost:8080").
	Addr string `yaml:"addr" json:"addr"`
	// Transport: "grpc" (default) or "http"
	Transport string `yaml:"transport" json:"transport"`
}

type VectorDatabaseConfig struct {
	Driver string `yaml:"driver" json:"driver"`
}

// ConversationConfig 对话服务配置
type ConversationConfig struct {
	MaxRounds            int            `yaml:"max_rounds"                       json:"max_rounds"`
	KeywordThreshold     float64        `yaml:"keyword_threshold"                json:"keyword_threshold"`
	EmbeddingTopK        int            `yaml:"embedding_top_k"                  json:"embedding_top_k"`
	VectorThreshold      float64        `yaml:"vector_threshold"                 json:"vector_threshold"`
	RerankTopK           int            `yaml:"rerank_top_k"                     json:"rerank_top_k"`
	RerankThreshold      float64        `yaml:"rerank_threshold"                 json:"rerank_threshold"`
	FallbackStrategy     string         `yaml:"fallback_strategy"                json:"fallback_strategy"`
	FallbackResponse     string         `yaml:"fallback_response"                json:"fallback_response"`
	EnableRewrite        bool           `yaml:"enable_rewrite"                   json:"enable_rewrite"`
	EnableQueryExpansion bool           `yaml:"enable_query_expansion"           json:"enable_query_expansion"`
	EnableRerank         bool           `yaml:"enable_rerank"                    json:"enable_rerank"`
	Summary              *SummaryConfig `yaml:"summary"                          json:"summary"`

	// Prompt template ID fields — resolved to text by backfillConversationDefaults
	FallbackPromptID             string `yaml:"fallback_prompt_id"                json:"fallback_prompt_id"`
	RewritePromptID              string `yaml:"rewrite_prompt_id"                 json:"rewrite_prompt_id"`
	GenerateSessionTitlePromptID string `yaml:"generate_session_title_prompt_id"  json:"generate_session_title_prompt_id"`
	GenerateSummaryPromptID      string `yaml:"generate_summary_prompt_id"        json:"generate_summary_prompt_id"`
	ExtractEntitiesPromptID      string `yaml:"extract_entities_prompt_id"        json:"extract_entities_prompt_id"`
	ExtractRelationshipsPromptID string `yaml:"extract_relationships_prompt_id"   json:"extract_relationships_prompt_id"`
	GenerateQuestionsPromptID    string `yaml:"generate_questions_prompt_id"      json:"generate_questions_prompt_id"`

	// Resolved prompt text fields (populated by backfill, not from YAML)
	FallbackPrompt             string `yaml:"-" json:"fallback_prompt"`
	RewritePromptSystem        string `yaml:"-" json:"rewrite_prompt_system"`
	RewritePromptUser          string `yaml:"-" json:"rewrite_prompt_user"`
	GenerateSessionTitlePrompt string `yaml:"-" json:"generate_session_title_prompt"`
	GenerateSummaryPrompt      string `yaml:"-" json:"generate_summary_prompt"`
	ExtractEntitiesPrompt      string `yaml:"-" json:"extract_entities_prompt"`
	ExtractRelationshipsPrompt string `yaml:"-" json:"extract_relationships_prompt"`
	GenerateQuestionsPrompt    string `yaml:"-" json:"generate_questions_prompt"`

	// IntentSystemPrompts maps intent values (e.g. "greeting", "chitchat") to
	// system prompt text. Populated by backfill from IntentPrompts templates.
	IntentSystemPrompts map[string]string `yaml:"-" json:"-"`
}

// SummaryConfig 摘要配置
type SummaryConfig struct {
	MaxInputChars       int     `yaml:"max_input_chars"       json:"max_input_chars"` // Max input characters for summary generation (default: 16384)
	MaxTokens           int     `yaml:"max_tokens"            json:"max_tokens"`
	RepeatPenalty       float64 `yaml:"repeat_penalty"        json:"repeat_penalty"`
	TopK                int     `yaml:"top_k"                 json:"top_k"`
	TopP                float64 `yaml:"top_p"                 json:"top_p"`
	FrequencyPenalty    float64 `yaml:"frequency_penalty"     json:"frequency_penalty"`
	PresencePenalty     float64 `yaml:"presence_penalty"      json:"presence_penalty"`
	Temperature         float64 `yaml:"temperature"           json:"temperature"`
	Seed                int     `yaml:"seed"                  json:"seed"`
	MaxCompletionTokens int     `yaml:"max_completion_tokens" json:"max_completion_tokens"`
	NoMatchPrefix       string  `yaml:"no_match_prefix"       json:"no_match_prefix"`
	Thinking            *bool   `yaml:"thinking"              json:"thinking"`

	// Prompt template ID fields — resolved to text by backfillConversationDefaults
	PromptID          string `yaml:"prompt_id"           json:"prompt_id"`
	ContextTemplateID string `yaml:"context_template_id" json:"context_template_id"`

	// Resolved prompt text fields (populated by backfill, not from YAML)
	Prompt          string `yaml:"-" json:"prompt"`
	ContextTemplate string `yaml:"-" json:"context_template"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Port            int           `yaml:"port"             json:"port"`
	Host            string        `yaml:"host"             json:"host"`
	LogPath         string        `yaml:"log_path"         json:"log_path"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout" json:"shutdown_timeout" default:"30s"`
}

// KnowledgeBaseConfig 知识库配置
type KnowledgeBaseConfig struct {
	ChunkSize              int                    `yaml:"chunk_size"       json:"chunk_size"`
	ChunkOverlap           int                    `yaml:"chunk_overlap"    json:"chunk_overlap"`
	SplitMarkers           []string               `yaml:"split_markers"    json:"split_markers"`
	KeepSeparator          bool                   `yaml:"keep_separator"   json:"keep_separator"`
	ImageProcessing        *ImageProcessingConfig `yaml:"image_processing" json:"image_processing"`
	DocumentProcessTimeout time.Duration          `yaml:"document_process_timeout"  json:"document_process_timeout"`
	// DocReaderCallTimeout caps a single DocReader RPC. Without this the
	// gRPC call inherits the asynq task context (whole DocumentProcessTimeout,
	// default 2h+), so a hung docreader would block a worker for hours and
	// leave knowledge in "processing". Default 30 minutes is generous enough
	// for OCR-heavy large PDFs while ensuring forward progress.
	DocReaderCallTimeout time.Duration `yaml:"docreader_call_timeout"   json:"docreader_call_timeout"`
}

// DefaultDocumentProcessTimeout is the ceiling for a single document:process
// Asynq task when document_process_timeout is unset or non-positive.
const DefaultDocumentProcessTimeout = 2 * time.Hour

// DocumentProcessTimeout returns the effective document-process task timeout.
// Partial configs (e.g. unit tests) receive the default when unset.
func DocumentProcessTimeout(cfg *Config) time.Duration {
	if cfg != nil && cfg.KnowledgeBase != nil && cfg.KnowledgeBase.DocumentProcessTimeout > 0 {
		return cfg.KnowledgeBase.DocumentProcessTimeout
	}
	return DefaultDocumentProcessTimeout
}

// ImageProcessingConfig 图像处理配置
type ImageProcessingConfig struct {
	EnableMultimodal bool `yaml:"enable_multimodal" json:"enable_multimodal"`
}

// AuditConfig governs durable audit log behaviour. Writes happen on
// access-control mutations and denials; the table grows monotonically unless
// this section turns on retention.
type AuditConfig struct {
	// RetentionDays is how many days of audit history to keep. Older
	// rows are deleted by a daily background sweep.
	//   > 0 — purge rows whose created_at < NOW() - retention_days.
	//   = 0 — disable purge entirely (the pre-rollout default).
	//   < 0 — invalid; ValidateConfig rejects it.
	// Default: 90 (set by applyAuditDefaults when the section is omitted).
	RetentionDays int `yaml:"retention_days" json:"retention_days"`
	// Global controls the automatic per-request audit middleware. When
	// enabled, every authenticated API request is recorded as an
	// http.request audit row. Off by default to preserve existing
	// behaviour; operators opt in explicitly.
	Global *GlobalAuditConfig `yaml:"global" json:"global"`
}

// GlobalAuditConfig controls the automatic per-request audit recorder.
// When enabled in config.yaml under audit.global, every authenticated
// API request produces an http.request row in the audit_logs table.
// The recorder is async (buffered channel + background goroutines) so
// it adds negligible latency to the hot path.
type GlobalAuditConfig struct {
	// Enable toggles global request audit recording. Default: false.
	// Set to true explicitly to start recording all API calls.
	Enabled bool `yaml:"enabled" json:"enabled"`
	// CaptureBody controls whether POST/PUT/PATCH request bodies are
	// included in the details JSONB payload. Bodies are truncated at
	// 8192 bytes and sensitive fields (password, token, etc.) are
	// redacted before storage. Default: true.
	CaptureBody bool `yaml:"capture_body" json:"capture_body"`
	// RecordGET controls whether GET requests are also recorded. GETs
	// are read-only and can generate significant volume; disable when
	// only mutation audit is required. Default: false.
	RecordGET bool `yaml:"record_get" json:"record_get"`
}

type OIDCUserInfoMapping struct {
	Username   string `yaml:"username"    json:"username"`
	Email      string `yaml:"email"       json:"email"`
	EmployeeID string `yaml:"employee_id" json:"employee_id"`
}

// RegistrationConfig controls public email/password registration. Role
// selection is deliberately a separate development switch: production
// deployments should leave it disabled so self-registration always receives
// the least-privilege default role.
type RegistrationConfig struct {
	Enable           bool                   `yaml:"enable"             json:"enable"`
	DefaultRole      types.RegistrationRole `yaml:"default_role"       json:"default_role"`
	DevRoleSelection bool                   `yaml:"dev_role_selection" json:"dev_role_selection"`
}

// LocalAuthConfig controls email/password authentication independently from
// public registration. Production SSO-only deployments disable this while
// development environments can keep password login available.
type LocalAuthConfig struct {
	PasswordLoginEnable bool `yaml:"password_login_enable" json:"password_login_enable"`
}

type OIDCAuthConfig struct {
	Enable                bool                 `yaml:"enable"                 json:"enable"`
	IssuerURL             string               `yaml:"issuer_url"             json:"issuer_url"`
	DiscoveryURL          string               `yaml:"discovery_url"          json:"discovery_url"`
	ProviderDisplayName   string               `yaml:"provider_display_name"  json:"provider_display_name"`
	ClientID              string               `yaml:"client_id"              json:"client_id"`
	ClientSecret          string               `yaml:"client_secret"          json:"-"`
	AuthorizationEndpoint string               `yaml:"authorization_endpoint" json:"authorization_endpoint"`
	TokenEndpoint         string               `yaml:"token_endpoint"         json:"token_endpoint"`
	UserInfoEndpoint      string               `yaml:"user_info_endpoint"     json:"user_info_endpoint"`
	Scopes                []string             `yaml:"scopes"                 json:"scopes"`
	UserInfoMapping       *OIDCUserInfoMapping `yaml:"user_info_mapping"      json:"user_info_mapping"`
	AutoProvision         bool                 `yaml:"auto_provision"         json:"auto_provision"`
}

// SAMLUserInfoMapping maps SAML assertion attributes to local user fields.
// Values are the friendly names (or full urn:oid:... URIs) of the SAML
// AttributeStatement attributes emitted by the IdP. If a mapped attribute is
// absent, the adapter falls back to the assertion NameID for the subject and
// the email local-part for the username.
type SAMLUserInfoMapping struct {
	Subject    string `yaml:"subject"     json:"subject"`
	Username   string `yaml:"username"    json:"username"`
	Email      string `yaml:"email"       json:"email"`
	EmployeeID string `yaml:"employee_id" json:"employee_id"`
}

// SAMLAuthConfig configures the service-provider side of SAML 2.0 single
// sign-on. The application always acts as the SP; the enterprise identity
// provider (IdP) is described by its SAML metadata.
type SAMLAuthConfig struct {
	// Enable toggles the SAML login entry point on the login page.
	Enable bool `yaml:"enable" json:"enable"`
	// IdPMetadataURL is a remote URL from which the IdP metadata XML is
	// fetched at startup (SSRF-validated). Mutually exclusive with
	// IdPMetadataFile / IdPMetadata.
	IdPMetadataURL string `yaml:"idp_metadata_url" json:"idp_metadata_url"`
	// IdPMetadataFile is a local path to the IdP metadata XML.
	IdPMetadataFile string `yaml:"idp_metadata_file" json:"idp_metadata_file"`
	// IdPMetadata is the raw IdP metadata XML embedded in configuration.
	IdPMetadata string `yaml:"idp_metadata" json:"idp_metadata"`
	// SPEntityID is this service's SAML entity ID advertised in metadata.
	SPEntityID string `yaml:"sp_entity_id" json:"sp_entity_id"`
	// ACSUrl is the absolute URL of the assertion consumer service
	// endpoint, e.g. https://kap.example.com/api/v1/auth/saml/acs.
	// The SP metadata URL is derived from it by replacing the "acs"
	// segment with "metadata".
	ACSUrl string `yaml:"acs_url" json:"acs_url"`
	// SPCert / SPKey are the PEM-encoded SP certificate and private key.
	// Prefer SPCertFile / SPKeyFile in deployment. An ephemeral self-signed
	// keypair is generated only when AllowEphemeralSPCert is explicitly enabled.
	SPCert string `yaml:"sp_cert" json:"-"`
	SPKey  string `yaml:"sp_key"  json:"-"`
	// SPCertFile / SPKeyFile are file paths to the PEM certificate and key.
	SPCertFile string `yaml:"sp_cert_file" json:"sp_cert_file"`
	SPKeyFile  string `yaml:"sp_key_file"  json:"sp_key_file"`
	// AllowEphemeralSPCert permits an in-memory self-signed SP certificate.
	// It is intended only for explicit development environments.
	AllowEphemeralSPCert bool `yaml:"allow_ephemeral_sp_cert" json:"allow_ephemeral_sp_cert"`
	// ProviderDisplayName is shown on the login page SSO entry.
	ProviderDisplayName string `yaml:"provider_display_name" json:"provider_display_name"`
	// AutoProvision creates a local account on first SAML login when true;
	// otherwise the account must exist (or be pre-provisioned) and SAML
	// login only links/authenticates the existing identity.
	AutoProvision bool `yaml:"auto_provision" json:"auto_provision"`
	// DevSystemAdminEmails is a development-only bootstrap allowlist. A newly
	// auto-provisioned SAML user whose normalized email is listed here becomes
	// a platform system administrator. It is ignored for existing users so
	// later role changes made in the application remain authoritative.
	DevSystemAdminEmails []string `yaml:"dev_system_admin_emails" json:"-"`
	// SignRequest signs outgoing AuthnRequests with the SP key.
	SignRequest bool `yaml:"sign_request" json:"sign_request"`
	// AllowIDPInitiated relaxes the InResponseTo check so the IdP can push
	// unsolicited assertions. When false a valid RelayState round-trip is
	// still required for browser-bound flows.
	AllowIDPInitiated bool `yaml:"allow_idp_initiated" json:"allow_idp_initiated"`
	// UserInfoMapping maps SAML attribute names to local fields.
	UserInfoMapping *SAMLUserInfoMapping `yaml:"user_info_mapping" json:"user_info_mapping"`
}

// WorkdayConfig describes the provider-neutral enterprise directory adapter.
// Secrets are accepted only through environment variables and are omitted from
// JSON so configuration inspection cannot disclose them.
type WorkdayConfig struct {
	Enable         bool          `yaml:"enable"              json:"enable"`
	Provider       string        `yaml:"provider"            json:"provider"`
	ConnectionKey  string        `yaml:"connection_key"      json:"connection_key"`
	MockFile       string        `yaml:"mock_file"           json:"mock_file,omitempty"`
	BaseURL        string        `yaml:"base_url"            json:"base_url,omitempty"`
	OrgUnitsPath   string        `yaml:"org_units_path"      json:"org_units_path,omitempty"`
	WorkersPath    string        `yaml:"workers_path"        json:"workers_path,omitempty"`
	TokenURL       string        `yaml:"token_url"           json:"token_url,omitempty"`
	ClientID       string        `yaml:"client_id"           json:"client_id,omitempty"`
	ClientSecret   string        `yaml:"client_secret"       json:"-"`
	Scope          string        `yaml:"scope"               json:"scope,omitempty"`
	PageSize       int           `yaml:"page_size"           json:"page_size"`
	// Pagination selects the query parameter style used to page the remote
	// API: "cursor" (default; cursor + page_size) or "offset" (offset + limit,
	// used by the Roche employee API: GET /employees?offset=1&limit=100).
	Pagination     string        `yaml:"pagination"          json:"pagination,omitempty"`
	RequestTimeout time.Duration `yaml:"request_timeout" json:"request_timeout"`
	// SyncOrgUnits controls whether a synchronization run also fetches
	// organizations. nil means enabled (default); set to false to sync workers
	// only, for example when the organization API is not available yet.
	SyncOrgUnits *bool `yaml:"sync_org_units" json:"sync_org_units,omitempty"`
}

// SyncOrgUnitsEnabled reports whether organization synchronization is enabled.
// It defaults to true so existing deployments keep their current behavior.
func (c *WorkdayConfig) SyncOrgUnitsEnabled() bool {
	return c == nil || c.SyncOrgUnits == nil || *c.SyncOrgUnits
}

// InternalServiceConfig holds the configuration for internal service-to-service
// communication, such as the GDrive sync worker calling the upload API.
// The Token is accepted only through environment variables and is omitted from
// JSON so configuration inspection cannot disclose it.
type InternalServiceConfig struct {
	BaseURL string `yaml:"base_url" json:"base_url"`
	Token   string `yaml:"token"   json:"-"`
}

// PromptTemplateI18n holds localized name and description for a prompt template.
type PromptTemplateI18n struct {
	Name        string `yaml:"name"        json:"name"`
	Description string `yaml:"description" json:"description"`
}

// PromptTemplate 提示词模板
//
// 字段设计：每个模板最多由两部分组成 —— 系统侧 (content) 和用户侧 (user)。
//   - content: 主要内容 / 系统 Prompt（所有模板都使用此字段）
//   - user:    用户侧 Prompt（仅在需要 system+user 配对的模板中使用，如 rewrite、keywords_extraction）
//   - i18n:    多语言 name/description，键为 locale（如 "zh-CN"、"en-US"、"ko-KR"），后端根据请求语言替换 Name/Description 再返回
type PromptTemplate struct {
	ID               string                        `yaml:"id"                 json:"id"`
	Name             string                        `yaml:"name"               json:"name"`
	Description      string                        `yaml:"description"        json:"description"`
	Content          string                        `yaml:"content"            json:"content"`
	User             string                        `yaml:"user"               json:"user,omitempty"`
	HasKnowledgeBase bool                          `yaml:"has_knowledge_base" json:"has_knowledge_base,omitempty"`
	HasWebSearch     bool                          `yaml:"has_web_search"     json:"has_web_search,omitempty"`
	Default          bool                          `yaml:"default"            json:"default,omitempty"`
	Mode             string                        `yaml:"mode"               json:"mode,omitempty"`
	I18n             map[string]PromptTemplateI18n `yaml:"i18n"               json:"-"`
}

// PromptTemplatesConfig 提示词模板配置
//
// 每种 Prompt 类型对应一个 YAML 文件，所有模板都在同一个字段（文件）中管理。
// 每个模板使用 content (system prompt) + user (user prompt) 两个字段。
type PromptTemplatesConfig struct {
	SystemPrompt    []PromptTemplate `yaml:"system_prompt"    json:"system_prompt"`
	ContextTemplate []PromptTemplate `yaml:"context_template" json:"context_template"`
	// Rewrite 合并了前端可选模板和运行时默认模板，每个模板同时包含 content + user
	Rewrite []PromptTemplate `yaml:"rewrite" json:"rewrite"`
	// Fallback 合并了固定回复模板和模型兜底 prompt（通过 mode:"model" 区分）
	Fallback []PromptTemplate `yaml:"fallback" json:"fallback"`

	GenerateSessionTitle []PromptTemplate `yaml:"generate_session_title" json:"generate_session_title,omitempty"`
	GenerateSummary      []PromptTemplate `yaml:"generate_summary"       json:"generate_summary,omitempty"`
	KeywordsExtraction   []PromptTemplate `yaml:"keywords_extraction"    json:"keywords_extraction,omitempty"`
	AgentSystemPrompt    []PromptTemplate `yaml:"agent_system_prompt"    json:"agent_system_prompt,omitempty"`
	GraphExtraction      []PromptTemplate `yaml:"graph_extraction"       json:"graph_extraction,omitempty"`
	GenerateQuestions    []PromptTemplate `yaml:"generate_questions"     json:"generate_questions,omitempty"`
	// IntentPrompts holds per-intent system prompt overrides (template ID = intent value).
	IntentPrompts []PromptTemplate `yaml:"intent_prompts" json:"intent_prompts,omitempty"`
	// UnifiedQA contains internal prompts for the configured multi-agent workflow.
	UnifiedQA []PromptTemplate `yaml:"unified_qa" json:"-"`
}

// DefaultTemplate returns the first template marked as default in the list,
// or the first template if none is marked, or nil if the list is empty.
func DefaultTemplate(templates []PromptTemplate) *PromptTemplate {
	for i := range templates {
		if templates[i].Default {
			return &templates[i]
		}
	}
	if len(templates) > 0 {
		return &templates[0]
	}
	return nil
}

// DefaultTemplateByMode returns the default template filtered by mode.
func DefaultTemplateByMode(templates []PromptTemplate, mode string) *PromptTemplate {
	for i := range templates {
		if templates[i].Mode == mode && templates[i].Default {
			return &templates[i]
		}
	}
	for i := range templates {
		if templates[i].Mode == mode {
			return &templates[i]
		}
	}
	return DefaultTemplate(templates)
}

// LocalizeTemplates returns a deep copy of the template list with Name and
// Description replaced according to the given locale.  Fallback chain:
//
//	locale → primary language (e.g. "zh" from "zh-CN") → original Name/Description.
//
// The returned slice is safe to serialise directly; it never mutates the original.
func LocalizeTemplates(templates []PromptTemplate, locale string) []PromptTemplate {
	if len(templates) == 0 {
		return templates
	}
	out := make([]PromptTemplate, len(templates))
	copy(out, templates)
	for i := range out {
		if len(out[i].I18n) == 0 {
			continue
		}
		// Try exact match first (e.g. "zh-CN"), then primary subtag (e.g. "zh")
		l10n, ok := out[i].I18n[locale]
		if !ok {
			if idx := strings.IndexByte(locale, '-'); idx > 0 {
				l10n, ok = out[i].I18n[locale[:idx]]
			}
		}
		if !ok {
			continue
		}
		if l10n.Name != "" {
			out[i].Name = l10n.Name
		}
		if l10n.Description != "" {
			out[i].Description = l10n.Description
		}
	}
	return out
}

// ModelConfig 模型配置
type ModelConfig struct {
	Type       string                 `yaml:"type"       json:"type"`
	Source     string                 `yaml:"source"     json:"source"`
	ModelName  string                 `yaml:"model_name" json:"model_name"`
	Parameters map[string]interface{} `yaml:"parameters" json:"parameters"`
}

// StreamManagerConfig 流管理器配置
type StreamManagerConfig struct {
	Type           string        `yaml:"type"            json:"type"`            // 类型: "memory" 或 "redis"
	Redis          RedisConfig   `yaml:"redis"           json:"redis"`           // Redis配置
	CleanupTimeout time.Duration `yaml:"cleanup_timeout" json:"cleanup_timeout"` // 清理超时，单位秒
}

// RedisConfig Redis配置
type RedisConfig struct {
	Address  string        `yaml:"address"  json:"address"`  // Redis地址
	Username string        `yaml:"username" json:"username"` // Redis用户名
	Password string        `yaml:"password" json:"password"` // Redis密码
	DB       int           `yaml:"db"       json:"db"`       // Redis数据库
	Prefix   string        `yaml:"prefix"   json:"prefix"`   // 键前缀
	TTL      time.Duration `yaml:"ttl"      json:"ttl"`      // 过期时间(小时)
}

// ExtractManagerConfig 抽取管理器配置
type ExtractManagerConfig struct {
	ExtractGraph  *types.PromptTemplateStructured `yaml:"extract_graph"  json:"extract_graph"`
	ExtractEntity *types.PromptTemplateStructured `yaml:"extract_entity" json:"extract_entity"`
	FabriText     *FebriText                      `yaml:"fabri_text"     json:"fabri_text"`
}

type FebriText struct {
	WithTag   string `yaml:"with_tag"    json:"with_tag"`
	WithNoTag string `yaml:"with_no_tag" json:"with_no_tag"`
}

// resolvedConfigDir holds the directory of the loaded config file. Populated by
// LoadConfig and read by ConfigDir(); empty until LoadConfig has run.
var resolvedConfigDir string

// ConfigDir returns the directory containing the loaded config.yaml. Other
// startup code (e.g. builtin model loader) uses this to locate sibling config
// files like builtin_models.yaml without re-implementing viper search rules.
// Falls back to "./config" when LoadConfig has not been called yet.
func ConfigDir() string {
	if resolvedConfigDir != "" {
		return resolvedConfigDir
	}
	if f := viper.ConfigFileUsed(); f != "" {
		return filepath.Dir(f)
	}
	return "./config"
}

// LoadConfig 从配置文件加载配置
func LoadConfig() (*Config, error) {
	// 设置配置文件名和路径
	viper.SetConfigName("config")         // 配置文件名称(不带扩展名)
	viper.SetConfigType("yaml")           // 配置文件类型
	viper.AddConfigPath(".")              // 当前目录
	viper.AddConfigPath("./config")       // config子目录
	viper.AddConfigPath("$HOME/.appname") // 用户目录
	viper.AddConfigPath("/etc/appname/")  // etc目录

	// 启用环境变量替换
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// 读取配置文件
	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	// 替换配置中的环境变量引用
	configFileContent, err := os.ReadFile(viper.ConfigFileUsed())
	if err != nil {
		return nil, fmt.Errorf("error reading config file content: %w", err)
	}

	// 替换${ENV_VAR}格式的环境变量引用
	re := regexp.MustCompile(`\${([^}]+)}`)
	result := re.ReplaceAllStringFunc(string(configFileContent), func(match string) string {
		// 提取环境变量名称（去掉${}部分）
		envVar := match[2 : len(match)-1]
		// 获取环境变量值，如果不存在则保持原样
		if value := os.Getenv(envVar); value != "" {
			return value
		}
		return match
	})

	// 使用处理后的配置内容
	viper.ReadConfig(strings.NewReader(result))

	// 解析配置到结构体
	var cfg Config
	if err := viper.Unmarshal(&cfg, func(dc *mapstructure.DecoderConfig) {
		dc.TagName = "yaml"
	}); err != nil {
		return nil, fmt.Errorf("unable to decode config into struct: %w", err)
	}
	fmt.Printf("Using configuration file: %s\n", viper.ConfigFileUsed())

	// 加载提示词模板（从目录或配置文件）
	configDir := filepath.Dir(viper.ConfigFileUsed())
	resolvedConfigDir = configDir
	promptTemplates, err := loadPromptTemplates(configDir)
	if err != nil {
		fmt.Printf("Warning: failed to load prompt templates from directory: %v\n", err)
		// 如果目录加载失败，使用配置文件中的模板（如果有）
	} else if promptTemplates != nil {
		cfg.PromptTemplates = promptTemplates
	}

	// Back-fill conversation config from prompt templates defaults
	// (so config.yaml can omit large prompt blocks and rely on template files)
	if cfg.PromptTemplates != nil && cfg.Conversation != nil {
		backfillConversationDefaults(&cfg)
	}

	// Load built-in agent definitions (i18n-aware) from builtin_agents.yaml
	if err := types.LoadBuiltinAgentsConfig(configDir); err != nil {
		fmt.Printf("Warning: failed to load builtin agents config: %v\n", err)
	}

	// Load smart-reasoning agent type presets.
	if err := types.LoadAgentTypePresetsConfig(configDir); err != nil {
		fmt.Printf("Warning: failed to load agent type presets: %v\n", err)
	}

	// Resolve prompt template ID references in builtin agent configs
	// (e.g. system_prompt_id -> actual content from agent_system_prompt.yaml)
	if cfg.PromptTemplates != nil {
		resolveBuiltinAgentPromptIDs(cfg.PromptTemplates)
		// Validate that every preset references an existing prompt template.
		types.ResolveAgentTypePresetPromptRefs(func(id string) string {
			if t := FindTemplateByID(cfg.PromptTemplates, id); t != nil {
				return t.Content
			}
			return ""
		})
	}

	// Validate configuration values
	applyLocalAuthEnvOverrides(&cfg)
	applyRegistrationEnvOverrides(&cfg)
	applyOIDCEnvOverrides(&cfg)
	applySAMLAuthDefaults(&cfg)
	applyWorkdayEnvOverrides(&cfg)
	applyAgentEnvOverrides(&cfg)
	applyUnifiedQAEnvOverrides(&cfg)
	applyKnowledgeBaseEnvOverrides(&cfg)
	applyFeatureOverrides(&cfg)
	applyAuditDefaults(&cfg)

	if err := ValidateConfig(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func applyFeatureOverrides(cfg *Config) {
	if cfg.Features == nil {
		cfg.Features = &FeatureConfig{}
	}
	for name, target := range map[string]*bool{
		"ROCHE_KAP_FEATURE_MCP":    &cfg.Features.MCP,
		"ROCHE_KAP_FEATURE_SKILLS": &cfg.Features.Skills,
	} {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			*target = strings.EqualFold(value, "true")
		}
	}
}

func applyLocalAuthEnvOverrides(cfg *Config) {
	if cfg.LocalAuth == nil {
		cfg.LocalAuth = &LocalAuthConfig{PasswordLoginEnable: true}
	}
	if value := strings.TrimSpace(os.Getenv("AUTH_PASSWORD_LOGIN_ENABLE")); value != "" {
		cfg.LocalAuth.PasswordLoginEnable = strings.EqualFold(value, "true")
	}
}

func applyRegistrationEnvOverrides(cfg *Config) {
	if cfg.Registration == nil {
		cfg.Registration = &RegistrationConfig{
			Enable:      true,
			DefaultRole: types.RegistrationRoleViewer,
		}
	}
	if cfg.Registration.DefaultRole == "" {
		cfg.Registration.DefaultRole = types.RegistrationRoleViewer
	}
	if value := strings.TrimSpace(os.Getenv("AUTH_REGISTRATION_ENABLE")); value != "" {
		cfg.Registration.Enable = strings.EqualFold(value, "true")
	}
	if value := strings.TrimSpace(os.Getenv("AUTH_REGISTRATION_DEFAULT_ROLE")); value != "" {
		cfg.Registration.DefaultRole = types.RegistrationRole(strings.ToLower(value))
	}
	if value := strings.TrimSpace(os.Getenv("AUTH_REGISTRATION_DEV_ROLE_SELECTION")); value != "" {
		cfg.Registration.DevRoleSelection = strings.EqualFold(value, "true")
	}
}

// ValidateConfig performs basic validation of the loaded configuration.
// It checks for obviously invalid or missing values that would cause runtime failures.
func ValidateConfig(cfg *Config) error {
	var errs []string

	if cfg.Registration != nil && cfg.Registration.Enable {
		switch cfg.Registration.DefaultRole {
		case types.RegistrationRoleViewer, types.RegistrationRoleSystemAdmin:
		default:
			errs = append(errs, "registration.default_role must be viewer or system_admin")
		}
	}

	if cfg.OIDCAuth != nil && cfg.OIDCAuth.Enable {
		if strings.TrimSpace(cfg.OIDCAuth.ClientID) == "" {
			errs = append(errs, "oidc_auth.client_id is required when OIDC is enabled")
		}
		if strings.TrimSpace(cfg.OIDCAuth.ClientSecret) == "" {
			errs = append(errs, "oidc_auth.client_secret is required when OIDC is enabled")
		}
		if strings.TrimSpace(cfg.OIDCAuth.DiscoveryURL) == "" &&
			(strings.TrimSpace(cfg.OIDCAuth.AuthorizationEndpoint) == "" ||
				strings.TrimSpace(cfg.OIDCAuth.TokenEndpoint) == "" ||
				strings.TrimSpace(cfg.OIDCAuth.UserInfoEndpoint) == "") {
			errs = append(
				errs,
				"oidc_auth.discovery_url or authorization_endpoint, token_endpoint and user_info_endpoint are required when OIDC is enabled",
			)
		}
	}

	if cfg.SAMLAuth != nil && cfg.SAMLAuth.Enable {
		if len(cfg.SAMLAuth.DevSystemAdminEmails) > 0 &&
			(cfg.Registration == nil || !cfg.Registration.DevRoleSelection) {
			errs = append(errs, "saml_auth.dev_system_admin_emails requires registration.dev_role_selection=true")
		}
		if strings.TrimSpace(cfg.SAMLAuth.IdPMetadataURL) == "" &&
			strings.TrimSpace(cfg.SAMLAuth.IdPMetadataFile) == "" &&
			strings.TrimSpace(cfg.SAMLAuth.IdPMetadata) == "" {
			errs = append(errs, "saml_auth.idp_metadata_url, idp_metadata_file or idp_metadata is required when SAML is enabled")
		}
		if strings.TrimSpace(cfg.SAMLAuth.SPEntityID) == "" {
			errs = append(errs, "saml_auth.sp_entity_id is required when SAML is enabled")
		}
		if strings.TrimSpace(cfg.SAMLAuth.ACSUrl) == "" {
			errs = append(errs, "saml_auth.acs_url is required when SAML is enabled")
		}
		hasRawCert := strings.TrimSpace(cfg.SAMLAuth.SPCert) != ""
		hasRawKey := strings.TrimSpace(cfg.SAMLAuth.SPKey) != ""
		hasCertFile := strings.TrimSpace(cfg.SAMLAuth.SPCertFile) != ""
		hasKeyFile := strings.TrimSpace(cfg.SAMLAuth.SPKeyFile) != ""
		if hasRawCert != hasRawKey {
			errs = append(errs, "saml_auth.sp_cert and sp_key must be configured together")
		}
		if hasCertFile != hasKeyFile {
			errs = append(errs, "saml_auth.sp_cert_file and sp_key_file must be configured together")
		}
		if !hasRawCert && !hasCertFile && !cfg.SAMLAuth.AllowEphemeralSPCert {
			errs = append(errs, "stable SAML SP certificate/key are required; ephemeral certificates must be explicitly enabled for development")
		}
	}

	if cfg.Workday != nil && cfg.Workday.Enable {
		switch cfg.Workday.Provider {
		case "mock":
			if strings.TrimSpace(cfg.Workday.MockFile) == "" {
				errs = append(errs, "workday.mock_file is required for the mock provider")
			}
		case "http":
			if strings.TrimSpace(cfg.Workday.BaseURL) == "" {
				errs = append(errs, "workday.base_url is required for the http provider")
			}
			if strings.TrimSpace(cfg.Workday.OrgUnitsPath) == "" ||
				strings.TrimSpace(cfg.Workday.WorkersPath) == "" {
				errs = append(errs, "workday.org_units_path and workday.workers_path are required for the http provider")
			}
			if strings.TrimSpace(cfg.Workday.TokenURL) != "" &&
				(strings.TrimSpace(cfg.Workday.ClientID) == "" ||
					strings.TrimSpace(cfg.Workday.ClientSecret) == "") {
				errs = append(errs, "workday.client_id and client_secret are required when token_url is configured")
			}
		default:
			errs = append(errs, "workday.provider must be mock or http")
		}
		if cfg.Workday.PageSize <= 0 || cfg.Workday.PageSize > 1000 {
			errs = append(errs, "workday.page_size must be between 1 and 1000")
		}
	}

	if cfg.Audit != nil && cfg.Audit.RetentionDays < 0 {
		errs = append(errs, fmt.Sprintf("audit.retention_days must be >= 0 (got %d); use 0 to disable purge",
			cfg.Audit.RetentionDays))
	}

	if cfg.Conversation != nil {
		if cfg.Conversation.EmbeddingTopK < 0 {
			errs = append(errs, "conversation.embedding_top_k must be >= 0")
		}
		if cfg.Conversation.RerankTopK < 0 {
			errs = append(errs, "conversation.rerank_top_k must be >= 0")
		}
		if cfg.Conversation.VectorThreshold < 0 || cfg.Conversation.VectorThreshold > 1 {
			errs = append(errs, "conversation.vector_threshold must be between 0 and 1")
		}
		if cfg.Conversation.RerankThreshold < -10 || cfg.Conversation.RerankThreshold > 10 {
			errs = append(errs, "conversation.rerank_threshold must be between -10 and 10")
		}
	}

	if cfg.KnowledgeBase != nil {
		if cfg.KnowledgeBase.ChunkSize <= 0 {
			errs = append(errs, "knowledge_base.chunk_size must be > 0")
		}
		if cfg.KnowledgeBase.ChunkOverlap < 0 {
			errs = append(errs, "knowledge_base.chunk_overlap must be >= 0")
		}
		if cfg.KnowledgeBase.ChunkOverlap >= cfg.KnowledgeBase.ChunkSize {
			errs = append(errs, "knowledge_base.chunk_overlap must be less than chunk_size")
		}
	}

	if cfg.Server != nil {
		if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
			errs = append(errs, "server.port must be between 1 and 65535")
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

func applyOIDCEnvOverrides(cfg *Config) {
	if cfg.OIDCAuth == nil {
		cfg.OIDCAuth = &OIDCAuthConfig{}
	}
	if cfg.OIDCAuth.UserInfoMapping == nil {
		cfg.OIDCAuth.UserInfoMapping = &OIDCUserInfoMapping{}
	}

	if value := strings.TrimSpace(os.Getenv("OIDC_AUTH_ENABLE")); value != "" {
		cfg.OIDCAuth.Enable = strings.EqualFold(value, "true")
	}
	if value := strings.TrimSpace(os.Getenv("OIDC_AUTH_ISSUER_URL")); value != "" {
		cfg.OIDCAuth.IssuerURL = value
	}
	if value := strings.TrimSpace(os.Getenv("OIDC_AUTH_DISCOVERY_URL")); value != "" {
		cfg.OIDCAuth.DiscoveryURL = value
	}
	if value := strings.TrimSpace(os.Getenv("OIDC_AUTH_PROVIDER_DISPLAY_NAME")); value != "" {
		cfg.OIDCAuth.ProviderDisplayName = value
	}
	if value := strings.TrimSpace(os.Getenv("OIDC_AUTH_CLIENT_ID")); value != "" {
		cfg.OIDCAuth.ClientID = value
	}
	if value := strings.TrimSpace(os.Getenv("OIDC_AUTH_CLIENT_SECRET")); value != "" {
		cfg.OIDCAuth.ClientSecret = value
	}
	if value := strings.TrimSpace(os.Getenv("OIDC_AUTH_AUTHORIZATION_ENDPOINT")); value != "" {
		cfg.OIDCAuth.AuthorizationEndpoint = value
	}
	if value := strings.TrimSpace(os.Getenv("OIDC_AUTH_TOKEN_ENDPOINT")); value != "" {
		cfg.OIDCAuth.TokenEndpoint = value
	}
	if value := strings.TrimSpace(os.Getenv("OIDC_AUTH_USER_INFO_ENDPOINT")); value != "" {
		cfg.OIDCAuth.UserInfoEndpoint = value
	}
	if value := strings.TrimSpace(os.Getenv("OIDC_AUTH_SCOPES")); value != "" {
		cfg.OIDCAuth.Scopes = strings.Fields(strings.ReplaceAll(value, ",", " "))
	}
	if value := strings.TrimSpace(os.Getenv("OIDC_USER_INFO_MAPPING_USER_NAME")); value != "" {
		cfg.OIDCAuth.UserInfoMapping.Username = value
	}
	if value := strings.TrimSpace(os.Getenv("OIDC_USER_INFO_MAPPING_EMAIL")); value != "" {
		cfg.OIDCAuth.UserInfoMapping.Email = value
	}
	if value := strings.TrimSpace(os.Getenv("OIDC_USER_INFO_MAPPING_EMPLOYEE_ID")); value != "" {
		cfg.OIDCAuth.UserInfoMapping.EmployeeID = value
	}

	if cfg.OIDCAuth.ProviderDisplayName == "" {
		cfg.OIDCAuth.ProviderDisplayName = "OIDC"
	}
	if len(cfg.OIDCAuth.Scopes) == 0 {
		cfg.OIDCAuth.Scopes = []string{"openid", "profile", "email"}
	}
	if cfg.OIDCAuth.UserInfoMapping.Username == "" {
		cfg.OIDCAuth.UserInfoMapping.Username = "name"
	}
	if cfg.OIDCAuth.UserInfoMapping.Email == "" {
		cfg.OIDCAuth.UserInfoMapping.Email = "email"
	}
	if cfg.OIDCAuth.UserInfoMapping.EmployeeID == "" {
		cfg.OIDCAuth.UserInfoMapping.EmployeeID = "employee_id"
	}
	if cfg.OIDCAuth.DiscoveryURL == "" && cfg.OIDCAuth.IssuerURL != "" {
		cfg.OIDCAuth.DiscoveryURL = strings.TrimRight(cfg.OIDCAuth.IssuerURL, "/") + "/.well-known/openid-configuration"
	}
	if value := strings.TrimSpace(os.Getenv("OIDC_AUTH_AUTO_PROVISION")); value != "" {
		cfg.OIDCAuth.AutoProvision = strings.EqualFold(value, "true")
	}
}

// applySAMLAuthDefaults initialises the SAML section (from YAML/env) and
// fills attribute-mapping defaults so downstream code can assume a non-nil
// mapping. SAML secrets (SP key) are only ever accepted through environment
// variables and never serialised to JSON.
func applySAMLAuthDefaults(cfg *Config) {
	if cfg.SAMLAuth == nil {
		cfg.SAMLAuth = &SAMLAuthConfig{}
	}
	if cfg.SAMLAuth.UserInfoMapping == nil {
		cfg.SAMLAuth.UserInfoMapping = &SAMLUserInfoMapping{}
	}

	if value := strings.TrimSpace(os.Getenv("SAML_AUTH_ENABLE")); value != "" {
		cfg.SAMLAuth.Enable = strings.EqualFold(value, "true")
	}
	if value := strings.TrimSpace(os.Getenv("SAML_AUTH_IDP_METADATA_URL")); value != "" {
		cfg.SAMLAuth.IdPMetadataURL = value
	}
	if value := strings.TrimSpace(os.Getenv("SAML_AUTH_IDP_METADATA_FILE")); value != "" {
		cfg.SAMLAuth.IdPMetadataFile = value
	}
	if value := strings.TrimSpace(os.Getenv("SAML_AUTH_IDP_METADATA")); value != "" {
		cfg.SAMLAuth.IdPMetadata = value
	}
	if value := strings.TrimSpace(os.Getenv("SAML_AUTH_SP_ENTITY_ID")); value != "" {
		cfg.SAMLAuth.SPEntityID = value
	}
	if value := strings.TrimSpace(os.Getenv("SAML_AUTH_ACS_URL")); value != "" {
		cfg.SAMLAuth.ACSUrl = value
	}
	if value := strings.TrimSpace(os.Getenv("SAML_AUTH_SP_CERT")); value != "" {
		cfg.SAMLAuth.SPCert = strings.ReplaceAll(value, "\\n", "\n")
	}
	if value := strings.TrimSpace(os.Getenv("SAML_AUTH_SP_KEY")); value != "" {
		cfg.SAMLAuth.SPKey = strings.ReplaceAll(value, "\\n", "\n")
	}
	if value := strings.TrimSpace(os.Getenv("SAML_AUTH_SP_CERT_FILE")); value != "" {
		cfg.SAMLAuth.SPCertFile = value
	}
	if value := strings.TrimSpace(os.Getenv("SAML_AUTH_SP_KEY_FILE")); value != "" {
		cfg.SAMLAuth.SPKeyFile = value
	}
	if value := strings.TrimSpace(os.Getenv("SAML_AUTH_ALLOW_EPHEMERAL_CERT")); value != "" {
		cfg.SAMLAuth.AllowEphemeralSPCert = strings.EqualFold(value, "true")
	}
	if value := strings.TrimSpace(os.Getenv("SAML_AUTH_PROVIDER_DISPLAY_NAME")); value != "" {
		cfg.SAMLAuth.ProviderDisplayName = value
	}
	if value := strings.TrimSpace(os.Getenv("SAML_AUTH_AUTO_PROVISION")); value != "" {
		cfg.SAMLAuth.AutoProvision = strings.EqualFold(value, "true")
	}
	if value, exists := os.LookupEnv("SAML_AUTH_DEV_SYSTEM_ADMIN_EMAILS"); exists {
		cfg.SAMLAuth.DevSystemAdminEmails = splitNormalizedCSV(value)
	}
	if value := strings.TrimSpace(os.Getenv("SAML_AUTH_SIGN_REQUEST")); value != "" {
		cfg.SAMLAuth.SignRequest = strings.EqualFold(value, "true")
	}
	if value := strings.TrimSpace(os.Getenv("SAML_AUTH_ALLOW_IDP_INITIATED")); value != "" {
		cfg.SAMLAuth.AllowIDPInitiated = strings.EqualFold(value, "true")
	}
	if value := strings.TrimSpace(os.Getenv("SAML_USER_INFO_MAPPING_SUBJECT")); value != "" {
		cfg.SAMLAuth.UserInfoMapping.Subject = value
	}
	if value := strings.TrimSpace(os.Getenv("SAML_USER_INFO_MAPPING_USER_NAME")); value != "" {
		cfg.SAMLAuth.UserInfoMapping.Username = value
	}
	if value := strings.TrimSpace(os.Getenv("SAML_USER_INFO_MAPPING_EMAIL")); value != "" {
		cfg.SAMLAuth.UserInfoMapping.Email = value
	}
	if value := strings.TrimSpace(os.Getenv("SAML_USER_INFO_MAPPING_EMPLOYEE_ID")); value != "" {
		cfg.SAMLAuth.UserInfoMapping.EmployeeID = value
	}

	if cfg.SAMLAuth.ProviderDisplayName == "" {
		cfg.SAMLAuth.ProviderDisplayName = "SAML SSO"
	}
	if cfg.SAMLAuth.SPEntityID == "" {
		cfg.SAMLAuth.SPEntityID = "urn:rochekap:sp"
	}
	if cfg.SAMLAuth.UserInfoMapping.Subject == "" {
		cfg.SAMLAuth.UserInfoMapping.Subject = "subject"
	}
	if cfg.SAMLAuth.UserInfoMapping.Username == "" {
		cfg.SAMLAuth.UserInfoMapping.Username = "username"
	}
	if cfg.SAMLAuth.UserInfoMapping.Email == "" {
		cfg.SAMLAuth.UserInfoMapping.Email = "email"
	}
	if cfg.SAMLAuth.UserInfoMapping.EmployeeID == "" {
		cfg.SAMLAuth.UserInfoMapping.EmployeeID = "employee_id"
	}
}

func splitNormalizedCSV(value string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0)
	for _, item := range strings.Split(value, ",") {
		normalized := strings.ToLower(strings.TrimSpace(item))
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result
}

func applyWorkdayEnvOverrides(cfg *Config) {
	if cfg.Workday == nil {
		cfg.Workday = &WorkdayConfig{}
	}
	if value := strings.TrimSpace(os.Getenv("WORKDAY_ENABLE")); value != "" {
		cfg.Workday.Enable = strings.EqualFold(value, "true")
	}
	stringOverrides := map[string]*string{
		"WORKDAY_PROVIDER":       &cfg.Workday.Provider,
		"WORKDAY_CONNECTION_KEY": &cfg.Workday.ConnectionKey,
		"WORKDAY_MOCK_FILE":      &cfg.Workday.MockFile,
		"WORKDAY_BASE_URL":       &cfg.Workday.BaseURL,
		"WORKDAY_ORG_UNITS_PATH": &cfg.Workday.OrgUnitsPath,
		"WORKDAY_WORKERS_PATH":   &cfg.Workday.WorkersPath,
		"WORKDAY_TOKEN_URL":      &cfg.Workday.TokenURL,
		"WORKDAY_CLIENT_ID":      &cfg.Workday.ClientID,
		"WORKDAY_CLIENT_SECRET":  &cfg.Workday.ClientSecret,
		"WORKDAY_SCOPE":          &cfg.Workday.Scope,
	}
	for name, target := range stringOverrides {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			*target = value
		}
	}
	if value := strings.TrimSpace(os.Getenv("WORKDAY_SYNC_ORG_UNITS")); value != "" {
		enabled := strings.EqualFold(value, "true")
		cfg.Workday.SyncOrgUnits = &enabled
	}
	if value := strings.TrimSpace(os.Getenv("WORKDAY_PAGE_SIZE")); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			cfg.Workday.PageSize = parsed
		}
	}
	if value := strings.TrimSpace(os.Getenv("WORKDAY_REQUEST_TIMEOUT")); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			cfg.Workday.RequestTimeout = parsed
		}
	}
	if cfg.Workday.Provider == "" {
		cfg.Workday.Provider = "mock"
	}
	if cfg.Workday.ConnectionKey == "" {
		cfg.Workday.ConnectionKey = "default"
	}
	if cfg.Workday.OrgUnitsPath == "" {
		cfg.Workday.OrgUnitsPath = "/org-units"
	}
	if cfg.Workday.WorkersPath == "" {
		cfg.Workday.WorkersPath = "/workers"
	}
	if cfg.Workday.PageSize <= 0 {
		cfg.Workday.PageSize = 200
	}
	if cfg.Workday.RequestTimeout <= 0 {
		cfg.Workday.RequestTimeout = 30 * time.Second
	}
}

func applyKnowledgeBaseEnvOverrides(cfg *Config) {
	if cfg.KnowledgeBase == nil {
		cfg.KnowledgeBase = &KnowledgeBaseConfig{}
	}
	if cfg.KnowledgeBase.DocumentProcessTimeout <= 0 {
		cfg.KnowledgeBase.DocumentProcessTimeout = DefaultDocumentProcessTimeout
	}
	if value := strings.TrimSpace(os.Getenv("ROCHE_KAP_DOCUMENT_PROCESS_TIMEOUT")); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			cfg.KnowledgeBase.DocumentProcessTimeout = d
		}
	}
	if cfg.KnowledgeBase.DocReaderCallTimeout <= 0 {
		cfg.KnowledgeBase.DocReaderCallTimeout = 30 * time.Minute
	}
	if value := strings.TrimSpace(os.Getenv("ROCHE_KAP_DOCREADER_CALL_TIMEOUT")); value != "" {
		if d, err := time.ParseDuration(value); err == nil && d > 0 {
			cfg.KnowledgeBase.DocReaderCallTimeout = d
		}
	}
}

func applyAgentEnvOverrides(cfg *Config) {
	if cfg.Agent == nil {
		cfg.Agent = &AgentConfig{}
	}
	if value := strings.TrimSpace(os.Getenv("ROCHE_KAP_AGENT_LLM_TIMEOUT")); value != "" {
		if timeout, err := time.ParseDuration(value); err == nil {
			cfg.Agent.LLMCallTimeout = int(timeout.Seconds())
		} else if sec, err := time.ParseDuration(value + "s"); err == nil {
			// Handle case where user just provides a number like "300"
			cfg.Agent.LLMCallTimeout = int(sec.Seconds())
		}
	}
	// MCP tool human-approval wait timeout (issue #1173). Accepts Go duration
	// (e.g. "10m", "30s") or a bare number interpreted as seconds.
	if value := strings.TrimSpace(os.Getenv("ROCHE_KAP_AGENT_TOOL_APPROVAL_TIMEOUT")); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			cfg.Agent.ToolApprovalTimeoutSeconds = int(d.Seconds())
		} else if d, err := time.ParseDuration(value + "s"); err == nil {
			cfg.Agent.ToolApprovalTimeoutSeconds = int(d.Seconds())
		}
	}
}

func applyUnifiedQAEnvOverrides(cfg *Config) {
	if cfg.UnifiedQA == nil {
		cfg.UnifiedQA = &UnifiedQAConfig{}
	}
	if cfg.UnifiedQA.RouteModel == nil {
		cfg.UnifiedQA.RouteModel = &UnifiedQARouteModelConfig{}
	}
	stringOverrides := map[string]*string{
		"UNIFIED_QA_ROUTE_MODEL_ID":            &cfg.UnifiedQA.RouteModel.ID,
		"UNIFIED_QA_ROUTE_MODEL_NAME":          &cfg.UnifiedQA.RouteModel.ModelName,
		"UNIFIED_QA_ROUTE_MODEL_BASE_URL":      &cfg.UnifiedQA.RouteModel.BaseURL,
		"UNIFIED_QA_ROUTE_MODEL_API_KEY":       &cfg.UnifiedQA.RouteModel.APIKey,
		"UNIFIED_QA_ROUTE_MODEL_PROVIDER":      &cfg.UnifiedQA.RouteModel.Provider,
		"UNIFIED_QA_ROUTE_MODEL_OUTPUT_SCHEMA": &cfg.UnifiedQA.RouteModel.OutputSchema,
	}
	for name, target := range stringOverrides {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			*target = value
		}
		*target = strings.TrimSpace(*target)
	}
	if value := strings.TrimSpace(os.Getenv("UNIFIED_QA_SUMMARY_MODEL_ID")); value != "" {
		cfg.UnifiedQA.SummaryModelID = value
	}
	cfg.UnifiedQA.SummaryModelID = strings.TrimSpace(cfg.UnifiedQA.SummaryModelID)
}

// applyAuditDefaults fills in defaults for the Audit config section
// and applies the env override commonly used to extend or disable
// retention without editing config.yaml.
//
// Defaults:
//   - When the `audit:` section is omitted entirely from YAML,
//     RetentionDays = 90 (purge rows older than 90 days).
//
// Operator intent is otherwise preserved: an explicit
// `audit.retention_days: 0` in YAML means "disable the purge", which
// is a supported posture for compliance use cases that handle archival
// off-database.
//
// Env overrides (when set and parseable; out-of-range is ignored):
//   - ROCHE_KAP_AUDIT_RETENTION_DAYS (non-negative integer)
//   - ROCHE_KAP_AUDIT_GLOBAL_ENABLED ("true"/"false")
//   - ROCHE_KAP_AUDIT_GLOBAL_CAPTURE_BODY ("true"/"false")
//   - ROCHE_KAP_AUDIT_GLOBAL_RECORD_GET ("true"/"false")
func applyAuditDefaults(cfg *Config) {
	// Section omitted entirely -> apply the default and no env wiring
	// is needed for the most common path.
	if cfg.Audit == nil {
		cfg.Audit = &AuditConfig{RetentionDays: 90}
	}

	// Env override always wins, but only when explicitly set so a
	// stale shell variable doesn't suddenly disable the purge for a
	// future deployment that committed a real value.
	if value := strings.TrimSpace(os.Getenv("ROCHE_KAP_AUDIT_RETENTION_DAYS")); value != "" {
		if n, err := strconv.Atoi(value); err == nil && n >= 0 {
			cfg.Audit.RetentionDays = n
		}
	}

	// Fill defaults and env overrides for GlobalAudit
	if cfg.Audit.Global == nil {
		cfg.Audit.Global = &GlobalAuditConfig{}
	}
	g := cfg.Audit.Global
	if value := strings.TrimSpace(os.Getenv("ROCHE_KAP_AUDIT_GLOBAL_ENABLED")); value != "" {
		g.Enabled = strings.EqualFold(value, "true")
	}
	if value := strings.TrimSpace(os.Getenv("ROCHE_KAP_AUDIT_GLOBAL_CAPTURE_BODY")); value != "" {
		g.CaptureBody = strings.EqualFold(value, "true")
	} else if !g.Enabled {
		// When enabling for the first time, default CaptureBody to true
		g.CaptureBody = true
	}
	if value := strings.TrimSpace(os.Getenv("ROCHE_KAP_AUDIT_GLOBAL_RECORD_GET")); value != "" {
		g.RecordGET = strings.EqualFold(value, "true")
	}
}

// into actual prompt text content. Only xxx_id fields are used;
// no fallback to default templates.
func backfillConversationDefaults(cfg *Config) {
	pt := cfg.PromptTemplates
	conv := cfg.Conversation

	if conv.FallbackPromptID != "" {
		if t := FindTemplateByID(pt, conv.FallbackPromptID); t != nil {
			conv.FallbackPrompt = t.Content
		} else {
			fmt.Printf("Warning: fallback_prompt_id %q not found\n", conv.FallbackPromptID)
		}
	}
	if conv.RewritePromptID != "" {
		if t := FindTemplateByID(pt, conv.RewritePromptID); t != nil {
			conv.RewritePromptSystem = t.Content
			conv.RewritePromptUser = t.User
		} else {
			fmt.Printf("Warning: rewrite_prompt_id %q not found\n", conv.RewritePromptID)
		}
	}
	if conv.GenerateSessionTitlePromptID != "" {
		if t := FindTemplateByID(pt, conv.GenerateSessionTitlePromptID); t != nil {
			conv.GenerateSessionTitlePrompt = t.Content
		} else {
			fmt.Printf("Warning: generate_session_title_prompt_id %q not found\n", conv.GenerateSessionTitlePromptID)
		}
	}
	if conv.GenerateSummaryPromptID != "" {
		if t := FindTemplateByID(pt, conv.GenerateSummaryPromptID); t != nil {
			conv.GenerateSummaryPrompt = t.Content
		} else {
			fmt.Printf("Warning: generate_summary_prompt_id %q not found\n", conv.GenerateSummaryPromptID)
		}
	}
	if conv.ExtractEntitiesPromptID != "" {
		if t := FindTemplateByID(pt, conv.ExtractEntitiesPromptID); t != nil {
			conv.ExtractEntitiesPrompt = t.Content
		} else {
			fmt.Printf("Warning: extract_entities_prompt_id %q not found\n", conv.ExtractEntitiesPromptID)
		}
	}
	if conv.ExtractRelationshipsPromptID != "" {
		if t := FindTemplateByID(pt, conv.ExtractRelationshipsPromptID); t != nil {
			conv.ExtractRelationshipsPrompt = t.Content
		} else {
			fmt.Printf("Warning: extract_relationships_prompt_id %q not found\n", conv.ExtractRelationshipsPromptID)
		}
	}
	if conv.GenerateQuestionsPromptID != "" {
		if t := FindTemplateByID(pt, conv.GenerateQuestionsPromptID); t != nil {
			conv.GenerateQuestionsPrompt = t.Content
		} else {
			fmt.Printf("Warning: generate_questions_prompt_id %q not found\n", conv.GenerateQuestionsPromptID)
		}
	}
	if conv.Summary != nil {
		if conv.Summary.PromptID != "" {
			if t := FindTemplateByID(pt, conv.Summary.PromptID); t != nil {
				conv.Summary.Prompt = t.Content
			} else {
				fmt.Printf("Warning: summary.prompt_id %q not found\n", conv.Summary.PromptID)
			}
		}
		if conv.Summary.ContextTemplateID != "" {
			if t := FindTemplateByID(pt, conv.Summary.ContextTemplateID); t != nil {
				conv.Summary.ContextTemplate = t.Content
			} else {
				fmt.Printf("Warning: summary.context_template_id %q not found\n", conv.Summary.ContextTemplateID)
			}
		}
	}

	// Build intent→system-prompt map from IntentPrompts templates.
	// Template ID must equal the QueryIntent string value (e.g. "greeting").
	if len(pt.IntentPrompts) > 0 {
		conv.IntentSystemPrompts = make(map[string]string, len(pt.IntentPrompts))
		for _, t := range pt.IntentPrompts {
			if t.ID != "" && t.Content != "" {
				conv.IntentSystemPrompts[t.ID] = t.Content
			}
		}
	}
}

// FindTemplateByID searches across all template lists for a template with the given ID.
// It returns the template if found, or nil otherwise.
func FindTemplateByID(pt *PromptTemplatesConfig, id string) *PromptTemplate {
	if pt == nil || id == "" {
		return nil
	}
	// Search all template collections
	for _, list := range [][]PromptTemplate{
		pt.SystemPrompt,
		pt.ContextTemplate,
		pt.Rewrite,
		pt.Fallback,
		pt.GenerateSessionTitle,
		pt.GenerateSummary,
		pt.KeywordsExtraction,
		pt.AgentSystemPrompt,
		pt.GraphExtraction,
		pt.GenerateQuestions,
		pt.IntentPrompts,
		pt.UnifiedQA,
	} {
		for i := range list {
			if list[i].ID == id {
				return &list[i]
			}
		}
	}
	return nil
}

// resolveBuiltinAgentPromptIDs resolves system_prompt_id and context_template_id
// references in builtin agent configs by looking up the actual content from
// prompt template YAML files.
func resolveBuiltinAgentPromptIDs(pt *PromptTemplatesConfig) {
	types.ResolveBuiltinAgentPromptRefs(func(id string) string {
		if t := FindTemplateByID(pt, id); t != nil {
			return t.Content
		}
		return ""
	})
}

// promptTemplateFile 用于解析模板文件
type promptTemplateFile struct {
	Templates []PromptTemplate `yaml:"templates"`
}

// loadPromptTemplates 从目录加载提示词模板
func loadPromptTemplates(configDir string) (*PromptTemplatesConfig, error) {
	templatesDir := filepath.Join(configDir, "prompt_templates")

	// 检查目录是否存在
	if _, err := os.Stat(templatesDir); os.IsNotExist(err) {
		return nil, nil // 目录不存在，返回nil让调用者使用配置文件中的模板
	}

	config := &PromptTemplatesConfig{}

	// 定义模板文件映射
	templateFiles := map[string]*[]PromptTemplate{
		"system_prompt.yaml":          &config.SystemPrompt,
		"context_template.yaml":       &config.ContextTemplate,
		"rewrite.yaml":                &config.Rewrite,
		"fallback.yaml":               &config.Fallback,
		"generate_session_title.yaml": &config.GenerateSessionTitle,
		"generate_summary.yaml":       &config.GenerateSummary,
		"keywords_extraction.yaml":    &config.KeywordsExtraction,
		"agent_system_prompt.yaml":    &config.AgentSystemPrompt,
		"graph_extraction.yaml":       &config.GraphExtraction,
		"generate_questions.yaml":     &config.GenerateQuestions,
		"intent_prompts.yaml":         &config.IntentPrompts,
	}

	// 加载每个模板文件
	for filename, target := range templateFiles {
		filePath := filepath.Join(templatesDir, filename)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			continue // 文件不存在，跳过
		}

		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", filename, err)
		}

		var file promptTemplateFile
		if err := yaml.Unmarshal(data, &file); err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", filename, err)
		}

		*target = file.Templates
	}

	// Unified QA prompts are split by responsibility. Keep accepting the
	// original unified_qa.yaml name so existing deployments remain compatible.
	unifiedQAFiles, err := filepath.Glob(filepath.Join(templatesDir, "unified_qa*.yaml"))
	if err != nil {
		return nil, fmt.Errorf("find unified QA prompt files: %w", err)
	}
	seenUnifiedQAIDs := make(map[string]string)
	for _, filePath := range unifiedQAFiles {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", filepath.Base(filePath), err)
		}

		var file promptTemplateFile
		if err := yaml.Unmarshal(data, &file); err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", filepath.Base(filePath), err)
		}
		for _, template := range file.Templates {
			if previousFile, duplicate := seenUnifiedQAIDs[template.ID]; duplicate {
				return nil, fmt.Errorf(
					"duplicate unified QA prompt ID %q in %s and %s",
					template.ID,
					previousFile,
					filepath.Base(filePath),
				)
			}
			seenUnifiedQAIDs[template.ID] = filepath.Base(filePath)
			config.UnifiedQA = append(config.UnifiedQA, template)
		}
	}

	return config, nil
}

// WebSearchConfig represents the web search configuration
type WebSearchConfig struct {
	Timeout int `yaml:"timeout" json:"timeout"` // 超时时间（秒）
}
