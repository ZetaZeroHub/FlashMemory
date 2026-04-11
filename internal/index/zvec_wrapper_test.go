package index

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- ZvecWrapper unit tests ---

// TestZvecWrapperDimension tests that the wrapper reports the correct dimension
func TestZvecWrapperDimension(t *testing.T) {
	zw := &ZvecWrapper{
		Dim:         384,
		Scores:      make(map[int]float32),
		vectorCache: make(map[string][]float32),
	}
	if zw.Dimension() != 384 {
		t.Errorf("Expected dimension 384, got %d", zw.Dimension())
	}
}

// TestZvecWrapperGetScore tests score retrieval
func TestZvecWrapperGetScore(t *testing.T) {
	zw := &ZvecWrapper{
		Dim:         384,
		Scores:      make(map[int]float32),
		vectorCache: make(map[string][]float32),
	}

	// No score set
	if score := zw.GetScore(42); score != 0 {
		t.Errorf("Expected 0 for missing score, got %f", score)
	}

	// Set and retrieve score
	zw.Scores[42] = 0.95
	if score := zw.GetScore(42); score != 0.95 {
		t.Errorf("Expected 0.95, got %f", score)
	}
}

// TestZvecWrapperCacheOperations tests cache enable/disable/clear/stats
func TestZvecWrapperCacheOperations(t *testing.T) {
	zw := &ZvecWrapper{
		Dim:          384,
		Scores:       make(map[int]float32),
		vectorCache:  make(map[string][]float32),
		cacheEnabled: true,
	}

	// Test EnableCache
	zw.EnableCache()
	if !zw.cacheEnabled {
		t.Error("Expected cache to be enabled")
	}

	// Test DisableCache
	zw.DisableCache()
	if zw.cacheEnabled {
		t.Error("Expected cache to be disabled")
	}

	// Test ClearCache
	zw.vectorCache["test"] = []float32{0.1, 0.2}
	zw.ClearCache()
	if len(zw.vectorCache) != 0 {
		t.Errorf("Expected cache to be empty, got %d entries", len(zw.vectorCache))
	}

	// Test GetCacheStats
	zw.EnableCache()
	stats := zw.GetCacheStats()
	if stats["engine"] != "zvec" {
		t.Errorf("Expected engine 'zvec', got %v", stats["engine"])
	}
	if stats["enabled"] != true {
		t.Error("Expected cache enabled in stats")
	}
}

// TestZvecWrapperSimilarityMetric tests the compatibility method
func TestZvecWrapperSimilarityMetric(t *testing.T) {
	zw := &ZvecWrapper{
		Dim:         384,
		Scores:      make(map[int]float32),
		vectorCache: make(map[string][]float32),
	}

	// Should not panic
	zw.SetSimilarityMetric("cosine")
	zw.SetSimilarityMetric("euclidean")
}

// TestZvecRequestSerialization tests JSON serialization of requests
func TestZvecRequestSerialization(t *testing.T) {
	req := zvecRequest{
		Action: "search",
		Params: map[string]interface{}{
			"query": []float64{0.1, 0.2, 0.3},
			"top_k": 5,
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to serialize request: %v", err)
	}

	var parsed zvecRequest
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to parse request: %v", err)
	}

	if parsed.Action != "search" {
		t.Errorf("Expected action 'search', got '%s'", parsed.Action)
	}
}

// TestZvecResponseDeserialization tests JSON deserialization of responses
func TestZvecResponseDeserialization(t *testing.T) {
	jsonStr := `{"status":"success","data":{"results":[{"id":"func_1","score":0.95},{"id":"func_2","score":0.88}],"count":2},"message":"ok"}`

	var resp zvecResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", resp.Status)
	}
	if resp.Message != "ok" {
		t.Errorf("Expected message 'ok', got '%s'", resp.Message)
	}

	results, ok := resp.Data["results"].([]interface{})
	if !ok {
		t.Fatal("Expected results array in data")
	}
	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}
}

// TestZvecResponseErrorDeserialization tests error response parsing
func TestZvecResponseErrorDeserialization(t *testing.T) {
	jsonStr := `{"status":"error","data":{},"message":"Collection 未初始化"}`

	var resp zvecResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		t.Fatalf("Failed to parse error response: %v", err)
	}

	if resp.Status != "error" {
		t.Errorf("Expected status 'error', got '%s'", resp.Status)
	}
	if !strings.Contains(resp.Message, "未初始化") {
		t.Errorf("Expected error message about initialization, got '%s'", resp.Message)
	}
}

