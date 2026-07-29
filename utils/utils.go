package utils

import (
	"fmt"
	"math/rand"
	"mime/multipart"
	"my-AIchat/model"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
	"golang.org/x/crypto/bcrypt"
)

// BcryptGenerate 密码加密
func BcryptGenerate(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// BcryptCompare 密码对比
func BcryptCompare(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

// GenerateRandomID 生成随机位数ID
func GenerateRandomID(num int) string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	code := ""
	for i := 0; i < num; i++ {
		// 0~9随机数
		digit := r.Intn(10)
		code += strconv.Itoa(digit)
	}
	return code
}

// 将 schema 消息转换为数据库可存储的格式
func ConvertToModelMessage(sessionID string, userName string, msg *schema.Message) *model.Message {
	return &model.Message{
		SessionID: sessionID,
		UserName:  userName,
		Content:   msg.Content,
	}
}

// 将数据库消息转换为 schema 消息（供 AI 使用）
func ConvertToSchemaMessages(msgs []*model.Message) []*schema.Message {
	schemaMsgs := make([]*schema.Message, 0, len(msgs))
	for _, m := range msgs {
		role := schema.Assistant
		if m.IsUser {
			role = schema.User
		}
		schemaMsgs = append(schemaMsgs, &schema.Message{
			Role:    role,
			Content: m.Content,
		})
	}
	return schemaMsgs
}

// ValidateFile 校验文件是否为允许的文本文件（.md 或 .txt）
func ValidateFile(file *multipart.FileHeader) error {
	// 校验文件扩展名
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext != ".md" && ext != ".txt" {
		return fmt.Errorf("文件类型不正确，只允许 .md 或 .txt 文件，当前扩展名: %s", ext)
	}
	// 校验文件大小
	if file.Size > 1024*1024*10 { // 10MB
		return fmt.Errorf("文件大小不能超过 10MB")
	}

	return nil
}
