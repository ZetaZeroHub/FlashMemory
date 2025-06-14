package analyzer

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/kinglegendzzh/flashmemory/config"
	"github.com/kinglegendzzh/flashmemory/internal/parser"
	"github.com/kinglegendzzh/flashmemory/internal/ranking"
	"github.com/kinglegendzzh/flashmemory/internal/utils"
	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"
)

// LLMAnalysisResult 保存函数的分析结果
type LLMAnalysisResult struct {
	Func            parser.FunctionInfo
	CodeSnippet     string   // 从文件中提取的代码片段
	Description     string   // 大模型生成的函数描述
	InternalDeps    []string // 内部调用列表
	ExternalDeps    []string // 外部依赖列表
	ImportanceScore float64  // 综合重要性评分
}

//todo 评估内部外部依赖分析

// LLMAnalyzer 分析函数并生成 LLMAnalysisResult
type LLMAnalyzer struct {
	// 已知函数描述缓存，用于依赖分析
	KnownDescriptions *sync.Map
	descMu            sync.RWMutex
	// 是否启用调试日志
	debug bool
	// 最大并发goroutine数量
	maxConcurrency int
	Db             *sql.DB
	projDir        string
}

// NewLLMAnalyzer 创建一个 LLMAnalyzer 实例
func NewLLMAnalyzer(initialKnown *sync.Map, debug bool, maxConcurrency int) *LLMAnalyzer {
	return &LLMAnalyzer{KnownDescriptions: initialKnown, debug: debug, maxConcurrency: maxConcurrency}
}

func NewLLMAnalyzerHttp(initialKnown *sync.Map, debug bool, maxConcurrency int, db *sql.DB, projDir string) *LLMAnalyzer {
	return &LLMAnalyzer{KnownDescriptions: initialKnown, debug: debug, maxConcurrency: maxConcurrency, Db: db, projDir: projDir}
}

// extractCodeSnippet 根据文件路径和起止行号提取代码片段
func extractCodeSnippet(path string, startLine, endLine int) (string, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	// 注意：行号从1开始计数
	if startLine < 1 || endLine > len(lines) || startLine > endLine {
		return "", fmt.Errorf("无效的行号范围: %d-%d", startLine, endLine)
	}
	snippet := strings.Join(lines[startLine-1:endLine], "\n")
	return snippet, nil
}

