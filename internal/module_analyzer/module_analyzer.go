package module_analyzer

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/kinglegendzzh/flashmemory/cmd/common"
	"github.com/kinglegendzzh/flashmemory/config"
	"github.com/kinglegendzzh/flashmemory/internal/analyzer"
	"github.com/kinglegendzzh/flashmemory/internal/cloud"
	"github.com/kinglegendzzh/flashmemory/internal/embedding"
	"github.com/kinglegendzzh/flashmemory/internal/index"
	"github.com/kinglegendzzh/flashmemory/internal/utils"
	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"
)

// TaskStatus 表示分析任务的状态
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"   // 等待执行
	TaskStatusRunning   TaskStatus = "running"   // 正在执行
	TaskStatusCompleted TaskStatus = "completed" // 已完成
	TaskStatusFailed    TaskStatus = "failed"    // 执行失败
)

// AnalysisTask 表示一个模块分析任务
type AnalysisTask struct {
	ID            string     `json:"id"`                       // 任务ID
	ProjectDir    string     `json:"project_dir"`              // 项目目录
	Status        TaskStatus `json:"status"`                   // 任务状态
	Total         int        `json:"total"`                    // 总模块数
	Completed     int        `json:"completed"`                // 已完成模块数
	Remaining     int        `json:"remaining"`                // 剩余模块数
	ErrorMessage  string     `json:"error_message,omitempty"`  // 错误信息
	StartTime     time.Time  `json:"start_time"`               // 开始时间
	CompletedTime time.Time  `json:"completed_time,omitempty"` // 完成时间
}

// TaskQueue 全局任务队列
var (
	tasksMutex sync.RWMutex
	tasks      = make(map[string]*AnalysisTask) // 任务ID到任务的映射
)

// RegisterTask 注册一个新任务并返回任务ID
func RegisterTask(projectDir string) string {
	taskID := generateTaskID()
	task := &AnalysisTask{
		ID:         taskID,
		ProjectDir: projectDir,
		Status:     TaskStatusPending,
		StartTime:  time.Now(),
	}

	tasksMutex.Lock()
	// 清除相关projectDir的任务
	var count int
	for k, v := range tasks {
		if v.ProjectDir == projectDir {
			delete(tasks, k)
			count++
		}
	}
	logs.Infof("已清理项目 %s 的 %d 个任务，注册新任务: %s", projectDir, count, taskID)
	tasks[taskID] = task
	tasksMutex.Unlock()

	return taskID
}

// GetTask 获取指定ID的任务
func GetTask(taskID string) (*AnalysisTask, bool) {
	tasksMutex.RLock()
	defer tasksMutex.RUnlock()

	task, exists := tasks[taskID]
	return task, exists
}

// GetTaskByProjDir 获取指定项目目录的任务（返回最新的任务）
func GetTaskByProjDir(projectDir string) (*AnalysisTask, bool) {
	tasksMutex.RLock()
	defer tasksMutex.RUnlock()
	var latestTask *AnalysisTask
	for _, task := range tasks {
		if task.ProjectDir == projectDir {
			if latestTask == nil || task.StartTime.After(latestTask.StartTime) {
				latestTask = task
			}
		}
	}
	return latestTask, latestTask != nil
}

func GetRunningTaskByProjDir(projectDir string) (*AnalysisTask, bool) {
	tasksMutex.RLock()
	defer tasksMutex.RUnlock()
	for _, task := range tasks {
		if task.ProjectDir == projectDir && task.Status == TaskStatusRunning {
			return task, true
		}
	}
	return nil, false
}

func GetPendingTaskByProjDir(projectDir string) (*AnalysisTask, bool) {
	tasksMutex.RLock()
	defer tasksMutex.RUnlock()
	for _, task := range tasks {
		if task.ProjectDir == projectDir && task.Status == TaskStatusPending {
			return task, true
		}
	}
	return nil, false
}

func GetFailedTaskByProjDir(projectDir string) (*AnalysisTask, bool) {
	tasksMutex.RLock()
	defer tasksMutex.RUnlock()
	for _, task := range tasks {
		if task.ProjectDir == projectDir && task.Status == TaskStatusFailed {
			return task, true
		}
	}
	return nil, false
}

func GetCompletedTaskByProjDir(projectDir string) (*AnalysisTask, bool) {
	tasksMutex.RLock()
	defer tasksMutex.RUnlock()
	for _, task := range tasks {
		if task.ProjectDir == projectDir && task.Status == TaskStatusCompleted {
			return task, true
		}
	}
	return nil, false
}

func ListTaskByProjDir(projectDir string) ([]*AnalysisTask, bool) {
	tasksMutex.RLock()
	defer tasksMutex.RUnlock()

	taskList := make([]*AnalysisTask, 0, len(tasks))
	for _, task := range tasks {
		if task.ProjectDir == projectDir {
			taskList = append(taskList, task)
		}
	}
	return taskList, true
}

// ListTasks 获取所有分析任务
func ListTasks() []*AnalysisTask {
	tasksMutex.RLock()
	defer tasksMutex.RUnlock()

	taskList := make([]*AnalysisTask, 0, len(tasks))
	for _, task := range tasks {
		taskList = append(taskList, task)
	}

	// 按创建时间排序，最新的在最后
	sort.Slice(taskList, func(i, j int) bool {
		return taskList[i].StartTime.Before(taskList[j].StartTime)
	})

	return taskList
}

// ForceClearTask 强制清理所有任务
func ForceClearTask() {
	tasksMutex.Lock()
	defer tasksMutex.Unlock()

	tasks = make(map[string]*AnalysisTask)
}

// UpdateTaskStatus 更新任务状态
func UpdateTaskStatus(taskID string, status TaskStatus, err error) bool {
	tasksMutex.Lock()
	defer tasksMutex.Unlock()

	task, exists := tasks[taskID]
	if !exists {
		return false
	}

	task.Status = status
	if status == TaskStatusCompleted || status == TaskStatusFailed {
		task.CompletedTime = time.Now()
	}

	if err != nil {
		task.ErrorMessage = err.Error()
	}

	return true
}

// UpdateTaskProgress 更新任务进度
func UpdateTaskProgress(taskID string, total, completed int) bool {
	tasksMutex.Lock()
	defer tasksMutex.Unlock()

	task, exists := tasks[taskID]
	if !exists {
		return false
	}

	task.Total = total
	task.Completed = completed
	task.Remaining = total - completed
	logs.Infof("任务 %s 进度更新，总模块数 %d，已完成 %d，剩余 %d", taskID, total, completed, task.Remaining)

	return true
}

// generateTaskID 生成唯一任务ID
func generateTaskID() string {
	now := time.Now().UnixNano()
	rnd := rand.Intn(1000000)
	return fmt.Sprintf("%d-%d", now, rnd)
}

// ModuleInfo 存储文件或目录的信息
type ModuleInfo struct {
	Name          string                       `json:"name"`
	Type          string                       `json:"type"`
	Path          string                       `json:"path"`
	ParentPath    string                       `json:"parent_path"`
	FunctionCount int                          `json:"function_count"`
	FileCount     int                          `json:"file_count"` // 文件数量（对于目录）
	Description   string                       `json:"description,omitempty"`
	HasDesc       bool                         `json:"has_desc,omitempty"`  // 标记是否有 LLM 生成的描述
	HasIndex      bool                         `json:"has_index,omitempty"` // 标记是否有索引
	UpdatedAt     time.Time                    `json:"updated_at"`
	CreatedAt     time.Time                    `json:"created_at"`
	Functions     []analyzer.LLMAnalysisResult // 包含的函数列表（仅用于生成描述，不存储）
	SubModules    []*ModuleInfo                // 子模块（仅用于生成描述，不存储）
	Depth         int                          `json:"depth,omitempty"` // 目录深度（子模块嵌套层次）
}

