package docs

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func skipIfNoTesseract(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tesseract"); err != nil {
		t.Skip("tesseract not installed; install via `brew install tesseract`")
	}
}

func skipIfLangMissing(t *testing.T, lang string) {
	t.Helper()
	out, err := exec.Command("tesseract", "--list-langs").CombinedOutput()
	if err != nil {
		t.Skipf("tesseract --list-langs failed: %v", err)
	}
	if !strings.Contains(string(out), lang) {
		t.Skipf("tesseract language pack '%s' not installed; install via `brew install tesseract-lang`", lang)
	}
}

// TestSplitImage_EnglishOCR — TC IMG-02 baseline
func TestSplitImage_EnglishOCR(t *testing.T) {
	skipIfNoTesseract(t)
	cfg := DefaultConfig()
	cfg.OCRLangs = "eng"
	path := filepath.Join(fixtureDir("images"), "english_text.png")

	sections, err := SplitImage(path, cfg)
	if err != nil {
		t.Fatalf("SplitImage failed: %v", err)
	}
	if len(sections) == 0 {
		t.Fatal("expected ≥ 1 section from OCR")
	}
	combined := ""
	for _, s := range sections {
		combined += s.Content + " "
	}
	if !strings.Contains(strings.ToLower(combined), "hello") {
		t.Errorf("OCR output missing 'hello'; got: %q", combined)
	}
}

// TestSplitImage_ChineseOCR — TC IMG-01 (the bilingual fix)
// Verifies the OCRLangs config flows through to tesseract -l invocation.
func TestSplitImage_ChineseOCR(t *testing.T) {
	skipIfNoTesseract(t)
	skipIfLangMissing(t, "chi_sim")
	cfg := DefaultConfig() // defaults to "chi_sim+eng"
	path := filepath.Join(fixtureDir("images"), "chinese_text.png")

	sections, err := SplitImage(path, cfg)
	if err != nil {
		t.Fatalf("SplitImage failed: %v", err)
	}
	combined := ""
	for _, s := range sections {
		combined += s.Content + " "
	}
	// 测试图含 "信用卡境外开通"，至少其中一个汉字应被识别
	if !strings.ContainsAny(combined, "信用卡境外开通") {
		t.Errorf("Chinese OCR output missing expected characters; got: %q", combined)
	}
}

// TestSplitImage_BilingualOCR — both Chinese and English packs installed,
// resolved string passes through unchanged and image.Quality stays "high".
func TestSplitImage_BilingualOCR(t *testing.T) {
	skipIfNoTesseract(t)
	skipIfLangMissing(t, "chi_sim")
	skipIfLangMissing(t, "eng")

	cfg := DefaultConfig() // chi_sim+eng
	path := filepath.Join(fixtureDir("images"), "chinese_text.png")
	sections, err := SplitImage(path, cfg)
	if err != nil {
		t.Fatalf("SplitImage failed: %v", err)
	}
	for _, s := range sections {
		if s.Quality == "medium" {
			t.Errorf("expected high quality with both langs installed, got medium")
		}
	}
}

// TestResolveOCRLangs_GracefulDegrade — when chi_sim is missing but eng is
// installed, ConvertToMarkdown should still succeed using only eng.
func TestResolveOCRLangs_GracefulDegrade(t *testing.T) {
	skipIfNoTesseract(t)
	avail := availableTesseractLangs()
	if _, ok := avail["eng"]; !ok {
		t.Skip("eng pack required for this test")
	}

	resolved, missing := resolveOCRLangs("chi_sim+eng+jpn+kor")
	// At least eng should make it through.
	if !strings.Contains(resolved, "eng") {
		t.Errorf("expected resolved langs to include eng, got %q", resolved)
	}
	// Anything not in avail should be in missing list.
	for _, m := range missing {
		if _, ok := avail[m]; ok {
			t.Errorf("lang %q reported missing but is actually installed", m)
		}
	}
}

// TestResolveOCRLangs_AllMissing — when every requested lang is missing and
// eng isn't installed either, returns empty resolved + non-empty missing.
func TestResolveOCRLangs_AllMissing(t *testing.T) {
	skipIfNoTesseract(t)
	resolved, missing := resolveOCRLangs("zzz_fake1+zzz_fake2")
	avail := availableTesseractLangs()
	if _, hasEng := avail["eng"]; hasEng {
		// eng is installed, so the function should fall back to it.
		if resolved != "eng" {
			t.Errorf("expected fallback to 'eng', got %q", resolved)
		}
	} else {
		if resolved != "" {
			t.Errorf("expected empty resolved, got %q", resolved)
		}
	}
	if len(missing) != 2 {
		t.Errorf("expected 2 missing langs, got %d: %v", len(missing), missing)
	}
}

// TestSplitImage_QualityMarkerOnPartialLangs — when chi_sim is missing but
// eng is present, output sections must be marked Quality=medium.
func TestSplitImage_QualityMarkerOnPartialLangs(t *testing.T) {
	skipIfNoTesseract(t)
	avail := availableTesseractLangs()
	if _, ok := avail["eng"]; !ok {
		t.Skip("eng pack required")
	}
	if _, ok := avail["chi_sim"]; ok {
		// To exercise the "missing" path on a machine where chi_sim IS
		// installed we request a fake lang alongside eng.
		cfg := DefaultConfig()
		cfg.OCRLangs = "zzz_fake+eng"
		sections, err := SplitImage(
			filepath.Join(fixtureDir("images"), "english_text.png"), cfg)
		if err != nil {
			t.Fatalf("SplitImage failed: %v", err)
		}
		mediumCount := 0
		for _, s := range sections {
			if s.Quality == "medium" {
				mediumCount++
			}
		}
		if mediumCount == 0 {
			t.Errorf("expected sections to be marked medium quality when fake lang dropped")
		}
		return
	}
	// chi_sim NOT installed → request chi_sim+eng, should degrade to eng.
	cfg := DefaultConfig() // chi_sim+eng
	sections, err := SplitImage(
		filepath.Join(fixtureDir("images"), "english_text.png"), cfg)
	if err != nil {
		t.Fatalf("SplitImage failed: %v", err)
	}
	mediumCount := 0
	for _, s := range sections {
		if s.Quality == "medium" {
			mediumCount++
		}
	}
	if mediumCount == 0 {
		t.Errorf("expected medium-quality marker when chi_sim missing")
	}
}

// TestSplitImage_NoTesseract — TC IMG-02 (error path)
func TestSplitImage_NoTesseract(t *testing.T) {
	if _, err := exec.LookPath("tesseract"); err == nil {
		t.Skip("tesseract installed; cannot verify missing-binary path here")
	}
	cfg := DefaultConfig()
	path := filepath.Join(fixtureDir("images"), "english_text.png")
	_, err := SplitImage(path, cfg)
	if err == nil {
		t.Fatal("expected error when tesseract missing")
	}
	if !strings.Contains(err.Error(), "tesseract") {
		t.Errorf("error should mention tesseract, got: %v", err)
	}
}
