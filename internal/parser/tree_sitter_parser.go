package parser

import (
	"fmt"
	"io/ioutil"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	bash "github.com/smacker/go-tree-sitter/bash"
	c "github.com/smacker/go-tree-sitter/c"
	cpp "github.com/smacker/go-tree-sitter/cpp"
	elixir "github.com/smacker/go-tree-sitter/elixir"
	gost "github.com/smacker/go-tree-sitter/golang"
	java "github.com/smacker/go-tree-sitter/java"
	js "github.com/smacker/go-tree-sitter/javascript"
	php "github.com/smacker/go-tree-sitter/php"
	py "github.com/smacker/go-tree-sitter/python"
	ruby "github.com/smacker/go-tree-sitter/ruby"
	rust "github.com/smacker/go-tree-sitter/rust"

	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"
)

// TreeSitterParser 使用 Tree-sitter 实现多语言代码解析
type TreeSitterParser struct {
	Lang  string
	Debug bool // 控制调试日志输出
}

// debugLog 输出调试日志（当 Debug 为 true 时）
func (tp *TreeSitterParser) debugLog(format string, a ...interface{}) {
	if tp.Debug {
		logs.Infof(format, a...)
	}
}

// ParseFile 使用 Tree-sitter 解析指定文件，提取函数信息
func (tp *TreeSitterParser) ParseFile(path string) ([]FunctionInfo, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var language *sitter.Language
	switch tp.Lang {
	case "go":
		language = gost.GetLanguage()
	case "python":
		language = py.GetLanguage()
	case "javascript":
		language = js.GetLanguage()
	case "c":
		language = c.GetLanguage()
	case "cpp":
		language = cpp.GetLanguage()
	case "java":
		language = java.GetLanguage()
	case "ruby":
		language = ruby.GetLanguage()
	case "rust":
		language = rust.GetLanguage()
	case "bash":
		language = bash.GetLanguage()
	case "elixir":
		language = elixir.GetLanguage()
	case "php":
		language = php.GetLanguage()
	case "typescript":
		// TypeScript 语法和 JavaScript 类似
		language = js.GetLanguage()
	case "vue":
		// TypeScript 语法和 JavaScript 类似
		language = js.GetLanguage()
	default:
		return nil, fmt.Errorf("Tree-sitter 不支持该语言：%s", tp.Lang)
	}

	parserTS := sitter.NewParser()
	parserTS.SetLanguage(language)
	// 修正：传入 nil 作为旧语法树，data 作为源码内容
	tree := parserTS.Parse(nil, data)
	rootNode := tree.RootNode()

	// 尝试提取 package 名称
	pkgName := tp.extractPackageName(rootNode, data, language)
	tp.debugLog("Extracted Package/Namespace for %s: '%s'", tp.Lang, pkgName)

	// 提取导入信息
	imports := tp.extractImports(rootNode, data, language)

	// 定义查询以提取函数定义（统一使用捕获名 func_decl）
	queryStr := getTreeSitterFunctionQuery(tp.Lang)
	if queryStr == "" {
		return nil, fmt.Errorf("未定义该语言的 Tree-sitter 函数查询: %s", tp.Lang)
	}
	query, err := sitter.NewQuery([]byte(queryStr), language)
	if err != nil {
		return nil, err
	}

	qc := sitter.NewQueryCursor()
	qc.Exec(query, rootNode)

	funcs := []FunctionInfo{}
	for {
		match, ok := qc.NextMatch()
		if !ok {
			break
		}
		for _, capture := range match.Captures {
			captureName := query.CaptureNameForId(capture.Index)
			if captureName != "func_decl" {
				continue
			}
			node := capture.Node

			var funcName string
			if tp.Lang == "cpp" || tp.Lang == "c" {
				if decl := node.ChildByFieldName("declarator"); decl != nil {
					var ids []string
					var collect func(n *sitter.Node)
					collect = func(n *sitter.Node) {
						switch n.Type() {
						case "parameter_list", "template_parameter_list", "trailing_return_type":
							return // 跳过参数列表、模板参数、尾置返回类型
						case "operator_function_id":
							ids = append(ids, n.Content(data)) // C++ 运算符重载
							return
						case "identifier":
							ids = append(ids, n.Content(data))
						}
						for i := 0; i < int(n.ChildCount()); i++ {
							collect(n.Child(i))
						}
					}
					collect(decl)
					if len(ids) > 0 {
						funcName = ids[len(ids)-1] // 取最后一个 identifier 作为函数名
					}
				}
			} else {
				// 默认提取逻辑：先查找字段 "name"
				nameNode := node.ChildByFieldName("name")
				if nameNode != nil {
					funcName = nameNode.Content(data)
				} else if node.ChildCount() > 0 {
					funcName = node.Child(0).Content(data)
				}
			}
			if funcName == "" {
				continue
			}

			// 若存在 receiver 字段，则视为方法
			receiver := ""
			recvNode := node.ChildByFieldName("receiver")
			if recvNode != nil {
				receiver = recvNode.Content(data)
			}

			// 计算函数在源码中的起始行与结束行（行号从1开始）
			startLine := node.StartPoint().Row + 1
			endLine := node.EndPoint().Row + 1
			linesCount := endLine - startLine + 1

			// 提取函数内部的调用信息
			calls := tp.extractCalls(node, data, language)

			fn := FunctionInfo{
				Name:         funcName,
				Receiver:     receiver,
				File:         path,
				Package:      pkgName,
				Imports:      imports,
				Calls:        calls,
				Lines:        int(linesCount),
				StartLine:    int(startLine),
				EndLine:      int(endLine),
				FunctionType: "function",
			}
			if receiver != "" {
				fn.FunctionType = "method"
			}
			funcs = append(funcs, fn)
		}
	}
	return funcs, nil
}

