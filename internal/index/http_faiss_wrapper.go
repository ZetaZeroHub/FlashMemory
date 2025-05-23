package index

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// 默认的Faiss HTTP服务URL
var DefaultFaissServerURL = "http://localhost:5533"

// HTTPFaissWrapper 是通过HTTP API调用Python Faiss服务的封装
type HTTPFaissWrapper struct {
	Dim        int             // 向量维度
	MetricType int             // 0: L2, 1: IP (内积)
	Scores     map[int]float32 // 存储搜索结果的分数
	ServerURL  string          // Faiss服务的URL
	IndexID    string          // 索引ID，用于在服务端区分不同的索引
	HTTPClient *http.Client    // HTTP客户端

	// 向量缓存，用于减少重复计算和提高性能
	vectorCache  map[string][]float32 // 缓存键到向量的映射
	cacheEnabled bool                 // 是否启用缓存
	maxCacheSize int                  // 最大缓存条目数
	cacheHits    int                  // 缓存命中次数
	cacheMisses  int                  // 缓存未命中次数

	// 持久化相关
	lastSavePath string // 上次保存的路径，用于增量更新
	dirtyFlag    bool   // 标记索引是否被修改过
	storagePath  string // 存储路径，用于保存和加载索引

	// 相似性计算相关
	useDistance bool // 是否使用距离度量
}

// Dimension 返回向量的维度
func (fw *HTTPFaissWrapper) Dimension() int {
	return fw.Dim
}

// GetScore 返回指定函数ID对应的相似度分数
func (fw *HTTPFaissWrapper) GetScore(funcID int) float32 {
	score, ok := fw.Scores[funcID]
	if !ok {
		return 0
	}
	return score
}

// NewHTTPFaissWrapper 创建一个新的Faiss HTTP客户端
func NewHTTPFaissWrapper(dimension int, useInnerProduct bool, serverURL string, indexID ...string) (*HTTPFaissWrapper, error) {
	metricType := 0 // METRIC_L2 默认使用L2距离
	if useInnerProduct {
		metricType = 1 // METRIC_INNER_PRODUCT 使用内积
	}

	// 创建HTTP客户端，设置超时
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	// 设置索引ID，如果提供了自定义ID则使用，否则使用默认值
	id := "default"
	if len(indexID) > 0 && indexID[0] != "" {
		id = indexID[0]
	}

	fw := &HTTPFaissWrapper{
		Dim:          dimension,
		MetricType:   metricType,
		Scores:       make(map[int]float32),
		ServerURL:    serverURL,
		IndexID:      id,
		HTTPClient:   httpClient,
		vectorCache:  make(map[string][]float32),
		cacheEnabled: true,
		maxCacheSize: 10000, // 默认最大缓存10000条
		cacheHits:    0,
		cacheMisses:  0,
		dirtyFlag:    false,
		useDistance:  !useInnerProduct,
		storagePath:  "", // 初始化存储路径为空字符串
	}

	// 在服务端创建索引
	reqBody, _ := json.Marshal(map[string]interface{}{
		"index_id":          fw.IndexID,
		"dimension":         dimension,
		"use_inner_product": useInnerProduct,
	})

	resp, err := httpClient.Post(serverURL+"/create_index", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Faiss server: %v", err)
	}
	defer resp.Body.Close()

	// 如果返回 400，认为索引已存在，默认成功
	if resp.StatusCode == http.StatusBadRequest {
		logs.Warnf("索引已存在，请勿重复创建")
		return fw, nil
	}
	// 如果不是 200，则读取响应体并报错
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create index, status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// 读取并解析响应
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %v", err)
	}

	// 解析响应
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	// 检查响应状态
	if status, ok := result["status"].(string); !ok || status != "success" {
		message := "unknown error"
		if msg, ok := result["message"].(string); ok {
			message = msg
		}
		return nil, fmt.Errorf("failed to create index: %s", message)
	}

	return fw, nil
}

// retryRequest 封装HTTP请求重试逻辑
func (fw *HTTPFaissWrapper) retryRequest(method, endpoint string, reqBody []byte, maxRetries int) ([]byte, error) {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		resp, err := fw.HTTPClient.Post(fw.ServerURL+endpoint, "application/json", bytes.NewBuffer(reqBody))
		if err != nil {
			lastErr = fmt.Errorf("attempt %d failed: %v", i+1, err)
			time.Sleep(time.Second * time.Duration(i+1)) // 指数退避
			continue
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = fmt.Errorf("attempt %d failed to read response: %v", i+1, err)
			continue
		}

		return respBody, nil
	}
	return nil, fmt.Errorf("all retries failed: %v", lastErr)
}

