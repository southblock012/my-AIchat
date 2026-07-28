package rag

import (
	_ "embed"
	"log"
	"regexp"
	"sort"
	"strings"

	"my-AIchat/common/mysql"
	"my-AIchat/model"
)

//go:embed knowledge.md
var knowledgeMD string

// SeedIfEmpty 首次调用时自动建表并灌入内置知识；已存在则跳过。
// 放在这里而非 mysql.go，是为了不改动已有的迁移代码。
func SeedIfEmpty() {
	if err := mysql.DB.AutoMigrate(&model.RAGChunk{}); err != nil {
		log.Println("[rag] migrate failed:", err)
		return
	}
	var cnt int64
	mysql.DB.Model(&model.RAGChunk{}).Count(&cnt)
	if cnt > 0 {
		return
	}
	chunks := splitDocument(knowledgeMD)
	for i, c := range chunks {
		if err := mysql.DB.Create(&model.RAGChunk{
			DocTitle:   "项目知识库",
			ChunkIndex: i,
			Content:    c,
			Source:     "knowledge.md",
		}).Error; err != nil {
			log.Println("[rag] seed chunk failed:", err)
		}
	}
	log.Printf("[rag] seeded %d chunks\n", len(chunks))
}

// splitDocument 按空行/换行分段，过滤过短片段（<10 字）
func splitDocument(text string) []string {
	paras := strings.Split(text, "\n")
	var raw []string
	buf := strings.Builder{}
	for _, p := range paras {
		line := strings.TrimSpace(p)
		if line == "" {
			if buf.Len() > 0 {
				raw = append(raw, strings.TrimSpace(buf.String()))
				buf.Reset()
			}
			continue
		}
		buf.WriteString(line)
		buf.WriteString("\n")
	}
	if buf.Len() > 0 {
		raw = append(raw, strings.TrimSpace(buf.String()))
	}
	var out []string
	for _, c := range raw {
		if len([]rune(c)) >= 10 {
			out = append(out, c)
		}
	}
	return out
}

// Search 读全部切片后按关键词命中数打分，返回 top-K（零外部依赖，库小全量读即可）
func Search(query string, k int) []model.RAGChunk {
	var all []model.RAGChunk
	mysql.DB.Find(&all)
	if len(all) == 0 {
		return nil
	}
	toks := tokenize(query)
	type scored struct {
		chunk model.RAGChunk
		score int
	}
	var list []scored
	for _, c := range all {
		if s := scoreChunk(c.Content, toks); s > 0 {
			list = append(list, scored{c, s})
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].score > list[j].score })
	if k > len(list) {
		k = len(list)
	}
	out := make([]model.RAGChunk, k)
	for i := 0; i < k; i++ {
		out[i] = list[i].chunk
	}
	return out
}

// RetrieveContext 取 top-K 相关片段并拼成带编号的文本，直接注入 prompt
func RetrieveContext(query string, k int) string {
	chunks := Search(query, k)
	if len(chunks) == 0 {
		return "(无相关资料)"
	}
	var b strings.Builder
	for i, c := range chunks {
		b.WriteString("【资料")
		b.WriteString(itoa(i + 1))
		b.WriteString("】")
		b.WriteString(c.Content)
		b.WriteString("\n")
	}
	return b.String()
}

var enRe = regexp.MustCompile(`[a-zA-Z0-9]+`)

// tokenize 中英文分词：英文/数字词整词提取；中文按连续二字 bigram 提取
func tokenize(text string) []string {
	text = strings.ToLower(text)
	var toks []string
	for _, w := range enRe.FindAllString(text, -1) {
		if len(w) >= 2 {
			toks = append(toks, w)
		}
	}
	runes := []rune(text)
	var cjk []rune
	flush := func() {
		if len(cjk) >= 2 {
			for i := 0; i < len(cjk)-1; i++ {
				toks = append(toks, string(cjk[i:i+2]))
			}
		}
		cjk = nil
	}
	for _, r := range runes {
		if r >= 0x4e00 && r <= 0x9fff {
			cjk = append(cjk, r)
		} else {
			flush()
		}
	}
	flush()
	return toks
}

// scoreChunk 统计命中 token 数（每个 token 只计一次）
func scoreChunk(content string, toks []string) int {
	lower := strings.ToLower(content)
	seen := map[string]bool{}
	score := 0
	for _, t := range toks {
		if seen[t] {
			continue
		}
		seen[t] = true
		if strings.Contains(lower, t) {
			score++
		}
	}
	return score
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
