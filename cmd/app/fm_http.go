package main

import (
	"encoding/json"
	"fmt"
	"github.com/kinglegendzzh/flashmemory/config"
	"github.com/kinglegendzzh/flashmemory/internal/back"
	"github.com/kinglegendzzh/flashmemory/internal/index"
	"github.com/kinglegendzzh/flashmemory/internal/parser"
	"github.com/kinglegendzzh/flashmemory/internal/search"
	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"log"
	"net/http"
	"os"
	"path/filepath"
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
	config.Init()
	if os.Getenv("FAISS_SERVICE_PATH") == "" {
		logs.Warnf("FAISS_SERVICE_PATH not set")
	}

	//fm, err := back.InitFaissManager(os.Getenv("FAISS_SERVICE_PATH"))
	//if err != nil {
	//	log.Fatalf("FaissManager 初始化失败: %v", err)
	//}
	//// 程序退出时统一停止 Faiss 服务
	//defer fm.Stop()

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

	// 新增：全局仓库混合检索
	api.POST("/search", searchHandler())
	api.POST("/functions", listFunctionsHandler())
	api.POST("/index", buildIndexHandler())
	api.DELETE("/index", deleteIndexHandler())
	api.POST("/index/incremental", incrementalIndexHandler())

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "5532"
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
		CodeSnippet string  `json:"code_snippet"`
	}

	return func(c echo.Context) error {
		var req Req
		if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "Invalid request body"})
		}
		if req.ProjectDir == "" {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "project_dir is required"})
		}
		if req.SearchMode == "" {
			req.SearchMode = "hybrid"
		}
		if req.Limit == 0 {
			req.Limit = 5
		}

		gitgoDir := filepath.Join(req.ProjectDir, ".gitgo")

		// 索引文件路径
		indexDBPath := filepath.Join(gitgoDir, "code_index.db")
		faissIndexPath := filepath.Join(gitgoDir, "code_index.faiss")

		// 检查.gitgo目录和索引文件是否存在
		if _, err := os.Stat(gitgoDir); os.IsNotExist(err) {
			log.Fatalf("错误：仅查询模式需要已有的索引文件，但.gitgo目录不存在。请先运行索引构建。")
		}
		if _, err := os.Stat(indexDBPath); os.IsNotExist(err) {
			log.Fatalf("错误：仅查询模式需要已有的索引文件，但索引数据库文件不存在。请先运行索引构建。")
		}
		if _, err := os.Stat(faissIndexPath); os.IsNotExist(err) {
			log.Fatalf("错误：仅查询模式需要已有的索引文件，但Faiss索引文件不存在。请先运行索引构建。")
		}

		log.Println("仅查询模式：跳过索引构建，直接加载现有索引...")

		// 打开数据库
		db, err := index.EnsureIndexDB(req.ProjectDir)
		if err != nil {
			log.Fatalf("打开索引数据库失败: %v", err)
		}
		defer db.Close()

		// 确保storage_path是绝对路径
		absGitgoDir, err := filepath.Abs(gitgoDir)
		if err != nil {
			log.Fatalf("获取gitgo目录绝对路径失败: %v", err)
		}

		// 创建FaissWrapper，传入存储路径选项
		faissOptions := map[string]interface{}{
			"storage_path": absGitgoDir,
			"server_url":   index.DefaultFaissServerURL,
			"index_id":     "code_index",
		}
		idx := &index.Indexer{DB: db, FaissIndex: index.NewFaissWrapper(128, faissOptions)}

		// 加载现有索引
		err = idx.FaissIndex.LoadFromFile(faissIndexPath)
		if err != nil {
			log.Fatalf("加载现有Faiss索引失败: %v", err)
		}

		err = back.EnsureEmbeddings(idx, gitgoDir, req.ProjectDir)
		if err != nil {
			log.Fatalf("加载嵌入向量失败: %v", err)
		}

		log.Println("成功加载现有Faiss索引")

		// 执行查询
		log.Println("执行查询...")

		// 构造搜索选项
		opts := search.SearchOptions{
			Limit:       req.Limit,
			MinScore:    0.1,
			IncludeCode: true,
			SearchMode:  req.SearchMode,
		}

		// SearchEngine，传入索引器
		engine := &search.SearchEngine{
			Indexer:      idx,
			Descriptions: make(map[int]string),
		}

		fmt.Println()
		fmt.Printf("查找: %s (模式: %s)\n", req.Query, opts.SearchMode)
		results := engine.Query(req.Query, opts)

		var data []FuncRes
		for _, r := range results {
			data = append(data, FuncRes{r.Name, r.Package, r.File, r.Score, r.Description, r.CodeSnippet})
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
		full := false
		if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "Invalid request body"})
		}
		if req.ProjectDir == "" {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "project_dir is required"})
		}
		if req.RelativeDir == "" {
			full = true
		}
		err := back.BuildIndex(req.ProjectDir, req.RelativeDir, full)
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