// TestZvecWrapperLoadFromFile tests compatibility method
func TestZvecWrapperLoadFromFile(t *testing.T) {
	zw := &ZvecWrapper{
		Dim:         384,
		Scores:      make(map[int]float32),
		vectorCache: make(map[string][]float32),
	}

	// LoadFromFile should be a no-op for Zvec (compatibility)
	err := zw.LoadFromFile("/tmp/test.faiss")
	if err != nil {
		t.Errorf("LoadFromFile should not return error, got: %v", err)
	}
}

// TestZvecWrapperSaveToFile tests save behavior when not dirty
func TestZvecWrapperSaveToFile(t *testing.T) {
	zw := &ZvecWrapper{
		Dim:         384,
		Scores:      make(map[int]float32),
		vectorCache: make(map[string][]float32),
		dirtyFlag:   false, // not dirty
	}

	// SaveToFile should skip when not dirty
	err := zw.SaveToFile("/tmp/test.faiss")
	if err != nil {
		t.Errorf("SaveToFile should not return error when not dirty, got: %v", err)
	}
}

// TestZvecWrapperFree tests free without running process
func TestZvecWrapperFree(t *testing.T) {
	zw := &ZvecWrapper{
		Dim:         384,
		Scores:      make(map[int]float32),
		vectorCache: make(map[string][]float32),
		ready:       false,
	}

	// Should not panic when process is nil
	zw.Free()
}

// TestFindBridgeScript tests script discovery logic
func TestFindBridgeScript(t *testing.T) {
	zw := &ZvecWrapper{
		Dim:         384,
		Scores:      make(map[int]float32),
		vectorCache: make(map[string][]float32),
	}

	script := zw.findBridgeScript()
	// In test environment, the script should be found relative to project root
	if script != "" {
		if _, err := os.Stat(script); err != nil {
			t.Errorf("Found script path does not exist: %s", script)
		}
		if !strings.HasSuffix(script, "zvec_bridge.py") {
			t.Errorf("Expected zvec_bridge.py, got %s", filepath.Base(script))
		}
	}
	// If script is empty, it's OK in test environment - just log
	t.Logf("Bridge script path: %s (empty is OK in test env)", script)
}

// --- Integration test with a mock Python bridge ---

