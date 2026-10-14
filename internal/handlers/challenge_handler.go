package handlers

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/a-h/templ"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/openlyfree/gopher-ctf/internal/models"
	"github.com/openlyfree/gopher-ctf/internal/shared"
	"github.com/openlyfree/gopher-ctf/ui/components"
	"github.com/openlyfree/gopher-ctf/ui/pages"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Each account may submit one flag every 20 seconds, across all challenges.
func SubmitFlagHandler(c *gin.Context) {
	userIDVal := sessions.Default(c).Get("user_id")
	if userIDVal == nil {
		c.String(401, "Login required")
		return
	}
	userID, ok := userIDVal.(uint)
	if !ok || userID == 0 {
		c.String(401, "Session error")
		return
	}
	challengeID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || challengeID == 0 {
		c.String(400, "Invalid challenge ID")
		return
	}
	flag := strings.TrimSpace(c.PostForm("flag"))
	if flag == "" {
		c.String(400, "Flag is required")
		return
	}
	var challenge models.Challenge
	if err := shared.DB.First(&challenge, challengeID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.String(404, "Challenge not found")
		} else {
			_ = c.Error(err)
			c.String(500, "Database error")
		}
		return
	}
	// An atomic database update enforces the cooldown even across server instances.
	attempt := shared.DB.Model(&models.User{}).Where("id = ? AND (last_flag_attempt IS NULL OR last_flag_attempt <= CURRENT_TIMESTAMP - INTERVAL '20 seconds')", userID).Update("last_flag_attempt", gorm.Expr("CURRENT_TIMESTAMP"))
	if attempt.Error != nil {
		_ = c.Error(attempt.Error)
		c.String(500, "Database error")
		return
	}
	if attempt.RowsAffected == 0 {
		var user models.User
		if err := shared.DB.First(&user, userID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.String(401, "Login required")
			} else {
				_ = c.Error(err)
				c.String(500, "Database error")
			}
			return
		}
		c.Header("Retry-After", "20")
		c.String(429, "Please wait 20 seconds between submissions")
		return
	}
	if strings.TrimSpace(challenge.Flag) == "" || challenge.Points < 0 {
		c.String(500, "Challenge is not configured correctly")
		return
	}
	if flag != strings.TrimSpace(challenge.Flag) {
		c.String(422, "Incorrect flag")
		return
	}
	alreadyCompleted, err := awardChallenge(shared.DB, userID, uint(challengeID), challenge.Points)
	if err != nil {
		_ = c.Error(err)
		c.String(500, "Database error")
		return
	}
	if alreadyCompleted {
		if c.GetHeader("HX-Request") == "true" {
			Render(c, 409, components.CompletionFeedback("Already completed!"))
		} else {
			c.String(409, "Already completed!")
		}
		return
	}
	if c.GetHeader("HX-Request") == "true" {
		Render(c, 200, components.CompletionFeedback(fmt.Sprintf("Correct! +%d points", challenge.Points)))
	} else {
		c.String(200, "Correct! +%d points", challenge.Points)
	}
}

var errInvalidChallengeID = errors.New("invalid challenge ID")

func GetChallenge(id string) (models.Challenge, error) {
	tempID, err := strconv.ParseUint(id, 10, 32)
	if err != nil || tempID == 0 {
		return models.Challenge{}, errInvalidChallengeID
	}
	var challenge models.Challenge
	if err := shared.DB.First(&challenge, tempID).Error; err != nil {
		return models.Challenge{}, err
	}
	return challenge, nil
}

func ChallengeIndividualHandler(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	challenge, err := GetChallenge(c.Param("id"))
	if err != nil {
		_ = c.Error(err)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.String(404, "Challenge not found")
		} else if errors.Is(err, errInvalidChallengeID) {
			c.String(400, "Invalid challenge ID")
		} else {
			c.String(500, "Database error")
		}
		return
	}
	completed, err := completedChallenges(c)
	if err != nil {
		_ = c.Error(err)
		c.String(500, "Database error")
		return
	}
	if c.GetHeader("HX-Request") == "true" {
		Render(c, 200, components.ChallengeCard(challenge, completed[challenge.ID]))
	} else {
		Render(c, 200, pages.ChallengeDetail(challenge, completed[challenge.ID]))
	}
}

