package handler

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"ocrshow/internal/middleware"
	"ocrshow/internal/store"
)

type loginBody struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginGuard struct {
	mu   sync.Mutex
	hits map[string]*loginHit
}

type loginHit struct {
	n    int
	seen time.Time
}

var logins = &loginGuard{hits: map[string]*loginHit{}}

func (a *API) Login(c *gin.Context) {
	ip := c.ClientIP()
	if !logins.allow(ip) {
		fail(c, http.StatusTooManyRequests, "尝试次数过多，请稍后再试")
		return
	}
	var body loginBody
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, "请求格式无效")
		return
	}
	user, err := a.store.Authenticate(body.Username, body.Password)
	if err != nil {
		if errors.Is(err, store.ErrInvalidLogin) {
			logins.fail(ip)
			fail(c, http.StatusUnauthorized, err.Error())
			return
		}
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	token, expires, err := a.store.CreateSession(user.ID)
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	logins.clear(ip)
	writeSessionCookie(c, token, expires)
	c.JSON(http.StatusOK, user)
}

func (a *API) Logout(c *gin.Context) {
	_ = a.store.DeleteSession(middleware.SessionToken(c))
	clearSessionCookie(c)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (a *API) Me(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		fail(c, http.StatusUnauthorized, "请先登录")
		return
	}
	c.JSON(http.StatusOK, user)
}

func writeSessionCookie(c *gin.Context, token string, expires time.Time) {
	http.SetCookie(c.Writer, sessionCookie(c, token, int(time.Until(expires).Seconds())))
}

func clearSessionCookie(c *gin.Context) {
	http.SetCookie(c.Writer, sessionCookie(c, "", -1))
}

func sessionCookie(c *gin.Context, value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     middleware.SessionCookie,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   cookieSecure(c),
		SameSite: http.SameSiteLaxMode,
	}
}

func cookieSecure(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}
	return strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
}

func (g *loginGuard) allow(ip string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.prune()
	hit := g.hits[ip]
	if hit == nil {
		return true
	}
	if time.Since(hit.seen) > 10*time.Minute {
		delete(g.hits, ip)
		return true
	}
	return hit.n < 8
}

func (g *loginGuard) fail(ip string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	hit := g.hits[ip]
	if hit == nil || time.Since(hit.seen) > 10*time.Minute {
		g.hits[ip] = &loginHit{n: 1, seen: time.Now()}
		return
	}
	hit.n++
	hit.seen = time.Now()
}

func (g *loginGuard) clear(ip string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.hits, ip)
}

func (g *loginGuard) prune() {
	for ip, hit := range g.hits {
		if time.Since(hit.seen) > 10*time.Minute {
			delete(g.hits, ip)
		}
	}
}