// TestZvecBridgeProtocol tests the full stdin/stdout protocol communication
// by creating a small Python script that mimics the bridge protocol
func TestZvecBridgeProtocol(t *testing.T) {
	// Create a minimal Python mock bridge
	mockScript := `
import json
import sys

# Send ready signal
sys.stdout.write(json.dumps({"status": "success", "data": {}, "message": "ready"}) + "\n")
sys.stdout.flush()

for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    try:
        req = json.loads(line)
        action = req.get("action", "")
        params = req.get("params", {})
        
        if action == "ping":
            resp = {"status": "success", "data": {"engine_ready": True}, "message": "pong"}
        elif action == "init":
            resp = {"status": "success", "data": {"stats": {}, "dimension": params.get("dimension", 384)}, "message": "ok"}
        elif action == "add_vector":
            resp = {"status": "success", "data": {}, "message": "added"}
        elif action == "search":
            resp = {"status": "success", "data": {"results": [{"id": "func_1", "score": 0.95, "fields": {"func_name": "test"}}], "count": 1}, "message": "ok"}
        elif action == "shutdown":
            resp = {"status": "success", "data": {}, "message": "bye"}
            sys.stdout.write(json.dumps(resp) + "\n")
            sys.stdout.flush()
            break
        else:
            resp = {"status": "error", "data": {}, "message": "unknown action: " + action}
        
        sys.stdout.write(json.dumps(resp) + "\n")
        sys.stdout.flush()
    except Exception as e:
        resp = {"status": "error", "data": {}, "message": str(e)}
        sys.stdout.write(json.dumps(resp) + "\n")
        sys.stdout.flush()
`
	// Write the mock script to a temp file
	tmpDir := filepath.Join(os.TempDir(), "fm_test_zvec")
	os.MkdirAll(tmpDir, 0755)
	defer os.RemoveAll(tmpDir)

	scriptPath := filepath.Join(tmpDir, "mock_bridge.py")
	if err := os.WriteFile(scriptPath, []byte(mockScript), 0644); err != nil {
		t.Fatalf("Failed to write mock script: %v", err)
	}

	// Start the mock bridge as a subprocess
	cmd := exec.Command("python3", "-u", scriptPath)
	cmd.Stderr = os.Stderr

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("Failed to get stdin pipe: %v", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("Failed to get stdout pipe: %v", err)
	}

	scanner := bufio.NewScanner(stdoutPipe)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start mock bridge: %v", err)
	}

	defer func() {
		stdinPipe.Close()
		cmd.Wait()
	}()

	readResp := func() (*zvecResponse, error) {
		done := make(chan bool, 1)
		var resp zvecResponse
		var readErr error

		go func() {
			if scanner.Scan() {
				readErr = json.Unmarshal([]byte(scanner.Text()), &resp)
			} else {
				readErr = fmt.Errorf("EOF")
			}
			done <- true
		}()

		select {
		case <-done:
			return &resp, readErr
		case <-time.After(5 * time.Second):
			return nil, fmt.Errorf("timeout")
		}
	}

	sendReq := func(action string, params interface{}) (*zvecResponse, error) {
		req := zvecRequest{Action: action, Params: params}
		data, _ := json.Marshal(req)
		if _, err := io.WriteString(stdinPipe, string(data)+"\n"); err != nil {
			return nil, err
		}
		return readResp()
	}

	// Test 1: Ready signal
	t.Run("ReadySignal", func(t *testing.T) {
		resp, err := readResp()
		if err != nil {
			t.Fatalf("Failed to read ready signal: %v", err)
		}
		if resp.Status != "success" || resp.Message != "ready" {
			t.Errorf("Expected ready signal, got: %+v", resp)
		}
	})

	// Test 2: Ping
	t.Run("Ping", func(t *testing.T) {
		resp, err := sendReq("ping", map[string]interface{}{})
		if err != nil {
			t.Fatalf("Ping failed: %v", err)
		}
		if resp.Status != "success" || resp.Message != "pong" {
			t.Errorf("Expected pong, got: %+v", resp)
		}
	})

	// Test 3: Init
	t.Run("Init", func(t *testing.T) {
		resp, err := sendReq("init", map[string]interface{}{
			"collection_path": "/tmp/test",
			"dimension":       384,
		})
		if err != nil {
			t.Fatalf("Init failed: %v", err)
		}
		if resp.Status != "success" {
			t.Errorf("Expected success, got: %+v", resp)
		}
		dim, ok := resp.Data["dimension"].(float64)
		if !ok || int(dim) != 384 {
			t.Errorf("Expected dimension 384, got: %v", resp.Data["dimension"])
		}
	})

	// Test 4: Add vector
	t.Run("AddVector", func(t *testing.T) {
		vec := make([]float64, 384)
		for i := range vec {
			vec[i] = 0.1
		}
		resp, err := sendReq("add_vector", map[string]interface{}{
			"func_id":  "func_42",
			"vector":   vec,
			"metadata": map[string]interface{}{"func_name": "hello"},
		})
		if err != nil {
			t.Fatalf("AddVector failed: %v", err)
		}
		if resp.Status != "success" {
			t.Errorf("Expected success, got: %+v", resp)
		}
	})

	// Test 5: Search
	t.Run("Search", func(t *testing.T) {
		vec := make([]float64, 384)
		for i := range vec {
			vec[i] = 0.1
		}
		resp, err := sendReq("search", map[string]interface{}{
			"query": vec,
			"top_k": 5,
		})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		if resp.Status != "success" {
			t.Errorf("Expected success, got: %+v", resp)
		}

		results, ok := resp.Data["results"].([]interface{})
		if !ok {
			t.Fatal("Expected results array")
		}
		if len(results) != 1 {
			t.Errorf("Expected 1 result, got %d", len(results))
		}

		// Verify result structure
		first := results[0].(map[string]interface{})
		if first["id"] != "func_1" {
			t.Errorf("Expected id func_1, got %v", first["id"])
		}
		if score, ok := first["score"].(float64); !ok || score != 0.95 {
			t.Errorf("Expected score 0.95, got %v", first["score"])
		}
	})

	// Test 6: Unknown action
	t.Run("UnknownAction", func(t *testing.T) {
		resp, err := sendReq("nonexistent", map[string]interface{}{})
		if err != nil {
			t.Fatalf("UnknownAction failed: %v", err)
		}
		if resp.Status != "error" {
			t.Errorf("Expected error, got: %+v", resp)
		}
		if !strings.Contains(resp.Message, "nonexistent") {
			t.Errorf("Expected error message about unknown action, got: %s", resp.Message)
		}
	})

	// Test 7: Shutdown
	t.Run("Shutdown", func(t *testing.T) {
		resp, err := sendReq("shutdown", map[string]interface{}{})
		if err != nil {
			t.Fatalf("Shutdown failed: %v", err)
		}
		if resp.Status != "success" {
			t.Errorf("Expected success, got: %+v", resp)
		}
	})
}