// AnalyzeFunction 分析单个函数：归类、提取代码片段、构建提示词并调用大模型生成描述
func (a *LLMAnalyzer) AnalyzeFunction(fn parser.FunctionInfo, startLlm bool) LLMAnalysisResult {
	if a.debug {
		log.Printf("[DEBUG] 开始分析函数: %s (文件: %s, 行数: %d)", fn.Name, fn.File, fn.Lines)
		log.Printf("[DEBUG] 函数元信息: %s", fn)
	}
	res := LLMAnalysisResult{Func: fn}
	if fn.FunctionType == "llm_parser" {
		// 函数描述已存在，直接返回
		logs.Warnf("[DEBUG] llm_parser 函数描述已存在，直接返回: %s", fn.Description)
		res.Description = fn.Description
		res.CodeSnippet = fn.CodeSnippet
		return res
	}
	// 提取代码片段
	snippet, err := extractCodeSnippet(fn.File, fn.StartLine, fn.EndLine)
	if err != nil {
		log.Printf("[ERROR] 提取文件 %s 的代码片段失败: %v", fn.File, err)
		snippet = ""
	} else if a.debug {
		log.Printf("[DEBUG] 成功提取代码片段，长度: %d 字符", len(snippet))
	}
	res.CodeSnippet = snippet

	// 区分内部和外部依赖（与之前的逻辑类似）
	internalDeps := []string{}
	externalDeps := []string{}
	for _, callee := range fn.Calls {
		if strings.Contains(callee, ".") {
			parts := strings.Split(callee, ".")
			prefix := parts[0]
			isExt := false
			for _, imp := range fn.Imports {
				impBase := imp
				if idx := strings.LastIndex(imp, "/"); idx != -1 {
					impBase = imp[idx+1:]
				}
				if impBase == prefix {
					isExt = true
					externalDeps = append(externalDeps, imp)
					break
				}
			}
			if isExt {
				continue
			}
		}
		internalDeps = append(internalDeps, callee)
	}
	res.InternalDeps = internalDeps
	res.ExternalDeps = externalDeps
	cfg, err := config.LoadConfig()
	if cfg == nil || err != nil {
		logs.Errorf("Warn: no config file found or parse error, fallback to env or default. Err: %v", err)
	}
	// 对 internalDeps 去重
	dedup := make(map[string]bool)
	uniqueDeps := []string{}
	for _, dep := range internalDeps {
		if !dedup[dep] {
			dedup[dep] = true
			uniqueDeps = append(uniqueDeps, dep)
		}
	}
	logs.Infof("内部函数去重：origin:%d=>unique:%d\n%s\n", len(internalDeps), len(uniqueDeps), strings.Join(uniqueDeps, ", "))
	res.InternalDeps = uniqueDeps
	if startLlm {
		// 构造提示词：将代码片段和依赖描述整合到一起
		prompt := fmt.Sprintf("%s \n%s\n", cfg.AnaPrompts.Role, snippet)
		// 如果snippet大于5000字符，则只保留前5000字符
		if len(prompt) > cfg.CodeLimit {
			logs.Infof("代码内容过长，截取前%d个字符", cfg.PromptLimit)
			prompt = prompt[:cfg.CodeLimit]
		}
		// 该函数的路径、类名和引入包

		// 只取最后一级目录和文件名
		filePath := fn.File
		// 统一将路径分隔符转换为系统分隔符
		filePath = filepath.FromSlash(filePath)
		// 使用filepath.Split来分割路径,支持跨平台
		dir, file := filepath.Split(filePath)
		dir = filepath.Clean(dir)
		var displayPath string
		// 获取父目录名
		parentDir := filepath.Base(dir)
		if parentDir != "." && parentDir != "/" {
			displayPath = filepath.Join(parentDir, file)
		} else {
			displayPath = file
		}
		// 转换回统一的斜杠格式用于显示
		displayPath = filepath.ToSlash(displayPath)
		if fn.Package != "" {
			prompt += fmt.Sprintf("%s \n%s(%s)\n", cfg.AnaPrompts.Route, displayPath, fn.Package)
		} else {
			prompt += fmt.Sprintf("%s \n%s\n", cfg.AnaPrompts.Route, fn.File)
		}
		if fn.Imports != nil {
			prompt += fmt.Sprintf("%s \n%s\n", cfg.AnaPrompts.Imports, strings.Join(fn.Imports, ", "))
		}
		if len(internalDeps) > 0 {
			// 附加内部依赖的描述（如果已有）
			tip := 1
			for _, dep := range uniqueDeps {
				desc := ""
				a.descMu.RLock()
				v, ok := a.KnownDescriptions.Load(dep)
				a.descMu.RUnlock()
				if ok {
					desc = v.(string)
					if tip == 1 {
						prompt += fmt.Sprintf("%s\n", cfg.AnaPrompts.InternalDeps)
					}
					var data map[string]interface{}
					err := json.Unmarshal([]byte(desc), &data)
					if err != nil {
						logs.Infof("解析 JSON 出错: %s, %v", desc, err)
						lines := strings.Split(desc, "\n")
						if len(lines) >= 2 {
							des := lines[1]
							des = strings.ReplaceAll(des, "\n", "")
							prompt += fmt.Sprintf("%d. %s(%s)\n", tip, dep, des)
						} else {
							prompt += fmt.Sprintf("%d. %s\n", tip, dep)
						}
					} else {
						if data["description"] != nil {
							data["description"] = strings.ReplaceAll(data["description"].(string), "\n", "")
							prompt += fmt.Sprintf("%d. %s(%s)\n", tip, dep, data["description"])
						}
					}
					tip++
				}
			}
		}
		// 附加外部依赖信息
		if len(externalDeps) > 0 {
			prompt += fmt.Sprintf("%s %s\n", cfg.AnaPrompts.ExternalDeps, strings.Join(externalDeps, ", "))
		}
		prompt += fmt.Sprintf(`
%s`, cfg.AnaPrompts.Main)
		//	prompt += `请基于代码内容和调用逻辑，从整体上分析该函数，并按照下列要求生成输出。输出必须为一个合法的 JSON 对象，且严格遵循以下格式和字段说明，每个字段的描述请用一句简明扼要的话说明：
		//
		//{
		//  "description": "一句话概括函数的主要目的和作用，说明它解决的问题或实现的功能。",
		//  "parameters": [
		//    {
		//      "入参名称": "数据类型、意义、以及可能的取值范围或默认值。"
		//    }
		//  ],
		//  "return":[
		//    {
		//      "返回值名称": "结果类型、意义以及在不同情况下返回的特殊值。"
		//    }
		//  ],
		//  "exception_handling": "一句话说明函数在处理异常、边界情况或错误输入时的行为，包括可能抛出的异常或返回的错误码。",
		//  "complexity": "一句话分析代码的圈复杂度、时间复杂度和空间复杂度。",
		//  "performance": "一句话描述函数的运行效率，特别是处理大规模数据时的性能表现。",
		//  "side-effect": "一句话说明函数执行过程中是否会修改外部状态、全局变量或其他对象的值。"
		//}`

		//绘制 mermaid 流程图

		if a.debug {
			log.Printf("[DEBUG] 调用大模型生成描述，内部依赖数: %d, 外部依赖数: %d", len(internalDeps), len(externalDeps))
			log.Printf("[DEBUG] 详情，内部依赖: %s, 外部依赖: %s", internalDeps, externalDeps)
		}
		result, err := utils.Completion(prompt)
		tokens := []string{"```", "json", "```json"}
		for _, v := range tokens {
			if strings.Contains(result, v) {
				logs.Tokenf("(!remove: %v!)", v)
				result = strings.Replace(result, v, "", 2)
			}
		}
		result = utils.FilterJSONContent(result)
		if err != nil {
			log.Printf("[ERROR] 调用大模型失败: %v", err)
		}
		if a.debug {
			log.Printf("[DEBUG] 大模型完成了生成描述完成，提示词长度: %d 字符，输出长度: %d 字符", len(prompt), len(result))
			logs.Infof("[DEBUG][ModelResponse] 提示词: \n%s\n", prompt)
			logs.Tokenf("[DEBUG][ModelResponse] 描述内容: \n%s\n", result)
		}
		res.Description = result
	}

	// 计算简单的重要性评分
	res.ImportanceScore = fn.Score
	logs.Infof("%s 重要性评分: %.5f", res.Func.Name, res.ImportanceScore)
	return res
}

