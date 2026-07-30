package rag

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"my-AIchat/common/weaviate"
	"my-AIchat/config"

	openaiEmbedding "github.com/cloudwego/eino-ext/components/embedding/openai"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/schema"
	wv "github.com/weaviate/weaviate-go-client/v5/weaviate"
	wvgql "github.com/weaviate/weaviate-go-client/v5/weaviate/graphql"
	wvmodels "github.com/weaviate/weaviate/entities/models"
)

// RAGIndexer 知识库写入器：负责把文档切块、向量化、写入 Weaviate。
// 对应参考项目里的 redisIndexer，这里用 Weaviate 替代 Redis。
type RAGIndexer struct {
	embedder  embedding.Embedder
	client    *wv.Client
	className string
}

// RAGQuery 知识库检索器：负责把问题向量化并在 Weaviate 中做相似度检索。
// 对应参考项目里的 redisRetriever。
type RAGQuery struct {
	embedder  embedding.Embedder
	client    *wv.Client
	className string
}

// getClient 返回全局 Weaviate 客户端；若尚未初始化则惰性初始化。
func getClient() *wv.Client {
	if weaviate.WeaviateClient == nil {
		weaviate.InitWeaviate()
	}
	return weaviate.WeaviateClient
}

// newEmbedder 构造一个 OpenAI 兼容的 embedding 客户端。
// 你的配置指向 dashscope（baseUrl=https://dashscope.aliyuncs.com/compatible-mode/v1），
// 所以这里天然兼容通义千问的 text-embedding-v4。换成 OpenAI 也只需改配置。
func newEmbedder(ctx context.Context, model string) (embedding.Embedder, error) {
	cfg := config.GetConfig().RagModelConfig
	apikey := os.Getenv("EMBEDDING_API_KEY")
	ec := &openaiEmbedding.EmbeddingConfig{
		BaseURL: cfg.RagBaseUrl,
		APIKey:  apikey,
		Model:   model,
	}
	emb, err := openaiEmbedding.NewEmbedder(ctx, ec)
	if err != nil {
		return nil, fmt.Errorf("failed to create embedder: %w", err)
	}
	return emb, nil
}

// GenerateClassName 把文件名（uuid.ext）转换成合法的 Weaviate class 名。
// Weaviate 的 class 名必须匹配 ^[A-Z][a-zA-Z0-9]*$：
// 去掉扩展名与所有非字母数字字符，若以数字开头则补 "Doc" 前缀，首字母转大写。
func GenerateClassName(filename string) string {
	name := filename
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		name = name[:idx]
	}
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	sanitized := b.String()
	if sanitized == "" {
		sanitized = "Doc"
	}
	if sanitized[0] >= '0' && sanitized[0] <= '9' {
		sanitized = "Doc" + sanitized
	}
	return strings.ToUpper(sanitized[:1]) + sanitized[1:]
}

// NewRAGIndexer 构建知识库索引器，并创建对应的 Weaviate class。
// embeddingModel 由调用方传入（config.RagModelConfig.RagEmbeddingModel）。
func NewRAGIndexer(filename, embeddingModel string) (*RAGIndexer, error) {
	ctx := context.Background()

	emb, err := newEmbedder(ctx, embeddingModel)
	if err != nil {
		return nil, err
	}
	client := getClient()
	if client == nil {
		return nil, fmt.Errorf("weaviate client is nil, check InitWeaviate")
	}

	className := GenerateClassName(filename)
	// 覆盖上传语义：先尝试删除旧 class（忽略错误，可能本来就不存在）
	_ = client.Schema().ClassDeleter().WithClassName(className).Do(ctx)

	classObj := &wvmodels.Class{
		Class:           className,
		Vectorizer:      "none", // 向量由我们在外部算好再传进去
		VectorIndexType: "hnsw",
		Properties: []*wvmodels.Property{
			{Name: "content", DataType: []string{"text"}},
			{Name: "source", DataType: []string{"text"}},
		},
	}
	if err := client.Schema().ClassCreator().WithClass(classObj).Do(ctx); err != nil {
		return nil, fmt.Errorf("failed to create class %s: %w", className, err)
	}

	return &RAGIndexer{embedder: emb, client: client, className: className}, nil
}

// IndexFile 读取文件、切块、向量化并批量写入 Weaviate。
func (r *RAGIndexer) IndexFile(ctx context.Context, filePath string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}
	chunks := splitDocument(string(content), 800)
	if len(chunks) == 0 {
		return fmt.Errorf("file content is empty")
	}

	vectors, err := r.embedder.EmbedStrings(ctx, chunks)
	if err != nil {
		return fmt.Errorf("failed to embed chunks: %w", err)
	}
	if len(vectors) != len(chunks) {
		return fmt.Errorf("embedding count mismatch: got %d want %d", len(vectors), len(chunks))
	}

	objs := make([]*wvmodels.Object, 0, len(chunks))
	for i, chunk := range chunks {
		objs = append(objs, &wvmodels.Object{
			Class: r.className,
			Properties: map[string]interface{}{
				"content": chunk,
				"source":  filePath,
			},
			Vector: toFloat32Slice(vectors[i]),
		})
	}

	if _, err := r.client.Batch().ObjectsBatcher().WithObjects(objs...).Do(ctx); err != nil {
		return fmt.Errorf("failed to batch insert: %w", err)
	}
	return nil
}

