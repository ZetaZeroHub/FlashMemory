# FlashMemory Zvec 集成指南（中文版）

> **版本**: 0.4.5
> **日期**: 2026-05-05
> **状态**: 生产就绪（Phase 1–4 全部完成 + 桥进程可靠性硬化）

---

## 概述

FlashMemory v0.4.0 引入了 **Zvec**——一款高性能进程内向量数据库——作为推荐的向量引擎，替代原有的外部 FAISS HTTP 服务。本指南覆盖所有新增能力：

| 阶段 | 功能 | 描述 |
|------|------|------|
| Phase 1 | 引擎替换 | FAISS → Zvec，subprocess 通信桥，双引擎支持 |
| Phase 2 | 混合搜索 | Dense + Sparse 向量，RRF 融合排序，标量过滤 |
| Phase 3 | AI 扩展 | 多源 Embedding，两阶段检索管线 |
| Phase 4 | Python SDK + MCP | `FlashMemoryClient`，MCP 工具定义 |

---

## 整体架构

```
┌────────────────────────────────────┐
│   FlashMemoryClient  (Python SDK)  │   ← Phase 4
├────────────────────────────────────┤
│   SearchPipeline（检索管线）        │   ← Phase 3
│   阶段一：Recall（Dense+Sparse）    │
│   阶段二：Rerank（交叉编码器重排）   │
├────────────────────────────────────┤
│   EmbeddingProvider（嵌入提供器）   │   ← Phase 3
│   Dense: local/OpenAI/Qwen/Jina   │
│   Sparse: BM25/SPLADE             │
├────────────────────────────────────┤
│   ZvecEngine（Collection 管理）    │   ← Phase 1+2
│   func_collection（Dense+Sparse）  │
│   module_collection（Dense）       │
├────────────────────────────────────┤
│   ZvecBridge（JSON-line 协议）     │   ← Phase 1
│   Go ←→ Python subprocess 通信     │
├────────────────────────────────────┤
│   Zvec（进程内向量数据库）           │
│   HNSW 索引 / Sparse 索引          │
│   标量过滤 / RRF 重排器             │
└────────────────────────────────────┘
```

---

## 1. 快速开始

### 1.1 安装

```bash
# 基础安装（使用 fallback embedding）
pip install flashmemory

# 含 Zvec + 本地模型（推荐）
pip install flashmemory[embedding]

# 全功能：Zvec + 本地 + 云端 Embedding
pip install flashmemory[full]
```

### 1.2 Go CLI

```bash
# 使用 Zvec 引擎（无需外部 FAISS 服务）
go run cmd/main/fm.go -dir /path/to/project -engine zvec -query "文件上传"

# 兼容 FAISS 引擎（默认，向后兼容）
go run cmd/main/fm.go -dir /path/to/project -query "文件上传"
```

### 1.3 Python SDK 快速使用

```python
from flashmemory import FlashMemoryClient

with FlashMemoryClient(project_dir="/path/to/project") as client:
    results = client.search("文件上传处理器", top_k=10)
    for r in results:
        print(f"{r['fields'].get('func_name', r['id'])} → 相关度: {r['score']:.3f}")
```

---

## 2. 引擎配置

### 2.1 CLI 参数

```bash
# 使用 Zvec 引擎
fm -dir /project -engine zvec

# 使用传统 FAISS（默认）
fm -dir /project -engine faiss
```

### 2.2 fm.yaml 配置

```yaml
zvec_config:
  collection_path: ".gitgo/zvec_collections"  # 向量集合存储路径
  dimension: 384                               # 向量维度（all-MiniLM-L6-v2 默认 384）
  metric_type: "cosine"                        # 相似度计算方式
```

### 2.3 引擎对比

| 功能特性 | FAISS（传统） | Zvec（推荐） |
|----------|-------------|------------|
| 架构方式 | 外部 HTTP 服务 | 进程内（subprocess） |
| 安装复杂度 | Python env + faiss_server.py | `pip install zvec` |
| Dense 搜索 | ✅（128 维） | ✅（384 维，可配置） |
| Sparse 搜索 | ❌ | ✅（BM25/SPLADE） |
| 混合搜索 | 手动融合 | 原生 RRF |
| 标量过滤 | SQLite LIKE | 原生过滤表达式 |
| 重排器 | ❌ | ✅（交叉编码器） |
| 持久化格式 | .faiss / .local 文件 | Collection 目录 |
| 启动延迟 | 3–5 秒（Python 进程） | <100ms（bridge 初始化） |

