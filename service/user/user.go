package user

import (
	"my-AIchat/common/code"
	"my-AIchat/common/email"
	"my-AIchat/common/redis"
	"my-AIchat/dao/user"
	"my-AIchat/utils"
	"my-AIchat/utils/myjwt"
)

// 登录业务逻辑
func Login(username, password string) (string, code.Code) {
	// 1.从数据库中查询用户是否存在
	ok, user := user.IsExistUser(username)
	if !ok {
		return "", code.CodeUserNotExist
	}
	// 2.判断密码是否正确
	if utils.BcryptCompare(user.Password, password) != nil {
		return "", code.CodeInvalidPassword
	}
	// 3.返回Token
	token, err := myjwt.GenerateToken(user.Username, user.ID)
	if err != nil {
		return "", code.CodeServerBusy
	}
	return token, code.CodeSuccess
}

// 注册业务逻辑
func Register(email_, password, captcha string) (string, code.Code) {
	// 1.判断邮箱是否存在
	if ok, _ := user.IsExistUserByEmail(email_); ok {
		return "", code.CodeEmailExist
	}
	// 2.判断密码是否为空
	if password == "" {
		return "", code.CodePasswordEmpty
	}
	// 3.生成11位ID
	username := utils.GenerateRandomID(11)
	// 4.验证验证码
	if ok, err := redis.CheckCaptchaForEmail(email_, captcha); !ok || err != nil {
		return "", code.CodeInvalidCaptcha
	}
	// 5.发送ID到邮箱
	if err := email.SendMail(email_, username, email.UserNameMsg); err != nil {
		return "", code.CodeServerBusy
	}
	// 6.注册用户，写到数据库
	user_, ok := user.Register(username, email_, password)
	if !ok {
		return "", code.CodeServerBusy
	}
	// 7.生成Token
	token, err := myjwt.GenerateToken(user_.Username, user_.ID)
	// 8.返回Token
	if err != nil {
		return "", code.CodeServerBusy
	}

	return token, code.CodeSuccess
}

// 往指定邮箱发送验证码
// 分为以下任务：
// 1：先存放redis
// 2：再进行远程发送
func SendCaptcha(email_ string) code.Code {
	send_code := utils.GenerateRandomID(6)
	//1:先存放到redis
	if err := redis.SetCaptchaForEmail(email_, send_code); err != nil {
		return code.CodeServerBusy
	}

	//2:再进行远程发送
	if err := email.SendMail(email_, send_code, email.CodeMsg); err != nil {
		return code.CodeServerBusy
	}

	return code.CodeSuccess
}
