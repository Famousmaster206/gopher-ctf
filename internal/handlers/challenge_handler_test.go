package handlers

import (
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
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

func requestFlag(userID uint, id, flag string) *httptest.ResponseRecorder {
	return requestFlagWithHTMX(userID, id, flag, false)
}

func requestFlagWithHTMX(userID uint, id, flag string, hx bool) *httptest.ResponseRecorder {
	r := gin.New()
	r.Use(sessions.Sessions("test", cookie.NewStore([]byte("test-secret"))))
	r.Use(func(c *gin.Context) {
		if userID != 0 {
			sessions.Default(c).Set("user_id", userID)
		}
		c.Next()
	})
	r.POST("/challenges/:id/submit", SubmitFlagHandler)
	req := httptest.NewRequest("POST", "/challenges/"+id+"/submit", strings.NewReader(url.Values{"flag": {flag}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if hx {
		req.Header.Set("HX-Request", "true")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestSubmissionValidation(t *testing.T) {
	for _, tc := range []struct {
		user     uint
		id, flag string
		status   int
	}{
		{0, "1", "flag{x}", 401}, {1, "abc", "flag{x}", 400}, {1, "0", "flag{x}", 400}, {1, "-1", "flag{x}", 400}, {1, "1", " \n ", 400},
	} {
		if w := requestFlag(tc.user, tc.id, tc.flag); w.Code != tc.status {
			t.Fatalf("%+v: got %d", tc, w.Code)
		}
	}
}

func TestChallengeValidation(t *testing.T) {
	for _, values := range []url.Values{
		{"challenge_name": {"Test"}, "challenge_description": {"Test"}, "challenge_points": {"10"}, "challenge_flag": {" "}},
		{"challenge_name": {"Test"}, "challenge_description": {"Test"}, "challenge_points": {"-1"}, "challenge_flag": {"flag{x}"}},
	} {
		r := gin.New()
		r.POST("/create", CreateChallengeHandler)
		req := httptest.NewRequest("POST", "/create", strings.NewReader(values.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 400 {
			t.Fatalf("got %d: %s", w.Code, w.Body.String())
		}
	}
}

// Uses an isolated schema; never changes application tables.
func TestPostgresScoring(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	namespace := fmt.Sprintf("flag_test_%d", time.Now().UnixNano())
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
	if err = db.AutoMigrate(&models.User{}, &models.Challenge{}, &models.ChallengeCompletion{}); err != nil {
		t.Fatal(err)
	}
	oldDB := shared.DB
	shared.DB = db
	defer func() { shared.DB = oldDB }()
	user := models.User{Username: "player", Password: "unused"}
	challenge := models.Challenge{Title: "Test", Flag: "flag{x}", Points: 100}
	if err = db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Create(&challenge).Error; err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := awardChallenge(db, user.ID, challenge.ID, challenge.Points); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	db.First(&user, user.ID)
	var count int64
	db.Model(&models.ChallengeCompletion{}).Count(&count)
	if user.Score != 100 || count != 1 {
		t.Fatalf("score=%d completions=%d", user.Score, count)
	}
	// A failed score update must roll back the completion insert.
	if _, err := awardChallenge(db, 999999, challenge.ID, 100); err == nil {
		t.Fatal("expected missing player error")
	}
	db.Model(&models.ChallengeCompletion{}).Count(&count)
	if count != 1 {
		t.Fatal("failed transaction retained completion")
	}
	other := models.User{Username: "other", Password: "unused"}
	if err = db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	loadUI := func(userID uint, path string, hx bool) *httptest.ResponseRecorder {
		r := gin.New()
		r.Use(sessions.Sessions("test", cookie.NewStore([]byte("test-secret"))))
		r.Use(func(c *gin.Context) {
			if userID != 0 {
				sessions.Default(c).Set("user_id", userID)
			}
			c.Next()
		})
		r.GET("/challenges", ChallengeIndexHandler)
		r.GET("/challenges/:id", ChallengeIndividualHandler)
		req := httptest.NewRequest("GET", path, nil)
		if hx {
			req.Header.Set("HX-Request", "true")
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	for _, hx := range []bool{false, true} {
		for _, tc := range []struct {
			id        uint
			completed bool
		}{{user.ID, true}, {other.ID, false}, {0, false}} {
			for _, path := range []string{"/challenges", fmt.Sprintf("/challenges/%d", challenge.ID)} {
				w := loadUI(tc.id, path, hx)
				if w.Code != 200 || strings.Contains(w.Body.String(), "completed! 🔥") != tc.completed {
					t.Fatalf("wrong completion for user %d at %s: %d %s", tc.id, path, w.Code, w.Body.String())
				}
				if path != "/challenges" && strings.Contains(w.Body.String(), `name="flag"`) == tc.completed {
					t.Fatal("completed challenges must hide the flag form")
				}
				if w.Header().Get("Cache-Control") != "private, no-store" {
					t.Fatal("completion pages must not be shared in caches")
				}
			}
		}
	}
	id := fmt.Sprint(challenge.ID)
	if w := requestFlagWithHTMX(other.ID, id, " \nflag{x}\t ", true); w.Code != 200 {
		t.Fatalf("trimmed success: %d %s", w.Code, w.Body.String())
	} else if !strings.Contains(w.Body.String(), "completed! 🔥") || !strings.Contains(w.Body.String(), `hx-swap-oob="outerHTML"`) {
		t.Fatal("correct submission must update the completion badge immediately")
	}
	if err = db.First(&other, other.ID).Error; err != nil {
		t.Fatal(err)
	}
	if other.Score != 100 {
		t.Fatalf("score=%d", other.Score)
	}
	if w := requestFlag(user.ID, id, " \nflag{x}\t "); w.Code != 409 {
		t.Fatalf("trimmed repeat: %d %s", w.Code, w.Body.String())
	}
	if w := requestFlag(user.ID, id, "guess"); w.Code != 429 || w.Header().Get("Retry-After") != "20" {
		t.Fatalf("cooldown: %d", w.Code)
	}
	db.Model(&user).Update("last_flag_attempt", nil)
	if w := requestFlag(user.ID, id, "FLAG{x}"); w.Code != 422 {
		t.Fatalf("case sensitive: %d", w.Code)
	}
	if w := requestFlag(user.ID, "999999", "flag{x}"); w.Code != 404 {
		t.Fatalf("missing challenge: %d", w.Code)
	}
}
