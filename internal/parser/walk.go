package parser

import (
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
		for _, fn := range funcs {
			cb(fn)
		}
		return nil
	})
}
