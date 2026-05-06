package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/kinglegendzzh/flashmemory/internal/analyzer"
	"github.com/kinglegendzzh/flashmemory/internal/anchor"
	"github.com/kinglegendzzh/flashmemory/internal/graph"
	"github.com/kinglegendzzh/flashmemory/internal/index"
	"github.com/kinglegendzzh/flashmemory/internal/parser"
)

// setupServer creates Echo with routes for testing
func setupServer() *echo.Echo {
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	auth := middleware.BasicAuth(func(username, password string, c echo.Context) (bool, error) {
		return username == os.Getenv("API_USER") && password == os.Getenv("API_PASS"), nil
	})
	api := e.Group("/api", auth)
	api.POST("/search", searchHandler())
	api.POST("/functions", listFunctionsHandler())
	api.POST("/index", buildIndexHandler())
	api.DELETE("/index", deleteIndexHandler())
	api.POST("/index/incremental", incrementalIndexHandler())
	api.POST("/tools/list", toolsListHandler())
	api.POST("/tools/invoke", toolsInvokeHandler())
	api.GET("/tools/mcp", toolsMCPHandler())
	api.POST("/events/query", eventsQueryHandler())
	api.POST("/substrate/object/resolve", substrateObjectResolveHandler())
	api.POST("/substrate/graph/neighbors", substrateNeighborsHandler())
	api.POST("/substrate/changes", substrateChangesHandler())
	api.POST("/substrate/project/meta", substrateProjectMetaHandler())
	api.POST("/substrate/tool-records", substrateToolRecordsHandler())
	api.POST("/substrate/doc/tree", substrateDocTreeHandler())
	api.POST("/substrate/doc/neighbors", substrateDocNeighborsHandler())
	api.POST("/substrate/doc/parse-artifacts", substrateDocParseArtifactsHandler())
	return e
}

// addAuth sets BasicAuth header
func addAuth(req *http.Request) {
	user := os.Getenv("API_USER")
	pass := os.Getenv("API_PASS")
	cred := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
	req.Header.Set("Authorization", "Basic "+cred)
}

func TestSearch_Unauthorized(t *testing.T) {
	e := setupServer()
	req := httptest.NewRequest(http.MethodPost, "/api/search", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized, got %d", rec.Code)
	}
}

func TestSearch_BadRequest(t *testing.T) {
	e := setupServer()
	// malformed JSON
	req := httptest.NewRequest(http.MethodPost, "/api/search", bytes.NewBufferString("{invalid}"))
	addAuth(req)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 BadRequest, got %d", rec.Code)
	}
}

func TestBuildIndex_Success(t *testing.T) {
	tmp := t.TempDir()
	os.Setenv("API_USER", "u")
	os.Setenv("API_PASS", "p")
	e := setupServer()
	// include required project_dir in body
	body, _ := json.Marshal(map[string]string{"project_dir": tmp})
	req := httptest.NewRequest(http.MethodPost, "/api/index", bytes.NewBuffer(body))
	addAuth(req)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec.Code)
	}
}

func TestDeleteIndex_Success(t *testing.T) {
	tmp := t.TempDir()
	os.Setenv("API_USER", "u")
	os.Setenv("API_PASS", "p")
	// create dummy .gitgo in tmp
	dir := tmp + "/.gitgo"
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	e := setupServer()
	// include project_dir
	reqBody, _ := json.Marshal(map[string]string{"project_dir": tmp})
	req := httptest.NewRequest(http.MethodDelete, "/api/index", bytes.NewBuffer(reqBody))
	addAuth(req)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec.Code)
	}
	// verify deletion
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("expected .gitgo removed, but exists")
	}
}

func TestIncrementalIndex_Success(t *testing.T) {
	tmp := t.TempDir()
	os.Setenv("API_USER", "u")
	os.Setenv("API_PASS", "p")
	e := setupServer()
	// include project_dir, branch, commit
	body, _ := json.Marshal(map[string]string{"project_dir": tmp, "branch": "b", "commit": "c"})
	req := httptest.NewRequest(http.MethodPost, "/api/index/incremental", bytes.NewBuffer(body))
	addAuth(req)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec.Code)
	}
}

