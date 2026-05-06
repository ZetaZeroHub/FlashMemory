// Package docs implements multi-modal document parsing (MD/PDF/DOCX/PPTX/Image)
// with a unified Section model. All format-specific splitters return []Section
// which the parent parser package converts to FunctionInfo.
package docs

// Section is the canonical chunk produced by document splitters.
// Exported so the parent parser package can construct FunctionInfo from it.
type Section struct {
	Title     string
	StartLine int
	EndLine   int
	Content   string
	Source    string
	Page      int
	Slide     int

	// Structural metadata populated by format-aware splitters.
	HeadingLevel int      // 1..6, 0 means non-heading chunk
	ParentTitle  string   // parent heading when nested
	Quality      string   // "high" / "medium" / "low" — drives auto-fallback
	KBIDs        []string // kb_qa_xxxxxxxxxx codes embedded in Content
}

// Config controls how documents are split. Mirrors fields from
// config.DocParserConfig but kept independent so the docs subpackage
// has no upward dependency.
type Config struct {
	DocChunkLines   int
	MaxSectionChars int
	MinChunkChars   int
	OCRLangs        string
	ExtractKBIDs    bool

	// Pandoc fallback knobs — only consulted by the orchestrator.
	PandocFallback string // "off" | "auto" | "force"
	PandocCacheDir string
	PandocBin      string
}

// DefaultConfig matches the historical hard-coded values from doc_parser.go
// so behavior is unchanged when callers don't supply a config.
func DefaultConfig() Config {
	return Config{
		DocChunkLines:   80,
		MaxSectionChars: 4000,
		MinChunkChars:   100,
		OCRLangs:        "chi_sim+eng",
		ExtractKBIDs:    false,
		PandocFallback:  "off",
		PandocCacheDir:  ".gitgo/pandoc_cache",
		PandocBin:       "pandoc",
	}
}