// ModuleAnalyzer 用于分析文件和目录级别的代码模块
type ModuleAnalyzer struct {
	db             *sql.DB
	projDir        string
	config         *config.Config
	maxConcurrency int
	debug          bool
	outputDir      string // 存放输出JSON文件的目录
	skipLLM        bool   // 是否跳过LLM描述生成

	// 用于批量处理的字段
	batchSize    int           // 批量插入的大小
	batchMutex   sync.Mutex    // 批量操作的互斥锁
	batchModules []*ModuleInfo // 待批量插入的模块缓存

	// 用于缓存已分析模块的描述，确保在批量处理过程中能够一致访问
	cacheMutex sync.RWMutex      // 缓存操作的互斥锁
	descCache  map[string]string // 路径到描述的映射

	// 任务追踪
	taskID      string // 当前分析任务的ID
	subAnalysis bool   // 是否局部分析
	subPath     string // 局部分析的路径
}

// NewModuleAnalyzer 创建一个新的模块分析器
func NewModuleAnalyzer(db *sql.DB, projDir string, config *config.Config, maxConcurrency int, debug bool, taskID string, skipLLM bool, subPath string) *ModuleAnalyzer {
	// 创建输出目录
	outputDir := filepath.Join(projDir, ".gitgo", "module_graphs")
	os.MkdirAll(outputDir, 0755)

	return &ModuleAnalyzer{
		db:             db,
		projDir:        projDir,
		config:         config,
		maxConcurrency: maxConcurrency,
		debug:          debug,
		outputDir:      outputDir,
		skipLLM:        skipLLM,
		batchSize:      3, // 默认批量处理大小为3
		batchModules:   make([]*ModuleInfo, 0),
		descCache:      make(map[string]string),
		taskID:         taskID,
		subAnalysis:    subPath != "",
		subPath:        subPath,
	}
}

// AnalyzeModules 基于函数分析结果，生成文件和目录级别的描述
func (ma *ModuleAnalyzer) AnalyzeModules(results []analyzer.LLMAnalysisResult) error {

	// 更新任务状态为运行中
	if ma.taskID != "" && !ma.skipLLM {
		UpdateTaskStatus(ma.taskID, TaskStatusRunning, nil)
	}

	logs.Infof("开始分析模块，共有 %d 个函数结果", len(results))
	if ma.subAnalysis {
		logs.Infof("局部分析，只分析 %s 目录", ma.subPath)
	}

	// 1. 按文件路径组织函数
	fileModules := ma.organizeByFile(results)
	logs.Infof("组织了 %d 个文件模块", len(fileModules))

	// 2. 构建目录树结构
	rootModule := ma.buildDirectoryTree(fileModules)
	logs.Infof("构建了目录树结构")

	// 预计总模块数（用于进度更新）
	totalModules := ma.countTotalModules(rootModule)
	if !ma.skipLLM {
		// 查询code_desc表中的记录数
		var count int
		if err := ma.db.QueryRow("SELECT COUNT(*) FROM code_desc").Scan(&count); err != nil {
			logs.Errorf("查询code_desc表中的记录数失败: %v", err)
		}
		UpdateTaskProgress(ma.taskID, totalModules, count)
	}
	// 3. 自底向上生成描述（现在这里会在分析每个模块时批量保存到数据库）
	// 使用并发控制来处理模块描述生成
	err := ma.generateDescriptionsWithConcurrency(rootModule)
	if err != nil {
		// 更新任务状态为失败
		if ma.taskID != "" && !ma.skipLLM {
			UpdateTaskStatus(ma.taskID, TaskStatusFailed, err)
		}
		return fmt.Errorf("生成模块描述失败: %w", err)
	}

	// 3.5 预处理：标记有 LLM 描述的模块
	err = ma.markModulesWithDescription(rootModule)
	if err != nil {
		logs.Warnf("标记 LLM 描述模块失败: %v", err)
		// 这里即使失败也继续执行，不影响主流程
	}

	// 4. 将结果生成模块的图谱结构树，并保存至文件
	err = ma.saveToFile(rootModule)
	if err != nil {
		// 更新任务状态为失败
		if ma.taskID != "" && !ma.skipLLM {
			UpdateTaskStatus(ma.taskID, TaskStatusFailed, err)
		}
		return fmt.Errorf("保存模块描述到文件失败: %w", err)
	}

	if !ma.skipLLM {
		logs.Infof("正在生成模块描述向量...")
		gitgoDir := filepath.Join(ma.projDir, ".gitgo")

		// 索引文件路径
		indexDBPath := filepath.Join(gitgoDir, "code_index.db")
		faissModulePath := filepath.Join(gitgoDir, "module.faiss")
		// 检查.gitgo目录和索引文件是否存在
		if _, err := os.Stat(gitgoDir); os.IsNotExist(err) {
			return fmt.Errorf("索引文件不存在")
		}
		if _, err := os.Stat(indexDBPath); os.IsNotExist(err) {
			return fmt.Errorf("索引文件不存在")
		}

		// 打开数据库
		db, err := index.EnsureIndexDB(ma.projDir)
		if err != nil {
			return fmt.Errorf("索引文件不存在")
		}
		defer db.Close()

		// 确保storage_path是绝对路径
		absGitgoDir, err := filepath.Abs(gitgoDir)
		if err != nil {
			return fmt.Errorf("索引文件不存在")
		}

		// 创建FaissWrapper，传入存储路径选项
		faissOptions := map[string]interface{}{
			"storage_path": absGitgoDir,
			"server_url":   index.DefaultFaissServerURL,
			"index_id":     fmt.Sprintf("%s_module", ma.projDir),
		}
		idx := &index.Indexer{DB: db, FaissIndex: index.NewFaissWrapper(128, faissOptions)}

		if _, err := os.Stat(faissModulePath); os.IsNotExist(err) {
			logs.Infof("正在初始化Faiss索引...")
			err = embedding.EnsureCodeDescEmbeddingsBatch(idx)
			if common.IsLLMError(err) {
				return err
			}
			if err != nil {
				return fmt.Errorf("索引文件重置失败")
			}
			err = idx.FaissIndex.SaveToFile(absGitgoDir + "/module.faiss")
			if err != nil {
				return fmt.Errorf("索引文件保存失败")
			}
		}
	}

	logs.Infof("模块分析完成，共 %d 个文件模块，%d 个目录模块，%d 个函数模块", len(fileModules), len(rootModule.SubModules), len(rootModule.Functions))
	logs.Infof("附加信息：批量处理大小为 %d， 最大并发数为 %d，调试模式为 %t，输出目录为 %s，路径映射缓存区大小为 %d", ma.batchSize, ma.maxConcurrency, ma.debug, ma.outputDir, len(ma.descCache))

	// 更新任务状态为完成
	if ma.taskID != "" && !ma.skipLLM {
		UpdateTaskProgress(ma.taskID, totalModules, totalModules)
		UpdateTaskStatus(ma.taskID, TaskStatusCompleted, nil)
	}

	return nil
}

// saveToFile 将结果生成模块的图谱结构树，并保存至文件
// calculateModuleDepth 递归计算目录的实际深度
func calculateModuleDepth(module *ModuleInfo) int {
	// 如果不是目录或没有子模块，深度为0
	if module.Type != "directory" || len(module.SubModules) == 0 {
		return 0
	}

	// 递归计算所有子模块的最大深度
	maxSubDepth := 0
	for _, subModule := range module.SubModules {
		subDepth := calculateModuleDepth(subModule)
		if subDepth > maxSubDepth {
			maxSubDepth = subDepth
		}
	}

	// 当前目录的深度是子模块最大深度+1
	return maxSubDepth + 1
}

// countTotalModules 统计模块总数
func (ma *ModuleAnalyzer) countTotalModules(module *ModuleInfo) int {
	count := 1 // 当前模块
	for _, subModule := range module.SubModules {
		count += ma.countTotalModules(subModule)
	}
	return count
}

func (ma *ModuleAnalyzer) saveToFile(module *ModuleInfo) error {
	// 生成不同的可视化格式
	err := ma.saveHierarchicalJson(module)
	if err != nil {
		return fmt.Errorf("保存层次图谱JSON失败: %w", err)
	}

	err = ma.saveNetworkJson(module)
	if err != nil {
		return fmt.Errorf("保存网络图谱JSON失败: %w", err)
	}

	err = ma.saveSunburstJson(module)
	if err != nil {
		return fmt.Errorf("保存旭日图JSON失败: %w", err)
	}

	err = ma.saveFlatJson(module)
	if err != nil {
		return fmt.Errorf("保存扁平JSON失败: %w", err)
	}

	logs.Infof("已生成所有图谱JSON文件")
	return nil
}

