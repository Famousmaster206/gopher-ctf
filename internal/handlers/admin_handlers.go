package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/openlyfree/gopher-ctf/internal/models"
	"github.com/openlyfree/gopher-ctf/internal/shared"
	"github.com/openlyfree/gopher-ctf/ui/pages"
)

func AdminLoginHandler(c *gin.Context) {
	if c.PostForm("password") == shared.Config.Password {
		token, err := shared.EncryptAdminToken(shared.Config.Password)
		if err != nil {
			_ = c.Error(err)
			c.String(http.StatusInternalServerError, "Auth setup error")
			return
		}
		c.Status(http.StatusOK)
		c.SetCookie("secret_candy_vault_access", token, 3153600000, "/", "", false, true)
		c.Header("HX-Redirect", "/secret-candy-vault")
	} else {
		_ = c.Error(errors.New("incorrect password"))
		c.String(http.StatusOK, "Incorrect password")
	}
}

func adminUsers(c *gin.Context) ([]models.User, bool) {
	c.Header("Cache-Control", "no-store")
	var users []models.User
	if err := shared.DB.Select("id", "username", "score", "verification_code").Order("score desc").Order("id asc").Find(&users).Error; err != nil {
		_ = c.Error(err)
		c.String(500, "couldn't load the players. try again")
		return nil, false
	}
	return users, true
}
func AdminIndexHandler(c *gin.Context) {
	if users, ok := adminUsers(c); ok {
		Render(c, 200, pages.Admin(users))
	}
}
func AdminPlayersHandler(c *gin.Context) {
	if users, ok := adminUsers(c); ok {
		Render(c, 200, pages.AdminPlayers(users))
	}
}

func AdminLoginPageHandler(c *gin.Context) { Render(c, 200, pages.AdminLogin()) }

func ConfigReloadHandler(c *gin.Context) {
	shared.InitConfig()
	c.String(http.StatusOK, "Config reloaded")
}
