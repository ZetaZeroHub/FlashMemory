<div align="center">

# ⚡ FlashMemory

**跨语言代码分析与语义搜索系统**

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go&logoColor=white)](https://go.dev)
[![Python](https://img.shields.io/badge/Python-3.8+-3776AB?style=flat&logo=python&logoColor=white)](https://python.org)
[![License](https://img.shields.io/badge/License-MIT-green?style=flat)](LICENSE)
[![Version](https://img.shields.io/badge/Version-0.4.0-blue?style=flat)]()

[English README](README.md)

</div>

---

## FlashMemory 是什么？

FlashMemory 通过 LLM 驱动的代码分析和向量搜索，为你的代码库建立索引，实现跨 Go、Python、JavaScript、Java、C++ 的**自然语言代码搜索**。

**核心能力：**

- 🔍 **混合搜索** — Dense（语义）+ Sparse（关键词）向量，RRF 融合排序
- 🧠 **LLM 分析** — 自动生成函数描述和重要性评分
- 📊 **知识图谱** — 函数调用关系和模块依赖可视化
- ⚡ **增量索引** — 基于 Git 的智能更新，仅重新索引变更文件
- 🔌 **MCP 集成** — 通过 Model Context Protocol 向 AI Agent 暴露搜索工具
- 🏎️ **Zvec 引擎** — 进程内向量数据库，无需外部服务

---

## 安装

```bash
# 安装 Python SDK（含本地向量模型，推荐）
pip install flashmemory[embedding]

# 基础安装（使用降级 embedding）
pip install flashmemory

# 全功能安装（含云端 Embedding）
pip install flashmemory[full]

# 从源码编译 Go CLI
go build -o fm cmd/main/fm.go
```

---

## 快速开始

### 命令行使用

```bash
# 索引项目（推荐使用 Zvec 引擎，无需启动 FAISS 服务）
fm -dir /path/to/project -engine zvec

# 自然语言搜索
fm -dir /path/to/project -engine zvec -query "文件上传处理"

# 混合搜索模式（语义 + 关键词）
fm -dir /path/to/project -query "认证鉴权" -search_mode hybrid

# 增量索引（只更新变更文件）
fm -dir /path/to/project

# 指定文件/目录重新索引
fm -dir /path/to/project -file src/handlers/

# 强制全量索引
fm -dir /path/to/project -force_full

# 兼容传统 FAISS 模式（向后兼容）
fm -dir /path/to/project
```

### Python SDK

```python
from flashmemory import FlashMemoryClient

# Context Manager 方式（推荐）
with FlashMemoryClient(project_dir="/path/to/project") as client:
    # 语义搜索
    results = client.search("文件上传处理器", top_k=10)
    for r in results:
        print(f"{r['fields'].get('func_name')} → {r['score']:.3f}")

    # 按语言过滤
    results = client.search_functions("认证中间件", language="go")

    # 搜索模块级别
    results = client.search_modules("搜索引擎模块")

    # 生成向量
    vec = client.embed("搜索查询文本")

    # 添加函数到索引
    client.add_function("func_1", "处理文件上传", {
        "func_name": "UploadFile",
        "package": "handlers",
        "language": "go",
    })
```

### MCP 工具集成（AI Agent）

```python
from flashmemory import get_mcp_tools, handle_mcp_tool_call

# 获取 MCP 工具定义，注册到 MCP Server
tools = get_mcp_tools()
# 包含三个工具：
# - flashmemory_search:  代码语义搜索
# - flashmemory_index:   添加函数到索引
# - flashmemory_info:    获取引擎诊断信息

# 处理 AI Agent 的工具调用
client_cache = {}  # 跨请求复用客户端
result = handle_mcp_tool_call(
    "flashmemory_search",
    {
        "project_dir": "/path/to/project",
        "query": "数据库连接池管理",
        "top_k": 5,
        "language": "go",
    },
    client_cache=client_cache,
)
```

---

## 整体架构

```
┌──────────────────────────────────────┐
│  FlashMemoryClient (Python SDK)      │  高级 SDK API
├──────────────────────────────────────┤
│  SearchPipeline（检索管线）           │  召回 → 精排
│  EmbeddingProvider（嵌入提供器）      │  Dense + Sparse 向量生成
├──────────────────────────────────────┤
│  ZvecEngine（Collection 管理）       │  向量存储与检索
│  ZvecBridge（JSON-line 协议）        │  Go ↔ Python 进程间通信
├──────────────────────────────────────┤
│  Go 核心层                           │
│  Parser · Analyzer · Graph · Index   │  代码分析管线
│  Search · Ranking · Embedding        │  搜索与评分
├──────────────────────────────────────┤
│  存储层                              │
│  Zvec（HNSW + Sparse）· SQLite       │  向量 + 元数据
│  FAISS（传统兼容）· 文件系统          │  向后兼容
└──────────────────────────────────────┘
```

---

## 命令行参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-dir` | `.` | 要索引的项目目录 |
| `-query` | `""` | 自然语言查询（支持中英文） |
| `-engine` | `faiss` | 向量引擎：`zvec`（推荐）或 `faiss` |
| `-search_mode` | `semantic` | 搜索模式：`semantic` / `keyword` / `hybrid` |
| `-force_full` | `false` | 强制全量索引，跳过增量检测 |
| `-branch` | `master` | Git 分支名 |
| `-commit` | `""` | 指定 commit hash |
| `-file` | `""` | 指定索引的文件或子目录 |
| `-query_only` | `false` | 仅查询模式，跳过索引构建 |

---

## HTTP API

启动 HTTP API 服务：

```bash
fm serve --port 5532
```

主要接口：

| 方法 | 路由 | 说明 |
|------|------|------|
| GET | `/api/health` | 健康检查 |
| POST | `/api/search` | 代码搜索（语义 / 关键词 / 混合） |
| POST | `/api/index` | 构建索引 |
| DELETE | `/api/index` | 删除索引 |
| POST | `/api/index/incremental` | 增量更新 |
| POST | `/api/index/check` | 检查索引状态 |
| POST | `/api/functions` | 函数列表 |
| POST | `/api/ranking` | 函数重要性评级 |
| POST | `/api/module-graphs/update` | 更新模块图谱（异步） |
| GET | `/c/config` | 获取配置 |
| PUT | `/c/config` | 更新配置 |

完整 API 文档见 [HTTP API 深度分析](docs/http_api_deep_analysis.md)。

---

## 支持语言

| 语言 | 解析器 | 文件扩展名 |
|------|--------|------------|
| Go | AST | `.go` |
| Python | Tree-sitter | `.py` |
| JavaScript / TypeScript | Tree-sitter | `.js` `.ts` `.jsx` `.tsx` |
| Java | Tree-sitter | `.java` |
| C / C++ | Tree-sitter | `.c` `.cpp` `.h` `.hpp` |

---

## 配置文件

FlashMemory 使用 `fm.yaml` 进行项目配置：

```yaml
# LLM 配置
api_url: "https://api.openai.com/v1"
api_model: "gpt-4o-mini"
api_token: "sk-..."

# Zvec 向量引擎（推荐，v0.4.0+）
zvec_config:
  collection_path: ".gitgo/zvec_collections"  # 向量数据存储路径
  dimension: 384                               # 向量维度
  metric_type: "cosine"                        # 相似度计算方式
```

---

## 项目结构

```
flashmemory/
├── cmd/
│   ├── main/fm.go              # CLI 入口（含 -engine 参数）
│   ├── app/fm_http.go          # HTTP API 服务器
│   └── cli/                    # Cobra 子命令
├── internal/
│   ├── parser/                 # 多语言代码解析
│   ├── analyzer/               # LLM 智能分析
│   ├── graph/                  # 知识图谱构建
│   ├── index/                  # SQLite + 向量索引（Zvec/FAISS）
│   ├── search/                 # 搜索引擎
│   ├── embedding/              # 向量嵌入生成
│   ├── ranking/                # 函数重要性评分
│   └── module_analyzer/        # 异步模块分析
├── pip-package/flashmemory/    # Python SDK
│   ├── zvec_engine.py          # Zvec Collection 管理
│   ├── zvec_bridge.py          # subprocess 通信协议（15 actions）
│   ├── embedding_provider.py   # 多源 Embedding 抽象
│   ├── search_pipeline.py      # 两阶段检索管线
│   └── client.py               # FlashMemoryClient + MCP 工具
├── config/                     # 配置管理
├── docs/                       # 文档目录
│   ├── zvec_integration_guide_cn.md   # Zvec 集成指南（中文）
│   ├── zvec_integration_guide.md      # Zvec Integration Guide (EN)
│   └── http_api_deep_analysis.md      # HTTP API 深度分析
└── fm.yaml                     # 项目配置文件
```

---

## 文档索引

| 文档 | 说明 |
|------|------|
| [Zvec 集成指南（中文）](docs/zvec_integration_guide_cn.md) | Zvec 引擎、混合搜索、Embedding、SDK、MCP 完整中文指南 |
| [Zvec Integration Guide (EN)](docs/zvec_integration_guide.md) | English version of the Zvec integration guide |
| [HTTP API 深度分析](docs/http_api_deep_analysis.md) | 完整 HTTP API 参考及调用链分析 |
| [发布指南](docs/release_guide.md) | 构建和发布说明 |

---

## 开源协议

MIT License — 详见 [LICENSE](LICENSE)。
