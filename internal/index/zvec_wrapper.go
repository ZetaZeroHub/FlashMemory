package index

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"
)

// ZvecWrapper 通过 subprocess stdin/stdout 调用 Python Zvec 引擎
// 实现 FaissWrapper 接口，可无缝替换 HTTPFaissWrapper / MemoryFaissWrapper
type ZvecWrapper struct {
	Dim            int
	CollectionPath string // .gitgo/zvec_collections
	PythonPath     string // Python 可执行文件路径

	Scores map[int]float32 // 搜索结果的分数缓存

	// subprocess 管理
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner
	mu     sync.Mutex // 保护 stdin/stdout 的并发访问

	// 向量缓存
	vectorCache  map[string][]float32
	cacheEnabled bool

	// 状态
	ready     bool
	dirtyFlag bool
}

// zvecRequest 发送给 Python Bridge 的 JSON-line 请求
type zvecRequest struct {
	Action string      `json:"action"`
	Params interface{} `json:"params"`
}

// zvecResponse Python Bridge 返回的 JSON-line 响应
type zvecResponse struct {
	Status  string                 `json:"status"`
	Data    map[string]interface{} `json:"data"`
	Message string                 `json:"message"`
}

// NewZvecWrapper 创建一个新的 Zvec 向量引擎封装
func NewZvecWrapper(dimension int, collectionPath string, pythonPath string) (*ZvecWrapper, error) {
	if pythonPath == "" {
		pythonPath = "python3"
	}

	zw := &ZvecWrapper{
		Dim:            dimension,
		CollectionPath: collectionPath,
		PythonPath:     pythonPath,
		Scores:         make(map[int]float32),
		vectorCache:    make(map[string][]float32),
		cacheEnabled:   true,
		dirtyFlag:      false,
	}

	// 启动 Python Bridge 子进程
	if err := zw.startBridge(); err != nil {
		return nil, fmt.Errorf("启动 Zvec Bridge 失败: %w", err)
	}

	// 初始化 Collection
	if err := zw.initCollection(false); err != nil {
		zw.Free()
		return nil, fmt.Errorf("初始化 Zvec Collection 失败: %w", err)
	}

	logs.Infof("ZvecWrapper 创建成功, dimension=%d, path=%s", dimension, collectionPath)
	return zw, nil
}

// startBridge 启动 Python zvec_bridge 子进程
func (zw *ZvecWrapper) startBridge() error {
	// 定位 zvec_bridge.py 脚本
	bridgeScript := zw.findBridgeScript()
	if bridgeScript == "" {
		return fmt.Errorf("找不到 zvec_bridge.py 脚本")
	}

	logs.Infof("启动 Zvec Bridge: %s %s", zw.PythonPath, bridgeScript)

	zw.cmd = exec.Command(zw.PythonPath, "-u", bridgeScript)
	zw.cmd.Stderr = os.Stderr // Python 日志输出到 stderr

	var err error
	zw.stdin, err = zw.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("获取 stdin pipe 失败: %w", err)
	}

	stdoutPipe, err := zw.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("获取 stdout pipe 失败: %w", err)
	}
	zw.stdout = bufio.NewScanner(stdoutPipe)
	// 增大 scanner 缓冲区以处理大的搜索结果
	zw.stdout.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	if err := zw.cmd.Start(); err != nil {
		return fmt.Errorf("启动 Python 进程失败: %w", err)
	}

	// 等待 ready 信号
	resp, err := zw.readResponse()
	if err != nil {
		return fmt.Errorf("等待 Bridge ready 失败: %w", err)
	}
	if resp.Status != "success" || resp.Message != "ready" {
		return fmt.Errorf("Bridge 返回异常: %s", resp.Message)
	}

	zw.ready = true
	logs.Infof("Zvec Bridge 已就绪")
	return nil
}

