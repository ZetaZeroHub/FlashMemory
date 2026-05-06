package docs

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestSplitPPTX_NumericSlideOrdering — TC PPT-03
// slide10.xml 应排在 slide2.xml 之后（数字序而非字典序）。
func TestSplitPPTX_NumericSlideOrdering(t *testing.T) {
	cfg := DefaultConfig()
	path := filepath.Join(fixtureDir("pptx"), "multi_slides.pptx")
	sections, err := SplitPPTX(path, cfg)
	if err != nil {
		t.Fatalf("SplitPPTX failed: %v", err)
	}
	if len(sections) < 11 {
		t.Fatalf("expected ≥ 11 sections (one per slide), got %d", len(sections))
	}

	// 抽取每个 section 的 Slide 字段，确认顺序为 1..N 升序
	prev := 0
	for _, s := range sections {
		if s.Slide == 0 {
			continue
		}
		if s.Slide < prev {
			t.Errorf("slide order regression: slide %d came after slide %d", s.Slide, prev)
		}
		if s.Slide > prev {
			prev = s.Slide
		}
	}
	if prev < 11 {
		t.Errorf("highest slide reached = %d, want ≥ 11", prev)
	}
}

// TestSplitPPTX_SpeakerNotes — TC PPT-05
func TestSplitPPTX_SpeakerNotes(t *testing.T) {
	cfg := DefaultConfig()
	path := filepath.Join(fixtureDir("pptx"), "with_notes.pptx")
	sections, err := SplitPPTX(path, cfg)
	if err != nil {
		t.Fatalf("SplitPPTX failed: %v", err)
	}

	var foundNote bool
	for _, s := range sections {
		if strings.Contains(s.Title, "notes") &&
			strings.Contains(s.Content, "speaker note") {
			foundNote = true
			break
		}
	}
	if !foundNote {
		titles := make([]string, 0, len(sections))
		for _, s := range sections {
			titles = append(titles, s.Title)
		}
		t.Errorf("no notes section found; titles=%v", titles)
	}
}
