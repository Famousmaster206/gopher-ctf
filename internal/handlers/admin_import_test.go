package handlers

import (
	"strings"
	"testing"
)

func TestDecodeImport(t *testing.T) {
	tests := []struct {
		name string
		json string
		wantErr bool
		wantHints string
		wantPoints string
	}{
		{"string hints and integer points", `[{"challenge_name":" A ","challenge_description":"D","challenge_points":3,"challenge_flag":"BASIS{x}","challenge_hints":"one\ntwo"}]`, false, "one\ntwo", "3"},
		{"array hints and numeric string points", `[{"challenge_name":"A","challenge_description":"D","challenge_points":"4","challenge_flag":"BASIS{x}","challenge_hints":["one","two"],"extra":true}]`, false, "one\ntwo", "4"},
		{"malformed", `[`, true, "", ""},
		{"wrong top level", `{}`, true, "", ""},
		{"null top level", `null`, true, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := decodeImport([]byte(tc.json))
			if (err != nil) != tc.wantErr { t.Fatalf("error=%v", err) }
			if err == nil && (rows[0].Hints != tc.wantHints || rows[0].Points != tc.wantPoints) { t.Fatalf("row=%+v", rows[0]) }
		})
	}
}

func TestValidateChallengeInput(t *testing.T) {
	for _, tc := range []struct { name, points string; wantErr error }{
		{"", "1", errChallengeRequired}, {"name", "-1", errChallengeNegativePoints}, {"name", "nope", errChallengePoints},
	} {
		_, err := validateChallengeInput(challengeInput{Name: tc.name, Description: "description", Points: tc.points, Flag: "flag"})
		if err != tc.wantErr { t.Errorf("%q: got %v, want %v", tc.name, err, tc.wantErr) }
	}
	challenge, err := validateChallengeInput(challengeInput{Name: " name ", Description: " description ", Points: "2", Flag: " flag ", Hints: " h "})
	if err != nil || challenge.Title != "name" || challenge.Description != "description" || challenge.Flag != "flag" || !strings.Contains(challenge.Hints, "h") { t.Fatalf("challenge=%+v err=%v", challenge, err) }
}
