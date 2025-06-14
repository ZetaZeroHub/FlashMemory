package parser

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/kinglegendzzh/flashmemory/cmd/common"
	"github.com/kinglegendzzh/flashmemory/config"
	"github.com/kinglegendzzh/flashmemory/internal/cloud"
	"github.com/kinglegendzzh/flashmemory/internal/utils"
	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"
)

// LLMParser uses a language model to enhance code understanding
type LLMParser struct {
	Lang    string
	NoLLM   bool
	Db      *sql.DB
	ProjDir string
}

// NewLLMParser creates a new LLM-enhanced parser with a base parser for initial parsing
func NewLLMParser(lang string, NoLLM bool, db *sql.DB, projDir string) *LLMParser {
	return &LLMParser{
		Lang:    lang,
		NoLLM:   NoLLM,
		Db:      db,
		ProjDir: projDir,
	}
}

// ParseFile 支持文件或目录，批量按文件分块调用 LLM 直接生成 FunctionInfo
func (lp *LLMParser) ParseFile(path string) ([]FunctionInfo, error) {
	//sys := runtime.GOOS
	//if sys == "windows" {
	//	logs.Warnf("LLMParser.ParseFile 不支持的操作系统，已忽略: %s", sys)
	//	return nil, nil
	//}
	logs.Infof("LLMParser 正在批量解析并增强: %s", path)

	// 1. 加载配置
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("加载配置文件失败: %w", err)
	}

	// 2. 收集所有待解析的文件路径
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
	} else if isSupportedFile(path) {
		files = append(files, path)
	}

	// 3. 逐文件读取、分块、调用 LLM 并收集结果
	var enhancedFuncs []FunctionInfo
	for _, file := range files {
		// 3.1 读取文件内容并按行拆分
		data, err := ioutil.ReadFile(file)
		if err != nil {
			logs.Warnf("读取文件失败，跳过增强: %s: %v", file, err)
			continue
		}
		lines := strings.Split(string(data), "\n")
		codeChunks := splitIntoChunks(lines, cfg.ParserCodeLineLimit)

		// 本文件最终的 FunctionInfo 列表
		var funcs []FunctionInfo
		// 3.2 对每个代码片段调用 LLM，解析并累积到 funcs
		for i, chunk := range codeChunks {
			if i >= cfg.ParserCodeChunkLimit {
				logs.Warnf("单文件代码块数超出限制，跳过模型调用: %s [%d-%d]", file, chunk.StartLine, chunk.EndLine)
				break
			}
			if lp.NoLLM {
				// 主要用于listFunctions接口做统计
				funcs = append(funcs, FunctionInfo{
					Name:         path,
					File:         path,
					FunctionType: "llm_parser",
					StartLine:    chunk.StartLine,
					EndLine:      chunk.EndLine,
					Lines:        chunk.LineCount,
				})
				continue
			}
			if lp.Db != nil && lp.ProjDir != "" {
				if _, found := lp.loadStoredResult(path, lp.ProjDir); found {
					logs.Infof("已从数据库中找到结果，跳过模型调用: %s [%d-%d]", path, chunk.StartLine, chunk.EndLine)
					continue
				}
			}
			segment := strings.Join(chunk.Lines, "\n")
			ctx := context.Background()
			var resp string
			resp, err = cloud.ParserInvoke(ctx, cfg, segment)
			if err != nil {
				logs.Warnf("LLM enhance failed (%s [%d-%d]): %v",
					file, chunk.StartLine, chunk.EndLine, err)
				continue
			}

			// parseLLMResponse 返回本片段解析出的 []FunctionInfo
			updated := lp.parseLLMResponse(
				resp,
				funcs,
				file,
				chunk.StartLine,
				chunk.EndLine,
				chunk.LineCount,
			)
			funcs = append(funcs, updated...)
			if lp.Db != nil && lp.ProjDir != "" {
				for i := range updated {
					err := SaveSingleResultToDB(lp.Db, updated[i], lp.ProjDir)
					if err != nil {
						logs.Warnf("保存到数据库失败: %v", err)
						return nil, err
					}
				}
			}
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
	logs.Infof("LLM enhance result: %s", raw)
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

// SaveSingleResultToDB 将单个分析结果写入数据库。
// - 对 functions 表用 INSERT OR REPLACE，以便更新已有描述。
// - 对 calls 和 externals 表用 INSERT OR IGNORE，跳过已存在的行。
// - 不使用事务，每条结果直接写入。
func SaveSingleResultToDB(db *sql.DB, res FunctionInfo, projDir string) error {
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
		filePath := res.File
		// 如果不是绝对路径，就把它视作相对于 projDir 的子路径
		if !filepath.IsAbs(filePath) {
			filePath = filepath.Join(projDir, filePath)
		}
		rel, err := filepath.Rel(projDir, filePath)
		if err != nil {
			logs.Errorf("%s, %s, 无法将文件路径转换为相对路径: %v", projDir, res.File, err)
		} else {
			res.File = filepath.ToSlash(rel)
			logs.Infof("[DEBUG][DB] 存储文件路径为: %s", res.File)
		}
	}

	// 3. 插入 functions
	if _, err := funcStmt.Exec(
		res.Name,
		res.Package,
		res.File,
		res.Description,
		res.StartLine,
		res.EndLine,
		res.FunctionType,
	); err != nil {
		return fmt.Errorf("functions 写入失败: %w", err)
	}

	return nil
}

func (lp *LLMParser) loadStoredResult(path string, projDir string) ([]FunctionInfo, bool) {
	// 1. 如果没有数据库连接，直接返回
	if lp.Db == nil {
		logs.Warnf("[ERROR] 无数据库连接")
		return []FunctionInfo{}, false
	}

	// 如果传入了项目根目录，则转换为相对路径
	if projDir != "" {
		filePath := path
		// 如果不是绝对路径，就把它视作相对于 projDir 的子路径
		if !filepath.IsAbs(filePath) {
			filePath = filepath.Join(projDir, filePath)
		}
		rel, err := filepath.Rel(projDir, filePath)
		if err != nil {
			logs.Warnf("[WARN] 无法计算相对路径: %v, 使用原始路径", err)
		} else {
			path = filepath.ToSlash(rel)
			logs.Infof("[DEBUG][DB] 存储文件路径为: %s", path)
		}
	}

	// 2. 查询可能存在的多条记录
	query := `
        SELECT name, file, description, start_line, end_line, function_type
        FROM functions
        WHERE file = ?
          AND description IS NOT NULL
          AND description != ''
          AND name != ''
          AND name IS NOT NULL
          AND function_type = 'llm_parser'
    `
	rows, err := lp.Db.Query(query, path)
	if err != nil {
		logs.Errorf("[ERROR] 查询已存结果失败: %v", err)
		return []FunctionInfo{}, false
	}
	defer rows.Close()

	var results []FunctionInfo
	for rows.Next() {
		var dto struct {
			Name         string `json:"name"`
			File         string `json:"file"`
			Description  string `json:"description"`
			StartLine    int    `json:"start_line"`
			EndLine      int    `json:"end_line"`
			FunctionType string `json:"function_type"`
		}
		if err := rows.Scan(&dto.Name, &dto.File, &dto.Description, &dto.StartLine, &dto.EndLine, &dto.FunctionType); err != nil {
			logs.Warnf("[WARN] 读取一条记录失败: %v", err)
			continue
		}

		logs.Infof("[DEBUG] 读取已存结果: %s, desc: %s (lines %d-%d)", dto.File, dto.Description, dto.StartLine, dto.EndLine)
		if dto.Description == "" {
			logs.Warnf("[WARN] 描述为空，暂且当作成功")
		}

		fi := FunctionInfo{
			Name:         dto.Name,
			File:         dto.File,
			Description:  dto.Description,
			StartLine:    dto.StartLine,
			EndLine:      dto.EndLine,
			FunctionType: dto.FunctionType,
		}
		results = append(results, fi)
	}
	if err := rows.Err(); err != nil {
		logs.Errorf("[ERROR] 遍历查询结果时出错: %v", err)
	}

	if len(results) == 0 {
		return []FunctionInfo{}, false
	}
	return results, true
}
