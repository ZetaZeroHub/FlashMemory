package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/kinglegendzzh/flashmemory/cmd/common"
	"github.com/kinglegendzzh/flashmemory/config"
	"github.com/kinglegendzzh/flashmemory/internal/anchor"
	"github.com/kinglegendzzh/flashmemory/internal/back"
	"github.com/kinglegendzzh/flashmemory/internal/embedding"
	"github.com/kinglegendzzh/flashmemory/internal/graph"
	"github.com/kinglegendzzh/flashmemory/internal/index"
	"github.com/kinglegendzzh/flashmemory/internal/llm"
	"github.com/kinglegendzzh/flashmemory/internal/module_analyzer"
	"github.com/kinglegendzzh/flashmemory/internal/parser"
	"github.com/kinglegendzzh/flashmemory/internal/ranking"
	"github.com/kinglegendzzh/flashmemory/internal/router"
	"github.com/kinglegendzzh/flashmemory/internal/search"
	"github.com/kinglegendzzh/flashmemory/internal/telemetry"
	"github.com/kinglegendzzh/flashmemory/internal/tool"
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

var (
	toolRegistryOnce sync.Once
	toolRegistryInst *tool.Registry
)

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
		logs.Warnf("Config file not explicitly provided via -c flag, automatically falling back to user home directory default config")
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

	useZvec := cfg != nil && cfg.ZvecConfig.Engine == "zvec"
	if useZvec {
		logs.Infof("Zvec mode enabled: skipping traditional FAISS HTTP service startup")
	} else {
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
	}

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

	// Graceful shutdown 路径，必须在 e.Start 之前注册——
	// e.Start 会阻塞直到服务退出，原代码把 signal.Notify 放在 e.Start 之后是死代码。
	//
	// 流程：
	//   1) SIGINT/SIGTERM 到达
	//   2) e.Shutdown(ctx) 等待所有 in-flight handler 跑完 defer（含 fm.Free → bridge SIGTERM）
	//      30s 超时兜底，避免无限等
	//   3) 不论 Shutdown 是否超时，再调 index.FreeAllActiveWrappers 强清残留 zvec_bridge，
	//      杜绝 RocksDB 写半截 segment/MANIFEST 的源头
	//   4) main 函数返回，进程正常退出
	shutdownDone := make(chan struct{})
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		logs.Infof("Received signal %s, initiating graceful shutdown (30s budget)...", sig)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := e.Shutdown(ctx); err != nil {
			logs.Warnf("Graceful shutdown ended with error (some handlers may have been interrupted): %v", err)
		} else {
			logs.Infof("All in-flight handlers drained")
		}

		// Defense-in-depth: 即便 e.Shutdown 完全成功，仍可能存在
		// 后台 goroutine（如 module_analyzer）持有的 wrapper，或某条 handler
		// 因为 panic 跳过了 defer 的情况。统一在这里收尾。
		if n := index.FreeAllActiveWrappers(); n > 0 {
			logs.Infof("Forcefully freed %d residual Zvec wrapper(s) after shutdown", n)
		}

		close(shutdownDone)
	}()

	log.Printf("Starting server on %s...", address)
	if err := e.Start(address); err != nil && err != http.ErrServerClosed {
		// 启动失败（端口占用等）：仍然清理一遍，防 wrapper 提前注册的极端情况
		index.FreeAllActiveWrappers()
		log.Fatalf("Server error: %v", err)
	}

	// e.Start 因为 Shutdown 调用而返回，等清理 goroutine 跑完
	<-shutdownDone
	logs.Infof("FM HTTP server stopped cleanly")
}

func healthCheckHandler() echo.HandlerFunc {
	return func(c echo.Context) error {
		return c.JSON(http.StatusOK, Response{Code: 0, Message: "OK"})
	}
}

func getToolRegistry() *tool.Registry {
	toolRegistryOnce.Do(func() {
		reg := tool.NewRegistry()
		reg.RegisterOrReplace(tool.ToolSpec{
			Name:        "fm.health.check",
			Title:       "Health Check",
			Description: "检查 FlashMemory HTTP 服务健康状态。",
			Version:     "v1",
			Category:    "system",
		}, func(ctx context.Context, req tool.InvokeRequest) (interface{}, error) {
			return map[string]interface{}{"code": 0, "message": "OK"}, nil
		})

		reg.RegisterOrReplace(tool.ToolSpec{
			Name:        "fm.intent.route",
			Title:       "Intent Router",
			Description: "对 query 进行意图路由，输出 intent 与 search_mode。",
			Version:     "v1",
			Category:    "routing",
			InputSchema: []tool.SchemaField{
				{Name: "query", Type: "string", Required: true, Description: "用户查询"},
				{Name: "search_mode", Type: "string", Required: false, Description: "auto/semantic/keyword/hybrid"},
			},
		}, func(ctx context.Context, req tool.InvokeRequest) (interface{}, error) {
			query := inputString(req.Input, "query")
			if strings.TrimSpace(query) == "" {
				return nil, fmt.Errorf("query is required")
			}
			searchMode := strings.TrimSpace(inputString(req.Input, "search_mode"))
			if searchMode == "" || strings.EqualFold(searchMode, router.SearchModeAuto) {
				decision := router.RouteQuery(query)
				return decision, nil
			}
			return router.IntentDecision{
				Intent:     "manual_override",
				SearchMode: searchMode,
				Confidence: 1.0,
				Signals:    []string{"manual_search_mode"},
				Reason:     "search_mode provided by tool caller",
			}, nil
		})

		reg.RegisterOrReplace(tool.ToolSpec{
			Name:        "fm.llm.route",
			Title:       "LLM Router",
			Description: "基于查询复杂度与信号进行模型路由决策。",
			Version:     "v1",
			Category:    "routing",
			InputSchema: []tool.SchemaField{
				{Name: "query", Type: "string", Required: true, Description: "用户查询"},
				{Name: "intent", Type: "string", Required: false, Description: "路由意图"},
				{Name: "search_mode", Type: "string", Required: false, Description: "semantic/keyword/hybrid/auto"},
				{Name: "strict", Type: "bool", Required: false, Description: "是否严格模式"},
				{Name: "enable_reranker", Type: "bool", Required: false, Description: "是否启用重排"},
				{Name: "limit", Type: "int", Required: false, Description: "结果条数"},
			},
		}, func(ctx context.Context, req tool.InvokeRequest) (interface{}, error) {
			query := inputString(req.Input, "query")
			if strings.TrimSpace(query) == "" {
				return nil, fmt.Errorf("query is required")
			}
			intent := inputString(req.Input, "intent")
			searchMode := inputString(req.Input, "search_mode")
			if strings.TrimSpace(intent) == "" || strings.TrimSpace(searchMode) == "" || strings.EqualFold(searchMode, router.SearchModeAuto) {
				route := router.RouteQuery(query)
				if strings.TrimSpace(intent) == "" {
					intent = route.Intent
				}
				if strings.TrimSpace(searchMode) == "" || strings.EqualFold(searchMode, router.SearchModeAuto) {
					searchMode = route.SearchMode
				}
			}
			cfg, _ := config.LoadConfig()
			decision := llm.DecideForSearch(cfg, llm.RouteInput{
				Query:          query,
				Intent:         intent,
				SearchMode:     searchMode,
				Strict:         inputBool(req.Input, "strict"),
				EnableReranker: inputBool(req.Input, "enable_reranker"),
				Limit:          inputInt(req.Input, "limit", 5),
			})
			return decision, nil
		})

		reg.RegisterOrReplace(tool.ToolSpec{
			Name:        "fm.search.plan",
			Title:       "Search Planner",
			Description: "返回 intent 路由 + LLM 路由组合决策，用于编排搜索调用链。",
			Version:     "v1",
			Category:    "planning",
			InputSchema: []tool.SchemaField{
				{Name: "query", Type: "string", Required: true, Description: "用户查询"},
				{Name: "search_mode", Type: "string", Required: false, Description: "auto/semantic/keyword/hybrid"},
				{Name: "strict", Type: "bool", Required: false, Description: "是否严格模式"},
				{Name: "enable_reranker", Type: "bool", Required: false, Description: "是否启用重排"},
				{Name: "limit", Type: "int", Required: false, Description: "结果条数"},
			},
		}, func(ctx context.Context, req tool.InvokeRequest) (interface{}, error) {
			query := inputString(req.Input, "query")
			if strings.TrimSpace(query) == "" {
				return nil, fmt.Errorf("query is required")
			}
			mode := inputString(req.Input, "search_mode")
			if strings.TrimSpace(mode) == "" || strings.EqualFold(mode, router.SearchModeAuto) {
				mode = router.RouteQuery(query).SearchMode
			}
			routeDecision := router.RouteQuery(query)
			if !strings.EqualFold(inputString(req.Input, "search_mode"), "") && !strings.EqualFold(inputString(req.Input, "search_mode"), router.SearchModeAuto) {
				routeDecision = router.IntentDecision{
					Intent:     "manual_override",
					SearchMode: mode,
					Confidence: 1.0,
					Signals:    []string{"manual_search_mode"},
					Reason:     "search_mode provided by tool caller",
				}
			}
			cfg, _ := config.LoadConfig()
			llmDecision := llm.DecideForSearch(cfg, llm.RouteInput{
				Query:          query,
				Intent:         routeDecision.Intent,
				SearchMode:     mode,
				Strict:         inputBool(req.Input, "strict"),
				EnableReranker: inputBool(req.Input, "enable_reranker"),
				Limit:          inputInt(req.Input, "limit", 5),
			})
			return map[string]interface{}{
				"route":     routeDecision,
				"llm_route": llmDecision,
			}, nil
		})

		toolRegistryInst = reg
	})
	return toolRegistryInst
}

