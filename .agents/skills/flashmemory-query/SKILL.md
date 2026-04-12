---
name: flashmemory-query
description: >-
  FlashMemory semantic code search skill — find implementations, understand
  architecture, and navigate large codebases using natural language queries
  instead of grep. Use this skill when the user asks to find code, locate
  implementations, understand how something works, or when you need to search
  a codebase that is too large to read entirely. Triggers on: code search,
  find implementation, where is, how does, semantic search, fm query,
  search codebase, locate function, find usage, code navigation, grep
  alternative, natural language search, hybrid search.
---

# FlashMemory Query — Semantic Code Search

This skill teaches you how to use FlashMemory's semantic search to find
relevant code across large indexed codebases using natural language queries.
Think of it as a smarter alternative to `grep` — it understands intent, not
just text patterns.

## When to Use This Skill

- You need to **find where something is implemented** but don't know the
  file or function name.
- The **codebase is too large** to read entirely — use search to retrieve
  only the relevant parts.
- The user asks questions like "where is X?", "how does Y work?",
  "find the Z logic".
- You want to **explore unfamiliar code** by asking conceptual questions.
- Traditional `grep` is **insufficient** because you're searching for
  behavior, not literal strings.

## Prerequisites

The project must be indexed first. Check for the `.gitgo/` directory:

```bash
ls -d .gitgo/ 2>/dev/null && echo "Index exists" || echo "Run: fm index ."
```

If no index exists, build one first:

```bash
fm index .
```

## Quick Reference

```bash
# Basic semantic search
fm query "authentication middleware"

# More results
fm query "database connection pool" --limit 10

# Include code snippets in output
fm query "error handling" --include-code

# Hybrid mode (semantic + keyword — best for function names)
fm query "handleLogin" --mode hybrid

# Keyword-only mode (fastest, like enhanced grep)
fm query "TODO fixme" --mode keyword

# Search in a specific directory
fm query "config loading" --dir /path/to/project
```

## Complete Flag Reference

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--mode <mode>` | string | `semantic` | Search mode (see below) |
| `--limit <n>` | int | `5` | Max number of results |
| `--include-code` | bool | `false` | Show code snippets in results |
| `--dir <path>` | string | `.` | Project directory to search |

### Global flags (inherited)

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--engine <e>` | string | `zvec` | Vector engine to use |
| `--lang <l>` | string | `en` | UI language |

## Search Modes Explained

### `semantic` (default)

Uses vector similarity to find code that is **conceptually related** to your
query, even if no words match literally.

**Best for:**
- "How does the app handle user sessions?"
- "Where is the rate limiting logic?"
- "Error retry mechanism"

**Example:**
```bash
fm query "user session management" --mode semantic --limit 5
```

### `keyword`

Traditional keyword-based search across indexed function/class descriptions.
Faster than semantic but less flexible.

**Best for:**
- Searching for specific identifiers: `handleLogin`, `UserService`
- Finding exact terms: `TODO`, `FIXME`, `deprecated`

**Example:**
```bash
fm query "UserService" --mode keyword
```

### `hybrid`

Combines semantic and keyword search, re-ranking results from both. Usually
gives the best results at the cost of slightly more compute.

**Best for:**
- When you have a mix of concepts and specific names.
- "JWT token validation in authMiddleware"
- General-purpose searching when unsure which mode to pick.

**Example:**
```bash
fm query "JWT validation middleware" --mode hybrid --limit 10
```

## Agent Search Strategies

### Strategy 1: Broad-to-Narrow

Start with a broad conceptual query, then narrow down:

```bash
# Step 1: Broad search to find the right area
fm query "authentication" --limit 10

# Step 2: Read the top results, then narrow
fm query "JWT token refresh logic" --limit 5 --include-code
```

### Strategy 2: Architecture Discovery

When you need to understand the overall project structure:

```bash
# Find entry points
fm query "main function entry point" --limit 5

# Find routing / handler registration
fm query "HTTP route registration" --limit 10

# Find database layer
fm query "database connection and migration" --limit 5

# Find configuration loading
fm query "config initialization" --limit 5
```

### Strategy 3: Bug Investigation

When debugging, search for the behavior, not the symptom:

```bash
# Don't search: "nil pointer dereference"
# Instead search for the behavior area:
fm query "user profile loading and caching" --include-code --limit 5
```

### Strategy 4: Pre-Modification Research

Before modifying code, find all related files:

```bash
# Find all code related to the feature you're about to change
fm query "payment processing flow" --limit 15

# Find test files for the area
fm query "payment test cases" --limit 5
```

### Strategy 5: Cross-Language Search

FlashMemory indexes multiple languages. Your query will find results across
all indexed languages:

```bash
# This finds Go handlers, Python scripts, JS clients, etc.
fm query "API authentication" --limit 10
```

## HTTP API Alternative

If the FlashMemory HTTP server is running (`fm serve`), you can also query
programmatically:

```bash
# Check if server is running
fm status

# Query via HTTP
curl -s -X POST http://localhost:5532/api/search \
  -H "Content-Type: application/json" \
  -d '{
    "query": "authentication middleware",
    "limit": 5,
    "search_mode": "hybrid"
  }' | python3 -m json.tool
```

The HTTP API returns structured JSON with file paths, line numbers, scores,
and code snippets — useful for programmatic integration.

## Interpreting Results

Query results are ranked by relevance score (higher = better match). Each
result typically includes:

- **File path** — where the code lives
- **Function/class name** — the semantic unit that matched
- **Description** — LLM-generated summary of what the code does
- **Score** — relevance ranking (0.0 to 1.0+)
- **Code snippet** — actual source code (when `--include-code` is used)

**Reading strategy for agents:**
1. Check the top 3-5 results.
2. Read the file paths and descriptions first.
3. Open and read the most relevant files for full context.
4. If results are not relevant, rephrase the query or try `hybrid` mode.

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| No results | Project not indexed | Run `fm index .` |
| Irrelevant results | Wrong search mode | Try `--mode hybrid` |
| Too few results | Low limit | Increase `--limit 15` |
| Stale results | Index outdated | Run `fm index . --force-full` |
| "Cannot find fm core binary" | Incomplete install | Reinstall FlashMemory |
