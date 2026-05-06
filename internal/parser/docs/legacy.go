package docs

import (
	"archive/zip"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// This file mirrors the historical splitter logic from internal/parser/doc_parser.go,
// repackaged so it can be invoked from the new docs subpackage as a fallback path.
// It is preserved verbatim (modulo type renames) so behavior matches v0.4.5 exactly
// when the new structure-aware splitters in markdown.go / docx.go / pdf.go are
// not yet in effect.

const (
	legacyDocChunkLines = 80
)

var (
	legacyMarkdownHeadingPattern = regexp.MustCompile(`^\s{0,3}#{1,6}\s+`)
	legacySlideFilePattern       = regexp.MustCompile(`slide(\d+)\.xml$`)
	legacyPDFHeadingPattern      = regexp.MustCompile(`^(\d+(\.\d+){0,3}|[A-Z][A-Z0-9 _-]{3,}|.{1,40}:)$`)
)

// LegacyExtractSections is the v0.4.5 dispatch used by the orchestrator
// before structure-aware splitters land.
func LegacyExtractSections(path, lang string) ([]Section, error) {
	sourcePath := filepath.ToSlash(path)

	switch lang {
	case "markdown", "text":
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read file failed: %w", err)
		}
		lines := strings.Split(string(raw), "\n")
		if lang == "markdown" {
			return splitMarkdownLegacy(lines, sourcePath, 0, 0), nil
		}
		return splitTextLegacy(lines, legacyDocChunkLines, sourcePath, 0, 0), nil
	case "pdf":
		return extractPDFLegacy(path)
	case "pptx":
		return extractPPTXLegacy(path)
	case "docx":
		return extractDOCXLegacy(path)
	case "image":
		return extractImageLegacy(path)
	default:
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read file failed: %w", err)
		}
		lines := strings.Split(string(raw), "\n")
		return splitTextLegacy(lines, legacyDocChunkLines, sourcePath, 0, 0), nil
	}
}

func splitMarkdownLegacy(lines []string, source string, page, slide int) []Section {
	sections := make([]Section, 0, 8)
	currentStart := 1
	currentTitle := "document_intro"

	for i, line := range lines {
		if !legacyMarkdownHeadingPattern.MatchString(line) {
			continue
		}
		lineNo := i + 1
		if lineNo > currentStart {
			content := strings.Join(lines[currentStart-1:i], "\n")
			if strings.TrimSpace(content) != "" {
				sections = append(sections, Section{
					Title:     currentTitle,
					StartLine: currentStart,
					EndLine:   lineNo - 1,
					Content:   content,
					Source:    source,
					Page:      page,
					Slide:     slide,
				})
			}
		}
		currentStart = lineNo
		currentTitle = strings.TrimSpace(strings.TrimLeft(line, "#"))
		if currentTitle == "" {
			currentTitle = fmt.Sprintf("section_%d", len(sections)+1)
		}
	}

	if currentStart <= len(lines) {
		content := strings.Join(lines[currentStart-1:], "\n")
		if strings.TrimSpace(content) != "" {
			sections = append(sections, Section{
				Title:     currentTitle,
				StartLine: currentStart,
				EndLine:   len(lines),
				Content:   content,
				Source:    source,
				Page:      page,
				Slide:     slide,
			})
		}
	}

	if len(sections) == 0 {
		return splitTextLegacy(lines, legacyDocChunkLines, source, page, slide)
	}
	return sections
}

func splitTextLegacy(lines []string, chunkSize int, source string, page, slide int) []Section {
	if chunkSize <= 0 {
		chunkSize = legacyDocChunkLines
	}
	sections := make([]Section, 0, (len(lines)/chunkSize)+1)
	for start := 0; start < len(lines); start += chunkSize {
		end := start + chunkSize
		if end > len(lines) {
			end = len(lines)
		}
		content := strings.Join(lines[start:end], "\n")
		if strings.TrimSpace(content) == "" {
			continue
		}
		sections = append(sections, Section{
			Title:     fmt.Sprintf("chunk_%d", (start/chunkSize)+1),
			StartLine: start + 1,
			EndLine:   end,
			Content:   content,
			Source:    source,
			Page:      page,
			Slide:     slide,
		})
	}
	return sections
}

func splitPDFLegacy(lines []string, source string, page, slide int) []Section {
	sections := make([]Section, 0, 8)
	currentTitle := ""
	currentStart := 1
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
			Title:     title,
			StartLine: currentStart,
			EndLine:   endLine,
			Content:   content,
			Source:    source,
			Page:      page,
			Slide:     slide,
		})
	}
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		lineNo := i + 1
		isHeading := legacyPDFHeadingPattern.MatchString(line)
		if isHeading && lineNo > currentStart {
			flush(lineNo - 1)
			currentStart = lineNo
			currentTitle = line
		} else if isHeading {
			currentTitle = line
		}
	}
	flush(len(lines))
	if len(sections) == 0 {
		return splitTextLegacy(lines, legacyDocChunkLines, source, page, slide)
	}
	return sections
}

