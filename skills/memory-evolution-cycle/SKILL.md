---
name: memory-evolution-cycle
description: >
  Run a periodic memory evolution cycle: decay stale observations, deprecate
  low-confidence memories, and prune excess per object. Call every 10 turns
  or on a daily schedule to keep the memory store healthy and relevant.
metadata:
  version: 1.0.0
  category: cognitive-memory
  tags: [evolution, decay, memory-management, deepmemory]
---

# Memory Evolution Cycle — Automated Memory Hygiene

This skill triggers DeepMemory's built-in evolution engine to automatically
maintain memory health. Without periodic evolution, memories accumulate
indefinitely — stale observations pollute recall, and low-confidence hints
waste context window budget.

## When to Use This Skill

- **Every 10 conversation turns** — lightweight maintenance
- **Daily scheduled job** — deeper cleanup
- **Before important decisions** — ensure context is fresh
- **After bulk ingestion** — prune duplicates and low-value memories

## Prerequisites

DeepMemory MCP Server must be running and have an active memory store.

## Instructions

### Step 1: Run Evolution

Call the evolution tool with appropriate parameters:

```
deepmemory_evolve(
  min_confidence=0.35,
  stale_after_hours=168,
  decay_confidence_delta=0.05,
  max_memories_per_object=20
)
```

**Parameter tuning guide:**

| Parameter | Conservative | Default | Aggressive |
|-----------|-------------|---------|------------|
| `min_confidence` | 0.20 | 0.35 | 0.50 |
| `stale_after_hours` | 336 (2 weeks) | 168 (1 week) | 72 (3 days) |
| `decay_confidence_delta` | 0.02 | 0.05 | 0.10 |
| `max_memories_per_object` | 50 | 20 | 10 |

### Step 2: Review the Report

The evolution tool returns a structured report:

```json
{
  "scanned_count": 142,
  "revised_count": 8,
  "deleted_count": 3,
  "actions": [
    {
      "memory_id": "mu_abc",
      "object_id": "user:preference:editor",
      "action": "decay",
      "reason": "stale (192h without verification)",
      "before_confidence": 0.45,
      "after_confidence": 0.40
    }
  ]
}
```

### Step 3: Check for Anomalies

Flag if any of these conditions are true:
- **High deletion rate** (>30% of scanned) → thresholds may be too aggressive
- **Zero actions** → store may be too small or thresholds too loose
- **Same object losing multiple memories** → possible over-pruning

### Step 4: Audit Events (Optional)

For deeper inspection, check the event log:

```
deepmemory_events(limit=20)
```

Look for `memory.deleted` and `memory.revised` events to verify the
evolution acted correctly.

## Scheduling Recommendations

| Context | Frequency | Parameters |
|---------|-----------|------------|
| Active coding session | Every 10 turns | Conservative |
| Daily background job | Once per day | Default |
| Pre-deployment review | Manual trigger | Aggressive |
| Low-activity project | Weekly | Conservative |

## Example Output

```
Evolution cycle complete:
  Scanned: 142 memories
  Decayed: 8 (confidence reduced by 0.05 each)
  Pruned: 3 (exceeded max per object)
  Deleted: 2 (confidence below 0.35)
  Healthy: 129 (no action needed)
```