// --- Factory method tests ---

// TestNewFaissWrapperByEngine tests the unified engine factory
func TestNewFaissWrapperByEngine(t *testing.T) {
	t.Run("DefaultEngine", func(t *testing.T) {
		// Default (faiss) should fallback to MemoryFaissWrapper when no HTTP server
		wrapper := NewFaissWrapperByEngine("", 128)
		if wrapper == nil {
			t.Fatal("Expected non-nil wrapper")
		}
		if wrapper.Dimension() != 128 {
			t.Errorf("Expected dimension 128, got %d", wrapper.Dimension())
		}
	})

	t.Run("FaissEngine", func(t *testing.T) {
		wrapper := NewFaissWrapperByEngine("faiss", 256)
		if wrapper == nil {
			t.Fatal("Expected non-nil wrapper")
		}
		// Will fallback to Memory when HTTP server is not available
		if wrapper.Dimension() != 256 {
			t.Errorf("Expected dimension 256, got %d", wrapper.Dimension())
		}
	})

	// Note: "zvec" engine test requires Python environment
	// It will fallback to MemoryFaissWrapper if zvec_bridge.py is not found
	t.Run("ZvecEngineFallback", func(t *testing.T) {
		wrapper := NewFaissWrapperByEngine("zvec", 384, map[string]interface{}{
			"collection_path": "/tmp/nonexistent_zvec_test",
		})
		if wrapper == nil {
			t.Fatal("Expected non-nil wrapper (should fallback)")
		}
		// Should fallback to memory since zvec bridge likely won't start in test env
		t.Logf("Wrapper type: %T, dimension: %d", wrapper, wrapper.Dimension())
	})
}

// TestNewZvecFaissWrapperFallback tests zvec wrapper fallback behavior
func TestNewZvecFaissWrapperFallback(t *testing.T) {
	// Should fallback to MemoryFaissWrapper when bridge can't start
	wrapper := NewZvecFaissWrapper(384, "/tmp/nonexistent_test_path", "nonexistent_python_binary")
	if wrapper == nil {
		t.Fatal("Expected non-nil wrapper")
	}
	// Verify it is a MemoryFaissWrapper (fallback)
	_, isMemory := wrapper.(*MemoryFaissWrapper)
	if !isMemory {
		t.Logf("Wrapper type: %T (may or may not be MemoryFaissWrapper depending on env)", wrapper)
	} else {
		t.Log("Correctly fell back to MemoryFaissWrapper")
	}
	if wrapper.Dimension() != 384 {
		t.Errorf("Expected dimension 384, got %d", wrapper.Dimension())
	}
}

// --- Phase 2: Hybrid Search tests ---

// TestParseSearchResults tests the shared result parsing function
func TestParseSearchResults(t *testing.T) {
	zw := &ZvecWrapper{
		Dim:         384,
		Scores:      make(map[int]float32),
		vectorCache: make(map[string][]float32),
	}

	t.Run("ValidResults", func(t *testing.T) {
		resp := &zvecResponse{
			Status:  "success",
			Message: "ok",
			Data: map[string]interface{}{
				"results": []interface{}{
					map[string]interface{}{"id": "func_10", "score": 0.95},
					map[string]interface{}{"id": "func_5", "score": 0.88},
					map[string]interface{}{"id": "func_20", "score": 0.92},
				},
			},
		}

		ids := zw.parseSearchResults(resp)
		if len(ids) != 3 {
			t.Fatalf("Expected 3 results, got %d", len(ids))
		}

		// Results should be sorted by score descending
		if ids[0] != 10 {
			t.Errorf("Expected first result to be func_10 (score=0.95), got func_%d", ids[0])
		}
		if ids[1] != 20 {
			t.Errorf("Expected second result to be func_20 (score=0.92), got func_%d", ids[1])
		}
		if ids[2] != 5 {
			t.Errorf("Expected third result to be func_5 (score=0.88), got func_%d", ids[2])
		}

		// Scores cache should be updated
		if zw.Scores[10] != 0.95 {
			t.Errorf("Expected score 0.95 for func_10, got %f", zw.Scores[10])
		}
	})

	t.Run("EmptyResults", func(t *testing.T) {
		resp := &zvecResponse{
			Status: "success",
			Data: map[string]interface{}{
				"results": []interface{}{},
			},
		}
		ids := zw.parseSearchResults(resp)
		if len(ids) != 0 {
			t.Errorf("Expected 0 results, got %d", len(ids))
		}
	})

	t.Run("InvalidFormat", func(t *testing.T) {
		resp := &zvecResponse{
			Status: "success",
			Data:   map[string]interface{}{},
		}
		ids := zw.parseSearchResults(resp)
		if len(ids) != 0 {
			t.Errorf("Expected 0 results for invalid format, got %d", len(ids))
		}
	})
}

