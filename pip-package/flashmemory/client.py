"""
FlashMemory Python SDK Client

High-level API for FlashMemory integration.
Provides a unified interface for:
  - Vector engine management (Zvec/FAISS)
  - Embedding generation (dense/sparse)
  - Semantic search with hybrid fusion
  - MCP tool integration

Usage:
    from flashmemory import FlashMemoryClient

    client = FlashMemoryClient(project_dir="/path/to/project")
    results = client.search("file upload handler", top_k=10)
    for r in results:
        print(f"{r['name']} ({r['score']:.2f}): {r['description']}")
"""

import os
import logging
from typing import List, Dict, Optional, Any

logger = logging.getLogger("flashmemory.client")


class FlashMemoryClient:
    """High-level FlashMemory client for Python SDK.

    Wraps ZvecEngine, EmbeddingProvider, and SearchPipeline
    into a single developer-friendly interface.
    """

    def __init__(
        self,
        project_dir: str,
        engine_type: str = "zvec",
        dimension: int = 384,
        dense_provider: str = "local",
        sparse_provider: str = "none",
        enable_reranker: bool = False,
        collection_subdir: str = ".gitgo/zvec_collections",
        **kwargs,
    ):
        """Initialize FlashMemory client.

        Args:
            project_dir: Root directory of the project to index/search
            engine_type: "zvec" (default) or "faiss"
            dimension: Embedding dimension (default 384 for all-MiniLM-L6-v2)
            dense_provider: Dense embedding source ("local", "openai", "qwen", "jina")
            sparse_provider: Sparse embedding source ("bm25_zh", "bm25_en", "splade", "none")
            enable_reranker: Whether to enable cross-encoder reranking
            collection_subdir: Subdirectory for Zvec collections (relative to project_dir)
            **kwargs: Additional config (api_key, model_name, etc.)
        """
        self.project_dir = os.path.abspath(project_dir)
        self.engine_type = engine_type
        self._engine = None
        self._embedding = None
        self._pipeline = None
        self._initialized = False

        self._config = {
            "dimension": dimension,
            "dense_provider": dense_provider,
            "sparse_provider": sparse_provider,
            "enable_reranker": enable_reranker,
            "collection_path": os.path.join(self.project_dir, collection_subdir),
            **kwargs,
        }

        logger.info(
            "FlashMemoryClient created: project=%s, engine=%s, dense=%s, sparse=%s",
            self.project_dir, engine_type, dense_provider, sparse_provider,
        )

    def initialize(self, force_new: bool = False):
        """Initialize engine, embedding provider, and search pipeline.

        Call this before search operations. Safe to call multiple times.

        Args:
            force_new: If True, recreate collections from scratch
        """
        if self._initialized and not force_new:
            logger.info("Client already initialized, skipping")
            return self

        # Step 1: Initialize Zvec engine
        if self.engine_type == "zvec":
            from flashmemory.zvec_engine import ZvecEngine
            collection_path = self._config["collection_path"]
            os.makedirs(os.path.dirname(collection_path), exist_ok=True)
            self._engine = ZvecEngine(collection_path, dimension=self._config["dimension"])
            self._engine.init_func_collection(force_new=force_new)
            self._engine.init_module_collection(force_new=force_new)
            logger.info("Zvec engine initialized at %s", collection_path)
        else:
            raise ValueError(f"Unsupported engine type: {self.engine_type}")

        # Step 2: Initialize embedding provider
        from flashmemory.embedding_provider import EmbeddingProvider
        self._embedding = EmbeddingProvider(config=self._config)
        logger.info("Embedding provider initialized: %s", self._embedding.get_info())

        # Step 3: Initialize search pipeline
        from flashmemory.search_pipeline import SearchPipeline
        self._pipeline = SearchPipeline(
            engine=self._engine,
            embedding_provider=self._embedding,
            config={
                "enable_reranker": self._config.get("enable_reranker", False),
                "recall_multiplier": self._config.get("recall_multiplier", 5),
                "use_rrf": True,
            },
        )

        self._initialized = True
        logger.info("FlashMemoryClient fully initialized")
        return self

    def _ensure_initialized(self):
        """Ensure client is initialized before operations."""
        if not self._initialized:
            self.initialize()

    # --- Search API ---

    def search(
        self,
        query: str,
        top_k: int = 10,
        language: str = None,
        package: str = None,
        search_type: str = "functions",
    ) -> List[Dict[str, Any]]:
        """Search for code functions/modules by natural language query.

        Args:
            query: Natural language search query
            top_k: Number of results to return
            language: Filter by programming language
            package: Filter by package name
            search_type: "functions" or "modules"

        Returns:
            List of result dicts with keys: id, score, fields (func_name, etc.)
        """
        self._ensure_initialized()

        if language or package:
            results = self._pipeline.search_with_context(
                query=query,
                top_k=top_k,
                language=language,
                package=package,
            )
        else:
            results = self._pipeline.search(
                query=query,
                top_k=top_k,
                search_type=search_type,
            )

        return [r.to_dict() for r in results]

    def search_functions(self, query: str, top_k: int = 10, **filters) -> List[Dict]:
        """Convenience: search functions only."""
        return self.search(query, top_k=top_k, search_type="functions", **filters)

    def search_modules(self, query: str, top_k: int = 10, **filters) -> List[Dict]:
        """Convenience: search modules only."""
        return self.search(query, top_k=top_k, search_type="modules", **filters)

    # --- Embedding API ---

    def embed(self, text: str) -> Dict[str, Any]:
        """Generate embedding for text.

        Returns:
            Dict with keys: dense (list), sparse (dict or None), dimension (int)
        """
        self._ensure_initialized()
        return {
            "dense": self._embedding.embed_dense(text),
            "sparse": self._embedding.embed_sparse(text),
            "dimension": self._embedding.dimension,
        }

    def embed_batch(self, texts: List[str]) -> Dict[str, Any]:
        """Generate embeddings for batch of texts."""
        self._ensure_initialized()
        return {
            "dense_batch": self._embedding.embed_dense_batch(texts),
            "dimension": self._embedding.dimension,
            "count": len(texts),
        }

    # --- Index API ---

    def add_function(
        self,
        func_id: str,
        text: str,
        metadata: Dict[str, Any],
    ):
        """Add a function to the search index.

        Automatically generates embedding from text description.

        Args:
            func_id: Unique function identifier (e.g. "func_42")
            text: Text to embed (description, signature, etc.)
            metadata: Scalar fields (func_name, package, file_path, etc.)
        """
        self._ensure_initialized()
        dense_vec = self._embedding.embed_dense(text)
        sparse_vec = self._embedding.embed_sparse(text)
        self._engine.upsert_function(func_id, dense_vec, metadata, sparse_embedding=sparse_vec)

    def add_functions_batch(self, items: List[Dict[str, Any]]):
        """Batch add functions.

        Args:
            items: List of dicts with keys: func_id, text, metadata
        """
        self._ensure_initialized()
        texts = [item["text"] for item in items]
        dense_vecs = self._embedding.embed_dense_batch(texts)

        batch = []
        for i, item in enumerate(items):
            sparse_vec = self._embedding.embed_sparse(item["text"])
            entry = (item["func_id"], dense_vecs[i], item.get("metadata", {}))
            if sparse_vec:
                entry = (item["func_id"], dense_vecs[i], item.get("metadata", {}), sparse_vec)
            batch.append(entry)

        self._engine.upsert_functions_batch(batch)

    def delete_by_file(self, file_path: str):
        """Delete all function vectors for a file (incremental update)."""
        self._ensure_initialized()
        self._engine.delete_by_file(file_path)

    def optimize(self):
        """Optimize search index for better performance."""
        self._ensure_initialized()
        self._engine.optimize()

    # --- Info API ---

    def get_info(self) -> Dict[str, Any]:
        """Get client status and diagnostics."""
        info = {
            "project_dir": self.project_dir,
            "engine_type": self.engine_type,
            "initialized": self._initialized,
            "config": {k: v for k, v in self._config.items() if k != "api_key"},
        }
        if self._initialized:
            info["engine_stats"] = self._engine.get_stats()
            info["embedding"] = self._embedding.get_info()
            info["pipeline"] = self._pipeline.get_pipeline_info()
        return info

    def close(self):
        """Close all resources."""
        if self._engine:
            self._engine.close()
            self._engine = None
        self._initialized = False
        logger.info("FlashMemoryClient closed")

    def __enter__(self):
        self.initialize()
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        self.close()
        return False