// AnalyzeAll 对所有函数进行依赖感知的自底向上分析
func (a *LLMAnalyzer) AnalyzeAll(funcs []parser.FunctionInfo) []LLMAnalysisResult {
	if a.debug {
		log.Printf("[DEBUG] 开始批量分析 %d 个函数", len(funcs))
	}

	// 使用函数重要性排序算法对函数进行重排序
	// 配置权重：重视调用层次（从依赖最底层到最顶层）
	rankingConfig := &ranking.RankingConfig{
		Alpha: 0.4, // Fan-In权重
		Beta:  0.2, // Fan-Out权重
		Gamma: 0.5, // 深度权重（重视调用层次）
		Delta: 0.1, // 复杂度权重
	}
	logs.Infof("配置权重：重视调用层次（从依赖最底层到最顶层）")
	logs.Infof("Alpha: %.2f, Beta: %.2f, Gamma: %.2f, Delta: %.2f", rankingConfig.Alpha, rankingConfig.Beta, rankingConfig.Gamma, rankingConfig.Delta)
	ranker := ranking.NewFunctionRanker(rankingConfig)

	// 对函数进行重要性排序（升序：从低分到高分，即从依赖最底层到最顶层）
	sortedFuncs := ranker.RankFunctions(funcs)
	if a.debug {
		log.Printf("[DEBUG] 函数重要性排序完成，按从底层到顶层顺序分析")
		for i, fn := range sortedFuncs {
			log.Printf("[DEBUG] 排序 %d: %s (Score: %.3f, FanIn: %d, FanOut: %d, Depth: %d)",
				i+1, fn.Name, fn.Score, fn.FanIn, fn.FanOut, fn.Depth)
		}
	}

	resultChan := make(chan LLMAnalysisResult, len(sortedFuncs))
	done := make(chan bool)
	results := []LLMAnalysisResult{}

	classified := make(map[string][]parser.FunctionInfo)
	classifiedMutex := sync.Mutex{}
	knownDescMutex := sync.Mutex{} // 用于保护 knownDesc 的并发访问

	// 归类函数
	for _, f := range sortedFuncs {
		key := fmt.Sprintf("%s_%s_%s", f.File, f.Name, f.FunctionType)
		classifiedMutex.Lock()
		classified[key] = append(classified[key], f)
		classifiedMutex.Unlock()
	}

	remaining := sortedFuncs
	// 注释掉原有的简单排序，使用ranking算法的结果
	// sort.SliceStable(remaining, func(i, j int) bool {
	//	return len(remaining[i].Calls) < len(remaining[j].Calls)
	// })

	// 启动结果收集goroutine
	go func() {
		for res := range resultChan {
			results = append(results, res)
		}
		done <- true
	}()

	pass := 0
	sem := make(chan struct{}, a.maxConcurrency)
	for len(remaining) > 0 && pass < 10 {
		pass++
		newRemaining := []parser.FunctionInfo{}
		var wg sync.WaitGroup

		for _, f := range remaining {
			if a.Db != nil {
				if stored, found := a.loadStoredResult(f); found {
					res := a.AnalyzeFunction(f, false)
					res.Description = stored.Description
					resultChan <- res
					knownDescMutex.Lock() // 锁定 knownDesc
					a.descMu.Lock()
					a.KnownDescriptions.Store(f.Name, res.Description)
					a.descMu.Unlock()
					knownDescMutex.Unlock()
					continue
				}
			}

			allDepsKnown := true
			a.descMu.RLock()
			for _, dep := range f.Calls {
				if _, ok := a.KnownDescriptions.Load(dep); !ok {
					allDepsKnown = false
					break
				}
			}
			a.descMu.RUnlock()

			if allDepsKnown {
				wg.Add(1)
				sem <- struct{}{}
				go func(fn parser.FunctionInfo) {
					defer wg.Done()
					defer func() { <-sem }()
					res := a.AnalyzeFunction(fn, true)
					if a.Db != nil {
						if err := SaveSingleResultToDB(a.Db, res, a.projDir); err != nil {
							log.Printf("保存到数据库失败: %v", err)
						}
					}
					resultChan <- res
					knownDescMutex.Lock() // 锁定 knownDesc
					a.descMu.Lock()
					a.KnownDescriptions.Store(fn.Name, res.Description)
					a.descMu.Unlock()
					knownDescMutex.Unlock()
				}(f)
			} else {
				newRemaining = append(newRemaining, f)
			}
		}

		wg.Wait()
		if len(newRemaining) == len(remaining) {
			log.Printf("[WARN] 检测到循环依赖或缺失依赖，开始强制分析剩余 %d 个函数", len(remaining))
			for _, f := range remaining {
				if a.Db != nil {
					if stored, found := a.loadStoredResult(f); found {
						res := a.AnalyzeFunction(f, false)
						res.Description = stored.Description
						resultChan <- res
						knownDescMutex.Lock() // 锁定 knownDesc
						a.descMu.Lock()
						a.KnownDescriptions.Store(f.Name, res.Description)
						a.descMu.Unlock()
						knownDescMutex.Unlock()
						continue
					}
				}

				wg.Add(1)
				sem <- struct{}{}
				go func(fn parser.FunctionInfo) {
					defer wg.Done()
					defer func() { <-sem }()
					res := a.AnalyzeFunction(fn, true)
					if a.Db != nil {
						if err := SaveSingleResultToDB(a.Db, res, a.projDir); err != nil {
							log.Printf("保存到数据库失败: %v", err)
						}
					}
					resultChan <- res
					knownDescMutex.Lock() // 锁定 knownDesc
					a.descMu.Lock()
					a.KnownDescriptions.Store(fn.Name, res.Description)
					a.descMu.Unlock()
					knownDescMutex.Unlock()
				}(f)
			}
			wg.Wait()
			break
		}
		remaining = newRemaining
	}

	close(resultChan)
	<-done
	var llmParserNum int
	for _, result := range results {
		if result.Func.FunctionType == "llm_parser" {
			llmParserNum++
		}
	}
	logs.Infof("LLM parser num: %v", llmParserNum)
	return results
}

