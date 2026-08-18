package router

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go.uber.org/dig"
	filesvc "roche.local/knowledge-agent-platform/internal/application/service/file"

	"roche.local/knowledge-agent-platform/internal/config"
	"roche.local/knowledge-agent-platform/internal/handler"
	"roche.local/knowledge-agent-platform/internal/handler/session"
	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/middleware"
	"roche.local/knowledge-agent-platform/internal/tracing/langfuse"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
	secutils "roche.local/knowledge-agent-platform/internal/utils"

	_ "roche.local/knowledge-agent-platform/docs" // swagger docs
)

// RouterParams contains the dependencies used to construct the HTTP router.
type RouterParams struct {
	dig.In

	Config                       *config.Config
	FileService                  interfaces.FileService
	UserService                  interfaces.UserService
	KBService                    interfaces.KnowledgeBaseService
	KnowledgeService             interfaces.KnowledgeService
	ChunkService                 interfaces.ChunkService
	SessionService               interfaces.SessionService
	MessageService               interfaces.MessageService
	ModelService                 interfaces.ModelService
	EvaluationService            interfaces.EvaluationService
	KBHandler                    *handler.KnowledgeBaseHandler
	KnowledgeHandler             *handler.KnowledgeHandler
	KnowledgeDomainHandler       *handler.KnowledgeDomainHandler
	KnowledgeDomainService       interfaces.KnowledgeDomainService
	KnowledgeDomainAdminService  interfaces.KnowledgeDomainAdminService
	KnowledgeDomainAdminHandler  *handler.KnowledgeDomainAdminHandler
	AuditLogHandler              *handler.AuditLogHandler
	AuditLogService              interfaces.AuditLogService
	ChunkHandler                 *handler.ChunkHandler
	SessionHandler               *session.Handler
	MessageHandler               *handler.MessageHandler
	ModelHandler                 *handler.ModelHandler
	ModelCredentialsHandler      *handler.ModelCredentialsHandler
	EvaluationHandler            *handler.EvaluationHandler
	InitializationHandler        *handler.InitializationHandler
	SystemHandler                *handler.SystemHandler
	MCPServiceHandler            *handler.MCPServiceHandler
	MCPCredentialsHandler        *handler.MCPCredentialsHandler
	MCPOAuthHandler              *handler.MCPOAuthHandler
	WebSearchHandler             *handler.WebSearchHandler
	WebSearchProviderHandler     *handler.WebSearchProviderHandler
	WebSearchCredentialsHandler  *handler.WebSearchProviderCredentialsHandler
	VectorStoreHandler           *handler.VectorStoreHandler
	FAQHandler                   *handler.FAQHandler
	TagHandler                   *handler.TagHandler
	CustomAgentHandler           *handler.CustomAgentHandler
	SuggestedQuestionHandler     *handler.SuggestedQuestionHandler
	ExchangeRateHandler          *handler.ExchangeRateHandler
	AdminAnswerRecordHandler     *handler.AdminAnswerRecordHandler
	UserFavoriteHandler          *handler.UserResourceFavoriteHandler
	SkillHandler                 *handler.SkillHandler
	DataSourceHandler            *handler.DataSourceHandler
	DataSourceCredentialsHandler *handler.DataSourceCredentialsHandler
	EnterpriseAccessHandler      *handler.EnterpriseAccessHandler
	EnterpriseIntegrationHandler *handler.EnterpriseIntegrationHandler
	UnifiedQAObservationHandler  *handler.UnifiedQAObservationHandler
	DashboardHandler             *handler.DashboardHandler
	MenuHandler                  *handler.MenuHandler
	GlobalAuditRecorder          *middleware.GlobalAuditRecorder
	BlacklistEntryRepository     interfaces.BlacklistEntryRepository
}

// NewRouter creates the HTTP router.
func NewRouter(params RouterParams) *gin.Engine {
	r := gin.New()
	r.ContextWithFallback = true

	// Trusted proxies: gin defaults to trusting ALL proxies, which makes
	// c.ClientIP() honor a client-supplied X-Forwarded-For. Restrict to the fronting proxy network so
	// only the real client IP (appended by nginx) is returned. Configurable via
	// ROCHE_KAP_TRUSTED_PROXIES (comma-separated CIDRs/IPs).
	if err := r.SetTrustedProxies(trustedProxies()); err != nil {
		logger.Errorf(context.Background(), "[Router] failed to set trusted proxies: %v", err)
	}

	// CORS configuration.
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Request-ID"},
		ExposeHeaders:    []string{"Content-Length", "Access-Control-Allow-Origin"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Common request middleware.
	r.Use(middleware.RequestID())
	r.Use(middleware.Language())
	r.Use(middleware.Logger())
	r.Use(middleware.Recovery())
	r.Use(middleware.ErrorHandler())

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Swagger is exposed only outside release mode.
	if gin.Mode() != gin.ReleaseMode {
		r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler,
			ginSwagger.DefaultModelsExpandDepth(-1),
			ginSwagger.DocExpansion("list"),
			ginSwagger.DeepLinking(true),
			ginSwagger.PersistAuthorization(true),
		))
	}

	if handler.Edition == "lite" {
		serveFrontendStatic(r)
	}

	// InternalServiceAuth injects a system-admin identity for requests
	// carrying a valid X-Internal-Service-Token header. It runs before
	// Auth so that the internal caller bypasses JWT validation entirely.
	r.Use(middleware.InternalServiceAuth(params.Config))
	r.Use(middleware.Auth(params.UserService))
	// BanCheck runs after Auth: it cross-checks the blacklist table
	// as a defense-in-depth layer. Banned users are rejected even if
	// their JWT is still valid.
	r.Use(middleware.BanCheck(params.BlacklistEntryRepository))

	serveFiles(r, params.FileService)

	// Presigned file access: no auth required, signature-verified.
	servePresignedFiles(r, params.KnowledgeDomainService)

	// Langfuse observability is active only when LANGFUSE_* variables are set.
	// The middleware is registered unconditionally; when disabled it's a no-op.
	r.Use(langfuse.GinMiddleware())

	// Middleware and administration handlers obtain the audit service from
	// admin-only /knowledge-domains/:id/audit-log endpoint pull the service out
	// of the gin context. Provider is a no-op when AuditLogService is
	// nil (e.g. lite mode without DB), so the rbac path degrades to
	// "log to stderr only" instead of crashing.
	r.Use(middleware.AuditServiceProvider(params.AuditLogService))
	r.Use(params.GlobalAuditRecorder.Middleware())

	v1 := r.Group("/api/v1")
	{
		// rbacGuards bundles platform, knowledge-domain and explicit resource
		// authorization for the route groups below.
		rbacGuards := newRBACGuards(
			params.Config,
			params.KBService,
			params.KnowledgeService,
			params.ChunkService,
			params.KnowledgeDomainAdminService,
		)

		RegisterKnowledgeDomainRoutes(v1, params.KnowledgeDomainHandler, params.KnowledgeDomainAdminHandler, params.AuditLogHandler, rbacGuards)
		RegisterKnowledgeBaseRoutes(v1, params.KBHandler, rbacGuards)
		RegisterKnowledgeTagRoutes(v1, params.TagHandler, rbacGuards)
		RegisterKnowledgeRoutes(v1, params.KnowledgeHandler, rbacGuards)
		RegisterFAQRoutes(v1, params.FAQHandler, rbacGuards)
		RegisterChunkRoutes(v1, params.ChunkHandler, rbacGuards)
		RegisterSessionRoutes(v1, params.SessionHandler, rbacGuards)
		RegisterChatRoutes(v1, params.SessionHandler, rbacGuards)
		RegisterUnifiedQAObservationRoutes(v1, params.UnifiedQAObservationHandler, rbacGuards)
		RegisterMessageRoutes(v1, params.MessageHandler, rbacGuards)
		RegisterModelRoutes(v1, params.ModelHandler, params.ModelCredentialsHandler, rbacGuards)
		RegisterEvaluationRoutes(v1, params.EvaluationHandler, rbacGuards)
		RegisterInitializationRoutes(v1, params.InitializationHandler, rbacGuards)
		RegisterSystemRoutes(v1, params.SystemHandler, rbacGuards)
		RegisterSystemAdminRoutes(v1, params.SystemHandler, params.AuditLogHandler, rbacGuards)
		if params.Config.IsMCPEnabled() {
			RegisterMCPServiceRoutes(v1, params.MCPServiceHandler, params.MCPCredentialsHandler, params.MCPOAuthHandler, rbacGuards)
		}
		RegisterWebSearchRoutes(v1, params.WebSearchHandler, rbacGuards)
		RegisterWebSearchProviderRoutes(v1, params.WebSearchProviderHandler, params.WebSearchCredentialsHandler, rbacGuards)
		RegisterVectorStoreRoutes(v1, params.VectorStoreHandler, rbacGuards)
		RegisterCustomAgentRoutes(v1, params.CustomAgentHandler, rbacGuards)
		RegisterSuggestedQuestionRoutes(v1, params.SuggestedQuestionHandler, rbacGuards)
		RegisterExchangeRateRoutes(v1, params.ExchangeRateHandler, rbacGuards)
		RegisterAdminAnswerRecordRoutes(v1, params.AdminAnswerRecordHandler, rbacGuards)
		RegisterEnterpriseAccessRoutes(v1, params.EnterpriseAccessHandler, rbacGuards)
		RegisterEnterpriseIntegrationRoutes(v1, params.EnterpriseIntegrationHandler, rbacGuards)
		RegisterUserFavoriteRoutes(v1, params.UserFavoriteHandler, rbacGuards)
		if params.Config.AreSkillsEnabled() {
			RegisterSkillRoutes(v1, params.SkillHandler, rbacGuards)
		}
		RegisterDataSourceRoutes(v1, params.DataSourceHandler, params.DataSourceCredentialsHandler, rbacGuards)
		RegisterChunkerDebugRoutes(v1, rbacGuards)
		RegisterDashboardRoutes(v1, params.DashboardHandler, rbacGuards) // 仪表盘
		if params.MenuHandler != nil {
			v1.GET("/menu", rbacGuards.Viewer(), params.MenuHandler.GetMenu)
		}
	}

	return r
}

