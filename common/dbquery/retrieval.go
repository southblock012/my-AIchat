package dbquery

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"my-AIchat/common/rag"
	"my-AIchat/common/weaviate"
	"my-AIchat/config"

	openaiEmbedding "github.com/cloudwego/eino-ext/components/embedding/openai"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/schema"
	wv "github.com/weaviate/weaviate-go-client/v5/weaviate"
	wvgql "github.com/weaviate/weaviate-go-client/v5/weaviate/graphql"
	wvmodels "github.com/weaviate/weaviate/entities/models"
)

// schemaClassName 是 Weaviate 中存放「库表结构」向量的 class 名（必须匹配 ^[A-Z][a-zA-Z0-9]*）。
// 每次重建目录（InitCatalog）时整体删除重建，保证索引与当前 catalog 完全一致。
const schemaClassName = "DbTableSchema"

// embeddingBatchSize 是 DashScope text-embedding-v4 单次请求的 contents 上限（超过返回 400）。
const embeddingBatchSize = 10

// weaviateBatchSize 是单次批量写入 Weaviate 的对象数（保守分批，避免超大 schema 一次性写入失败）。
const weaviateBatchSize = 100

// ---- 对外主入口 ----

// RetrieveRelevantTables 根据用户自然语言问题，从目录中挑选最相关的若干张表。
//
// 实现策略（仿照项目 RAG）：
//  1. 优先走 Weaviate 向量 + BM25 混合检索（hybrid），再用百炼 cross-encoder 重排精排；
//  2. 当 Weaviate / embedding 不可用（未配置、服务宕机）时，自动回退到轻量关键词打分，
//     保证「自然语言查库」功能不整体中断。
//
// 为了对问题进行向量化，这里需要 ctx；Phase 3 调用方传入即可。
func RetrieveRelevantTables(ctx context.Context, question string, cat *Catalog, topK int) []TableInfo {
	if cat == nil {
		return nil
	}
	if topK <= 0 {
		topK = 10
	}

	tables, err := retrieveByVector(ctx, question, cat, topK)
	if err != nil {
		log.Printf("[dbquery] 向量检索失败，回退关键词检索: %v", err)
		return lexicalRetrieve(question, cat, topK)
	}
	if len(tables) == 0 {
		log.Printf("[dbquery] 向量检索无结果，回退关键词检索")
		return lexicalRetrieve(question, cat, topK)
	}
	return tables
}

// ---- 向量检索链路（混合 + 重排） ----

// getWeaviateClient 返回全局 Weaviate 客户端；未初始化时惰性初始化。
func getWeaviateClient() *wv.Client {
	if weaviate.WeaviateClient == nil {
		weaviate.InitWeaviate()
	}
	return weaviate.WeaviateClient
}

// newEmbedder 构造一个 OpenAI 兼容的 embedding 客户端（接入 DashScope，天然兼容 text-embedding-v4）。
// 与 common/rag 共用同一套配置与密钥（RagModelConfig + EMBEDDING_API_KEY）。
func newEmbedder(ctx context.Context, model string) (embedding.Embedder, error) {
	cfg := config.GetConfig().RagModelConfig
	ec := &openaiEmbedding.EmbeddingConfig{
		BaseURL: cfg.RagBaseUrl,
		APIKey:  os.Getenv("EMBEDDING_API_KEY"),
		Model:   model,
		Timeout: 30 * time.Second,
	}
	emb, err := openaiEmbedding.NewEmbedder(ctx, ec)
	if err != nil {
		return nil, fmt.Errorf("创建 embedding 客户端失败: %w", err)
	}
	return emb, nil
}

