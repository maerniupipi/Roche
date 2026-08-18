package service

import (
	"context"
	"encoding/json"
	"fmt"

	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

// BusinessAuditRecorder provides domain-specific audit recording methods
// for business operations. It wraps AuditLogService and ensures
// consistent details formatting across services. Audit failures are
// logged but never propagated — audit must not break business operations.
type BusinessAuditRecorder struct {
	svc interfaces.AuditLogService
}

// NewBusinessAuditRecorder creates a recorder. Pass nil to create a
// no-op recorder (all methods are safe to call on nil receiver).
func NewBusinessAuditRecorder(svc interfaces.AuditLogService) *BusinessAuditRecorder {
	return &BusinessAuditRecorder{svc: svc}
}

// log is the internal helper that writes an audit row and swallows errors.
// ActorName defaults to the ctx user's username so every business audit
// row records 操作人 name without each call site repeating the lookup.
func (r *BusinessAuditRecorder) log(ctx context.Context, entry *types.AuditLog) {
	if r == nil || r.svc == nil {
		return
	}
	if entry != nil && entry.ActorName == "" {
		entry.ActorName = auditActorName(ctx)
	}
	// 操作人发起请求的 IP：调用方未显式提供（RecordLogin 等已传参）时
	// 从 ctx 兜底（RequestID middleware 注入），所有业务审计事件统一带上。
	if entry != nil && entry.ClientIP == "" {
		entry.ClientIP = types.ClientIPFromContext(ctx)
	}
	_ = r.svc.Log(ctx, entry)
}

// fmtID converts a uint64 to its string representation.
func fmtID(v uint64) string { return fmt.Sprintf("%d", v) }

// truncateStr truncates a string to maxLen with a suffix marker.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...[truncated]"
}

// ------------------------------------------------------------------
// 知识文档 (knowledge.*)
// ------------------------------------------------------------------

// RecordKnowledgeCreated records a knowledge document creation event.
func (r *BusinessAuditRecorder) RecordKnowledgeCreated(
	ctx context.Context,
	knowledgeID, title, fileName, fileType string,
	fileSize int64, knowledgeBaseID, knowledgeBaseName string,
) {
	detailMap := map[string]interface{}{
		"knowledge_id":        knowledgeID,
		"title":               title,
		"knowledge_base_id":   knowledgeBaseID,
		"knowledge_base_name": knowledgeBaseName,
	}
	if fileName != "" {
		detailMap["file_name"] = fileName
		detailMap["file_type"] = fileType
		detailMap["file_size"] = fileSize
	}
	detailBytes, _ := json.Marshal(detailMap)
	r.log(ctx, &types.AuditLog{
		ActorUserID: auditActor(ctx),
		Action:      types.AuditActionKnowledgeCreated,
		TargetType:  "knowledge",
		TargetID:    knowledgeID,
		Outcome:     types.AuditOutcomeSuccess,
		Details:     types.JSON(detailBytes),
	})
}

// RecordKnowledgeUpdated records a knowledge document modification event.
func (r *BusinessAuditRecorder) RecordKnowledgeUpdated(
	ctx context.Context,
	knowledgeID, title string,
	knowledgeBaseID string,
	changedFields []string,
	oldValues, newValues map[string]interface{},
) {
	detailMap := map[string]interface{}{
		"knowledge_id":      knowledgeID,
		"title":             title,
		"knowledge_base_id": knowledgeBaseID,
		"changed_fields":    changedFields,
	}
	if len(oldValues) > 0 {
		detailMap["old_values"] = sanitizeDetails(oldValues)
	}
	if len(newValues) > 0 {
		detailMap["new_values"] = sanitizeDetails(newValues)
	}
	detailBytes, _ := json.Marshal(detailMap)
	r.log(ctx, &types.AuditLog{
		ActorUserID: auditActor(ctx),
		Action:      types.AuditActionKnowledgeUpdated,
		TargetType:  "knowledge",
		TargetID:    knowledgeID,
		Outcome:     types.AuditOutcomeSuccess,
		Details:     types.JSON(detailBytes),
	})
}