// HierarchicalNode 层次结构树节点，适用于树形图可视化
type HierarchicalNode struct {
	Name        string              `json:"name"`
	Path        string              `json:"path"`
	Type        string              `json:"type"`
	Description string              `json:"description,omitempty"`
	HasDesc     bool                `json:"hasDesc,omitempty"`
	HasIndex    bool                `json:"hasIndex,omitempty"`
	Size        int                 `json:"size"` // 文件大小或函数计数
	Children    []*HierarchicalNode `json:"children,omitempty"`
}

// NetworkNode 网络图节点，适用于力导向图可视化
type NetworkNode struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	HasDesc     bool   `json:"hasDesc,omitempty"`
	HasIndex    bool   `json:"hasIndex,omitempty"`
	Group       int    `json:"group"` // 根据模块类型或深度分组
	Size        int    `json:"size"`  // 文件大小或函数计数
}

// NetworkLink 网络图连接，表示节点间的关系
type NetworkLink struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Value  int    `json:"value"` // 连接强度
}

// NetworkGraph 完整的网络图结构
type NetworkGraph struct {
	Nodes []NetworkNode `json:"nodes"`
	Links []NetworkLink `json:"links"`
}

// FlatNode 扁平化的节点列表，方便搜索和过滤
type FlatNode struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Path        string   `json:"path"`
	Type        string   `json:"type"`
	Parent      string   `json:"parent,omitempty"`
	Description string   `json:"description,omitempty"`
	HasDesc     bool     `json:"hasDesc,omitempty"`
	HasIndex    bool     `json:"hasIndex,omitempty"`
	Size        int      `json:"size"`
	Depth       int      `json:"depth"`
	Children    []string `json:"children,omitempty"`
}

// SunburstNode 旭日图节点，用于层次化展示并支持钻取
type SunburstNode struct {
	Name     string          `json:"name"`
	Path     string          `json:"path"`
	Type     string          `json:"type"`
	HasDesc  bool            `json:"hasDesc,omitempty"`
	HasIndex bool            `json:"hasIndex,omitempty"`
	Size     int             `json:"size,omitempty"`  // 叶子节点的大小
	Value    int             `json:"value,omitempty"` // 值（可选）
	Children []*SunburstNode `json:"children,omitempty"`
}

// saveHierarchicalJson 保存层次结构JSON，适合树形图可视化
func (ma *ModuleAnalyzer) saveHierarchicalJson(module *ModuleInfo) error {
	// 构建层次结构树
	root := ma.buildHierarchicalTree(module)

	// 生成JSON数据
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("层次结构树JSON序列化失败: %w", err)
	}

	// 保存到文件
	filePath := filepath.Join(ma.outputDir, "hierarchical_tree.json")
	err = os.WriteFile(filePath, data, 0644)
	if err != nil {
		return fmt.Errorf("写入层次结构树JSON文件失败: %w", err)
	}

	logs.Infof("已保存层次结构树JSON文件: %s", filePath)
	return nil
}

// buildHierarchicalTree 从ModuleInfo构建层次结构树
func (ma *ModuleAnalyzer) buildHierarchicalTree(module *ModuleInfo) *HierarchicalNode {
	node := &HierarchicalNode{
		Name:        module.Name,
		Path:        module.Path,
		Type:        module.Type,
		Description: module.Description,
		HasDesc:     module.HasDesc,
		HasIndex:    module.HasIndex,
	}

	if module.Type == "file" {
		// 文件节点使用函数数量作为大小
		node.Size = module.FunctionCount
	} else {
		// 目录节点使用文件数量作为大小
		node.Size = module.FileCount

		// 递归处理子模块
		for _, subModule := range module.SubModules {
			childNode := ma.buildHierarchicalTree(subModule)
			node.Children = append(node.Children, childNode)
		}
	}

	return node
}

// saveNetworkJson 保存网络图JSON，适合力导向图可视化
func (ma *ModuleAnalyzer) saveNetworkJson(module *ModuleInfo) error {
	// 创建网络图结构
	graph := NetworkGraph{
		Nodes: []NetworkNode{},
		Links: []NetworkLink{},
	}

	// 填充节点和连接
	ma.buildNetworkGraph(module, &graph, 0)

	// 生成JSON数据
	data, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		return fmt.Errorf("网络图JSON序列化失败: %w", err)
	}

	// 保存到文件
	filePath := filepath.Join(ma.outputDir, "network_graph.json")
	err = os.WriteFile(filePath, data, 0644)
	if err != nil {
		return fmt.Errorf("写入网络图JSON文件失败: %w", err)
	}

	logs.Infof("已保存网络图JSON文件: %s", filePath)
	return nil
}

// buildNetworkGraph 从ModuleInfo构建网络图
func (ma *ModuleAnalyzer) buildNetworkGraph(module *ModuleInfo, graph *NetworkGraph, depth int) {
	var id string
	if module.Path == "" && module.ParentPath == "" {
		// 项目根目录
		id = ma.projDir
	} else {
		// 模块路径,正常处理
		id = module.Path
	}
	// 添加当前节点
	node := NetworkNode{
		ID:          id,
		Name:        module.Name,
		Type:        module.Type,
		Description: module.Description,
		HasDesc:     module.HasDesc,
		HasIndex:    module.HasIndex,
		Group:       depth, // 根据深度分组
	}

	if module.Type == "file" {
		node.Size = module.FunctionCount
	} else {
		node.Size = module.FileCount
	}

	graph.Nodes = append(graph.Nodes, node)

	// 处理子模块并添加连接
	for _, subModule := range module.SubModules {
		// 递归处理子模块
		ma.buildNetworkGraph(subModule, graph, depth+1)

		// 添加从当前模块到子模块的连接
		link := NetworkLink{
			Source: id,
			Target: subModule.Path,
			Value:  1, // 基本连接强度
		}

		// 如果是文件到目录的连接，可以增加权重
		if module.Type == "directory" && subModule.Type == "file" {
			link.Value = 2
		}

		graph.Links = append(graph.Links, link)
	}
}

// saveSunburstJson 保存旭日图JSON，适合层次化圆形可视化
func (ma *ModuleAnalyzer) saveSunburstJson(module *ModuleInfo) error {
	// 构建旭日图数据结构
	root := ma.buildSunburstTree(module)

	// 生成JSON数据
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("旭日图JSON序列化失败: %w", err)
	}

	// 保存到文件
	filePath := filepath.Join(ma.outputDir, "sunburst_chart.json")
	err = os.WriteFile(filePath, data, 0644)
	if err != nil {
		return fmt.Errorf("写入旭日图JSON文件失败: %w", err)
	}

	logs.Infof("已保存旭日图JSON文件: %s", filePath)
	return nil
}

// buildSunburstTree 从ModuleInfo构建旭日图树
func (ma *ModuleAnalyzer) buildSunburstTree(module *ModuleInfo) *SunburstNode {
	node := &SunburstNode{
		Name:     module.Name,
		Path:     module.Path,
		Type:     module.Type,
		HasDesc:  module.HasDesc,
		HasIndex: module.HasIndex,
	}

	if module.Type == "file" {
		// 文件节点使用函数数量作为大小
		node.Size = module.FunctionCount
		node.Value = module.FunctionCount
	} else {
		// 目录节点不设置大小，仅添加子节点
		node.Value = module.FileCount

		// 递归处理子模块
		for _, subModule := range module.SubModules {
			childNode := ma.buildSunburstTree(subModule)
			node.Children = append(node.Children, childNode)
		}
	}

	return node
}

// saveFlatJson 保存扁平化的节点列表，方便搜索和过滤
func (ma *ModuleAnalyzer) saveFlatJson(module *ModuleInfo) error {
	// 创建扁平化节点列表
	nodes := make([]FlatNode, 0)

	// 填充节点列表
	ma.buildFlatList(module, "", 0, &nodes)

	// 生成JSON数据
	data, err := json.MarshalIndent(nodes, "", "  ")
	if err != nil {
		return fmt.Errorf("扁平节点列表JSON序列化失败: %w", err)
	}

	// 保存到文件
	filePath := filepath.Join(ma.outputDir, "flat_nodes.json")
	err = os.WriteFile(filePath, data, 0644)
	if err != nil {
		return fmt.Errorf("写入扁平节点列表JSON文件失败: %w", err)
	}

	logs.Infof("已保存扁平节点列表JSON文件: %s", filePath)
	return nil
}

