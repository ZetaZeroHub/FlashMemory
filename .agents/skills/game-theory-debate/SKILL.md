---
name: game-theory-debate
description: >
  Run a multi-agent debate around contested memories. Multiple agents argue
  for/against a memory claim, then vote to determine its final social status.
  This is the "game theory enhances collective memory" engine — consensus
  emerges through structured argumentation, not just majority counting.
metadata:
  version: 1.0.0
  category: cognitive-memory
  tags: [game-theory, debate, consensus, multi-agent, deepmemory]
---

# Game Theory Debate — Multi-Agent Memory Arbitration

This skill orchestrates a structured debate between multiple AI agents
around a contested or uncertain memory. Through argumentation and voting,
the memory's social status is updated to reflect collective judgment.

This is the core implementation of "using game theory to enhance collective memory."

## When to Use This Skill

- **Conflicting memories detected** — two memories about the same object contradict
- **Low-confidence critical decisions** — a memory with high impact but low confidence
- **User disputes a memory** — user says "that's wrong" or provides counter-evidence
- **Periodic review** — scheduled review of `CONTESTED` status memories
- **Cross-agent disagreement** — different agents recorded different claims

## Prerequisites

DeepMemory MCP Server must be running. This skill uses:
- `deepmemory_recall` — to retrieve the contested memories
- `deepmemory_promote` — to update status after verdict
- `deepmemory_revise` — to update claim if revised
- `deepmemory_delete` — to remove if deprecated

## Instructions

### Step 1: Identify the Contested Topic

Retrieve all memories for the disputed object:

```
deepmemory_recall(object_id="{object_id}")
```

Identify the conflict:
- **Direct contradiction**: Memory A says X, Memory B says NOT X
- **Nuance disagreement**: Memory A says X strongly, Memory B says X weakly
- **Scope conflict**: Memory A applies globally, Memory B only locally
- **Staleness**: Memory A is old, Memory B is recent with new evidence

### Step 2: Assign Debate Roles

Select 3 agents with distinct perspectives:

| Role | Perspective | Bias |
|------|-------------|------|
| **Advocate** | Argues FOR the memory's current claim | Stability-biased |
| **Challenger** | Argues AGAINST or proposes revision | Change-biased |
| **Arbiter** | Weighs evidence neutrally | Evidence-biased |

If the system has named agents (e.g., `coder-agent`, `reviewer-agent`,
`analyst-agent`), assign them based on domain expertise.

### Step 3: Run the Debate (3 Rounds)

**Round 1 — Opening Statements**

Each agent presents their position:
- What evidence supports their view?
- What is the confidence level?
- What would change their mind?

**Round 2 — Cross-Examination**

Each agent responds to the others:
- Address specific counter-arguments
- Present additional evidence
- Identify logical gaps in opposing arguments

**Round 3 — Closing Statements**

Each agent gives final position:
- Summary of strongest argument
- Remaining uncertainties
- Final recommendation: CONFIRM / REVISE / DEPRECATE

### Step 4: Vote and Decide

Each agent casts a vote:

| Vote | Action |
|------|--------|
| `CONFIRM` | Promote to `provisional_consensus` or `canonical` |
| `REVISE` | Update `operational_claim` and/or `confidence` via `deepmemory_revise` |
| `DEPRECATE` | Delete via `deepmemory_delete` with reason |
| `CONTEST` | Mark as `contested` for future review |

**Decision rules:**
- Unanimous CONFIRM → promote to `canonical`
- Majority CONFIRM → promote to `provisional_consensus`
- Majority REVISE → revise claim, keep status
- Majority DEPRECATE → delete the memory
- No majority → mark as `contested`

### Step 5: Execute the Verdict

Based on the vote outcome, call the appropriate MCP tool:

```
# If CONFIRM (majority)
deepmemory_promote(memory_id="mu_xxx", status="provisional_consensus")

# If REVISE (majority)
deepmemory_revise(
  memory_id="mu_xxx",
  operational_claim="revised claim text",
  confidence=0.75
)

# If DEPRECATE (majority)
deepmemory_delete(memory_id="mu_xxx", reason="deprecated_by_debate")
```

### Step 6: Generate Debate Report

Output a structured report:

```json
{
  "topic": "lock strategy for upload module",
  "object_id": "module:upload:concurrency",
  "memory_id": "mu_abc123",
  "rounds": [
    {
      "round": 1,
      "statements": [
        {"agent": "advocate", "position": "CONFIRM", "argument": "..."},
        {"agent": "challenger", "position": "REVISE", "argument": "..."},
        {"agent": "arbiter", "position": "CONFIRM", "argument": "..."}
      ]
    }
  ],
  "votes": {
    "advocate": "CONFIRM",
    "challenger": "REVISE",
    "arbiter": "CONFIRM"
  },
  "verdict": "CONFIRM",
  "final_status": "provisional_consensus",
  "reasoning": "2/3 agents confirmed based on production evidence"
}
```

## Convergence Safeguards

- **Max 3 rounds** — prevent infinite argumentation
- **Timeout per round** — each agent gets one response per round
- **Deadlock resolution** — if no majority after 3 rounds, status becomes `contested`
- **No re-debate within 24h** — prevent flip-flopping on the same memory

## Example: Contradicting Packaging Preferences

**Situation**: Memory A says "user prefers pip", Memory B says "user prefers uv"

**Round 1**:
- Advocate (for A): "pip was stated in session-001, explicit user preference"
- Challenger (for B): "uv was stated later in session-003, supersedes earlier"
- Arbiter: "temporal precedence suggests B is more current"

**Round 2**:
- Advocate: "session-001 had higher confidence (0.9 vs 0.7)"
- Challenger: "user explicitly corrected themselves in session-003"
- Arbiter: "correction signals are stronger than initial statements"

**Round 3 & Vote**:
- Advocate: REVISE (accepts newer evidence)
- Challenger: CONFIRM B (confident in newer claim)
- Arbiter: CONFIRM B

**Verdict**: Deprecate Memory A, promote Memory B to `provisional_consensus`
