package internal

import (
	"context"
	"log"
	"net/http"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/openlyfree/gopher-ctf/internal/shared"
)

func UserLoader() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.Default(c)
		session.Options(sessions.Options{
			Path: "/", MaxAge: 86400 * 30, HttpOnly: true, SameSite: http.SameSiteLaxMode,
			Secure: c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https",
		})
		if val := session.Get("username"); val != nil {
			c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), "username", val))
		}
		c.Next()
	}
}

func AdminAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie("secret_candy_vault_access")
		if err != nil {
			_ = c.Error(err)
			c.AbortWithStatus(401)
			return
		}
		decrypted, err := shared.DecryptAdminToken(token)
		if err != nil {
			c.AbortWithStatus(401)
			return
		}
		if decrypted != shared.Config.Password {
			c.AbortWithStatus(403)
			return
		}
		c.Next()
	}
}

func NoCache() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		c.Next()
	}
}

func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Errors != nil {
			for i := range c.Errors {
				log.Println("[ERROR] ", c.Request.URL, " ", c.ClientIP(), c.Errors[i].Err)
			}
		}
	}
}
