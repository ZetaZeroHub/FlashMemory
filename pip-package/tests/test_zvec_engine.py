"""
ZvecEngine 单元测试

通过 Mock zvec 模块来测试所有引擎功能，
不依赖 zvec 实际安装。
"""

import unittest
from unittest.mock import MagicMock, patch, PropertyMock
import sys
import os
import json

# Add package path
sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))


class MockZvecModule:
    """Mock zvec module to simulate the real zvec API"""

    __version__ = "0.1.1-mock"

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
        def __init__(self, name, data_type, index_param=None):
            self.name = name
            self.data_type = data_type
            self.index_param = index_param

    class VectorSchema:
        def __init__(self, name, data_type, dimension=None, index_param=None):
            self.name = name
            self.data_type = data_type
            self.dimension = dimension
            self.index_param = index_param

    class CollectionSchema:
        def __init__(self, name, fields=None, vectors=None):
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
        def __init__(self, vector_name, vector=None):
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
        coll.upsert = MagicMock()
        coll.query = MagicMock(return_value=[])
        coll.delete_by_filter = MagicMock()
        coll.delete = MagicMock()
        coll.optimize = MagicMock()
        coll.close = MagicMock()
        return coll


class TestZvecEngineInit(unittest.TestCase):
    """Test ZvecEngine initialization"""

    def setUp(self):
        # Inject mock zvec module
        self.mock_zvec = MockZvecModule()
        sys.modules["zvec"] = self.mock_zvec

        from flashmemory.zvec_engine import ZvecEngine
        self.ZvecEngine = ZvecEngine

    def tearDown(self):
        if "zvec" in sys.modules:
            del sys.modules["zvec"]

    def test_default_dimension(self):
        """Test default dimension is 384 (all-MiniLM-L6-v2)"""
        engine = self.ZvecEngine("/tmp/test_zvec")
        self.assertEqual(engine.dimension, 384)

    def test_custom_dimension(self):
        """Test custom dimension override"""
        engine = self.ZvecEngine("/tmp/test_zvec", dimension=1024)
        self.assertEqual(engine.dimension, 1024)

    def test_collection_paths(self):
        """Test collection path generation"""
        engine = self.ZvecEngine("/data/.gitgo/zvec_collections")
        self.assertEqual(
            engine._get_func_collection_path(),
            "/data/.gitgo/zvec_collections/functions",
        )
        self.assertEqual(
            engine._get_module_collection_path(),
            "/data/.gitgo/zvec_collections/modules",
        )

    def test_lazy_zvec_import(self):
        """Test zvec module is lazily imported"""
        engine = self.ZvecEngine("/tmp/test_zvec")
        self.assertIsNone(engine._zvec)
        # Should import on first call
        zvec = engine._ensure_zvec()
        self.assertIsNotNone(zvec)

    def test_zvec_not_installed_error(self):
        """Test proper error when zvec is not installed"""
        del sys.modules["zvec"]
        engine = self.ZvecEngine("/tmp/test_zvec")
        engine._zvec = None
        with self.assertRaises(ImportError) as ctx:
            engine._ensure_zvec()
        self.assertIn("zvec", str(ctx.exception))