// RecordKnowledgeDeleted records a knowledge document deletion event.
func (r *BusinessAuditRecorder) RecordKnowledgeDeleted(
	ctx context.Context,
	knowledgeID, title string,
	knowledgeBaseID string,
	cascade bool,
) {
	detailMap := map[string]interface{}{
		"knowledge_id":      knowledgeID,
		"title":             title,
		"knowledge_base_id": knowledgeBaseID,
		"cascade":           cascade,
	}
	detailBytes, _ := json.Marshal(detailMap)
	r.log(ctx, &types.AuditLog{
		ActorUserID: auditActor(ctx),
		Action:      types.AuditActionKnowledgeDeleted,
		TargetType:  "knowledge",
		TargetID:    knowledgeID,
		Outcome:     types.AuditOutcomeSuccess,
		Details:     types.JSON(detailBytes),
	})
}

// RecordKnowledgePublished records a knowledge document publish/enable event.
func (r *BusinessAuditRecorder) RecordKnowledgePublished(
	ctx context.Context,
	knowledgeID, title string,
	knowledgeBaseID string,
) {
	detailMap := map[string]interface{}{
		"knowledge_id":      knowledgeID,
		"title":             title,
		"knowledge_base_id": knowledgeBaseID,
	}
	detailBytes, _ := json.Marshal(detailMap)
	r.log(ctx, &types.AuditLog{
		ActorUserID: auditActor(ctx),
		Action:      types.AuditActionKnowledgePublished,
		TargetType:  "knowledge",
		TargetID:    knowledgeID,
		Outcome:     types.AuditOutcomeSuccess,
		Details:     types.JSON(detailBytes),
	})
}

// RecordKnowledgeUnpublished records a knowledge document unpublish/disable event.
func (r *BusinessAuditRecorder) RecordKnowledgeUnpublished(
	ctx context.Context,
	knowledgeID, title string,
	knowledgeBaseID string,
) {
	detailMap := map[string]interface{}{
		"knowledge_id":      knowledgeID,
		"title":             title,
		"knowledge_base_id": knowledgeBaseID,
	}
	detailBytes, _ := json.Marshal(detailMap)
	r.log(ctx, &types.AuditLog{
		ActorUserID: auditActor(ctx),
		Action:      types.AuditActionKnowledgeUnpublished,
		TargetType:  "knowledge",
		TargetID:    knowledgeID,
		Outcome:     types.AuditOutcomeSuccess,
		Details:     types.JSON(detailBytes),
	})
}

// ------------------------------------------------------------------
// 知识库 (knowledgebase.*)
// ------------------------------------------------------------------

// RecordKnowledgeBaseCreated records a knowledge base creation event.
func (r *BusinessAuditRecorder) RecordKnowledgeBaseCreated(
	ctx context.Context,
	kbID uint64, kbName string,
	vectorStoreID uint64, embeddingModel string,
) {
	detailMap := map[string]interface{}{
		"knowledge_base_id": kbID,
		"name":              kbName,
		"vector_store_id":   vectorStoreID,
		"embedding_model":   embeddingModel,
	}
	detailBytes, _ := json.Marshal(detailMap)
	r.log(ctx, &types.AuditLog{
		ActorUserID: auditActor(ctx),
		Action:      types.AuditActionKnowledgeBaseCreated,
		TargetType:  "knowledge_base",
		TargetID:    fmtID(kbID),
		Outcome:     types.AuditOutcomeSuccess,
		Details:     types.JSON(detailBytes),
	})
}

// RecordKnowledgeBaseUpdated records a knowledge base modification event.
func (r *BusinessAuditRecorder) RecordKnowledgeBaseUpdated(
	ctx context.Context,
	kbID uint64, kbName string,
	changedFields []string,
) {
	detailMap := map[string]interface{}{
		"knowledge_base_id": kbID,
		"name":              kbName,
		"changed_fields":    changedFields,
	}
	detailBytes, _ := json.Marshal(detailMap)
	r.log(ctx, &types.AuditLog{
		ActorUserID: auditActor(ctx),
		Action:      types.AuditActionKnowledgeBaseUpdated,
		TargetType:  "knowledge_base",
		TargetID:    fmtID(kbID),
		Outcome:     types.AuditOutcomeSuccess,
		Details:     types.JSON(detailBytes),
	})
}