// DeleteIndex 删除指定文件对应的 Weaviate class（静态方法，不依赖实例）。
func DeleteIndex(ctx context.Context, filename string) error {
	client := getClient()
	if client == nil {
		return fmt.Errorf("weaviate client is nil, check InitWeaviate")
	}
	className := GenerateClassName(filename)
	if err := client.Schema().ClassDeleter().WithClassName(className).Do(ctx); err != nil {
		return fmt.Errorf("failed to delete class %s: %w", className, err)
	}
	return nil
}

// NewRAGQuery 构建检索器：找到该用户已上传的文件，定位对应的 Weaviate class。
// 任一步失败（没上传文件等）返回 error，上层据此降级为普通对话。
func NewRAGQuery(ctx context.Context, username string) (*RAGQuery, error) {
	emb, err := newEmbedder(ctx, config.GetConfig().RagModelConfig.RagEmbeddingModel)
	if err != nil {
		return nil, err
	}
	client := getClient()
	if client == nil {
		return nil, fmt.Errorf("weaviate client is nil, check InitWeaviate")
	}

	userDir := fmt.Sprintf("uploads/%s", username)
	files, err := os.ReadDir(userDir)
	if err != nil || len(files) == 0 {
		return nil, fmt.Errorf("no uploaded file found for user %s", username)
	}
	var filename string
	for _, f := range files {
		if !f.IsDir() {
			filename = f.Name()
			break
		}
	}
	if filename == "" {
		return nil, fmt.Errorf("no valid file found for user %s", username)
	}

	className := GenerateClassName(filename)
	return &RAGQuery{embedder: emb, client: client, className: className}, nil
}

// RetrieveDocuments 把问题向量化，并在 Weaviate 中做相似度检索，返回 Top5 文档块。
func (r *RAGQuery) RetrieveDocuments(ctx context.Context, query string) ([]*schema.Document, error) {
	vectors, err := r.embedder.EmbedStrings(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}
	queryVector := vectors[0]
	log.Printf("[RAG] query vector dim=%d, class=%s", len(queryVector), r.className)

	resp, err := r.client.GraphQL().Get().
		WithClassName(r.className).
		WithFields(
			wvgql.Field{Name: "content"},
			wvgql.Field{Name: "source"},
			wvgql.Field{Name: "_additional", Fields: []wvgql.Field{
				{Name: "id"},
				{Name: "distance"},
				{Name: "vector"},
			}},
		).
		WithNearVector(r.client.GraphQL().NearVectorArgBuilder().WithVector(toFloat32Slice(queryVector))).
		WithLimit(5).
		Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve: %w", err)
	}
	return parseGraphQLResponse(resp, r.className)
}

// keysOf 取出 map 的 key 列表，用于把 Get 下实际存在的 class 名打到日志里辅助排查。
func keysOf(m map[string]interface{}) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

// parseGraphQLResponse 把 Weaviate GraphQL 返回的 JSON 结构解析成 eino 的 Document 列表。
func parseGraphQLResponse(resp *wvmodels.GraphQLResponse, className string) ([]*schema.Document, error) {
	if resp == nil {
		return nil, fmt.Errorf("empty weaviate response")
	}
	if len(resp.Errors) > 0 {
		log.Printf("[RAG] weaviate graphql errors: %+v", resp.Errors)
		return nil, fmt.Errorf("weaviate graphql error: %v", resp.Errors[0].Message)
	}
	data := resp.Data
	if data == nil {
		return nil, fmt.Errorf("no data in response")
	}
	get, ok := data["Get"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("no Get field in response")
	}
	items, ok := get[className].([]interface{})
	if !ok {
		// 关键诊断：把 Get 下实际存在的 class 名打出来，确认是否 class 名不一致导致"no class"
		log.Printf("[RAG] no class %s in Get response; available classes=%v", className, keysOf(get))
		return nil, fmt.Errorf("no class %s in response", className)
	}

	docs := make([]*schema.Document, 0, len(items))
	ids := make([]string, 0, len(items))
	dists := make([]float64, 0, len(items))
	classVecDim := 0
	for _, it := range items {
		obj, ok := it.(map[string]interface{})
		if !ok {
			continue
		}
		content, _ := obj["content"].(string)
		source, _ := obj["source"].(string)
		additional, _ := obj["_additional"].(map[string]interface{})
		id, _ := additional["id"].(string)
		dist, _ := additional["distance"].(float64)
		// 取一个召回对象的向量长度作为 class 的向量维度（用于核对是否与查询向量维度一致）
		if vec, ok := additional["vector"].([]interface{}); ok && classVecDim == 0 {
			classVecDim = len(vec)
		}
		if id != "" {
			ids = append(ids, id)
			dists = append(dists, dist)
		}
		docs = append(docs, &schema.Document{
			Content:  content,
			MetaData: map[string]any{"source": source, "id": id},
		})
	}
	log.Printf("[RAG] retrieved %d docs; classVectorDim=%d; ids=%v; distances=%v", len(docs), classVecDim, ids, dists)
	return docs, nil
}

