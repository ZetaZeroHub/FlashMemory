package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/kinglegendzzh/flashmemory/cmd/common"
	"github.com/kinglegendzzh/flashmemory/config"
	"github.com/kinglegendzzh/flashmemory/internal/back"
	"github.com/kinglegendzzh/flashmemory/internal/embedding"
	"github.com/kinglegendzzh/flashmemory/internal/graph"
	"github.com/kinglegendzzh/flashmemory/internal/index"
	"github.com/kinglegendzzh/flashmemory/internal/module_analyzer"
	"github.com/kinglegendzzh/flashmemory/internal/parser"
	"github.com/kinglegendzzh/flashmemory/internal/ranking"
	"github.com/kinglegendzzh/flashmemory/internal/search"
	"github.com/kinglegendzzh/flashmemory/internal/utils"
	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"

	_ "modernc.org/sqlite" // SQLite driver
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

// Bearer token loaded from environment
var bearerToken = os.Getenv("API_TOKEN")

// API URL and Model loaded from environment
var apiURL = os.Getenv("API_URL")
var apiModel = os.Getenv("API_MODEL")

func main() {
	// 提前嗅探 lang 参数，以便在定义 flag 时能判断语言
	for i, arg := range os.Args {
		if arg == "-lang" || arg == "--lang" {
			if i+1 < len(os.Args) {
				common.SetLang(os.Args[i+1])
			}
		} else if strings.HasPrefix(arg, "-lang=") {
			common.SetLang(strings.TrimPrefix(arg, "-lang="))
		} else if strings.HasPrefix(arg, "--lang=") {
			common.SetLang(strings.TrimPrefix(arg, "--lang="))
		}
	}

	i18nFlag := func(zhStr, enStr string) string {
		if common.IsZH() {
			return zhStr
		}
		return enStr
	}

	langFlag := flag.String("lang", "", i18nFlag("指定语言 (zh/en)", "Target language (zh/en)"))
	_ = langFlag // keep declared

	// 捕获 panic 错误并记录日志
	defer func() {
		if err := recover(); err != nil {
			logs.Errorf("捕获 panic 错误: %v", err)
			os.Exit(1)
		}
	}()

	configPath := config.Init()
	if configPath == "" {
		logs.Fatalf("请设置配置文件路径")
	}

	// 加载配置文件
	cfg, err := config.LoadConfig()
	if err != nil {
		logs.Warnf("加载配置文件失败: %v", err)
	}

	// 初始化参数到公共管理器（环境变量优先，配置文件回退）
	finalToken := bearerToken
	if finalToken == "" && cfg != nil {
		finalToken = cfg.ApiToken
	}
	if finalToken != "" {
		common.SetEnvToken(finalToken)
		logs.Infof("API Token initialized: %s", finalToken)
	}

	finalURL := apiURL
	if finalURL == "" && cfg != nil {
		finalURL = cfg.ApiUrl
	}
	if finalURL != "" {
		common.SetURL(finalURL)
		logs.Infof("API URL initialized: %s", finalURL)
	}

	finalModel := apiModel
	if finalModel == "" && cfg != nil {
		finalModel = cfg.ApiModel
	}
	if finalModel != "" {
		common.SetModel(finalModel)
		logs.Infof("API Model initialized: %s", finalModel)
	}

	// 更新全局变量以供认证中间件使用
	if finalToken != "" {
		bearerToken = finalToken
	}
	if os.Getenv("FAISS_SERVICE_PATH") == "" {
		logs.Warnf("FAISS_SERVICE_PATH not set")
	}
	proc, serviceDir, err := back.InitFaiss()
	if err != nil {
		log.Fatalf("Faiss 初始化失败: %v", err)
	}
	// 程序退出时统一停止 Faiss 服务
	defer utils.StopFaissService(proc)

	// 启动 Faiss 服务监控
	faissMonitor := back.NewFaissMonitor(proc, serviceDir)
	faissMonitor.SetCheckInterval(5 * time.Second) // 每5秒检查一次
	faissMonitor.Start()
	defer faissMonitor.Stop()

	// Create Echo instance
	e := echo.New()
	// e.Use(middleware.Logger())
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

	// Authentication middleware (supports both Basic Auth and Bearer Token)
	var auth echo.MiddlewareFunc
	if bearerToken != "" {
		// Bearer token authentication
		auth = func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				authHeader := c.Request().Header.Get("Authorization")
				if authHeader == "" {
					return c.JSON(http.StatusUnauthorized, Response{Code: 1, Message: "Authorization header required"})
				}

				if !strings.HasPrefix(authHeader, "Bearer ") {
					return c.JSON(http.StatusUnauthorized, Response{Code: 1, Message: "Bearer token required"})
				}

				token := strings.TrimPrefix(authHeader, "Bearer ")
				if token != bearerToken {
					return c.JSON(http.StatusUnauthorized, Response{Code: 1, Message: "Invalid token"})
				}

				// 将请求头中的token注入到公共管理器
				common.SetCurrentToken(token)
				// 请求结束后清理当前token
				defer common.ClearCurrentToken()

				return next(c)
			}
		}
	} else if authUser != "" && authPass != "" {
		// Basic Auth authentication
		auth = middleware.BasicAuth(func(username, password string, c echo.Context) (bool, error) {
			return username == authUser && password == authPass, nil
		})
	} else {
		// If no authentication is configured, still extract token if present
		auth = func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				// 即使没有配置认证，也尝试提取并注入token
				authHeader := c.Request().Header.Get("Authorization")
				if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
					token := strings.TrimPrefix(authHeader, "Bearer ")
					common.SetCurrentToken(token)
					// 请求结束后清理当前token
					defer common.ClearCurrentToken()
				}
				return next(c)
			}
		}
	}

	// API group with auth
	api := e.Group("/api", auth)

	api.POST("/search", searchHandler())
	api.POST("/functions", listFunctionsHandler())
	api.POST("/index", buildIndexHandler())
	api.DELETE("/index", deleteIndexHandler())
	api.DELETE("/index/some", deleteSomeIndexHandler())
	api.DELETE("/index/reset", resetIndexHandler())
	api.POST("/index/refresh", refreshFaissHandler())
	api.POST("/index/check", checkIndexHandler())
	api.POST("/index/incremental", incrementalIndexHandler())
	api.POST("/listGraph", listGraphHandler())
	api.POST("/exclude", excludeHandler())
	api.POST("/exclude/read", excludeReadHandler())
	api.POST("/llm/analyzer", llmAnalyzerHandler())
	api.POST("/ranking", functionRankingHandler())
	api.POST("/function-info", getFunctionInfoHandler())
	api.POST("/module-graphs", getModuleGraphsHandler())
	api.POST("/module-graphs/update", updateModuleGraphsHandler())
	api.POST("/module-graphs/status", moduleGraphsStatusHandler())
	api.POST("/module-graphs/delete", deleteModuleDescHandler())
	api.POST("/module-graphs/reset", resetModuleDescHandler())
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
		panic(err)
	}

	// 捕获系统信号来优雅关闭服务
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		sig := <-c
		logs.Infof("Received signal %s, shutting down server...", sig)
		os.Exit(0)
	}()
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
		Type        string  `json:"type"`
		Path        string  `json:"path"`
		ParentPath  string  `json:"parent_path"`
	}
	type ResData struct {
		FuncRes []FuncRes `json:"func_res"`
		Tags    []string  `json:"tags"`
		Funcs   []FuncRes `json:"funcs"`
		Modules []FuncRes `json:"modules"`
	}

	return func(c echo.Context) error {
		var req Req
		var Funcs []FuncRes
		var Modules []FuncRes
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
			err = embedding.EnsureEmbeddingsBatch(idx)
			if common.IsLLMError(err) {
				return c.JSON(http.StatusOK, Response{Code: 5, Message: err.Error()})
			}
			if err != nil {
				return c.JSON(http.StatusOK, Response{Code: 1, Message: "索引文件重置失败"})
			}
			err = idx.FaissIndex.SaveToFile(absGitgoDir + "/code_index" + ext)
			if err != nil {
				return c.JSON(http.StatusOK, Response{Code: 1, Message: "索引文件保存失败"})
			}
		}

		// 加载现有索引
		err = idx.FaissIndex.LoadFromFile(faissIndexPath)
		if common.IsLLMError(err) {
			return c.JSON(http.StatusOK, Response{Code: 5, Message: err.Error()})
		}
		if err != nil {
			logs.Errorf("加载嵌入向量失败: %v", err)
			return c.JSON(http.StatusOK, Response{Code: 1, Message: "索引文件加载失败"})
		}

		logs.Infof("成功加载现有Faiss索引")

		// 执行查询
		log.Println("执行查询...")

		// 解析子命令并提取 -lang 参数
		rawArgs := os.Args[1:]
		var args []string
		for i := 0; i < len(rawArgs); i++ {
			if rawArgs[i] == "-lang" || rawArgs[i] == "--lang" {
				if i+1 < len(rawArgs) {
					common.SetLang(rawArgs[i+1])
					i++
				}
			} else if strings.HasPrefix(rawArgs[i], "-lang=") {
				common.SetLang(strings.TrimPrefix(rawArgs[i], "-lang="))
			} else if strings.HasPrefix(rawArgs[i], "--lang=") {
				common.SetLang(strings.TrimPrefix(rawArgs[i], "--lang="))
			} else {
				args = append(args, rawArgs[i])
			}
		}

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

		fmt.Printf("查找函数: %s (模式: %s)\n", req.Query, opts.SearchMode)
		results, err := engine.Query(req.Query, opts)
		if common.IsLLMError(err) {
			return c.JSON(http.StatusOK, Response{Code: 5, Message: err.Error()})
		}
		if err != nil {
			return c.JSON(http.StatusOK, Response{Code: 1, Message: "搜索失败"})
		}

		var data []FuncRes
		for _, r := range results {
			funcRes := FuncRes{
				r.Name,
				r.Package,
				r.File,
				r.Score,
				r.Description,
				r.CodeSnippet,
				r.Type,
				r.Path,
				r.ParentPath,
			}
			data = append(data, funcRes)
			Funcs = append(Funcs, funcRes)
		}
		var keywords []string
		keywords = append(keywords, engine.Keywords...)

		faissModulePath := filepath.Join(gitgoDir, "module.faiss")
		// 创建FaissWrapper，传入存储路径选项
		faissModuleOptions := map[string]interface{}{
			"storage_path": absGitgoDir,
			"server_url":   index.DefaultFaissServerURL,
			"index_id":     fmt.Sprintf("%s_module", req.ProjectDir),
		}
		idxModule := &index.Indexer{DB: db, FaissIndex: index.NewFaissWrapper(128, faissModuleOptions)}

		if _, err := os.Stat(faissModulePath); os.IsNotExist(err) {
			logs.Infof("正在初始化模块描述向量...")
			err = embedding.EnsureCodeDescEmbeddingsBatch(idxModule)
			if common.IsLLMError(err) {
				return c.JSON(http.StatusOK, Response{Code: 5, Message: err.Error()})
			}
			if err != nil {
				return c.JSON(http.StatusOK, Response{Code: 1, Message: "索引文件重置失败"})
			}
			err = idxModule.FaissIndex.SaveToFile(absGitgoDir + "/module.faiss")
			if err != nil {
				return c.JSON(http.StatusOK, Response{Code: 1, Message: "索引文件保存失败"})
			}
		}

		// 加载现有索引
		err = idxModule.FaissIndex.LoadFromFile(faissModulePath)
		if common.IsLLMError(err) {
			logs.Warnf("加载模块描述向量失败: %v", err)
		}
		if err != nil {
			logs.Warnf("加载模块描述向量失败: %v", err)
		} else {
			logs.Infof("成功加载现有模块描述向量")
			opts = search.SearchOptions{
				Limit:       req.Limit,
				MinScore:    0.1,
				IncludeCode: true,
				SearchMode:  req.SearchMode,
			}
			// SearchEngine，传入索引器
			engine = &search.SearchEngine{
				Indexer:      idxModule,
				Descriptions: make(map[int]string),
				ProjDir:      req.ProjectDir,
				Module:       true,
			}
			fmt.Printf("查找模块: %s (模式: %s)\n", req.Query, opts.SearchMode)
			results, err = engine.Query(req.Query, opts)
			if common.IsLLMError(err) {
				return c.JSON(http.StatusOK, Response{Code: 5, Message: err.Error()})
			}
			if err != nil {
				return c.JSON(http.StatusOK, Response{Code: 1, Message: "搜索失败"})
			}
			for _, r := range results {
				funcRes := FuncRes{
					r.Name,
					r.Package,
					r.File,
					r.Score,
					r.Description,
					r.CodeSnippet,
					r.Type,
					r.Path,
					r.ParentPath,
				}
				data = append(data, funcRes)
				Modules = append(Modules, funcRes)
			}
			keywords = append(keywords, engine.Keywords...)
		}

		// 对data基于Score得分排序
		sort.Slice(data, func(i, j int) bool {
			return data[i].Score > data[j].Score
		})

		var resData = ResData{
			data,
			keywords,
			Funcs,
			Modules,
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
				logs.Errorf("Error opening database: %v", err)
				return err
			}
			defer db.Close()
			countQuery := "SELECT COUNT(*) FROM functions"
			row := db.QueryRow(countQuery)
			if err := row.Scan(&count); err != nil {
				logs.Warnf("Error counting functions: %v", err)
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
		// 先清理索引
		err = back.RefreshFaiss(req.ProjectDir)
		if err != nil {
			logs.Warnf("Error refreshing faiss: %v", err)
		}
		err = back.BuildIndex(req.ProjectDir, req.RelativeDir, full, req.Faiss)
		if common.IsLLMError(err) {
			return c.JSON(http.StatusOK, Response{Code: 5, Message: err.Error()})
		}
		if err != nil {
			return c.JSON(http.StatusOK, Response{Code: 2, Message: err.Error()})
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

// refreshFaissHandler refreshes faiss index for dynamic project path
func refreshFaissHandler() echo.HandlerFunc {
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
		err := back.RefreshFaiss(req.ProjectDir)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, Response{Code: 2, Message: err.Error()})
		}
		return c.JSON(http.StatusOK, Response{Code: 0, Message: "Faiss index refreshed successfully"})
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
		err := back.ResetIndex(target, req.RelativeDir)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, Response{Code: 2, Message: err.Error()})
		}
		return c.JSON(http.StatusOK, Response{Code: 0, Message: "Index deleted successfully"})
	}
}

