package middleware

import (
	"context"
	stderrors "errors"

	"github.com/gin-gonic/gin"
	apprepo "roche.local/knowledge-agent-platform/internal/application/repository"
	apperrors "roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/types"
)

// This file centralizes knowledge-base authorization for every KB-scoped route.
// Access is resolved from system-administrator or knowledge-domain-administrator
// status, plus explicit user or enterprise-organization grants.

// KBAccess captures the result of a successful KB access resolution.
// Stashed on gin.Context under KBAccessContextKey so handlers that
// need the resolved KB / permission (e.g. to render
// my_permission in the response) can pull it without re-running the
// resolution.
type KBAccess struct {
	KnowledgeBase              *types.KnowledgeBase
	EffectiveKnowledgeDomainID uint64
	Permission                 types.KnowledgeBasePermissionLevel
	Scope                      *types.KnowledgeBaseAccessScope
}

// KBAccessContextKey is the gin.Context key under which a successful
// KB access resolution is stored.
const KBAccessContextKey = "rbac.kb_access"

const kbResolvedKnowledgeIDContextKey = "rbac.kb_access.knowledge_id"

// KBAccessFromContext returns the KBAccess stashed by the guard, if
// any. Handlers that don't care can just rely on the rewritten
// c.Request.Context() for knowledge-domain scoping.
func KBAccessFromContext(c *gin.Context) (*KBAccess, bool) {
	v, ok := c.Get(KBAccessContextKey)
	if !ok {
		return nil, false
	}
	a, ok := v.(*KBAccess)
	return a, ok
}

// KBLookup is the minimum surface ResolveKBAccess needs from the
// knowledge-base service: a single method that turns an ID into a
// KnowledgeBase pointer (or repo.ErrKnowledgeBaseNotFound). Defining
// it as a tiny dedicated interface keeps the guard testable without
// forcing test stubs to satisfy the full KnowledgeBaseService surface.
type KBLookup interface {
	GetKnowledgeBaseByID(ctx context.Context, id string) (*types.KnowledgeBase, error)
}

// enterpriseKBAccessChecker is implemented by the production KB service.
// Keeping it separate from KBLookup avoids forcing narrow test doubles and
// unrelated service implementations to grow enterprise authorization methods.
type enterpriseKBAccessChecker interface {
	CanReadKnowledgeBase(ctx context.Context, kb *types.KnowledgeBase) (bool, error)
	CanManageKnowledgeBase(ctx context.Context, kb *types.KnowledgeBase) (bool, error)
}

type enterpriseKBAccessScopeChecker interface {
	ResolveKnowledgeBaseAccess(ctx context.Context, kb *types.KnowledgeBase) (*types.KnowledgeBaseAccessScope, error)
}

// KnowledgeLookup mirrors KBLookup but for resolving a knowledge id
// (document id) back to its parent KB. Used by the chunk routes whose
// URL param is a knowledge_id, not a kb_id.
type KnowledgeLookup interface {
	GetKnowledgeByIDOnly(ctx context.Context, id string) (*types.Knowledge, error)
}

// ChunkLookup mirrors KBLookup for resolving a chunk id back to its
// owning knowledge document, which then resolves to the parent KB.
// Used by the /chunks/by-id/:id routes that address chunks directly.
type ChunkLookup interface {
	GetChunkByIDOnly(ctx context.Context, id string) (*types.Chunk, error)
}

// KBIDResolver tells the guard how to find the kb_id for a given
// request. Built-in resolvers below cover the param shapes we use:
// :id, :kb_id, :kbId, :knowledge_id (-> parent KB).
//
// On error, resolvers MUST return either a 4xx apperror (bad request /
// not found) or a generic Go error for transient/internal failures;
// the guard maps the latter to 503.
type KBIDResolver func(c *gin.Context) (string, error)

// KBIDFromParam returns a resolver that reads a fixed gin param.
func KBIDFromParam(param string) KBIDResolver {
	return func(c *gin.Context) (string, error) {
		v := c.Param(param)
		if v == "" {
			return "", apperrors.NewBadRequestError("missing " + param + " in path")
		}
		return v, nil
	}
}

