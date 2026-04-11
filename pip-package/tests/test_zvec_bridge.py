"""
ZvecBridge 单元测试

测试 stdin/stdout JSON-line 协议的正确性，
包括所有 action 的请求/响应逻辑。
"""

import unittest
from unittest.mock import MagicMock, patch
import sys
import os
import json
import io

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))


class MockZvecModule:
    """Minimal mock of zvec for bridge tests"""

    __version__ = "0.1.1-mock"

    class DataType:
        STRING = "string"
        INT32 = "int32"
        DOUBLE = "double"
        VECTOR_FP32 = "vector_fp32"
        SPARSE_VECTOR_FP32 = "sparse_vector_fp32"

    class MetricType:
        COSINE = "cosine"

    class FieldSchema:
        def __init__(self, **kwargs):
            pass

    class VectorSchema:
        def __init__(self, **kwargs):
            pass

    class CollectionSchema:
        def __init__(self, **kwargs):
            pass

    class HnswIndexParam:
        def __init__(self, **kwargs):
            pass

    class InvertIndexParam:
        pass

    class SparseIndexParam:
        pass

    class Doc:
        def __init__(self, **kwargs):
            self.id = kwargs.get("id")
            self.score = 0.9
            self.fields = kwargs.get("fields", {})

    class VectorQuery:
        def __init__(self, *args, **kwargs):
            pass

    @staticmethod
    def create_and_open(**kwargs):
        coll = MagicMock()
        coll.stats = {"doc_count": 0}
        return coll

    @staticmethod
    def open(**kwargs):
        coll = MagicMock()
        coll.stats = {"doc_count": 5}
        return coll