// tableDocText 把一张表的结构拼成一段「文档」文本，既用于建索引，也作为重排的输入。
func tableDocText(t TableInfo) string {
	var b strings.Builder
	b.WriteString("表名: ")
	b.WriteString(t.Name)
	if t.Comment != "" {
		b.WriteString(" (")
		b.WriteString(t.Comment)
		b.WriteString(")")
	}
	b.WriteString("\n字段:\n")
	for _, c := range t.Columns {
		b.WriteString("  - ")
		b.WriteString(c.Name)
		b.WriteString(" ")
		b.WriteString(c.Type)
		if c.Comment != "" {
			b.WriteString(" -- ")
			b.WriteString(c.Comment)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// embedInBatches 分批调用 embedding，避开 DashScope text-embedding-v4 单次
// input.contents 上限 10 条的限制（超过会返回 400 Bad Request）。
func embedInBatches(ctx context.Context, emb embedding.Embedder, texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	out := make([][]float64, 0, len(texts))
	for start := 0; start < len(texts); start += embeddingBatchSize {
		end := start + embeddingBatchSize
		if end > len(texts) {
			end = len(texts)
		}
		vs, err := emb.EmbedStrings(ctx, texts[start:end])
		if err != nil {
			return nil, err
		}
		out = append(out, vs...)
	}
	return out, nil
}

// IndexSchema 把整库表结构向量化并写入 Weaviate（覆盖式：先删旧 class 再重建）。
// 在 InitCatalog 启动时调用一次；失败仅返回 error，由上层 best-effort 忽略。
func IndexSchema(ctx context.Context, cat *Catalog) error {
	if cat == nil || len(cat.Tables) == 0 {
		return fmt.Errorf("catalog 为空，无法建索引")
	}
	client := getWeaviateClient()
	if client == nil {
		return fmt.Errorf("weaviate 客户端不可用，跳过库表向量索引")
	}
	emb, err := newEmbedder(ctx, config.GetConfig().RagModelConfig.RagEmbeddingModel)
	if err != nil {
		return fmt.Errorf("创建 embedding 客户端失败: %w", err)
	}

	texts := make([]string, 0, len(cat.Tables))
	for _, t := range cat.Tables {
		texts = append(texts, tableDocText(t))
	}
	vectors, err := embedInBatches(ctx, emb, texts)
	if err != nil {
		return fmt.Errorf("库表向量化失败: %w", err)
	}
	if len(vectors) != len(texts) {
		return fmt.Errorf("embedding 数量不一致: got %d want %d", len(vectors), len(texts))
	}

	// 覆盖式重建 class，保证索引与当前 catalog 完全一致
	_ = client.Schema().ClassDeleter().WithClassName(schemaClassName).Do(ctx)
	classObj := &wvmodels.Class{
		Class:           schemaClassName,
		Vectorizer:      "none", // 向量由我们在外部算好再传入
		VectorIndexType: "hnsw",
		Properties: []*wvmodels.Property{
			{Name: "tableName", DataType: []string{"text"}},
			{Name: "dbName", DataType: []string{"text"}},
			{Name: "content", DataType: []string{"text"}},
		},
	}
	if err := client.Schema().ClassCreator().WithClass(classObj).Do(ctx); err != nil {
		return fmt.Errorf("创建 schema class 失败: %w", err)
	}

	objs := make([]*wvmodels.Object, 0, len(cat.Tables))
	for i, t := range cat.Tables {
		objs = append(objs, &wvmodels.Object{
			Class: schemaClassName,
			Properties: map[string]interface{}{
				"tableName": t.Name,
				"dbName":    cat.DBName,
				"content":   texts[i],
			},
			Vector: toFloat32Slice(vectors[i]),
		})
	}
	for start := 0; start < len(objs); start += weaviateBatchSize {
		end := start + weaviateBatchSize
		if end > len(objs) {
			end = len(objs)
		}
		if _, err := client.Batch().ObjectsBatcher().WithObjects(objs[start:end]...).Do(ctx); err != nil {
			return fmt.Errorf("批量写入库表向量失败: %w", err)
		}
	}
	log.Printf("[dbquery] 已索引 %d 张表到 Weaviate(class=%s)", len(cat.Tables), schemaClassName)
	return nil
}

// retrieveByVector 混合检索 + 可选重排，返回与 cat 中完整 TableInfo 对应的相关表。
func retrieveByVector(ctx context.Context, question string, cat *Catalog, topK int) ([]TableInfo, error) {
	client := getWeaviateClient()
	if client == nil {
		return nil, fmt.Errorf("weaviate client 为 nil")
	}
	emb, err := newEmbedder(ctx, config.GetConfig().RagModelConfig.RagEmbeddingModel)
	if err != nil {
		return nil, err
	}
	qv, err := emb.EmbedStrings(ctx, []string{question})
	if err != nil {
		return nil, fmt.Errorf("问题向量化失败: %w", err)
	}
	queryVector := toFloat32Slice(qv[0])

	cfg := config.GetConfig().RagModelConfig
	alpha := cfg.RagHybridAlpha
	if alpha == 0 {
		alpha = 0.5
	}
	retrieveK := cfg.RagRetrieveK
	if retrieveK == 0 {
		retrieveK = 20
	}
	if retrieveK < topK {
		retrieveK = topK
	}

	hb := client.GraphQL().HybridArgumentBuilder().
		WithQuery(question).
		WithVector(queryVector).
		WithAlpha(float32(alpha))

	resp, err := client.GraphQL().Get().
		WithClassName(schemaClassName).
		WithFields(
			wvgql.Field{Name: "tableName"},
			wvgql.Field{Name: "dbName"},
			wvgql.Field{Name: "content"},
			wvgql.Field{Name: "_additional", Fields: []wvgql.Field{
				{Name: "id"},
				{Name: "score"},
			}},
		).
		WithHybrid(hb).
		WithLimit(retrieveK).
		Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("混合检索失败: %w", err)
	}

	cands, err := parseTableResponse(resp)
	if err != nil {
		return nil, err
	}
	if len(cands) == 0 {
		return nil, nil
	}

	// 配置了重排模型则做 cross-encoder 精排，否则按混合得分取前 topK
	if cfg.RagRerankModel != "" {
		if reranked, e := rerankTables(ctx, question, cands, topK); e != nil {
			log.Printf("[dbquery] 重排失败，退化为混合排序: %v", e)
		} else {
			return mapTables(cat, reranked), nil
		}
	}
	if len(cands) > topK {
		cands = cands[:topK]
	}
	return mapTables(cat, cands), nil
}

// tableCandidate 混合检索召回的一张候选表（名称 + 文本 + 综合得分）。
type tableCandidate struct {
	name    string
	content string
	score   float64
}

// parseTableResponse 解析 Weaviate 返回的库表对象。
func parseTableResponse(resp *wvmodels.GraphQLResponse) ([]tableCandidate, error) {
	if resp == nil {
		return nil, fmt.Errorf("empty weaviate response")
	}
	if len(resp.Errors) > 0 {
		return nil, fmt.Errorf("weaviate graphql error: %v", resp.Errors[0].Message)
	}
	if resp.Data == nil {
		return nil, fmt.Errorf("no data in response")
	}
	get, ok := resp.Data["Get"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("no Get field in response")
	}
	items, ok := get[schemaClassName].([]interface{})
	if !ok {
		return nil, fmt.Errorf("class %s 不在响应中（可能尚未建索引，请先 InitCatalog）", schemaClassName)
	}
	cands := make([]tableCandidate, 0, len(items))
	for _, it := range items {
		obj, ok := it.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := obj["tableName"].(string)
		if name == "" {
			continue
		}
		content, _ := obj["content"].(string)
		score := 0.0
		if add, ok := obj["_additional"].(map[string]interface{}); ok {
			if s, ok := add["score"].(float64); ok {
				score = s
			}
		}
		cands = append(cands, tableCandidate{name: name, content: content, score: score})
	}
	return cands, nil
}

// rerankTables 用百炼 cross-encoder 对候选表做精排，返回按相关度排序的候选。
func rerankTables(ctx context.Context, question string, cands []tableCandidate, topK int) ([]tableCandidate, error) {
	cfg := config.GetConfig().RagModelConfig
	rr, err := rag.NewDashScopeRerankerWithParams(ctx,
		os.Getenv("EMBEDDING_API_KEY"),
		cfg.RagRerankModel,
		cfg.RagRerankBaseUrl,
		topK,
	)
	if err != nil {
		return nil, fmt.Errorf("创建重排器失败: %w", err)
	}
	docs := make([]*schema.Document, 0, len(cands))
	for _, c := range cands {
		docs = append(docs, &schema.Document{
			Content:  c.content,
			MetaData: map[string]any{"name": c.name},
		})
	}
	ranked, err := rr.Rerank(ctx, question, docs)
	if err != nil {
		return nil, err
	}
	out := make([]tableCandidate, 0, len(ranked))
	for _, d := range ranked {
		name, _ := d.MetaData["name"].(string)
		out = append(out, tableCandidate{name: name, content: d.Content, score: 0})
	}
	return out, nil
}

// mapTables 把候选表名映射回 catalog 中的完整 TableInfo（保留全部字段）。
func mapTables(cat *Catalog, cands []tableCandidate) []TableInfo {
	byName := make(map[string]TableInfo, len(cat.Tables))
	for _, t := range cat.Tables {
		byName[t.Name] = t
	}
	out := make([]TableInfo, 0, len(cands))
	for _, c := range cands {
		if t, ok := byName[c.name]; ok {
			out = append(out, t)
		}
	}
	return out
}

// ---- 兜底：轻量关键词检索（与旧实现一致） ----

// lexicalRetrieve 当向量检索不可用时，用轻量打分选出相关表。
func lexicalRetrieve(question string, cat *Catalog, topK int) []TableInfo {
	type scored struct {
		t TableInfo
		s float64
	}
	ranked := make([]scored, 0, len(cat.Tables))
	for _, t := range cat.Tables {
		score := 0.0
		score += textScore(question, t.Name) * 3    // 表名权重最高
		score += textScore(question, t.Comment) * 3 // 表注释同样高权重
		for _, c := range t.Columns {
			score += textScore(question, c.Name)    // 字段名
			score += textScore(question, c.Comment) // 字段注释
		}
		if score > 0 {
			ranked = append(ranked, scored{t, score})
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].s > ranked[j].s
	})
	if len(ranked) > topK {
		ranked = ranked[:topK]
	}
	out := make([]TableInfo, 0, len(ranked))
	for _, r := range ranked {
		out = append(out, r.t)
	}
	return out
}

// textScore 计算问题与一段文本的相关度（轻量、无向量）：
//   - 双向子串包含：+2（中文 COMMENT 主要靠这个命中，如 "用户" 命中 "用户信息表"）
//   - 英文分词 overlap：每命中一个 >=3 字符的词 +1
func textScore(question, text string) float64 {
	if text == "" {
		return 0
	}
	q := strings.ToLower(strings.TrimSpace(question))
	t := strings.ToLower(strings.TrimSpace(text))
	if q == "" || t == "" {
		return 0
	}
	if strings.Contains(q, t) || strings.Contains(t, q) {
		return 2.0
	}
	qWords := splitWords(q)
	tWords := splitWords(t)
	overlap := 0
	for w := range qWords {
		if _, ok := tWords[w]; ok {
			overlap++
		}
	}
	return float64(overlap)
}

// splitWords 提取长度 >=3 的英文/数字词（用于表名、字段名匹配）。中文不分词，交给子串匹配。
func splitWords(s string) map[string]struct{} {
	words := map[string]struct{}{}
	for _, part := range strings.FieldsFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') &&
			!(r >= '0' && r <= '9') && r != '_'
	}) {
		part = strings.Trim(part, "_")
		if len(part) >= 3 {
			words[strings.ToLower(part)] = struct{}{}
		}
	}
	return words
}

// toFloat32Slice 把 eino 返回的 []float64 向量转换为 Weaviate 需要的 []float32。
func toFloat32Slice(v []float64) []float32 {
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = float32(x)
	}
	return out
}

// SchemaPromptText 把若干张表的结构格式化为文本，供后续 Text-to-SQL 的 prompt 注入。
// 只描述表名 / 中文注释 / 字段名 / 类型 / 字段注释，不暴露行数据。
func SchemaPromptText(tables []TableInfo) string {
	if len(tables) == 0 {
		return "(无可用表)"
	}
	var b strings.Builder
	for _, t := range tables {
		fmt.Fprintf(&b, "表名: %s", t.Name)
		if t.Comment != "" {
			fmt.Fprintf(&b, " (%s)", t.Comment)
		}
		b.WriteString("\n字段:\n")
		for _, c := range t.Columns {
			fmt.Fprintf(&b, "  - %s %s", c.Name, c.Type)
			if c.Comment != "" {
				fmt.Fprintf(&b, " -- %s", c.Comment)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}
