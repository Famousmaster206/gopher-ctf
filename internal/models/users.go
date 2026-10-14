package models

import (
	"gorm.io/gorm"
	"time"
)

type User struct {
	gorm.Model
	Username         string `gorm:"unique;not null"`
	Password         string `gorm:"not null"`
	VerificationCode *string
	LastLoginAttempt *time.Time
	LastFlagAttempt  *time.Time
	Score            int `gorm:"default:0"`
	CandiesGiven     int `gorm:"not null;default:0"`
}

// SignupDraft keeps rerolls on the server so refreshing cannot reset the count.
type SignupDraft struct {
	gorm.Model
	Token    string `gorm:"uniqueIndex;not null"`
	Username string `gorm:"uniqueIndex;not null"`
	Rerolls  int
	UserID   *uint
}

// Redeeming candy never subtracts competition points.
func (u User) CandyOwed(pointsPerCandy int) int {
	if pointsPerCandy <= 0 {
		pointsPerCandy = 100
	}
	owed := u.Score/pointsPerCandy - u.CandiesGiven
	if owed < 0 {
		return 0
	}
	return owed
}
