package index

import (
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/kinglegendzzh/flashmemory/internal/analyzer"
)

const (
	DocNodeTypeDocument   = "document"
	DocNodeTypeChapter    = "chapter"
	DocNodeTypeSection    = "section"
	DocNodeTypeSubsection = "subsection"
	DocNodeTypeChunk      = "chunk"

	ParseArtifactStatusSuccess  = "success"
	ParseArtifactStatusDegraded = "degraded"
	ParseArtifactStatusFailed   = "failed"
)

func ensureDocSchema(db *sql.DB) error {
	schema := `
CREATE TABLE IF NOT EXISTS doc_nodes (
  node_id TEXT PRIMARY KEY,
  project_dir TEXT NOT NULL,
  doc_id TEXT NOT NULL,
  parent_id TEXT,
  node_type TEXT NOT NULL,
  level INTEGER NOT NULL,
  title TEXT,
  content TEXT,
  source TEXT NOT NULL,
  page INTEGER DEFAULT 0,
  slide INTEGER DEFAULT 0,
  start_line INTEGER DEFAULT 0,
  end_line INTEGER DEFAULT 0,
  anchor_id TEXT,
  parse_quality REAL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_doc_nodes_doc_id_level ON doc_nodes(doc_id, level);
CREATE INDEX IF NOT EXISTS idx_doc_nodes_parent_id ON doc_nodes(parent_id);
CREATE INDEX IF NOT EXISTS idx_doc_nodes_anchor_id ON doc_nodes(anchor_id);

CREATE TABLE IF NOT EXISTS doc_edges (
  edge_id TEXT PRIMARY KEY,
  project_dir TEXT NOT NULL,
  doc_id TEXT NOT NULL,
  from_node_id TEXT NOT NULL,
  to_node_id TEXT NOT NULL,
  edge_type TEXT NOT NULL,
  weight REAL DEFAULT 1.0,
  evidence TEXT,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_doc_edges_doc_id_type ON doc_edges(doc_id, edge_type);
CREATE INDEX IF NOT EXISTS idx_doc_edges_from_to ON doc_edges(from_node_id, to_node_id);

CREATE TABLE IF NOT EXISTS parse_artifacts (
  artifact_id TEXT PRIMARY KEY,
  project_dir TEXT NOT NULL,
  source TEXT NOT NULL,
  mime_type TEXT,
  status TEXT NOT NULL,
  error_code TEXT,
  error_message TEXT,
  fallback_mode TEXT,
  quality_json TEXT,
  created_at TEXT NOT NULL
);
`
	_, err := db.Exec(schema)
	return err
}

func buildHashID(parts ...string) string {
	raw := strings.Join(parts, "|")
	sum := sha1.Sum([]byte(raw))
	return hex.EncodeToString(sum[:8])
}

func deriveDocID(projectDir, source, file string) string {
	base := strings.TrimSpace(source)
	if base == "" {
		base = strings.TrimSpace(file)
	}
	base = strings.Split(base, "::")[0]
	base = filepath.ToSlash(base)
	return "doc_" + buildHashID(projectDir, base)
}

func headingLevelFromSnippet(snippet string) int {
	lines := strings.Split(snippet, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "#") {
			return 0
		}
		n := 0
		for i := 0; i < len(line); i++ {
			if line[i] == '#' {
				n++
				continue
			}
			break
		}
		if n < 1 {
			return 0
		}
		if n > 6 {
			n = 6
		}
		return n
	}
	return 0
}

func isMarkdownPath(path string) bool {
	p := strings.ToLower(strings.TrimSpace(path))
	return strings.HasSuffix(p, ".md") || strings.HasSuffix(p, ".markdown")
}

var markdownLinkPattern = regexp.MustCompile(`\[[^\]]+\]\(([^)]+)\)`)

