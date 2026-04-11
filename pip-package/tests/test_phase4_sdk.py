"""
Phase 4 单元测试 - Python SDK & MCP Integration

测试内容:
- FlashMemoryClient 初始化和生命周期
- FlashMemoryClient search/embed/index API
- FlashMemoryClient context manager
- MCP tool definitions
- MCP tool call handler
"""

import unittest
from unittest.mock import MagicMock, patch
import sys
import os

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))


class MockZvecModule:
    """Mock zvec module for SDK tests."""

    __version__ = "0.4.0-mock"

    class DataType:
        STRING = "string"
        INT32 = "int32"
        DOUBLE = "double"
        VECTOR_FP32 = "vector_fp32"
        SPARSE_VECTOR_FP32 = "sparse_vector_fp32"

    class MetricType:
        COSINE = "cosine"
        L2 = "l2"
        IP = "ip"

    class FieldSchema:
        def __init__(self, **kwargs): pass

    class VectorSchema:
        def __init__(self, **kwargs):
            self.name = kwargs.get("name")

    class CollectionSchema:
        def __init__(self, **kwargs): pass

    class HnswIndexParam:
        def __init__(self, **kwargs): pass

    class InvertIndexParam: pass
    class SparseIndexParam: pass

    class Doc:
        def __init__(self, **kwargs):
            self.id = kwargs.get("id")
            self.vectors = kwargs.get("vectors", {})
            self.fields = kwargs.get("fields", {})
            self.score = 0.95

    class VectorQuery:
        def __init__(self, *a, **kw): pass

    @staticmethod
    def create_and_open(**kwargs):
        coll = MagicMock()
        coll.stats = {"doc_count": 0}
        return coll

    @staticmethod
    def open(**kwargs):
        coll = MagicMock()
        coll.stats = {"doc_count": 10}
        return coll


# ============================================================
# FlashMemoryClient Tests
# ============================================================


class TestClientInit(unittest.TestCase):
    """Test FlashMemoryClient initialization."""

    def setUp(self):
        sys.modules["zvec"] = MockZvecModule()

    def tearDown(self):
        if "zvec" in sys.modules:
            del sys.modules["zvec"]

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_basic_init(self, mock_makedirs, mock_exists):
        """Test client creation without initialization."""
        from flashmemory.client import FlashMemoryClient
        client = FlashMemoryClient(project_dir="/tmp/test_project")
        self.assertFalse(client._initialized)
        self.assertEqual(client.engine_type, "zvec")

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_initialize(self, mock_makedirs, mock_exists):
        """Test client full initialization."""
        from flashmemory.client import FlashMemoryClient
        client = FlashMemoryClient(project_dir="/tmp/test_project")
        client.initialize()
        self.assertTrue(client._initialized)
        self.assertIsNotNone(client._engine)
        self.assertIsNotNone(client._embedding)
        self.assertIsNotNone(client._pipeline)

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_double_init_skips(self, mock_makedirs, mock_exists):
        """Test double initialization is safely skipped."""
        from flashmemory.client import FlashMemoryClient
        client = FlashMemoryClient(project_dir="/tmp/test_project")
        client.initialize()
        # Second call should skip
        client.initialize()
        self.assertTrue(client._initialized)

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_close(self, mock_makedirs, mock_exists):
        """Test client close."""
        from flashmemory.client import FlashMemoryClient
        client = FlashMemoryClient(project_dir="/tmp/test_project")
        client.initialize()
        client.close()
        self.assertFalse(client._initialized)

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_context_manager(self, mock_makedirs, mock_exists):
        """Test client as context manager."""
        from flashmemory.client import FlashMemoryClient
        with FlashMemoryClient(project_dir="/tmp/test_project") as client:
            self.assertTrue(client._initialized)
        self.assertFalse(client._initialized)

    def test_unsupported_engine(self):
        """Test unsupported engine type raises error."""
        from flashmemory.client import FlashMemoryClient
        client = FlashMemoryClient(project_dir="/tmp/test", engine_type="unknown")
        with self.assertRaises(ValueError):
            client.initialize()


