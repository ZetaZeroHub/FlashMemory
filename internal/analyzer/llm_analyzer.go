package analyzer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kinglegendzzh/flashmemory/cmd/common"
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
	// 数据库写操作互斥锁 (已废弃，由 DbWriter 替代)
	dbWriteMu sync.Mutex
	// 数据库写入管理器，用于串行化写入和自动重试
	DbWriter *utils.DbWriter
}

// NewLLMAnalyzer 创建一个 LLMAnalyzer 实例
func NewLLMAnalyzer(initialKnown *sync.Map, debug bool, maxConcurrency int) *LLMAnalyzer {
	return &LLMAnalyzer{KnownDescriptions: initialKnown, debug: debug, maxConcurrency: maxConcurrency}
}

func NewLLMAnalyzerHttp(initialKnown *sync.Map, debug bool, maxConcurrency int, db *sql.DB, projDir string) *LLMAnalyzer {
	// 创建 LLMAnalyzer 实例
	analyzer := &LLMAnalyzer{
		KnownDescriptions: initialKnown,
		debug:             debug,
		maxConcurrency:    maxConcurrency,
		Db:                db,
		projDir:           projDir,
	}

	// 配置SQLite连接并初始化 DbWriter
	if db != nil {
		// 创建 DbWriter 实例
		analyzer.DbWriter = utils.NewDbWriter(db)

		// 设置SQLite的busy_timeout (由 DbWriter 处理，此处可保留做双保险)
		_, err := db.Exec("PRAGMA busy_timeout = 5000;") // 5秒
		if err != nil {
			log.Printf("[WARN] 设置SQLite busy_timeout失败: %v", err)
		}

		// 设置SQLite的journal模式为WAL，提高并发性能 (由 DbWriter 处理，此处可保留做双保险)
		_, err = db.Exec("PRAGMA journal_mode = WAL;")
		if err != nil {
			log.Printf("[WARN] 设置SQLite journal_mode失败: %v", err)
		}
	}

	return analyzer
}

// AnalyzeFunction 分析单个函数：归类、提取代码片段、构建提示词并调用大模型生成描述
func (a *LLMAnalyzer) AnalyzeFunction(fn parser.FunctionInfo, startLlm bool) (llmres LLMAnalysisResult, err error) {
	if a.debug {
		log.Printf("[DEBUG] 开始分析函数: %s (文件: %s, 行数: %d)", fn.Name, fn.File, fn.Lines)
		log.Printf("[DEBUG] 函数元信息: %v", fn)
	}
	res := LLMAnalysisResult{Func: fn}
	if fn.FunctionType == "llm_parser" {
		// 函数描述已存在，直接返回
		logs.Warnf("[DEBUG] llm_parser 函数描述已存在，直接返回")
		res.Description = fn.Description
		res.CodeSnippet = fn.CodeSnippet
		return res, nil
	}
	// 提取代码片段
	snippet, err := utils.ExtractCodeSnippet(fn.File, fn.StartLine, fn.EndLine)
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
			prompt = prompt[:cfg.CodeLimit] + "..."
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
			prompt += fmt.Sprintf("\n%s \n%s(%s)\n", cfg.AnaPrompts.Route, displayPath, fn.Package)
		} else {
			prompt += fmt.Sprintf("\n%s \n%s\n", cfg.AnaPrompts.Route, fn.File)
		}
		if fn.Imports != nil {
			prompt += fmt.Sprintf("\n%s \n%s\n", cfg.AnaPrompts.Imports, strings.Join(fn.Imports, ", "))
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
				} else {
					prompt += fmt.Sprintf("%d. %s\n", tip, dep)
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

		// 添加EOF错误重试机制
		var result string
		var err error
		isRetryable := func(e error) bool {
			if e == nil {
				return false
			}
			s := e.Error()
			if strings.Contains(s, "EOF") {
				return true
			}
			if strings.Contains(s, "connection reset") || strings.Contains(s, "broken pipe") {
				return true
			}
			if strings.Contains(s, "context deadline exceeded") || strings.Contains(s, "Client.Timeout exceeded") {
				return true
			}
			if strings.Contains(strings.ToLower(s), "timeout") {
				return true
			}
			if strings.Contains(s, "status code: 500") || strings.Contains(s, "status code: 502") || strings.Contains(s, "status code: 503") || strings.Contains(s, "status code: 504") {
				return true
			}
			return false
		}
		maxRetries := 3
		for retry := 0; retry < maxRetries; retry++ {
			result, err = utils.Completion(prompt)
			if err != nil {
				if isRetryable(err) && retry < maxRetries-1 {
					backoff := time.Duration(500*(1<<retry)) * time.Millisecond
					if backoff > 8*time.Second {
						backoff = 8 * time.Second
					}
					logs.Warnf("LLM请求失败，将重试 %d/%d (sleep=%s): %v", retry+2, maxRetries, backoff, err)
					time.Sleep(backoff)
					continue
				}
				logs.Errorf("调用大模型失败: %v", err)
				return LLMAnalysisResult{}, err
			}
			break
		}

		tokens := []string{"```", "json", "```json"}
		for _, v := range tokens {
			if strings.Contains(result, v) {
				logs.Tokenf("(!remove: %v!)", v)
				result = strings.Replace(result, v, "", 2)
			}
		}
		result = utils.FilterJSONContent(result)
		if err != nil {
			logs.Errorf("调用大模型失败: %v", err)
			return LLMAnalysisResult{}, err
		}
		if a.debug {
			logs.Infof("[DEBUG] 大模型完成了生成描述完成，提示词长度: %d 字符，输出长度: %d 字符", len(prompt), len(result))
			logs.Infof("[DEBUG][ModelResponse] 提示词: \n%s\n", prompt)
			logs.Tokenf("[DEBUG][ModelResponse] 描述内容: \n%s\n", result)
		}

		// result检查：1. 是否为json 2. 是否包含description字段 3. 是否包含process字段
		var data map[string]interface{}
		err = json.Unmarshal([]byte(result), &data)
		logs.Infof("---CHECK JSON---")
		if err == nil {
			if data["description"] != nil && data["process"] != nil {
				logs.Infof("[DEBUG] JSON 格式校验通过")
			} else {
				logs.Warnf("[ERROR] JSON 格式错误，缺少description或process字段: %s", result)
			}
		} else {
			logs.Warnf("[ERROR] 解析 JSON 出错: %s, %v", result, err)
		}
		logs.Infof("---CHECK JSON END---")
		if result == "" {
			logs.Warnf("[ERROR] 生成描述为空，跳过: %s", fn.Name)
			return LLMAnalysisResult{}, fmt.Errorf("生成描述为空")
		} else {
			res.Description = result
		}
	}

	// 计算简单的重要性评分
	res.ImportanceScore = fn.Score
	logs.Infof("%s 重要性评分: %.5f", res.Func.Name, res.ImportanceScore)
	return res, nil
}

