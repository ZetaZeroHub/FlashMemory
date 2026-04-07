package search

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/kinglegendzzh/flashmemory/internal/index"
	"github.com/kinglegendzzh/flashmemory/internal/utils"
	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"
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
	StartLine   int     // 代码片段的起始行
	EndLine     int
	Type        string // 模块类型
	Path        string // 模块路径
	ParentPath  string // 父模块路径
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
	Keywords     []string
	Module       bool
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
func (se *SearchEngine) Query(query string, opts SearchOptions) ([]SearchResult, error) {
	var results []SearchResult
	var err error
	if opts.SearchMode != "semantic" {
		logs.Infof("Keywords llm search for query: %s", query)
		// 1. 调用大模型拿到 Keywords JSON 数组字符串
		keywordJsonArray, err := utils.DefaultModelCompletion(query)
		tokens := []string{"```", "json", "```json"}
		for _, v := range tokens {
			if strings.Contains(keywordJsonArray, v) {
				logs.Tokenf("(!remove: %v!)", v)
				keywordJsonArray = strings.Replace(keywordJsonArray, v, "", 2)
			}
		}
		if err != nil {
			logs.Errorf("Failed to invoke keyword model: %v", err)
		}
		logs.Infof("Keyword JSON Array: %s", keywordJsonArray)
		if keywordJsonArray == "" {
			logs.Warnf("Keyword JSON Array is empty")
		}

		// 2. 反序列化到 []string
		var keywords []string
		if err := json.Unmarshal([]byte(keywordJsonArray), &keywords); err != nil {
			logs.Errorf("Failed to unmarshal keyword JSON Array: %v", err)
		}
		se.Keywords = keywords
	}
	switch opts.SearchMode {
	case "keyword":
		results, err = keywordSearch(se, query, opts)
	case "hybrid":
		results, err = hybridSearch(se, query, opts)
	default: // 默认使用语义搜索
		logs.Infof("semantic llm search for query: %s", query)
		results, err = semanticSearch(se, query, opts)
	}
	if err != nil {
		log.Printf("Search error: %v", err)
		return nil, err
	}
	if len(results) == 0 {
		fmt.Println("No relevant functions found for query.")
		return nil, nil
	}
	// 打印结果
	fmt.Println("Search Results:")
	for _, res := range results {
		fmt.Printf("- %s (Package: %s, File: %s) Score: %.3f\n", res.Name, res.Package, res.File, res.Score)
		fmt.Printf("  Description: %s\n", res.Description)
		if opts.IncludeCode && res.CodeSnippet != "" {
			fmt.Printf(" Has Code Snippet.\n")
		}
	}
	return results, nil
}

