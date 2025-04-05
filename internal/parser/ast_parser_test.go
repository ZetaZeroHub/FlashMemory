package parser

import (
	"path/filepath"
	"testing"
)

// TestGoASTParser tests the Go AST parser implementation
func TestGoASTParser(t *testing.T) {
	// Create a parser instance
	parser := &GoASTParser{}

	// Get the absolute path to the test file
	testFilePath, err := filepath.Abs("testdata/test_samples.go")
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
		t.Errorf("No functions found in test file")
	}

	// Create a map for easier lookup
	funcMap := make(map[string]FunctionInfo)
	for _, fn := range funcs {
		funcMap[fn.Name] = fn
		t.Log("Function:", fn.Name, "Receiver:", fn.Receiver, "Calls:", fn.Calls)
	}

	// Test for specific functions
	testCases := []struct {
		name     string
		expected bool
		receiver string
		calls    []string
	}{
		{"SimpleFunction", true, "", []string{"fmt.Println"}},
		{"FunctionWithParams", true, "", []string{"fmt.Sprintf"}},
		{"*StructWithMethods.GetInfo", true, "*StructWithMethods", []string{"fmt.Sprintf"}},
		{"*StructWithMethods.UpdateAge", true, "*StructWithMethods", []string{"SimpleFunction", "FunctionWithParams", "*StructWithMethods.privateHelper"}},
		{"*StructWithMethods.privateHelper", true, "*StructWithMethods", []string{"strings.ToUpper"}},
		{"NonExistentFunction", false, "", nil},
	}

	for _, tc := range testCases {
		fn, exists := funcMap[tc.name]
		if exists != tc.expected {
			if tc.expected {
				t.Errorf("Expected function %s to exist, but it doesn't", tc.name)
			} else {
				t.Errorf("Expected function %s to not exist, but it does", tc.name)
			}
			continue
		}

		if !tc.expected {
			continue // Skip further checks for functions that shouldn't exist
		}

		// Check receiver
		if fn.Receiver != tc.receiver {
			t.Errorf("Function %s: expected receiver %s, got %s", tc.name, tc.receiver, fn.Receiver)
		}

		// Check function calls
		for _, expectedCall := range tc.calls {
			found := false
			for _, actualCall := range fn.Calls {
				if actualCall == expectedCall {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Function %s: expected call to %s not found in %v", tc.name, expectedCall, fn.Calls)
			}
		}
	}
}
