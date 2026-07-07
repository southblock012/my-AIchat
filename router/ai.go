package router

import (
	"github.com/gin-gonic/gin"

	"my-AIchat/middleware/jwt"
)

func AIRouter(r *gin.RouterGroup) {
	aiRouter := r.Group("/ai")
	aiRouter.Use(jwt.Auth())
	{
		//aiRouter.POST("/chat", ai.Chat)
	}
}
