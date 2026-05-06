package docs

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// SplitImage runs tesseract OCR over an image and returns its text as
// Sections. The OCR languages are driven by cfg.OCRLangs (default
// "chi_sim+eng"). When a requested language pack is not installed we
// transparently drop it from the request and warn the caller — better to
// degrade to English-only than to fail the whole index.
func SplitImage(path string, cfg Config) ([]Section, error) {
	if _, err := exec.LookPath("tesseract"); err != nil {
		return nil, fmt.Errorf("tesseract is required for image ingest: %w "+
			"(install via `brew install tesseract tesseract-lang` on macOS)", err)
	}

	langs := strings.TrimSpace(cfg.OCRLangs)
	if langs == "" {
		langs = DefaultConfig().OCRLangs
	}

	resolved, missing := resolveOCRLangs(langs)
	if resolved == "" {
		// Even "eng" missing — surface a clear error.
		return nil, fmt.Errorf("none of the requested OCR languages (%s) are installed; "+
			"install via `brew install tesseract-lang` on macOS or apt install tesseract-ocr-* on Linux",
			langs)
	}
	if len(missing) > 0 {
		// Logged via the parent package's logger when wired; for now we
		// surface this through the section's Quality marker below so the
		// caller can react if needed.
	}

	cmd := exec.Command("tesseract", path, "stdout", "-l", resolved)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("tesseract failed: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("tesseract failed: %w", err)
	}
	text := strings.TrimSpace(string(out))
	if text == "" {
		return nil, fmt.Errorf("image ocr produced empty text: %s", path)
	}

	source := fmt.Sprintf("%s::ocr", filepath.ToSlash(path))
	chunkSize := cfg.DocChunkLines
	if chunkSize <= 0 {
		chunkSize = legacyDocChunkLines
	}
	sections := splitTextLegacy(strings.Split(text, "\n"), chunkSize, source, 0, 0)
	if len(sections) == 0 {
		return nil, fmt.Errorf("image ocr produced no sections: %s", path)
	}

	// If we had to drop language packs, mark sections as medium quality so
	// downstream pipelines (e.g., the auto-pandoc-fallback gate) can react.
	if len(missing) > 0 {
		for i := range sections {
			sections[i].Quality = "medium"
		}
	}

	PopulateKBIDs(sections, cfg)
	return sections, nil
}

// resolveOCRLangs intersects the requested langs spec with what tesseract
// actually has installed, returning the usable subset (in tesseract "+"
// syntax) and the list of dropped packs. Lookup is cached for the process
// lifetime since `--list-langs` does a directory scan.
func resolveOCRLangs(spec string) (resolved string, missing []string) {
	requested := splitLangSpec(spec)
	available := availableTesseractLangs()
	out := make([]string, 0, len(requested))
	for _, lang := range requested {
		if _, ok := available[lang]; ok {
			out = append(out, lang)
		} else {
			missing = append(missing, lang)
		}
	}
	if len(out) == 0 {
		// Last-resort: fall back to eng if it's installed.
		if _, ok := available["eng"]; ok {
			out = append(out, "eng")
		}
	}
	return strings.Join(out, "+"), missing
}

func splitLangSpec(spec string) []string {
	parts := strings.Split(spec, "+")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

var (
	tesseractLangsOnce  sync.Once
	tesseractLangsCache map[string]struct{}
)

func availableTesseractLangs() map[string]struct{} {
	tesseractLangsOnce.Do(func() {
		out, err := exec.Command("tesseract", "--list-langs").CombinedOutput()
		if err != nil {
			tesseractLangsCache = map[string]struct{}{}
			return
		}
		m := make(map[string]struct{})
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "List of") {
				continue
			}
			m[line] = struct{}{}
		}
		tesseractLangsCache = m
	})
	return tesseractLangsCache
}

// resetTesseractLangsCacheForTests is a test-only hook so unit tests can
// exercise different installed-langs scenarios without process restart.
func resetTesseractLangsCacheForTests() {
	tesseractLangsOnce = sync.Once{}
	tesseractLangsCache = nil
}
