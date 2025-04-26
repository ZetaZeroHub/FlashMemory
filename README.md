> # 快速开始

## 扫描外部代码

```shell
cd ~/Public/openProject
go run flashmemory/cmd/main/main.go -query 'getEnvPythonPath'
```

## 扫描本项目

```shell
cd /Users/apple/Public/openProject/flashmemory
go run cmd/main/main.go -query 'getEnvPythonPath' -faiss_path '/Users/apple/Public/openProject/flashmemory/cmd/main/FAISSService'
```

```shell
# 语义搜索模式
-search_mode semantic, keyword, hybrid
```

# FlashMemory 主程序说明书

## 概述

FlashMemory 是一个跨语言代码分析和语义搜索系统，具有以下核心功能：

1. 多语言支持：解析Go/Python/JavaScript/Java/C++代码
2. 智能分析：使用LLM生成函数描述和语义理解
3. 增量索引：基于Git变更记录的高效更新机制
4. 混合搜索：支持语义搜索、关键词搜索和混合模式

## 命令行参数

| 参数             | 默认值       | 说明                                                                |
| ---------------- | ------------ | ------------------------------------------------------------------- |
| `-dir`         | `.`        | 要索引的项目目录路径                                                |
| `-query`       | `""`       | 自然语言查询，支持中文和英文                                        |
| `-faiss`       | `cpu`      | Faiss类型：`cpu`(默认)或 `gpu`(需CUDA)                          |
| `-search_mode` | `semantic` | 搜索模式：`semantic`(语义)、`keyword`(关键词)、`hybrid`(混合) |
| `-force_full`  | `false`    | 强制全量索引，忽略增量更新                                          |
| `-commit`      | `""`       | 指定commit hash进行索引，空则使用当前HEAD                           |
| `-branch`      | `master`   | 指定分支名称                                                        |
| `-faiss_path`  | `""`       | 手动指定FAISSService目录绝对路径                                    |
| `-file`        | `""`       | 指定要定量更新的文件/文件夹路径，存在则更新否则新增                 |
| `-query_only`  | `false`    | 仅查询模式，跳过索引构建                                            |

## 核心模块

1. **Parser模块**：

   - 基于AST和正则的多语言解析器
   - 支持函数/类/方法级别的代码提取
2. **Analyzer模块**：

   - LLM驱动的智能分析
   - 自动生成函数描述和重要性评分
3. **Index模块**：

   - 双存储引擎(SQLite+Faiss)
   - 支持增量更新和分支管理
4. **Search模块**：

   - 混合搜索算法
   - 结果排序和相关性评分

## 工作流程

### 1. 环境准备

1. 查找 FAISSService 目录：

   - 尝试从源文件目录查找（适用于 `go run`）
   - 尝试从可执行文件目录查找（适用于编译后的二进制文件）
   - 尝试从当前工作目录查找
2. 检查 Python 环境：

   - 验证 Python 是否已安装
   - 检查必要的库是否已安装
   - 如果缺少，自动创建 `.env` 虚拟环境并安装所需依赖
3. 启动 Faiss 服务：

   - 启动 Python 脚本 `faiss_server.py`
   - 轮询检测服务是否成功启动
   - 设置程序结束时自动停止服务

### 2. 索引策略确定

1. 创建 `.gitgo` 目录用于存储索引文件
2. 分支索引管理：

   - 不同分支的索引独立存储，通过 `-branch`参数指定
   - 每个分支记录最后索引的commit hash，用于增量更新
   - 切换分支时会自动加载对应分支的索引
3. 检查是否存在索引文件，决定是全量索引还是增量更新：

   - 如果指定了 `-force_full`，则进行全量索引
   - 如果存在索引文件且有 `.git` 目录，则尝试增量更新
   - 增量更新时，获取变更文件列表，只处理变更的文件

### 3. 代码解析与分析

1. 遍历项目目录，查找支持的代码文件（`.go`、`.py`、`.js`、`.java`、`.cpp`）
2. 使用适当的解析器解析每个文件，提取函数信息
3. 使用 LLM 分析器分析每个函数：
   - 提取代码片段
   - 构建提示词
   - 调用大语言模型生成函数描述
   - 计算重要性评分

### 4. 知识图谱与索引构建

1. 构建知识图谱，表示函数之间的依赖关系
2. 初始化索引存储（SQLite 和 Faiss）
3. 将分析结果保存到数据库
4. 为每个函数生成嵌入向量并添加到 Faiss 索引
5. 保存索引到文件
6. 更新分支索引信息（如果适用）

