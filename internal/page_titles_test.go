package internal

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openlyfree/gopher-ctf/internal/shared"
)

func TestNavigationFragmentsIncludeTitle(t *testing.T) {
	oldConfig := shared.Config
	shared.Config.Name = "BASIC CTF"
	defer func() { shared.Config = oldConfig }()
	r := accountRouter()
	for _, hx := range []bool{false, true} {
		req := httptest.NewRequest("GET", "/login", nil)
		if hx {
			req.Header.Set("HX-Request", "true")
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "<title>Log in | BASIC CTF</title>") {
			t.Fatalf("missing page title for HX=%v", hx)
		}
	}
}
