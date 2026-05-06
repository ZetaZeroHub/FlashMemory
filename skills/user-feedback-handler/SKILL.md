---
name: user-feedback-handler
description: >
  Process user feedback on AI memories: agreement promotes status,
  disagreement deletes, refinement revises claims. Translates natural
  user reactions into DeepMemory governance operations.
metadata:
  version: 1.0.0
  category: cognitive-memory
  tags: [feedback, user-correction, memory-governance, deepmemory]
---

# User Feedback Handler — Human-in-the-Loop Memory Governance

This skill translates user feedback signals (thumbs up, corrections,
disagreements) into DeepMemory governance operations. It closes the loop
between AI observations and human ground truth.

## When to Use This Skill

- **User says "yes", "correct", "exactly"** → promote the relevant memory
- **User says "no", "wrong", "that's not right"** → delete or contest
- **User says "actually..." or provides correction** → revise the claim
- **User clicks thumbs up/down** on a displayed memory → promote or delete
- **User explicitly asks to remember/forget** → ingest or delete

## Prerequisites

DeepMemory MCP Server must be running. The system should track which
memories were surfaced in the current conversation so feedback can be
mapped to specific `memory_id` values.

## Instructions

### Step 1: Classify the Feedback

Analyze the user's message to determine feedback type:

| Signal | Type | Confidence Boost |
|--------|------|-----------------|
| "yes", "correct", "exactly", "good", 👍 | `agree` | +0.15 |
| "no", "wrong", "that's not right", 👎 | `disagree` | N/A (delete) |
| "actually...", "not exactly, it's...", correction | `refine` | reset to 0.70 |
| "remember that...", "keep in mind..." | `reinforce` | +0.20 |
| "forget that", "ignore that", "never mind" | `forget` | N/A (delete) |

### Step 2: Identify the Target Memory

Determine which memory the feedback refers to:

1. **Explicit reference** — user mentions the concept by name
2. **Most recent** — feedback likely targets the last surfaced memory
3. **Context-based** — match feedback content to recalled memories

If ambiguous, ask the user to clarify which observation they're responding to.

### Step 3: Execute the Operation

**For `agree` feedback:**
```
deepmemory_promote(
  memory_id="mu_xxx",
  status="provisional_consensus"
)
```
If already at `provisional_consensus`, promote to `canonical`.

**For `disagree` feedback:**
```
deepmemory_delete(
  memory_id="mu_xxx",
  reason="user_disagreement"
)
```

**For `refine` feedback:**
```
deepmemory_revise(
  memory_id="mu_xxx",
  operational_claim="the corrected claim from user",
  confidence=0.70
)
```

**For `reinforce` feedback:**
```
deepmemory_revise(
  memory_id="mu_xxx",
  confidence=0.95
)
deepmemory_promote(
  memory_id="mu_xxx",
  status="canonical"
)
```

**For `forget` feedback:**
```
deepmemory_delete(
  memory_id="mu_xxx",
  reason="user_requested_removal"
)
```

### Step 4: Acknowledge to User

Confirm the action briefly:

| Action | Response Template |
|--------|------------------|
| Promoted | "Got it, I've noted that as confirmed." |
| Deleted | "Understood, I've removed that observation." |
| Revised | "Updated — I'll remember the corrected version." |
| Reinforced | "Marked as a strong preference." |

## Examples

### Example 1: User Agrees

**Context**: AI previously recalled "user prefers uv for packaging"
**User**: "对，一直用 uv"

**Action**: `deepmemory_promote(memory_id, "canonical")` — user explicitly confirmed

### Example 2: User Corrects

**Context**: AI recalled "project uses PostgreSQL"
**User**: "不是 PostgreSQL，我们用的 SQLite"

**Action**: `deepmemory_revise(memory_id, operational_claim="项目使用SQLite而非PostgreSQL", confidence=0.90)`

### Example 3: User Disagrees

**Context**: AI recalled "user wants tests before implementation"
**User**: "不需要这个规则了，现在我们直接写代码"

**Action**: `deepmemory_delete(memory_id, reason="user_changed_workflow")`

## Edge Cases

- **Partial agreement** ("mostly right, but...") → treat as `refine`, keep the
  agreed part, update the corrected part
- **Sarcastic agreement** ("oh sure, that worked great /s") → treat as `disagree`
- **Conditional feedback** ("that's right for Python, but not for Go") → revise
  the scope rather than the claim
- **No target memory found** — if the user corrects something that was never
  recorded, ingest it as a new memory with high confidence (0.85)