// getTreeSitterFunctionQuery 根据语言返回对应的 Tree-sitter 查询，用于捕获函数定义
func getTreeSitterFunctionQuery(lang string) string {
	switch lang {
	case "go":
		return `(function_declaration) @func_decl
(method_declaration) @func_decl`
	case "python":
		return `(function_definition) @func_decl`
	case "javascript":
		return `(function_declaration) @func_decl`
	case "vue":
		// 对 Vue，取决于上面你用的是 js 还是 ts 语法树
		return `(function_declaration) @func_decl`
	case "c":
		return `(function_definition) @func_decl`
	case "cpp":
		return `(function_definition) @func_decl`
	case "java":
		return `(method_declaration) @func_decl`
	case "ruby":
		// 可能为 def 或 method 节点
		return `((def) @func_decl | (method) @func_decl)`
	case "rust":
		return `(function_item) @func_decl`
	case "typescript":
		return `(function_declaration) @func_decl`
	case "bash":
		return `(function_definition) @func_decl`
	case "elixir":
		return `(function_definition) @func_decl`
	case "php":
		return `
            (function_definition) @func_decl
            (method_declaration)   @func_decl
        `
	default:
		return ""
	}
}

// extractImports 提取文件中的导入信息
func (tp *TreeSitterParser) extractImports(rootNode *sitter.Node, data []byte, language *sitter.Language) []string {
	imports := []string{}
	// 根据不同语言定义导入查询
	var importQueryStr string
	switch tp.Lang {
	case "go":
		importQueryStr = `(import_spec path: (interpreted_string_literal) @import_path)`
	case "python":
		importQueryStr = `(import_statement (dotted_name) @import_path)
(import_from_statement module_name: (dotted_name) @import_path)`
	case "javascript":
		importQueryStr = `(import_statement source: (string) @import_path)`
	case "typescript":
		importQueryStr = `(import_statement source: (string) @import_path)`
	case "java":
		importQueryStr = `(import_declaration (scoped_identifier) @import_path)`
	case "rust":
		importQueryStr = `(use_declaration (scoped_identifier) @import_path)`
	case "php":
		importQueryStr = `(namespace_use_clause (qualified_name) @import_path)`
	default:
		return imports // 对于不支持的语言，返回空列表
	}

	tp.debugLog("导入查询语句: %s", importQueryStr)
	importQuery, err := sitter.NewQuery([]byte(importQueryStr), language)
	if err != nil {
		tp.debugLog("创建导入查询失败: %v", err)
		return imports
	}

	qc := sitter.NewQueryCursor()
	qc.Exec(importQuery, rootNode)

	for {
		match, ok := qc.NextMatch()
		if !ok {
			break
		}

		for _, capture := range match.Captures {
			captureName := importQuery.CaptureNameForId(capture.Index)
			if captureName == "import_path" {
				importPath := capture.Node.Content(data)
				// 处理不同语言的导入路径格式
				if tp.Lang == "go" || tp.Lang == "javascript" || tp.Lang == "typescript" {
					// 去除引号
					importPath = strings.Trim(importPath, "\"`'")
				}
				imports = append(imports, importPath)
			}
		}
	}
	return imports
}