func deleteSomeIndexHandler() echo.HandlerFunc {
	type Req struct {
		ProjectDir  string `json:"project_dir"`
		RelativeDir string `json:"relative_dir"`
	}

	return func(c echo.Context) error {
		var req Req
		if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "Invalid request body"})
		}
		if req.ProjectDir == "" {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "project_dir is required"})
		}
		if req.RelativeDir == "" {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "relative_dir is required"})
		}
		err := back.DeleteSomeIndex(req.ProjectDir, req.RelativeDir)
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
		if common.IsLLMError(err) {
			return c.JSON(http.StatusOK, Response{Code: 5, Message: err.Error()})
		}
		if err != nil {
			return c.JSON(http.StatusOK, Response{Code: 2, Message: err.Error()})
		}
		return c.JSON(http.StatusOK, Response{Code: 0, Message: "Incremental update completed"})
	}
}

func checkIndexHandler() echo.HandlerFunc {
	type Req struct {
		ProjectDir  string `json:"project_dir"`
		RelativeDir string `json:"relative_dir,omitempty"`
	}

	type FunctionDetail struct {
		Name        string `json:"name"`
		Package     string `json:"package"`
		File        string `json:"file"`
		StartLine   int    `json:"start_line"`
		EndLine     int    `json:"end_line"`
		Description string `json:"description,omitempty"`
	}

	type ModuleDetail struct {
		Name          string    `json:"name"`
		Type          string    `json:"type"`
		Path          string    `json:"path"`
		ParentPath    string    `json:"parent_path"`
		FunctionCount int       `json:"function_count"`
		FileCount     int       `json:"file_count"`
		Description   string    `json:"description,omitempty"`
		CreatedAt     time.Time `json:"created_at"`
		UpdatedAt     time.Time `json:"updated_at"`
	}

	type CheckIndexResponse struct {
		TotalFunctionCount int                         `json:"total_function_count"`
		TotalFileCount     int                         `json:"total_file_count"`
		RealFileCount      int                         `json:"real_file_count"`
		Functions          map[string][]FunctionDetail `json:"functions"`
		Modules            map[string][]ModuleDetail   `json:"modules"`
	}

	return func(c echo.Context) error {
		var req Req
		if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
			return c.JSON(http.StatusOK, Response{Code: 1, Message: "Invalid request body"})
		}
		if req.ProjectDir == "" {
			return c.JSON(http.StatusOK, Response{Code: 1, Message: "project_dir is required"})
		}

		// 构建数据库路径
		gitgoDir := filepath.Join(req.ProjectDir, ".gitgo")
		indexDBPath := filepath.Join(gitgoDir, "code_index.db")

		// 计算扫描的文件数量
		realFileCount := 0
		var totalPath string
		if req.RelativeDir != "" {
			// 如果提供了子路径，则更新路径
			totalPath = filepath.Join(req.ProjectDir, req.RelativeDir)
		} else {
			// 全量索引模式：遍历整个项目目录
			totalPath = req.ProjectDir
		}

		// 遍历路径并计算文件数量
		err := filepath.Walk(totalPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			// 跳过exclude.json中指定的路径
			fullWalkPath := filepath.Join(totalPath, path)
			excludeFile := filepath.Join(req.ProjectDir, ".gitgo", "exclude.json")
			jsonFile, _ := utils.ReadJSONArrayFile(excludeFile)
			if utils.IsExcludedPath(fullWalkPath, jsonFile) {
				return filepath.SkipDir
			}
			// 如果是目录，进行排除处理
			if info.IsDir() {
				// 跳过以点开头的隐藏目录
				if info.Name() != "." && info.Name() != ".." && strings.HasPrefix(info.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			// 仅考虑特定的文件扩展名
			ext := filepath.Ext(path)
			if utils.Contains(common.SupportedLanguages, ext) && !strings.HasSuffix(path, "__init__.py") {
				realFileCount++
			}
			return nil
		})
		if err != nil {
			log.Printf("遍历目录失败: %v", err)
		}

		// 检查.gitgo目录和索引文件是否存在
		if _, err := os.Stat(gitgoDir); os.IsNotExist(err) {
			logs.Errorf("索引目录不存在: %s", gitgoDir)
		}
		if _, err := os.Stat(indexDBPath); os.IsNotExist(err) {
			logs.Errorf("索引文件不存在: %s", indexDBPath)
		}

		// 打开数据库连接
		db, err := sql.Open("sqlite", indexDBPath)
		if err != nil {
			logs.Errorf("打开数据库连接失败: %v", err)
		}
		defer db.Close()

		// 设置数据库连接池参数以提高性能
		db.SetMaxOpenConns(10)
		db.SetMaxIdleConns(5)

		// 构建查询SQL
		var query string
		var args []interface{}
		if req.RelativeDir != "" {
			// 查询特定子路径，使用LIKE模糊匹配以支持目录查询
			query = "SELECT name, package, file, start_line, end_line, description FROM functions WHERE file LIKE ? ORDER BY file, start_line"
			// 确保路径以/结尾用于目录匹配，或精确匹配文件
			subPathPattern := strings.TrimPrefix(req.RelativeDir, "/")
			if !strings.HasSuffix(subPathPattern, "/") && !strings.Contains(subPathPattern, ".") {
				// 如果是目录路径，添加通配符
				logs.Infof("目录路径: %s", subPathPattern)
				subPathPattern += "/%"
			} else if strings.Contains(subPathPattern, ".") {
				// 如果是文件路径，精确匹配
				logs.Infof("文件路径: %s", subPathPattern)
			} else {
				// 其他情况使用模糊匹配
				logs.Infof("模糊匹配: %s", subPathPattern)
				subPathPattern = "%" + subPathPattern + "%"
			}
			args = append(args, subPathPattern)
		} else {
			// 查询所有函数
			query = "SELECT name, package, file, start_line, end_line, description FROM functions ORDER BY file, start_line"
		}

		// 处理查询结果
		functionsByFile := make(map[string][]FunctionDetail)
		fileSet := make(map[string]bool)
		totalFunctions := 0

		// 执行查询
		rows, err := db.Query(query, args...)
		if err == nil {
			defer rows.Close()

			for rows.Next() {
				var fn FunctionDetail
				var packageName sql.NullString
				var description sql.NullString

				err := rows.Scan(&fn.Name, &packageName, &fn.File, &fn.StartLine, &fn.EndLine, &description)
				if err != nil {
					return c.JSON(http.StatusOK, Response{Code: 1, Message: "数据解析失败: " + err.Error()})
				}

				if packageName.Valid {
					fn.Package = packageName.String
				}
				if description.Valid {
					fn.Description = description.String
				}

				// 按文件分组
				functionsByFile[fn.File] = append(functionsByFile[fn.File], fn)
				fileSet[fn.File] = true
				totalFunctions++
			}

			// 检查是否有扫描错误
			if err = rows.Err(); err != nil {
				return c.JSON(http.StatusOK, Response{Code: 1, Message: "结果集处理失败: " + err.Error()})
			}
		} else {
			logs.Errorf("执行查询失败: %v", err)
			// 当查询失败时rows为nil，不应调用Close
		}

		// 获取模块信息
		modulesByPath := make(map[string][]ModuleDetail)
		var moduleQuery string
		var moduleArgs []interface{}

		if req.RelativeDir != "" {
			// 查询特定子路径的模块，使用LIKE模糊匹配
			moduleQuery = `
				SELECT name, 
				type, 
				path, 
				parent_path, 
				function_count, 
				file_count, 
				description, 
				updated_at, 
				created_at 
				FROM code_desc 
				WHERE path LIKE ?`
			subPathPattern := strings.TrimPrefix(req.RelativeDir, "/")
			if !strings.HasSuffix(subPathPattern, "/") && !strings.Contains(subPathPattern, ".") {
				// 如果是目录路径，添加通配符
				subPathPattern = subPathPattern + "%"
			} else {
				// 其他情况使用模糊匹配
				subPathPattern = subPathPattern + "%"
			}
			moduleArgs = append(moduleArgs, subPathPattern)
		} else {
			// 查询根模块（代表整个项目）
			moduleQuery = `
				SELECT name, 
				type, 
				path, 
				parent_path, 
				function_count, 
				file_count, 
				description, 
				updated_at, 
				created_at 
				FROM code_desc 
				WHERE path = '' OR parent_path = ''`
		}

		// 执行模块查询
		moduleRows, err := db.Query(moduleQuery, moduleArgs...)
		if err == nil {
			defer moduleRows.Close()

			// 如果是查询特定目录，需要特殊处理
			specificDir := ""
			if req.RelativeDir != "" {
				// 去除前后斜杠
				specificDir = strings.Trim(req.RelativeDir, "/")
				// 如果包含全路径，只取最后一部分
				if strings.Contains(specificDir, "/") {
					parts := strings.Split(specificDir, "/")
					specificDir = parts[len(parts)-1]
				}
				logs.Infof("specificDir: %s", specificDir)
			}

			// 首先处理查询结果，建立全部模块的映射
			allModules := make(map[string]ModuleDetail)
			for moduleRows.Next() {
				var module ModuleDetail
				var updatedAt, createdAt string

				err := moduleRows.Scan(
					&module.Name,
					&module.Type,
					&module.Path,
					&module.ParentPath,
					&module.FunctionCount,
					&module.FileCount,
					&module.Description,
					&updatedAt,
					&createdAt,
				)

				if err != nil {
					logs.Warnf("扫描模块行失败: %v", err)
					continue
				}

				// 解析时间
				module.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
				module.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)

				// 将模块添加到映射中
				allModules[module.Path] = module
			}

			// 整合结果，处理特殊情况
			if specificDir != "" {
				// 当查询特定目录时，为了确保目录自身也显示在结果中
				for _, module := range allModules {
					// 找到当前查询的目录自身
					if module.Path == specificDir || module.Name == specificDir {
						// 将目录自身放在以目录名命名的分组中
						modulesByPath["current"] = []ModuleDetail{module}
						logs.Infof("已将查询目录自身添加到results[current]: %s", module.Path)
					} else {
						// 将子模块放在目录名命名的分组中
						parentPath := module.ParentPath
						if parentPath == "" {
							parentPath = "root"
						}

						if _, ok := modulesByPath[parentPath]; !ok {
							modulesByPath[parentPath] = []ModuleDetail{}
						}
						modulesByPath[parentPath] = append(modulesByPath[parentPath], module)
					}
				}
			} else {
				// 没有指定目录查询时，按正常分类组织模块
				for _, module := range allModules {
					parentPath := module.ParentPath
					if parentPath == "" {
						parentPath = "root"
					}

					if _, ok := modulesByPath[parentPath]; !ok {
						modulesByPath[parentPath] = []ModuleDetail{}
					}
					modulesByPath[parentPath] = append(modulesByPath[parentPath], module)
				}
			}
		} else {
			logs.Errorf("执行模块查询失败: %v", err)
		}

		// 构建响应
		response := CheckIndexResponse{
			TotalFunctionCount: totalFunctions,
			TotalFileCount:     len(fileSet),
			RealFileCount:      realFileCount,
			Functions:          functionsByFile,
			Modules:            modulesByPath,
		}

		return c.JSON(http.StatusOK, Response{Code: 0, Message: "查询成功", Data: response})
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
			logs.Errorf("project_dir is required")
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "project_dir is required"})
		}
		err := makeGitIgnore(req.ProjectDir)
		if err != nil {
			logs.Warnf("Error making gitignore: %v", err)
			return c.JSON(http.StatusInternalServerError, Response{Code: 2, Message: err.Error()})
		}
		// err = back.RefreshFaiss(req.ProjectDir)
		// if err != nil {
		// 	logs.Warnf("Error refreshing faiss: %v", err)
		// }
		err = back.ListGraph(req.ProjectDir, req.SubPath)
		if common.IsLLMError(err) {
			return c.JSON(http.StatusOK, Response{Code: 5, Message: err.Error()})
		}
		if err != nil {
			logs.Errorf("Error listing graph: %v", err)
			return c.JSON(http.StatusOK, Response{Code: 2, Message: err.Error()})
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
	// 追加一行 .zip
	if !strings.Contains(string(content), ".zip") {
		f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			logs.Warnf("打开 .gitignore 失败: %v", err)
			return err
		}
		defer f.Close()

		if _, err := f.WriteString("\n*.zip\n"); err != nil {
			logs.Warnf("写入 .gitignore 失败: %v", err)
			return err
		}
	}
	return nil
}

