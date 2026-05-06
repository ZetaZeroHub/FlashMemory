package docs

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// kbAssetsDir locates the hackathon KB. Falls back to the main repo when the
// worktree doesn't contain a copy (assets/ is at master's working tree only).
func kbAssetsDir() string {
	candidates := []string{
		// worktree path
		filepath.Join("..", "..", "..", "assets", "data", "knowledge_base"),
		// main repo path
		"/Users/apple/Public/openProject/flashmemory/assets/data/knowledge_base",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

// TestE2E_KBCoverage walks every knowledge base file in assets/data/knowledge_base
// and verifies our structure-aware splitters preserve the kb_qa codes. The
// hackathon awards "可追溯性" (traceability) for predictions that cite the
// correct kb_qa code; if the splitter can't recall a code from its source
// document then no downstream component can either.
//
// Acceptance gate (P0): ≥ 95% of kb_qa codes present in the source files
// must show up in some Section.KBIDs.
func TestE2E_KBCoverage(t *testing.T) {
	root := kbAssetsDir()
	if root == "" {
		t.Skip("hackathon KB assets not available; copy data into worktree to enable")
	}

	cfg := DefaultConfig()
	cfg.ExtractKBIDs = true

	type fileResult struct {
		path        string
		expected    int
		extracted   int
		coverage    float64
		sections    int
	}
	var results []fileResult
	totalExpected := 0
	totalExtracted := 0

	codePattern := regexp.MustCompile(`kb_qa_[a-f0-9]{10}`)

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		ext := strings.ToLower(filepath.Ext(path))
		var sections []Section
		var splitErr error

		switch ext {
		case ".md":
			sections, splitErr = SplitMarkdown(path, cfg)
		case ".docx":
			sections, splitErr = SplitDOCX(path, cfg)
		case ".pdf":
			if _, err := exec.LookPath("pdftotext"); err != nil {
				return nil // skip silently without pdftotext
			}
			sections, splitErr = SplitPDF(path, cfg)
		default:
			return nil
		}
		if splitErr != nil {
			t.Logf("[WARN] %s: %v", filepath.Base(path), splitErr)
			return nil
		}

		// Ground truth: regex over raw bytes (or pdftotext output for PDFs).
		var rawText string
		if ext == ".pdf" {
			if data, err := exec.Command("pdftotext", "-layout", "-enc", "UTF-8", path, "-").Output(); err == nil {
				rawText = string(data)
			}
		} else if ext == ".md" {
			if data, err := os.ReadFile(path); err == nil {
				rawText = string(data)
			}
		} else {
			// docx — get raw text via pandoc if available, else use the
			// extracted content from sections (best-effort approximation).
			conv := &PandocConverter{Bin: "pandoc"}
			if conv.IsAvailable() {
				if md, err := conv.ConvertToMarkdown(path); err == nil {
					rawText = md
				}
			}
			if rawText == "" {
				for _, s := range sections {
					rawText += s.Content + "\n"
				}
			}
		}

		expectedCodes := uniqueCodes(codePattern.FindAllString(rawText, -1))
		extractedCodes := map[string]struct{}{}
		for _, s := range sections {
			for _, id := range s.KBIDs {
				extractedCodes[id] = struct{}{}
			}
		}

		matched := 0
		for _, c := range expectedCodes {
			if _, ok := extractedCodes[c]; ok {
				matched++
			}
		}

		var cov float64
		if len(expectedCodes) > 0 {
			cov = float64(matched) / float64(len(expectedCodes))
		}
		results = append(results, fileResult{
			path:      filepath.Base(path),
			expected:  len(expectedCodes),
			extracted: matched,
			coverage:  cov,
			sections:  len(sections),
		})
		totalExpected += len(expectedCodes)
		totalExtracted += matched
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(results) == 0 {
		t.Skip("no KB files processed (pdftotext missing?)")
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].coverage < results[j].coverage
	})

	fmt.Println("\n========== KB kb_qa coverage report ==========")
	fmt.Printf("%-32s %8s %8s %8s %8s\n", "file", "sections", "expected", "extracted", "coverage")
	for _, r := range results {
		fmt.Printf("%-32s %8d %8d %8d %7.1f%%\n",
			truncateRight(r.path, 32), r.sections, r.expected, r.extracted, r.coverage*100)
	}
	totalCov := 0.0
	if totalExpected > 0 {
		totalCov = float64(totalExtracted) / float64(totalExpected)
	}
	fmt.Printf("%-32s %8s %8d %8d %7.1f%%\n",
		"TOTAL", "-", totalExpected, totalExtracted, totalCov*100)
	fmt.Println("==============================================")

	// P0 acceptance gate: ≥ 95% coverage overall.
	if totalCov < 0.95 {
		t.Errorf("kb_qa coverage %.1f%% below P0 threshold 95%%", totalCov*100)
	}
}

func uniqueCodes(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func truncateRight(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
