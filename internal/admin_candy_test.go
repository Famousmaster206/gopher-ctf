package internal

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/openlyfree/gopher-ctf/internal/models"
	"github.com/openlyfree/gopher-ctf/internal/shared"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestCandyOwed(t *testing.T) {
	for _, tc := range []struct{ points, given, owed int }{{0, 0, 0}, {99, 0, 0}, {100, 0, 1}, {250, 0, 2}, {250, 1, 1}, {250, 2, 0}, {300, 2, 1}, {50, 2, 0}} {
		if got := (models.User{Score: tc.points, CandiesGiven: tc.given}).CandyOwed(100); got != tc.owed {
			t.Fatalf("%+v: owed %d", tc, got)
		}
	}
	if got := (models.User{Score: 250, CandiesGiven: 1}).CandyOwed(50); got != 4 {
		t.Fatalf("custom rate owed %d", got)
	}
	r := accountRouter()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/admin/accounts/1/candy", nil))
	if w.Code != 401 {
		t.Fatal("candy redemption must require admin login")
	}
}

func TestPostgresCandyRedemption(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL for candy integration tests")
	}
	namespace := fmt.Sprintf("candy_test_%d", time.Now().UnixNano())
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
	if err = db.AutoMigrate(&models.User{}); err != nil {
		t.Fatal(err)
	}
	oldDB, oldConfig := shared.DB, shared.Config
	shared.DB = db
	shared.Config.Key = "candy-key"
	shared.Config.Password = "candy-password"
	defer func() { shared.DB = oldDB; shared.Config = oldConfig }()
	user := models.User{Username: "CandyPlayer", Password: "unused", Score: 250}
	if err = db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	adminToken, err := shared.EncryptAdminToken(shared.Config.Password)
	if err != nil {
		t.Fatal(err)
	}
	r := accountRouter()
	request := func(method, path string, values url.Values) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(values.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("HX-Request", "true")
		req.AddCookie(&http.Cookie{Name: "secret_candy_vault_access", Value: adminToken})
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	valuesFrom := func(w *httptest.ResponseRecorder) url.Values {
		t.Helper()
		body := w.Body.String()
		code := regexp.MustCompile(`name="candy_token" value="([a-f0-9]+)"`).FindStringSubmatch(body)
		given := regexp.MustCompile(`name="candies_given" value="([0-9]+)"`).FindStringSubmatch(body)
		if len(code) != 2 || len(given) != 2 {
			t.Fatal("missing give candy form")
		}
		return url.Values{"candy_token": {code[1]}, "candies_given": {given[1]}}
	}
	path := fmt.Sprintf("/admin/accounts/%d/candy", user.ID)
	first := valuesFrom(request("GET", "/secret-candy-vault/players", nil))
	if w := request("POST", path, url.Values{"candies_given": {"0"}}); w.Code != 403 {
		t.Fatal("missing token accepted")
	}
	// Concurrent copies of the same click must redeem one candy, not twelve.
	var wg sync.WaitGroup
	codes := make(chan int, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); codes <- request("POST", path, first).Code }()
	}
	wg.Wait()
	close(codes)
	for code := range codes {
		if code != 200 {
			t.Fatalf("unexpected redemption response %d", code)
		}
	}
	if err = db.First(&user, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if user.CandiesGiven != 1 || user.CandyOwed(100) != 1 || user.Score != 250 {
		t.Fatalf("bad first redemption: %+v", user)
	}
	second := valuesFrom(request("GET", "/secret-candy-vault/players", nil))
	if w := request("POST", path, second); w.Code != 200 {
		t.Fatal("second redemption failed")
	}
	db.First(&user, user.ID)
	if user.CandiesGiven != 2 || user.CandyOwed(100) != 0 || user.Score != 250 {
		t.Fatal("redemption changed points or went negative")
	}
	w := request("POST", path, second)
	if !strings.Contains(w.Body.String(), "candy balance changed") || !strings.Contains(w.Body.String(), "disabled") {
		t.Fatal("exhausted balance must block more candy")
	}
	db.First(&user, user.ID)
	if user.CandiesGiven != 2 {
		t.Fatal("replayed click redeemed another candy")
	}
	db.Model(&user).Update("score", 300)
	third := valuesFrom(request("GET", "/secret-candy-vault/players", nil))
	if w := request("POST", path, third); w.Code != 200 {
		t.Fatal("newly earned candy could not be redeemed")
	}
	db.First(&user, user.ID)
	if user.CandiesGiven != 3 || user.Score != 300 || user.CandyOwed(100) != 0 {
		t.Fatal("incorrect new reward balance")
	}
	shared.Config.PointsPerCandy = 50
	fourth := valuesFrom(request("GET", "/secret-candy-vault/players", nil))
	if w := request("POST", path, fourth); w.Code != 200 {
		t.Fatal("custom candy rate redemption failed")
	}
	db.First(&user, user.ID)
	if user.CandiesGiven != 4 || user.Score != 300 || user.CandyOwed(50) != 2 {
		t.Fatal("custom candy rate not applied consistently")
	}
	fifth := valuesFrom(request("GET", "/secret-candy-vault/players", nil))
	shared.Config.PointsPerCandy = 200
	page := request("GET", "/secret-candy-vault/", nil)
	if !strings.Contains(page.Body.String(), "200 points = 1 candy") {
		t.Fatal("admin rate label did not update")
	}
	if w := request("POST", path, fifth); !strings.Contains(w.Body.String(), "candy balance changed") {
		t.Fatal("higher rate must prevent further redemption")
	}

}
