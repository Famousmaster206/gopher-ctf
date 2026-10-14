package internal

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/openlyfree/gopher-ctf/internal/models"
	"github.com/openlyfree/gopher-ctf/internal/shared"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestAccountDeletionRequiresAdmin(t *testing.T) {
	r := accountRouter()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/admin/accounts/1/delete", nil))
	if w.Code != 401 {
		t.Fatalf("unauthenticated deletion returned %d", w.Code)
	}
}

func TestPostgresAccountDeletion(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL for deletion integration tests")
	}
	namespace := fmt.Sprintf("delete_test_%d", time.Now().UnixNano())
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: namespace + "."}})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err = db.Exec("CREATE SCHEMA " + namespace).Error; err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP SCHEMA " + namespace + " CASCADE")
	if err = db.AutoMigrate(&models.User{}, &models.SignupDraft{}, &models.ChallengeCompletion{}); err != nil {
		t.Fatal(err)
	}
	oldDB, oldConfig := shared.DB, shared.Config
	shared.DB = db
	shared.Config.Key = "delete-key"
	shared.Config.Password = "delete-password"
	defer func() { shared.DB = oldDB; shared.Config = oldConfig }()
	hash, err := bcrypt.GenerateFromPassword([]byte("123456"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	user := models.User{Username: "DeleteMe", Password: string(hash), Score: 123}
	other := models.User{Username: "KeepMe", Password: string(hash), Score: 456}
	for _, u := range []*models.User{&user, &other} {
		if err = db.Create(u).Error; err != nil {
			t.Fatal(err)
		}
	}
	draft := models.SignupDraft{Token: "draft-token", Username: user.Username, UserID: &user.ID}
	if err = db.Create(&draft).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []uint{user.ID, other.ID} {
		if err = db.Create(&models.ChallengeCompletion{UserID: id, ChallengeID: 1}).Error; err != nil {
			t.Fatal(err)
		}
	}
	adminToken, err := shared.EncryptAdminToken(shared.Config.Password)
	if err != nil {
		t.Fatal(err)
	}
	r := accountRouter()
	r.POST("/test-session", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("user_id", user.ID)
		session.Set("username", user.Username)
		if err := session.Save(); err != nil {
			t.Error(err)
		}
	})
	request := func(method, path string, values url.Values, admin bool, oldSession *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(values.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("HX-Request", "true")
		if admin {
			req.AddCookie(&http.Cookie{Name: "secret_candy_vault_access", Value: adminToken})
		}
		if oldSession != nil {
			req.AddCookie(oldSession)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	sessionResponse := request("POST", "/test-session", nil, false, nil)
	var oldSession *http.Cookie
	for _, cookie := range sessionResponse.Result().Cookies() {
		if cookie.Name == "test-session" {
			oldSession = cookie
		}
	}
	if oldSession == nil {
		t.Fatal("failed to create old login cookie")
	}
	table := request("GET", "/secret-candy-vault/players", nil, true, nil).Body.String()
	tokenFor := func(id uint) string {
		t.Helper()
		pattern := fmt.Sprintf(`(?s)action="/admin/accounts/%d/delete".*?name="delete_token" value="([a-f0-9]+)"`, id)
		match := regexp.MustCompile(pattern).FindStringSubmatch(table)
		if len(match) != 2 {
			t.Fatal("missing account delete form")
		}
		return match[1]
	}
	path := fmt.Sprintf("/admin/accounts/%d/delete", user.ID)
	values := url.Values{"username": {user.Username}}
	if w := request("POST", path, values, true, nil); w.Code != 403 {
		t.Fatal("missing confirmation token was accepted")
	}
	values.Set("delete_token", tokenFor(other.ID))
	if w := request("POST", path, values, true, nil); w.Code != 403 {
		t.Fatal("another account's token was accepted")
	}
	values.Set("delete_token", tokenFor(user.ID))
	values.Set("username", "wrong")
	if w := request("POST", path, values, true, nil); w.Code != 409 {
		t.Fatal("wrong account name was accepted")
	}
	values.Set("username", user.Username)
	if w := request("POST", path, values, true, nil); w.Code != 303 {
		t.Fatalf("deletion failed: %d %s", w.Code, w.Body.String())
	}
	for _, model := range []interface{}{&models.User{}, &models.SignupDraft{}, &models.ChallengeCompletion{}} {
		var count int64
		query := db.Model(model)
		if _, ok := model.(*models.User); ok {
			query = query.Where("id = ?", user.ID)
		} else {
			query = query.Where("user_id = ?", user.ID)
		}
		if err = query.Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatal("deleted account retained active records")
		}
	}
	if err = db.First(&other, other.ID).Error; err != nil || other.Score != 456 {
		t.Fatal("another account was changed")
	}
	if w := request("GET", "/account", nil, false, oldSession); w.Header().Get("HX-Redirect") != "/login" {
		t.Fatal("old login cookie still works")
	}
	if w := request("POST", "/challenges/1/submit", url.Values{"flag": {"flag{x}"}}, false, oldSession); w.Code != 401 {
		t.Fatal("deleted player can submit flags")
	}
	if w := request("POST", "/login", url.Values{"username": {user.Username}, "login_code": {"123456"}}, false, nil); !strings.Contains(w.Body.String(), "Invalid credentials") {
		t.Fatal("deleted account can log in")
	}
	if w := request("GET", "/scoreboard", nil, false, nil); strings.Contains(w.Body.String(), user.Username) || !strings.Contains(w.Body.String(), other.Username) {
		t.Fatal("leaderboard still shows deleted account")
	}
	if w := request("POST", path, values, true, nil); w.Code != 404 {
		t.Fatal("repeated deletion must report missing account")
	}
	// Force the last cleanup operation to fail; preceding changes must roll back.
	if err = db.Migrator().DropTable(&models.SignupDraft{}); err != nil {
		t.Fatal(err)
	}
	values.Set("username", other.Username)
	values.Set("delete_token", tokenFor(other.ID))
	if w := request("POST", fmt.Sprintf("/admin/accounts/%d/delete", other.ID), values, true, nil); w.Code != 500 {
		t.Fatal("expected cleanup failure")
	}
	var count int64
	db.Model(&models.ChallengeCompletion{}).Where("user_id = ?", other.ID).Count(&count)
	if count != 1 || db.First(&other, other.ID).Error != nil {
		t.Fatal("failed deletion was not rolled back")
	}
}