func RegisterUnifiedQAObservationRoutes(r *gin.RouterGroup, h *handler.UnifiedQAObservationHandler, g *rbacGuards) {
	runs := r.Group("/unified-qa/runs", g.Viewer())
	{
		runs.GET("/:id", h.GetRun)
	}
}

func RegisterEnterpriseIntegrationRoutes(
	r *gin.RouterGroup,
	h *handler.EnterpriseIntegrationHandler,
	g *rbacGuards,
) {
	if h == nil {
		return
	}
	workday := r.Group("/system/admin/integrations/workday", g.SystemAdmin())
	workday.POST("/sync", h.TriggerWorkdaySync)
	workday.GET("/runs", h.ListWorkdaySyncRuns)
	workday.GET("/runs/:run_id", h.GetWorkdaySyncRun)
	workday.GET("/directory/org-units", h.ListWorkdayOrgUnits)
	workday.GET("/directory/workers", h.ListWorkdayWorkers)
	// This endpoint is deliberately system-admin authenticated. A future
	// public Workday webhook must add provider signature verification before
	// being placed on the auth allow-list.
	workday.POST("/events", h.AcceptWorkdayEvent)
}

// RegisterEnterpriseAccessRoutes exposes the independent enterprise
// organization directory and explicit knowledge-base/document grants.
func RegisterEnterpriseAccessRoutes(r *gin.RouterGroup, h *handler.EnterpriseAccessHandler, g *rbacGuards) {
	if h == nil {
		return
	}

	directory := r.Group("/enterprise", g.Viewer())
	directory.POST("/knowledge-base-grants", h.GrantResourceBatch)
	directory.GET("/org-units", h.ListOrgUnits)
	directory.POST("/org-units", h.CreateOrgUnit)
	directory.PUT("/org-units/:org_unit_id", h.UpdateOrgUnit)
	directory.DELETE("/org-units/:org_unit_id", h.DeleteOrgUnit)
	directory.GET("/org-units/:org_unit_id/members", h.ListOrgUnitMembers)
	directory.GET("/users/:user_id/org-memberships", h.ListUserOrgMemberships)
	directory.PUT("/users/:user_id/org-memberships", h.ReplaceUserOrgMemberships)
	directory.GET("/users", h.SearchGrantUsers)

	kb := r.Group("/knowledge-bases/:id", g.Viewer())
	kb.GET(
		"/resources/:resource_type/:resource_id/grants",
		h.ListResourceGrants,
	)
	kb.GET(
		"/resources/:resource_type/:resource_id/grant-subjects",
		h.ListResourceGrantSubjects,
	)
	kb.PUT(
		"/resources/:resource_type/:resource_id/grants",
		h.GrantResource,
	)
	kb.DELETE("/resource-grants/:grant_id", h.RevokeResource)
	kb.DELETE("/folders/:folder_id", h.DeleteKnowledgeFolder)

	// Knowledge-base-officer bindings: viewable by AdminBackend users,
	// mutations restricted to SystemAdmin.
	kbOfficers := r.Group("/knowledge-bases/:id/officers", g.AdminBackend())
	{
		kbOfficers.GET("", h.ListKnowledgeBaseOfficers)
		kbOfficers.POST("", g.SystemAdmin(), h.AddKnowledgeBaseOfficer)
		kbOfficers.DELETE("/:user_id", g.SystemAdmin(), h.RemoveKnowledgeBaseOfficer)
	}
}

// RegisterChunkerDebugRoutes wires the stateless chunker preview endpoint used
// by the knowledge-base editor.
func RegisterChunkerDebugRoutes(r *gin.RouterGroup, g *rbacGuards) {
	r.POST("/chunker/preview", g.Viewer(), handler.PreviewChunking)
}

// RegisterChunkRoutes wires chunk read and mutation endpoints.
// Mutating routes resolve the parent KB and require effective KB management
// permission. Direct read grants never permit chunk edits.
func RegisterChunkRoutes(r *gin.RouterGroup, handler *handler.ChunkHandler, g *rbacGuards) {
	chunks := r.Group("/chunks")
	chunkRead := chunks
	{
		chunkRead.GET("/:knowledge_id", g.Viewer(), g.KBAccessReadFromKnowledgeIDParam("knowledge_id"), handler.ListKnowledgeChunks)
	
	// 新增：按页码查询 chunks
	chunkRead.GET("/:knowledge_id/pages/:page_number", g.Viewer(), g.KBAccessReadFromKnowledgeIDParam("knowledge_id"), handler.ListChunksByPage)
		chunkRead.GET("/by-id/:id", g.Viewer(), g.KBAccessReadFromChunkIDParam("id"), handler.GetChunkByIDOnly)
		chunks.DELETE("/:knowledge_id/:id", g.Viewer(), g.KBAccessWriteFromKnowledgeIDParam("knowledge_id"), handler.DeleteChunk)
		chunks.DELETE("/:knowledge_id", g.Viewer(), g.KBAccessWriteFromKnowledgeIDParam("knowledge_id"), handler.DeleteChunksByKnowledgeID)
		chunks.PUT("/:knowledge_id/:id", g.Viewer(), g.KBAccessWriteFromKnowledgeIDParam("knowledge_id"), handler.UpdateChunk)
		chunks.DELETE("/by-id/:id/questions", g.Viewer(), g.KBAccessWriteFromChunkIDParam("id"), handler.DeleteGeneratedQuestion)
	}
}

