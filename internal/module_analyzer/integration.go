package module_analyzer

import (
	"database/sql"
	"os"
	"path/filepath"

	"github.com/kinglegendzzh/flashmemory/config"
	"github.com/kinglegendzzh/flashmemory/internal/analyzer"
	"github.com/kinglegendzzh/flashmemory/internal/index"
	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"

	_ "modernc.org/sqlite"
)

// AnalyzeAllModules 在函数分析完成后，进行模块级分析
// 该函数应在 llmAnalyzer.AnalyzeAll 之后调用
// skipLLM 参数控制是否跳过 LLM 生成描述，如果为 true，则所有描述将为空字符串
func AnalyzeAllModules(results []analyzer.LLMAnalysisResult, db *sql.DB, projDir string, cfg *config.Config, skipLLM bool, subPath string) error {

	logs.Infof("开始模块级分析，共有 %d 个函数结果", len(results))
	if subPath != "" {
		logs.Infof("局部分析，只分析 %s 目录", subPath)
	}

	// 初始化 code_desc 表，但即使失败也继续执行
	if err := index.InitCodeDescDb(db); err != nil {
		logs.Warnf("初始化 code_desc 表失败: %v，但将继续执行模块分析", err)
		// 不再返回错误，也不更新任务状态为失败
	}

	// 复制原始结果数据，避免依赖区域外的数据
	resultsCopy := make([]analyzer.LLMAnalysisResult, len(results))
	copy(resultsCopy, results)

	// 获取项目数据库文件路径并创建独立的数据库连接
	gitgoDir := filepath.Join(projDir, ".gitgo")
	dbFilePath := filepath.Join(gitgoDir, "code_index.db")

	// 检查数据库文件，但即使不存在也继续
	var newDb *sql.DB
	dbFileExists := true
	if _, err := os.Stat(dbFilePath); os.IsNotExist(err) {
		logs.Warnf("数据库文件不存在: %s，将以无数据库模式继续执行", dbFilePath)
		dbFileExists = false
	}

	// 如果数据库文件存在，尝试创建连接，但失败也继续
	if dbFileExists {
		var err error
		newDb, err = sql.Open("sqlite", dbFilePath)
		if err != nil {
			logs.Warnf("创建数据库连接失败: %v，将以无数据库模式继续执行", err)
			newDb = nil
		} else {
			// 测试数据库连接，失败则关闭连接并继续
			if err := newDb.Ping(); err != nil {
				logs.Warnf("测试数据库连接失败: %v，将以无数据库模式继续执行", err)
				newDb.Close()
				newDb = nil
			}
		}
	}

	// 创建任务ID并返回给调用方
	taskID := ""
	if !skipLLM {
		taskID = RegisterTask(projDir)
	}
	// 创建模块分析器，即使数据库连接为nil也能正常工作
	moduleAnalyzer := NewModuleAnalyzer(
		newDb, // 可能为nil，ModuleAnalyzer内部需要处理此情况
		projDir,
		cfg,
		2,
		true,    // 启用调试模式
		taskID,  // 传入任务ID
		skipLLM, // 是否跳过LLM描述生成
		subPath,
	)

	// 在新协程中执行模块分析，不阻塞调用方
	go func() {
		var tempFile string
		if !skipLLM {
			// 在本地生成一个临时temp文件，标记为正在分析
			tempFile = filepath.Join(projDir, ".gitgo", "module_analyzer.temp")
			os.Create(tempFile)
		}
		// 确保当goroutine完成时关闭数据库连接（如果不为nil）
		if newDb != nil {
			defer newDb.Close()

			// 尝试再次初始化代码描述表，但失败也继续
			if err := index.InitCodeDescDb(newDb); err != nil {
				logs.Warnf("在异步任务中初始化code_desc表失败: %v，但将继续执行模块分析", err)
				// 不再返回，继续执行后续代码
			}
		}

		err := moduleAnalyzer.AnalyzeModules(resultsCopy)
		if err != nil {
			logs.Errorf("模块分析失败: %v", err)
			// 任务状态更新已在 AnalyzeModules 中处理
		} else {
			logs.Infof("模块分析完成")
		}
		if tempFile != "" {
			logs.Infof("删除临时文件: %s", tempFile)
			// 删除临时文件
			os.Remove(tempFile)
		}
	}()

	return nil
}
