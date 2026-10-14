package handlers

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/openlyfree/gopher-ctf/internal/models"
	"github.com/openlyfree/gopher-ctf/internal/shared"
	"github.com/openlyfree/gopher-ctf/ui/pages"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const maxRerolls = 3

var errSignupFinished = errors.New("account already made. log in with your code :D")
var errSignupMissing = errors.New("open the sign up page first")
var nameWords = []string{"Cyber", "Crypto", "Cipher", "Secure", "Stealth", "Shadow", "Binary", "Digital", "Zero", "Root", "Kernel", "Network", "Packet", "Proxy", "Quantum", "Encrypted", "Hardened", "Hidden", "Silent", "Secret", "Virtual", "Firewall", "Hash", "Hex"}
var securityWords = []string{"Hacker", "Analyst", "Defender", "Hunter", "Sentinel", "Guardian", "Tracer", "Sleuth", "Decoder", "Breaker", "Warden", "Scout", "Agent", "Auditor", "Researcher", "Tester", "Watcher", "Tracker", "Cracker", "Operator", "Detective", "Shield", "Sentry", "Investigator"}

func randomNumber(limit int64) (int64, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(limit))
	if err != nil {
		return 0, err
	}
	return n.Int64(), nil
}

func randomUsername() (string, error) {
	a, err := randomNumber(int64(len(nameWords)))
	if err != nil {
		return "", err
	}
	b, err := randomNumber(int64(len(securityWords)))
	if err != nil {
		return "", err
	}
	n, err := randomNumber(1000)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%s%03d", nameWords[a], securityWords[b], n), nil
}

func randomCode() (string, error) {
	n, err := randomNumber(900000)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n+100000), nil
}

func availableUsername(db *gorm.DB, previous string) (string, error) {
	for i := 0; i < 50; i++ {
		name, err := randomUsername()
		if err != nil {
			return "", err
		}
		if name == previous {
			continue
		}
		var count int64
		if err = db.Unscoped().Model(&models.User{}).Where("username = ?", name).Count(&count).Error; err != nil {
			return "", err
		}
		if count == 0 {
			if err = db.Unscoped().Model(&models.SignupDraft{}).Where("username = ?", name).Count(&count).Error; err != nil {
				return "", err
			}
			if count == 0 {
				return name, nil
			}
		}
	}
	return "", errors.New("could not find a free username")
}

func signupToken(c *gin.Context) string {
	token, _ := sessions.Default(c).Get("signup_token").(string)
	return token
}

func RegisterPageHandler(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if sessions.Default(c).Get("user_id") != nil {
		redirectAccount(c, "/account")
		return
	}
	var draft models.SignupDraft
	err := shared.DB.Where("token = ?", signupToken(c)).First(&draft).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		_ = c.Error(err)
		c.String(500, "couldn't load your name. try again")
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		for i := 0; i < 50; i++ {
			name, err := availableUsername(shared.DB, "")
			if err != nil {
				_ = c.Error(err)
				c.String(500, "couldn't make a name. try again")
				return
			}
			draft = models.SignupDraft{Token: shared.RandString(32), Username: name}
			result := shared.DB.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "username"}}, DoNothing: true}).Create(&draft)
			if result.Error != nil {
				_ = c.Error(result.Error)
				c.String(500, "couldn't make a name. try again")
				return
			}
			if result.RowsAffected == 1 {
				break
			}
		}
		if draft.ID == 0 {
			c.String(500, "couldn't make a name. try again")
			return
		}

		session := sessions.Default(c)
		session.Set("signup_token", draft.Token)
		if err = session.Save(); err != nil {
			_ = c.Error(err)
			c.String(500, "couldn't save your name. try again")
			return
		}
	}
	if draft.UserID != nil {
		redirectAccount(c, "/login")
		return
	}
	if c.GetHeader("HX-Request") == "true" {
		RenderPageFragment(c, "Join", pages.Signup(draft.Username, draft.Token, maxRerolls-draft.Rerolls))
	} else {
		Render(c, 200, pages.SignupPage(draft.Username, draft.Token, maxRerolls-draft.Rerolls))
	}
}

