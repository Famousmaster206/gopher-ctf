package handlers

import (
	goaway "github.com/TwiN/go-away"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/openlyfree/gopher-ctf/internal/models"
	"github.com/openlyfree/gopher-ctf/internal/shared"
	"github.com/openlyfree/gopher-ctf/ui/pages"
	"golang.org/x/crypto/bcrypt"
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
	if err := shared.DB.Where("username = ?", c.PostForm("username")).First(&user).Error; err != nil {
		c.String(200, "Invalid credentials")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(c.PostForm("password"))); err != nil {
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

func RegisterHandler(c *gin.Context) {
	if goaway.IsProfane(c.PostForm("username")) {
		c.String(200, "Invalid Username")
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(c.PostForm("password")), 14)
	if err != nil {
		c.String(200, "Error creating account")
		return
	}

	if err := shared.DB.Create(&models.User{Username: c.PostForm("username"), Password: string(hashedPassword)}).Error; err != nil {
		c.String(200, "Username already taken")
		return
	}
	c.Header("HX-Redirect", "/login")
	c.Status(200)
}

func ScoreboardHandler(c *gin.Context) {
	var users []models.User
	if err := shared.DB.Order("score desc").Order("id asc").Find(&users).Error; err != nil {
		c.String(500, "Database error")
		return
	}
	if c.GetHeader("HX-Request") == "true" {
		Render(c, 200, pages.Scoreboard(users))
	} else {
		Render(c, 200, pages.ScoreboardPage(users))
	}
}

func RegisterPageHandler(c *gin.Context) {
	if c.GetHeader("HX-Request") == "true" {
		Render(c, 200, pages.Signup())
	} else {
		Render(c, 200, pages.SignupPage())
	}
}

func LoginPageHandler(c *gin.Context) {
	if c.GetHeader("HX-Request") == "true" {
		Render(c, 200, pages.Login())
	} else {
		Render(c, 200, pages.LoginPage())
	}
}