// AnalyzeAll 对所有函数进行依赖感知的自底向上分析
func (a *LLMAnalyzer) LoadAll(funcs []parser.FunctionInfo) []LLMAnalysisResult {
	if a.debug {
		log.Printf("[DEBUG] 开始批量分析 %d 个函数", len(funcs))
	}

	resultChan := make(chan LLMAnalysisResult, len(funcs))
	done := make(chan bool)
	results := []LLMAnalysisResult{}

	classified := make(map[string][]parser.FunctionInfo)
	classifiedMutex := sync.Mutex{}
	knownDescMutex := sync.Mutex{}

	for _, f := range funcs {
		key := fmt.Sprintf("%s_%s_%s", f.File, f.Name, f.FunctionType)
		classifiedMutex.Lock()
		classified[key] = append(classified[key], f)
		classifiedMutex.Unlock()
	}

	remaining := funcs
	sort.SliceStable(remaining, func(i, j int) bool {
		return len(remaining[i].Calls) < len(remaining[j].Calls)
	})

	go func() {
		for res := range resultChan {
			results = append(results, res)
		}
		done <- true
	}()

	pass := 0
	sem := make(chan struct{}, a.maxConcurrency)
	for len(remaining) > 0 && pass < 10 {
		pass++
		newRemaining := []parser.FunctionInfo{}
		var wg sync.WaitGroup

		for _, f := range remaining {

			allDepsKnown := true
			a.descMu.RLock()
			for _, dep := range f.Calls {
				if _, ok := a.KnownDescriptions.Load(dep); !ok {
					allDepsKnown = false
					break
				}
			}
			a.descMu.RUnlock()

			if allDepsKnown {
				wg.Add(1)
				sem <- struct{}{}
				go func(fn parser.FunctionInfo) {
					defer wg.Done()
					defer func() { <-sem }()
					res := a.AnalyzeFunction(fn, false)
					if stored, found := a.loadStoredResult(f); found {
						res.Description = stored.Description
					}
					resultChan <- res
					knownDescMutex.Lock()
					a.descMu.Lock()
					a.KnownDescriptions.Store(fn.Name, res.Description)
					a.descMu.Unlock()
					knownDescMutex.Unlock()
				}(f)
			} else {
				newRemaining = append(newRemaining, f)
			}
		}

		wg.Wait()
		if len(newRemaining) == len(remaining) {
			log.Printf("[WARN] 检测到循环依赖或缺失依赖，开始强制分析剩余 %d 个函数", len(remaining))
			for _, f := range remaining {

				wg.Add(1)
				sem <- struct{}{}
				go func(fn parser.FunctionInfo) {
					defer wg.Done()
					defer func() { <-sem }()
					res := a.AnalyzeFunction(fn, false)
					if stored, found := a.loadStoredResult(f); found {
						res.Description = stored.Description
					}
					resultChan <- res
					knownDescMutex.Lock()
					a.descMu.Lock()
					a.KnownDescriptions.Store(fn.Name, res.Description)
					a.descMu.Unlock()
					knownDescMutex.Unlock()
				}(f)
			}
			wg.Wait()
			break
		}
		remaining = newRemaining
	}

	close(resultChan)
	<-done
	return results
}

