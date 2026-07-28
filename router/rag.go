package router

import (
	"my-AIchat/controller/rag"
	"my-AIchat/middleware/jwt"

	"github.com/gin-gonic/gin"
)

// RagRouter 文档问答路由（内置预置型 RAG），挂 JWT 鉴权
func RagRouter(r *gin.RouterGroup) {
	ragGroup := r.Group("/rag")
	ragGroup.Use(jwt.Auth())
	ragGroup.POST("/chat", rag.RAGChat)
}