class TestZvecBridge(unittest.TestCase):
    """Test ZvecBridge JSON-line protocol"""

    def setUp(self):
        # Inject mock zvec
        sys.modules["zvec"] = MockZvecModule()

        from flashmemory.zvec_bridge import ZvecBridge
        self.ZvecBridge = ZvecBridge

    def tearDown(self):
        if "zvec" in sys.modules:
            del sys.modules["zvec"]

    def _create_bridge_with_output(self):
        """Create a bridge with captured stdout"""
        bridge = self.ZvecBridge()
        self.output = io.StringIO()
        return bridge

    def _send_and_capture(self, bridge, action, params=None):
        """Send a request and capture the response"""
        old_stdout = sys.stdout
        sys.stdout = io.StringIO()

        handler = bridge.ACTION_HANDLERS.get(action)
        self.assertIsNotNone(handler, f"Handler for action '{action}' not found")
        handler(bridge, params or {})

        output = sys.stdout.getvalue()
        sys.stdout = old_stdout

        # Parse the JSON-line response
        lines = [l for l in output.strip().split("\n") if l.strip()]
        self.assertTrue(len(lines) > 0, f"No response for action '{action}'")

        resp = json.loads(lines[-1])
        return resp

    def test_ping_action(self):
        """Test ping health check"""
        bridge = self.ZvecBridge()
        resp = self._send_and_capture(bridge, "ping")

        self.assertEqual(resp["status"], "success")
        self.assertEqual(resp["message"], "pong")
        self.assertFalse(resp["data"]["engine_ready"])

    def test_init_action_missing_path(self):
        """Test init with missing collection_path returns error"""
        bridge = self.ZvecBridge()
        resp = self._send_and_capture(bridge, "init", {})

        self.assertEqual(resp["status"], "error")
        self.assertIn("collection_path", resp["message"])

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_init_action_success(self, mock_makedirs, mock_exists):
        """Test successful initialization"""
        bridge = self.ZvecBridge()
        resp = self._send_and_capture(bridge, "init", {
            "collection_path": "/tmp/test_zvec_coll",
            "dimension": 384,
            "force_new": False,
            "collection_type": "functions",
        })

        self.assertEqual(resp["status"], "success")
        self.assertIn("dimension", resp["data"])
        self.assertEqual(resp["data"]["dimension"], 384)

    def test_add_vector_without_init(self):
        """Test adding vector without initialization returns error"""
        bridge = self.ZvecBridge()
        resp = self._send_and_capture(bridge, "add_vector", {
            "func_id": "func_1",
            "vector": [0.1] * 384,
        })

        self.assertEqual(resp["status"], "error")
        self.assertIn("未初始化", resp["message"])

    def test_add_vector_missing_params(self):
        """Test adding vector with missing params returns error"""
        bridge = self.ZvecBridge()
        # Mock engine as initialized
        bridge.engine = MagicMock()
        bridge.engine.upsert_function = MagicMock()

        resp = self._send_and_capture(bridge, "add_vector", {})

        self.assertEqual(resp["status"], "error")

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_add_vector_success(self, mock_makedirs, mock_exists):
        """Test successful vector addition"""
        bridge = self.ZvecBridge()
        bridge.handle_init({
            "collection_path": "/tmp/test_coll",
            "dimension": 384,
            "collection_type": "functions",
        })

        # Redirect stdout for the second call
        resp = self._send_and_capture(bridge, "add_vector", {
            "func_id": "func_42",
            "vector": [0.5] * 384,
            "metadata": {"func_name": "testFunc"},
        })

        self.assertEqual(resp["status"], "success")

    def test_search_without_init(self):
        """Test search without initialization returns error"""
        bridge = self.ZvecBridge()
        resp = self._send_and_capture(bridge, "search", {
            "query": [0.1] * 384,
        })

        self.assertEqual(resp["status"], "error")

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_search_success(self, mock_makedirs, mock_exists):
        """Test successful search"""
        bridge = self.ZvecBridge()

        # Suppress init stdout
        old_stdout = sys.stdout
        sys.stdout = io.StringIO()
        bridge.handle_init({
            "collection_path": "/tmp/test_coll",
            "dimension": 384,
            "collection_type": "functions",
        })
        sys.stdout = old_stdout

        # Mock search results
        mock_doc = MagicMock()
        mock_doc.id = "func_1"
        mock_doc.score = 0.95
        mock_doc.fields = {"func_name": "hello"}
        bridge.engine.func_collection.query.return_value = [mock_doc]

        resp = self._send_and_capture(bridge, "search", {
            "query": [0.1] * 384,
            "top_k": 5,
            "collection_type": "functions",
        })

        self.assertEqual(resp["status"], "success")
        self.assertIn("results", resp["data"])

    def test_search_empty_query(self):
        """Test search with empty query vector returns error"""
        bridge = self.ZvecBridge()
        bridge.engine = MagicMock()

        resp = self._send_and_capture(bridge, "search", {
            "query": [],
        })

        self.assertEqual(resp["status"], "error")

    def test_delete_missing_params(self):
        """Test delete with missing params returns error"""
        bridge = self.ZvecBridge()
        bridge.engine = MagicMock()

        resp = self._send_and_capture(bridge, "delete", {})

        self.assertEqual(resp["status"], "error")

    def test_optimize_without_init(self):
        """Test optimize without init returns error"""
        bridge = self.ZvecBridge()
        resp = self._send_and_capture(bridge, "optimize")

        self.assertEqual(resp["status"], "error")

    def test_stats_without_init(self):
        """Test stats without init returns error"""
        bridge = self.ZvecBridge()
        resp = self._send_and_capture(bridge, "stats")

        self.assertEqual(resp["status"], "error")

    def test_close_without_init(self):
        """Test close without init succeeds gracefully"""
        bridge = self.ZvecBridge()
        resp = self._send_and_capture(bridge, "close")

        self.assertEqual(resp["status"], "success")

    def test_shutdown(self):
        """Test shutdown stops the bridge"""
        bridge = self.ZvecBridge()
        self.assertTrue(bridge.running)

        resp = self._send_and_capture(bridge, "shutdown")

        self.assertEqual(resp["status"], "success")
        self.assertFalse(bridge.running)

    def test_unknown_action(self):
        """Test main run loop handles unknown actions"""
        bridge = self.ZvecBridge()

        # Simulate stdin with unknown action
        old_stdin = sys.stdin
        old_stdout = sys.stdout

        input_data = json.dumps({"action": "nonexistent", "params": {}}) + "\n"
        input_data += json.dumps({"action": "shutdown", "params": {}}) + "\n"

        sys.stdin = io.StringIO(input_data)
        sys.stdout = io.StringIO()

        bridge.run()

        output = sys.stdout.getvalue()
        sys.stdin = old_stdin
        sys.stdout = old_stdout

        # Parse all responses
        lines = [l for l in output.strip().split("\n") if l.strip()]
        # First should be ready, then error for unknown, then shutdown success
        self.assertTrue(len(lines) >= 3)

        ready_resp = json.loads(lines[0])
        self.assertEqual(ready_resp["message"], "ready")

        error_resp = json.loads(lines[1])
        self.assertEqual(error_resp["status"], "error")
        self.assertIn("nonexistent", error_resp["message"])

    def test_invalid_json_input(self):
        """Test main run loop handles invalid JSON gracefully"""
        bridge = self.ZvecBridge()

        old_stdin = sys.stdin
        old_stdout = sys.stdout

        input_data = "this is not valid json\n"
        input_data += json.dumps({"action": "shutdown", "params": {}}) + "\n"

        sys.stdin = io.StringIO(input_data)
        sys.stdout = io.StringIO()

        bridge.run()

        output = sys.stdout.getvalue()
        sys.stdin = old_stdin
        sys.stdout = old_stdout

        lines = [l for l in output.strip().split("\n") if l.strip()]
        # ready + json error + shutdown
        self.assertTrue(len(lines) >= 3)

        error_resp = json.loads(lines[1])
        self.assertEqual(error_resp["status"], "error")
        self.assertIn("JSON", error_resp["message"])

    def test_ready_signal_on_start(self):
        """Test bridge sends ready signal immediately on start"""
        bridge = self.ZvecBridge()

        old_stdin = sys.stdin
        old_stdout = sys.stdout

        # Immediately shutdown
        input_data = json.dumps({"action": "shutdown", "params": {}}) + "\n"
        sys.stdin = io.StringIO(input_data)
        sys.stdout = io.StringIO()

        bridge.run()

        output = sys.stdout.getvalue()
        sys.stdin = old_stdin
        sys.stdout = old_stdout

        lines = [l for l in output.strip().split("\n") if l.strip()]
        self.assertTrue(len(lines) >= 1)

        ready_resp = json.loads(lines[0])
        self.assertEqual(ready_resp["status"], "success")
        self.assertEqual(ready_resp["message"], "ready")

    def test_batch_add_vectors(self):
        """Test batch adding vectors"""
        bridge = self.ZvecBridge()
        bridge.engine = MagicMock()
        bridge.engine.upsert_functions_batch = MagicMock()

        resp = self._send_and_capture(bridge, "add_vectors_batch", {
            "items": [
                {"func_id": "func_1", "vector": [0.1] * 384, "metadata": {}},
                {"func_id": "func_2", "vector": [0.2] * 384, "metadata": {}},
            ],
        })

        self.assertEqual(resp["status"], "success")
        self.assertEqual(resp["data"]["count"], 2)

    def test_batch_add_empty_items(self):
        """Test batch adding with empty items returns error"""
        bridge = self.ZvecBridge()
        bridge.engine = MagicMock()

        resp = self._send_and_capture(bridge, "add_vectors_batch", {
            "items": [],
        })

        self.assertEqual(resp["status"], "error")


if __name__ == "__main__":
    unittest.main(verbosity=2)
