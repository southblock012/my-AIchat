package usermemory

import (
	"strings"
	"time"

	"my-AIchat/common/mysql"
	"my-AIchat/model"
)

// SaveMemory 保存一条记忆；同 user+category+content 已存在则刷新时间（去重），否则插入
func SaveMemory(m *model.UserMemory) error {
	var existing model.UserMemory
	err := mysql.DB.Where("user_name = ? AND category = ? AND content = ?", m.UserName, m.Category, m.Content).First(&existing).Error
	if err == nil {
		return mysql.DB.Model(&existing).Updates(map[string]interface{}{
			"updated_at":   time.Now(),
			"access_count": existing.AccessCount + 1,
		}).Error
	}
	if m.Importance <= 0 {
		m.Importance = 3
	}
	// time.Time 零值会被 MySQL 驱动写成 0000-00-00，触发 NO_ZERO_DATE 报错；
	// LastAccessedAt 不是 GORM 约定字段，不会自动填充，必须显式赋值。
	if m.LastAccessedAt.IsZero() {
		m.LastAccessedAt = time.Now()
	}
	return mysql.DB.Create(m).Error
}

// GetActiveMemories 取该用户活跃记忆，按 importance 降序、最近更新降序，限制条数
func GetActiveMemories(userName string, limit int) ([]model.UserMemory, error) {
	var list []model.UserMemory
	// 模型无 is_active 字段，"有效记忆"由 gorm.DeletedAt 软删除自动过滤（deleted_at IS NULL）
	err := mysql.DB.Where("user_name = ?", userName).
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
