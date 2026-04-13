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

// startBridge launches the Python zvec_bridge subprocess using a multi-strategy approach:
//
//	Strategy 1: python3 -m flashmemory.zvec_bridge (pip-installed package)
//	Strategy 2: local zvec_bridge.py script (dev mode)
//	Strategy 3: auto-provision a venv with flashmemory[embedding], then retry Strategy 1
func (zw *ZvecWrapper) startBridge() error {
	// Strategy 1: Try module mode (pip installed globally or in active venv)
	logs.Infof("[bridge] Strategy 1: trying module mode (python3 -m flashmemory.zvec_bridge)")
	if err := zw.tryStartBridgeModule(zw.PythonPath); err == nil {
		return nil
	}

	// Strategy 2: Try local script path (dev/source mode)
	if script := zw.findBridgeScript(); script != "" {
		logs.Infof("[bridge] Strategy 2: trying local script: %s", script)
		if err := zw.tryStartBridgeScript(zw.PythonPath, script); err == nil {
			return nil
		}
	}

	// Strategy 3: Check managed venv, auto-provision if needed
	venvPython := zw.getManagedVenvPython()
	if venvPython != "" {
		// Managed venv exists, try using it
		logs.Infof("[bridge] Strategy 3a: trying managed venv: %s", venvPython)
		if err := zw.tryStartBridgeModule(venvPython); err == nil {
			return nil
		}
		logs.Warnf("[bridge] Managed venv exists but bridge failed, will re-provision")
	}

	// Auto-provision: create venv and install flashmemory[embedding]
	logs.Infof("[bridge] Strategy 3b: auto-provisioning Python environment...")
	provisionedPython, err := zw.autoProvisionPythonEnv()
	if err != nil {
		return fmt.Errorf("all bridge strategies failed, auto-provision also failed: %w", err)
	}

	// Retry module mode with provisioned venv
	logs.Infof("[bridge] Retrying module mode with provisioned venv: %s", provisionedPython)
	if err := zw.tryStartBridgeModule(provisionedPython); err != nil {
		return fmt.Errorf("bridge failed even after auto-provision: %w", err)
	}

	return nil
}

// tryStartBridgeModule tries to start the bridge via `python -m flashmemory.zvec_bridge`
func (zw *ZvecWrapper) tryStartBridgeModule(pythonPath string) error {
	cmd := exec.Command(pythonPath, "-u", "-m", "flashmemory.zvec_bridge")
	cmd.Stderr = os.Stderr
	return zw.launchAndWaitReady(cmd)
}

// tryStartBridgeScript tries to start the bridge via a local .py script
func (zw *ZvecWrapper) tryStartBridgeScript(pythonPath string, scriptPath string) error {
	cmd := exec.Command(pythonPath, "-u", scriptPath)
	cmd.Stderr = os.Stderr
	return zw.launchAndWaitReady(cmd)
}

// launchAndWaitReady starts the subprocess command and waits for the "ready" JSON-line response
func (zw *ZvecWrapper) launchAndWaitReady(cmd *exec.Cmd) error {
	var err error
	zw.stdin, err = cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	zw.stdout = bufio.NewScanner(stdoutPipe)
	// Increase scanner buffer for large search results
	zw.stdout.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start Python process: %w", err)
	}
	zw.cmd = cmd

	// Wait for ready signal
	resp, err := zw.readResponse()
	if err != nil {
		// Process started but didn't respond correctly — kill it
		cmd.Process.Kill()
		cmd.Wait()
		zw.cmd = nil
		return fmt.Errorf("bridge did not become ready: %w", err)
	}
	if resp.Status != "success" || resp.Message != "ready" {
		cmd.Process.Kill()
		cmd.Wait()
		zw.cmd = nil
		return fmt.Errorf("bridge returned unexpected status: %s", resp.Message)
	}

	zw.ready = true
	logs.Infof("[bridge] Zvec Bridge is ready (pid=%d)", cmd.Process.Pid)
	return nil
}

// getManagedVenvDir returns the path to the managed venv directory (~/.flashmemory/pyenv)
func (zw *ZvecWrapper) getManagedVenvDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".flashmemory", "pyenv")
}

