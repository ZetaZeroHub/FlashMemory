package docs

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func skipIfNoPandoc(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("pandoc"); err != nil {
		t.Skip("pandoc not installed; install via `brew install pandoc`")
	}
}

// TestPandocConverter_BasicConversion ensures pandoc produces non-empty markdown.
func TestPandocConverter_BasicConversion(t *testing.T) {
	skipIfNoPandoc(t)
	tmp := t.TempDir()
	conv := &PandocConverter{Bin: "pandoc", CacheDir: tmp}
	if !conv.IsAvailable() {
		t.Fatal("pandoc reported unavailable despite being on PATH")
	}

	src := filepath.Join(fixtureDir("docx"), "headings_en.docx")
	md, err := conv.ConvertToMarkdown(src)
	if err != nil {
		t.Fatalf("ConvertToMarkdown failed: %v", err)
	}
	if len(md) == 0 {
		t.Fatal("pandoc output empty")
	}
	// pandoc should preserve heading text
	if !strings.Contains(md, "Chapter One") {
		t.Errorf("expected 'Chapter One' in pandoc output; got first 200 chars: %s",
			md[:min(200, len(md))])
	}
}

// TestPandocConverter_CacheHit — TC FB-06
func TestPandocConverter_CacheHit(t *testing.T) {
	skipIfNoPandoc(t)
	tmp := t.TempDir()
	conv := &PandocConverter{Bin: "pandoc", CacheDir: tmp}
	src := filepath.Join(fixtureDir("docx"), "headings_en.docx")

	first, err := conv.ConvertToMarkdown(src)
	if err != nil {
		t.Fatalf("first conversion failed: %v", err)
	}

	// Verify a cache file exists.
	entries, _ := os.ReadDir(tmp)
	if len(entries) == 0 {
		t.Fatal("expected cache file after first conversion")
	}

	// Mutate pandoc binary to a no-op so cache miss would fail. Instead of
	// faking PATH we simply call ConvertToMarkdown again and verify identical
	// output — combined with timestamp check below.
	cachePath := filepath.Join(tmp, entries[0].Name())
	infoBefore, _ := os.Stat(cachePath)

	second, err := conv.ConvertToMarkdown(src)
	if err != nil {
		t.Fatalf("second conversion failed: %v", err)
	}
	if first != second {
		t.Errorf("cached output differs from first conversion")
	}
	infoAfter, _ := os.Stat(cachePath)
	if infoAfter.ModTime() != infoBefore.ModTime() {
		t.Errorf("cache file modtime changed; pandoc was re-invoked")
	}
}

// TestPandocConverter_Concurrent — TC FB-07
func TestPandocConverter_Concurrent(t *testing.T) {
	skipIfNoPandoc(t)
	tmp := t.TempDir()
	conv := &PandocConverter{Bin: "pandoc", CacheDir: tmp}
	src := filepath.Join(fixtureDir("docx"), "headings_en.docx")

	const goroutines = 8
	results := make([]string, goroutines)
	errs := make([]error, goroutines)
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = conv.ConvertToMarkdown(src)
		}(i)
	}
	wg.Wait()

	for i, e := range errs {
		if e != nil {
			t.Fatalf("worker %d: %v", i, e)
		}
	}
	first := results[0]
	for i, r := range results {
		if r != first {
			t.Errorf("worker %d output differs from worker 0", i)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
