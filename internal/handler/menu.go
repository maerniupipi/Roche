package handler

import (
	"net/http"
	"sort"
	"strconv"

	"github.com/gin-gonic/gin"
	apperrors "roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

// menuNode is one node of the navigation tree returned by GET /api/v1/menu.
type menuNode struct {
	ID       string          `json:"id"`
	ParentID *string         `json:"parentId"`
	Title    string          `json:"title"`
	TitleEn  string          `json:"titleEn"`
	Icon     string          `json:"icon,omitempty"`
	Path     string          `json:"path"`
	Order    int             `json:"order"`
	Visible  bool            `json:"visible"`
	Meta     map[string]bool `json:"meta"`
	Children []*menuNode     `json:"children"`
}

// MenuHandler serves the navigation tree scoped to the current user's
// manageable knowledge bases.
type MenuHandler struct {
	accessService interfaces.EnterpriseAccessService
	kbService     interfaces.KnowledgeBaseService
	domainService interfaces.KnowledgeDomainService
}

// NewMenuHandler creates a new menu handler.
func NewMenuHandler(
	accessService interfaces.EnterpriseAccessService,
	kbService interfaces.KnowledgeBaseService,
	domainService interfaces.KnowledgeDomainService,
) *MenuHandler {
	return &MenuHandler{
		accessService: accessService,
		kbService:     kbService,
		domainService: domainService,
	}
}

// GetMenu godoc
// @Summary      获取当前用户菜单
// @Description  返回登录用户权限范围内可管理的知识库导航树（按知识域分组）
// @Tags         Menu
// @Produce      json
// @Success      200 {object} map[string]interface{}
// @Security     Bearer
// @Router       /menu [get]
func (h *MenuHandler) GetMenu(c *gin.Context) {
	ctx := c.Request.Context()

	tree := []*menuNode{dashboardMenuNode()}

	// System admins see the full knowledge-domain tree, including empty
	// domains that have no knowledge bases yet. This lets them navigate
	// to and start populating a freshly created domain before any KB
	// exists. Non-admin users still go through the per-KB access filter
	// below so they only see domains hosting at least one KB they can
	// manage.
	if types.IsSystemAdminFromContext(ctx) {
		allDomains, err := h.domainService.ListKnowledgeDomains(ctx)
		if err != nil {
			_ = c.Error(apperrors.NewInternalServerError(err.Error()))
			return
		}

		knowledgeNode := &menuNode{
			ID:       "knowledge",
			Title:    "知识库",
			TitleEn:  "Knowledge",
			Icon:     "knowledge",
			Path:     "/platform/knowledge-bases",
			Order:    3,
			Visible:  true,
			Meta:     map[string]bool{"requiresKnowledgeManagement": true},
			Children: []*menuNode{},
		}
		// Sort by domain ID for a stable order, then build one child per domain.
		sort.Slice(allDomains, func(i, j int) bool { return allDomains[i].ID < allDomains[j].ID })
		for i, domain := range allDomains {
			knowledgeNode.Children = append(knowledgeNode.Children, domainMenuNode(domain, i+1))
		}
		tree = append(tree, knowledgeNode)
	} else {
		// Non-admin: only show domains that host at least one KB they can manage.
		// Empty domains are hidden. Returns nil on error (already reported via c.Error).
		if sub := h.buildKnowledgeSubtreeForNonAdmin(c); sub != nil {
			tree = append(tree, sub)
		}
	}

	// 4. System admins see the full management surface (recommend questions,
	// answer records, role config, exchange rate). These are gated to admin
	// only — non-admin knowledge managers do not get them.
	if types.IsSystemAdminFromContext(ctx) {
		tree = append(tree,
			&menuNode{
				ID:       "recommend-questions",
				Title:    "推荐问题",
				TitleEn:  "Recommend Questions",
				Icon:     "recommend",
				Path:     "/platform/recommend-questions",
				Order:    4,
				Visible:  true,
				Meta:     map[string]bool{},
				Children: []*menuNode{},
			},
			&menuNode{
				ID:       "answer-records",
				Title:    "用户问答记录",
				TitleEn:  "Answer Records",
				Icon:     "records",
				Path:     "/platform/answer-records",
				Order:    5,
				Visible:  true,
				Meta:     map[string]bool{},
				Children: []*menuNode{},
			},
			&menuNode{
				ID:       "roles",
				Title:    "用户角色配置",
				TitleEn:  "Roles",
				Icon:     "roles",
				Path:     "/platform/roles",
				Order:    6,
				Visible:  true,
				Meta:     map[string]bool{},
				Children: []*menuNode{},
			},
			&menuNode{
				ID:       "exchange-rate",
				Title:    "汇率换算",
				TitleEn:  "Exchange Rate",
				Icon:     "rate",
				Path:     "/platform/exchange-rate",
				Order:    7,
				Visible:  true,
				Meta:     map[string]bool{},
				Children: []*menuNode{},
			},
		)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"tree": tree}, "error": nil})
}