// semanticSearch performs vector similarity search using embeddings.
func semanticSearch(se *SearchEngine, query string, opts SearchOptions) ([]SearchResult, error) {
	var vector []float32
	if opts.SearchMode != "semantic" {
		se.Keywords = append(se.Keywords, query)
		logs.Infof("semanticSearch mixed semantic search: %s", se.Keywords)
		// 使用 Ollama 的 embedding 计算查询向量
		vector = SimpleEmbedding(strings.Join(se.Keywords, " "), se.Indexer.FaissIndex.Dimension())
	} else {
		logs.Infof("semanticSearch semantic search: %s", query)
		// 使用 Ollama 的 embedding 计算查询向量
		vector = SimpleEmbedding(query, se.Indexer.FaissIndex.Dimension())
	}
	// 使用 Faiss 搜索向量
	funcIDs := se.Indexer.FaissIndex.SearchVectors(vector, opts.Limit)
	if len(funcIDs) == 0 {
		return nil, nil // 没有结果
	}

	results := make([]SearchResult, 0, len(funcIDs))
	for _, id := range funcIDs {
		var res SearchResult
		var err error
		if se.Module {
			res, err = fetchModuleFromDB(se.Indexer.DB, id, opts.IncludeCode, se.ProjDir)
		} else {
			res, err = fetchFunctionFromDB(se.Indexer.DB, id, opts.IncludeCode, se.ProjDir)
		}
		if err != nil {
			logs.Warnf("fetch function from db error: %v", err)
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
	// 3. 构造模糊匹配的 WHERE 子句
	//    同时包含原始 query 和大模型给出的每个 keyword
	var (
		conditions []string
		args       []interface{}
	)

	// 把 query 也作为一个模糊匹配项
	allTerms := append([]string{query}, se.Keywords...)
	logs.Infof("All Terms: %v", allTerms)
	for _, term := range allTerms {
		pat := "%" + term + "%"
		// 针对 name/package/description 三个字段都做 LIKE
		conditions = append(conditions,
			"name LIKE ?",
			"package LIKE ?",
			"description LIKE ?",
		)
		args = append(args, pat, pat, pat)
	}

	// LIMIT 参数
	args = append(args, opts.Limit)

	// 4. 动态拼接 SQL
	sqlQuery := fmt.Sprintf(`
        SELECT rowid, name, package, file, description, start_line, end_line
        FROM functions
        WHERE %s
        LIMIT ?`,
		strings.Join(conditions, " OR "),
	)

	// 5. 执行查询
	rows, err := se.Indexer.DB.Query(sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// 6. 收集结果
	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(
			&r.ID, &r.Name, &r.Package, &r.File,
			&r.Description, &r.StartLine, &r.EndLine,
		); err != nil {
			logs.Errorf("Row scan error: %v", err)
			continue
		}

		// 简单给一个固定分数，或者你也可以根据匹配次数累加
		r.Score = 1

		if opts.IncludeCode {
			logs.Infof("Fetching code snippet: %s [%d:%d]", r.File, r.StartLine, r.EndLine)
			if snippet, err := getCodeSnippet(r.File, r.StartLine, r.EndLine, se.ProjDir); err == nil {
				r.CodeSnippet = snippet
			}
		}

		results = append(results, r)
	}

	// 7. 查询 code_desc 表
	// 为 code_desc 表构建类似的查询条件
	var (
		codeDescConditions []string
		codeDescArgs       []interface{}
	)

	for _, term := range allTerms {
		pat := "%" + term + "%"
		// 针对 code_desc 表的 name/type/path/description 字段都做 LIKE
		codeDescConditions = append(codeDescConditions,
			"name LIKE ?",
			"type LIKE ?",
			"path LIKE ?",
			"description LIKE ?",
		)
		codeDescArgs = append(codeDescArgs, pat, pat, pat, pat)
	}

	// LIMIT 参数
	codeDescArgs = append(codeDescArgs, opts.Limit)

	// 为 code_desc 表动态拼接 SQL
	codeDescQuery := fmt.Sprintf(`
        SELECT rowid, name, type, path, parent_path, description
        FROM code_desc
        WHERE %s
        LIMIT ?`,
		strings.Join(codeDescConditions, " OR "),
	)

	// 直接尝试查询 code_desc 表，如不存在则捕获错误
	codeDescRows, err := se.Indexer.DB.Query(codeDescQuery, codeDescArgs...)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			logs.Infof("code_desc table not exists, skip.")
		} else {
			logs.Warnf("Querying code_desc table failed: %v", err)
		}
	} else {
		logs.Infof("code_desc table exists, start search.")
		defer codeDescRows.Close()
		var count int
		for codeDescRows.Next() {
			var r SearchResult
			var id int
			var name, typ, path, parentPath, description string
			if err := codeDescRows.Scan(&id, &name, &typ, &path, &parentPath, &description); err != nil {
				logs.Errorf("Row scan error for code_desc: %v", err)
				continue
			}

			r.ID = id
			r.Name = name
			r.Type = typ
			r.Path = path
			r.ParentPath = parentPath
			r.Description = description
			// 简单给一个固定分数，与函数搜索结果相同
			r.Score = 1

			// 如果是文件类型且需要代码片段，则获取代码
			if opts.IncludeCode && typ == "file" && path != "" {
				logs.Infof("Fetching code snippet for module: %s", path)
				if snippet, err := getCodeSnippet(path, 0, 0, se.ProjDir); err == nil {
					r.CodeSnippet = snippet
				}
			}

			results = append(results, r)
			count++
		}
		logs.Infof("code_desc table searched count: %d", count)
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
	embedding, err := utils.EmbeddingsList([]string{query}, dim)
	if err != nil {
		log.Printf("embedding error: %v, falling back to dummy embedding", err)
		return dummyEmbedding(query, dim)
	}
	logs.Infof("Ollama embedding success:dim=%d", dim)
	return embedding[0]
}

func SimpleEmbeddingBatch(query []string, dim int) ([][]float32, error) {
	embedding, err := utils.EmbeddingsList(query, dim)
	if err != nil {
		log.Printf("batch embedding error: %v", err)
		return nil, err
	}
	logs.Infof("batch Ollama embedding success:len=%v, dim=%d", len(embedding), dim)
	return embedding, nil
}

// dummyEmbedding 为故障情况下的备用实现（与原实现类似）
// Deprecated: 不推荐使用，仅作为 embedding 失败时的兜底方案。
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
		logs.Tokenf("Retrieving code snippets: %s, %d, %d\n", file, startLine, endLine)
		if snippet, err := getCodeSnippet(file, startLine, endLine, projDir); err == nil {
			res.CodeSnippet = snippet
		}
	}
	return res, nil
}

func fetchModuleFromDB(db *sql.DB, id int, includeCode bool, projDir string) (SearchResult, error) {
	var name, type_, path, parentPath, desc string
	err := db.QueryRow("SELECT name, type, path, parent_path, description FROM code_desc WHERE rowid = ?", id).
		Scan(&name, &type_, &path, &parentPath, &desc)
	if err != nil {
		return SearchResult{}, err
	}
	res := SearchResult{
		ID:          id,
		Name:        name,
		Type:        type_,
		Path:        path,
		ParentPath:  parentPath,
		Description: desc,
	}
	if includeCode && path != "" && type_ == "file" {
		logs.Tokenf("Retrieving code snippet: %s\n", path)
		if snippet, err := getCodeSnippet(path, 0, 0, projDir); err == nil {
			res.CodeSnippet = snippet
		}
	}
	return res, nil
}

// getCodeSnippet 从指定文件中提取从 startLine 到 endLine 的代码片段。如果 startLine 和 endLine 都为0，则默认读取全部（最大限制2000行）
// 如果文件不存在或读取失败，返回空字符串而不是错误
func getCodeSnippet(filePath string, startLine, endLine int, projDir string) (string, error) {
	if projDir == "" {
		logs.Warnf("projDir is empty and the code snippet cannot be read")
		return "", nil
	}

	fullPath := filepath.Join(projDir, filePath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		logs.Warnf("Failed to read file %s: %v, returning empty string", fullPath, err)
		return "", nil
	}

	lines := strings.Split(string(data), "\n")
	total := len(lines)

	// 默认全量读取（最大2000行）
	if startLine == 0 && endLine == 0 {
		startLine = 1
		if total > 2000 {
			endLine = 2000
		} else {
			endLine = total
		}
	}

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
