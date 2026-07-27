package model

import (
	"time"

	"gorm.io/gorm"
)

// 记忆类别
const (
	MemoryCategoryFact       = "fact"       // 事实
	MemoryCategoryPreference = "preference" // 偏好
	MemoryCategoryProject    = "project"    // 项目/工作
	MemoryCategoryConclusion = "conclusion" // 结论
	MemoryCategoryAvoid      = "avoid"      // 禁忌
)

// 记忆状态：0 生效 / 1 待确认（低权重，多次出现才升权）/ 2 已失效（被新记忆推翻）
const (
	MemoryStatusActive      int8 = 0
	MemoryStatusPending     int8 = 1
	MemoryStatusInvalidated int8 = 2
)

// UserMemory 长期用户记忆条目（跨会话有效）
type UserMemory struct {
	ID             uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	UserName       string         `gorm:"type:varchar(50);index;not null" json:"username"`
	Category       string         `gorm:"type:varchar(20);not null" json:"category"`
	Content        string         `gorm:"type:text;not null" json:"content"`
	Importance     int            `gorm:"default:3" json:"importance"`
	Source         string         `gorm:"type:varchar(36)" json:"source"` // 来源 sessionID 或 "manual"
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`                 // 支持软删除
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	LastAccessedAt time.Time      `json:"last_accessed_at"`
	AccessCount    int            `gorm:"default:0" json:"access_count"`
	Status         int8           `gorm:"default:0" json:"status"` // 0 active / 1 pending / 2 invalidated
}
