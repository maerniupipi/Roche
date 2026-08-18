package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"roche.local/knowledge-agent-platform/internal/application/repository"
	"roche.local/knowledge-agent-platform/internal/application/service"
	apperrors "roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

type EnterpriseAccessHandler struct {
	accessService    interfaces.EnterpriseAccessService
	kbService        interfaces.KnowledgeBaseService
	knowledgeService interfaces.KnowledgeService
}

func NewEnterpriseAccessHandler(
	accessService interfaces.EnterpriseAccessService,
	kbService interfaces.KnowledgeBaseService,
	knowledgeService interfaces.KnowledgeService,
) *EnterpriseAccessHandler {
	return &EnterpriseAccessHandler{
		accessService:    accessService,
		kbService:        kbService,
		knowledgeService: knowledgeService,
	}
}

type orgUnitRequest struct {
	ParentID   *string             `json:"parent_id"`
	Code       string              `json:"code" binding:"required"`
	Name       string              `json:"name" binding:"required"`
	Status     types.OrgUnitStatus `json:"status"`
	SortOrder  int                 `json:"sort_order"`
	Attributes types.JSON          `json:"attributes"`
}

type orgMembershipRequest struct {
	OrgUnitID string `json:"org_unit_id" binding:"required"`
	IsPrimary bool   `json:"is_primary"`
}

type replaceOrgMembershipsRequest struct {
	Memberships []orgMembershipRequest `json:"memberships" binding:"required,min=1"`
}

type knowledgeResourceGrantRequest struct {
	SubjectType       types.GrantSubjectType             `json:"subject_type" binding:"required"`
	SubjectID         string                             `json:"subject_id" binding:"required"`
	Permission        types.KnowledgeBasePermissionLevel `json:"permission" binding:"required"`
	Effect            types.GrantEffect                  `json:"effect" binding:"required"`
	InheritToChildren *bool                              `json:"inherit_to_children"`
}

type knowledgeResourceBatchGrantRequest struct {
	SubjectType       types.GrantSubjectType             `json:"subject_type" binding:"required"`
	SubjectID         string                             `json:"subject_id" binding:"required"`
	Permission        types.KnowledgeBasePermissionLevel `json:"permission" binding:"required"`
	Effect            types.GrantEffect                  `json:"effect" binding:"required"`
	InheritToChildren *bool                              `json:"inherit_to_children"`
	KnowledgeBaseIDs  []string                           `json:"knowledge_base_ids" binding:"required,min=1"`
}