---

## 3. 混合搜索（Phase 2）

### 3.1 原理说明

Zvec 支持多向量联合查询：**Dense 向量**（语义级别）+ **Sparse 向量**（关键词级别），通过 **RRF（Reciprocal Rank Fusion，倒数排名融合）** 合并结果。

```
┌──────────────┐    ┌──────────────┐
│  Dense 向量   │    │  Sparse 向量  │
│（384维 FP32） │    │（BM25/SPLADE）│
└──────┬───────┘    └──────┬───────┘
       │                    │
       └────────┬───────────┘
                │
         ┌──────▼──────┐
         │  RRF 融合     │
         │（k=60，默认） │
         └──────┬──────┘
                │
         ┌──────▼──────┐
         │  Top-K 结果  │
         └─────────────┘
```

### 3.2 Python API 示例

```python
from flashmemory.zvec_engine import ZvecEngine

engine = ZvecEngine("/path/to/.gitgo/zvec_collections", dimension=384)
engine.init_func_collection()

# 写入时同时存储 Dense + Sparse 向量
engine.upsert_function(
    "func_42",
    dense_vector=[0.1] * 384,
    fields={"func_name": "Upload", "language": "go"},
    sparse_embedding={"upload": 0.8, "file": 0.6, "handler": 0.4},
)

# 执行混合搜索
results = engine.hybrid_search_functions(
    dense_vector=[0.1] * 384,
    sparse_vector={"upload": 0.9},
    top_k=10,
    filter_expr='language = "go"',   # 标量过滤表达式
    use_rrf=True,                    # 启用 RRF 融合
)
```

### 3.3 Go API（通过 Bridge）

```go
// HybridSearchVectors: Dense + Sparse 混合搜索
ids, err := wrapper.HybridSearchVectors(denseVec, sparseVec, topK, useRRF, filterExpr)

// SearchVectorsWithFilter: 带标量过滤的 Dense 搜索
ids, err := wrapper.SearchVectorsWithFilter(queryVec, topK, filterExpr)

// 共享的结果解析函数
ids = wrapper.parseSearchResults(resp)
```

### 3.4 Schema 定义

函数 Collection 的完整 Schema 如下：

| 字段名 | 类型 | 索引类型 | 用途 |
|--------|------|----------|------|
| `embedding` | VECTOR_FP32（384维） | HNSW（cosine） | Dense 语义搜索 |
| `sparse_embedding` | SPARSE_VECTOR_FP32 | SparseIndex | 关键词级匹配 |
| `func_name` | STRING | InvertIndex | 标量过滤 |
| `package` | STRING | InvertIndex | 按包名过滤 |
| `file_path` | STRING | InvertIndex | 按文件路径过滤 |
| `language` | STRING | InvertIndex | 按编程语言过滤 |
| `func_type` | STRING | InvertIndex | 按函数类型过滤 |
| `description` | STRING | — | 描述元数据 |

### 3.5 生产级健壮性与优化点

近期的核心代码已针对 Zvec 的混合搜索体验进行了全面优化排雷：
1. **精确的 Token 分块安全机制**：全面修复了云端 API 遭遇大段代码/描述文本时引起的 HTTP 413 超限错误，采取了目前最保守（1:1 rune 计数）的字符集分块估算逻辑，确保万段代码无痛注入。
2. **Metadata 元数据完整注入**：Go 在向 Python Zvec 发送底层新增向量时，补全了底层所必须的函数的名称、代码描述、路径参数，使得 BM25 分词器能够以原始文本形式构建高精度的反向稀疏索引。
3. **Bridge 通信读写超时抗性**：解决了首次拉起 Zvec 搜索时由于后台强行分配与编译动态字典（Jieba Cache / 等级预热）需要超长 15~20 秒的卡顿问题，将 Go 发送侧的守护进程超时等待安全提升至 600 秒级，彻底杜绝单边异常中断。
4. **单次搜索 `-query_only` 极速起走**：去除了旧版命令每次查询时无谓触发了全量“重新构建并灌入API”的逻辑，改为直接加载 Zvec 原生数据库动态识别 384/128 等维护维度。避免 API 资产浪费并在毫秒间吐出结果。

---

## 4. 嵌入提供器（Phase 3）