// RegisterKnowledgeRoutes wires document endpoints.
// Per-document writes resolve the parent KB and require effective management
// permission. Cross-KB batch writes are restricted to knowledge-domain or
// system administrators.
func RegisterKnowledgeRoutes(r *gin.RouterGroup, handler *handler.KnowledgeHandler, g *rbacGuards) {
	kb := r.Group("/knowledge-bases/:id/knowledge")
	kbRead := kb
	{
		kb.POST("/file", g.Viewer(), g.KBAccessWrite("id"), handler.CreateKnowledgeFromFile)
		kb.POST("/url", g.Viewer(), g.KBAccessWrite("id"), handler.CreateKnowledgeFromURL)
		kb.POST("/manual", g.Viewer(), g.KBAccessWrite("id"), handler.CreateManualKnowledge)
		kbRead.GET("", g.Viewer(), g.KBAccessRead("id"), handler.ListKnowledge)
		kbRead.GET("/folders", g.Viewer(), g.KBAccessRead("id"), handler.ListKnowledgeFolders)
		// Clearing all contents is destructive and remains Admin-gated.
		kb.DELETE("", g.Viewer(), g.KBAccessWrite("id"), handler.ClearKnowledgeBaseContents)
	}

	kgrp := r.Group("/knowledge")
	k := kgrp
	kRead := k
	{
		// Cross-knowledge endpoints cannot be gated on one path KB because
		// they accept arbitrary knowledge IDs. The handler must
		// fan out the access check itself. /batch and /search are read
		// routes; /move and /batch-delete stay Admin-gated and are
		// not declared for API keys.
		kRead.GET("/batch", g.Viewer(), handler.GetKnowledgeBatch)
		kRead.GET("/:id", g.Viewer(), g.KBAccessReadFromKnowledgeIDParam("id"), handler.GetKnowledge)
		kRead.GET("/:id/stages", g.Viewer(), g.KBAccessReadFromKnowledgeIDParam("id"), handler.GetKnowledgeSpans)
		kRead.GET("/:id/spans", g.Viewer(), g.KBAccessReadFromKnowledgeIDParam("id"), handler.GetKnowledgeSpans)
		k.DELETE("/:id", g.Viewer(), g.KBAccessWriteFromKnowledgeIDParam("id"), handler.DeleteKnowledge)
		k.PUT("/:id", g.Viewer(), g.KBAccessWriteFromKnowledgeIDParam("id"), handler.UpdateKnowledge)
		k.PUT("/manual/:id", g.Viewer(), g.KBAccessWriteFromKnowledgeIDParam("id"), handler.UpdateManualKnowledge)
		k.POST("/:id/reparse", g.Viewer(), g.KBAccessWriteFromKnowledgeIDParam("id"), handler.ReparseKnowledge)
		k.POST("/:id/cancel-parse", g.Viewer(), g.KBAccessWriteFromKnowledgeIDParam("id"), handler.CancelKnowledgeParse)
		kRead.GET("/:id/download", g.Viewer(), g.KBAccessReadFromKnowledgeIDParam("id"), handler.DownloadKnowledgeFile)
		kRead.GET("/:id/preview", g.Viewer(), g.KBAccessReadFromKnowledgeIDParam("id"), handler.PreviewKnowledgeFile)
		k.PUT("/image/:id/:chunk_id", g.Viewer(), g.KBAccessWriteFromKnowledgeIDParam("id"), handler.UpdateImageInfo)
		kRead.GET("/search", g.Viewer(), handler.SearchKnowledge)
		kRead.GET("/move/progress/:task_id", g.Viewer(), handler.GetKnowledgeMoveProgress)
		// Batch / cross-KB write ops stay Admin-gated for JWT and are
		// NOT declared for API keys (default-deny): they fan out to arbitrary
		// KBs with no single owning KB to bound a key's scope against.
		kgrp.PUT("/tags", g.Viewer(), handler.UpdateKnowledgeTagBatch)
		kgrp.POST("/batch-reparse", g.Viewer(), handler.BatchReparseKnowledge)
		kgrp.POST("/batch-delete", g.Viewer(), handler.BatchDeleteKnowledge)
		kgrp.POST("/move", g.Viewer(), handler.MoveKnowledge)
	}
}

// RegisterFAQRoutes wires FAQ entry and search endpoints.
// FAQ entries are KB content: reads require KB read access and mutations
// require effective KB management permission. Search is read-only.
func RegisterFAQRoutes(r *gin.RouterGroup, handler *handler.FAQHandler, g *rbacGuards) {
	if handler == nil {
		return
	}
	faq := r.Group("/knowledge-bases/:id/faq")
	faqRead := faq
	{
		// KBAccessRead/Write resolve enterprise access and install the owning
		// knowledge-domain context for downstream resource lookups.
		faqRead.GET("/entries", g.Viewer(), g.KBAccessRead("id"), handler.ListEntries)
		faqRead.GET("/entries/export", g.Viewer(), g.KBAccessRead("id"), handler.ExportEntries)
		faqRead.GET("/entries/:entry_id", g.Viewer(), g.KBAccessRead("id"), handler.GetEntry)
		faq.POST("/entries", g.Viewer(), g.KBAccessWrite("id"), handler.UpsertEntries)
		faq.POST("/entry", g.Viewer(), g.KBAccessWrite("id"), handler.CreateEntry)
		faq.PUT("/entries/:entry_id", g.Viewer(), g.KBAccessWrite("id"), handler.UpdateEntry)
		faq.POST("/entries/:entry_id/similar-questions", g.Viewer(), g.KBAccessWrite("id"), handler.AddSimilarQuestions)
		// Unified batch update API - supports is_enabled, is_recommended, tag_id
		faq.PUT("/entries/fields", g.Viewer(), g.KBAccessWrite("id"), handler.UpdateEntryFieldsBatch)
		faq.PUT("/entries/tags", g.Viewer(), g.KBAccessWrite("id"), handler.UpdateEntryTagBatch)
		faq.DELETE("/entries", g.Viewer(), g.KBAccessWrite("id"), handler.DeleteEntries)
		// Search is a read route: scoped API keys may call it with retrieve
		// even though POST is otherwise an unsafe method.
		faqRead.POST("/search", g.Viewer(), g.KBAccessRead("id"), handler.SearchFAQ)
		// FAQ import result display status
		faq.PUT("/import/last-result/display", g.Viewer(), g.KBAccessWrite("id"), handler.UpdateLastImportResultDisplayStatus)
	}
	// FAQ import progress is outside a single knowledge-base path.
	faqImport := r.Group("/faq/import")
	{
		faqImport.GET("/progress/:task_id", g.Viewer(), handler.GetImportProgress)
	}
}

// RegisterKnowledgeBaseRoutes wires knowledge-base management endpoints.
func RegisterKnowledgeBaseRoutes(r *gin.RouterGroup, handler *handler.KnowledgeBaseHandler, g *rbacGuards) {
	kbgrp := r.Group("/knowledge-bases")
	kb := kbgrp
	kbManagement := kb
	{
		// The handler verifies that the caller may create a knowledge base in
		// the requested knowledge domain.
		kbgrp.POST("", g.Viewer(), handler.CreateKnowledgeBase)
		// List results are filtered to the current user's effective access.
		kb.GET("", g.Viewer(), handler.ListKnowledgeBases)
		kb.GET("/:id", g.Viewer(), g.KBAccessRead("id"), handler.GetKnowledgeBase)
		kbManagement.PUT("/:id", g.Viewer(), g.KBAccessWrite("id"), handler.UpdateKnowledgeBase)
		kbManagement.DELETE("/:id", g.Viewer(), g.KBAccessWrite("id"), handler.DeleteKnowledgeBase)
		// Pin state is per user and knowledge base. Anyone with effective read
		// access to the knowledge base may pin it for themselves;
		// no edit permission is required. The route only requires KB read access
		// so callers can't poke at KBs they can't see.
		kb.PUT("/:id/pin", g.Viewer(), g.KBAccessRead("id"), handler.TogglePinKnowledgeBase)
		// Public toggle requires KB management permission.
		kbManagement.PUT("/:id/public", g.Viewer(), g.KBAccessWrite("id"), handler.TogglePublicKnowledgeBase)
		// Hybrid search is read-only and requires effective KB read access.
		// POST is preferred; GET remains available for the browser client.
		kb.POST("/:id/hybrid-search", g.Viewer(), g.KBAccessRead("id"), handler.HybridSearch)
		kb.GET("/:id/hybrid-search", g.Viewer(), g.KBAccessRead("id"), handler.HybridSearch)
		kbgrp.POST("/copy", g.Viewer(), handler.CopyKnowledgeBase)
		kb.GET("/copy/progress/:task_id", g.Viewer(), handler.GetKBCloneProgress)
		kb.GET("/:id/move-targets", g.Viewer(), g.KBAccessWrite("id"), handler.ListMoveTargets)
	}
}

