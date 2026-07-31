package aihelper

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"my-AIchat/common/dbquery"
	"my-AIchat/common/mysql"
	"my-AIchat/config"

	openaiModel "github.com/cloudwego/eino-ext/components/model/openai"
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// ExternalQueryModel 自然语言查库模型（modelType="4"）。
//
// 编排链路（完全不依赖业务库，只查外部库）：
//
//	用户问题 → 相关表检索(Weaviate混合+重排/关键词兜底) → 生成 SQL(LLM)
//	         → 只读护栏(SanitizeSQL) → 执行(Executor) → 失败自愈 → LLM 总结成自然语言
//
// 复用：DashScope chat 模型（与 RAG 同款、同密钥）、mysql.ExternalDB 连接、dbquery 包的全部能力。
type ExternalQueryModel struct {
	llm      einomodel.ToolCallingChatModel
	username string
	cat      *dbquery.Catalog
}

// NewExternalQueryModel 构造查库模型：建 chat 客户端 + 预热 catalog（复用 InitCatalog 的进程内缓存）。
func NewExternalQueryModel(ctx context.Context, username string) (*ExternalQueryModel, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	modelName := os.Getenv("OPENAI_MODEL_NAME")
	baseURL := os.Getenv("OPENAI_BASE_URL")
	conf := config.GetConfig()

	llm, err := openaiModel.NewChatModel(ctx, &openaiModel.ChatModelConfig{
		BaseURL:    baseURL,
		Model:      modelName,
		APIKey:     apiKey,
		HTTPClient: dashScopeHTTPClient(),
	})
	if err != nil {
		return nil, fmt.Errorf("创建查库 chat 模型失败: %v", err)
	}

	// 复用进程内已预热的 catalog；未预热则兜底读盘一次（仍失败则留 nil，运行时给友好提示）。
	cat := dbquery.GetCatalog()
	if cat == nil {
		cat, err = dbquery.LoadOrBuildCatalog(ctx, conf.ExternalDBConfig.Annotate, false)
		if err != nil {
			log.Printf("[external_query] 加载库表目录失败（运行时将提示未配置）: %v", err)
		}
	}

	return &ExternalQueryModel{llm: llm, username: username, cat: cat}, nil
}

// GetModelType 返回工厂注册号 "4"。
func (o *ExternalQueryModel) GetModelType() string { return "4" }

// questionFrom 取对话中最后一条用户消息作为查询问题。
func questionFrom(messages []*schema.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == schema.User {
			return strings.TrimSpace(messages[i].Content)
		}
	}
	return ""
}

// dashScopeHTTPClient 返回一个带较长 TLS 握手/总体超时的 HTTP 客户端，
// 用于连接 DashScope（容器到公网偶发 TLS handshake timeout 时增加容忍度）。
func dashScopeHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 60 * time.Second,
		Transport: &http.Transport{
			TLSHandshakeTimeout:   20 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
		},
	}
}

// summarizeMessages 构造「把查询结果总结成自然语言」的 prompt。
func (o *ExternalQueryModel) summarizeMessages(question, sql, resultText string) []*schema.Message {
	system := `你是数据库查询结果解读助手。请根据用户问题和查询结果，用简洁、口语化的中文回答用户。
要求：
1. 直接回答用户的问题，不要罗列原始数据；
2. 如果查询无结果，明确告知"没有查到相关数据"，不要编造；
3. 可以在必要时用一句话说明你依据的是哪些表/字段；
4. 不要输出 SQL 代码块（SQL 会另行展示）。`
	user := fmt.Sprintf("用户问题：%s\n\n执行的 SQL：\n%s\n\n查询结果：\n%s\n\n请用中文总结回答：",
		question, sql, resultText)
	return []*schema.Message{
		{Role: schema.System, Content: system},
		{Role: schema.User, Content: user},
	}
}

// GenerateResponse 同步生成：跑完整查库链路，返回自然语言总结。
func (o *ExternalQueryModel) GenerateResponse(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	sql, answer, err := o.run(ctx, messages)
	if err != nil {
		log.Printf("[external_query] 同步链路异常: %v", err)
		return &schema.Message{Role: schema.Assistant, Content: "服务暂时不可用，请稍后重试。"}, nil
	}
	_ = sql // 同步模式下 SQL 已并入 answer 文本展示
	return &schema.Message{Role: schema.Assistant, Content: answer}, nil
}

