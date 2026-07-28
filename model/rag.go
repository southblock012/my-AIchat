package model

import "time"

// RAGChunk RAG 内置知识库的切片（预置型：文档由项目方提前灌入）
type RAGChunk struct {
	ID         uint      `gorm:"primaryKey"`
	DocTitle   string    `gorm:"type:varchar(200)"`
	ChunkIndex int       `gorm:"index"`
	Content    string    `gorm:"type:text"`
	Source     string    `gorm:"type:varchar(200)"`
	CreatedAt  time.Time
}

// TableName 显式指定表名，避免 GORM 复数推断差异
func (RAGChunk) TableName() string {
	return "rag_chunks"
}