// TestHybridSearchRequestSerialization tests JSON serialization for hybrid search
func TestHybridSearchRequestSerialization(t *testing.T) {
	req := zvecRequest{
		Action: "hybrid_search",
		Params: map[string]interface{}{
			"dense_query":  []float64{0.1, 0.2, 0.3},
			"sparse_query": map[string]float64{"hello": 0.5, "world": 0.3},
			"top_k":        10,
			"use_rrf":      true,
			"filter":       `language = "go"`,
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to serialize hybrid search request: %v", err)
	}

	var parsed zvecRequest
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to parse hybrid search request: %v", err)
	}

	if parsed.Action != "hybrid_search" {
		t.Errorf("Expected action 'hybrid_search', got '%s'", parsed.Action)
	}

	paramsMap, ok := parsed.Params.(map[string]interface{})
	if !ok {
		t.Fatal("Expected params to be a map")
	}
	if paramsMap["use_rrf"] != true {
		t.Error("Expected use_rrf to be true")
	}
	if paramsMap["filter"] != `language = "go"` {
		t.Errorf("Expected filter 'language = \"go\"', got '%v'", paramsMap["filter"])
	}
}

// TestHybridSearchResponseDeserialization tests hybrid response parsing
func TestHybridSearchResponseDeserialization(t *testing.T) {
	jsonStr := `{"status":"success","data":{"results":[{"id":"func_1","score":0.95,"fields":{"func_name":"test"}}],"count":1,"search_type":"hybrid"},"message":"ok"}`

	var resp zvecResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		t.Fatalf("Failed to parse hybrid response: %v", err)
	}

	if resp.Status != "success" {
		t.Errorf("Expected success, got %s", resp.Status)
	}

	searchType, ok := resp.Data["search_type"].(string)
	if !ok || searchType != "hybrid" {
		t.Errorf("Expected search_type 'hybrid', got: %v", resp.Data["search_type"])
	}
}

