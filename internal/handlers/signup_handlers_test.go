package handlers

import (
	"regexp"
	"testing"
)

func TestGeneratedCredentials(t *testing.T) {
	names := regexp.MustCompile(`^[A-Z][a-z]+[A-Z][a-z]+[0-9]{3}$`)
	codes := regexp.MustCompile(`^[1-9][0-9]{5}$`)
	for i := 0; i < 100; i++ {
		name, err := randomUsername()
		if err != nil {
			t.Fatal(err)
		}
		if !names.MatchString(name) {
			t.Fatalf("invalid username %q", name)
		}
		code, err := randomCode()
		if err != nil {
			t.Fatal(err)
		}
		if !codes.MatchString(code) {
			t.Fatalf("invalid code %q", code)
		}
	}
}