// Row locking makes both rerolls and account creation safe against duplicate clicks.
func changeSignup(db *gorm.DB, token string, fn func(*gorm.DB, *models.SignupDraft) error) error {
	if token == "" {
		return errSignupMissing
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var draft models.SignupDraft
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("token = ?", token).First(&draft).Error; err != nil {
			return err
		}
		if draft.UserID != nil {
			return errSignupFinished
		}
		return fn(tx, &draft)
	})
}

func RerollUsernameHandler(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	token := signupToken(c)
	if token == "" || c.PostForm("signup_token") != token {
		c.String(200, "open the sign up page first")
		return
	}
	var draft models.SignupDraft
	err := changeSignup(shared.DB, token, func(tx *gorm.DB, d *models.SignupDraft) error {
		if d.Rerolls < maxRerolls {
			name, err := availableUsername(tx, d.Username)
			if err != nil {
				return err
			}
			d.Username = name
			d.Rerolls++
			if err = tx.Save(d).Error; err != nil {
				return err
			}
		}
		draft = *d
		return nil
	})
	if err != nil {
		signupError(c, err)
		return
	}
	RenderPageFragment(c, "Join", pages.Signup(draft.Username, draft.Token, maxRerolls-draft.Rerolls))
}

func signupError(c *gin.Context, err error) {
	_ = c.Error(err)
	if errors.Is(err, errSignupFinished) {
		c.String(200, errSignupFinished.Error())
		return
	}
	c.String(200, "couldn't make your account. refresh and try again")
}

func RegisterHandler(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	token := signupToken(c)
	if token == "" || c.PostForm("signup_token") != token {
		c.String(200, "open the sign up page first")
		return
	}
	loginCode, err := randomCode()
	if err != nil {
		signupError(c, err)
		return
	}
	pointsCode, err := randomCode()
	if err != nil {
		signupError(c, err)
		return
	}
	for pointsCode == loginCode {
		pointsCode, err = randomCode()
		if err != nil {
			signupError(c, err)
			return
		}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(loginCode), 12)
	if err != nil {
		signupError(c, err)
		return
	}
	var user models.User
	err = changeSignup(shared.DB, token, func(tx *gorm.DB, d *models.SignupDraft) error {
		user = models.User{Username: d.Username, Password: string(hash), VerificationCode: &pointsCode}
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		d.UserID = &user.ID
		return tx.Save(d).Error
	})
	if err != nil {
		signupError(c, err)
		return
	}
	session := sessions.Default(c)
	session.Clear()
	session.Set("user_id", user.ID)
	session.Set("username", user.Username)
	// Always show the new credentials, even if the automatic login cookie fails.
	if err = session.Save(); err != nil {
		_ = c.Error(err)
	}
	RenderPageFragment(c, "My codes", pages.AccountCreated(user.Username, loginCode, pointsCode))
}

func AccountHandler(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	userID, ok := sessions.Default(c).Get("user_id").(uint)
	if !ok {
		redirectAccount(c, "/login")
		return
	}
	var user models.User
	if err := shared.DB.First(&user, userID).Error; err != nil {
		_ = c.Error(err)
		c.String(500, "couldn't load your codes. try again")
		return
	}
	if c.GetHeader("HX-Request") == "true" {
		RenderPageFragment(c, "My codes", pages.Account(user))
	} else {
		Render(c, 200, pages.AccountPage(user))
	}
}

func redirectAccount(c *gin.Context, path string) {
	if c.GetHeader("HX-Request") == "true" {
		c.Header("HX-Redirect", path)
		c.Status(200)
	} else {
		c.Redirect(303, path)
	}
}