// buildFlatList 从ModuleInfo构建扁平节点列表
func (ma *ModuleAnalyzer) buildFlatList(module *ModuleInfo, parentID string, depth int, nodes *[]FlatNode) {
	// 创建当前节点
	node := FlatNode{
		ID:          module.Path,
		Name:        module.Name,
		Path:        module.Path,
		Type:        module.Type,
		Parent:      parentID,
		Description: module.Description,
		HasDesc:     module.HasDesc,
		HasIndex:    module.HasIndex,
		Depth:       depth,
		Children:    make([]string, 0),
	}

	if module.Type == "file" {
		node.Size = module.FunctionCount
	} else {
		node.Size = module.FileCount
	}

	// 收集子模块ID作为children引用
	for _, subModule := range module.SubModules {
		node.Children = append(node.Children, subModule.Path)
	}

	// 添加到结果列表
	*nodes = append(*nodes, node)

	// 递归处理子模块
	for _, subModule := range module.SubModules {
		ma.buildFlatList(subModule, module.Path, depth+1, nodes)
	}
}

// organizeByFile 将函数按文件路径组织
func (ma *ModuleAnalyzer) organizeByFile(results []analyzer.LLMAnalysisResult) map[string]*ModuleInfo {
	fileModules := make(map[string]*ModuleInfo)

	for _, result := range results {
		// 标准化文件路径（使用相对路径）
		filePath := result.Func.File
		if !filepath.IsAbs(filePath) {
			filePath = filepath.Join(ma.projDir, filePath)
		}
		relPath, err := filepath.Rel(ma.projDir, filePath)
		if err != nil {
			logs.Errorf("获取相对路径失败 %s: %v", filePath, err)
			continue
		}
		relPath = filepath.ToSlash(relPath)

		// 获取文件名和父目录
		fileName := filepath.Base(relPath)
		parentPath := filepath.Dir(relPath)
		if parentPath == "." {
			parentPath = ""
		}

		// 如果文件模块不存在，创建一个新的
		if _, exists := fileModules[relPath]; !exists {
			fileModules[relPath] = &ModuleInfo{
				Name:       fileName,
				Type:       "file",
				Path:       relPath,
				ParentPath: parentPath,
				UpdatedAt:  time.Now(),
				CreatedAt:  time.Now(),
			}
		}

		// 添加函数到文件模块
		fileModules[relPath].Functions = append(fileModules[relPath].Functions, result)
		fileModules[relPath].FunctionCount++
	}

	return fileModules
}

// buildDirectoryTree 构建目录树结构 - 先构建全量目录树，再合并fileModules
func (ma *ModuleAnalyzer) buildDirectoryTree(fileModules map[string]*ModuleInfo) *ModuleInfo {
	// 创建根模块
	rootModule := &ModuleInfo{
		Name:       filepath.Base(ma.projDir),
		Type:       "directory",
		Path:       "", // 根路径用空字符串表示
		ParentPath: "",
		UpdatedAt:  time.Now(),
		CreatedAt:  time.Now(),
	}

	// 创建目录模块映射
	dirModules := make(map[string]*ModuleInfo)
	dirModules[""] = rootModule

	// 文件模块映射，包括现有的和新发现的文件
	allFileModules := make(map[string]*ModuleInfo)
	// 复制现有的文件模块
	for path, module := range fileModules {
		allFileModules[path] = module
	}

	// 1. 全量遍历项目目录，构建完整的目录结构和文件列表
	err := filepath.Walk(ma.projDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 跳过根目录本身
		if path == ma.projDir {
			return nil
		}

		// 获取相对于项目根目录的路径
		relPath, err := filepath.Rel(ma.projDir, path)
		if err != nil {
			logs.Errorf("获取相对路径失败: %v", err)
			return nil
		}

		// 统一使用斜杠，与文件模块保持一致
		relPath = filepath.ToSlash(relPath)

		// 处理隐藏文件和目录
		if strings.HasPrefix(filepath.Base(relPath), ".") && relPath != "." {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		// 跳过exclude.json中指定的路径
		fullWalkPath := filepath.Join(ma.projDir, path)
		excludeFile := filepath.Join(ma.projDir, ".gitgo", "exclude.json")
		jsonFile, _ := utils.ReadJSONArrayFile(excludeFile)
		if utils.IsExcludedPath(fullWalkPath, jsonFile) {
			logs.Warnf("跳过指定目录: %s", fullWalkPath)
			return filepath.SkipDir
		}

		// 获取父目录路径
		parentPath := filepath.Dir(relPath)
		if parentPath == "." {
			parentPath = "" // 根目录的子目录
		} else {
			parentPath = filepath.ToSlash(parentPath)
		}

		if info.IsDir() {
			// 处理目录
			if _, exists := dirModules[relPath]; !exists {
				// 创建目录模块
				dirModules[relPath] = &ModuleInfo{
					Name:       filepath.Base(relPath),
					Type:       "directory",
					Path:       relPath,
					ParentPath: parentPath,
					UpdatedAt:  time.Now(),
					CreatedAt:  time.Now(),
				}

				// 确保父目录也存在
				currentParent := parentPath
				for currentParent != "" && currentParent != "." {
					if _, exists := dirModules[currentParent]; !exists {
						grandParent := filepath.Dir(currentParent)
						if grandParent == "." {
							grandParent = ""
						} else {
							grandParent = filepath.ToSlash(grandParent)
						}

						dirModules[currentParent] = &ModuleInfo{
							Name:       filepath.Base(currentParent),
							Type:       "directory",
							Path:       currentParent,
							ParentPath: grandParent,
							UpdatedAt:  time.Now(),
							CreatedAt:  time.Now(),
						}
					}

					// 继续向上找父目录
					currentParent = filepath.Dir(currentParent)
					if currentParent == "." {
						currentParent = ""
					} else {
						currentParent = filepath.ToSlash(currentParent)
					}
				}
			}
		} else {
			// 处理文件 - 如果不在现有的fileModules中，则创建新的文件模块
			if _, exists := allFileModules[relPath]; !exists {
				// 跳过一些不需要分析的文件类型
				ext := strings.ToLower(filepath.Ext(relPath))
				skipExts := []string{".jpg", ".jpeg", ".png", ".gif", ".svg", ".ico", ".pdf", ".zip", ".tar", ".gz", ".rar", ".exe", ".bin"}
				for _, skipExt := range skipExts {
					if ext == skipExt {
						return nil
					}
				}

				// 创建新的文件模块
				allFileModules[relPath] = &ModuleInfo{
					Name:       filepath.Base(relPath),
					Type:       "file",
					Path:       relPath,
					ParentPath: parentPath,
					UpdatedAt:  time.Now(),
					CreatedAt:  time.Now(),
					// 注意：这里没有解析函数信息，函数数量默认为0
				}
			}
		}

		return nil
	})

	if err != nil {
		logs.Errorf("遍历项目目录失败: %v", err)
		// 即使遍历失败，也继续使用已获取的目录结构
	}

	// 2. 将目录添加到其父目录，构建树状结构
	for path, module := range dirModules {
		if path == "" {
			continue // 跳过根目录
		}
		parentModule := dirModules[module.ParentPath]
		if parentModule != nil {
			parentModule.SubModules = append(parentModule.SubModules, module)
		}
	}

	// 3. 将allFileModules中的所有文件模块合并到目录树中
	// 这包括原有的fileModules和新发现的文件
	for _, fileModule := range allFileModules {
		parentPath := fileModule.ParentPath
		parentModule := dirModules[parentPath]
		if parentModule != nil {
			// 添加文件到父目录
			parentModule.SubModules = append(parentModule.SubModules, fileModule)

			// 更新父目录及其所有祖先目录的统计数据
			currentParent := parentPath
			for {
				parentMod := dirModules[currentParent]
				if parentMod != nil {
					parentMod.FileCount++
					parentMod.FunctionCount += fileModule.FunctionCount
				}

				// 如果已经到达根目录，则结束
				if currentParent == "" {
					break
				}

				// 继续向上更新
				currentParent = dirModules[currentParent].ParentPath
			}
		} else {
			// 如果父目录不存在于目录树中，可能是因为这个目录被过滤掉了
			// 此时可以添加到根目录下
			rootModule.SubModules = append(rootModule.SubModules, fileModule)
			rootModule.FileCount++
			rootModule.FunctionCount += fileModule.FunctionCount
			logs.Warnf("文件 %s 的父目录 %s 不存在于目录树中，已添加到根目录", fileModule.Path, fileModule.ParentPath)
		}
	}

	// 可选：记录一些调试信息
	logs.Infof("目录树构建完成，共包含 %d 个目录和 %d 个文件", len(dirModules), len(allFileModules))

	return rootModule
}

