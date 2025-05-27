package parser

import (
	"database/sql"
	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"
	"os"
	"path/filepath"
	"strings"

	"github.com/kinglegendzzh/flashmemory/internal/utils"
)

var (
	SupportedLanguages = []string{
		".go",
		".py",
		".vue",
		".js", ".jsx",
		".ts", ".tsx",
		".java",
		".cpp", ".cc", ".cc", ".cxx", ".c++", ".hpp", ".h",
		".c",
		".rb",
		".php",
	}
)

// WalkAndParse 遍历 root，调用 DetectLang/NewParser/ParseFile 解析函数
// 仅当 .gitgo/code_index.db 同时存在时，才执行 SQL 查询并设置 fn.Scan，否则默认 Scan = false
func WalkAndParse(root string, cb func(info FunctionInfo)) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// 跳过exclude.json中指定的路径
			excludeFile := filepath.Join(root, ".gitgo", "exclude.json")
			jsonFile, err := utils.ReadJSONArrayFile(excludeFile)
			if err == nil {
				if utils.IsExcludedPath(path, jsonFile) {
					logs.Warnf("跳过指定目录: %s", path)
					return filepath.SkipDir
				}
			}
			// 跳过以点开头的隐藏目录
			if info.Name() != "." && info.Name() != ".." && strings.HasPrefix(info.Name(), ".") && info.Name() != "node_modules" {
				logs.Warnf("跳过目录: %s", info.Name())
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		if !utils.Contains(SupportedLanguages, ext) {
			return nil
		}
		lang := DetectLang(path)
		if lang == "" {
			return nil
		}
		p := NewParserNoLLM(lang)
		funcs, err := p.ParseFile(path)
		if err != nil {
			logs.Errorf("Error parsing file %s: %v\n", path, err)
			return nil
		}

		// 准备 .gitgo 和数据库路径
		idxDir := filepath.Join(root, ".gitgo")
		dbPath := filepath.Join(idxDir, "code_index.db")

		// 检查 .gitgo 和 code_index.db 是否都存在
		needQuery := true
		if stat, err := os.Stat(idxDir); err != nil || !stat.IsDir() {
			needQuery = false
		} else if stat, err := os.Stat(dbPath); err != nil || stat.IsDir() {
			needQuery = false
		}

		if needQuery {
			// 打开数据库
			db, err := sql.Open("sqlite", dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			for _, fn := range funcs {
				logs.Tokenf("正在解析文件 %v\n", fn)
				// 将绝对路径 fn.File 转成相对于 root 的相对路径
				relPath, err := filepath.Rel(root, fn.File)
				if err != nil {
					// 转换失败时保留原始绝对路径
					relPath = fn.File
				}
				fn.File = filepath.ToSlash(relPath)

				// 查询是否已经扫描过
				query := "SELECT 1 FROM functions WHERE file = ? AND name = ? LIMIT 1"
				row := db.QueryRow(query, fn.File, fn.Name)
				var tmp int
				if err := row.Scan(&tmp); err == nil {
					fn.Scan = true
				} else {
					fn.Scan = false
				}
				cb(fn)
			}
		} else {
			// 数据库不存在，全部标记为未扫描
			for _, fn := range funcs {
				logs.Tokenf("正在解析文件 %v\n", fn)
				// 同样转换文件路径，保证 fn.File 统一为相对路径
				if relPath, err := filepath.Rel(root, fn.File); err == nil {
					fn.File = filepath.ToSlash(relPath)
				}
				fn.Scan = false
				cb(fn)
			}
		}
		return nil
	})
}