// AddVector 添加一个向量到索引
func (fw *HTTPFaissWrapper) AddVector(funcID int, vector []float32) error {
	// 标记索引已被修改
	fw.dirtyFlag = true

	// 缓存向量
	if fw.cacheEnabled {
		// 创建向量的副本以避免外部修改
		vectorCopy := make([]float32, len(vector))
		copy(vectorCopy, vector)

		// 使用函数ID作为缓存键
		cacheKey := fmt.Sprintf("func_%d", funcID)
		fw.vectorCache[cacheKey] = vectorCopy
	}

	// 准备请求体
	reqBody, _ := json.Marshal(map[string]interface{}{
		"index_id": fw.IndexID,
		"func_id":  funcID,
		"vector":   vector,
	})

	// 发送请求（带重试）
	respBody, err := fw.retryRequest("POST", "/add_vector", reqBody, 3)
	if err != nil {
		return fmt.Errorf("failed to add vector: %v", err)
	}

	// 解析响应
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("failed to parse response: %v", err)
	}

	// 检查响应状态
	if status, ok := result["status"].(string); !ok || status != "success" {
		message := "unknown error"
		if msg, ok := result["message"].(string); ok {
			message = msg
		}
		return fmt.Errorf("failed to add vector: %s", message)
	}

	return nil
}

// AddVectorsBatch 批量添加向量到索引
func (fw *HTTPFaissWrapper) AddVectorsBatch(funcIDs []int, vectors []float32) error {
	// 准备请求体
	reqBody, _ := json.Marshal(map[string]interface{}{
		"index_id": fw.IndexID,
		"func_ids": funcIDs,
		"vectors":  vectors,
	})

	// 发送请求（带重试）
	respBody, err := fw.retryRequest("POST", "/add_vectors_batch", reqBody, 3)
	if err != nil {
		return fmt.Errorf("failed to add vectors batch: %v", err)
	}

	// 解析响应
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("failed to parse response: %v", err)
	}

	// 检查响应状态
	if status, ok := result["status"].(string); !ok || status != "success" {
		message := "unknown error"
		if msg, ok := result["message"].(string); ok {
			message = msg
		}
		return fmt.Errorf("failed to add vectors batch: %s", message)
	}

	return nil
}

// SearchVectors 查找最接近查询向量的topK个向量
func (fw *HTTPFaissWrapper) SearchVectors(query []float32, topK int) []int {
	// 生成查询向量的缓存键
	queryKey := fmt.Sprintf("query_%d", len(query))

	// 检查缓存中是否有相同的查询向量
	cachedVector, hasCached := fw.vectorCache[queryKey]
	if fw.cacheEnabled && hasCached && cosineSimilarity(query, cachedVector) > 0.99 {
		fw.cacheHits++
		// 如果缓存中有几乎相同的查询向量，可以考虑使用缓存的结果
		// 这里简化处理，仍然执行搜索，但可以在此扩展缓存搜索结果
		fmt.Println("Using similar cached query vector")
	} else {
		fw.cacheMisses++
	}

	// 清理过期缓存
	fw.cleanCache()

	// 准备请求体
	reqBody, _ := json.Marshal(map[string]interface{}{
		"index_id": fw.IndexID,
		"query":    query,
		"top_k":    topK,
	})

	// 发送请求（带重试）
	respBody, err := fw.retryRequest("POST", "/search_vectors", reqBody, 3)
	if err != nil {
		fmt.Printf("failed to search vectors: %v\n", err)
		return []int{}
	}

	// 解析响应
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		fmt.Printf("failed to parse response: %v\n", err)
		return []int{}
	}

	// 检查响应状态
	if status, ok := result["status"].(string); !ok || status != "success" {
		message := "unknown error"
		if msg, ok := result["message"].(string); ok {
			message = msg
		}
		fmt.Printf("search error: %s\n", message)
		return []int{}
	}

	// 缓存查询向量
	if fw.cacheEnabled {
		vectorCopy := make([]float32, len(query))
		copy(vectorCopy, query)
		fw.vectorCache[queryKey] = vectorCopy
	}

	// 获取结果
	resultsRaw, ok := result["results"].([]interface{})
	if !ok {
		fmt.Printf("invalid results format\n")
		return []int{}
	}

	// 转换结果为int切片
	results := make([]int, len(resultsRaw))
	for i, v := range resultsRaw {
		if id, ok := v.(float64); ok {
			results[i] = int(id)
		}
	}

	// 获取分数
	scoresRaw, ok := result["scores"].(map[string]interface{})
	if ok {
		for idStr, scoreRaw := range scoresRaw {
			var id int
			fmt.Sscanf(idStr, "%d", &id)
			if score, ok := scoreRaw.(float64); ok {
				fw.Scores[id] = float32(score)
			}
		}
	}

	return results
}