// FunctionRankingRequest 函数重要性评级请求
type FunctionRankingRequest struct {
	ProjectDir string                 `json:"project_dir"`
	Config     *ranking.RankingConfig `json:"config,omitempty"`
}

// functionRankingHandler 函数重要性评级处理器
// getModuleGraphsHandler handles retrieving the four module graph structures for a project.
func getModuleGraphsHandler() echo.HandlerFunc {
	return func(c echo.Context) error {
		var req struct {
			ProjectDir string `json:"project_dir"`
			GraphType  string `json:"graph_type"`
		}
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, Response{
				Code:    1,
				Message: "Invalid request format: " + err.Error(),
			})
		}

		if req.ProjectDir == "" {
			return c.JSON(http.StatusBadRequest, Response{
				Code:    1,
				Message: "project_dir is required",
			})
		}

		graphDir := filepath.Join(req.ProjectDir, ".gitgo", "module_graphs")
		graphDirExists, err := os.Stat(graphDir)
		if err != nil {
			return c.JSON(http.StatusOK, Response{
				Code:    3,
				Message: "图谱目录不存在，需要初始化: " + err.Error(),
			})
		}
		if !graphDirExists.IsDir() {
			return c.JSON(http.StatusOK, Response{
				Code:    3,
				Message: "图谱目录不存在，需要初始化",
			})
		}
		graphTypes := []string{"hierarchical", "network", "sunburst", "flat"}
		mapping := map[string]string{
			"hierarchical": "hierarchical_tree.json", // "层次图谱",
			"network":      "network_graph.json",     // "网络图谱",
			"sunburst":     "sunburst_chart.json",    // "旭日图谱",
			"flat":         "flat_nodes.json",        // "扁平图谱",
		}
		graphs := make(map[string]interface{})

		for _, graphType := range graphTypes {
			if req.GraphType != "" && graphType != req.GraphType {
				continue
			}
			graphPath := filepath.Join(graphDir, mapping[graphType])
			data, err := os.ReadFile(graphPath)
			if err != nil {
				return c.JSON(http.StatusOK, Response{
					Code:    1,
					Message: fmt.Sprintf("未更新%s图谱", graphType),
				})
			}

			var jsonData interface{}
			if err := json.Unmarshal(data, &jsonData); err != nil {
				return c.JSON(http.StatusOK, Response{
					Code:    1,
					Message: fmt.Sprintf("未更新%s图谱", graphType),
				})
			}
			graphs[graphType] = jsonData
		}

		return c.JSON(http.StatusOK, Response{
			Code:    0,
			Message: "图谱获取成功",
			Data:    graphs,
		})
	}
}