// RecordKnowledgeBaseDeleted records a knowledge base deletion event.
func (r *BusinessAuditRecorder) RecordKnowledgeBaseDeleted(
	ctx context.Context,
	kbID uint64, kbName string,
	cascadeDocuments bool,
) {
	detailMap := map[string]interface{}{
		"knowledge_base_id": kbID,
		"name":              kbName,
		"cascade_documents": cascadeDocuments,
	}
	detailBytes, _ := json.Marshal(detailMap)
	r.log(ctx, &types.AuditLog{
		ActorUserID: auditActor(ctx),
		Action:      types.AuditActionKnowledgeBaseDeleted,
		TargetType:  "knowledge_base",
		TargetID:    fmtID(kbID),
		Outcome:     types.AuditOutcomeSuccess,
		Details:     types.JSON(detailBytes),
	})
}

// ------------------------------------------------------------------
// 知识域管理员 (domain.*)
// ------------------------------------------------------------------

// RecordDomainAdminGranted records a domain admin grant event.
func (r *BusinessAuditRecorder) RecordDomainAdminGranted(
	ctx context.Context,
	knowledgeDomainID uint64, domainName string,
	targetUserID, targetEmail, targetUsername string,
	idempotent bool,
) {
	detailMap := map[string]interface{}{
		"knowledge_domain_id":   knowledgeDomainID,
		"knowledge_domain_name": domainName,
		"target_user_id":        targetUserID,
		"target_email":          targetEmail,
		"target_username":       targetUsername,
		"idempotent":            idempotent,
	}
	detailBytes, _ := json.Marshal(detailMap)
	r.log(ctx, &types.AuditLog{
		ActorUserID:  auditActor(ctx),
		Action:       types.AuditActionDomainAdminGranted,
		TargetType:   "user",
		TargetID:     targetUserID,
		TargetUserID: targetUserID,
		Outcome:      types.AuditOutcomeSuccess,
		Details:      types.JSON(detailBytes),
	})
}

// RecordDomainAdminRevoked records a domain admin revoke event.
func (r *BusinessAuditRecorder) RecordDomainAdminRevoked(
	ctx context.Context,
	knowledgeDomainID uint64, domainName string,
	targetUserID, targetEmail, targetUsername string,
	changed bool,
) {
	detailMap := map[string]interface{}{
		"knowledge_domain_id":   knowledgeDomainID,
		"knowledge_domain_name": domainName,
		"target_user_id":        targetUserID,
		"target_email":          targetEmail,
		"target_username":       targetUsername,
		"changed":               changed,
	}
	detailBytes, _ := json.Marshal(detailMap)
	r.log(ctx, &types.AuditLog{
		ActorUserID:  auditActor(ctx),
		Action:       types.AuditActionDomainAdminRevoked,
		TargetType:   "user",
		TargetID:     targetUserID,
		TargetUserID: targetUserID,
		Outcome:      types.AuditOutcomeSuccess,
		Details:      types.JSON(detailBytes),
	})
}

// ------------------------------------------------------------------
// 权限 (permission.*)
// ------------------------------------------------------------------

// RecordPermissionGranted records a permission grant event.
func (r *BusinessAuditRecorder) RecordPermissionGranted(
	ctx context.Context,
	resourceType, resourceID, resourceName string,
	granteeUserID, granteeEmail string,
	permissionLevel string,
) {
	detailMap := map[string]interface{}{
		"resource_type":    resourceType,
		"resource_id":      resourceID,
		"resource_name":    resourceName,
		"grantee_user_id":  granteeUserID,
		"grantee_email":    granteeEmail,
		"permission_level": permissionLevel,
	}
	detailBytes, _ := json.Marshal(detailMap)
	r.log(ctx, &types.AuditLog{
		ActorUserID:  auditActor(ctx),
		Action:       types.AuditActionPermissionGranted,
		TargetType:   resourceType,
		TargetID:     resourceID,
		TargetUserID: granteeUserID,
		Outcome:      types.AuditOutcomeSuccess,
		Details:      types.JSON(detailBytes),
	})
}

// RecordPermissionRevoked records a permission revoke event.
func (r *BusinessAuditRecorder) RecordPermissionRevoked(
	ctx context.Context,
	resourceType, resourceID, resourceName string,
	revokeeUserID, revokeeEmail string,
	permissionLevel string,
) {
	detailMap := map[string]interface{}{
		"resource_type":    resourceType,
		"resource_id":      resourceID,
		"resource_name":    resourceName,
		"revokee_user_id":  revokeeUserID,
		"revokee_email":    revokeeEmail,
		"permission_level": permissionLevel,
	}
	detailBytes, _ := json.Marshal(detailMap)
	r.log(ctx, &types.AuditLog{
		ActorUserID:  auditActor(ctx),
		Action:       types.AuditActionPermissionRevoked,
		TargetType:   resourceType,
		TargetID:     resourceID,
		TargetUserID: revokeeUserID,
		Outcome:      types.AuditOutcomeSuccess,
		Details:      types.JSON(detailBytes),
	})
}

