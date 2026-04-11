"""
Phase 3 单元测试 - AI Extension Integration

测试内容:
- EmbeddingProvider 初始化和 fallback 逻辑
- EmbeddingProvider dense/sparse embedding
- SearchPipeline 两阶段搜索
- SearchPipeline 上下文过滤
- Bridge embed/init_embedding/pipeline_search actions
"""

import unittest
from unittest.mock import MagicMock, patch
import sys
import os
import json
import io

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))


# ============================================================
# EmbeddingProvider Tests
# ============================================================


class TestEmbeddingProviderInit(unittest.TestCase):
    """Test EmbeddingProvider initialization and fallback logic."""

    def test_default_init(self):
        """Default init should use fallback dense (no sentence-transformers assumed)."""
        from flashmemory.embedding_provider import EmbeddingProvider
        provider = EmbeddingProvider()
        self.assertIsNotNone(provider)
        self.assertEqual(provider.dimension, 384)
        self.assertFalse(provider.has_sparse)
        info = provider.get_info()
        self.assertEqual(info["dense_provider"], "local")
        self.assertEqual(info["sparse_provider"], "none")

    def test_custom_dimension(self):
        """Custom dimension should be respected in fallback mode."""
        from flashmemory.embedding_provider import EmbeddingProvider
        provider = EmbeddingProvider(config={"dimension": 768})
        self.assertEqual(provider.dimension, 768)

    def test_unknown_dense_provider_raises(self):
        """Unknown dense provider should raise ValueError."""
        from flashmemory.embedding_provider import EmbeddingProvider
        with self.assertRaises(ValueError):
            EmbeddingProvider(config={"dense_provider": "nonexistent"})

    def test_unknown_sparse_provider_raises(self):
        """Unknown sparse provider should raise ValueError."""
        from flashmemory.embedding_provider import EmbeddingProvider
        with self.assertRaises(ValueError):
            EmbeddingProvider(config={"sparse_provider": "nonexistent"})

    def test_sparse_none_disables_sparse(self):
        """sparse_provider=none should disable sparse embedding."""
        from flashmemory.embedding_provider import EmbeddingProvider
        provider = EmbeddingProvider(config={"sparse_provider": "none"})
        self.assertFalse(provider.has_sparse)
        self.assertIsNone(provider.embed_sparse("test"))


class TestEmbeddingProviderDense(unittest.TestCase):
    """Test dense embedding generation."""

    def test_fallback_dense_embedding(self):
        """Fallback embedding should return vector of correct dimension."""
        from flashmemory.embedding_provider import EmbeddingProvider
        provider = EmbeddingProvider(config={"dimension": 128})
        vec = provider.embed_dense("hello world")
        self.assertIsInstance(vec, list)
        self.assertEqual(len(vec), 128)
        # All values should be floats
        for v in vec:
            self.assertIsInstance(v, float)

    def test_fallback_deterministic(self):
        """Same text should produce same embedding."""
        from flashmemory.embedding_provider import EmbeddingProvider
        provider = EmbeddingProvider(config={"dimension": 64})
        vec1 = provider.embed_dense("test query")
        vec2 = provider.embed_dense("test query")
        self.assertEqual(vec1, vec2)

    def test_fallback_different_texts(self):
        """Different texts should produce different embeddings."""
        from flashmemory.embedding_provider import EmbeddingProvider
        provider = EmbeddingProvider(config={"dimension": 64})
        vec1 = provider.embed_dense("hello")
        vec2 = provider.embed_dense("world")
        self.assertNotEqual(vec1, vec2)

    def test_batch_embedding(self):
        """Batch embedding should return list of vectors."""
        from flashmemory.embedding_provider import EmbeddingProvider
        provider = EmbeddingProvider(config={"dimension": 64})
        texts = ["hello", "world", "test"]
        vecs = provider.embed_dense_batch(texts)
        self.assertEqual(len(vecs), 3)
        for vec in vecs:
            self.assertEqual(len(vec), 64)