func inputString(input map[string]interface{}, key string) string {
	if input == nil {
		return ""
	}
	v, ok := input[key]
	if !ok || v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	default:
		return fmt.Sprintf("%v", val)
	}
}

func inputBool(input map[string]interface{}, key string) bool {
	if input == nil {
		return false
	}
	v, ok := input[key]
	if !ok || v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return strings.EqualFold(strings.TrimSpace(val), "true")
	default:
		return false
	}
}

func inputInt(input map[string]interface{}, key string, fallback int) int {
	if input == nil {
		return fallback
	}
	v, ok := input[key]
	if !ok || v == nil {
		return fallback
	}
	switch val := v.(type) {
	case float64:
		return int(val)
	case int:
		return val
	case int32:
		return int(val)
	case int64:
		return int(val)
	default:
		return fallback
	}
}

func toolsListHandler() echo.HandlerFunc {
	type Req struct {
		Category string `json:"category"`
	}
	return func(c echo.Context) error {
		var req Req
		_ = json.NewDecoder(c.Request().Body).Decode(&req)
		tools := getToolRegistry().List()
		if strings.TrimSpace(req.Category) == "" {
			return c.JSON(http.StatusOK, Response{Code: 0, Message: "OK", Data: tools})
		}
		filtered := make([]tool.ToolSpec, 0, len(tools))
		for _, t := range tools {
			if strings.EqualFold(t.Category, req.Category) {
				filtered = append(filtered, t)
			}
		}
		return c.JSON(http.StatusOK, Response{Code: 0, Message: "OK", Data: filtered})
	}
}

func toolsInvokeHandler() echo.HandlerFunc {
	type Req struct {
		ProjectDir string                 `json:"project_dir"`
		RequestID  string                 `json:"request_id"`
		TraceID    string                 `json:"trace_id"`
		Tool       string                 `json:"tool"`
		Input      map[string]interface{} `json:"input"`
	}
	return func(c echo.Context) error {
		var req Req
		if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "Invalid request body"})
		}
		if strings.TrimSpace(req.Tool) == "" {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "tool is required"})
		}

		invokeRes := getToolRegistry().Invoke(c.Request().Context(), tool.InvokeRequest{
			Name:       req.Tool,
			ProjectDir: req.ProjectDir,
			RequestID:  req.RequestID,
			Input:      req.Input,
		})
		status := "success"
		msg := "OK"
		code := 0
		if !invokeRes.Success {
			status = "failed"
			msg = invokeRes.Error
			code = 1
		}
		if req.ProjectDir != "" {
			if err := telemetry.RecordToolEventWithMeta(req.ProjectDir, telemetry.EventMeta{
				RequestID: req.RequestID,
				TraceID:   req.TraceID,
			}, status, req.Tool, msg, telemetry.ToolEventData{
				Tool:       req.Tool,
				DurationMs: invokeRes.DurationMs,
				Success:    invokeRes.Success,
				Error:      invokeRes.Error,
				Input:      req.Input,
				Output:     invokeRes.Output,
			}); err != nil {
				logs.Warnf("append tool event failed: %v", err)
			}
		}
		return c.JSON(http.StatusOK, Response{Code: code, Message: msg, Data: invokeRes})
	}
}

func eventsQueryHandler() echo.HandlerFunc {
	type Req struct {
		ProjectDir string `json:"project_dir"`
		Limit      int    `json:"limit"`
		EventType  string `json:"event_type"`
		RequestID  string `json:"request_id"`
		TraceID    string `json:"trace_id"`
	}
	type ResData struct {
		Count  int               `json:"count"`
		Events []telemetry.Event `json:"events"`
	}
	return func(c echo.Context) error {
		var req Req
		if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "Invalid request body"})
		}
		if strings.TrimSpace(req.ProjectDir) == "" {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "project_dir is required"})
		}
		if req.Limit == 0 {
			req.Limit = 50
		}
		events, err := telemetry.ReadProjectEventsWithQuery(req.ProjectDir, telemetry.EventQuery{
			Limit:     req.Limit,
			EventType: req.EventType,
			RequestID: req.RequestID,
			TraceID:   req.TraceID,
		})
		if err != nil {
			return c.JSON(http.StatusInternalServerError, Response{Code: 1, Message: "读取事件失败: " + err.Error()})
		}
		return c.JSON(http.StatusOK, Response{
			Code:    0,
			Message: "OK",
			Data: ResData{
				Count:  len(events),
				Events: events,
			},
		})
	}
}

type substrateObject struct {
	ObjectID      string `json:"object_id"`
	ObjectType    string `json:"object_type"`
	AnchorKind    string `json:"anchor_kind"`
	AnchorLocator string `json:"anchor_locator"`
	Name          string `json:"name,omitempty"`
	Package       string `json:"package,omitempty"`
	File          string `json:"file,omitempty"`
	Source        string `json:"source,omitempty"`
	Page          int    `json:"page,omitempty"`
	Slide         int    `json:"slide,omitempty"`
	StartLine     int    `json:"start_line,omitempty"`
	EndLine       int    `json:"end_line,omitempty"`
}

