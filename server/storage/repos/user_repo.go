package repos

import (
	"errors"

	"github.com/THPTUHA/kairos/server/storage/models"
)

type UserRepository struct {
	Users []models.User
}

var UserRepo = UserRepository{
	Users: []models.User{},
}

func (r *UserRepository) FindByID(id int) (models.User, error) {

	if id > 0 {
		return models.User{
			ID:    1,
			Email: "abc",
		}, nil
	}

	return models.User{}, errors.New("Not found")
}

func (r *UserRepository) FindByUserName(username string) (models.User, error) {

	if username != "" {
		return models.User{
			ID:    1,
			Email: "abc",
		}, nil
	}

	return models.User{}, errors.New("Not found")
}