// ------------------------------------------------------------------
// 用户管理 (user.*)
// ------------------------------------------------------------------

// RecordUserCreated records a user registration event.
func (r *BusinessAuditRecorder) RecordUserCreated(
	ctx context.Context,
	userID, email, username, provider string,
) {
	detailMap := map[string]interface{}{
		"user_id":  userID,
		"email":    email,
		"username": username,
		"provider": provider,
	}
	detailBytes, _ := json.Marshal(detailMap)
	r.log(ctx, &types.AuditLog{
		ActorUserID:  auditActor(ctx),
		Action:       types.AuditActionUserCreated,
		TargetType:   "user",
		TargetID:     userID,
		TargetUserID: userID,
		Outcome:      types.AuditOutcomeSuccess,
		Details:      types.JSON(detailBytes),
	})
}

// RecordUserPasswordChanged records a password change event.
func (r *BusinessAuditRecorder) RecordUserPasswordChanged(
	ctx context.Context,
	userID string,
) {
	detailMap := map[string]interface{}{
		"user_id": userID,
	}
	detailBytes, _ := json.Marshal(detailMap)
	r.log(ctx, &types.AuditLog{
		ActorUserID:  auditActor(ctx),
		Action:       types.AuditActionUserPasswordChanged,
		TargetType:   "user",
		TargetID:     userID,
		TargetUserID: userID,
		Outcome:      types.AuditOutcomeSuccess,
		Details:      types.JSON(detailBytes),
	})
}

// RecordUserBanned records a user ban event. `actorID` is the
// administrator who performed the ban; it is preferred over the ctx
// actor when non-empty (some admin flows pass the operator explicitly).
func (r *BusinessAuditRecorder) RecordUserBanned(
	ctx context.Context,
	actorID, targetUserID, targetEmail, targetUsername, reason string,
) {
	actor := actorID
	if actor == "" {
		actor = auditActor(ctx)
	}
	detailMap := map[string]interface{}{
		"target_user_id":  targetUserID,
		"target_email":    targetEmail,
		"target_username": targetUsername,
	}
	if reason != "" {
		detailMap["reason"] = reason
	}
	detailBytes, _ := json.Marshal(detailMap)
	r.log(ctx, &types.AuditLog{
		ActorUserID:  actor,
		Action:       types.AuditActionUserBanned,
		TargetType:   "user",
		TargetID:     targetUserID,
		TargetUserID: targetUserID,
		Outcome:      types.AuditOutcomeSuccess,
		Details:      types.JSON(detailBytes),
	})
}

// RecordUserUnbanned records a user unban event.
func (r *BusinessAuditRecorder) RecordUserUnbanned(
	ctx context.Context,
	actorID, targetUserID, targetEmail, targetUsername string,
) {
	actor := actorID
	if actor == "" {
		actor = auditActor(ctx)
	}
	detailMap := map[string]interface{}{
		"target_user_id":  targetUserID,
		"target_email":    targetEmail,
		"target_username": targetUsername,
	}
	detailBytes, _ := json.Marshal(detailMap)
	r.log(ctx, &types.AuditLog{
		ActorUserID:  actor,
		Action:       types.AuditActionUserUnbanned,
		TargetType:   "user",
		TargetID:     targetUserID,
		TargetUserID: targetUserID,
		Outcome:      types.AuditOutcomeSuccess,
		Details:      types.JSON(detailBytes),
	})
}

// RecordUserOfflined records a force-offline (session revoke) event.
func (r *BusinessAuditRecorder) RecordUserOfflined(
	ctx context.Context,
	actorID, targetUserID, targetEmail, targetUsername string,
) {
	actor := actorID
	if actor == "" {
		actor = auditActor(ctx)
	}
	detailMap := map[string]interface{}{
		"target_user_id":  targetUserID,
		"target_email":    targetEmail,
		"target_username": targetUsername,
	}
	detailBytes, _ := json.Marshal(detailMap)
	r.log(ctx, &types.AuditLog{
		ActorUserID:  actor,
		Action:       types.AuditActionUserOfflined,
		TargetType:   "user",
		TargetID:     targetUserID,
		TargetUserID: targetUserID,
		Outcome:      types.AuditOutcomeSuccess,
		Details:      types.JSON(detailBytes),
	})
}

