# FlashMemory Zvec Integration Guide

> **Version**: 0.4.0  
> **Date**: 2026-04-11  
> **Status**: Production Ready (Phase 1–4 Complete)

---

## Overview

FlashMemory v0.4.0 introduces **Zvec** — a high-performance, in-process vector database — as the recommended vector engine, replacing the external FAISS HTTP service. This guide covers all new capabilities:

| Phase | Feature | Description |
|-------|---------|-------------|
| 1 | Engine Replacement | FAISS → Zvec, subprocess bridge, dual-engine support |
| 2 | Hybrid Search | Dense + Sparse vectors, RRF fusion, scalar filtering |
| 3 | AI Extensions | Multi-source embedding, two-stage retrieval pipeline |
| 4 | Python SDK + MCP | `FlashMemoryClient`, MCP tool definitions |

---

## Architecture

```
┌────────────────────────────────────┐
│   FlashMemoryClient  (Python SDK)  │   ← Phase 4
├────────────────────────────────────┤
│   SearchPipeline                   │   ← Phase 3
│   Stage 1: Recall (Dense+Sparse)   │
│   Stage 2: Rerank (Cross-Encoder)  │
├────────────────────────────────────┤
│   EmbeddingProvider                │   ← Phase 3
│   Dense: local/OpenAI/Qwen/Jina   │
│   Sparse: BM25/SPLADE             │
├────────────────────────────────────┤
│   ZvecEngine  (Collection CRUD)    │   ← Phase 1+2
│   func_collection (Dense+Sparse)   │
│   module_collection (Dense)        │
├────────────────────────────────────┤
│   ZvecBridge  (JSON-line Protocol) │   ← Phase 1
│   Go ←→ Python subprocess          │
├────────────────────────────────────┤
│   Zvec (In-process Vector DB)      │
│   HNSW index / Sparse index        │
│   Scalar filtering / RRF reranker  │
└────────────────────────────────────┘
```

---

## 1. Quick Start

### 1.1 Installation

```bash
# Basic install (fallback embedding)
pip install flashmemory

# With Zvec + local embedding (recommended)
pip install flashmemory[embedding]

# Full: Zvec + local + cloud providers
pip install flashmemory[full]
```

### 1.2 Go CLI

```bash
# Zvec engine (no external FAISS service needed)
go run cmd/main/fm.go -dir /path/to/project -engine zvec -query "file upload"

# Legacy FAISS engine (default, backward compatible)
go run cmd/main/fm.go -dir /path/to/project -query "file upload"
```

### 1.3 Python SDK

```python
from flashmemory import FlashMemoryClient

with FlashMemoryClient(project_dir="/path/to/project") as client:
    results = client.search("file upload handler", top_k=10)
    for r in results:
        print(f"{r['fields'].get('func_name', r['id'])} → score: {r['score']:.3f}")
```

---

## 2. Engine Configuration

### 2.1 CLI Flag

```bash
# Use Zvec engine
fm -dir /project -engine zvec

# Use legacy FAISS (default)
fm -dir /project -engine faiss
```

### 2.2 fm.yaml Configuration

```yaml
zvec_config:
  collection_path: ".gitgo/zvec_collections"
  dimension: 384
  metric_type: "cosine"
```

### 2.3 Engine Comparison

| Feature | FAISS (legacy) | Zvec (recommended) |
|---------|---------------|-------------------|
| Architecture | External HTTP service | In-process (subprocess) |
| Setup complexity | Python env + faiss_server.py | `pip install zvec` |
| Dense search | ✅ (128-dim) | ✅ (384-dim, configurable) |
| Sparse search | ❌ | ✅ (BM25/SPLADE) |
| Hybrid search | Manual fusion | Native RRF |
| Scalar filtering | SQLite LIKE | Native filter expressions |
| Reranking | ❌ | ✅ (Cross-encoder) |
| Persistence | .faiss / .local files | Collection directory |
| Startup time | 3–5s (Python process) | <100ms (bridge init) |

---

## 3. Hybrid Search (Phase 2)

### 3.1 Concept

