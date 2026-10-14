package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/openlyfree/gopher-ctf/internal/models"
	"github.com/openlyfree/gopher-ctf/internal/shared"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var errAccountChanged = errors.New("account changed")

// The confirmation token is bound to this admin cookie and the selected account.
func accountDeleteToken(c *gin.Context, userID uint) string {
	return accountActionToken(c, userID, "delete-account")
}

func accountActionToken(c *gin.Context, userID uint, action string) string {
	cookie, _ := c.Cookie("secret_candy_vault_access")
	mac := hmac.New(sha256.New, []byte(shared.Config.Key))
	mac.Write([]byte(action + ":" + strconv.FormatUint(uint64(userID), 10) + ":" + cookie))
	return hex.EncodeToString(mac.Sum(nil))
}

func accountDeleteTokens(c *gin.Context, users []models.User) map[uint]string {
	tokens := make(map[uint]string, len(users))
	for _, user := range users {
		tokens[user.ID] = accountDeleteToken(c, user.ID)
	}
	return tokens
}

func DeleteAccountHandler(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		c.String(400, "invalid account id")
		return
	}
	if !hmac.Equal([]byte(c.PostForm("delete_token")), []byte(accountDeleteToken(c, uint(id)))) {
		c.String(403, "refresh the admin page and try again")
		return
	}
	err = shared.DB.Transaction(func(tx *gorm.DB) error {
		var user models.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, id).Error; err != nil {
			return err
		}
		if user.Username != c.PostForm("username") {
			return errAccountChanged
		}
		if err := tx.Where("user_id = ?", id).Delete(&models.ChallengeCompletion{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", id).Delete(&models.SignupDraft{}).Error; err != nil {
			return err
		}
		return tx.Delete(&user).Error
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.String(404, "account not found")
		} else if errors.Is(err, errAccountChanged) {
			c.String(409, "account changed. refresh the admin page and try again")
		} else {
			_ = c.Error(err)
			c.String(500, "couldn't delete the account. try again")
		}
		return
	}
	c.Redirect(303, "/secret-candy-vault/")
}
