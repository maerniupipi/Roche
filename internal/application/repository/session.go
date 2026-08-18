package repository

import (
	"context"
	stderrors "errors"
	"strings"
	"time"

	"gorm.io/gorm"
	apperrors "roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

// sessionRepository implements the SessionRepository interface
type sessionRepository struct {
	db *gorm.DB
}

func applySessionUserScope(db *gorm.DB, userID string) *gorm.DB {
	if userID == "" {
		return db
	}
	return db.Where("user_id = ?", userID)
}

// NewSessionRepository creates a new session repository instance
func NewSessionRepository(db *gorm.DB) interfaces.SessionRepository {
	return &sessionRepository{db: db}
}

// Create creates a new session
func (r *sessionRepository) Create(ctx context.Context, session *types.Session) (*types.Session, error) {
	session.CreatedAt = time.Now()
	session.UpdatedAt = time.Now()
	if err := r.db.WithContext(ctx).Create(session).Error; err != nil {
		return nil, err
	}
	// Return the session with generated ID
	return session, nil
}

// Get retrieves a session by ID
func (r *sessionRepository) Get(ctx context.Context, userID string, id string) (*types.Session, error) {
	var session types.Session
	err := applySessionUserScope(
		r.db.WithContext(ctx).Where("id = ?", id),
		userID,
	).First(&session).Error
	if err != nil {
		if stderrors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrSessionNotFound
		}
		return nil, err
	}
	return &session, nil
}

// GetByID retrieves a session by id without user scoping.
func (r *sessionRepository) GetByID(ctx context.Context, id string) (*types.Session, error) {
	var session types.Session
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&session).Error
	if err != nil {
		if stderrors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrSessionNotFound
		}
		return nil, err
	}
	return &session, nil
}

// GetByUserID retrieves all sessions owned by a user.
func (r *sessionRepository) GetByUserID(ctx context.Context, userID string) ([]*types.Session, error) {
	var sessions []*types.Session
	err := applySessionUserScope(
		r.db.WithContext(ctx),
		userID,
	).Order("updated_at DESC").Find(&sessions).Error
	if err != nil {
		return nil, err
	}
	return sessions, nil
}

