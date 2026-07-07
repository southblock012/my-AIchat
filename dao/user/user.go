package user

import (
	"my-AIchat/common/mysql"
	"my-AIchat/model"
	"my-AIchat/utils"

	"gorm.io/gorm"
)

// InsertUser 插入用户
func InsertUser(user *model.User) (*model.User, error) {
	err := mysql.DB.Create(user).Error
	return user, err
}

// GetUserByUsername 根据用户名查询用户
func GetUserByUsername(username string) (*model.User, error) {
	user := &model.User{}
	err := mysql.DB.Where("username = ?", username).First(user).Error
	return user, err
}

// GetUserByEmail 根据邮箱查询用户
func GetUserByEmail(email string) (*model.User, error) {
	user := &model.User{}
	err := mysql.DB.Where("email = ?", email).First(user).Error
	return user, err
}

// IsExistUser 判断用户名是否存在
func IsExistUser(username string) (bool, *model.User) {
	user, err := GetUserByUsername(username)
	if err == gorm.ErrRecordNotFound {
		return false, nil
	}
	return true, user
}

// IsExistUserByEmail 判断邮箱是否存在
func IsExistUserByEmail(email string) (bool, *model.User) {
	user, err := GetUserByEmail(email)
	if err == gorm.ErrRecordNotFound {
		return false, nil
	}
	return true, user
}

// Register 注册用户
func Register(username, email, password string) (*model.User, bool) {
	password, err := utils.BcryptGenerate(password)
	if err != nil {
		return nil, false
	}
	user := &model.User{
		Name:     username,
		Username: username,
		Password: password,
		Email:    email,
	}
	if user_, err := InsertUser(user); err != nil {
		return nil, false
	} else {
		return user_, true
	}
}
