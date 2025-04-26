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

// helper to create Echo server with routes for testing
func setupServer(projDir string) *echo.Echo {
	// set project directory and auth env
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	auth := middleware.BasicAuth(func(username, password string, c echo.Context) (bool, error) {
		if username == os.Getenv("API_USER") && password == os.Getenv("API_PASS") {
			return true, nil
		}
		return false, nil
	})
	api := e.Group("/api", auth)
	api.POST("/search", searchHandler(projDir))
	api.POST("/functions", listFunctionsHandler(projDir))
	api.POST("/index", buildIndexHandler(projDir))
	api.DELETE("/index", deleteIndexHandler(projDir))
	api.POST("/index/incremental", incrementalIndexHandler(projDir))
	return e
}

// helper to add BasicAuth header
func addAuth(req *http.Request) {
	user := os.Getenv("API_USER")
	pass := os.Getenv("API_PASS")
	cred := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
	req.Header.Set("Authorization", "Basic "+cred)
}

func TestSearch_Unauthorized(t *testing.T) {
	e := setupServer(".")
	// request without auth
	req := httptest.NewRequest(http.MethodPost, "/api/search", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 Unauthorized, got %d", rec.Code)
	}
}

func TestSearch_BadRequest(t *testing.T) {
	e := setupServer(".")
	// prepare invalid JSON
	req := httptest.NewRequest(http.MethodPost, "/api/search", bytes.NewBufferString("invalid-json"))
	addAuth(req)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 BadRequest for invalid JSON, got %d", rec.Code)
	}
}

func TestListFunctions_BadRequest(t *testing.T) {
	e := setupServer(".")
	// invalid JSON
	req := httptest.NewRequest(http.MethodPost, "/api/functions", bytes.NewBufferString("{"))
	addAuth(req)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 BadRequest, got %d", rec.Code)
	}
}

func TestBuildIndex_Success(t *testing.T) {
	tmp := t.TempDir()
	os.Setenv("API_USER", "u")
	os.Setenv("API_PASS", "p")
	e := setupServer(tmp)
	body, _ := json.Marshal(map[string]string{})
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
	// create dummy .gitgo
	dir := tmp + "/.gitgo"
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		t.Fatal(err)
	}
	e := setupServer(tmp)
	req := httptest.NewRequest(http.MethodDelete, "/api/index", bytes.NewBufferString("{}"))
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
	e := setupServer(tmp)
	body, _ := json.Marshal(map[string]string{"branch": "b", "commit": "c"})
	req := httptest.NewRequest(http.MethodPost, "/api/index/incremental", bytes.NewBuffer(body))
	addAuth(req)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec.Code)
	}
}
