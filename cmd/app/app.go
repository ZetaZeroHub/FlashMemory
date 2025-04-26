package main

import (
	"encoding/json"
	"fmt"
	"github.com/kinglegendzzh/flashmemory/cmd/app/back"
	"log"
	"net/http"
	"os"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/kinglegendzzh/flashmemory/internal/index"
	"github.com/kinglegendzzh/flashmemory/internal/parser"
	"github.com/kinglegendzzh/flashmemory/internal/search"
)

// Response is the standard API response format
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// BasicAuth credentials loaded from environment
var authUser = os.Getenv("API_USER")
var authPass = os.Getenv("API_PASS")

func main() {
	// FAISS service path from env
	if os.Getenv("FAISS_SERVICE_PATH") == "" {
		log.Fatal("FAISS_SERVICE_PATH must be set")
	}

	// Create Echo instance
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// Basic Auth middleware
	auth := middleware.BasicAuth(func(username, password string, c echo.Context) (bool, error) {
		return username == authUser && password == authPass, nil
	})

	// API group with auth
	api := e.Group("/api", auth)

	// Routes
	api.POST("/search", searchHandler())
	api.POST("/functions", listFunctionsHandler())
	api.POST("/index", buildIndexHandler())
	api.DELETE("/index", deleteIndexHandler())
	api.POST("/index/incremental", incrementalIndexHandler())

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	address := fmt.Sprintf(":%s", port)
	log.Printf("Starting server on %s...", address)
	if err := e.Start(address); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}

// searchHandler handles deep search with dynamic project path
func searchHandler() echo.HandlerFunc {
	type Req struct {
		ProjectDir string `json:"project_dir"`
		Query      string `json:"query"`
		SearchMode string `json:"search_mode"` // semantic, keyword, hybrid
		Limit      int    `json:"limit"`
	}
	type FuncRes struct {
		Name        string  `json:"name"`
		Package     string  `json:"package"`
		File        string  `json:"file"`
		Score       float32 `json:"score"`
		Description string  `json:"description"`
	}

	return func(c echo.Context) error {
		var req Req
		if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "Invalid request body"})
		}
		if req.ProjectDir == "" {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "project_dir is required"})
		}
		if req.Limit == 0 {
			req.Limit = 5
		}
		// Initialize indexer and search engine
		db, _ := index.EnsureIndexDB(req.ProjectDir)
		faissWrapper := index.NewFaissWrapper(128, map[string]interface{}{
			"storage_path": req.ProjectDir + "/.gitgo",
			"index_id":     "code_index",
		})
		idx := &index.Indexer{DB: db, FaissIndex: faissWrapper}
		engine := &search.SearchEngine{Indexer: idx, Descriptions: make(map[int]string)}

		opts := search.SearchOptions{Limit: req.Limit, SearchMode: req.SearchMode}
		results := engine.Query(req.Query, opts)

		var data []FuncRes
		for _, r := range results {
			data = append(data, FuncRes{r.Name, r.Package, r.File, r.Score, r.Description})
		}
		return c.JSON(http.StatusOK, Response{Code: 0, Message: "OK", Data: data})
	}
}

// listFunctionsHandler returns list of functions for dynamic project path
func listFunctionsHandler() echo.HandlerFunc {
	type Req struct {
		ProjectDir  string `json:"project_dir"`
		RelativeDir string `json:"relative_dir,omitempty"`
	}
	type FuncInfo struct {
		Name    string `json:"name"`
		Package string `json:"package"`
		File    string `json:"file"`
	}

	return func(c echo.Context) error {
		var req Req
		if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "Invalid request body"})
		}
		if req.ProjectDir == "" {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "project_dir is required"})
		}
		var funcs []FuncInfo
		root := req.ProjectDir
		if req.RelativeDir != "" {
			root = req.ProjectDir + "/" + req.RelativeDir
		}
		parser.WalkAndParse(root, func(info parser.FunctionInfo) {
			funcs = append(funcs, FuncInfo{info.Name, info.Package, info.File})
		})
		return c.JSON(http.StatusOK, Response{Code: 0, Message: "OK", Data: funcs})
	}
}

// buildIndexHandler builds index for dynamic project path
func buildIndexHandler() echo.HandlerFunc {
	type Req struct {
		ProjectDir  string `json:"project_dir"`
		RelativeDir string `json:"relative_dir,omitempty"`
	}

	return func(c echo.Context) error {
		var req Req
		if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "Invalid request body"})
		}
		if req.ProjectDir == "" {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "project_dir is required"})
		}
		target := req.ProjectDir
		if req.RelativeDir != "" {
			target = req.ProjectDir + "/" + req.RelativeDir
		}
		err := back.BuildIndex(target, true)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, Response{Code: 2, Message: err.Error()})
		}
		return c.JSON(http.StatusOK, Response{Code: 0, Message: "Index built successfully"})
	}
}

// deleteIndexHandler deletes index for dynamic project path
func deleteIndexHandler() echo.HandlerFunc {
	type Req struct {
		ProjectDir  string `json:"project_dir"`
		RelativeDir string `json:"relative_dir,omitempty"`
	}

	return func(c echo.Context) error {
		var req Req
		if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "Invalid request body"})
		}
		if req.ProjectDir == "" {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "project_dir is required"})
		}
		target := req.ProjectDir
		if req.RelativeDir != "" {
			target = req.ProjectDir + "/" + req.RelativeDir
		}
		err := back.DeleteIndex(target)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, Response{Code: 2, Message: err.Error()})
		}
		return c.JSON(http.StatusOK, Response{Code: 0, Message: "Index deleted successfully"})
	}
}

// incrementalIndexHandler updates index for dynamic project path
func incrementalIndexHandler() echo.HandlerFunc {
	type Req struct {
		ProjectDir string `json:"project_dir"`
		Branch     string `json:"branch,omitempty"`
		Commit     string `json:"commit,omitempty"`
	}

	return func(c echo.Context) error {
		var req Req
		if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "Invalid request body"})
		}
		if req.ProjectDir == "" {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "project_dir is required"})
		}
		err := back.IncrementalUpdate(req.ProjectDir, req.Branch, req.Commit)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, Response{Code: 2, Message: err.Error()})
		}
		return c.JSON(http.StatusOK, Response{Code: 0, Message: "Incremental update completed"})
	}
}
