package index

import (
	"database/sql"
	"testing"

	"github.com/kinglegendzzh/flashmemory/internal/analyzer"
	"github.com/kinglegendzzh/flashmemory/internal/parser"
)

func TestPersistDocHierarchyFromResults(t *testing.T) {
	tmp := t.TempDir()
	db, err := EnsureIndexDB(tmp)
	if err != nil {
		t.Fatalf("EnsureIndexDB failed: %v", err)
	}
	defer db.Close()

	results := []analyzer.LLMAnalysisResult{
		{
			Func: parser.FunctionInfo{
				Name:         "doc_section_1_intro",
				File:         "docs/a.md",
				Source:       "docs/a.md",
				StartLine:    1,
				EndLine:      3,
				FunctionType: "llm_parser",
				CodeSnippet:  "# Intro\nhello",
			},
		},
		{
			Func: parser.FunctionInfo{
				Name:         "doc_section_2_setup",
				File:         "docs/a.md",
				Source:       "docs/a.md",
				StartLine:    4,
				EndLine:      9,
				FunctionType: "llm_parser",
				CodeSnippet:  "## Setup\nstep1\nstep2",
			},
		},
	}

	if err := PersistDocHierarchyFromResults(db, tmp, results); err != nil {
		t.Fatalf("PersistDocHierarchyFromResults failed: %v", err)
	}

	var nodeCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM doc_nodes WHERE project_dir = ?", tmp).Scan(&nodeCount); err != nil {
		t.Fatalf("count doc_nodes failed: %v", err)
	}
	if nodeCount < 3 {
		t.Fatalf("expected at least 3 nodes (root + chunks), got %d", nodeCount)
	}

	var edgeCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM doc_edges WHERE project_dir = ?", tmp).Scan(&edgeCount); err != nil {
		t.Fatalf("count doc_edges failed: %v", err)
	}
	if edgeCount < 2 {
		t.Fatalf("expected at least 2 edges, got %d", edgeCount)
	}
}

func TestEnsureDocSchemaTablesExist(t *testing.T) {
	tmp := t.TempDir()
	db, err := EnsureIndexDB(tmp)
	if err != nil {
		t.Fatalf("EnsureIndexDB failed: %v", err)
	}
	defer db.Close()

	assertTable := func(name string) {
		var got string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name = ?", name).Scan(&got)
		if err != nil {
			if err == sql.ErrNoRows {
				t.Fatalf("table %s not found", name)
			}
			t.Fatalf("query sqlite_master failed: %v", err)
		}
	}

	assertTable("doc_nodes")
	assertTable("doc_edges")
	assertTable("parse_artifacts")
}

func TestRecordParseArtifact(t *testing.T) {
	tmp := t.TempDir()
	db, err := EnsureIndexDB(tmp)
	if err != nil {
		t.Fatalf("EnsureIndexDB failed: %v", err)
	}
	defer db.Close()

	if err := RecordParseArtifact(db, tmp, "docs/guide.md", ParseArtifactStatusSuccess, "", "", "none", `{"ok":true}`); err != nil {
		t.Fatalf("RecordParseArtifact failed: %v", err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM parse_artifacts WHERE project_dir = ?", tmp).Scan(&count); err != nil {
		t.Fatalf("count parse_artifacts failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 artifact, got %d", count)
	}
}

func TestPersistDocHierarchyFromResults_ReferencesEdge(t *testing.T) {
	tmp := t.TempDir()
	db, err := EnsureIndexDB(tmp)
	if err != nil {
		t.Fatalf("EnsureIndexDB failed: %v", err)
	}
	defer db.Close()

	results := []analyzer.LLMAnalysisResult{
		{
			Func: parser.FunctionInfo{
				Name:         "doc_section_1_intro",
				File:         "docs/a.md",
				Source:       "docs/a.md",
				StartLine:    1,
				EndLine:      5,
				FunctionType: "llm_parser",
				CodeSnippet:  "# Intro\nsee [setup](#setup)",
			},
		},
		{
			Func: parser.FunctionInfo{
				Name:         "doc_section_2_setup",
				File:         "docs/a.md",
				Source:       "docs/a.md",
				StartLine:    6,
				EndLine:      10,
				FunctionType: "llm_parser",
				CodeSnippet:  "## Setup\nsteps",
			},
		},
	}
	if err := PersistDocHierarchyFromResults(db, tmp, results); err != nil {
		t.Fatalf("PersistDocHierarchyFromResults failed: %v", err)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM doc_edges WHERE project_dir = ? AND edge_type = 'references'", tmp).Scan(&count); err != nil {
		t.Fatalf("count references edges failed: %v", err)
	}
	if count == 0 {
		t.Fatalf("expected at least one references edge")
	}
}

func TestPersistDocHierarchyFromResults_IncrementalPreservesOtherDocs(t *testing.T) {
	tmp := t.TempDir()
	db, err := EnsureIndexDB(tmp)
	if err != nil {
		t.Fatalf("EnsureIndexDB failed: %v", err)
	}
	defer db.Close()

	docA := []analyzer.LLMAnalysisResult{
		{
			Func: parser.FunctionInfo{
				Name:         "doc_section_1_a",
				File:         "docs/a.md",
				Source:       "docs/a.md",
				StartLine:    1,
				EndLine:      3,
				FunctionType: "llm_parser",
				CodeSnippet:  "# A\nalpha",
			},
		},
	}
	docB := []analyzer.LLMAnalysisResult{
		{
			Func: parser.FunctionInfo{
				Name:         "doc_section_1_b",
				File:         "docs/b.md",
				Source:       "docs/b.md",
				StartLine:    1,
				EndLine:      3,
				FunctionType: "llm_parser",
				CodeSnippet:  "# B\nbeta",
			},
		},
	}
	if err := PersistDocHierarchyFromResults(db, tmp, docA); err != nil {
		t.Fatalf("persist docA failed: %v", err)
	}
	if err := PersistDocHierarchyFromResults(db, tmp, docB); err != nil {
		t.Fatalf("persist docB failed: %v", err)
	}
	if err := PersistDocHierarchyFromResults(db, tmp, docA); err != nil {
		t.Fatalf("incremental persist docA failed: %v", err)
	}

	var docBNodes int
	if err := db.QueryRow("SELECT COUNT(*) FROM doc_nodes WHERE project_dir = ? AND source = ?", tmp, "docs/b.md").Scan(&docBNodes); err != nil {
		t.Fatalf("count docB nodes failed: %v", err)
	}
	if docBNodes == 0 {
		t.Fatalf("expected docB nodes to survive docA rebuild")
	}

	var docBEdges int
	if err := db.QueryRow(`
SELECT COUNT(*)
FROM doc_edges
WHERE project_dir = ?
  AND doc_id IN (SELECT doc_id FROM doc_nodes WHERE project_dir = ? AND source = ?)`, tmp, tmp, "docs/b.md").Scan(&docBEdges); err != nil {
		t.Fatalf("count docB edges failed: %v", err)
	}
	if docBEdges == 0 {
		t.Fatalf("expected docB edges to survive docA rebuild")
	}
}
