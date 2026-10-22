package models

// EventSettings stores the single event-wide challenge release switch.
type EventSettings struct {
	ID               uint `gorm:"primaryKey"`
	ChallengesPublic bool
}
