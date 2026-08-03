package dbquery

import (
	"context"
	"fmt"
	"strings"
	"time"

	"my-AIchat/config"

	"gorm.io/gorm"
)

// ExecSQL 在外部库上执行一条已通过 SanitizeSQL 规整的只读 SQL。
//   - 用 gorm Raw + Rows 把结果扫描成 []map[string]interface{}（列名→值）；
//   - 最多返回 maxRows 行，超出则 truncated=true，避免把超大结果集塞给 LLM；
//   - 调用方应传入带超时的 ctx（见 RunQuery 里的 QueryTimeoutSeconds）。
//
// 任何执行错误都会原样返回，由上层决定是否触发自愈修正。
func ExecSQL(ctx context.Context, db *gorm.DB, sql string, maxRows int) (rows []map[string]interface{}, truncated bool, err error) {
	if db == nil {
		return nil, false, fmt.Errorf("外部数据库连接未初始化（请检查 externalDBConfig）")
	}
	if maxRows <= 0 {
		maxRows = 100
	}

	rawRows, err := db.WithContext(ctx).Raw(sql).Rows()
	if err != nil {
		return nil, false, err
	}
	defer rawRows.Close()

	cols, err := rawRows.Columns()
	if err != nil {
		return nil, false, err
	}

	out := make([]map[string]interface{}, 0, maxRows)
	for rawRows.Next() {
		if len(out) >= maxRows {
			truncated = true
			break
		}
		// 每列一个 *interface{} 接收，再转成可读类型
		holders := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range holders {
			ptrs[i] = &holders[i]
		}
		if err := rawRows.Scan(ptrs...); err != nil {
			return nil, false, err
		}
		row := make(map[string]interface{}, len(cols))
		for i, c := range cols {
			v := holders[i]
			// []byte 转 string，否则 json 会 base64 编码、可读性差
			if b, ok := v.([]byte); ok {
				v = string(b)
			}
			row[c] = v
		}
		out = append(out, row)
	}
	if err := rawRows.Err(); err != nil {
		return nil, false, err
	}
	return out, truncated, nil
}

// RowsToText 把查询结果格式化为 markdown 表格，用于喂给总结 LLM。
// markdown 表格更易读，也方便前端直接渲染成 HTML 表格。
func RowsToText(rows []map[string]interface{}, truncated bool) string {
	if len(rows) == 0 {
		return "(查询无结果 / 没有匹配的数据)"
	}
	// 取首行的键顺序作为列顺序
	cols := make([]string, 0, len(rows[0]))
	for k := range rows[0] {
		cols = append(cols, k)
	}

	var b strings.Builder
	// 表头
	b.WriteString("| ")
	b.WriteString(strings.Join(cols, " | "))
	b.WriteString(" |\n")
	// 分隔符
	b.WriteString("| ")
	sepParts := make([]string, len(cols))
	for i := range cols {
		sepParts[i] = strings.Repeat("-", 3)
	}
	b.WriteString(strings.Join(sepParts, " | "))
	b.WriteString(" |\n")
	// 数据行
	for _, r := range rows {
		b.WriteString("| ")
		parts := make([]string, 0, len(cols))
		for _, c := range cols {
			parts = append(parts, fmt.Sprintf("%v", r[c]))
		}
		b.WriteString(strings.Join(parts, " | "))
		b.WriteString(" |\n")
	}
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("(共 %d 行结果", len(rows)))
	if truncated {
		b.WriteString("，已截断，仅展示前 N 行)")
	} else {
		b.WriteString(")")
	}
	return b.String()
}

// QueryContext 生成一个带执行超时的 context（防慢查询拖垮服务）。
func QueryContext(ctx context.Context) (context.Context, context.CancelFunc) {
	sec := config.GetConfig().ExternalDBConfig.QueryTimeoutSeconds
	if sec <= 0 {
		sec = 10
	}
	return context.WithTimeout(ctx, time.Duration(sec)*time.Second)
}

// RunQuery 是「生成 SQL → 执行 → 失败自愈」的完整编排（纯数据层，不碰 LLM 总结）。
// 返回最终执行的 SQL 与结果文本；执行报错且自愈仍失败时返回 error。
func RunQuery(ctx context.Context, llm chatModel, db *gorm.DB, question, schemaText string, topK, maxRows int) (sql string, resultText string, err error) {
	raw, genErr := GenerateSQL(ctx, llm, question, schemaText)
	if genErr != nil {
		return "", "", genErr
	}
	sql, sanErr := SanitizeSQL(raw, maxRows)
	if sanErr != nil {
		return "", "", sanErr
	}

	qctx, cancel := QueryContext(ctx)
	defer cancel()
	rows, truncated, execErr := ExecSQL(qctx, db, sql, maxRows)
	if execErr == nil {
		return sql, RowsToText(rows, truncated), nil
	}

	// 自愈：让 LLM 根据错误修正 SQL，再试一次
	fixedRaw, fixErr := FixSQL(ctx, llm, question, schemaText, sql, execErr)
	if fixErr != nil {
		return sql, "", fmt.Errorf("SQL 执行失败且无法自愈: %w", execErr)
	}
	fixedSQL, sanErr2 := SanitizeSQL(fixedRaw, maxRows)
	if sanErr2 != nil {
		return sql, "", fmt.Errorf("自愈后 SQL 仍不合规: %w", sanErr2)
	}
	qctx2, cancel2 := QueryContext(ctx)
	defer cancel2()
	rows2, truncated2, execErr2 := ExecSQL(qctx2, db, fixedSQL, maxRows)
	if execErr2 != nil {
		return fixedSQL, "", fmt.Errorf("自愈后执行仍失败: %w", execErr2)
	}
	return fixedSQL, RowsToText(rows2, truncated2), nil
}