// KBIDFromKnowledgeIDParam reads `:knowledge_id` from the URL, looks
// up the knowledge document, and returns its KB id. Used by the chunk
// routes that address a chunk via /chunks/:knowledge_id.
//
// A genuine "not found" maps to 404; transient errors (DB hiccup,
// service unavailable) are surfaced as a plain Go error so the guard
// can return 503 instead of pretending the resource doesn't exist
// (a 404 here would also short-circuit any retry / monitoring).
func KBIDFromKnowledgeIDParam(param string, kgService KnowledgeLookup) KBIDResolver {
	return func(c *gin.Context) (string, error) {
		v := c.Param(param)
		if v == "" {
			return "", apperrors.NewBadRequestError("missing " + param + " in path")
		}
		k, err := kgService.GetKnowledgeByIDOnly(c.Request.Context(), v)
		if err != nil {
			if isResourceNotFound(err) {
				return "", apperrors.NewNotFoundError("Knowledge not found")
			}
			return "", err
		}
		if k == nil {
			return "", apperrors.NewNotFoundError("Knowledge not found")
		}
		c.Set(kbResolvedKnowledgeIDContextKey, k.ID)
		return k.KnowledgeBaseID, nil
	}
}

// KBIDFromChunkIDParam walks chunk_id -> knowledge_id -> kb_id.
// Used by /chunks/by-id/:id routes that address a chunk directly. The
// chunk's KnowledgeBaseID is denormalised on the row, so a single
// lookup is enough — no need to chain through GetKnowledgeByIDOnly.
//
// Not-found / transient split mirrors KBIDFromKnowledgeIDParam.
func KBIDFromChunkIDParam(param string, chunkService ChunkLookup) KBIDResolver {
	return func(c *gin.Context) (string, error) {
		v := c.Param(param)
		if v == "" {
			return "", apperrors.NewBadRequestError("missing " + param + " in path")
		}
		ch, err := chunkService.GetChunkByIDOnly(c.Request.Context(), v)
		if err != nil {
			if isResourceNotFound(err) {
				return "", apperrors.NewNotFoundError("Chunk not found")
			}
			return "", err
		}
		if ch == nil {
			return "", apperrors.NewNotFoundError("Chunk not found")
		}
		if ch.KnowledgeBaseID == "" {
			// Should-never-happen on a fresh schema; on legacy rows the
			// chunk effectively isn't resolvable to a KB so the client
			// gets the same 404 they'd get for a missing chunk rather
			// than a 500 that pollutes alerting.
			logger.Warnf(c.Request.Context(),
				"[kb_access] chunk %s has empty knowledge_base_id; treating as not-found", v)
			return "", apperrors.NewNotFoundError("Chunk not found")
		}
		c.Set(kbResolvedKnowledgeIDContextKey, ch.KnowledgeID)
		return ch.KnowledgeBaseID, nil
	}
}

// isResourceNotFound recognises the various "not found" sentinels we
// might see from the underlying services. Keeps the resolvers above
// from forcing every service to standardise on a single error type
// before this refactor is useful.
func isResourceNotFound(err error) bool {
	// ErrChunkNotFound is defined in the repository layer and aliased by the
	// service; match the canonical repo sentinel so this predicate depends
	// only on the repository package (KB / Knowledge / Chunk are all here).
	return stderrors.Is(err, apprepo.ErrKnowledgeBaseNotFound) ||
		stderrors.Is(err, apprepo.ErrKnowledgeNotFound) ||
		stderrors.Is(err, apprepo.ErrChunkNotFound)
}

