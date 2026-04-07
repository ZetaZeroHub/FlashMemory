package analyzer

import (
	"fmt"
	"log"
	"strings"

	"github.com/kinglegendzzh/flashmemory/internal/parser"
)

// AnalysisResult 保存函数的 AI 生成的信息.
type AnalysisResult struct {
	Func            parser.FunctionInfo
	Description     string   // AI生成的函数行为描述
	InternalDeps    []string // 调用的内部函数列表（可能解析为package.name格式）
	ExternalDeps    []string // 使用的外部依赖包列表
	ImportanceScore float64  // 可选：基于函数规模和依赖关系的重要性评分（用于排序）
}

// Analyzer 分析函数并生成 AnalysisResult.
// 它可以使用 LLM （此处模拟） 来生成描述.
type Analyzer struct {
	// 可能包含依赖关系图或已知函数描述的缓存
	KnownDescriptions map[string]string // 函数名到描述信息的映射（用于依赖分析）
}

// NewAnalyzer 创建一个具有可选已知描述映射的 Analyzer（最初可以为空）。
func NewAnalyzer(initialKnown map[string]string) *Analyzer {
	return &Analyzer{KnownDescriptions: initialKnown}
}

// AnalyzeFunction 为单个函数生成分析.
func (a *Analyzer) AnalyzeFunction(fn parser.FunctionInfo) AnalysisResult {
	res := AnalysisResult{Func: fn}
	// 根据调用列表和导入列表区分内部/外部依赖
	internalDeps := []string{}
	externalDeps := []string{}

	// 遍历每个调用，判断是内部（项目内）还是外部（导入包）依赖
	for _, callee := range fn.Calls {
		// 如果被调用者包含点号且前缀匹配导入包的别名或包名，则视为外部依赖
		if strings.Contains(callee, ".") {
			parts := strings.Split(callee, ".")
			prefix := parts[0]
			// 粗略检查：前缀是否匹配导入包名（取导入路径的最后部分）
			isExt := false
			for _, imp := range fn.Imports {
				impBase := imp
				if slash := strings.LastIndex(imp, "/"); slash != -1 {
					impBase = imp[slash+1:]
				}
				if impBase == prefix {
					isExt = true
					externalDeps = append(externalDeps, imp)
					break
				}
			}
			if isExt {
				continue // skip adding to internalDeps
			}
		}
		// 走到此处视为内部依赖（可通过检查内部函数名列表进一步优化）
		internalDeps = append(internalDeps, callee)
	}
	res.InternalDeps = internalDeps
	res.ExternalDeps = externalDeps

	// 生成AI描述（当前为模拟实现）
	// 如果已知其依赖项的描述信息，则进行整合
	description := ""
	if len(internalDeps) == 0 {
		// 基本情况：无内部依赖的简单函数
		description = fmt.Sprintf("%s is a small function in package %s. It likely performs a single task.", fn.Name, fn.Package)
	} else {
		description = fmt.Sprintf("%s is a higher-level function in package %s that calls %d other functions: %s. ",
			fn.Name, fn.Package, len(internalDeps), strings.Join(internalDeps, ", "))
		description += "It orchestrates their results to achieve its goal."
	}
	// 如果有外部依赖则追加说明
	if len(externalDeps) > 0 {
		description += fmt.Sprintf(" It utilizes external libraries such as %s.", strings.Join(externalDeps, ", "))
	}
	// 函数规模判断：
	if fn.Lines > 100 {
		description += " This is a relatively large function, indicating it may be doing quite a lot."
	} else if fn.Lines > 0 {
		description += fmt.Sprintf(" (~%d lines of code)", fn.Lines)
	}
	res.Description = description

	// 简单的重要性评分公式：内部依赖数 + 外部依赖数 + 代码行数因子
	res.ImportanceScore = float64(len(internalDeps)) + 0.1*float64(fn.Lines)
	return res
}

// AnalyzeAll 处理具有依赖关系感知排序的函数列表.
func (a *Analyzer) AnalyzeAll(funcs []parser.FunctionInfo) []AnalysisResult {
	results := []AnalysisResult{}
	// 创建函数名到函数信息的映射便于快速查找
	funcMap := map[string]parser.FunctionInfo{}
	for _, f := range funcs {
		key := f.Name
		if f.Package != "" {
			key = f.Package + "." + f.Name
		}
		funcMap[key] = f
	}
	// 简单策略：先分析无依赖函数，再处理其他
	remaining := make([]parser.FunctionInfo, len(funcs))
	copy(remaining, funcs)

	pass := 0
	knownDesc := a.KnownDescriptions
	for len(remaining) > 0 && pass < 999 {
		pass++
		newRemaining := []parser.FunctionInfo{}
		log.Printf("Round %d of analysis, number of remaining functions: %d", pass, len(remaining))

		for _, f := range remaining {
			allKnown := true
			missingDeps := []string{}

			for _, callee := range f.Calls {
				if _, ok := knownDesc[callee]; !ok {
					allKnown = false
					missingDeps = append(missingDeps, callee)
				}
			}

			if !allKnown {
				log.Printf("Function %s.%s is deferred, missing dependencies: %v", f.Package, f.Name, missingDeps)
				newRemaining = append(newRemaining, f)
			} else {
				res := a.AnalyzeFunction(f)
				results = append(results, res)
				knownDesc[f.Name] = res.Description
				log.Printf("Analyzed function: %s.%s (number of dependencies: %d)", f.Package, f.Name, len(f.Calls))
			}
		}

		log.Printf("Number of remaining functions after this round of processing: %d", len(newRemaining))
		if len(newRemaining) == len(remaining) {
			log.Printf("Warning: Possible circular dependency or unknown identifier detected, unhandled function example: %s.%s",
				remaining[0].Package, remaining[0].Name)
			break
		}
		remaining = newRemaining
	}
	return results
}