// RegisterKnowledgeTagRoutes wires knowledge-base tag endpoints.
// Tags are KB metadata: reads require KB read access and mutations require
// effective KB management permission.
func RegisterKnowledgeTagRoutes(r *gin.RouterGroup, tagHandler *handler.TagHandler, g *rbacGuards) {
	if tagHandler == nil {
		return
	}
	kbTags := r.Group("/knowledge-bases/:id/tags")
	kbTagsRead := kbTags
	{
		// KBAccessRead/Write resolve enterprise access and install the owning
		// knowledge-domain context for downstream resource lookups.
		kbTagsRead.GET("", g.Viewer(), g.KBAccessRead("id"), tagHandler.ListTags)
		kbTags.POST("", g.Viewer(), g.KBAccessWrite("id"), tagHandler.CreateTag)
		kbTags.PUT("/:tag_id", g.Viewer(), g.KBAccessWrite("id"), tagHandler.UpdateTag)
		kbTags.DELETE("/:tag_id", g.Viewer(), g.KBAccessWrite("id"), tagHandler.DeleteTag)
	}
}

// RegisterMessageRoutes wires user-owned message endpoints.
func RegisterMessageRoutes(r *gin.RouterGroup, handler *handler.MessageHandler, g *rbacGuards) {
	// Session ownership is enforced by the message service.
	messages := r.Group("/messages")
	{
		messages.GET("/:session_id/load", g.Viewer(), handler.LoadMessages)
		messages.DELETE("/:session_id/:id", g.Viewer(), handler.DeleteMessage)
		messages.PUT("/:session_id/:id/feedback", g.Viewer(), handler.SetMessageFeedback)
		messages.DELETE("/:session_id/:id/feedback", g.Viewer(), handler.DeleteMessageFeedback)
	}
}

// RegisterSessionRoutes wires user-owned conversation endpoints.
// Sessions are per-user resources; the handler enforces user ownership.
func RegisterSessionRoutes(r *gin.RouterGroup, handler *session.Handler, g *rbacGuards) {
	// Sessions are per-user chat state, not knowledge-base content. The
	// chat capability lets a scoped key run the full conversation flow
	// (create/manage its own sessions) without knowledge-management rights.
	sessions := r.Group("/sessions", g.Viewer())
	{
		sessions.POST("", handler.CreateSession)
		sessions.DELETE("/batch", handler.BatchDeleteSessions)
		sessions.GET("/:id", handler.GetSession)
		sessions.GET("", handler.GetSessions)
		sessions.PUT("/:id", handler.UpdateSession)
		sessions.DELETE("/:id", handler.DeleteSession)
		sessions.DELETE("/:id/messages", handler.ClearSessionMessages)
		sessions.POST("/:session_id/generate_title", handler.GenerateTitle)
		sessions.POST("/:session_id/stop", handler.StopSession)
		// POST and DELETE share this path but gin maintains a separate radix tree
		// per HTTP verb, and the existing trees use different wildcard names
		// (POST uses :session_id, DELETE uses :id). Use whatever matches each
		// tree to avoid "wildcard conflicts" panic at route registration.
		sessions.POST("/:session_id/pin", handler.PinSession)
		sessions.DELETE("/:id/pin", handler.UnpinSession)
		sessions.GET("/continue-stream/:session_id", handler.ContinueStream)
	}
}

// RegisterChatRoutes wires regular RAG and agent chat endpoints. Session
// ownership and explicit knowledge grants are enforced inside the handlers.
func RegisterChatRoutes(r *gin.RouterGroup, handler *session.Handler, g *rbacGuards) {
	// These POST routes append messages and run generation, so a scoped key
	// needs the explicit chat capability.
	knowledgeChat := r.Group("/knowledge-chat", g.Viewer())
	{
		knowledgeChat.POST("/:session_id", handler.KnowledgeQA)
	}

	// Agent-based chat
	agentChat := r.Group("/agent-chat", g.Viewer())
	{
		agentChat.POST("/:session_id", handler.AgentQA)
	}

	knowledgeSearch := r.Group("/knowledge-search", g.Viewer())
	{
		knowledgeSearch.POST("", handler.SearchKnowledge)
	}
}

// RegisterKnowledgeDomainRoutes exposes knowledge-management domains.
// System administrators create/delete domains and manage platform runtime
// configuration. A domain administrator may update only an assigned domain.
func RegisterKnowledgeDomainRoutes(
	r *gin.RouterGroup,
	domainHandler *handler.KnowledgeDomainHandler,
	adminHandler *handler.KnowledgeDomainAdminHandler,
	auditLogHandler *handler.AuditLogHandler,
	g *rbacGuards,
) {
	r.GET("/system/runtime-config/:key", g.SystemAdmin(), domainHandler.GetPlatformRuntimeConfig)
	r.PUT("/system/runtime-config/:key", g.SystemAdmin(), domainHandler.UpdatePlatformRuntimeConfig)

	domains := r.Group("/knowledge-domains")
	domains.POST("", g.SystemAdmin(), domainHandler.CreateKnowledgeDomain)
	domains.GET("", domainHandler.ListKnowledgeDomains)
	domains.GET("/all", g.SystemAdmin(), domainHandler.ListAllKnowledgeDomains)
	domains.GET("/search", g.SystemAdmin(), domainHandler.SearchKnowledgeDomains)

	domain := domains.Group("/:id", g.PathKnowledgeDomainMatch())
	domain.GET("", domainHandler.GetKnowledgeDomain)
	domain.PUT("", domainHandler.UpdateKnowledgeDomain)
	domain.DELETE("", g.SystemAdmin(), domainHandler.DeleteKnowledgeDomain)
	if adminHandler != nil {
		domain.GET("/administrators", adminHandler.List)
		domain.POST("/administrators", g.SystemAdmin(), adminHandler.Grant)
		domain.DELETE("/administrators/:user_id", g.SystemAdmin(), adminHandler.Revoke)
	}
	if auditLogHandler != nil {
		domain.GET("/audit-log", auditLogHandler.ListKnowledgeDomainAuditLog)
	}
}

// Models are platform-wide infrastructure (LLM credentials, embeddings and
// rerankers). Reads require authentication; mutations are restricted by the
// platform administration guards configured below.
func RegisterModelRoutes(
	r *gin.RouterGroup,
	handler *handler.ModelHandler,
	credHandler *handler.ModelCredentialsHandler,
	g *rbacGuards,
) {
	models := r.Group("/models")
	{
		// Authenticated users may read the provider catalog and model metadata.
		models.GET("/providers", g.Viewer(), handler.ListModelProviders)
		// Model mutations and diagnostics require system administration.
		models.POST("", g.Admin(), handler.CreateModel)
		models.GET("", g.Viewer(), handler.ListModels)
		models.POST("/:id/debug", g.Admin(), handler.DebugModel)
		models.GET("/:id", g.Viewer(), handler.GetModel)
		models.PUT("/:id", g.Admin(), handler.UpdateModel)
		models.DELETE("/:id", g.Admin(), handler.DeleteModel)
		// Per-field credential subresource; see model_credentials.go.
		models.PUT("/:id/credentials", g.Admin(), credHandler.Put)
		models.DELETE("/:id/credentials/:field", g.Admin(), credHandler.DeleteField)
	}
}

// RegisterEvaluationRoutes registers evaluation endpoints. Starting an
// evaluation drives LLM calls and is therefore restricted to system
// administrators. Reading a completed result only requires authentication.
func RegisterEvaluationRoutes(r *gin.RouterGroup, handler *handler.EvaluationHandler, g *rbacGuards) {
	evaluationRoutes := r.Group("/evaluation")
	{
		evaluationRoutes.POST("", g.Admin(), handler.Evaluation)
		evaluationRoutes.GET("", g.Viewer(), handler.GetEvaluationResult)
	}
}

