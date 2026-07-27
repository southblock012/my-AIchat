package aihelper

import (
	"context"
	"my-AIchat/common/rabbitmq"
	"my-AIchat/model"
	"my-AIchat/utils"
	"sync"

	"github.com/cloudwego/eino/schema"
)

// AIHelper AI助手结构体，包含消息历史和AI模型
type AIHelper struct {
	model    AIModel
	messages []*model.Message
	mu       sync.RWMutex
	//一个会话绑定一个AIHelper
	SessionID string
	saveFunc  func(*model.Message) (*model.Message, error)
}

// NewAIHelper 创建新的AIHelper实例
func NewAIHelper(model_ AIModel, SessionID string) *AIHelper {
	return &AIHelper{
		model:    model_,
		messages: make([]*model.Message, 0),
		// 异步经 RabbitMQ 持久化消息（保留原有链路，未改动）
		saveFunc: func(msg *model.Message) (*model.Message, error) {
			data := rabbitmq.GenerateMessageMQParam(msg.SessionID, msg.Content, msg.UserName, msg.IsUser)
			err := rabbitmq.RMQMessage.Publish(data)
			return msg, err
		},
		SessionID: SessionID,
	}
}

// addMessage 添加消息到内存中并调用自定义存储函数
func (a *AIHelper) AddMessage(Content string, UserName string, IsUser bool, Save bool) {
	userMsg := model.Message{
		SessionID: a.SessionID,
		Content:   Content,
		UserName:  UserName,
		IsUser:    IsUser,
	}
	a.messages = append(a.messages, &userMsg)
	if Save {
		a.saveFunc(&userMsg)
	}
}

// SaveMessage 保存消息到数据库（通过回调函数避免循环依赖）
// 通过传入func，自己调用外部的保存函数，即可支持同步异步等多种策略
func (a *AIHelper) SetSaveFunc(saveFunc func(*model.Message) (*model.Message, error)) {
	a.saveFunc = saveFunc
}

// GetMessages 获取所有消息历史
func (a *AIHelper) GetMessages() []*model.Message {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]*model.Message, len(a.messages))
	copy(out, a.messages)
	return out
}

// 同步生成
func (a *AIHelper) GenerateResponse(userName string, ctx context.Context, userQuestion string) (*model.Message, error) {

	//调用存储函数（同步写库）
	a.AddMessage(userQuestion, userName, true, true)

	// 从 Redis 上下文窗口取最近 N 条历史 + 当前问题拼成发给模型的 messages
	hist := loadContextWindow(a.SessionID)
	messages := utils.ConvertToSchemaMessages(hist)
	messages = append(messages, &schema.Message{Role: schema.User, Content: userQuestion})

	//注入长期记忆（prepend 一条 system message，带 Redis 缓存）
	if memSys := a.BuildMemorySystemMessage(userName); memSys != nil {
		messages = append([]*schema.Message{memSys}, messages...)
	}

	//调用模型生成回复
	schemaMsg, err := a.model.GenerateResponse(ctx, messages)
	if err != nil {
		return nil, err
	}

	//将schema.Message转化成model.Message
	modelMsg := utils.ConvertToModelMessage(a.SessionID, userName, schemaMsg)

	//调用存储函数（同步写库）
	a.AddMessage(modelMsg.Content, userName, false, true)

	//更新 Redis 上下文窗口（追加本轮 user+assistant，内部截断到最近 N 条）
	hist = append(hist,
		&model.Message{SessionID: a.SessionID, UserName: userName, Content: userQuestion, IsUser: true},
		&model.Message{SessionID: a.SessionID, UserName: userName, Content: modelMsg.Content, IsUser: false},
	)
	_ = saveContextWindow(a.SessionID, hist)

	//最小闭环：后台抽取本轮记忆（goroutine 不阻塞回复；生产建议改 RabbitMQ 异步队列）
	go func() {
		defer func() { _ = recover() }()
		a.ExtractAndSaveMemories(ctx, userName, userQuestion, modelMsg.Content)
	}()

	return modelMsg, nil
}

// 流式生成
func (a *AIHelper) StreamResponse(userName string, ctx context.Context, cb StreamCallback, userQuestion string) (*model.Message, error) {

	//调用存储函数（同步写库）
	a.AddMessage(userQuestion, userName, true, true)

	// 从 Redis 上下文窗口取最近 N 条历史 + 当前问题拼成发给模型的 messages
	hist := loadContextWindow(a.SessionID)
	messages := utils.ConvertToSchemaMessages(hist)
	messages = append(messages, &schema.Message{Role: schema.User, Content: userQuestion})

	//注入长期记忆（prepend 一条 system message，带 Redis 缓存）
	if memSys := a.BuildMemorySystemMessage(userName); memSys != nil {
		messages = append([]*schema.Message{memSys}, messages...)
	}

	content, err := a.model.StreamResponse(ctx, messages, cb)
	if err != nil {
		return nil, err
	}
	//转化成model.Message
	modelMsg := &model.Message{
		SessionID: a.SessionID,
		UserName:  userName,
		Content:   content,
		IsUser:    false,
	}

	//调用存储函数（同步写库）
	a.AddMessage(modelMsg.Content, userName, false, true)

	//更新 Redis 上下文窗口（追加本轮 user+assistant，内部截断到最近 N 条）
	hist = append(hist,
		&model.Message{SessionID: a.SessionID, UserName: userName, Content: userQuestion, IsUser: true},
		&model.Message{SessionID: a.SessionID, UserName: userName, Content: modelMsg.Content, IsUser: false},
	)
	_ = saveContextWindow(a.SessionID, hist)

	//最小闭环：后台抽取本轮记忆
	go func() {
		defer func() { _ = recover() }()
		a.ExtractAndSaveMemories(ctx, userName, userQuestion, modelMsg.Content)
	}()

	return modelMsg, nil
}

// GetModelType 获取模型类型
func (a *AIHelper) GetModelType() string {
	return a.model.GetModelType()
}
