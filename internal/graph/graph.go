package graph

import (
	"encoding/json"
	"os"
	"strconv"

	"github.com/kinglegendzzh/flashmemory/internal/analyzer"
)

// KnowledgeGraph holds the nodes (functions) and relationships.
type KnowledgeGraph struct {
	Functions []analyzer.LLMAnalysisResult // all function analysis results
	Calls     map[string][]string          // adjacency list: func -> funcs it calls
	CalledBy  map[string][]string          // reverse adjacency: func -> funcs that call it
	Packages  map[string][]string          // package -> functions in that package
	Externals map[string][]string          // external lib -> functions using it
}

// BuildGraph constructs the knowledge graph from analysis results.
func BuildGraph(results []analyzer.LLMAnalysisResult) KnowledgeGraph {
	// 先清空所有 CodeSnippet
	for i := range results {
		results[i].CodeSnippet = ""
	}

	kg := KnowledgeGraph{
		Functions: results,
		Calls:     make(map[string][]string),
		CalledBy:  make(map[string][]string),
		Packages:  make(map[string][]string),
		Externals: make(map[string][]string),
	}
	// 后面的逻辑就不用再清空 CodeSnippet 了
	for _, res := range kg.Functions {
		name := res.Func.Name
		if res.Func.Package != "" {
			name = res.Func.Package + "." + name
		}
		pkg := res.Func.Package
		if pkg == "" {
			pkg = "(root)"
		}
		kg.Packages[pkg] = append(kg.Packages[pkg], name)
		for _, callee := range res.InternalDeps {
			kg.Calls[name] = append(kg.Calls[name], callee)
			kg.CalledBy[callee] = append(kg.CalledBy[callee], name)
		}
		for _, lib := range res.ExternalDeps {
			kg.Externals[lib] = append(kg.Externals[lib], name)
		}
	}
	return kg
}

// SaveGraphJSON 以“查漏补缺”的方式保存知识图谱到 JSON 文件。
// 先读取已有的 JSON 文件（如果存在），然后将 kg 中的数据与其合并：
// 1. 如果 kg 中有新内容，则新增；
// 2. 如果已存在，则替换；
// 3. 其余内容保持不变。
func (kg *KnowledgeGraph) SaveGraphJSON(path string) error {
	var oldKG KnowledgeGraph

	// 读取已有文件（如果存在）
	data, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(data, &oldKG)
	} else if !os.IsNotExist(err) {
		return err // 其他读取错误
	}

	// 合并 Functions（以 Func.Name + Package 唯一标识）
	funcKey := func(f analyzer.LLMAnalysisResult) string {
		return f.Func.FunctionType +
			"." + f.Func.Name +
			"." + f.Func.File +
			"." + f.Func.Package +
			":" + strconv.Itoa(f.Func.StartLine) +
			"-" + strconv.Itoa(f.Func.EndLine)
	}
	oldFuncMap := make(map[string]analyzer.LLMAnalysisResult)
	for _, f := range oldKG.Functions {
		oldFuncMap[funcKey(f)] = f
	}
	for _, f := range kg.Functions {
		oldFuncMap[funcKey(f)] = f // 新的覆盖旧的
	}
	mergedFuncs := make([]analyzer.LLMAnalysisResult, 0, len(oldFuncMap))
	for _, f := range oldFuncMap {
		mergedFuncs = append(mergedFuncs, f)
	}

	// 合并 Calls
	mergedCalls := make(map[string][]string)
	for k, v := range oldKG.Calls {
		mergedCalls[k] = append([]string{}, v...)
	}
	for k, v := range kg.Calls {
		mergedCalls[k] = v // 新的覆盖旧的
	}

	// 合并 CalledBy
	mergedCalledBy := make(map[string][]string)
	for k, v := range oldKG.CalledBy {
		mergedCalledBy[k] = append([]string{}, v...)
	}
	for k, v := range kg.CalledBy {
		mergedCalledBy[k] = v
	}

	// 合并 Packages
	mergedPackages := make(map[string][]string)
	for k, v := range oldKG.Packages {
		mergedPackages[k] = append([]string{}, v...)
	}
	for k, v := range kg.Packages {
		mergedPackages[k] = v
	}

	// 合并 Externals
	mergedExternals := make(map[string][]string)
	for k, v := range oldKG.Externals {
		mergedExternals[k] = append([]string{}, v...)
	}
	for k, v := range kg.Externals {
		mergedExternals[k] = v
	}

	// 构造合并后的 KnowledgeGraph
	mergedKG := KnowledgeGraph{
		Functions: mergedFuncs,
		Calls:     mergedCalls,
		CalledBy:  mergedCalledBy,
		Packages:  mergedPackages,
		Externals: mergedExternals,
	}

	// 写回文件
	out, err := json.MarshalIndent(mergedKG, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0644)
}

func (kg *KnowledgeGraph) SaveGraphJSONOnlyFunctions(path string) error {
	var oldKG KnowledgeGraph

	// 读取已有文件（如果存在）
	data, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(data, &oldKG)
	} else if !os.IsNotExist(err) {
		return err // 其他读取错误
	}

	// 合并 Functions（以 Func.Name + Package 唯一标识）
	funcKey := func(f analyzer.LLMAnalysisResult) string {
		return f.Func.FunctionType +
			"." + f.Func.Name +
			"." + f.Func.File +
			"." + f.Func.Package +
			":" + strconv.Itoa(f.Func.StartLine) +
			"-" + strconv.Itoa(f.Func.EndLine)
	}
	oldFuncMap := make(map[string]analyzer.LLMAnalysisResult)
	for _, f := range oldKG.Functions {
		oldFuncMap[funcKey(f)] = f
	}
	for _, f := range kg.Functions {
		oldFuncMap[funcKey(f)] = f // 新的覆盖旧的
	}
	mergedFuncs := make([]analyzer.LLMAnalysisResult, 0, len(oldFuncMap))
	for _, f := range oldFuncMap {
		mergedFuncs = append(mergedFuncs, f)
	}
	// 构造合并后的 KnowledgeGraph
	mergedKG := KnowledgeGraph{
		Functions: mergedFuncs,
		Calls:     oldKG.CalledBy,
		CalledBy:  oldKG.CalledBy,
		Packages:  oldKG.Packages,
		Externals: oldKG.Externals,
	}

	// 写回文件
	out, err := json.MarshalIndent(mergedKG, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0644)
}
