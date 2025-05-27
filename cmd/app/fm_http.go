package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/kinglegendzzh/flashmemory/config"
	"github.com/kinglegendzzh/flashmemory/internal/back"
	"github.com/kinglegendzzh/flashmemory/internal/index"
	"github.com/kinglegendzzh/flashmemory/internal/parser"
	"github.com/kinglegendzzh/flashmemory/internal/search"
	"github.com/kinglegendzzh/flashmemory/internal/utils"
	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
	configPath := config.Init()
	if configPath == "" {
		logs.Fatalf("请设置配置文件路径")
	}
	if os.Getenv("FAISS_SERVICE_PATH") == "" {
		logs.Warnf("FAISS_SERVICE_PATH not set")
	}
	proc, _, err := back.InitFaiss()
	if err != nil {
		log.Fatalf("Faiss 初始化失败: %v", err)
	}
	// 程序退出时统一停止 Faiss 服务
	defer utils.StopFaissService(proc)

	// Create Echo instance
	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	e.Use(middleware.CORS())
	// **在这里开启 CORS 支持，允许来自 http://localhost:5173 的跨域请求**
	//e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
	//	AllowOrigins: []string{"http://localhost:5173"},
	//	AllowMethods: []string{
	//		http.MethodGet,
	//		http.MethodPost,
	//		http.MethodPut,
	//		http.MethodDelete,
	//		http.MethodOptions,
	//	},
	//	AllowHeaders: []string{
	//		echo.HeaderOrigin,
	//		echo.HeaderContentType,
	//		echo.HeaderAccept,
	//		echo.HeaderAuthorization,
	//	},
	//}))

	// Basic Auth middleware (only if user and password are set)
	var auth echo.MiddlewareFunc
	if authUser != "" && authPass != "" {
		auth = middleware.BasicAuth(func(username, password string, c echo.Context) (bool, error) {
			return username == authUser && password == authPass, nil
		})
	} else {
		// If no user or password is set, no authentication is applied
		auth = func(next echo.HandlerFunc) echo.HandlerFunc {
			return next
		}
	}

	// API group with auth
	api := e.Group("/api", auth)

	api.POST("/search", searchHandler())
	api.POST("/functions", listFunctionsHandler())
	api.POST("/index", buildIndexHandler())
	api.DELETE("/index", deleteIndexHandler())
	api.DELETE("/index/reset", resetIndexHandler())
	api.POST("/index/incremental", incrementalIndexHandler())
	api.POST("/listGraph", listGraphHandler())
	api.POST("/exclude", excludeHandler())
	api.POST("/exclude/read", excludeReadHandler())
	api.POST("/llm/analyzer", llmAnalyzerHandler())
	api.GET("/health", healthCheckHandler())

	c := e.Group("/c", auth)
	cfgHandler := config.NewHandler(configPath)
	c.GET("/config", func(c echo.Context) error {
		// 调用 net/http 风格的 Handler
		cfgHandler.GetConfig(c.Response().Writer, c.Request())
		return nil
	})

	c.PUT("/config", func(c echo.Context) error {
		cfgHandler.UpdateConfig(c.Response().Writer, c.Request())
		return nil
	})

	// Start server
	port := os.Getenv("FM_PORT")
	if port == "" {
		port = "5532"
	}
	address := fmt.Sprintf(":%s", port)
	log.Printf("Starting server on %s...", address)
	if err := e.Start(address); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}

func healthCheckHandler() echo.HandlerFunc {
	return func(c echo.Context) error {
		return c.JSON(http.StatusOK, Response{Code: 0, Message: "OK"})
	}
}