Zvec supports multi-vector queries: **Dense** (semantic) + **Sparse** (keyword-level) vectors, fused with **RRF (Reciprocal Rank Fusion)**.

```
┌──────────────┐    ┌──────────────┐
│ Dense Vector  │    │ Sparse Vector │
│ (384-dim FP32)│    │ (BM25/SPLADE) │
└──────┬───────┘    └──────┬───────┘
       │                    │
       └────────┬───────────┘
                │
         ┌──────▼──────┐
         │  RRF Fusion  │
         │ (k=60 default)│
         └──────┬──────┘
                │
         ┌──────▼──────┐
         │  Top-K Results│
         └─────────────┘
```

### 3.2 Python API

```python
from flashmemory.zvec_engine import ZvecEngine

engine = ZvecEngine("/path/to/.gitgo/zvec_collections", dimension=384)
engine.init_func_collection()

# Upsert with sparse embedding
engine.upsert_function(
    "func_42",
    dense_vector=[0.1] * 384,
    fields={"func_name": "Upload", "language": "go"},
    sparse_embedding={"upload": 0.8, "file": 0.6, "handler": 0.4},
)

# Hybrid search
results = engine.hybrid_search_functions(
    dense_vector=[0.1] * 384,
    sparse_vector={"upload": 0.9},
    top_k=10,
    filter_expr='language = "go"',
    use_rrf=True,
)
```

### 3.3 Go API (via Bridge)

```go
// HybridSearchVectors sends a hybrid_search request to the bridge
ids, err := wrapper.HybridSearchVectors(denseVec, sparseVec, topK, useRRF, filterExpr)

// SearchVectorsWithFilter adds scalar filtering
ids, err := wrapper.SearchVectorsWithFilter(queryVec, topK, filterExpr)
```

### 3.4 Schema Definition

The function collection schema includes:

| Column | Type | Index | Purpose |
|--------|------|-------|---------|
| `embedding` | VECTOR_FP32 (384-dim) | HNSW (cosine) | Dense semantic search |
| `sparse_embedding` | SPARSE_VECTOR_FP32 | SparseIndex | Keyword-level matching |
| `func_name` | STRING | InvertIndex | Scalar filtering |
| `package` | STRING | InvertIndex | Scalar filtering |
| `file_path` | STRING | InvertIndex | Scalar filtering |
| `language` | STRING | InvertIndex | Scalar filtering |
| `func_type` | STRING | InvertIndex | Scalar filtering |
| `description` | STRING | — | Metadata |

---

## 4. Embedding Provider (Phase 3)

### 4.1 Supported Providers

| Provider | Type | Model | Dimension |
|----------|------|-------|-----------|
| `local` | Dense | all-MiniLM-L6-v2 | 384 |
| `openai` | Dense | text-embedding-3-small | 1536 |
| `qwen` | Dense | DashScope Embedding | 1024 |
| `jina` | Dense | Jina Embeddings | 1024 |
| `bm25_zh` | Sparse | BM25 (Chinese) | — |
| `bm25_en` | Sparse | BM25 (English) | — |
| `splade` | Sparse | Learned Sparse | — |
| Fallback | Dense | Hash-based (testing) | configurable |

### 4.2 Usage

```python
from flashmemory.embedding_provider import EmbeddingProvider

# Local embedding (default, no API key needed)
provider = EmbeddingProvider(config={
    "dense_provider": "local",
    "sparse_provider": "bm25_zh",
})

# Generate embeddings
dense = provider.embed_dense("search file upload handler")  # → List[float]
sparse = provider.embed_sparse("search file upload handler")  # → Dict[str, float]
batch = provider.embed_dense_batch(["query1", "query2"])  # → List[List[float]]

# Provider info
print(provider.get_info())
```

### 4.3 Cloud Providers

```python
# OpenAI
provider = EmbeddingProvider(config={
    "dense_provider": "openai",
    "api_key": "sk-...",
    "dimension": 1536,
})

# Qwen (Alibaba DashScope)
provider = EmbeddingProvider(config={
    "dense_provider": "qwen",
    "api_key": "sk-...",
    "dimension": 1024,
})
```

