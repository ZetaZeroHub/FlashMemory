package ranking

import (
	"fmt"
	"strings"

	"github.com/kinglegendzzh/flashmemory/internal/parser"
)

// Example 展示如何使用函数排序算法
func Example() {
	// 1. 创建示例函数数据
	functions := []parser.FunctionInfo{
		{
			Name:         "main",
			Package:      "main",
			File:         "main.go",
			Calls:        []string{"config.Load", "service.Start", "utils.Init"},
			Lines:        25,
			StartLine:    1,
			EndLine:      25,
			FunctionType: "function",
		},
		{
			Name:         "Load",
			Package:      "config",
			File:         "config/config.go",
			Calls:        []string{"os.Getenv", "json.Unmarshal"},
			Lines:        15,
			StartLine:    10,
			EndLine:      25,
			FunctionType: "function",
		},
		{
			Name:         "Start",
			Package:      "service",
			File:         "service/server.go",
			Calls:        []string{"utils.Init", "http.ListenAndServe"},
			Lines:        30,
			StartLine:    50,
			EndLine:      80,
			FunctionType: "function",
		},
		{
			Name:         "Init",
			Package:      "utils",
			File:         "utils/utils.go",
			Calls:        []string{"log.SetOutput", "config.Load"},
			Lines:        20,
			StartLine:    1,
			EndLine:      20,
			FunctionType: "function",
		},
		{
			Name:         "ProcessData",
			Package:      "service",
			File:         "service/processor.go",
			Calls:        []string{"utils.Init", "database.Query", "cache.Set"},
			Lines:        45,
			StartLine:    100,
			EndLine:      145,
			FunctionType: "function",
		},
	}

	fmt.Println("=== Example of function importance ranking algorithm ===")
	fmt.Printf("Number of original functions: %d\n\n", len(functions))

	// 2. 创建排序器（使用默认配置）
	ranker := NewFunctionRanker(nil)
	fmt.Println("Use default configuration:")
	config := ranker.GetConfig()
	fmt.Printf("Alpha (Fan-In weight): %.2f\n", config.Alpha)
	fmt.Printf("Beta (Fan-Out weight): %.2f\n", config.Beta)
	fmt.Printf("Gamma (depth weight): %.2f\n", config.Gamma)
	fmt.Printf("Delta (complexity weight): %.2f\n\n", config.Delta)

	// 3. 执行排序（从低分到高分）
	rankedFunctions := ranker.RankFunctions(functions)

	// 4. 显示排序结果
	fmt.Println("=== Sort results (from low to high importance) ===")
	fmt.Printf("%-20s %-15s %-6s %-7s %-5s %-10s %-8s\n",
		"函数名", "包名", "Fan-In", "Fan-Out", "深度", "复杂度", "得分")
	fmt.Println(strings.Repeat("-", 80))

	for i, fn := range rankedFunctions {
		fmt.Printf("%-20s %-15s %-6d %-7d %-5d %-10d %-8.3f\n",
			fn.Name, fn.Package, fn.FanIn, fn.FanOut, fn.Depth, fn.Complexity, fn.Score)
		if i == 0 {
			fmt.Println("↑ Minimum importance (last analysis recommended)")
		} else if i == len(rankedFunctions)-1 {
			fmt.Println("↑ Highest importance (recommended to be analyzed first)")
		}
	}

	// 5. 演示降序排序（高分到低分）
	fmt.Println("\n=== Sort results in descending order (from high to low importance) ===")
	descendingFunctions := ranker.RankFunctionsByScore(functions, false)
	for i, fn := range descendingFunctions {
		if i < 3 { // 只显示前3个最重要的函数
			fmt.Printf("%d. %s.%s (score: %.3f)\n", i+1, fn.Package, fn.Name, fn.Score)
		}
	}

	// 6. 演示自定义配置
	fmt.Println("\n=== Use custom configuration ===")
	customConfig := &RankingConfig{
		Alpha: 0.6, // 更重视Fan-In
		Beta:  0.1, // 降低Fan-Out权重
		Gamma: 0.2, // 保持深度权重
		Delta: 0.1, // 降低复杂度权重
	}
	customRanker := NewFunctionRanker(customConfig)
	customRanked := customRanker.RankFunctionsByScore(functions, false)

	fmt.Println("Custom configuration emphasizes the importance of the number of calls (Fan-In):")
	for i, fn := range customRanked {
		if i < 3 {
			fmt.Printf("%d. %s.%s (Fan-In: %d, Score: %.3f)\n",
				i+1, fn.Package, fn.Name, fn.FanIn, fn.Score)
		}
	}

	// 7. 性能统计
	fmt.Println("\n=== Algorithm characteristics ===")
	fmt.Println("✓ High performance: supports concurrent computing and is suitable for large-scale function collections")
	fmt.Println("✓ High scalability: customizable weight configuration")
	fmt.Println("✓ High concurrency: Internally uses coroutine pool for parallel computing")
	fmt.Println("✓ Intelligent sorting: taking into account Fan-In, Fan-Out, depth and complexity")
}

// ExampleUsageInAnalysis 展示在代码分析中的实际应用
func ExampleUsageInAnalysis(functions []parser.FunctionInfo) {
	fmt.Println("=== Application examples in code analysis ===")

	// 创建排序器
	ranker := NewFunctionRanker(nil)

	// 按重要性排序（从高到低）
	sortedFunctions := ranker.RankFunctionsByScore(functions, false)

	// 分批处理：优先处理重要函数
	highPriorityFunctions := []parser.FunctionInfo{}
	mediumPriorityFunctions := []parser.FunctionInfo{}
	lowPriorityFunctions := []parser.FunctionInfo{}

	// 根据得分分组
	for _, fn := range sortedFunctions {
		if fn.Score > 2.0 {
			highPriorityFunctions = append(highPriorityFunctions, fn)
		} else if fn.Score > 1.0 {
			mediumPriorityFunctions = append(mediumPriorityFunctions, fn)
		} else {
			lowPriorityFunctions = append(lowPriorityFunctions, fn)
		}
	}

	fmt.Printf("High priority functions: %d\n", len(highPriorityFunctions))
	fmt.Printf("Medium priority functions: %d\n", len(mediumPriorityFunctions))
	fmt.Printf("Low priority functions: %d\n", len(lowPriorityFunctions))

	// 建议的分析策略
	fmt.Println("\nRecommended analysis strategies:")
	fmt.Println("1. Prioritize analysis of high-priority functions (core business logic)")
	fmt.Println("2. Secondly, analyze the medium priority functions (important supporting functions)")
	fmt.Println("3. Finally analyze low-priority functions (auxiliary tool functions)")
}
