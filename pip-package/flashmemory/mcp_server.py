"""
FlashMemory MCP Server — code search and indexing via standard MCP protocol.

Wraps the FlashMemory Python SDK (FlashMemoryClient, get_mcp_tools,
handle_mcp_tool_call) as a stdio MCP server for zero-coupling integration.

Start:
    python -m flashmemory.mcp_server

MCP client config:
    {
        "mcpServers": {
            "flashmemory": {
                "command": "python",
                "args": ["-m", "flashmemory.mcp_server"],
                "env": {"FM_DEFAULT_PROJECT": "/path/to/your/project"}
            }
        }
    }

If the Go HTTP service is already running (fm serve --port 5532), you can
also use the /tools/mcp endpoint directly instead of this stdio wrapper.
"""

from __future__ import annotations

import json
import logging
import os
import sys
from typing import Optional

logger = logging.getLogger("flashmemory.mcp_server")

_MCP_AVAILABLE = False
try:
    from mcp.server.fastmcp import FastMCP

    _MCP_AVAILABLE = True
except ImportError:
    FastMCP = None  # type: ignore[assignment,misc]

from flashmemory.client import FlashMemoryClient, handle_mcp_tool_call

# ---------------------------------------------------------------------------
# Client cache
# ---------------------------------------------------------------------------

_client_cache: dict[str, FlashMemoryClient] = {}


def _serialize(obj: object) -> str:
    return json.dumps(obj, ensure_ascii=False, indent=2, default=str)


# ---------------------------------------------------------------------------
# MCP Server
# ---------------------------------------------------------------------------


def create_mcp_server() -> FastMCP:
    """Create and return the FlashMemory MCP server with all tools."""

    mcp = FastMCP(
        "flashmemory",
        instructions=(
            "FlashMemory: cross-language code analysis and semantic search. "
            "Search code functions and modules by natural language, "
            "add items to the vector index, and get engine diagnostics. "
            "Supports Go, Python, JavaScript, Java, C++ with Zvec vector engine."
        ),
    )

    # ── Search ─────────────────────────────────────────────

    @mcp.tool()
    def flashmemory_search(
        query: str,
        project_dir: str = "",
        top_k: int = 10,
        language: str = "",
        search_type: str = "functions",
    ) -> str:
        """Search code functions and modules by natural language query.

        Uses semantic vector search with optional keyword matching (hybrid mode).
        Returns function names, descriptions, file paths, and relevance scores.

        Args:
            query: Natural language search query (e.g. "file upload handler")
            project_dir: Absolute path to the project root (default: $FM_DEFAULT_PROJECT)
            top_k: Number of results to return (default: 10)
            language: Filter by language (e.g. "go", "python", "javascript")
            search_type: "functions" (default) or "modules"
        """
        pdir = project_dir or os.environ.get("FM_DEFAULT_PROJECT", "")
        if not pdir:
            return _serialize({"error": "project_dir is required"})

        result = handle_mcp_tool_call(
            "flashmemory_search",
            {
                "query": query,
                "project_dir": pdir,
                "top_k": top_k,
                "language": language or None,
                "search_type": search_type,
            },
            client_cache=_client_cache,
        )
        return _serialize(result)

    # ── Index ──────────────────────────────────────────────

    @mcp.tool()
    def flashmemory_index(
        func_id: str,
        text: str,
        project_dir: str = "",
        metadata: str = "{}",
    ) -> str:
        """Add or update a code function in the search index.

        Use after parsing new code to make it searchable. Generates
        embeddings and stores them in the Zvec vector collection.

        Args:
            func_id: Unique function identifier (e.g. "func_42")
            text: Text to index — function description, signature, docstring
            project_dir: Absolute path to the project root (default: $FM_DEFAULT_PROJECT)
            metadata: JSON string with scalar fields for filtering (func_name, package, file_path, language)
        """
        pdir = project_dir or os.environ.get("FM_DEFAULT_PROJECT", "")
        if not pdir:
            return _serialize({"error": "project_dir is required"})

        meta = json.loads(metadata) if metadata else {}
        result = handle_mcp_tool_call(
            "flashmemory_index",
            {
                "project_dir": pdir,
                "func_id": func_id,
                "text": text,
                "metadata": meta,
            },
            client_cache=_client_cache,
        )
        return _serialize(result)

    # ── Info ───────────────────────────────────────────────

    @mcp.tool()
    def flashmemory_info(
        project_dir: str = "",
    ) -> str:
        """Get FlashMemory engine status and diagnostics.

        Returns engine type, vector dimension, collection stats,
        embedding provider info, and index health.

        Args:
            project_dir: Absolute path to the project root (default: $FM_DEFAULT_PROJECT)
        """
        pdir = project_dir or os.environ.get("FM_DEFAULT_PROJECT", "")
        if not pdir:
            return _serialize({"error": "project_dir is required"})

        result = handle_mcp_tool_call(
            "flashmemory_info",
            {"project_dir": pdir},
            client_cache=_client_cache,
        )
        return _serialize(result)

    return mcp


# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------


def main() -> None:
    if not _MCP_AVAILABLE:
        print(
            "flashmemory MCP server requires the 'mcp' package.\n"
            "Install with:  pip install 'mcp>=1.2.0,<2'",
            file=sys.stderr,
        )
        sys.exit(1)

    logging.basicConfig(
        level=logging.DEBUG if os.environ.get("FM_DEBUG") else logging.INFO,
        format="%(asctime)s %(name)s %(levelname)s %(message)s",
        stream=sys.stderr,
    )

    server = create_mcp_server()
    server.run()


if __name__ == "__main__":
    main()
