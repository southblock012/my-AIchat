package router

import (
	"github.com/gin-gonic/gin"

	"my-AIchat/controller/session"
	"my-AIchat/middleware/jwt"
)

func AIRouter(r *gin.RouterGroup) {
	aiRouter := r.Group("/chat")
	aiRouter.Use(jwt.Auth())
	{
		aiRouter.GET("sessions", session.GetUserSessionsByUserName)
		aiRouter.POST("send-new-session", session.CreateSessionAndSendMessage)
		aiRouter.POST("send", session.ChatSend)
		aiRouter.POST("history", session.ChatHistory)

		// TTS相关接口
		//r.POST("tts", tts.CreateTTSTask)
		//r.GET("tts/query", tts.QueryTTSTask)

		aiRouter.POST("send-stream-new-session", session.CreateStreamSessionAndSendMessage)
		aiRouter.POST("send-stream", session.ChatStreamSend)
	}
}