func Render(c *gin.Context, status int, cmp templ.Component) {
	c.Status(status)
	_ = cmp.Render(c.Request.Context(), c.Writer)
}

func ChallengeIndexHandler(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	var challenges []models.Challenge
	if err := shared.DB.Find(&challenges).Error; err != nil {
		_ = c.Error(err)
		c.String(500, "Database error")
		return
	}
	completed, err := completedChallenges(c)
	if err != nil {
		_ = c.Error(err)
		c.String(500, "Database error")
		return
	}
	hideCompleted := c.Query("hide_completed") == "1"
	if hideCompleted {
		visible := make([]models.Challenge, 0, len(challenges))
		for _, challenge := range challenges {
			if !completed[challenge.ID] {
				visible = append(visible, challenge)
			}
		}
		challenges = visible
	}
	if c.GetHeader("HX-Request") == "true" {
		Render(c, 200, pages.Challenges(challenges, completed, hideCompleted))
	} else {
		Render(c, 200, pages.ChallengesPage(challenges, completed, hideCompleted))
	}
}

func CreateChallengeHandler(c *gin.Context) {
	name := strings.TrimSpace(c.PostForm("challenge_name"))
	description := strings.TrimSpace(c.PostForm("challenge_description"))
	pointsStr := strings.TrimSpace(c.PostForm("challenge_points"))
	flag := strings.TrimSpace(c.PostForm("challenge_flag"))

	if name == "" || description == "" || pointsStr == "" || flag == "" {
		c.String(400, "All fields are required")
		return
	}

	points, err := strconv.Atoi(pointsStr)
	if err != nil || points < 0 {
		c.String(400, "Points must be a non-negative integer")
		return
	}

	challenge := models.Challenge{
		Title:       name,
		Description: description,
		Points:      points,
		Flag:        flag,
	}

	if err := shared.DB.Create(&challenge).Error; err != nil {
		_ = c.Error(err)
		c.String(500, "Database error")
		return
	}

	file, err := c.FormFile("challenge_files")
	if err == nil {
		challengeDir := filepath.Join("ctf", "challenges", fmt.Sprintf("%d", challenge.ID))
		if err := os.MkdirAll(challengeDir, 0o755); err != nil {
			_ = c.Error(err)
			c.String(500, "Challenge created but failed to create directory")
			return
		}
		dst := filepath.Join(challengeDir, file.Filename)
		if err := c.SaveUploadedFile(file, dst); err != nil {
			_ = c.Error(err)
			c.String(500, "Challenge created but failed to save file")
			return
		}
	}

	c.String(200, "Challenge '%s' created! (%d pts)", name, points)
}

// The unique completion and score increment must commit together.
func awardChallenge(db *gorm.DB, userID, challengeID uint, points int) (bool, error) {
	alreadyCompleted := false
	err := db.Transaction(func(tx *gorm.DB) error {
		result := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}, {Name: "challenge_id"}}, DoNothing: true}).Create(&models.ChallengeCompletion{UserID: userID, ChallengeID: challengeID})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			alreadyCompleted = true
			return nil
		}
		result = tx.Model(&models.User{}).Where("id = ?", userID).Update("score", gorm.Expr("score + ?", points))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("player disappeared during score update")
		}
		return nil
	})
	return alreadyCompleted, err
}

// One query loads only this player's completions for the list or detail page.
func completedChallenges(c *gin.Context) (map[uint]bool, error) {
	completed := make(map[uint]bool)
	userID, ok := sessions.Default(c).Get("user_id").(uint)
	if !ok || userID == 0 {
		return completed, nil
	}
	var ids []uint
	if err := shared.DB.Model(&models.ChallengeCompletion{}).Where("user_id = ?", userID).Pluck("challenge_id", &ids).Error; err != nil {
		return nil, err
	}
	for _, id := range ids {
		completed[id] = true
	}
	return completed, nil
}