// findBridgeScript 查找 zvec_bridge.py 脚本路径
func (zw *ZvecWrapper) findBridgeScript() string {
	// 查找策略：
	// 1. 从可执行文件目录查找
	// 2. 从源代码目录查找
	// 3. 从工作目录查找

	candidates := []string{}

	// 可执行文件同目录
	if exePath, err := os.Executable(); err == nil {
		candidates = append(candidates,
			filepath.Join(filepath.Dir(exePath), "pip-package", "flashmemory", "zvec_bridge.py"),
		)
	}

	// 当前工作目录
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(cwd, "pip-package", "flashmemory", "zvec_bridge.py"),
		)
	}

	// 源码目录 (开发模式)
	// 通过 runtime 获取源文件路径
	candidates = append(candidates,
		filepath.Join("pip-package", "flashmemory", "zvec_bridge.py"),
	)

	for _, path := range candidates {
		if absPath, err := filepath.Abs(path); err == nil {
			if _, err := os.Stat(absPath); err == nil {
				logs.Infof("找到 zvec_bridge.py: %s", absPath)
				return absPath
			}
		}
	}

	return ""
}

// sendRequest 向 Python Bridge 发送请求并读取响应
func (zw *ZvecWrapper) sendRequest(action string, params interface{}) (*zvecResponse, error) {
	zw.mu.Lock()
	defer zw.mu.Unlock()

	if !zw.ready {
		return nil, fmt.Errorf("Zvec Bridge 未就绪")
	}

	req := zvecRequest{
		Action: action,
		Params: params,
	}

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	// 写入 stdin
	if _, err := zw.stdin.Write(append(reqBytes, '\n')); err != nil {
		return nil, fmt.Errorf("写入 stdin 失败: %w", err)
	}

	// 读取响应
	return zw.readResponse()
}

// readResponse 从 stdout 读取一行 JSON 响应
func (zw *ZvecWrapper) readResponse() (*zvecResponse, error) {
	// 设置读取超时 (通过 context 或简单轮询)
	done := make(chan bool, 1)
	var resp zvecResponse
	var scanErr error

	go func() {
		if zw.stdout.Scan() {
			line := zw.stdout.Text()
			if err := json.Unmarshal([]byte(line), &resp); err != nil {
				scanErr = fmt.Errorf("解析响应 JSON 失败: %w, 原始数据: %s", err, line)
			}
		} else {
			if err := zw.stdout.Err(); err != nil {
				scanErr = fmt.Errorf("读取 stdout 失败: %w", err)
			} else {
				scanErr = fmt.Errorf("Bridge 进程已退出")
			}
		}
		done <- true
	}()

	select {
	case <-done:
		if scanErr != nil {
			return nil, scanErr
		}
		return &resp, nil
	case <-time.After(60 * time.Second):
		return nil, fmt.Errorf("读取响应超时 (60s)")
	}
}

// initCollection 初始化 Zvec Collection
func (zw *ZvecWrapper) initCollection(forceNew bool) error {
	resp, err := zw.sendRequest("init", map[string]interface{}{
		"collection_path": zw.CollectionPath,
		"dimension":       zw.Dim,
		"force_new":       forceNew,
		"collection_type": "functions", // Phase 1 先只初始化函数级
	})
	if err != nil {
		return err
	}
	if resp.Status != "success" {
		return fmt.Errorf("初始化失败: %s", resp.Message)
	}
	logs.Infof("Zvec Collection 初始化成功: %s", resp.Message)
	return nil
}

// --- FaissWrapper 接口实现 ---

// Dimension 返回向量维度
func (zw *ZvecWrapper) Dimension() int {
	return zw.Dim
}

// GetScore 返回搜索结果中指定函数ID的分数
func (zw *ZvecWrapper) GetScore(funcID int) float32 {
	score, ok := zw.Scores[funcID]
	if !ok {
		return 0
	}
	return score
}

// AddVector 添加单个向量到索引
func (zw *ZvecWrapper) AddVector(funcID int, vector []float32) error {
	zw.dirtyFlag = true

	// 截断或填充向量到正确维度
	if len(vector) != zw.Dim {
		resized := make([]float32, zw.Dim)
		copy(resized, vector)
		vector = resized
	}

	funcIdStr := fmt.Sprintf("func_%d", funcID)

	resp, err := zw.sendRequest("add_vector", map[string]interface{}{
		"func_id":  funcIdStr,
		"vector":   vector,
		"metadata": map[string]interface{}{},
	})
	if err != nil {
		return fmt.Errorf("添加向量失败 (id=%d): %w", funcID, err)
	}
	if resp.Status != "success" {
		return fmt.Errorf("添加向量失败: %s", resp.Message)
	}

	return nil
}

