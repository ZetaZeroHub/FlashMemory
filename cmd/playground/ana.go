package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kinglegendzzh/flashmemory/internal/analyzer"
	"github.com/kinglegendzzh/flashmemory/internal/parser"
	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"
)

func main() {
	// 使用项目中的真实Go文件进行测试
	//testFile := "/Users/apple/Public/openProject/flashmemory/internal/index/memory_faiss_wrapper.go"
	//testFile := "/Users/apple/Public/asiainfo/ideaProjects/migu-cpcnc-pricing-repository/migu_cpn/cpn-charge/charge-modules/charge-modules-external/src/main/java/com/asiainfo/external/facade/rest/service/impl/OrdResourceDtoCommonService.java"
	testFile := "/Users/apple/Public/openProject/util/git_util.go"

	// 解析文件获取函数信息
	var allFuncs []parser.FunctionInfo
	lang := parser.DetectLang(testFile)
	p := parser.NewParser(lang)
	funcs, _ := p.ParseFile(testFile)
	allFuncs = append(allFuncs, funcs...)
	indent, _ := json.MarshalIndent(funcs, "", "  ")
	logs.Infof("FunctionInfo %s", indent)

	// 创建分析器实例
	llmAnalyzer := analyzer.NewLLMAnalyzer(&sync.Map{}, true, 3)

	// 分析函数
	results, err := llmAnalyzer.AnalyzeAll(allFuncs)
	if err != nil {
		logs.Errorf("Error analyzing functions: %v", err)
		return
	}

	// 将验证结果以json格式保存至本地log
	resultJSON, _ := json.MarshalIndent(results, "", "  ")
	timeNow := time.Now().Format("2006-01-02_15_04_05")
	logDir := "/Users/apple/Public/openProject/logs"
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		err = os.MkdirAll(logDir, 0755)
	}
	logFile := filepath.Join(logDir, "analysis_results_"+timeNow+".json")
	_ = os.WriteFile(logFile, resultJSON, 0644)
	logs.Infof("Analysis results saved to: %s", logFile)
}