---

## 5. Search Pipeline (Phase 3)

### 5.1 Two-Stage Architecture

```
Query: "file upload handler"
         │
    ┌────▼────┐
    │ Embed   │  EmbeddingProvider.embed_dense() + embed_sparse()
    └────┬────┘
         │
    ┌────▼─────────────┐
    │ Stage 1: Recall   │  ZvecEngine.hybrid_search_functions()
    │ top_k × multiplier│  Dense + Sparse + RRF + scalar filter
    └────┬─────────────┘
         │ candidates
    ┌────▼─────────────┐
    │ Stage 2: Rerank   │  Cross-encoder reranking (optional)
    │ top_k final       │  DefaultLocalReRanker
    └────┬─────────────┘
         │
    ┌────▼────┐
    │ Results │  List[SearchResult]
    └─────────┘
```

### 5.2 Usage

```python
from flashmemory.zvec_engine import ZvecEngine
from flashmemory.embedding_provider import EmbeddingProvider
from flashmemory.search_pipeline import SearchPipeline

engine = ZvecEngine("/path/to/collections")
engine.init_func_collection()

provider = EmbeddingProvider({"dense_provider": "local"})

pipeline = SearchPipeline(engine, provider, config={
    "enable_reranker": False,
    "recall_multiplier": 5,
    "use_rrf": True,
})

# Basic search
results = pipeline.search("file upload", top_k=10)

# Filtered search
results = pipeline.search_with_context(
    "upload handler",
    top_k=10,
    language="go",
    package="main",
)
```

---

## 6. Python SDK (Phase 4)

### 6.1 FlashMemoryClient

The highest-level API, aggregating Engine + Embedding + Pipeline:

```python
from flashmemory import FlashMemoryClient

# One-line initialization
client = FlashMemoryClient(
    project_dir="/path/to/project",
    engine_type="zvec",
    dimension=384,
    dense_provider="local",
    sparse_provider="none",
)
client.initialize()

# Search
results = client.search("file upload handler", top_k=10)
results = client.search_functions("auth", language="go")
results = client.search_modules("search engine")

# Embed
embedding = client.embed("upload file to server")
batch = client.embed_batch(["query1", "query2", "query3"])

# Index management
client.add_function("func_1", "Handle file upload and save to disk", {
    "func_name": "UploadFile",
    "package": "handlers",
    "language": "go",
})
client.optimize()

# Diagnostics
info = client.get_info()

# Cleanup
client.close()
```

### 6.2 Context Manager

```python
with FlashMemoryClient(project_dir="/path/to/project") as client:
    results = client.search("authentication middleware")
    # Auto-closes on exit
```

---

## 7. MCP Integration (Phase 4)

### 7.1 Tool Definitions

FlashMemory exposes three MCP-compatible tools for AI agent integration:

```python
from flashmemory import get_mcp_tools

tools = get_mcp_tools()
# Returns:
# - flashmemory_search: Natural language code search
# - flashmemory_index: Add functions to search index
# - flashmemory_info: Get engine status and diagnostics
```

### 7.2 MCP Server Integration

```python
from flashmemory import get_mcp_tools, handle_mcp_tool_call

# Register tools with your MCP server
tools = get_mcp_tools()

# Handle tool calls (with client caching)
client_cache = {}
result = handle_mcp_tool_call(
    tool_name="flashmemory_search",
    arguments={
        "project_dir": "/path/to/project",
        "query": "file upload handler",
        "top_k": 10,
        "language": "go",
    },
    client_cache=client_cache,
)
```

### 7.3 Tool Schemas

#### `flashmemory_search`

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | ✅ | Natural language search query |
| `project_dir` | string | ✅ | Project root directory |
| `top_k` | integer | ❌ | Number of results (default: 10) |
| `language` | string | ❌ | Filter by language |
| `search_type` | string | ❌ | "functions" or "modules" |

#### `flashmemory_index`

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `project_dir` | string | ✅ | Project root directory |
| `func_id` | string | ✅ | Unique function ID |
| `text` | string | ✅ | Text to embed and index |
| `metadata` | object | ❌ | Scalar fields for filtering |