func RegisterInitializationRoutes(r *gin.RouterGroup, handler *handler.InitializationHandler, g *rbacGuards) {
	r.GET("/initialization/config/:kbId", g.Viewer(), g.KBAccessRead("kbId"), handler.GetCurrentConfigByKB)
	r.POST("/initialization/initialize/:kbId", g.Viewer(), g.KBAccessWrite("kbId"), handler.InitializeByKB)
	r.PUT("/initialization/config/:kbId", g.Viewer(), g.KBAccessWrite("kbId"), handler.UpdateKBConfig)

	// Platform-level model connectivity and extraction diagnostics.
	r.POST("/initialization/remote/check", g.Admin(), handler.CheckRemoteModel)
	r.POST("/initialization/embedding/test", g.Admin(), handler.TestEmbeddingModel)
	r.POST("/initialization/rerank/check", g.Admin(), handler.CheckRerankModel)
	r.POST("/initialization/asr/check", g.Admin(), handler.CheckASRModel)
	r.POST("/initialization/multimodal/test", g.Admin(), handler.TestMultimodalFunction)

	r.POST("/initialization/extract/text-relation", g.Admin(), handler.ExtractTextRelations)
	r.POST("/initialization/extract/fabri-tag", g.Admin(), handler.FabriTag)
	r.POST("/initialization/extract/fabri-text", g.Admin(), handler.FabriText)
}

// RegisterSystemRoutes registers system information routes
//
// Authenticated users may read status. Connectivity checks and reconnect
// operations actively probe infrastructure and require system administration.
func RegisterSystemRoutes(r *gin.RouterGroup, handler *handler.SystemHandler, g *rbacGuards) {
	systemRoutes := r.Group("/system")
	{
		systemRoutes.GET("/info", g.Viewer(), handler.GetSystemInfo)
		systemRoutes.GET("/parser-engines", g.Viewer(), handler.ListParserEngines)
		systemRoutes.POST("/parser-engines/check", g.SystemAdmin(), handler.CheckParserEngines)
		systemRoutes.POST("/docreader/reconnect", g.SystemAdmin(), handler.ReconnectDocReader)
		systemRoutes.GET("/storage-engine-status", g.Viewer(), handler.GetStorageEngineStatus)
		systemRoutes.POST("/storage-engine-check", g.SystemAdmin(), handler.CheckStorageEngine)
	}
}

// RegisterSystemAdminRoutes registers system administration routes.
//
// Admin-backend routes (user management, audit logs) are gated to
// AdminBackend (system_admin OR knowledge_officer). Super-admin-only
// operations (promote/revoke, system settings) remain SystemAdmin-gated.
//
// Mounted under /api/v1/system/admin/* so the URL scheme stays aligned
// with the existing /api/v1/system/info family. Front-end clients live
// in frontend/src/api/system/index.ts.
//
// auditLogHandler may be nil in environments wired without the audit
// dependency; the /audit-log subroute is then omitted. This mirrors
// the optional wiring in RegisterKnowledgeDomainRoutes.
func RegisterSystemAdminRoutes(
	r *gin.RouterGroup,
	handler *handler.SystemHandler,
	auditLogHandler *handler.AuditLogHandler,
	g *rbacGuards,
) {
	// Super-admin-only operations.
	superOnly := r.Group("/system/admin", g.SystemAdmin())
	{
		// P0: SystemAdmin role management
		superOnly.POST("/promote", handler.PromoteUserToSystemAdmin)
		superOnly.POST("/revoke", handler.RevokeSystemAdmin)
		superOnly.GET("/list", handler.ListSystemAdmins)

		// P1: platform-wide system settings (DB-backed runtime tunables).
		superOnly.GET("/settings", handler.ListSystemSettings)
		superOnly.GET("/settings/:key", handler.GetSystemSetting)
		superOnly.PUT("/settings/:key", handler.UpdateSystemSetting)
		superOnly.DELETE("/settings/:key", handler.ResetSystemSetting)

		// Write the current default quota onto every existing knowledge domain.
		superOnly.POST(
			"/knowledge-domains/apply-default-storage-quota",
			handler.ApplyDefaultStorageQuotaToAllKnowledgeDomains,
		)
	}

	// Admin-backend routes: system_admin AND knowledge_officer can access.
	adminRoutes := r.Group("/system/admin", g.AdminBackend())
	{
		adminRoutes.GET("/users", handler.ListUsers)
		adminRoutes.POST("/users", handler.CreateUser)
		adminRoutes.GET("/users/:id", handler.GetUser)
		adminRoutes.POST("/users/ban", handler.BanUser)
		adminRoutes.POST("/users/unban", handler.UnbanUser)
		adminRoutes.POST("/users/offline", handler.OfflineUser)
		adminRoutes.PUT("/users/roles", handler.BatchUpdateUserRoles)
		adminRoutes.PUT("/users/roles/single", handler.UpdateUserRoles)
		adminRoutes.PUT("/users/roles/operator", handler.BatchUpdateOperatorRole)
		adminRoutes.PUT("/users/roles/operator/single", handler.UpdateOperatorRole)

		// Per-user audit feed for the user detail page.
		if auditLogHandler != nil {
			adminRoutes.GET("/users/:id/audit-log", auditLogHandler.GetUserAuditLog)
		}

		// Platform-wide audit feed (knowledge_domain_id=0 rows).
		if auditLogHandler != nil {
			adminRoutes.GET("/audit-log", auditLogHandler.ListSystemAuditLog)
		}
	}
}

// RegisterMCPServiceRoutes registers platform-level MCP service routes.
//
// Authenticated users may read service metadata and manage their own OAuth
// authorization. System administrators manage service definitions, credentials,
// connection tests, and approval policies.
func RegisterMCPServiceRoutes(
	r *gin.RouterGroup,
	handler *handler.MCPServiceHandler,
	credHandler *handler.MCPCredentialsHandler,
	oauthHandler *handler.MCPOAuthHandler,
	g *rbacGuards,
) {
	// MCP OAuth provider redirect. Registered OUTSIDE the /mcp-services group
	// to avoid a static-vs-":id" route conflict, and left unauthenticated
	// (allow-listed in middleware/auth.go) because the third-party browser
	// redirect carries no platform bearer token; the single-use state authenticates it.
	r.GET("/mcp-oauth/callback", oauthHandler.Callback)

	mcpServices := r.Group("/mcp-services")
	{
		// System administrators manage MCP services and credentials.
		mcpServices.POST("", g.Admin(), handler.CreateMCPService)
		// Authenticated users may read service metadata.
		mcpServices.GET("", g.Viewer(), handler.ListMCPServices)
		mcpServices.GET("/:id", g.Viewer(), handler.GetMCPService)
		mcpServices.PUT("/:id", g.Admin(), handler.UpdateMCPService)
		mcpServices.DELETE("/:id", g.Admin(), handler.DeleteMCPService)
		// Connection tests probe external infrastructure.
		mcpServices.POST("/:id/test", g.Admin(), handler.TestMCPService)
		mcpServices.GET("/:id/tools", g.Viewer(), handler.GetMCPServiceTools)
		mcpServices.GET("/:id/resources", g.Viewer(), handler.GetMCPServiceResources)
		// Per-field credential subresource: secrets never travel via the main
		// PUT body. See internal/handler/mcp_credentials.go for the contract.
		mcpServices.PUT("/:id/credentials", g.Admin(), credHandler.Put)
		mcpServices.DELETE("/:id/credentials/:field", g.Admin(), credHandler.DeleteField)
		// Users may read approval policy; system administrators set it.
		mcpServices.GET("/:id/tool-approvals", g.Viewer(), handler.ListMCPToolApprovals)
		mcpServices.PUT("/:id/tool-approvals/:tool_name", g.Admin(), handler.SetMCPToolApproval)
		// Per-user OAuth authorization flow. Any authenticated user may authorize,
		// inspect, or
		// revoke their own token; the callback is the separate public route
		// registered above.
		mcpServices.POST("/:id/oauth/authorize-url", g.Viewer(), oauthHandler.AuthorizeURL)
		mcpServices.GET("/:id/oauth/status", g.Viewer(), oauthHandler.Status)
		mcpServices.DELETE("/:id/oauth/token", g.Viewer(), oauthHandler.Revoke)
	}

	// /agent tool-approval + OAuth resolution are interactive human flows;
	// not declared for API keys (default-deny).
	agentTool := r.Group("/agent")
	{
		// Any authenticated user may resolve a pending approval surfaced in that
		// user's agent conversation. The handler validates the pending request.
		agentTool.POST("/tool-approvals/:pending_id", g.Viewer(), handler.ResolveToolApproval)
		// Resume an agent run paused on an in-conversation MCP OAuth prompt.
		// The handler validates the pending request and initiating principal.
		agentTool.POST("/mcp-oauth-resolutions/:pending_id", g.Viewer(), oauthHandler.ResolveMCPOAuth)
		agentTool.POST("/mcp-oauth-resolutions/:pending_id/cancel", g.Viewer(), oauthHandler.CancelMCPOAuth)
	}
}

