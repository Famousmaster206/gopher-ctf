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
	Score            int `gorm:"default:0"`
}

// SignupDraft keeps rerolls on the server so refreshing cannot reset the count.
type SignupDraft struct {
	gorm.Model
	Token    string `gorm:"uniqueIndex;not null"`
	Username string `gorm:"uniqueIndex;not null"`
	Rerolls  int
	UserID   *uint
}
