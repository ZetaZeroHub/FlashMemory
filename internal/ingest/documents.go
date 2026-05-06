package ingest

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

var textDocumentExtensions = []string{
	".md", ".markdown", ".txt", ".rst", ".pdf", ".pptx", ".docx",
	".png", ".jpg", ".jpeg", ".webp", ".bmp", ".tif", ".tiff",
}

func SupportedTextDocumentExtensions() []string {
	out := make([]string, len(textDocumentExtensions))
	copy(out, textDocumentExtensions)
	return out
}

func IsSupportedTextDocument(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return slices.Contains(textDocumentExtensions, ext)
}

func CollectTextDocuments(target string, recursive bool) ([]string, error) {
	info, err := os.Stat(target)
	if err != nil {
		return nil, fmt.Errorf("stat target failed: %w", err)
	}

	if !info.IsDir() {
		if !IsSupportedTextDocument(target) {
			return nil, nil
		}
		return []string{target}, nil
	}

	matches := make([]string, 0, 16)
	if recursive {
		err := filepath.WalkDir(target, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				if isHiddenDir(path, target) {
					return filepath.SkipDir
				}
				return nil
			}
			if IsSupportedTextDocument(path) {
				matches = append(matches, path)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walk target failed: %w", err)
		}
		return matches, nil
	}

	entries, err := os.ReadDir(target)
	if err != nil {
		return nil, fmt.Errorf("read directory failed: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		p := filepath.Join(target, entry.Name())
		if IsSupportedTextDocument(p) {
			matches = append(matches, p)
		}
	}
	return matches, nil
}

func isHiddenDir(path string, root string) bool {
	if path == root {
		return false
	}
	name := filepath.Base(path)
	return strings.HasPrefix(name, ".")
}
