package analyzer

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"sort"
	"strings"
	"sync"

	"github.com/kinglegendzzh/flashmemory/internal/parser"
	"github.com/kinglegendzzh/flashmemory/internal/utils"
	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"

	_ "path/filepath"
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
	KnownDescriptions map[string]string
	// 是否启用调试日志
	debug bool
	// 最大并发goroutine数量
	maxConcurrency int
}

// NewLLMAnalyzer 创建一个 LLMAnalyzer 实例
func NewLLMAnalyzer(initialKnown map[string]string, debug bool, maxConcurrency int) *LLMAnalyzer {
	return &LLMAnalyzer{KnownDescriptions: initialKnown, debug: debug, maxConcurrency: maxConcurrency}
}

// extractCodeSnippet 根据文件路径和起止行号提取代码片段
func extractCodeSnippet(filePath string, startLine, endLine int) (string, error) {
	data, err := ioutil.ReadFile(filePath)
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
func (a *LLMAnalyzer) AnalyzeFunction(fn parser.FunctionInfo) LLMAnalysisResult {
	if a.debug {
		log.Printf("[DEBUG] 开始分析函数: %s (文件: %s, 行数: %d)", fn.Name, fn.File, fn.Lines)
	}
	res := LLMAnalysisResult{Func: fn}

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

	// 构造提示词：将代码片段和依赖描述整合到一起
	prompt := fmt.Sprintf("你是一个专业的架构师，请仔细阅读以下函数代码：\n%s\n", snippet)
	if len(internalDeps) > 0 {
		// 对 internalDeps 去重
		dedup := make(map[string]bool)
		uniqueDeps := []string{}
		for _, dep := range internalDeps {
			if !dedup[dep] {
				dedup[dep] = true
				uniqueDeps = append(uniqueDeps, dep)
			}
		}
		logs.Infof("内部函数去重：\n%s\n", strings.Join(uniqueDeps, ", "))
		// 附加内部依赖的描述（如果已有）
		tip := 1
		for _, dep := range uniqueDeps {
			desc, ok := a.KnownDescriptions[dep]
			if ok {
				if tip == 1 {
					prompt += "该函数调用了以下内部函数：\n"
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
					data["description"] = strings.ReplaceAll(data["description"].(string), "\n", "")
					prompt += fmt.Sprintf("%d. %s(%s)\n", tip, dep, data["description"])
				}
				tip++
			}
		}
	}
	// 附加外部依赖信息
	if len(externalDeps) > 0 {
		prompt += fmt.Sprintf("并使用了外部库: %s\n", strings.Join(externalDeps, ", "))
	}
	prompt += `
请用几句话为以上代码生成该实现的<功能描述>，并说明它的<执行流程>。
（输出必须为一个合法的 JSON 对象，<功能描述>的Key是"description"，Value是字符串类型；<执行流程>的Key是"process"，Value是字符串数组；所有Value均使用中文描述。）`
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
	result, err := utils.OllamaCompletion(prompt)
	if err != nil {
		log.Printf("[ERROR] 调用大模型失败: %v", err)
	}
	if a.debug {
		log.Printf("[DEBUG] 大模型完成了生成描述完成，提示词长度: %d 字符，输出长度: %d 字符", len(prompt), len(result))
		logs.Infof("[DEBUG][ModelResponse] 提示词: \n%s\n", prompt)
		logs.Tokenf("[DEBUG][ModelResponse] 描述内容: \n%s\n", result)
	}
	res.Description = result

	// 计算简单的重要性评分（例如：内部依赖数量 + 0.1*代码行数）
	res.ImportanceScore = float64(len(internalDeps)) + 0.1*float64(fn.Lines)
	return res
}

// AnalyzeAll 对所有函数进行依赖感知的自底向上分析
func (a *LLMAnalyzer) AnalyzeAll(funcs []parser.FunctionInfo) []LLMAnalysisResult {
	if a.debug {
		log.Printf("[DEBUG] 开始批量分析 %d 个函数", len(funcs))
	}

	// 创建结果通道和工作池
	resultChan := make(chan LLMAnalysisResult, len(funcs))
	done := make(chan bool)
	results := []LLMAnalysisResult{}

	// 按照函数在文件中的出现顺序或名称+类型进行初步归类（这里仅作归类记录）
	classified := make(map[string][]parser.FunctionInfo)
	for _, f := range funcs {
		key := fmt.Sprintf("%s_%s_%s", f.File, f.Name, f.FunctionType)
		classified[key] = append(classified[key], f)
	}

	// 自底向上排序：依赖调用数较少的函数排在前面
	remaining := funcs
	sort.SliceStable(remaining, func(i, j int) bool {
		return len(remaining[i].Calls) < len(remaining[j].Calls)
	})

	// 启动结果收集goroutine
	go func() {
		for res := range resultChan {
			results = append(results, res)
		}
		done <- true
	}()

	pass := 0
	knownDesc := a.KnownDescriptions
	sem := make(chan struct{}, a.maxConcurrency) // 并发控制信号量
	for len(remaining) > 0 && pass < 10 {
		pass++
		newRemaining := []parser.FunctionInfo{}
		var wg sync.WaitGroup

		for _, f := range remaining {
			allDepsKnown := true
			for _, dep := range f.Calls {
				if _, ok := knownDesc[dep]; !ok {
					allDepsKnown = false
					break
				}
			}

			if allDepsKnown {
				wg.Add(1)
				sem <- struct{}{} // 获取信号量
				go func(fn parser.FunctionInfo) {
					defer wg.Done()
					defer func() { <-sem }() // 释放信号量
					res := a.AnalyzeFunction(fn)
					resultChan <- res
					knownDesc[fn.Name] = res.Description
				}(f)
			} else {
				newRemaining = append(newRemaining, f)
			}
		}

		wg.Wait()
		// 如果本轮没有取得任何进展，则强制分析剩余的函数
		if len(newRemaining) == len(remaining) {
			log.Printf("[WARN] 检测到循环依赖或缺失依赖，开始强制分析剩余 %d 个函数", len(remaining))
			for _, f := range remaining {
				wg.Add(1)
				sem <- struct{}{}
				go func(fn parser.FunctionInfo) {
					defer wg.Done()
					defer func() { <-sem }()
					// 在强制分析时，提示词中依然会列出已知的依赖描述，缺失的部分留空，由大模型根据上下文补全
					res := a.AnalyzeFunction(fn)
					resultChan <- res
					knownDesc[fn.Name] = res.Description
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
