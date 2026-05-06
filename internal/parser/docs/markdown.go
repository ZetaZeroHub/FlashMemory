package docs

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// markdownHeadingRe captures the leading "#"s and the title text on a line.
var markdownHeadingRe = regexp.MustCompile(`^\s{0,3}(#{1,6})\s+(.*)$`)

// SplitMarkdown is the canonical entry for markdown chunking. It is
// structure-aware (heading level), populates KBIDs when configured, and
// rechunks oversized sections by natural paragraph instead of truncating.
func SplitMarkdown(path string, cfg Config) ([]Section, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	source := filepath.ToSlash(path)
	lines := strings.Split(string(raw), "\n")

	sections := splitMarkdownStructured(lines, source, 0, 0)

	// Apply natural-paragraph rechunking when sections exceed the cap.
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

// splitMarkdownStructured walks the lines and emits one Section per heading,
// tracking heading level and parent title. Falls back to fixed-window chunking
// when the document contains zero markdown headings.
func splitMarkdownStructured(lines []string, source string, page, slide int) []Section {
	sections := make([]Section, 0, 8)
	currentStart := 1
	currentTitle := "document_intro"
	currentLevel := 0
	headingsSeen := 0

	// Track parent title at each heading depth so we can populate ParentTitle.
	parentByLevel := [7]string{}

	flush := func(endLine int) {
		if endLine < currentStart {
			return
		}
		content := strings.Join(lines[currentStart-1:endLine], "\n")
		if strings.TrimSpace(content) == "" {
			return
		}
		parent := ""
		if currentLevel > 1 {
			for lvl := currentLevel - 1; lvl >= 1; lvl-- {
				if parentByLevel[lvl] != "" {
					parent = parentByLevel[lvl]
					break
				}
			}
		}
		sections = append(sections, Section{
			Title:        currentTitle,
			StartLine:    currentStart,
			EndLine:      endLine,
			Content:      content,
			Source:       source,
			Page:         page,
			Slide:        slide,
			HeadingLevel: currentLevel,
			ParentTitle:  parent,
		})
	}

	for i, line := range lines {
		m := markdownHeadingRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		level := len(m[1])
		title := strings.TrimSpace(m[2])
		if title == "" {
			title = fmt.Sprintf("section_%d", len(sections)+1)
		}

		lineNo := i + 1
		flush(lineNo - 1)

		currentStart = lineNo
		currentTitle = title
		currentLevel = level
		parentByLevel[level] = title
		// Reset deeper levels when a higher-level heading opens a new branch.
		for lvl := level + 1; lvl <= 6; lvl++ {
			parentByLevel[lvl] = ""
		}
		headingsSeen++
	}
	flush(len(lines))

	// No headings at all → fall back to fixed-window chunks.
	if headingsSeen == 0 {
		return splitTextLegacy(lines, legacyDocChunkLines, source, page, slide)
	}
	return sections
}

// SplitText handles plain text (and markdown that the orchestrator routes
// here for any reason). Always uses fixed-window chunking.
func SplitText(path string, cfg Config) ([]Section, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	source := filepath.ToSlash(path)
	lines := strings.Split(string(raw), "\n")
	chunkSize := cfg.DocChunkLines
	if chunkSize <= 0 {
		chunkSize = legacyDocChunkLines
	}
	return splitTextLegacy(lines, chunkSize, source, 0, 0), nil
}
