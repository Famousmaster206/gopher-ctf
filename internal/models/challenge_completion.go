package models

import "gorm.io/gorm"

type ChallengeCompletion struct {
	gorm.Model
	UserID      uint `gorm:"index;not null;uniqueIndex:unique_user_challenge"`
	ChallengeID uint `gorm:"index;not null;uniqueIndex:unique_user_challenge"`
}
