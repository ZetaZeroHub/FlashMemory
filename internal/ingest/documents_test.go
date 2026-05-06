package ingest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsSupportedTextDocument(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/tmp/a.md", true},
		{"/tmp/a.markdown", true},
		{"/tmp/a.txt", true},
		{"/tmp/a.rst", true},
		{"/tmp/a.pdf", true},
		{"/tmp/a.pptx", true},
		{"/tmp/a.docx", true},
		{"/tmp/a.png", true},
		{"/tmp/a.jpg", true},
		{"/tmp/a.jpeg", true},
		{"/tmp/a.webp", true},
		{"/tmp/a.bmp", true},
		{"/tmp/a.tif", true},
		{"/tmp/a.tiff", true},
		{"/tmp/a.go", false},
	}

	for _, tc := range cases {
		got := IsSupportedTextDocument(tc.path)
		if got != tc.want {
			t.Fatalf("IsSupportedTextDocument(%q)=%v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestCollectTextDocuments(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.md"), []byte("# A\nhello"), 0644); err != nil {
		t.Fatalf("write a.md failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.go"), []byte("package main"), 0644); err != nil {
		t.Fatalf("write b.go failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "manual.pdf"), []byte("%PDF-1.4"), 0644); err != nil {
		t.Fatalf("write manual.pdf failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "diagram.png"), []byte("png"), 0644); err != nil {
		t.Fatalf("write diagram.png failed: %v", err)
	}
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatalf("mkdir sub failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sub, "c.txt"), []byte("hello"), 0644); err != nil {
		t.Fatalf("write c.txt failed: %v", err)
	}
	hidden := filepath.Join(root, ".hidden")
	if err := os.MkdirAll(hidden, 0755); err != nil {
		t.Fatalf("mkdir .hidden failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(hidden, "d.md"), []byte("# hidden"), 0644); err != nil {
		t.Fatalf("write hidden md failed: %v", err)
	}

	recursive, err := CollectTextDocuments(root, true)
	if err != nil {
		t.Fatalf("CollectTextDocuments recursive failed: %v", err)
	}
	if len(recursive) != 4 {
		t.Fatalf("expected 4 docs in recursive mode, got %d (%v)", len(recursive), recursive)
	}

	flat, err := CollectTextDocuments(root, false)
	if err != nil {
		t.Fatalf("CollectTextDocuments flat failed: %v", err)
	}
	if len(flat) != 3 {
		t.Fatalf("expected 3 docs in flat mode, got %d (%v)", len(flat), flat)
	}
}
