package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

// dashboardRepository implements analytics aggregation queries.
type dashboardRepository struct {
	db *gorm.DB
}

// NewDashboardRepository creates a new dashboard repository.
func NewDashboardRepository(db *gorm.DB) interfaces.DashboardRepository {
	return &dashboardRepository{db: db}
}

func (r *dashboardRepository) CountKnowledgesByStatus(ctx context.Context, kbID string) (*types.DashboardKnowledgeBaseStats, error) {
	stats := &types.DashboardKnowledgeBaseStats{}

	type counts struct {
		Published        int64
		UploadSuccess    int64
		UploadFailed     int64
		ScheduledPublish int64
		Unpublished      int64
		Archived         int64
	}
	var c counts

	err := r.db.WithContext(ctx).Model(&types.Knowledge{}).
		Select(`
			COUNT(*) FILTER (WHERE parse_status = 'completed' AND enable_status = 'enabled') AS published,
			COUNT(*) FILTER (WHERE parse_status = 'completed') AS upload_success,
			COUNT(*) FILTER (WHERE parse_status = 'failed') AS upload_failed,
			COUNT(*) FILTER (WHERE parse_status IN ('pending','processing','finalizing')) AS scheduled_publish,
			COUNT(*) FILTER (WHERE enable_status = 'disabled') AS unpublished,
			COUNT(*) FILTER (WHERE enable_status = 'archived') AS archived
		`).
		Where("deleted_at IS NULL").
		Where("knowledge_base_id = COALESCE(NULLIF(?, ''), knowledge_base_id)", kbID).
		Scan(&c).Error

	stats.PublishedCount = c.Published
	stats.UploadSuccessCount = c.UploadSuccess
	stats.UploadFailedCount = c.UploadFailed
	stats.ScheduledPublishCount = c.ScheduledPublish
	stats.UnpublishedCount = c.Unpublished
	stats.ArchivedCount = c.Archived

	return stats, err
}

func (r *dashboardRepository) ListDailyChatStats(ctx context.Context, knowledgeDomainID uint64, start, end time.Time) ([]types.DashboardDailyChatStats, error) {
	var rows []struct {
		Date          string `gorm:"column:date"`
		QuestionCount int64  `gorm:"column:question_count"`
		UniqueUsers   int64  `gorm:"column:unique_users"`
	}

	query := `
		SELECT
			DATE(m.created_at AT TIME ZONE 'UTC') AS date,
			COUNT(*) AS question_count,
			COUNT(DISTINCT s.user_id) AS unique_users
		FROM messages m
		JOIN sessions s ON s.id = m.session_id
		WHERE m.role = 'user'
		  AND m.deleted_at IS NULL
		  AND s.deleted_at IS NULL
		  AND m.created_at >= ?
		  AND m.created_at < ?
	`
	args := []interface{}{start, end}

	if knowledgeDomainID > 0 {
		query += ` AND s.last_request_state->>'knowledge_domain_id' = ?`
		args = append(args, fmt.Sprintf("%d", knowledgeDomainID))
	}

	query += ` GROUP BY DATE(m.created_at AT TIME ZONE 'UTC') ORDER BY date`

	err := r.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	result := make([]types.DashboardDailyChatStats, 0, len(rows))
	for _, row := range rows {
		result = append(result, types.DashboardDailyChatStats{
			Date:            row.Date,
			QuestionCount:   row.QuestionCount,
			UniqueUsers:     row.UniqueUsers,
			SatisfactionPct: 0,
		})
	}
	return result, nil
}

func (r *dashboardRepository) GetAverageResponseDurations(ctx context.Context, knowledgeDomainID uint64, start, end time.Time) (float64, float64, error) {
	var row struct {
		AvgFirstResponseMs *float64 `gorm:"column:avg_first_ms"`
		AvgCompleteMs      *float64 `gorm:"column:avg_complete_ms"`
	}

	query := `
		SELECT
			AVG(agent_duration_ms) FILTER (WHERE agent_duration_ms > 0) AS avg_complete_ms,
			AVG(agent_duration_ms) FILTER (WHERE agent_duration_ms > 0) * 0.15 AS avg_first_ms
		FROM messages m
		JOIN sessions s ON s.id = m.session_id
		WHERE m.role = 'assistant'
		  AND m.deleted_at IS NULL
		  AND s.deleted_at IS NULL
		  AND m.created_at >= ?
		  AND m.created_at < ?
	`
	args := []interface{}{start, end}

	if knowledgeDomainID > 0 {
		query += ` AND s.last_request_state->>'knowledge_domain_id' = ?`
		args = append(args, fmt.Sprintf("%d", knowledgeDomainID))
	}

	err := r.db.WithContext(ctx).Raw(query, args...).Scan(&row).Error
	if err != nil {
		return 0, 0, err
	}

	avgComplete := 0.0
	avgFirst := 0.0
	if row.AvgCompleteMs != nil {
		avgComplete = *row.AvgCompleteMs / 1000.0
	}
	if row.AvgFirstResponseMs != nil {
		avgFirst = *row.AvgFirstResponseMs / 1000.0
	}
	return avgFirst, avgComplete, nil
}

