package index

import (
	"database/sql"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"

	"github.com/kinglegendzzh/flashmemory/internal/analyzer"
	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"

	_ "modernc.org/sqlite"
)

// SimilarityFunc 定义了向量相似度计算的函数类型。
// 当 useDistance 为 false 时，返回值越大表示越相似（例如余弦相似度）；
// 当 useDistance 为 true 时，返回值越小表示越相似（例如欧几里得距离）。
type SimilarityFunc func(a, b []float32) float32

// Indexer handles saving to and querying from the index (DB + vector store).
type Indexer struct {
	DB         *sql.DB
	FaissIndex FaissWrapper // FaissWrapper 是一个接口类型
}

// EnsureIndexDB opens or creates the SQLite DB in .gitgo directory.
func EnsureIndexDB(projectRoot string) (*sql.DB, error) {
	idxDir := filepath.Join(projectRoot, ".gitgo")
	os.MkdirAll(idxDir, 0755)
	dbPath := filepath.Join(idxDir, "code_index.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	// Create tables if not exist
	schema := `
CREATE TABLE IF NOT EXISTS functions (
    id INTEGER PRIMARY KEY,
    name TEXT,
    package TEXT,
    file TEXT,
    description TEXT,
    start_line INTEGER,
    end_line INTEGER,
    function_type TEXT
);
CREATE TABLE IF NOT EXISTS calls (
    caller TEXT,
    callee TEXT
);
CREATE TABLE IF NOT EXISTS externals (
    function TEXT,
    external TEXT
);
`
	_, err = db.Exec(schema)
	if err != nil {
		return nil, err
	}
	// 为 functions 表添加唯一索引，避免重复插入
	_, err = db.Exec(`
CREATE UNIQUE INDEX IF NOT EXISTS idx_func_unique
  ON functions(name, package, file, start_line, end_line, function_type);
`)
	if err != nil {
		return nil, err
	}
	return db, nil
}

// SaveAnalysisToDB writes analysis results into the SQLite database.
func (idx *Indexer) SaveAnalysisToDB(results []analyzer.LLMAnalysisResult) error {
	tx, err := idx.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// 使用 INSERT OR IGNORE，遇到冲突时跳过
	funcStmt, _ := tx.Prepare(`
INSERT OR IGNORE INTO functions(
    name, package, file, description, start_line, end_line, function_type
) VALUES(?, ?, ?, ?, ?, ?, ?)`)
	callStmt, _ := tx.Prepare("INSERT INTO calls(caller, callee) VALUES(?, ?)")
	extStmt, _ := tx.Prepare("INSERT INTO externals(function, external) VALUES(?, ?)")
	for _, res := range results {
		_, err = funcStmt.Exec(
			res.Func.Name,
			res.Func.Package,
			res.Func.File,
			res.Description,
			res.Func.StartLine,
			res.Func.EndLine,
			res.Func.FunctionType,
		)
		if err != nil {
			log.Printf("插入 functions 失败（已跳过？）: %v", err)
		}
		// calls 和 externals 如果也可能重复，可以同样建唯一索引并用 OR IGNORE
		for _, callee := range res.InternalDeps {
			if _, err := callStmt.Exec(res.Func.Name, callee); err != nil {
				log.Printf("插入 calls 失败: %v", err)
			}
		}
		for _, ext := range res.ExternalDeps {
			if _, err := extStmt.Exec(res.Func.Name, ext); err != nil {
				log.Printf("插入 externals 失败: %v", err)
			}
		}
	}

	return tx.Commit()
}

func (idx *Indexer) SaveAnalysisToDBHttp(results []analyzer.LLMAnalysisResult, projDir string) error {
	tx, err := idx.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// 使用 INSERT OR IGNORE，遇到冲突时跳过
	funcStmt, _ := tx.Prepare(`
INSERT OR IGNORE INTO functions(
    name, package, file, description, start_line, end_line, function_type
) VALUES(?, ?, ?, ?, ?, ?, ?)`)
	callStmt, _ := tx.Prepare("INSERT INTO calls(caller, callee) VALUES(?, ?)")
	extStmt, _ := tx.Prepare("INSERT INTO externals(function, external) VALUES(?, ?)")
	for _, res := range results {
		if res.Func.FunctionType == "llm_parser" {
			logs.Warnf("[WARN] 忽略 LLM_PARSER 函数的库录入 %s", res.Func.Name)
			continue
		}
		if projDir != "" {
			res.Func.File, err = filepath.Rel(projDir, res.Func.File)
			if err != nil {
				fmt.Errorf("%s, %s, 无法将文件路径转换为相对路径: %w", projDir, res.Func.File, err)
			}
			res.Func.File = filepath.ToSlash(res.Func.File)
			logs.Infof("[DEBUG][DB] 存储文件路径为: %s", res.Func.File)
		}
		_, err = funcStmt.Exec(
			res.Func.Name,
			res.Func.Package,
			res.Func.File,
			res.Description,
			res.Func.StartLine,
			res.Func.EndLine,
			res.Func.FunctionType,
		)
		if err != nil {
			log.Printf("插入 functions 失败（已跳过？）: %v", err)
		}
		// calls 和 externals 如果也可能重复，可以同样建唯一索引并用 OR IGNORE
		for _, callee := range res.InternalDeps {
			if _, err := callStmt.Exec(res.Func.Name, callee); err != nil {
				log.Printf("插入 calls 失败: %v", err)
			}
		}
		for _, ext := range res.ExternalDeps {
			if _, err := extStmt.Exec(res.Func.Name, ext); err != nil {
				log.Printf("插入 externals 失败: %v", err)
			}
		}
	}

	return tx.Commit()
}

// --- Vector indexing (Faiss) ---

// NewFaissWrapper 创建一个新的 Faiss 索引实现
// 优先尝试创建 HTTP 实现，如果失败则回退到内存实现
// 可选参数 options 可以包含额外的配置选项
func NewFaissWrapper(dimension int, options ...map[string]interface{}) FaissWrapper {
	// 解析可选参数
	var opts map[string]interface{}
	if len(options) > 0 {
		opts = options[0]
	}

	// 获取服务器URL，默认使用DefaultFaissServerURL
	serverURL := DefaultFaissServerURL
	if url, ok := opts["server_url"].(string); ok && url != "" {
		serverURL = url
	}

	// 获取索引ID，默认使用"default"
	indexID := "default"
	if id, ok := opts["index_id"].(string); ok && id != "" {
		indexID = id
	}

	// 尝试创建 HTTP 实现
	httpWrapper, err := NewHTTPFaissWrapper(dimension, true, serverURL, indexID)
	if err == nil {
		// 如果提供了存储路径，设置到wrapper中
		if path, ok := opts["storage_path"].(string); ok && path != "" {
			// 设置存储路径到wrapper中
			httpWrapper.storagePath = path
			fmt.Printf("Faiss index will use storage path: %s\n", path)
		}
		return httpWrapper
	}

	// 如果 HTTP 实现创建失败，回退到内存实现
	fmt.Printf("Warning: Failed to create HTTP Faiss index: %v, falling back to in-memory implementation\n", err)
	return NewMemoryFaissWrapper(dimension)
}

// SetIndexSimilarityMetric 允许在运行时切换相似度计算方法，metric 可取 "cosine" 或 "euclidean"。
func (idx *Indexer) SetIndexSimilarityMetric(metric string) {
	idx.FaissIndex.SetSimilarityMetric(metric)
}

// AddVectorToIndex 为函数ID添加嵌入向量（使用函数的 rowid）
func (idx *Indexer) AddVectorToIndex(funcID int, vector []float32) error {
	return idx.FaissIndex.AddVector(funcID, vector)
}

// SearchVectorsInIndex 查找与查询向量最接近的 topK 个向量
func (idx *Indexer) SearchVectorsInIndex(query []float32, topK int) []int {
	return idx.FaissIndex.SearchVectors(query, topK)
}

// SaveIndexToFile 将 Faiss 索引保存到磁盘，支持自定义保存路径
func (idx *Indexer) SaveIndexToFile(path string) error {
	return idx.FaissIndex.SaveToFile(path)
}

// cosineSimilarity calculates the cosine similarity between two vectors.
// 返回值范围 [-1, 1]，1 表示方向完全相同。
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0 // 向量维度必须相同
	}

	var dotProduct float32
	var normA float32
	var normB float32

	for i := 0; i < len(a); i++ {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (float32(math.Sqrt(float64(normA))) * float32(math.Sqrt(float64(normB))))
}

// euclideanDistance calculates the Euclidean distance between two vectors.
// 当维度不匹配时返回一个较大的数。
func euclideanDistance(a, b []float32) float32 {
	if len(a) != len(b) {
		return 1e9
	}
	var sum float32
	for i := 0; i < len(a); i++ {
		diff := a[i] - b[i]
		sum += diff * diff
	}
	return float32(math.Sqrt(float64(sum)))
}

// EnableIndexCache 启用向量缓存
func (idx *Indexer) EnableIndexCache() {
	idx.FaissIndex.EnableCache()
}

// DisableIndexCache 禁用向量缓存
func (idx *Indexer) DisableIndexCache() {
	idx.FaissIndex.DisableCache()
}

// ClearIndexCache 清除向量缓存
func (idx *Indexer) ClearIndexCache() {
	idx.FaissIndex.ClearCache()
}

// GetIndexCacheStats 获取缓存统计信息
func (idx *Indexer) GetIndexCacheStats() map[string]interface{} {
	return idx.FaissIndex.GetCacheStats()
}

// LoadIndexFromFile 从文件加载索引
func (idx *Indexer) LoadIndexFromFile(path string) error {
	return idx.FaissIndex.LoadFromFile(path)
}
