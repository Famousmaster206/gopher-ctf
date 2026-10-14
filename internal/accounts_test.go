package internal

import (
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/openlyfree/gopher-ctf/internal/models"
	"github.com/openlyfree/gopher-ctf/internal/shared"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func accountRouter() *gin.Engine {
	r := gin.New()
	r.Use(sessions.Sessions("test-session", cookie.NewStore([]byte("test-session-key"))))
	RegisterRoutes(r)
	return r
}

func TestAccountAndTeacherRoutesRequireLogin(t *testing.T) {
	r := accountRouter()
	for _, path := range []string{"/secret-candy-vault/", "/secret-candy-vault/players"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 401 {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
	req := httptest.NewRequest("GET", "/account", nil)
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Header().Get("HX-Redirect") != "/login" {
		t.Fatal("account codes must require login")
	}
	for _, path := range []string{"/register", "/register/reroll"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("POST", path, nil))
		if !strings.Contains(w.Body.String(), "open the sign up page first") {
			t.Fatal("missing signup draft was accepted")
		}
	}
}

// Set TEST_DATABASE_URL to run the full flow in a temporary PostgreSQL schema.
func TestPostgresAccountFlow(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	namespace := fmt.Sprintf("account_test_%d", time.Now().UnixNano())
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
	if err = db.AutoMigrate(&models.User{}, &models.SignupDraft{}); err != nil {
		t.Fatal(err)
	}
	oldDB, oldConfig := shared.DB, shared.Config
	shared.DB = db
	shared.Config.Key = "admin-test-key"
	shared.Config.Password = "admin-password"
	defer func() { shared.DB = oldDB; shared.Config = oldConfig }()
	server := httptest.NewServer(accountRouter())
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	fetch := func(method, path string, values url.Values) string {
		t.Helper()
		req, err := http.NewRequest(method, server.URL+path, strings.NewReader(values.Encode()))
		if err != nil {
			t.Fatal(err)
		}
		if method == "POST" {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("HX-Request", "true")
		}
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != 200 {
			t.Fatalf("%s: %d %s", path, res.StatusCode, body)
		}
		return string(body)
	}
	body := fetch("GET", "/register", nil)
	tokenMatch := regexp.MustCompile(`name="signup_token" value="([a-z0-9]+)"`).FindStringSubmatch(body)
	if len(tokenMatch) != 2 {
		t.Fatal("missing signup token")
	}
	values := url.Values{"signup_token": {tokenMatch[1]}}
	var draft models.SignupDraft
	if err = db.Where("token = ?", tokenMatch[1]).First(&draft).Error; err != nil {
		t.Fatal(err)
	}
	// Simultaneous clicks must share the same three-reroll budget.
	var wg sync.WaitGroup
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, err := http.NewRequest("POST", server.URL+"/register/reroll", strings.NewReader(values.Encode()))
			if err != nil {
				failures <- err
				return
			}
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("HX-Request", "true")
			res, err := client.Do(req)
			if err != nil {
				failures <- err
				return
			}
			defer res.Body.Close()
			body, err := io.ReadAll(res.Body)
			if err != nil {
				failures <- err
				return
			}
			if res.StatusCode != 200 || strings.Contains(string(body), "couldn't") {
				failures <- fmt.Errorf("reroll failed: %d %s", res.StatusCode, body)
			}
		}()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	if err = db.First(&draft, draft.ID).Error; err != nil {
		t.Fatal(err)
	}
	if draft.Rerolls != 3 {
		t.Fatalf("concurrent rerolls=%d", draft.Rerolls)
	}

	lastName := draft.Username
	fetch("GET", "/register", nil)
	fetch("POST", "/register/reroll", values)
	db.First(&draft, draft.ID)
	if draft.Rerolls != 3 || draft.Username != lastName {
		t.Fatal("refresh or extra request reset rerolls")
	}
	values.Set("username", "CustomName999")
	values.Set("password", "custom-password")
	signupURL, _ := url.Parse(server.URL)
	oldCookies := jar.Cookies(signupURL)
	body = fetch("POST", "/register", values)
	matches := regexp.MustCompile(`class="account-code">([0-9]{6})</p>`).FindAllStringSubmatch(body, -1)
	if len(matches) != 2 {
		t.Fatal("account must receive two codes")
	}
	loginCode, pointsCode := matches[0][1], matches[1][1]
	if loginCode == pointsCode {
		t.Fatal("codes must differ")
	}
	var user models.User
	if err = db.Where("username = ?", lastName).First(&user).Error; err != nil {
		t.Fatal(err)
	}
	if user.VerificationCode == nil || *user.VerificationCode != pointsCode || user.Password == loginCode || bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(loginCode)) != nil {
		t.Fatal("wrong code storage")
	}
	if body = fetch("GET", "/account", nil); !strings.Contains(body, pointsCode) || strings.Contains(body, loginCode) {
		t.Fatal("account must reveal points code only")
	}
	replay, err := http.NewRequest("POST", server.URL+"/register", strings.NewReader(values.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	replay.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, cookie := range oldCookies {
		replay.AddCookie(cookie)
	}
	replayResponse, err := http.DefaultClient.Do(replay)
	if err != nil {
		t.Fatal(err)
	}
	replayBody, err := io.ReadAll(replayResponse.Body)
	replayResponse.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(replayBody), "account already made") {
		t.Fatal("old signup cookie could be reused")
	}
	var count int64
	db.Model(&models.User{}).Count(&count)
	if count != 1 {
		t.Fatal("duplicate account created")
	}
	fetch("POST", "/logout", nil)
	body = fetch("POST", "/login", url.Values{"username": {lastName}, "login_code": {pointsCode}})
	if !strings.Contains(body, "Invalid credentials") {
		t.Fatal("points code was accepted for login")
	}
	db.Model(&user).Update("last_login_attempt", nil)
	fetch("POST", "/login", url.Values{"username": {lastName}, "login_code": {" " + loginCode + " "}})
	if body = fetch("GET", "/account", nil); !strings.Contains(body, pointsCode) {
		t.Fatal("code login failed")
	}
	db.Model(&user).Update("score", 125)
	fetch("POST", "/gimme-secret-candy-vault", url.Values{"password": {"admin-password"}})
	body = fetch("GET", "/secret-candy-vault/players", nil)
	if !strings.Contains(body, lastName) || !strings.Contains(body, pointsCode) || !strings.Contains(body, "125") || strings.Contains(body, loginCode) {
		t.Fatal("teacher view has incorrect details")
	}
	body = fetch("GET", "/scoreboard", nil)
	if strings.Contains(body, pointsCode) || strings.Contains(body, loginCode) {
		t.Fatal("scoreboard exposed a code")
	}
}
