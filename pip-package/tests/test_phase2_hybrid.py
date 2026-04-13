"""
Phase 2 单元测试

测试混合搜索相关功能：
- Sparse 向量 Schema
- hybrid_search 方法
- 带 sparse embedding 的 upsert
- Bridge hybrid_search action
"""

import unittest
from unittest.mock import MagicMock, patch
import sys
import os
import json
import io

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))


class MockZvecModule:
    """Mock zvec module with Phase 2 extensions"""

    __version__ = "0.2.0-mock"

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
        def __init__(self, name=None, data_type=None, index_param=None):
            self.name = name
            self.data_type = data_type
            self.index_param = index_param

    class VectorSchema:
        def __init__(self, name=None, data_type=None, dimension=None, index_param=None):
            self.name = name
            self.data_type = data_type
            self.dimension = dimension
            self.index_param = index_param

    class CollectionSchema:
        def __init__(self, name=None, fields=None, vectors=None):
            self.name = name
            self.fields = fields or []
            self.vectors = vectors or []

    class HnswIndexParam:
        def __init__(self, metric_type=None):
            self.metric_type = metric_type

    class InvertIndexParam:
        pass

    class SparseIndexParam:
        pass

    class Doc:
        def __init__(self, id=None, vectors=None, fields=None):
            self.id = id
            self.vectors = vectors or {}
            self.fields = fields or {}
            self.score = 0.95

    class VectorQuery:
        def __init__(self, vector_name=None, vector=None):
            self.vector_name = vector_name
            self.vector = vector

    @staticmethod
    def create_and_open(path=None, schema=None):
        coll = MagicMock()
        coll.stats = {"doc_count": 0}
        coll.upsert = MagicMock()
        coll.query = MagicMock(return_value=[])
        coll.delete_by_filter = MagicMock()
        coll.delete = MagicMock()
        coll.optimize = MagicMock()
        coll.close = MagicMock()
        return coll

    @staticmethod
    def open(path=None):
        coll = MagicMock()
        coll.stats = {"doc_count": 10}
        return coll


# ============================================================
# ZvecEngine Phase 2 Tests
# ============================================================

class TestHybridSearchFunctions(unittest.TestCase):
    """Test hybrid_search method"""

    def setUp(self):
        sys.modules["zvec"] = MockZvecModule()
        from flashmemory.zvec_engine import ZvecEngine
        self.engine = ZvecEngine("/tmp/test_zvec", dimension=384)

    def tearDown(self):
        if "zvec" in sys.modules:
            del sys.modules["zvec"]

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_hybrid_search_dense_only(self, mock_makedirs, mock_exists):
        """Test hybrid search with dense vector only (no sparse)"""
        self.engine.init_func_collection()
        mock_doc = MagicMock()
        mock_doc.id = "func_1"
        mock_doc.score = 0.92
        mock_doc.fields = {"func_name": "test"}
        self.engine.func_collection.query.return_value = [mock_doc]

        results = self.engine.hybrid_search(
            dense_vector=[0.1] * 384,
            sparse_vector=None,
            top_k=5,
        )

        self.assertEqual(len(results), 1)
        # Should use single vector query (no RRF)
        call_args = self.engine.func_collection.query.call_args
        self.assertEqual(len(call_args.kwargs["vectors"]), 1)

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_hybrid_search_dense_and_sparse(self, mock_makedirs, mock_exists):
        """Test hybrid search with both dense and sparse vectors"""
        self.engine.init_func_collection()
        self.engine.func_collection.query.return_value = []

        results = self.engine.hybrid_search(
            dense_vector=[0.1] * 384,
            sparse_vector={"hello": 0.5, "world": 0.3},
            top_k=10,
        )

        call_args = self.engine.func_collection.query.call_args
        # Should have 2 vector queries (dense + sparse)
        self.assertEqual(len(call_args.kwargs["vectors"]), 2)

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_hybrid_search_with_filter(self, mock_makedirs, mock_exists):
        """Test hybrid search with scalar filter"""
        self.engine.init_func_collection()
        self.engine.func_collection.query.return_value = []

        self.engine.hybrid_search(
            dense_vector=[0.1] * 384,
            top_k=5,
            filter_expr='language = "go"',
        )

        call_args = self.engine.func_collection.query.call_args
        self.assertEqual(call_args.kwargs["filter"], 'language = "go"')

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_hybrid_search_rrf_disabled(self, mock_makedirs, mock_exists):
        """Test hybrid search with RRF disabled"""
        self.engine.init_func_collection()
        self.engine.func_collection.query.return_value = []

        self.engine.hybrid_search(
            dense_vector=[0.1] * 384,
            sparse_vector={"test": 1.0},
            top_k=5,
            use_rrf=False,
        )

        call_args = self.engine.func_collection.query.call_args
        # Should NOT have reranker when RRF is disabled
        self.assertNotIn("reranker", call_args.kwargs)

    def test_hybrid_search_without_init(self):
        """Test hybrid search before init raises error"""
        with self.assertRaises(RuntimeError):
            self.engine.hybrid_search([0.1] * 384)