// getManagedVenvPython returns the python binary path inside the managed venv, or "" if not exists
func (zw *ZvecWrapper) getManagedVenvPython() string {
	venvDir := zw.getManagedVenvDir()
	if venvDir == "" {
		return ""
	}

	// Platform-aware python path
	var pythonBin string
	if _, err := os.Stat(filepath.Join(venvDir, "bin", "python3")); err == nil {
		pythonBin = filepath.Join(venvDir, "bin", "python3")
	} else if _, err := os.Stat(filepath.Join(venvDir, "Scripts", "python.exe")); err == nil {
		pythonBin = filepath.Join(venvDir, "Scripts", "python.exe")
	}

	return pythonBin
}

// autoProvisionPythonEnv creates a managed venv at ~/.flashmemory/pyenv and installs flashmemory[embedding]
func (zw *ZvecWrapper) autoProvisionPythonEnv() (string, error) {
	venvDir := zw.getManagedVenvDir()
	if venvDir == "" {
		return "", fmt.Errorf("cannot determine home directory for managed venv")
	}

	logs.Infof("[provision] Creating managed Python environment at: %s", venvDir)
	fmt.Fprintf(os.Stderr, "\n⚡ FlashMemory: Setting up Zvec Python environment (first-time only)...\n")

	// Step 1: Find a working python3
	pythonPath := zw.findSystemPython()
	if pythonPath == "" {
		return "", fmt.Errorf("python3 not found on system, please install Python 3.8+ first")
	}
	logs.Infof("[provision] Using system Python: %s", pythonPath)

	// Step 2: Create venv (remove old if exists)
	os.RemoveAll(venvDir)
	os.MkdirAll(filepath.Dir(venvDir), 0755)

	createCmd := exec.Command(pythonPath, "-m", "venv", venvDir)
	createCmd.Stderr = os.Stderr
	createCmd.Stdout = os.Stdout
	if err := createCmd.Run(); err != nil {
		return "", fmt.Errorf("failed to create venv: %w", err)
	}
	logs.Infof("[provision] Venv created successfully")

	// Step 3: Get venv python path
	venvPython := zw.getManagedVenvPython()
	if venvPython == "" {
		return "", fmt.Errorf("venv created but python binary not found in %s", venvDir)
	}

	// Step 4: Upgrade pip (quiet)
	upgradeCmd := exec.Command(venvPython, "-m", "pip", "install", "--upgrade", "pip", "-q")
	upgradeCmd.Stderr = os.Stderr
	upgradeCmd.Run() // best-effort, don't fail on this

	// Step 5: Install flashmemory[embedding]
	fmt.Fprintf(os.Stderr, "📦 Installing flashmemory[embedding]...\n")
	installCmd := exec.Command(venvPython, "-m", "pip", "install", "flashmemory[embedding]", "-q")
	installCmd.Stderr = os.Stderr
	installCmd.Stdout = os.Stdout
	if err := installCmd.Run(); err != nil {
		// Fallback: try without [embedding] extras
		logs.Warnf("[provision] flashmemory[embedding] install failed, trying base package")
		fallbackCmd := exec.Command(venvPython, "-m", "pip", "install", "flashmemory", "-q")
		fallbackCmd.Stderr = os.Stderr
		fallbackCmd.Stdout = os.Stdout
		if err2 := fallbackCmd.Run(); err2 != nil {
			return "", fmt.Errorf("pip install flashmemory failed: %w (original: %v)", err2, err)
		}
	}

	fmt.Fprintf(os.Stderr, "✅ Zvec Python environment ready!\n\n")
	logs.Infof("[provision] flashmemory[embedding] installed successfully in managed venv")

	return venvPython, nil
}

// findSystemPython locates a usable python3 binary on the system
func (zw *ZvecWrapper) findSystemPython() string {
	candidates := []string{"python3", "python"}

	for _, name := range candidates {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		// Verify it's Python 3.x
		out, err := exec.Command(path, "-c", "import sys; print(sys.version_info.major)").Output()
		if err != nil {
			continue
		}
		version := string(out)
		if len(version) > 0 && version[0] == '3' {
			return path
		}
	}

	return ""
}

