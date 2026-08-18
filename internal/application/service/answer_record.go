package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"strings"
	"time"

	apperrors "roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
)

type adminAnswerRecordService struct {
	repo interfaces.AdminAnswerRecordRepository
}

func NewAdminAnswerRecordService(repo interfaces.AdminAnswerRecordRepository) interfaces.AdminAnswerRecordService {
	return &adminAnswerRecordService{repo: repo}
}

func (s *adminAnswerRecordService) List(
	ctx context.Context,
	query *types.AdminAnswerRecordQuery,
) (*types.PageResult, error) {
	if query == nil {
		query = &types.AdminAnswerRecordQuery{}
	}
	query.Channel = strings.ToLower(strings.TrimSpace(query.Channel))
	query.Feedback = strings.ToLower(strings.TrimSpace(query.Feedback))
	if query.Channel != "" && query.Channel != "web" && query.Channel != "app" {
		return nil, apperrors.NewBadRequestError("channel must be web or app")
	}
	if query.Feedback != "" && query.Feedback != "like" && query.Feedback != "dislike" && query.Feedback != "none" {
		return nil, apperrors.NewBadRequestError("feedback must be like, dislike or none")
	}
	if query.StartTime != nil && query.EndTime != nil && query.StartTime.After(*query.EndTime) {
		return nil, apperrors.NewBadRequestError("start_time must not be later than end_time")
	}
	pagination := &types.Pagination{Page: query.Page, PageSize: query.PageSize}
	query.Page, query.PageSize = pagination.GetPage(), pagination.GetPageSize()
	records, total, err := s.repo.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	return types.NewPageResult(total, pagination, records), nil
}

// Export applies the same filters as List and returns an Excel-friendly UTF-8
// CSV. Records are read in bounded pages so a large export does not require a
// single unbounded database query.
func (s *adminAnswerRecordService) Export(
	ctx context.Context,
	query *types.AdminAnswerRecordQuery,
) ([]byte, error) {
	if query == nil {
		query = &types.AdminAnswerRecordQuery{}
	}
	if _, err := s.List(ctx, &types.AdminAnswerRecordQuery{
		Channel: query.Channel, Username: query.Username, Feedback: query.Feedback,
		IsFallback: query.IsFallback, StartTime: query.StartTime, EndTime: query.EndTime,
		Page: 1, PageSize: 1,
	}); err != nil {
		return nil, err
	}
	query.Channel = strings.ToLower(strings.TrimSpace(query.Channel))
	query.Feedback = strings.ToLower(strings.TrimSpace(query.Feedback))

	var buffer bytes.Buffer
	buffer.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(&buffer)
	if err := writer.Write([]string{
		"应用端", "用户名", "会话名称", "问题", "系统回答", "所属知识库",
		"反馈", "反馈原因编码", "反馈原因中文", "反馈原因英文", "具体内容", "用户提问时间",
	}); err != nil {
		return nil, err
	}
	const exportPageSize = 1000
	for page := 1; ; page++ {
		pageQuery := *query
		pageQuery.Page, pageQuery.PageSize = page, exportPageSize
		records, _, err := s.repo.Query(ctx, &pageQuery)
		if err != nil {
			return nil, err
		}
		for _, record := range records {
			feedback, reason, reasonZh, reasonEn, comment := "", "", "", "", ""
			if record.Feedback != nil {
				feedback = string(record.Feedback.Rating)
				reason, reasonZh, reasonEn = record.Feedback.Reason, record.Feedback.ReasonZh, record.Feedback.ReasonEn
				comment = record.Feedback.Comment
			}
			if err := writer.Write([]string{
				safeCSVCell(record.Channel), safeCSVCell(record.Username), safeCSVCell(record.SessionTitle),
				safeCSVCell(record.Question), safeCSVCell(record.Answer), safeCSVCell(strings.Join(record.KnowledgeBases, ";")),
				feedback, reason, safeCSVCell(reasonZh), safeCSVCell(reasonEn), safeCSVCell(comment),
				record.QuestionedAt.Format(time.RFC3339),
			}); err != nil {
				return nil, err
			}
		}
		if len(records) < exportPageSize {
			break
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("write answer record CSV: %w", err)
	}
	return buffer.Bytes(), nil
}

func safeCSVCell(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + value
	}
	return value
}