// extractCalls 提取函数内部的调用信息
func (tp *TreeSitterParser) extractCalls(funcNode *sitter.Node, data []byte, language *sitter.Language) []string {
	calls := []string{}

	if tp.Lang == "php" {
		tp.debugLog("PHP 函数调用调试：funcNode 语法树:\n%s", funcNode.String())
	}

	if tp.Lang == "go" {
		queries := []string{
			`(call_expression function: (identifier) @func_call)`,
			`(call_expression function: (selector_expression field: (field_identifier) @method_call))`,
		}

		for _, queryStr := range queries {
			tp.debugLog("Go 查询语句: %s", queryStr)
			callQuery, err := sitter.NewQuery([]byte(queryStr), language)
			if err != nil {
				tp.debugLog("创建 Go 查询失败: %v", err)
				continue
			}
			qc := sitter.NewQueryCursor()
			qc.Exec(callQuery, funcNode)
			for {
				match, ok := qc.NextMatch()
				if !ok {
					break
				}
				for _, capture := range match.Captures {
					captureName := callQuery.CaptureNameForId(capture.Index)
					if captureName == "func_call" || captureName == "method_call" {
						callName := capture.Node.Content(data)
						if callName != "" {
							calls = append(calls, callName)
						}
					}
				}
			}
		}
		return filterBuiltInCalls(calls)
	}

	if tp.Lang == "php" {
		queries := []string{
			`(function_call_expression function: (name) @func_call)`,
			`(function_call_expression function: (qualified_name) @func_call)`,
			`(member_call_expression name: (name) @method_call)`,
		}

		for _, queryStr := range queries {
			tp.debugLog("PHP 查询语句: %s", queryStr)
			callQuery, err := sitter.NewQuery([]byte(queryStr), language)
			if err != nil {
				tp.debugLog("创建 PHP 查询失败: %v", err)
				continue
			}
			qc := sitter.NewQueryCursor()
			qc.Exec(callQuery, funcNode)
			for {
				match, ok := qc.NextMatch()
				if !ok {
					break
				}
				tp.debugLog("PHP 匹配结果: %v", match)
				for _, capture := range match.Captures {
					captureName := callQuery.CaptureNameForId(capture.Index)
					tp.debugLog("PHP 捕获: %s -> %s", captureName, capture.Node.Content(data))
					if captureName == "func_call" || captureName == "method_call" {
						callName := capture.Node.Content(data)
						if callName != "" {
							calls = append(calls, callName)
						}
					}
				}
			}
		}
		return filterBuiltInCalls(calls)
	}

	var callQueryStr string
	switch tp.Lang {
	case "python":
		callQueryStr = `(call function: [(identifier) @func_call (attribute attribute: (identifier) @method_call)])`
	case "javascript", "typescript":
		callQueryStr = `(call_expression function: [(identifier) @func_call (member_expression property: (property_identifier) @method_call)])`
	case "java":
		callQueryStr = `(method_invocation name: (identifier) @method_call)`
	case "c", "cpp":
		callQueryStr = `(call_expression function: (identifier) @func_call)`
	case "rust":
		callQueryStr = `(call_expression function: [(identifier) @func_call (field_expression field: (field_identifier) @method_call)])`
	default:
		return calls
	}

	//tp.debugLog("其它语言查询语句: %s", callQueryStr)
	callQuery, err := sitter.NewQuery([]byte(callQueryStr), language)
	if err != nil {
		tp.debugLog("创建其它语言查询失败: %v", err)
		return calls
	}

	qc := sitter.NewQueryCursor()
	qc.Exec(callQuery, funcNode)

	for {
		match, ok := qc.NextMatch()
		if !ok {
			break
		}

		for _, capture := range match.Captures {
			captureName := callQuery.CaptureNameForId(capture.Index)
			if captureName == "func_call" || captureName == "method_call" {
				callName := capture.Node.Content(data)
				if callName != "" {
					calls = append(calls, callName)
				}
			}
		}
	}

	return filterBuiltInCalls(calls)
}