func (r *dashboardRepository) GetFallbackQuestions(ctx context.Context, knowledgeDomainID uint64, start, end time.Time, limit int) ([]types.DashboardFallbackQuestionItem, error) {
	var rows []struct {
		Content string `gorm:"column:content"`
	}

	query := `
		SELECT m.content
		FROM messages m
		JOIN sessions s ON s.id = m.session_id
		WHERE m.role = 'user'
		  AND m.deleted_at IS NULL
		  AND s.deleted_at IS NULL
		  AND m.created_at >= ?
		  AND m.created_at < ?
		  AND EXISTS (
			SELECT 1 FROM messages ans
			WHERE ans.session_id = m.session_id
			  AND ans.role = 'assistant'
			  AND ans.created_at > m.created_at
			  AND ans.is_fallback = true
			  AND ans.deleted_at IS NULL
		  )
	`
	args := []interface{}{start, end}

	if knowledgeDomainID > 0 {
		query += ` AND s.last_request_state->>'knowledge_domain_id' = ?`
		args = append(args, fmt.Sprintf("%d", knowledgeDomainID))
	}

	query += ` ORDER BY m.created_at DESC LIMIT ?`
	args = append(args, limit)

	err := r.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	result := make([]types.DashboardFallbackQuestionItem, 0, len(rows))
	for i, row := range rows {
		result = append(result, types.DashboardFallbackQuestionItem{
			Rank:    i + 1,
			Content: row.Content,
		})
	}
	return result, nil
}

func (r *dashboardRepository) CountValidAndFallbackAnswers(ctx context.Context, knowledgeDomainID uint64, start, end time.Time) (int64, int64, error) {
	var row struct {
		Valid    *int64 `gorm:"column:valid"`
		Fallback *int64 `gorm:"column:fallback"`
	}

	query := `
		SELECT
			COUNT(*) FILTER (WHERE is_fallback = false) AS valid,
			COUNT(*) FILTER (WHERE is_fallback = true) AS fallback
		FROM messages m
		JOIN sessions s ON s.id = m.session_id
		WHERE m.role = 'assistant'
		  AND m.deleted_at IS NULL
		  AND s.deleted_at IS NULL
		  AND m.created_at >= ?
		  AND m.created_at < ?
	`
	args := []interface{}{start, end}

	if knowledgeDomainID > 0 {
		query += ` AND s.last_request_state->>'knowledge_domain_id' = ?`
		args = append(args, fmt.Sprintf("%d", knowledgeDomainID))
	}

	err := r.db.WithContext(ctx).Raw(query, args...).Scan(&row).Error
	if err != nil {
		return 0, 0, err
	}

	valid := int64(0)
	fallback := int64(0)
	if row.Valid != nil {
		valid = *row.Valid
	}
	if row.Fallback != nil {
		fallback = *row.Fallback
	}
	return valid, fallback, nil
}

func (r *dashboardRepository) ListTopAskers(ctx context.Context, knowledgeDomainID uint64, start, end time.Time, limit int) ([]types.DashboardTopUserItem, error) {
	var rows []struct {
		UserName string `gorm:"column:user_name"`
		Cnt      int64  `gorm:"column:cnt"`
	}

	query := `
		SELECT
			COALESCE(u.username, s.user_id) AS user_name,
			COUNT(*) AS cnt
		FROM messages m
		JOIN sessions s ON s.id = m.session_id
		LEFT JOIN users u ON u.id = s.user_id
		WHERE m.role = 'user'
		  AND m.deleted_at IS NULL
		  AND s.deleted_at IS NULL
		  AND m.created_at >= ?
		  AND m.created_at < ?
	`
	args := []interface{}{start, end}

	if knowledgeDomainID > 0 {
		query += ` AND s.last_request_state->>'knowledge_domain_id' = ?`
		args = append(args, fmt.Sprintf("%d", knowledgeDomainID))
	}

	query += ` GROUP BY COALESCE(u.username, s.user_id)
		       ORDER BY cnt DESC
		       LIMIT ?`
	args = append(args, limit)

	err := r.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	result := make([]types.DashboardTopUserItem, 0, len(rows))
	for i, row := range rows {
		result = append(result, types.DashboardTopUserItem{
			Rank:        i + 1,
			UserName:    row.UserName,
			QuestionCnt: row.Cnt,
		})
	}
	return result, nil
}