// TestZvecBridgeHybridSearchProtocol tests hybrid search via subprocess protocol
func TestZvecBridgeHybridSearchProtocol(t *testing.T) {
	// Mock bridge with hybrid_search support
	mockScript := `
import json
import sys

sys.stdout.write(json.dumps({"status": "success", "data": {}, "message": "ready"}) + "\n")
sys.stdout.flush()

for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    try:
        req = json.loads(line)
        action = req.get("action", "")
        params = req.get("params", {})

        if action == "hybrid_search":
            dense = params.get("dense_query", [])
            sparse = params.get("sparse_query")
            filter_expr = params.get("filter", "")
            use_rrf = params.get("use_rrf", True)
            
            search_type = "hybrid" if sparse else "dense_only"
            results = [
                {"id": "func_1", "score": 0.95, "fields": {"func_name": "a"}},
                {"id": "func_2", "score": 0.88, "fields": {"func_name": "b"}},
            ]
            if filter_expr:
                results = [r for r in results if True]  # simplified filter
            
            resp = {
                "status": "success",
                "data": {"results": results, "count": len(results), "search_type": search_type},
                "message": "ok"
            }
        elif action == "search":
            filter_expr = params.get("filter", "")
            results = [{"id": "func_3", "score": 0.91, "fields": {}}]
            resp = {"status": "success", "data": {"results": results, "count": 1}, "message": "ok"}
        elif action == "shutdown":
            resp = {"status": "success", "data": {}, "message": "bye"}
            sys.stdout.write(json.dumps(resp) + "\n")
            sys.stdout.flush()
            break
        else:
            resp = {"status": "error", "data": {}, "message": "unknown: " + action}

        sys.stdout.write(json.dumps(resp) + "\n")
        sys.stdout.flush()
    except Exception as e:
        sys.stdout.write(json.dumps({"status": "error", "data": {}, "message": str(e)}) + "\n")
        sys.stdout.flush()
`
	tmpDir := filepath.Join(os.TempDir(), "fm_test_zvec_phase2")
	os.MkdirAll(tmpDir, 0755)
	defer os.RemoveAll(tmpDir)

	scriptPath := filepath.Join(tmpDir, "mock_bridge_p2.py")
	if err := os.WriteFile(scriptPath, []byte(mockScript), 0644); err != nil {
		t.Fatalf("Failed to write mock script: %v", err)
	}

	cmd := exec.Command("python3", "-u", scriptPath)
	cmd.Stderr = os.Stderr

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("Failed to get stdin pipe: %v", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("Failed to get stdout pipe: %v", err)
	}

	scanner := bufio.NewScanner(stdoutPipe)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start mock bridge: %v", err)
	}
	defer func() {
		stdinPipe.Close()
		cmd.Wait()
	}()

	readResp := func() (*zvecResponse, error) {
		done := make(chan bool, 1)
		var resp zvecResponse
		var readErr error
		go func() {
			if scanner.Scan() {
				readErr = json.Unmarshal([]byte(scanner.Text()), &resp)
			} else {
				readErr = fmt.Errorf("EOF")
			}
			done <- true
		}()
		select {
		case <-done:
			return &resp, readErr
		case <-time.After(5 * time.Second):
			return nil, fmt.Errorf("timeout")
		}
	}

	sendReq := func(action string, params interface{}) (*zvecResponse, error) {
		req := zvecRequest{Action: action, Params: params}
		data, _ := json.Marshal(req)
		if _, err := io.WriteString(stdinPipe, string(data)+"\n"); err != nil {
			return nil, err
		}
		return readResp()
	}

	// Skip ready signal
	readResp()

	// Test 1: Hybrid search with dense only
	t.Run("HybridDenseOnly", func(t *testing.T) {
		vec := make([]float64, 384)
		for i := range vec {
			vec[i] = 0.1
		}
		resp, err := sendReq("hybrid_search", map[string]interface{}{
			"dense_query": vec,
			"top_k":       5,
			"use_rrf":     true,
		})
		if err != nil {
			t.Fatalf("Failed: %v", err)
		}
		if resp.Status != "success" {
			t.Errorf("Expected success, got %+v", resp)
		}
		if resp.Data["search_type"] != "dense_only" {
			t.Errorf("Expected search_type 'dense_only', got '%v'", resp.Data["search_type"])
		}
		results := resp.Data["results"].([]interface{})
		if len(results) != 2 {
			t.Errorf("Expected 2 results, got %d", len(results))
		}
	})

	// Test 2: Hybrid search with dense + sparse
	t.Run("HybridDenseAndSparse", func(t *testing.T) {
		vec := make([]float64, 384)
		for i := range vec {
			vec[i] = 0.2
		}
		resp, err := sendReq("hybrid_search", map[string]interface{}{
			"dense_query":  vec,
			"sparse_query": map[string]float64{"keyword1": 0.5, "keyword2": 0.3},
			"top_k":        10,
			"use_rrf":      true,
		})
		if err != nil {
			t.Fatalf("Failed: %v", err)
		}
		if resp.Status != "success" {
			t.Errorf("Expected success, got %+v", resp)
		}
		if resp.Data["search_type"] != "hybrid" {
			t.Errorf("Expected search_type 'hybrid', got '%v'", resp.Data["search_type"])
		}
	})

	// Test 3: Search with filter (SearchVectorsWithFilter test via protocol)
	t.Run("SearchWithFilter", func(t *testing.T) {
		vec := make([]float64, 384)
		resp, err := sendReq("search", map[string]interface{}{
			"query":  vec,
			"top_k":  5,
			"filter": `language = "go"`,
		})
		if err != nil {
			t.Fatalf("Failed: %v", err)
		}
		if resp.Status != "success" {
			t.Errorf("Expected success, got %+v", resp)
		}
	})

	// Cleanup
	sendReq("shutdown", map[string]interface{}{})
}

