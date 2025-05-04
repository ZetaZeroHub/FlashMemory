package parser

import (
	"database/sql"
	"os"
	"path/filepath"

	"github.com/kinglegendzzh/flashmemory/internal/utils"
)

var (
	SupportedLanguages = []string{
		".go",
		".py",
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
func WalkAndParse(root string, cb func(info FunctionInfo)) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// 跳过隐藏目录
			if info.Name() != "." && info.Name() != ".." && utils.IsHiddenDir(info.Name()) {
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
		p := NewParser(lang)
		funcs, err := p.ParseFile(path)
		if err != nil {
			// 可根据需求记录日志
			return nil
		}

		// Prepare database path
		idxDir := filepath.Join(root, ".gitgo")
		os.MkdirAll(idxDir, 0755)
		dbPath := filepath.Join(idxDir, "code_index.db")
		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			return err
		}
		defer db.Close()

		// Query the database for functions that match the file and name
		for _, fn := range funcs {
			//todo 预处理fn.File，fn.File当前为绝对路径（如‘/Users/apple/Public/openProject/flashmemory/internal/parser/tree_sitter_parser.go"’），通过root项目路径（绝对路径）截取该fn.File变成相对路径（如“internal/parser/tree_sitter_parser.go"）

			// Check if the function's file and name exist in the database
			query := "SELECT id, name, file FROM functions WHERE file = ? AND name = ?"
			rows, err := db.Query(query, fn.File, fn.Name)
			if err != nil {
				return err
			}
			defer rows.Close()

			// If the function exists in the database, mark it as Scan = true
			if rows.Next() {
				fn.Scan = true
			} else {
				fn.Scan = false
			}
			// Proceed to the callback function
			cb(fn)
		}
		return nil
	})
}