// generateDescriptionsWithConcurrency 使用并发方式自底向上遍历模块树，生成描述并批量插入数据库
func (ma *ModuleAnalyzer) generateDescriptionsWithConcurrency(root *ModuleInfo) error {
	var globalErr error

	// 自底向上遍历模块树，使用深度优先遍历
	logs.Infof("开始自底向上并发生成模块描述，最大并发数: %d", ma.maxConcurrency)

	// 构建每个节点的归并前顺序（后序遍历）
	var nodeList []*ModuleInfo

	// 1. 首先收集所有的节点，并按深度排序
	var collectNodes func(m *ModuleInfo, depth int)
	collectNodes = func(m *ModuleInfo, depth int) {
		// 递归处理所有子节点
		for _, sub := range m.SubModules {
			collectNodes(sub, depth+1)
		}

		// 在后序遍历中添加节点
		m.FileCount = len(m.Functions) // 对文件节点，记录函数数量
		nodeList = append(nodeList, m)
	}

	// 从根开始收集所有节点
	collectNodes(root, 0)

	// 2. 并发处理节点，使用信号量控制并发数
	logs.Infof("共收集了 %d 个节点进行处理", len(nodeList))

	// 创建一个错误通道，用于收集并发处理中的错误
	errChan := make(chan error, len(nodeList))

	// 创建一个等待组，用于等待所有goroutine完成
	var wg sync.WaitGroup

	// 创建一个信号量通道，用于控制并发数
	sem := make(chan struct{}, ma.maxConcurrency)

	// 并发处理每个节点
	for _, m := range nodeList {
		// 如果有全局错误，终止处理
		if globalErr != nil {
			break
		}

		// 获取信号量（如果通道已满，将阻塞直到有空位）
		sem <- struct{}{}

		// 增加等待组计数
		wg.Add(1)

		// 启动goroutine处理节点
		go func(module *ModuleInfo, rootModule *ModuleInfo) {
			defer wg.Done()
			defer func() { <-sem }() // 释放信号量

			// 处理单个模块，传递根模块以便批量插入时使用
			if err := ma.processModule(module, rootModule); err != nil {
				logs.Errorf("处理模块 %s 失败: %v", module.Path, err)
				errChan <- err
			}
		}(m, root)
	}

	// 等待所有goroutine完成
	wg.Wait()

	// 关闭错误通道
	close(errChan)

	// 检查是否有错误
	for err := range errChan {
		if globalErr == nil {
			globalErr = err
		}
	}

	// 插入剩余未满 batchSize 的模块
	ma.batchMutex.Lock()
	remaining := len(ma.batchModules)
	ma.batchMutex.Unlock()
	if remaining > 0 {
		logs.Infof("处理剩余 %d 个模块的最终批量插入", remaining)
		if err := ma.batchInsertModules(root); err != nil {
			logs.Errorf("最终批量插入失败: %v", err)
			if globalErr == nil {
				globalErr = err
			}
		}
	}

	return globalErr
}

// processModule 处理单个模块，生成描述并加入批量插入队列
func (ma *ModuleAnalyzer) processModule(m *ModuleInfo, rootModule *ModuleInfo) error {
	// 检查是否需要更新
	needUpdate, err := ma.checkForChanges(m)
	if err != nil {
		logs.Errorf("检查模块 %s 变更失败: %v", m.Path, err)
		return err
	}

	if !needUpdate {
		logs.Infof("模块 %s 无变化，检查描述", m.Path)

		// 即使模块无变化，也要确保其描述存在于内存缓存中
		var existingDesc string

		// 1. 先检查模块自身是否有描述
		if m.Description != "" {
			existingDesc = m.Description
		} else {
			// 2. 检查缓存中是否有描述
			ma.cacheMutex.RLock()
			cachedDesc, exists := ma.descCache[m.Path]
			ma.cacheMutex.RUnlock()

			if exists && cachedDesc != "" {
				logs.Infof("从缓存中获取模块 %s 的描述", m.Path)
				existingDesc = cachedDesc
			} else if ma.db != nil {
				// 3. 从数据库查询描述
				var desc string
				err := ma.db.QueryRow("SELECT description FROM code_desc WHERE path = ? LIMIT 1", m.Path).Scan(&desc)
				if err == nil && desc != "" {
					logs.Infof("从数据库查询到模块 %s 的描述", m.Path)
					existingDesc = desc
				}
			}
		}

		// 如果找到描述，更新到模块和缓存
		if existingDesc != "" {
			// 更新模块描述
			m.Description = existingDesc

			// 更新全局缓存
			ma.cacheMutex.Lock()
			ma.descCache[m.Path] = existingDesc
			ma.cacheMutex.Unlock()

			logs.Infof("为无变化模块 %s 设置了描述", m.Path)
		} else {
			logs.Warnf("无法为模块 %s 找到描述", m.Path)
		}

		return nil
	}
	if !ma.skipLLM {
		// 判断是否存在模块索引临时文件，若不存在，则判断为退出信号，抛出异常
		if _, err := os.Stat(filepath.Join(ma.projDir, ".gitgo", "module_analyzer.temp")); os.IsNotExist(err) {
			logs.Infof("模块索引临时文件不存在，判断为退出信号，抛出异常")
			return fmt.Errorf("模块索引分析中断")
		}
	}

	// 生成描述
	var desc string
	if m.Type == "file" {
		if ma.subAnalysis && !strings.Contains(m.Path, ma.subPath) {
			logs.Warnf("非局部路径，跳过生成描述: %s", m.Path)
			return nil
		}
		desc, err = ma.generateFileDescription(m)
		if common.IsLLMError(err) {
			logs.Warnf("生成文件 %s 描述失败，属于LLM错误，抛出异常: %v", m.Path, err)
			return err
		}
		if err != nil {
			logs.Warnf("生成文件 %s 描述失败: %v", m.Path, err)
			return nil // 继续处理其他模块
		}
	} else if m.Type == "directory" && len(m.SubModules) > 0 {
		if ma.subAnalysis && !strings.Contains(m.Path, ma.subPath) {
			logs.Warnf("非局部路径，跳过生成描述: %s", m.Path)
			return nil
		}
		// 目录节点处理 - 此时已经确保所有子模块都已处理完毕
		desc, err = ma.generateDirectoryDescription(m)
		if common.IsLLMError(err) {
			logs.Warnf("生成目录 %s 描述失败，属于LLM错误，抛出异常: %v", m.Path, err)
			return err
		}
		if err != nil {
			logs.Warnf("生成目录 %s 描述失败: %v", m.Path, err)
			return nil // 继续处理其他模块
		}
	}

	if desc != "" {
		// 设置模块描述
		m.Description = desc

		// 将描述添加到全局缓存
		ma.cacheMutex.Lock()
		ma.descCache[m.Path] = desc
		ma.cacheMutex.Unlock()

		// 加入批量缓存
		ma.batchMutex.Lock()
		ma.batchModules = append(ma.batchModules, m)
		currentBatch := len(ma.batchModules)
		ma.batchMutex.Unlock()

		// 达到阈值时批量插入
		if currentBatch >= ma.batchSize {
			if err := ma.batchInsertModules(rootModule); err != nil {
				logs.Errorf("批量插入失败: %v", err)
				return err
			}
		}
	}

	return nil
}

