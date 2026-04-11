"""
FlashMemory Embedding Provider

Multi-source embedding abstraction supporting:
- Dense: local (all-MiniLM-L6-v2), OpenAI, Qwen, Jina
- Sparse: BM25 (zh/en), SPLADE

Designed as a unified interface for both Phase 2 hybrid search
and Phase 3 AI extension integration.
"""

import logging
from typing import List, Dict, Optional, Any

logger = logging.getLogger("flashmemory.embedding_provider")


class EmbeddingProvider:
    """Unified embedding provider supporting multiple dense and sparse sources.

    Usage:
        provider = EmbeddingProvider(config={
            "dense_provider": "local",    # or "openai", "qwen", "jina"
            "sparse_provider": "bm25_zh", # or "bm25_en", "splade", "none"
        })
        dense_vec = provider.embed_dense("search file upload")
        sparse_vec = provider.embed_sparse("search file upload")
    """

    # Registry of dense embedding providers
    DENSE_PROVIDERS = {
        "local": "_create_local_dense",
        "openai": "_create_openai_dense",
        "qwen": "_create_qwen_dense",
        "jina": "_create_jina_dense",
    }

    # Registry of sparse embedding providers
    SPARSE_PROVIDERS = {
        "bm25_zh": "_create_bm25_zh",
        "bm25_en": "_create_bm25_en",
        "splade": "_create_splade",
        "none": None,
    }

    def __init__(self, config: Dict[str, Any] = None):
        """Initialize embedding provider from config.

        Args:
            config: Dict with keys:
                - dense_provider: str (default "local")
                - sparse_provider: str (default "none")
                - model_source: str (default "huggingface") for local provider
                - api_key: str (required for cloud providers)
                - dimension: int (for cloud providers)
                - model_name: str (override default model)
        """
        self.config = config or {}
        self._dense = None
        self._sparse = None
        self._dimension = self.config.get("dimension", 384)

        # Initialize dense provider
        dense_name = self.config.get("dense_provider", "local")
        self._init_dense(dense_name)

        # Initialize sparse provider
        sparse_name = self.config.get("sparse_provider", "none")
        self._init_sparse(sparse_name)

        logger.info(
            "EmbeddingProvider initialized: dense=%s, sparse=%s, dimension=%d",
            dense_name, sparse_name, self._dimension,
        )

    def _init_dense(self, provider_name: str):
        """Initialize dense embedding provider."""
        if provider_name not in self.DENSE_PROVIDERS:
            raise ValueError(
                f"Unknown dense provider: {provider_name}. "
                f"Available: {list(self.DENSE_PROVIDERS.keys())}"
            )

        factory_method = getattr(self, self.DENSE_PROVIDERS[provider_name])
        try:
            self._dense = factory_method()
            logger.info("Dense embedding provider '%s' initialized", provider_name)
        except ImportError as e:
            logger.warning(
                "Failed to import dense provider '%s': %s. "
                "Using fallback simple embedding.",
                provider_name, e,
            )
            self._dense = self._create_fallback_dense()
        except Exception as e:
            logger.error(
                "Failed to initialize dense provider '%s': %s. "
                "Using fallback.",
                provider_name, e,
            )
            self._dense = self._create_fallback_dense()

    def _init_sparse(self, provider_name: str):
        """Initialize sparse embedding provider."""
        if provider_name == "none" or provider_name is None:
            self._sparse = None
            logger.info("Sparse embedding disabled")
            return

        if provider_name not in self.SPARSE_PROVIDERS:
            raise ValueError(
                f"Unknown sparse provider: {provider_name}. "
                f"Available: {list(self.SPARSE_PROVIDERS.keys())}"
            )

        factory_name = self.SPARSE_PROVIDERS[provider_name]
        if factory_name is None:
            self._sparse = None
            return

        factory_method = getattr(self, factory_name)
        try:
            self._sparse = factory_method()
            logger.info("Sparse embedding provider '%s' initialized", provider_name)
        except ImportError as e:
            logger.warning(
                "Failed to import sparse provider '%s': %s. Sparse disabled.",
                provider_name, e,
            )
            self._sparse = None
        except Exception as e:
            logger.error(
                "Failed to initialize sparse provider '%s': %s. Sparse disabled.",
                provider_name, e,
            )
            self._sparse = None

    # --- Dense provider factories ---

    def _create_local_dense(self):
        """Create local dense embedding using sentence-transformers or zvec extension."""
        model_name = self.config.get("model_name", "all-MiniLM-L6-v2")
        model_source = self.config.get("model_source", "huggingface")

        # Try zvec's built-in embedding first
        try:
            from zvec.extension import DefaultLocalDenseEmbedding
            emb = DefaultLocalDenseEmbedding(model_source=model_source)
            self._dimension = emb.dimension
            logger.info("Using zvec DefaultLocalDenseEmbedding (model_source=%s)", model_source)
            return emb
        except ImportError:
            pass

        # Fallback to sentence-transformers
        try:
            from sentence_transformers import SentenceTransformer
            model = SentenceTransformer(model_name)
            self._dimension = model.get_sentence_embedding_dimension()
            logger.info("Using SentenceTransformer model: %s (dim=%d)", model_name, self._dimension)
            return _SentenceTransformerWrapper(model)
        except ImportError:
            pass

        # Final fallback
        logger.warning("No embedding library available, using hash-based fallback")
        return self._create_fallback_dense()

    def _create_openai_dense(self):
        """Create OpenAI dense embedding provider."""
        try:
            from zvec.extension import OpenAIDenseEmbedding
            api_key = self.config.get("api_key", "")
            if not api_key:
                raise ValueError("api_key is required for OpenAI embedding")
            dimension = self.config.get("dimension", 1536)
            model = self.config.get("model_name", "text-embedding-3-small")
            emb = OpenAIDenseEmbedding(api_key=api_key, dimension=dimension, model=model)
            self._dimension = dimension
            return emb
        except ImportError:
            raise ImportError(
                "OpenAI embedding requires zvec[embedding] or openai package. "
                "Install with: pip install zvec[embedding]"
            )

    def _create_qwen_dense(self):
        """Create Qwen (Alibaba DashScope) dense embedding provider."""
        try:
            from zvec.extension import QwenDenseEmbedding
            api_key = self.config.get("api_key", "")
            if not api_key:
                raise ValueError("api_key is required for Qwen embedding")
            dimension = self.config.get("dimension", 1024)
            emb = QwenDenseEmbedding(api_key=api_key, dimension=dimension)
            self._dimension = dimension
            return emb
        except ImportError:
            raise ImportError(
                "Qwen embedding requires zvec[embedding] or dashscope package. "
                "Install with: pip install zvec[embedding]"
            )

    def _create_jina_dense(self):
        """Create Jina dense embedding provider."""
        try:
            from zvec.extension import JinaDenseEmbedding
            api_key = self.config.get("api_key", "")
            if not api_key:
                raise ValueError("api_key is required for Jina embedding")
            dimension = self.config.get("dimension", 1024)
            emb = JinaDenseEmbedding(api_key=api_key, dimension=dimension)
            self._dimension = dimension
            return emb
        except ImportError:
            raise ImportError(
                "Jina embedding requires zvec[embedding] or jina package."
            )

    def _create_fallback_dense(self):
        """Create a simple hash-based fallback embedding (for testing/offline)."""
        dimension = self.config.get("dimension", 384)
        self._dimension = dimension
        return _FallbackDenseEmbedding(dimension)

    # --- Sparse provider factories ---

    def _create_bm25_zh(self):
        """Create BM25 sparse embedding for Chinese text."""
        try:
            from zvec.extension import BM25EmbeddingFunction
            return BM25EmbeddingFunction(language="zh", encoding_type="query")
        except ImportError:
            raise ImportError(
                "BM25 embedding requires zvec[embedding]. "
                "Install with: pip install zvec[embedding]"
            )

    def _create_bm25_en(self):
        """Create BM25 sparse embedding for English text."""
        try:
            from zvec.extension import BM25EmbeddingFunction
            return BM25EmbeddingFunction(language="en", encoding_type="query")
        except ImportError:
            raise ImportError(
                "BM25 embedding requires zvec[embedding]. "
                "Install with: pip install zvec[embedding]"
            )

    def _create_splade(self):
        """Create SPLADE sparse embedding."""
        try:
            from zvec.extension import DefaultLocalSparseEmbedding
            return DefaultLocalSparseEmbedding()
        except ImportError:
            raise ImportError(
                "SPLADE embedding requires zvec[embedding]. "
                "Install with: pip install zvec[embedding]"
            )

    # --- Public API ---

    def embed_dense(self, text: str) -> List[float]:
        """Generate dense embedding vector for text.

        Args:
            text: Input text string

        Returns:
            List[float]: Dense vector of dimension self.dimension
        """
        if self._dense is None:
            raise RuntimeError("Dense embedding provider not initialized")

        try:
            result = self._dense.embed(text)
            if isinstance(result, list) and len(result) > 0 and isinstance(result[0], list):
                # Some providers return batch results
                return result[0]
            return result
        except Exception as e:
            logger.error("Dense embedding failed: %s", e)
            raise

    def embed_dense_batch(self, texts: List[str]) -> List[List[float]]:
        """Generate dense embedding vectors for a batch of texts.

        Args:
            texts: List of input text strings

        Returns:
            List[List[float]]: List of dense vectors
        """
        if self._dense is None:
            raise RuntimeError("Dense embedding provider not initialized")

        try:
            if hasattr(self._dense, 'embed_batch'):
                return self._dense.embed_batch(texts)
            # Fallback to individual embedding
            return [self.embed_dense(text) for text in texts]
        except Exception as e:
            logger.error("Batch dense embedding failed: %s", e)
            raise

    def embed_sparse(self, text: str) -> Optional[Dict[str, float]]:
        """Generate sparse embedding vector for text.

        Args:
            text: Input text string

        Returns:
            Dict mapping token_id/term to weight, or None if sparse is disabled
        """
        if self._sparse is None:
            return None

        try:
            result = self._sparse.embed(text)
            return result
        except Exception as e:
            logger.error("Sparse embedding failed: %s", e)
            return None

    @property
    def dimension(self) -> int:
        """Return the dimension of dense embeddings."""
        return self._dimension

    @property
    def has_sparse(self) -> bool:
        """Return whether sparse embedding is available."""
        return self._sparse is not None

    def get_info(self) -> Dict[str, Any]:
        """Return provider information for diagnostics."""
        return {
            "dense_provider": self.config.get("dense_provider", "local"),
            "sparse_provider": self.config.get("sparse_provider", "none"),
            "dimension": self._dimension,
            "has_sparse": self.has_sparse,
            "dense_type": type(self._dense).__name__ if self._dense else None,
            "sparse_type": type(self._sparse).__name__ if self._sparse else None,
        }