// --- NEW Method ---
// extractPackageName 提取文件中的包或命名空间信息
func (tp *TreeSitterParser) extractPackageName(rootNode *sitter.Node, data []byte, language *sitter.Language) string {
	var pkgQueryStr string

	switch tp.Lang {
	case "go":
		// Example: package main
		pkgQueryStr = fmt.Sprintf(`(package_clause) @package`)
	case "java":
		// Example: package com.example.myapp;
		// Handles simple and scoped identifiers
		pkgQueryStr = fmt.Sprintf(`(package_declaration (scoped_identifier) @package)`)
	case "cpp":
		// Example: namespace MyNamespace { ... }
		// Extracts the *first* namespace definition found. Might not always be the primary one.
		// 只匹配 namespace_definition 的 name 字段，捕获 identifier 或者 nested_name_specifier
		pkgQueryStr = `(namespace_definition 
                          name: (_) @namespace
                       )`
	case "php":
		// Example: namespace App\Http\Controllers;
		pkgQueryStr = fmt.Sprintf(`(namespace_definition) @namespace`) // Name node contains identifier(s)
	case "ruby":
		// Example: module MyModule ... end
		// Extracts the *first* module definition. Classes can also act as namespaces.
		pkgQueryStr = fmt.Sprintf(`(module (constant) @namespace)`)
	case "elixir":
		// Example: defmodule MyApp.Web.MyController do ... end
		// Extracts the main module defined with defmodule
		// Alias usually holds the module name.
		pkgQueryStr = "(module_definition name: (alias) @namespace)"
	// --- Languages where package isn't typically declared in file syntax ---
	case "python":
		// Package is defined by directory structure and __init__.py
		return "" // Cannot reliably extract from single file syntax
	case "javascript", "typescript":
		pkgQueryStr = `
			(import_statement source: (string) @import_path) |
			(export_statement) @export
		`
	case "rust":
		// Crate name defined in Cargo.toml, modules defined by `mod` or file structure
		pkgQueryStr = "(mod_item) @module"
	case "c", "bash":
		// No built-in package/namespace concept in the same way
		return ""
	default:
		tp.debugLog("Package/namespace extraction not implemented for language: %s", tp.Lang)
		return ""
	}

	// Execute the query if one was defined
	if pkgQueryStr == "" {
		return ""
	}

	tp.debugLog("Package query for %s: %s", tp.Lang, pkgQueryStr)
	pkgQuery, err := sitter.NewQuery([]byte(pkgQueryStr), language)
	if err != nil {
		tp.debugLog("Error creating package query for %s: %v", tp.Lang, err)
		return "" // Query failed
	}

	qc := sitter.NewQueryCursor()
	qc.Exec(pkgQuery, rootNode)

	// Find the first match for the package/namespace declaration
	match, ok := qc.NextMatch()
	if !ok {
		tp.debugLog("No package/namespace declaration found for %s", tp.Lang)
		return "" // No match found
	}

	for _, capture := range match.Captures {
		// Check if this capture is the one we named (e.g., "pkg_name")
		packageName := capture.Node.Content(data)
		// Optional: Clean up C++ nested namespace if needed, e.g., `A::B` -> `A::B`
		// Optional: Clean up PHP namespace separator `\` if desired, though raw name is often fine.
		tp.debugLog("Found package name: %s", packageName)
		return packageName
	}

	return "" // Match found, but the specific capture wasn't present (shouldn't usually happen with correct queries)
}

