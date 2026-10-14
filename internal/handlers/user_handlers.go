package handlers

import (
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/openlyfree/gopher-ctf/internal/models"
	"github.com/openlyfree/gopher-ctf/internal/shared"
	"github.com/openlyfree/gopher-ctf/ui/pages"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func LogoutHandler(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	if err := session.Save(); err != nil {
		c.String(200, "Session error")
		return
	}
	c.Header("HX-Redirect", "/")
	c.Status(200)
}

func LoginHandler(c *gin.Context) {
	var user models.User
	if err := shared.DB.Where("username = ?", strings.TrimSpace(c.PostForm("username"))).First(&user).Error; err != nil {
		c.String(200, "Invalid credentials")
		return
	}
	attempt := shared.DB.Model(&models.User{}).Where("id = ? AND (last_login_attempt IS NULL OR last_login_attempt <= CURRENT_TIMESTAMP - INTERVAL '2 seconds')", user.ID).Update("last_login_attempt", gorm.Expr("CURRENT_TIMESTAMP"))
	if attempt.Error != nil {
		_ = c.Error(attempt.Error)
		c.String(200, "couldn't log in. try again")
		return
	}
	if attempt.RowsAffected == 0 {
		c.String(200, "wait 2 seconds, then try your login code again")
		return
	}
	code := strings.TrimSpace(c.PostForm("login_code"))
	// Older accounts can still use their existing password.
	if code == "" {
		code = c.PostForm("password")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(code)); err != nil {
		c.String(200, "Invalid credentials")
		return
	}

	session := sessions.Default(c)
	session.Set("user_id", user.ID)
	session.Set("username", user.Username)
	if err := session.Save(); err != nil {
		c.String(200, "Session error")

		return
	}
	c.Header("HX-Redirect", "/")
	c.Status(200)
}

func ScoreboardHandler(c *gin.Context) {
	var users []models.User
	if err := shared.DB.Order("score desc").Order("id asc").Find(&users).Error; err != nil {
		c.String(500, "Database error")
		return
	}
	if c.GetHeader("HX-Request") == "true" {
		RenderPageFragment(c, "Scoreboard", pages.Scoreboard(users))
	} else {
		Render(c, 200, pages.ScoreboardPage(users))
	}
}

func LoginPageHandler(c *gin.Context) {
	if c.GetHeader("HX-Request") == "true" {
		RenderPageFragment(c, "Log in", pages.Login())
	} else {
		Render(c, 200, pages.LoginPage())
	}
}
