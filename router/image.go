package router

import (
	"github.com/gin-gonic/gin"

	"my-AIchat/middleware/jwt"
)

func ImageRouter(r *gin.RouterGroup) {
	imageRouter := r.Group("/image")
	imageRouter.Use(jwt.Auth())
	{
		//imageRouter.POST("/upload", image.Upload)
	}
}
