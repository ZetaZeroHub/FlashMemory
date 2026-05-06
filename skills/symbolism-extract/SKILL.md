---
name: symbolism-extract
description: >
  Extract symbolic order from conversations: identify key concepts as Signs,
  anchor them to ObjectRefs, and record interpretant claims via DeepMemory MCP.
  This is the "semiotics deconstructs semantics" engine — it turns natural language
  into structured, contestable memory units instead of opaque vectors.
metadata:
  version: 1.0.0
  category: cognitive-memory
  tags: [semiotics, symbol-extraction, deepmemory, memory-ingest]
---

# Symbolism Extract — Semiotic Deconstruction Engine

This skill automatically extracts symbolic structures from dialog messages
and persists them as DeepMemory MemoryUnits. It implements the core idea of
"using semiotics to deconstruct semantics" — every observation becomes a
structured triple of Sign + ObjectRef + Interpretant, not just a vector.

## When to Use This Skill

- **After each AI reply** — extract key decisions, observations, preferences
- **After user messages** — capture user intent, corrections, stated preferences
- **After code changes** — record the reasoning behind architectural decisions
- **When switching context** — summarize the current session's key insights

## Prerequisites

The DeepMemory MCP Server must be running and registered:

```json
{
  "mcpServers": {
    "deepmemory": {
      "command": "python",
      "args": ["-m", "deepmemory.mcp_server"],
      "env": {"DEEPMEMORY_STORE": "./artifacts/memory.jsonl"}
    }
  }
}
```

## Instructions

### Step 1: Analyze the Message

Read the input message and identify extractable triples:

1. **Signs** — Key concepts, preferences, decisions, or observations
   - Examples: "packaging tool preference", "auth middleware pattern", "retry strategy"
   - Use `snake_case` English for `canonical_sign`
   - List natural-language variants in the user's language

2. **ObjectRefs** — What concrete object does this sign point to?
   - `object_id`: hierarchical identifier (e.g. `user:preference:packaging`, `module:auth:middleware`)
   - `object_type`: one of `concept`, `module`, `user_preference`, `decision`, `bug`, `feature`
   - `anchor_kind`: `symbol`, `locator`, or `config`
   - `anchor_locator`: file path or logical location

3. **Interpretants** — Who believes what, and how strongly?
   - `actor_ref`: the speaker (`user:alice`, `coder-agent`, `reviewer-agent`)
   - `operational_claim`: one-sentence claim in the speaker's language
   - `confidence`: inferred from language cues
     - Definitive statements ("always use", "never do") → 0.85-0.95
     - Preferences ("I prefer", "I like") → 0.70-0.85
     - Tentative ("maybe", "could try") → 0.40-0.60
     - Questions or uncertainty → 0.20-0.40

### Step 2: Check for Existing Memories

Before ingesting, call `deepmemory_recall` for each `object_id` to check
if this concept already has memories:

```
deepmemory_recall(object_id="user:preference:packaging")
```

- **No existing memories** → proceed to ingest
- **Existing + consistent** → skip (don't duplicate)
- **Existing + supplementary** → ingest as new observation
- **Existing + contradictory** → revise the old memory via `deepmemory_revise`,
  or ingest as a new competing claim (the game-theory-debate skill will resolve it)

### Step 3: Ingest New Memories

For each extracted triple, call the DeepMemory MCP tool:

```
deepmemory_ingest(
  summary="user prefers uv over pip for dependency management",
  object_id="user:preference:packaging",
  object_type="user_preference",
  anchor_kind="config",
  anchor_locator="pyproject.toml",
  template="task",
  actor_ref="user:alice",
  source_ref="session-001-turn-005",
  context='{"session_id": "abc", "turn": 5}'
)
```

### Step 4: Report Extraction Results

Output a structured summary:

```json
{
  "extracted": [
    {
      "sign": "packaging_tool_preference",
      "object_id": "user:preference:packaging",
      "claim": "用户偏好uv而非pip",
      "confidence": 0.85,
      "action": "ingested"
    }
  ],
  "skipped": 0,
  "revised": 0
}
```

## Template Selection Guide

| Scenario | Template | Source Kind |
|----------|----------|------------|
| User states a preference or observation | `task` | `task_note` |
| Architectural decision or design choice | `architecture` | `architecture_decision` |
| Bug fix reasoning or incident response | `repair` | `incident_repair` |

## Examples

### Example 1: User Preference

**Input**: "我更喜欢用 uv 而不是 pip 来管理 Python 依赖"

**Extraction**:
- Sign: `packaging_tool_preference`
- ObjectRef: `user:preference:packaging` (type: `user_preference`)
- Interpretant: actor=`user:alice`, claim="偏好uv管理Python依赖", confidence=0.82

### Example 2: Architectural Decision

**Input**: "We should use RLock instead of Lock here to allow recursive acquisition"

**Extraction**:
- Sign: `lock_strategy_decision`
- ObjectRef: `module:cache:concurrency` (type: `decision`)
- Interpretant: actor=`coder-agent`, claim="use RLock for recursive lock acquisition", confidence=0.88

### Example 3: Code Change Reasoning

**Input**: "Narrowed the lock scope to reduce contention under high concurrency"

**Extraction**:
- Sign: `lock_scope_optimization`
- ObjectRef: `module:upload:service` (type: `decision`)
- Interpretant: actor=`coder-agent`, claim="narrow lock scope to reduce contention", confidence=0.90
