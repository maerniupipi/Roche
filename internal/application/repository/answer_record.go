package repository

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

type adminAnswerRecordRepository struct{ db *gorm.DB }

func NewAdminAnswerRecordRepository(db *gorm.DB) interfaces.AdminAnswerRecordRepository {
	return &adminAnswerRecordRepository{db: db}
}

type adminAnswerRecordDBRow struct {
	ID                  string     `gorm:"column:id"`
	SessionID           string     `gorm:"column:session_id"`
	RequestID           string     `gorm:"column:request_id"`
	Channel             string     `gorm:"column:channel"`
	UserID              string     `gorm:"column:user_id"`
	Username            string     `gorm:"column:username"`
	SessionTitle        string     `gorm:"column:session_title"`
	Question            string     `gorm:"column:question"`
	Answer              string     `gorm:"column:answer"`
	KnowledgeReferences types.JSON `gorm:"column:knowledge_references"`
	QuestionedAt        time.Time  `gorm:"column:questioned_at"`
	AnswerFinishedAt    time.Time  `gorm:"column:answer_finished_at"`
	FeedbackID          *string    `gorm:"column:feedback_id"`
	FeedbackRating      *string    `gorm:"column:feedback_rating"`
	FeedbackReason      *string    `gorm:"column:feedback_reason"`
	FeedbackComment     *string    `gorm:"column:feedback_comment"`
	FeedbackCreatedAt   *time.Time `gorm:"column:feedback_created_at"`
	FeedbackUpdatedAt   *time.Time `gorm:"column:feedback_updated_at"`
}

