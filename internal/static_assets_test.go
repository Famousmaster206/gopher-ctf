package internal

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStaticAssetsWithoutSourceTree(t *testing.T) {
	t.Chdir(t.TempDir())
	router := editorRouter()
	for _, tc := range []struct{ path, contentType string }{
		{"/static/css/style.css?v=1.0.19", "text/css"},
		{"/static/images/background.png", "image/png"},
	} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		if w.Code != 200 || w.Body.Len() == 0 || !strings.Contains(w.Header().Get("Content-Type"), tc.contentType) {
			t.Fatalf("%s: status=%d content-type=%s bytes=%d", tc.path, w.Code, w.Header().Get("Content-Type"), w.Body.Len())
		}
	}
}
