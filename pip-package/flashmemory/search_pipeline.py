"""
FlashMemory Search Pipeline

Two-stage retrieval pipeline:
  Stage 1 (Recall): Dense + Sparse multi-vector search with RRF fusion
  Stage 2 (Rerank): Cross-encoder reranking for precision

Designed to work with ZvecEngine and EmbeddingProvider.
"""

import logging
from typing import List, Dict, Optional, Any

logger = logging.getLogger("flashmemory.search_pipeline")


class SearchResult:
    """Unified search result representation."""

    def __init__(self, doc_id: str, score: float, fields: Dict[str, Any] = None):
        self.doc_id = doc_id
        self.score = score
        self.fields = fields or {}

    def to_dict(self) -> Dict[str, Any]:
        return {
            "id": self.doc_id,
            "score": self.score,
            "fields": self.fields,
        }

    def __repr__(self):
        name = self.fields.get("func_name", self.doc_id)
        return f"SearchResult(id={self.doc_id}, score={self.score:.4f}, name={name})"


class SearchPipeline:
    """Two-stage search pipeline: Recall → Rerank.

    Stage 1 (Recall):
        - Generate dense embedding from query text
        - Optionally generate sparse embedding
        - Multi-vector search in Zvec collection
        - RRF fusion when using both dense + sparse

    Stage 2 (Rerank, optional):
        - Cross-encoder reranking on recall candidates
        - Improves precision for top-K results

    Usage:
        from flashmemory.embedding_provider import EmbeddingProvider
        from flashmemory.zvec_engine import ZvecEngine
        from flashmemory.search_pipeline import SearchPipeline

        engine = ZvecEngine("/path/to/collections")
        engine.init_func_collection()

        provider = EmbeddingProvider({"dense_provider": "local"})
        pipeline = SearchPipeline(engine, provider)

        results = pipeline.search("file upload handler", top_k=10)
    """

    def __init__(
        self,
        engine,
        embedding_provider,
        config: Dict[str, Any] = None,
    ):
        """Initialize search pipeline.

        Args:
            engine: ZvecEngine instance with initialized collections
            embedding_provider: EmbeddingProvider instance
            config: Optional pipeline config with keys:
                - enable_reranker: bool (default False)
                - reranker_topn: int (default 10)
                - recall_multiplier: int (default 5, recall = top_k * multiplier)
                - rerank_field: str (default "description")
                - use_rrf: bool (default True)
        """
        self.engine = engine
        self.emb = embedding_provider
        self.config = config or {}

        self._reranker = None
        if self.config.get("enable_reranker", False):
            self._init_reranker()

        logger.info(
            "SearchPipeline initialized: reranker=%s, recall_multiplier=%d",
            self._reranker is not None,
            self.config.get("recall_multiplier", 5),
        )

    def _init_reranker(self):
        """Initialize reranker if available."""
        try:
            from zvec.extension import DefaultLocalReRanker
            self._reranker = DefaultLocalReRanker
            logger.info("Reranker available: DefaultLocalReRanker")
        except ImportError:
            logger.warning(
                "Reranker not available (zvec.extension.DefaultLocalReRanker not found). "
                "Reranking will be skipped."
            )
            self._reranker = None

    def search(
        self,
        query: str,
        top_k: int = 10,
        filter_expr: str = None,
        use_reranker: bool = None,
        search_type: str = "functions",
    ) -> List[SearchResult]:
        """Execute two-stage search pipeline.

        Args:
            query: Natural language search query
            top_k: Number of final results to return
            filter_expr: Optional scalar filter (e.g. 'language = "go"')
            use_reranker: Override pipeline config for reranker usage
            search_type: "functions" or "modules"

        Returns:
            List[SearchResult]: Ranked search results
        """
        should_rerank = use_reranker if use_reranker is not None else self.config.get("enable_reranker", False)
        recall_multiplier = self.config.get("recall_multiplier", 5)
        use_rrf = self.config.get("use_rrf", True)

        # Determine recall count
        recall_count = top_k * recall_multiplier if should_rerank and self._reranker else top_k

        logger.info(
            "Search pipeline: query='%s', top_k=%d, recall=%d, rerank=%s, filter=%s",
            query[:50], top_k, recall_count, should_rerank, filter_expr,
        )

        # ---- Stage 1: Recall ----
        dense_vec = self.emb.embed_dense(query)
        sparse_vec = self.emb.embed_sparse(query) if self.emb.has_sparse else None

        if search_type == "modules":
            raw_results = self._recall_modules(dense_vec, recall_count, filter_expr)
        else:
            raw_results = self._recall_functions(
                dense_vec, sparse_vec, recall_count, filter_expr, use_rrf,
                query_text=query,
                enable_reranker=should_rerank,
                final_top_k=top_k,
            )

        logger.info("Stage 1 (Recall): %d candidates retrieved", len(raw_results))

        if not raw_results:
            return []

        # ---- Stage 2: Rerank ----
        # If engine-level reranker was used (via hybrid_search), results are already reranked
        if should_rerank and self._reranker and len(raw_results) > top_k:
            # Fallback: pipeline-level reranking if engine didn't handle it
            results = self._rerank(query, raw_results, top_k)
            logger.info("Stage 2 (Rerank): %d results after pipeline reranking", len(results))
        else:
            results = raw_results[:top_k]

        return results

    def _recall_functions(
        self,
        dense_vec: List[float],
        sparse_vec: Optional[Dict],
        recall_count: int,
        filter_expr: str,
        use_rrf: bool,
        query_text: str = None,
        enable_reranker: bool = False,
        final_top_k: int = None,
    ) -> List[SearchResult]:
        """Stage 1: Function-level recall using hybrid search.

        When query_text is provided, the engine can auto-generate BM25 sparse vectors
        and optionally apply cross-encoder reranking.
        """
        if sparse_vec and self.emb.has_sparse:
            # Hybrid search: Dense + Sparse + RRF
            raw_docs = self.engine.hybrid_search(
                dense_vector=dense_vec,
                sparse_vector=sparse_vec,
                top_k=final_top_k or recall_count,
                filter_expr=filter_expr,
                use_rrf=use_rrf,
                query_text=query_text,
                enable_reranker=enable_reranker,
            )
        elif query_text:
            # Dense + auto BM25 sparse from query_text
            raw_docs = self.engine.hybrid_search(
                dense_vector=dense_vec,
                sparse_vector=None,  # Engine auto-generates from query_text
                top_k=final_top_k or recall_count,
                filter_expr=filter_expr,
                use_rrf=use_rrf,
                query_text=query_text,
                enable_reranker=enable_reranker,
            )
        else:
            # Dense-only search
            raw_docs = self.engine.search_functions(
                query_vector=dense_vec,
                top_k=recall_count,
                filter_expr=filter_expr,
            )

        return self._docs_to_results(raw_docs)

    def _recall_modules(
        self,
        dense_vec: List[float],
        recall_count: int,
        filter_expr: str,
    ) -> List[SearchResult]:
        """Stage 1: Module-level recall (dense only)."""
        raw_docs = self.engine.search_modules(
            query_vector=dense_vec,
            top_k=recall_count,
            filter_expr=filter_expr,
        )
        return self._docs_to_results(raw_docs)

    def _docs_to_results(self, docs) -> List[SearchResult]:
        """Convert zvec Doc objects to SearchResult list."""
        results = []
        for doc in docs:
            result = SearchResult(
                doc_id=doc.id if hasattr(doc, "id") else str(doc),
                score=doc.score if hasattr(doc, "score") else 0.0,
                fields=doc.fields if hasattr(doc, "fields") else {},
            )
            results.append(result)
        return results

    def _rerank(
        self,
        query: str,
        candidates: List[SearchResult],
        top_k: int,
    ) -> List[SearchResult]:
        """Stage 2: Cross-encoder reranking."""
        rerank_field = self.config.get("rerank_field", "description")

        try:
            reranker = self._reranker(
                query=query,
                topn=top_k,
                rerank_field=rerank_field,
            )

            # Prepare documents for reranking
            # zvec reranker expects {name: [docs]} format
            doc_groups = {}
            for i, result in enumerate(candidates):
                key = f"candidate_{i}"
                # Create a minimal doc-like object
                doc_groups[key] = [result]

            reranked = reranker.rerank(doc_groups)

            # Convert back to SearchResult list
            reranked_results = []
            for doc in reranked:
                if isinstance(doc, SearchResult):
                    reranked_results.append(doc)
                else:
                    reranked_results.append(SearchResult(
                        doc_id=doc.id if hasattr(doc, "id") else str(doc),
                        score=doc.score if hasattr(doc, "score") else 0.0,
                        fields=doc.fields if hasattr(doc, "fields") else {},
                    ))

            return reranked_results[:top_k]
        except Exception as e:
            logger.error("Reranking failed: %s. Returning recall results.", e)
            return candidates[:top_k]

    def search_with_context(
        self,
        query: str,
        top_k: int = 10,
        language: str = None,
        package: str = None,
        file_path: str = None,
        func_type: str = None,
    ) -> List[SearchResult]:
        """Convenience method: search with structured context filters.

        Builds filter expression from keyword arguments.

        Args:
            query: Natural language query
            top_k: Number of results
            language: Filter by programming language ("go", "python", etc.)
            package: Filter by package name
            file_path: Filter by file path
            func_type: Filter by function type ("function", "method", etc.)

        Returns:
            List[SearchResult]: Filtered and ranked results
        """
        filters = []
        if language:
            filters.append(f'language = "{language}"')
        if package:
            filters.append(f'package = "{package}"')
        if file_path:
            filters.append(f'file_path = "{file_path}"')
        if func_type:
            filters.append(f'func_type = "{func_type}"')

        filter_expr = " AND ".join(filters) if filters else None
        return self.search(query, top_k=top_k, filter_expr=filter_expr)

    def get_pipeline_info(self) -> Dict[str, Any]:
        """Return pipeline configuration info for diagnostics."""
        return {
            "embedding": self.emb.get_info(),
            "pipeline_config": self.config,
            "reranker_available": self._reranker is not None,
            "engine_type": type(self.engine).__name__,
        }
