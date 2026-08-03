package aihelper

import (
	"context"
	"fmt"
	"io"
	"log"
	"my-AIchat/common/rag"
	"my-AIchat/config"
	"os"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type StreamCallback func(msg string)

// AIModel 定义AI模型接口
type AIModel interface {
	GenerateResponse(ctx context.Context, messages []*schema.Message) (*schema.Message, error)
	StreamResponse(ctx context.Context, messages []*schema.Message, cb StreamCallback) (string, error)
	GetModelType() string
}

// =================== OpenAI 实现 ===================
type OpenAIModel struct {
	llm model.ToolCallingChatModel
}

func NewOpenAIModel(ctx context.Context) (*OpenAIModel, error) {
	key := os.Getenv("OPENAI_API_KEY")
	modelName := os.Getenv("OPENAI_MODEL_NAME")
	baseURL := os.Getenv("OPENAI_BASE_URL")

	llm, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: baseURL,
		Model:   modelName,
		APIKey:  key,
	})
	if err != nil {
		return nil, fmt.Errorf("create openai model failed: %v", err)
	}
	return &OpenAIModel{llm: llm}, nil
}

func (o *OpenAIModel) GenerateResponse(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	resp, err := o.llm.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("openai generate failed: %v", err)
	}
	return resp, nil
}

func (o *OpenAIModel) StreamResponse(ctx context.Context, messages []*schema.Message, cb StreamCallback) (string, error) {
	stream, err := o.llm.Stream(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("openai stream failed: %v", err)
	}
	defer stream.Close()

	var fullResp strings.Builder

	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("openai stream recv failed: %v", err)
		}
		if len(msg.Content) > 0 {
			fullResp.WriteString(msg.Content) // 聚合

			cb(msg.Content) // 实时调用cb函数，方便主动发送给前端
		}
	}

	return fullResp.String(), nil //返回完整内容，方便后续存储
}

func (o *OpenAIModel) GetModelType() string { return "openai" }

// =================== Ollama 实现 ===================

// OllamaModel Ollama模型实现
type OllamaModel struct {
	llm model.ToolCallingChatModel
}

func NewOllamaModel(ctx context.Context, baseURL, modelName string) (*OllamaModel, error) {
	llm, err := ollama.NewChatModel(ctx, &ollama.ChatModelConfig{
		BaseURL: baseURL,
		Model:   modelName,
	})
	if err != nil {
		return nil, fmt.Errorf("create ollama model failed: %v", err)
	}
	return &OllamaModel{llm: llm}, nil
}

func (o *OllamaModel) GenerateResponse(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	resp, err := o.llm.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("ollama generate failed: %v", err)
	}
	return resp, nil
}

func (o *OllamaModel) StreamResponse(ctx context.Context, messages []*schema.Message, cb StreamCallback) (string, error) {
	stream, err := o.llm.Stream(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("ollama stream failed: %v", err)
	}
	defer stream.Close()
	var fullResp strings.Builder
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("openai stream recv failed: %v", err)
		}
		if len(msg.Content) > 0 {
			fullResp.WriteString(msg.Content) // 聚合
			cb(msg.Content)                   // 实时调用cb函数，方便主动发送给前端
		}
	}
	return fullResp.String(), nil //返回完整内容，方便后续存储
}

func (o *OllamaModel) GetModelType() string { return "ollama" }

// =================== RAG 实现 ===================
type AliRAGModel struct {
	llm      model.ToolCallingChatModel
	username string // 用于获取用户的文档
}

func NewAliRAGModel(ctx context.Context, username string) (*AliRAGModel, error) {
	// 聊天走 dashscope（与 embedding 同一供应商，统一用 EMBEDDING_API_KEY）
	apiKey := os.Getenv("EMBEDDING_API_KEY")
	conf := config.GetConfig()
	modelName := conf.RagModelConfig.RagChatModelName
	baseURL := conf.RagModelConfig.RagBaseUrl

	llm, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: baseURL,
		Model:   modelName,
		APIKey:  apiKey,
	})
	if err != nil {
		return nil, fmt.Errorf("create rag chat model failed: %v", err)
	}
	return &AliRAGModel{
		llm:      llm,
		username: username,
	}, nil
}