// SaveSingleResultToDB 将单个分析结果写入数据库。
// - 对 functions 表用 INSERT OR REPLACE，以便更新已有描述。
// - 对 calls 和 externals 表用 INSERT OR IGNORE，跳过已存在的行。
// - 不使用事务，每条结果直接写入。
func SaveSingleResultToDB(db *sql.DB, res LLMAnalysisResult, projDir string) error {
	if res.Func.FunctionType == "llm_parser" {
		logs.Warnf("[WARN] 忽略 LLM_PARSER 函数的库录入 %s", res.Func.Name)
		return nil
	}
	// 判断是否存在临时文件，如果不存在则抛出特殊异常码
	gitgoDir := filepath.Join(projDir, ".gitgo")
	tempFilePath := filepath.Join(gitgoDir, "indexing.temp")
	if _, err := os.Stat(tempFilePath); os.IsNotExist(err) {
		panic("索引临时文件已被删除，终止扫描")
	}
	// 1. 准备 statements
	funcStmt, err := db.Prepare(`
        INSERT OR REPLACE INTO functions
        (name, package, file, description, start_line, end_line, function_type)
        VALUES (?, ?, ?, ?, ?, ?, ?)
    `)
	if err != nil {
		return fmt.Errorf("prepare functions 失败: %w", err)
	}
	defer funcStmt.Close()

	callStmt, err := db.Prepare(`
        INSERT OR IGNORE INTO calls (caller, callee)
        VALUES (?, ?)
    `)
	if err != nil {
		return fmt.Errorf("prepare calls 失败: %w", err)
	}
	defer callStmt.Close()

	extStmt, err := db.Prepare(`
        INSERT OR IGNORE INTO externals (function, external)
        VALUES (?, ?)
    `)
	if err != nil {
		return fmt.Errorf("prepare externals 失败: %w", err)
	}
	defer extStmt.Close()

	// 2. 处理相对路径
	if projDir != "" {
		filePath := res.Func.File
		// 如果不是绝对路径，就把它视作相对于 projDir 的子路径
		if !filepath.IsAbs(filePath) {
			filePath = filepath.Join(projDir, filePath)
		}
		// 再去算相对路径
		rel, err := filepath.Rel(projDir, filePath)
		if err != nil {
			logs.Errorf("%s, %s, 无法将文件路径转换为相对路径: %v", projDir, res.Func.File, err)
		} else {
			// 统一成 slash 格式
			res.Func.File = filepath.ToSlash(rel)
			logs.Infof("[DEBUG][DB] 存储文件路径为: %s", res.Func.File)
		}
	}

	// 3. 插入 functions
	if _, err := funcStmt.Exec(
		res.Func.Name,
		res.Func.Package,
		res.Func.File,
		res.Description,
		res.Func.StartLine,
		res.Func.EndLine,
		res.Func.FunctionType,
	); err != nil {
		return fmt.Errorf("functions 写入失败: %w", err)
	}

	// 4. 插入 calls
	for _, callee := range res.InternalDeps {
		if _, err := callStmt.Exec(res.Func.Name, callee); err != nil {
			log.Printf("calls 写入失败 (跳过继续): %v", err)
		}
	}

	// 5. 插入 externals
	for _, ext := range res.ExternalDeps {
		if _, err := extStmt.Exec(res.Func.Name, ext); err != nil {
			log.Printf("externals 写入失败 (跳过继续): %v", err)
		}
	}

	return nil
}