class TestClientSearch(unittest.TestCase):
    """Test FlashMemoryClient search operations."""

    def setUp(self):
        sys.modules["zvec"] = MockZvecModule()

    def tearDown(self):
        if "zvec" in sys.modules:
            del sys.modules["zvec"]

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_search_auto_initializes(self, mock_makedirs, mock_exists):
        """Test search auto-initializes client."""
        from flashmemory.client import FlashMemoryClient
        client = FlashMemoryClient(project_dir="/tmp/test_project")
        # Mock the engine's search to return results
        mock_doc = MagicMock()
        mock_doc.id = "func_1"
        mock_doc.score = 0.92
        mock_doc.fields = {"func_name": "UploadFile"}
        
        # The search will auto-initialize, then use the pipeline
        client.initialize()
        client._engine.search_functions = MagicMock(return_value=[mock_doc])
        
        results = client.search("file upload", top_k=5)
        self.assertIsInstance(results, list)

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_search_functions_convenience(self, mock_makedirs, mock_exists):
        """Test search_functions convenience method."""
        from flashmemory.client import FlashMemoryClient
        client = FlashMemoryClient(project_dir="/tmp/test_project")
        client.initialize()
        client._engine.search_functions = MagicMock(return_value=[])

        results = client.search_functions("test", top_k=3)
        self.assertIsInstance(results, list)

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_search_modules_convenience(self, mock_makedirs, mock_exists):
        """Test search_modules convenience method."""
        from flashmemory.client import FlashMemoryClient
        client = FlashMemoryClient(project_dir="/tmp/test_project")
        client.initialize()
        client._engine.search_modules = MagicMock(return_value=[])

        results = client.search_modules("auth module", top_k=3)
        self.assertIsInstance(results, list)


class TestClientEmbed(unittest.TestCase):
    """Test FlashMemoryClient embedding operations."""

    def setUp(self):
        sys.modules["zvec"] = MockZvecModule()

    def tearDown(self):
        if "zvec" in sys.modules:
            del sys.modules["zvec"]

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_embed_single(self, mock_makedirs, mock_exists):
        """Test single text embedding."""
        from flashmemory.client import FlashMemoryClient
        client = FlashMemoryClient(project_dir="/tmp/test", dimension=64)
        client.initialize()

        result = client.embed("file upload handler")
        self.assertIn("dense", result)
        self.assertEqual(len(result["dense"]), 64)
        self.assertEqual(result["dimension"], 64)

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_embed_batch(self, mock_makedirs, mock_exists):
        """Test batch embedding."""
        from flashmemory.client import FlashMemoryClient
        client = FlashMemoryClient(project_dir="/tmp/test", dimension=32)
        client.initialize()

        result = client.embed_batch(["hello", "world"])
        self.assertIn("dense_batch", result)
        self.assertEqual(len(result["dense_batch"]), 2)
        self.assertEqual(result["count"], 2)


class TestClientIndex(unittest.TestCase):
    """Test FlashMemoryClient index operations."""

    def setUp(self):
        sys.modules["zvec"] = MockZvecModule()

    def tearDown(self):
        if "zvec" in sys.modules:
            del sys.modules["zvec"]

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_add_function(self, mock_makedirs, mock_exists):
        """Test adding a function to index."""
        from flashmemory.client import FlashMemoryClient
        client = FlashMemoryClient(project_dir="/tmp/test", dimension=64)
        client.initialize()

        client.add_function(
            func_id="func_1",
            text="Handle file upload and save to disk",
            metadata={"func_name": "UploadFile", "language": "go"},
        )
        client._engine.func_collection.upsert.assert_called()

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_get_info(self, mock_makedirs, mock_exists):
        """Test client info."""
        from flashmemory.client import FlashMemoryClient
        client = FlashMemoryClient(project_dir="/tmp/test")
        client.initialize()

        info = client.get_info()
        self.assertIn("project_dir", info)
        self.assertIn("engine_type", info)
        self.assertTrue(info["initialized"])
        self.assertIn("engine_stats", info)
        self.assertIn("embedding", info)

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_optimize(self, mock_makedirs, mock_exists):
        """Test index optimization."""
        from flashmemory.client import FlashMemoryClient
        client = FlashMemoryClient(project_dir="/tmp/test")
        client.initialize()
        # Should not raise
        client.optimize()


# ============================================================
# MCP Tool Tests
# ============================================================


