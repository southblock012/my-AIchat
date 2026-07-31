package dbquery

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"my-AIchat/common/mysql"
	"my-AIchat/config"
	"os"
	"time"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// catalogCacheFile schema 分析结果的缓存文件（相对程序工作目录）。
// 分析只在首次（或结构变更强制刷新时）发生，之后直接读缓存，省 token。
const catalogCacheFile = "schema_catalog.json"

// globalCatalog 进程内缓存的目录（InitCatalog 启动时写入），供查库模型直接复用，
// 避免每个会话都读盘。无锁安全：只在启动时写一次。
var globalCatalog *Catalog

// GetCatalog 返回已预热的目录；InitCatalog 未运行或失败时返回 nil。
func GetCatalog() *Catalog {
	return globalCatalog
}

// Catalog schema 的缓存结构（含 LLM 注解后的中文含义）。
type Catalog struct {
	DBName      string      `json:"dbName"`
	Tables      []TableInfo `json:"tables"`
	GeneratedAt time.Time   `json:"generatedAt"`
	Annotated   bool        `json:"annotated"` // 是否经过 LLM 含义补全
}

// dbName 取外部库当前库名（用于缓存标记）。
// 优先用配置显式指定的 schema；未配置才回退到连接默认库（SELECT DATABASE()）。
func dbName(ctx context.Context) string {
	if s := config.GetConfig().ExternalDBConfig.Schema; s != "" {
		return s
	}
	if mysql.ExternalDB == nil {
		return ""
	}
	var row struct {
		Name string `gorm:"column:Name"`
	}
	if err := mysql.ExternalDB.WithContext(ctx).Raw("SELECT DATABASE() AS Name").Scan(&row).Error; err != nil {
		return ""
	}
	return row.Name
}

// BuildCatalog 读取外部库结构并（按需）用 LLM 补全中文含义。
// annotate=true 时，仅对 COMMENT 缺失的表/字段调用 LLM 推断，已存在的 COMMENT 保留。
func BuildCatalog(ctx context.Context, annotate bool) (*Catalog, error) {
	tables, err := LoadExternalSchema(ctx)
	if err != nil {
		return nil, err
	}
	cat := &Catalog{
		DBName:      dbName(ctx),
		Tables:      tables,
		GeneratedAt: time.Now(),
	}

	if annotate {
		if err := annotateCatalog(ctx, cat); err != nil {
			// 注解失败不致命：保留原始（仅 COMMENT）结构，仍可使用。
			log.Printf("[dbquery] LLM 含义补全失败，使用原始结构继续: %v", err)
		} else {
			cat.Annotated = true
		}
	}
	return cat, nil
}

// needsAnnotation 判断该表是否仍有缺失的 COMMENT（表级或字段级）。
func needsAnnotation(t TableInfo) bool {
	if t.Comment == "" {
		return true
	}
	for _, c := range t.Columns {
		if c.Comment == "" {
			return true
		}
	}
	return false
}

// annotateCatalog 对缺失 COMMENT 的表调用一次 LLM，补全表用途与字段含义。
func annotateCatalog(ctx context.Context, cat *Catalog) error {
	// 找出需要补全的表
	pending := make([]*TableInfo, 0)
	for i := range cat.Tables {
		if needsAnnotation(cat.Tables[i]) {
			pending = append(pending, &cat.Tables[i])
		}
	}
	if len(pending) == 0 {
		return nil // 全部已有 COMMENT，无需调用 LLM
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("未设置 OPENAI_API_KEY，无法做 LLM 含义补全")
	}
	modelName := os.Getenv("OPENAI_MODEL_NAME")
	baseURL := os.Getenv("OPENAI_BASE_URL")

	llm, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: baseURL,
		Model:   modelName,
		APIKey:  apiKey,
	})
	if err != nil {
		return fmt.Errorf("创建注解用 LLM 客户端失败: %v", err)
	}

	for _, t := range pending {
		meaning, colMeanings, err := llmAnnotateTable(ctx, llm, *t)
		if err != nil {
			log.Printf("[dbquery] 表 %s 注解失败(跳过): %v", t.Name, err)
			continue
		}
		if t.Comment == "" && meaning != "" {
			t.Comment = meaning
		}
		for j := range t.Columns {
			if t.Columns[j].Comment == "" {
				if m, ok := colMeanings[t.Columns[j].Name]; ok && m != "" {
					t.Columns[j].Comment = m
				}
			}
		}
	}
	return nil
}

