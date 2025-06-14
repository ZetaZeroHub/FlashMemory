package ranking

import (
	"math"
	"sort"
	"sync"

	"github.com/kinglegendzzh/flashmemory/internal/parser"
)

// RankingConfig 配置参数
type RankingConfig struct {
	Alpha float64 // Fan-In权重
	Beta  float64 // Fan-Out权重
	Gamma float64 // 深度权重
	Delta float64 // 复杂度权重
}

// DefaultRankingConfig 返回默认配置
func DefaultRankingConfig() *RankingConfig {
	return &RankingConfig{
		Alpha: 0.4, // Fan-In权重较高，因为被调用次数多的函数通常更重要
		Beta:  0.2, // Fan-Out权重中等
		Gamma: 0.2, // 深度权重中等
		Delta: 0.2, // 复杂度权重中等
	}
}

// FunctionRanker 函数排序器
type FunctionRanker struct {
	config *RankingConfig
	mu     sync.RWMutex
}

// NewFunctionRanker 创建新的函数排序器
func NewFunctionRanker(config *RankingConfig) *FunctionRanker {
	if config == nil {
		config = DefaultRankingConfig()
	}
	return &FunctionRanker{
		config: config,
	}
}

// callGraph 表示函数调用图
type callGraph struct {
	functions map[string]*parser.FunctionInfo // 函数名到函数信息的映射
	callers   map[string][]string             // 被调用者到调用者的映射 (Fan-In)
	callees   map[string][]string             // 调用者到被调用者的映射 (Fan-Out)
	depthMap  map[string]int                  // 函数深度映射
}

// buildCallGraph 构建函数调用图
func (fr *FunctionRanker) buildCallGraph(functions []parser.FunctionInfo) *callGraph {
	graph := &callGraph{
		functions: make(map[string]*parser.FunctionInfo),
		callers:   make(map[string][]string),
		callees:   make(map[string][]string),
		depthMap:  make(map[string]int),
	}

	// 构建函数名映射（简化为直接使用函数名）
	for i := range functions {
		fn := &functions[i]
		funcName := fn.Name
		graph.functions[funcName] = fn
		graph.callers[funcName] = []string{}
		graph.callees[funcName] = []string{}
	}

	// 构建调用关系（简化逻辑，直接通过函数名匹配）
	for _, fn := range functions {
		callerName := fn.Name
		// 对fn.Calls进行去重
		uniqueCalls := fr.deduplicateCalls(fn.Calls)
		for _, call := range uniqueCalls {
			// 直接检查被调用的函数名是否在函数列表中
			if _, exists := graph.functions[call]; exists {
				// 添加到调用关系中
				graph.callees[callerName] = append(graph.callees[callerName], call)
				graph.callers[call] = append(graph.callers[call], callerName)
			}
		}
	}

	// 计算深度
	fr.calculateDepth(graph)

	return graph
}

// getFunctionKey 获取函数的唯一标识（简化为直接使用函数名）
func (fr *FunctionRanker) getFunctionKey(fn *parser.FunctionInfo) string {
	return fn.Name
}

// deduplicateCalls 对函数调用列表进行去重
func (fr *FunctionRanker) deduplicateCalls(calls []string) []string {
	if len(calls) == 0 {
		return calls
	}

	// 使用map进行去重
	seen := make(map[string]bool)
	unique := make([]string, 0, len(calls))

	for _, call := range calls {
		if !seen[call] {
			seen[call] = true
			unique = append(unique, call)
		}
	}

	return unique
}

// calculateDepth 计算函数在调用图中的深度
func (fr *FunctionRanker) calculateDepth(graph *callGraph) {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	// 使用DFS计算深度
	var dfs func(string) int
	dfs = func(funcName string) int {
		if visited[funcName] {
			return graph.depthMap[funcName]
		}

		if recStack[funcName] {
			// 检测到循环依赖，返回默认深度
			return 1
		}

		visited[funcName] = true
		recStack[funcName] = true

		maxDepth := 0
		for _, callee := range graph.callees[funcName] {
			depth := dfs(callee)
			if depth > maxDepth {
				maxDepth = depth
			}
		}

		graph.depthMap[funcName] = maxDepth + 1
		recStack[funcName] = false
		return graph.depthMap[funcName]
	}

	// 为所有函数计算深度
	for funcName := range graph.functions {
		if !visited[funcName] {
			dfs(funcName)
		}
	}
}

// calculateMetrics 计算函数的各项指标
func (fr *FunctionRanker) calculateMetrics(functions []parser.FunctionInfo) {
	graph := fr.buildCallGraph(functions)

	// 并发计算指标
	var wg sync.WaitGroup
	workerCount := 4 // 可配置的工作协程数
	funcChan := make(chan *parser.FunctionInfo, len(functions))

	// 启动工作协程
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for fn := range funcChan {
				fr.calculateSingleFunctionMetrics(fn, graph)
			}
		}()
	}

	// 发送任务
	for i := range functions {
		funcChan <- &functions[i]
	}
	close(funcChan)

	// 等待所有工作完成
	wg.Wait()
}

