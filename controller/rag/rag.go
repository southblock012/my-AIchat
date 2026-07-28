package rag

import (
	"net/http"

	"my-AIchat/common/code"
	"my-AIchat/controller"
	ragservice "my-AIchat/service/rag"

	"github.com/gin-gonic/gin"
)

type RAGChatRequest struct {
	Question string `json:"question" binding:"required"`
}

type RAGChatResponse struct {
	Answer string `json:"answer,omitempty"`
	controller.Response
}

// RAGChat 文档问答接口 handler
func RAGChat(c *gin.Context) {
	req := new(RAGChatRequest)
	res := new(RAGChatResponse)

	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusOK, res.SetCode(code.CodeServerBusy))
		return
	}

	userName := c.GetString("userName") // From JWT middleware
	answer, err := ragservice.RAGChat(userName, req.Question)
	if err != nil {
		c.JSON(http.StatusOK, res.SetCode(code.CodeServerBusy))
		return
	}

	res.SetSuccess()
	res.Answer = answer
	c.JSON(http.StatusOK, res)
}