type substrateNeighbor struct {
	Relation string          `json:"relation"`
	Object   substrateObject `json:"object"`
}

type substrateChangeSignal struct {
	FilePath   string `json:"file_path"`
	ChangeType string `json:"change_type"`
	Revision   string `json:"revision"`
	ObservedAt string `json:"observed_at"`
}

func substrateObjectResolveHandler() echo.HandlerFunc {
	type Req struct {
		ProjectDir string `json:"project_dir"`
		Locator    string `json:"locator"`
	}
	return func(c echo.Context) error {
		var req Req
		if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "Invalid request body"})
		}
		if strings.TrimSpace(req.ProjectDir) == "" {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "project_dir is required"})
		}
		if strings.TrimSpace(req.Locator) == "" {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "locator is required"})
		}

		kg, err := loadKnowledgeGraph(req.ProjectDir)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, Response{Code: 1, Message: "读取 graph.json 失败: " + err.Error()})
		}

		obj, _, ok := resolveSubstrateObject(kg, req.Locator)
		if !ok {
			return c.JSON(http.StatusOK, Response{Code: 1, Message: "object not found"})
		}
		return c.JSON(http.StatusOK, Response{Code: 0, Message: "OK", Data: obj})
	}
}

func substrateNeighborsHandler() echo.HandlerFunc {
	type Req struct {
		ProjectDir string `json:"project_dir"`
		ObjectID   string `json:"object_id"`
		Locator    string `json:"locator"`
		Direction  string `json:"direction"`
		Limit      int    `json:"limit"`
	}
	type ResData struct {
		Target    substrateObject     `json:"target"`
		Neighbors []substrateNeighbor `json:"neighbors"`
	}
	return func(c echo.Context) error {
		var req Req
		if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "Invalid request body"})
		}
		if strings.TrimSpace(req.ProjectDir) == "" {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "project_dir is required"})
		}
		if req.Limit <= 0 {
			req.Limit = 20
		}
		direction := strings.ToLower(strings.TrimSpace(req.Direction))
		if direction == "" {
			direction = "both"
		}

		kg, err := loadKnowledgeGraph(req.ProjectDir)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, Response{Code: 1, Message: "读取 graph.json 失败: " + err.Error()})
		}

		targetLocator := strings.TrimSpace(req.Locator)
		if targetLocator == "" {
			targetLocator = strings.TrimSpace(req.ObjectID)
		}
		target, targetKey, ok := resolveSubstrateObject(kg, targetLocator)
		if !ok {
			return c.JSON(http.StatusOK, Response{Code: 1, Message: "object not found"})
		}

		neighbors := make([]substrateNeighbor, 0, req.Limit)
		appendByNames := func(relation string, names []string) {
			for _, name := range names {
				if len(neighbors) >= req.Limit {
					return
				}
				obj, found := findSubstrateObjectByFunctionKey(kg, strings.TrimSpace(name))
				if !found {
					continue
				}
				neighbors = append(neighbors, substrateNeighbor{
					Relation: relation,
					Object:   obj,
				})
			}
		}

		if direction == "out" || direction == "both" {
			appendByNames("calls", kg.Calls[targetKey])
		}
		if direction == "in" || direction == "both" {
			appendByNames("called_by", kg.CalledBy[targetKey])
		}

		return c.JSON(http.StatusOK, Response{
			Code:    0,
			Message: "OK",
			Data: ResData{
				Target:    target,
				Neighbors: neighbors,
			},
		})
	}
}

func substrateChangesHandler() echo.HandlerFunc {
	type Req struct {
		ProjectDir string `json:"project_dir"`
		Since      string `json:"since"`
	}
	type ResData struct {
		Changes  []substrateChangeSignal `json:"changes"`
		Degraded bool                    `json:"degraded,omitempty"`
		Reason   string                  `json:"reason,omitempty"`
	}
	return func(c echo.Context) error {
		var req Req
		if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "Invalid request body"})
		}
		if strings.TrimSpace(req.ProjectDir) == "" {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "project_dir is required"})
		}
		observedAt := time.Now().Format(time.RFC3339Nano)

		if strings.TrimSpace(req.Since) != "" {
			out, err := runGitCommand(req.ProjectDir, "diff", "--name-status", fmt.Sprintf("%s...HEAD", req.Since))
			if err != nil {
				return c.JSON(http.StatusOK, Response{
					Code:    0,
					Message: "OK",
					Data: ResData{
						Changes:  []substrateChangeSignal{},
						Degraded: true,
						Reason:   "git diff unavailable",
					},
				})
			}
			return c.JSON(http.StatusOK, Response{
				Code:    0,
				Message: "OK",
				Data: ResData{
					Changes: parseGitNameStatus(out, req.Since, observedAt),
				},
			})
		}

		out, err := runGitCommand(req.ProjectDir, "status", "--porcelain")
		if err != nil {
			return c.JSON(http.StatusOK, Response{
				Code:    0,
				Message: "OK",
				Data: ResData{
					Changes:  []substrateChangeSignal{},
					Degraded: true,
					Reason:   "git status unavailable",
				},
			})
		}
		return c.JSON(http.StatusOK, Response{
			Code:    0,
			Message: "OK",
			Data: ResData{
				Changes: parseGitPorcelain(out, observedAt),
			},
		})
	}
}

func substrateProjectMetaHandler() echo.HandlerFunc {
	type Req struct {
		ProjectDir string `json:"project_dir"`
	}
	type ResData struct {
		ProjectDir    string `json:"project_dir"`
		GitgoReady    bool   `json:"gitgo_ready"`
		IndexReady    bool   `json:"index_ready"`
		GraphReady    bool   `json:"graph_ready"`
		EventsReady   bool   `json:"events_ready"`
		GitRevision   string `json:"git_revision,omitempty"`
		GeneratedAt   string `json:"generated_at"`
		SchemaVersion string `json:"schema_version"`
	}
	return func(c echo.Context) error {
		var req Req
		if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "Invalid request body"})
		}
		if strings.TrimSpace(req.ProjectDir) == "" {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "project_dir is required"})
		}
		absProjectDir, _ := filepath.Abs(req.ProjectDir)
		gitgoDir := filepath.Join(req.ProjectDir, ".gitgo")
		indexDB := filepath.Join(gitgoDir, "code_index.db")
		graphPath := filepath.Join(gitgoDir, "graph.json")
		eventsPath := filepath.Join(gitgoDir, "events.jsonl")

		revision := ""
		if out, err := runGitCommand(req.ProjectDir, "rev-parse", "--short", "HEAD"); err == nil {
			revision = strings.TrimSpace(out)
		}
		return c.JSON(http.StatusOK, Response{
			Code:    0,
			Message: "OK",
			Data: ResData{
				ProjectDir:    absProjectDir,
				GitgoReady:    pathExists(gitgoDir),
				IndexReady:    pathExists(indexDB),
				GraphReady:    pathExists(graphPath),
				EventsReady:   pathExists(eventsPath),
				GitRevision:   revision,
				GeneratedAt:   time.Now().Format(time.RFC3339Nano),
				SchemaVersion: telemetry.EventSchemaVersion,
			},
		})
	}
}

