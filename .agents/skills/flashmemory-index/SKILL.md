---
name: flashmemory-index
description: >-
  FlashMemory code indexing skill — build and maintain semantic vector indexes
  for large codebases. Use this skill when you need to index a new project,
  re-index after code changes, perform incremental file updates, or switch
  between vector engine backends (Zvec, FAISS). Triggers on: code indexing,
  build index, re-index, vector index, incremental update, full reindex,
  flashmemory index, fm index, zvec, faiss, embedding, vectorize code.
---

# FlashMemory Index — Code Vectorization & Index Management

This skill teaches you how to build, update, and manage FlashMemory's semantic
vector indexes. These indexes power the `fm query` semantic search by converting
source code into vector embeddings.

## When to Use This Skill

- A user asks you to **index a project** for the first time.
- You have **modified files** and need the search index to reflect changes.
- The user reports **stale or missing search results**.
- You need to **switch vector engines** (e.g., from FAISS to Zvec).
- A **large refactor or branch switch** requires a fresh full index.

## Quick Reference

```bash
# First-time full index
fm index .

# Force complete re-index (ignore cache)
fm index . --force-full

# Incremental update after editing a file
fm index . --file path/to/changed_file.go

# Index with explicit engine selection
fm --engine zvec index .

# Index a specific branch
fm index . --branch main

# Index at a specific commit
fm index . --commit abc1234
```

## How Indexing Works

1. **File discovery** — FlashMemory walks the project directory, respecting
   `.gitignore` rules, and discovers source files across supported languages
   (Go, Python, JavaScript, TypeScript, Java, C++, etc.).

2. **Code parsing** — Each file is parsed into semantic units (functions,
   classes, methods, structs) with metadata like package name, dependencies,
   and call graph edges.

3. **LLM analysis** — An LLM generates natural-language descriptions for each
   code unit, capturing intent and behavior beyond what static analysis reveals.

4. **Embedding** — Descriptions are converted to dense vector embeddings
   (default dimension: 384, using all-MiniLM-L6-v2 or equivalent).

5. **Storage** — Embeddings are stored in the configured vector engine:
   - **Zvec** (default, recommended): In-process Python-based engine, zero
     external dependencies.
   - **FAISS**: High-performance C++ engine via subprocess.
   - **Memory**: In-memory store for testing.

6. **Incremental updates** — On subsequent runs, only new or modified files are
   re-processed (unless `--force-full` is specified).

## Complete Flag Reference

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--force-full` | bool | `false` | Discard incremental cache; re-index everything |
| `--file <path>` | string | — | Only re-index this specific file or directory |
| `--branch <name>` | string | `master` | Git branch to associate with the index |
| `--commit <hash>` | string | — | Index code at a specific commit |
| `--search-mode <m>` | string | `semantic` | Index mode: `semantic`, `keyword`, `hybrid` |
| `--faiss <type>` | string | `cpu` | FAISS backend: `cpu` or `gpu` |
| `--use-faiss` | bool | `false` | Use native FAISS index storage |
| `--faiss-path <p>` | string | — | Path to FAISSService directory |

### Global flags (inherited)

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--engine <e>` | string | `zvec` | Vector engine: `zvec`, `faiss`, `memory` |
| `--lang <l>` | string | `en` | UI language |
| `-c, --config <p>` | string | `~/.flashmemory/config.yaml` | Config file |

## Practical Scenarios

### Scenario 1: First-Time Project Setup

```bash
# Step 1: Initialize FlashMemory (if not done yet)
fm init

# Step 2: Navigate to the project root
cd /path/to/project

# Step 3: Build the full index
fm index .
```

The index artifacts are stored in `.gitgo/` within the project directory.

### Scenario 2: After Editing Code

When you (the AI agent) have modified a file, immediately refresh the index for
that file so subsequent queries return accurate results:

```bash
fm index . --file src/services/auth_service.go
```

This is much faster than a full re-index. Use it after every significant edit.

### Scenario 3: After a `git pull` or Branch Switch

When the codebase has changed substantially:

```bash
# If many files changed, do a full re-index
fm index . --force-full

# If you know the branch
fm index . --branch main --force-full
```

### Scenario 4: Switching Vector Engines

The default engine is `zvec`. To use FAISS instead:

```bash
fm --engine faiss index .
```

Or set it permanently in `~/.flashmemory/config.yaml`:

```yaml
zvec_config:
  engine: faiss
```

### Scenario 5: CI/CD Integration

In automated pipelines, index on every push:

```bash
fm init
fm --engine zvec index . --force-full --branch $CI_BRANCH
```

## Index Storage Layout

After indexing, the project directory will contain:

```
project/
├── .gitgo/
│   ├── zvec_collections/    # Zvec vector data
│   ├── *.db                 # SQLite metadata
│   └── *.json               # Function analysis cache
```

The `.gitgo/` directory is typically gitignored. Add this to `.gitignore`:

```
.gitgo/
```

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| Index takes very long | Large codebase + LLM calls | Use `--file` for incremental updates |
| LLM 401 error during index | Invalid API token | Set `api_token` in config.yaml |
| "Cannot find fm core binary" | `fm_core` not in PATH | Ensure full installation |
| Stale results after edits | Index not refreshed | Run `fm index . --file <changed>` |
| Out of memory | Very large FAISS index | Switch to `--engine zvec` |