# --- MCP Tool Definitions ---

def get_mcp_tools() -> List[Dict[str, Any]]:
    """Return MCP-compatible tool definitions for FlashMemory.

    These tools can be registered with any MCP server to provide
    AI agents with code search capabilities.

    Returns:
        List of MCP tool definition dicts
    """
    return [
        {
            "name": "flashmemory_search",
            "description": (
                "Search for code functions and modules using natural language queries. "
                "Uses semantic vector search with optional keyword matching. "
                "Returns function names, descriptions, file paths, and relevance scores."
            ),
            "inputSchema": {
                "type": "object",
                "properties": {
                    "query": {
                        "type": "string",
                        "description": "Natural language search query describing what you're looking for",
                    },
                    "project_dir": {
                        "type": "string",
                        "description": "Absolute path to the project root directory",
                    },
                    "top_k": {
                        "type": "integer",
                        "description": "Number of results to return (default: 10)",
                        "default": 10,
                    },
                    "language": {
                        "type": "string",
                        "description": "Filter by programming language (e.g. 'go', 'python', 'javascript')",
                    },
                    "search_type": {
                        "type": "string",
                        "enum": ["functions", "modules"],
                        "description": "Search code functions or module descriptions",
                        "default": "functions",
                    },
                },
                "required": ["query", "project_dir"],
            },
        },
        {
            "name": "flashmemory_index",
            "description": (
                "Add or update a code function in the FlashMemory search index. "
                "Use this after parsing new code to make it searchable."
            ),
            "inputSchema": {
                "type": "object",
                "properties": {
                    "project_dir": {
                        "type": "string",
                        "description": "Absolute path to the project root directory",
                    },
                    "func_id": {
                        "type": "string",
                        "description": "Unique function identifier (e.g. 'func_42')",
                    },
                    "text": {
                        "type": "string",
                        "description": "Text to index (function description, signature, etc.)",
                    },
                    "metadata": {
                        "type": "object",
                        "description": "Scalar fields for filtering (func_name, package, file_path, language)",
                    },
                },
                "required": ["project_dir", "func_id", "text"],
            },
        },
        {
            "name": "flashmemory_info",
            "description": "Get FlashMemory client status, engine stats, and diagnostics.",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "project_dir": {
                        "type": "string",
                        "description": "Absolute path to the project root directory",
                    },
                },
                "required": ["project_dir"],
            },
        },
    ]


