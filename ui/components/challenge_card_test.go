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

func TestFreeHints(t *testing.T) {
	challenge := models.Challenge{Hints: " first hint \r\n\n<script>alert(1)</script>\n", Points: 100}
	if hints := challenge.HintList(); len(hints) != 2 || hints[0] != "first hint" {
		t.Fatal("hints must ignore empty lines and whitespace")
	}
	for _, completed := range []bool{false, true} {
		var html bytes.Buffer
		if err := ChallengeCard(challenge, completed).Render(context.Background(), &html); err != nil {
			t.Fatal(err)
		}
		if strings.Count(html.String(), `<details class="challenge-hint">`) != 2 || !strings.Contains(html.String(), "hint 1 (free)") || !strings.Contains(html.String(), "your points stay the same") {
			t.Fatal("missing free hint controls")
		}
		if strings.Contains(html.String(), "<script>alert") || !strings.Contains(html.String(), "&lt;script&gt;") {
			t.Fatal("hint text must be escaped")
		}
	}
	var html bytes.Buffer
	if err := ChallengeCard(models.Challenge{}, false).Render(context.Background(), &html); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html.String(), "need a hint?") {
		t.Fatal("empty hints must not create an empty section")
	}
}