class _SentenceTransformerWrapper:
    """Wrapper around sentence-transformers SentenceTransformer for unified interface."""

    def __init__(self, model):
        self.model = model
        self.dimension = model.get_sentence_embedding_dimension()

    def embed(self, text: str) -> List[float]:
        """Embed a single text."""
        return self.model.encode(text).tolist()

    def embed_batch(self, texts: List[str]) -> List[List[float]]:
        """Embed a batch of texts."""
        embeddings = self.model.encode(texts)
        return [emb.tolist() for emb in embeddings]


class _FallbackDenseEmbedding:
    """Hash-based fallback embedding for offline/testing environments.

    Produces deterministic pseudo-random vectors based on text hash.
    NOT suitable for production semantic search.
    """

    def __init__(self, dimension: int = 384):
        self.dimension = dimension
        logger.warning(
            "Using FallbackDenseEmbedding (dim=%d). "
            "This is NOT suitable for production use.",
            dimension,
        )

    def embed(self, text: str) -> List[float]:
        """Generate a deterministic pseudo-random vector from text hash."""
        import hashlib
        import struct

        h = hashlib.sha256(text.encode("utf-8")).digest()
        # Expand hash to fill dimension
        result = []
        idx = 0
        while len(result) < self.dimension:
            seed = h + struct.pack("I", idx)
            h2 = hashlib.md5(seed).digest()
            # Extract 4 floats from 16-byte hash
            for i in range(0, 16, 4):
                val = struct.unpack("f", h2[i:i + 4])[0]
                # Normalize to [-1, 1]
                val = max(-1.0, min(1.0, val / 1e38))
                result.append(val)
            idx += 1

        return result[:self.dimension]

    def embed_batch(self, texts: List[str]) -> List[List[float]]:
        """Batch embed texts."""
        return [self.embed(text) for text in texts]
