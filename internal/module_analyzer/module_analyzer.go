package module_analyzer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kinglegendzzh/flashmemory/config"
	"github.com/kinglegendzzh/flashmemory/internal/analyzer"
	"github.com/kinglegendzzh/flashmemory/internal/cloud"
	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"
)

// ModuleInfo 存储文件或目录的信息
type ModuleInfo struct {
	Name          string                       // 文件名或目录名
	Type          string                       // "file" 或 "directory"
	Path          string                       // 相对路径
	ParentPath    string                       // 父目录路径
	FunctionCount int                          // 函数数量
	FileCount     int                          // 文件数量（对于目录）
	Description   string                       // 模块描述
	UpdatedAt     time.Time                    // 更新时间
	CreatedAt     time.Time                    // 创建时间
	Functions     []analyzer.LLMAnalysisResult // 包含的函数列表（仅用于生成描述，不存储）
	SubModules    []*ModuleInfo                // 子模块（仅用于生成描述，不存储）
}

// ModuleAnalyzer 用于分析文件和目录级别的代码模块
type ModuleAnalyzer struct {
	db             *sql.DB
	projDir        string
	config         *config.Config
	maxConcurrency int
	debug          bool
	outputDir      string // 存放输出JSON文件的目录

	// 用于批量处理的字段
	batchSize    int           // 批量插入的大小
	batchMutex   sync.Mutex    // 批量操作的互斥锁
	batchModules []*ModuleInfo // 待批量插入的模块缓存
}

// NewModuleAnalyzer 创建一个新的模块分析器
func NewModuleAnalyzer(db *sql.DB, projDir string, config *config.Config, maxConcurrency int, debug bool) *ModuleAnalyzer {
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
		batchSize:      20, // 默认批量处理大小为20
		batchModules:   make([]*ModuleInfo, 0),
	}
}

// AnalyzeModules 基于函数分析结果，生成文件和目录级别的描述
func (ma *ModuleAnalyzer) AnalyzeModules(results []analyzer.LLMAnalysisResult) error {
	if len(results) == 0 {
		logs.Warnf("没有函数分析结果，跳过模块分析")
		return fmt.Errorf("没有函数分析结果")
	}

	logs.Infof("开始分析模块，共有 %d 个函数结果", len(results))

	// 1. 按文件路径组织函数
	fileModules := ma.organizeByFile(results)
	logs.Infof("组织了 %d 个文件模块", len(fileModules))

	// 2. 构建目录树结构
	rootModule := ma.buildDirectoryTree(fileModules)
	logs.Infof("构建了目录树结构")

	// 3. 自底向上生成描述（现在这里会在分析每个模块时批量保存到数据库）
	err := ma.generateDescriptions(rootModule)
	if err != nil {
		return fmt.Errorf("生成模块描述失败: %w", err)
	}

	// 4. 将结果生成模块的图谱结构树，并保存至文件
	err = ma.saveToFile(rootModule)
	if err != nil {
		return fmt.Errorf("保存模块描述到文件失败: %w", err)
	}

	logs.Infof("模块分析完成")
	return nil
}

// saveToFile 将结果生成模块的图谱结构树，并保存至文件
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
	Size        int                 `json:"size"` // 文件大小或函数计数
	Children    []*HierarchicalNode `json:"children,omitempty"`
}

// NetworkNode 网络图节点，适用于力导向图可视化
type NetworkNode struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
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
	Size        int      `json:"size"`
	Depth       int      `json:"depth"`
	Children    []string `json:"children,omitempty"`
}