func (r *adminAnswerRecordRepository) Query(
	ctx context.Context,
	query *types.AdminAnswerRecordQuery,
) ([]types.AdminAnswerRecord, int64, error) {
	if query == nil {
		query = &types.AdminAnswerRecordQuery{}
	}
	base := r.applyAdminAnswerRecordFilters(r.db.WithContext(ctx).
		Table("messages AS question").
		Joins("JOIN messages AS answer ON answer.session_id = question.session_id AND answer.request_id = question.request_id AND answer.role = ? AND answer.deleted_at IS NULL", "assistant").
		Joins("JOIN sessions AS session ON session.id = question.session_id AND session.deleted_at IS NULL").
		Joins("LEFT JOIN users AS app_user ON app_user.id = session.user_id").
		Joins("LEFT JOIN message_feedbacks AS feedback ON feedback.message_id = answer.id"), query)

	var total int64
	if err := base.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page, pageSize := query.Page, query.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 1000 {
		pageSize = 1000
	}
	var rows []adminAnswerRecordDBRow
	err := base.Session(&gorm.Session{}).
		Select(`answer.id AS id,
                question.session_id AS session_id,
                question.request_id AS request_id,
                COALESCE(NULLIF(question.channel, ''), NULLIF(answer.channel, ''), 'web') AS channel,
                session.user_id AS user_id,
                COALESCE(NULLIF(app_user.username, ''), NULLIF(app_user.chinese_name, ''), NULLIF(app_user.english_name, ''), session.user_id) AS username,
                session.title AS session_title,
                question.content AS question,
                answer.content AS answer,
                answer.knowledge_references AS knowledge_references,
                question.created_at AS questioned_at,
                answer.updated_at AS answer_finished_at,
                feedback.id AS feedback_id,
                feedback.rating AS feedback_rating,
                feedback.reason AS feedback_reason,
                feedback.comment AS feedback_comment,
                feedback.created_at AS feedback_created_at,
                feedback.updated_at AS feedback_updated_at`).
		Order("question.created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}

	kbNames, err := r.resolveKnowledgeBaseNames(ctx, rows)
	if err != nil {
		return nil, 0, err
	}
	records := make([]types.AdminAnswerRecord, 0, len(rows))
	for _, row := range rows {
		references := decodeAnswerRecordReferences(row.KnowledgeReferences)
		record := types.AdminAnswerRecord{
			ID: row.ID, SessionID: row.SessionID, RequestID: row.RequestID,
			Channel: row.Channel, UserID: row.UserID, Username: row.Username,
			SessionTitle: row.SessionTitle, Question: row.Question, Answer: row.Answer,
			KnowledgeBases: knowledgeBaseNamesForReferences(references, kbNames),
			QuestionedAt:   row.QuestionedAt, AnswerFinishedAt: row.AnswerFinishedAt,
		}
		if row.FeedbackID != nil {
			record.Feedback = &types.MessageFeedback{
				ID: valueOrEmpty(row.FeedbackID), MessageID: row.ID, SessionID: row.SessionID,
				Rating: types.FeedbackRating(valueOrEmpty(row.FeedbackRating)),
				Reason: valueOrEmpty(row.FeedbackReason), Comment: valueOrEmpty(row.FeedbackComment),
			}
			if row.FeedbackCreatedAt != nil {
				record.Feedback.CreatedAt = *row.FeedbackCreatedAt
			}
			if row.FeedbackUpdatedAt != nil {
				record.Feedback.UpdatedAt = *row.FeedbackUpdatedAt
			}
			record.Feedback.EnrichReasonLabels()
		}
		records = append(records, record)
	}
	return records, total, nil
}

func (r *adminAnswerRecordRepository) applyAdminAnswerRecordFilters(
	db *gorm.DB,
	query *types.AdminAnswerRecordQuery,
) *gorm.DB {
	db = db.Where("question.role = ? AND question.deleted_at IS NULL AND question.request_id <> ''", "user")
	if channel := strings.TrimSpace(query.Channel); channel != "" {
		db = db.Where("COALESCE(NULLIF(question.channel, ''), NULLIF(answer.channel, ''), 'web') = ?", channel)
	}
	if username := strings.TrimSpace(query.Username); username != "" {
		like := "%" + strings.ToLower(username) + "%"
		db = db.Where(`(LOWER(COALESCE(app_user.username, '')) LIKE ? OR
            LOWER(COALESCE(app_user.chinese_name, '')) LIKE ? OR
            LOWER(COALESCE(app_user.english_name, '')) LIKE ? OR
			LOWER(COALESCE(app_user.email, '')) LIKE ?)`, like, like, like, like)
	}
	switch strings.ToLower(strings.TrimSpace(query.Feedback)) {
	case "none":
		db = db.Where("feedback.id IS NULL")
	case "like", "dislike":
		db = db.Where("feedback.rating = ?", strings.ToLower(strings.TrimSpace(query.Feedback)))
	}
	if query.IsFallback != nil {
		db = db.Where("answer.is_fallback = ?", *query.IsFallback)
	}
	if query.StartTime != nil {
		db = db.Where("question.created_at >= ?", *query.StartTime)
	}
	if query.EndTime != nil {
		db = db.Where("question.created_at <= ?", *query.EndTime)
	}
	return db
}

func (r *adminAnswerRecordRepository) resolveKnowledgeBaseNames(
	ctx context.Context,
	rows []adminAnswerRecordDBRow,
) (map[string]string, error) {
	ids := make(map[string]struct{})
	for _, row := range rows {
		for _, reference := range decodeAnswerRecordReferences(row.KnowledgeReferences) {
			if reference != nil && strings.TrimSpace(reference.KnowledgeBaseID) != "" {
				ids[reference.KnowledgeBaseID] = struct{}{}
			}
		}
	}
	if len(ids) == 0 {
		return map[string]string{}, nil
	}
	list := make([]string, 0, len(ids))
	for id := range ids {
		list = append(list, id)
	}
	type knowledgeBaseName struct {
		ID   string
		Name string
	}
	var names []knowledgeBaseName
	if err := r.db.WithContext(ctx).Table("knowledge_bases").
		Select("id, name").Where("id IN ? AND deleted_at IS NULL", list).Scan(&names).Error; err != nil {
		return nil, err
	}
	result := make(map[string]string, len(names))
	for _, item := range names {
		result[item.ID] = item.Name
	}
	return result, nil
}

func decodeAnswerRecordReferences(raw types.JSON) types.References {
	if len(raw) == 0 {
		return nil
	}
	var references types.References
	if err := json.Unmarshal(raw, &references); err != nil {
		return nil
	}
	return references
}

func knowledgeBaseNamesForReferences(references types.References, names map[string]string) []string {
	unique := make(map[string]struct{})
	result := make([]string, 0)
	for _, reference := range references {
		if reference == nil {
			continue
		}
		name := strings.TrimSpace(names[reference.KnowledgeBaseID])
		if name == "" {
			continue
		}
		if _, exists := unique[name]; exists {
			continue
		}
		unique[name] = struct{}{}
		result = append(result, name)
	}
	if len(result) == 0 {
		return []string{}
	}
	sort.Strings(result)
	return result
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
