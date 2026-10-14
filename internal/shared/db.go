package shared

import (
	"log"
	"os"

	"github.com/openlyfree/gopher-ctf/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

func InitDB() *gorm.DB {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Panicln("DATABASE_URL environment variable not set")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		SkipDefaultTransaction: true,
		PrepareStmt:            true,
	})
	if err != nil {
		log.Panicln(err)
	}

	if err := db.AutoMigrate(&models.User{}); err != nil {
		log.Panicln(err)
	}
	if err := db.AutoMigrate(&models.SignupDraft{}); err != nil {
		log.Panicln(err)
	}
	if err := db.AutoMigrate(&models.Challenge{}); err != nil {
		log.Panicln(err)
	}
	if err := db.AutoMigrate(&models.ChallengeCompletion{}); err != nil {
		log.Panicln(err)
	}

	return db
}
