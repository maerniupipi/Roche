package types

import (
	"time"
)

// AuditAction names a single audited action class. Action constants are
// dot-namespaced (`<area>.<event>`) so future PRs can plug in their own
// areas without colliding with existing platform events.
type AuditAction string

const (
	// AuditActionAccessDenied records a rejected platform or knowledge
	// resource authorization check.
	AuditActionAccessDenied AuditAction = "访问.拒绝"
	// VectorStore lifecycle actions. Emitted by VectorStoreService.
	// Cover both env-store-derived (__env_*) and DB store create /
	// update / delete paths. Details payload identifies the store_id
	// and the changed key set; secret values (Password, APIKey,
	// connection_config encrypted blob) MUST NOT appear in details.

	// AuditActionVectorStoreCreated fires when a new VectorStore row
	// is committed to the DB (Phase 1 CRUD path). Actor is the knowledgeDomain
	// user; resource is the VectorStore.
	AuditActionVectorStoreCreated AuditAction = "向量库.创建"
	// AuditActionVectorStoreUpdated fires on UPDATE of any VectorStore
	// mutable field. Details payload carries the changed key set
	// (never the secret values themselves — only the field names).
	AuditActionVectorStoreUpdated AuditAction = "向量库.更新"
	// AuditActionVectorStoreDeleted fires when a VectorStore is
	// (soft-)deleted. Phase 2's delete guard already prevents deletion
	// of stores with bound KBs; the audit row records the actor and
	// the store_id for forensic traceability.
	AuditActionVectorStoreDeleted AuditAction = "向量库.删除"

	// OpenSearch-specific actions emitted by the driver shipped in
	// Phase 3. The OpenSearch index is a derived resource of the
	// VectorStore — these events capture cluster-side side effects
	// (PUT /<index>, DELETE /<index>, POST /_reindex) that operators
	// may need to correlate with VectorStore lifecycle events.

	// AuditActionOpenSearchIndexCreated fires when the OpenSearch
	// driver lazily creates a per-dimension index (the first time a
	// KB with a given embedding dim binds to the store). Details
	// payload: index name, alias name, dimension.
	AuditActionOpenSearchIndexCreated AuditAction = "检索引擎.索引创建"
	// AuditActionOpenSearchIndexDeleted fires when the OpenSearch
	// driver drops an index (e.g. cascade from VectorStore delete).
	AuditActionOpenSearchIndexDeleted AuditAction = "检索引擎.索引删除"
	// AuditActionOpenSearchReindexExecuted fires when CopyIndices
	// initiates a _reindex (sync or async). Details payload: source
	// KB id, target KB id, sync-or-async, doc count if known.
	AuditActionOpenSearchReindexExecuted AuditAction = "检索引擎.重建索引"

// AuditActionSystemSettingChanged fires when a SystemAdmin updates
// a row in the platform-wide system_settings table via
// PUT /api/v1/system/admin/settings/:key. Details payload carries
// {key, value_type, old_value, new_value} — sensitive values are
// redacted server-side before logging when is_secret=true (P3+;
// for now no setting is marked secret).
	AuditActionSystemSettingChanged AuditAction = "系统.设置变更"

	// AuditActionSystemAdminPromoted fires when a SystemAdmin grants
	// system-administrator privileges to another user via
	// POST /api/v1/system/admin/promote. ActorUserID is the promoter,
	// TargetUserID is the user being promoted. Details payload carries
	// {target_email, target_username, idempotent} — `idempotent=true`
	// means the user was already a system admin and no row was written
	// (we still emit the row so probing the endpoint leaves a trail).
	AuditActionSystemAdminPromoted AuditAction = "系统.管理员提升"
	// AuditActionSystemAdminRevoked fires when a SystemAdmin removes
	// system-administrator privileges from another user via
	// POST /api/v1/system/admin/revoke. Details payload carries
	// {target_email, target_username, changed} — `changed=false` covers
	// the idempotent path (target was already not an admin) so an audit
	// reader can distinguish a real revoke from a noop attempt.
	AuditActionSystemAdminRevoked AuditAction = "系统.管理员撤销"

	// AuditActionHTTPRequest is the generic action emitted by the global
	// audit middleware for every authenticated API request. The actual
	// route pattern and HTTP method are captured inside the Details JSON
	// (request_path / request_method keys) so the single action constant
	// keeps the table cardinality manageable while still allowing
	// filtering in the UI. Details also captures request_body,
	// status_code, duration_ms, client_ip, and query string (for GET).
	AuditActionHTTPRequest AuditAction = "请求.记录"

	// ------------------------------------------------------------------
	// Knowledge document lifecycle (knowledge.* namespace)
	// ------------------------------------------------------------------

	// AuditActionKnowledgeCreated fires when a new knowledge document is
	// uploaded or created via API / import. Details carries {title, file_name,
	// file_size, file_type, knowledge_base_id, knowledge_base_name}.
	AuditActionKnowledgeCreated AuditAction = "知识.创建"
	// AuditActionKnowledgeUpdated fires when a knowledge document's content
	// or metadata is modified. Details carries {title, changed_fields[],
	// knowledge_base_id}.
	AuditActionKnowledgeUpdated AuditAction = "知识.更新"
	// AuditActionKnowledgeDeleted fires when a knowledge document is
	// (soft-)deleted. Details carries {title, knowledge_base_id, cascade}.
	AuditActionKnowledgeDeleted AuditAction = "知识.删除"
	// AuditActionKnowledgePublished fires when a knowledge document is
	// published or enabled. Details carries {title, knowledge_base_id}.
	AuditActionKnowledgePublished AuditAction = "知识.发布"
	// AuditActionKnowledgeUnpublished fires when a knowledge document is
	// unpublished or disabled. Details carries {title, knowledge_base_id}.
	AuditActionKnowledgeUnpublished AuditAction = "知识.下架"

	// ------------------------------------------------------------------
	// FAQ lifecycle (faq.* namespace)
	// ------------------------------------------------------------------

	// AuditActionFAQCreated fires when a FAQ entry is created.
	// Details carries {question, answer, knowledge_base_id}.
	AuditActionFAQCreated AuditAction = "FAQ.创建"
	// AuditActionFAQUpdated fires when a FAQ entry is modified.
	// Details carries {question, knowledge_base_id, changed_fields[]}.
	AuditActionFAQUpdated AuditAction = "FAQ.更新"
	// AuditActionFAQDeleted fires when a FAQ entry is deleted.
	// Details carries {question, knowledge_base_id}.
	AuditActionFAQDeleted AuditAction = "FAQ.删除"
	// AuditActionFAQImported fires when FAQ entries are batch-imported.
	// Details carries {count, knowledge_base_id, file_name}.
	AuditActionFAQImported AuditAction = "FAQ.导入"

	// ------------------------------------------------------------------
	// Knowledge base lifecycle (knowledgebase.* namespace)
	// ------------------------------------------------------------------

	// AuditActionKnowledgeBaseCreated fires when a knowledge base is created.
	// Details carries {name, vector_store_id, embedding_model}.
	AuditActionKnowledgeBaseCreated AuditAction = "知识库.创建"
	// AuditActionKnowledgeBaseUpdated fires when a knowledge base is modified.
	// Details carries {name, changed_fields[]}.
	AuditActionKnowledgeBaseUpdated AuditAction = "知识库.更新"
	// AuditActionKnowledgeBaseDeleted fires when a knowledge base is deleted.
	// Details carries {name, cascade_documents}.
	AuditActionKnowledgeBaseDeleted AuditAction = "知识库.删除"

	// ------------------------------------------------------------------
	// Permission & authorization (permission.* / domain.* namespace)
	// ------------------------------------------------------------------

	// AuditActionPermissionGranted fires when a resource permission is
	// granted to a user or group. Details carries {resource_type, resource_id,
	// resource_name, grantee_user_id, grantee_email, permission_level}.
	AuditActionPermissionGranted AuditAction = "权限.授予"
	// AuditActionPermissionRevoked fires when a resource permission is
	// revoked. Details carries {resource_type, resource_id, resource_name,
	// revokee_user_id, revokee_email, permission_level}.
	AuditActionPermissionRevoked AuditAction = "权限.撤销"
	// AuditActionDomainAdminGranted fires when a user is promoted to
	// knowledge-domain administrator. Details carries {target_email,
	// target_username, knowledge_domain_id, knowledge_domain_name}.
	AuditActionDomainAdminGranted AuditAction = "域.管理员授予"
	// AuditActionDomainAdminRevoked fires when knowledge-domain admin
	// privileges are removed. Details carries {target_email, target_username,
	// knowledge_domain_id, knowledge_domain_name}.
	AuditActionDomainAdminRevoked AuditAction = "域.管理员撤销"

	// ------------------------------------------------------------------
	// User management (user.* namespace)
	// ------------------------------------------------------------------

	// AuditActionUserCreated fires when a new user account is registered.
	// Details carries {email, username, provider}.
	AuditActionUserCreated AuditAction = "用户.创建"
	// AuditActionUserUpdated fires when user profile or settings are modified.
	// Details carries {email, changed_fields[]}.
	AuditActionUserUpdated AuditAction = "用户.更新"
	// AuditActionUserDeleted fires when a user account is removed.
	// Details carries {email, username}.
	AuditActionUserDeleted AuditAction = "用户.删除"
	// AuditActionUserPasswordChanged fires when a user changes their password.
	// Details carries {} (no sensitive data).
	AuditActionUserPasswordChanged AuditAction = "用户.密码变更"

	// ------------------------------------------------------------------
	// Session & Q&A (session.* / qa.* namespace)
	// ------------------------------------------------------------------

	// AuditActionSessionCreated fires when a new Q&A session is started.
	// Details carries {knowledge_base_id, session_title}.
	AuditActionSessionCreated AuditAction = "会话.创建"
	// AuditActionSessionDeleted fires when a Q&A session is removed.
	// Details carries {knowledge_base_id, session_title}.
	AuditActionSessionDeleted AuditAction = "会话.删除"
	// AuditActionQACompleted fires when a question is answered.
	// Details carries {session_id, question, knowledge_base_id,
	// response_type, citation_count}.
	AuditActionQACompleted AuditAction = "问答.完成"

	// ------------------------------------------------------------------
	// Data source lifecycle (datasource.* namespace)
	// ------------------------------------------------------------------

	// AuditActionDataSourceCreated fires when a data source is registered.
	AuditActionDataSourceCreated AuditAction = "数据源.创建"
	// AuditActionDataSourceUpdated fires when a data source config is modified.
	AuditActionDataSourceUpdated AuditAction = "数据源.更新"
	// AuditActionDataSourceDeleted fires when a data source is removed.
	AuditActionDataSourceDeleted AuditAction = "数据源.删除"

	// ------------------------------------------------------------------
	// Knowledge domain lifecycle (knowledge_domain.* namespace)
	// ------------------------------------------------------------------

	// AuditActionKnowledgeDomainCreated fires when a knowledge domain is created.
	AuditActionKnowledgeDomainCreated AuditAction = "知识域.创建"
	// AuditActionKnowledgeDomainUpdated fires when a knowledge domain is modified.
	AuditActionKnowledgeDomainUpdated AuditAction = "知识域.更新"
	// AuditActionKnowledgeDomainDeleted fires when a knowledge domain is deleted.
	AuditActionKnowledgeDomainDeleted AuditAction = "知识域.删除"

	// ------------------------------------------------------------------
	// User management (user.* namespace — continuation)
	// ------------------------------------------------------------------

	// AuditActionUserBanned fires when a user is banned.
	// Details carries {target_email, target_username, reason}.
	AuditActionUserBanned AuditAction = "用户.封禁"
	// AuditActionUserUnbanned fires when a user is unbanned.
	// Details carries {target_email, target_username}.
	AuditActionUserUnbanned AuditAction = "用户.解封"
	// AuditActionUserRolesUpdated fires when user roles (knowledge_officer)
	// are batch-updated. Details carries {affected_count, roles_changed}.
	AuditActionUserRolesUpdated AuditAction = "用户.角色更新"
	// AuditActionUserOfflined fires when an administrator forces a user
	// offline by revoking every outstanding session. Details carries
	// {target_email, target_username}.
	AuditActionUserOfflined AuditAction = "用户.下线"

	// ------------------------------------------------------------------
	// Authentication lifecycle (auth.* namespace)
	// ------------------------------------------------------------------

	// AuditActionLogin fires when a user successfully authenticates via
	// any method (password, OIDC, SAML). Details carries {email, method}.
	AuditActionLogin AuditAction = "认证.登录"
	// AuditActionLoginFailed fires when a login attempt is rejected.
	// Details carries {email, method, reason}.
	AuditActionLoginFailed AuditAction = "认证.登录失败"
	// AuditActionLogout fires when a user explicitly signs out.
	// Details carries {user_id, email}.
	AuditActionLogout AuditAction = "认证.登出"

	// ------------------------------------------------------------------
	// Knowledge document download (knowledge.* namespace)
	// ------------------------------------------------------------------

	// AuditActionKnowledgeDownloaded fires when a knowledge document file
	// is downloaded by a user. Details carries {knowledge_id, title,
	// file_name, file_size, knowledge_base_id}.
	AuditActionKnowledgeDownloaded AuditAction = "知识.下载"
)

