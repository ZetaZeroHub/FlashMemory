---
name: memory-augmented-reply
description: >
  Before generating an AI reply, recall relevant memories from DeepMemory and
  search related code from FlashMemory, then inject both as augmented context.
  This is the memory-enhanced RAG pipeline — combining symbolic recall with
  vector search for richer, history-aware responses.
metadata:
  version: 1.0.0
  category: cognitive-memory
  tags: [rag, memory-recall, context-augmentation, deepmemory, flashmemory]
---

# Memory-Augmented Reply — Recall + RAG Context Injection

This skill enriches AI responses by combining two context sources before
reply generation:

1. **DeepMemory recall** — past decisions, user preferences, session history
2. **FlashMemory search** — relevant code functions and modules

Together they provide the AI with both "what we've learned" (memory) and
"what exists in the codebase" (retrieval).

## When to Use This Skill

- **Before every AI reply** in a multi-turn conversation
- **When context matters** — the user references past decisions or preferences
- **When code is involved** — the user asks about implementation details
- **When continuity matters** — cross-session memory should inform the reply

## Prerequisites

Both MCP servers must be running:

```json
{
  "mcpServers": {
    "deepmemory": {
      "command": "python",
      "args": ["-m", "deepmemory.mcp_server"]
    },
    "flashmemory": {
      "command": "python",
      "args": ["-m", "flashmemory.mcp_server"]
    }
  }
}
```

## Instructions

### Step 1: Recall Memories

Query DeepMemory across multiple dimensions:

```
# Session-level memories
deepmemory_recall(object_id="session:{session_id}")

# User-level long-term preferences
deepmemory_recall(object_id="user:{user_id}")

# Topic-specific memories (extract from user message)
deepmemory_recall(object_id="module:{inferred_module}")
deepmemory_recall(object_id="concept:{inferred_concept}")
```

Filter the recalled memories:
- Prioritize `CANONICAL` and `PROVISIONAL_CONSENSUS` status memories
- Include `LOCAL_HYPOTHESIS` if relevant and recent
- Deprioritize or skip `CONTESTED` memories (flag them as uncertain)
- Ignore `PRIVATE_HINT` with confidence < 0.3

### Step 2: Search Code (if applicable)

If the user's message involves code, architecture, or technical questions:

```
flashmemory_search(
  query="{user_message}",
  project_dir="{project_dir}",
  top_k=5,
  search_type="functions"
)
```

If the query is about project structure or module-level concerns:

```
flashmemory_search(
  query="{user_message}",
  project_dir="{project_dir}",
  top_k=3,
  search_type="modules"
)
```

### Step 3: Build Augmented Context

Format the recalled information as a structured context block:

```markdown
## Historical Decisions & Observations

- **[packaging_tool_preference]** (PROVISIONAL_CONSENSUS, confidence: 0.85)
  Claim: user prefers uv over pip — Source: user:alice, session-001

- **[lock_strategy_decision]** (LOCAL_HYPOTHESIS, confidence: 0.72)
  Claim: use RLock for recursive acquisition — Source: coder-agent

## Relevant Code

- **HandleUpload** (handlers package) — score: 0.89
  File: pkg/upload/handler.go:42
  Description: Handles multipart file upload with size validation

- **RetryWithBackoff** (utils package) — score: 0.76
  File: internal/utils/retry.go:15
  Description: Exponential backoff retry wrapper
```

### Step 4: Inject into System Prompt

Prepend the augmented context to the system prompt before generating the reply.
Do NOT generate the reply yourself — return only the context block for the
dialog engine to use.

## Memory Status Priority

| Status | Priority | Usage |
|--------|----------|-------|
| `CANONICAL` | Highest | Treat as established fact |
| `PROVISIONAL_CONSENSUS` | High | Reliable, multiple confirmations |
| `LOCAL_HYPOTHESIS` | Medium | Single-source, use with caveat |
| `CONTESTED` | Low | Flag as uncertain, present both sides |
| `PRIVATE_HINT` | Lowest | Only if nothing better available |

## Examples

### Example: User Asks About Upload Feature

**User message**: "上传功能现在支持什么文件格式？"

**Memory recall** returns:
- `module:upload:config` → "支持 jpg/png/pdf，最大 10MB"（CANONICAL）
- `user:preference:upload` → "用户之前要求支持 webp 格式"（LOCAL_HYPOTHESIS）

**Code search** returns:
- `ValidateFileType()` in `pkg/upload/validator.go` — score 0.91
- `HandleUpload()` in `pkg/upload/handler.go` — score 0.85

**Augmented context**:
```
## Historical Decisions
- [upload_format_config] (CANONICAL): 支持 jpg/png/pdf，最大 10MB
- [upload_format_request] (LOCAL_HYPOTHESIS): 用户曾要求支持 webp

## Relevant Code
- ValidateFileType (upload.validator): 文件类型校验逻辑 — validator.go:28
- HandleUpload (upload.handler): 上传处理入口 — handler.go:42
```
