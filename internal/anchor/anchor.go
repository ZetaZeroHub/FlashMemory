package anchor

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
)

// Locator is a stable, serializable object anchor.
type Locator struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Package      string `json:"package"`
	FunctionType string `json:"function_type"`
	File         string `json:"file"`
	Source       string `json:"source"`
	Page         int    `json:"page"`
	Slide        int    `json:"slide"`
	StartLine    int    `json:"start_line"`
	EndLine      int    `json:"end_line"`
}

// EffectiveSource returns source when present, otherwise falls back to file.
func EffectiveSource(file, source string) string {
	if strings.TrimSpace(source) != "" {
		return source
	}
	return file
}

// NormalizePath normalizes to slash path and keeps `::suffix` provenance.
func NormalizePath(path, projDir string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	base, suffix := splitSuffix(path)
	if projDir == "" {
		return filepath.ToSlash(base) + suffix
	}
	if !filepath.IsAbs(base) {
		base = filepath.Join(projDir, base)
	}
	if rel, err := filepath.Rel(projDir, base); err == nil {
		return filepath.ToSlash(rel) + suffix
	}
	return filepath.ToSlash(base) + suffix
}

// BuildID produces a deterministic ID from anchor fields.
func BuildID(name, pkg, functionType, file, source string, page, slide, startLine, endLine int) string {
	raw := fmt.Sprintf(
		"%s|%s|%s|%s|%s|%d|%d|%d|%d",
		strings.TrimSpace(name),
		strings.TrimSpace(pkg),
		strings.TrimSpace(functionType),
		strings.TrimSpace(file),
		strings.TrimSpace(source),
		page, slide, startLine, endLine,
	)
	sum := sha1.Sum([]byte(raw))
	return "anc_" + hex.EncodeToString(sum[:8])
}

func splitSuffix(path string) (base, suffix string) {
	if strings.Contains(path, "::") {
		parts := strings.SplitN(path, "::", 2)
		return parts[0], "::" + parts[1]
	}
	return path, ""
}