// PersistDocHierarchyFromResults writes minimal document hierarchy nodes/edges from parsed doc chunks.
func PersistDocHierarchyFromResults(db *sql.DB, projectDir string, results []analyzer.LLMAnalysisResult) error {
	if err := ensureDocSchema(db); err != nil {
		return err
	}

	type item struct {
		res analyzer.LLMAnalysisResult
	}
	grouped := map[string][]item{}
	for _, res := range results {
		if res.Func.FunctionType != "llm_parser" {
			continue
		}
		docID := deriveDocID(projectDir, res.Func.Source, res.Func.File)
		grouped[docID] = append(grouped[docID], item{res: res})
	}
	if len(grouped) == 0 {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().Format(time.RFC3339Nano)
	for docID := range grouped {
		if _, err := tx.Exec("DELETE FROM doc_edges WHERE project_dir = ? AND doc_id = ?", projectDir, docID); err != nil {
			return err
		}
		if _, err := tx.Exec("DELETE FROM doc_nodes WHERE project_dir = ? AND doc_id = ?", projectDir, docID); err != nil {
			return err
		}
	}

	nodeStmt, err := tx.Prepare(`
INSERT OR REPLACE INTO doc_nodes(
  node_id, project_dir, doc_id, parent_id, node_type, level, title, content, source, page, slide, start_line, end_line, anchor_id, parse_quality, created_at, updated_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer nodeStmt.Close()

	edgeStmt, err := tx.Prepare(`
INSERT OR REPLACE INTO doc_edges(
  edge_id, project_dir, doc_id, from_node_id, to_node_id, edge_type, weight, evidence, created_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer edgeStmt.Close()

	for docID, items := range grouped {
		sort.Slice(items, func(i, j int) bool {
			a := items[i].res.Func
			b := items[j].res.Func
			if a.Page != b.Page {
				return a.Page < b.Page
			}
			if a.Slide != b.Slide {
				return a.Slide < b.Slide
			}
			return a.StartLine < b.StartLine
		})

		rootSource := items[0].res.Func.Source
		if strings.TrimSpace(rootSource) == "" {
			rootSource = items[0].res.Func.File
		}
		rootID := "node_" + buildHashID(projectDir, docID, "root")
		if _, err := nodeStmt.Exec(rootID, projectDir, docID, nil, DocNodeTypeDocument, 0, docID, "", rootSource, 0, 0, 0, 0, "", 1.0, now, now); err != nil {
			return err
		}

		levelParent := map[int]string{0: rootID}
		prevNode := ""
		for idx, it := range items {
			fn := it.res.Func
			level := 1
			nodeType := DocNodeTypeChunk
			if isMarkdownPath(fn.File) || isMarkdownPath(fn.Source) {
				if h := headingLevelFromSnippet(fn.CodeSnippet); h > 0 {
					level = h
					switch {
					case h <= 1:
						nodeType = DocNodeTypeChapter
					case h == 2:
						nodeType = DocNodeTypeSection
					default:
						nodeType = DocNodeTypeSubsection
					}
				}
			}

			parentID := rootID
			for p := level - 1; p >= 0; p-- {
				if candidate, ok := levelParent[p]; ok {
					parentID = candidate
					break
				}
			}

			nodeID := "node_" + buildHashID(projectDir, docID, fmt.Sprintf("%d", idx), fn.Name, fn.Source, fmt.Sprintf("%d", fn.StartLine), fmt.Sprintf("%d", fn.Page), fmt.Sprintf("%d", fn.Slide))
			source := fn.Source
			if strings.TrimSpace(source) == "" {
				source = fn.File
			}
			if _, err := nodeStmt.Exec(
				nodeID, projectDir, docID, parentID, nodeType, level,
				fn.Name, fn.CodeSnippet, source, fn.Page, fn.Slide, fn.StartLine, fn.EndLine, "", 0.8, now, now,
			); err != nil {
				return err
			}
			containsID := "edge_" + buildHashID(projectDir, docID, parentID, nodeID, "contains")
			if _, err := edgeStmt.Exec(containsID, projectDir, docID, parentID, nodeID, "contains", 1.0, "", now); err != nil {
				return err
			}
			if prevNode != "" {
				followsID := "edge_" + buildHashID(projectDir, docID, prevNode, nodeID, "follows")
				if _, err := edgeStmt.Exec(followsID, projectDir, docID, prevNode, nodeID, "follows", 1.0, "", now); err != nil {
					return err
				}
			}
			prevNode = nodeID
			levelParent[level] = nodeID

			// Minimal references extraction for markdown links.
			if isMarkdownPath(fn.File) || isMarkdownPath(fn.Source) {
				matches := markdownLinkPattern.FindAllStringSubmatch(fn.CodeSnippet, -1)
				for _, m := range matches {
					if len(m) < 2 {
						continue
					}
					target := strings.TrimSpace(m[1])
					if target == "" {
						continue
					}
					// MVP behavior:
					// - in-doc anchor (#foo): link to current document root
					// - external .md path: also link to current root with evidence target
					if strings.HasPrefix(target, "#") || strings.HasSuffix(strings.ToLower(strings.Split(target, "#")[0]), ".md") || strings.HasSuffix(strings.ToLower(strings.Split(target, "#")[0]), ".markdown") {
						refID := "edge_" + buildHashID(projectDir, docID, nodeID, rootID, "references", target)
						if _, err := edgeStmt.Exec(refID, projectDir, docID, nodeID, rootID, "references", 0.6, target, now); err != nil {
							return err
						}
					}
				}
			}
		}
	}

	return tx.Commit()
}

// ResolveDocIDBySource resolves a document id from either an exact internal
// source or its base source before any :: page/slide/ocr anchor suffix.
func ResolveDocIDBySource(db *sql.DB, projectDir, source string) (string, error) {
	if err := ensureDocSchema(db); err != nil {
		return "", err
	}
	normalized := filepath.ToSlash(strings.TrimSpace(source))
	if normalized == "" {
		return "", sql.ErrNoRows
	}
	base := strings.Split(normalized, "::")[0]
	var docID string
	err := db.QueryRow(`
SELECT doc_id
FROM doc_nodes
WHERE project_dir = ?
  AND (source = ? OR source = ? OR source LIKE ? ESCAPE '\')
ORDER BY
  CASE
    WHEN source = ? THEN 0
    WHEN source = ? THEN 1
    ELSE 2
  END,
  level ASC,
  page ASC,
  slide ASC,
  start_line ASC
LIMIT 1`,
		projectDir,
		normalized,
		base,
		escapeLikePattern(base)+"::%",
		normalized,
		base,
	).Scan(&docID)
	if err != nil {
		return "", err
	}
	return docID, nil
}

func escapeLikePattern(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(value)
}

func guessMimeType(source string) string {
	s := strings.ToLower(strings.TrimSpace(source))
	ext := path.Ext(strings.Split(s, "::")[0])
	switch ext {
	case ".md", ".markdown":
		return "text/markdown"
	case ".txt", ".rst":
		return "text/plain"
	case ".pdf":
		return "application/pdf"
	case ".pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	case ".tif", ".tiff":
		return "image/tiff"
	default:
		return ""
	}
}

func RecordParseArtifact(
	db *sql.DB,
	projectDir, source, status, errorCode, errorMessage, fallbackMode, qualityJSON string,
) error {
	if err := ensureDocSchema(db); err != nil {
		return err
	}
	now := time.Now().Format(time.RFC3339Nano)
	artifactID := "art_" + buildHashID(projectDir, source, now, status)
	_, err := db.Exec(`
INSERT INTO parse_artifacts(
  artifact_id, project_dir, source, mime_type, status, error_code, error_message, fallback_mode, quality_json, created_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		artifactID, projectDir, source, guessMimeType(source), status, errorCode, errorMessage, fallbackMode, qualityJSON, now,
	)
	return err
}

func RecordParseArtifactSuccessFromResults(db *sql.DB, projectDir string, results []analyzer.LLMAnalysisResult) error {
	type agg struct {
		chunkTotal  int
		titledTotal int
	}
	seen := map[string]*agg{}
	for _, res := range results {
		if res.Func.FunctionType != "llm_parser" {
			continue
		}
		source := strings.TrimSpace(res.Func.Source)
		if source == "" {
			source = strings.TrimSpace(res.Func.File)
		}
		if source == "" {
			continue
		}
		if _, ok := seen[source]; !ok {
			seen[source] = &agg{}
		}
		seen[source].chunkTotal++
		if strings.Contains(strings.ToLower(res.Func.Name), "section") {
			seen[source].titledTotal++
		}
	}
	for source, a := range seen {
		titleRatio := 0.0
		if a.chunkTotal > 0 {
			titleRatio = float64(a.titledTotal) / float64(a.chunkTotal)
		}
		quality := map[string]interface{}{
			"doc_tree":             "persisted",
			"chunk_total":          a.chunkTotal,
			"titled_chunk_total":   a.titledTotal,
			"title_candidate_rate": titleRatio,
		}
		raw, _ := json.Marshal(quality)
		if err := RecordParseArtifact(
			db,
			projectDir,
			source,
			ParseArtifactStatusSuccess,
			"",
			"",
			"none",
			string(raw),
		); err != nil {
			return err
		}
	}
	return nil
}