// RegisterWebSearchRoutes registers web search routes
func RegisterWebSearchRoutes(r *gin.RouterGroup, webSearchHandler *handler.WebSearchHandler, g *rbacGuards) {
	// Authenticated users may read the provider catalog.
	webSearch := r.Group("/web-search")
	{
		webSearch.GET("/providers", g.Viewer(), webSearchHandler.GetProviders)
	}
}

// RegisterWebSearchProviderRoutes registers CRUD routes for web search
// provider configurations.
//
// Provider rows hold external service credentials (Bing, Tavily, Google,
// etc.). Authenticated users may read provider metadata; system administrators
// manage providers, credentials, and connection tests.
func RegisterWebSearchProviderRoutes(
	r *gin.RouterGroup,
	h *handler.WebSearchProviderHandler,
	credHandler *handler.WebSearchProviderCredentialsHandler,
	g *rbacGuards,
) {
	providers := r.Group("/web-search-providers")
	{
		// Provider types are metadata used by configuration forms.
		providers.GET("/types", g.Viewer(), h.ListProviderTypes)
		// Test raw credentials without persisting them.
		providers.POST("/test", g.Admin(), h.TestProviderRaw)
		// CRUD
		providers.POST("", g.Admin(), h.CreateProvider)
		providers.GET("", g.Viewer(), h.ListProviders)
		providers.GET("/:id", g.Viewer(), h.GetProvider)
		providers.PUT("/:id", g.Admin(), h.UpdateProvider)
		providers.DELETE("/:id", g.Admin(), h.DeleteProvider)
		// Per-field credential subresource.
		providers.PUT("/:id/credentials", g.Admin(), credHandler.Put)
		providers.DELETE("/:id/credentials/:field", g.Admin(), credHandler.DeleteField)
		// Test an existing saved provider.
		providers.POST("/:id/test", g.Admin(), h.TestProviderByID)
	}
}

// RegisterVectorStoreRoutes registers CRUD routes for vector store configurations.
//
// Vector stores are platform infrastructure. Authenticated users may read
// metadata; system administrators manage stores and run connection tests.
func RegisterVectorStoreRoutes(r *gin.RouterGroup, h *handler.VectorStoreHandler, g *rbacGuards) {
	stores := r.Group("/vector-stores")
	{
		// Knowledge base editors consume only this credential-free projection.
		stores.GET("/options", g.Viewer(), h.ListStoreOptions)
		// Full infrastructure metadata is reserved for system administrators.
		stores.GET("/types", g.SystemAdmin(), h.ListStoreTypes)
		// Test raw credentials without persisting them.
		stores.POST("/test", g.SystemAdmin(), h.TestStoreRaw)
		// CRUD
		stores.POST("", g.SystemAdmin(), h.CreateStore)
		stores.GET("", g.SystemAdmin(), h.ListStores)
		stores.GET("/:id", g.SystemAdmin(), h.GetStore)
		stores.PUT("/:id", g.SystemAdmin(), h.UpdateStore)
		stores.DELETE("/:id", g.SystemAdmin(), h.DeleteStore)
		// Test an existing saved or environment-backed store.
		stores.POST("/:id/test", g.SystemAdmin(), h.TestStoreByID)
	}
}

// RegisterCustomAgentRoutes registers custom agent routes.
//
// Reads are available to authenticated users. Creating, copying, updating, and
// deleting platform-wide custom agents require system-administrator rights.
// Built-in agents may be used but cannot be modified through these routes.
func RegisterCustomAgentRoutes(r *gin.RouterGroup, agentHandler *handler.CustomAgentHandler, g *rbacGuards) {
	agents := r.Group("/agents")
	// agentsRead are the agent read endpoints. They stay full-access only for
	// plain scoped keys (agent config can carry sensitive model/MCP bindings),
	// but read_agents, chat, or manage_agents may read them.
	agentsRead := agents
	// agentsWrite are the Admin-only agent authoring endpoints.
	agentsWrite := agents
	{
		// Placeholder definitions must be registered before /:id.
		agentsRead.GET("/placeholders", g.Viewer(), agentHandler.GetPlaceholders)
		// Authenticated users may list agent presets and definitions.
		agentsRead.GET("/type-presets", g.Viewer(), agentHandler.GetAgentTypePresets)
		// System administrators author platform-wide agents.
		agentsWrite.POST("", g.Admin(), agentHandler.CreateAgent)
		agentsRead.GET("", g.Viewer(), agentHandler.ListAgents)
		agentsRead.GET("/:id", g.Viewer(), agentHandler.GetAgent)
		agentsWrite.PUT("/:id", g.Admin(), agentHandler.UpdateAgent)
		agentsWrite.DELETE("/:id", g.Admin(), agentHandler.DeleteAgent)
		agentsWrite.POST("/:id/copy", g.Admin(), agentHandler.CopyAgent)
	}
	// Registered outside the group because this endpoint has a distinct capability set.
	r.GET("/agents/:id/suggested-questions", g.Viewer(), agentHandler.GetSuggestedQuestions)
}

// RegisterSuggestedQuestionRoutes registers Agent-independent FAQ suggestions.
func RegisterSuggestedQuestionRoutes(r *gin.RouterGroup, h *handler.SuggestedQuestionHandler, g *rbacGuards) {
	suggestions := r.Group("/suggested-questions")
	suggestions.GET("", g.Viewer(), h.List)
	suggestions.PUT("/config", g.SystemAdmin(), h.Configure)
}

// RegisterExchangeRateRoutes exposes the global RMB/CHF configuration.
func RegisterExchangeRateRoutes(r *gin.RouterGroup, h *handler.ExchangeRateHandler, g *rbacGuards) {
	rates := r.Group("/exchange-rate")
	rates.GET("", g.Viewer(), h.Get)
	rates.PUT("/config", g.SystemAdmin(), h.Configure)
}

// RegisterAdminAnswerRecordRoutes exposes cross-user Q&A records to system administrators only.
func RegisterAdminAnswerRecordRoutes(r *gin.RouterGroup, h *handler.AdminAnswerRecordHandler, g *rbacGuards) {
	r.GET("/system/admin/answer-records", g.SystemAdmin(), h.List)
	r.GET("/system/admin/answer-records/export", g.SystemAdmin(), h.Export)
}

// RegisterUserFavoriteRoutes wires the per-user starred-resource endpoints.
//
// Authorization: the handler always derives the user ID from the authenticated
// context. There is no administrator path for another user's favorites, so
// the endpoints intentionally
// don't follow the OwnedXOrAdmin pattern: favorites aren't owned by the
// resource's creator, they're owned by the user *doing* the starring.
func RegisterUserFavoriteRoutes(r *gin.RouterGroup, h *handler.UserResourceFavoriteHandler, g *rbacGuards) {
	// Favorites are per-user; not declared for API keys (default-deny).
	favs := r.Group("/user/favorites")
	{
		favs.GET("", g.Viewer(), h.ListFavorites)
		favs.POST("", g.Viewer(), h.AddFavorite)
		favs.DELETE("/:type/:id", g.Viewer(), h.RemoveFavorite)
	}
}

