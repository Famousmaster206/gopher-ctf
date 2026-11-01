package pages

import "fmt"

// ChallengePointsStyle gives the common 50- and 100-point challenges visibly
// different greens, then transitions through yellow to red at 500 points.
func ChallengePointsStyle(points int) string {
	if points < 50 {
		points = 50
	}
	if points > 500 {
		points = 500
	}
	hue := 0
	switch {
	case points <= 100:
		hue = 120 - (points-50)*30/50 // green to lime
	case points <= 275:
		hue = 90 - (points-100)*30/175 // lime to yellow
	default:
		hue = 60 - (points-275)*60/225 // yellow to red
	}
	return fmt.Sprintf("color: hsl(%d 75%% 45%%)", hue)
}