// GenerateResponse 同步生成：先尝试检索用户文档，命中则用 RAG 提示词替换最后一条消息；
// 用户未上传文档或检索失败时降级为普通对话（不会报错中断）。
func (o *AliRAGModel) GenerateResponse(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	ragQuery, err := rag.NewRAGQuery(ctx, o.username)
	if err != nil {
		// 用户没有上传文档，直接走普通对话
		log.Printf("RAG: no document for user %s, fallback to normal chat: %v", o.username, err)
		resp, err := o.llm.Generate(ctx, messages)
		if err != nil {
			return nil, fmt.Errorf("rag model generate failed: %v", err)
		}
		return resp, nil
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}
	lastMessage := messages[len(messages)-1]
	query := lastMessage.Content

	docs, err := ragQuery.RetrieveDocumentsEnhanced(ctx, query)
	if err != nil {
		log.Printf("RAG: retrieve failed, fallback to normal chat: %v", err)
		resp, err := o.llm.Generate(ctx, messages)
		if err != nil {
			return nil, fmt.Errorf("rag model generate failed: %v", err)
		}
		return resp, nil
	}

	ragPrompt := rag.BuildRAGPrompt(query, docs)

	ragMessages := make([]*schema.Message, len(messages))
	copy(ragMessages, messages)
	ragMessages[len(ragMessages)-1] = &schema.Message{
		Role:    schema.User,
		Content: ragPrompt,
	}

	resp, err := o.llm.Generate(ctx, ragMessages)
	if err != nil {
		return nil, fmt.Errorf("rag model generate failed: %v", err)
	}
	return resp, nil
}

// StreamResponse 流式生成：检索逻辑同 GenerateResponse，命中则替换最后一条消息为 RAG 提示词后流式输出。
func (o *AliRAGModel) StreamResponse(ctx context.Context, messages []*schema.Message, cb StreamCallback) (string, error) {
	ragQuery, err := rag.NewRAGQuery(ctx, o.username)
	if err != nil {
		log.Printf("RAG: no document for user %s, fallback to normal chat: %v", o.username, err)
		return o.streamWith(ctx, messages, cb)
	}

	if len(messages) == 0 {
		return "", fmt.Errorf("no messages provided")
	}
	lastMessage := messages[len(messages)-1]
	query := lastMessage.Content

	docs, err := ragQuery.RetrieveDocumentsEnhanced(ctx, query)
	if err != nil {
		log.Printf("RAG: retrieve failed, fallback to normal chat: %v", err)
		return o.streamWith(ctx, messages, cb)
	}

	ragPrompt := rag.BuildRAGPrompt(query, docs)
	ragMessages := make([]*schema.Message, len(messages))
	copy(ragMessages, messages)
	ragMessages[len(ragMessages)-1] = &schema.Message{
		Role:    schema.User,
		Content: ragPrompt,
	}

	return o.streamWith(ctx, ragMessages, cb)
}

// streamWith 调用底层 LLM 流式输出（带/不带 RAG 提示词共用）。
func (o *AliRAGModel) streamWith(ctx context.Context, messages []*schema.Message, cb StreamCallback) (string, error) {
	stream, err := o.llm.Stream(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("rag model stream failed: %v", err)
	}
	defer stream.Close()

	var fullResp strings.Builder
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("rag model stream recv failed: %v", err)
		}
		if len(msg.Content) > 0 {
			fullResp.WriteString(msg.Content)
			cb(msg.Content)
		}
	}
	return fullResp.String(), nil
}

// GetModelType 返回模型语义名称。
func (o *AliRAGModel) GetModelType() string { return "rag" }
