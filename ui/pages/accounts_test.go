package pages

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/openlyfree/gopher-ctf/internal/models"
)

func renderAccountTest(t *testing.T, component templ.Component) string {
	t.Helper()
	var b bytes.Buffer
	if err := component.Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestAccountScreens(t *testing.T) {
	html := renderAccountTest(t, Signup("CipherHunter042", "token", 3))
	if !strings.Contains(html, "3 name changes left") || !strings.Contains(html, "new name pls") || strings.Contains(html, `name="username"`) {
		t.Fatal("signup must show a generated name and three rerolls")
	}
	html = renderAccountTest(t, Signup("CipherHunter042", "token", 0))
	if strings.Contains(html, "new name pls") || !strings.Contains(html, "keep this name") {
		t.Fatal("exhausted rerolls must keep the last name")
	}
	html = renderAccountTest(t, AccountCreated("CipherHunter042", "123456", "654321"))
	for _, want := range []string{"123456", "654321", "you need your points code to get candy", "only shown here once"} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q", want)
		}
	}
	code := "654321"
	user := models.User{Username: "CipherHunter042", Score: 100, VerificationCode: &code, Password: "LOGIN_SECRET"}
	html = renderAccountTest(t, AdminPlayers([]models.User{user}))
	if !strings.Contains(html, code) || !strings.Contains(html, "100") || strings.Contains(html, user.Password) {
		t.Fatal("teacher table must show points and points code, never login credentials")
	}
	html = renderAccountTest(t, Scoreboard([]models.User{user}))
	if strings.Contains(html, code) || strings.Contains(html, user.Password) {
		t.Fatal("public scoreboard leaked a code")
	}
	html = renderAccountTest(t, Account(user))
	if !strings.Contains(html, code) || !strings.Contains(html, "get candy") || strings.Contains(html, user.Password) {
		t.Fatal("account must show the candy code only")
	}
}
