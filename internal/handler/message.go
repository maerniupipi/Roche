package handler

import (
	stderrors "errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/logger"
	"roche.local/knowledge-agent-platform/internal/types"
	"roche.local/knowledge-agent-platform/internal/types/interfaces"
	secutils "roche.local/knowledge-agent-platform/internal/utils"
)

// MessageHandler handles HTTP requests related to messages within chat sessions
// It provides endpoints for loading and managing message history
type MessageHandler struct {
	MessageService interfaces.MessageService // Service that implements message business logic
}

// NewMessageHandler creates a new message handler instance with the required service
// Parameters:
//   - messageService: Service that implements message business logic
//
// Returns a pointer to a new MessageHandler
func NewMessageHandler(messageService interfaces.MessageService) *MessageHandler {
	return &MessageHandler{
		MessageService: messageService,
	}
}

// LoadMessages godoc
// @Summary      加载消息历史
// @Description  加载会话的消息历史，支持分页和时间筛选
// @Tags         消息
// @Accept       json
// @Produce      json
// @Param        session_id   path      string  true   "会话ID"
// @Param        limit        query     int     false  "返回数量"  default(50)
// @Param        before_time  query     string  false  "在此时间之前的消息（RFC3339Nano格式）"
// @Success      200          {object}  map[string]interface{}  "消息列表"
// @Failure      400          {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Router       /messages/{session_id}/load [get]
func (h *MessageHandler) LoadMessages(c *gin.Context) {
	ctx := c.Request.Context()

	logger.Info(ctx, "Start loading messages")

	// Get path parameters and query parameters
	sessionID := secutils.SanitizeForLog(c.Param("session_id"))
	limit := secutils.SanitizeForLog(c.DefaultQuery("limit", "50"))
	beforeTimeStr := secutils.SanitizeForLog(c.DefaultQuery("before_time", ""))

	logger.Infof(ctx, "Loading messages params, session ID: %s, limit: %s, before time: %s",
		sessionID, limit, beforeTimeStr)

	// Parse limit parameter with fallback to default
	limitInt, err := strconv.Atoi(limit)
	if err != nil {
		logger.Warnf(ctx, "Invalid limit value, using default value 50, input: %s", limit)
		limitInt = 50
	}

	// If no beforeTime is provided, retrieve the most recent messages
	if beforeTimeStr == "" {
		logger.Infof(ctx, "Getting recent messages for session, session ID: %s, limit: %d", sessionID, limitInt)
		messages, err := h.MessageService.GetRecentMessagesBySession(ctx, sessionID, limitInt)
		if err != nil {
			if stderrors.Is(err, errors.ErrSessionNotFound) {
				// PR #1309 plumbed user-scope into the message service's
				// session existence check; non-owner / wrong-knowledgeDomain lookups
				// surface as ErrSessionNotFound. Map to 404 so clients can
				// tell "wrong URL" from a real 5xx.
				logger.Warnf(ctx, "Session not found, ID: %s", sessionID)
				c.Error(errors.NewNotFoundError(err.Error()))
				return
			}
			logger.ErrorWithFields(ctx, err, nil)
			c.Error(errors.NewInternalServerError(err.Error()))
			return
		}

		logger.Infof(
			ctx,
			"Successfully retrieved recent messages, session ID: %s, message count: %d",
			sessionID, len(messages),
		)
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    messages,
		})
		return
	}

	// If beforeTime is provided, parse the timestamp (RFC3339Nano or RFC3339).
	beforeTime, err := parseMessageBeforeTime(beforeTimeStr)
	if err != nil {
		logger.Errorf(
			ctx,
			"Invalid time format, please use RFC3339/RFC3339Nano format, err: %v, beforeTimeStr: %s",
			err, beforeTimeStr,
		)
		c.Error(errors.NewBadRequestError("Invalid time format, please use RFC3339 or RFC3339Nano format"))
		return
	}

	// Retrieve messages before the specified timestamp
	logger.Infof(ctx, "Getting messages before specific time, session ID: %s, before time: %s, limit: %d",
		sessionID, beforeTime.Format(time.RFC3339Nano), limitInt)
	messages, err := h.MessageService.GetMessagesBySessionBeforeTime(ctx, sessionID, beforeTime, limitInt)
	if err != nil {
		if stderrors.Is(err, errors.ErrSessionNotFound) {
			// See note on the GetRecentMessagesBySession path above.
			logger.Warnf(ctx, "Session not found, ID: %s", sessionID)
			c.Error(errors.NewNotFoundError(err.Error()))
			return
		}
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	logger.Infof(
		ctx,
		"Successfully retrieved messages before time, session ID: %s, message count: %d",
		sessionID, len(messages),
	)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    messages,
	})
}

