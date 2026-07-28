package rag

import (
	"context"
	"my-AIchat/common/aihelper"
	ragdao "my-AIchat/dao/rag"
	"sync"

	"github.com/cloudwego/eino/schema"
)

var (
	modelOnce   sync.Once
	sharedModel aihelper.AIModel
	modelErr    error
)

// RAGChat 内置预置型 RAG：检索内置知识 → 注入 system 指令 → 直接调模型。
// 完全不经过 AIHelper，因此不改动已有的记忆/上下文窗口逻辑。
func RAGChat(userName, question string) (string, error) {
	// 懒加载灌库（首次聊天时执行，此时 DB 已就绪）
	ragdao.SeedIfEmpty()

	// 复用项目中已有的模型工厂创建 DeepSeek 模型（从环境变量读配置）
	modelOnce.Do(func() {
		sharedModel, modelErr = aihelper.GetGlobalFactory().CreateAIModel(context.Background(), "1", nil)
	})
	if modelErr != nil {
		return "", modelErr
	}

	ctxText := ragdao.RetrieveContext(question, 4)
	sys := "你是 my-AIchat 项目的文档助手。只能根据下面【资料】回答用户问题；" +
		"如果资料中没有相关信息，请明确说明你不知道，不要编造任何内容。\n\n【资料】\n" + ctxText

	msgs := []*schema.Message{
		{Role: schema.System, Content: sys},
		{Role: schema.User, Content: question},
	}
	resp, err := sharedModel.GenerateResponse(context.Background(), msgs)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}
