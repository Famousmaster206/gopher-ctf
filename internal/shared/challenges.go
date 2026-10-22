package shared

import (
	"errors"
	"github.com/openlyfree/gopher-ctf/internal/models"
	"gorm.io/gorm"
)

func ChallengesPublic() (bool, error) {
	var settings models.EventSettings
	err := DB.First(&settings, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return settings.ChallengesPublic, err
}