func TestToolsList_Success(t *testing.T) {
	os.Setenv("API_USER", "u")
	os.Setenv("API_PASS", "p")
	e := setupServer()
	req := httptest.NewRequest(http.MethodPost, "/api/tools/list", bytes.NewBufferString(`{}`))
	addAuth(req)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("fm.intent.route")) {
		t.Fatalf("expected fm.intent.route in response, got: %s", rec.Body.String())
	}
}

func TestToolsInvoke_Success(t *testing.T) {
	os.Setenv("API_USER", "u")
	os.Setenv("API_PASS", "p")
	e := setupServer()
	body := `{"tool":"fm.intent.route","input":{"query":"分析依赖关系图"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/tools/invoke", bytes.NewBufferString(body))
	addAuth(req)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"success":true`)) {
		t.Fatalf("expected success=true in invoke response, got: %s", rec.Body.String())
	}
}

func TestToolsMCP_Success(t *testing.T) {
	os.Setenv("API_USER", "u")
	os.Setenv("API_PASS", "p")
	e := setupServer()
	req := httptest.NewRequest(http.MethodGet, "/api/tools/mcp", nil)
	addAuth(req)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"type":"function"`)) {
		t.Fatalf("expected function tool schema in response, got: %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"name":"fm.intent.route"`)) {
		t.Fatalf("expected fm.intent.route in schema response, got: %s", rec.Body.String())
	}
}

func TestEventsQuery_Success(t *testing.T) {
	tmp := t.TempDir()
	os.Setenv("API_USER", "u")
	os.Setenv("API_PASS", "p")
	e := setupServer()

	invokeBody := fmt.Sprintf(`{"project_dir":"%s","request_id":"req-evt-1","trace_id":"trace-evt-1","tool":"fm.intent.route","input":{"query":"分析依赖"}}`, tmp)
	invokeReq := httptest.NewRequest(http.MethodPost, "/api/tools/invoke", bytes.NewBufferString(invokeBody))
	addAuth(invokeReq)
	invokeRec := httptest.NewRecorder()
	e.ServeHTTP(invokeRec, invokeReq)
	if invokeRec.Code != http.StatusOK {
		t.Fatalf("expected invoke 200 OK, got %d", invokeRec.Code)
	}

	queryBody := fmt.Sprintf(`{"project_dir":"%s","event_type":"tool_invoke","request_id":"req-evt-1","trace_id":"trace-evt-1","limit":10}`, tmp)
	req := httptest.NewRequest(http.MethodPost, "/api/events/query", bytes.NewBufferString(queryBody))
	addAuth(req)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"event_type":"tool_invoke"`)) {
		t.Fatalf("expected tool_invoke event in response, got: %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"request_id":"req-evt-1"`)) {
		t.Fatalf("expected request_id in response, got: %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"trace_id":"trace-evt-1"`)) {
		t.Fatalf("expected trace_id in response, got: %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"schema_version":"v1"`)) {
		t.Fatalf("expected schema_version in response, got: %s", rec.Body.String())
	}
}

func writeTestGraph(t *testing.T, projectDir string) {
	t.Helper()
	gitgoDir := filepath.Join(projectDir, ".gitgo")
	if err := os.MkdirAll(gitgoDir, 0755); err != nil {
		t.Fatalf("mkdir .gitgo failed: %v", err)
	}
	fnA := parser.FunctionInfo{
		Name:      "A",
		Package:   "pkg",
		File:      "pkg/a.go",
		Source:    "pkg/a.go",
		StartLine: 10,
		EndLine:   20,
	}
	fnB := parser.FunctionInfo{
		Name:      "B",
		Package:   "pkg",
		File:      "pkg/b.go",
		Source:    "pkg/b.go",
		StartLine: 30,
		EndLine:   45,
	}
	anchorA := anchor.BuildID(fnA.Name, fnA.Package, fnA.FunctionType, fnA.File, fnA.Source, fnA.Page, fnA.Slide, fnA.StartLine, fnA.EndLine)
	anchorB := anchor.BuildID(fnB.Name, fnB.Package, fnB.FunctionType, fnB.File, fnB.Source, fnB.Page, fnB.Slide, fnB.StartLine, fnB.EndLine)

	kg := graph.KnowledgeGraph{
		Functions: []analyzer.LLMAnalysisResult{
			{Func: fnA},
			{Func: fnB},
		},
		Anchors: map[string]anchor.Locator{
			anchorA: {
				ID:        anchorA,
				Name:      fnA.Name,
				Package:   fnA.Package,
				File:      fnA.File,
				Source:    fnA.Source,
				StartLine: fnA.StartLine,
				EndLine:   fnA.EndLine,
			},
			anchorB: {
				ID:        anchorB,
				Name:      fnB.Name,
				Package:   fnB.Package,
				File:      fnB.File,
				Source:    fnB.Source,
				StartLine: fnB.StartLine,
				EndLine:   fnB.EndLine,
			},
		},
		Calls:     map[string][]string{"pkg.A": []string{"pkg.B"}},
		CalledBy:  map[string][]string{"pkg.B": []string{"pkg.A"}},
		Packages:  map[string][]string{"pkg": []string{"pkg.A", "pkg.B"}},
		Externals: map[string][]string{},
	}
	raw, err := json.Marshal(kg)
	if err != nil {
		t.Fatalf("marshal graph failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitgoDir, "graph.json"), raw, 0644); err != nil {
		t.Fatalf("write graph.json failed: %v", err)
	}
}

func TestSubstrateObjectResolve_Success(t *testing.T) {
	tmp := t.TempDir()
	writeTestGraph(t, tmp)
	os.Setenv("API_USER", "u")
	os.Setenv("API_PASS", "p")
	e := setupServer()

	body := fmt.Sprintf(`{"project_dir":"%s","locator":"pkg.A"}`, tmp)
	req := httptest.NewRequest(http.MethodPost, "/api/substrate/object/resolve", bytes.NewBufferString(body))
	addAuth(req)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"name":"A"`)) {
		t.Fatalf("expected resolved object A, got: %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"object_type":"function"`)) {
		t.Fatalf("expected object_type=function, got: %s", rec.Body.String())
	}
}

