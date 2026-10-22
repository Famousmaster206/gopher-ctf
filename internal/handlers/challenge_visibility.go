package handlers

import (
	"crypto/hmac"
	"github.com/gin-gonic/gin"
	"github.com/openlyfree/gopher-ctf/internal/models"
	"github.com/openlyfree/gopher-ctf/internal/shared"
	"github.com/openlyfree/gopher-ctf/ui/pages"
	"gorm.io/gorm/clause"
)

func challengesAvailable(c *gin.Context) bool {
	c.Header("Cache-Control", "private, no-store")
	public, err := shared.ChallengesPublic()
	if err != nil {
		_ = c.Error(err)
		c.String(500, "Couldn't load challenge availability. Please try again.")
		return false
	}
	if public {
		return true
	}
	if c.Request.Method == "POST" {
		c.String(403, "Challenges coming when the event starts.")
		return false
	}
	if c.GetHeader("HX-Request") == "true" {
		RenderPageFragment(c, "Challenges", pages.ChallengesComing())
	} else {
		Render(c, 200, pages.ChallengesComingPage())
	}
	return false
}

func ChallengeVisibilityHandler(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !hmac.Equal([]byte(c.PostForm("visibility_token")), []byte(accountActionToken(c, 0, "challenge-visibility"))) {
		c.String(403, "Refresh the admin page and try again")
		return
	}
	value := c.PostForm("challenges_public")
	if value != "true" && value != "false" {
		c.String(400, "Invalid challenge visibility")
		return
	}
	settings := models.EventSettings{ID: 1, ChallengesPublic: value == "true"}
	if err := shared.DB.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.AssignmentColumns([]string{"challenges_public"})}).Create(&settings).Error; err != nil {
		_ = c.Error(err)
		c.String(500, "Couldn't save challenge visibility. Please try again.")
		return
	}
	c.Redirect(303, "/secret-candy-vault/")
}