// DeleteMessage godoc
// @Summary      删除消息
// @Description  从会话中删除指定消息
// @Tags         消息
// @Accept       json
// @Produce      json
// @Param        session_id  path      string  true  "会话ID"
// @Param        id          path      string  true  "消息ID"
// @Success      200         {object}  map[string]interface{}  "删除成功"
// @Failure      500         {object}  errors.AppError         "服务器错误"
// @Security     Bearer
// @Router       /messages/{session_id}/{id} [delete]
func (h *MessageHandler) DeleteMessage(c *gin.Context) {
	ctx := c.Request.Context()

	logger.Info(ctx, "Start deleting message")

	// Get path parameters for session and message identification
	sessionID := secutils.SanitizeForLog(c.Param("session_id"))
	messageID := secutils.SanitizeForLog(c.Param("id"))

	logger.Infof(ctx, "Deleting message, session ID: %s, message ID: %s", sessionID, messageID)

	// Delete the message using the message service
	if err := h.MessageService.DeleteMessage(ctx, sessionID, messageID); err != nil {
		if stderrors.Is(err, errors.ErrSessionNotFound) {
			// See note on LoadMessages above — message-service operations
			// surface ErrSessionNotFound when the caller can't see the
			// owning session (post-#1309 user scope).
			logger.Warnf(ctx, "Session not found, ID: %s", sessionID)
			c.Error(errors.NewNotFoundError(err.Error()))
			return
		}
		if stderrors.Is(err, gorm.ErrRecordNotFound) {
			// The message_id doesn't exist under this session — a client error,
			// not a server fault. 404 so callers read resource.not_found (a
			// permanent condition, not retryable) instead of a 5xx. Mirrors the
			// ContinueStream / kb / doc / chunk not-found handling.
			logger.Warnf(ctx, "Message not found, session ID: %s, message ID: %s", sessionID, messageID)
			c.Error(errors.NewNotFoundError(err.Error()))
			return
		}
		logger.ErrorWithFields(ctx, err, nil)
		c.Error(errors.NewInternalServerError(err.Error()))
		return
	}

	logger.Infof(ctx, "Message deleted successfully, session ID: %s, message ID: %s", sessionID, messageID)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Message deleted successfully",
	})
}

// MessageFeedbackRequest is the payload used to like or dislike an assistant message.
type MessageFeedbackRequest struct {
	Rating  types.FeedbackRating `json:"rating" binding:"required"`
	Reason  string               `json:"reason"`
	Comment string               `json:"comment"`
}

// SetMessageFeedback godoc
// @Summary      设置回答反馈
// @Description  点赞或点踩一条属于当前用户会话的助手消息；重复调用会覆盖旧反馈
// @Tags         消息
// @Accept       json
// @Produce      json
// @Param        session_id  path  string                  true  "会话ID"
// @Param        id          path  string                  true  "助手消息ID"
// @Param        request     body  MessageFeedbackRequest  true  "反馈内容"
// @Success      200  {object}  map[string]interface{}
// @Security     Bearer
// @Router       /messages/{session_id}/{id}/feedback [put]
func (h *MessageHandler) SetMessageFeedback(c *gin.Context) {
	ctx := c.Request.Context()
	sessionID := strings.TrimSpace(c.Param("session_id"))
	messageID := strings.TrimSpace(c.Param("id"))

	var request MessageFeedbackRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.Error(errors.NewBadRequestError("Invalid feedback request").WithDetails(err.Error()))
		return
	}

	feedback, err := h.MessageService.SetMessageFeedback(ctx, sessionID, messageID, &types.MessageFeedback{
		Rating:  request.Rating,
		Reason:  request.Reason,
		Comment: request.Comment,
	})
	if err != nil {
		handleMessageFeedbackError(c, err)
		return
	}
	feedback.EnrichReasonLabels()
	c.JSON(http.StatusOK, gin.H{"success": true, "data": feedback})
}

// DeleteMessageFeedback godoc
// @Summary      取消回答反馈
// @Description  删除当前用户对助手消息的点赞或点踩
// @Tags         消息
// @Produce      json
// @Param        session_id  path  string  true  "会话ID"
// @Param        id          path  string  true  "助手消息ID"
// @Success      200  {object}  map[string]interface{}
// @Security     Bearer
// @Router       /messages/{session_id}/{id}/feedback [delete]
func (h *MessageHandler) DeleteMessageFeedback(c *gin.Context) {
	err := h.MessageService.DeleteMessageFeedback(
		c.Request.Context(), strings.TrimSpace(c.Param("session_id")), strings.TrimSpace(c.Param("id")),
	)
	if err != nil {
		handleMessageFeedbackError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func handleMessageFeedbackError(c *gin.Context, err error) {
	if _, ok := errors.IsAppError(err); ok {
		c.Error(err)
		return
	}
	if stderrors.Is(err, errors.ErrSessionNotFound) || stderrors.Is(err, gorm.ErrRecordNotFound) {
		c.Error(errors.NewNotFoundError("message not found"))
		return
	}
	logger.ErrorWithFields(c.Request.Context(), err, nil)
	c.Error(errors.NewInternalServerError("failed to update message feedback"))
}

// parseMessageBeforeTime parses the `before_time` query used by LoadMessages.
// Frontend cursors may be RFC3339 (no fractional seconds) or RFC3339Nano.
func parseMessageBeforeTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, stderrors.New("empty before_time")
	}
	layouts := []string{time.RFC3339Nano, time.RFC3339}
	var lastErr error
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, nil
		} else {
			lastErr = err
		}
	}
	return time.Time{}, lastErr
}