// BuildRAGPrompt 把检索到的文档拼进提示词，让 LLM 基于参考文档作答。
// 与参考项目完全一致，可直接复用。
func BuildRAGPrompt(query string, docs []*schema.Document) string {
	if len(docs) == 0 {
		return query
	}
	contextText := ""
	for i, doc := range docs {
		contextText += fmt.Sprintf("[文档 %d]: %s\n\n", i+1, doc.Content)
	}
	return fmt.Sprintf(`基于以下参考文档回答用户的问题。如果文档中没有相关信息，请说明无法找到相关信息。

参考文档：
%s

用户问题：%s

请提供准确、完整的回答：`, contextText, query)
}

// toFloat32Slice 把 eino 返回的 []float64 向量转换为 Weaviate 需要的 []float32。
func toFloat32Slice(v []float64) []float32 {
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = float32(x)
	}
	return out
}

// splitDocument 语义感知切片（改进版）：
//  1. 按句子边界切分，不在句中硬切断，保留语义完整；
//  2. Markdown 标题传播：每个 chunk 携带其所属最近标题（如 [安装]），增强检索上下文；
//  3. 跨 chunk 以“上一个 chunk 末尾的完整句子”做重叠，避免半句截断；
//  4. 单句超长且无标点时按字符兜底切分。
func splitDocument(content string, chunkSize int) []string {
	// 预处理：统一换行、去掉 \r
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")

	lines := strings.Split(content, "\n")

	type seg struct {
		text  string // 一个完整句子（含结尾标点）
		title string // 该句所属的最近标题（已去掉 # 前缀），可能为空
	}

	var segs []seg
	currentTitle := ""
	var titleBuf strings.Builder

	flushTitle := func() {
		if titleBuf.Len() > 0 {
			currentTitle = strings.TrimSpace(titleBuf.String())
			titleBuf.Reset()
		}
	}

	for _, raw := range lines {
		line := strings.Join(strings.Fields(raw), " ") // 合并行内多余空白
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") { // Markdown 标题行
			titleBuf.WriteString(" ")
			titleBuf.WriteString(strings.TrimLeft(line, "#"))
			continue
		}
		flushTitle()
		for _, s := range splitSentences(line) {
			segs = append(segs, seg{text: s, title: currentTitle})
		}
	}
	flushTitle()

	// 聚合句子为 chunk，按字符上限在句子边界落盘（单位统一用 rune，兼容中文）
	var chunks []string
	var buf strings.Builder
	bufRuneLen := 0
	var lastSentence string // 上一个 chunk 末尾的句子（带标题前缀），用于重叠

	appendChunk := func(text string) {
		if t := strings.TrimSpace(text); t != "" {
			chunks = append(chunks, t)
		}
	}

	for _, s := range segs {
		full := s.text
		if s.title != "" {
			full = "[" + s.title + "] " + s.text
		}
		fullRunes := len([]rune(full))

		if bufRuneLen > 0 && bufRuneLen+fullRunes > chunkSize {
			appendChunk(buf.String())
			buf.Reset()
			bufRuneLen = 0
			// 语义重叠：用上一个 chunk 末尾的完整句子开头（太长的句子不重叠，避免噪声）
			if lastRunes := len([]rune(lastSentence)); lastRunes > 0 && lastRunes < chunkSize/3 {
				buf.WriteString(lastSentence)
				bufRuneLen = lastRunes
			}
		}
		if bufRuneLen > 0 {
			buf.WriteString(" ")
			bufRuneLen++
		}
		buf.WriteString(full)
		bufRuneLen += fullRunes
		lastSentence = full
	}
	appendChunk(buf.String())

	// 兜底：若全部是超长无标点文本导致无 chunk，按字符硬切
	if len(chunks) == 0 {
		runes := []rune(content)
		for i := 0; i < len(runes); i += chunkSize {
			end := i + chunkSize
			if end > len(runes) {
				end = len(runes)
			}
			appendChunk(string(runes[i:end]))
		}
	}
	return chunks
}

// splitSentences 按句子结束符切分一行文本，保留结尾标点。
// 中文标点 。！？； 以及英文 . ! ? ; 均视为句末（简单策略，适合中文文档为主的场景）。
func splitSentences(line string) []string {
	if line == "" {
		return nil
	}
	var res []string
	var b strings.Builder
	for _, r := range line {
		b.WriteRune(r)
		if strings.ContainsRune("。！？!?；;", r) {
			if t := strings.TrimSpace(b.String()); t != "" {
				res = append(res, t)
			}
			b.Reset()
		}
	}
	if t := strings.TrimSpace(b.String()); t != "" {
		res = append(res, t)
	}
	return res
}
