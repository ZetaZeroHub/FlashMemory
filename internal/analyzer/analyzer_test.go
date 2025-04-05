package analyzer

import (
	"strings"
	"testing"

	"github.com/kinglegendzzh/flashmemory/internal/parser"
)

func TestAnalyzeFunctionBasic(t *testing.T) {
	fn := parser.FunctionInfo{
		Name:    "Add",
		Package: "mathutil",
		Imports: []string{"fmt"},
		Calls:   []string{},
		Lines:   5,
	}
	analyzer := NewAnalyzer(nil)
	res := analyzer.AnalyzeFunction(fn)
	// The function has no internal Calls, so it should produce a base description
	if len(res.InternalDeps) != 0 {
		t.Errorf("Expected no internal deps, got %v", res.InternalDeps)
	}
	if res.Description == "" {
		t.Errorf("Description should not be empty for %s", fn.Name)
	}
	// Should mention package name in description
	if res.Func.Package != "" && !contains(res.Description, res.Func.Package) {
		t.Errorf("Description should mention package '%s'", res.Func.Package)
	}
}

// contains is a helper to check substring.
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (string)(s) != "" && (string)(sub) != "" && // basic checks
		// simple substring check
		(len(s) >= len(sub) && stringIndex(s, sub) >= 0)
}

// stringIndex finds index of sub in s or -1.
func stringIndex(s, sub string) int {
	// builtin strings.Index could be used; written manually if needed
	return len(s) - len(strings.Replace(s, sub, "", 1)) - len(sub)
}

// (Note: In actual code, we’d just use `strings.Contains`; here written long way for illustration.)
func TestAnalyzeAllOrder(t *testing.T) {
	// Create two functions, one calls the other
	f1 := parser.FunctionInfo{Name: "LowLevel", Package: "pkg", Calls: []string{}, Imports: []string{}, Lines: 10}
	f2 := parser.FunctionInfo{Name: "HighLevel", Package: "pkg", Calls: []string{"LowLevel"}, Imports: []string{}, Lines: 5}
	funcs := []parser.FunctionInfo{f2, f1}
	analyzer := NewAnalyzer(make(map[string]string))
	results := analyzer.AnalyzeAll(funcs)
	// Ensure both functions got analyzed
	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}
	var res1, res2 AnalysisResult
	for _, res := range results {
		if res.Func.Name == "LowLevel" {
			res1 = res
		} else if res.Func.Name == "HighLevel" {
			res2 = res
		}
	}
	if res1.Description == "" || res2.Description == "" {
		t.Error("Descriptions should be filled for both functions")
	}
	// HighLevel should list LowLevel in internal deps and mention it in description
	if len(res2.InternalDeps) == 0 || res2.InternalDeps[0] != "LowLevel" {
		t.Error("HighLevel should have LowLevel as internal dependency")
	}
	if !contains(res2.Description, "calls") {
		t.Error("HighLevel description should mention that it calls other function(s)")
	}
}