// generateDescriptions 自底向上遍历模块树，生成描述并批量插入数据库
func (ma *ModuleAnalyzer) generateDescriptions(root *ModuleInfo) error {
	var globalErr error

	// 自底向上遍历模块树，使用深度优先遍历
	logs.Infof("开始自底向上生成模块描述")

	// 构建每个节点的归并前顺序（后序遍历）
	var nodeList []*ModuleInfo

	// 1. 首先收集所有的节点，并按深度排序
	var collectNodes func(m *ModuleInfo, depth int)
	collectNodes = func(m *ModuleInfo, depth int) {
		// 递归处理所有子节点
		for _, sub := range m.SubModules {
			collectNodes(sub, depth+1)
		}

		// 在后序遍历中添加节点
		m.FileCount = len(m.Functions) // 对文件节点，记录函数数量
		nodeList = append(nodeList, m)
	}

	// 从根开始收集所有节点
	collectNodes(root, 0)

	// 2. 按照收集顺序（自底向上）处理每个节点
	logs.Infof("共收集了 %d 个节点进行处理", len(nodeList))
	for _, m := range nodeList {
		// 如果有全局错误，终止处理
		if globalErr != nil {
			break
		}

		// 检查是否需要更新
		needUpdate, err := ma.checkForChanges(m)
		if err != nil {
			logs.Errorf("检查模块 %s 变更失败: %v", m.Path, err)
			globalErr = err
			continue
		}

		if !needUpdate {
			logs.Infof("模块 %s 无变化，检查描述", m.Path)

			// 即使模块无变化，也要确保其描述存在于内存缓存中
			var existingDesc string

			// 1. 先检查模块自身是否有描述
			if m.Description != "" {
				existingDesc = m.Description
			} else {
				// 2. 检查缓存中是否有描述
				ma.cacheMutex.RLock()
				cachedDesc, exists := ma.descCache[m.Path]
				ma.cacheMutex.RUnlock()

				if exists && cachedDesc != "" {
					logs.Infof("从缓存中获取模块 %s 的描述", m.Path)
					existingDesc = cachedDesc
				} else if ma.db != nil {
					// 3. 从数据库查询描述
					var desc string
					err := ma.db.QueryRow("SELECT description FROM code_desc WHERE path = ? LIMIT 1", m.Path).Scan(&desc)
					if err == nil && desc != "" {
						logs.Infof("从数据库查询到模块 %s 的描述", m.Path)
						existingDesc = desc
					}
				}
			}

			// 如果找到描述，更新到模块和缓存
			if existingDesc != "" {
				// 更新模块描述
				m.Description = existingDesc

				// 更新全局缓存
				ma.cacheMutex.Lock()
				ma.descCache[m.Path] = existingDesc
				ma.cacheMutex.Unlock()

				logs.Infof("为无变化模块 %s 设置了描述", m.Path)
			} else {
				logs.Warnf("无法为模块 %s 找到描述", m.Path)
			}

			continue
		}
		if !ma.skipLLM {
			// 判断是否存在模块索引临时文件，若不存在，则判断为退出信号，抛出异常
			if _, err := os.Stat(filepath.Join(ma.projDir, ".gitgo", "module_analyzer.temp")); os.IsNotExist(err) {
				logs.Infof("模块索引临时文件不存在，判断为退出信号，抛出异常")
				return fmt.Errorf("模块索引分析中断")
			}
		}

		// 生成描述
		var desc string
		if m.Type == "file" {
			desc, err = ma.generateFileDescription(m)
			if err != nil {
				logs.Warnf("生成文件 %s 描述失败: %v", m.Path, err)
				continue
			}
		} else if m.Type == "directory" && len(m.SubModules) > 0 {
			// 目录节点处理 - 此时已经确保所有子模块都已处理完毕
			desc, err = ma.generateDirectoryDescription(m)
			if common.IsLLMError(err) {
				logs.Warnf("生成目录 %s 描述失败，属于LLM错误，抛出异常: %v", m.Path, err)
				return err
			}
			if err != nil {
				logs.Warnf("生成目录 %s 描述失败: %v", m.Path, err)
				continue
			}
		}

		if desc != "" {
			// 设置模块描述
			m.Description = desc

			// 将描述添加到全局缓存
			ma.cacheMutex.Lock()
			ma.descCache[m.Path] = desc
			ma.cacheMutex.Unlock()

			// 加入批量缓存
			ma.batchMutex.Lock()
			ma.batchModules = append(ma.batchModules, m)
			currentBatch := len(ma.batchModules)
			ma.batchMutex.Unlock()

			// 达到阈值时批量插入
			if currentBatch >= ma.batchSize {
				if err := ma.batchInsertModules(root); err != nil {
					logs.Errorf("批量插入失败: %v", err)
					globalErr = err
					break
				}
			}
		}
	}

	// 插入剩余未满 batchSize 的模块
	ma.batchMutex.Lock()
	remaining := len(ma.batchModules)
	ma.batchMutex.Unlock()
	if remaining > 0 {
		logs.Infof("处理剩余 %d 个模块的最终批量插入", remaining)
		if err := ma.batchInsertModules(root); err != nil {
			logs.Errorf("最终批量插入失败: %v", err)
			if globalErr == nil {
				globalErr = err
			}
		}
	}

	return globalErr
}

// generateDirectoryDescription 生成目录级别的描述
func (ma *ModuleAnalyzer) generateDirectoryDescription(module *ModuleInfo) (string, error) {
	// 如果设置了跳过LLM描述生成，直接返回空字符串
	if ma.skipLLM {
		logs.Infof("跳过LLM描述生成，目录 %s 返回空字符串", module.Path)
		return "", nil
	}
	// 构建提示词
	var sb strings.Builder
	if module.ParentPath == "" && module.Path == "" {
		logs.Infof("生成根目录描述")
		sb.WriteString(fmt.Sprintf("%s %s\n\n", ma.config.RepoAnalyzerPrompts.Header, module.Path))
		sb.WriteString(fmt.Sprintf("%s\n\n", ma.config.RepoAnalyzerPrompts.SubModuleHeader))
	} else {
		sb.WriteString(fmt.Sprintf("%s %s\n\n", ma.config.ModuleAnalyzerPrompts.Header, module.Path))
		sb.WriteString(fmt.Sprintf("%s\n\n", ma.config.ModuleAnalyzerPrompts.SubModuleHeader))
	}

	// 将子模块分为目录和文件两组
	var directories []*ModuleInfo
	var files []*ModuleInfo

	for _, subModule := range module.SubModules {
		if subModule.Type == "directory" {
			// 计算并设置目录深度
			subModule.Depth = calculateModuleDepth(subModule)
			directories = append(directories, subModule)
		} else {
			files = append(files, subModule)
		}
	}

	logs.Infof("目录 %s 的子模块数量: %d, 文件数量: %d, 总数: %d", module.Path, len(directories), len(files), len(module.SubModules))

	// 对目录按实际深度排序，深度越深的目录越靠前
	sort.Slice(directories, func(i, j int) bool {
		// 如果深度相同，则按文件数量排序
		if directories[i].Depth == directories[j].Depth {
			return directories[i].FileCount > directories[j].FileCount
		}

		// 深度越深的目录越靠前
		return directories[i].Depth > directories[j].Depth
	})

	// 文件仍然按文件数量排序
	sort.Slice(files, func(i, j int) bool {
		return files[i].FileCount > files[j].FileCount
	})

	// 合并排序后的结果，先放目录，后放文件
	sortedModules := append(directories, files...)

	if len(sortedModules) > 50 {
		sortedModules = sortedModules[:50]
	}

	for i, subModule := range sortedModules {
		// 优先从内存缓存获取子模块描述，其次从实例中获取，最后尝试从数据库查询
		var description string

		// 1. 从内存缓存查询
		ma.cacheMutex.RLock()
		cachedDesc, exists := ma.descCache[subModule.Path]
		ma.cacheMutex.RUnlock()

		if exists && cachedDesc != "" {
			logs.Infof("从内存缓存获取到子模块 %s 的描述", subModule.Path)
			description = cachedDesc
			// 更新子模块的描述，保持一致性
			subModule.Description = cachedDesc
		} else {
			// 2. 从实例获取
			description = subModule.Description

			// 3. 如果仍然为空，从数据库查询
			if description == "" && ma.db != nil {
				var desc string
				err := ma.db.QueryRow("SELECT description FROM code_desc WHERE path = ? LIMIT 1", subModule.Path).Scan(&desc)
				if err == nil && desc != "" {
					logs.Infof("从数据库查询到子模块 %s 的描述", subModule.Path)
					description = desc
					// 更新子模块的描述
					subModule.Description = desc

					// 同时更新内存缓存
					ma.cacheMutex.Lock()
					ma.descCache[subModule.Path] = desc
					ma.cacheMutex.Unlock()
				}
			}
		}
		if len(sortedModules) > 15 {
			logs.Warnf("目录 %s 的子模块数量超过15个，进行智能文本语义切割优化", module.Path)
			//对description进行智能文本语义切割优化，只取第一句话(根据换行符进行切割，对于切分后的文本；如果字符大于300，根据标点符号"。"进行切割；如果字符仍然大于300，取前300个字符)
			if i > 15 && len(description) > 300 && strings.Contains(description, "。") {
				description = strings.Split(description, "。")[0]
				logs.Warnf("子模块 %s 的描述包含标点符号，已截断 %d", subModule.Path, len(description))
			} else if i > 10 && len(description) > 300 && strings.Contains(description, "\n") {
				description = strings.Split(description, "\n")[0]
				logs.Warnf("子模块 %s 的描述包含换行符，已截断 %d", subModule.Path, len(description))
			} else if i > 3 && len(description) > 300 {
				description = description[:300]
				logs.Warnf("子模块 %s 的描述超过300个字符，已截断 %d", subModule.Path, len(description))
			}
		}

		sb.WriteString(fmt.Sprintf("- %s: %s\n", subModule.Path, description))
	}

	if module.ParentPath == "" && module.Path == "" {
		sb.WriteString(fmt.Sprintf("%s\n\n", ma.config.RepoAnalyzerPrompts.Footer))
	} else {
		sb.WriteString(fmt.Sprintf("%s\n\n", ma.config.ModuleAnalyzerPrompts.Footer))
	}

	prompt := sb.String()

	logs.Infof("目录 %s 的提示词: %s", module.Path, prompt)
	// 调用大模型生成描述
	description, err := cloud.FastFunction(ma.config, prompt)
	if err != nil {
		logs.Errorf("为目录 %s 生成描述失败: %v", module.Path, err)
		return "", err
	}
	logs.Tokenf("\n目录 %s 的描述: %s\n", module.Path, description)

	return description, nil
}

