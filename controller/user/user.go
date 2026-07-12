package user

import (
	"github.com/gin-gonic/gin"

	"my-AIchat/common/code"
	"my-AIchat/controller"
	"my-AIchat/service/user"
	"net/http"
)

type (
	// LoginRequest 用户登录请求体
	LoginRequest struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	// LoginResponse 用户登录响应体
	LoginResponse struct {
		controller.Response
		Token string `json:"token,omitempty"`
	}
	// RegisterRequest 用户注册请求体
	RegisterRequest struct {
		Email    string `json:"email" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	// RegisterResponse 用户注册响应体
	RegisterResponse struct {
		controller.Response
		Token string `json:"token,omitempty"`
	}
	// CaptchaRequest 用户获取验证码请求体
	CaptchaRequest struct {
		Email string `json:"email" binding:"required"`
	}
	// CaptchaResponse 用户获取验证码响应体
	CaptchaResponse struct {
		controller.Response
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
		//判断用户名和密码是否正确
		//在service/user/user.go中实现
	}
	token, code_ := user.Login(req.Username, req.Password)
	if code_ != code.CodeSuccess || token == "" {
		res.SetCode(code_)
		c.JSON(http.StatusOK, res)
		return
	}
	res.SetSuccess()
	res.Token = token
	c.JSON(http.StatusOK, res)
}

// UserRegister 用户注册
func UserRegister(c *gin.Context) {
	res := &RegisterResponse{}
	req := &RegisterRequest{}
	if err := c.ShouldBindJSON(req); err != nil {
		res.SetCode(code.CodeInvalidParams)
		c.JSON(http.StatusOK, res)
		return
	}
	//调用user.Register服务层注册用户
	token, code_ := user.Register(req.Email, req.Password)
	if code_ != code.CodeSuccess || token == "" {
		res.SetCode(code_)
		c.JSON(http.StatusOK, res)
		return
	}
	res.SetSuccess()
	res.Token = token
	c.JSON(http.StatusOK, res)
}

// UserCaptcha 用户获取验证码
func UserCaptcha(c *gin.Context) {
	req := new(CaptchaRequest)
	res := new(CaptchaResponse)
	//解析参数
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusOK, res.SetCode(code.CodeInvalidParams))
		return
	}

	//给service层进行处理
	code_ := user.SendCaptcha(req.Email)
	if code_ != code.CodeSuccess {
		c.JSON(http.StatusOK, res.SetCode(code_))
		return
	}
	//匿名字段，其实本身res.Success()调用就是res.Response.Success()
	//res.Response.Success()
	res.SetSuccess()
	c.JSON(http.StatusOK, res)
}
