package jwt

import (
	"github.com/gin-gonic/gin"

	"log"
	"my-AIchat/common/code"
	"my-AIchat/controller"
	"my-AIchat/utils/myjwt"
	"net/http"
	"strings"
)

func Auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		res := controller.Response{}
		var token string
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		} else {
			// 兼容 URL 参数传 token
			token = c.Query("token")
		}

		if token == "" {
			c.JSON(http.StatusOK, res.SetCode(code.CodeInvalidToken))
			c.Abort()
			return
		}

		log.Println("token is ", token)
		claim, ok := myjwt.ParseToken(token)
		if !ok {
			c.JSON(http.StatusOK, res.SetCode(code.CodeInvalidToken))
			c.Abort()
			return
		}

		c.Set("userName", claim.Username)
		c.Next()
	}
}
