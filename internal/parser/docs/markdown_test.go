package docs

import (
	"path/filepath"
	"strings"
	"testing"
)

// fixtureDir resolves testdata files relative to the package source.
func fixtureDir(sub string) string {
	return filepath.Join("testdata", sub)
}

// TestSplitMarkdown_LongSectionRechunking — TC MD-04
// 单个 H2 下塞 5000+ 字符的 section 应被切成 2-3 个子 section。
func TestSplitMarkdown_LongSectionRechunking(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxSectionChars = 4000
	path := filepath.Join(fixtureDir("md"), "long_section.md")
	sections, err := SplitMarkdown(path, cfg)
	if err != nil {
		t.Fatalf("SplitMarkdown failed: %v", err)
	}

	// long_section.md 含一个超长 H2 (~19500 字符)，期望切成至少 2 块
	for _, s := range sections {
		if len([]rune(s.Content)) > cfg.MaxSectionChars*2 {
			t.Errorf("section %q has %d runes, exceeds 2x maxSectionChars=%d (rechunk did not run)",
				s.Title, len([]rune(s.Content)), cfg.MaxSectionChars)
		}
	}

	// kb_qa 编号必须保留在某个 section 中
	const expected = "kb_qa_aaaa1111ff"
	found := false
	for _, s := range sections {
		if strings.Contains(s.Content, expected) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("kb_qa code %s lost during rechunking", expected)
	}
}

// TestSplitMarkdown_PreservesKBIDs — TC MD-06
// 启用 ExtractKBIDs 时，每个含编号的 section 都应填充 KBIDs 字段。
func TestSplitMarkdown_PreservesKBIDs(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ExtractKBIDs = true
	path := filepath.Join(fixtureDir("md"), "kb_sample.md")

	sections, err := SplitMarkdown(path, cfg)
	if err != nil {
		t.Fatalf("SplitMarkdown failed: %v", err)
	}

	expected := map[string]bool{
		"kb_qa_98a5cb04ff": false,
		"kb_qa_c03f5681a2": false,
		"kb_qa_3a1d5f2d55": false,
	}
	for _, s := range sections {
		for _, id := range s.KBIDs {
			if _, ok := expected[id]; ok {
				expected[id] = true
			}
		}
	}
	for id, found := range expected {
		if !found {
			t.Errorf("kb_qa code %s not extracted into Section.KBIDs", id)
		}
	}
}

// TestSplitMarkdown_IntroBeforeHeading — TC MD-01
func TestSplitMarkdown_IntroBeforeHeading(t *testing.T) {
	cfg := DefaultConfig()
	path := filepath.Join(fixtureDir("md"), "intro_before_heading.md")
	sections, err := SplitMarkdown(path, cfg)
	if err != nil {
		t.Fatalf("SplitMarkdown failed: %v", err)
	}
	if len(sections) < 2 {
		t.Fatalf("expected ≥ 2 sections (intro + heading), got %d", len(sections))
	}
	if sections[0].Title != "document_intro" {
		t.Errorf("first section title = %q, want %q", sections[0].Title, "document_intro")
	}
}

// TestSplitMarkdown_NestedHeadings — TC MD-02
// H1 / H2 / H3 嵌套时，每级标题独立成 section，HeadingLevel 字段正确填充。
func TestSplitMarkdown_NestedHeadings(t *testing.T) {
	cfg := DefaultConfig()
	path := filepath.Join(fixtureDir("md"), "nested_headings.md")
	sections, err := SplitMarkdown(path, cfg)
	if err != nil {
		t.Fatalf("SplitMarkdown failed: %v", err)
	}

	// 期望 5 个 section: 顶级 / 二级 A / 三级 A.1 / 三级 A.2 / 二级 B
	if len(sections) != 5 {
		t.Errorf("expected 5 sections, got %d", len(sections))
	}

	// HeadingLevel 应反映 markdown 标题级别
	wantLevels := []int{1, 2, 3, 3, 2}
	for i, want := range wantLevels {
		if i >= len(sections) {
			break
		}
		if sections[i].HeadingLevel != want {
			t.Errorf("section[%d] %q HeadingLevel = %d, want %d",
				i, sections[i].Title, sections[i].HeadingLevel, want)
		}
	}
}

// TestSplitMarkdown_NoHeadingsFallback — TC MD-03
func TestSplitMarkdown_NoHeadingsFallback(t *testing.T) {
	cfg := DefaultConfig()
	path := filepath.Join(fixtureDir("md"), "no_headings.md")
	sections, err := SplitMarkdown(path, cfg)
	if err != nil {
		t.Fatalf("SplitMarkdown failed: %v", err)
	}
	if len(sections) == 0 {
		t.Fatal("expected ≥ 1 fallback chunk, got 0")
	}
	if !strings.HasPrefix(sections[0].Title, "chunk_") {
		t.Errorf("fallback section title = %q, want chunk_* prefix", sections[0].Title)
	}
}

// TestSplitMarkdown_EmojiTitles — TC MD-05
func TestSplitMarkdown_EmojiTitles(t *testing.T) {
	cfg := DefaultConfig()
	path := filepath.Join(fixtureDir("md"), "emoji_titles.md")
	sections, err := SplitMarkdown(path, cfg)
	if err != nil {
		t.Fatalf("SplitMarkdown failed: %v", err)
	}
	if len(sections) < 2 {
		t.Fatalf("expected ≥ 2 sections, got %d", len(sections))
	}
	for _, s := range sections {
		if strings.TrimSpace(s.Title) == "" {
			t.Errorf("section has empty title: %+v", s)
		}
	}
}

// TestExtractKBIDs_DeduplicatesAndOrders — chunking helper
func TestExtractKBIDs_DeduplicatesAndOrders(t *testing.T) {
	content := "before kb_qa_aaaa111122 middle kb_qa_bbbb222233 again kb_qa_aaaa111122 end"
	got := ExtractKBIDs(content)
	want := []string{"kb_qa_aaaa111122", "kb_qa_bbbb222233"}
	if len(got) != len(want) {
		t.Fatalf("len(got)=%d, want %d, got=%v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("got[%d]=%s, want %s", i, got[i], w)
		}
	}
}
