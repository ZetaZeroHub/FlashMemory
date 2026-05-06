package docs

import (
	"fmt"
	"regexp"
	"strings"
)

// kbIDPattern matches the kb_qa_xxxxxxxxxx codes embedded in hackathon KBs.
// Exposed here because both Markdown and DOCX splitters benefit from extracting
// these regardless of structural rules.
var kbIDPattern = regexp.MustCompile(`kb_qa_[a-f0-9]{10}`)

// ExtractKBIDs returns the unique kb_qa_xxx codes found in a content string,
// preserving first-seen order.
func ExtractKBIDs(content string) []string {
	matches := kbIDPattern.FindAllString(content, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(matches))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if _, ok := seen[m]; ok {
			continue
		}
		seen[m] = struct{}{}
		out = append(out, m)
	}
	return out
}

// PopulateKBIDs annotates each section with the codes it contains. No-op when
// disabled via cfg.ExtractKBIDs == false.
func PopulateKBIDs(sections []Section, cfg Config) {
	if !cfg.ExtractKBIDs {
		return
	}
	for i := range sections {
		sections[i].KBIDs = ExtractKBIDs(sections[i].Content)
	}
}

// RechunkBySection splits an oversized section into smaller pieces, preferring
// natural paragraph boundaries over hard truncation. The 4000-char limit in the
// historical splitter dropped tail content (incl. kb_qa codes); this helper
// preserves them by re-emitting overflow as additional Sections.
//
// Strategy:
//  1. If content fits within maxChars → return unchanged.
//  2. Try to split on blank lines (paragraph boundary). Avoid cutting within
//     ±200 chars of a kb_qa_xxxxxxxxxx marker so the code is never severed.
//  3. If a single paragraph still exceeds maxChars, fall back to sentence-level
//     splits ("。" / "."). This covers walls of text without paragraph breaks.
//  4. As a last resort, emit a hard rune-based slice — but always at full rune
//     boundaries so we never produce invalid UTF-8.
//
// All emitted sub-sections inherit the parent's title, source, and structural
// metadata; line ranges are best-effort approximations.
func RechunkBySection(s Section, maxChars int) []Section {
	if maxChars <= 0 || runeLen(s.Content) <= maxChars {
		return []Section{s}
	}

	pieces := splitByParagraphSafe(s.Content, maxChars)
	if len(pieces) <= 1 {
		pieces = splitBySentence(s.Content, maxChars)
	}
	if len(pieces) <= 1 {
		pieces = hardSplitByRune(s.Content, maxChars)
	}

	out := make([]Section, 0, len(pieces))
	for i, p := range pieces {
		title := s.Title
		if len(pieces) > 1 {
			title = fmt.Sprintf("%s_part_%d", s.Title, i+1)
		}
		out = append(out, Section{
			Title:        title,
			StartLine:    s.StartLine,
			EndLine:      s.EndLine,
			Content:      p,
			Source:       s.Source,
			Page:         s.Page,
			Slide:        s.Slide,
			HeadingLevel: s.HeadingLevel,
			ParentTitle:  s.ParentTitle,
		})
	}
	return out
}

// runeLen counts characters by rune (Chinese-friendly).
func runeLen(s string) int { return len([]rune(s)) }

// splitByParagraphSafe splits content on blank lines while ensuring no kb_qa
// code sits within ±guard runes of a cut point.
func splitByParagraphSafe(content string, maxChars int) []string {
	const guard = 200
	paragraphs := strings.Split(content, "\n\n")
	if len(paragraphs) <= 1 {
		return []string{content}
	}

	// Locate kb_qa positions in the full content (rune offsets) so we can
	// validate cut points fall outside the protected zone.
	protected := kbIDProtectedRanges(content, guard)

	var (
		out      []string
		current  strings.Builder
		curRunes int
		offset   int
	)
	for i, para := range paragraphs {
		paraRunes := runeLen(para)
		sepRunes := 0
		if i < len(paragraphs)-1 {
			sepRunes = 2 // the "\n\n" we split on
		}

		// Decide whether adding this paragraph overflows the current chunk.
		if curRunes > 0 && curRunes+paraRunes > maxChars {
			cutAt := offset
			if !rangesContain(protected, cutAt) {
				out = append(out, current.String())
				current.Reset()
				curRunes = 0
			}
		}

		if current.Len() > 0 {
			current.WriteString("\n\n")
			curRunes += 2
		}
		current.WriteString(para)
		curRunes += paraRunes
		offset += paraRunes + sepRunes
	}
	if current.Len() > 0 {
		out = append(out, current.String())
	}
	return out
}

