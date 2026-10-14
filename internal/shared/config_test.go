package shared

import "testing"

func TestPointsPerCandyEnvironment(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  int
	}{{"", 100}, {"50", 50}, {" 200 ", 200}, {"0", 100}, {"-5", 100}, {"abc", 100}, {"1.5", 100}} {
		t.Setenv("CTF_POINTS_PER_CANDY", tc.value)
		if got := envPositiveInt("CTF_POINTS_PER_CANDY", 100); got != tc.want {
			t.Fatalf("%q: got %d", tc.value, got)
		}
	}
	old := Config
	defer func() { Config = old }()
	Config.PointsPerCandy = 0
	if CandyRate() != 100 {
		t.Fatal("unset configuration must default to 100")
	}
	Config.PointsPerCandy = 75
	if CandyRate() != 75 {
		t.Fatal("configured rate not applied")
	}
}
