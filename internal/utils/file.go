package utils

import (
	"fmt"
	"io/ioutil"
	"strings"
)

// ExtractCodeSnippet 根据文件路径和起止行号提取代码片段
func ExtractCodeSnippet(path string, startLine, endLine int) (string, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	// 注意：行号从1开始计数
	if startLine < 1 || endLine > len(lines) || startLine > endLine {
		return "", fmt.Errorf("Invalid line number range: %d-%d", startLine, endLine)
	}
	snippet := strings.Join(lines[startLine-1:endLine], "\n")
	return snippet, nil
}

// ExtractCodeSnippetWithLimit 根据文件路径提取文件内容
func ExtractCodeSnippetWithLimit(path string, charLimit int) (string, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return "", err
	}
	if len(data) > charLimit {
		return string(data[:charLimit]), nil
	}
	return string(data), nil
}