// splitBySentence breaks content at "。" or "." boundaries.
func splitBySentence(content string, maxChars int) []string {
	delims := []string{"。", "."}
	parts := []string{content}
	for _, d := range delims {
		next := []string{}
		for _, p := range parts {
			if runeLen(p) <= maxChars {
				next = append(next, p)
				continue
			}
			pieces := strings.SplitAfter(p, d)
			next = append(next, mergeSmallPieces(pieces, maxChars)...)
		}
		parts = next
	}
	return parts
}

// mergeSmallPieces concatenates pieces until the next addition would exceed
// maxChars. Avoids an explosion of tiny shards.
func mergeSmallPieces(pieces []string, maxChars int) []string {
	out := make([]string, 0, len(pieces))
	var current strings.Builder
	curRunes := 0
	for _, p := range pieces {
		pr := runeLen(p)
		if curRunes > 0 && curRunes+pr > maxChars {
			out = append(out, current.String())
			current.Reset()
			curRunes = 0
		}
		current.WriteString(p)
		curRunes += pr
	}
	if current.Len() > 0 {
		out = append(out, current.String())
	}
	return out
}

// hardSplitByRune is the last-resort splitter — used when no whitespace or
// punctuation exists. Always cuts on rune boundaries.
func hardSplitByRune(content string, maxChars int) []string {
	runes := []rune(content)
	if len(runes) <= maxChars {
		return []string{content}
	}
	out := make([]string, 0, (len(runes)/maxChars)+1)
	for start := 0; start < len(runes); start += maxChars {
		end := start + maxChars
		if end > len(runes) {
			end = len(runes)
		}
		out = append(out, string(runes[start:end]))
	}
	return out
}

// kbIDProtectedRanges returns rune-offset spans where a cut would split a kb_qa
// code (or fall within ±guard of one).
func kbIDProtectedRanges(content string, guard int) [][2]int {
	idxs := kbIDPattern.FindAllStringIndex(content, -1)
	if len(idxs) == 0 {
		return nil
	}
	// Convert byte indexes to rune indexes once, then expand by guard.
	runeOffset := byteToRuneOffsets(content)
	out := make([][2]int, 0, len(idxs))
	for _, span := range idxs {
		startRune := runeOffset[span[0]]
		endRune := runeOffset[span[1]]
		out = append(out, [2]int{startRune - guard, endRune + guard})
	}
	return out
}

func byteToRuneOffsets(s string) map[int]int {
	m := make(map[int]int, len(s))
	r := 0
	for i := range s {
		m[i] = r
		r++
	}
	m[len(s)] = r
	return m
}

func rangesContain(ranges [][2]int, pos int) bool {
	for _, r := range ranges {
		if pos >= r[0] && pos <= r[1] {
			return true
		}
	}
	return false
}

// IsLowQuality is a heuristic that detects when the native splitter likely
// produced poor structure (e.g., DOCX with only fixed-window chunks, PDF with
// a single mega-section). Used to drive auto-fallback in T019-T020. The
// initial implementation is conservative — only flags the obvious cases.
func IsLowQuality(sections []Section, lang string) bool {
	if len(sections) == 0 {
		return true
	}
	switch lang {
	case "docx":
		// Legacy DOCX has no heading awareness; if every title looks like
		// "chunk_N" the structure is definitely lost.
		allChunk := true
		for _, s := range sections {
			if !strings.HasPrefix(s.Title, "chunk_") {
				allChunk = false
				break
			}
		}
		return allChunk
	case "pdf":
		// A single section spanning > 80 lines means heading detection failed.
		if len(sections) == 1 && (sections[0].EndLine-sections[0].StartLine) > legacyDocChunkLines {
			return true
		}
	}
	return false
}
