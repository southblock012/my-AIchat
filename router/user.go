package router

import (
	"github.com/gin-gonic/gin"

	"my-AIchat/controller/user"
)

func UserRouter(r *gin.RouterGroup) {
	userRouter := r.Group("/user")
	{
		userRouter.POST("/login", user.UserLogin)
		userRouter.POST("/register", user.UserRegister)
		userRouter.POST("/captcha", user.UserCaptcha)
	}
}
