package docs

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// pdfHeadingRe is the structure-aware heading detector. It covers:
//
//   - "1", "1.2", "1.2.3"           — pure numeric outlines
//   - "1. INTRODUCTION"             — numbered all-caps English titles
//   - "1. 我想申诉"                 — numbered Chinese titles (核心修复点)
//   - "第一章 总则" / "第3节 ..."   — Chinese chapter/section markers
//   - "## Heading"                  — markdown-style (occasionally preserved by pdftotext)
//   - "ALL CAPS HEADING"            — bare all-caps English headings
//   - "Foo Bar:"                    — short label-style with trailing colon
//
// We tolerate trailing whitespace because pdftotext -layout sometimes pads.
var pdfHeadingRe = regexp.MustCompile(
	`^(` +
		`#{1,6}\s+\S` + // markdown
		`|\d+(\.\d+){0,3}\.?\s*[\p{Han}A-Za-z]` + // 1. xxx / 1.2.3 xxx
		`|第[一二三四五六七八九十百千零0-9]+[章节条篇部分]` + // 第N章/第N节/...
		`|[A-Z][A-Z0-9 _\-]{3,}` + // ALL CAPS
		`|.{1,40}:\s*$` + // label:
		`)`,
)

// SplitPDF parses a PDF via pdftotext, splits on form-feed pages, and within
// each page detects Chinese-aware headings to produce structured Sections.
func SplitPDF(path string, cfg Config) ([]Section, error) {
	text, err := extractTextFromPDF(path)
	if err != nil {
		return nil, err
	}

	sourcePath := filepath.ToSlash(path)
	pages := strings.Split(text, "\f")
	sections := make([]Section, 0, len(pages))
	for i, page := range pages {
		page = strings.TrimSpace(page)
		if page == "" {
			continue
		}
		pageNo := i + 1
		pageSource := fmt.Sprintf("%s::page_%d", sourcePath, pageNo)
		lines := strings.Split(page, "\n")
		sections = append(sections, splitPDFPage(lines, pageSource, pageNo)...)
	}
	if len(sections) == 0 {
		return nil, fmt.Errorf("pdf extraction produced no sections: %s", path)
	}

	maxChars := cfg.MaxSectionChars
	if maxChars <= 0 {
		maxChars = DefaultConfig().MaxSectionChars
	}
	rechunked := make([]Section, 0, len(sections))
	for _, s := range sections {
		rechunked = append(rechunked, RechunkBySection(s, maxChars)...)
	}
	PopulateKBIDs(rechunked, cfg)
	return rechunked, nil
}

// splitPDFPage walks a single PDF page line-by-line, treating any line that
// matches pdfHeadingRe as a section delimiter.
func splitPDFPage(lines []string, source string, page int) []Section {
	sections := make([]Section, 0, 4)
	currentTitle := ""
	currentLevel := 0
	currentStart := 1
	headingsSeen := 0

	flush := func(endLine int) {
		if endLine < currentStart {
			return
		}
		content := strings.TrimSpace(strings.Join(lines[currentStart-1:endLine], "\n"))
		if content == "" {
			return
		}
		title := currentTitle
		if strings.TrimSpace(title) == "" {
			title = fmt.Sprintf("page_%d_chunk_%d", page, len(sections)+1)
		}
		sections = append(sections, Section{
			Title:        title,
			StartLine:    currentStart,
			EndLine:      endLine,
			Content:      content,
			Source:       source,
			Page:         page,
			Slide:        0,
			HeadingLevel: currentLevel,
		})
	}

	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		lineNo := i + 1
		if pdfHeadingRe.MatchString(line) {
			if lineNo > currentStart {
				flush(lineNo - 1)
				currentStart = lineNo
			}
			currentTitle = line
			currentLevel = inferPDFHeadingLevel(line)
			headingsSeen++
		}
	}
	flush(len(lines))

	if headingsSeen == 0 {
		// No structural cues found → fall back to fixed-window chunking.
		return splitTextLegacy(lines, legacyDocChunkLines, source, page, 0)
	}
	return sections
}

// inferPDFHeadingLevel produces a coarse heading level from the matched line.
//
//   - "1.2.3" or "1.2.3 xxx"  → level = number of dotted segments
//   - "第N章"                 → level 1
//   - "第N节"                 → level 2
//   - other matches           → level 1
func inferPDFHeadingLevel(line string) int {
	if strings.HasPrefix(line, "#") {
		level := 0
		for _, r := range line {
			if r != '#' {
				break
			}
			level++
		}
		if level > 6 {
			level = 6
		}
		return level
	}
	if strings.HasPrefix(line, "第") {
		switch {
		case strings.Contains(line, "章"):
			return 1
		case strings.Contains(line, "节"):
			return 2
		case strings.Contains(line, "条") || strings.Contains(line, "篇"):
			return 3
		}
		return 1
	}
	if dots := strings.Count(strings.SplitN(line, " ", 2)[0], "."); dots > 0 {
		return dots + 1
	}
	return 1
}

// extractTextFromPDF mirrors the legacy pdftotext invocation but lives in
// this file so SplitPDF stays self-contained.
func extractTextFromPDF(path string) (string, error) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		return "", fmt.Errorf("pdftotext is required for pdf ingest: %w", err)
	}
	cmd := exec.Command("pdftotext", "-layout", "-enc", "UTF-8", path, "-")
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("pdftotext failed: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", fmt.Errorf("pdftotext failed: %w", err)
	}
	text := strings.TrimSpace(string(out))
	if text == "" {
		return "", fmt.Errorf("pdf extraction produced empty text: %s", path)
	}
	return text, nil
}