// QueryPaged lists sessions for a user with keyword filters and
// pin-aware ordering.
func (r *sessionRepository) QueryPaged(
	ctx context.Context, q *types.SessionListQuery,
) ([]*types.SessionListItem, int64, error) {
	// Dialect-aware bits so the same query works on Postgres and SQLite (Lite build).
	isPostgres := r.db.Dialector.Name() == "postgres"
	titleLikeExpr := "LOWER(s.title) LIKE LOWER(?)"
	if isPostgres {
		titleLikeExpr = "s.title ILIKE ?"
	}
	// SQLite (the driver used by Lite) does not support NULLS LAST; its default
	// nulls ordering puts NULLs first for DESC, which is actually what we want
	// for pinned_at (rows with pinned_at=NULL are never pinned, so they get
	// filtered to the tail by the preceding is_pinned DESC anyway).
	orderClause := "s.is_pinned DESC, s.pinned_at DESC NULLS LAST, s.updated_at DESC"
	if !isPostgres {
		orderClause = "s.is_pinned DESC, s.pinned_at DESC, s.updated_at DESC"
	}

	// Base filter shared by count and list queries.
	applyBase := func(db *gorm.DB) *gorm.DB {
		db = db.Where("s.deleted_at IS NULL")
		if q.UserID != "" {
			db = db.Where("s.user_id = ?", q.UserID)
		}
		if kw := strings.TrimSpace(q.Keyword); kw != "" {
			db = db.Where(titleLikeExpr, "%"+escapeLikeKeyword(kw)+"%")
		}
		return db
	}

	var total int64
	countQ := applyBase(r.db.WithContext(ctx).Table("sessions AS s"))
	if err := countQ.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page := q.Page
	if page < 1 {
		page = 1
	}
	size := q.PageSize
	if size < 1 {
		size = 20
	}

	items := make([]*types.SessionListItem, 0)
	rowsQ := applyBase(r.db.WithContext(ctx).Table("sessions AS s")).
		Select("s.*").
		Order(orderClause).
		Offset((page - 1) * size).
		Limit(size)
	if err := rowsQ.Find(&items).Error; err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

// SetPinned toggles is_pinned/pinned_at for a single session.
// Scope: must match knowledgeDomain, and user_id (when provided) to prevent pinning
// other users' sessions. Legacy rows with user_id NULL/” remain mutable
// at the knowledgeDomain level (same visibility rule as QueryPaged).
//
// Returns the number of rows affected so callers can distinguish "session
// doesn't exist / not visible to this user" (0) from a real DB error.
func (r *sessionRepository) SetPinned(
	ctx context.Context, userID string, id string, pinned bool,
) (int64, error) {
	now := time.Now()
	updates := map[string]interface{}{
		"is_pinned":  pinned,
		"updated_at": now,
	}
	if pinned {
		updates["pinned_at"] = now
	} else {
		updates["pinned_at"] = nil
	}

	q := r.db.WithContext(ctx).
		Model(&types.Session{}).
		Where("id = ?", id)
	if userID != "" {
		q = q.Where("user_id = ?", userID)
	}
	res := q.Updates(updates)
	return res.RowsAffected, res.Error
}

// Update updates a session
func (r *sessionRepository) Update(ctx context.Context, session *types.Session, userID string) (int64, error) {
	session.UpdatedAt = time.Now()
	res := applySessionUserScope(r.db.WithContext(ctx).
		Model(&types.Session{}).
		Where("id = ?", session.ID), userID).
		Updates(map[string]interface{}{
			"title":       session.Title,
			"description": session.Description,
			"updated_at":  session.UpdatedAt,
		})
	return res.RowsAffected, res.Error
}

// SetOwnerID assigns sessions.user_id for a row.
func (r *sessionRepository) SetOwnerID(ctx context.Context, id, ownerID string) (int64, error) {
	res := r.db.WithContext(ctx).
		Model(&types.Session{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"user_id":    ownerID,
			"updated_at": time.Now(),
		})
	return res.RowsAffected, res.Error
}

// UpdateLastRequestState writes only the last_request_state column and bumps
// updated_at. We deliberately bypass
// the regular Update path so the call doesn't perturb title/description and
// stays cheap (single-row UPDATE by PK).
func (r *sessionRepository) UpdateLastRequestState(
	ctx context.Context, userID string, sessionID string,
	state *types.SessionLastRequestState,
) (int64, error) {
	now := time.Now()
	var stateValue interface{}
	if state != nil {
		v, err := state.Value()
		if err != nil {
			return 0, err
		}
		stateValue = v
	}
	res := applySessionUserScope(r.db.WithContext(ctx).
		Model(&types.Session{}).
		Where("id = ?", sessionID), userID).
		Updates(map[string]interface{}{
			"last_request_state": stateValue,
			"updated_at":         now,
		})
	return res.RowsAffected, res.Error
}

// Delete deletes a session
func (r *sessionRepository) Delete(ctx context.Context, userID string, id string) (int64, error) {
	res := applySessionUserScope(
		r.db.WithContext(ctx).Where("id = ?", id),
		userID,
	).Delete(&types.Session{})
	return res.RowsAffected, res.Error
}

// BatchDelete deletes multiple sessions by IDs
func (r *sessionRepository) BatchDelete(ctx context.Context, userID string, ids []string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	res := applySessionUserScope(
		r.db.WithContext(ctx).Where("id IN ?", ids),
		userID,
	).Delete(&types.Session{})
	return res.RowsAffected, res.Error
}

// DeleteAllByUserID deletes all sessions owned by a user.
func (r *sessionRepository) DeleteAllByUserID(ctx context.Context, userID string) (int64, error) {
	res := applySessionUserScope(
		r.db.WithContext(ctx),
		userID,
	).Delete(&types.Session{})
	return res.RowsAffected, res.Error
}
