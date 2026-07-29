package file

import (
	"log"
	"my-AIchat/common/code"
	"my-AIchat/controller"
	"my-AIchat/service/file"
	"net/http"

	"github.com/gin-gonic/gin"
)

type (
	UploadFileResponse struct {
		FilePath string `json:"file_path,omitempty"`
		controller.Response
	}
)

func UploadRagFile(c *gin.Context) {
	res := new(UploadFileResponse)
	uploadedFile, err := c.FormFile("file")
	if err != nil {
		log.Println("FormFile fail ", err)
		c.JSON(http.StatusOK, res.SetCode(code.CodeInvalidParams))
		return
	}

	username := c.GetString("userName")
	if username == "" {
		log.Println("Username not found in context")
		c.JSON(http.StatusOK, res.SetCode(code.CodeInvalidToken))
		return
	}

	//indexer 会在 service 层根据实际文件名创建
	filePath, err := file.UploadRagFile(username, uploadedFile)
	if err != nil {
		log.Println("UploadRagFile fail ", err)
		c.JSON(http.StatusOK, res.SetCode(code.CodeServerBusy))
		return
	}

	res.SetSuccess()
	res.FilePath = filePath
	c.JSON(http.StatusOK, res)
}