// —— 新增文件顶部 imports ——
// import "database/sql"
// import "log"

// —— 原来：LLMAnalyzer 类型定义之后，没有此方法 ——

// —— 新增：loadStoredResult 方法 ——
// 从数据库查询已存在的分析结果（仅取 description）
// 若存在，返回对应 LLMAnalysisResult 且 ok=true，否则 ok=false
func (a *LLMAnalyzer) loadStoredResult(fn parser.FunctionInfo) (LLMAnalysisResult, bool) {
	// 1. 如果没有数据库连接，直接返回
	if a.Db == nil {
		return LLMAnalysisResult{}, false
	}
	var desc string
	// 2. 按 File、Name、Package、StartLine、EndLine 精确匹配
	query := `
      SELECT description 
      FROM functions 
      WHERE file=? AND name=? AND package=? AND start_line=? AND end_line=?
    `
	if a.projDir != "" {
		filePath := fn.File
		// 如果不是绝对路径，就把它视作相对于 projDir 的子路径
		if !filepath.IsAbs(filePath) {
			filePath = filepath.Join(a.projDir, filePath)
		}
		fn.File, _ = filepath.Rel(a.projDir, filePath)
		logs.Infof("[DEBUG][DB] 存储文件路径为: %s", fn.File)
		fn.File = filepath.ToSlash(fn.File)
	}
	row := a.Db.QueryRow(query, fn.File, fn.Name, fn.Package, fn.StartLine, fn.EndLine)
	if err := row.Scan(&desc); err != nil {
		if err == sql.ErrNoRows {
			return LLMAnalysisResult{}, false
		}
		log.Printf("[ERROR] 查询已存结果失败: %v", err)
		return LLMAnalysisResult{}, false
	}
	logs.Infof("[DEBUG] 读取已存结果: %s", fn.File)
	// 3. 构造简单的返回值（只带 Func 和 Description，其它字段可按需扩展）
	return LLMAnalysisResult{
		Func:        fn,
		Description: desc,
	}, true
}
