package parser

import (
	"fmt"
	"testing"
)

// TestDetectLang tests the language detection function
func TestDetectLang(t *testing.T) {
	tests := []struct {
		filePath string
		expected string
	}{
		{"test.go", "go"},
		{"test.py", "python"},
		{"test.js", "javascript"},
		{"test.jsx", "javascript"},
		{"test.ts", "javascript"},
		{"test.tsx", "javascript"},
		{"test.java", "java"},
		{"test.cpp", "cpp"},
		{"test.cc", "cpp"},
		{"test.c", "cpp"},
		{"test.hpp", "cpp"},
		{"test.h", "cpp"},
		{"test.txt", ""},
	}

	for _, test := range tests {
		result := DetectLang(test.filePath)
		if result != test.expected {
			t.Errorf("DetectLang(%s) = %s; want %s", test.filePath, result, test.expected)
		}
	}
}

// TestNewParser tests the parser factory function
func TestNewParser(t *testing.T) {
	tests := []struct {
		lang     string
		expected string
	}{
		{"go", "*parser.GoASTParser"},
		{"python", "*parser.RegexParser"},
		{"javascript", "*parser.RegexParser"},
		{"java", "*parser.RegexParser"},
		{"cpp", "*parser.RegexParser"},
		{"", "*parser.RegexParser"},
	}

	for _, test := range tests {
		parser := NewParser(test.lang)
		// Get the type as a string using %T format
		parserType := fmt.Sprintf("%T", parser)
		if parserType != test.expected {
			t.Errorf("NewParser(%s) = %s; want %s", test.lang, parserType, test.expected)
		}
	}
}