func TestSubstrateNeighbors_Success(t *testing.T) {
	tmp := t.TempDir()
	writeTestGraph(t, tmp)
	os.Setenv("API_USER", "u")
	os.Setenv("API_PASS", "p")
	e := setupServer()

	body := fmt.Sprintf(`{"project_dir":"%s","locator":"pkg.A","direction":"both","limit":10}`, tmp)
	req := httptest.NewRequest(http.MethodPost, "/api/substrate/graph/neighbors", bytes.NewBufferString(body))
	addAuth(req)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"relation":"calls"`)) {
		t.Fatalf("expected calls relation, got: %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"name":"B"`)) {
		t.Fatalf("expected neighbor B, got: %s", rec.Body.String())
	}
}

func TestSubstrateProjectMeta_Success(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ".gitgo"), 0755); err != nil {
		t.Fatalf("mkdir .gitgo failed: %v", err)
	}
	os.Setenv("API_USER", "u")
	os.Setenv("API_PASS", "p")
	e := setupServer()

	body := fmt.Sprintf(`{"project_dir":"%s"}`, tmp)
	req := httptest.NewRequest(http.MethodPost, "/api/substrate/project/meta", bytes.NewBufferString(body))
	addAuth(req)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"schema_version":"v1"`)) {
		t.Fatalf("expected schema_version in meta, got: %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"gitgo_ready":true`)) {
		t.Fatalf("expected gitgo_ready=true, got: %s", rec.Body.String())
	}
}

func TestSubstrateToolRecords_Success(t *testing.T) {
	tmp := t.TempDir()
	os.Setenv("API_USER", "u")
	os.Setenv("API_PASS", "p")
	e := setupServer()

	invokeBody := fmt.Sprintf(`{"project_dir":"%s","request_id":"req-sub-1","trace_id":"trace-sub-1","tool":"fm.intent.route","input":{"query":"分析依赖"}}`, tmp)
	invokeReq := httptest.NewRequest(http.MethodPost, "/api/tools/invoke", bytes.NewBufferString(invokeBody))
	addAuth(invokeReq)
	invokeRec := httptest.NewRecorder()
	e.ServeHTTP(invokeRec, invokeReq)
	if invokeRec.Code != http.StatusOK {
		t.Fatalf("expected invoke 200 OK, got %d", invokeRec.Code)
	}

	queryBody := fmt.Sprintf(`{"project_dir":"%s","trace_id":"trace-sub-1","limit":10}`, tmp)
	req := httptest.NewRequest(http.MethodPost, "/api/substrate/tool-records", bytes.NewBufferString(queryBody))
	addAuth(req)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"event_type":"tool_invoke"`)) {
		t.Fatalf("expected tool_invoke in response, got: %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"trace_id":"trace-sub-1"`)) {
		t.Fatalf("expected trace_id in response, got: %s", rec.Body.String())
	}
}