// searchHandler handles deep search with dynamic project path
func searchHandler() echo.HandlerFunc {
	type Req struct {
		ProjectDir string `json:"project_dir"`
		Query      string `json:"query"`
		SearchMode string `json:"search_mode"` // semantic, keyword, hybrid
		Limit      int    `json:"limit"`
		Faiss      bool   `json:"faiss"`
	}
	type FuncRes struct {
		Name        string  `json:"name"`
		Package     string  `json:"package"`
		File        string  `json:"file"`
		Score       float32 `json:"score"`
		Description string  `json:"description"`
		CodeSnippet string  `json:"code_snippet"`
	}
	type ResData struct {
		FuncRes []FuncRes `json:"func_res"`
		Tags    []string  `json:"tags"`
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
		faissIndexPath := filepath.Join(gitgoDir, "code_index.local")
		if req.Faiss {
			faissIndexPath = filepath.Join(gitgoDir, "code_index.faiss")
		}
		//var proc *os.Process
		ext := ".local"
		if req.Faiss {
			logs.Infof("正在启动Faiss服务...")
			ext = ".faiss"
			//proc, _, _ = back.InitFaiss()
		}

		// 检查.gitgo目录和索引文件是否存在
		if _, err := os.Stat(gitgoDir); os.IsNotExist(err) {
			return c.JSON(http.StatusInternalServerError, Response{Code: 1, Message: "索引文件不存在"})
		}
		if _, err := os.Stat(indexDBPath); os.IsNotExist(err) {
			return c.JSON(http.StatusInternalServerError, Response{Code: 1, Message: "索引文件不存在"})
		}

		// 打开数据库
		db, err := index.EnsureIndexDB(req.ProjectDir)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, Response{Code: 1, Message: "索引文件不存在"})
		}
		defer db.Close()

		// 确保storage_path是绝对路径
		absGitgoDir, err := filepath.Abs(gitgoDir)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, Response{Code: 1, Message: "索引文件不存在"})
		}

		// 创建FaissWrapper，传入存储路径选项
		faissOptions := map[string]interface{}{
			"storage_path": absGitgoDir,
			"server_url":   index.DefaultFaissServerURL,
			"index_id":     req.ProjectDir,
		}
		idx := &index.Indexer{DB: db, FaissIndex: index.NewFaissWrapper(128, faissOptions)}

		if _, err := os.Stat(faissIndexPath); os.IsNotExist(err) {
			logs.Infof("正在初始化Faiss索引...")
			err = back.EnsureEmbeddingsBatch(idx)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, Response{Code: 1, Message: "索引文件重置失败"})
			}
			err = idx.FaissIndex.SaveToFile(absGitgoDir + "/code_index" + ext)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, Response{Code: 1, Message: "索引文件保存失败"})
			}
		}

		// 加载现有索引
		err = idx.FaissIndex.LoadFromFile(faissIndexPath)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, Response{Code: 1, Message: "索引文件加载失败"})
		}

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
			ProjDir:      req.ProjectDir,
		}

		fmt.Println()
		fmt.Printf("查找: %s (模式: %s)\n", req.Query, opts.SearchMode)
		results := engine.Query(req.Query, opts)

		var data []FuncRes
		for _, r := range results {
			data = append(data, FuncRes{
				r.Name,
				r.Package,
				r.File,
				r.Score,
				r.Description,
				r.CodeSnippet,
			})
		}
		var resData = ResData{
			data,
			engine.Keywords,
		}
		if req.Faiss {
			//e := utils.StopFaissService(proc)
			//if e != nil {
			//	return c.JSON(http.StatusInternalServerError, Response{Code: 1, Message: "Faiss服务停止失败"})
			//}
		}
		return c.JSON(http.StatusOK, Response{Code: 0, Message: "OK", Data: resData})
	}
}

// listFunctionsHandler returns list of functions for dynamic project path
func listFunctionsHandler() echo.HandlerFunc {
	type Req struct {
		ProjectDir string `json:"project_dir"`
		Scan       bool   `json:"scan"`
	}
	type FuncInfo struct {
		Name    string `json:"name"`
		Package string `json:"package"`
		File    string `json:"file"`
		Scan    bool   `json:"scan"`
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
		idxDir := filepath.Join(root, ".gitgo")
		dbPath := filepath.Join(idxDir, "code_index.db")

		// 检查 .gitgo 和 code_index.db 是否都存在
		needQuery := true
		if stat, err := os.Stat(idxDir); err != nil || !stat.IsDir() {
			needQuery = false
		} else if stat, err := os.Stat(dbPath); err != nil || stat.IsDir() {
			needQuery = false
		}
		var count int
		if needQuery {
			db, err := sql.Open("sqlite", dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			countQuery := "SELECT COUNT(*) FROM functions"
			row := db.QueryRow(countQuery)
			if err := row.Scan(&count); err != nil {
				return err
			}
		}
		if req.Scan {
			return c.JSON(http.StatusOK, Response{Code: 0, Message: "OK", Data: count})
		}
		err := parser.WalkAndParse(root, func(info parser.FunctionInfo) {
			funcs = append(funcs, FuncInfo{info.Name, info.Package, info.File, info.Scan})
		})
		if err != nil {
			return c.JSON(http.StatusInternalServerError, Response{Code: 1, Message: err.Error()})
		}
		var scanned int
		for _, f := range funcs {
			if f.Scan {
				scanned++
			}
		}
		logs.Infof("Found %d functions, %d scanned", len(funcs), scanned)

		return c.JSON(http.StatusOK, Response{Code: 0, Message: "OK", Data: funcs})
	}
}

// buildIndexHandler builds index for dynamic project path
func buildIndexHandler() echo.HandlerFunc {
	type Req struct {
		ProjectDir  string   `json:"project_dir"`
		RelativeDir string   `json:"relative_dir,omitempty"`
		Faiss       bool     `json:"Faiss,omitempty"`
		Exclude     []string `json:"exclude,omitempty"`
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
		if req.Exclude != nil {
			logs.Infof("Excluding: %v", req.Exclude)
			err := back.MakeExcludeFile(req.ProjectDir, req.Exclude)
			if err != nil {
				logs.Warnf("Error making exclude file: %v", err)
				return c.JSON(http.StatusInternalServerError, Response{Code: 2, Message: err.Error()})
			}
		}
		// 为该项目生成一个.gitignore，并将.gitgo目录添加到.gitignore中
		err := makeGitIgnore(req.ProjectDir)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, Response{Code: 2, Message: err.Error()})
		}
		err = back.BuildIndex(req.ProjectDir, req.RelativeDir, full, req.Faiss)
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
		err := back.DeleteIndex(target)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, Response{Code: 2, Message: err.Error()})
		}
		return c.JSON(http.StatusOK, Response{Code: 0, Message: "Index deleted successfully"})
	}
}