### 4.1 支持的提供器

| 提供器 | 类型 | 模型 | 维度 | 备注 |
|--------|------|------|------|------|
| `local` | Dense | all-MiniLM-L6-v2 | 384 | 推荐，无需 API Key |
| `openai` | Dense | text-embedding-3-small | 1536 | 需要 OpenAI API Key |
| `qwen` | Dense | 通义千问 Embedding | 1024 | 需要阿里云 DashScope Key |
| `jina` | Dense | Jina Embeddings | 1024 | 需要 Jina API Key |
| `bm25_zh` | Sparse | BM25（中文） | — | 中文关键词匹配 |
| `bm25_en` | Sparse | BM25（英文） | — | 英文关键词匹配 |
| `splade` | Sparse | 学习型稀疏模型 | — | 精度更高的稀疏向量 |
| Fallback | Dense | 哈希伪随机（仅测试） | 可配置 | 离线/测试环境降级 |

### 4.2 基础用法

```python
from flashmemory.embedding_provider import EmbeddingProvider

# 本地 Embedding（默认，无需 API Key）
provider = EmbeddingProvider(config={
    "dense_provider": "local",
    "sparse_provider": "bm25_zh",  # 中文关键词 Embedding
})

# 生成向量
dense = provider.embed_dense("搜索文件上传处理器")     # → List[float]
sparse = provider.embed_sparse("搜索文件上传处理器")   # → Dict[str, float]
batch = provider.embed_dense_batch(["查询1", "查询2"]) # → List[List[float]]

# 查看提供器信息
print(provider.get_info())
# {
#   "dense_provider": "local",
#   "sparse_provider": "bm25_zh",
#   "dimension": 384,
#   "has_sparse": true,
#   "dense_type": "_SentenceTransformerWrapper"
# }
```

### 4.3 云端 Embedding 配置

```python
# OpenAI Embedding
provider = EmbeddingProvider(config={
    "dense_provider": "openai",
    "api_key": "sk-...",
    "dimension": 1536,
    "model_name": "text-embedding-3-small",
})

# 通义千问 Embedding（阿里云 DashScope）
provider = EmbeddingProvider(config={
    "dense_provider": "qwen",
    "api_key": "sk-...",
    "dimension": 1024,
})
```

> **注意**：云端 Embedding 需要 `pip install flashmemory[cloud]` 并确保网络可访问。

---

## 5. 检索管线（Phase 3）

### 5.1 两阶段检索架构

```
查询文本："文件上传处理器"
         │
    ┌────▼────┐
    │  向量化  │  EmbeddingProvider.embed_dense() + embed_sparse()
    └────┬────┘
         │
    ┌────▼──────────────────┐
    │  阶段一：召回（Recall） │  ZvecEngine.hybrid_search_functions()
    │  top_k × recall_mult  │  Dense + Sparse + RRF + 标量过滤
    └────┬──────────────────┘
         │ top_k × N 个候选
    ┌────▼──────────────────┐
    │  阶段二：精排（Rerank） │  交叉编码器重排（可选）
    │  最终 top_k            │  DefaultLocalReRanker
    └────┬──────────────────┘
         │
    ┌────▼────┐
    │  搜索结果 │  List[SearchResult]
    └─────────┘
```

召回阶段放大：`recall_count = top_k × recall_multiplier（默认 5）`  
若未启用重排器，则直接返回 `top_k` 结果。

### 5.2 用法示例

```python
from flashmemory.zvec_engine import ZvecEngine
from flashmemory.embedding_provider import EmbeddingProvider
from flashmemory.search_pipeline import SearchPipeline

engine = ZvecEngine("/path/to/collections")
engine.init_func_collection()

provider = EmbeddingProvider({"dense_provider": "local"})

pipeline = SearchPipeline(engine, provider, config={
    "enable_reranker": False,  # 是否启用精排
    "recall_multiplier": 5,    # 召回阶段放大倍数
    "use_rrf": True,           # 启用 RRF 融合
})

# 基本搜索
results = pipeline.search("文件上传", top_k=10)
print(results[0])  # SearchResult(id=func_42, score=0.9531, name=UploadFile)

# 带上下文过滤的搜索
results = pipeline.search_with_context(
    "上传处理器",
    top_k=10,
    language="go",       # 仅搜索 Go 代码
    package="handlers",  # 仅搜索指定包
)
```