def handle_mcp_tool_call(
    tool_name: str,
    arguments: Dict[str, Any],
    client_cache: Dict[str, "FlashMemoryClient"] = None,
) -> Dict[str, Any]:
    """Handle an MCP tool call.

    This function is designed to be called from an MCP server implementation.
    It manages a cache of FlashMemoryClient instances per project_dir.

    Args:
        tool_name: Name of the MCP tool to execute
        arguments: Tool call arguments
        client_cache: Optional dict to cache clients by project_dir

    Returns:
        Dict with tool execution results
    """
    if client_cache is None:
        client_cache = {}

    project_dir = arguments.get("project_dir", "")
    if not project_dir:
        return {"error": "project_dir is required"}

    # Get or create client for this project
    if project_dir not in client_cache:
        try:
            client = FlashMemoryClient(project_dir=project_dir)
            client.initialize()
            client_cache[project_dir] = client
        except Exception as e:
            logger.error("Failed to initialize client for %s: %s", project_dir, e)
            return {"error": f"Failed to initialize: {e}"}

    client = client_cache[project_dir]

    try:
        if tool_name == "flashmemory_search":
            results = client.search(
                query=arguments["query"],
                top_k=arguments.get("top_k", 10),
                language=arguments.get("language"),
                search_type=arguments.get("search_type", "functions"),
            )
            return {"results": results, "count": len(results)}

        elif tool_name == "flashmemory_index":
            client.add_function(
                func_id=arguments["func_id"],
                text=arguments["text"],
                metadata=arguments.get("metadata", {}),
            )
            return {"status": "indexed", "func_id": arguments["func_id"]}

        elif tool_name == "flashmemory_info":
            return client.get_info()

        else:
            return {"error": f"Unknown tool: {tool_name}"}

    except Exception as e:
        logger.error("MCP tool call failed: %s(%s) -> %s", tool_name, arguments, e)
        return {"error": str(e)}
