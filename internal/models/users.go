package models

import (
	"gorm.io/gorm"
	"time"
)

type User struct {
	gorm.Model
	Username        string `gorm:"unique;not null"`
	Password        string `gorm:"not null"`
	Score           int    `gorm:"default:0"`
	LastFlagAttempt *time.Time
}
