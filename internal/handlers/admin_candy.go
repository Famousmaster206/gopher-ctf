package handlers

import (
	"crypto/hmac"
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/openlyfree/gopher-ctf/internal/models"
	"github.com/openlyfree/gopher-ctf/internal/shared"
	"github.com/openlyfree/gopher-ctf/ui/pages"
	"gorm.io/gorm"
)

func candyTokens(c *gin.Context, users []models.User) map[uint]string {
	tokens := make(map[uint]string, len(users))
	for _, user := range users {
		tokens[user.ID] = accountActionToken(c, user.ID, "give-candy:"+strconv.Itoa(user.CandiesGiven))
	}
	return tokens
}

func GiveCandyHandler(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		c.String(400, "invalid account id")
		return
	}
	given, err := strconv.Atoi(c.PostForm("candies_given"))
	if err != nil || given < 0 {
		c.String(400, "invalid candy count")
		return
	}
	expected := accountActionToken(c, uint(id), "give-candy:"+strconv.Itoa(given))
	if !hmac.Equal([]byte(c.PostForm("candy_token")), []byte(expected)) {
		c.String(403, "refresh the admin page and try again")
		return
	}
	// The expected count makes retries and simultaneous clicks redeem at most once.
	result := shared.DB.Model(&models.User{}).Where("id = ? AND candies_given = ? AND score / ? > candies_given", id, given, shared.CandyRate()).Update("candies_given", gorm.Expr("candies_given + 1"))
	if result.Error != nil {
		_ = c.Error(result.Error)
		c.String(500, "couldn't update candy owed. try again")
		return
	}
	message := ""
	if result.RowsAffected == 0 {
		var user models.User
		if err := shared.DB.Select("id").First(&user, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.String(404, "account not found")
			} else {
				_ = c.Error(err)
				c.String(500, "couldn't load the account")
			}
			return
		}
		message = "candy balance changed. check candy owed before handing any over."
	}
	if c.GetHeader("HX-Request") == "true" {
		if users, ok := adminUsers(c); ok {
			Render(c, 200, pages.AdminPlayers(users, accountDeleteTokens(c, users), candyTokens(c, users), message))
		}
	} else if message != "" {
		c.String(409, message)
	} else {
		c.Redirect(303, "/secret-candy-vault/")
	}
}
