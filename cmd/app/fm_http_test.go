package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
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