---

## 6. Python SDK（Phase 4）

### 6.1 FlashMemoryClient 完整示例

`FlashMemoryClient` 是最高级别的 API，封装了 Engine + Embedding + Pipeline：

```python
from flashmemory import FlashMemoryClient

# 初始化客户端
client = FlashMemoryClient(
    project_dir="/path/to/project",
    engine_type="zvec",
    dimension=384,
    dense_provider="local",
    sparse_provider="none",  # 可选 "bm25_zh" / "bm25_en" / "splade"
    enable_reranker=False,
)
client.initialize()

# ===== 搜索 =====
# 基础语义搜索
results = client.search("文件上传处理", top_k=10)

# 按语言过滤
results = client.search_functions("认证中间件", language="go")

# 搜索模块（目录级别）
results = client.search_modules("搜索引擎模块")

# 结果格式
for r in results:
    print(f"ID: {r['id']}")
    print(f"相关度: {r['score']:.3f}")
    print(f"函数名: {r['fields'].get('func_name')}")
    print(f"文件: {r['fields'].get('file_path')}")

# ===== 向量生成 =====
# 单文本向量
result = client.embed("上传文件到服务器")
# → {"dense": [0.1, ...], "sparse": None, "dimension": 384}

# 批量向量
result = client.embed_batch(["查询1", "查询2", "查询3"])
# → {"dense_batch": [[...], [...], [...]], "count": 3}

# ===== 索引管理 =====
# 添加单个函数到索引
client.add_function(
    func_id="func_42",
    text="处理文件上传并保存到磁盘",
    metadata={
        "func_name": "UploadFile",
        "package": "handlers",
        "file_path": "internal/handlers/upload.go",
        "language": "go",
    },
)

# 批量添加
client.add_functions_batch([
    {"func_id": "func_1", "text": "...", "metadata": {"func_name": "A"}},
    {"func_id": "func_2", "text": "...", "metadata": {"func_name": "B"}},
])

# 删除指定文件的所有向量（增量更新时使用）
client.delete_by_file("internal/handlers/upload.go")

# 优化索引性能
client.optimize()

# 查看诊断信息
info = client.get_info()

# 手动关闭
client.close()
```

### 6.2 Context Manager（推荐用法）

```python
# 使用 with 语句自动管理生命周期
with FlashMemoryClient(project_dir="/path/to/project") as client:
    results = client.search("认证中间件")
    # 退出时自动关闭所有资源
```

### 6.3 初始化参数说明

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `project_dir` | str | — | 项目根目录（必填） |
| `engine_type` | str | `"zvec"` | 向量引擎类型 |
| `dimension` | int | `384` | 向量维度 |
| `dense_provider` | str | `"local"` | Dense Embedding 来源 |
| `sparse_provider` | str | `"none"` | Sparse Embedding 来源 |
| `enable_reranker` | bool | `False` | 是否启用精排器 |
| `collection_subdir` | str | `".gitgo/zvec_collections"` | Collection 相对路径 |
| `api_key` | str | — | 云端 Embedding API Key |
| `model_name` | str | — | 覆盖默认模型 |

---

## 7. MCP 集成（Phase 4）

### 7.1 概述

FlashMemory 提供三个符合 MCP（Model Context Protocol）规范的工具，可直接集成到任何 MCP Server，供 AI Agent 调用。

```python
from flashmemory import get_mcp_tools

tools = get_mcp_tools()
# 返回三个工具定义：
# - flashmemory_search:  自然语言代码搜索
# - flashmemory_index:   添加函数到搜索索引
# - flashmemory_info:    获取引擎状态和诊断信息
```

### 7.2 集成到 MCP Server

```python
from flashmemory import get_mcp_tools, handle_mcp_tool_call

# 注册工具到 MCP Server
tools = get_mcp_tools()

# 处理 AI Agent 的工具调用（内置客户端缓存）
client_cache = {}  # 跨请求复用 FlashMemoryClient

result = handle_mcp_tool_call(
    tool_name="flashmemory_search",
    arguments={
        "project_dir": "/path/to/project",
        "query": "数据库连接池管理",
        "top_k": 10,
        "language": "go",
    },
    client_cache=client_cache,
)
# → {"results": [...], "count": 5}
```

### 7.3 工具参数详细说明

