package parser

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/kinglegendzzh/flashmemory/cmd/common"
	"github.com/kinglegendzzh/flashmemory/config"
	"github.com/kinglegendzzh/flashmemory/internal/cloud"
	"github.com/kinglegendzzh/flashmemory/internal/utils"
	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

// LLMParser uses a language model to enhance code understanding
type LLMParser struct {
	Lang  string
	NoLLM bool
}

// NewLLMParser creates a new LLM-enhanced parser with a base parser for initial parsing
func NewLLMParser(lang string, NoLLM bool) *LLMParser {
	return &LLMParser{
		Lang:  lang,
		NoLLM: NoLLM,
	}
}

// ParseFile 支持文件或目录，批量解析并增强
func (lp *LLMParser) ParseFile(path string) ([]FunctionInfo, error) {
	logs.Infof("LLMParser 正在批量解析并增强: %s", path)
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("加载配置文件失败: %w", err)
	}
	// 1. 收集所有待解析文件路径
	var files []string
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("路径错误: %w", err)
	}
	if info.IsDir() {
		err = filepath.Walk(path, func(p string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !fi.IsDir() && isSupportedFile(p) {
				files = append(files, p)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("遍历目录失败: %w", err)
		}
	} else {
		if isSupportedFile(path) {
			files = append(files, path)
		}
	}

	// 2. 用 BaseParser 收集初步的 FunctionInfo BUG！！
	var allFuncs []FunctionInfo
	for _, f := range files {
		baseFuncs := FunctionInfo{}
		baseFuncs.File = f
		allFuncs = append(allFuncs, baseFuncs)
		//baseFuncs, err := lp.BaseParser.ParseFile(f)
		//if err != nil {
		//	return nil, fmt.Errorf("基础解析失败 (%s): %w", f, err)
		//}
		//// 填充文件路径字段
		//for i := range baseFuncs {
		//	baseFuncs[i].File = f
		//}
		//allFuncs = append(allFuncs, baseFuncs...)
	}

	// 3. 按文件分组，逐文件增强
	enhancedFuncs := make([]FunctionInfo, 0, len(allFuncs))
	funcsByFile := make(map[string][]FunctionInfo)
	for _, fn := range allFuncs {
		funcsByFile[fn.File] = append(funcsByFile[fn.File], fn)
	}

	for file, funcs := range funcsByFile {
		// 读取文件内容并按行拆分
		data, err := ioutil.ReadFile(file)
		if err != nil {
			// 读取失败时跳过增强，直接使用基础信息
			enhancedFuncs = append(enhancedFuncs, funcs...)
			continue
		}
		lines := strings.Split(string(data), "\n")
		codeChunks := splitIntoChunks(lines, cfg.ParserCodeLineLimit)

		// 对每个片段批量调用 LLM，解析并更新 funcs
		for _, chunk := range codeChunks {
			segment := strings.Join(chunk.Lines, "\n")
			ctx := context.Background()
			var resp string
			if !lp.NoLLM {
				resp, err = cloud.ParserInvoke(ctx, cfg, segment)
				if err != nil {
					// 本段增强失败时忽略，继续下一个片段
					logs.Warnf("LLM enhance failed (%s): %v", file, err)
					continue
				}
			}
			// 解析 LLM 返回，并合并到 funcs
			updated := lp.parseLLMResponse(resp, funcs, path, chunk.StartLine, chunk.EndLine, chunk.LineCount)
			funcs = mergeFunctionInfos(funcs, updated)
		}

		enhancedFuncs = append(enhancedFuncs, funcs...)
	}

	return enhancedFuncs, nil
}

// isSupportedFile 判断文件是否为我们要解析的代码文件
func isSupportedFile(path string) bool {
	ext := filepath.Ext(path)
	for _, e := range common.SupportedLanguages {
		if e == ext {
			return true
		}
	}
	return false
}

// CodeChunk 表示一个代码片段以及它在原文件中的位置
type CodeChunk struct {
	Lines     []string // 该片段的所有行
	StartLine int      // 在原文件中的起始行号（从 1 开始）
	EndLine   int      // 在原文件中的结束行号
	LineCount int      // 该片段的行数（EndLine-StartLine+1）
}

// splitIntoChunks 按行数将内容拆分成若干片段，并记录每段的位置信息
func splitIntoChunks(lines []string, chunkSize int) []CodeChunk {
	if chunkSize <= 0 {
		chunkSize = len(lines) // 直接一片全给它
	}
	var chunks []CodeChunk
	total := len(lines)
	for i := 0; i < total; i += chunkSize {
		// Go 的 slice 索引是 0-based，但我们希望 StartLine/EndLine 是 1-based
		startIdx := i
		endIdx := i + chunkSize
		if endIdx > total {
			endIdx = total
		}
		chunks = append(chunks, CodeChunk{
			Lines:     lines[startIdx:endIdx],
			StartLine: startIdx + 1,
			EndLine:   endIdx,
			LineCount: endIdx - startIdx,
		})
	}
	return chunks
}

// getNames 从 FunctionInfo 列表中提取所有函数名
func getNames(funcs []FunctionInfo) []string {
	names := make([]string, len(funcs))
	for i, fn := range funcs {
		names[i] = fn.Name
	}
	return names
}

// parseLLMResponse 将 LLM 返回的 JSON 或文本解析成 []FunctionInfo
func (lp *LLMParser) parseLLMResponse(response string, base []FunctionInfo, path string, startLine, endLine, lineCount int) []FunctionInfo {
	// 1. 过滤掉非 JSON 部分，拿到干净的 JSON 字符串
	raw := utils.FilterJSONContent(response)

	// 2. 定义一个临时结构体用来反序列化
	type llmItem struct {
		FunctionName string `json:"function_name"`
		Description  string `json:"description"`
	}

	var items []llmItem
	// 3. 判断是单个对象还是数组，并反序列化
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "[") {
		// JSON 数组
		if err := json.Unmarshal([]byte(raw), &items); err != nil {
			logs.Warnf("LLM enhance failed: %v", err)
			base = append(base, FunctionInfo{
				Name:         path,
				Description:  raw,
				File:         path,
				FunctionType: "llm_parser",
				StartLine:    startLine,
				EndLine:      endLine,
				Lines:        lineCount,
			})
			return base
		}
		logs.Infof("LLM enhance array result: %v", items)
	} else {
		// 单个 JSON 对象
		var single llmItem
		if err := json.Unmarshal([]byte(raw), &single); err != nil {
			logs.Warnf("LLM enhance failed(solo): %v", err)
			base = append(base, FunctionInfo{
				Name:         path,
				Description:  raw,
				File:         path,
				FunctionType: "llm_parser",
				StartLine:    startLine,
				EndLine:      endLine,
				Lines:        lineCount,
			})
			return base
		}
		logs.Infof("LLM enhance solo result: %v", single)
		items = []llmItem{single}
	}

	// 4. 根据 function_name 把 description 填写至 FunctionInfo
	for _, it := range items {
		base = append(base, FunctionInfo{
			Name:         it.FunctionName,
			Description:  it.Description,
			File:         path,
			FunctionType: "llm_parser",
			StartLine:    startLine,
			EndLine:      endLine,
			Lines:        lineCount,
		})
	}
	logs.Infof("LLM enhance result: %v", base)
	return base
}

// mergeFunctionInfos 将更新后的信息合并到原列表中
func mergeFunctionInfos(orig, updated []FunctionInfo) []FunctionInfo {
	out := make([]FunctionInfo, len(orig))
	copy(out, orig)
	// 按 Name+File 匹配并更新
	for i, o := range out {
		for _, u := range updated {
			if o.Name == u.Name && o.File == u.File {
				out[i] = u
				break
			}
		}
	}
	return out
}