// StreamResponse 流式生成：先推送已执行的 SQL（透明展示），再流式输出自然语言总结。
func (o *ExternalQueryModel) StreamResponse(ctx context.Context, messages []*schema.Message, cb StreamCallback) (string, error) {
	sql, answer, err := o.run(ctx, messages)
	if err != nil {
		log.Printf("[external_query] 查库链路异常: %v", err)
		friendly := "服务暂时不可用，请稍后重试。"
		cb(friendly)
		return friendly, nil
	}
	// 先把执行的 SQL 推给前端，再流式输出总结
	if sql != "" {
		cb("执行 SQL:\n```sql\n" + sql + "\n```\n\n")
	}
	full, err := o.streamText(ctx, o.summarizeMessages(questionFrom(messages), sql, answer), cb)
	if err != nil {
		log.Printf("[external_query] 流式总结失败: %v", err)
		friendly := "模型响应超时或网络异常，已为你执行 SQL，请稍后重试。"
		cb(friendly)
		return friendly, nil
	}
	return full, nil
}

// run 是查库链路的公共实现：检索 → 生成 → 执行(自愈) → 拼装「SQL + 结果文本」。
// 失败时返回 (sql, friendlyAnswer, nilError) 而非 error，保证聊天不中断；仅在完全不可用时返回 error。
func (o *ExternalQueryModel) run(ctx context.Context, messages []*schema.Message) (sql string, answer string, err error) {
	question := questionFrom(messages)
	if question == "" {
		return "", "未识别到您的问题，请重新描述。", nil
	}
	if mysql.ExternalDB == nil {
		return "", "当前未配置外部数据库连接（externalDBConfig），无法执行自然语言查库。", nil
	}
	if o.cat == nil {
		return "", "尚未加载外部库表结构（可能外部库不可达），暂无法查询。", nil
	}

	conf := config.GetConfig().ExternalDBConfig
	topK := conf.TopK
	if topK <= 0 {
		topK = 5
	}
	maxRows := conf.MaxRows
	if maxRows <= 0 {
		maxRows = 100
	}

	// 1) 相关表检索
	tables := dbquery.RetrieveRelevantTables(ctx, question, o.cat, topK)
	if len(tables) == 0 {
		return "", "没能从外部库中匹配到相关表，请换个说法或确认表结构。", nil
	}
	schemaText := dbquery.SchemaPromptText(tables)

	// 2) 生成 SQL → 3) 执行(含自愈)
	sql, resultText, runErr := dbquery.RunQuery(ctx, o.llm, mysql.ExternalDB, question, schemaText, topK, maxRows)
	if runErr != nil {
		log.Printf("[external_query] 查询失败: %v", runErr)
		return sql, fmt.Sprintf("抱歉，查询执行失败：%s", runErr.Error()), nil
	}

	// 4) LLM 总结（失败则退化为直接展示结果文本，不让聊天整体挂掉）
	sumMsgs := o.summarizeMessages(question, sql, resultText)
	summary, sumErr := o.llm.Generate(ctx, sumMsgs)
	if sumErr != nil {
		log.Printf("[external_query] 总结失败，退化为原始结果: %v", sumErr)
		return sql, fmt.Sprintf("执行 SQL:\n```sql\n%s\n```\n\n查询结果：\n%s", sql, resultText), nil
	}
	return sql, summary.Content, nil
}

// streamText 调用底层 LLM 流式输出，逐块回调 cb，并返回完整内容。
func (o *ExternalQueryModel) streamText(ctx context.Context, messages []*schema.Message, cb StreamCallback) (string, error) {
	stream, err := o.llm.Stream(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("external query stream failed: %w", err)
	}
	defer stream.Close()

	var full strings.Builder
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("external query stream recv failed: %w", err)
		}
		if len(msg.Content) > 0 {
			full.WriteString(msg.Content)
			cb(msg.Content)
		}
	}
	return full.String(), nil
}
