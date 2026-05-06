package graph

import (
	"path/filepath"
	"testing"

	"github.com/kinglegendzzh/flashmemory/internal/analyzer"
	"github.com/kinglegendzzh/flashmemory/internal/parser"
)

func TestBuildGraphBuildsAnchorsAndNormalizesPaths(t *testing.T) {
	projDir := t.TempDir()
	fileA := filepath.Join(projDir, "docs", "a.md")
	fileB := filepath.Join(projDir, "docs", "b.pdf")

	results := []analyzer.LLMAnalysisResult{
		{
			Func: parser.FunctionInfo{
				Name:         "A",
				File:         fileA,
				Package:      "docs",
				StartLine:    1,
				EndLine:      20,
				FunctionType: "llm_parser",
			},
		},
		{
			Func: parser.FunctionInfo{
				Name:         "B",
				File:         fileB,
				Source:       fileB + "::page_3",
				Package:      "docs",
				StartLine:    5,
				EndLine:      18,
				FunctionType: "llm_parser",
				Page:         3,
			},
		},
	}

	kg := BuildGraph(results, projDir)
	if len(kg.Anchors) != 2 {
		t.Fatalf("expected 2 anchors, got %d", len(kg.Anchors))
	}
	if got := kg.Functions[0].Func.File; got != "docs/a.md" {
		t.Fatalf("unexpected normalized file: %q", got)
	}
	if got := kg.Functions[0].Func.Source; got != "docs/a.md" {
		t.Fatalf("expected source fallback to normalized file, got %q", got)
	}
	if got := kg.Functions[1].Func.Source; got != "docs/b.pdf::page_3" {
		t.Fatalf("expected source suffix to be preserved, got %q", got)
	}
}