// SunburstNode 旭日图节点，用于层次化展示并支持钻取
type SunburstNode struct {
	Name     string          `json:"name"`
	Path     string          `json:"path"`
	Type     string          `json:"type"`
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
	// 添加当前节点
	node := NetworkNode{
		ID:          module.Path,
		Name:        module.Name,
		Type:        module.Type,
		Description: module.Description,
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
			Source: module.Path,
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
		Name: module.Name,
		Path: module.Path,
		Type: module.Type,
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

// buildDirectoryTree 构建目录树结构
func (ma *ModuleAnalyzer) buildDirectoryTree(fileModules map[string]*ModuleInfo) *ModuleInfo {
	// 创建根模块
	rootModule := &ModuleInfo{
		Name:       "",
		Type:       "directory",
		Path:       "",
		ParentPath: "",
		UpdatedAt:  time.Now(),
		CreatedAt:  time.Now(),
	}

	// 创建目录模块映射
	dirModules := make(map[string]*ModuleInfo)
	dirModules[""] = rootModule

	// 首先确保所有目录都被创建
	for _, fileModule := range fileModules {
		// 确保父目录路径存在
		parentPath := fileModule.ParentPath
		for parentPath != "" {
			if _, exists := dirModules[parentPath]; !exists {
				// 创建父目录模块
				dirName := filepath.Base(parentPath)
				grandParentPath := filepath.Dir(parentPath)
				if grandParentPath == "." {
					grandParentPath = ""
				}

				dirModules[parentPath] = &ModuleInfo{
					Name:       dirName,
					Type:       "directory",
					Path:       parentPath,
					ParentPath: grandParentPath,
					UpdatedAt:  time.Now(),
					CreatedAt:  time.Now(),
				}
			}

			// 移动到上一级目录
			parentPath = filepath.Dir(parentPath)
			if parentPath == "." {
				parentPath = ""
			}
		}
	}

	// 构建目录树结构
	for _, module := range fileModules {
		// 将文件添加到其父目录
		parentModule := dirModules[module.ParentPath]
		if parentModule != nil {
			parentModule.SubModules = append(parentModule.SubModules, module)
			parentModule.FileCount++
			parentModule.FunctionCount += module.FunctionCount
		}
	}

	// 将目录添加到其父目录
	for path, module := range dirModules {
		if path == "" {
			continue // 跳过根目录
		}
		parentModule := dirModules[module.ParentPath]
		if parentModule != nil {
			parentModule.SubModules = append(parentModule.SubModules, module)
		}
	}

	return rootModule
}

// generateDescriptions 自底向上生成模块描述
func (ma *ModuleAnalyzer) generateDescriptions(module *ModuleInfo) error {
	// 创建带120秒超时的上下文，防止整个操作无限期挂起
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// 使用WaitGroup和互斥锁控制并发
	var wg sync.WaitGroup
	var mu sync.Mutex
	var globalErr error

	// 使用任务队列而不是递归生成描述
	taskQueue := make(chan *ModuleInfo, 1000) // 设置足够大的缓冲区
	logs.Infof("初始化任务队列，开始生成模块描述")

	// 启动工作协程
	for i := 0; i < ma.maxConcurrency; i++ {
		go func() {
			for {
				select {
				case m, ok := <-taskQueue:
					if !ok {
						return // 通道关闭，退出协程
					}

					// 检查是否需要更新（函数或文件数量是否变化）
					needUpdate, err := ma.checkForChanges(m)
					if err != nil {
						mu.Lock()
						if globalErr == nil {
							globalErr = err
							logs.Errorf("检查模块变更失败: %v", err)
						}
						mu.Unlock()
						wg.Done()
						continue
					}

					// 如果不需要更新，直接跳过
					if !needUpdate {
						logs.Infof("模块 %s 无变化，跳过处理", m.Path)
						wg.Done()
						continue
					}

					// 处理当前模块
					if m.Type == "file" {
						// 生成文件描述
						desc, err := ma.generateFileDescription(m)
						if err != nil {
							mu.Lock()
							if globalErr == nil {
								globalErr = err
								logs.Errorf("生成文件描述失败: %v", err)
							}
							mu.Unlock()
						} else {
							m.Description = desc

							// 添加到批量插入队列
							ma.batchMutex.Lock()
							logs.Infof("添加模块到批处理队列: %s", m.Path)
							ma.batchModules = append(ma.batchModules, m)
							currentBatchSize := len(ma.batchModules)

							// 检查是否达到批量插入阈值
							if currentBatchSize >= ma.batchSize {
								logs.Infof("批处理队列达到阈值 %d，开始批量插入", currentBatchSize)
								// 确保在解锁前已经复制了需要处理的项目
								ma.batchMutex.Unlock()
								err := ma.batchInsertModules()
								if err != nil {
									logs.Errorf("批量插入模块失败: %v", err)
									mu.Lock()
									if globalErr == nil {
										globalErr = err
									}
									mu.Unlock()
								}
							} else {
								ma.batchMutex.Unlock()
							}
						}
					} else if m.Type == "directory" && len(m.SubModules) > 0 {
						// 生成目录描述
						desc, err := ma.generateDirectoryDescription(m)
						if err != nil {
							mu.Lock()
							if globalErr == nil {
								globalErr = err
								logs.Errorf("生成目录描述失败: %v", err)
							}
							mu.Unlock()
						} else {
							m.Description = desc

							// 添加到批量插入队列
							ma.batchMutex.Lock()
							logs.Infof("添加模块到批处理队列: %s", m.Path)
							ma.batchModules = append(ma.batchModules, m)
							currentBatchSize := len(ma.batchModules)

							// 检查是否达到批量插入阈值
							if currentBatchSize >= ma.batchSize {
								logs.Infof("批处理队列达到阈值 %d，开始批量插入", currentBatchSize)
								// 确保在解锁前已经复制了需要处理的项目
								ma.batchMutex.Unlock()
								err := ma.batchInsertModules()
								if err != nil {
									logs.Errorf("批量插入模块失败: %v", err)
									mu.Lock()
									if globalErr == nil {
										globalErr = err
									}
									mu.Unlock()
								}
							} else {
								ma.batchMutex.Unlock()
							}
						}
					}

					wg.Done()
				case <-ctx.Done():
					// 超时处理
					mu.Lock()
					if globalErr == nil {
						globalErr = ctx.Err()
						logs.Errorf("生成模块描述超时: %v", ctx.Err())
					}
					mu.Unlock()
					return
				}
			}
		}()
	}

	// 递归遍历模块树，将所有模块加入队列
	var enqueueModules func(m *ModuleInfo)
	enqueueModules = func(m *ModuleInfo) {
		wg.Add(1)
		taskQueue <- m

		// 先将所有子模块加入队列
		for _, subModule := range m.SubModules {
			enqueueModules(subModule)
		}
	}

	// 开始处理根模块
	enqueueModules(module)

	// 关闭队列并等待所有任务完成
	close(taskQueue)
	wg.Wait()

	// 确保所有剩余的模块都被插入
	ma.batchMutex.Lock()
	remaining := len(ma.batchModules)
	ma.batchMutex.Unlock()
	
	if remaining > 0 {
		logs.Infof("处理剩余 %d 个模块的最终批量插入", remaining)
		err := ma.batchInsertModules()
		if err != nil {
			logs.Errorf("最终批量插入模块失败: %v", err)
			if globalErr == nil {
				globalErr = err
			}
		}
	}

	// 检查是否超时
	select {
	case <-ctx.Done():
		return fmt.Errorf("生成模块描述超时: %w", ctx.Err())
	default:
		return globalErr
	}
}

// generateDirectoryDescription 生成目录级别的描述
func (ma *ModuleAnalyzer) generateDirectoryDescription(module *ModuleInfo) (string, error) {
	// 构建提示词
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s %s\n\n", ma.config.ModuleAnalyzerPrompts.Header, module.Path))
	sb.WriteString(fmt.Sprintf("%s\n\n", ma.config.ModuleAnalyzerPrompts.SubModuleHeader))

	// 只取前50个子模块
	if len(module.SubModules) > 50 {
		module.SubModules = module.SubModules[:50]
	}
	for _, subModule := range module.SubModules {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", subModule.Path, subModule.Description))
	}

	sb.WriteString(fmt.Sprintf("%s\n\n", ma.config.ModuleAnalyzerPrompts.Footer))

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

// generateFileDescription 生成文件级别的描述
func (ma *ModuleAnalyzer) generateFileDescription(module *ModuleInfo) (string, error) {
	// 构建提示词
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s %s\n\n", ma.config.FileAnalyzerPrompts.Header, module.Path))
	sb.WriteString(fmt.Sprintf("%s\n\n", ma.config.FileAnalyzerPrompts.SubModuleHeader))
	// 对module.Functions基于ImportantScore进行重排序
	sort.Slice(module.Functions, func(i, j int) bool {
		return module.Functions[i].Func.Score > module.Functions[j].Func.Score
	})

	// 只取前50个函数
	if len(module.Functions) > 50 {
		module.Functions = module.Functions[:50]
	}
	for _, fn := range module.Functions {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", fn.Func.Name, fn.Description))
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
func (ma *ModuleAnalyzer) batchInsertModules() error {
	ma.batchMutex.Lock()
	
	// 检查是否有模块需要插入
	if len(ma.batchModules) == 0 {
		ma.batchMutex.Unlock()
		return nil
	}
	
	// 复制要插入的模块，然后释放锁
	modulesToInsert := make([]*ModuleInfo, len(ma.batchModules))
	copy(modulesToInsert, ma.batchModules)
	ma.batchModules = make([]*ModuleInfo, 0) // 立即清空批处理缓存
	ma.batchMutex.Unlock() // 提前解锁，减少锁的持有时间

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
		if m.Description == "" && m.Type != "directory" {
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
		// 跳过没有描述的模块
		if m.Description == "" && m.Type != "directory" {
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