// RegisterSkillRoutes registers skill routes.
//
// Enterprise Skills are instruction-only and disabled by default. The
// read endpoint remains available when the feature flag is enabled.
func RegisterSkillRoutes(r *gin.RouterGroup, skillHandler *handler.SkillHandler, g *rbacGuards) {
	skills := r.Group("/skills")
	{
		skills.GET("", g.Viewer(), skillHandler.ListSkills)
	}
}

// RegisterDashboardRoutes registers dashboard analytics routes.
//
// Dashboard reads are available to authenticated users with viewer access.
func RegisterDashboardRoutes(r *gin.RouterGroup, h *handler.DashboardHandler, g *rbacGuards) {
	if h == nil {
		return
	}
	dash := r.Group("/dashboard", g.Viewer())
	{
		dash.GET("/knowledge-base-stats", h.GetKnowledgeBaseStats)
		dash.GET("/chat-stats", h.GetChatStats)
		dash.GET("/overview", h.GetOverview)
		// Manual recompute of the pre-aggregated daily stats. Mutates the
		// dashboard_daily_stats table, so it stays admin-gated even though the
		// read endpoints are viewer-accessible.
		dash.POST("/stats/recompute", g.Admin(), h.RecomputeStats)
	}
}

// trustedProxies returns the proxy CIDRs/IPs whose X-Forwarded-For headers
// gin should trust when resolving the client IP. Defaults to loopback and
// private ranges (covers the bundled nginx in a container network); override
// with ROCHE_KAP_TRUSTED_PROXIES (comma-separated). An explicit empty value
// disables proxy trust entirely so ClientIP() returns the direct peer.
func trustedProxies() []string {
	raw, ok := os.LookupEnv("ROCHE_KAP_TRUSTED_PROXIES")
	if !ok {
		return []string{
			"127.0.0.0/8",
			"::1/128",
			"10.0.0.0/8",
			"172.16.0.0/12",
			"192.168.0.0/16",
			"fc00::/7",
		}
	}
	proxies := make([]string, 0)
	for _, p := range strings.Split(raw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			proxies = append(proxies, p)
		}
	}
	return proxies
}

// serveFrontendStatic registers a middleware that serves the frontend SPA
// from the ./web directory if it exists. Must be called BEFORE auth middleware
// so static files are served without authentication.
func serveFrontendStatic(r *gin.Engine) {
	webDir := os.Getenv("ROCHE_KAP_WEB_DIR")
	if webDir == "" {
		webDir = "./web"
	}
	absDir, _ := filepath.Abs(webDir)
	indexPath := filepath.Join(absDir, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		return
	}

	logger.Infof(context.Background(), "[Router] Serving frontend static files from %s", absDir)

	fs := http.Dir(absDir)
	fileServer := http.FileServer(fs)

	r.Use(func(c *gin.Context) {
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Next()
			return
		}
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/health") || strings.HasPrefix(path, "/swagger/") {
			c.Next()
			return
		}
		fullPath := filepath.Join(absDir, path)
		if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
			setFrontendCacheHeaders(c.Writer, path)
			fileServer.ServeHTTP(c.Writer, c.Request)
			c.Abort()
			return
		}
		setFrontendCacheHeaders(c.Writer, "/index.html")
		c.File(indexPath)
		c.Abort()
	})
}

// setFrontendCacheHeaders sets Cache-Control headers for frontend static resources.
// setFrontendCacheHeaders applies cache policy to embedded frontend files.
// setFrontendCacheHeaders applies cache policy to embedded frontend files.
func setFrontendCacheHeaders(w http.ResponseWriter, path string) {
	if strings.HasPrefix(path, "/assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}
	w.Header().Set("Cache-Control", "no-cache, must-revalidate")
}

// serveFiles serves files via query parameters and knowledgeDomain storage settings.
// It is registered after auth middleware, so knowledgeDomain context comes from authentication.
//
// Route:
//   - /files?file_path=<provider://...>
type getRouteRegistrar interface {
	GET(string, ...gin.HandlerFunc) gin.IRoutes
}

// newFileServeHandler builds the authenticated file-proxy handler. KnowledgeDomain
// ownership of the requested path is enforced via ValidateStoragePathKnowledgeDomain.
func newFileServeHandler(globalFileService interfaces.FileService) gin.HandlerFunc {
	baseDir := os.Getenv("LOCAL_STORAGE_BASE_DIR")
	if baseDir == "" {
		baseDir = "/data/files"
	}
	absDir, _ := filepath.Abs(baseDir)
	if info, err := os.Stat(absDir); err != nil || !info.IsDir() {
		if err := os.MkdirAll(absDir, 0o755); err != nil {
			logger.Warnf(context.Background(), "[Router] Cannot create local storage dir %s: %v", absDir, err)
		}
	}

	return func(c *gin.Context) {
		filePath := strings.TrimSpace(c.Query("file_path"))
		if filePath == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing required parameter: file_path"})
			return
		}
		if strings.Contains(filePath, "..") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file path"})
			return
		}

		provider := types.ParseProviderScheme(filePath)

		knowledgeDomain, _ := c.Request.Context().Value(types.KnowledgeDomainInfoContextKey).(*types.KnowledgeDomain)
		if knowledgeDomain == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized: knowledgeDomain context missing"})
			return
		}

		if err := secutils.ValidateStoragePathKnowledgeDomain(filePath, knowledgeDomain.ID); err != nil {
			logger.Warnf(context.Background(), "[Router] /files denied cross-knowledgeDomain or invalid path: knowledge_domain_id=%d file_path=%q err=%v", knowledgeDomain.ID, filePath, err)
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: file path not accessible"})
			return
		}

		var (
			fileSvc          interfaces.FileService
			resolvedProvider string
			err              error
		)

		if knowledgeDomain.StorageEngineConfig != nil {
			fileSvc, resolvedProvider, err = filesvc.NewFileServiceFromStorageConfig(provider, knowledgeDomain.StorageEngineConfig, absDir)
		} else {
			err = http.ErrMissingFile
		}
		if err != nil {
			globalStorageType := strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_TYPE")))
			if globalStorageType == "" {
				globalStorageType = "local"
			}
			if provider == globalStorageType && globalFileService != nil {
				logger.Warnf(context.Background(), "[Router] /files knowledgeDomain storage config missing or invalid, fallback to global file service: knowledge_domain_id=%d provider=%s err=%v", knowledgeDomain.ID, provider, err)
				fileSvc = globalFileService
				resolvedProvider = globalStorageType
			} else {
				logger.Warnf(context.Background(), "[Router] /files resolve file service failed without fallback: knowledge_domain_id=%d provider=%s global_storage_type=%s err=%v", knowledgeDomain.ID, provider, globalStorageType, err)
				c.Status(http.StatusBadRequest)
				return
			}
		}

		reader, err := fileSvc.GetFile(c.Request.Context(), filePath)
		if err != nil {
			logger.Warnf(context.Background(), "[Router] /files get file failed: knowledge_domain_id=%d provider=%s path=%q err=%v", knowledgeDomain.ID, resolvedProvider, filePath, err)
			c.Status(http.StatusNotFound)
			return
		}
		defer reader.Close()

		contentType, inline := secutils.SafeContentTypeByFilename(filePath)
		c.Header("Content-Type", contentType)
		c.Header("X-Content-Type-Options", "nosniff")
		if !inline {
			c.Header("Content-Disposition", "attachment")
		}
		c.Header("Cache-Control", "public, max-age=86400")
		c.Status(http.StatusOK)
		if _, err := io.Copy(c.Writer, reader); err != nil {
			logger.Warnf(context.Background(), "[Router] /files write response failed: %v", err)
		}
	}
}

func serveFiles(r getRouteRegistrar, globalFileService interfaces.FileService) {
	logger.Infof(context.Background(), "[Router] Serving files from /files")
	r.GET("/files", newFileServeHandler(globalFileService))
}

