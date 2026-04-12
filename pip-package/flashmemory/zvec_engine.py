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

    def init_func_collection(self, force_new: bool = False):
        """创建或打开函数级 Collection
        
        Schema:
        - 向量: dense_embedding (FP32, HNSW+Cosine)
        - 标量: func_name, package, file_path, description, func_type, 
                 language, start_line, end_line, importance_score
        """
        zvec = self._ensure_zvec()
        path = self._get_func_collection_path()

        if os.path.exists(path) and not force_new:
            logger.info("打开已有函数 Collection: %s", path)
            self.func_collection = zvec.open(path=path)
        else:
            logger.info("创建新函数 Collection: %s", path)
            os.makedirs(os.path.dirname(path), exist_ok=True)

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

        logger.info("函数 Collection 就绪, stats=%s", self.func_collection.stats)
        return self.func_collection

    def init_module_collection(self, force_new: bool = False):
        """创建或打开模块级 Collection (code_desc)
        
        Schema:
        - 向量: dense_embedding (FP32, HNSW+Cosine)
        - 标量: name, type, path, parent_path, description
        """
        zvec = self._ensure_zvec()
        path = self._get_module_collection_path()

        if os.path.exists(path) and not force_new:
            logger.info("打开已有模块 Collection: %s", path)
            self.module_collection = zvec.open(path=path)
        else:
            logger.info("创建新模块 Collection: %s", path)
            os.makedirs(os.path.dirname(path), exist_ok=True)

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

        logger.info("模块 Collection 就绪, stats=%s", self.module_collection.stats)
        return self.module_collection

    def upsert_function(self, func_id: str, embedding: list, metadata: dict,
                        sparse_embedding: dict = None):
        """Add or update function vector and metadata
        
        Args:
            func_id: Unique identifier (e.g. "func_42")
            embedding: Dense vector list (length should equal self.dimension)
            metadata: Scalar field dict (func_name, package, file_path, etc.)
            sparse_embedding: Optional sparse vector dict {token_id: weight}
        """
        if self.func_collection is None:
            raise RuntimeError("函数 Collection 未初始化，请先调用 init_func_collection()")

        zvec = self._ensure_zvec()
        vectors = {"dense_embedding": embedding}
        if sparse_embedding:
            vectors["sparse_embedding"] = sparse_embedding

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

            vectors = {"dense_embedding": embedding}
            if sparse_emb:
                vectors["sparse_embedding"] = sparse_emb

            docs.append(zvec.Doc(
                id=func_id,
                vectors=vectors,
                fields=metadata,
            ))

        self.func_collection.upsert(docs=docs)
        logger.info("Batch upserted %d functions", len(docs))

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

    def search_functions(self, query_vector: list, top_k: int = 10,
                         filter_expr: str = None):
        """在函数 Collection 中进行向量相似度搜索
        
        Args:
            query_vector: 查询向量
            top_k: 返回结果数
            filter_expr: 可选的标量过滤表达式，如 'language = "go"'
        
        Returns:
            list of Doc: 搜索结果列表
        """
        if self.func_collection is None:
            raise RuntimeError("函数 Collection 未初始化")

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

    def hybrid_search_functions(self, dense_vector: list, sparse_vector: dict = None,
                                top_k: int = 10, filter_expr: str = None,
                                use_rrf: bool = True, rrf_topn: int = None):
        """Hybrid search with Dense + Sparse multi-vector query and RRF fusion
        
        Args:
            dense_vector: Dense query vector
            sparse_vector: Sparse query vector dict {token_id: weight}
            top_k: Number of results to return
            filter_expr: Optional scalar filter expression
            use_rrf: Whether to use RRF fusion (when both vectors provided)
            rrf_topn: Top-N for RRF reranker (defaults to top_k)
        
        Returns:
            list of Doc: Search results with fused scores
        """
        if self.func_collection is None:
            raise RuntimeError("函数 Collection 未初始化")

        zvec = self._ensure_zvec()

        # Build multi-vector query
        query_vectors = [
            zvec.VectorQuery("dense_embedding", vector=dense_vector),
        ]
        if sparse_vector:
            query_vectors.append(
                zvec.VectorQuery("sparse_embedding", vector=sparse_vector),
            )
            logger.info("Hybrid search: Dense + Sparse, RRF=%s", use_rrf)
        else:
            logger.info("Hybrid search: Dense only (no sparse vector)")

        query_params = {
            "vectors": query_vectors,
            "topk": top_k,
        }

        # Apply RRF reranker when using multiple vectors
        if use_rrf and len(query_vectors) > 1:
            try:
                from zvec.extension import RrfReRanker
                reranker = RrfReRanker(topn=rrf_topn or top_k)
                query_params["reranker"] = reranker
                logger.info("RRF reranker enabled, topn=%d", rrf_topn or top_k)
            except ImportError:
                logger.warning("RrfReRanker not available, fallback to default fusion")

        if filter_expr:
            query_params["filter"] = filter_expr
            logger.info("Hybrid search with filter: %s", filter_expr)

        results = self.func_collection.query(**query_params)
        logger.info("Hybrid search returned %d results", len(results))
        return results

    def search_modules(self, query_vector: list, top_k: int = 10,
                       filter_expr: str = None):
        """在模块 Collection 中进行向量相似度搜索"""
        if self.module_collection is None:
            raise RuntimeError("模块 Collection 未初始化")

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
        logger.info("模块搜索返回 %d 条结果", len(results))
        return results

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
        """关闭所有 Collection"""
        if self.func_collection:
            self.func_collection.close()
            self.func_collection = None
            logger.info("函数 Collection 已关闭")
        if self.module_collection:
            self.module_collection.close()
            self.module_collection = None
            logger.info("模块 Collection 已关闭")