// moduleGraphsStatusHandler handles querying the status of module analysis tasks.
func moduleGraphsStatusHandler() echo.HandlerFunc {
	return func(c echo.Context) error {
		var req struct {
			TaskID     string `json:"task_id"`
			ProjectDir string `json:"project_dir"`
		}
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, Response{
				Code:    1,
				Message: "Invalid request format: " + err.Error(),
			})
		}

		if req.TaskID == "" && req.ProjectDir == "" {
			return c.JSON(http.StatusBadRequest, Response{
				Code:    1,
				Message: "task_id or project_dir is required",
			})
		}

		var task *module_analyzer.AnalysisTask
		var exists bool
		if req.TaskID != "" {
			task, exists = module_analyzer.GetTask(req.TaskID)
			if !exists {
				return c.JSON(http.StatusOK, Response{
					Code:    1,
					Message: "Task not found",
				})
			}
		}

		if req.ProjectDir != "" {
			task, exists = module_analyzer.GetTaskByProjDir(req.ProjectDir)
			if !exists {
				return c.JSON(http.StatusOK, Response{
					Code:    1,
					Message: "Task not found",
				})
			}
		}

		// 返回任务状态和进度信息
		return c.JSON(http.StatusOK, Response{
			Code:    0,
			Message: "Task status retrieved successfully",
			Data:    task,
		})
	}
}

