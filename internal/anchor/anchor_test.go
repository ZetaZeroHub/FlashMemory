package anchor

import "testing"

func TestNormalizePathKeepsSuffix(t *testing.T) {
	projDir := "/repo"
	got := NormalizePath("/repo/docs/spec.pdf::page_3", projDir)
	want := "docs/spec.pdf::page_3"
	if got != want {
		t.Fatalf("NormalizePath mismatch: got=%q want=%q", got, want)
	}
}

func TestBuildIDStable(t *testing.T) {
	id1 := BuildID("Parse", "parser", "function", "internal/parser/a.go", "internal/parser/a.go", 0, 0, 10, 20)
	id2 := BuildID("Parse", "parser", "function", "internal/parser/a.go", "internal/parser/a.go", 0, 0, 10, 20)
	if id1 != id2 {
		t.Fatalf("BuildID should be deterministic: %q != %q", id1, id2)
	}

	id3 := BuildID("Parse", "parser", "function", "internal/parser/a.go", "internal/parser/a.go", 1, 0, 10, 20)
	if id1 == id3 {
		t.Fatalf("BuildID should change when anchor fields change")
	}
}
