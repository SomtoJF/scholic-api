package middleware

import (
	"net/http"
	"strings"

	"scholic/internal/utils/jwt"

	"github.com/gin-gonic/gin"
)

// AuthMiddleware requires authentication via cookie or Bearer token.
// Used for endpoints that must have an authenticated user.
//
// Supports:
//   - Cookie: auth_token (used by the web client)
//   - Header: Authorization: Bearer <token> (used by the browser extension)
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		var tokenString string

		// Try Bearer token first (browser extension)
		authHeader := c.GetHeader("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenString = strings.TrimPrefix(authHeader, "Bearer ")
		}

		// Fall back to cookie (web client)
		if tokenString == "" {
			tokenString = c.Query("token")
		}

		if tokenString == "" {
			var err error
			tokenString, err = c.Cookie("auth_token")
			if err != nil || tokenString == "" {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
				c.Abort()
				return
			}
		}

		userID, err := jwt.ValidateToken(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}

		c.Set("userID", userID)
		c.Next()
	}
}
