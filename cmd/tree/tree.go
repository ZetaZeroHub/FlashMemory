package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
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

// getLanguage 根据传入的语言标识返回对应的 tree-sitter 语言对象
func getLanguage(lang string) *sitter.Language {
	switch strings.ToLower(lang) {
	case "bash":
		return bash.GetLanguage()
	case "c":
		return c.GetLanguage()
	case "cpp":
		return cpp.GetLanguage()
	case "elixir":
		return elixir.GetLanguage()
	case "go", "golang":
		return gost.GetLanguage()
	case "java":
		return java.GetLanguage()
	case "js", "javascript":
		return js.GetLanguage()
	case "php":
		return php.GetLanguage()
	case "py", "python":
		return py.GetLanguage()
	case "ruby":
		return ruby.GetLanguage()
	case "rust":
		return rust.GetLanguage()
	default:
		return nil
	}
}

// readCode 根据 -file 参数或者实时输入读取代码内容
func readCode(filePath string) (string, error) {
	if filePath != "" {
		data, err := ioutil.ReadFile(filePath)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	// 从标准输入读取代码（输入结束时输入单独一行 "EOF"）
	fmt.Println("Please enter the code (enter a separate line ending with 'EOF'):")
	scanner := bufio.NewScanner(os.Stdin)
	var sb strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "EOF" {
			break
		}
		sb.WriteString(line + "\n")
	}
	return sb.String(), nil
}

func main() {
	// 定义命令行参数：-file 指定代码文件；-lang 指定语言（默认 go）
	filePath := flag.String("file", "", "待解析的代码文件路径（可选）")
	langStr := flag.String("lang", "go", "代码语言，可选值：bash, c, cpp, elixir, go, java, js, php, py, ruby, rust")
	flag.Parse()

	logs.Infof("Start reading code...")
	code, err := readCode(*filePath)
	if err != nil {
		logs.Errorf("Reading code error:", err)
		os.Exit(1)
	}

	language := getLanguage(*langStr)
	if language == nil {
		logs.Errorf("Unsupported languages:", *langStr)
		os.Exit(1)
	}
	logs.Infof("Languages ​​used:", *langStr)

	// 初始化 tree-sitter 解析器，并设置为所选语言
	parser := sitter.NewParser()
	parser.SetLanguage(language)

	// 解析代码
	tree := parser.Parse(nil, []byte(code))
	if tree == nil {
		logs.Errorf("Code parsing failed")
		os.Exit(1)
	}
	rootNode := tree.RootNode()

	// 进入交互式查询循环
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("Please enter the tree-sitter query syntax (enter a single line ending with 'EOF'):")
		var queryInput strings.Builder
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				logs.Errorf("Reading input error:", err)
				break
			}
			line = strings.TrimSpace(line)
			if line == "EOF" {
				break
			}
			queryInput.WriteString(line + "\n")
		}
		queryInputStr := strings.TrimSpace(queryInput.String())
		if queryInputStr == "exit" {
			break
		}
		if queryInputStr == "" {
			continue
		}

		// 编译查询语句，例如：(function_declaration name: (identifier) @func_name)
		query, err := sitter.NewQuery([]byte(queryInputStr), language)
		if err != nil {
			logs.Errorf("Query syntax error:", err)
			continue
		}

		cursor := sitter.NewQueryCursor()
		cursor.Exec(query, rootNode)

		fmt.Println("Matching results:")
		for {
			match, ok := cursor.NextMatch()
			if !ok {
				break
			}
			for _, capture := range match.Captures {
				node := capture.Node
				text := string(code[node.StartByte():node.EndByte()])
				fmt.Printf("Capture: %s (type: %s) [bytes %d to %d]\n", text, node.Type(), node.StartByte(), node.EndByte())
			}
		}
		cursor.Close()
	}
}
