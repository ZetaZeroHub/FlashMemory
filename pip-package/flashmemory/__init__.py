"""
FlashMemory - Cross-language code analysis and semantic search system.

Supports Go, Python, JavaScript, Java, C++ code indexing with
LLM-powered analysis and vector-based semantic search.

Vector engine backends:
  - Zvec (recommended): In-process, high-performance vector database
  - FAISS (legacy): HTTP-based FAISS service

AI extensions:
  - EmbeddingProvider: Multi-source dense/sparse embedding
  - SearchPipeline: Two-stage Recall → Rerank pipeline

SDK:
  - FlashMemoryClient: High-level client API
  - MCP tools: AI agent integration (get_mcp_tools, handle_mcp_tool_call)
"""

__version__ = "0.4.0"

__all__ = [
    "FlashMemoryClient",
    "ZvecEngine",
    "EmbeddingProvider",
    "SearchPipeline",
    "get_mcp_tools",
    "handle_mcp_tool_call",
]


def __getattr__(name):
    """Lazy imports for optional components."""
    if name == "FlashMemoryClient":
        from flashmemory.client import FlashMemoryClient
        return FlashMemoryClient
    if name == "ZvecEngine":
        from flashmemory.zvec_engine import ZvecEngine
        return ZvecEngine
    if name == "EmbeddingProvider":
        from flashmemory.embedding_provider import EmbeddingProvider
        return EmbeddingProvider
    if name == "SearchPipeline":
        from flashmemory.search_pipeline import SearchPipeline
        return SearchPipeline
    if name == "get_mcp_tools":
        from flashmemory.client import get_mcp_tools
        return get_mcp_tools
    if name == "handle_mcp_tool_call":
        from flashmemory.client import handle_mcp_tool_call
        return handle_mcp_tool_call
    raise AttributeError(f"module 'flashmemory' has no attribute {name}")
