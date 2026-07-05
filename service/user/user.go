package user

import (
	"my-AIchat/common/code"
	"my-AIchat/controller/user"
	"my-AIchat/dao/user"
)

// 登录业务逻辑
func Login(req *user.LoginRequest) (string, code.Code) {
	// 从数据库中查询用户
	ok, user := user.IsExistUser(req.Username)
	if !ok {
		return "", nil
	}

}
