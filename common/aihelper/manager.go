package aihelper

import (
	"context"
	"sync"
)

var ctx = context.Background()

// AIHelperManager AI助手管理器，管理 用户-会话-模型 三级映射的 AIHelper。
//
// 第三级按 modelType 区分：同一会话切换到不同模型（如普通对话 ⇄ RAG 文档问答）时会
// 重新创建对应的 AIHelper，而不是复用旧 helper 导致 RAG 不触发。
// 这样前端在「同一个会话」里切换模型也能正确拿到对应能力，无需强制新建会话。
type AIHelperManager struct {
	helpers map[string]map[string]map[string]*AIHelper // map[用户账号]map[会话ID]map[模型类型]*AIHelper
	mu      sync.RWMutex
}

// NewAIHelperManager 创建新的管理器实例
func NewAIHelperManager() *AIHelperManager {
	return &AIHelperManager{
		helpers: make(map[string]map[string]map[string]*AIHelper),
	}
}

// 获取或创建AIHelper
func (m *AIHelperManager) GetOrCreateAIHelper(userName string, sessionID string, modelType string, config map[string]interface{}) (*AIHelper, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 获取用户的会话映射
	userHelpers, exists := m.helpers[userName]
	if !exists {
		userHelpers = make(map[string]map[string]*AIHelper)
		m.helpers[userName] = userHelpers
	}

	// 获取该会话的模型映射
	sessionHelpers, exists := userHelpers[sessionID]
	if !exists {
		sessionHelpers = make(map[string]*AIHelper)
		userHelpers[sessionID] = sessionHelpers
	}

	// 检查 (会话, 模型) 是否已存在：存在则复用；否则按当前模型类型重建（切模型即重建）
	helper, exists := sessionHelpers[modelType]
	if exists {
		return helper, nil
	}

	// 创建新的AIHelper
	factory := GetGlobalFactory()
	helper, err := factory.CreateAIHelper(ctx, modelType, sessionID, config)
	if err != nil {
		return nil, err
	}

	sessionHelpers[modelType] = helper
	return helper, nil
}

// 获取指定用户的指定会话、指定模型的AIHelper
func (m *AIHelperManager) GetAIHelper(userName string, sessionID string, modelType string) (*AIHelper, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	userHelpers, exists := m.helpers[userName]
	if !exists {
		return nil, false
	}
	sessionHelpers, exists := userHelpers[sessionID]
	if !exists {
		return nil, false
	}
	helper, exists := sessionHelpers[modelType]
	return helper, exists
}

// 移除指定用户的指定会话下所有模型的AIHelper
func (m *AIHelperManager) RemoveAIHelper(userName string, sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	userHelpers, exists := m.helpers[userName]
	if !exists {
		return
	}

	delete(userHelpers, sessionID)

	// 如果用户没有会话了，清理用户映射
	if len(userHelpers) == 0 {
		delete(m.helpers, userName)
	}
}

// 获取指定用户的所有会话ID（按 sessionID 维度返回，与模型类型无关）
func (m *AIHelperManager) GetUserSessions(userName string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	userHelpers, exists := m.helpers[userName]
	if !exists {
		return []string{}
	}

	sessionIDs := make([]string, 0, len(userHelpers))
	//取出所有的 key（均为 sessionID）
	for sessionID := range userHelpers {
		sessionIDs = append(sessionIDs, sessionID)
	}

	return sessionIDs
}

// 全局管理器实例
var globalManager *AIHelperManager
var once sync.Once

// GetGlobalManager 获取全局管理器实例
func GetGlobalManager() *AIHelperManager {
	once.Do(func() {
		globalManager = NewAIHelperManager()
	})
	return globalManager
}