// servePresignedFiles serves files via HMAC-signed URLs without requiring authentication.
// This serves images referenced by generated answers and model requests.
//
// Routes:
//   - GET  /api/v1/files/presigned?file_path=<provider://...>&knowledge_domain_id=<id>&expires=<unix>&sig=<hmac>
//   - HEAD /api/v1/files/presigned?...  (clients may validate metadata first)
//
// Failure paths log client IP + User-Agent + a truncated file path so operators
// can correlate a failed fetch with the signing log.
func servePresignedFiles(r *gin.Engine, knowledgeDomainService interfaces.KnowledgeDomainService) {
	baseDir := os.Getenv("LOCAL_STORAGE_BASE_DIR")
	if baseDir == "" {
		baseDir = "/data/files"
	}
	absDir, _ := filepath.Abs(baseDir)

	handler := presignedFileHandler(knowledgeDomainService, absDir)
	r.GET("/api/v1/files/presigned", handler)
	r.HEAD("/api/v1/files/presigned", handler)
}

// presignedFileHandler returns the shared Gin handler used by both GET and HEAD.
// For HEAD requests it returns the same status + headers but does not stream
// the body, saving a full read of the backing object.
func presignedFileHandler(knowledgeDomainService interfaces.KnowledgeDomainService, absDir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		clientIP := c.ClientIP()
		userAgent := c.Request.UserAgent()

		filePath := strings.TrimSpace(c.Query("file_path"))
		knowledgeDomainIDStr := strings.TrimSpace(c.Query("knowledge_domain_id"))
		expiresStr := strings.TrimSpace(c.Query("expires"))
		sig := strings.TrimSpace(c.Query("sig"))

		if filePath == "" || knowledgeDomainIDStr == "" || expiresStr == "" || sig == "" {
			logger.Warnf(ctx, "[Router] /files/presigned missing params: client_ip=%s ua=%q file_path=%q knowledge_domain_id=%q expires=%q has_sig=%v",
				clientIP, userAgent, filePath, knowledgeDomainIDStr, expiresStr, sig != "")
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing required parameters"})
			return
		}
		if strings.Contains(filePath, "..") {
			logger.Warnf(ctx, "[Router] /files/presigned rejected path traversal: client_ip=%s file_path=%q", clientIP, filePath)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file path"})
			return
		}

		knowledgeDomainID, err := strconv.ParseUint(knowledgeDomainIDStr, 10, 64)
		if err != nil {
			logger.Warnf(ctx, "[Router] /files/presigned invalid knowledge_domain_id: client_ip=%s knowledge_domain_id=%q err=%v", clientIP, knowledgeDomainIDStr, err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid knowledge_domain_id"})
			return
		}

		// Verify HMAC signature and expiry. Logged at Warn because every 403
		// here is a signal worth investigating: either the URL was tampered
		// with, a client cached an expired URL, or SYSTEM_AES_KEY was
		// rotated without invalidating in-flight links.
		if !secutils.VerifyFileURLSig(filePath, knowledgeDomainID, expiresStr, sig) {
			logger.Warnf(ctx, "[Router] /files/presigned sig invalid or expired: client_ip=%s ua=%q knowledge_domain_id=%d file_path=%q expires=%s",
				clientIP, userAgent, knowledgeDomainID, filePath, expiresStr)
			c.JSON(http.StatusForbidden, gin.H{"error": "invalid or expired signature"})
			return
		}

		provider := types.ParseProviderScheme(filePath)
		knowledgeDomain, err := knowledgeDomainService.GetKnowledgeDomainByID(ctx, knowledgeDomainID)
		if err != nil {
			logger.Warnf(ctx, "[Router] /files/presigned knowledgeDomain lookup failed: client_ip=%s knowledge_domain_id=%d err=%v", clientIP, knowledgeDomainID, err)
			c.Status(http.StatusNotFound)
			return
		}

		fileSvc, resolvedProvider, err := filesvc.NewFileServiceFromStorageConfig(provider, knowledgeDomain.StorageEngineConfig, absDir)
		if err != nil {
			logger.Warnf(ctx, "[Router] /files/presigned resolve file service failed: client_ip=%s knowledge_domain_id=%d provider=%s err=%v",
				clientIP, knowledgeDomainID, provider, err)
			c.Status(http.StatusBadRequest)
			return
		}

		contentType, inline := secutils.SafeContentTypeByFilename(filePath)

		// HEAD short-circuits the body read. We still need to confirm the
		// object exists, but we use a 0-byte content length and skip io.Copy.
		// Skipping GetFile entirely for HEAD would risk reporting 200 for a
		// signed URL that no longer points at a real object; that mismatch
		// would make subsequent GETs from the same client mysteriously fail.
		reader, err := fileSvc.GetFile(ctx, filePath)
		if err != nil {
			logger.Warnf(ctx, "[Router] /files/presigned get file failed: client_ip=%s knowledge_domain_id=%d provider=%s path=%q err=%v",
				clientIP, knowledgeDomainID, resolvedProvider, filePath, err)
			c.Status(http.StatusNotFound)
			return
		}
		defer reader.Close()

		c.Header("Content-Type", contentType)
		c.Header("X-Content-Type-Options", "nosniff")
		if !inline {
			c.Header("Content-Disposition", "attachment")
		}
		c.Header("Cache-Control", "public, max-age=86400")
		if c.Request.Method == http.MethodHead {
			c.Status(http.StatusOK)
			return
		}
		c.Status(http.StatusOK)
		if _, err := io.Copy(c.Writer, reader); err != nil {
			logger.Warnf(ctx, "[Router] /files/presigned write response failed: client_ip=%s knowledge_domain_id=%d err=%v", clientIP, knowledgeDomainID, err)
		}
	}
}

// RegisterDataSourceRoutes wires connector configuration and sync endpoints.
// Data sources hold external-service credentials and trigger jobs that mutate
// knowledge-base content. Reads require authentication; all mutations,
// validation, sync control and credential operations require system
// administration.
func RegisterDataSourceRoutes(
	r *gin.RouterGroup,
	handler *handler.DataSourceHandler,
	credHandler *handler.DataSourceCredentialsHandler,
	g *rbacGuards,
) {
	// Data source routes
	ds := r.Group("/datasource")
	{
		// Connector types are metadata used by configuration forms.
		ds.GET("/types", g.Viewer(), handler.GetAvailableConnectors)

		// Validate credentials without persistence.
		ds.POST("/validate-credentials", g.Admin(), handler.ValidateCredentials)

		// CRUD operations
		ds.POST("", g.Admin(), handler.CreateDataSource)
		ds.GET("", g.Viewer(), handler.ListDataSources)
		ds.GET("/:id", g.Viewer(), handler.GetDataSource)
		ds.PUT("/:id", g.Admin(), handler.UpdateDataSource)
		ds.DELETE("/:id", g.Admin(), handler.DeleteDataSource)

		// Credential subresource. Single logical field "credentials" because
		// connector credentials are a per-connector atomic map (see
		// internal/handler/datasource_credentials.go).
		ds.PUT("/:id/credentials", g.Admin(), credHandler.Put)
		ds.DELETE("/:id/credentials/:field", g.Admin(), credHandler.DeleteField)

		// Connection and resource management.
		ds.POST("/:id/validate", g.Admin(), handler.ValidateConnection)
		ds.GET("/:id/resources", g.Admin(), handler.ListAvailableResources)
		ds.POST("/:id/resource-ancestors", g.Admin(), handler.ResolveResourceAncestors)

		// Sync management.
		// NOTE: /:id/sync is intentionally open (no RBAC and no auth) so the
		// system can trigger it daily at 00:00 without a user session; the
		// middleware auth allow-list also skips JWT for this path.
		ds.POST("/:id/sync", handler.ManualSync)
		ds.POST("/:id/pause", g.Admin(), handler.PauseDataSource)
		ds.POST("/:id/resume", g.Admin(), handler.ResumeDataSource)

		// Sync logs are a read-only audit trail.
		ds.GET("/:id/logs", g.Viewer(), handler.GetSyncLogs)
		ds.GET("/logs/:log_id", g.Viewer(), handler.GetSyncLog)
	}
}
