#!/usr/bin/env python3
"""
End-to-end verification for DeepMemory + FlashMemory MCP integration.

Validates:
  1. DeepMemory MCP Server — all 7 tools (ingest → recall → promote → revise → evolve → events → delete)
  2. FlashMemory MCP Server — tool registration (search, index, info)
  3. Cross-server workflow — symbolism extract → memory recall → evolution cycle

Usage:
    python scripts/verify_mcp_integration.py
"""

import json
import os
import sys
import tempfile
from pathlib import Path

# ---------------------------------------------------------------------------
# Setup paths — resolve across worktree and main repo
# ---------------------------------------------------------------------------

ROOT = Path(__file__).resolve().parent.parent

# Try worktree first, fall back to main repo
def _find_pkg(name: str) -> Path:
    candidates = [
        ROOT / name,
        ROOT.parent.parent.parent / name,  # worktree → main repo
    ]
    for c in candidates:
        # Check for a Python package (has __init__.py or known subdir)
        pkg_dir = c / name if (c / name).is_dir() else c
        if (pkg_dir / "__init__.py").exists():
            return c
    return ROOT / name  # fallback

DEEPMEMORY_PKG = _find_pkg("deepmemory")
FLASHMEMORY_PKG = _find_pkg("pip-package")

sys.path.insert(0, str(DEEPMEMORY_PKG))
sys.path.insert(0, str(FLASHMEMORY_PKG))

PASS = "\033[92mPASS\033[0m"
FAIL = "\033[91mFAIL\033[0m"
SKIP = "\033[93mSKIP\033[0m"

results = []


def check(name: str, condition: bool, detail: str = ""):
    status = PASS if condition else FAIL
    results.append((name, condition))
    msg = f"  [{status}] {name}"
    if detail and not condition:
        msg += f" — {detail}"
    print(msg)
    return condition


# ---------------------------------------------------------------------------
# Test 1: DeepMemory MCP Server
# ---------------------------------------------------------------------------

print("=" * 60)
print("Test Suite 1: DeepMemory MCP Server")
print("=" * 60)

store_file = tempfile.mktemp(suffix=".jsonl")
os.environ["DEEPMEMORY_STORE"] = store_file

try:
    from deepmemory.mcp_server import create_mcp_server as create_dm_server

    dm_server = create_dm_server()
    dm_tools = dm_server._tool_manager._tools

    check("DM: server created", dm_server is not None)
    check("DM: 7 tools registered", len(dm_tools) == 7, f"got {len(dm_tools)}")

    expected_dm = {
        "deepmemory_ingest", "deepmemory_recall", "deepmemory_promote",
        "deepmemory_revise", "deepmemory_delete", "deepmemory_evolve",
        "deepmemory_events",
    }
    check("DM: tool names correct", set(dm_tools.keys()) == expected_dm)

    # --- Ingest ---
    result = json.loads(dm_tools["deepmemory_ingest"].fn(
        summary="user prefers dark mode UI",
        object_id="user:preference:theme",
        object_type="user_preference",
        anchor_kind="config",
        anchor_locator="settings.json",
        template="task",
        actor_ref="user:alice",
        source_ref="session-001-turn-001",
        context="{}",
        store_path=store_file,
    ))
    mem_id = result.get("memory_id", "")
    check("DM: ingest returns memory_id", mem_id.startswith("mu_"))
    check("DM: ingest claim correct", "dark mode" in result.get("interpretant", {}).get("operational_claim", ""))

    # --- Recall ---
    recalled = json.loads(dm_tools["deepmemory_recall"].fn(
        object_id="user:preference:theme",
        store_path=store_file,
    ))
    check("DM: recall returns 1 memory", len(recalled) == 1)
    check("DM: recall memory_id matches", recalled[0].get("memory_id") == mem_id)

    # --- Promote ---
    promoted = json.loads(dm_tools["deepmemory_promote"].fn(
        memory_id=mem_id,
        status="provisional_consensus",
        store_path=store_file,
    ))
    check("DM: promote status updated", promoted.get("social_status") == "provisional_consensus")

    # --- Revise ---
    revised = json.loads(dm_tools["deepmemory_revise"].fn(
        memory_id=mem_id,
        operational_claim="user strongly prefers dark mode with OLED-friendly colors",
        confidence=0.92,
        context_patch="",
        store_path=store_file,
    ))
    check("DM: revise claim updated", "OLED" in revised.get("interpretant", {}).get("operational_claim", ""))
    check("DM: revise confidence updated", revised.get("interpretant", {}).get("confidence") == 0.92)

    # --- Evolve ---
    report = json.loads(dm_tools["deepmemory_evolve"].fn(store_path=store_file))
    check("DM: evolve scanned ≥ 1", report.get("scanned_count", 0) >= 1)

    # --- Events ---
    events = json.loads(dm_tools["deepmemory_events"].fn(limit=20, store_path=store_file))
    event_types = [e.get("event_type") for e in events]
    check("DM: events include ingested", "memory.ingested" in event_types)
    check("DM: events include promoted", "memory.promoted" in event_types)

    # --- Delete ---
    deleted = json.loads(dm_tools["deepmemory_delete"].fn(
        memory_id=mem_id,
        reason="test_cleanup",
        store_path=store_file,
    ))
    check("DM: delete returns True", deleted.get("deleted") is True)

    # Verify empty after delete
    post_delete = json.loads(dm_tools["deepmemory_recall"].fn(
        object_id="user:preference:theme",
        store_path=store_file,
    ))
    check("DM: recall empty after delete", len(post_delete) == 0)

