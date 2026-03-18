package middleware

import (
	"net/http"
	"strings"

	"github.com/AlexMeiko/guchat/internal/model"
	"github.com/AlexMeiko/guchat/internal/service"
	"github.com/gin-gonic/gin"
)

const CurrentUserKey = "currentUser"

func Auth(jwtService *service.JWTService) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, model.ErrorResponse{
				Error: "missing Authorization header",
			})
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" || parts[1] == "" {
			c.JSON(http.StatusUnauthorized, model.ErrorResponse{
				Error: "invalid Authorization header",
			})
			c.Abort()
			return
		}

		user, err := jwtService.ParseAccessToken(parts[1])
		if err != nil {
			c.JSON(http.StatusUnauthorized, model.ErrorResponse{
				Error: "invalid access token",
			})
			c.Abort()
			return
		}

		c.Set(CurrentUserKey, user)
		c.Next()
	}
}
