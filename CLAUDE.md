# CLAUDE.md

Guidance for Claude Code in this repository. Detailed rules auto-load from `.claude/rules/`.

## Project Overview

FlashMemory is a cross-language code analysis & semantic search system: **Go core** (parsing, indexing, ranking, HTTP API, CLI) + **Python SDK** (`pip-package/`) wrapping an in-process **Zvec** vector engine.

Module path: `github.com/kinglegendzzh/flashmemory` · Go 1.23+ (CGO_ENABLED=1)

**IMPORTANT**: Project storage (`.gitgo/` — SQLite + vector collections) lives inside the *indexed project's* root, NOT this repo's working dir.

## Build & Run

```bash
./build_mac.sh            # macOS host build (also copies to githave/bin)
./build_linux.sh          # Linux/amd64 cross-compile via zig (brew install zig required)
./start_fm_http_dev.sh    # Dev HTTP server: API_USER/API_PASS=root, config from fm.yaml
go build -o fm cmd/main/fm.go          # manual core binary
go build -o fm_http cmd/app/fm_http.go # manual HTTP binary
```

## Tests

```bash
go test ./...                                    # whole repo
go test ./internal/index/...                     # one package
go test -run TestEnsureIndexDB ./internal/index  # single test
```

**IMPORTANT**: Prefer single-package or single-test runs; avoid `go test ./...` during active development.

## Architecture — Indexing Pipeline

A single `fm` invocation passes data through 8 stages in order:

1. **`internal/parser/`** — multi-language parsing (Go AST; Tree-sitter for Python/JS/TS/Java/C/C++; regex + LLM fallbacks; doc parser for md/txt/pdf/pptx/docx)
2. **`internal/analyzer/`** — LLM-driven function description & importance scoring
3. **`internal/graph/`** — function call graph & module dependency graph
4. **`internal/index/`** — SQLite persistence + FaissWrapper interface (zvec / faiss / in-memory)
5. **`internal/embedding/`** — dense+sparse vector generation (Ollama, cloud)
6. **`internal/search/`** + **`internal/ranking/`** — recall and rerank
7. **`internal/router/intent_router.go`** — routes queries to semantic/keyword/hybrid based on intent signals
8. **`internal/module_analyzer/`** — async module-level analysis (own goroutines)

## Critical Conventions

**NEVER** commit or rely on `patch.py`, `patch2.py`, `patch3.py`, `patch_faiss.py` — one-off migration helpers only.

**YOU MUST** use `common.I18n(zh, en)` or `common.IsZH()` for all user-facing CLI strings. Both entry points sniff `-lang`/`--lang` before `flag.Parse()`.

**IMPORTANT**: There are two Go entry points + one Cobra wrapper:
- `cmd/main/fm.go` — core binary (flag-style CLI)
- `cmd/app/fm_http.go` — HTTP API (Echo, port 5532)
- `cmd/cli/` — Cobra wrapper that shells out to the core binary via `findFmCoreBinary()`

When changing CLI behavior, update **both** the Cobra command and the underlying core flag.

**IMPORTANT**: The core binary was historically named `fm_core`; build scripts now produce `fm`. The Cobra wrapper searches both names. Keep this in mind when reading shell scripts.

## Config Resolution

`config/config.go` loads `fm.yaml` (project-local) merged with `~/.flashmemory/config.yaml`. Engine defaults come from `config.GetEngine()` when `-engine` is unset.

## Compaction Instructions

When compacting, preserve:
- Full list of modified files
- Test commands run and their pass/fail results
- Any incomplete tasks or open decisions
- Active engine mode in use (zvec vs faiss)

## Rules Index

Detailed rules are in `.claude/rules/` (auto-loaded at session start):

- `@.claude/rules/architecture.md` — engine selection, HTTP API, config, i18n deep-dive
- `@.claude/rules/testing.md` — TDD rules, test-file protection, anti-hallucination
- `@.claude/rules/workflow.md` — SDD process, commit conventions, PR, compression
- `@.claude/rules/security.md` — .env protection, .gitgo safety, migration guards

## Prompt Templates

Structured templates for common tasks: `@docs/templates/`