// updateModuleGraphsHandler handles triggering an update for the module graphs.
func updateModuleGraphsHandler() echo.HandlerFunc {
	return func(c echo.Context) error {
		var req struct {
			ProjectDir string `json:"project_dir"`
			SkipLLM    bool   `json:"skip_llm"`
		}
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, Response{
				Code:    1,
				Message: "Invalid request format: " + err.Error(),
			})
		}

		if req.ProjectDir == "" {
			return c.JSON(http.StatusBadRequest, Response{
				Code:    1,
				Message: "project_dir is required",
			})
		}

		// 查询当前任务状态，若存在则返回
		task, exists := module_analyzer.GetRunningTaskByProjDir(req.ProjectDir)
		if exists && !req.SkipLLM {
			logs.Infof("图谱更新任务已存在，任务ID: %s", task.ID)
			return c.JSON(http.StatusOK, Response{
				Code:    1,
				Message: "图谱更新任务已存在，任务ID: " + task.ID,
				Data: map[string]interface{}{
					"task_id":     task.ID,
					"status":      task.Status,
					"project_dir": req.ProjectDir,
				},
			})
		}

		cfg, err := config.LoadConfig()
		if err != nil {
			logs.Errorf("配置加载失败: %v", err)
			return c.JSON(http.StatusOK, Response{
				Code:    1,
				Message: "配置加载失败: " + err.Error(),
			})
		}

		dbPath := filepath.Join(req.ProjectDir, ".gitgo", "code_index.db")
		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			logs.Errorf("图谱数据库打开失败: %v", err)
			return c.JSON(http.StatusOK, Response{
				Code:    1,
				Message: "图谱数据库打开失败: " + err.Error(),
			})
		}
		defer db.Close()

		// 读取graph.json文件
		graphPath := filepath.Join(req.ProjectDir, ".gitgo", "graph.json")
		data, err := os.ReadFile(graphPath)
		// 解析graph.json
		var kg graph.KnowledgeGraph
		if err == nil {
			if err := json.Unmarshal(data, &kg); err != nil {
				return c.JSON(http.StatusOK, Response{
					Code:    1,
					Message: "图谱解析失败: " + err.Error(),
				})
			}
		}

		// 启动分析任务，返回任务ID
		logs.Infof("图谱更新任务启动: %s", req.ProjectDir)

		// AnalyzeAllModules 现在会返回一个任务ID并在后台异步运行
		// 默认不跳过LLM描述生成
		skipLLM := req.SkipLLM
		if !skipLLM {
			// 如果存在module_analyzer.temp文件，则删除该文件，然后清理异步任务（强制重新开始）
			logs.Infof("skipLLM为false，删除module_analyzer.temp文件")
			tempFile := filepath.Join(req.ProjectDir, ".gitgo", "module_analyzer.temp")
			if _, err := os.Stat(tempFile); err == nil {
				logs.Infof("模块分析正在执行中，请等待")
				os.Remove(tempFile)
			}
			// 先清理索引
			err = back.RefreshFaiss(req.ProjectDir)
			if err != nil {
				logs.Warnf("Error refreshing faiss: %v", err)
			}
		}
		err = module_analyzer.AnalyzeAllModules(kg.Functions, db, req.ProjectDir, cfg, skipLLM, "")
		if common.IsLLMError(err) {
			return c.JSON(http.StatusOK, Response{Code: 5, Message: err.Error()})
		}
		if err != nil {
			logs.Errorf("图谱更新失败 %v", err)
			return c.JSON(http.StatusOK, Response{
				Code:    1,
				Message: "图谱更新失败: " + err.Error(),
			})
		}

		// 查询最新任务状态
		task, exists = module_analyzer.GetTaskByProjDir(req.ProjectDir)
		if exists {
			return c.JSON(http.StatusOK, Response{
				Code:    0,
				Message: "图谱更新任务已启动",
				Data: map[string]interface{}{
					"task_id":     task.ID,
					"status":      task.Status,
					"project_dir": req.ProjectDir,
				},
			})
		}

		return c.JSON(http.StatusOK, Response{
			Code:    0,
			Message: "图谱更新任务已启动",
			Data: map[string]interface{}{
				"project_dir": req.ProjectDir,
			},
		})
	}
}

