// Package server — Dashboard handlers (page rendering, login, logout).
package server

import (
	"crypto/subtle"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// handleDashboard renders the main dashboard page.
func (s *Server) handleDashboard(c *gin.Context) {
	glmAccounts := s.accounts.List(ProviderGLM)
	mimoAccounts := s.accounts.List(ProviderMimo)

	c.HTML(http.StatusOK, "dashboard.html", gin.H{
		"Version":     s.cfg.Version,
		"GatewayAddr": s.cfg.ListenAddr,
		"StartTime":   s.startTime.Format(time.RFC3339),
		"GLMCount":    len(glmAccounts),
		"MimoCount":   len(mimoAccounts),
		"GLMActive":   countActive(glmAccounts),
		"MimoActive":  countActive(mimoAccounts),
	})
}

func countActive(accounts []*Account) int {
	n := 0
	for _, a := range accounts {
		if a.Enabled.Load() {
			n++
		}
	}
	return n
}

// handleLoginPage renders the login page (if DASHBOARD_TOKEN is set).
func (s *Server) handleLoginPage(c *gin.Context) {
	if s.cfg.DashboardToken == "" {
		c.Redirect(http.StatusFound, "/")
		return
	}
	c.HTML(http.StatusOK, "login.html", gin.H{
		"Error": c.Query("error"),
	})
}

// handleLoginSubmit processes login form submissions.
func (s *Server) handleLoginSubmit(c *gin.Context) {
	token := c.PostForm("token")
	if subtle.ConstantTimeCompare([]byte(token), []byte(s.cfg.DashboardToken)) != 1 {
		c.Redirect(http.StatusFound, "/login?error=invalid")
		return
	}

	// SameSite=Lax for CSRF protection; Secure based on ENV
	c.SetSameSite(http.SameSiteLaxMode)
	isProd := os.Getenv("ENV") == "production"
	c.SetCookie("dashboard_token", token, 86400, "/", "", isProd, true)
	c.Redirect(http.StatusFound, "/")
}

// handleLogout clears the dashboard cookie.
func (s *Server) handleLogout(c *gin.Context) {
	c.SetCookie("dashboard_token", "", -1, "/", "", false, true)
	c.Redirect(http.StatusFound, "/login")
}

// DashboardAuthMiddleware protects dashboard routes.
// If token is empty, only localhost is allowed.
// If token is set, checks cookie or Authorization header (timing-safe).
func DashboardAuthMiddleware(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if token == "" {
			// No token set — only allow localhost
			ip := c.ClientIP()
			if ip != "127.0.0.1" && ip != "::1" {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"error": gin.H{
						"type":    "dashboard_locked",
						"message": "Dashboard only accessible from localhost. Set DASHBOARD_TOKEN to enable remote access.",
					},
				})
				return
			}
			c.Next()
			return
		}

		// Token set — check cookie or Authorization header
		authHeader := c.GetHeader("Authorization")
		cookie, _ := c.Cookie("dashboard_token")
		provided := ""
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			provided = authHeader[7:]
		} else if cookie != "" {
			provided = cookie
		}

		if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			// For API routes (/api/...), return a JSON 401 so the JS client
			// can handle auth errors programmatically. For page routes,
			// redirect to the login page.
			if strings.HasPrefix(c.Request.URL.Path, "/api/") {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"error": gin.H{
						"type":    "dashboard_auth_required",
						"message": "Dashboard authentication required. Please login.",
					},
				})
				return
			}
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}
		c.Next()
	}
}