#### `flashmemory_search`（代码搜索）

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `query` | string | ✅ | 自然语言搜索查询 |
| `project_dir` | string | ✅ | 项目根目录绝对路径 |
| `top_k` | integer | ❌ | 返回结果数量（默认 10） |
| `language` | string | ❌ | 按编程语言过滤 |
| `search_type` | string | ❌ | `"functions"` 或 `"modules"` |

#### `flashmemory_index`（函数索引）

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `project_dir` | string | ✅ | 项目根目录绝对路径 |
| `func_id` | string | ✅ | 函数唯一标识符 |
| `text` | string | ✅ | 要向量化并索引的文本 |
| `metadata` | object | ❌ | 标量字段（func_name, package 等） |

#### `flashmemory_info`（诊断信息）

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `project_dir` | string | ✅ | 项目根目录绝对路径 |

---

## 8. Bridge 通信协议参考

Go 和 Python 之间通过 subprocess 的 stdin/stdout 进行 JSON-line 格式通信。

### 8.1 请求格式

```json
{"action": "search", "params": {"query": [0.1, 0.2, ...], "top_k": 10}}
```

### 8.2 响应格式

```json
{"status": "success", "data": {"results": [...]}, "message": "ok"}
```

错误响应：
```json
{"status": "error", "data": {}, "message": "错误描述"}
```

### 8.3 全部 Action 列表（共 15 个）

| Action | Phase | 说明 |
|--------|-------|------|
| `init` | 1 | 初始化 Zvec Collection |
| `add_vector` | 1 | 添加单个函数向量 |
| `add_vectors_batch` | 1 | 批量添加函数向量 |
| `add_module_vector` | 1 | 添加模块向量 |
| `search` | 1 | Dense 向量搜索 |
| `delete` | 1 | 按过滤条件删除向量 |
| `optimize` | 1 | 优化索引 |
| `stats` | 1 | 获取 Collection 统计信息 |
| `close` | 1 | 关闭 Collection |
| `ping` | 1 | 心跳检测 |
| `shutdown` | 1 | 终止 Bridge 进程 |
| `hybrid_search` | 2 | Dense + Sparse 混合搜索 |
| `init_embedding` | 3 | 初始化嵌入提供器 |
| `embed` | 3 | 生成嵌入向量 |
| `pipeline_search` | 3 | 执行全链路检索管线 |

---

## 9. 文件布局

Zvec 集成后，项目文件结构如下：

```
flashmemory/
├── cmd/main/fm.go                          # CLI 入口，含 -engine 参数
├── config/config.go                        # ZvecConfig 结构体
├── fm.yaml                                 # zvec_config 配置节
├── internal/index/
│   ├── index.go                            # NewFaissWrapperByEngine() 工厂方法
│   ├── zvec_wrapper.go                     # ZvecWrapper（实现 FaissWrapper 接口）
│   └── zvec_wrapper_test.go                # Go 单元测试
├── pip-package/flashmemory/
│   ├── __init__.py                         # v0.4.0，lazy import + __all__
│   ├── zvec_engine.py                      # ZvecEngine（Collection 管理）
│   ├── zvec_bridge.py                      # Bridge 进程（15 个 action）
│   ├── embedding_provider.py               # 多源 Embedding 抽象
│   ├── search_pipeline.py                  # 两阶段检索管线
│   ├── client.py                           # FlashMemoryClient + MCP 工具
│   └── cli.py                              # CLI 入口点
├── pip-package/tests/
│   ├── test_zvec_engine.py                 # 23 个测试
│   ├── test_zvec_bridge.py                 # 19 个测试
│   ├── test_phase2_hybrid.py               # 15 个测试
│   ├── test_phase3_ai.py                   # 31 个测试
│   └── test_phase4_sdk.py                  # 22 个测试
└── pip-package/pyproject.toml              # v0.4.0
```

### 项目数据目录（运行时生成）

```
project/
└── .gitgo/
    ├── code_index.db                       # SQLite（functions / calls 等表）
    ├── zvec_collections/                   # Zvec 向量集合（新增）
    │   ├── functions/                      # 函数级别向量（Dense + Sparse）
    │   └── modules/                        # 模块级别向量（Dense）
    ├── code_index.local                    # FAISS 索引（兼容保留）
    ├── graph.json                          # 知识图谱
    └── module_graphs/                      # 模块可视化图谱
```

---

## 10. 测试

### 运行所有 Python 测试