class TestSparseVectorUpsert(unittest.TestCase):
    """Test upsert with sparse vectors (Phase 2)"""

    def setUp(self):
        sys.modules["zvec"] = MockZvecModule()
        from flashmemory.zvec_engine import ZvecEngine
        self.engine = ZvecEngine("/tmp/test_zvec", dimension=384)

    def tearDown(self):
        if "zvec" in sys.modules:
            del sys.modules["zvec"]

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_upsert_with_sparse(self, mock_makedirs, mock_exists):
        """Test upserting function with both dense and sparse vectors"""
        self.engine.init_func_collection()
        self.engine.upsert_function(
            "func_1",
            [0.1] * 384,
            {"func_name": "test"},
            sparse_embedding={"keyword1": 0.8, "keyword2": 0.3},
        )

        call_args = self.engine.func_collection.upsert.call_args
        doc = call_args.kwargs["docs"][0]
        self.assertIn("dense_embedding", doc.vectors)
        self.assertIn("sparse_embedding", doc.vectors)

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_upsert_without_sparse(self, mock_makedirs, mock_exists):
        """Test upserting function without sparse vector (backward compat)"""
        self.engine.init_func_collection()
        self.engine.upsert_function(
            "func_1",
            [0.1] * 384,
            {"func_name": "test"},
        )

        call_args = self.engine.func_collection.upsert.call_args
        doc = call_args.kwargs["docs"][0]
        self.assertIn("dense_embedding", doc.vectors)
        self.assertNotIn("sparse_embedding", doc.vectors)

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_batch_upsert_with_sparse(self, mock_makedirs, mock_exists):
        """Test batch upsert with optional sparse vectors"""
        self.engine.init_func_collection()

        items = [
            ("func_1", [0.1] * 384, {"func_name": "a"}, {"kw1": 0.5}),
            ("func_2", [0.2] * 384, {"func_name": "b"}),  # No sparse
            ("func_3", [0.3] * 384, {"func_name": "c"}, {"kw2": 0.8}),
        ]
        self.engine.upsert_functions_batch(items)

        call_args = self.engine.func_collection.upsert.call_args
        docs = call_args.kwargs["docs"]
        self.assertEqual(len(docs), 3)

        # First doc should have sparse
        self.assertIn("sparse_embedding", docs[0].vectors)
        # Second doc should NOT have sparse
        self.assertNotIn("sparse_embedding", docs[1].vectors)
        # Third doc should have sparse
        self.assertIn("sparse_embedding", docs[2].vectors)


class TestSparseSchemaCreation(unittest.TestCase):
    """Test that schema includes sparse vector column"""

    def setUp(self):
        self.mock_zvec = MockZvecModule()
        sys.modules["zvec"] = self.mock_zvec

    def tearDown(self):
        if "zvec" in sys.modules:
            del sys.modules["zvec"]

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_func_collection_has_sparse_vector(self, mock_makedirs, mock_exists):
        """Test that function collection schema defines sparse_embedding vector"""
        # Track CollectionSchema calls
        schema_calls = []
        original_schema = self.mock_zvec.CollectionSchema

        class TrackingSchema:
            def __init__(self, **kwargs):
                schema_calls.append(kwargs)
                self._inner = original_schema(**kwargs)

        self.mock_zvec.CollectionSchema = TrackingSchema

        from flashmemory.zvec_engine import ZvecEngine
        engine = ZvecEngine("/tmp/test", dimension=384)
        engine.init_func_collection(force_new=True)

        # Verify schema was created with sparse vector
        self.assertTrue(len(schema_calls) > 0)
        schema_kwargs = schema_calls[0]
        vectors = schema_kwargs.get("vectors", [])

        # Should have 2 vectors: dense + sparse
        self.assertEqual(len(vectors), 2)
        vector_names = [v.name for v in vectors]
        self.assertIn("dense_embedding", vector_names)
        self.assertIn("sparse_embedding", vector_names)


