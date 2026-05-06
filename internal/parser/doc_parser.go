package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kinglegendzzh/flashmemory/internal/parser/docs"
)

const (
	maxDocSnippetChars = 4000
)

// DocParser parses text-like documents into section-level FunctionInfo records.
// All structural splitting is delegated to internal/parser/docs; this layer
// translates docs.Section → FunctionInfo and applies the snippet-length cap
// expected by downstream code.
type DocParser struct {
	Lang string

	// Optional configuration. Zero value preserves v0.4.5 behavior except for
	// the structural improvements landed in the docs subpackage.
	Config *docs.Config
}

func (p *DocParser) ParseFile(path string) ([]FunctionInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat file failed: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("DocParser.ParseFile expects a file path, got directory: %s", path)
	}

	cfg := docs.DefaultConfig()
	if p.Config != nil {
		cfg = *p.Config
	}

	sections, err := dispatchDocSplit(path, p.Lang, cfg)
	if err != nil {
		// Auto-fallback path: when the native splitter fails (e.g., DOCX
		// without heading styles, scanned PDF with no extractable text)
		// and the operator opted in, try pandoc.
		if cfg.PandocFallback == "auto" {
			if alt, perr := tryPandocFallback(path, cfg); perr == nil {
				sections = alt
			} else {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	if cfg.PandocFallback == "auto" && docs.IsLowQuality(sections, p.Lang) {
		if alt, perr := tryPandocFallback(path, cfg); perr == nil {
			sections = alt
		}
	}

	if len(sections) == 0 {
		return nil, nil
	}

	module := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	out := make([]FunctionInfo, 0, len(sections))
	for i, s := range sections {
		normalizedTitle := normalizeSectionTitle(s.Title)
		name := fmt.Sprintf("doc_section_%d", i+1)
		if normalizedTitle != "" {
			name = fmt.Sprintf("doc_section_%d_%s", i+1, normalizedTitle)
		}

		snippet := strings.TrimSpace(s.Content)
		if len(snippet) > maxDocSnippetChars {
			snippet = snippet[:maxDocSnippetChars] + "..."
		}

		description := snippet
		if len(description) > 220 {
			description = description[:220] + "..."
		}

		out = append(out, FunctionInfo{
			Name:         name,
			File:         path,
			Package:      module,
			Lines:        max(0, s.EndLine-s.StartLine+1),
			StartLine:    s.StartLine,
			EndLine:      s.EndLine,
			FunctionType: "llm_parser",
			Source:       s.Source,
			Page:         s.Page,
			Slide:        s.Slide,
			Description:  description,
			CodeSnippet:  snippet,
		})
	}
	return out, nil
}

// dispatchDocSplit routes a path to the format-specific splitter in the docs
// subpackage. Force-mode pandoc fallback short-circuits the native splitters.
func dispatchDocSplit(path, lang string, cfg docs.Config) ([]docs.Section, error) {
	if cfg.PandocFallback == "force" && lang != "markdown" && lang != "text" {
		if sections, err := tryPandocFallback(path, cfg); err == nil {
			return sections, nil
		}
		// fall through to native on error
	}

	switch lang {
	case "markdown":
		return docs.SplitMarkdown(path, cfg)
	case "text":
		return docs.SplitText(path, cfg)
	case "pdf":
		return docs.SplitPDF(path, cfg)
	case "pptx":
		return docs.SplitPPTX(path, cfg)
	case "docx":
		return docs.SplitDOCX(path, cfg)
	case "image":
		return docs.SplitImage(path, cfg)
	default:
		return docs.SplitText(path, cfg)
	}
}

// tryPandocFallback attempts conversion via pandoc when the operator enabled it.
func tryPandocFallback(path string, cfg docs.Config) ([]docs.Section, error) {
	if cfg.PandocFallback == "off" {
		return nil, fmt.Errorf("pandoc fallback disabled")
	}
	conv := &docs.PandocConverter{
		Bin:      cfg.PandocBin,
		CacheDir: cfg.PandocCacheDir,
	}
	if !conv.IsAvailable() {
		return nil, fmt.Errorf("pandoc not available on PATH")
	}
	return conv.SplitFromPandoc(path, cfg)
}

// normalizeSectionTitle is preserved here (vs. moving to docs subpackage) so
// it can be unit-tested against historical FunctionInfo naming guarantees.
func normalizeSectionTitle(title string) string {
	if title == "" {
		return ""
	}
	s := strings.ToLower(strings.TrimSpace(title))
	s = strings.ReplaceAll(s, " ", "_")
	builder := strings.Builder{}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			builder.WriteRune(r)
		}
	}
	res := strings.Trim(builder.String(), "_")
	if len(res) > 40 {
		return res[:40]
	}
	return res
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
