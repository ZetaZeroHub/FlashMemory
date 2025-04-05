> # quick start

# 扫描外部代码
```shell
cd ~/Public/openProject
go run flashmemory/cmd/main/main.go -query 'getEnvPythonPath'
```

# 扫描本项目
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

FlashMemory 是一个代码分析和搜索系统，能够解析多种编程语言的代码，提取函数信息，使用大语言模型生成函数描述，构建知识图谱，并支持自然语言查询。主程序 `main.go` 是整个系统的入口点，负责协调各个模块完成索引和搜索功能。

## 命令行参数

主程序支持以下命令行参数：

| 参数             | 默认值       | 说明                                                                   |
| ---------------- | ------------ | ---------------------------------------------------------------------- |
| `-dir`         | `.`        | 要索引的项目目录路径                                                   |
| `-query`       | `""`       | 用于搜索代码库的自然语言查询                                           |
| `-faiss`       | `cpu`      | 要安装的 Faiss 类型：`cpu` 或 `gpu`（使用 GPU 版本需要 CUDA 支持） |
| `-search_mode` | `semantic` | 搜索模式：`semantic`、`keyword` 或 `hybrid`                      |
| `-force_full`  | `false`    | 强制进行全量索引，忽略增量更新                                         |
| `-commit`      | `""`       | 指定特定的 commit hash 进行索引，为空则使用当前 HEAD                   |
| `-branch`      | `master`   | 指定分支名称                                                           |

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
2. 检查是否存在索引文件，决定是全量索引还是增量更新：
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

## 注意事项

1. 首次运行时会自动安装必要的 Python 依赖，可能需要一些时间
2. 索引过程中会创建 `.gitgo` 目录存储索引文件
3. 增量更新依赖于 Git 历史记录，非 Git 项目将始终进行全量索引
4. 大型项目的初始索引可能需要较长时间，特别是在使用大语言模型分析函数时