// ListResourceGrants godoc
// @Summary      List grants for a knowledge resource
// @Description  Lists direct user and organization allow/deny entries for a knowledge base, folder, or document.
// @Tags         Enterprise Authorization
// @Produce      json
// @Param        id path string true "Knowledge base ID"
// @Param        resource_type path string true "Resource type" Enums(knowledge_base,folder,knowledge)
// @Param        resource_id path string true "Knowledge base, folder, or document ID"
// @Success      200 {object} map[string]interface{}
// @Security     Bearer
// @Router       /knowledge-bases/{id}/resources/{resource_type}/{resource_id}/grants [get]
func (h *EnterpriseAccessHandler) ListResourceGrants(c *gin.Context) {
	kb, err := h.loadKnowledgeBase(c)
	if err != nil {
		h.writeResourceError(c, err)
		return
	}
	resourceType, resourceID, err := knowledgeResourceFromPath(c)
	if err != nil {
		_ = c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	grants, err := h.accessService.ListResourceGrants(
		c.Request.Context(),
		kb,
		resourceType,
		resourceID,
	)
	if err != nil {
		h.writeAccessError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": grants})
}

// ListResourceGrantSubjects godoc
// @Summary      List users and organizations available for a resource grant
// @Description  Returns a safe directory projection after verifying that the caller manages this knowledge resource.
// @Tags         Enterprise Authorization
// @Produce      json
// @Param        id path string true "Knowledge base ID"
// @Param        resource_type path string true "Resource type" Enums(knowledge_base,folder,knowledge)
// @Param        resource_id path string true "Knowledge base, folder, or document ID"
// @Param        search query string false "Optional user search"
// @Param        limit query int false "Maximum users"
// @Success      200 {object} map[string]interface{}
// @Security     Bearer
// @Router       /knowledge-bases/{id}/resources/{resource_type}/{resource_id}/grant-subjects [get]
func (h *EnterpriseAccessHandler) ListResourceGrantSubjects(c *gin.Context) {
	kb, err := h.loadKnowledgeBase(c)
	if err != nil {
		h.writeResourceError(c, err)
		return
	}
	resourceType, resourceID, err := knowledgeResourceFromPath(c)
	if err != nil {
		_ = c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	limit := 100
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil || parsed < 1 || parsed > 500 {
			_ = c.Error(apperrors.NewBadRequestError("limit must be between 1 and 500"))
			return
		}
		limit = parsed
	}
	subjects, err := h.accessService.ListResourceGrantSubjects(
		c.Request.Context(),
		kb,
		resourceType,
		resourceID,
		c.Query("search"),
		limit,
	)
	if err != nil {
		h.writeAccessError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": subjects})
}

// GrantResource godoc
// @Summary      Add or update a knowledge-resource grant
// @Description  Creates an allow/deny read/manage entry. Knowledge-base and folder entries may inherit to descendants.
// @Tags         Enterprise Authorization
// @Accept       json
// @Produce      json
// @Param        id path string true "Knowledge base ID"
// @Param        resource_type path string true "Resource type" Enums(knowledge_base,folder,knowledge)
// @Param        resource_id path string true "Knowledge base, folder, or document ID"
// @Param        request body knowledgeResourceGrantRequest true "Grant rule"
// @Success      200 {object} map[string]interface{}
// @Failure      400 {object} apperrors.AppError
// @Security     Bearer
// @Router       /knowledge-bases/{id}/resources/{resource_type}/{resource_id}/grants [put]
func (h *EnterpriseAccessHandler) GrantResource(c *gin.Context) {
	kb, err := h.loadKnowledgeBase(c)
	if err != nil {
		h.writeResourceError(c, err)
		return
	}
	resourceType, resourceID, err := knowledgeResourceFromPath(c)
	if err != nil {
		_ = c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	var req knowledgeResourceGrantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	inherit := resourceType != types.KnowledgeResourceKnowledge
	if req.InheritToChildren != nil {
		inherit = *req.InheritToChildren
	}
	grant := &types.KnowledgeResourceGrant{
		ResourceType:      resourceType,
		ResourceID:        resourceID,
		SubjectType:       req.SubjectType,
		SubjectID:         req.SubjectID,
		Permission:        req.Permission,
		Effect:            req.Effect,
		InheritToChildren: inherit,
	}
	if err := h.accessService.GrantResource(c.Request.Context(), kb, grant); err != nil {
		h.writeAccessError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": grant})
}

// GrantResourceBatch godoc
// @Summary      Grant a subject access to multiple knowledge bases at once
// @Description  Writes one allow/deny knowledge-base grant for the same subject across the given knowledge bases. Knowledge bases the caller cannot manage (or that do not exist) are skipped; each knowledge base reports its own outcome.
// @Tags         Enterprise Authorization
// @Accept       json
// @Produce      json
// @Param        body body knowledgeResourceBatchGrantRequest true "Batch grant request"
// @Success      200 {object} map[string]interface{}
// @Security     Bearer
// @Router       /enterprise/knowledge-base-grants [post]
func (h *EnterpriseAccessHandler) GrantResourceBatch(c *gin.Context) {
	var req knowledgeResourceBatchGrantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}

	// Deduplicate and drop blank knowledge-base IDs, keeping request order.
	seen := make(map[string]struct{}, len(req.KnowledgeBaseIDs))
	ids := make([]string, 0, len(req.KnowledgeBaseIDs))
	for _, raw := range req.KnowledgeBaseIDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		_ = c.Error(apperrors.NewBadRequestError("knowledge_base_ids must not be empty"))
		return
	}

	kbs, err := h.kbService.GetKnowledgeBasesByIDsOnly(c.Request.Context(), ids)
	if err != nil {
		_ = c.Error(apperrors.NewInternalServerError(err.Error()))
		return
	}
	kbByID := make(map[string]*types.KnowledgeBase, len(kbs))
	for _, kb := range kbs {
		if kb != nil {
			kbByID[kb.ID] = kb
		}
	}

	inherit := true
	if req.InheritToChildren != nil {
		inherit = *req.InheritToChildren
	}
	grant := &types.KnowledgeResourceGrant{
		SubjectType:       req.SubjectType,
		SubjectID:         req.SubjectID,
		Permission:        req.Permission,
		Effect:            req.Effect,
		InheritToChildren: inherit,
	}
	results, err := h.accessService.GrantResourceBatch(c.Request.Context(), kbs, grant)
	if err != nil {
		h.writeAccessError(c, err)
		return
	}

	// Preserve request order and fill in knowledge bases that could not be
	// loaded so callers see every requested ID in the response.
	resultByID := make(map[string]*types.KnowledgeResourceGrantResult, len(results))
	for _, result := range results {
		if result != nil {
			resultByID[result.KnowledgeBaseID] = result
		}
	}
	final := make([]*types.KnowledgeResourceGrantResult, 0, len(ids))
	for _, id := range ids {
		if result, ok := resultByID[id]; ok {
			final = append(final, result)
			continue
		}
		if _, exists := kbByID[id]; !exists {
			final = append(final, &types.KnowledgeResourceGrantResult{
				KnowledgeBaseID: id,
				Reason:          "knowledge base not found",
			})
			continue
		}
		// Loaded but skipped by the service (defensive; should not happen).
		final = append(final, &types.KnowledgeResourceGrantResult{
			KnowledgeBaseID: id,
			Reason:          "skipped",
		})
	}

	// Overall outcome: the batch only succeeds when every requested
	// knowledge base was granted.
	success := len(final) > 0
	message := "success"
	for _, result := range final {
		if result == nil || !result.Granted {
			success = false
			message = "fail"
			break
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": success, "message": message, "data": []any{}})
}

// RevokeResource godoc
// @Summary      Delete a knowledge-resource grant
// @Tags         Enterprise Authorization
// @Produce      json
// @Param        id path string true "Knowledge base ID"
// @Param        grant_id path int true "Grant ID"
// @Success      200 {object} map[string]interface{}
// @Security     Bearer
// @Router       /knowledge-bases/{id}/resource-grants/{grant_id} [delete]
func (h *EnterpriseAccessHandler) RevokeResource(c *gin.Context) {
	kb, err := h.loadKnowledgeBase(c)
	if err != nil {
		h.writeResourceError(c, err)
		return
	}
	grantID, err := strconv.ParseUint(c.Param("grant_id"), 10, 64)
	if err != nil || grantID == 0 {
		_ = c.Error(apperrors.NewBadRequestError("invalid grant id"))
		return
	}
	if err := h.accessService.RevokeResource(c.Request.Context(), kb, grantID); err != nil {
		h.writeAccessError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// DeleteKnowledgeFolder godoc
// @Summary      Recursively delete a logical knowledge folder
// @Description  Requires knowledge-base management permission. Deletes descendant documents and cleans storage, chunks, vector data, graph data, tags, and resource grants.
// @Tags         Enterprise Authorization
// @Produce      json
// @Param        id path string true "Knowledge base ID"
// @Param        folder_id path string true "Folder ID"
// @Success      200 {object} map[string]interface{}
// @Failure      409 {object} apperrors.AppError
// @Security     Bearer
// @Router       /knowledge-bases/{id}/folders/{folder_id} [delete]
func (h *EnterpriseAccessHandler) DeleteKnowledgeFolder(c *gin.Context) {
	kb, err := h.loadKnowledgeBase(c)
	if err != nil {
		h.writeResourceError(c, err)
		return
	}
	folderID := strings.TrimSpace(c.Param("folder_id"))
	if folderID == "" {
		_ = c.Error(apperrors.NewBadRequestError("folder id is required"))
		return
	}
	allowed, err := h.accessService.CanManageKnowledgeBase(c.Request.Context(), kb)
	if err != nil {
		h.writeAccessError(c, err)
		return
	}
	if !allowed {
		h.writeAccessError(c, service.ErrEnterpriseAccessDenied)
		return
	}
	ctx := context.WithValue(
		c.Request.Context(),
		types.KnowledgeDomainIDContextKey,
		kb.KnowledgeDomainID,
	)
	if err := h.knowledgeService.DeleteKnowledgeFolder(ctx, kb.ID, folderID); err != nil {
		switch {
		case errors.Is(err, repository.ErrKnowledgeFolderNotFound):
			_ = c.Error(apperrors.NewNotFoundError(err.Error()))
		case errors.Is(err, repository.ErrKnowledgeFolderNotEmpty):
			_ = c.Error(apperrors.NewConflictError(err.Error()))
		default:
			_ = c.Error(apperrors.NewInternalServerError(err.Error()))
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ListOrgUnits godoc
// @Summary      List enterprise organization units
// @Description  Returns the organization tree used as a bulk authorization subject.
// @Tags         Enterprise Directory
// @Produce      json
// @Success      200 {object} map[string]interface{}
// @Security     Bearer
// @Router       /enterprise/org-units [get]
func (h *EnterpriseAccessHandler) ListOrgUnits(c *gin.Context) {
	units, err := h.accessService.ListOrgUnits(c.Request.Context())
	if err != nil {
		h.writeAccessError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": units})
}

// CreateOrgUnit godoc
// @Summary      Create an enterprise organization unit
// @Tags         Enterprise Directory
// @Accept       json
// @Produce      json
// @Param        request body orgUnitRequest true "Organization unit"
// @Success      201 {object} map[string]interface{}
// @Failure      400 {object} apperrors.AppError
// @Security     Bearer
// @Router       /enterprise/org-units [post]
func (h *EnterpriseAccessHandler) CreateOrgUnit(c *gin.Context) {
	var req orgUnitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	unit := &types.OrgUnit{
		ParentID:   normalizeOptionalString(req.ParentID),
		Code:       req.Code,
		Name:       req.Name,
		Status:     req.Status,
		SortOrder:  req.SortOrder,
		Attributes: req.Attributes,
	}
	if err := h.accessService.CreateOrgUnit(c.Request.Context(), unit); err != nil {
		h.writeAccessError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": unit})
}

// UpdateOrgUnit godoc
// @Summary      Update an enterprise organization unit
// @Tags         Enterprise Directory
// @Accept       json
// @Produce      json
// @Param        org_unit_id path string true "Organization unit ID"
// @Param        request body orgUnitRequest true "Organization unit"
// @Success      200 {object} map[string]interface{}
// @Failure      400 {object} apperrors.AppError
// @Security     Bearer
// @Router       /enterprise/org-units/{org_unit_id} [put]
func (h *EnterpriseAccessHandler) UpdateOrgUnit(c *gin.Context) {
	var req orgUnitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	unit := &types.OrgUnit{
		ID:         c.Param("org_unit_id"),
		ParentID:   normalizeOptionalString(req.ParentID),
		Code:       req.Code,
		Name:       req.Name,
		Status:     req.Status,
		SortOrder:  req.SortOrder,
		Attributes: req.Attributes,
	}
	if err := h.accessService.UpdateOrgUnit(c.Request.Context(), unit); err != nil {
		h.writeAccessError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// DeleteOrgUnit godoc
// @Summary      Delete an enterprise organization unit
// @Tags         Enterprise Directory
// @Produce      json
// @Param        org_unit_id path string true "Organization unit ID"
// @Success      200 {object} map[string]interface{}
// @Security     Bearer
// @Router       /enterprise/org-units/{org_unit_id} [delete]
func (h *EnterpriseAccessHandler) DeleteOrgUnit(c *gin.Context) {
	if err := h.accessService.DeleteOrgUnit(
		c.Request.Context(),
		c.Param("org_unit_id"),
	); err != nil {
		h.writeAccessError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ListOrgUnitMembers godoc
// @Summary      List members of an enterprise organization unit
// @Tags         Enterprise Directory
// @Produce      json
// @Param        org_unit_id path string true "Organization unit ID"
// @Success      200 {object} map[string]interface{}
// @Security     Bearer
// @Router       /enterprise/org-units/{org_unit_id}/members [get]
func (h *EnterpriseAccessHandler) ListOrgUnitMembers(c *gin.Context) {
	members, err := h.accessService.ListOrgUnitMembers(
		c.Request.Context(),
		c.Param("org_unit_id"),
	)
	if err != nil {
		h.writeAccessError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": members})
}

// ReplaceUserOrgMemberships godoc
// @Summary      Replace a user's manual organization memberships
// @Description  Replaces manual memberships as one set. Workday-managed memberships cannot be overwritten here.
// @Tags         Enterprise Directory
// @Accept       json
// @Produce      json
// @Param        user_id path string true "User ID"
// @Param        request body replaceOrgMembershipsRequest true "Organization memberships"
// @Success      200 {object} map[string]interface{}
// @Failure      409 {object} apperrors.AppError
// @Security     Bearer
// @Router       /enterprise/users/{user_id}/org-memberships [put]
func (h *EnterpriseAccessHandler) ReplaceUserOrgMemberships(c *gin.Context) {
	var req replaceOrgMembershipsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	memberships := make([]*types.UserOrgMembership, 0, len(req.Memberships))
	for _, item := range req.Memberships {
		memberships = append(memberships, &types.UserOrgMembership{
			OrgUnitID: item.OrgUnitID,
			IsPrimary: item.IsPrimary,
		})
	}
	if err := h.accessService.ReplaceUserOrgMemberships(
		c.Request.Context(),
		c.Param("user_id"),
		memberships,
	); err != nil {
		h.writeAccessError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ListUserOrgMemberships godoc
// @Summary      List a user's organization memberships
// @Tags         Enterprise Directory
// @Produce      json
// @Param        user_id path string true "User ID"
// @Success      200 {object} map[string]interface{}
// @Security     Bearer
// @Router       /enterprise/users/{user_id}/org-memberships [get]
func (h *EnterpriseAccessHandler) ListUserOrgMemberships(c *gin.Context) {
	memberships, err := h.accessService.ListUserOrgMemberships(
		c.Request.Context(),
		c.Param("user_id"),
	)
	if err != nil {
		h.writeAccessError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": memberships})
}

// SearchGrantUsers godoc
// @Summary      Search users available as grant subjects
// @Tags         Enterprise Directory
// @Produce      json
// @Param        search query string false "Email or display-name search"
// @Param        limit query int false "Maximum records" default(50)
// @Success      200 {object} map[string]interface{}
// @Security     Bearer
// @Router       /enterprise/users [get]
func (h *EnterpriseAccessHandler) SearchGrantUsers(c *gin.Context) {
	limit := 50
	if rawLimit := c.Query("limit"); rawLimit != "" {
		if parsed, err := strconv.Atoi(rawLimit); err == nil {
			limit = parsed
		}
	}
	users, err := h.accessService.SearchGrantUsers(
		c.Request.Context(),
		c.Query("search"),
		limit,
	)
	if err != nil {
		h.writeAccessError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": users})
}

func (h *EnterpriseAccessHandler) loadKnowledgeBase(c *gin.Context) (*types.KnowledgeBase, error) {
	return h.kbService.GetKnowledgeBaseByID(c.Request.Context(), c.Param("id"))
}

func knowledgeResourceFromPath(
	c *gin.Context,
) (types.KnowledgeResourceType, string, error) {
	resourceType := types.KnowledgeResourceType(
		strings.ToLower(strings.TrimSpace(c.Param("resource_type"))),
	)
	resourceID := strings.TrimSpace(c.Param("resource_id"))
	if !resourceType.IsValid() || resourceID == "" {
		return "", "", service.ErrKnowledgeResourceMissing
	}
	return resourceType, resourceID, nil
}

func normalizeOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func (h *EnterpriseAccessHandler) writeResourceError(c *gin.Context, err error) {
	if errors.Is(err, repository.ErrKnowledgeBaseNotFound) ||
		errors.Is(err, repository.ErrKnowledgeNotFound) ||
		errors.Is(err, gorm.ErrRecordNotFound) {
		_ = c.Error(apperrors.NewNotFoundError("resource not found"))
		return
	}
	_ = c.Error(apperrors.NewInternalServerError(err.Error()))
}

func (h *EnterpriseAccessHandler) writeAccessError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrEnterpriseAccessDenied):
		_ = c.Error(apperrors.NewForbiddenError("permission denied"))
	case errors.Is(err, service.ErrOrgUnitNotFound):
		_ = c.Error(apperrors.NewNotFoundError(err.Error()))
	case errors.Is(err, service.ErrKnowledgeResourceMissing):
		_ = c.Error(apperrors.NewNotFoundError(err.Error()))
	case errors.Is(err, service.ErrOrgUnitNotEmpty),
		errors.Is(err, service.ErrOrgUnitCycle),
		errors.Is(err, service.ErrProtectedOrgUnit),
		errors.Is(err, service.ErrWorkdayMembershipManaged):
		_ = c.Error(apperrors.NewConflictError(err.Error()))
	case errors.Is(err, service.ErrEnterpriseMemberMissing),
		errors.Is(err, service.ErrInvalidPermission),
		errors.Is(err, service.ErrInvalidGrantSubject):
		_ = c.Error(apperrors.NewBadRequestError(err.Error()))
	default:
		_ = c.Error(apperrors.NewInternalServerError(err.Error()))
	}
}

// -- Knowledge-base-officer handler methods --

func (h *EnterpriseAccessHandler) ListKnowledgeBaseOfficers(c *gin.Context) {
	kbID := c.Param("id")
	officers, err := h.accessService.ListKnowledgeBaseOfficers(c.Request.Context(), kbID)
	if err != nil {
		h.writeAccessError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": officers})
}

func (h *EnterpriseAccessHandler) AddKnowledgeBaseOfficer(c *gin.Context) {
	kbID := c.Param("id")
	var req struct {
		UserID string `json:"user_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	if err := h.accessService.AddKnowledgeBaseOfficer(c.Request.Context(), kbID, req.UserID); err != nil {
		h.writeAccessError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *EnterpriseAccessHandler) RemoveKnowledgeBaseOfficer(c *gin.Context) {
	kbID := c.Param("id")
	userID := c.Param("user_id")
	if err := h.accessService.RemoveKnowledgeBaseOfficer(c.Request.Context(), kbID, userID); err != nil {
		h.writeAccessError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
