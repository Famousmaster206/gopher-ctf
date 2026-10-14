package models

import (
	"gorm.io/gorm"
	"strings"
)

type Challenge struct {
	gorm.Model
	Title       string
	Description string
	Points      int
	Flag        string
	Hints       string
}

// Admins enter one optional hint per line; blank lines are ignored.
func (c Challenge) HintList() []string {
	var hints []string
	for _, line := range strings.Split(c.Hints, "\n") {
		if hint := strings.TrimSpace(line); hint != "" {
			hints = append(hints, hint)
		}
	}
	return hints
}
