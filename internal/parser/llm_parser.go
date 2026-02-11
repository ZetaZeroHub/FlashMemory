package parser

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kinglegendzzh/flashmemory/cmd/common"
	"github.com/kinglegendzzh/flashmemory/config"
	"github.com/kinglegendzzh/flashmemory/internal/cloud"
	"github.com/kinglegendzzh/flashmemory/internal/utils"
	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"
)

// LLMParser uses a language model to enhance code understanding
type LLMParser struct {
	Lang     string
	NoLLM    bool
	Db       *sql.DB
	ProjDir  string
	DbWriter *utils.DbWriter // 数据库写入管理器
}

// NewLLMParser creates a new LLM-enhanced parser with a base parser for initial parsing
func NewLLMParser(lang string, NoLLM bool, db *sql.DB, projDir string) *LLMParser {
	var dbWriter *utils.DbWriter
	if db != nil {
		dbWriter = utils.NewDbWriter(db)
	}
	return &LLMParser{
		Lang:     lang,
		NoLLM:    NoLLM,
		Db:       db,
		ProjDir:  projDir,
		DbWriter: dbWriter,
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
			// 区块小于5行，不调用LLM
			if chunk.LineCount < 5 {
				logs.Warnf("单文件代码块数小于5行，跳过模型调用: %s [%d-%d]", file, chunk.StartLine, chunk.EndLine)
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
				// 传入当前代码块的行号范围，查询是否已存在与该范围有重叠的记录
				if results, found := lp.loadStoredResult(path, lp.ProjDir, chunk.StartLine, chunk.EndLine); found {
					logs.Infof("已从数据库中找到结果，跳过模型调用: %s [%d-%d]，找到 %d 条记录",
						path, chunk.StartLine, chunk.EndLine, len(results))
					// 将找到的结果添加到 funcs 中，避免重复解析
					funcs = append(funcs, results...)
					continue
				}
			}
			segment := strings.Join(chunk.Lines, "\n")
			ctx := context.Background()
			var resp string
			resp, err = cloud.ParserInvoke(ctx, cfg, segment)
			if err != nil {
				if common.IsLLMError(err) {
					logs.Errorf("LLM enhance failed (%s [%d-%d]): %v",
						file, chunk.StartLine, chunk.EndLine, err)
					return nil, err
				}
				logs.Warnf("LLM enhance failed (%s [%d-%d]): %v",
					file, chunk.StartLine, chunk.EndLine, err)
				continue
			}

			// parseLLMResponse 返回本片段解析出的 []FunctionInfo
			updated, err := lp.parseLLMResponse(
				resp,
				funcs,
				file,
				chunk.StartLine,
				chunk.EndLine,
				chunk.LineCount,
			)
			if err != nil {
				logs.Warnf("LLM enhance failed (%s [%d-%d]): %v",
					file, chunk.StartLine, chunk.EndLine, err)
				continue
			}
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
		if e == ext && !strings.HasSuffix(path, "__init__.py") {
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
func (lp *LLMParser) parseLLMResponse(response string, base []FunctionInfo, path string, startLine, endLine, lineCount int) ([]FunctionInfo, error) {
	// 1. 过滤掉非 JSON 部分，拿到干净的 JSON 字符串
	raw := utils.FilterJSONContent(response)
	// 用json.Indent让raw内容更易读并输出
	var prettyRaw bytes.Buffer
	if err := json.Indent(&prettyRaw, []byte(raw), "", "  "); err == nil {
		logs.Infof("LLM enhance result raw (pretty):\n%s", prettyRaw.String())
	} else {
		logs.Warnf("LLM enhance result raw (pretty)失败: %s", raw)
		return nil, fmt.Errorf("LLM enhance result raw (pretty)失败: %s", raw)
	}

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
			// base = append(base, FunctionInfo{
			// 	Name:         path,
			// 	Description:  raw,
			// 	File:         path,
			// 	FunctionType: "llm_parser",
			// 	StartLine:    startLine,
			// 	EndLine:      endLine,
			// 	Lines:        lineCount,
			// })
			return base, err
		}
		logs.Infof("LLM enhance array result: %+v", items)
	} else {
		// 单个 JSON 对象
		var single llmItem
		if err := json.Unmarshal([]byte(raw), &single); err != nil {
			logs.Warnf("LLM enhance failed(solo): %v", err)
			// base = append(base, FunctionInfo{
			// 	Name:         path,
			// 	Description:  raw,
			// 	File:         path,
			// 	FunctionType: "llm_parser",
			// 	StartLine:    startLine,
			// 	EndLine:      endLine,
			// 	Lines:        lineCount,
			// })
			return base, err
		}
		logs.Infof("LLM enhance solo result: %+v", single)
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

	// 如果模型返回空数组或空对象，也要保存一条记录以标记该代码块已处理
	if len(items) == 0 {
		logs.Infof("LLM返回空结果，保存占位记录以避免重复解析: %s [%d-%d]", path, startLine, endLine)
		base = append(base, FunctionInfo{
			Name:         "", // 函数名为空
			Description:  "", // 描述为空
			File:         path,
			FunctionType: "llm_parser",
			StartLine:    startLine,
			EndLine:      endLine,
			Lines:        lineCount,
		})
	}

	// 5. 对 base 中的对象进行英文名称正则检查
	var validBase []FunctionInfo
	englishNameRegex := regexp.MustCompile(`^[a-zA-Z0-9_]+$`)
	for _, info := range base {
		// 如果函数名为空（占位记录），直接加入有效列表
		if info.Name == "" {
			validBase = append(validBase, info)
		} else if englishNameRegex.MatchString(info.Name) {
			validBase = append(validBase, info)
		} else {
			logs.Warnf("Function name not in English, skipped: %s", info.Name)
		}
	}

	// 6. 对 base 中所有对象根据 name/file/function_type/start_line/end_line 进行去重
	deduped := make([]FunctionInfo, 0, len(validBase))
	seen := make(map[string]bool)

	for _, info := range validBase {
		// 创建唯一标识符
		key := fmt.Sprintf("%s|%s|%s|%d|%d",
			info.Name, info.File, info.FunctionType, info.StartLine, info.EndLine)

		if !seen[key] {
			seen[key] = true
			deduped = append(deduped, info)
		} else {
			logs.Infof("Duplicate function info skipped: %s", key)
		}
	}

	logs.Infof("LLM enhance result: original=%d, valid=%d, deduped=%d",
		len(base), len(validBase), len(deduped))
	return deduped, nil
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

// SaveSingleResultToDB 将单个解析结果写入数据库。
// - 对 functions 表用 INSERT OR REPLACE，以便更新已有描述。
// - 对 calls 和 externals 表用 INSERT OR IGNORE，跳过已存在的行。
// - 使用 DbWriter 进行串行化写入和自动重试。
func SaveSingleResultToDB(db *sql.DB, res FunctionInfo, projDir string) error {
	if db == nil {
		return fmt.Errorf("无数据库连接")
	}

	// 判断是否存在临时文件，如果不存在则抛出特殊异常码
	gitgoDir := filepath.Join(projDir, ".gitgo")
	tempFilePath := filepath.Join(gitgoDir, "indexing.temp")
	if _, err := os.Stat(tempFilePath); os.IsNotExist(err) {
		logs.Errorf("索引临时文件已被删除，终止扫描")
		return fmt.Errorf("索引临时文件已被删除，终止扫描")
	}

	// 创建临时 DbWriter 或使用已有的
	var dbWriter *utils.DbWriter
	if lp, ok := res.Parser.(*LLMParser); ok && lp != nil && lp.DbWriter != nil {
		// 如果是 LLMParser 的结果，使用其 DbWriter
		dbWriter = lp.DbWriter
		logs.Infof("使用 LLMParser 的 DbWriter 进行写入")
	} else {
		// 否则创建一个新的 DbWriter
		dbWriter = utils.NewDbWriter(db)
		defer dbWriter.Close()
		logs.Infof("创建新的 DbWriter 进行写入")
	}

	return saveSingleResultToDBWithWriter(dbWriter, res, projDir)
}

func saveSingleResultToDBWithWriter(dbWriter *utils.DbWriter, res FunctionInfo, projDir string) error {
	// 1. 处理文件路径
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

	// 2. 准备 SQL 语句
	funcSQL := `
		INSERT OR REPLACE INTO functions
		(name, package, file, description, start_line, end_line, function_type)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	// 3. 使用 DbWriter 写入 functions 表
	err := dbWriter.Write("function_insert", funcSQL,
		res.Name,
		res.Package,
		res.File,
		res.Description,
		res.StartLine,
		res.EndLine,
		res.FunctionType,
	)
	if err != nil {
		return fmt.Errorf("functions 写入失败: %w", err)
	}

	return nil
}

// loadStoredResult 从数据库加载指定文件和行号范围内的解析结果
// 如果 startLine 和 endLine 都为 0，则查询整个文件的所有结果
// 如果指定了行号范围，则只返回与该范围有重叠的结果
func (lp *LLMParser) loadStoredResult(path string, projDir string, startLine int, endLine int) ([]FunctionInfo, bool) {
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
	var query string
	var args []interface{}

	if startLine > 0 && endLine > 0 {
		// 如果指定了行号范围，则查询与该范围有重叠的记录
		// 重叠条件：(start_line <= endLine) AND (end_line >= startLine)
		query = `
			SELECT name, file, description, start_line, end_line, function_type
			FROM functions
			WHERE file = ?
			  AND function_type = 'llm_parser'
			  AND start_line <= ?
			  AND end_line >= ?
		`
		args = []interface{}{path, endLine, startLine}
		logs.Infof("[DEBUG] 查询行号范围内的结果: %s [%d-%d]", path, startLine, endLine)
	} else {
		// 如果没有指定行号范围，则查询整个文件的所有记录
		query = `
			SELECT name, file, description, start_line, end_line, function_type
			FROM functions
			WHERE file = ?
			  AND function_type = 'llm_parser'
		`
		args = []interface{}{path}
	}

	rows, err := lp.Db.Query(query, args...)
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

		logs.Infof("[DEBUG] 读取已存结果: %s, name: %s, desc: %s (lines %d-%d)", dto.File, dto.Name, dto.Description, dto.StartLine, dto.EndLine)
		if dto.Description == "" && dto.Name == "" {
			logs.Infof("[INFO] 找到占位记录，该代码块已处理过")
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
