package parser

import (
	"path/filepath"
	"testing"
)

// TestRegexParser tests the regex-based parser implementation for different languages
func TestRegexParser(t *testing.T) {
	// Test cases for different languages
	testCases := []struct {
		lang          string
		fileName      string
		expectedFuncs []string
	}{
		{"python", "test_samples.py", []string{"simple_function", "function_with_params", "ClassWithMethods.__init__", "ClassWithMethods.get_info", "ClassWithMethods.update_age", "ClassWithMethods._private_helper"}},
		{"javascript", "test_samples.js", []string{"simpleFunction", "functionWithParams", "ClassWithMethods", "getInfo", "updateAge", "_privateHelper", "arrowFunction", "functionExpression"}},
		{"cpp", "test_samples.cpp", []string{"simpleFunction", "functionWithParams", "privateHelper", "getInfo", "updateAge", "main"}},
	}

	for _, tc := range testCases {
		t.Run(tc.lang, func(t *testing.T) {
			// Create a parser instance
			parser := &RegexParser{Lang: tc.lang}

			// Get the absolute path to the test file
			testFilePath, err := filepath.Abs("testdata/" + tc.fileName)
			if err != nil {
				t.Fatalf("Failed to get absolute path: %v", err)
			}

			// Parse the test file
			funcs, err := parser.ParseFile(testFilePath)
			if err != nil {
				t.Fatalf("Failed to parse file: %v", err)
			}

			// Verify that we found the expected functions
			if len(funcs) == 0 {
				t.Errorf("No functions found in %s test file", tc.lang)
			}

			// Create a map for easier lookup
			funcMap := make(map[string]bool)
			for _, fn := range funcs {
				funcMap[fn.Name] = true
				t.Logf("Found function: %s", fn.Name)
			}

			// Check for expected functions
			for _, expectedFunc := range tc.expectedFuncs {
				if _, exists := funcMap[expectedFunc]; !exists {
					t.Errorf("Expected function %s not found in %s parser results", expectedFunc, tc.lang)
				}
			}

			// Check that package/module was set
			for _, fn := range funcs {
				if fn.Package == "" {
					t.Errorf("Package/module name not set for function %s in %s parser", fn.Name, tc.lang)
				}
			}

			// Check that imports were detected
			importsFound := false
			for _, fn := range funcs {
				if len(fn.Imports) > 0 {
					importsFound = true
					break
				}
			}
			if !importsFound {
				t.Errorf("No imports detected in %s parser results", tc.lang)
			}
		})
	}
}