func deleteModuleDescHandler() echo.HandlerFunc {
	return func(c echo.Context) error {
		var req struct {
			ProjectDir string `json:"project_dir"`
		}
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, Response{
				Code:    1,
				Message: "Invalid request format: " + err.Error(),
			})
		}

		if req.ProjectDir == "" {
			return c.JSON(http.StatusBadRequest, Response{
				Code:    1,
				Message: "project_dir is required",
			})
		}

		err := back.DeleteModuleDesc(req.ProjectDir)
		if err != nil {
			return c.JSON(http.StatusOK, Response{
				Code:    1,
				Message: "删除模块分析记录失败: " + err.Error(),
			})
		}

		return c.JSON(http.StatusOK, Response{
			Code:    0,
			Message: "删除模块分析记录成功",
		})
	}
}

func resetModuleDescHandler() echo.HandlerFunc {
	return func(c echo.Context) error {
		var req struct {
			ProjectDir string `json:"project_dir"`
		}
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, Response{
				Code:    1,
				Message: "Invalid request format: " + err.Error(),
			})
		}

		if req.ProjectDir == "" {
			return c.JSON(http.StatusBadRequest, Response{
				Code:    1,
				Message: "project_dir is required",
			})
		}

		err := back.ResetModuleDesc(req.ProjectDir)
		if err != nil {
			return c.JSON(http.StatusOK, Response{
				Code:    1,
				Message: "重置模块分析记录失败: " + err.Error(),
			})
		}

		return c.JSON(http.StatusOK, Response{
			Code:    0,
			Message: "重置模块分析记录成功",
		})
	}
}