class TestEmbeddingProviderInfo(unittest.TestCase):
    """Test provider info and diagnostics."""

    def test_get_info(self):
        """get_info should return correct structure."""
        from flashmemory.embedding_provider import EmbeddingProvider
        provider = EmbeddingProvider()
        info = provider.get_info()
        self.assertIn("dense_provider", info)
        self.assertIn("sparse_provider", info)
        self.assertIn("dimension", info)
        self.assertIn("has_sparse", info)
        self.assertIn("dense_type", info)

    def test_openai_without_key_fallback(self):
        """OpenAI provider without zvec[embedding] should fallback gracefully."""
        from flashmemory.embedding_provider import EmbeddingProvider
        # Without zvec.extension.OpenAIDenseEmbedding, it falls back to FallbackDenseEmbedding
        provider = EmbeddingProvider(config={"dense_provider": "openai"})
        info = provider.get_info()
        # Should have fallen back
        self.assertEqual(info["dense_type"], "_FallbackDenseEmbedding")


# ============================================================
# SearchPipeline Tests
# ============================================================


class TestSearchPipeline(unittest.TestCase):
    """Test SearchPipeline two-stage search."""

    def setUp(self):
        """Set up mock engine and embedding provider."""
        from flashmemory.embedding_provider import EmbeddingProvider
        self.provider = EmbeddingProvider(config={"dimension": 384})

        # Mock ZvecEngine
        self.engine = MagicMock()
        mock_doc1 = MagicMock()
        mock_doc1.id = "func_1"
        mock_doc1.score = 0.95
        mock_doc1.fields = {"func_name": "UploadFile", "language": "go"}

        mock_doc2 = MagicMock()
        mock_doc2.id = "func_2"
        mock_doc2.score = 0.88
        mock_doc2.fields = {"func_name": "DownloadFile", "language": "go"}

        self.engine.search_functions.return_value = [mock_doc1, mock_doc2]
        self.engine.hybrid_search_functions.return_value = [mock_doc1, mock_doc2]
        self.engine.search_modules.return_value = [mock_doc1]

    def test_basic_search(self):
        """Test basic search pipeline."""
        from flashmemory.search_pipeline import SearchPipeline
        pipeline = SearchPipeline(self.engine, self.provider)

        results = pipeline.search("file upload", top_k=2)
        self.assertEqual(len(results), 2)
        self.assertEqual(results[0].doc_id, "func_1")
        self.assertEqual(results[0].score, 0.95)
        self.assertEqual(results[0].fields["func_name"], "UploadFile")

    def test_search_with_filter(self):
        """Test search with scalar filter."""
        from flashmemory.search_pipeline import SearchPipeline
        pipeline = SearchPipeline(self.engine, self.provider)

        pipeline.search("test", top_k=5, filter_expr='language = "go"')
        # Should call search_functions (dense-only since no sparse)
        self.engine.search_functions.assert_called()
        call_kwargs = self.engine.search_functions.call_args.kwargs
        self.assertEqual(call_kwargs["filter_expr"], 'language = "go"')

    def test_search_modules(self):
        """Test module search."""
        from flashmemory.search_pipeline import SearchPipeline
        pipeline = SearchPipeline(self.engine, self.provider)

        results = pipeline.search("search engine", top_k=5, search_type="modules")
        self.engine.search_modules.assert_called()
        self.assertEqual(len(results), 1)

    def test_search_with_context(self):
        """Test search_with_context builds filter expression."""
        from flashmemory.search_pipeline import SearchPipeline
        pipeline = SearchPipeline(self.engine, self.provider)

        pipeline.search_with_context(
            "upload handler",
            top_k=10,
            language="go",
            package="main",
        )
        call_kwargs = self.engine.search_functions.call_args.kwargs
        filter_expr = call_kwargs["filter_expr"]
        self.assertIn("language", filter_expr)
        self.assertIn("go", filter_expr)
        self.assertIn("package", filter_expr)
        self.assertIn("main", filter_expr)

    def test_search_empty_results(self):
        """Test search returning no results."""
        from flashmemory.search_pipeline import SearchPipeline
        self.engine.search_functions.return_value = []
        pipeline = SearchPipeline(self.engine, self.provider)

        results = pipeline.search("nonexistent", top_k=5)
        self.assertEqual(len(results), 0)

    def test_result_to_dict(self):
        """Test SearchResult serialization."""
        from flashmemory.search_pipeline import SearchResult
        r = SearchResult("func_1", 0.95, {"func_name": "test"})
        d = r.to_dict()
        self.assertEqual(d["id"], "func_1")
        self.assertEqual(d["score"], 0.95)
        self.assertEqual(d["fields"]["func_name"], "test")

    def test_pipeline_info(self):
        """Test pipeline diagnostic info."""
        from flashmemory.search_pipeline import SearchPipeline
        pipeline = SearchPipeline(self.engine, self.provider, config={
            "enable_reranker": False,
            "recall_multiplier": 3,
        })
        info = pipeline.get_pipeline_info()
        self.assertIn("embedding", info)
        self.assertIn("pipeline_config", info)
        self.assertIn("reranker_available", info)

    def test_recall_multiplier(self):
        """Test recall multiplier is applied when reranker is disabled."""
        from flashmemory.search_pipeline import SearchPipeline
        pipeline = SearchPipeline(self.engine, self.provider, config={
            "recall_multiplier": 3,
        })
        pipeline.search("test", top_k=5)
        # Without reranker, recall_count = top_k (not multiplied)
        call_kwargs = self.engine.search_functions.call_args.kwargs
        self.assertEqual(call_kwargs["top_k"], 5)


