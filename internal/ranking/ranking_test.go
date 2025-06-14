package ranking

import (
	"fmt"
	"testing"
	"time"

	"github.com/kinglegendzzh/flashmemory/internal/parser"
)

// 创建测试用的函数信息
func createTestFunctions() []parser.FunctionInfo {
	return []parser.FunctionInfo{
		{
			Name:         "main",
			Package:      "main",
			Calls:        []string{"utils.Init", "service.Start", "config.Load"},
			Lines:        20,
			FunctionType: "function",
		},
		{
			Name:         "Init",
			Package:      "utils",
			Calls:        []string{"logger.Setup", "db.Connect"},
			Lines:        15,
			FunctionType: "function",
		},
		{
			Name:         "Start",
			Package:      "service",
			Calls:        []string{"utils.Init", "handler.Register"},
			Lines:        25,
			FunctionType: "function",
		},
		{
			Name:         "Load",
			Package:      "config",
			Calls:        []string{},
			Lines:        10,
			FunctionType: "function",
		},
		{
			Name:         "Setup",
			Package:      "logger",
			Calls:        []string{},
			Lines:        8,
			FunctionType: "function",
		},
		{
			Name:         "Connect",
			Package:      "db",
			Calls:        []string{"config.Load"},
			Lines:        12,
			FunctionType: "function",
		},
		{
			Name:         "Register",
			Package:      "handler",
			Calls:        []string{"utils.Init"},
			Lines:        18,
			FunctionType: "function",
		},
	}
}

func TestNewFunctionRanker(t *testing.T) {
	// 测试默认配置
	ranker := NewFunctionRanker(nil)
	if ranker == nil {
		t.Fatal("Expected non-nil ranker")
	}

	config := ranker.GetConfig()
	if config.Alpha != 0.4 {
		t.Errorf("Expected Alpha=0.4, got %f", config.Alpha)
	}

	// 测试自定义配置
	customConfig := &RankingConfig{
		Alpha: 0.5,
		Beta:  0.3,
		Gamma: 0.1,
		Delta: 0.1,
	}
	ranker2 := NewFunctionRanker(customConfig)
	config2 := ranker2.GetConfig()
	if config2.Alpha != 0.5 {
		t.Errorf("Expected Alpha=0.5, got %f", config2.Alpha)
	}
}

func TestRankFunctions(t *testing.T) {
	functions := createTestFunctions()
	ranker := NewFunctionRanker(nil)

	// 执行排序
	rankedFunctions := ranker.RankFunctions(functions)

	// 验证结果
	if len(rankedFunctions) != len(functions) {
		t.Errorf("Expected %d functions, got %d", len(functions), len(rankedFunctions))
	}

	// 验证指标已被计算
	for i, fn := range rankedFunctions {
		if fn.Score == 0 {
			t.Errorf("Function %d (%s) has zero score", i, fn.Name)
		}
		t.Logf("Function: %s.%s, FanIn: %d, FanOut: %d, Depth: %d, Complexity: %d, Score: %.3f",
			fn.Package, fn.Name, fn.FanIn, fn.FanOut, fn.Depth, fn.Complexity, fn.Score)
	}

	// 验证排序是否正确（升序）
	for i := 1; i < len(rankedFunctions); i++ {
		if rankedFunctions[i-1].Score > rankedFunctions[i].Score {
			t.Errorf("Functions not sorted correctly: %f > %f",
				rankedFunctions[i-1].Score, rankedFunctions[i].Score)
		}
	}
}

func TestRankFunctionsByScore(t *testing.T) {
	functions := createTestFunctions()
	ranker := NewFunctionRanker(nil)

	// 测试升序排序
	ascending := ranker.RankFunctionsByScore(functions, true)
	for i := 1; i < len(ascending); i++ {
		if ascending[i-1].Score > ascending[i].Score {
			t.Errorf("Ascending sort failed: %f > %f",
				ascending[i-1].Score, ascending[i].Score)
		}
	}

	// 测试降序排序
	descending := ranker.RankFunctionsByScore(functions, false)
	for i := 1; i < len(descending); i++ {
		if descending[i-1].Score < descending[i].Score {
			t.Errorf("Descending sort failed: %f < %f",
				descending[i-1].Score, descending[i].Score)
		}
	}
}