// findBridgeScript locates the zvec_bridge.py script for dev/source mode
func (zw *ZvecWrapper) findBridgeScript() string {
	candidates := []string{}

	// Next to the executable binary
	if exePath, err := os.Executable(); err == nil {
		candidates = append(candidates,
			filepath.Join(filepath.Dir(exePath), "pip-package", "flashmemory", "zvec_bridge.py"),
		)
	}

	// Current working directory
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(cwd, "pip-package", "flashmemory", "zvec_bridge.py"),
		)
	}

	// Relative path (source tree)
	candidates = append(candidates,
		filepath.Join("pip-package", "flashmemory", "zvec_bridge.py"),
	)

	for _, p := range candidates {
		if absPath, err := filepath.Abs(p); err == nil {
			if _, err := os.Stat(absPath); err == nil {
				logs.Infof("[bridge] Found zvec_bridge.py: %s", absPath)
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
	case <-time.After(600 * time.Second):
		return nil, fmt.Errorf("读取响应超时 (600s)")
	}
}

// initCollection 初始化 Zvec Collection
func (zw *ZvecWrapper) initCollection(forceNew bool) error {
	resp, err := zw.sendRequest("init", map[string]interface{}{
		"collection_path": zw.CollectionPath,
		"dimension":       zw.Dim,
		"force_new":       forceNew,
		"collection_type": "both", // 初始化函数级和模块级
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

// AddFunctionVector 添加带有完整元数据的单个向量到全量引擎索引中
// Metadata必须包含 func_name 和 description 等字段供引擎建立 Sparse BM25 索引。
func (zw *ZvecWrapper) AddFunctionVector(funcID int, vector []float32, metadata map[string]interface{}) error {
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
		"metadata": metadata,
	})
	if err != nil {
		return fmt.Errorf("添加向量失败 (id=%d): %w", funcID, err)
	}
	if resp.Status != "success" {
		return fmt.Errorf("添加向量失败: %s", resp.Message)
	}

	return nil
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

// AddModuleVector 添加单个模块的向量到索引
func (zw *ZvecWrapper) AddModuleVector(modID int, vector []float32) error {
	zw.dirtyFlag = true

	// 截断或填充向量到正确维度
	if len(vector) != zw.Dim {
		resized := make([]float32, zw.Dim)
		copy(resized, vector)
		vector = resized
	}

	modIdStr := fmt.Sprintf("mod_%d", modID)

	resp, err := zw.sendRequest("add_module_vector", map[string]interface{}{
		"module_id":  modIdStr,
		"vector":   vector,
		"metadata": map[string]interface{}{},
	})
	if err != nil {
		return fmt.Errorf("添加模块向量失败 (id=%d): %w", modID, err)
	}
	if resp.Status != "success" {
		return fmt.Errorf("添加模块向量失败: %s", resp.Message)
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

	return zw.parseSearchResults(resp, "func_")
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

	return zw.parseSearchResults(resp, "func_")
}

// SearchModuleVectors finds the topK module vectors closest to the query vector
func (zw *ZvecWrapper) SearchModuleVectors(query []float32, topK int) []int {
	return zw.hybridSearchVectorsInternal(query, nil, topK, "", "modules", "mod_", "", false)
}

// HybridSearchVectors performs Dense + Sparse multi-vector search with RRF fusion.
// queryText is the original search text for auto BM25 sparse generation and reranker.
func (zw *ZvecWrapper) HybridSearchVectors(denseQuery []float32, sparseQuery map[string]float64, topK int, filter string, queryText string, enableReranker bool) []int {
	return zw.hybridSearchVectorsInternal(denseQuery, sparseQuery, topK, filter, "functions", "func_", queryText, enableReranker)
}

func (zw *ZvecWrapper) hybridSearchVectorsInternal(denseQuery []float32, sparseQuery map[string]float64, topK int, filter string, collectionType string, idPrefix string, queryText string, enableReranker bool) []int {
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
		"dense_query":      denseF64,
		"top_k":            topK,
		"use_rrf":          true,
		"collection_type":  collectionType,
		"enable_reranker":  enableReranker,
	}
	// Pass original query text for auto BM25 sparse generation and cross-encoder reranking
	if queryText != "" {
		params["query_text"] = queryText
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
		logs.Infof("Hybrid search completed, type=%s, collection=%s, reranker=%v", searchType, collectionType, enableReranker)
	}

	return zw.parseSearchResults(resp, idPrefix)
}

// parseSearchResults extracts IDs from a zvec search response
func (zw *ZvecWrapper) parseSearchResults(resp *zvecResponse, idPrefix string) []int {
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
		var objID int
		fmt.Sscanf(idStr, idPrefix+"%d", &objID)

		if objID > 0 {
			zw.Scores[objID] = float32(score)
			pairs = append(pairs, idScorePair{id: objID, score: float32(score)})
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
