package search

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/kinglegendzzh/flashmemory/internal/index"
	"github.com/kinglegendzzh/flashmemory/internal/utils"
	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// SearchResult represents a single search result with metadata.
type SearchResult struct {
	ID          int     // Database ID
	Name        string  // Function name
	Package     string  // Package name
	File        string  // File path
	Description string  // Function description
	Score       float32 // Similarity score (综合得分)
	CodeSnippet string  // 提取的代码片段（如果请求包含代码）
}

// SearchOptions configures the search behavior.
type SearchOptions struct {
	Limit       int     // Maximum number of results to return
	MinScore    float32 // Minimum similarity score threshold
	IncludeCode bool    // Whether to include code snippets
	SearchMode  string  // "semantic", "keyword", "hybrid"
}

// SearchEngine ties together the SQLite DB and Faiss index for queries.
type SearchEngine struct {
	Indexer *index.Indexer
	// For fallback text search
	Descriptions map[int]string
	ProjDir      string
}

// Searcher encapsulates different search implementations.
type Searcher struct {
	Engine *SearchEngine
}

// NewSearcher creates a new searcher with the given search engine.
func NewSearcher(engine *SearchEngine) *Searcher {
	return &Searcher{Engine: engine}
}

// Query 根据SearchOptions的SearchMode调用相应的搜索策略
func (se *SearchEngine) Query(query string, opts SearchOptions) []SearchResult {
	var results []SearchResult
	var err error

	switch opts.SearchMode {
	case "keyword":
		results, err = keywordSearch(se, query, opts)
	case "hybrid":
		results, err = hybridSearch(se, query, opts)
	default: // 默认使用语义搜索
		results, err = semanticSearch(se, query, opts)
	}
	if err != nil {
		log.Printf("Search error: %v", err)
		return nil
	}
	if len(results) == 0 {
		fmt.Println("No relevant functions found for query.")
		return nil
	}
	// 打印结果
	fmt.Println("Search Results:")
	for _, res := range results {
		fmt.Printf("- %s (Package: %s, File: %s) Score: %.3f\n", res.Name, res.Package, res.File, res.Score)
		fmt.Printf("  Description: %s\n", res.Description)
		if opts.IncludeCode && res.CodeSnippet != "" {
			fmt.Printf("  Code Snippet:\n%s\n", res.CodeSnippet)
		}
	}
	return results
}

// semanticSearch performs vector similarity search using embeddings.
func semanticSearch(se *SearchEngine, query string, opts SearchOptions) ([]SearchResult, error) {
	// 使用 Ollama 的 embedding 计算查询向量
	vector := SimpleEmbedding(query, se.Indexer.FaissIndex.Dimension())
	// 使用 Faiss 搜索向量
	funcIDs := se.Indexer.FaissIndex.SearchVectors(vector, opts.Limit)
	if len(funcIDs) == 0 {
		return nil, nil // 没有结果
	}

	results := make([]SearchResult, 0, len(funcIDs))
	for _, id := range funcIDs {
		res, err := fetchFunctionFromDB(se.Indexer.DB, id, opts.IncludeCode, se.ProjDir)
		if err != nil {
			continue
		}
		// 获取语义相似得分
		res.Score = se.Indexer.FaissIndex.GetScore(id)
		// 过滤得分过低的结果
		if res.Score >= opts.MinScore {
			results = append(results, res)
		}
	}
	return results, nil
}