class TestMCPTools(unittest.TestCase):
    """Test MCP tool definitions."""

    def test_get_mcp_tools(self):
        """Test MCP tool definitions are valid."""
        from flashmemory.client import get_mcp_tools
        tools = get_mcp_tools()
        self.assertIsInstance(tools, list)
        self.assertEqual(len(tools), 3)

        # Check tool names
        names = {t["name"] for t in tools}
        self.assertIn("flashmemory_search", names)
        self.assertIn("flashmemory_index", names)
        self.assertIn("flashmemory_info", names)

        # Check each tool has required MCP fields
        for tool in tools:
            self.assertIn("name", tool)
            self.assertIn("description", tool)
            self.assertIn("inputSchema", tool)
            schema = tool["inputSchema"]
            self.assertEqual(schema["type"], "object")
            self.assertIn("properties", schema)
            self.assertIn("required", schema)

    def test_search_tool_schema(self):
        """Test search tool has proper schema."""
        from flashmemory.client import get_mcp_tools
        tools = get_mcp_tools()
        search_tool = next(t for t in tools if t["name"] == "flashmemory_search")
        props = search_tool["inputSchema"]["properties"]
        self.assertIn("query", props)
        self.assertIn("project_dir", props)
        self.assertIn("top_k", props)
        self.assertIn("language", props)
        self.assertIn("search_type", props)
        required = search_tool["inputSchema"]["required"]
        self.assertIn("query", required)
        self.assertIn("project_dir", required)


class TestMCPHandler(unittest.TestCase):
    """Test MCP tool call handler."""

    def setUp(self):
        sys.modules["zvec"] = MockZvecModule()

    def tearDown(self):
        if "zvec" in sys.modules:
            del sys.modules["zvec"]

    def test_missing_project_dir(self):
        """Test handler with missing project_dir."""
        from flashmemory.client import handle_mcp_tool_call
        result = handle_mcp_tool_call("flashmemory_search", {})
        self.assertIn("error", result)

    def test_unknown_tool(self):
        """Test handler with unknown tool name."""
        from flashmemory.client import handle_mcp_tool_call
        result = handle_mcp_tool_call("unknown_tool", {"project_dir": "/tmp/test"})
        self.assertIn("error", result)

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_info_tool(self, mock_makedirs, mock_exists):
        """Test info tool call."""
        from flashmemory.client import handle_mcp_tool_call
        cache = {}
        result = handle_mcp_tool_call(
            "flashmemory_info",
            {"project_dir": "/tmp/test"},
            client_cache=cache,
        )
        self.assertIn("project_dir", result)
        self.assertTrue(result["initialized"])
        # Client should be cached
        self.assertIn("/tmp/test", cache)

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_search_tool(self, mock_makedirs, mock_exists):
        """Test search tool call."""
        from flashmemory.client import handle_mcp_tool_call
        cache = {}
        result = handle_mcp_tool_call(
            "flashmemory_search",
            {"project_dir": "/tmp/test", "query": "file upload", "top_k": 5},
            client_cache=cache,
        )
        self.assertIn("results", result)
        self.assertIn("count", result)

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_index_tool(self, mock_makedirs, mock_exists):
        """Test index tool call."""
        from flashmemory.client import handle_mcp_tool_call
        cache = {}
        result = handle_mcp_tool_call(
            "flashmemory_index",
            {
                "project_dir": "/tmp/test",
                "func_id": "func_1",
                "text": "Upload file handler",
                "metadata": {"func_name": "Upload"},
            },
            client_cache=cache,
        )
        self.assertEqual(result["status"], "indexed")
        self.assertEqual(result["func_id"], "func_1")

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_client_cache_reuse(self, mock_makedirs, mock_exists):
        """Test client cache is reused across calls."""
        from flashmemory.client import handle_mcp_tool_call
        cache = {}

        # First call creates client
        handle_mcp_tool_call("flashmemory_info", {"project_dir": "/tmp/test"}, cache)
        self.assertEqual(len(cache), 1)

        # Second call reuses client
        handle_mcp_tool_call("flashmemory_info", {"project_dir": "/tmp/test"}, cache)
        self.assertEqual(len(cache), 1)  # Still just 1 client


if __name__ == "__main__":
    unittest.main(verbosity=2)