class TestSearchPipelineWithSparse(unittest.TestCase):
    """Test search pipeline with sparse embedding enabled (mocked)."""

    def setUp(self):
        from flashmemory.embedding_provider import EmbeddingProvider

        # Create provider with mocked sparse
        self.provider = EmbeddingProvider(config={"dimension": 384})
        # Inject a mock sparse provider
        mock_sparse = MagicMock()
        mock_sparse.embed.return_value = {"test": 0.5, "query": 0.3}
        self.provider._sparse = mock_sparse

        self.engine = MagicMock()
        mock_doc = MagicMock()
        mock_doc.id = "func_1"
        mock_doc.score = 0.92
        mock_doc.fields = {"func_name": "test"}
        self.engine.hybrid_search_functions.return_value = [mock_doc]

    def test_hybrid_search_called(self):
        """When sparse is available, hybrid search should be used."""
        from flashmemory.search_pipeline import SearchPipeline
        pipeline = SearchPipeline(self.engine, self.provider)

        results = pipeline.search("test query", top_k=5)
        # Should use hybrid_search_functions (not search_functions)
        self.engine.hybrid_search_functions.assert_called()
        self.engine.search_functions.assert_not_called()
        self.assertEqual(len(results), 1)


# ============================================================
# Bridge Phase 3 Action Tests
# ============================================================


class MockZvecModule:
    """Mock zvec module for bridge tests."""

    __version__ = "0.3.0-mock"

    class DataType:
        STRING = "string"
        INT32 = "int32"
        DOUBLE = "double"
        VECTOR_FP32 = "vector_fp32"
        SPARSE_VECTOR_FP32 = "sparse_vector_fp32"

    class MetricType:
        COSINE = "cosine"

    class FieldSchema:
        def __init__(self, **kwargs): pass

    class VectorSchema:
        def __init__(self, **kwargs): pass

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
        def __init__(self, *args, **kwargs): pass

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