// keywordSearch performs a SQL LIKE-based search.
func keywordSearch(se *SearchEngine, query string, opts SearchOptions) ([]SearchResult, error) {
	logs.Infof("Keyword search for query: %s", query)
	keywordJsonArray, err := utils.DefaultModelCompletion(query)
	if err != nil {
		logs.Errorf("Failed to invoke keyword model: %v", err)
		return nil, err
	}
	logs.Infof("Keyword JSON Array: %s", keywordJsonArray)
	if keywordJsonArray == "" {
		logs.Warnf("Keyword JSON Array is empty")
	}
	var keywords []string
	// 反序列化
	if err := json.Unmarshal([]byte(keywordJsonArray), &keywords); err != nil {
		logs.Errorf("Failed to unmarshal keyword JSON Array: %v", err)
	}
	// 构建模糊匹配模式
	pattern := "%" + query + "%"
	sqlQuery := `
		SELECT rowid, name, package, file, description, start_line, end_line 
		FROM functions 
		WHERE name LIKE ? OR package LIKE ? OR description LIKE ?
		LIMIT ?`
	rows, err := se.Indexer.DB.Query(sqlQuery, pattern, pattern, pattern, opts.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := []SearchResult{}
	for rows.Next() {
		var id int
		var name, pkg, file, desc string
		var startLine, endLine int
		if err := rows.Scan(&id, &name, &pkg, &file, &desc, &startLine, &endLine); err != nil {
			continue
		}
		res := SearchResult{
			ID:          id,
			Name:        name,
			Package:     pkg,
			File:        file,
			Description: desc,
			// 关键词搜索的得分可以简单设置为1（或根据匹配次数计算）
			Score: 1,
		}
		if opts.IncludeCode {
			logs.Tokenf("正在获取代码片段: %s, %d, %d\n", file, startLine, endLine)
			if snippet, err := getCodeSnippet(file, startLine, endLine, se.ProjDir); err == nil {
				res.CodeSnippet = snippet
			}
		}
		results = append(results, res)
	}
	return results, nil
}

// hybridSearch combines semantic and keyword search results.
func hybridSearch(se *SearchEngine, query string, opts SearchOptions) ([]SearchResult, error) {
	semResults, err := semanticSearch(se, query, opts)
	if err != nil {
		return nil, err
	}
	kwResults, err := keywordSearch(se, query, opts)
	if err != nil {
		return nil, err
	}
	// 使用map去重（根据ID）
	resultMap := make(map[int]SearchResult)
	// 语义搜索权重0.7，关键词搜索权重0.3
	for _, res := range semResults {
		resultMap[res.ID] = res
	}
	for _, res := range kwResults {
		if existing, ok := resultMap[res.ID]; ok {
			// 如果两者都命中，则加权合并得分
			combinedScore := 0.7*existing.Score + 0.3*res.Score
			existing.Score = combinedScore
			resultMap[res.ID] = existing
		} else {
			// 只命中关键词的结果使用关键词权重
			res.Score = 0.3 * res.Score
			resultMap[res.ID] = res
		}
	}
	// 转换map为slice
	results := []SearchResult{}
	for _, res := range resultMap {
		// 过滤低于阈值的结果
		if res.Score >= opts.MinScore {
			results = append(results, res)
		}
	}
	// 简单排序：分数从高到低
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
	// 若结果数超过限制则截断
	if len(results) > opts.Limit {
		results = results[:opts.Limit]
	}
	return results, nil
}

// SimpleEmbedding calls OllamaEmbedding to convert query text into an embedding vector.
func SimpleEmbedding(query string, dim int) []float32 {
	embedding, err := utils.OllamaEmbedding(query, dim)
	if err != nil {
		log.Printf("Ollama embedding error: %v, falling back to dummy embedding", err)
		return dummyEmbedding(query, dim)
	}
	logs.Infof("Ollama embedding success:dim=%d", dim)
	return embedding
}

func SimpleEmbeddingBatch(query []string, dim int) ([][]float32, error) {
	embedding, err := utils.OllamaEmbeddingsList(query, dim)
	if err != nil {
		log.Printf("Ollama embedding error: %v, falling back to dummy embedding", err)
		return nil, err
	}
	logs.Infof("batch Ollama embedding success:len=%v, dim=%d", len(embedding), dim)
	return embedding, nil
}

// dummyEmbedding 为故障情况下的备用实现（与原实现类似）
func dummyEmbedding(query string, dim int) []float32 {
	vec := make([]float32, dim)
	words := strings.Fields(query)
	if len(words) == 0 {
		return vec
	}
	for _, word := range words {
		hash := 0
		for i, c := range word {
			hash += int(c) * (i + 1)
		}
		for i := 0; i < dim; i++ {
			position := (hash + i) % dim
			value := float32(math.Cos(float64(hash+i) * 0.1))
			vec[position] += value
		}
	}
	var sum float32
	for _, v := range vec {
		sum += v * v
	}
	if sum > 0 {
		norm := float32(math.Sqrt(float64(sum)))
		for i := range vec {
			vec[i] /= norm
		}
	}
	return vec
}

// fetchFunctionFromDB 根据rowid获取函数信息，并在需要时提取代码片段
func fetchFunctionFromDB(db *sql.DB, id int, includeCode bool, projDir string) (SearchResult, error) {
	var name, pkg, file, desc string
	var startLine, endLine int
	err := db.QueryRow("SELECT name, package, file, description, start_line, end_line FROM functions WHERE rowid = ?", id).
		Scan(&name, &pkg, &file, &desc, &startLine, &endLine)
	if err != nil {
		return SearchResult{}, err
	}
	res := SearchResult{
		ID:          id,
		Name:        name,
		Package:     pkg,
		File:        file,
		Description: desc,
	}
	if includeCode {
		logs.Tokenf("正在获取代码片段: %s, %d, %d\n", file, startLine, endLine)
		if snippet, err := getCodeSnippet(file, startLine, endLine, projDir); err == nil {
			res.CodeSnippet = snippet
		}
	}
	return res, nil
}

// getCodeSnippet 从指定文件中提取从 startLine 到 endLine 的代码片段
func getCodeSnippet(filePath string, startLine, endLine int, projDir string) (string, error) {
	if projDir == "" {
		return "", fmt.Errorf("projDir 不能为空")
	}

	fullPath := filepath.Join(projDir, filePath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		logs.Errorf("读取文件失败 %s: %v", fullPath, err)
		return "", err
	}

	lines := strings.Split(string(data), "\n")
	total := len(lines)

	// 边界调整
	if startLine < 1 {
		startLine = 1
	}
	if endLine > total {
		endLine = total
	}
	if startLine > endLine {
		return "", fmt.Errorf("startLine (%d) > endLine (%d)", startLine, endLine)
	}

	snippet := strings.Join(lines[startLine-1:endLine], "\n")
	return snippet, nil
}