func TestSubstrateDocTree_Success(t *testing.T) {
	tmp := t.TempDir()
	os.Setenv("API_USER", "u")
	os.Setenv("API_PASS", "p")
	e := setupServer()

	db, err := index.EnsureIndexDB(tmp)
	if err != nil {
		t.Fatalf("EnsureIndexDB failed: %v", err)
	}
	results := []analyzer.LLMAnalysisResult{
		{
			Func: parser.FunctionInfo{
				Name:         "doc_section_1_intro",
				File:         "docs/guide.md",
				Source:       "docs/guide.md",
				StartLine:    1,
				EndLine:      3,
				FunctionType: "llm_parser",
				CodeSnippet:  "# Intro\nhello",
			},
		},
	}
	if err := index.PersistDocHierarchyFromResults(db, tmp, results); err != nil {
		t.Fatalf("PersistDocHierarchyFromResults failed: %v", err)
	}
	_ = db.Close()

	body := fmt.Sprintf(`{"project_dir":"%s","source":"docs/guide.md"}`, tmp)
	req := httptest.NewRequest(http.MethodPost, "/api/substrate/doc/tree", bytes.NewBufferString(body))
	addAuth(req)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"node_type":"document"`)) {
		t.Fatalf("expected document node in response, got: %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"edge_type":"contains"`)) {
		t.Fatalf("expected contains edge in response, got: %s", rec.Body.String())
	}
}

func TestSubstrateDocTree_ResolvesBaseSourceForAnchoredSources(t *testing.T) {
	tmp := t.TempDir()
	os.Setenv("API_USER", "u")
	os.Setenv("API_PASS", "p")
	e := setupServer()

	db, err := index.EnsureIndexDB(tmp)
	if err != nil {
		t.Fatalf("EnsureIndexDB failed: %v", err)
	}
	results := []analyzer.LLMAnalysisResult{
		{
			Func: parser.FunctionInfo{
				Name:         "doc_section_1_pdf",
				File:         "docs/report.pdf",
				Source:       "docs/report.pdf::page_1",
				Page:         1,
				StartLine:    1,
				EndLine:      5,
				FunctionType: "llm_parser",
				CodeSnippet:  "Overview\nbody",
			},
		},
	}
	if err := index.PersistDocHierarchyFromResults(db, tmp, results); err != nil {
		t.Fatalf("PersistDocHierarchyFromResults failed: %v", err)
	}
	_ = db.Close()

	body := fmt.Sprintf(`{"project_dir":"%s","source":"docs/report.pdf"}`, tmp)
	req := httptest.NewRequest(http.MethodPost, "/api/substrate/doc/tree", bytes.NewBufferString(body))
	addAuth(req)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"node_type":"document"`)) {
		t.Fatalf("expected document node for base pdf source, got: %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`docs/report.pdf::page_1`)) {
		t.Fatalf("expected anchored pdf source in response, got: %s", rec.Body.String())
	}
}

func TestSubstrateDocTree_ResolvesBaseSourceForPPTXAnchoredSources(t *testing.T) {
	tmp := t.TempDir()
	os.Setenv("API_USER", "u")
	os.Setenv("API_PASS", "p")
	e := setupServer()

	db, err := index.EnsureIndexDB(tmp)
	if err != nil {
		t.Fatalf("EnsureIndexDB failed: %v", err)
	}
	results := []analyzer.LLMAnalysisResult{
		{
			Func: parser.FunctionInfo{
				Name:         "doc_section_1_slide",
				File:         "slides/deck.pptx",
				Source:       "slides/deck.pptx::ppt/slides/slide1.xml",
				Slide:        1,
				StartLine:    1,
				EndLine:      3,
				FunctionType: "llm_parser",
				CodeSnippet:  "Title\nbody",
			},
		},
	}
	if err := index.PersistDocHierarchyFromResults(db, tmp, results); err != nil {
		t.Fatalf("PersistDocHierarchyFromResults failed: %v", err)
	}
	_ = db.Close()

	body := fmt.Sprintf(`{"project_dir":"%s","source":"slides/deck.pptx"}`, tmp)
	req := httptest.NewRequest(http.MethodPost, "/api/substrate/doc/tree", bytes.NewBufferString(body))
	addAuth(req)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"node_type":"document"`)) {
		t.Fatalf("expected document node for base pptx source, got: %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`slides/deck.pptx::ppt/slides/slide1.xml`)) {
		t.Fatalf("expected anchored pptx source in response, got: %s", rec.Body.String())
	}
}