// AddVectorsBatch 批量添加向量
func (zw *ZvecWrapper) AddVectorsBatch(funcIDs []int, vectors []float32) error {
	zw.dirtyFlag = true

	items := make([]map[string]interface{}, len(funcIDs))
	for i, id := range funcIDs {
		vec := vectors[i*zw.Dim : (i+1)*zw.Dim]
		vecCopy := make([]float64, len(vec))
		for j, v := range vec {
			vecCopy[j] = float64(v)
		}
		items[i] = map[string]interface{}{
			"func_id":  fmt.Sprintf("func_%d", id),
			"vector":   vecCopy,
			"metadata": map[string]interface{}{},
		}
	}

	resp, err := zw.sendRequest("add_vectors_batch", map[string]interface{}{
		"items": items,
	})
	if err != nil {
		return fmt.Errorf("批量添加向量失败: %w", err)
	}
	if resp.Status != "success" {
		return fmt.Errorf("批量添加向量失败: %s", resp.Message)
	}

	return nil
}

// SearchVectors 搜索最相似的 topK 个向量
func (zw *ZvecWrapper) SearchVectors(query []float32, topK int) []int {
	// 确保查询向量维度正确
	if len(query) != zw.Dim {
		resized := make([]float32, zw.Dim)
		copy(resized, query)
		query = resized
	}

	// 将 float32 转换为 float64 以便 JSON 序列化
	queryF64 := make([]float64, len(query))
	for i, v := range query {
		queryF64[i] = float64(v)
	}

	resp, err := zw.sendRequest("search", map[string]interface{}{
		"query":           queryF64,
		"top_k":           topK,
		"collection_type": "functions",
	})
	if err != nil {
		logs.Errorf("Zvec search failed: %v", err)
		return []int{}
	}
	if resp.Status != "success" {
		logs.Errorf("Zvec search error: %s", resp.Message)
		return []int{}
	}

	return zw.parseSearchResults(resp)
}

// SearchVectorsWithFilter performs vector search with scalar filter expression
func (zw *ZvecWrapper) SearchVectorsWithFilter(query []float32, topK int, filter string) []int {
	if len(query) != zw.Dim {
		resized := make([]float32, zw.Dim)
		copy(resized, query)
		query = resized
	}

	queryF64 := make([]float64, len(query))
	for i, v := range query {
		queryF64[i] = float64(v)
	}

	params := map[string]interface{}{
		"query":           queryF64,
		"top_k":           topK,
		"collection_type": "functions",
	}
	if filter != "" {
		params["filter"] = filter
	}

	resp, err := zw.sendRequest("search", params)
	if err != nil {
		logs.Errorf("Zvec filtered search failed: %v", err)
		return []int{}
	}
	if resp.Status != "success" {
		logs.Errorf("Zvec filtered search error: %s", resp.Message)
		return []int{}
	}

	return zw.parseSearchResults(resp)
}

// HybridSearchVectors performs Dense + Sparse multi-vector search with RRF fusion
// This is the Phase 2 core method that replaces hardcoded weight fusion in search.go
func (zw *ZvecWrapper) HybridSearchVectors(denseQuery []float32, sparseQuery map[string]float64, topK int, filter string) []int {
	if len(denseQuery) != zw.Dim {
		resized := make([]float32, zw.Dim)
		copy(resized, denseQuery)
		denseQuery = resized
	}

	denseF64 := make([]float64, len(denseQuery))
	for i, v := range denseQuery {
		denseF64[i] = float64(v)
	}

	params := map[string]interface{}{
		"dense_query": denseF64,
		"top_k":       topK,
		"use_rrf":     true,
	}
	if sparseQuery != nil && len(sparseQuery) > 0 {
		params["sparse_query"] = sparseQuery
	}
	if filter != "" {
		params["filter"] = filter
	}

	resp, err := zw.sendRequest("hybrid_search", params)
	if err != nil {
		logs.Errorf("Zvec hybrid search failed: %v", err)
		return []int{}
	}
	if resp.Status != "success" {
		logs.Errorf("Zvec hybrid search error: %s", resp.Message)
		return []int{}
	}

	// Log search type for debugging
	if searchType, ok := resp.Data["search_type"].(string); ok {
		logs.Infof("Hybrid search completed, type=%s", searchType)
	}

	return zw.parseSearchResults(resp)
}