```bash
cd pip-package
python -m pytest tests/ -v
# 110 个测试，约 0.13 秒全部通过
```

### 运行所有 Go 测试

```bash
go test ./internal/index/ -v
# 27+ 个测试全部通过
```

### 各阶段测试覆盖情况

| 阶段 | 测试数 | 覆盖范围 |
|------|--------|---------|
| Phase 1（引擎替换） | 42 Python + 27 Go | 引擎初始化、CRUD、搜索、Bridge 协议 |
| Phase 2（混合搜索） | 15 Python + 8 Go | RRF、Sparse upsert、Schema、过滤 |
| Phase 3（AI 扩展） | 31 Python | Embedding 提供器、管线、Bridge actions |
| Phase 4（SDK） | 22 Python | Client 生命周期、MCP 工具、处理器 |
| **合计** | **137+** | **全部通过，零回归** |

---

## 11. 可靠性与桥进程生命周期

> **新增于 v0.4.5（2026-05-04 → 05-05）。** 这一节专门面向运维 / 二开者，
> 解释 Zvec 桥进程为何容易卡死、当前架构是怎么把它兜住的，以及在出现疑似
> RocksDB 损坏时该期望看到什么日志。

### 11.1 桥进程的本质风险

Zvec collection 由桥进程通过 RocksDB 持有 fcntl LOCK。一旦：

- 桥进程**还活着**，但调用方丢失了引用（GC 不杀子进程）→ LOCK 永远占着
- 桥进程**异常退出**前 RocksDB 写到一半 → segment / MANIFEST 残缺
- `kill -9` 父进程或机器掉电 → 桥进程被 reparent 到 init，atexit 不跑

任意一种都会让下次启动的桥 `zvec.open()` 失败，常见错误形态：

| 错误信息 | 含义 |
|----------|------|
| `Can't lock` | LOCK 文件被活进程持有 |
| `Can't open lock file` | LOCK 文件残留但被以为占用 |
| `recovery idmap failed` | RocksDB 重放 idmap WAL 失败 |
| `Segment open failed: segment path not found [.../N]` | segment 索引指向的目录消失 |
| `Corruption: ...` / `checksum mismatch` | SST/MANIFEST 损坏 |

### 11.2 三层防护设计

```
┌──────────────────────────────────────────────────────────────┐
│ Layer 1：Wrapper 生命周期（Go 侧主动释放）                    │
│   FaissManager.allWrappers + defer fm.Free()                  │
│   → 索引完成立刻 SIGTERM 桥，正常路径下零残留                  │
├──────────────────────────────────────────────────────────────┤
│ Layer 2：进程退出兜底（信号处理路径）                          │
│   e.Shutdown(ctx,30s) + index.FreeAllActiveWrappers()         │
│   → SIGINT/SIGTERM 时把所有活跃 wrapper 的桥扫一遍             │
├──────────────────────────────────────────────────────────────┤
│ Layer 3：自愈（Python 桥侧打开恢复）                           │
│   _try_open_collection 三层 attempt：open → 清 LOCK → 重建    │
│   → 即便 Layer 1/2 都漏了，下次启动还能从残骸里爬起来          │
└──────────────────────────────────────────────────────────────┘
```

### 11.3 Layer 1 — Wrapper 生命周期

代码位置：`internal/back/faiss.go`、`internal/back/backwork.go`。

- `FaissManager.allWrappers []FaissWrapper` 跟踪本管理器**曾经持有过**的所有 wrapper。`InitFaissManager` 创建第一个，`Reset()` 换新 wrapper 时**追加**到列表而不是丢弃。
- `FaissManager.Free()` 遍历列表逐个调用 `wrapper.Free()`，用 `seen` 去重防双 Free，用 `recover()` 防单个 wrapper panic 拖垮其他释放，幂等。
- `BuildIndex` / `IncrementalUpdate` 在 `InitFaissManager` 之后立即 `defer fm.Free()`，保证函数返回（含错误返回、panic）时桥一定收尾。

**结果**：正常索引完成后 `ps aux | grep zvec_bridge` 应该是空的；任何后续 search/check 创建的新桥都能拿到 LOCK。

### 11.4 Layer 2 — 进程退出兜底

代码位置：`cmd/app/fm_http.go`、`internal/index/zvec_wrapper.go`。