// llmAnnotateTable 让 LLM 返回一张表的中文用途 + 各字段含义（JSON）。
func llmAnnotateTable(ctx context.Context, llm model.ToolCallingChatModel, t TableInfo) (tableMeaning string, colMeanings map[string]string, err error) {
	colLines := ""
	for _, c := range t.Columns {
		colLines += fmt.Sprintf("  - %s %s\n", c.Name, c.Type)
	}
	prompt := fmt.Sprintf(`你正在分析一个 MySQL 数据库的表结构，请为下面这张表补全中文含义。
表名: %s
字段:
%s
请只输出一个 JSON 对象（不要任何解释性文字），格式如下：
{
  "table_meaning": "这张表的主要用途（一句话）",
  "columns": {
    "字段名": "该字段的含义"
  }
}`, t.Name, colLines)

	msgs := []*schema.Message{
		{Role: schema.System, Content: "你是资深数据库架构师，擅长根据英文表/字段名推断其业务含义，并用简洁中文描述。"},
		{Role: schema.User, Content: prompt},
	}
	resp, err := llm.Generate(ctx, msgs)
	if err != nil {
		return "", nil, err
	}

	var out struct {
		TableMeaning string            `json:"table_meaning"`
		Columns      map[string]string `json:"columns"`
	}
	if err := json.Unmarshal([]byte(resp.Content), &out); err != nil {
		return "", nil, fmt.Errorf("注解结果 JSON 解析失败: %v (raw=%s)", err, resp.Content)
	}
	return out.TableMeaning, out.Columns, nil
}

// LoadCatalogCache 从 schema_catalog.json 读取已缓存的目录。
func LoadCatalogCache() (*Catalog, error) {
	data, err := os.ReadFile(catalogCacheFile)
	if err != nil {
		return nil, err
	}
	cat := &Catalog{}
	if err := json.Unmarshal(data, cat); err != nil {
		return nil, err
	}
	return cat, nil
}

// SaveCatalogCache 将目录写入 schema_catalog.json。
func SaveCatalogCache(cat *Catalog) error {
	data, err := json.MarshalIndent(cat, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(catalogCacheFile, data, 0644)
}

// LoadOrBuildCatalog 优先读缓存；无缓存或 forceRefresh 时重新分析并写回缓存。
func LoadOrBuildCatalog(ctx context.Context, annotate, forceRefresh bool) (*Catalog, error) {
	if !forceRefresh {
		if cat, err := LoadCatalogCache(); err == nil {
			log.Printf("[dbquery] 命中 schema 缓存（%d 张表，annotated=%v）", len(cat.Tables), cat.Annotated)
			return cat, nil
		}
	}
	cat, err := BuildCatalog(ctx, annotate)
	if err != nil {
		return nil, err
	}
	if err := SaveCatalogCache(cat); err != nil {
		log.Printf("[dbquery] 写 schema 缓存失败(不影响功能): %v", err)
	} else {
		log.Printf("[dbquery] schema 已分析并缓存：%d 张表，annotated=%v", len(cat.Tables), cat.Annotated)
	}
	return cat, nil
}

// InitCatalog 启动期 best-effort 预热：分析外部库结构并缓存（只发生一次）。
// 失败仅日志返回，不阻断主流程。
func InitCatalog(ctx context.Context) error {
	annotate := config.GetConfig().ExternalDBConfig.Annotate
	cat, err := LoadOrBuildCatalog(ctx, annotate, false)
	if err != nil {
		return err
	}
	globalCatalog = cat // 写入进程内缓存，供查库模型直接复用
	// 最佳努力：把库表结构向量化写入 Weaviate，供后续语义检索（混合+重排）使用。
	// 失败仅日志，不阻断主流程；检索时会自动回退到关键词打分。
	if err := IndexSchema(ctx, cat); err != nil {
		log.Printf("[dbquery] 库表向量索引失败（将退化为关键词检索）: %v", err)
	}
	return nil
}