// getFunctionInfoHandler handles retrieving function information from graph.json
func getFunctionInfoHandler() echo.HandlerFunc {
	type FunctionInfoRequest struct {
		ProjectDir   string `json:"project_dir"`
		FilePath     string `json:"file_path"`               // 文件相对路径
		PackageName  string `json:"package,omitempty"`       // 可选的包名
		FunctionName string `json:"function_name,omitempty"` // 可选的函数名
	}

	type FunctionDetail struct {
		Name         string   `json:"name"`
		Package      string   `json:"package"`
		File         string   `json:"file"`
		Imports      []string `json:"imports"`
		Calls        []string `json:"calls"`
		StartLine    int      `json:"start_line"`
		EndLine      int      `json:"end_line"`
		Lines        int      `json:"lines"`
		FunctionType string   `json:"function_type"`
		Description  string   `json:"description"`
		FanIn        int      `json:"fan_in"`
		FanOut       int      `json:"fan_out"`
		Complexity   int      `json:"complexity"`
		Depth        int      `json:"depth"`
		Score        float64  `json:"score"` // 重要性评分
	}

	return func(c echo.Context) error {
		var req FunctionInfoRequest
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, Response{
				Code:    1,
				Message: "Invalid request format: " + err.Error(),
			})
		}

		if req.ProjectDir == "" {
			return c.JSON(http.StatusBadRequest, Response{
				Code:    1,
				Message: "project_dir is required",
			})
		}

		if req.FilePath == "" {
			return c.JSON(http.StatusBadRequest, Response{
				Code:    1,
				Message: "file_path is required",
			})
		}

		// 读取graph.json文件
		graphPath := filepath.Join(req.ProjectDir, ".gitgo", "graph.json")
		data, err := os.ReadFile(graphPath)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, Response{
				Code:    1,
				Message: "Failed to read graph.json: " + err.Error(),
			})
		}

		// 解析graph.json
		var kg graph.KnowledgeGraph
		if err := json.Unmarshal(data, &kg); err != nil {
			return c.JSON(http.StatusInternalServerError, Response{
				Code:    1,
				Message: "Failed to parse graph.json: " + err.Error(),
			})
		}

		// 过滤函数信息
		var matchedFunctions []FunctionDetail
		for _, funcResult := range kg.Functions {
			funcInfo := funcResult.Func

			// 检查文件路径匹配
			if !strings.Contains(funcInfo.File, req.FilePath) {
				continue
			}

			// 如果指定了包名，检查包名匹配
			if req.PackageName != "" && funcInfo.Package != req.PackageName {
				continue
			}

			// 如果指定了函数名，检查函数名匹配
			if req.FunctionName != "" && funcInfo.Name != req.FunctionName {
				continue
			}

			// 构建函数详细信息
			funcDetail := FunctionDetail{
				Name:         funcInfo.Name,
				Package:      funcInfo.Package,
				File:         funcInfo.File,
				Imports:      funcInfo.Imports,
				Calls:        funcInfo.Calls,
				StartLine:    funcInfo.StartLine,
				EndLine:      funcInfo.EndLine,
				Lines:        funcInfo.Lines,
				FunctionType: funcInfo.FunctionType,
				Description:  funcResult.Description,
				FanIn:        funcInfo.FanIn,
				FanOut:       funcInfo.FanOut,
				Complexity:   funcInfo.Complexity,
				Depth:        funcInfo.Depth,
				Score:        funcResult.ImportanceScore,
			}

			matchedFunctions = append(matchedFunctions, funcDetail)
		}

		if len(matchedFunctions) == 0 {
			return c.JSON(http.StatusOK, Response{
				Code:    1,
				Message: "No matching functions found",
			})
		}

		return c.JSON(http.StatusOK, Response{
			Code:    0,
			Message: "Function information retrieved successfully",
			Data: map[string]interface{}{
				"functions": matchedFunctions,
				"total":     len(matchedFunctions),
			},
		})
	}
}

