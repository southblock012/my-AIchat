package router

import (
	"github.com/gin-gonic/gin"
)

func InitRouter() *gin.Engine {
	r := gin.Default()
	enterRouter := r.Group("/ai-chat")
	//登录注册路由
	{
		UserRouter(enterRouter)
	}
	//后续路由需要jwt鉴权
	{
		AIRouter(enterRouter)
	}
	{
		ImageRouter(enterRouter)
	}
	{
		FileRouter(enterRouter)
	}
	return r
}
