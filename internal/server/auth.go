package server

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"icloud-hme/internal/token"
)

const ctxTokenKey = "auth_token"

// authMiddleware 校验请求鉴权头。
//
// 支持两种鉴权头(任选其一):
//   - Authorization: Bearer <secret>
//   - X-API-Key: <secret>
func (s *Server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		secret := extractSecret(c)
		if secret == "" {
			fail(c, http.StatusUnauthorized, "missing token: 请提供 Authorization: Bearer <token> 或 X-API-Key")
			c.Abort()
			return
		}
		tok := s.tokens.FindBySecret(secret)
		if tok == nil {
			fail(c, http.StatusUnauthorized, "invalid token")
			c.Abort()
			return
		}
		s.tokens.Touch(tok.ID)
		c.Set(ctxTokenKey, tok)
		c.Next()
	}
}

// requireAdmin 要求当前请求 token 是 admin。
func requireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		tok := currentToken(c)
		if tok == nil || tok.Role != token.RoleAdmin {
			fail(c, http.StatusForbidden, "admin only")
			c.Abort()
			return
		}
		c.Next()
	}
}

func extractSecret(c *gin.Context) string {
	if v := strings.TrimSpace(c.GetHeader("X-API-Key")); v != "" {
		return v
	}
	auth := strings.TrimSpace(c.GetHeader("Authorization"))
	if auth == "" {
		return ""
	}
	if lower := strings.ToLower(auth); strings.HasPrefix(lower, "bearer ") {
		return strings.TrimSpace(auth[len("bearer "):])
	}
	return ""
}

func currentToken(c *gin.Context) *token.Token {
	v, ok := c.Get(ctxTokenKey)
	if !ok {
		return nil
	}
	t, _ := v.(*token.Token)
	return t
}

func isAdmin(c *gin.Context) bool {
	t := currentToken(c)
	return t != nil && t.Role == token.RoleAdmin
}