class TestBridgeEmbeddingActions(unittest.TestCase):
    """Test bridge init_embedding and embed actions."""

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
        self.assertIsNotNone(handler, f"Handler for '{action}' not found")
        handler(bridge, params or {})
        output = sys.stdout.getvalue()
        sys.stdout = old_stdout
        lines = [l for l in output.strip().split("\n") if l.strip()]
        self.assertTrue(len(lines) > 0)
        return json.loads(lines[-1])

    def test_init_embedding_action_exists(self):
        """Test init_embedding is registered."""
        bridge = self.ZvecBridge()
        self.assertIn("init_embedding", bridge.ACTION_HANDLERS)

    def test_embed_action_exists(self):
        """Test embed is registered."""
        bridge = self.ZvecBridge()
        self.assertIn("embed", bridge.ACTION_HANDLERS)

    def test_pipeline_search_action_exists(self):
        """Test pipeline_search is registered."""
        bridge = self.ZvecBridge()
        self.assertIn("pipeline_search", bridge.ACTION_HANDLERS)

    def test_init_embedding_success(self):
        """Test init_embedding with default (fallback) config."""
        bridge = self.ZvecBridge()
        resp = self._send_and_capture(bridge, "init_embedding", {
            "dimension": 128,
        })
        self.assertEqual(resp["status"], "success")
        self.assertIn("dimension", resp["data"])
        self.assertEqual(resp["data"]["dimension"], 128)

    def test_embed_without_init(self):
        """Test embed before init_embedding returns error."""
        bridge = self.ZvecBridge()
        resp = self._send_and_capture(bridge, "embed", {"text": "test"})
        self.assertEqual(resp["status"], "error")

    def test_embed_dense_text(self):
        """Test dense embedding of single text."""
        bridge = self.ZvecBridge()

        # Init embedding first
        self._send_and_capture(bridge, "init_embedding", {"dimension": 64})

        resp = self._send_and_capture(bridge, "embed", {
            "text": "file upload handler",
            "type": "dense",
        })
        self.assertEqual(resp["status"], "success")
        self.assertIn("dense", resp["data"])
        self.assertEqual(len(resp["data"]["dense"]), 64)
        self.assertEqual(resp["data"]["dimension"], 64)

    def test_embed_batch(self):
        """Test batch dense embedding."""
        bridge = self.ZvecBridge()
        self._send_and_capture(bridge, "init_embedding", {"dimension": 32})

        resp = self._send_and_capture(bridge, "embed", {
            "texts": ["hello", "world", "test"],
            "type": "dense",
        })
        self.assertEqual(resp["status"], "success")
        self.assertIn("dense_batch", resp["data"])
        self.assertEqual(len(resp["data"]["dense_batch"]), 3)
        self.assertEqual(resp["data"]["count"], 3)

    def test_embed_empty_params(self):
        """Test embed with empty params returns error."""
        bridge = self.ZvecBridge()
        self._send_and_capture(bridge, "init_embedding", {})
        resp = self._send_and_capture(bridge, "embed", {})
        self.assertEqual(resp["status"], "error")

    def test_pipeline_search_without_engine(self):
        """Test pipeline_search without engine returns error."""
        bridge = self.ZvecBridge()
        self._send_and_capture(bridge, "init_embedding", {})
        resp = self._send_and_capture(bridge, "pipeline_search", {
            "query": "test",
        })
        self.assertEqual(resp["status"], "error")

    def test_pipeline_search_without_embedding(self):
        """Test pipeline_search without embedding returns error."""
        bridge = self.ZvecBridge()
        bridge.engine = MagicMock()
        resp = self._send_and_capture(bridge, "pipeline_search", {
            "query": "test",
        })
        self.assertEqual(resp["status"], "error")

    def test_pipeline_search_empty_query(self):
        """Test pipeline_search with empty query returns error."""
        bridge = self.ZvecBridge()
        bridge.engine = MagicMock()
        self._send_and_capture(bridge, "init_embedding", {})
        resp = self._send_and_capture(bridge, "pipeline_search", {})
        self.assertEqual(resp["status"], "error")


if __name__ == "__main__":
    unittest.main(verbosity=2)
