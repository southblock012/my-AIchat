package usermemory

import (
	"strings"
	"time"

	"my-AIchat/common/mysql"
	"my-AIchat/model"
)

// SaveMemory 保存一条记忆；同 user+category+content 已存在则刷新（去重）。
// - 已存在且为 pending（待确认）：升级为 active 并取较高 importance（多次出现才生效）
// - 新记忆：importance<=2 标记为 pending（待确认），>=3 直接 active
func SaveMemory(m *model.UserMemory) error {
	var existing model.UserMemory
	err := mysql.DB.Where("user_name = ? AND category = ? AND content = ?", m.UserName, m.Category, m.Content).First(&existing).Error
	if err == nil {
		updates := map[string]interface{}{
			"updated_at":   time.Now(),
			"access_count": existing.AccessCount + 1,
		}
		if existing.Status == model.MemoryStatusPending {
			updates["status"] = model.MemoryStatusActive
			if m.Importance > existing.Importance {
				updates["importance"] = m.Importance
			}
		}
		return mysql.DB.Model(&existing).Updates(updates).Error
	}
	if m.Importance <= 0 {
		m.Importance = 3
	}
	// time.Time 零值会被 MySQL 驱动写成 0000-00-00，触发 NO_ZERO_DATE 报错；
	// LastAccessedAt 不是 GORM 约定字段，不会自动填充，必须显式赋值。
	if m.LastAccessedAt.IsZero() {
		m.LastAccessedAt = time.Now()
	}
	// 低权重记忆先置 pending（待确认），多次出现才升权为 active
	if m.Importance <= 2 {
		m.Status = model.MemoryStatusPending
	} else {
		m.Status = model.MemoryStatusActive
	}
	return mysql.DB.Create(m).Error
}

// GetActiveMemories 取该用户生效中的记忆（status=active，且未被软删），按 importance 降序、最近更新降序
func GetActiveMemories(userName string, limit int) ([]model.UserMemory, error) {
	var list []model.UserMemory
	err := mysql.DB.Where("user_name = ? AND status = ?", userName, model.MemoryStatusActive).
		Order("importance desc, updated_at desc").
		Limit(limit).Find(&list).Error
	return list, err
}

// GetMemoryPrompt 拼成注入 system prompt 的文本，无记忆返回空串
func GetMemoryPrompt(userName string, limit int) string {
	list, err := GetActiveMemories(userName, limit)
	if err != nil || len(list) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("以下是关于这个用户的长期信息（跨会话有效），回答时请自然参考，不要生硬提及：\n")
	for _, m := range list {
		b.WriteString("- ")
		b.WriteString(m.Content)
		b.WriteString("\n")
	}
	return b.String()
}

// DeleteMemory 用户主动删除自己的某条记忆
func DeleteMemory(id uint, userName string) error {
	return mysql.DB.Where("id = ? AND user_name = ?", id, userName).Delete(&model.UserMemory{}).Error
}

// GetMemoriesForExtract 取供 LLM 判断冲突的现有记忆（active + pending，不含已失效）
func GetMemoriesForExtract(userName string, limit int) ([]model.UserMemory, error) {
	var list []model.UserMemory
	err := mysql.DB.Where("user_name = ? AND status IN ?", userName,
		[]int8{model.MemoryStatusActive, model.MemoryStatusPending}).
		Order("importance desc, updated_at desc").
		Limit(limit).Find(&list).Error
	return list, err
}

// InvalidateMemory 将某条记忆标记为已失效（被新记忆推翻）；仅能操作该用户自己的记忆
func InvalidateMemory(userName string, id uint) error {
	return mysql.DB.Model(&model.UserMemory{}).
		Where("id = ? AND user_name = ?", id, userName).
		Update("status", model.MemoryStatusInvalidated).Error
}