// AuditOutcome distinguishes successful mutations from middleware-level
// rejections. The split lets the audit-log UI highlight denials in red
// without needing to enumerate every action class.
type AuditOutcome string

const (
	AuditOutcomeSuccess AuditOutcome = "success"
	AuditOutcomeDenied  AuditOutcome = "denied"
)

// AuditLog is a single immutable audit event. The schema is intentionally
// generic: TargetType / TargetID / Details absorb user / file / knowledge
// base / knowledge domain / etc. events without another migration.
//
// A flat, denormalised shape is kept on purpose:
//   - CreatedAt   —— 时间
//   - ActorName   —— 操作人 name
//   - ActorUserID —— 操作人 id
//   - ActorRole   —— 操作人角色
//   - Action      —— 操作类型（中文短语，如 知识.创建）
//   - TargetType / TargetID / TargetUserID —— 操作对象（用户、文件、知识库、知识域…）
//   - Outcome     —— 结果（success / denied）
//   - Details     —— 操作详情（JSONB，按 action 不同承载不同键）
//
// Rows are append-only — no UpdatedAt, no soft-delete column. The
// monotonic ID acts as both primary key and pagination cursor (newest-
// first is `WHERE id < AfterID ORDER BY id DESC`).
type AuditLog struct {
	ID           uint64       `json:"id"                  gorm:"primaryKey;autoIncrement"`
	ActorUserID  string       `json:"actor_user_id"  gorm:"type:varchar(36);default:'';index:idx_audit_logs_actor"`
	ActorName    string       `json:"actor_name"     gorm:"type:varchar(100);default:''"`
	ActorRole    string       `json:"actor_role"     gorm:"type:varchar(32);default:''"`
	Action       AuditAction  `json:"action"         gorm:"type:varchar(64);not null"`
	TargetType   string       `json:"target_type"    gorm:"type:varchar(32);default:''"`
	TargetID     string       `json:"target_id"      gorm:"type:varchar(64);default:''"`
	TargetUserID string       `json:"target_user_id" gorm:"type:varchar(36);default:''"`
	ClientIP     string       `json:"client_ip"      gorm:"type:varchar(64);default:''"`
	Outcome      AuditOutcome `json:"outcome"        gorm:"type:varchar(16);default:success"`
	Details      JSON         `json:"details"        gorm:"type:jsonb;default:'{}'"`
	CreatedAt    time.Time    `json:"created_at"`
}

// TableName pins the table name even if a future GORM convention
// pluralisation refactor would otherwise rename it.
func (AuditLog) TableName() string { return "audit_logs" }