// calculateSingleFunctionMetrics 计算单个函数的指标
func (fr *FunctionRanker) calculateSingleFunctionMetrics(fn *parser.FunctionInfo, graph *callGraph) {
	funcName := fn.Name

	// Fan-In: 调用该函数的其他函数数量
	fn.FanIn = len(graph.callers[funcName])

	// Fan-Out: 该函数调用的其他函数数量
	fn.FanOut = len(graph.callees[funcName])

	// 深度: 函数在调用图中的层级深度
	fn.Depth = graph.depthMap[funcName]

	// 复杂度: 基于代码行数的简单复杂度估算
	fn.Complexity = fr.calculateComplexity(fn)

	// 计算综合得分
	fn.Score = fr.calculateScore(fn)
}

// calculateComplexity 计算函数复杂度
func (fr *FunctionRanker) calculateComplexity(fn *parser.FunctionInfo) int {
	// 基础复杂度基于代码行数
	complexity := fn.Lines

	// 根据函数类型调整复杂度
	switch fn.FunctionType {
	case "method":
		complexity = int(float64(complexity) * 1.1) // 方法稍微复杂一些
	case "constructor":
		complexity = int(float64(complexity) * 1.2) // 构造函数更复杂
	}

	// 根据调用数量调整复杂度（去重后的调用数量）
	uniqueCalls := fr.deduplicateCalls(fn.Calls)
	callComplexity := len(uniqueCalls)
	complexity += callComplexity

	return complexity
}

// calculateScore 计算函数的综合重要性得分
func (fr *FunctionRanker) calculateScore(fn *parser.FunctionInfo) float64 {
	fr.mu.RLock()
	defer fr.mu.RUnlock()

	// 标准化各项指标（避免某一项指标过大影响结果）
	normalizedFanIn := math.Log1p(float64(fn.FanIn))
	normalizedFanOut := math.Log1p(float64(fn.FanOut))
	normalizedDepth := math.Log1p(float64(fn.Depth))
	normalizedComplexity := math.Log1p(float64(fn.Complexity))

	// 添加imports数量作为权重参考（一般情况下同一文件中的函数imports相同）
	normalizedImports := math.Log1p(float64(len(fn.Imports)))
	importsWeight := 0.1 // imports权重较小，作为辅助参考

	// 计算加权得分
	score := fr.config.Alpha*normalizedFanIn +
		fr.config.Beta*normalizedFanOut +
		fr.config.Gamma*normalizedDepth +
		fr.config.Delta*normalizedComplexity +
		importsWeight*normalizedImports

	return score
}

// RankFunctions 对函数列表进行重要性排序
func (fr *FunctionRanker) RankFunctions(functions []parser.FunctionInfo) []parser.FunctionInfo {
	if len(functions) == 0 {
		return functions
	}

	// 计算所有函数的指标
	fr.calculateMetrics(functions)

	// 按得分排序（从低到高）
	sort.Slice(functions, func(i, j int) bool {
		return functions[i].Score < functions[j].Score
	})

	return functions
}

// RankFunctionsByScore 按指定顺序排序（ascending: true为升序，false为降序）
func (fr *FunctionRanker) RankFunctionsByScore(functions []parser.FunctionInfo, ascending bool) []parser.FunctionInfo {
	if len(functions) == 0 {
		return functions
	}

	// 计算所有函数的指标
	fr.calculateMetrics(functions)

	// 按得分排序
	sort.Slice(functions, func(i, j int) bool {
		if ascending {
			return functions[i].Score < functions[j].Score
		}
		return functions[i].Score > functions[j].Score
	})

	return functions
}

// UpdateConfig 更新排序配置
func (fr *FunctionRanker) UpdateConfig(config *RankingConfig) {
	fr.mu.Lock()
	defer fr.mu.Unlock()
	fr.config = config
}

// GetConfig 获取当前配置
func (fr *FunctionRanker) GetConfig() *RankingConfig {
	fr.mu.RLock()
	defer fr.mu.RUnlock()
	return &RankingConfig{
		Alpha: fr.config.Alpha,
		Beta:  fr.config.Beta,
		Gamma: fr.config.Gamma,
		Delta: fr.config.Delta,
	}
}

// CalculateImportanceScores 计算函数重要性评分，返回函数名到评分的映射
func CalculateImportanceScores(functions []parser.FunctionInfo, config *RankingConfig) map[string]float64 {
	if len(functions) == 0 {
		return make(map[string]float64)
	}

	// 创建函数排序器
	ranker := NewFunctionRanker(config)

	// 计算所有函数的指标
	ranker.calculateMetrics(functions)

	// 构建结果映射
	scores := make(map[string]float64)
	for _, fn := range functions {
		scores[fn.Name] = fn.Score
	}

	return scores
}
