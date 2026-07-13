package router

import (
	"github.com/gin-gonic/gin"

	"my-AIchat/controller/session"
	"my-AIchat/controller/tts"
	"my-AIchat/middleware/jwt"
)

func AIRouter(r *gin.RouterGroup) {
	aiRouter := r.Group("/chat")
	aiRouter.Use(jwt.Auth())
	{
		r.GET("sessions", session.GetUserSessionsByUserName)
		r.POST("send-new-session", session.CreateSessionAndSendMessage)
		r.POST("send", session.ChatSend)
		r.POST("history", session.ChatHistory)

		// TTS相关接口
		r.POST("tts", tts.CreateTTSTask)
		r.GET("tts/query", tts.QueryTTSTask)

		r.POST("send-stream-new-session", session.CreateStreamSessionAndSendMessage)
		r.POST("send-stream", session.ChatStreamSend)
	}
}
