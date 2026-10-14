package handlers

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/openlyfree/gopher-ctf/internal/models"
	"github.com/openlyfree/gopher-ctf/internal/shared"
	"github.com/openlyfree/gopher-ctf/ui/pages"
	"gorm.io/gorm"
)

func renderAdminChallenges(c *gin.Context, status int, message string, edit *models.Challenge) {
	c.Header("Cache-Control", "no-store")
	var challenges []models.Challenge
	if err := shared.DB.Order("id asc").Find(&challenges).Error; err != nil {
		_ = c.Error(err)
		c.String(500, "couldn't load the challenges. try again")
		return
	}
	selected := edit
	if selected == nil && c.Query("id") != "" {
		id, err := strconv.ParseUint(c.Query("id"), 10, 32)
		if err != nil || id == 0 {
			c.String(400, "invalid challenge id")
			return
		}
		for i := range challenges {
			if uint64(challenges[i].ID) == id {
				selected = &challenges[i]
				break
			}
		}
		if selected == nil {
			c.String(404, "challenge not found")
			return
		}
	}
	if selected == nil && len(challenges) > 0 {
		selected = &challenges[0]
	}
	query := strings.TrimSpace(c.Query("q"))
	visible := challenges
	if query != "" {
		visible = make([]models.Challenge, 0, len(challenges))
		for _, challenge := range challenges {
			if strings.Contains(strings.ToLower(challenge.Title), strings.ToLower(query)) {
				visible = append(visible, challenge)
			}
		}
	}
	if c.GetHeader("HX-Request") == "true" && c.Request.Method == "GET" {
		Render(c, status, pages.AdminChallengeWorkspace(visible, selected, message, query))
	} else {
		Render(c, status, pages.AdminChallenges(visible, selected, message, query))
	}
}

func AdminChallengesHandler(c *gin.Context) {
	message := ""
	if c.Query("saved") == "1" {
		message = "challenge saved :D"
	}
	renderAdminChallenges(c, 200, message, nil)
}

func UpdateChallengeHandler(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		c.String(400, "invalid challenge id")
		return
	}
	var challenge models.Challenge
	if err = shared.DB.First(&challenge, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.String(404, "challenge not found")
		} else {
			_ = c.Error(err)
			c.String(500, "couldn't load the challenge. try again")
		}
		return
	}
	challenge.Title = strings.TrimSpace(c.PostForm("challenge_name"))
	challenge.Description = strings.TrimSpace(c.PostForm("challenge_description"))
	challenge.Flag = strings.TrimSpace(c.PostForm("challenge_flag"))
	points, err := strconv.Atoi(strings.TrimSpace(c.PostForm("challenge_points")))
	if err != nil || points < 0 {
		renderAdminChallenges(c, 400, "points need to be a whole number, zero or more", &challenge)
		return
	}
	challenge.Points = points
	if challenge.Title == "" || challenge.Description == "" || challenge.Flag == "" {
		renderAdminChallenges(c, 400, "fill in the name, description, points, and flag pls", &challenge)
		return
	}
	result := shared.DB.Model(&models.Challenge{}).Where("id = ?", id).Updates(map[string]interface{}{
		"title": challenge.Title, "description": challenge.Description, "points": challenge.Points, "flag": challenge.Flag,
	})
	if result.Error != nil {
		_ = c.Error(result.Error)
		renderAdminChallenges(c, 500, "couldn't save the challenge. try again", &challenge)
		return
	}
	if result.RowsAffected == 0 {
		c.String(404, "challenge not found")
		return
	}
	destination := "/admin/challenges?saved=1&id=" + strconv.FormatUint(id, 10)
	if query := strings.TrimSpace(c.Query("q")); query != "" {
		destination += "&q=" + url.QueryEscape(query)
	}
	c.Redirect(http.StatusSeeOther, destination)
}
