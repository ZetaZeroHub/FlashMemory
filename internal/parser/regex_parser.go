package parser

import (
	"io/ioutil"
	"path/filepath"
	"strings"
)

// --- Implementation: Regex-based Parser (simplified, for non-Go code) ---

type RegexParser struct {
	Lang string
}

// ParseFile for RegexParser does a simplistic parse based on language-specific patterns.
func (rp *RegexParser) ParseFile(path string) ([]FunctionInfo, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	content := string(data)
	lines := strings.Split(content, "\n")
	funcs := []FunctionInfo{}
	pkgName := ""

	// 设置 package 名称（仅对部分语言有效）
	switch rp.Lang {
	case "python", "javascript", "typescript":
		pkgName = filepath.Base(filepath.Dir(path))
	case "java":
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "package ") {
				pkgName = strings.TrimSuffix(strings.TrimPrefix(line, "package "), ";")
				break
			}
		}
	case "cpp", "c":
		pkgName = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "namespace ") {
				parts := strings.Fields(line)
				if len(parts) > 1 {
					pkgName = strings.TrimSuffix(parts[1], "{")
					break
				}
			}
		}
	}

	imports := []string{}
	// 简单扫描提取 import/include 信息
	for _, line := range lines {
		line = strings.TrimSpace(line)
		switch rp.Lang {
		case "python":
			if strings.HasPrefix(line, "import ") || strings.HasPrefix(line, "from ") {
				imports = append(imports, line)
			}
		case "javascript", "typescript":
			if strings.HasPrefix(line, "import ") || strings.HasPrefix(line, "require(") {
				imports = append(imports, line)
			}
		case "java":
			if strings.HasPrefix(line, "import ") {
				imports = append(imports, strings.TrimSuffix(strings.TrimPrefix(line, "import "), ";"))
			}
		case "cpp", "c":
			if strings.HasPrefix(line, "#include ") {
				imports = append(imports, line)
			}
		case "ruby":
			if strings.HasPrefix(line, "require ") || strings.HasPrefix(line, "load ") {
				imports = append(imports, line)
			}
		case "php":
			if strings.HasPrefix(line, "require") || strings.HasPrefix(line, "include") {
				imports = append(imports, line)
			}
		}
	}

	// 逐行扫描提取函数定义，根据不同语言匹配不同模式
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch rp.Lang {
		case "python":
			if strings.HasPrefix(trimmed, "def ") {
				name := strings.SplitN(strings.TrimPrefix(trimmed, "def "), "(", 2)[0]
				fn := FunctionInfo{
					Name:    name,
					File:    path,
					Package: pkgName,
					Imports: imports,
					Calls:   []string{},
					Lines:   countFunctionLines(lines, i, "python"),
				}
				funcs = append(funcs, fn)
			}
		case "javascript", "typescript":
			if strings.Contains(trimmed, "function") || strings.Contains(trimmed, "=>") {
				var name string
				if strings.HasPrefix(trimmed, "function ") {
					parts := strings.SplitN(strings.TrimPrefix(trimmed, "function "), "(", 2)
					if len(parts) > 0 {
						name = strings.TrimSpace(parts[0])
					}
				} else if strings.Contains(trimmed, " = function") {
					parts := strings.SplitN(trimmed, " = function", 2)
					if len(parts) > 0 {
						name = strings.TrimSpace(strings.TrimPrefix(parts[0], "const "))
						name = strings.TrimPrefix(name, "let ")
						name = strings.TrimPrefix(name, "var ")
					}
				} else if strings.Contains(trimmed, " => ") {
					parts := strings.SplitN(trimmed, " = ", 2)
					if len(parts) > 0 {
						name = strings.TrimSpace(strings.TrimPrefix(parts[0], "const "))
						name = strings.TrimPrefix(name, "let ")
						name = strings.TrimPrefix(name, "var ")
					}
				}
				if name != "" {
					fn := FunctionInfo{
						Name:    name,
						File:    path,
						Package: pkgName,
						Imports: imports,
						Calls:   []string{},
						Lines:   countFunctionLines(lines, i, "javascript"),
					}
					funcs = append(funcs, fn)
				}
			}
		case "java":
			if (strings.Contains(trimmed, "public ") || strings.Contains(trimmed, "private ") ||
				strings.Contains(trimmed, "protected ")) && strings.Contains(trimmed, "(") &&
				!strings.HasPrefix(trimmed, "//") && !strings.Contains(trimmed, ";") {
				parts := strings.Split(trimmed, "(")
				if len(parts) > 0 {
					nameParts := strings.Fields(parts[0])
					if len(nameParts) > 0 {
						name := nameParts[len(nameParts)-1]
						fn := FunctionInfo{
							Name:    name,
							File:    path,
							Package: pkgName,
							Imports: imports,
							Calls:   []string{},
							Lines:   countFunctionLines(lines, i, "java"),
						}
						funcs = append(funcs, fn)
					}
				}
			}
		case "cpp", "c":
			if strings.Contains(trimmed, "(") && strings.Contains(trimmed, ")") &&
				(strings.HasSuffix(trimmed, "{") || (i+1 < len(lines) && strings.TrimSpace(lines[i+1]) == "{")) &&
				!strings.HasPrefix(trimmed, "//") && !strings.HasPrefix(trimmed, "#") &&
				!strings.Contains(trimmed, ";") && !strings.HasPrefix(trimmed, "if") &&
				!strings.HasPrefix(trimmed, "for") && !strings.HasPrefix(trimmed, "while") {
				parts := strings.Split(trimmed, "(")
				if len(parts) > 0 {
					nameParts := strings.Fields(parts[0])
					if len(nameParts) > 0 {
						name := nameParts[len(nameParts)-1]
						if strings.Contains(name, "::") {
							nameParts = strings.Split(name, "::")
							name = nameParts[len(nameParts)-1]
						}
						fn := FunctionInfo{
							Name:    name,
							File:    path,
							Package: pkgName,
							Imports: imports,
							Calls:   []string{},
							Lines:   countFunctionLines(lines, i, "cpp"),
						}
						funcs = append(funcs, fn)
					}
				}
			}
		case "ruby":
			if strings.HasPrefix(trimmed, "def ") {
				// 例如 "def func_name" 或 "def self.func_name"
				words := strings.Fields(trimmed)
				if len(words) >= 2 {
					name := words[1]
					fn := FunctionInfo{
						Name:    name,
						File:    path,
						Package: pkgName,
						Imports: imports,
						Calls:   []string{},
						Lines:   countFunctionLines(lines, i, "ruby"),
					}
					funcs = append(funcs, fn)
				}
			}
		case "php":
			if strings.Contains(trimmed, "function ") && !strings.HasPrefix(trimmed, "//") {
				idx := strings.Index(trimmed, "function ")
				if idx != -1 {
					remaining := trimmed[idx+len("function "):]
					parts := strings.SplitN(remaining, "(", 2)
					if len(parts) > 0 {
						name := strings.TrimSpace(parts[0])
						fn := FunctionInfo{
							Name:    name,
							File:    path,
							Package: pkgName,
							Imports: imports,
							Calls:   []string{},
							Lines:   countFunctionLines(lines, i, "php"),
						}
						funcs = append(funcs, fn)
					}
				}
			}
		case "rust":
			if strings.HasPrefix(trimmed, "fn ") {
				name := strings.SplitN(strings.TrimPrefix(trimmed, "fn "), "(", 2)[0]
				fn := FunctionInfo{
					Name:    name,
					File:    path,
					Package: pkgName,
					Imports: imports,
					Calls:   []string{},
					Lines:   countFunctionLines(lines, i, "rust"),
				}
				funcs = append(funcs, fn)
			}
		case "bash":
			// 支持 "name() {" 或 "function name {" 形式
			if (strings.Contains(trimmed, "()") && strings.Contains(trimmed, "{")) ||
				strings.HasPrefix(trimmed, "function ") {
				var name string
				if strings.HasPrefix(trimmed, "function ") {
					parts := strings.Fields(trimmed)
					if len(parts) >= 2 {
						name = parts[1]
					}
				} else {
					parts := strings.Split(trimmed, "(")
					if len(parts) > 0 {
						name = strings.TrimSpace(parts[0])
					}
				}
				if name != "" {
					fn := FunctionInfo{
						Name:    name,
						File:    path,
						Package: pkgName,
						Imports: imports,
						Calls:   []string{},
						Lines:   countFunctionLines(lines, i, "bash"),
					}
					funcs = append(funcs, fn)
				}
			}
		case "elixir":
			// 支持 "def func_name" 和 "defp func_name"
			if strings.HasPrefix(trimmed, "def ") || strings.HasPrefix(trimmed, "defp ") {
				words := strings.Fields(trimmed)
				if len(words) >= 2 {
					name := words[1]
					// Elixir 的函数名可能带有括号或尾随冒号，这里简单处理
					name = strings.Trim(name, "(:")
					fn := FunctionInfo{
						Name:    name,
						File:    path,
						Package: pkgName,
						Imports: imports,
						Calls:   []string{},
						Lines:   countFunctionLines(lines, i, "elixir"),
					}
					funcs = append(funcs, fn)
				}
			}
		}
	}

	// 注意：这种简单的方式不能覆盖所有函数签名和类定义
	return funcs, nil
}
