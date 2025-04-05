package index

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestEnsureIndexDB 测试数据库初始化
func TestEnsureIndexDB(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "testindexdb")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	db, err := EnsureIndexDB(tempDir)
	if err != nil {
		t.Fatalf("EnsureIndexDB 失败: %v", err)
	}
	if db == nil {
		t.Fatalf("数据库返回 nil")
	}

	// 检查表是否创建成功
	_, err = db.Exec("SELECT * FROM functions LIMIT 1")
	if err != nil {
		t.Fatalf("查询表失败: %v", err)
	}
}

// TestMemoryFaissWrapper_AddAndSearch 测试内存实现向量添加和搜索
func TestMemoryFaissWrapper_AddAndSearch(t *testing.T) {
	dim := 10
	mfw := NewMemoryFaissWrapper(dim)
	vector := make([]float32, dim)
	for i := 0; i < dim; i++ {
		vector[i] = float32(i)
	}

	// 添加向量
	if err := mfw.AddVector(1, vector); err != nil {
		t.Fatalf("AddVector 失败: %v", err)
	}

	// 搜索相同的向量
	results := mfw.SearchVectors(vector, 1)
	if len(results) != 1 || results[0] != 1 {
		t.Fatalf("SearchVectors 返回不符合预期: %v", results)
	}
}

// TestMemoryFaissWrapper_AddVectorsBatch 测试内存实现的批量添加
func TestMemoryFaissWrapper_AddVectorsBatch(t *testing.T) {
	dim := 5
	mfw := NewMemoryFaissWrapper(dim)
	funcIDs := []int{1, 2}
	vectors := []float32{
		1, 2, 3, 4, 5,
		5, 4, 3, 2, 1,
	}

	if err := mfw.AddVectorsBatch(funcIDs, vectors); err != nil {
		t.Fatalf("AddVectorsBatch 失败: %v", err)
	}

	// 搜索一个与第一个向量相似的向量
	query := []float32{1, 2, 3, 4, 5}
	results := mfw.SearchVectors(query, 2)
	if len(results) == 0 {
		t.Fatalf("SearchVectors 未返回结果")
	}
}

// TestMemoryFaissWrapper_SaveToFile 测试内存实现保存索引到文件
func TestMemoryFaissWrapper_SaveToFile(t *testing.T) {
	dim := 3
	mfw := NewMemoryFaissWrapper(dim)
	vector := []float32{1, 2, 3}
	if err := mfw.AddVector(1, vector); err != nil {
		t.Fatalf("AddVector 失败: %v", err)
	}

	tmpFile, err := os.CreateTemp("", "faiss_index_*.txt")
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	tmpFileName := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpFileName)

	if err := mfw.SaveToFile(tmpFileName); err != nil {
		t.Fatalf("SaveToFile 失败: %v", err)
	}

	// 读取保存的文件，检查是否包含正确的头部信息
	content, err := os.ReadFile(tmpFileName)
	if err != nil {
		t.Fatalf("读取保存文件失败: %v", err)
	}
	expectedHeader := fmt.Sprintf("FaissIndex Dim=%d", dim)
	if !bytes.Contains(content, []byte(expectedHeader)) {
		t.Fatalf("保存文件中不包含预期头部, got: %s", string(content))
	}
}

