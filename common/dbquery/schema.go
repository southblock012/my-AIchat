package dbquery

import (
	"context"
	"fmt"
	"my-AIchat/common/mysql"
	"my-AIchat/config"
)

// ColumnInfo 单个字段的结构信息
type ColumnInfo struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Comment string `json:"comment"`
}

// TableInfo 单张表的结构信息
type TableInfo struct {
	Name    string       `json:"name"`
	Comment string       `json:"comment"`
	Columns []ColumnInfo `json:"columns"`
}

// LoadExternalSchema 从外部库的 information_schema 机械读取全部表/字段结构。
// 不依赖 LLM：只读取 TABLE_NAME/TABLE_COMMENT 与 COLUMNS 的字段名/类型/注释。
// TABLE_SCHEMA 用 DATABASE() 取当前连接默认库（DSN 中已指定库名）。
func LoadExternalSchema(ctx context.Context) ([]TableInfo, error) {
	if mysql.ExternalDB == nil {
		return nil, fmt.Errorf("external DB 未初始化（ExternalDB == nil），请先调用 mysql.InitExternalDB")
	}
	db := mysql.ExternalDB

	// 库名优先用配置显式指定（externalDBConfig.schema），未配置则回退到连接默认库。
	schemaName := config.GetConfig().ExternalDBConfig.Schema
	if schemaName == "" {
		schemaName = dbName(ctx)
	}

	type tableRow struct {
		TableName    string `gorm:"column:TABLE_NAME"`
		TableComment string `gorm:"column:TABLE_COMMENT"`
	}
	var tables []tableRow
	if err := db.WithContext(ctx).Raw(`
		SELECT TABLE_NAME, TABLE_COMMENT
		FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = ?
		  AND TABLE_TYPE = 'BASE TABLE'
		ORDER BY TABLE_NAME
	`, schemaName).Scan(&tables).Error; err != nil {
		return nil, fmt.Errorf("读取外部库表列表失败: %v", err)
	}

	result := make([]TableInfo, 0, len(tables))
	for _, t := range tables {
		type colRow struct {
			ColumnName    string `gorm:"column:COLUMN_NAME"`
			ColumnType    string `gorm:"column:COLUMN_TYPE"`
			ColumnComment string `gorm:"column:COLUMN_COMMENT"`
		}
		var cols []colRow
		if err := db.WithContext(ctx).Raw(`
			SELECT COLUMN_NAME, COLUMN_TYPE, COLUMN_COMMENT
			FROM information_schema.COLUMNS
			WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
			ORDER BY ORDINAL_POSITION
		`, schemaName, t.TableName).Scan(&cols).Error; err != nil {
			return nil, fmt.Errorf("读取表 %s 字段失败: %v", t.TableName, err)
		}

		ti := TableInfo{Name: t.TableName, Comment: t.TableComment}
		for _, c := range cols {
			ti.Columns = append(ti.Columns, ColumnInfo{
				Name:    c.ColumnName,
				Type:    c.ColumnType,
				Comment: c.ColumnComment,
			})
		}
		result = append(result, ti)
	}
	return result, nil
}