// filterBuiltInCalls 根据内置白名单过滤掉系统内置函数调用，保留用户自定义调用
func filterBuiltInCalls(calls []string) []string {
	// 内置白名单，涵盖全语言常见的系统内置函数（可根据需要扩展）
	builtinWhitelist := map[string]bool{
		// Go 内置函数
		"append":  true,
		"cap":     true,
		"close":   true,
		"complex": true,
		"copy":    true,
		"delete":  true,
		"imag":    true,
		"len":     true,
		"make":    true,
		"new":     true,
		"panic":   true,
		"print":   true,
		"println": true,
		// Python 内置函数
		"abs":         true,
		"all":         true,
		"any":         true,
		"bin":         true,
		"bool":        true,
		"bytearray":   true,
		"bytes":       true,
		"callable":    true,
		"chr":         true,
		"classmethod": true,
		"compile":     true,
		// "complex":     true,
		"delattr":    true,
		"dict":       true,
		"dir":        true,
		"divmod":     true,
		"enumerate":  true,
		"eval":       true,
		"exec":       true,
		"filter":     true,
		"float":      true,
		"format":     true,
		"frozenset":  true,
		"getattr":    true,
		"globals":    true,
		"hasattr":    true,
		"hash":       true,
		"help":       true,
		"hex":        true,
		"id":         true,
		"input":      true,
		"int":        true,
		"isinstance": true,
		"issubclass": true,
		"iter":       true,
		"list":       true,
		"locals":     true,
		"map":        true,
		"max":        true,
		"memoryview": true,
		"min":        true,
		"next":       true,
		"object":     true,
		"oct":        true,
		"open":       true,
		"ord":        true,
		"pow":        true,
		// "print":       true,
		"property":     true,
		"range":        true,
		"repr":         true,
		"reversed":     true,
		"round":        true,
		"set":          true,
		"setattr":      true,
		"slice":        true,
		"sorted":       true,
		"staticmethod": true,
		"str":          true,
		"sum":          true,
		"super":        true,
		"tuple":        true,
		"type":         true,
		"vars":         true,
		"zip":          true,
		"__import__":   true,
		// JavaScript/TypeScript 内置
		"Array": true,
		"Date":  true,
		// "eval":           true,
		"function":       true,
		"hasOwnProperty": true,
		"Infinity":       true,
		"isFinite":       true,
		"isNaN":          true,
		"isPrototypeOf":  true,
		"length":         true,
		"Math":           true,
		"NaN":            true,
		"Number":         true,
		"Object":         true,
		"prototype":      true,
		"String":         true,
		"undefined":      true,
		"valueOf":        true,
		// 新增常用 JS 数组方法
		"push":    true,
		"pop":     true,
		"shift":   true,
		"unshift": true,
		"concat":  true,
		"join":    true,
		//"slice":   true,
		"splice": true,
		//"map":     true,
		//"filter":  true,
		"reduce":  true,
		"forEach": true,

		// 新增常用 JS 字符串方法
		"split":       true,
		"replace":     true,
		"indexOf":     true,
		"includes":    true,
		"substr":      true,
		"substring":   true,
		"toLowerCase": true,
		"toUpperCase": true,
		"trim":        true,
		"charAt":      true,
		"charCodeAt":  true,
		// Java 内置（java.lang 包）
		// "Math":      true,
		"System": true,
		// "String":    true,
		"Integer":   true,
		"Double":    true,
		"Float":     true,
		"Boolean":   true,
		"Character": true,
		//"Math":   true,
		//"String": true,
		// C/C++ 内置
		"printf":  true,
		"scanf":   true,
		"malloc":  true,
		"free":    true,
		"calloc":  true,
		"realloc": true,
		"memcpy":  true,
		"memset":  true,
		"strcpy":  true,
		"strlen":  true,
		"fopen":   true,
		"fclose":  true,
		//"exit":    true,
		"abort": true,
		// Rust 内置
		// "println": true,
		// "format":  true,
		//"panic":   true,
		// PHP 内置
		"echo": true,
		// "print": true,
		"isset": true,
		"empty": true,
		"die":   true,
		"exit":  true,
		//"print":        true,
		"include":      true,
		"include_once": true,
		//"require":      true,
		"require_once": true,
		//"printf":       true,
		"sprintf":  true,
		"var_dump": true,
		"print_r":  true,
		//"strlen":       true,
		"count": true,

		// Ruby 内置
		"puts": true,
		//"print":   true,
		"p":       true,
		"require": true,
		"load":    true,
		//"eval":    true,
		//"exec":    true,
		"system": true,
		//"abort":   true,
		//"exit":    true,

	}

	var filtered []string
	for _, call := range calls {
		if !builtinWhitelist[call] {
			filtered = append(filtered, call)
		}
	}
	return filtered
}