func functionRankingHandler() echo.HandlerFunc {
	return func(c echo.Context) error {
		var req FunctionRankingRequest
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, Response{
				Code:    1,
				Message: "Invalid request format: " + err.Error(),
			})
		}

		if req.ProjectDir == "" {
			return c.JSON(http.StatusBadRequest, Response{
				Code:    1,
				Message: "project_dir is required",
			})
		}

		// 默认配置：均衡配置
		if req.Config == nil {
			req.Config = &ranking.RankingConfig{
				Alpha: 0.4, // FanIn权重
				Beta:  0.2, // FanOut权重
				Gamma: 0.2, // Depth权重
				Delta: 0.2, // Complexity权重
			}
		}

		// 读取graph.json文件
		graphPath := filepath.Join(req.ProjectDir, ".gitgo", "graph.json")
		data, err := os.ReadFile(graphPath)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, Response{
				Code:    1,
				Message: "Failed to read graph.json: " + err.Error(),
			})
		}

		// 解析graph.json
		var kg graph.KnowledgeGraph
		if err := json.Unmarshal(data, &kg); err != nil {
			return c.JSON(http.StatusInternalServerError, Response{
				Code:    1,
				Message: "Failed to parse graph.json: " + err.Error(),
			})
		}

		// 转换为ranking算法需要的格式
		functions := make([]parser.FunctionInfo, len(kg.Functions))
		for i, result := range kg.Functions {
			functions[i] = result.Func
		}

		// 计算重要性评分
		scores := ranking.CalculateImportanceScores(functions, req.Config)

		// 将评分回填到ImportanceScore字段
		for i := range kg.Functions {
			funcName := kg.Functions[i].Func.Name
			if score, exists := scores[funcName]; exists {
				kg.Functions[i].ImportanceScore = score
			}
		}

		// 保存更新后的graph.json
		updatedData, err := json.MarshalIndent(kg, "", "  ")
		if err != nil {
			return c.JSON(http.StatusInternalServerError, Response{
				Code:    1,
				Message: "Failed to marshal updated graph: " + err.Error(),
			})
		}

		if err := os.WriteFile(graphPath, updatedData, 0644); err != nil {
			return c.JSON(http.StatusInternalServerError, Response{
				Code:    1,
				Message: "Failed to save updated graph.json: " + err.Error(),
			})
		}

		return c.JSON(http.StatusOK, Response{
			Code:    0,
			Message: "Function importance scores calculated and saved successfully",
			Data: map[string]interface{}{
				"total_functions": len(functions),
				"config":          req.Config,
				"scores":          scores,
			},
		})
	}
}
