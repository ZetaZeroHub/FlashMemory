// ===== file: internal/index/api_test.go =====
package back

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDeleteIndex verifies that DeleteIndex removes the .gitgo directory
func TestDeleteIndex(t *testing.T) {
	tmp := t.TempDir()
	gitgo := filepath.Join(tmp, ".gitgo")
	// create dummy .gitgo
	if err := os.MkdirAll(filepath.Join(gitgo, "subdir"), 0755); err != nil {
		t.Fatalf("failed to create .gitgo: %v", err)
	}
	// call DeleteIndex
	if err := DeleteIndex(tmp); err != nil {
		t.Fatalf("DeleteIndex error: %v", err)
	}
	// verify removal
	if _, err := os.Stat(gitgo); !os.IsNotExist(err) {
		t.Errorf(".gitgo should be removed, but exists: %v", err)
	}
}

// TestBuildIndexFull runs BuildIndex with full=true and checks .gitgo creation
func TestBuildIndexFull(t *testing.T) {
	tmp := t.TempDir()
	// precreate .gitgo to ensure it's cleared
	if err := os.MkdirAll(filepath.Join(tmp, ".gitgo", "old"), 0755); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	// build full index
	if err := BuildIndex(tmp, true); err != nil {
		t.Fatalf("BuildIndex(full) error: %v", err)
	}
	// verify .gitgo exists
	if info, err := os.Stat(filepath.Join(tmp, ".gitgo")); err != nil {
		t.Errorf("expected .gitgo directory, got error: %v", err)
	} else if !info.IsDir() {
		t.Errorf(".gitgo exists but is not a directory")
	}
}

// TestIncrementalUpdate creates an index incrementally and checks .gitgo
func TestIncrementalUpdate(t *testing.T) {
	tmp := t.TempDir()
	// run incremental update
	if err := IncrementalUpdate(tmp, "feature", "commit123"); err != nil {
		t.Fatalf("IncrementalUpdate error: %v", err)
	}
	// verify .gitgo exists
	if _, err := os.Stat(filepath.Join(tmp, ".gitgo")); err != nil {
		t.Errorf(".gitgo not created by IncrementalUpdate: %v", err)
	}
}

// TestIndexCodeDirect calls the unexported indexCode function
func TestIndexCodeDirect(t *testing.T) {
	tmp := t.TempDir()
	// direct call to indexCode (requires test in same package)
	if err := indexCode(tmp, "dev", "abc123", false, ""); err != nil {
		t.Fatalf("indexCode error: %v", err)
	}
	// verify .gitgo exists
	if _, err := os.Stat(filepath.Join(tmp, ".gitgo")); err != nil {
		t.Errorf(".gitgo not created by indexCode: %v", err)
	}
}