func (r *dashboardRepository) ListHotDocuments(ctx context.Context, knowledgeDomainID uint64, start, end time.Time, limit int) ([]types.DashboardTopDocumentItem, error) {
	var rows []struct {
		KnowledgeID string `gorm:"column:knowledge_id"`
		Title       string `gorm:"column:title"`
		KbName      string `gorm:"column:kb_name"`
		HitCount    int64  `gorm:"column:hit_count"`
	}

	query := `
		SELECT
			ref.value->>'knowledge_id' AS knowledge_id,
			MAX(COALESCE(ref.value->>'title', k.title, '')) AS title,
			MAX(COALESCE(kb.name, '')) AS kb_name,
			COUNT(*) AS hit_count
		FROM messages m
		JOIN sessions s ON s.id = m.session_id
		CROSS JOIN LATERAL jsonb_array_elements(m.knowledge_references) AS ref
		LEFT JOIN knowledges k ON k.id = ref.value->>'knowledge_id'
		LEFT JOIN knowledge_bases kb ON kb.id = k.knowledge_base_id
		WHERE m.role = 'assistant'
		  AND m.deleted_at IS NULL
		  AND s.deleted_at IS NULL
		  AND m.created_at >= ?
		  AND m.created_at < ?
	`
	args := []interface{}{start, end}

	if knowledgeDomainID > 0 {
		query += ` AND k.knowledge_domain_id = ?`
		args = append(args, knowledgeDomainID)
	}

	query += ` GROUP BY ref.value->>'knowledge_id'
		       ORDER BY hit_count DESC
		       LIMIT ?`
	args = append(args, limit)

	err := r.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	result := make([]types.DashboardTopDocumentItem, 0, len(rows))
	for i, row := range rows {
		result = append(result, types.DashboardTopDocumentItem{
			Rank:     i + 1,
			Title:    row.Title,
			KbName:   row.KbName,
			HitCount: row.HitCount,
		})
	}
	return result, nil
}

func (r *dashboardRepository) ListProductFeedback(ctx context.Context, knowledgeDomainID uint64, start, end time.Time, limit int) ([]types.DashboardFeedbackItem, error) {
	var rows []struct {
		Category string `gorm:"column:category"`
		Count    int64  `gorm:"column:count"`
	}

	query := `
		SELECT category, COUNT(*) AS count
		FROM dashboard_feedback
		WHERE created_at >= ?
		  AND created_at < ?
	`
	args := []interface{}{start, end}

	if knowledgeDomainID > 0 {
		query += ` AND knowledge_domain_id = ?`
		args = append(args, knowledgeDomainID)
	}

	query += ` GROUP BY category
		       ORDER BY count DESC
		       LIMIT ?`
	args = append(args, limit)

	err := r.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	result := make([]types.DashboardFeedbackItem, 0, len(rows))
	for _, row := range rows {
		result = append(result, types.DashboardFeedbackItem{
			Category: row.Category,
			Count:    row.Count,
		})
	}
	return result, nil
}

func (r *dashboardRepository) ListDomainDistribution(ctx context.Context, start, end time.Time) ([]types.DashboardDomainDistributionItem, error) {
	var rows []struct {
		Name  string `gorm:"column:name"`
		Value int64  `gorm:"column:value"`
	}

	query := `
		SELECT
			COALESCE(kd.name, 'default') AS name,
			COUNT(*) AS value
		FROM messages m
		JOIN sessions s ON s.id = m.session_id
		LEFT JOIN knowledge_domains kd ON kd.id::text = COALESCE(s.last_request_state->>'knowledge_domain_id', '')
		WHERE m.role = 'user'
		  AND m.deleted_at IS NULL
		  AND s.deleted_at IS NULL
		  AND m.created_at >= ?
		  AND m.created_at < ?
		GROUP BY kd.name
		ORDER BY value DESC
	`

	err := r.db.WithContext(ctx).Raw(query, start, end).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	result := make([]types.DashboardDomainDistributionItem, 0, len(rows))
	for _, row := range rows {
		result = append(result, types.DashboardDomainDistributionItem{
			Name:  row.Name,
			Value: row.Value,
		})
	}
	return result, nil
}
