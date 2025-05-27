package parser

import (
	"database/sql"
	"path/filepath"
	"strings"
)

// FunctionInfo 保存函数或方法的关键信息
type FunctionInfo struct {
	Name         string   // 函数/方法名，例如 "CalculateTax"
	Receiver     string   // 方法接收者，例如 "(u *User)"，如果不是方法则为空字符串
	Parameters   []string // 参数列表（仅用于信息展示）
	File         string   // 函数定义所在的文件路径
	Package      string   // 所属包或模块名
	Imports      []string // 文件中的导入列表（用于外部依赖分析）
	Calls        []string // 该函数调用的内部函数名列表
	Lines        int      // 函数的代码行数（LOC）
	StartLine    int      // 函数的起始行号
	EndLine      int      // 函数的结束行号
	FunctionType string   // 函数类型（例如 "method"方法、"function"函数、"constructor"构造函数等）
	Description  string
	CodeSnippet  string
	Scan         bool
}

// Parser is an interface to parse a file and extract functions and imports.
type Parser interface {
	ParseFile(path string) ([]FunctionInfo, error)
}

// DetectLang returns a simple language identifier based on file extension.
func DetectLang(path string) string {
	ext := filepath.Ext(path)
	ext = strings.ToLower(ext)
	switch ext {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".vue":
		return "javascript"
	case ".js", ".jsx":
		return "javascript"
	case ".ts", ".tsx":
		return "typescript"
	case ".java":
		return "java"
	case ".cpp", ".cc", ".cxx", ".c++", ".hpp", ".h":
		// 可将 C 和 C++ 区分开来，简单起见这里统一为 "cpp"
		return "cpp"
	case ".c":
		return "c"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	default:
		return ""
	}
}

// NewParser returns an appropriate Parser implementation for the given language.
func NewParser(lang string) Parser {
	switch lang {
	case "go", "python", "javascript", "typescript", "java", "cpp", "c", "h", "ruby", "rust", "bash", "elixir", "php":
		return &TreeSitterParser{Lang: lang, Debug: true}
	default:
		return &LLMParser{Lang: lang}
	}
}

func NewParserDb(lang string, db *sql.DB, projDir string) Parser {
	switch lang {
	case "go", "python", "javascript", "typescript", "java", "cpp", "c", "h", "ruby", "rust", "bash", "elixir", "php":
		return &TreeSitterParser{Lang: lang, Debug: true, Db: db, ProjDir: projDir}
	default:
		return &LLMParser{Lang: lang, Db: db, ProjDir: projDir}
	}
}

func NewParserNoLLM(lang string) Parser {
	switch lang {
	case "go", "python", "javascript", "typescript", "java", "cpp", "c", "h", "ruby", "rust", "bash", "elixir", "php":
		return &TreeSitterParser{Lang: lang, Debug: true, NoLLM: true}
	default:
		return &LLMParser{Lang: lang, NoLLM: true}
	}
}
