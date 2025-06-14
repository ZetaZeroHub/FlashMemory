# 函数重要性排序算法 (Function Ranking Algorithm)

## 概述

这个包实现了一个高性能的函数重要性评级算法，用于对代码库中的函数进行智能排序。算法综合考虑函数的调用关系、复杂度和在调用图中的位置，为每个函数计算重要性得分，从而帮助开发者优先分析对整体语义理解贡献最大的函数。

## 核心特性

- **高性能**: 使用并发计算，支持大规模函数集合的快速排序
- **高扩展性**: 支持自定义权重配置，适应不同的分析需求
- **高并发**: 内部使用协程池进行并行计算
- **智能评分**: 综合多个维度计算函数重要性

## 算法原理

### 评分维度

算法从以下四个维度评估函数重要性：

1. **Fan-In (入度)**: 调用该函数的其他函数数量
   - 反映函数作为公共依赖的程度
   - 被调用次数多的函数通常更重要

2. **Fan-Out (出度)**: 该函数调用的其他函数数量
   - 反映函数的逻辑广度
   - 调用很多其他函数的函数通常是协调者角色

3. **Depth (深度)**: 函数在调用图中的层级位置
   - 较顶层的函数往往对全局逻辑影响更直接
   - 使用DFS算法计算调用深度

4. **Complexity (复杂度)**: 函数自身的代码复杂度
   - 基于代码行数和调用数量估算
   - 复杂函数通常包含更多业务逻辑

### 评分公式

```
重要性得分 = α×log(1+Fan-In) + β×log(1+Fan-Out) + γ×log(1+Depth) + δ×log(1+Complexity)
```

其中：
- α, β, γ, δ 是可配置的权重参数
- 使用对数函数进行标准化，避免某一维度过度影响结果
- 默认权重：α=0.4, β=0.2, γ=0.2, δ=0.2

## 使用方法

### 基本使用

```go
package main

import (
    "fmt"
    "github.com/kinglegendzzh/flashmemory/internal/ranking"
    "github.com/kinglegendzzh/flashmemory/internal/parser"
)

func main() {
    // 1. 准备函数数据
    functions := []parser.FunctionInfo{
        {
            Name:    "main",
            Package: "main",
            Calls:   []string{"utils.Init", "service.Start"},
            Lines:   20,
        },
        // ... 更多函数
    }

    // 2. 创建排序器
    ranker := ranking.NewFunctionRanker(nil) // 使用默认配置

    // 3. 执行排序（从低分到高分）
    rankedFunctions := ranker.RankFunctions(functions)

    // 4. 查看结果
    for _, fn := range rankedFunctions {
        fmt.Printf("函数: %s, 得分: %.3f\n", fn.Name, fn.Score)
    }
}
```

### 自定义配置

```go
// 创建自定义配置
customConfig := &ranking.RankingConfig{
    Alpha: 0.6, // 更重视Fan-In
    Beta:  0.1, // 降低Fan-Out权重
    Gamma: 0.2, // 保持深度权重
    Delta: 0.1, // 降低复杂度权重
}

// 使用自定义配置创建排序器
ranker := ranking.NewFunctionRanker(customConfig)

// 按降序排序（高分到低分）
highToLow := ranker.RankFunctionsByScore(functions, false)
```

### 动态更新配置

```go
ranker := ranking.NewFunctionRanker(nil)

// 运行时更新配置
newConfig := &ranking.RankingConfig{
    Alpha: 0.5,
    Beta:  0.3,
    Gamma: 0.1,
    Delta: 0.1,
}
ranker.UpdateConfig(newConfig)
```

## API 参考

### 类型定义

```go
type RankingConfig struct {
    Alpha float64 // Fan-In权重
    Beta  float64 // Fan-Out权重
    Gamma float64 // 深度权重
    Delta float64 // 复杂度权重
}

type FunctionRanker struct {
    // 内部实现
}
```

### 主要方法

#### `NewFunctionRanker(config *RankingConfig) *FunctionRanker`
创建新的函数排序器。如果config为nil，则使用默认配置。

#### `RankFunctions(functions []parser.FunctionInfo) []parser.FunctionInfo`
对函数列表进行重要性排序（升序，从低分到高分）。

#### `RankFunctionsByScore(functions []parser.FunctionInfo, ascending bool) []parser.FunctionInfo`
按指定顺序排序函数列表。
- `ascending=true`: 升序排序（低分到高分）
- `ascending=false`: 降序排序（高分到低分）

#### `UpdateConfig(config *RankingConfig)`
更新排序器的配置参数。

#### `GetConfig() *RankingConfig`
获取当前的配置参数。

## 性能特性

### 并发处理
- 使用协程池并发计算函数指标
- 默认使用4个工作协程，可根据需要调整
- 适合处理大规模函数集合

### 时间复杂度
- 构建调用图: O(n + m)，其中n是函数数量，m是调用关系数量
- 计算深度: O(n + m)
- 排序: O(n log n)
- 总体: O(n log n + m)

### 空间复杂度
- O(n + m)，主要用于存储调用图和中间结果

## 应用场景

### 代码分析优化
```go
// 按重要性分组处理
highPriority := []parser.FunctionInfo{}
mediumPriority := []parser.FunctionInfo{}
lowPriority := []parser.FunctionInfo{}

for _, fn := range rankedFunctions {
    if fn.Score > 2.0 {
        highPriority = append(highPriority, fn)
    } else if fn.Score > 1.0 {
        mediumPriority = append(mediumPriority, fn)
    } else {
        lowPriority = append(lowPriority, fn)
    }
}

// 优先分析重要函数
analyzeHighPriorityFunctions(highPriority)
```

### 代码重构指导
- 识别核心函数，重点优化
- 发现过度耦合的函数（高Fan-Out）
- 找出关键依赖（高Fan-In）

### 测试优先级
- 优先为重要函数编写测试
- 重点测试高复杂度函数

## 配置建议

### 不同场景的权重配置

1. **重视核心依赖**（推荐用于库分析）:
   ```go
   config := &RankingConfig{Alpha: 0.6, Beta: 0.1, Gamma: 0.2, Delta: 0.1}
   ```

2. **重视业务复杂度**（推荐用于业务代码分析）:
   ```go
   config := &RankingConfig{Alpha: 0.2, Beta: 0.2, Gamma: 0.1, Delta: 0.5}
   ```

3. **重视调用层次**（推荐用于架构分析）:
   ```go
   config := &RankingConfig{Alpha: 0.2, Beta: 0.2, Gamma: 0.5, Delta: 0.1}
   ```

4. **均衡配置**（默认配置）:
   ```go
   config := &RankingConfig{Alpha: 0.4, Beta: 0.2, Gamma: 0.2, Delta: 0.2}
   ```

## 测试

运行测试：
```bash
go test ./internal/ranking
```

运行基准测试：
```bash
go test -bench=. ./internal/ranking
```

## 注意事项

1. **循环依赖处理**: 算法能够检测并处理循环依赖，避免无限递归
2. **函数标识**: 使用"包名.函数名"作为函数的唯一标识
3. **并发安全**: 排序器支持并发使用，内部使用读写锁保护配置
4. **内存使用**: 对于大型代码库，注意内存使用情况

## 扩展性

算法设计具有良好的扩展性，可以轻松添加新的评分维度：

1. 在`FunctionInfo`结构体中添加新字段
2. 在`calculateSingleFunctionMetrics`中添加计算逻辑
3. 在`calculateScore`中添加新维度的权重
4. 在`RankingConfig`中添加对应的权重参数

这种设计使得算法能够适应不同的分析需求和代码特征。