### 5. 查询处理

如果提供了 `-query` 参数：

1. 构造搜索选项
2. 创建搜索引擎
3. 执行查询并显示结果

## 主要模块交互

主程序与以下内部模块交互：

- `parser`：解析代码文件，提取函数信息
- `analyzer`：分析函数，生成描述
- `graph`：构建知识图谱
- `index`：处理 SQLite 和 Faiss 存储
- `search`：实现对索引的查询
- `utils`：提供各种工具函数
- `visualize`：生成统计数据

## 使用示例

组合使用-file与分支参数：

```bash
# 更新指定文件并关联到develop分支

go run cmd/main/main.go -dir /path/to/project -file src/utils/logger.go -branch develop

# 指定commit更新特定文件夹

go run cmd/main/main.go -dir /path/to/project -file config/ -commit 8a3b1f2 -branch feature-auth
```

基本索引：

```bash
go run flashmemory/cmd/main/main.go -dir /path/to/project
```

索引并查询：

```bash
go run flashmemory/cmd/main/main.go -dir /path/to/project -query "如何处理文件上传"
```

强制全量索引：

```bash
go run flashmemory/cmd/main/main.go -dir /path/to/project -force_full
```

指定分支和提交：

```bash
go run flashmemory/cmd/main/main.go -dir /path/to/project -branch develop -commit abc123
```

增量索引（仅更新变更文件）：

```bash
go run flashmemory/cmd/main/main.go -dir /path/to/project
```

混合搜索模式（语义+关键词）：

```bash
go run flashmemory/cmd/main/main.go -dir /path/to/project -query "文件上传" -search_mode hybrid
```

GPU加速索引（需要CUDA环境）：

```bash
go run flashmemory/cmd/main/main.go -dir /path/to/project -faiss gpu
```

仅查询模式（使用已有索引）：

```bash
go run flashmemory/cmd/main/main.go -dir /path/to/project -query "文件上传" -query_only
```

自定义FAISS索引路径：

```bash
go run flashmemory/cmd/main/main.go -dir /path/to/project -faiss_path /custom/path/to/faiss_index
```

## 注意事项

1. 首次运行时会自动安装必要的 Python 依赖，可能需要一些时间
2. 索引过程中会创建 `.gitgo` 目录存储索引文件
3. 增量更新依赖于 Git 历史记录，非 Git 项目将始终进行全量索引
4. 大型项目的初始索引可能需要较长时间，特别是在使用大语言模型分析函数时



# 代码组织结构：

```
flashmemory/                 (project root)
├─ cmd/
│   └─ tree/               (Tree-sitter 交互式查询工具)
│       ├─ tree.go        (命令行工具入口)
│       └─ README.md      (使用说明)
├─ internal/
│   ├─ parser/
│   │    ├─ parser.go            (定义 Parser 接口和通用结构)
│   │    ├─ tree_sitter_parser.go (Tree-sitter 实现的多语言解析器)
│   │    ├─ tree_sitter_parser_test.go (Tree-sitter 解析器测试)
│   │    ├─ tree_sitter_parser_imports_calls_test.go (导入和调用解析测试)
│   │    ├─ ast_parser.go        (基于 AST 的实现)
│   │    ├─ regex_parser.go      (基于正则表达式的实现)
│   │    └─ llm_parser.go        (基于 LLM 的实现)
│   ├─ analyzer/
│   │    └─ analyzer.go          (分析函数，生成描述)
│   ├─ graph/
│   │    └─ graph.go             (构建依赖图，知识图谱结构)
│   ├─ index/
│   │    ├─ index.go             (处理 SQLite 和 Faiss 存储)
│   │    ├─ faiss_wrapper.go     (Faiss 封装接口)
│   │    ├─ http_faiss_wrapper.go (HTTP 方式的 Faiss 调用)
│   │    └─ memory_faiss_wrapper.go (内存方式的 Faiss 实现)
│   ├─ search/
│   │    └─ search.go            (实现对索引的查询)
│   └─ visualize/
│        └─ visualize.go         (生成统计数据和可能的图表)
├─ pkg/                          (外部使用的包)
│   └─ ... (可选)
├─ .gitgo/                       (输出索引目录，运行时创建)
└─ go.mod                        (模块定义，依赖项配置)
```

