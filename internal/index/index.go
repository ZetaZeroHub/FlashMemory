package index

import (
	"database/sql"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"

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
    function_type TEXT,
    source TEXT,
    page INTEGER DEFAULT 0,
    slide INTEGER DEFAULT 0
);
CREATE TABLE IF NOT EXISTS calls (
    caller TEXT,
    callee TEXT
);
CREATE TABLE IF NOT EXISTS externals (
    function TEXT,
    external TEXT
);
CREATE TABLE IF NOT EXISTS code_desc (
    id INTEGER PRIMARY KEY,
    name TEXT,                  -- 文件名或文件夹名
    type TEXT,                  -- 类型：'file' 或 'directory'
    path TEXT,                  -- 相对路径
    parent_path TEXT,           -- 上层目录路径
    function_count INTEGER,     -- 子函数数量
    file_count INTEGER,         -- 子文件数量
    description TEXT,           -- 模块功能描述
    updated_at TIMESTAMP,       -- 更新时间
    created_at TIMESTAMP        -- 创建时间
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

	// 为 code_desc 表添加索引，提高按路径查询的性能
	_, err = db.Exec(`
CREATE INDEX IF NOT EXISTS idx_code_desc_path ON code_desc(path);
CREATE INDEX IF NOT EXISTS idx_code_desc_parent ON code_desc(parent_path);
CREATE UNIQUE INDEX IF NOT EXISTS idx_code_desc_unique ON code_desc(path, type);
`)
	if err != nil {
		return nil, err
	}
	if err := ensureFunctionsProvenanceColumns(db); err != nil {
		return nil, err
	}
	if err := ensureDocSchema(db); err != nil {
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
    name, package, file, description, start_line, end_line, function_type, source, page, slide
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	callStmt, _ := tx.Prepare("INSERT INTO calls(caller, callee) VALUES(?, ?)")
	extStmt, _ := tx.Prepare("INSERT INTO externals(function, external) VALUES(?, ?)")
	for _, res := range results {
		source := res.Func.Source
		if source == "" {
			source = res.Func.File
		}
		_, err = funcStmt.Exec(
			res.Func.Name,
			res.Func.Package,
			res.Func.File,
			res.Description,
			res.Func.StartLine,
			res.Func.EndLine,
			res.Func.FunctionType,
			source,
			res.Func.Page,
			res.Func.Slide,
		)
		if err != nil {
			log.Printf("Insertion of functions failed (skipped?): %v", err)
		}
		// calls 和 externals 如果也可能重复，可以同样建唯一索引并用 OR IGNORE
		for _, callee := range res.InternalDeps {
			if _, err := callStmt.Exec(res.Func.Name, callee); err != nil {
				log.Printf("Failed to insert calls: %v", err)
			}
		}
		for _, ext := range res.ExternalDeps {
			if _, err := extStmt.Exec(res.Func.Name, ext); err != nil {
				log.Printf("Failed to insert externals: %v", err)
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
    name, package, file, description, start_line, end_line, function_type, source, page, slide
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	callStmt, _ := tx.Prepare("INSERT INTO calls(caller, callee) VALUES(?, ?)")
	extStmt, _ := tx.Prepare("INSERT INTO externals(function, external) VALUES(?, ?)")
	for _, res := range results {
		if res.Func.FunctionType == "llm_parser" && res.Func.Source == "" && res.Func.Page == 0 && res.Func.Slide == 0 {
			logs.Warnf("[WARN] Ignoring library entry %s for LLM_PARSER function", res.Func.Name)
			continue
		}
		if projDir != "" {
			res.Func.File, err = filepath.Rel(projDir, res.Func.File)
			if err != nil {
				fmt.Errorf("%s, %s, cannot convert file path to relative path: %w", projDir, res.Func.File, err)
			}
			res.Func.File = filepath.ToSlash(res.Func.File)
			logs.Infof("[DEBUG][DB] The storage file path is: %s", res.Func.File)

			if res.Func.Source != "" {
				sourcePath := res.Func.Source
				sourceSuffix := ""
				if strings.Contains(sourcePath, "::") {
					parts := strings.SplitN(sourcePath, "::", 2)
					sourcePath = parts[0]
					sourceSuffix = "::" + parts[1]
				}
				if !filepath.IsAbs(sourcePath) {
					sourcePath = filepath.Join(projDir, sourcePath)
				}
				if rel, relErr := filepath.Rel(projDir, sourcePath); relErr == nil {
					res.Func.Source = filepath.ToSlash(rel) + sourceSuffix
				}
			}
		}
		source := res.Func.Source
		if source == "" {
			source = res.Func.File
		}
		_, err = funcStmt.Exec(
			res.Func.Name,
			res.Func.Package,
			res.Func.File,
			res.Description,
			res.Func.StartLine,
			res.Func.EndLine,
			res.Func.FunctionType,
			source,
			res.Func.Page,
			res.Func.Slide,
		)
		if err != nil {
			log.Printf("Insertion of functions failed (skipped?): %v", err)
		}
		// calls 和 externals 如果也可能重复，可以同样建唯一索引并用 OR IGNORE
		for _, callee := range res.InternalDeps {
			if _, err := callStmt.Exec(res.Func.Name, callee); err != nil {
				log.Printf("Failed to insert calls: %v", err)
			}
		}
		for _, ext := range res.ExternalDeps {
			if _, err := extStmt.Exec(res.Func.Name, ext); err != nil {
				log.Printf("Failed to insert externals: %v", err)
			}
		}
	}

	return tx.Commit()
}

func ensureFunctionsProvenanceColumns(db *sql.DB) error {
	rows, err := db.Query("PRAGMA table_info(functions)")
	if err != nil {
		return err
	}
	defer rows.Close()

	existing := map[string]bool{}
	for rows.Next() {
		var (
			cid       int
			name      string
			colType   string
			notNull   int
			defaultV  sql.NullString
			primaryID int
		)
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultV, &primaryID); err != nil {
			return err
		}
		existing[name] = true
	}

	needed := map[string]string{
		"source": "TEXT",
		"page":   "INTEGER DEFAULT 0",
		"slide":  "INTEGER DEFAULT 0",
	}

	for col, ddl := range needed {
		if existing[col] {
			continue
		}
		if _, err := db.Exec(fmt.Sprintf("ALTER TABLE functions ADD COLUMN %s %s", col, ddl)); err != nil {
			return err
		}
	}
	return nil
}

// --- Vector indexing (Faiss) ---

// NewFaissWrapper creates a new HTTP-based FAISS wrapper.
// Returns error if the FAISS HTTP server is not reachable (no in-memory fallback).
func NewFaissWrapper(dimension int, options ...map[string]interface{}) (FaissWrapper, error) {
	// Parse optional arguments
	var opts map[string]interface{}
	if len(options) > 0 {
		opts = options[0]
	}

	// Get server URL, default to DefaultFaissServerURL
	serverURL := DefaultFaissServerURL
	if url, ok := opts["server_url"].(string); ok && url != "" {
		serverURL = url
	}

	// Get index ID, default to "default"
	indexID := "default"
	if id, ok := opts["index_id"].(string); ok && id != "" {
		indexID = id
	}

	// Create HTTP FAISS wrapper (no in-memory fallback)
	httpWrapper, err := NewHTTPFaissWrapper(dimension, true, serverURL, indexID)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to FAISS HTTP server at %s: %w", serverURL, err)
	}

	// Set storage path if provided
	if path, ok := opts["storage_path"].(string); ok && path != "" {
		httpWrapper.storagePath = path
		logs.Infof("Faiss index will use storage path: %s", path)
	}
	return httpWrapper, nil
}

// NewZvecFaissWrapper creates a Zvec-based vector engine wrapper.
// collectionPath: Collection storage directory (e.g. .gitgo/zvec_collections)
// pythonPath: Python executable path, empty uses default python3
// Returns error if Zvec engine initialization fails (no in-memory fallback).
func NewZvecFaissWrapper(dimension int, collectionPath string, pythonPath string) (FaissWrapper, error) {
	wrapper, err := NewZvecWrapper(dimension, collectionPath, pythonPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create Zvec engine: %w", err)
	}
	return wrapper, nil
}

// NewFaissWrapperByEngine selects vector engine by engine parameter.
// engine: "zvec" uses Zvec engine, "faiss" or "" uses HTTP FAISS engine.
// Returns (FaissWrapper, error) - no silent fallback to in-memory implementation.
func NewFaissWrapperByEngine(engine string, dimension int, options ...map[string]interface{}) (FaissWrapper, error) {
	var opts map[string]interface{}
	if len(options) > 0 {
		opts = options[0]
	} else {
		opts = map[string]interface{}{}
	}

	switch engine {
	case "zvec":
		collectionPath, _ := opts["collection_path"].(string)
		if collectionPath == "" {
			collectionPath = filepath.Join(".gitgo", "zvec_collections")
		}
		pythonPath, _ := opts["python_path"].(string)
		logs.Infof("Using Zvec engine, collection_path=%s, dimension=%d", collectionPath, dimension)
		return NewZvecFaissWrapper(dimension, collectionPath, pythonPath)

	default:
		// Default: use HTTP FAISS engine (no in-memory fallback)
		logs.Infof("Using FAISS HTTP engine (engine=%s), dimension=%d", engine, dimension)
		return NewFaissWrapper(dimension, opts)
	}
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

// InitCodeDescDb 初始化 code_desc 表
func InitCodeDescDb(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS code_desc (
    id INTEGER PRIMARY KEY,
    name TEXT,                  -- 文件名或文件夹名
    type TEXT,                  -- 类型：'file' 或 'directory'
    path TEXT,                  -- 相对路径
    parent_path TEXT,           -- 上层目录路径
    function_count INTEGER,     -- 子函数数量
    file_count INTEGER,         -- 子文件数量
    description TEXT,           -- 模块功能描述
    updated_at TIMESTAMP,       -- 更新时间
    created_at TIMESTAMP        -- 创建时间
);
`)
	if err != nil {
		return err
	}
	logs.Infof("Initialization of code_desc table successful")
	// 为 code_desc 表添加索引，提高按路径查询的性能
	_, err = db.Exec(`
CREATE INDEX IF NOT EXISTS idx_code_desc_path ON code_desc(path);
CREATE INDEX IF NOT EXISTS idx_code_desc_parent ON code_desc(parent_path);
CREATE UNIQUE INDEX IF NOT EXISTS idx_code_desc_unique ON code_desc(path, type);
`)
	if err != nil {
		return err
	}
	logs.Infof("Initialization of code_desc index successful")
	return nil
}
