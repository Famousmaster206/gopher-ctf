package components

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/openlyfree/gopher-ctf/internal/models"
)

func TestFlagFeedbackTarget(t *testing.T) {
	var html bytes.Buffer
	if err := ChallengeCard(models.Challenge{Title: "Test"}, false).Render(context.Background(), &html); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`hx-target="#flag-result"`, `id="flag-result"`, `aria-live="polite"`} {
		if !strings.Contains(html.String(), want) {
			t.Fatalf("missing %s", want)
		}
	}
	html.Reset()
	if err := Layout("Test").Render(context.Background(), &html); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html.String(), "event.detail.shouldSwap = true") || !strings.Contains(html.String(), "target &&") {
		t.Fatal("missing error feedback handler")
	}
}