// parseSearchResults extracts func IDs from a zvec search response
func (zw *ZvecWrapper) parseSearchResults(resp *zvecResponse) []int {
	resultsRaw, ok := resp.Data["results"].([]interface{})
	if !ok {
		logs.Errorf("Invalid search result format")
		return []int{}
	}

	type idScorePair struct {
		id    int
		score float32
	}
	var pairs []idScorePair

	for _, item := range resultsRaw {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		idStr, _ := itemMap["id"].(string)
		score, _ := itemMap["score"].(float64)

		var funcID int
		fmt.Sscanf(idStr, "func_%d", &funcID)

		if funcID > 0 {
			zw.Scores[funcID] = float32(score)
			pairs = append(pairs, idScorePair{id: funcID, score: float32(score)})
		}
	}

	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].score > pairs[j].score
	})

	results := make([]int, 0, len(pairs))
	for _, p := range pairs {
		results = append(results, p.id)
	}

	return results
}

// SaveToFile 保存索引 (Zvec 自动持久化，这里触发 optimize)
func (zw *ZvecWrapper) SaveToFile(path string) error {
	if !zw.dirtyFlag {
		logs.Infof("Zvec 索引未修改，跳过保存")
		return nil
	}

	resp, err := zw.sendRequest("optimize", map[string]interface{}{})
	if err != nil {
		return fmt.Errorf("优化索引失败: %w", err)
	}
	if resp.Status != "success" {
		return fmt.Errorf("优化索引失败: %s", resp.Message)
	}

	zw.dirtyFlag = false
	logs.Infof("Zvec 索引已优化并持久化")
	return nil
}

// LoadFromFile 加载索引 (Zvec 通过 open 自动加载，这里仅做兼容)
func (zw *ZvecWrapper) LoadFromFile(path string) error {
	logs.Infof("ZvecWrapper: Collection 通过 init_collection 自动加载，LoadFromFile 为兼容接口")
	return nil
}

// SetSimilarityMetric 设置相似度计算方法 (Zvec 使用 HNSW+Cosine，此方法为兼容)
func (zw *ZvecWrapper) SetSimilarityMetric(metric string) {
	logs.Infof("ZvecWrapper: 相似度计算由 HNSW 索引决定，SetSimilarityMetric(%s) 为兼容接口", metric)
}

// EnableCache 启用向量缓存
func (zw *ZvecWrapper) EnableCache() {
	zw.cacheEnabled = true
}

// DisableCache 禁用向量缓存
func (zw *ZvecWrapper) DisableCache() {
	zw.cacheEnabled = false
}

// ClearCache 清除向量缓存
func (zw *ZvecWrapper) ClearCache() {
	zw.vectorCache = make(map[string][]float32)
}

// GetCacheStats 获取缓存统计
func (zw *ZvecWrapper) GetCacheStats() map[string]interface{} {
	return map[string]interface{}{
		"enabled":    zw.cacheEnabled,
		"cache_size": len(zw.vectorCache),
		"dirty":      zw.dirtyFlag,
		"engine":     "zvec",
	}
}

// Free 释放资源，关闭 Python 子进程
func (zw *ZvecWrapper) Free() {
	if zw.ready {
		// 尝试优雅关闭
		zw.sendRequest("shutdown", map[string]interface{}{})
		zw.ready = false
	}

	if zw.stdin != nil {
		zw.stdin.Close()
	}

	if zw.cmd != nil && zw.cmd.Process != nil {
		// 等待进程退出，最多2秒
		done := make(chan error, 1)
		go func() {
			done <- zw.cmd.Wait()
		}()

		select {
		case <-done:
			logs.Infof("Zvec Bridge 进程已正常退出")
		case <-time.After(2 * time.Second):
			logs.Warnf("Zvec Bridge 进程超时未退出，强制终止")
			zw.cmd.Process.Kill()
		}
	}
}
