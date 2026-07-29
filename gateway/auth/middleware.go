// Package auth — gateway authentication middleware.
package auth

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// GatewayAuthMiddleware validates the Authorization header against
// the configured GATEWAY_TOKEN using timing-safe comparison.
// If token is empty, auth is disabled.
func GatewayAuthMiddleware(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if token == "" {
			c.Next()
			return
		}

		authHeader := c.GetHeader("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"type":    "authentication_error",
					"message": "Missing or invalid Authorization header. Expected: Bearer <token>",
				},
			})
			return
		}

		provided := strings.TrimPrefix(authHeader, "Bearer ")
		// Use constant-time comparison to prevent timing attacks
		if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"type":    "authentication_error",
					"message": "Invalid gateway token",
				},
			})
			return
		}

		c.Next()
	}
}
