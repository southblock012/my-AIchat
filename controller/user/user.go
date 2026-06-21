package user

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"my-AIchat/controller"

	"my-AIchat/common/code"
)

type (
	LoginRequest struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	LoginResponse struct {
		controller.Response
		Token string `json:"token,omitempty"`
	}
)

// UserLogin 用户登录
func UserLogin(c *gin.Context) {
	req := &LoginRequest{}
	res := &LoginResponse{}
	if err := c.ShouldBindJSON(req); err != nil {
		res.SetCode(code.CodeInvalidParams)
		c.JSON(http.StatusOK, res)
		return
	}
	//判断用户名和密码是否正确
	//在service/user/user.go中实现
	res.SetSuccess()
	res.Token = "123456"
	c.JSON(http.StatusOK, res)
}

// UserRegister 用户注册
func UserRegister(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"msg": "user register",
	})
}

// UserCaptcha 用户获取验证码
func UserCaptcha(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"msg": "user captcha",
	})
}