func substrateToolRecordsHandler() echo.HandlerFunc {
	type Req struct {
		ProjectDir string `json:"project_dir"`
		Limit      int    `json:"limit"`
		RequestID  string `json:"request_id"`
		TraceID    string `json:"trace_id"`
		Status     string `json:"status"`
	}
	type ResData struct {
		Count  int               `json:"count"`
		Events []telemetry.Event `json:"events"`
	}
	return func(c echo.Context) error {
		var req Req
		if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "Invalid request body"})
		}
		if strings.TrimSpace(req.ProjectDir) == "" {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "project_dir is required"})
		}
		if req.Limit <= 0 {
			req.Limit = 50
		}
		events, err := telemetry.ReadProjectEventsWithQuery(req.ProjectDir, telemetry.EventQuery{
			Limit:     req.Limit,
			EventType: telemetry.EventTypeToolInvoke,
			RequestID: req.RequestID,
			TraceID:   req.TraceID,
		})
		if err != nil {
			return c.JSON(http.StatusInternalServerError, Response{Code: 1, Message: "读取事件失败: " + err.Error()})
		}
		if strings.TrimSpace(req.Status) != "" {
			filtered := make([]telemetry.Event, 0, len(events))
			for _, ev := range events {
				if strings.EqualFold(ev.Status, req.Status) {
					filtered = append(filtered, ev)
				}
			}
			events = filtered
		}
		return c.JSON(http.StatusOK, Response{
			Code:    0,
			Message: "OK",
			Data: ResData{
				Count:  len(events),
				Events: events,
			},
		})
	}
}

func substrateDocTreeHandler() echo.HandlerFunc {
	type Req struct {
		ProjectDir string `json:"project_dir"`
		DocID      string `json:"doc_id"`
		Source     string `json:"source"`
	}
	type Node struct {
		NodeID    string  `json:"node_id"`
		ParentID  string  `json:"parent_id,omitempty"`
		NodeType  string  `json:"node_type"`
		Level     int     `json:"level"`
		Title     string  `json:"title,omitempty"`
		Source    string  `json:"source"`
		Page      int     `json:"page,omitempty"`
		Slide     int     `json:"slide,omitempty"`
		StartLine int     `json:"start_line,omitempty"`
		EndLine   int     `json:"end_line,omitempty"`
		Quality   float64 `json:"parse_quality,omitempty"`
	}
	type Edge struct {
		EdgeID   string  `json:"edge_id"`
		From     string  `json:"from_node_id"`
		To       string  `json:"to_node_id"`
		EdgeType string  `json:"edge_type"`
		Weight   float64 `json:"weight"`
	}
	type ResData struct {
		DocID string `json:"doc_id"`
		Nodes []Node `json:"nodes"`
		Edges []Edge `json:"edges"`
	}
	return func(c echo.Context) error {
		var req Req
		if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "Invalid request body"})
		}
		if strings.TrimSpace(req.ProjectDir) == "" {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "project_dir is required"})
		}
		db, err := index.EnsureIndexDB(req.ProjectDir)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, Response{Code: 1, Message: "open index db failed: " + err.Error()})
		}
		defer db.Close()

		docID := strings.TrimSpace(req.DocID)
		if docID == "" {
			source := strings.TrimSpace(req.Source)
			if source == "" {
				return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "doc_id or source is required"})
			}
			docID, err = index.ResolveDocIDBySource(db, req.ProjectDir, source)
			if err != nil {
				return c.JSON(http.StatusOK, Response{Code: 1, Message: "document tree not found"})
			}
		}

		rows, err := db.Query(`
SELECT node_id, parent_id, node_type, level, title, source, page, slide, start_line, end_line, parse_quality
FROM doc_nodes
WHERE project_dir = ? AND doc_id = ?
ORDER BY level ASC, start_line ASC, page ASC, slide ASC`, req.ProjectDir, docID)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, Response{Code: 1, Message: "query doc nodes failed: " + err.Error()})
		}
		defer rows.Close()
		nodes := make([]Node, 0, 64)
		for rows.Next() {
			var n Node
			var parentID sql.NullString
			if err := rows.Scan(&n.NodeID, &parentID, &n.NodeType, &n.Level, &n.Title, &n.Source, &n.Page, &n.Slide, &n.StartLine, &n.EndLine, &n.Quality); err != nil {
				return c.JSON(http.StatusInternalServerError, Response{Code: 1, Message: "scan doc nodes failed: " + err.Error()})
			}
			if parentID.Valid {
				n.ParentID = parentID.String
			}
			nodes = append(nodes, n)
		}

		edgeRows, err := db.Query(`
SELECT edge_id, from_node_id, to_node_id, edge_type, weight
FROM doc_edges
WHERE project_dir = ? AND doc_id = ?
ORDER BY created_at ASC`, req.ProjectDir, docID)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, Response{Code: 1, Message: "query doc edges failed: " + err.Error()})
		}
		defer edgeRows.Close()
		edges := make([]Edge, 0, 64)
		for edgeRows.Next() {
			var e Edge
			if err := edgeRows.Scan(&e.EdgeID, &e.From, &e.To, &e.EdgeType, &e.Weight); err != nil {
				return c.JSON(http.StatusInternalServerError, Response{Code: 1, Message: "scan doc edges failed: " + err.Error()})
			}
			edges = append(edges, e)
		}

		return c.JSON(http.StatusOK, Response{
			Code:    0,
			Message: "OK",
			Data: ResData{
				DocID: docID,
				Nodes: nodes,
				Edges: edges,
			},
		})
	}
}