# ============================================================
# ZvecBridge Phase 2 Tests
# ============================================================

class TestBridgeHybridSearch(unittest.TestCase):
    """Test bridge hybrid_search action"""

    def setUp(self):
        sys.modules["zvec"] = MockZvecModule()
        from flashmemory.zvec_bridge import ZvecBridge
        self.ZvecBridge = ZvecBridge

    def tearDown(self):
        if "zvec" in sys.modules:
            del sys.modules["zvec"]

    def _send_and_capture(self, bridge, action, params=None):
        old_stdout = sys.stdout
        sys.stdout = io.StringIO()

        handler = bridge.ACTION_HANDLERS.get(action)
        self.assertIsNotNone(handler, f"Handler for action '{action}' not found")
        handler(bridge, params or {})

        output = sys.stdout.getvalue()
        sys.stdout = old_stdout

        lines = [l for l in output.strip().split("\n") if l.strip()]
        self.assertTrue(len(lines) > 0)
        return json.loads(lines[-1])

    def test_hybrid_search_action_exists(self):
        """Test hybrid_search is registered in action handlers"""
        bridge = self.ZvecBridge()
        self.assertIn("hybrid_search", bridge.ACTION_HANDLERS)

    def test_hybrid_search_without_init(self):
        """Test hybrid search before init returns error"""
        bridge = self.ZvecBridge()
        resp = self._send_and_capture(bridge, "hybrid_search", {
            "dense_query": [0.1] * 384,
        })
        self.assertEqual(resp["status"], "error")

    def test_hybrid_search_missing_dense(self):
        """Test hybrid search with missing dense query returns error"""
        bridge = self.ZvecBridge()
        bridge.engine = MagicMock()

        resp = self._send_and_capture(bridge, "hybrid_search", {})
        self.assertEqual(resp["status"], "error")
        self.assertIn("dense_query", resp["message"])

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_hybrid_search_dense_only_success(self, mock_makedirs, mock_exists):
        """Test hybrid search with dense vector only"""
        bridge = self.ZvecBridge()

        # Init engine
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
        mock_doc.score = 0.88
        mock_doc.fields = {"func_name": "test"}
        bridge.engine.hybrid_search = MagicMock(return_value=[mock_doc])

        resp = self._send_and_capture(bridge, "hybrid_search", {
            "dense_query": [0.1] * 384,
            "top_k": 5,
        })

        self.assertEqual(resp["status"], "success")
        self.assertIn("results", resp["data"])
        self.assertEqual(resp["data"]["count"], 1)
        self.assertEqual(resp["data"]["search_type"], "dense_only")

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_hybrid_search_with_sparse(self, mock_makedirs, mock_exists):
        """Test hybrid search with dense + sparse"""
        bridge = self.ZvecBridge()

        old_stdout = sys.stdout
        sys.stdout = io.StringIO()
        bridge.handle_init({
            "collection_path": "/tmp/test_coll",
            "dimension": 384,
            "collection_type": "functions",
        })
        sys.stdout = old_stdout

        bridge.engine.hybrid_search = MagicMock(return_value=[])

        resp = self._send_and_capture(bridge, "hybrid_search", {
            "dense_query": [0.1] * 384,
            "sparse_query": {"hello": 0.5, "world": 0.3},
            "top_k": 10,
            "use_rrf": True,
        })

        self.assertEqual(resp["status"], "success")
        self.assertEqual(resp["data"]["search_type"], "hybrid")

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_hybrid_search_with_filter(self, mock_makedirs, mock_exists):
        """Test hybrid search passes through filter expression"""
        bridge = self.ZvecBridge()

        old_stdout = sys.stdout
        sys.stdout = io.StringIO()
        bridge.handle_init({
            "collection_path": "/tmp/test_coll",
            "dimension": 384,
            "collection_type": "functions",
        })
        sys.stdout = old_stdout

        bridge.engine.hybrid_search = MagicMock(return_value=[])

        resp = self._send_and_capture(bridge, "hybrid_search", {
            "dense_query": [0.1] * 384,
            "top_k": 5,
            "filter": 'language = "go"',
        })

        self.assertEqual(resp["status"], "success")
        # Verify filter was passed through
        call_args = bridge.engine.hybrid_search.call_args
        self.assertEqual(call_args.kwargs["filter_expr"], 'language = "go"')


if __name__ == "__main__":
    unittest.main(verbosity=2)
