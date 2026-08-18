package router

import (
	"github.com/gin-gonic/gin"
	"roche.local/knowledge-agent-platform/internal/config"
	"roche.local/knowledge-agent-platform/internal/middleware"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

// rbacGuards centralizes the clean enterprise authorization model: platform
// administration, knowledge-domain administration, and explicit resource
// grants. Creator ownership and active-space roles are intentionally absent.
type rbacGuards struct {
	cfg              *config.Config
	kbService        middleware.KBLookup
	knowledgeService middleware.KnowledgeLookup
	chunkService     middleware.ChunkLookup
	domainAdmins     interfaces.KnowledgeDomainAdminService
}

func newRBACGuards(
	cfg *config.Config,
	kbService interfaces.KnowledgeBaseService,
	knowledgeService interfaces.KnowledgeService,
	chunkService interfaces.ChunkService,
	domainAdmins interfaces.KnowledgeDomainAdminService,
) *rbacGuards {
	return &rbacGuards{
		cfg:              cfg,
		kbService:        kbService,
		knowledgeService: knowledgeService,
		chunkService:     chunkService,
		domainAdmins:     domainAdmins,
	}
}

func (g *rbacGuards) Viewer() gin.HandlerFunc {
	return func(c *gin.Context) { c.Next() }
}

// Admin is retained as a route-level name for platform configuration routes.
// It is deliberately equivalent to SystemAdmin.
func (g *rbacGuards) Admin() gin.HandlerFunc {
	return middleware.RequireSystemAdmin(g.cfg)
}

func (g *rbacGuards) SystemAdmin() gin.HandlerFunc {
	return middleware.RequireSystemAdmin(g.cfg)
}

func (g *rbacGuards) CrossKnowledgeDomain() gin.HandlerFunc {
	return middleware.RequireCrossKnowledgeDomainAccess(g.cfg)
}

func (g *rbacGuards) KnowledgeOfficer() gin.HandlerFunc {
	return middleware.RequireKnowledgeOfficer(g.cfg)
}

// AdminBackend allows access to the administration backend for system
// administrators and knowledge officers (role_knowledge_officer=1).
// Regular viewers are denied.
func (g *rbacGuards) AdminBackend() gin.HandlerFunc {
	return middleware.RequireAdminBackend(g.cfg)
}

func (g *rbacGuards) PathKnowledgeDomainMatch() gin.HandlerFunc {
	return middleware.RequireKnowledgeDomainAdmin("id", g.domainAdmins)
}

func (g *rbacGuards) KBAccessRead(param string) gin.HandlerFunc {
	return middleware.RequireKBAccess(
		middleware.KBIDFromParam(param),
		types.KnowledgeBasePermissionRead,
		g.kbService,
	)
}

func (g *rbacGuards) KBAccessWrite(param string) gin.HandlerFunc {
	return middleware.RequireKBAccess(
		middleware.KBIDFromParam(param),
		types.KnowledgeBasePermissionManage,
		g.kbService,
	)
}

func (g *rbacGuards) KBAccessReadFromKnowledgeIDParam(param string) gin.HandlerFunc {
	return middleware.RequireKBAccess(
		middleware.KBIDFromKnowledgeIDParam(param, g.knowledgeService),
		types.KnowledgeBasePermissionRead,
		g.kbService,
	)
}

func (g *rbacGuards) KBAccessWriteFromKnowledgeIDParam(param string) gin.HandlerFunc {
	return middleware.RequireKBAccess(
		middleware.KBIDFromKnowledgeIDParam(param, g.knowledgeService),
		types.KnowledgeBasePermissionManage,
		g.kbService,
	)
}

func (g *rbacGuards) KBAccessReadFromChunkIDParam(param string) gin.HandlerFunc {
	return middleware.RequireKBAccess(
		middleware.KBIDFromChunkIDParam(param, g.chunkService),
		types.KnowledgeBasePermissionRead,
		g.kbService,
	)
}

func (g *rbacGuards) KBAccessWriteFromChunkIDParam(param string) gin.HandlerFunc {
	return middleware.RequireKBAccess(
		middleware.KBIDFromChunkIDParam(param, g.chunkService),
		types.KnowledgeBasePermissionManage,
		g.kbService,
	)
}