func splitPPTXLegacy(lines []string, source string, page, slide int) []Section {
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t != "" {
			cleaned = append(cleaned, t)
		}
	}
	if len(cleaned) == 0 {
		return nil
	}
	title := cleaned[0]
	body := cleaned[1:]
	out := []Section{{
		Title:     title,
		StartLine: 1,
		EndLine:   1,
		Content:   title,
		Source:    source,
		Page:      page,
		Slide:     slide,
	}}
	if len(body) > 0 {
		out = append(out, splitTextLegacy(body, legacyDocChunkLines, source, page, slide)...)
	}
	return out
}

func extractPDFLegacy(path string) ([]Section, error) {
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
		sections = append(sections, splitPDFLegacy(lines, pageSource, pageNo, 0)...)
	}
	if len(sections) == 0 {
		return nil, fmt.Errorf("pdf extraction produced no sections: %s", path)
	}
	return sections, nil
}

func extractPPTXLegacy(path string) ([]Section, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open pptx failed: %w", err)
	}
	defer reader.Close()

	slideNames := make([]string, 0, 8)
	for _, f := range reader.File {
		if strings.HasPrefix(f.Name, "ppt/slides/slide") && strings.HasSuffix(f.Name, ".xml") {
			slideNames = append(slideNames, f.Name)
		}
	}
	sort.Strings(slideNames)
	if len(slideNames) == 0 {
		return nil, fmt.Errorf("no slide xml found in pptx: %s", path)
	}

	sourcePath := filepath.ToSlash(path)
	sections := make([]Section, 0, len(slideNames))
	for i, name := range slideNames {
		text, err := extractTextFromZipXML(&reader.Reader, name)
		if err != nil {
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		slideNo := parseSlideNumber(name, i+1)
		source := fmt.Sprintf("%s::%s", sourcePath, name)
		sections = append(sections, splitPPTXLegacy(strings.Split(text, "\n"), source, 0, slideNo)...)
	}
	if len(sections) == 0 {
		return nil, fmt.Errorf("pptx extraction produced empty text: %s", path)
	}
	return sections, nil
}

func extractDOCXLegacy(path string) ([]Section, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open docx failed: %w", err)
	}
	defer reader.Close()

	text, err := extractTextFromZipXML(&reader.Reader, "word/document.xml")
	if err != nil {
		return nil, fmt.Errorf("extract docx xml failed: %w", err)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("docx extraction produced empty text: %s", path)
	}
	source := fmt.Sprintf("%s::word/document.xml", filepath.ToSlash(path))
	sections := splitTextLegacy(strings.Split(text, "\n"), legacyDocChunkLines, source, 0, 0)
	if len(sections) == 0 {
		return nil, fmt.Errorf("docx extraction produced no sections: %s", path)
	}
	return sections, nil
}

func extractImageLegacy(path string) ([]Section, error) {
	if _, err := exec.LookPath("tesseract"); err != nil {
		return nil, fmt.Errorf("tesseract is required for image ingest: %w", err)
	}
	cmd := exec.Command("tesseract", path, "stdout", "-l", "eng")
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
	sections := splitTextLegacy(strings.Split(text, "\n"), legacyDocChunkLines, source, 0, 0)
	if len(sections) == 0 {
		return nil, fmt.Errorf("image ocr produced no sections: %s", path)
	}
	return sections, nil
}

func extractTextFromZipXML(r *zip.Reader, name string) (string, error) {
	for _, f := range r.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		defer rc.Close()
		return extractTextNodesFromXML(rc)
	}
	return "", fmt.Errorf("zip entry not found: %s", name)
}

func extractTextNodesFromXML(r io.Reader) (string, error) {
	decoder := xml.NewDecoder(r)
	var b strings.Builder
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "t" {
			continue
		}
		var text string
		if err := decoder.DecodeElement(&text, &start); err != nil {
			return "", err
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(text)
	}
	return b.String(), nil
}

func parseSlideNumber(name string, fallback int) int {
	matches := legacySlideFilePattern.FindStringSubmatch(name)
	if len(matches) < 2 {
		return fallback
	}
	val := strings.TrimSpace(matches[1])
	if val == "" {
		return fallback
	}
	var num int
	if _, err := fmt.Sscanf(val, "%d", &num); err != nil || num <= 0 {
		return fallback
	}
	return num
}
