package internal

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/openlyfree/gopher-ctf/internal/models"
	"github.com/openlyfree/gopher-ctf/internal/shared"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func editorRouter() *gin.Engine {
	r := gin.New()
	r.Use(sessions.Sessions("editor-test", cookie.NewStore([]byte("editor-test-key"))))
	RegisterRoutes(r)
	return r
}

func TestChallengeEditorRequiresAdmin(t *testing.T) {
	r := editorRouter()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/admin/challenges/new", nil))
	if w.Code != 401 {
		t.Fatal("challenge creation page must require admin login")
	}
	for _, method := range []string{"GET", "POST"} {
		path := "/admin/challenges"
		if method == "POST" {
			path += "/1"
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(method, path, nil))
		if w.Code != 401 {
			t.Fatalf("%s: got %d", method, w.Code)
		}
	}
}

// Uses a temporary schema so application tables are never changed.
func TestPostgresChallengeEditor(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL for PostgreSQL integration tests")
	}
	namespace := fmt.Sprintf("edit_test_%d", time.Now().UnixNano())
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
	if err = db.AutoMigrate(&models.Challenge{}, &models.User{}); err != nil {
		t.Fatal(err)
	}
	oldDB, oldConfig := shared.DB, shared.Config
	shared.DB = db
	shared.Config.Key = "editor-key"
	shared.Config.Password = "editor-password"
	defer func() { shared.DB = oldDB; shared.Config = oldConfig }()
	token, err := shared.EncryptAdminToken(shared.Config.Password)
	if err != nil {
		t.Fatal(err)
	}
	challenge := models.Challenge{Title: "Original", Description: "Old description", Points: 100, Flag: "flag{old}"}
	if err = db.Create(&challenge).Error; err != nil {
		t.Fatal(err)
	}
	second := models.Challenge{Title: "Second challenge", Description: "Other description", Points: 200, Flag: "flag{second}"}
	if err = db.Create(&second).Error; err != nil {
		t.Fatal(err)
	}
	user := models.User{Username: "player", Password: "unused", Score: 100}
	if err = db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	r := editorRouter()
	request := func(method, path string, values url.Values) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(values.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{Name: "secret_candy_vault_access", Value: token})
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	w := request("GET", "/admin/challenges", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Original") || !strings.Contains(w.Body.String(), "flag{old}") || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("editor did not list existing challenges")
	}
	if !strings.Contains(w.Body.String(), `hx-target="#admin-challenge-search-results"`) || !strings.Contains(w.Body.String(), `hx-select="#admin-challenge-search-results"`) {
		t.Fatal("search must only replace the sidebar, preserving editor inputs")
	}
	if strings.Count(w.Body.String(), `name="challenge_flag"`) != 1 || strings.Contains(w.Body.String(), "flag{second}") {
		t.Fatal("the page must contain only the selected edit form")
	}
	secondPath := "/admin/challenges?id=" + fmt.Sprint(second.ID)
	w = request("GET", secondPath, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `value="flag{second}"`) || strings.Contains(w.Body.String(), `value="flag{old}"`) {
		t.Fatal("sidebar selection did not change the editor")
	}
	req := httptest.NewRequest("GET", secondPath+"&q=Second", nil)
	req.Header.Set("HX-Request", "true")
	req.AddCookie(&http.Cookie{Name: "secret_candy_vault_access", Value: token})
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 || strings.Contains(w.Body.String(), "<!doctype") || !strings.Contains(w.Body.String(), "admin-challenge-workspace") || strings.Contains(w.Body.String(), ">Original</span>") {
		t.Fatal("search should return the filtered workspace for HTMX")
	}
	if w = request("GET", "/admin/challenges?id=nope", nil); w.Code != 400 {
		t.Fatal("invalid selection accepted")
	}
	if w = request("GET", "/admin/challenges?id=999999", nil); w.Code != 404 {
		t.Fatal("missing selection accepted")
	}
	path := fmt.Sprintf("/admin/challenges/%d", challenge.ID)
	values := url.Values{"challenge_name": {" Updated "}, "challenge_description": {"new\ndescription"}, "challenge_points": {"0"}, "challenge_flag": {" flag{new} "}}
	w = request("POST", path, values)
	if w.Code != 303 || w.Header().Get("Location") != "/admin/challenges?saved=1&id="+fmt.Sprint(challenge.ID) {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	if err = db.First(&challenge, challenge.ID).Error; err != nil {
		t.Fatal(err)
	}
	if challenge.Title != "Updated" || challenge.Description != "new\ndescription" || challenge.Points != 0 || challenge.Flag != "flag{new}" {
		t.Fatalf("incorrect saved challenge: %+v", challenge)
	}
	db.First(&user, user.ID)
	if user.Score != 100 {
		t.Fatal("editing changed previously earned points")
	}
	for _, bad := range []string{"-1", "abc", ""} {
		values.Set("challenge_points", bad)
		w = request("POST", path, values)
		if w.Code != 400 {
			t.Fatalf("invalid points %q accepted", bad)
		}
	}
	values.Set("challenge_points", "20")
	values.Set("challenge_flag", " ")
	w = request("POST", path, values)
	if w.Code != 400 {
		t.Fatal("blank flag accepted")
	}
	db.First(&challenge, challenge.ID)
	if challenge.Points != 0 || challenge.Flag != "flag{new}" {
		t.Fatal("invalid save changed the database")
	}
	if w = request("POST", "/admin/challenges/nope", values); w.Code != 400 {
		t.Fatal("bad ID accepted")
	}
	if w = request("POST", "/admin/challenges/999999", values); w.Code != 404 {
		t.Fatal("missing challenge was accepted")
	}
}
