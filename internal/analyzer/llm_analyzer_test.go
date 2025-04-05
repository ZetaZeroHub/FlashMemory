package analyzer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/kinglegendzzh/flashmemory/internal/parser"
	"github.com/kinglegendzzh/flashmemory/internal/utils/logs"
)

func TestNewLLMAnalyzer(t *testing.T) {
	initialKnown := map[string]string{
		"testFunc": "测试函数描述",
	}

	// 测试创建实例
	analyzer := NewLLMAnalyzer(initialKnown, true, 3)
	assert.NotNil(t, analyzer)
	assert.Equal(t, initialKnown, analyzer.KnownDescriptions)
	assert.True(t, analyzer.debug)
}

func TestExtractCodeSnippet(t *testing.T) {
	// 创建临时测试文件
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")
	testContent := "package test\n\nfunc TestFunc() {\n\treturn\n}\n"
	err := os.WriteFile(testFile, []byte(testContent), 0644)
	assert.NoError(t, err)

	// 测试正常提取
	snippet, err := extractCodeSnippet(testFile, 1, 3)
	assert.NoError(t, err)
	assert.Contains(t, snippet, "package test")
	assert.Contains(t, snippet, "func TestFunc()")

	// 测试无效行号
	_, err = extractCodeSnippet(testFile, 0, 10)
	assert.Error(t, err)

	// 测试文件不存在
	_, err = extractCodeSnippet("nonexistent.go", 1, 2)
	assert.Error(t, err)
}

func TestAnalyzeFunction(t *testing.T) {
	// 创建测试用例
	testFunc := parser.FunctionInfo{
		Name:      "TestFunc",
		File:      "test.go",
		Package:   "test",
		StartLine: 1,
		EndLine:   5,
		Lines:     5,
		Calls:     []string{"internalFunc", "external.Func"},
		Imports:   []string{"external"},
	}

	// 创建分析器实例
	analyzer := NewLLMAnalyzer(map[string]string{"internalFunc": "内部函数描述"}, true, 3)

	// 分析函数
	result := analyzer.AnalyzeFunction(testFunc)

	// 验证结果
	assert.Equal(t, testFunc, result.Func)
	assert.Contains(t, result.InternalDeps, "internalFunc")
	assert.Contains(t, result.ExternalDeps, "external")
	assert.NotEmpty(t, result.Description)
	assert.Greater(t, result.ImportanceScore, float64(0))
}

func TestAnalyzeAll(t *testing.T) {
	// 创建临时测试文件
	tmpDir := t.TempDir()
	testFile1 := filepath.Join(tmpDir, "test1.go")
	testFile2 := filepath.Join(tmpDir, "test2.go")
	testContent1 := "package test\n\nfunc Func1() {\n\tFunc2()\n}"
	testContent2 := "package test\n\nfunc Func2() {\n\treturn\n}"
	err := os.WriteFile(testFile1, []byte(testContent1), 0644)
	assert.NoError(t, err)
	err = os.WriteFile(testFile2, []byte(testContent2), 0644)
	assert.NoError(t, err)

	// 创建测试函数列表
	testFuncs := []parser.FunctionInfo{
		{
			Name:      "Func1",
			File:      testFile1,
			Package:   "test",
			StartLine: 3,
			EndLine:   5,
			Lines:     3,
			Calls:     []string{"Func2"},
		},
		{
			Name:      "Func2",
			File:      testFile2,
			Package:   "test",
			StartLine: 3,
			EndLine:   5,
			Lines:     3,
			Calls:     []string{},
		},
	}

	// 创建分析器实例并初始化KnownDescriptions
	analyzer := NewLLMAnalyzer(map[string]string{}, true, 3)

	// 分析所有函数
	results := analyzer.AnalyzeAll(testFuncs)

	// 验证结果
	assert.Len(t, results, 2)
	assert.Equal(t, "Func2", results[0].Func.Name) // 应该先分析无依赖的函数
	assert.Equal(t, "Func1", results[1].Func.Name)
}

func TestRealFileAnalysis(t *testing.T) {
	// 使用项目中的真实Go文件进行测试
	//testFile := "/Users/apple/Public/openProject/flashmemory/internal/index/memory_faiss_wrapper.go"
	testFile := "/Users/apple/Public/asiainfo/ideaProjects/migu-cpcnc-pricing-repository/migu_cpn/cpn-charge/charge-modules/charge-modules-external/src/main/java/com/asiainfo/external/facade/rest/service/impl/OrdResourceDtoCommonService.java"

	// 解析文件获取函数信息
	var allFuncs []parser.FunctionInfo
	lang := parser.DetectLang(testFile)
	p := parser.NewParser(lang)
	funcs, err := p.ParseFile(testFile)
	allFuncs = append(allFuncs, funcs...)
	indent, err := json.MarshalIndent(funcs, "", "  ")
	logs.Infof("FunctionInfo %s", indent)
	assert.NoError(t, err)
	assert.NotEmpty(t, funcs)

	// 创建分析器实例
	analyzer := NewLLMAnalyzer(map[string]string{}, true, 3)

	// 分析函数
	results := analyzer.AnalyzeAll(allFuncs)

	// 将验证结果以json格式保存至本地log
	resultJSON, err := json.MarshalIndent(results, "", "  ")
	assert.NoError(t, err)
	timeNow := time.Now().Format("2006-01-02_15_04_05")
	logDir := "/Users/apple/Public/openProject/logs"
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		err = os.MkdirAll(logDir, 0755)
		assert.NoError(t, err, "Failed to create logs directory")
	}
	logFile := filepath.Join(logDir, "analysis_results_"+timeNow+".json")
	err = os.WriteFile(logFile, resultJSON, 0644)
	assert.NoError(t, err, "Failed to write analysis results to file")
	logs.Infof("Analysis results saved to: %s", logFile)
}
