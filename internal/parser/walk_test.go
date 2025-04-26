// ===== file: internal/parser/walk_test.go =====
package parser

import (
	"os"
	"path/filepath"
	"testing"
)

// TestWalkAndParse ensures that WalkAndParse visits .go files but skips hidden directories
func TestWalkAndParse(t *testing.T) {
	tmp := t.TempDir()
	// visible file in root
	visible := filepath.Join(tmp, "visible.go")
	err := os.WriteFile(visible, []byte("package main\nfunc Foo() {}"), 0644)
	if err != nil {
		t.Fatalf("failed to write visible.go: %v", err)
	}
	// nested directory with test.go
	nested := filepath.Join(tmp, "dir1")
	if err := os.Mkdir(nested, 0755); err != nil {
		t.Fatalf("mkdir dir1: %v", err)
	}
	testf := filepath.Join(nested, "test.go")
	if err := os.WriteFile(testf, []byte("package main\nfunc Bar() {}"), 0644); err != nil {
		t.Fatalf("failed to write test.go: %v", err)
	}
	// hidden directory with hidden.go
	hiddenDir := filepath.Join(tmp, ".hidden")
	if err := os.Mkdir(hiddenDir, 0755); err != nil {
		t.Fatalf("mkdir .hidden: %v", err)
	}
	hiddenf := filepath.Join(hiddenDir, "hidden.go")
	if err := os.WriteFile(hiddenf, []byte("package main\nfunc Hidden() {}"), 0644); err != nil {
		t.Fatalf("failed to write hidden.go: %v", err)
	}

	// collect function names
	var names []string
	err = WalkAndParse(tmp, func(fi FunctionInfo) {
		names = append(names, fi.Name)
	})
	if err != nil {
		t.Fatalf("WalkAndParse error: %v", err)
	}
	// verify Foo and Bar found, Hidden not
	found := make(map[string]bool)
	for _, n := range names {
		found[n] = true
	}
	if !found["Foo"] {
		t.Errorf("expected Foo, got %v", names)
	}
	if !found["Bar"] {
		t.Errorf("expected Bar, got %v", names)
	}
	if found["Hidden"] {
		t.Errorf("did not expect Hidden in results: %v", names)
	}
}
