package docs

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// SplitDOCX is the structure-aware DOCX splitter. It parses word/document.xml
// streamingly, recognizes paragraph styles (English + Chinese variants), and
// emits one Section per heading paragraph. Tables are preserved as inline
// markers; SmartArt / drawings / equations are skipped.
func SplitDOCX(path string, cfg Config) ([]Section, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open docx failed: %w", err)
	}
	defer reader.Close()

	rc, err := openZipEntry(&reader.Reader, "word/document.xml")
	if err != nil {
		return nil, fmt.Errorf("read docx body: %w", err)
	}
	defer rc.Close()

	source := fmt.Sprintf("%s::word/document.xml", filepath.ToSlash(path))
	sections, err := splitDOCXStream(rc, source)
	if err != nil {
		return nil, err
	}
	if len(sections) == 0 {
		return nil, fmt.Errorf("docx extraction produced no sections: %s", path)
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

// docxHeadingStyles maps OOXML paragraph style names to heading levels.
// Covers the common English builds and the Chinese version of Word that
// ships with Office for China (uses "标题 1" / "标题 2" / etc).
var docxHeadingStyles = map[string]int{
	"Heading1": 1, "Heading2": 2, "Heading3": 3,
	"Heading4": 4, "Heading5": 5, "Heading6": 6,
	"heading 1": 1, "heading 2": 2, "heading 3": 3,
	"heading 4": 4, "heading 5": 5, "heading 6": 6,
	"Title": 1,

	// Chinese-localized Word style names.
	"标题 1": 1, "标题 2": 2, "标题 3": 3,
	"标题 4": 4, "标题 5": 5, "标题 6": 6,
	"标题1": 1, "标题2": 2, "标题3": 3,
	"标题4": 4, "标题5": 5, "标题6": 6,
}

// splitDOCXStream walks word/document.xml token-by-token. It enters a "paragraph"
// state on each <w:p>, tracks heading style via <w:pStyle w:val="..."/>, and
// concatenates <w:t> runs as the paragraph body. <w:tbl> blocks are flattened
// into a markdown-ish table marker.
func splitDOCXStream(r io.Reader, source string) ([]Section, error) {
	dec := xml.NewDecoder(r)

	var (
		sections     []Section
		currentTitle = "document_intro"
		currentLevel = 0
		curBody      strings.Builder
		flushed      = false

		paraStyle string
		paraText  strings.Builder
		inPara    bool

		inTable bool
		tableSB strings.Builder

		lineCounter = 1
		startLine   = 1
	)

	flush := func() {
		body := strings.TrimSpace(curBody.String())
		if body == "" && currentLevel == 0 && !flushed {
			return
		}
		sections = append(sections, Section{
			Title:        currentTitle,
			StartLine:    startLine,
			EndLine:      lineCounter,
			Content:      body,
			Source:       source,
			Page:         0,
			Slide:        0,
			HeadingLevel: currentLevel,
		})
		curBody.Reset()
		startLine = lineCounter + 1
		flushed = true
	}

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "p":
				inPara = true
				paraStyle = ""
				paraText.Reset()
			case "pStyle":
				if inPara {
					for _, a := range t.Attr {
						if a.Name.Local == "val" {
							paraStyle = a.Value
						}
					}
				}
			case "t":
				var s string
				if err := dec.DecodeElement(&s, &t); err == nil {
					if inTable {
						tableSB.WriteString(s)
						tableSB.WriteString(" ")
					} else if inPara {
						paraText.WriteString(s)
					}
				}
			case "tbl":
				inTable = true
				tableSB.Reset()
				tableSB.WriteString("[TABLE]\n")
			case "tr":
				if inTable {
					tableSB.WriteString("| ")
				}
			case "tc":
				if inTable {
					tableSB.WriteString(" | ")
				}
			case "drawing", "object":
				// skip the entire subtree to avoid pulling in alt-text noise
				_ = dec.Skip()
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "p":
				if inPara {
					text := strings.TrimSpace(paraText.String())
					if level, ok := docxHeadingStyles[paraStyle]; ok {
						// Heading paragraph → flush previous, start new section.
						flush()
						currentTitle = text
						if currentTitle == "" {
							currentTitle = fmt.Sprintf("section_%d", len(sections)+1)
						}
						currentLevel = level
					} else if text != "" {
						if curBody.Len() > 0 {
							curBody.WriteByte('\n')
						}
						curBody.WriteString(text)
					}
					lineCounter++
					inPara = false
				}
			case "tbl":
				if inTable {
					tableSB.WriteString("\n")
					if curBody.Len() > 0 {
						curBody.WriteByte('\n')
					}
					curBody.WriteString(tableSB.String())
					inTable = false
				}
			case "tr":
				if inTable {
					tableSB.WriteString(" |\n")
				}
			}
		}
	}
	flush()

	// If no heading was ever found, return a single Section containing the full
	// concatenated body. The caller (SplitDOCX) will then run RechunkBySection,
	// which splits by character count with paragraph/sentence-aware boundaries —
	// far better recall granularity than the legacy line-window fallback, which
	// produced a single chunk for any docx with < legacyDocChunkLines paragraphs.
	if !hasAnyHeading(sections) {
		body := joinAllContent(sections)
		if strings.TrimSpace(body) == "" {
			return sections, nil
		}
		return []Section{{
			Title:     "document_body",
			StartLine: 1,
			EndLine:   lineCounter,
			Content:   body,
			Source:    source,
		}}, nil
	}
	return sections, nil
}

func hasAnyHeading(sections []Section) bool {
	for _, s := range sections {
		if s.HeadingLevel > 0 {
			return true
		}
	}
	return false
}

func joinAllContent(sections []Section) string {
	parts := make([]string, 0, len(sections))
	for _, s := range sections {
		if s.Content != "" {
			parts = append(parts, s.Content)
		}
	}
	return strings.Join(parts, "\n")
}

func openZipEntry(r *zip.Reader, name string) (io.ReadCloser, error) {
	for _, f := range r.File {
		if f.Name == name {
			return f.Open()
		}
	}
	return nil, fmt.Errorf("zip entry not found: %s", name)
}