// isReadableTextFile 检测文件是否为可读的文本文件
// 通过读取文件前100个字节，检测是否为有效的UTF-8编码或其他常见文本编码
func isReadableTextFile(filePath string) bool {
	// 打开文件
	file, err := os.Open(filePath)
	if err != nil {
		logs.Warnf("无法打开文件进行编码检测: %s, %v", filePath, err)
		return false
	}
	defer file.Close()

	// 读取前100个字节
	buf := make([]byte, 100)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		logs.Warnf("读取文件失败: %s, %v", filePath, err)
		return false
	}

	// 如果文件小于100字节，调整缓冲区大小
	buf = buf[:n]

	// 检查是否为有效的UTF-8编码
	if !utf8.Valid(buf) {
		// 检查是否包含二进制数据
		binaryCount := 0
		for _, b := range buf {
			// 检测非打印字符和控制字符
			if b < 32 && b != '\n' && b != '\r' && b != '\t' {
				binaryCount++
			}
		}

		// 如果二进制字符超过10%，则认为是二进制文件
		if float64(binaryCount)/float64(len(buf)) > 0.1 {
			logs.Warnf("文件可能是二进制格式，跳过: %s", filePath)
			return false
		}
	}

	return true
}

// generateFileDescription 生成文件级别的描述
func (ma *ModuleAnalyzer) generateFileDescription(module *ModuleInfo) (string, error) {
	// 如果设置了跳过LLM描述生成，直接返回空字符串
	if ma.skipLLM {
		logs.Infof("跳过LLM描述生成，文件 %s 返回空字符串", module.Path)
		return "", nil
	}

	// 构建提示词
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s %s\n\n", ma.config.FileAnalyzerPrompts.Header, module.Path))

	// 如果module.Functions为空，直接读取本地代码文件拼接提示词
	if len(module.Functions) == 0 {
		// 将ProjectDir与module.Path拼接成完整路径
		fullPath := filepath.Join(ma.projDir, module.Path)

		// 仅考虑特定的文件扩展名
		ext := filepath.Ext(fullPath)
		if utils.Contains(common.BlackList, ext) {
			logs.Warnf("跳过不支持的文件类型: %s %s", fullPath, ext)
			return "", nil
		}

		// 检测文件编码，判断是否为可读的文本文件
		if !isReadableTextFile(fullPath) {
			logs.Warnf("跳过不支持的文件编码或二进制文件: %s", fullPath)
			return "", nil
		}

		var MaxCharLimit = 15000
		logs.Infof("文件 %s 的函数列表为空，直接读取本地代码文件拼接提示词，完整路径: %s，最大字符限制: %d", module.Path, fullPath, MaxCharLimit)
		code, err := utils.ExtractCodeSnippetWithLimit(fullPath, MaxCharLimit)
		if err != nil {
			logs.Errorf("为文件 %s 提取代码失败: %v", module.Path, err)
			return "", err
		}
		sb.WriteString(code)
	} else {
		sb.WriteString(fmt.Sprintf("%s\n\n", ma.config.FileAnalyzerPrompts.SubModuleHeader))
		// 对module.Functions基于ImportantScore进行重排序
		sort.Slice(module.Functions, func(i, j int) bool {
			return module.Functions[i].Func.Score > module.Functions[j].Func.Score
		})

		// 只取20个函数
		if len(module.Functions) > 20 {
			module.Functions = module.Functions[:20]
		}
		// 对Func.Name去重，仅保留第一个出现的
		seen := make(map[string]struct{})
		for i := 0; i < len(module.Functions); i++ {
			name := module.Functions[i].Func.Name
			if _, exists := seen[name]; exists {
				continue
			}
			seen[name] = struct{}{}
			desc := module.Functions[i].Description
			if desc == "" {
				err := ma.db.QueryRow("SELECT description FROM functions WHERE file = ? LIMIT 1", module.Functions[i].Func.File).Scan(&desc)
				if err == nil && desc != "" {
					logs.Infof("从数据库查询到函数 %s 的描述", module.Functions[i].Func.Name)
					module.Functions[i].Description = desc
				} else {
					logs.Warnf("从数据库查询不到函数 %s 的描述", module.Functions[i].Func.Name)
				}
			}
			sb.WriteString(fmt.Sprintf("- %s: %s\n", module.Functions[i].Func.Name, desc))
		}
	}

	sb.WriteString(fmt.Sprintf("%s\n\n", ma.config.FileAnalyzerPrompts.Footer))

	prompt := sb.String()

	logs.Infof("文件 %s 的提示词: %s", module.Path, prompt)
	// 调用大模型生成描述
	description, err := cloud.FastFunction(ma.config, prompt)
	if err != nil {
		logs.Errorf("为文件 %s 生成描述失败: %v", module.Path, err)
		return "", err
	}
	logs.Tokenf("\n文件 %s 的描述: %s\n", module.Path, description)

	return description, nil
}

// checkForChanges 检查模块是否需要更新（文件或函数数量是否变化）
func (ma *ModuleAnalyzer) checkForChanges(module *ModuleInfo) (bool, error) {
	if ma.db == nil {
		logs.Warnf("数据库连接为空，跳过检查")
		return false, nil
	}
	// 查询数据库中的记录
	var functionCount, fileCount int
	var path string
	err := ma.db.QueryRow(
		"SELECT path, function_count, file_count FROM code_desc WHERE path = ?",
		module.Path,
	).Scan(&path, &functionCount, &fileCount)

	// 如果记录不存在，需要更新
	if err == sql.ErrNoRows {
		logs.Infof("模块 %s 在数据库中不存在，需要新增", module.Path)
		return true, nil
	}

	// 查询出错
	if err != nil && err != sql.ErrNoRows {
		return false, fmt.Errorf("查询模块 %s 失败: %w", module.Path, err)
	}

	// 检查文件数或函数数是否变化
	if module.FunctionCount != functionCount || module.FileCount != fileCount {
		logs.Infof("模块 %s 有变化，原函数数:%d 当前:%d，原文件数:%d 当前:%d",
			module.Path, functionCount, module.FunctionCount, fileCount, module.FileCount)
		return true, nil
	}

	logs.Infof("模块 %s 无变化，跳过处理", module.Path)
	return false, nil
}