func substrateDocNeighborsHandler() echo.HandlerFunc {
	type Req struct {
		ProjectDir string `json:"project_dir"`
		NodeID     string `json:"node_id"`
		Direction  string `json:"direction"` // out|in|both
		EdgeType   string `json:"edge_type"` // contains|follows|references|...
		Limit      int    `json:"limit"`
	}
	type Neighbor struct {
		EdgeID     string  `json:"edge_id"`
		EdgeType   string  `json:"edge_type"`
		FromNodeID string  `json:"from_node_id"`
		ToNodeID   string  `json:"to_node_id"`
		Weight     float64 `json:"weight"`
		Evidence   string  `json:"evidence,omitempty"`
		NeighborID string  `json:"neighbor_node_id"`
		Direction  string  `json:"direction"`
	}
	type ResData struct {
		NodeID    string     `json:"node_id"`
		Count     int        `json:"count"`
		Neighbors []Neighbor `json:"neighbors"`
	}
	return func(c echo.Context) error {
		var req Req
		if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "Invalid request body"})
		}
		if strings.TrimSpace(req.ProjectDir) == "" {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "project_dir is required"})
		}
		if strings.TrimSpace(req.NodeID) == "" {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "node_id is required"})
		}
		if req.Limit <= 0 {
			req.Limit = 50
		}
		direction := strings.ToLower(strings.TrimSpace(req.Direction))
		if direction == "" {
			direction = "both"
		}

		db, err := index.EnsureIndexDB(req.ProjectDir)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, Response{Code: 1, Message: "open index db failed: " + err.Error()})
		}
		defer db.Close()

		out := make([]Neighbor, 0, req.Limit)
		addRows := func(query string, args ...interface{}) error {
			rows, err := db.Query(query, args...)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				if len(out) >= req.Limit {
					break
				}
				var n Neighbor
				var evidence sql.NullString
				if err := rows.Scan(&n.EdgeID, &n.EdgeType, &n.FromNodeID, &n.ToNodeID, &n.Weight, &evidence); err != nil {
					return err
				}
				if evidence.Valid {
					n.Evidence = evidence.String
				}
				out = append(out, n)
			}
			return nil
		}

		edgeTypeFilter := strings.TrimSpace(req.EdgeType)
		if direction == "out" || direction == "both" {
			query := `
SELECT edge_id, edge_type, from_node_id, to_node_id, weight, evidence
FROM doc_edges
WHERE project_dir = ? AND from_node_id = ?`
			args := []interface{}{req.ProjectDir, req.NodeID}
			if edgeTypeFilter != "" {
				query += " AND edge_type = ?"
				args = append(args, edgeTypeFilter)
			}
			query += " ORDER BY created_at DESC LIMIT ?"
			args = append(args, req.Limit)
			if err := addRows(query, args...); err != nil {
				return c.JSON(http.StatusInternalServerError, Response{Code: 1, Message: "query doc_edges(out) failed: " + err.Error()})
			}
			for i := range out {
				if out[i].Direction == "" {
					out[i].Direction = "out"
					out[i].NeighborID = out[i].ToNodeID
				}
			}
		}
		if (direction == "in" || direction == "both") && len(out) < req.Limit {
			remain := req.Limit - len(out)
			query := `
SELECT edge_id, edge_type, from_node_id, to_node_id, weight, evidence
FROM doc_edges
WHERE project_dir = ? AND to_node_id = ?`
			args := []interface{}{req.ProjectDir, req.NodeID}
			if edgeTypeFilter != "" {
				query += " AND edge_type = ?"
				args = append(args, edgeTypeFilter)
			}
			query += " ORDER BY created_at DESC LIMIT ?"
			args = append(args, remain)

			before := len(out)
			if err := addRows(query, args...); err != nil {
				return c.JSON(http.StatusInternalServerError, Response{Code: 1, Message: "query doc_edges(in) failed: " + err.Error()})
			}
			for i := before; i < len(out); i++ {
				out[i].Direction = "in"
				out[i].NeighborID = out[i].FromNodeID
			}
		}

		return c.JSON(http.StatusOK, Response{
			Code:    0,
			Message: "OK",
			Data: ResData{
				NodeID:    req.NodeID,
				Count:     len(out),
				Neighbors: out,
			},
		})
	}
}

func substrateDocParseArtifactsHandler() echo.HandlerFunc {
	type Req struct {
		ProjectDir string `json:"project_dir"`
		Source     string `json:"source"`
		Status     string `json:"status"`
		Limit      int    `json:"limit"`
	}
	type Artifact struct {
		ArtifactID   string `json:"artifact_id"`
		Source       string `json:"source"`
		MimeType     string `json:"mime_type,omitempty"`
		Status       string `json:"status"`
		ErrorCode    string `json:"error_code,omitempty"`
		ErrorMessage string `json:"error_message,omitempty"`
		FallbackMode string `json:"fallback_mode,omitempty"`
		QualityJSON  string `json:"quality_json,omitempty"`
		CreatedAt    string `json:"created_at"`
	}
	type ResData struct {
		Count     int        `json:"count"`
		Artifacts []Artifact `json:"artifacts"`
	}
	return func(c echo.Context) error {
		var req Req
		if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "Invalid request body"})
		}
		if strings.TrimSpace(req.ProjectDir) == "" {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "project_dir is required"})
		}
		if req.Limit <= 0 {
			req.Limit = 50
		}

		db, err := index.EnsureIndexDB(req.ProjectDir)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, Response{Code: 1, Message: "open index db failed: " + err.Error()})
		}
		defer db.Close()

		query := `
SELECT artifact_id, source, mime_type, status, error_code, error_message, fallback_mode, quality_json, created_at
FROM parse_artifacts
WHERE project_dir = ?`
		args := []interface{}{req.ProjectDir}
		if strings.TrimSpace(req.Source) != "" {
			query += " AND source = ?"
			args = append(args, req.Source)
		}
		if strings.TrimSpace(req.Status) != "" {
			query += " AND status = ?"
			args = append(args, req.Status)
		}
		query += " ORDER BY created_at DESC LIMIT ?"
		args = append(args, req.Limit)

		rows, err := db.Query(query, args...)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, Response{Code: 1, Message: "query parse_artifacts failed: " + err.Error()})
		}
		defer rows.Close()

		list := make([]Artifact, 0, req.Limit)
		for rows.Next() {
			var a Artifact
			var mimeType sql.NullString
			var errorCode sql.NullString
			var errorMessage sql.NullString
			var fallbackMode sql.NullString
			var qualityJSON sql.NullString
			if err := rows.Scan(&a.ArtifactID, &a.Source, &mimeType, &a.Status, &errorCode, &errorMessage, &fallbackMode, &qualityJSON, &a.CreatedAt); err != nil {
				return c.JSON(http.StatusInternalServerError, Response{Code: 1, Message: "scan parse_artifacts failed: " + err.Error()})
			}
			if mimeType.Valid {
				a.MimeType = mimeType.String
			}
			if errorCode.Valid {
				a.ErrorCode = errorCode.String
			}
			if errorMessage.Valid {
				a.ErrorMessage = errorMessage.String
			}
			if fallbackMode.Valid {
				a.FallbackMode = fallbackMode.String
			}
			if qualityJSON.Valid {
				a.QualityJSON = qualityJSON.String
			}
			list = append(list, a)
		}

		return c.JSON(http.StatusOK, Response{
			Code:    0,
			Message: "OK",
			Data: ResData{
				Count:     len(list),
				Artifacts: list,
			},
		})
	}
}

func loadKnowledgeGraph(projectDir string) (graph.KnowledgeGraph, error) {
	var kg graph.KnowledgeGraph
	graphPath := filepath.Join(projectDir, ".gitgo", "graph.json")
	data, err := os.ReadFile(graphPath)
	if err != nil {
		return kg, err
	}
	if err := json.Unmarshal(data, &kg); err != nil {
		return kg, err
	}
	if kg.Anchors == nil {
		kg.Anchors = map[string]anchor.Locator{}
	}
	if kg.Calls == nil {
		kg.Calls = map[string][]string{}
	}
	if kg.CalledBy == nil {
		kg.CalledBy = map[string][]string{}
	}
	return kg, nil
}

func fullFunctionName(pkg, name string) string {
	if strings.TrimSpace(pkg) == "" {
		return strings.TrimSpace(name)
	}
	return strings.TrimSpace(pkg) + "." + strings.TrimSpace(name)
}

