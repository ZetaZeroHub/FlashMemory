package docs

import (
	"archive/zip"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	pptxSlideEntryRe = regexp.MustCompile(`^ppt/slides/slide(\d+)\.xml$`)
	pptxNotesEntryRe = regexp.MustCompile(`^ppt/notesSlides/notesSlide(\d+)\.xml$`)
)

// SplitPPTX walks slide XMLs in numeric order, emits one Section per slide
// (with first text line as title and the rest as body), and additionally
// emits speaker notes as their own sections so they remain searchable.
func SplitPPTX(path string, cfg Config) ([]Section, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open pptx failed: %w", err)
	}
	defer reader.Close()

	type slideEntry struct {
		num   int
		name  string
		isNotes bool
	}
	var entries []slideEntry
	for _, f := range reader.File {
		if m := pptxSlideEntryRe.FindStringSubmatch(f.Name); m != nil {
			n, _ := strconv.Atoi(m[1])
			entries = append(entries, slideEntry{num: n, name: f.Name})
			continue
		}
		if m := pptxNotesEntryRe.FindStringSubmatch(f.Name); m != nil {
			n, _ := strconv.Atoi(m[1])
			entries = append(entries, slideEntry{num: n, name: f.Name, isNotes: true})
		}
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no slide xml found in pptx: %s", path)
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].num != entries[j].num {
			return entries[i].num < entries[j].num
		}
		// Notes follow their slide.
		return !entries[i].isNotes && entries[j].isNotes
	})

	sourcePath := filepath.ToSlash(path)
	sections := make([]Section, 0, len(entries)*2)
	for _, e := range entries {
		text, err := extractTextFromZipXML(&reader.Reader, e.name)
		if err != nil {
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		source := fmt.Sprintf("%s::%s", sourcePath, e.name)

		if e.isNotes {
			sections = append(sections, Section{
				Title:     fmt.Sprintf("slide_%d_notes", e.num),
				StartLine: 1,
				EndLine:   len(strings.Split(text, "\n")),
				Content:   text,
				Source:    source,
				Page:      0,
				Slide:     e.num,
			})
			continue
		}

		// Slide body: first non-empty line as title, rest as body section.
		sections = append(sections, splitPPTXSlideText(text, source, e.num)...)
	}

	if len(sections) == 0 {
		return nil, fmt.Errorf("pptx extraction produced empty text: %s", path)
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

func splitPPTXSlideText(text, source string, slideNum int) []Section {
	cleaned := make([]string, 0, 8)
	for _, line := range strings.Split(text, "\n") {
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
		Title:        title,
		StartLine:    1,
		EndLine:      1,
		Content:      title,
		Source:       source,
		Slide:        slideNum,
		HeadingLevel: 1,
	}}
	if len(body) > 0 {
		bodyChunks := splitTextLegacy(body, legacyDocChunkLines, source, 0, slideNum)
		out = append(out, bodyChunks...)
	}
	return out
}
