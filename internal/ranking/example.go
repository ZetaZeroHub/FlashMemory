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

	fmt.Println("=== 函数重要性排序算法示例 ===")
	fmt.Printf("原始函数数量: %d\n\n", len(functions))

	// 2. 创建排序器（使用默认配置）
	ranker := NewFunctionRanker(nil)
	fmt.Println("使用默认配置:")
	config := ranker.GetConfig()
	fmt.Printf("  Alpha (Fan-In权重): %.2f\n", config.Alpha)
	fmt.Printf("  Beta (Fan-Out权重): %.2f\n", config.Beta)
	fmt.Printf("  Gamma (深度权重): %.2f\n", config.Gamma)
	fmt.Printf("  Delta (复杂度权重): %.2f\n\n", config.Delta)

	// 3. 执行排序（从低分到高分）
	rankedFunctions := ranker.RankFunctions(functions)

	// 4. 显示排序结果
	fmt.Println("=== 排序结果（按重要性从低到高） ===")
	fmt.Printf("%-20s %-15s %-6s %-7s %-5s %-10s %-8s\n",
		"函数名", "包名", "Fan-In", "Fan-Out", "深度", "复杂度", "得分")
	fmt.Println(strings.Repeat("-", 80))

	for i, fn := range rankedFunctions {
		fmt.Printf("%-20s %-15s %-6d %-7d %-5d %-10d %-8.3f\n",
			fn.Name, fn.Package, fn.FanIn, fn.FanOut, fn.Depth, fn.Complexity, fn.Score)
		if i == 0 {
			fmt.Println("  ↑ 最低重要性（建议最后分析）")
		} else if i == len(rankedFunctions)-1 {
			fmt.Println("  ↑ 最高重要性（建议优先分析）")
		}
	}

	// 5. 演示降序排序（高分到低分）
	fmt.Println("\n=== 降序排序结果（按重要性从高到低） ===")
	descendingFunctions := ranker.RankFunctionsByScore(functions, false)
	for i, fn := range descendingFunctions {
		if i < 3 { // 只显示前3个最重要的函数
			fmt.Printf("%d. %s.%s (得分: %.3f)\n", i+1, fn.Package, fn.Name, fn.Score)
		}
	}

	// 6. 演示自定义配置
	fmt.Println("\n=== 使用自定义配置 ===")
	customConfig := &RankingConfig{
		Alpha: 0.6, // 更重视Fan-In
		Beta:  0.1, // 降低Fan-Out权重
		Gamma: 0.2, // 保持深度权重
		Delta: 0.1, // 降低复杂度权重
	}
	customRanker := NewFunctionRanker(customConfig)
	customRanked := customRanker.RankFunctionsByScore(functions, false)

	fmt.Println("自定义配置强调被调用次数(Fan-In)的重要性:")
	for i, fn := range customRanked {
		if i < 3 {
			fmt.Printf("%d. %s.%s (Fan-In: %d, 得分: %.3f)\n",
				i+1, fn.Package, fn.Name, fn.FanIn, fn.Score)
		}
	}

	// 7. 性能统计
	fmt.Println("\n=== 算法特性 ===")
	fmt.Println("✓ 高性能：支持并发计算，适合大规模函数集合")
	fmt.Println("✓ 高扩展性：可自定义权重配置")
	fmt.Println("✓ 高并发：内部使用协程池进行并行计算")
	fmt.Println("✓ 智能排序：综合考虑Fan-In、Fan-Out、深度和复杂度")
}

// ExampleUsageInAnalysis 展示在代码分析中的实际应用
func ExampleUsageInAnalysis(functions []parser.FunctionInfo) {
	fmt.Println("=== 在代码分析中的应用示例 ===")

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

	fmt.Printf("高优先级函数: %d 个\n", len(highPriorityFunctions))
	fmt.Printf("中优先级函数: %d 个\n", len(mediumPriorityFunctions))
	fmt.Printf("低优先级函数: %d 个\n", len(lowPriorityFunctions))

	// 建议的分析策略
	fmt.Println("\n建议的分析策略:")
	fmt.Println("1. 优先分析高优先级函数（核心业务逻辑）")
	fmt.Println("2. 其次分析中优先级函数（重要支撑功能）")
	fmt.Println("3. 最后分析低优先级函数（辅助工具函数）")
}