// TestHTTPFaissWrapper 使用 httptest 模拟 HTTP Faiss 服务测试 HTTP 版实现
func TestHTTPFaissWrapper(t *testing.T) {
	// 模拟的 Faiss HTTP 服务处理器
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var response map[string]interface{}
		switch r.URL.Path {
		case "/create_index":
			response = map[string]interface{}{"status": "success"}
		case "/add_vector":
			response = map[string]interface{}{"status": "success"}
		case "/add_vectors_batch":
			response = map[string]interface{}{"status": "success"}
		case "/search_vectors":
			// 返回一个模拟结果：函数ID为 1，分数为 1.0
			response = map[string]interface{}{
				"status":  "success",
				"results": []interface{}{1},
				"scores":  map[string]interface{}{"1": 1.0},
			}
		case "/save_index":
			response = map[string]interface{}{"status": "success"}
		case "/load_index":
			response = map[string]interface{}{
				"status":      "success",
				"dimension":   10,
				"metric_type": 1,
			}
		case "/delete_index":
			response = map[string]interface{}{"status": "success"}
		default:
			response = map[string]interface{}{"status": "error", "message": "unknown endpoint"}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer ts.Close()

	// 使用模拟服务地址创建 HTTPFaissWrapper
	fw, err := NewHTTPFaissWrapper(10, true, ts.URL)
	if err != nil {
		t.Fatalf("NewHTTPFaissWrapper 失败: %v", err)
	}

	// 测试 AddVector
	vector := make([]float32, 10)
	for i := 0; i < 10; i++ {
		vector[i] = float32(i)
	}
	if err := fw.AddVector(1, vector); err != nil {
		t.Fatalf("HTTPFaissWrapper AddVector 失败: %v", err)
	}

	// 测试 SearchVectors
	results := fw.SearchVectors(vector, 1)
	if len(results) != 1 || results[0] != 1 {
		t.Fatalf("HTTPFaissWrapper SearchVectors 返回不符合预期: %v", results)
	}

	// 测试 SaveToFile
	tmpFile, err := os.CreateTemp("", "http_faiss_index_*.txt")
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	tmpFileName := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpFileName)
	if err := fw.SaveToFile(tmpFileName); err != nil {
		t.Fatalf("HTTPFaissWrapper SaveToFile 失败: %v", err)
	}

	// 测试 LoadFromFile
	if err := fw.LoadFromFile(tmpFileName); err != nil {
		t.Fatalf("HTTPFaissWrapper LoadFromFile 失败: %v", err)
	}

	// 测试 Free (删除索引)
	fw.Free()
}

// TestNewFaissWrapperFallback 测试 NewFaissWrapper 的回退逻辑（当 HTTP 服务不可用时应使用内存实现）
func TestNewFaissWrapperFallback(t *testing.T) {
	// 临时修改 DefaultFaissServerURL 为一个无效地址，迫使 HTTP 实现创建失败
	originalURL := DefaultFaissServerURL
	DefaultFaissServerURL = "http://nonexistent.invalid"
	defer func() { DefaultFaissServerURL = originalURL }()

	wrapper := NewFaissWrapper(10)
	// 期望 fallback 到 MemoryFaissWrapper
	if _, ok := wrapper.(*MemoryFaissWrapper); !ok {
		t.Fatalf("预期回退为 MemoryFaissWrapper, 实际类型: %T", wrapper)
	}
}

// TestDirectoryCreationForSaveIndex 测试 SaveToFile 保存时目录不存在的情况
func TestDirectoryCreationForSaveIndex(t *testing.T) {
	dim := 3
	mfw := NewMemoryFaissWrapper(dim)
	vector := []float32{1, 2, 3}
	if err := mfw.AddVector(1, vector); err != nil {
		t.Fatalf("AddVector 失败: %v", err)
	}

	// 创建一个临时目录，然后删除，确保保存时目录不存在
	tempDir, err := os.MkdirTemp("", "faiss_save_test")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	os.RemoveAll(tempDir) // 删除目录

	// 指定一个新目录中的文件路径
	savePath := filepath.Join(tempDir, "subdir", "index.txt")
	err = mfw.SaveToFile(savePath)
	if err != nil {
		t.Fatalf("SaveToFile 在不存在的目录下失败: %v", err)
	}

	// 检查文件是否存在且可读
	if _, err := os.Stat(savePath); err != nil && !os.IsNotExist(err) {
		t.Fatalf("保存的索引文件不存在: %v", err)
	}
}
