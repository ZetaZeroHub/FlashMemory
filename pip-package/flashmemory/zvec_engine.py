"""
FlashMemory Zvec 向量引擎封装

基于阿里巴巴 Zvec 进程内向量数据库，替代 FAISS HTTP 服务。
提供 Collection 管理、向量 CRUD 和语义搜索能力。
"""

import os
import logging

logger = logging.getLogger("flashmemory.zvec_engine")


class ZvecEngine:
    """FlashMemory Zvec 向量引擎封装
    
    管理 Zvec Collection 的生命周期，包括：
    - 创建/打开 Collection (函数级 + 模块级)
    - 向量 upsert / delete / search
    - 标量过滤搜索
    - 索引优化和持久化
    """

    # 默认使用 all-MiniLM-L6-v2 的 384 维
    DEFAULT_DIMENSION = 384

    def __init__(self, collection_base_path: str, dimension: int = DEFAULT_DIMENSION):
        """
        Args:
            collection_base_path: Collection 存储的基础目录 (如 .gitgo/zvec_collections)
            dimension: 向量维度，默认 384 (all-MiniLM-L6-v2)
        """
        self.collection_base_path = collection_base_path
        self.dimension = dimension
        self.func_collection = None      # 函数级 Collection
        self.module_collection = None    # 模块级 Collection (code_desc)
        self._zvec = None                # 延迟导入 zvec 模块

        logger.info(
            "ZvecEngine 初始化，基础路径=%s, 维度=%d",
            collection_base_path, dimension,
        )

    def _ensure_zvec(self):
        """延迟导入 zvec 模块，避免未安装时立即报错"""
        if self._zvec is None:
            try:
                import zvec
                self._zvec = zvec
                logger.info("zvec 模块加载成功, 版本=%s", getattr(zvec, '__version__', 'unknown'))
            except ImportError as e:
                raise ImportError(
                    "zvec 未安装。请执行 pip install zvec 安装。"
                ) from e
        return self._zvec

    def _get_func_collection_path(self):
        """获取函数级 Collection 路径"""
        return os.path.join(self.collection_base_path, "functions")

    def _get_module_collection_path(self):
        """获取模块级 Collection 路径"""
        return os.path.join(self.collection_base_path, "modules")

    def _purge_stale_locks(self, path: str) -> int:
        """Recursively remove all LOCK files under a collection directory.

        zvec/RocksDB layouts have nested LOCK files (e.g. ``functions/LOCK``,
        ``functions/idmap.0/LOCK``, ``functions/0/scalar.index.X.rocksdb/LOCK``).
        Removing only the top-level LOCK is not enough — when the second
        ``zvec.open()`` attempt walks into a sub-directory it hits the nested
        LOCK and surfaces a different error like "Can't open lock file".

        Returns the count of LOCK files removed (for logging/diagnostics).
        """
        if not os.path.isdir(path):
            return 0
        removed = 0
        for root, _dirs, files in os.walk(path):
            for fname in files:
                if fname == "LOCK":
                    full = os.path.join(root, fname)
                    try:
                        os.remove(full)
                        removed += 1
                        logger.info("Removed stale LOCK file: %s", full)
                    except FileNotFoundError:
                        pass
                    except OSError as rm_err:
                        logger.warning("Failed to remove %s: %s", full, rm_err)
        # Force fsync of the collection directory so subsequent open() sees a
        # fully consistent state (some filesystems delay parent-dir mtime).
        try:
            dir_fd = os.open(path, os.O_RDONLY)
            try:
                os.fsync(dir_fd)
            finally:
                os.close(dir_fd)
        except OSError:
            pass
        return removed

    # Recoverable-error patterns that justify nuking & rebuilding the collection.
    # Covers fcntl LOCK problems (left-over locks, RocksDB sub-LOCKs) AND
    # internal-state corruption (incomplete WAL, missing LDB segments after
    # a bridge crash). The user shouldn't be permanently locked out for either.
    _RECOVERABLE_ERROR_FRAGMENTS = (
        "lock",            # "Can't lock", "Can't open lock file"
        "recovery",        # "recovery idmap failed", RocksDB recovery loop
        "corrupt",         # generic corruption messages
        "no such file",    # missing .ldb / WAL segment after crash
        "checksum",        # SST file checksum mismatch
        "manifest",        # broken MANIFEST file
        "idmap",           # idmap subdirectory issues
        "segment",         # "Segment open failed: segment path not found [.../N]"
    )

    @classmethod
    def _is_recoverable_open_error(cls, msg: str) -> bool:
        """Decide whether an open-time error warrants attempt 3 (rebuild)."""
        if not msg:
            return False
        lower = msg.lower()
        return any(frag in lower for frag in cls._RECOVERABLE_ERROR_FRAGMENTS)

    def _try_open_collection(self, zvec_module, path: str, force_new: bool, create_func):
        """Open or create a zvec collection with LOCK conflict auto-recovery.

        Three-tier recovery:
          attempt 1: open as-is
          attempt 2: purge all nested LOCK files + retry open
          attempt 3 (LAST RESORT): treat collection as corrupt, recreate from scratch

        attempt 3 is destructive — the existing collection data is wiped. We only
        reach it when attempts 1/2 fail with errors that belong to the
        ``_RECOVERABLE_ERROR_FRAGMENTS`` set (LOCK contention, RocksDB internal
        state corruption such as ``recovery idmap failed`` or missing LDB
        segments). Without it, a user is permanently locked out of their
        collection. The trade-off was made explicit at FM team request:
        prefer recoverable-but-empty over forever-broken.

        Exception types we catch:
          - RuntimeError  — most RocksDB errors (Can't lock, recovery, ...)
          - ValueError    — zvec 0.3.x wraps IOError as ValueError
                            (e.g. "Failed to open local file '.../scalar.0.ipc'
                            [errno 2] No such file or directory" after concurrent
                            BuildIndex stomped the SST/IPC files)
          - OSError       — defensive: low-level filesystem errors
        Anything outside these classes (config error, dimension mismatch, etc.)
        propagates up so we don't silently destroy data on unrelated bugs.
        """
        import time
        import shutil
        recoverable_excs = (RuntimeError, ValueError, OSError)
        max_retries = 3
        for attempt in range(max_retries):
            try:
                if os.path.exists(path) and not force_new:
                    logger.info("Opening existing collection: %s (attempt %d)", path, attempt + 1)
                    return zvec_module.open(path=path)
                else:
                    logger.info("Creating new collection: %s (attempt %d)", path, attempt + 1)
                    os.makedirs(os.path.dirname(path), exist_ok=True)
                    return create_func()
            except recoverable_excs as e:
                msg = str(e)
                recoverable = self._is_recoverable_open_error(msg)

                if not recoverable or attempt >= max_retries - 1:
                    # Either non-recoverable, or we already burned attempt 3.
                    raise

                if attempt == 0:
                    # Tier 1 → Tier 2: purge all nested LOCK files.
                    logger.warning(
                        "Recoverable open error on %s (attempt 1): %s — purging all nested LOCK files",
                        path, msg,
                    )
                    removed = self._purge_stale_locks(path)
                    logger.info("Purged %d stale LOCK files under %s", removed, path)
                    time.sleep(0.1)  # let APFS/ext4 flush dirent updates
                    continue

                # attempt == 1: tier 2 failed too. Could be LOCK still wedged
                # OR internal RocksDB state corruption (recovery idmap, missing
                # LDB, broken MANIFEST). Either way, the only path forward is
                # to wipe and recreate; force_new will be honored on next loop.
                logger.error(
                    "Open still failing after LOCK purge (attempt 2): %s. "
                    "Treating collection at %s as corrupt and rebuilding from scratch. "
                    "EXISTING DATA IN THIS COLLECTION WILL BE LOST.",
                    msg, path,
                )
                try:
                    shutil.rmtree(path, ignore_errors=True)
                except Exception as rm_err:
                    logger.warning("Failed to rmtree %s: %s — recreate may still fail", path, rm_err)
                force_new = True
                time.sleep(0.1)
                continue

    def init_func_collection(self, force_new: bool = False):
        """Create or open function-level Collection

        Schema:
        - Vector: dense_embedding (FP32, HNSW+Cosine)
        - Scalar: func_name, package, file_path, description, func_type,
                 language, start_line, end_line, importance_score
        """
        zvec = self._ensure_zvec()
        path = self._get_func_collection_path()

        def _create_func():
            schema = zvec.CollectionSchema(
                name="flashmemory_functions",
                fields=[
                    zvec.FieldSchema(
                        name="func_name",
                        data_type=zvec.DataType.STRING,
                        index_param=zvec.InvertIndexParam(),
                    ),
                    zvec.FieldSchema(
                        name="package",
                        data_type=zvec.DataType.STRING,
                        index_param=zvec.InvertIndexParam(),
                    ),
                    zvec.FieldSchema(
                        name="file_path",
                        data_type=zvec.DataType.STRING,
                        index_param=zvec.InvertIndexParam(),
                    ),
                    zvec.FieldSchema(
                        name="description",
                        data_type=zvec.DataType.STRING,
                    ),
                    zvec.FieldSchema(
                        name="func_type",
                        data_type=zvec.DataType.STRING,
                        index_param=zvec.InvertIndexParam(),
                    ),
                    zvec.FieldSchema(
                        name="language",
                        data_type=zvec.DataType.STRING,
                        index_param=zvec.InvertIndexParam(),
                    ),
                    zvec.FieldSchema(
                        name="start_line",
                        data_type=zvec.DataType.INT32,
                    ),
                    zvec.FieldSchema(
                        name="end_line",
                        data_type=zvec.DataType.INT32,
                    ),
                    zvec.FieldSchema(
                        name="importance_score",
                        data_type=zvec.DataType.DOUBLE,
                    ),
                ],
                vectors=[
                    zvec.VectorSchema(
                        name="dense_embedding",
                        data_type=zvec.DataType.VECTOR_FP32,
                        dimension=self.dimension,
                        index_param=zvec.HnswIndexParam(
                            metric_type=zvec.MetricType.COSINE,
                        ),
                    ),
                    # Phase 2: Sparse vector for keyword-level matching (BM25/SPLADE)
                    zvec.VectorSchema(
                        name="sparse_embedding",
                        data_type=zvec.DataType.SPARSE_VECTOR_FP32,
                    ),
                ],
            )
            self.func_collection = zvec.create_and_open(
                path=path,
                schema=schema,
            )
            return self.func_collection

        self.func_collection = self._try_open_collection(zvec, path, force_new, _create_func)
        logger.info("Function collection ready, stats=%s", self.func_collection.stats)
        return self.func_collection

    def init_module_collection(self, force_new: bool = False):
        """Create or open module-level Collection (code_desc)

        Schema:
        - Vector: dense_embedding (FP32, HNSW+Cosine)
        - Scalar: name, type, path, parent_path, description
        """
        zvec = self._ensure_zvec()
        path = self._get_module_collection_path()

        def _create_module():
            schema = zvec.CollectionSchema(
                name="flashmemory_modules",
                fields=[
                    zvec.FieldSchema(
                        name="name",
                        data_type=zvec.DataType.STRING,
                        index_param=zvec.InvertIndexParam(),
                    ),
                    zvec.FieldSchema(
                        name="type",
                        data_type=zvec.DataType.STRING,
                        index_param=zvec.InvertIndexParam(),
                    ),
                    zvec.FieldSchema(
                        name="path",
                        data_type=zvec.DataType.STRING,
                        index_param=zvec.InvertIndexParam(),
                    ),
                    zvec.FieldSchema(
                        name="parent_path",
                        data_type=zvec.DataType.STRING,
                        index_param=zvec.InvertIndexParam(),
                    ),
                    zvec.FieldSchema(
                        name="description",
                        data_type=zvec.DataType.STRING,
                    ),
                ],
                vectors=[
                    zvec.VectorSchema(
                        name="dense_embedding",
                        data_type=zvec.DataType.VECTOR_FP32,
                        dimension=self.dimension,
                        index_param=zvec.HnswIndexParam(
                            metric_type=zvec.MetricType.COSINE,
                        ),
                    ),
                ],
            )
            self.module_collection = zvec.create_and_open(
                path=path,
                schema=schema,
            )
            return self.module_collection

        self.module_collection = self._try_open_collection(zvec, path, force_new, _create_module)
        logger.info("Module collection ready, stats=%s", self.module_collection.stats)
        return self.module_collection

    def upsert_function(self, func_id: str, embedding: list, metadata: dict,
                        sparse_embedding: dict = None):
        """Add or update function vector and metadata
        
        Args:
            func_id: Unique identifier (e.g. "func_42")
            embedding: Dense vector list (length should equal self.dimension)
            metadata: Scalar field dict (func_name, package, file_path, etc.)
            sparse_embedding: Optional sparse vector dict {token_id: weight}.
                If None, auto-generates from description using BM25.
        """
        if self.func_collection is None:
            raise RuntimeError("函数 Collection 未初始化，请先调用 init_func_collection()")

        zvec = self._ensure_zvec()
        vectors = {"dense_embedding": embedding}

        # Auto-generate BM25 sparse from description if not provided
        if not sparse_embedding:
            desc_text = metadata.get("description", "")
            func_name = metadata.get("func_name", "")
            if desc_text or func_name:
                sparse_text = f"{func_name} {desc_text}".strip()
                sparse_embedding = self._generate_bm25_sparse(sparse_text, encoding_type="document")

        vectors["sparse_embedding"] = sparse_embedding if sparse_embedding else {}

        doc = zvec.Doc(
            id=func_id,
            vectors=vectors,
            fields=metadata,
        )
        self.func_collection.upsert(docs=[doc])
        logger.debug("Upserted function: %s (sparse=%s)", func_id, bool(sparse_embedding))

    def upsert_functions_batch(self, items: list):
        """Batch upsert functions
        
        Args:
            items: list of (func_id: str, embedding: list, metadata: dict[, sparse_embedding: dict])
        """
        if self.func_collection is None:
            raise RuntimeError("函数 Collection 未初始化")

        zvec = self._ensure_zvec()
        docs = []
        for item in items:
            func_id = item[0]
            embedding = item[1]
            metadata = item[2]
            sparse_emb = item[3] if len(item) > 3 else None

            # Auto-generate BM25 sparse from description if not provided
            if not sparse_emb:
                desc_text = metadata.get("description", "")
                func_name = metadata.get("func_name", "")
                if desc_text or func_name:
                    sparse_text = f"{func_name} {desc_text}".strip()
                    sparse_emb = self._generate_bm25_sparse(sparse_text, encoding_type="document")

            vectors = {"dense_embedding": embedding}
            vectors["sparse_embedding"] = sparse_emb if sparse_emb else {}

            docs.append(zvec.Doc(
                id=func_id,
                vectors=vectors,
                fields=metadata,
            ))

        self.func_collection.upsert(docs=docs)
        logger.info("Batch upserted %d functions (with BM25 sparse)", len(docs))

    def upsert_module(self, module_id: str, embedding: list, metadata: dict):
        """新增或更新模块向量和元数据"""
        if self.module_collection is None:
            raise RuntimeError("模块 Collection 未初始化")

        zvec = self._ensure_zvec()
        doc = zvec.Doc(
            id=module_id,
            vectors={"dense_embedding": embedding},
            fields=metadata,
        )
        self.module_collection.upsert(docs=[doc])
        logger.debug("已 upsert 模块: %s", module_id)

    def _is_collection_empty(self, collection) -> bool:
        """Check if a zvec collection has zero documents"""
        if collection is None:
            return True
        try:
            stats = collection.stats
            if hasattr(stats, 'doc_count') and stats.doc_count == 0:
                return True
            if isinstance(stats, dict) and stats.get('doc_count', 0) == 0:
                return True
        except Exception:
            pass
        return False

    def search_functions(self, query_vector: list, top_k: int = 10,
                         filter_expr: str = None):
        """Search for similar function vectors

        Args:
            query_vector: Query vector
            top_k: Number of results to return
            filter_expr: Optional scalar filter expression, e.g. 'language = "go"'

        Returns:
            list of Doc: Search results (empty list if collection is empty)
        """
        if self.func_collection is None:
            raise RuntimeError("Function Collection not initialized")

        if self._is_collection_empty(self.func_collection):
            logger.info("Function collection is empty, returning no results")
            return []

        zvec = self._ensure_zvec()
        query_params = {
            "vectors": [
                zvec.VectorQuery("dense_embedding", vector=query_vector),
            ],
            "topk": top_k,
        }
        if filter_expr:
            query_params["filter"] = filter_expr
            logger.info("带过滤搜索: filter=%s", filter_expr)

        results = self.func_collection.query(**query_params)
        logger.info("Function search returned %d results", len(results))
        return results

    def hybrid_search(self, dense_vector: list, sparse_vector: dict = None,
                      top_k: int = 10, filter_expr: str = None,
                      use_rrf: bool = True, rrf_topn: int = None,
                      collection_type: str = "functions",
                      query_text: str = None,
                      enable_reranker: bool = False,
                      reranker_type: str = "rrf",
                      weighted_params: list = None,
                      recall_multiplier: int = 5):
        """Hybrid search with Dense + Sparse multi-vector query and RRF/Weighted fusion

        Args:
            dense_vector: Dense query vector
            sparse_vector: Sparse query vector dict {token_id: weight}
            top_k: Number of results to return
            filter_expr: Optional scalar filter expression
            use_rrf: Whether to use RRF fusion (when both vectors provided)
            rrf_topn: Top-N for RRF reranker (defaults to top_k)
            collection_type: "functions" or "modules"
            query_text: Original query text for auto BM25 sparse generation and reranker
            enable_reranker: Whether to use DefaultLocalReRanker cross-encoder
            reranker_type: "rrf" (default) or "weighted" for multi-vector fusion
            weighted_params: Weights list for WeightedReRanker, e.g. [0.7, 0.3]
            recall_multiplier: Multiplier for recall stage when reranker is enabled (default 5)

        Returns:
            list of Doc: Search results with fused scores
        """
        collection = self.module_collection if collection_type == "modules" else self.func_collection
        if collection is None:
            raise RuntimeError(f"{collection_type} Collection not initialized")

        if self._is_collection_empty(collection):
            logger.info("%s collection is empty, returning no results", collection_type)
            return []

        zvec = self._ensure_zvec()

        # Auto-generate BM25 sparse vector from query_text if no sparse_vector provided
        if sparse_vector is None and query_text and collection_type == "functions":
            sparse_vector = self._generate_bm25_sparse(query_text, encoding_type="query")

        # Build multi-vector query
        query_vectors = [
            zvec.VectorQuery("dense_embedding", vector=dense_vector),
        ]
        if sparse_vector:
            query_vectors.append(
                zvec.VectorQuery("sparse_embedding", vector=sparse_vector),
            )
            logger.info("Hybrid search: Dense + Sparse, reranker_type=%s", reranker_type)
        else:
            logger.info("Hybrid search: Dense only (no sparse vector)")

        # Determine actual recall count (expand for cross-encoder reranking)
        actual_topk = top_k * recall_multiplier if enable_reranker else top_k

        query_params = {
            "vectors": query_vectors,
            "topk": actual_topk,
        }

        # Apply fusion reranker for multi-vector queries
        if len(query_vectors) > 1:
            fusion_reranker = self._create_fusion_reranker(
                reranker_type=reranker_type,
                topn=rrf_topn or actual_topk,
                use_rrf=use_rrf,
                weighted_params=weighted_params,
            )
            if fusion_reranker:
                query_params["reranker"] = fusion_reranker

        if filter_expr:
            query_params["filter"] = filter_expr
            logger.info("Hybrid search with filter: %s", filter_expr)

        results = collection.query(**query_params)
        logger.info("Hybrid search recall: %d candidates (actual_topk=%d)", len(results), actual_topk)

        # Stage 2: Cross-encoder reranking (if enabled)
        if enable_reranker and query_text and len(results) > top_k:
            results = self._apply_cross_encoder_reranker(
                query_text=query_text,
                results=results,
                top_k=top_k,
                rerank_field="description",
            )

        return results

    def search_modules(self, query_vector: list, top_k: int = 10,
                       filter_expr: str = None):
        """Search for similar module vectors"""
        if self.module_collection is None:
            raise RuntimeError("Module Collection not initialized")

        if self._is_collection_empty(self.module_collection):
            logger.info("Module collection is empty, returning no results")
            return []

        zvec = self._ensure_zvec()
        query_params = {
            "vectors": [
                zvec.VectorQuery("dense_embedding", vector=query_vector),
            ],
            "topk": top_k,
        }
        if filter_expr:
            query_params["filter"] = filter_expr

        results = self.module_collection.query(**query_params)
        logger.info("Module search returned %d results", len(results))
        return results

    # --- AI Extension helper methods ---

    def _generate_bm25_sparse(self, text: str, encoding_type: str = "document") -> dict:
        """Generate BM25 sparse vector using zvec built-in BM25EmbeddingFunction.

        Auto-initializes BM25 on first call. Falls back to empty dict on failure.
        Caches failure state to avoid retrying and spamming logs.

        Args:
            text: Text content to generate sparse embedding for
            encoding_type: "document" for indexing, "query" for searching

        Returns:
            dict: Sparse vector {dimension_index: weight}
        """
        if not hasattr(self, '_bm25_cache'):
            self._bm25_cache = {}

        cache_key = f"bm25_{encoding_type}"

        # Check if we already failed for this encoding_type (sentinel: None)
        if cache_key in self._bm25_cache and self._bm25_cache[cache_key] is None:
            return {}

        if cache_key not in self._bm25_cache:
            try:
                from zvec.extension import BM25EmbeddingFunction
                self._bm25_cache[cache_key] = BM25EmbeddingFunction(
                    language="zh", encoding_type=encoding_type,
                )
                logger.info("BM25 embedding initialized: encoding_type=%s", encoding_type)
            except Exception as e:
                # Cache failure state (sentinel=None) to avoid retrying on every call
                self._bm25_cache[cache_key] = None
                logger.warning(
                    "BM25 sparse vectors disabled: %s: %s. "
                    "Install missing deps: pip install dashtext jieba rank_bm25",
                    type(e).__name__, e,
                )
                return {}

        try:
            sparse = self._bm25_cache[cache_key].embed(text)
            logger.debug("BM25 sparse generated: %d non-zero dims", len(sparse) if sparse else 0)
            return sparse or {}
        except Exception as e:
            logger.warning("BM25 embed failed: %s", e)
            return {}

    def _create_fusion_reranker(self, reranker_type: str = "rrf", topn: int = 10,
                                use_rrf: bool = True, weighted_params: list = None):
        """Create a fusion reranker for multi-vector queries.

        Args:
            reranker_type: "rrf" or "weighted"
            topn: Top-N results for the reranker
            use_rrf: Legacy flag, True means use RRF
            weighted_params: Weight list for WeightedReRanker, e.g. [0.7, 0.3]

        Returns:
            Reranker instance or None if not available
        """
        try:
            if reranker_type == "weighted" and weighted_params:
                from zvec.extension import WeightedReRanker
                reranker = WeightedReRanker(weights=weighted_params, topn=topn)
                logger.info("WeightedReRanker enabled: weights=%s, topn=%d", weighted_params, topn)
                return reranker
            elif use_rrf or reranker_type == "rrf":
                from zvec.extension import RrfReRanker
                reranker = RrfReRanker(topn=topn)
                logger.info("RRF reranker enabled, topn=%d", topn)
                return reranker
        except ImportError as e:
            logger.warning("Fusion reranker not available: %s, fallback to default", e)
        except Exception as e:
            logger.warning("Fusion reranker creation failed: %s", e)
        return None

    def _apply_cross_encoder_reranker(self, query_text: str, results: list,
                                       top_k: int, rerank_field: str = "description"):
        """Apply DefaultLocalReRanker cross-encoder for precise re-ranking.

        Stage 2 of two-stage retrieval. Uses cross-encoder model to compute
        pairwise relevance between query and each candidate.

        Args:
            query_text: Original search query text
            results: Candidate results from Stage 1 (recall)
            top_k: Number of final results to return
            rerank_field: Field in doc.fields to use for reranking

        Returns:
            list: Re-ranked and truncated results
        """
        try:
            from zvec.extension import DefaultLocalReRanker

            reranker = DefaultLocalReRanker(
                query=query_text,
                topn=top_k,
                rerank_field=rerank_field,
            )

            # DefaultLocalReRanker expects {vector_name: [Doc, ...]} format
            # Wrap all results under a single key
            from zvec import Doc
            doc_groups = {"recall": []}
            for doc in results:
                doc_groups["recall"].append(doc)

            reranked = reranker.rerank(doc_groups)
            logger.info(
                "Cross-encoder reranker: %d candidates → %d results",
                len(results), len(reranked),
            )
            return reranked[:top_k]

        except ImportError:
            logger.warning("DefaultLocalReRanker not available, skipping cross-encoder reranking")
            return results[:top_k]
        except Exception as e:
            logger.error("Cross-encoder reranking failed: %s, returning recall results", e)
            return results[:top_k]

    def delete_by_file(self, file_path: str):
        """按文件路径删除函数向量（用于增量更新）"""
        if self.func_collection is None:
            return

        self.func_collection.delete_by_filter(
            filter=f'file_path = "{file_path}"'
        )
        logger.info("已删除文件 %s 的所有函数向量", file_path)

    def delete_function(self, func_id: str):
        """按 ID 删除单个函数向量"""
        if self.func_collection is None:
            return

        self.func_collection.delete(ids=func_id)
        logger.debug("已删除函数: %s", func_id)

    def optimize(self):
        """优化所有 Collection 的索引性能"""
        if self.func_collection:
            self.func_collection.optimize()
            logger.info("函数 Collection 已优化")
        if self.module_collection:
            self.module_collection.optimize()
            logger.info("模块 Collection 已优化")

    def get_stats(self) -> dict:
        """获取 Collection 统计信息"""
        stats = {}
        if self.func_collection:
            stats["functions"] = str(self.func_collection.stats)
        if self.module_collection:
            stats["modules"] = str(self.module_collection.stats)
        return stats

    def close(self):
        """Close all collections, flush pending data, and release LOCK files"""
        for name, attr_name in [("func_collection", "func_collection"), ("module_collection", "module_collection")]:
            coll = getattr(self, attr_name, None)
            if coll is not None:
                try:
                    coll.flush()
                    coll.close()
                    logger.info("%s flushed and closed (LOCK released)", name)
                except AttributeError:
                    # zvec < 0.3 may not have close(), flush is sufficient
                    try:
                        coll.flush()
                        logger.info("%s flushed (close() not available, LOCK may persist)", name)
                    except Exception as e:
                        logger.warning("Failed to flush %s: %s", name, e)
                except Exception as e:
                    logger.warning("Failed to close %s: %s", name, e)
                finally:
                    setattr(self, attr_name, None)
        logger.info("ZvecEngine closed, all LOCK files should be released")