- 包级 `var activeZvecWrappers sync.Map`：每次 `NewZvecWrapper` 成功就 `Store(zw, struct{}{})`；`zw.Free()` 入口 `Delete(zw)`。
- 公开函数 `index.FreeAllActiveWrappers() int`：Range 整张 sync.Map 逐个 Free，每个 Free 用 `recover()` 隔离，返回实际释放数量供日志审计。
- `fm_http` 启动流程：

```go
shutdownDone := make(chan struct{})
go func() {
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    <-sigCh
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    e.Shutdown(ctx)                       // 等所有 in-flight handler 跑 defer
    index.FreeAllActiveWrappers()         // 兜底，扫掉后台 goroutine / panic 漏掉的
    close(shutdownDone)
}()
e.Start(address)                          // 阻塞，被 Shutdown 触发后返回
<-shutdownDone
```

**结果**：`kill -INT <fm_http_pid>` 不再触发 `os.Exit(0)` 硬切；in-flight 的 `BuildIndex` 能跑完自己的 defer；后台 `module_analyzer` goroutine 持有的 wrapper 也会被强清。

### 11.5 Layer 3 — `_try_open_collection` 三层自愈

代码位置：`pip-package/flashmemory/zvec_engine.py`。

```python
attempt 1: zvec.open(path)                          # 直接打开
attempt 2: 递归删 path 下所有 LOCK + sleep(0.1) + open
attempt 3: shutil.rmtree(path) + force_new=True     # 销毁重建（破坏性）
```

attempt 3 仅在错误信息**命中** `_RECOVERABLE_ERROR_FRAGMENTS` 时触发：

```python
_RECOVERABLE_ERROR_FRAGMENTS = (
    "lock", "recovery", "corrupt", "no such file",
    "checksum", "manifest", "idmap", "segment",
)
```

正是 §11.1 列出的全部典型不可恢复状态。命中即触发；非该集合内的错误（例如配置错、维度不匹配）直接 raise，不会误删数据。

attempt 3 触发时日志会打印：

```
[ERROR] Open still failing after LOCK purge (attempt 2): <原始错误>.
        Treating collection at <path> as corrupt and rebuilding from scratch.
        EXISTING DATA IN THIS COLLECTION WILL BE LOST.
```

这是审计 trail，便于事后追查"为什么 collection 被重置了"。

### 11.6 故障验证清单（运维参考）

| 场景 | 期望日志 | 期望桥进程数 |
|------|---------|-------------|
| 正常索引完成 | `Zvec Bridge 进程已正常退出` | 0 |
| 多次连续上传 | 每次结束后桥退出，下一次启动不报 LOCK | 0 |
| `kill -INT fm_http`（空闲） | `All in-flight handlers drained` + `FM HTTP server stopped cleanly` | 0 |
| `kill -INT fm_http`（in-flight 索引中） | 等索引 defer 跑完再退；可能 `Forcefully freed N residual Zvec wrapper(s)` | 0 |
| `kill -9 fm_http` 后重启 | 下次 `zvec.open` 触发 attempt 2 / attempt 3 自愈 | 视情况 |
| 人工 `rm 000008.ldb`（模拟磁盘损坏） | `Treating collection ... as corrupt and rebuilding` | 视情况 |

如果在生产看到 attempt 3 的"DATA WILL BE LOST"日志，**不是 bug**，是兜底兜住了一次。但同时应该排查上游：为什么会留下损坏状态？最常见的源头是 `os.Exit` / `kill -9` / OOM kill / 容器突然销毁——只要 Layer 1+2 走通，Layer 3 在生产应当极少触发。

### 11.7 已知限制

- `FaissManager.Reset()` 在 zvec 模式下用 `NewFaissWrapper`（HTTP）替换，会让 full re-index 之后的操作走到没启动的 FAISS 服务上失败。这是 zvec 模式下的预存设计缺陷，与 LOCK / 生命周期议题正交，留作后续单独处理。
- `cmd/main/fm.go`（CLI 二进制）目前没有装 `FreeAllActiveWrappers` 的信号兜底——CLI 单次执行结束即退，BuildIndex 内部的 `defer fm.Free()` 已经够用；但若用户在 CLI 进程长时间运行中 `Ctrl+C`，后台 `module_analyzer` goroutine 持有的 wrapper 仍可能漏。如对 CLI 也有要求，可参考 fm_http.go 的信号 handler 加到 `cmd/main/fm.go`。
