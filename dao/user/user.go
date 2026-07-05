package user

import (
	"my-AIchat/common/mysql"
	"my-AIchat/model"

	"gorm.io/gorm"
)

func InsertUser(user *model.User) (*model.User, error) {
	err := mysql.DB.Create(&user).Error
	return user, err
}

func GetUserByUsername(username string) (*model.User, error) {
	user := &model.User{}
	err := mysql.DB.Where("username = ?", username).First(user).Error
	return user, err
}

func IsExistUser(username string) (bool, *model.User) {
	user, err := GetUserByUsername(username)
	if err == gorm.ErrRecordNotFound {
		return false, nil
	}
	return true, user
}