func TestSubstrateDocParseArtifacts_Success(t *testing.T) {
	tmp := t.TempDir()
	os.Setenv("API_USER", "u")
	os.Setenv("API_PASS", "p")
	e := setupServer()

	db, err := index.EnsureIndexDB(tmp)
	if err != nil {
		t.Fatalf("EnsureIndexDB failed: %v", err)
	}
	if err := index.RecordParseArtifact(db, tmp, "docs/guide.md", index.ParseArtifactStatusSuccess, "", "", "none", `{"doc_tree":"persisted"}`); err != nil {
		t.Fatalf("RecordParseArtifact failed: %v", err)
	}
	_ = db.Close()

	body := fmt.Sprintf(`{"project_dir":"%s","status":"success","limit":10}`, tmp)
	req := httptest.NewRequest(http.MethodPost, "/api/substrate/doc/parse-artifacts", bytes.NewBufferString(body))
	addAuth(req)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"status":"success"`)) {
		t.Fatalf("expected success status in response, got: %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"source":"docs/guide.md"`)) {
		t.Fatalf("expected source in response, got: %s", rec.Body.String())
	}
}

func TestSubstrateDocNeighbors_References_Success(t *testing.T) {
	tmp := t.TempDir()
	os.Setenv("API_USER", "u")
	os.Setenv("API_PASS", "p")
	e := setupServer()

	db, err := index.EnsureIndexDB(tmp)
	if err != nil {
		t.Fatalf("EnsureIndexDB failed: %v", err)
	}
	results := []analyzer.LLMAnalysisResult{
		{
			Func: parser.FunctionInfo{
				Name:         "doc_section_1_intro",
				File:         "docs/guide.md",
				Source:       "docs/guide.md",
				StartLine:    1,
				EndLine:      6,
				FunctionType: "llm_parser",
				CodeSnippet:  "# Intro\nsee [setup](#setup)",
			},
		},
		{
			Func: parser.FunctionInfo{
				Name:         "doc_section_2_setup",
				File:         "docs/guide.md",
				Source:       "docs/guide.md",
				StartLine:    7,
				EndLine:      10,
				FunctionType: "llm_parser",
				CodeSnippet:  "## Setup\nsteps",
			},
		},
	}
	if err := index.PersistDocHierarchyFromResults(db, tmp, results); err != nil {
		t.Fatalf("PersistDocHierarchyFromResults failed: %v", err)
	}
	var nodeID string
	if err := db.QueryRow("SELECT node_id FROM doc_nodes WHERE project_dir = ? AND title = ? LIMIT 1", tmp, "doc_section_1_intro").Scan(&nodeID); err != nil {
		t.Fatalf("query node_id failed: %v", err)
	}
	_ = db.Close()

	body := fmt.Sprintf(`{"project_dir":"%s","node_id":"%s","direction":"out","edge_type":"references","limit":10}`, tmp, nodeID)
	req := httptest.NewRequest(http.MethodPost, "/api/substrate/doc/neighbors", bytes.NewBufferString(body))
	addAuth(req)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"edge_type":"references"`)) {
		t.Fatalf("expected references edge, got: %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"evidence":"#setup"`)) {
		t.Fatalf("expected markdown anchor evidence, got: %s", rec.Body.String())
	}
}
