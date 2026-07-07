package router

import (
	"github.com/gin-gonic/gin"

	"my-AIchat/middleware/jwt"
)

func FileRouter(r *gin.RouterGroup) {
	fileRouter := r.Group("/file")
	fileRouter.Use(jwt.Auth())
	{
		//fileRouter.POST("/upload", file.Upload)
	}
}
