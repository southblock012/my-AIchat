package session

import (
	"my-AIchat/common/mysql"
	"my-AIchat/model"
)

func GetSessionsByUserName(userName string) ([]model.Session, error) {
	var sessions []model.Session
	// 仅查未删除的会话，按最近更新时间倒序（最新的排在最前）
	err := mysql.DB.
		Where("user_name = ? AND deleted_at IS NULL", userName).
		Order("updated_at DESC").
		Find(&sessions).Error
	return sessions, err
}

func CreateSession(session *model.Session) (*model.Session, error) {
	err := mysql.DB.Create(session).Error
	return session, err
}

func GetSessionByID(sessionID string) (*model.Session, error) {
	var session model.Session
	err := mysql.DB.Where("id = ?", sessionID).First(&session).Error
	return &session, err
}
