package docs

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// PandocConverter wraps the pandoc CLI with content-addressed caching and a
// per-source-path mutex so concurrent index runs don't trample each other.
//
// Cache key = sha256(absPath + "\0" + size + "\0" + mtimeNanos). When the
// underlying file changes, the key changes, so stale cache entries are
// effectively orphaned (cleaned up out-of-band by callers if needed).
type PandocConverter struct {
	Bin      string
	CacheDir string

	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// IsAvailable reports whether the pandoc binary is callable.
func (c *PandocConverter) IsAvailable() bool {
	bin := c.Bin
	if bin == "" {
		bin = "pandoc"
	}
	_, err := exec.LookPath(bin)
	return err == nil
}

// ConvertToMarkdown produces a GFM markdown rendering of path. Subsequent calls
// for the same source content are served from disk cache.
func (c *PandocConverter) ConvertToMarkdown(path string) (string, error) {
	if !c.IsAvailable() {
		return "", errPandocUnavailable
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("pandoc abs path: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("pandoc stat: %w", err)
	}

	cacheKey := pandocCacheKey(abs, info)
	cachePath := filepath.Join(c.cacheRoot(), cacheKey+".md")

	// Hold a per-source lock so two goroutines converting the same file
	// don't both invoke pandoc and race each other on the cache file.
	mu := c.lockFor(cacheKey)
	mu.Lock()
	defer mu.Unlock()

	if data, err := os.ReadFile(cachePath); err == nil {
		return string(data), nil
	}

	if err := os.MkdirAll(c.cacheRoot(), 0o755); err != nil {
		return "", fmt.Errorf("pandoc mkdir cache: %w", err)
	}

	bin := c.Bin
	if bin == "" {
		bin = "pandoc"
	}
	args := []string{
		"-t", "gfm",
		"--wrap=none",
		"--markdown-headings=atx",
		abs,
		"-o", cachePath,
	}
	// Pandoc infers the input format from the file extension; only override
	// when the extension is ambiguous (e.g., bare ".txt" or unknown types).
	if fmtName := pandocInputFormat(abs); fmtName != "" {
		args = append([]string{"-f", fmtName}, args...)
	}
	cmd := exec.Command(bin, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("pandoc failed: %s", string(out))
		}
		return "", fmt.Errorf("pandoc failed: %w", err)
	}

	data, err := os.ReadFile(cachePath)
	if err != nil {
		return "", fmt.Errorf("pandoc read cache: %w", err)
	}
	return string(data), nil
}

// SplitFromPandoc converts an arbitrary supported document to markdown via
// pandoc and runs it through the markdown splitter. The orchestrator uses
// this as the auto / force fallback path.
func (c *PandocConverter) SplitFromPandoc(path string, cfg Config) ([]Section, error) {
	md, err := c.ConvertToMarkdown(path)
	if err != nil {
		return nil, err
	}

	tmp, err := os.CreateTemp("", "fm-pandoc-*.md")
	if err != nil {
		return nil, fmt.Errorf("pandoc tempfile: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.WriteString(md); err != nil {
		tmp.Close()
		return nil, fmt.Errorf("pandoc tempfile write: %w", err)
	}
	tmp.Close()

	sections, err := SplitMarkdown(tmpPath, cfg)
	if err != nil {
		return nil, err
	}
	// Re-anchor source to the original document so downstream surfacing is
	// meaningful (e.g., 客服文档.docx::pandoc rather than tmp/xxx.md).
	source := fmt.Sprintf("%s::pandoc", filepath.ToSlash(path))
	for i := range sections {
		sections[i].Source = source
	}
	return sections, nil
}

func (c *PandocConverter) cacheRoot() string {
	if c.CacheDir != "" {
		return c.CacheDir
	}
	return filepath.Join(".gitgo", "pandoc_cache")
}

func (c *PandocConverter) lockFor(key string) *sync.Mutex {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.locks == nil {
		c.locks = make(map[string]*sync.Mutex)
	}
	if mu, ok := c.locks[key]; ok {
		return mu
	}
	mu := &sync.Mutex{}
	c.locks[key] = mu
	return mu
}

// pandocInputFormat maps a file extension to a pandoc -f value. Returns empty
// string when extension-based inference is sufficient.
func pandocInputFormat(path string) string {
	switch ext := filepath.Ext(path); ext {
	case ".docx":
		return "docx"
	case ".pdf":
		return ""
	case ".pptx":
		return "pptx"
	case ".md", ".markdown":
		return "gfm"
	case ".html", ".htm":
		return "html"
	case ".rst":
		return "rst"
	case ".epub":
		return "epub"
	}
	return ""
}

func pandocCacheKey(abs string, info os.FileInfo) string {
	hasher := sha256.New()
	fmt.Fprintf(hasher, "%s\x00%d\x00%d", abs, info.Size(), info.ModTime().UnixNano())
	_ = time.Now() // import marker
	return hex.EncodeToString(hasher.Sum(nil))[:32]
}

var errPandocUnavailable = errors.New("pandoc binary not found on PATH")
