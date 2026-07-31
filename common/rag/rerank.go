package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"

	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/schema"
)

// DashScopeReranker 实现 eino 的 document.Transformer 接口，
// 底层调用阿里云百炼的 text-rerank 服务（模型 gte-rerank）对召回文档做交叉编码器精排。
//
// 为什么用 eino 的 Transformer 契约：eino-ext 自带的重排器（如 ScoreReranker）本身就是
// document.Transformer 的实现，因此按这个接口写，重排器可直接嵌入 eino 的 Graph / 编排链路，
// 与 eino 生态保持一致（v0.9.12 时代还没有独立的 components/rerank 包，Transformer 是标准做法）。
type DashScopeReranker struct {
	apiKey  string
	model   string
	baseURL string
	topN    int
	query   string // 通过 Rerank 注入；Transform 时若为空则报错
}

type dashScopeRerankerConfig struct {
	APIKey  string
	Model   string
	BaseURL string
	TopN    int
}

// NewDashScopeReranker 构造百炼重排器。APIKey 留空时回退读取环境变量 EMBEDDING_API_KEY。
func NewDashScopeReranker(ctx context.Context, cfg *dashScopeRerankerConfig) (*DashScopeReranker, error) {
	if cfg == nil {
		return nil, fmt.Errorf("rerank config is nil")
	}
	apiKey := cfg.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("EMBEDDING_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("rerank api key is empty (set EMBEDDING_API_KEY or pass APIKey)")
	}
	model := cfg.Model
	if model == "" {
		model = "gte-rerank-v2"
	}
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://dashscope.aliyuncs.com/api/v1/services/rerank/text-rerank/text-rerank"
	}
	topN := cfg.TopN
	if topN <= 0 {
		topN = 5
	}
	return &DashScopeReranker{apiKey: apiKey, model: model, baseURL: baseURL, topN: topN}, nil
}

// NewDashScopeRerankerWithParams 给其它包（如 common/dbquery）用的导出便捷封装：
// 用平铺参数构造百炼重排器，内部仍走 NewDashScopeReranker，保持实现单一、避免重复。
func NewDashScopeRerankerWithParams(ctx context.Context, apiKey, model, baseURL string, topN int) (*DashScopeReranker, error) {
	return NewDashScopeReranker(ctx, &dashScopeRerankerConfig{
		APIKey:  apiKey,
		Model:   model,
		BaseURL: baseURL,
		TopN:    topN,
	})
}

// Rerank 对候选文档用 query 做精排，返回重排后的前 topN 条。
// 这是给检索链路直接调用的便捷方法（Transformer 接口本身不带 query，query 在这里注入）。
func (r *DashScopeReranker) Rerank(ctx context.Context, query string, docs []*schema.Document) ([]*schema.Document, error) {
	r.query = query
	return r.Transform(ctx, docs)
}

// Transform 实现 eino document.Transformer 接口。
func (r *DashScopeReranker) Transform(ctx context.Context, src []*schema.Document, opts ...document.TransformerOption) ([]*schema.Document, error) {
	if r.query == "" {
		return nil, fmt.Errorf("rerank query is empty; call Rerank(ctx, query, docs) instead")
	}
	if len(src) == 0 {
		return src, nil
	}

	docs4api := make([]string, len(src))
	for i, d := range src {
		docs4api[i] = d.Content
	}
	reqBody := dashScopeRerankRequest{
		Model:      r.model,
		Input:      dashScopeRerankInput{Query: r.query, Documents: docs4api},
		Parameters: dashScopeRerankParams{ReturnDocuments: false, TopN: r.topN},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal rerank request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build rerank request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+r.apiKey)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("rerank http call failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read rerank response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rerank api returned %d: %s", resp.StatusCode, string(body))
	}

	var rr dashScopeRerankResponse
	if err := json.Unmarshal(body, &rr); err != nil {
		return nil, fmt.Errorf("unmarshal rerank response: %w", err)
	}
	if rr.Output == nil || len(rr.Output.Results) == 0 {
		// 兜底：重排服务异常时退化为原始顺序，保证检索链路不中断
		log.Printf("[RAG] rerank returned empty, fallback to original order")
		return src, nil
	}

	type indexed struct {
		doc   *schema.Document
		score float64
	}
	ranked := make([]indexed, 0, len(rr.Output.Results))
	for _, res := range rr.Output.Results {
		if res.Index < 0 || res.Index >= len(src) {
			continue
		}
		d := src[res.Index]
		if d.MetaData == nil {
			d.MetaData = map[string]any{}
		}
		d.MetaData["rerank_score"] = res.RelevanceScore
		ranked = append(ranked, indexed{doc: d, score: res.RelevanceScore})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].score > ranked[j].score
	})

	out := make([]*schema.Document, 0, len(ranked))
	for _, it := range ranked {
		out = append(out, it.doc)
	}
	topScore := 0.0
	if len(ranked) > 0 {
		topScore = ranked[0].score
	}
	log.Printf("[RAG] reranked %d docs, top score=%.4f", len(out), topScore)
	return out, nil
}

// ---- 百炼 text-rerank API 数据结构 ----

type dashScopeRerankRequest struct {
	Model      string                `json:"model"`
	Input      dashScopeRerankInput  `json:"input"`
	Parameters dashScopeRerankParams `json:"parameters"`
}

type dashScopeRerankInput struct {
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
}

type dashScopeRerankParams struct {
	ReturnDocuments bool `json:"return_documents"`
	TopN            int  `json:"top_n"`
}

type dashScopeRerankResponse struct {
	Output    *dashScopeRerankOutput `json:"output"`
	RequestID string                 `json:"request_id"`
}

type dashScopeRerankOutput struct {
	Results []dashScopeRerankResult `json:"results"`
}

type dashScopeRerankResult struct {
	Index          int     `json:"index"`
	RelevanceScore float64 `json:"relevance_score"`
}