class TestZvecEngineFuncCollection(unittest.TestCase):
    """Test function-level collection operations"""

    def setUp(self):
        self.mock_zvec = MockZvecModule()
        sys.modules["zvec"] = self.mock_zvec

        from flashmemory.zvec_engine import ZvecEngine
        self.engine = ZvecEngine("/tmp/test_zvec", dimension=384)

    def tearDown(self):
        if "zvec" in sys.modules:
            del sys.modules["zvec"]

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_init_func_collection_new(self, mock_makedirs, mock_exists):
        """Test creating a new function collection"""
        coll = self.engine.init_func_collection(force_new=False)
        self.assertIsNotNone(coll)
        self.assertIsNotNone(self.engine.func_collection)

    @patch("os.path.exists", return_value=True)
    def test_init_func_collection_existing(self, mock_exists):
        """Test opening an existing function collection"""
        coll = self.engine.init_func_collection(force_new=False)
        self.assertIsNotNone(coll)

    @patch("os.path.exists", return_value=True)
    def test_init_func_collection_force_new(self, mock_exists):
        """Test force creating a new collection even if exists"""
        with patch("os.makedirs"):
            coll = self.engine.init_func_collection(force_new=True)
            self.assertIsNotNone(coll)

    def test_upsert_function_without_init(self):
        """Test upsert before collection init raises error"""
        with self.assertRaises(RuntimeError) as ctx:
            self.engine.upsert_function("func_1", [0.1] * 384, {})
        self.assertIn("未初始化", str(ctx.exception))

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_upsert_function(self, mock_makedirs, mock_exists):
        """Test upserting a function vector"""
        self.engine.init_func_collection()
        self.engine.upsert_function(
            "func_42",
            [0.1] * 384,
            {"func_name": "doSomething", "package": "main", "file_path": "main.go"},
        )
        self.engine.func_collection.upsert.assert_called_once()

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_upsert_functions_batch(self, mock_makedirs, mock_exists):
        """Test batch upserting function vectors"""
        self.engine.init_func_collection()
        items = [
            ("func_1", [0.1] * 384, {"func_name": "a"}),
            ("func_2", [0.2] * 384, {"func_name": "b"}),
            ("func_3", [0.3] * 384, {"func_name": "c"}),
        ]
        self.engine.upsert_functions_batch(items)
        self.engine.func_collection.upsert.assert_called_once()
        call_args = self.engine.func_collection.upsert.call_args
        self.assertEqual(len(call_args.kwargs["docs"]), 3)

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_search_functions(self, mock_makedirs, mock_exists):
        """Test function search"""
        self.engine.init_func_collection()
        # Mock search results
        mock_doc = MagicMock()
        mock_doc.id = "func_1"
        mock_doc.score = 0.95
        mock_doc.fields = {"func_name": "test"}
        self.engine.func_collection.query.return_value = [mock_doc]

        results = self.engine.search_functions([0.1] * 384, top_k=5)
        self.assertEqual(len(results), 1)
        self.engine.func_collection.query.assert_called_once()

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_search_functions_with_filter(self, mock_makedirs, mock_exists):
        """Test function search with scalar filter"""
        self.engine.init_func_collection()
        self.engine.func_collection.query.return_value = []

        self.engine.search_functions(
            [0.1] * 384, top_k=10, filter_expr='language = "go"'
        )
        call_args = self.engine.func_collection.query.call_args
        self.assertEqual(call_args.kwargs["filter"], 'language = "go"')

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_delete_by_file(self, mock_makedirs, mock_exists):
        """Test deleting vectors by file path"""
        self.engine.init_func_collection()
        self.engine.delete_by_file("internal/search/search.go")
        self.engine.func_collection.delete_by_filter.assert_called_once()

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_delete_function(self, mock_makedirs, mock_exists):
        """Test deleting a single function vector"""
        self.engine.init_func_collection()
        self.engine.delete_function("func_42")
        self.engine.func_collection.delete.assert_called_once()

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_optimize(self, mock_makedirs, mock_exists):
        """Test optimization"""
        self.engine.init_func_collection()
        self.engine.optimize()
        self.engine.func_collection.optimize.assert_called_once()

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_close(self, mock_makedirs, mock_exists):
        """Test closing collection"""
        self.engine.init_func_collection()
        self.engine.close()
        self.assertIsNone(self.engine.func_collection)

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_get_stats(self, mock_makedirs, mock_exists):
        """Test getting stats"""
        self.engine.init_func_collection()
        stats = self.engine.get_stats()
        self.assertIn("functions", stats)


class TestZvecEngineModuleCollection(unittest.TestCase):
    """Test module-level collection operations"""

    def setUp(self):
        self.mock_zvec = MockZvecModule()
        sys.modules["zvec"] = self.mock_zvec

        from flashmemory.zvec_engine import ZvecEngine
        self.engine = ZvecEngine("/tmp/test_zvec", dimension=384)

    def tearDown(self):
        if "zvec" in sys.modules:
            del sys.modules["zvec"]

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_init_module_collection(self, mock_makedirs, mock_exists):
        """Test creating a module collection"""
        coll = self.engine.init_module_collection()
        self.assertIsNotNone(coll)
        self.assertIsNotNone(self.engine.module_collection)

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_upsert_module(self, mock_makedirs, mock_exists):
        """Test upserting a module vector"""
        self.engine.init_module_collection()
        self.engine.upsert_module(
            "mod_1",
            [0.1] * 384,
            {"name": "search", "type": "directory", "path": "internal/search"},
        )
        self.engine.module_collection.upsert.assert_called_once()

    def test_upsert_module_without_init(self):
        """Test upsert module before init raises error"""
        with self.assertRaises(RuntimeError):
            self.engine.upsert_module("mod_1", [0.1] * 384, {})

    @patch("os.path.exists", return_value=False)
    @patch("os.makedirs")
    def test_search_modules(self, mock_makedirs, mock_exists):
        """Test module search"""
        self.engine.init_module_collection()
        self.engine.module_collection.query.return_value = []
        results = self.engine.search_modules([0.1] * 384, top_k=5)
        self.assertEqual(len(results), 0)

    def test_search_modules_without_init(self):
        """Test search modules before init raises error"""
        with self.assertRaises(RuntimeError):
            self.engine.search_modules([0.1] * 384)


if __name__ == "__main__":
    unittest.main(verbosity=2)
