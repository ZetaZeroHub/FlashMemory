package docs

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestSplitDOCX_RecognizesHeadingStyles — TC DOCX-01
// 英文版 office Heading 1/2/3 应触发独立 section 并填充 HeadingLevel。
func TestSplitDOCX_RecognizesHeadingStyles(t *testing.T) {
	cfg := DefaultConfig()
	path := filepath.Join(fixtureDir("docx"), "headings_en.docx")

	sections, err := SplitDOCX(path, cfg)
	if err != nil {
		t.Fatalf("SplitDOCX failed: %v", err)
	}

	if len(sections) < 4 {
		t.Fatalf("expected ≥ 4 sections (Chapter One + 1.1 + 1.1.1 + Chapter Two), got %d:\n%s",
			len(sections), debugSections(sections))
	}

	// 期望某个 section 的 Title 含 "Chapter One" 且 HeadingLevel = 1
	var foundCh1 bool
	for _, s := range sections {
		if strings.Contains(s.Title, "Chapter One") && s.HeadingLevel == 1 {
			foundCh1 = true
			break
		}
	}
	if !foundCh1 {
		t.Errorf("no section with Title containing 'Chapter One' and HeadingLevel=1\n%s",
			debugSections(sections))
	}

	// 期望某个 section 的 Title 含 "Subsection" 且 HeadingLevel = 3
	var foundSub bool
	for _, s := range sections {
		if strings.Contains(s.Title, "Subsection") && s.HeadingLevel == 3 {
			foundSub = true
			break
		}
	}
	if !foundSub {
		t.Errorf("no section with Title containing 'Subsection' and HeadingLevel=3")
	}
}

// TestSplitDOCX_ChineseStyleNames — TC DOCX-05
// 中文版 office 用 "标题 1" / "标题 2" 也应被识别。
func TestSplitDOCX_ChineseStyleNames(t *testing.T) {
	cfg := DefaultConfig()
	path := filepath.Join(fixtureDir("docx"), "headings_cn.docx")

	sections, err := SplitDOCX(path, cfg)
	if err != nil {
		t.Fatalf("SplitDOCX failed: %v", err)
	}
	if len(sections) < 2 {
		t.Fatalf("expected ≥ 2 sections, got %d:\n%s", len(sections), debugSections(sections))
	}

	var levels []int
	for _, s := range sections {
		if s.HeadingLevel > 0 {
			levels = append(levels, s.HeadingLevel)
		}
	}
	if len(levels) < 2 {
		t.Errorf("expected ≥ 2 heading-level annotated sections, got %d (%v)",
			len(levels), levels)
	}
}

// TestSplitDOCX_PreservesKBIDs — extends DOCX-01 with kb_qa extraction.
func TestSplitDOCX_PreservesKBIDs(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ExtractKBIDs = true
	path := filepath.Join(fixtureDir("docx"), "headings_en.docx")

	sections, err := SplitDOCX(path, cfg)
	if err != nil {
		t.Fatalf("SplitDOCX failed: %v", err)
	}
	const want = "kb_qa_bbbb2222ff"
	for _, s := range sections {
		for _, id := range s.KBIDs {
			if id == want {
				return
			}
		}
	}
	t.Errorf("kb_qa code %s not extracted from DOCX", want)
}

// TestSplitDOCX_WithTable — TC DOCX-02
// 含表格的 DOCX 应在某个 section 中带有 [TABLE] 标记，且单元格文本被保留。
func TestSplitDOCX_WithTable(t *testing.T) {
	cfg := DefaultConfig()
	path := filepath.Join(fixtureDir("docx"), "with_table.docx")

	sections, err := SplitDOCX(path, cfg)
	if err != nil {
		t.Fatalf("SplitDOCX failed: %v", err)
	}

	var foundTable bool
	for _, s := range sections {
		if strings.Contains(s.Content, "[TABLE]") &&
			strings.Contains(s.Content, "数据1") &&
			strings.Contains(s.Content, "列A") {
			foundTable = true
			break
		}
	}
	if !foundTable {
		t.Errorf("no section contains [TABLE] marker with cell data:\n%s",
			debugSections(sections))
	}
}

// TestSplitDOCX_NoStylesFallback — TC DOCX-04
// 无样式 DOCX → fallback 到固定窗口切片。
func TestSplitDOCX_NoStylesFallback(t *testing.T) {
	cfg := DefaultConfig()
	path := filepath.Join(fixtureDir("docx"), "no_styles.docx")

	sections, err := SplitDOCX(path, cfg)
	if err != nil {
		t.Fatalf("SplitDOCX failed: %v", err)
	}
	if len(sections) == 0 {
		t.Fatal("expected ≥ 1 fallback chunk, got 0")
	}
	// 没有任何 HeadingLevel > 0 的 section
	for _, s := range sections {
		if s.HeadingLevel > 0 {
			t.Errorf("no_styles.docx produced HeadingLevel=%d, want all 0", s.HeadingLevel)
		}
	}
}

// debugSections renders a compact section overview for error messages.
func debugSections(sections []Section) string {
	var b strings.Builder
	for i, s := range sections {
		// Truncate content for readable output.
		content := s.Content
		if len(content) > 80 {
			content = content[:80] + "..."
		}
		b.WriteString("  [")
		b.WriteString(itoa(i))
		b.WriteString("] L=")
		b.WriteString(itoa(s.HeadingLevel))
		b.WriteString(" Title=")
		b.WriteString(s.Title)
		b.WriteString(" | ")
		b.WriteString(strings.ReplaceAll(content, "\n", " "))
		b.WriteString("\n")
	}
	return b.String()
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}
