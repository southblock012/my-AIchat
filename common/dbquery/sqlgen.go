package dbquery

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// chatModel 是本项目调用 LLM 生成文本所需的最小接口（eino 的 ToolCallingChatModel 天然满足）。
// 用最小接口而非具体类型，保持 dbquery 与 aihelper 解耦。注意签名需与 eino 的
// ChatModel.Generate(ctx, msgs, ...Option) 完全一致，否则具体类型无法满足该接口。
type chatModel interface {
	Generate(ctx context.Context, msgs []*schema.Message, opts ...model.Option) (*schema.Message, error)
}

// allowedSQLPrefix 只读语句允许的首个关键字（不区分大小写）。
// 这是「白名单」式护栏：只有这些前缀才放行，其余一律拒绝，从语法层面杜绝写操作。
var allowedSQLPrefix = regexp.MustCompile(`(?i)^\s*(SELECT|SHOW|DESCRIBE|DESC|WITH|EXPLAIN)\b`)

// hasLimitClause 判断 SQL 是否已有 LIMIT。
var hasLimitClause = regexp.MustCompile(`(?i)\blimit\s+\d+`)

// GenerateSQL 调用 LLM 把自然语言问题转成 SQL（返回带 ```sql 围栏的原始文本）。
// schemaText 来自 SchemaPromptText，只暴露相关表结构，不暴露行数据。
func GenerateSQL(ctx context.Context, llm chatModel, question, schemaText string) (string, error) {
	system := `你是一个严谨的 MySQL 专家。下面会给你若干张相关表的结构（表名、字段名、类型、注释），
请根据用户的自然语言问题，生成一条正确的 MySQL 查询语句。

严格要求：
1. 只能生成只读查询：SELECT / SHOW / DESCRIBE / WITH / EXPLAIN。绝不允许 INSERT/UPDATE/DELETE/DROP/ALTER/TRUNCATE 等写操作。
2. 只使用上方给出的表与字段，不要臆造不存在的表或列。
3. 只输出一个 SQL 语句，用 ` + "```sql" + ` 代码块包裹，不要任何解释性文字。
4. 若问题无法用给定表结构回答，输出 ` + "```sql" + ` 注释说明无法回答的原因。`

	user := fmt.Sprintf("相关表结构：\n%s\n\n用户问题：%s\n\n请生成 SQL：", schemaText, question)

	resp, err := llm.Generate(ctx, []*schema.Message{
		{Role: schema.System, Content: system},
		{Role: schema.User, Content: user},
	})
	if err != nil {
		return "", fmt.Errorf("生成 SQL 失败: %w", err)
	}
	return resp.Content, nil
}

// SanitizeSQL 校验并规整 LLM 产出的 SQL：
//  1. 去除 ```sql 围栏与首尾空白；
//  2. 白名单校验（只允许只读前缀），否则返回错误；
//  3. 拒绝多条语句（含内部分号）；
//  4. 对 SELECT / WITH 类查询，若未显式 LIMIT，则强制追加 LIMIT maxRows，防止超大结果集。
//
// 返回规整后可安全执行的 SQL；任何违规都返回 error（由上层决定是否自愈）。
func SanitizeSQL(raw string, maxRows int) (string, error) {
	if maxRows <= 0 {
		maxRows = 100
	}
	sql := extractSQL(raw)
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return "", fmt.Errorf("SQL 为空")
	}

	// 去除可能存在的尾部分号，便于后续统一判断
	sqlNoSemi := strings.TrimRight(sql, "; \t\n\r")
	// 拒绝多条语句：去掉尾部分号后仍含分号 => 多语句
	if strings.Contains(sqlNoSemi, ";") {
		return "", fmt.Errorf("拒绝执行：不允许多条语句")
	}

	// 白名单前缀校验
	if !allowedSQLPrefix.MatchString(sqlNoSemi) {
		return "", fmt.Errorf("拒绝执行：只允许只读查询(SELECT/SHOW/DESCRIBE/WITH/EXPLAIN)，得到: %s", truncate(sqlNoSemi, 60))
	}

	// 对 SELECT / WITH 强制 LIMIT（SHOW/DESCRIBE/EXPLAIN 不加）
	upper := strings.ToUpper(sqlNoSemi)
	if (strings.HasPrefix(strings.TrimSpace(upper), "SELECT") || strings.HasPrefix(strings.TrimSpace(upper), "WITH")) &&
		!hasLimitClause.MatchString(sqlNoSemi) {
		sqlNoSemi = fmt.Sprintf("%s LIMIT %d", sqlNoSemi, maxRows)
	}

	return sqlNoSemi, nil
}

// FixSQL 在 SQL 执行报错时，让 LLM 根据错误和原 SQL 修正一条新 SQL（仍带围栏）。
// 用于「执行失败自愈」：最多重试一次，避免把错误直接抛给用户。
func FixSQL(ctx context.Context, llm chatModel, question, schemaText, badSQL string, execErr error) (string, error) {
	system := `你是一个 MySQL 专家。之前生成的 SQL 执行报错了，请根据错误信息和表结构修正它。
只输出修正后的单条只读 SQL（SELECT/SHOW/DESCRIBE/WITH/EXPLAIN），用 ` + "```sql" + ` 代码块包裹，不要解释。`

	user := fmt.Sprintf(`相关表结构：
%s

用户问题：%s

之前错误的 SQL：
%s

执行报错：
%s

请输出修正后的 SQL：`, schemaText, question, badSQL, execErr.Error())

	resp, err := llm.Generate(ctx, []*schema.Message{
		{Role: schema.System, Content: system},
		{Role: schema.User, Content: user},
	})
	if err != nil {
		return "", fmt.Errorf("自愈修正 SQL 失败: %w", err)
	}
	return resp.Content, nil
}

// extractSQL 从 LLM 返回文本中提取 ```sql ... ```（或 ``` ... ```）代码块内容；
// 若无围栏则原样返回（容错）。
func extractSQL(raw string) string {
	re := regexp.MustCompile("(?is)```(?:sql)?\\s*(.*?)```")
	if m := re.FindStringSubmatch(raw); m != nil {
		return m[1]
	}
	return raw
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