// RecordUserDeleted records a user account deletion event.
func (r *BusinessAuditRecorder) RecordUserDeleted(
	ctx context.Context,
	actorID, targetUserID, targetEmail, targetUsername string,
) {
	actor := actorID
	if actor == "" {
		actor = auditActor(ctx)
	}
	detailMap := map[string]interface{}{
		"user_id":         targetUserID,
		"email":           targetEmail,
		"username":        targetUsername,
		"target_user_id":  targetUserID,
		"target_email":    targetEmail,
		"target_username": targetUsername,
	}
	detailBytes, _ := json.Marshal(detailMap)
	r.log(ctx, &types.AuditLog{
		ActorUserID:  actor,
		Action:       types.AuditActionUserDeleted,
		TargetType:   "user",
		TargetID:     targetUserID,
		TargetUserID: targetUserID,
		Outcome:      types.AuditOutcomeSuccess,
		Details:      types.JSON(detailBytes),
	})
}

// RecordUserRolesUpdated records a role_knowledge_officer batch/single
// update event.
func (r *BusinessAuditRecorder) RecordUserRolesUpdated(
	ctx context.Context,
	actorID string,
	affectedCount int,
	rolesChanged map[string]interface{},
) {
	actor := actorID
	if actor == "" {
		actor = auditActor(ctx)
	}
	detailMap := map[string]interface{}{
		"affected_count": affectedCount,
	}
	if len(rolesChanged) > 0 {
		detailMap["roles_changed"] = sanitizeDetails(rolesChanged)
	}
	detailBytes, _ := json.Marshal(detailMap)
	r.log(ctx, &types.AuditLog{
		ActorUserID:  actor,
		Action:       types.AuditActionUserRolesUpdated,
		Outcome:      types.AuditOutcomeSuccess,
		Details:      types.JSON(detailBytes),
	})
}

// RecordSystemAdminPromoted records a system-admin privilege grant.
func (r *BusinessAuditRecorder) RecordSystemAdminPromoted(
	ctx context.Context,
	actorID, targetUserID, targetEmail, targetUsername string,
	idempotent bool,
) {
	actor := actorID
	if actor == "" {
		actor = auditActor(ctx)
	}
	detailMap := map[string]interface{}{
		"target_email":    targetEmail,
		"target_username": targetUsername,
		"idempotent":      idempotent,
	}
	detailBytes, _ := json.Marshal(detailMap)
	r.log(ctx, &types.AuditLog{
		ActorUserID:  actor,
		Action:       types.AuditActionSystemAdminPromoted,
		TargetType:   "user",
		TargetID:     targetUserID,
		TargetUserID: targetUserID,
		Outcome:      types.AuditOutcomeSuccess,
		Details:      types.JSON(detailBytes),
	})
}

// RecordSystemAdminRevoked records a system-admin privilege removal.
func (r *BusinessAuditRecorder) RecordSystemAdminRevoked(
	ctx context.Context,
	actorID, targetUserID, targetEmail, targetUsername string,
	changed bool,
) {
	actor := actorID
	if actor == "" {
		actor = auditActor(ctx)
	}
	detailMap := map[string]interface{}{
		"target_email":    targetEmail,
		"target_username": targetUsername,
		"changed":         changed,
	}
	detailBytes, _ := json.Marshal(detailMap)
	r.log(ctx, &types.AuditLog{
		ActorUserID:  actor,
		Action:       types.AuditActionSystemAdminRevoked,
		TargetType:   "user",
		TargetID:     targetUserID,
		TargetUserID: targetUserID,
		Outcome:      types.AuditOutcomeSuccess,
		Details:      types.JSON(detailBytes),
	})
}

// ------------------------------------------------------------------
// 问答记录 (qa.* / session.*)
// ------------------------------------------------------------------

