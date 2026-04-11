<div align="center">

# ⚡ FlashMemory

**Cross-language Code Analysis & Semantic Search System**

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go&logoColor=white)](https://go.dev)
[![Python](https://img.shields.io/badge/Python-3.8+-3776AB?style=flat&logo=python&logoColor=white)](https://python.org)
[![License](https://img.shields.io/badge/License-MIT-green?style=flat)](LICENSE)
[![Version](https://img.shields.io/badge/Version-0.4.0-blue?style=flat)]()

[中文文档](README_CN.md)

</div>

---

## What is FlashMemory?

FlashMemory indexes your codebase using LLM-powered analysis and vector search, enabling **natural language code discovery** across Go, Python, JavaScript, Java, and C++.

**Key capabilities:**

- 🔍 **Hybrid Search** — Dense (semantic) + Sparse (keyword) vectors with RRF fusion
- 🧠 **LLM Analysis** — Automatic function description and importance scoring
- 📊 **Knowledge Graph** — Function call relationships and module dependencies
- ⚡ **Incremental Index** — Git-aware updates, only re-index changed files
- 🔌 **MCP Integration** — Expose search tools to AI agents via Model Context Protocol
- 🏎️ **Zvec Engine** — In-process vector database, no external service required

---

## Installation

```bash
# Python SDK with local embedding (recommended)
pip install flashmemory[embedding]

# Basic install (fallback embedding)
pip install flashmemory

# Full install with cloud embedding providers
pip install flashmemory[full]

# Build Go CLI from source
go build -o fm cmd/main/fm.go
```

---

## Quick Start

### CLI Usage

```bash
# Index a project (Zvec engine, recommended — no FAISS service needed)
fm -dir /path/to/project -engine zvec

# Search with natural language
fm -dir /path/to/project -engine zvec -query "file upload handler"

# Hybrid search mode (semantic + keyword)
fm -dir /path/to/project -query "authentication" -search_mode hybrid

# Incremental update (only changed files)
fm -dir /path/to/project

# Index a specific directory
fm -dir /path/to/project -file src/handlers/

# Force full re-index
fm -dir /path/to/project -force_full

# Legacy FAISS mode (backward compatible)
fm -dir /path/to/project
```

### Python SDK

```python
from flashmemory import FlashMemoryClient

# Context manager (recommended)
with FlashMemoryClient(project_dir="/path/to/project") as client:
    # Semantic search
    results = client.search("file upload handler", top_k=10)
    for r in results:
        print(f"{r['fields'].get('func_name')} → {r['score']:.3f}")

    # Filter by language
    results = client.search_functions("auth middleware", language="go")

    # Search module descriptions
    results = client.search_modules("search engine")

    # Generate embeddings
    vec = client.embed("search query text")

    # Add a function to the search index
    client.add_function("func_1", "Handle file upload and save to disk", {
        "func_name": "UploadFile",
        "package": "handlers",
        "language": "go",
    })
```

### MCP Integration

```python
from flashmemory import get_mcp_tools, handle_mcp_tool_call

# Get MCP tool definitions to register with your MCP server
tools = get_mcp_tools()
# Returns three tools:
# - flashmemory_search: Natural language code search
# - flashmemory_index:  Add functions to search index
# - flashmemory_info:   Get engine status and diagnostics

# Handle AI agent tool calls (with built-in client caching)
client_cache = {}
result = handle_mcp_tool_call(
    "flashmemory_search",
    {
        "project_dir": "/path/to/project",
        "query": "database connection pool",
        "top_k": 5,
        "language": "go",
    },
    client_cache=client_cache,
)
```

---

## Architecture

```
┌──────────────────────────────────────┐
│  FlashMemoryClient (Python SDK)      │  High-level API
├──────────────────────────────────────┤
│  SearchPipeline                      │  Recall → Rerank
│  EmbeddingProvider                   │  Dense + Sparse embedding
├──────────────────────────────────────┤
│  ZvecEngine (Collection CRUD)        │  Vector storage & retrieval
│  ZvecBridge (JSON-line Protocol)     │  Go ↔ Python IPC
├──────────────────────────────────────┤
│  Go Core                             │
│  Parser · Analyzer · Graph · Index   │  Code analysis pipeline
│  Search · Ranking · Embedding        │  Search & scoring
├──────────────────────────────────────┤
│  Storage                             │
│  Zvec (HNSW + Sparse) · SQLite       │  Vectors + metadata
│  FAISS (legacy) · File system        │  Backward compatible
└──────────────────────────────────────┘
```

---

## CLI Reference

| Flag | Default | Description |
|------|---------|-------------|
| `-dir` | `.` | Project directory to index |
| `-query` | `""` | Natural language search query |
| `-engine` | `faiss` | Vector engine: `zvec` (recommended) or `faiss` |
| `-search_mode` | `semantic` | `semantic`, `keyword`, or `hybrid` |
| `-force_full` | `false` | Force full re-index |
| `-branch` | `master` | Git branch name |
| `-commit` | `""` | Specific commit hash |
| `-file` | `""` | Index specific file or directory |
| `-query_only` | `false` | Search only, skip indexing |

---

## HTTP API

Start the API server:

```bash
fm serve --port 5532
```

Key endpoints:

| Method | Route | Description |
|--------|-------|-------------|
| GET | `/api/health` | Health check |
| POST | `/api/search` | Code search (semantic / keyword / hybrid) |
| POST | `/api/index` | Build index |
| DELETE | `/api/index` | Delete index |
| POST | `/api/index/incremental` | Incremental update |
| POST | `/api/index/check` | Check index status |
| POST | `/api/functions` | List functions |
| POST | `/api/ranking` | Function importance ranking |
| POST | `/api/module-graphs/update` | Update module graphs (async) |
| GET | `/c/config` | Get configuration |
| PUT | `/c/config` | Update configuration |

See [HTTP API Deep Analysis](docs/http_api_deep_analysis.md) for the full reference.

---

## Supported Languages

| Language | Parser | Extensions |
|----------|--------|------------|
| Go | AST | `.go` |
| Python | Tree-sitter | `.py` |
| JavaScript / TypeScript | Tree-sitter | `.js` `.ts` `.jsx` `.tsx` |
| Java | Tree-sitter | `.java` |
| C / C++ | Tree-sitter | `.c` `.cpp` `.h` `.hpp` |

---

## Configuration

FlashMemory uses `fm.yaml` for project configuration:

```yaml
# LLM settings
api_url: "https://api.openai.com/v1"
api_model: "gpt-4o-mini"
api_token: "sk-..."

# Zvec vector engine (recommended, v0.4.0+)
zvec_config:
  collection_path: ".gitgo/zvec_collections"
  dimension: 384
  metric_type: "cosine"
```

---

## Project Structure

```
flashmemory/
├── cmd/
│   ├── main/fm.go              # CLI entry point (with -engine flag)
│   ├── app/fm_http.go          # HTTP API server
│   └── cli/                    # Cobra sub-commands
├── internal/
│   ├── parser/                 # Multi-language code parsing
│   ├── analyzer/               # LLM-powered analysis
│   ├── graph/                  # Knowledge graph
│   ├── index/                  # SQLite + vector index (Zvec/FAISS)
│   ├── search/                 # Search engine
│   ├── embedding/              # Vector embedding
│   ├── ranking/                # Function importance scoring
│   └── module_analyzer/        # Async module analysis
├── pip-package/flashmemory/    # Python SDK
│   ├── zvec_engine.py          # Zvec collection management
│   ├── zvec_bridge.py          # Subprocess bridge (15 actions)
│   ├── embedding_provider.py   # Multi-source embedding
│   ├── search_pipeline.py      # Two-stage retrieval
│   └── client.py               # FlashMemoryClient + MCP tools
├── config/                     # Configuration management
├── docs/                       # Documentation
│   ├── zvec_integration_guide.md      # Zvec Integration Guide (EN)
│   ├── zvec_integration_guide_cn.md   # Zvec 集成指南（中文）
│   └── http_api_deep_analysis.md      # HTTP API reference
└── fm.yaml                     # Project configuration
```

---

## Documentation

| Document | Description |
|----------|-------------|
| [Zvec Integration Guide](docs/zvec_integration_guide.md) | Complete guide: engine, hybrid search, embedding, SDK, MCP |
| [Zvec 集成指南（中文）](docs/zvec_integration_guide_cn.md) | Zvec 集成中文版完整指南 |
| [HTTP API Deep Analysis](docs/http_api_deep_analysis.md) | Full HTTP API reference with call chain analysis |
| [Release Guide](docs/release_guide.md) | Build and release instructions |

---

## License

MIT License — see [LICENSE](LICENSE) for details.