// resetIndexHandler deletes index for dynamic project path
func resetIndexHandler() echo.HandlerFunc {
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
		err := back.ResetIndex(target)
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
		Faiss      bool   `json:"faiss,omitempty"`
	}

	return func(c echo.Context) error {
		var req Req
		if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "Invalid request body"})
		}
		if req.ProjectDir == "" {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "project_dir is required"})
		}
		err := back.IncrementalUpdate(req.ProjectDir, req.Branch, req.Commit, req.Faiss)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, Response{Code: 2, Message: err.Error()})
		}
		return c.JSON(http.StatusOK, Response{Code: 0, Message: "Incremental update completed"})
	}
}

func listGraphHandler() echo.HandlerFunc {
	type Req struct {
		ProjectDir string `json:"project_dir"`
		SubPath    string `json:"sub_path,omitempty"`
	}

	return func(c echo.Context) error {
		var req Req
		if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "Invalid request body"})
		}
		if req.ProjectDir == "" {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "project_dir is required"})
		}
		err := makeGitIgnore(req.ProjectDir)
		if err != nil {
			logs.Warnf("Error making gitignore: %v", err)
			return c.JSON(http.StatusInternalServerError, Response{Code: 2, Message: err.Error()})
		}
		err = back.ListGraph(req.ProjectDir, req.SubPath)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, Response{Code: 2, Message: err.Error()})
		}
		return c.JSON(http.StatusOK, Response{
			Code:    0,
			Message: "List completed",
			Data:    nil,
		})
	}
}

func excludeHandler() echo.HandlerFunc {
	type Req struct {
		ProjectDir string   `json:"project_dir"`
		Exclude    []string `json:"exclude,omitempty"`
	}

	return func(c echo.Context) error {
		var req Req
		if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "Invalid request body"})
		}
		if req.ProjectDir == "" {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "project_dir is required"})
		}
		if req.Exclude == nil {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "exclude is required"})
		}
		err := makeGitIgnore(req.ProjectDir)
		if err != nil {
			logs.Warnf("Error making gitignore: %v", err)
			return c.JSON(http.StatusInternalServerError, Response{Code: 2, Message: err.Error()})
		}
		err = back.MakeExcludeFile(req.ProjectDir, req.Exclude)
		if err != nil {
			logs.Warnf("Error making exclude file: %v", err)
			return c.JSON(http.StatusInternalServerError, Response{Code: 2, Message: err.Error()})
		}
		return c.JSON(http.StatusOK, Response{
			Code:    0,
			Message: "Exclude completed",
			Data:    nil,
		})
	}
}

func excludeReadHandler() echo.HandlerFunc {
	type Req struct {
		ProjectDir string `json:"project_dir"`
	}

	return func(c echo.Context) error {
		var req Req
		if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "Invalid request body"})
		}
		if req.ProjectDir == "" {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "project_dir is required"})
		}
		excludeFile := filepath.Join(req.ProjectDir, ".gitgo", "exclude.json")
		jsonFile, err := utils.ReadJSONArrayFile(excludeFile)
		if err != nil {
			logs.Warnf("Error reading exclude file: %v", err)
			return c.JSON(http.StatusOK, Response{
				Code:    0,
				Message: "List is Empty",
				Data:    []string{},
			})
		}
		return c.JSON(http.StatusOK, Response{
			Code:    0,
			Message: "List completed",
			Data:    jsonFile,
		})
	}
}

func llmAnalyzerHandler() echo.HandlerFunc {
	type Req struct {
		ProjectDir  string `json:"project_dir"`
		RelativeDir string `json:"relative_dir,omitempty"`
	}
	return func(c echo.Context) error {
		//todo 添加LLM分析功能
		return c.JSON(http.StatusOK, Response{
			Code:    0,
			Message: "LLM SUCCESS",
			Data:    nil,
		})
	}
}

func makeGitIgnore(projDir string) error {
	// 为该项目生成一个 .gitignore，并将 .gitgo 目录添加到 .gitignore 中
	gitignorePath := filepath.Join(projDir, ".gitignore")

	// 读取已有内容（如果文件不存在则认为是空）
	var content []byte
	if _, err := os.Stat(gitignorePath); os.IsNotExist(err) {
		content = []byte{}
	} else {
		content, err = os.ReadFile(gitignorePath)
		if err != nil {
			logs.Warnf("读取 .gitignore 失败: %v", err)
			return err
		}
	}

	// 如果还没有忽略 .gitgo，就追加一行
	if !strings.Contains(string(content), ".gitgo/") {
		f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			logs.Warnf("打开 .gitignore 失败: %v", err)
			return err
		}
		defer f.Close()

		if _, err := f.WriteString("\n.gitgo/\n"); err != nil {
			logs.Warnf("写入 .gitignore 失败: %v", err)
			return err
		}
	}
	return nil
}