#### `flashmemory_info`

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `project_dir` | string | ✅ | Project root directory |

---

## 8. Bridge Protocol Reference

The Go ↔ Python communication uses a JSON-line protocol over subprocess stdin/stdout.

### 8.1 Request Format

```json
{"action": "search", "params": {"query": [0.1, 0.2, ...], "top_k": 10}}
```

### 8.2 Response Format

```json
{"status": "success", "data": {"results": [...]}, "message": "ok"}
```

### 8.3 All Actions (18 total)

| Action | Phase | Description |
|--------|-------|-------------|
| `init` | 1 | Initialize Zvec collections |
| `add_vector` | 1 | Add single function vector |
| `add_vectors_batch` | 1 | Batch add function vectors |
| `add_module_vector` | 1 | Add module vector |
| `search` | 1 | Dense vector search |
| `delete` | 1 | Delete vectors by filter |
| `optimize` | 1 | Optimize index |
| `stats` | 1 | Get collection stats |
| `close` | 1 | Close collections |
| `ping` | 1 | Health check |
| `shutdown` | 1 | Terminate bridge process |
| `hybrid_search` | 2 | Dense + Sparse hybrid search |
| `init_embedding` | 3 | Initialize embedding provider |
| `embed` | 3 | Generate embeddings |
| `pipeline_search` | 3 | Full search pipeline |

---

## 9. File Layout

After Zvec integration, the project file layout includes:

```
flashmemory/
├── cmd/main/fm.go                          # CLI with -engine flag
├── config/config.go                        # ZvecConfig struct
├── fm.yaml                                 # zvec_config section
├── internal/index/
│   ├── index.go                            # NewFaissWrapperByEngine()
│   ├── zvec_wrapper.go                     # ZvecWrapper (FaissWrapper interface)
│   └── zvec_wrapper_test.go                # Go unit tests
├── pip-package/flashmemory/
│   ├── __init__.py                         # v0.4.0, lazy imports
│   ├── zvec_engine.py                      # ZvecEngine (Collection CRUD)
│   ├── zvec_bridge.py                      # Bridge process (18 actions)
│   ├── embedding_provider.py              # Multi-source embedding
│   ├── search_pipeline.py                 # Two-stage retrieval
│   ├── client.py                          # FlashMemoryClient + MCP tools
│   └── cli.py                             # CLI entry point
├── pip-package/tests/
│   ├── test_zvec_engine.py                # 23 tests
│   ├── test_zvec_bridge.py                # 19 tests
│   ├── test_phase2_hybrid.py              # 15 tests
│   ├── test_phase3_ai.py                  # 31 tests
│   └── test_phase4_sdk.py                 # 22 tests
└── pip-package/pyproject.toml             # v0.4.0
```

### Data Directory (per project)

```
project/
└── .gitgo/
    ├── code_index.db                      # SQLite (functions, calls, etc.)
    ├── zvec_collections/                  # Zvec collections (new)
    │   ├── functions/                     # Function-level vectors
    │   └── modules/                       # Module-level vectors
    ├── code_index.local                   # FAISS index (legacy)
    ├── graph.json                         # Knowledge graph
    └── module_graphs/                     # Module visualization
```

---

## 10. Testing

### Run All Python Tests

```bash
cd pip-package
python -m pytest tests/ -v
# 110 tests, 0.13s
```

### Run All Go Tests

```bash
go test ./internal/index/ -v
# 27+ tests
```

### Test Coverage by Phase

| Phase | Tests | Coverage |
|-------|-------|----------|
| 1 (Engine) | 42 Python + 27 Go | Engine init, CRUD, search, bridge protocol |
| 2 (Hybrid) | 15 Python + 8 Go | RRF, sparse upsert, schema, filter |
| 3 (AI) | 31 Python | Embedding providers, pipeline, bridge actions |
| 4 (SDK) | 22 Python | Client lifecycle, MCP tools, handler |
| **Total** | **137+** | **All passing** |