// dashboardMenuNode is the fixed dashboard entry shown to every user.
func dashboardMenuNode() *menuNode {
	return &menuNode{
		ID:       "dashboard",
		Title:    "仪表盘",
		TitleEn:  "Dashboard",
		Icon:     "dashboard",
		Path:     "/platform/dashboard",
		Order:    1,
		Visible:  true,
		Meta:     map[string]bool{},
		Children: []*menuNode{},
	}
}

// domainMenuNode maps one knowledge domain owning at least one manageable
// knowledge base to a knowledge subtree child.
func domainMenuNode(domain *types.KnowledgeDomain, order int) *menuNode {
	id := "kb-" + domain.Code
	if id == "kb-" {
		id = "domain-" + strconv.FormatUint(domain.ID, 10)
	}
	parent := "knowledge"
	titleEn := domain.NameEn
	if titleEn == "" {
		titleEn = domain.Name
	}
	return &menuNode{
		ID:       id,
		ParentID: &parent,
		Title:    domain.Name,
		TitleEn:  titleEn,
		Path:     "/platform/knowledge-bases/domain/" + strconv.FormatUint(domain.ID, 10),
		Order:    order,
		Visible:  true,
		Meta:     map[string]bool{},
		Children: []*menuNode{},
	}
}

// buildKnowledgeSubtreeForNonAdmin builds the knowledge subtree for non-admin
// users. Unlike the admin path, this only includes domains hosting at least
// one knowledge base the caller can manage — empty domains are hidden, and
// domains whose KBs the caller has no manage permission on are also hidden.
// Returns nil (and reports the error via c.Error) on failure, or when the
// caller has no manageable KBs at all (caller should skip appending nil).
func (h *MenuHandler) buildKnowledgeSubtreeForNonAdmin(c *gin.Context) *menuNode {
	ctx := c.Request.Context()
	allIDs, err := h.accessService.ListAllKnowledgeBaseIDs(ctx)
	if err != nil {
		_ = c.Error(apperrors.NewInternalServerError(err.Error()))
		return nil
	}
	kbs, err := h.kbService.GetKnowledgeBasesByIDsOnly(ctx, allIDs)
	if err != nil {
		_ = c.Error(apperrors.NewInternalServerError(err.Error()))
		return nil
	}

	manageable := make([]*types.KnowledgeBase, 0, len(kbs))
	domainIDs := make(map[uint64]struct{}, len(kbs))
	for _, kb := range kbs {
		canManage, err := h.accessService.CanManageKnowledgeBase(ctx, kb)
		if err != nil {
			_ = c.Error(apperrors.NewInternalServerError(err.Error()))
			return nil
		}
		if !canManage {
			continue
		}
		manageable = append(manageable, kb)
		domainIDs[kb.KnowledgeDomainID] = struct{}{}
	}

	if len(manageable) == 0 {
		return nil
	}

	ids := make([]uint64, 0, len(domainIDs))
	for id := range domainIDs {
		ids = append(ids, id)
	}
	domains, err := h.domainService.GetKnowledgeDomainsByIDs(ctx, ids)
	if err != nil {
		_ = c.Error(apperrors.NewInternalServerError(err.Error()))
		return nil
	}

	knowledgeNode := &menuNode{
		ID:       "knowledge",
		Title:    "知识库",
		TitleEn:  "Knowledge",
		Icon:     "knowledge",
		Path:     "/platform/knowledge-bases",
		Order:    3,
		Visible:  true,
		Meta:     map[string]bool{"requiresKnowledgeManagement": true},
		Children: []*menuNode{},
	}
	sorted := make([]uint64, 0, len(domains))
	for id := range domains {
		sorted = append(sorted, id)
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	for i, id := range sorted {
		domain := domains[id]
		if domain == nil {
			continue
		}
		knowledgeNode.Children = append(knowledgeNode.Children, domainMenuNode(domain, i+1))
	}
	return knowledgeNode
}
