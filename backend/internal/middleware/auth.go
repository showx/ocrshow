package middleware

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"ocrshow/internal/store"
)

const (
	SessionCookie = "ocrshow_session"
	ContextUser   = "auth_user"
)

func RequireAuth(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, err := userFromRequest(c, st)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
			return
		}
		c.Set(ContextUser, user)
		c.Next()
	}
}

func OptionalAuth(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		if user, err := userFromRequest(c, st); err == nil {
			c.Set(ContextUser, user)
		}
		c.Next()
	}
}

func CurrentUser(c *gin.Context) *store.User {
	v, ok := c.Get(ContextUser)
	if !ok {
		return nil
	}
	user, _ := v.(*store.User)
	return user
}

func SessionToken(c *gin.Context) string {
	if token, err := c.Cookie(SessionCookie); err == nil && strings.TrimSpace(token) != "" {
		return strings.TrimSpace(token)
	}
	auth := strings.TrimSpace(c.GetHeader("Authorization"))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	return ""
}

func userFromRequest(c *gin.Context, st *store.Store) (*store.User, error) {
	token := SessionToken(c)
	if token == "" {
		return nil, sql.ErrNoRows
	}
	user, err := st.UserBySession(token)
	if err != nil {
		return nil, err
	}
	return user, nil
}