// SaveToFile 将Faiss索引保存到文件
// 如果设置了storagePath，则使用storagePath作为local_path参数
func (fw *HTTPFaissWrapper) SaveToFile(path string) error {
	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory for index: %v", err)
	}

	// 确定本地路径
	localPath := path
	if fw.storagePath != "" {
		// 如果设置了存储路径，则使用存储路径下的同名文件
		localPath = filepath.Join(fw.storagePath, filepath.Base(path))
		// 确保本地存储目录存在
		if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
			return fmt.Errorf("failed to create directory for local index: %v", err)
		}
	}

	// 准备请求体
	reqBody, _ := json.Marshal(map[string]interface{}{
		"index_id":   fw.IndexID,
		"path":       path,
		"local_path": localPath, // 使用确定的本地路径
	})

	// 发送请求（带重试）
	respBody, err := fw.retryRequest("POST", "/save_index", reqBody, 3)
	if err != nil {
		return fmt.Errorf("failed to save index: %v", err)
	}

	// 解析响应
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("failed to parse response: %v", err)
	}

	// 检查响应状态
	if status, ok := result["status"].(string); !ok || status != "success" {
		message := "unknown error"
		if msg, ok := result["message"].(string); ok {
			message = msg
		}
		return fmt.Errorf("failed to save index: %s", message)
	}

	// 记录最后保存的路径
	fw.lastSavePath = path
	// 重置脏标记
	fw.dirtyFlag = false

	return nil
}

// LoadFromFile 从文件加载Faiss索引
func (fw *HTTPFaissWrapper) LoadFromFile(path string) error {
	logs.Infof("HTTPFaissWrapper Loading index from %s", path)
	// 确定本地路径
	localPath := path
	if fw.storagePath != "" {
		// 如果设置了存储路径，则使用存储路径下的同名文件
		localPath = filepath.Join(fw.storagePath, filepath.Base(path))
	}

	// 准备请求体
	reqBody, _ := json.Marshal(map[string]interface{}{
		"index_id":   fw.IndexID,
		"path":       path,
		"local_path": localPath, // 使用确定的本地路径
	})

	// 发送请求（带重试）
	respBody, err := fw.retryRequest("POST", "/load_index", reqBody, 3)
	if err != nil {
		return fmt.Errorf("failed to load index: %v", err)
	}

	// 解析响应
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("failed to parse response: %v", err)
	}

	// 检查响应状态
	if status, ok := result["status"].(string); !ok || status != "success" {
		message := "unknown error"
		if msg, ok := result["message"].(string); ok {
			message = msg
		}
		return fmt.Errorf("failed to load index: %s", message)
	}

	// 更新维度和度量类型
	if dim, ok := result["dimension"].(float64); ok {
		fw.Dim = int(dim)
	}
	if metricType, ok := result["metric_type"].(float64); ok {
		fw.MetricType = int(metricType)
	}

	// 记录加载的路径
	fw.lastSavePath = path
	// 重置脏标记
	fw.dirtyFlag = false

	return nil
}

// SetSimilarityMetric 设置相似度计算方法
func (fw *HTTPFaissWrapper) SetSimilarityMetric(metric string) {
	if metric == "euclidean" {
		fw.useDistance = true
	} else {
		fw.useDistance = false
	}
}

// EnableCache 启用向量缓存
func (fw *HTTPFaissWrapper) EnableCache() {
	fw.cacheEnabled = true
	fmt.Println("Vector cache enabled")
}

// DisableCache 禁用向量缓存
func (fw *HTTPFaissWrapper) DisableCache() {
	fw.cacheEnabled = false
	fmt.Println("Vector cache disabled")
}

// ClearCache 清除向量缓存
func (fw *HTTPFaissWrapper) ClearCache() {
	fw.vectorCache = make(map[string][]float32)
	fmt.Println("Vector cache cleared")
}

// GetCacheStats 获取缓存统计信息
func (fw *HTTPFaissWrapper) GetCacheStats() map[string]interface{} {
	return map[string]interface{}{
		"enabled":        fw.cacheEnabled,
		"cache_size":     len(fw.vectorCache),
		"max_cache_size": fw.maxCacheSize,
		"cache_hits":     fw.cacheHits,
		"cache_misses":   fw.cacheMisses,
		"dirty":          fw.dirtyFlag,
		"last_save":      fw.lastSavePath,
	}
}

// Free 释放资源
func (fw *HTTPFaissWrapper) Free() {
	// 删除服务器上的索引
	reqBody, _ := json.Marshal(map[string]interface{}{
		"index_id": fw.IndexID,
	})

	// 发送请求（带重试）
	_, err := fw.retryRequest("POST", "/delete_index", reqBody, 3)
	if err != nil {
		fmt.Printf("failed to delete index: %v\n", err)
	}
}

// cleanCache 清理过期的缓存条目
func (fw *HTTPFaissWrapper) cleanCache() {
	if len(fw.vectorCache) > fw.maxCacheSize {
		// 简单策略：当缓存超过限制时，清空一半的缓存
		numToDelete := len(fw.vectorCache) / 2
		deleted := 0
		for key := range fw.vectorCache {
			delete(fw.vectorCache, key)
			deleted++
			if deleted >= numToDelete {
				break
			}
		}
	}
}
