package embedding

import (
	"strings"
	"testing"
)

// TestSplitTextByToken_ChunkSize — TC EMB-01/02
// chunkTokenMax 应是 chunk 的 rune 上限，与 estimateTokens（1 rune = 1 token）一致；
// 历史实现错误地按 *2 切，导致中文 chunk 实际逼近 BGE-512 上限。
func TestSplitTextByToken_ChunkSize(t *testing.T) {
	text := strings.Repeat("中", 800)

	chunks := splitTextByToken(text, 200, 300)
	if len(chunks) < 2 {
		t.Fatalf("expected ≥ 2 chunks for 800-rune input, got %d", len(chunks))
	}

	for i, c := range chunks {
		runes := []rune(c)
		if len(runes) > 300 {
			t.Errorf("chunk[%d] has %d runes, exceeds chunkMax=300 (×2 bug not fixed?)",
				i, len(runes))
		}
		// 非末尾 chunk 不应短于 chunkMin
		if i < len(chunks)-1 && len(runes) < 200 {
			t.Errorf("non-final chunk[%d] has %d runes, below chunkMin=200", i, len(runes))
		}
	}
}

// TestSplitTextByToken_NoEmptyChunks — covers a previous off-by-one in the
// trailing slice handling.
func TestSplitTextByToken_NoEmptyChunks(t *testing.T) {
	text := strings.Repeat("a", 500)
	chunks := splitTextByToken(text, 100, 200)
	for i, c := range chunks {
		if len([]rune(c)) == 0 {
			t.Errorf("chunk[%d] is empty", i)
		}
	}
}

// TestSplitTextByToken_ShortInput — input shorter than chunkMin should return
// a single chunk equal to the input.
func TestSplitTextByToken_ShortInput(t *testing.T) {
	text := "短文本"
	chunks := splitTextByToken(text, 200, 300)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk for short input, got %d", len(chunks))
	}
	if chunks[0] != text {
		t.Errorf("chunk[0] = %q, want %q", chunks[0], text)
	}
}

// TestEstimateTokens_RuneCount — confirm 1 rune = 1 token.
func TestEstimateTokens_RuneCount(t *testing.T) {
	cases := map[string]int{
		"":         0,
		"abc":      3,
		"中文":       2,
		"a中b文c":    5,
		"  spaces": 8,
	}
	for in, want := range cases {
		if got := estimateTokens(in); got != want {
			t.Errorf("estimateTokens(%q) = %d, want %d", in, got, want)
		}
	}
}
