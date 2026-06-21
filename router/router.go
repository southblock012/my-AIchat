package router

import (
	"github.com/gin-gonic/gin"
)

func InitRouter() *gin.Engine {
	r := gin.Default()
	enterRouter := r.Group("/ai-chat")
	{
		UserRouter(enterRouter)
	}
	return r
}
