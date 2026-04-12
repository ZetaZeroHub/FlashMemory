---
name: flashmemory-core
description: >
  FlashMemory CLI for cross-language code analysis and semantic search.
  Use this skill when you need to understand project architecture, locate
  implementations, or index large codebases using natural language queries.
  Make sure to use this skill whenever the user asks about the codebase structure, finding code, or searching files.
metadata:
  version: 1.0.0
  category: developer-tools
  tags: [search, index, code-analysis, flashmemory]
---

# FlashMemory Core — AI Agent Integration Guide

FlashMemory is a cross-language code analysis and semantic search system. It
indexes source code (Go, Python, JavaScript, Java, C++, etc.) using LLM-driven
analysis and vector embeddings, then exposes blazing-fast semantic search over
the indexed codebase.

## When to Use FlashMemory

Use FlashMemory when you need to:

- **Understand a large codebase** — index the project first, then query for
  concepts instead of grepping filenames.
- **Locate implementations** — ask natural-language questions like
  "where is the authentication middleware?" and get ranked results.
- **Augment your context window** — when the codebase is too large to read
  entirely, use `fm query` to retrieve the most relevant files.
- **After modifying code** — re-index changed files so future searches stay
  accurate.

## Prerequisites

FlashMemory must be installed. Verify with:

```bash
fm version
```

If not installed, the user can install via one of:

```bash
# Homebrew (macOS / Linux)
brew tap ZetaZeroHub/flashmemory && brew install flashmemory

# npm (cross-platform, auto-downloads binaries)
npm install -g @zhouzhanghao001111/flashmemory

# pip (Python SDK + CLI wrapper)
pip install flashmemory
```

## Complete CLI Reference

### Global Flags

Every `fm` subcommand accepts these persistent flags:

| Flag | Description | Default |
|------|-------------|---------|
| `--engine <zvec\|faiss>` | Vector engine backend | `zvec` |
| `--lang <en\|zh>` | UI language | `en` |
| `-c, --config <path>` | Config file path | `~/.flashmemory/config.yaml` |

### Workflow Overview

```
fm init          → One-time setup (creates ~/.flashmemory/)
fm index <dir>   → Build / update vector index for a project
fm query <text>  → Semantic search over indexed code
fm serve         → Start HTTP API server (background)
fm status        → Check if HTTP service is running
fm stop          → Stop HTTP service
fm logs          → View HTTP service logs
fm config        → Display current configuration
fm version       → Show version
```

---

## Instructions

### 1. Initialize FlashMemory

Run once per machine. Creates `~/.flashmemory/` with a default `config.yaml`.

```bash
fm init
```

If the user already has a config from a previous version, `fm init` will
**merge** new default keys (like `zvec_config`) without overwriting existing
user settings.

### 2. Index a Project

Build the semantic vector index for a project directory:

```bash
# Index current directory (most common)
fm index .

# Index a specific path
fm index /path/to/project

# Force a full re-index (ignore incremental cache)
fm index . --force-full

# Incrementally update a single file or directory
fm index . --file src/auth/login.go

# Use a specific vector engine
fm --engine zvec index .
```

**Key flags for `fm index`:**

| Flag | Description | Default |
|------|-------------|---------|
| `--force-full` | Force full re-index | `false` |
| `--file <path>` | Partial update for a single file/dir | — |
| `--branch <name>` | Git branch to index | `master` |
| `--commit <hash>` | Index at a specific commit | — |
| `--search-mode <mode>` | `semantic`, `keyword`, `hybrid` | `semantic` |
| `--faiss <cpu\|gpu>` | FAISS backend type (legacy) | `cpu` |

**When to re-index:**
- After significant code changes (new files, refactors).
- After pulling new commits from upstream.
- Use `--file` for surgical updates after editing a single file.

### 3. Query the Codebase

Search using natural language:

```bash
# Basic semantic search
fm query "authentication middleware"

# Get more results
fm query "database connection pooling" --limit 10

# Include code snippets in output
fm query "error handling pattern" --include-code

# Use hybrid mode (semantic + keyword)
fm query "JWT token validation" --mode hybrid

# Search in a specific project directory
fm query "config loading" --dir /path/to/project
```

**Key flags for `fm query`:**

| Flag | Description | Default |
|------|-------------|---------|
| `--mode <mode>` | `semantic`, `keyword`, `hybrid` | `semantic` |
| `--limit <n>` | Number of results to return | `5` |
| `--include-code` | Include code snippets in results | `false` |
| `--dir <path>` | Project directory to search | `.` |

**Tips for effective queries:**
- Use descriptive natural language, not grep patterns.
- `hybrid` mode often gives the best results for specific function names.
- Increase `--limit` when exploring unfamiliar code.

### 4. HTTP API Server

Start a persistent HTTP server for programmatic access:

```bash
# Start in background (default)
fm serve

# Start on a custom port
fm serve --port 8080

# Start in foreground (for debugging)
fm serve --foreground

# Check status
fm status

# View logs
fm logs

# Stop
fm stop
```

### 5. Configuration

View current configuration:

```bash
fm config
```

The config file lives at `~/.flashmemory/config.yaml`. Key settings:

```yaml
api_token: ""           # LLM API token for code analysis
api_url: ""             # LLM API endpoint
api_model: ""           # LLM model name
lang: en                # Default UI language
zvec_config:
  engine: zvec           # Vector engine: zvec | faiss | memory
  collection_dir: .gitgo/zvec_collections
  dimension: 384
```

## Agent Integration Patterns

### Pattern 1: Pre-flight Index Check

Before answering architecture questions, check if an index exists:

```bash
# Check if .gitgo/ directory exists in the project root
ls -la .gitgo/
```

If it does not exist, run `fm index .` first.

### Pattern 2: Search Before Reading

When you need to find where something is implemented:

```bash
fm query "user authentication flow" --limit 5 --include-code
```

Then read the top-ranked files to build context.

### Pattern 3: Post-Edit Refresh

After modifying a file, update the index:

```bash
fm index . --file src/auth/login.go
```

### Pattern 4: HTTP API for Programmatic Access

If the HTTP server is running (`fm status`), you can also query via curl:

```bash
curl -X POST http://localhost:5532/api/search \
  -H "Content-Type: application/json" \
  -d '{"query": "authentication", "limit": 5}'
```

## Troubleshooting

| Symptom | Solution |
|---------|----------|
| `fm: command not found` | Install via brew/npm/pip (see Prerequisites) |
| `Config file not found` | Run `fm init` |
| `Cannot find fm core binary` | Ensure `fm_core` is in PATH or `~/.flashmemory/bin/` |
| LLM errors (401 Unauthorized) | Set valid `api_token` in `~/.flashmemory/config.yaml` |
| Stale search results | Re-index with `fm index .` or `fm index . --force-full` |