// batchInsertModules 批量插入模块到数据库
func (ma *ModuleAnalyzer) batchInsertModules(root *ModuleInfo) error {
	ma.batchMutex.Lock()

	// 检查skipLLM
	if ma.skipLLM {
		ma.batchMutex.Unlock()
		return nil
	}

	// 检查是否有模块需要插入
	if len(ma.batchModules) == 0 {
		ma.batchMutex.Unlock()
		return nil
	}

	// 复制要插入的模块，然后释放锁
	modulesToInsert := make([]*ModuleInfo, len(ma.batchModules))
	copy(modulesToInsert, ma.batchModules)
	ma.batchModules = make([]*ModuleInfo, 0) // 立即清空批处理缓存
	ma.batchMutex.Unlock()                   // 提前解锁，减少锁的持有时间

	logs.Infof("开始批量保存")

	// 开始事务
	tx, err := ma.db.Begin()
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}

	// 改进defer以确保始终发生提交或回滚
	committed := false
	defer func() {
		if !committed {
			logs.Warnf("事务回滚")
			tx.Rollback()
		}
	}()

	// 准备插入语句
	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO code_desc (
			name, type, path, parent_path, function_count, file_count, description, updated_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("准备SQL语句失败: %w", err)
	}
	defer stmt.Close()

	// 插入所有模块
	now := time.Now()
	insertedCount := 0
	for _, m := range modulesToInsert {
		// 跳过没有描述的模块
		if m.Description == "" {
			logs.Warnf("跳过没有描述的模块: %s", m.Path)
			continue
		}

		// 插入当前模块
		_, err := stmt.Exec(
			m.Name,
			m.Type,
			m.Path,
			m.ParentPath,
			m.FunctionCount,
			m.FileCount,
			m.Description,
			now,
			m.CreatedAt,
		)
		if err != nil {
			return fmt.Errorf("插入模块 %s 失败: %w", m.Path, err)
		}
		insertedCount++
	}

	// 提交事务
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	// 标记事务已提交
	committed = true

	logs.Infof("批量插入成功完成，共插入 %d 个模块", insertedCount)

	if ma.taskID != "" && !ma.skipLLM {
		totalModules := ma.countTotalModules(root)
		newCompletedSql := "select count(*) from code_desc"
		newCompleted := 0
		ma.db.QueryRow(newCompletedSql).Scan(&newCompleted)
		UpdateTaskProgress(ma.taskID, totalModules, newCompleted)
	}

	return nil
}

// saveToDatabase 将模块描述保存到数据库
func (ma *ModuleAnalyzer) saveToDatabase(module *ModuleInfo) error {
	logs.Infof("开始批量保存")

	// 开始事务
	tx, err := ma.db.Begin()
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}

	// 改进defer以确保始终发生提交或回滚
	committed := false
	defer func() {
		if !committed {
			logs.Warnf("事务回滚")
			tx.Rollback()
		}
	}()

	// 准备插入语句
	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO code_desc (
			name, type, path, parent_path, function_count, file_count, description, updated_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("准备SQL语句失败: %w", err)
	}
	defer stmt.Close()

	// 递归保存模块及其子模块
	var saveModule func(m *ModuleInfo) error
	saveModule = func(m *ModuleInfo) error {
		// 当没有跳过LLM时，跳过没有描述的非目录模块
		// 当跳过LLM时，即使描述为空也保存模块结构
		if !ma.skipLLM && m.Description == "" && m.Type != "directory" {
			return nil
		}

		// 插入当前模块
		now := time.Now()
		_, err := stmt.Exec(
			m.Name,
			m.Type,
			m.Path,
			m.ParentPath,
			m.FunctionCount,
			m.FileCount,
			m.Description,
			now,
			m.CreatedAt,
		)
		if err != nil {
			return fmt.Errorf("插入模块 %s 失败: %w", m.Path, err)
		}

		// 递归处理子模块
		for _, subModule := range m.SubModules {
			if err := saveModule(subModule); err != nil {
				return err
			}
		}

		return nil
	}

	// 保存根模块及其所有子模块
	if err := saveModule(module); err != nil {
		return err
	}

	// 提交事务
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	// 标记事务已提交
	committed = true

	return nil
}

// markModulesWithDescription 通过查询数据库标记有 LLM 描述的模块
func (ma *ModuleAnalyzer) markModulesWithDescription(root *ModuleInfo) error {
	// 如果数据库连接为空，不执行操作
	if ma.db == nil {
		logs.Warnf("数据库连接为空，无法标记有描述的模块")
		return nil
	}

	// 收集所有模块路径
	var allModules []*ModuleInfo
	var collectModules func(m *ModuleInfo)
	collectModules = func(m *ModuleInfo) {
		allModules = append(allModules, m)
		for _, sub := range m.SubModules {
			collectModules(sub)
		}
	}
	collectModules(root)

	logs.Infof("开始标记有 LLM 描述的模块，共 %d 个模块", len(allModules))

	// 创建批处理切片
	batchSize := 100
	for i := 0; i < len(allModules); i += batchSize {
		end := i + batchSize
		if end > len(allModules) {
			end = len(allModules)
		}

		batch := allModules[i:end]

		// 构建查询文件和目录模块的 SQL 语句
		if len(batch) > 0 {
			// 标记文件和目录
			var pathParams []string
			var args []interface{}

			for idx, m := range batch {
				pathParams = append(pathParams, fmt.Sprintf("$%d", idx+1))
				args = append(args, m.Path)
			}

			// 查询 code_desc 表
			query := fmt.Sprintf(
				"SELECT path FROM code_desc WHERE path IN (%s) AND description IS NOT NULL AND description != ''",
				strings.Join(pathParams, ","),
			)

			rows, err := ma.db.Query(query, args...)
			if err != nil {
				logs.Warnf("查询 code_desc 表失败: %v", err)
				continue // 继续下一批
			}

			// 收集有描述的路径
			descPaths := make(map[string]bool)
			for rows.Next() {
				var path string
				if err := rows.Scan(&path); err != nil {
					logs.Warnf("扫描 code_desc 行失败: %v", err)
					continue
				}
				descPaths[path] = true
			}
			rows.Close()

			// 标记有描述的模块
			for _, m := range batch {
				if descPaths[m.Path] {
					m.HasDesc = true
				}
			}

			// 对于文件模块，还需要查询 functions 表确认是否有函数描述
			var filePaths []string
			var fileArgs []interface{}
			var fileModules []*ModuleInfo

			for _, m := range batch {
				if m.Type == "file" {
					filePaths = append(filePaths, m.Path)
					fileArgs = append(fileArgs, m.Path)
					fileModules = append(fileModules, m)
				}
			}

			if len(filePaths) > 0 {
				// 构建新的参数列表
				filePathParams := make([]string, len(filePaths))
				for i := range filePaths {
					filePathParams[i] = fmt.Sprintf("$%d", i+1)
				}

				// 查询 functions 表
				funcQuery := fmt.Sprintf(
					"SELECT DISTINCT file FROM functions WHERE file IN (%s) AND description IS NOT NULL AND description != ''",
					strings.Join(filePathParams, ","),
				)

				funcRows, err := ma.db.Query(funcQuery, fileArgs...)
				if err != nil {
					logs.Warnf("查询 functions 表失败: %v", err)
					continue // 继续下一批
				}

				// 收集有函数描述的文件路径
				funcDescPaths := make(map[string]bool)
				for funcRows.Next() {
					var path string
					if err := funcRows.Scan(&path); err != nil {
						logs.Warnf("扫描 functions 行失败: %v", err)
						continue
					}
					funcDescPaths[path] = true
				}
				funcRows.Close()

				// 标记有函数描述的文件模块
				for _, m := range fileModules {
					if funcDescPaths[m.Path] {
						m.HasIndex = true
					}
				}
			}
		}
	}

	logs.Infof("模块标记完成")
	return nil
}
