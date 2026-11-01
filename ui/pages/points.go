package pages

import "fmt"

// ChallengePointsStyle maps 50 points to green, 275 to yellow, and 500 to red,
// with a smooth hue transition between those endpoints.
func ChallengePointsStyle(points int) string {
	if points < 50 {
		points = 50
	}
	if points > 500 {
		points = 500
	}
	hue := 120 - (points-50)*120/450
	return fmt.Sprintf("color: hsl(%d 75%% 45%%)", hue)
}