func TestFanInFanOut(t *testing.T) {
	functions := createTestFunctions()
	ranker := NewFunctionRanker(nil)
	rankedFunctions := ranker.RankFunctions(functions)

	// 验证特定函数的Fan-In和Fan-Out
	for _, fn := range rankedFunctions {
		switch fn.Package + "." + fn.Name {
		case "utils.Init":
			// utils.Init被main、service.Start、handler.Register调用
			if fn.FanIn != 3 {
				t.Errorf("Expected utils.Init FanIn=3, got %d", fn.FanIn)
			}
			// utils.Init调用logger.Setup和db.Connect
			if fn.FanOut != 2 {
				t.Errorf("Expected utils.Init FanOut=2, got %d", fn.FanOut)
			}
		case "config.Load":
			// config.Load被main和db.Connect调用
			if fn.FanIn != 2 {
				t.Errorf("Expected config.Load FanIn=2, got %d", fn.FanIn)
			}
			// config.Load不调用其他函数
			if fn.FanOut != 0 {
				t.Errorf("Expected config.Load FanOut=0, got %d", fn.FanOut)
			}
		case "logger.Setup":
			// logger.Setup只被utils.Init调用
			if fn.FanIn != 1 {
				t.Errorf("Expected logger.Setup FanIn=1, got %d", fn.FanIn)
			}
		}
	}
}

func TestUpdateConfig(t *testing.T) {
	ranker := NewFunctionRanker(nil)
	newConfig := &RankingConfig{
		Alpha: 0.6,
		Beta:  0.2,
		Gamma: 0.1,
		Delta: 0.1,
	}

	ranker.UpdateConfig(newConfig)
	updatedConfig := ranker.GetConfig()

	if updatedConfig.Alpha != 0.6 {
		t.Errorf("Expected Alpha=0.6, got %f", updatedConfig.Alpha)
	}
}

func TestEmptyFunctions(t *testing.T) {
	ranker := NewFunctionRanker(nil)
	emptyFunctions := []parser.FunctionInfo{}

	result := ranker.RankFunctions(emptyFunctions)
	if len(result) != 0 {
		t.Errorf("Expected empty result, got %d functions", len(result))
	}
}

func TestPerformance(t *testing.T) {
	// 创建大量测试函数
	functions := make([]parser.FunctionInfo, 1000)
	for i := 0; i < 1000; i++ {
		functions[i] = parser.FunctionInfo{
			Name:         fmt.Sprintf("func%d", i),
			Package:      fmt.Sprintf("pkg%d", i%10),
			Calls:        []string{fmt.Sprintf("pkg%d.func%d", (i+1)%10, (i+1)%1000)},
			Lines:        10 + i%50,
			FunctionType: "function",
		}
	}

	ranker := NewFunctionRanker(nil)
	start := time.Now()
	ranker.RankFunctions(functions)
	duration := time.Since(start)

	t.Logf("Ranking 1000 functions took: %v", duration)

	// 性能要求：1000个函数的排序应该在1秒内完成
	if duration > time.Second {
		t.Errorf("Performance test failed: took %v, expected < 1s", duration)
	}
}

func BenchmarkRankFunctions(b *testing.B) {
	functions := createTestFunctions()
	ranker := NewFunctionRanker(nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 每次都需要重新创建函数切片，因为排序会修改原始数据
		testFuncs := make([]parser.FunctionInfo, len(functions))
		copy(testFuncs, functions)
		ranker.RankFunctions(testFuncs)
	}
}

func BenchmarkRankFunctionsLarge(b *testing.B) {
	// 创建较大的函数集合进行基准测试
	functions := make([]parser.FunctionInfo, 100)
	for i := 0; i < 100; i++ {
		functions[i] = parser.FunctionInfo{
			Name:         fmt.Sprintf("func%d", i),
			Package:      fmt.Sprintf("pkg%d", i%5),
			Calls:        []string{fmt.Sprintf("pkg%d.func%d", (i+1)%5, (i+1)%100)},
			Lines:        10 + i%30,
			FunctionType: "function",
		}
	}

	ranker := NewFunctionRanker(nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		testFuncs := make([]parser.FunctionInfo, len(functions))
		copy(testFuncs, functions)
		ranker.RankFunctions(testFuncs)
	}
}