// RecordQACompleted records a question-answering completion event.
func (r *BusinessAuditRecorder) RecordQACompleted(
	ctx context.Context,
	sessionID uint64, question string,
	knowledgeBaseID string,
	responseType string,
	citationCount int,
) {
	detailMap := map[string]interface{}{
		"session_id":        sessionID,
		"question":          truncateStr(question, 500),
		"knowledge_base_id": knowledgeBaseID,
		"response_type":     responseType,
		"citation_count":    citationCount,
	}
	detailBytes, _ := json.Marshal(detailMap)
	r.log(ctx, &types.AuditLog{
		ActorUserID: auditActor(ctx),
		Action:      types.AuditActionQACompleted,
		TargetType:  "session",
		TargetID:    fmtID(sessionID),
		Outcome:     types.AuditOutcomeSuccess,
		Details:     types.JSON(detailBytes),
	})
}

// ------------------------------------------------------------------
// 登录/登出 (auth.*)
// ------------------------------------------------------------------

// RecordLogin records a successful login event.
func (r *BusinessAuditRecorder) RecordLogin(
	ctx context.Context,
	userID, email, method, clientIP string,
) {
	detailMap := map[string]interface{}{
		"user_id":   userID,
		"email":     email,
		"method":    method,
		"client_ip": clientIP,
	}
	detailBytes, _ := json.Marshal(detailMap)
	r.log(ctx, &types.AuditLog{
		ActorUserID:  auditActor(ctx),
		Action:       types.AuditActionLogin,
		TargetType:   "user",
		TargetID:     userID,
		TargetUserID: userID,
		ClientIP:     clientIP,
		Outcome:      types.AuditOutcomeSuccess,
		Details:      types.JSON(detailBytes),
	})
}

// RecordLoginFailed records a failed login attempt.
func (r *BusinessAuditRecorder) RecordLoginFailed(
	ctx context.Context,
	email, method, reason, clientIP string,
) {
	detailMap := map[string]interface{}{
		"email":     email,
		"method":    method,
		"reason":    reason,
		"client_ip": clientIP,
	}
	detailBytes, _ := json.Marshal(detailMap)
	r.log(ctx, &types.AuditLog{
		ActorUserID: "",
		Action:      types.AuditActionLoginFailed,
		TargetType:  "user",
		ClientIP:    clientIP,
		Outcome:     types.AuditOutcomeDenied,
		Details:     types.JSON(detailBytes),
	})
}

// RecordLogout records a user logout event.
func (r *BusinessAuditRecorder) RecordLogout(
	ctx context.Context,
	userID, email string,
) {
	detailMap := map[string]interface{}{
		"user_id": userID,
		"email":   email,
	}
	detailBytes, _ := json.Marshal(detailMap)
	r.log(ctx, &types.AuditLog{
		ActorUserID:  auditActor(ctx),
		Action:       types.AuditActionLogout,
		TargetType:   "user",
		TargetID:     userID,
		TargetUserID: userID,
		Outcome:      types.AuditOutcomeSuccess,
		Details:      types.JSON(detailBytes),
	})
}

// ------------------------------------------------------------------
// 知识文档下载 (knowledge.*)
// ------------------------------------------------------------------

// RecordKnowledgeDownloaded records a knowledge document download event.
func (r *BusinessAuditRecorder) RecordKnowledgeDownloaded(
	ctx context.Context,
	knowledgeID, title, fileName string,
	fileSize int64, knowledgeBaseID string,
) {
	detailMap := map[string]interface{}{
		"knowledge_id":      knowledgeID,
		"title":             title,
		"file_name":         fileName,
		"file_size":         fileSize,
		"knowledge_base_id": knowledgeBaseID,
	}
	detailBytes, _ := json.Marshal(detailMap)
	r.log(ctx, &types.AuditLog{
		ActorUserID: auditActor(ctx),
		Action:      types.AuditActionKnowledgeDownloaded,
		TargetType:  "knowledge",
		TargetID:    knowledgeID,
		Outcome:     types.AuditOutcomeSuccess,
		Details:     types.JSON(detailBytes),
	})
}

// ------------------------------------------------------------------
// helpers
// ------------------------------------------------------------------

// sanitizeDetails removes sensitive key patterns from a map to avoid
// logging passwords, tokens, or secrets in audit details.
func sanitizeDetails(m map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	sensitive := map[string]bool{
		"password": true, "token": true, "secret": true,
		"api_key": true, "apikey": true, "key": true,
		"credential": true, "passwd": true, "private_key": true,
	}
	for k, v := range m {
		if sensitive[k] {
			out[k] = "[REDACTED]"
		} else {
			out[k] = v
		}
	}
	return out
}
