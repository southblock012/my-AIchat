package aihelper

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"my-AIchat/dao/usermemory"
	"my-AIchat/model"

	"github.com/cloudwego/eino/schema"
)

const extractPromptTemplate = `你是一个记忆提取器。请从下面这段用户与助手的对话中，提取值得长期记住的、关于用户本人的信息（跨会话有效）。
只提取明确表达的事实/偏好/项目/结论/禁忌，不要提取临时闲聊。
可选类别：fact(事实) / preference(偏好) / project(项目或工作) / conclusion(结论) / avoid(禁忌)。
请只输出 JSON 数组，格式：[{"category":"preference","content":"用户偏好直接给可运行代码","importance":4}]。没有可提取内容则输出 []。
不要输出任何解释文字，只输出 JSON。

对话：
用户：%s
助手：%s`

type extractItem struct {
	Category   string `json:"category"`
	Content    string `json:"content"`
	Importance int    `json:"importance"`
}

// ExtractAndSaveMemories 从一轮对话中抽取并保存记忆。
// 最小闭环：同步调用 LLM 抽取；由调用方决定是否用 goroutine 包裹以避免阻塞主回复。
func (a *AIHelper) ExtractAndSaveMemories(ctx context.Context, userName, userQuestion, aiResponse string) {
	prompt := fmt.Sprintf(extractPromptTemplate, userQuestion, aiResponse)
	resp, err := a.model.GenerateResponse(ctx, []*schema.Message{{Role: schema.User, Content: prompt}})
	if err != nil {
		log.Println("[memory] extract llm failed:", err)
		return
	}
	for _, it := range parseExtractJSON(resp.Content) {
		content := strings.TrimSpace(it.Content)
		if content == "" {
			continue
		}
		if it.Importance <= 0 || it.Importance > 5 {
			it.Importance = 3
		}
		if !isValidCategory(it.Category) {
			it.Category = model.MemoryCategoryFact
		}
		if err := usermemory.SaveMemory(&model.UserMemory{
			UserName:   userName,
			Category:   it.Category,
			Content:    content,
			Importance: it.Importance,
			Source:     a.SessionID,
		}); err != nil {
			log.Println("[memory] save failed:", err)
		}
	}
}

func isValidCategory(c string) bool {
	switch c {
	case model.MemoryCategoryFact, model.MemoryCategoryPreference, model.MemoryCategoryProject,
		model.MemoryCategoryConclusion, model.MemoryCategoryAvoid:
		return true
	}
	return false
}

func parseExtractJSON(raw string) []extractItem {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	var items []extractItem
	if err := json.Unmarshal([]byte(s), &items); err != nil {
		log.Println("[memory] parse json failed:", err)
		return nil
	}
	return items
}

// BuildMemorySystemMessage 检索用户长期记忆，拼成 system message；无记忆返回 nil
// 内部走 Redis 缓存的 profile（见 cache.go 的 GetMemoryProfile），避免每次 top-K 查 MySQL
func (a *AIHelper) BuildMemorySystemMessage(userName string) *schema.Message {
	prompt := GetMemoryProfile(userName)
	if prompt == "" {
		return nil
	}
	return &schema.Message{Role: schema.System, Content: prompt}
}