// RequireKBAccess resolves the KB, enforces the requested enterprise
// permission, and stores the result under
// KBAccessContextKey AND rewrites c.Request.Context() to carry the
// effective knowledge-domain ID. Handlers downstream read the domain from
// context as before.
//
// On failure the guard aborts with the appropriate HTTP status (400 /
// 401 / 404 / 403 / 503). Behaviour matches what each handler's
// effectiveCtxForKB helper used to do; the guard is what consolidates
// the repetition so a fix in the resolution order propagates to every
// gated route at once.
func RequireKBAccess(
	resolveKBID KBIDResolver,
	requiredPermission types.KnowledgeBasePermissionLevel,
	kbService KBLookup,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		kbID, err := resolveKBID(c)
		if err != nil {
			_ = c.Error(err)
			c.Abort()
			return
		}

		ctx := c.Request.Context()
		knowledgeID, _ := c.Get(kbResolvedKnowledgeIDContextKey)
		resolvedKnowledgeID, _ := knowledgeID.(string)
		access, err := resolveKBAccessOnce(
			ctx,
			kbID,
			resolvedKnowledgeID,
			requiredPermission,
			kbService,
		)
		switch {
		case stderrors.Is(err, errKBAccessUnauthorized):
			_ = c.Error(apperrors.NewUnauthorizedError("Unauthorized"))
			c.Abort()
			return
		case stderrors.Is(err, errKBAccessNotFound):
			// 404 still fires when enforcement is off — a missing KB is
			// not an authorisation event, the client genuinely asked
			// for nothing.
			_ = c.Error(apperrors.NewNotFoundError("knowledge base not found"))
			c.Abort()
			return
		case stderrors.Is(err, errKBAccessForbidden):
			_ = c.Error(apperrors.NewForbiddenError("Permission denied to access this knowledge base"))
			c.Abort()
			return
		case err != nil:
			logger.ErrorWithFields(ctx, err, nil)
			// Transient/internal -> 503 so monitoring catches the
			// underlying failure rather than a misleading 500.
			_ = c.Error(apperrors.NewServiceUnavailableError("cannot verify KB access right now"))
			c.Abort()
			return
		}

		// Stash the resolution and keep the request knowledge-domain context aligned
		// with the authorized KB.
		c.Set(KBAccessContextKey, access)
		newCtx := context.WithValue(ctx, types.KnowledgeDomainIDContextKey, access.EffectiveKnowledgeDomainID)
		c.Request = c.Request.WithContext(newCtx)
		c.Next()
	}
}

// resolveKBAccessOnce performs the authorization resolution. Kept
// unexported and using package-private sentinel errors so the guard's
// error mapping is the only public surface.
func resolveKBAccessOnce(
	ctx context.Context,
	kbID string,
	knowledgeID string,
	requiredPermission types.KnowledgeBasePermissionLevel,
	kbService KBLookup,
) (*KBAccess, error) {
	if _, ok := types.UserIDFromContext(ctx); !ok {
		return nil, errKBAccessUnauthorized
	}

	kb, err := kbService.GetKnowledgeBaseByID(ctx, kbID)
	if err != nil {
		if stderrors.Is(err, apprepo.ErrKnowledgeBaseNotFound) {
			return nil, errKBAccessNotFound
		}
		return nil, err
	}
	if kb == nil {
		return nil, errKBAccessNotFound
	}

	if checker, ok := kbService.(enterpriseKBAccessScopeChecker); ok {
		scope, scopeErr := checker.ResolveKnowledgeBaseAccess(ctx, kb)
		if scopeErr != nil {
			return nil, scopeErr
		}
		if scope == nil || !scope.Allowed {
			return nil, errKBAccessForbidden
		}
		if requiredPermission == types.KnowledgeBasePermissionManage && !scope.CanManage {
			return nil, errKBAccessForbidden
		}
		if requiredPermission == types.KnowledgeBasePermissionRead &&
			knowledgeID != "" &&
			!scope.AllowsKnowledge(knowledgeID) {
			return nil, errKBAccessForbidden
		}
		permission := scope.Permission
		if requiredPermission == types.KnowledgeBasePermissionManage {
			permission = types.KnowledgeBasePermissionManage
		}
		return &KBAccess{
			KnowledgeBase:              kb,
			EffectiveKnowledgeDomainID: kb.KnowledgeDomainID,
			Permission:                 permission,
			Scope:                      scope,
		}, nil
	}

	if checker, ok := kbService.(enterpriseKBAccessChecker); ok {
		var allowed bool
		if requiredPermission == types.KnowledgeBasePermissionRead {
			allowed, err = checker.CanReadKnowledgeBase(ctx, kb)
		} else {
			allowed, err = checker.CanManageKnowledgeBase(ctx, kb)
		}
		if err != nil {
			return nil, err
		}
		if !allowed {
			return nil, errKBAccessForbidden
		}
		return &KBAccess{
			KnowledgeBase:              kb,
			EffectiveKnowledgeDomainID: kb.KnowledgeDomainID,
			Permission:                 requiredPermission,
			Scope: &types.KnowledgeBaseAccessScope{
				Allowed:    true,
				CanManage:  requiredPermission == types.KnowledgeBasePermissionManage,
				FullAccess: true,
				Permission: requiredPermission,
			},
		}, nil
	}

	return nil, errKBAccessForbidden
}

var (
	errKBAccessUnauthorized = stderrors.New("kb_access: unauthorized")
	errKBAccessNotFound     = stderrors.New("kb_access: not found")
	errKBAccessForbidden    = stderrors.New("kb_access: forbidden")
)