func resolveSubstrateObject(kg graph.KnowledgeGraph, locator string) (substrateObject, string, bool) {
	locator = strings.TrimSpace(locator)
	if locator == "" {
		return substrateObject{}, "", false
	}
	if loc, ok := kg.Anchors[locator]; ok {
		return substrateObject{
			ObjectID:      loc.ID,
			ObjectType:    "function",
			AnchorKind:    "flashmemory_anchor",
			AnchorLocator: loc.Source,
			Name:          loc.Name,
			Package:       loc.Package,
			File:          loc.File,
			Source:        loc.Source,
			Page:          loc.Page,
			Slide:         loc.Slide,
			StartLine:     loc.StartLine,
			EndLine:       loc.EndLine,
		}, fullFunctionName(loc.Package, loc.Name), true
	}
	for _, item := range kg.Functions {
		fn := item.Func
		key := fullFunctionName(fn.Package, fn.Name)
		if locator == key || locator == fn.Name || strings.EqualFold(locator, fn.File) || strings.HasSuffix(fn.File, locator) {
			return toSubstrateObject(kg, fn), key, true
		}
	}
	return substrateObject{}, "", false
}

func findSubstrateObjectByFunctionKey(kg graph.KnowledgeGraph, key string) (substrateObject, bool) {
	key = strings.TrimSpace(key)
	if key == "" {
		return substrateObject{}, false
	}
	for _, item := range kg.Functions {
		fn := item.Func
		current := fullFunctionName(fn.Package, fn.Name)
		if key == current || key == fn.Name || strings.HasSuffix(current, "."+key) {
			return toSubstrateObject(kg, fn), true
		}
	}
	if loc, ok := kg.Anchors[key]; ok {
		obj, _, found := resolveSubstrateObject(kg, loc.ID)
		return obj, found
	}
	return substrateObject{}, false
}

func toSubstrateObject(kg graph.KnowledgeGraph, fn parser.FunctionInfo) substrateObject {
	key := fullFunctionName(fn.Package, fn.Name)
	anchorID := ""
	for id, loc := range kg.Anchors {
		if fullFunctionName(loc.Package, loc.Name) != key {
			continue
		}
		if strings.TrimSpace(loc.File) != "" && strings.TrimSpace(fn.File) != "" && loc.File != fn.File {
			continue
		}
		anchorID = id
		break
	}
	if anchorID == "" {
		anchorID = key
	}
	source := fn.Source
	if strings.TrimSpace(source) == "" {
		source = fn.File
	}
	return substrateObject{
		ObjectID:      anchorID,
		ObjectType:    "function",
		AnchorKind:    "flashmemory_anchor",
		AnchorLocator: source,
		Name:          fn.Name,
		Package:       fn.Package,
		File:          fn.File,
		Source:        source,
		Page:          fn.Page,
		Slide:         fn.Slide,
		StartLine:     fn.StartLine,
		EndLine:       fn.EndLine,
	}
}

func parseGitNameStatus(output, revision, observedAt string) []substrateChangeSignal {
	signals := make([]substrateChangeSignal, 0, 32)
	lines := strings.Split(output, "\n")
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}
		statusToken := strings.TrimSpace(parts[0])
		filePath := strings.TrimSpace(parts[len(parts)-1])
		if filePath == "" {
			continue
		}
		signals = append(signals, substrateChangeSignal{
			FilePath:   filePath,
			ChangeType: mapGitStatusToken(statusToken),
			Revision:   revision,
			ObservedAt: observedAt,
		})
	}
	return signals
}

func parseGitPorcelain(output, observedAt string) []substrateChangeSignal {
	signals := make([]substrateChangeSignal, 0, 32)
	lines := strings.Split(output, "\n")
	for _, raw := range lines {
		if len(raw) < 4 {
			continue
		}
		statusToken := strings.TrimSpace(raw[0:2])
		filePath := strings.TrimSpace(raw[3:])
		if filePath == "" {
			continue
		}
		signals = append(signals, substrateChangeSignal{
			FilePath:   filePath,
			ChangeType: mapGitStatusToken(statusToken),
			Revision:   "WORKTREE",
			ObservedAt: observedAt,
		})
	}
	return signals
}

func mapGitStatusToken(token string) string {
	first := "M"
	if strings.TrimSpace(token) != "" {
		first = strings.ToUpper(strings.TrimSpace(token[:1]))
	}
	switch first {
	case "A":
		return "added"
	case "M":
		return "modified"
	case "D":
		return "deleted"
	case "R":
		return "renamed"
	case "C":
		return "copied"
	case "U":
		return "conflicted"
	case "?":
		return "untracked"
	default:
		return "modified"
	}
}

func runGitCommand(projectDir string, args ...string) (string, error) {
	allArgs := append([]string{"-C", projectDir}, args...)
	cmd := exec.Command("git", allArgs...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func toolsMCPHandler() echo.HandlerFunc {
	return func(c echo.Context) error {
		specs := getToolRegistry().List()
		tools := make([]map[string]interface{}, 0, len(specs))
		for _, s := range specs {
			props := make(map[string]interface{})
			required := make([]string, 0, len(s.InputSchema))
			for _, field := range s.InputSchema {
				props[field.Name] = map[string]interface{}{
					"type":        field.Type,
					"description": field.Description,
				}
				if field.Required {
					required = append(required, field.Name)
				}
			}
			tools = append(tools, map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name":        s.Name,
					"description": s.Description,
					"parameters": map[string]interface{}{
						"type":       "object",
						"properties": props,
						"required":   required,
					},
				},
			})
		}
		return c.JSON(http.StatusOK, Response{Code: 0, Message: "OK", Data: map[string]interface{}{"tools": tools}})
	}
}