// AnalyzeAll 对所有函数进行依赖感知的自底向上分析
func (a *LLMAnalyzer) AnalyzeAll(funcs []parser.FunctionInfo) ([]LLMAnalysisResult, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if a.debug {
		log.Printf("[DEBUG] 开始批量分析 %d 个函数", len(funcs))
	}

	// 预处理：检查并删除已更新的函数记录
	if a.Db != nil {
		for _, fn := range funcs {
			var dbLineInfo []string
			// 处理文件路径，转换为项目相对路径
			processedFn := fn
			if a.projDir != "" {
				filePath := fn.File
				if !filepath.IsAbs(filePath) {
					filePath = filepath.Join(a.projDir, filePath)
				}
				var err error
				processedFn.File, err = filepath.Rel(a.projDir, filePath)
				if err != nil {
					logs.Errorf("[ERROR] 获取相对路径失败 %s: %v", filePath, err)
					continue
				}
				processedFn.File = filepath.ToSlash(processedFn.File)
			}

			// 查询数据库中是否存在此函数的记录
			query := `
				SELECT start_line, end_line 
				FROM functions 
				WHERE file=? AND name=? AND package=?
			`
			rows, err := a.Db.Query(query, processedFn.File, processedFn.Name, processedFn.Package)
			if err != nil {
				logs.Errorf("[ERROR] 预处理查询失败 %s:%s: %v", processedFn.File, processedFn.Name, err)
				continue
			}

			foundMatch := false
			foundAny := false
			for rows.Next() {
				foundAny = true
				var dbStartLine, dbEndLine int
				if err := rows.Scan(&dbStartLine, &dbEndLine); err != nil {
					logs.Errorf("[ERROR] 预处理扫描结果失败: %v", err)
					continue
				}
				dbLineInfo = append(dbLineInfo, fmt.Sprintf("%d-%d", dbStartLine, dbEndLine))
				// 检查行号是否匹配
				if dbStartLine == processedFn.StartLine && dbEndLine == processedFn.EndLine {
					foundMatch = true
					break
				}
			}
			rows.Close()

			// 如果找到记录但行号不匹配，删除旧记录
			if foundAny && !foundMatch && processedFn.Package != "" {
				a.dbWriteMu.Lock()
				deleteQuery := `DELETE FROM functions WHERE file=? AND name=? AND package=?`
				if _, err := a.Db.Exec(deleteQuery, processedFn.File, processedFn.Name, processedFn.Package); err != nil {
					logs.Errorf("[ERROR][DB] 预处理删除旧记录失败 %s:%s: %v", processedFn.File, processedFn.Name, err)
				} else {
					logs.Infof("已知的所有旧行号信息中未找到新行号匹配记录: %v, 新行号 %d-%d", dbLineInfo, processedFn.StartLine, processedFn.EndLine)
					logs.Infof("[INFO][DB] 预处理成功删除 %s:%s:%s 的旧记录", processedFn.File, processedFn.Name, processedFn.Package)
				}
				a.dbWriteMu.Unlock()
			}
		}
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
	var dbErr error
	var dbErrOnce sync.Once
	for len(remaining) > 0 && pass < 10 {
		pass++
		newRemaining := []parser.FunctionInfo{}
		var wg sync.WaitGroup

		for _, f := range remaining {
			if a.Db != nil {
				if stored, found := a.safeLoadStoredResult(f); found {
					res, _ := a.AnalyzeFunction(f, false)
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
					// 检查 ctx 是否已被取消
					if ctx.Err() != nil {
						return
					}
					res, err := a.AnalyzeFunction(fn, true)
					if err != nil {
						if common.IsLLMError(err) {
							logs.Errorf("分析函数 %s 失败（LLM Error）: %v", fn.Name, err)
							dbErrOnce.Do(func() {
								dbErr = err
								cancel()
							})
							return
						}
						logs.Warnf("分析函数 %s 失败: %v", fn.Name, err)
						return
					}
					if ctx.Err() != nil {
						return
					}
					if a.Db != nil {
						if err := SaveSingleResultToDB(a.Db, res, a.projDir); err != nil {
							dbErrOnce.Do(func() {
								dbErr = err
								cancel()
							})
							return
						}
					}
					if ctx.Err() != nil {
						return
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
		if dbErr != nil {
			close(resultChan)
			<-done
			return nil, dbErr
		}
		if len(newRemaining) == len(remaining) {
			log.Printf("[WARN] 检测到循环依赖或缺失依赖，开始强制分析剩余 %d 个函数", len(remaining))
			for _, f := range remaining {
				if a.Db != nil {
					if stored, found := a.safeLoadStoredResult(f); found {
						res, _ := a.AnalyzeFunction(f, false)
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
					if ctx.Err() != nil {
						return
					}
					res, err := a.AnalyzeFunction(fn, true)
					if err != nil {
						if common.IsLLMError(err) {
							logs.Errorf("分析函数 %s 失败（LLM Error）: %v", fn.Name, err)
							dbErrOnce.Do(func() {
								dbErr = err
								cancel()
							})
							return
						}
						logs.Warnf("分析函数 %s 失败: %v", fn.Name, err)
						return
					}
					if ctx.Err() != nil {
						return
					}
					if a.Db != nil {
						if err := SaveSingleResultToDB(a.Db, res, a.projDir); err != nil {
							dbErrOnce.Do(func() {
								dbErr = err
								cancel()
							})
							return
						}
					}
					if ctx.Err() != nil {
						return
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
			if dbErr != nil {
				close(resultChan)
				<-done
				return nil, dbErr
			}
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
	return results, dbErr
}

// AnalyzeAll 对所有函数进行依赖感知的自底向上分析
func (a *LLMAnalyzer) LoadAll(funcs []parser.FunctionInfo) []LLMAnalysisResult {
	// 设置SQLite的busy超时，避免立即返回SQLITE_BUSY错误
	if a.Db != nil {
		_, err := a.Db.Exec("PRAGMA busy_timeout = 5000;") // 设置5秒超时
		if err != nil {
			log.Printf("[WARN] 设置SQLite busy_timeout失败: %v", err)
		}
	}
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
					res, _ := a.AnalyzeFunction(fn, false)
					if stored, found := a.safeLoadStoredResult(fn); found {
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
					res, _ := a.AnalyzeFunction(fn, false)
					if stored, found := a.safeLoadStoredResult(fn); found {
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
// - 使用 DbWriter 进行串行化写入和自动重试。
func SaveSingleResultToDB(db *sql.DB, res LLMAnalysisResult, projDir string) error {
	// 如果 res 中有 Analyzer 字段，优先使用其 DbWriter
	if analyzer, ok := res.Func.Parser.(*LLMAnalyzer); ok && analyzer.DbWriter != nil {
		return saveSingleResultToDBWithWriter(analyzer.DbWriter, res, projDir)
	}

	// 创建临时 DbWriter
	dbWriter := utils.NewDbWriter(db)
	defer dbWriter.Close()

	return saveSingleResultToDBWithWriter(dbWriter, res, projDir)
}

// saveSingleResultToDBWithWriter 使用 DbWriter 进行数据库写入操作
func saveSingleResultToDBWithWriter(dbWriter *utils.DbWriter, res LLMAnalysisResult, projDir string) error {
	// 判断是否存在临时文件，如果不存在则抛出特殊异常码
	gitgoDir := filepath.Join(projDir, ".gitgo")
	tempFilePath := filepath.Join(gitgoDir, "indexing.temp")
	if _, err := os.Stat(tempFilePath); os.IsNotExist(err) {
		logs.Errorf("索引临时文件已被删除，终止扫描")
		return fmt.Errorf("索引临时文件已被删除，终止扫描")
	}

	// 处理相对路径
	filePath := res.Func.File
	if projDir != "" {
		// 如果不是绝对路径，就把它视作相对于 projDir 的子路径
		if !filepath.IsAbs(filePath) {
			filePath = filepath.Join(projDir, filePath)
		}
		rel, err := filepath.Rel(projDir, filePath)
		if err != nil {
			logs.Errorf("%s, %s, 无法将文件路径转换为相对路径: %v", projDir, res.Func.File, err)
		} else {
			res.Func.File = filepath.ToSlash(rel)
			logs.Infof("[DEBUG][DB] 存储文件路径为: %s", res.Func.File)
		}
	}

	// 使用 DbWriter 写入 functions 表
	funcSQL := `
        INSERT OR REPLACE INTO functions
        (name, package, file, description, start_line, end_line, function_type)
        VALUES (?, ?, ?, ?, ?, ?, ?)
    `
	err := dbWriter.Write("functions", funcSQL,
		res.Func.Name,
		res.Func.Package,
		res.Func.File,
		res.Description,
		res.Func.StartLine,
		res.Func.EndLine,
		res.Func.FunctionType,
	)
	if err != nil {
		return fmt.Errorf("functions 写入失败: %w", err)
	}

	// 写入 calls
	callSQL := `INSERT OR IGNORE INTO calls (caller, callee) VALUES (?, ?)`
	for _, callee := range res.InternalDeps {
		dbWriter.WriteAsync("calls", callSQL, res.Func.Name, callee)
	}

	// 写入 externals
	extSQL := `INSERT OR IGNORE INTO externals (function, external) VALUES (?, ?)`
	for _, ext := range res.ExternalDeps {
		dbWriter.WriteAsync("externals", extSQL, res.Func.Name, ext)
	}

	return nil
}

// saveSingleResultToDBOnce 是原有的单次写入逻辑
func saveSingleResultToDBOnce(db *sql.DB, res LLMAnalysisResult, projDir string) error {
	if res.Func.FunctionType == "llm_parser" {
		logs.Warnf("[WARN] 忽略 LLM_PARSER 函数的库录入 %s", res.Func.Name)
		return nil
	}
	if strings.TrimSpace(res.Description) == "" {
		logs.Warnf("[WARN] 函数 %s 描述为空，跳过入库", res.Func.Name)
		return nil
	}
	// 判断是否存在临时文件，如果不存在则抛出特殊异常码
	gitgoDir := filepath.Join(projDir, ".gitgo")
	tempFilePath := filepath.Join(gitgoDir, "indexing.temp")
	if _, err := os.Stat(tempFilePath); os.IsNotExist(err) {
		logs.Errorf("索引临时文件已被删除，终止扫描")
		return fmt.Errorf("索引临时文件已被删除，终止扫描")
	}
	// 1. 开始事务
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}

	// 准备事务内的statements
	funcStmt, err := tx.Prepare(`
        INSERT OR REPLACE INTO functions
        (name, package, file, description, start_line, end_line, function_type)
        VALUES (?, ?, ?, ?, ?, ?, ?)
    `)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("prepare functions 失败: %w", err)
	}
	defer funcStmt.Close()

	callStmt, err := tx.Prepare(`
        INSERT OR IGNORE INTO calls (caller, callee)
        VALUES (?, ?)
    `)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("prepare calls 失败: %w", err)
	}
	defer callStmt.Close()

	extStmt, err := tx.Prepare(`
        INSERT OR IGNORE INTO externals (function, external)
        VALUES (?, ?)
    `)
	if err != nil {
		tx.Rollback()
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
	logs.Infof("[DEBUG][DB] 插入函数 %s:%s %s %d-%d", res.Func.File, res.Func.Name, res.Func.Package, res.Func.StartLine, res.Func.EndLine)

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

	// 6. 提交事务
	if err := tx.Commit(); err != nil {
		tx.Rollback()
		return fmt.Errorf("提交事务失败: %w", err)
	}

	return nil
}

// —— 新增文件顶部 imports ——
// import "database/sql"
// import "log"

// —— 原来：LLMAnalyzer 类型定义之后，没有此方法 ——

// safeLoadStoredResult 安全地从数据库查询已存在的分析结果，带有互斥锁保护
func (a *LLMAnalyzer) safeLoadStoredResult(fn parser.FunctionInfo) (LLMAnalysisResult, bool) {
	a.dbWriteMu.Lock()
	defer a.dbWriteMu.Unlock()
	return a.loadStoredResult(fn)
}

// loadStoredResult 从数据库查询已存在的分析结果
// 1. 按 File, Name, Package 查询
// 2. 若找到，则比对 StartLine 和 EndLine
// 3. 若 Line 未变，则返回缓存结果，ok=true
// 4. 若 Line 已变或未找到，则返回 ok=false，以便重新分析
// 注意：此函数不再负责删除旧记录，删除逻辑已移至 AnalyzeAll 方法开头
func (a *LLMAnalyzer) loadStoredResult(fn parser.FunctionInfo) (LLMAnalysisResult, bool) {
	// 1. 如果没有数据库连接，直接返回
	if a.Db == nil {
		return LLMAnalysisResult{}, false
	}

	// 2. 处理文件路径，转换为项目相对路径
	if a.projDir != "" {
		filePath := fn.File
		if !filepath.IsAbs(filePath) {
			filePath = filepath.Join(a.projDir, filePath)
		}
		var err error
		fn.File, err = filepath.Rel(a.projDir, filePath)
		if err != nil {
			logs.Errorf("[ERROR] 获取相对路径失败 %s: %v", filePath, err)
			return LLMAnalysisResult{}, false
		}
		fn.File = filepath.ToSlash(fn.File)
		logs.Infof("[DEBUG][DB] 标准化文件路径为: %s", fn.File)
	}

	// 3. 按 File, Name, Package 查询，查找所有可能的记录，遍历比对 StartLine 和 EndLine
	query := `
        SELECT start_line, end_line, package, description 
        FROM functions 
        WHERE file=? AND name=? AND package=?
    `
	rows, err := a.Db.Query(query, fn.File, fn.Name, fn.Package)
	if err != nil {
		logs.Errorf("[ERROR] 查询已存结果失败 %s:%s: %v", fn.File, fn.Name, err)
		return LLMAnalysisResult{}, false
	}
	defer rows.Close()

	foundAny := false
	var dbStartLine, dbEndLine int
	var dbPackage, desc string
	// 查询行号
	logs.Infof("[DEBUG][DB] 查询行号 %s:%s %s %d-%d", fn.File, fn.Name, fn.Package, fn.StartLine, fn.EndLine)
	for rows.Next() {
		foundAny = true
		if err := rows.Scan(&dbStartLine, &dbEndLine, &dbPackage, &desc); err != nil {
			logs.Errorf("[ERROR] 遍历查询结果时出错: %v", err)
			continue
		}
		logs.Infof("[DEBUG][DB] 查询到 %s:%s %s %d-%d", fn.File, fn.Name, fn.Package, dbStartLine, dbEndLine)
		if dbStartLine == fn.StartLine && dbEndLine == fn.EndLine {
			logs.Infof("[DEBUG][DB] 发现 %s:%s 的可用结果", fn.File, fn.Name)
			return LLMAnalysisResult{
				Func:        fn,
				Description: desc,
			}, true
		}
		// package为空时跳过
		if dbPackage == "" || fn.Package == "" {
			logs.Infof("[INFO][DB] package字段为空(%s:%s)，视为新函数，继续分析 %s:%s", dbPackage, fn.Package, fn.File, fn.Name)
			return LLMAnalysisResult{}, false
		}
	}
	if err := rows.Err(); err != nil {
		logs.Errorf("[ERROR] rows.Err(): %v", err)
	}
	if !foundAny {
		logs.Infof("[DEBUG][DB] 未找到 %s:%s 的已存结果，将进行分析", fn.File, fn.Name)
	}

	// 代码位置已更新或未找到记录，返回 false 以便重新分析
	if foundAny {
		logs.Infof("[INFO][DB] 代码已更新 %s:%s. 未找到匹配的起止行号，旧行号 %d-%d，新行号 %d-%d", fn.File, fn.Name, dbStartLine, dbEndLine, fn.StartLine, fn.EndLine)
	}
	return LLMAnalysisResult{}, false
}
