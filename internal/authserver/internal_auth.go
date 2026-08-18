package authserver

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"roche.local/knowledge-agent-platform/internal/types"
)

const internalSecretHeader = "X-Auth-Service-Secret"

// InternalTokenValidator is called only by the API gateway through an
// internal-only nginx auth_request location. It never returns a response body;
// identity is conveyed in trusted response headers that the gateway overwrites
// before forwarding the original request to a business backend.
type InternalTokenValidator struct {
	userService  accessTokenValidator
	sharedSecret string
}

type accessTokenValidator interface {
	ValidateToken(ctx context.Context, token string) (*types.User, error)
}

func NewInternalTokenValidator(userService accessTokenValidator, sharedSecret string) *InternalTokenValidator {
	return &InternalTokenValidator{userService: userService, sharedSecret: sharedSecret}
}

func (h *InternalTokenValidator) Validate(c *gin.Context) {
	provided := c.GetHeader(internalSecretHeader)
	if !constantTimeEqual(provided, h.sharedSecret) {
		c.Status(http.StatusForbidden)
		return
	}

	authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
	if !strings.HasPrefix(authHeader, "Bearer ") {
		c.Status(http.StatusUnauthorized)
		return
	}
	token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	if token == "" {
		c.Status(http.StatusUnauthorized)
		return
	}

	user, err := h.userService.ValidateToken(c.Request.Context(), token)
	if err != nil || user == nil {
		c.Status(http.StatusUnauthorized)
		return
	}

	c.Header("X-Authenticated-User-ID", user.ID)
	c.Header("X-Authenticated-Email", user.Email)
	c.Header("X-Authenticated-System-Admin", strconv.FormatBool(user.IsSystemAdmin))
	c.Header("X-Authenticated-Knowledge-Officer", strconv.Itoa(user.RoleKnowledgeOfficer))
	c.Status(http.StatusNoContent)
}

func constantTimeEqual(left, right string) bool {
	if left == "" || right == "" || len(left) != len(right) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}