// searchHandler handles deep search with dynamic project path
func searchHandler() echo.HandlerFunc {
	type Req struct {
		ProjectDir     string `json:"project_dir"`
		RequestID      string `json:"request_id"`
		TraceID        string `json:"trace_id"`
		Query          string `json:"query"`
		SearchMode     string `json:"search_mode"` // semantic, keyword, hybrid, auto
		Limit          int    `json:"limit"`
		Engine         string `json:"engine"`          // "zvec" | "faiss" (default: faiss, HTTP-based)
		EnableReranker bool   `json:"enable_reranker"` // Enable cross-encoder reranking
		Strict         bool   `json:"strict"`          // Strict mode (fewer results, higher quality)
	}
	type FuncRes struct {
		AnchorID    string  `json:"anchor_id,omitempty"`
		Name        string  `json:"name"`
		Package     string  `json:"package"`
		File        string  `json:"file"`
		Source      string  `json:"source,omitempty"`
		Page        int     `json:"page,omitempty"`
		Slide       int     `json:"slide,omitempty"`
		Score       float32 `json:"score"`
		Description string  `json:"description"`
		CodeSnippet string  `json:"code_snippet"`
		Type        string  `json:"type"`
		Path        string  `json:"path"`
		ParentPath  string  `json:"parent_path"`
	}
	type ResData struct {
		FuncRes  []FuncRes             `json:"func_res"`
		Tags     []string              `json:"tags"`
		Funcs    []FuncRes             `json:"funcs"`
		Modules  []FuncRes             `json:"modules"`
		Route    router.IntentDecision `json:"route,omitempty"`
		LLMRoute llm.RouteDecision     `json:"llm_route,omitempty"`
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
			req.SearchMode = router.SearchModeAuto
		}
		if req.Limit == 0 {
			req.Limit = 5
		}
		if req.Engine == "" {
			req.Engine = config.GetEngine()
		}
		resolvedMode := req.SearchMode
		routeDecision := router.IntentDecision{
			Intent:     "manual_override",
			SearchMode: req.SearchMode,
			Confidence: 1.0,
			Signals:    []string{"manual_search_mode"},
			Reason:     "search_mode provided by client",
		}
		if strings.EqualFold(req.SearchMode, router.SearchModeAuto) {
			routeDecision = router.RouteQuery(req.Query)
			resolvedMode = routeDecision.SearchMode
		}
		if resolvedMode == "" {
			resolvedMode = router.SearchModeSemantic
		}
		logs.Infof("Search request: query=%s, mode=%s, intent=%s, engine=%s, reranker=%v, strict=%v",
			req.Query, resolvedMode, routeDecision.Intent, req.Engine, req.EnableReranker, req.Strict)

		gitgoDir := filepath.Join(req.ProjectDir, ".gitgo")

		// 索引文件路径
		indexDBPath := filepath.Join(gitgoDir, "code_index.db")
		faissIndexPath := filepath.Join(gitgoDir, "code_index.local")
		if req.Engine != "local" {
			faissIndexPath = filepath.Join(gitgoDir, "code_index.faiss")
		}
		ext := ".local"
		if req.Engine != "local" {
			logs.Infof("正在启动高级向量服务...")
			ext = ".faiss"
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

		cfg, _ := config.LoadConfig()
		llmDecision := llm.DecideForSearch(cfg, llm.RouteInput{
			Query:          req.Query,
			Intent:         routeDecision.Intent,
			SearchMode:     resolvedMode,
			Strict:         req.Strict,
			EnableReranker: req.EnableReranker,
			Limit:          req.Limit,
		})
		logs.Infof("LLM route: provider=%s, model=%s, reasoning=%s, budget=%d, complexity=%d",
			llmDecision.Provider, llmDecision.ModelID, llmDecision.ReasoningMode, llmDecision.ContextBudget, llmDecision.Complexity)

		// 维度统一从 cfg.ZvecConfig.Dimension 读取，faiss 和 zvec 都生效。
		// 旧实现里 faiss 硬编码 128，导致 1024 维 BGE 向量被截断，召回质量崩塌。
		dim := config.ResolveVectorDim(config.GetEngine(), cfg)

		// Determine if we need to auto-build vectors BEFORE creating the wrapper
		// (wrapper creation via initCollection(type="both") creates empty zvec collection directories,
		// so these checks MUST happen before NewFaissWrapperByEngine)
		needBuildVectors := false
		needBuildModuleVectors := false
		if config.GetEngine() == "zvec" || req.Engine == "zvec" {
			zvecFuncPath := filepath.Join(absGitgoDir, "zvec_collections", "functions")
			if _, err := os.Stat(zvecFuncPath); os.IsNotExist(err) {
				needBuildVectors = true
			}
			zvecModulePath := filepath.Join(absGitgoDir, "zvec_collections", "modules")
			if _, err := os.Stat(zvecModulePath); os.IsNotExist(err) {
				needBuildModuleVectors = true
			}
		} else {
			if _, err := os.Stat(faissIndexPath); os.IsNotExist(err) {
				needBuildVectors = true
			}
		}

		zvecCollPath := filepath.Join(absGitgoDir, "zvec_collections")
		// 创建FaissWrapper，传入存储路径选项
		faissOptions := map[string]interface{}{
			"storage_path":    absGitgoDir,
			"collection_path": zvecCollPath,
			"server_url":      index.DefaultFaissServerURL,
			"index_id":        req.ProjectDir,
			"python_path":     cfg.ZvecConfig.PythonPath,
		}
		faissWrapper, err := index.NewFaissWrapperByEngine(req.Engine, dim, faissOptions)
		if err != nil {
			logs.Errorf("Failed to initialize vector engine: %v", err)
			return c.JSON(http.StatusInternalServerError, Response{Code: 1, Message: fmt.Sprintf("Vector engine init failed: %v", err)})
		}
		idx := &index.Indexer{DB: db, FaissIndex: faissWrapper}
		defer idx.FaissIndex.Free()

		if needBuildVectors {
			logs.Infof("Auto-building vector index from existing DB data...")
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

		minScore := float32(0.1)
		if config.GetEngine() == "zvec" || req.Engine == "zvec" {
			minScore = 0.0 // RRF scores can be very small, disable MinScore filtering for Zvec hybrids
		}

		// 构造搜索选项
		opts := search.SearchOptions{
			Limit:        req.Limit,
			MinScore:     minScore,
			IncludeCode:  true,
			SearchMode:   resolvedMode,
			KeywordModel: llmDecision.ModelID,
		}

		// SearchEngine，传入索引器
		engine := &search.SearchEngine{
			Indexer:      idx,
			Descriptions: make(map[int]string),
			ProjDir:      req.ProjectDir,
		}

		fmt.Printf("查找函数: %s (模式: %s, 意图: %s)\n", req.Query, opts.SearchMode, routeDecision.Intent)
		results, err := engine.Query(req.Query, opts)
		if common.IsLLMError(err) {
			return c.JSON(http.StatusOK, Response{Code: 5, Message: err.Error()})
		}
		if err != nil {
			return c.JSON(http.StatusOK, Response{Code: 1, Message: "搜索失败"})
		}

		// Top-K-per-file rebalance: 防止 chunk 数极不均衡的项目里某一份文档（如 53 chunk
		// 的 spec.md）按 score 排序后垄断 top-K，让每个文件都至少有露脸机会。
		// 策略：先按原 score 顺序，每个文件先取前 maxPerFile（=3）条进 head；超出的进
		// tail，最终 head + tail 拼接（tail 仍按原 score 排）。
		const maxPerFile = 3
		results = rebalanceTopKPerFile(results, maxPerFile)

		var data []FuncRes
		for _, r := range results {
			funcRes := FuncRes{
				AnchorID:    r.AnchorID,
				Name:        r.Name,
				Package:     r.Package,
				File:        r.File,
				Source:      r.Source,
				Page:        r.Page,
				Slide:       r.Slide,
				Score:       r.Score,
				Description: r.Description,
				CodeSnippet: r.CodeSnippet,
				Type:        r.Type,
				Path:        r.Path,
				ParentPath:  r.ParentPath,
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
			"python_path":  cfg.ZvecConfig.PythonPath,
		}

		var faissModuleIndex index.FaissWrapper
		if config.GetEngine() == "zvec" {
			faissModuleIndex = idx.FaissIndex
		} else {
			var moduleErr error
			faissModuleIndex, moduleErr = index.NewFaissWrapperByEngine(req.Engine, dim, faissModuleOptions)
			if moduleErr != nil {
				logs.Warnf("Failed to initialize module vector engine: %v", moduleErr)
				// Continue without module search - not fatal
				faissModuleIndex = nil
			} else {
				defer faissModuleIndex.Free()
			}
		}

		// Module search: only proceed if module engine initialized successfully
		if faissModuleIndex != nil {
			idxModule := &index.Indexer{DB: db, FaissIndex: faissModuleIndex}

			// needBuildModuleVectors was pre-computed before wrapper creation (see above)
			// For Zvec engines, also check if DB has code_desc rows but collection is empty
			// (directory exists but was created empty by initCollection)
			if !needBuildModuleVectors && (config.GetEngine() == "zvec" || req.Engine == "zvec") {
				var codeDescCount int
				err := db.QueryRow("SELECT COUNT(*) FROM code_desc").Scan(&codeDescCount)
				if err == nil && codeDescCount > 0 {
					// DB has module data - check if Zvec collection actually has vectors
					// by doing a probe search with a zero vector
					zeroVec := make([]float32, dim)
					probeResults := faissModuleIndex.SearchModuleVectors(zeroVec, 1)
					if len(probeResults) == 0 {
						logs.Infof("Zvec module collection is empty but DB has %d code_desc records, triggering rebuild", codeDescCount)
						needBuildModuleVectors = true
					}
				}
			} else if !needBuildModuleVectors && config.GetEngine() != "zvec" && req.Engine != "zvec" {
				// For non-zvec engines, check faiss module file existence here
				if _, err := os.Stat(faissModulePath); os.IsNotExist(err) {
					needBuildModuleVectors = true
				}
			}

			if needBuildModuleVectors {
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
					Limit:        req.Limit,
					MinScore:     0.1,
					IncludeCode:  true,
					SearchMode:   resolvedMode,
					KeywordModel: llmDecision.ModelID,
				}
				// SearchEngine，传入索引器
				engine = &search.SearchEngine{
					Indexer:      idxModule,
					Descriptions: make(map[int]string),
					ProjDir:      req.ProjectDir,
					Module:       true,
				}
				fmt.Printf("查找模块: %s (模式: %s, 意图: %s)\n", req.Query, opts.SearchMode, routeDecision.Intent)
				results, err = engine.Query(req.Query, opts)
				if common.IsLLMError(err) {
					return c.JSON(http.StatusOK, Response{Code: 5, Message: err.Error()})
				}
				if err != nil {
					return c.JSON(http.StatusOK, Response{Code: 1, Message: "搜索失败"})
				}
				for _, r := range results {
					funcRes := FuncRes{
						AnchorID:    r.AnchorID,
						Name:        r.Name,
						Package:     r.Package,
						File:        r.File,
						Source:      r.Source,
						Page:        r.Page,
						Slide:       r.Slide,
						Score:       r.Score,
						Description: r.Description,
						CodeSnippet: r.CodeSnippet,
						Type:        r.Type,
						Path:        r.Path,
						ParentPath:  r.ParentPath,
					}
					data = append(data, funcRes)
					Modules = append(Modules, funcRes)
				}
				keywords = append(keywords, engine.Keywords...)
			}
		} // end if faissModuleIndex != nil

		// 对data基于Score得分排序
		sort.Slice(data, func(i, j int) bool {
			return data[i].Score > data[j].Score
		})

		var resData = ResData{
			data,
			keywords,
			Funcs,
			Modules,
			routeDecision,
			llmDecision,
		}
		if strings.TrimSpace(req.ProjectDir) != "" {
			if err := telemetry.RecordSearchEventWithMeta(req.ProjectDir, telemetry.EventMeta{
				RequestID: req.RequestID,
				TraceID:   req.TraceID,
				Cost: &telemetry.EventCost{
					Hint:            llmDecision.CostHint,
					EstimatedTokens: llmDecision.ContextBudget,
				},
			}, "success", "search completed", telemetry.SearchEventData{
				Query:       req.Query,
				SearchMode:  resolvedMode,
				Route:       routeDecision,
				LLMRoute:    llmDecision,
				ResultCount: len(data),
			}); err != nil {
				logs.Warnf("append search event failed: %v", err)
			}
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
		Engine      string   `json:"engine,omitempty"`
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
		if req.Engine == "" {
			req.Engine = config.GetEngine()
		}
		err = back.BuildIndex(req.ProjectDir, req.RelativeDir, full, req.Engine != "local", req.Engine)
		if common.IsLLMError(err) {
			return c.JSON(http.StatusOK, Response{Code: 5, Message: err.Error()})
		}
		// 并发拒绝走单独的 code=409，便于 gateway 区分"暂时性、应等待重试"
		// 与"业务/系统错误、不应继续重试"。配合 acquireIndexLock 可消除
		// gateway retry 风暴并发踩坏 zvec collection 的根因。
		if errors.Is(err, back.ErrIndexInProgress) {
			return c.JSON(http.StatusOK, Response{Code: 409, Message: err.Error()})
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
		Engine     string `json:"engine,omitempty"`
	}

	return func(c echo.Context) error {
		var req Req
		if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "Invalid request body"})
		}
		if req.ProjectDir == "" {
			return c.JSON(http.StatusBadRequest, Response{Code: 1, Message: "project_dir is required"})
		}
		if req.Engine == "" {
			req.Engine = config.GetEngine()
		}
		err := back.IncrementalUpdate(req.ProjectDir, req.Branch, req.Commit, req.Engine != "local", req.Engine)
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
		Source      string `json:"source,omitempty"`
		Page        int    `json:"page,omitempty"`
		Slide       int    `json:"slide,omitempty"`
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

		// 打开数据库连接并执行 schema/migration 对齐
		db, err := index.EnsureIndexDB(req.ProjectDir)
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
			query = "SELECT name, package, file, source, page, slide, start_line, end_line, description FROM functions WHERE file LIKE ? ORDER BY file, start_line"
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
			query = "SELECT name, package, file, source, page, slide, start_line, end_line, description FROM functions ORDER BY file, start_line"
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
				var source sql.NullString
				var page sql.NullInt64
				var slide sql.NullInt64
				var description sql.NullString

				err := rows.Scan(&fn.Name, &packageName, &fn.File, &source, &page, &slide, &fn.StartLine, &fn.EndLine, &description)
				if err != nil {
					return c.JSON(http.StatusOK, Response{Code: 1, Message: "数据解析失败: " + err.Error()})
				}

				if packageName.Valid {
					fn.Package = packageName.String
				}
				if description.Valid {
					fn.Description = description.String
				}
				if source.Valid {
					fn.Source = source.String
				}
				if page.Valid {
					fn.Page = int(page.Int64)
				}
				if slide.Valid {
					fn.Slide = int(slide.Int64)
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

// rebalanceTopKPerFile reorders search results so that no single file occupies
// more than maxPerFile slots in the head of the list. Files with overflow have
// their extra chunks pushed to the tail (preserving relative order). This is a
// purely cosmetic post-process; final ordering remains stable with respect to
// the underlying score ranking when the input already has balanced files.
//
// Why this exists: in dev_files projects it is common for a single oversized
// markdown spec to produce 50+ chunks while sibling docx files produce 1-2.
// Without rebalance, the spec's chunks dominate every search and other files
// are functionally invisible. maxPerFile = 3 keeps the spec's most relevant
// pieces visible while reserving slots for cross-file diversity.
func rebalanceTopKPerFile(results []search.SearchResult, maxPerFile int) []search.SearchResult {
	if maxPerFile <= 0 || len(results) == 0 {
		return results
	}
	fileCount := make(map[string]int, 8)
	head := make([]search.SearchResult, 0, len(results))
	tail := make([]search.SearchResult, 0)
	for _, r := range results {
		key := r.File
		if key == "" {
			// Module-level results have empty File; treat each as its own bucket.
			head = append(head, r)
			continue
		}
		if fileCount[key] < maxPerFile {
			head = append(head, r)
			fileCount[key]++
		} else {
			tail = append(tail, r)
		}
	}
	if len(tail) == 0 {
		return head
	}
	return append(head, tail...)
}
