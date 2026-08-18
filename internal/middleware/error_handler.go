package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"roche.local/knowledge-agent-platform/internal/errors"
	"roche.local/knowledge-agent-platform/internal/response"
)

// ErrorHandler 是一个处理应用错误的中间件，使用统一响应格式 {code, msg, data}
func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if len(c.Errors) > 0 {
			err := c.Errors.Last().Err

			if appErr, ok := errors.IsAppError(err); ok {
				response.Error(c, appErr.HTTPCode, int(appErr.Code), appErr.Message)
				return
			}

			response.Error(c, http.StatusInternalServerError, int(errors.ErrInternalServer), "Internal server error")
		}
	}
}