except Exception as e:
    check(f"DM: EXCEPTION — {e}", False)

finally:
    for f in [store_file, store_file.replace(".jsonl", "_events.jsonl")]:
        if os.path.exists(f):
            os.unlink(f)


# ---------------------------------------------------------------------------
# Test 2: FlashMemory MCP Server
# ---------------------------------------------------------------------------

print()
print("=" * 60)
print("Test Suite 2: FlashMemory MCP Server")
print("=" * 60)

try:
    from flashmemory.mcp_server import create_mcp_server as create_fm_server

    fm_server = create_fm_server()
    fm_tools = fm_server._tool_manager._tools

    check("FM: server created", fm_server is not None)
    check("FM: 3 tools registered", len(fm_tools) == 3, f"got {len(fm_tools)}")

    expected_fm = {"flashmemory_search", "flashmemory_index", "flashmemory_info"}
    check("FM: tool names correct", set(fm_tools.keys()) == expected_fm)

    # Test without project_dir → should return error gracefully
    result = json.loads(fm_tools["flashmemory_search"].fn(
        query="test",
        project_dir="",
        top_k=5,
        language="",
        search_type="functions",
    ))
    check("FM: search without project_dir returns error", "error" in result)

except ImportError as e:
    check(f"FM: import (expected if flashmemory not installed) — {e}", False)
except Exception as e:
    check(f"FM: EXCEPTION — {e}", False)


# ---------------------------------------------------------------------------
# Test 3: Cross-Server Workflow Simulation
# ---------------------------------------------------------------------------

print()
print("=" * 60)
print("Test Suite 3: Cross-Server Workflow (Symbolism → Memory → Evolution)")
print("=" * 60)

store_file2 = tempfile.mktemp(suffix=".jsonl")

try:
    dm_server2 = create_dm_server()
    tools = dm_server2._tool_manager._tools

    # Simulate symbolism extraction: 3 observations from a conversation
    observations = [
        ("user prefers Python for scripting tasks", "user:preference:language", "user_preference", "runtime"),
        ("project uses FastAPI for REST endpoints", "project:tech:framework", "decision", "app/main.py"),
        ("team agreed on 90% test coverage target", "team:policy:testing", "decision", "pyproject.toml"),
    ]

    ingested_ids = []
    for summary, obj_id, obj_type, locator in observations:
        result = json.loads(tools["deepmemory_ingest"].fn(
            summary=summary,
            object_id=obj_id,
            object_type=obj_type,
            anchor_locator=locator,
            template="task",
            actor_ref="symbolism-extractor",
            source_ref="session-workflow-test",
            context="{}",
            store_path=store_file2,
        ))
        ingested_ids.append(result["memory_id"])

    check("Workflow: 3 memories ingested", len(ingested_ids) == 3)

    # Simulate memory-augmented recall
    all_recalled = []
    for _, obj_id, _, _ in observations:
        recalled = json.loads(tools["deepmemory_recall"].fn(
            object_id=obj_id,
            store_path=store_file2,
        ))
        all_recalled.extend(recalled)

    check("Workflow: all 3 memories recalled", len(all_recalled) == 3)

    # Simulate user feedback: user confirms one, corrects another
    tools["deepmemory_promote"].fn(
        memory_id=ingested_ids[0],
        status="provisional_consensus",
        store_path=store_file2,
    )
    tools["deepmemory_revise"].fn(
        memory_id=ingested_ids[1],
        operational_claim="project uses FastAPI with Pydantic v2 models",
        confidence=0.88,
        context_patch="",
        store_path=store_file2,
    )

    # Verify updates
    recalled = json.loads(tools["deepmemory_recall"].fn(
        object_id="user:preference:language",
        store_path=store_file2,
    ))
    check("Workflow: promoted memory has correct status",
          recalled[0].get("social_status") == "provisional_consensus")

    recalled2 = json.loads(tools["deepmemory_recall"].fn(
        object_id="project:tech:framework",
        store_path=store_file2,
    ))
    check("Workflow: revised memory has updated claim",
          "Pydantic v2" in recalled2[0].get("interpretant", {}).get("operational_claim", ""))

    # Run evolution
    report = json.loads(tools["deepmemory_evolve"].fn(store_path=store_file2))
    check("Workflow: evolution scanned all", report.get("scanned_count") == 3)

    # Check events tell the full story
    events = json.loads(tools["deepmemory_events"].fn(limit=50, store_path=store_file2))
    event_types = set(e.get("event_type") for e in events)
    check("Workflow: events cover full lifecycle",
          {"memory.ingested", "memory.recalled", "memory.promoted", "memory.revised"}.issubset(event_types))

except Exception as e:
    check(f"Workflow: EXCEPTION — {e}", False)

finally:
    for f in [store_file2, store_file2.replace(".jsonl", "_events.jsonl")]:
        if os.path.exists(f):
            os.unlink(f)


# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------

print()
print("=" * 60)
total = len(results)
passed = sum(1 for _, ok in results if ok)
failed = total - passed
print(f"Results: {passed}/{total} passed, {failed} failed")

if failed > 0:
    print()
    print("Failures:")
    for name, ok in results:
        if not ok:
            print(f"  ✗ {name}")

print("=" * 60)
sys.exit(0 if failed == 0 else 1)
