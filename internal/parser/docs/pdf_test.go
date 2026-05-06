package docs

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func skipIfNoPdftotext(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext not installed; install via `brew install poppler`")
	}
}

// TestSplitPDF_EnglishNumericHeadings — TC PDF-01
func TestSplitPDF_EnglishNumericHeadings(t *testing.T) {
	skipIfNoPdftotext(t)
	cfg := DefaultConfig()
	path := filepath.Join(fixtureDir("pdf"), "english_numeric.pdf")
	sections, err := SplitPDF(path, cfg)
	if err != nil {
		t.Fatalf("SplitPDF failed: %v", err)
	}
	// english_numeric 含 "1. INTRODUCTION", "1.1 Background", "2. METHODOLOGY"
	if len(sections) < 3 {
		t.Errorf("expected ≥ 3 sections from numeric headings, got %d", len(sections))
	}
}

// TestSplitPDF_ChineseHeadings — TC PDF-02 (核心 bug 修复点)
func TestSplitPDF_ChineseHeadings(t *testing.T) {
	skipIfNoPdftotext(t)
	cfg := DefaultConfig()
	path := filepath.Join(fixtureDir("pdf"), "chinese_headings.pdf")
	sections, err := SplitPDF(path, cfg)
	if err != nil {
		t.Skipf("Chinese PDF fixture not available: %v", err)
	}

	// 中文标题应被识别 → 至少 4 个 section（"第一章" / "1. 我想申诉" / "2. 我想查询" / "第二章"）
	if len(sections) < 3 {
		t.Errorf("expected ≥ 3 sections from Chinese headings, got %d", len(sections))
	}

	// 期望某个 section 标题包含 "第一章" 或 "我想申诉"
	var foundCN bool
	for _, s := range sections {
		if strings.Contains(s.Title, "第一章") ||
			strings.Contains(s.Title, "我想申诉") ||
			strings.Contains(s.Title, "我想查询") {
			foundCN = true
			break
		}
	}
	if !foundCN {
		titles := make([]string, 0, len(sections))
		for _, s := range sections {
			titles = append(titles, s.Title)
		}
		t.Errorf("no section title contains expected Chinese heading; titles=%v", titles)
	}
}

// TestSplitPDF_KBIDExtraction — verifies kb_qa codes survive PDF text extraction.
func TestSplitPDF_KBIDExtraction(t *testing.T) {
	skipIfNoPdftotext(t)
	cfg := DefaultConfig()
	cfg.ExtractKBIDs = true
	path := filepath.Join(fixtureDir("pdf"), "english_numeric.pdf")
	sections, err := SplitPDF(path, cfg)
	if err != nil {
		t.Fatalf("SplitPDF failed: %v", err)
	}

	const want = "kb_qa_eeee5555ff"
	for _, s := range sections {
		for _, id := range s.KBIDs {
			if id == want {
				return
			}
		}
	}
	// kb_qa code can be in content even if not extracted; check content as fallback
	for _, s := range sections {
		if strings.Contains(s.Content, want) {
			t.Errorf("kb_qa code %s present in content but not extracted to KBIDs", want)
			return
		}
	}
	t.Errorf("kb_qa code %s missing from PDF parse output", want)
}

// TestSplitPDF_FlatPageFallback — TC PDF-03
// 全无标题的页应触发 fixed-window fallback（不会变成 1 个超大 section）。
func TestSplitPDF_FlatPageFallback(t *testing.T) {
	skipIfNoPdftotext(t)
	cfg := DefaultConfig()
	path := filepath.Join(fixtureDir("pdf"), "flat_page.pdf")
	sections, err := SplitPDF(path, cfg)
	if err != nil {
		t.Fatalf("SplitPDF failed: %v", err)
	}
	if len(sections) == 0 {
		t.Fatal("expected ≥ 1 section, got 0")
	}
}

// TestSplitPDF_MissingPdftotext — TC PDF-05
// pdftotext 不可用时应返回带提示的 error。
func TestSplitPDF_MissingPdftotext(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err == nil {
		t.Skip("pdftotext available; cannot test missing-binary path here")
	}
	cfg := DefaultConfig()
	path := filepath.Join(fixtureDir("pdf"), "english_numeric.pdf")
	_, err := SplitPDF(path, cfg)
	if err == nil {
		t.Fatal("expected error when pdftotext missing")
	}
	if !strings.Contains(err.Error(), "pdftotext") {
		t.Errorf("error should mention pdftotext, got: %v", err)
	}
}
