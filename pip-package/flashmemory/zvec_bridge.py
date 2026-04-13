"""
FlashMemory Zvec Bridge

通过 subprocess stdin/stdout 与 Go 主程序通信的 JSON-line 协议服务。
Go 端启动此 Python 进程，通过 stdin 发送 JSON 请求，从 stdout 读取 JSON 响应。

协议格式 (每行一个 JSON 对象):
  请求: {"action": "...", "params": {...}}
  响应: {"status": "success"|"error", "data": {...}, "message": "..."}
"""

import json
import sys
import os
import logging
import traceback

# 将 flashmemory 包目录加入 sys.path
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from flashmemory.zvec_engine import ZvecEngine

# 配置日志输出到 stderr，避免污染 stdout 协议通道
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(name)s: %(message)s",
    stream=sys.stderr,
)
logger = logging.getLogger("flashmemory.zvec_bridge")


class ZvecBridge:
    """stdin/stdout JSON-line 协议的 Zvec 服务桥接器
    
    Go 主程序通过 subprocess 启动此进程，通过 stdin/stdout 通信。
    日志和调试信息输出到 stderr，不影响协议通道。
    """

    def __init__(self):
        self.engine = None
        self.embedding_provider = None
        self.zvec_embedding_func = None  # P2-1: zvec built-in embedding
        self.running = True
        logger.info("ZvecBridge initialized")

    def _init_zvec_embedding(self, model_source: str = "huggingface"):
        """P2-1: Initialize zvec built-in embedding function.

        Args:
            model_source: "huggingface" or "modelscope"
        """
        try:
            from zvec.extension import DefaultLocalDenseEmbedding
            self.zvec_embedding_func = DefaultLocalDenseEmbedding(
                model_source=model_source,
            )
            logger.info(
                "Zvec built-in embedding initialized: model_source=%s, dim=%d",
                model_source, self.zvec_embedding_func.dimension,
            )
        except ImportError:
            logger.warning("DefaultLocalDenseEmbedding not available, zvec embedding disabled")
        except Exception as e:
            logger.warning("Zvec embedding init failed: %s", e)

    def _respond(self, status: str, data: dict = None, message: str = ""):
        """向 stdout 写入一行 JSON 响应"""
        response = {
            "status": status,
            "data": data or {},
            "message": message,
        }
        line = json.dumps(response, ensure_ascii=False)
        sys.stdout.write(line + "\n")
        sys.stdout.flush()

    def _respond_ok(self, data: dict = None, message: str = "ok"):
        self._respond("success", data, message)

    def _respond_error(self, message: str, data: dict = None):
        self._respond("error", data, message)

    def handle_init(self, params: dict):
        """Initialize Zvec engine and Collection

        params:
            collection_path: str - Collection base directory
            dimension: int - Vector dimension (default 384)
            force_new: bool - Force recreate (default false)
            collection_type: str - "functions" | "modules" | "both" (default "both")
            use_zvec_embedding: bool - Use zvec built-in embedding (default false)
            embedding_model_source: str - "huggingface" | "modelscope" (default "huggingface")
        """
        collection_path = params.get("collection_path", "")
        if not collection_path:
            return self._respond_error("collection_path cannot be empty")

        dimension = params.get("dimension", 384)
        force_new = params.get("force_new", False)
        collection_type = params.get("collection_type", "both")

        try:
            self.engine = ZvecEngine(collection_path, dimension=dimension)

            if collection_type in ("functions", "both"):
                self.engine.init_func_collection(force_new=force_new)
            if collection_type in ("modules", "both"):
                self.engine.init_module_collection(force_new=force_new)

            # P2-1: Optionally initialize zvec built-in embedding
            use_zvec_embedding = params.get("use_zvec_embedding", False)
            if use_zvec_embedding:
                self._init_zvec_embedding(
                    model_source=params.get("embedding_model_source", "huggingface"),
                )

            stats = self.engine.get_stats()
            self._respond_ok(
                data={"stats": stats, "dimension": dimension},
                message="Zvec engine initialized successfully",
            )
        except Exception as e:
            logger.error("Init failed: %s\n%s", e, traceback.format_exc())
            self._respond_error(f"Init failed: {e}")

    def handle_add_vector(self, params: dict):
        """添加单个函数向量
        
        params:
            func_id: str - 函数唯一ID (如 "func_42")
            vector: list[float] - 向量数据
            metadata: dict - 标量字段
        """
        if self.engine is None:
            return self._respond_error("引擎未初始化")

        func_id = params.get("func_id", "")
        vector = params.get("vector", [])
        metadata = params.get("metadata", {})

        if not func_id or not vector:
            return self._respond_error("func_id 和 vector 不能为空")

        try:
            safe_meta = {
                "func_name": metadata.get("func_name", "unknown"),
                "package": metadata.get("package", "unknown"),
                "file_path": metadata.get("file_path", "unknown"),
                "description": str(metadata.get("description", "")),
                "func_type": metadata.get("func_type", "function"),
                "language": metadata.get("language", "unknown"),
                "start_line": int(metadata.get("start_line", 0)),
                "end_line": int(metadata.get("end_line", 0)),
                "importance_score": float(metadata.get("importance_score", 0.0)),
            }
            self.engine.upsert_function(func_id, vector, safe_meta)
            self._respond_ok(message=f"已添加函数向量: {func_id}")
        except Exception as e:
            logger.error("添加向量失败: %s", e)
            self._respond_error(f"添加向量失败: {e}")

    def handle_add_vectors_batch(self, params: dict):
        """批量添加函数向量
        
        params:
            items: list[{func_id, vector, metadata}]
        """
        if self.engine is None:
            return self._respond_error("引擎未初始化")

        items_raw = params.get("items", [])
        if not items_raw:
            return self._respond_error("items 不能为空")

        try:
            items = []
            for item in items_raw:
                meta = item.get("metadata", {})
                safe_meta = {
                    "func_name": meta.get("func_name", "unknown"),
                    "package": meta.get("package", "unknown"),
                    "file_path": meta.get("file_path", "unknown"),
                    "description": str(meta.get("description", "")),
                    "func_type": meta.get("func_type", "function"),
                    "language": meta.get("language", "unknown"),
                    "start_line": int(meta.get("start_line", 0)),
                    "end_line": int(meta.get("end_line", 0)),
                    "importance_score": float(meta.get("importance_score", 0.0)),
                }
                items.append((
                    item["func_id"],
                    item["vector"],
                    safe_meta,
                ))
            self.engine.upsert_functions_batch(items)
            self._respond_ok(
                data={"count": len(items)},
                message=f"批量添加 {len(items)} 个向量",
            )
        except Exception as e:
            logger.error("批量添加失败: %s", e)
            self._respond_error(f"批量添加失败: {e}")

    def handle_add_module_vector(self, params: dict):
        """添加模块向量"""
        if self.engine is None:
            return self._respond_error("引擎未初始化")

        module_id = params.get("module_id", "")
        vector = params.get("vector", [])
        metadata = params.get("metadata", {})

        if not module_id or not vector:
            return self._respond_error("module_id 和 vector 不能为空")

        try:
            safe_meta = {
                "name": metadata.get("name", "unknown"),
                "type": metadata.get("type", "unknown"),
                "path": metadata.get("path", "unknown"),
                "parent_path": metadata.get("parent_path", "unknown"),
                "description": str(metadata.get("description", "")),
            }
            self.engine.upsert_module(module_id, vector, safe_meta)
            self._respond_ok(message=f"已添加模块向量: {module_id}")
        except Exception as e:
            logger.error("添加模块向量失败: %s", e)
            self._respond_error(f"添加模块向量失败: {e}")

    def handle_search(self, params: dict):
        """向量搜索
        
        params:
            query: list[float] - 查询向量
            top_k: int - 返回结果数 (默认 10)
            filter: str - 标量过滤表达式 (可选)
            collection_type: str - "functions" | "modules" (默认 "functions")
        """
        if self.engine is None:
            return self._respond_error("引擎未初始化")

        query = params.get("query", [])
        top_k = params.get("top_k", 10)
        filter_expr = params.get("filter", None)
        collection_type = params.get("collection_type", "functions")

        if not query:
            return self._respond_error("查询向量不能为空")

        try:
            if collection_type == "modules":
                results = self.engine.search_modules(query, top_k, filter_expr)
            else:
                results = self.engine.search_functions(query, top_k, filter_expr)

            # 将搜索结果转换为可序列化的格式
            result_list = []
            for doc in results:
                item = {
                    "id": doc.id,
                    "score": doc.score if hasattr(doc, 'score') else 0.0,
                    "fields": doc.fields if hasattr(doc, 'fields') else {},
                }
                result_list.append(item)

            self._respond_ok(
                data={"results": result_list, "count": len(result_list)},
            )
        except Exception as e:
            logger.error("搜索失败: %s\n%s", e, traceback.format_exc())
            self._respond_error(f"搜索失败: {e}")

    def handle_delete(self, params: dict):
        """删除向量
        
        params:
            func_id: str - 按 ID 删除
            file_path: str - 按文件路径删除 (增量更新时使用)
        """
        if self.engine is None:
            return self._respond_error("引擎未初始化")

        try:
            if "file_path" in params:
                self.engine.delete_by_file(params["file_path"])
                self._respond_ok(message=f"已删除文件 {params['file_path']} 的向量")
            elif "func_id" in params:
                self.engine.delete_function(params["func_id"])
                self._respond_ok(message=f"已删除函数 {params['func_id']}")
            else:
                self._respond_error("需要 func_id 或 file_path 参数")
        except Exception as e:
            logger.error("删除失败: %s", e)
            self._respond_error(f"删除失败: {e}")

    def handle_optimize(self, params: dict):
        """优化索引"""
        if self.engine is None:
            return self._respond_error("引擎未初始化")

        try:
            self.engine.optimize()
            self._respond_ok(message="索引优化完成")
        except Exception as e:
            logger.error("优化失败: %s", e)
            self._respond_error(f"优化失败: {e}")

    def handle_stats(self, params: dict):
        """获取统计信息"""
        if self.engine is None:
            return self._respond_error("引擎未初始化")

        try:
            stats = self.engine.get_stats()
            self._respond_ok(data={"stats": stats})
        except Exception as e:
            self._respond_error(f"获取统计信息失败: {e}")

    def handle_close(self, params: dict):
        """关闭引擎"""
        if self.engine:
            self.engine.close()
            self.engine = None
        self._respond_ok(message="引擎已关闭")

    def handle_ping(self, params: dict):
        """健康检查"""
        self._respond_ok(
            data={"engine_ready": self.engine is not None},
            message="pong",
        )

    def handle_shutdown(self, params: dict):
        """关闭服务进程"""
        if self.engine:
            self.engine.close()
        self.running = False
        self._respond_ok(message="服务已关闭")

    def handle_hybrid_search(self, params: dict):
        """Phase 2+: Hybrid search with Dense + Sparse + Reranker

        params:
            dense_query: list[float] - Dense query vector
            sparse_query: dict - Sparse query vector {token_id: weight} (optional)
            top_k: int - Number of results (default 10)
            filter: str - Scalar filter expression (optional)
            use_rrf: bool - Use RRF fusion (default true)
            rrf_topn: int - RRF top-N (defaults to top_k)
            collection_type: str - "functions" | "modules" (default "functions")
            query_text: str - Original query text for auto BM25 + reranker (optional)
            enable_reranker: bool - Enable cross-encoder reranking (default false)
            reranker_type: str - "rrf" | "weighted" (default "rrf")
            weighted_params: list[float] - Weights for WeightedReRanker (optional)
            recall_multiplier: int - Recall stage multiplier (default 5)
        """
        if self.engine is None:
            return self._respond_error("Engine not initialized")

        dense_query = params.get("dense_query", [])
        sparse_query = params.get("sparse_query", None)
        top_k = params.get("top_k", 10)
        filter_expr = params.get("filter", None)
        use_rrf = params.get("use_rrf", True)
        rrf_topn = params.get("rrf_topn", None)
        query_text = params.get("query_text", None)
        enable_reranker = params.get("enable_reranker", False)
        reranker_type = params.get("reranker_type", "rrf")
        weighted_params = params.get("weighted_params", None)
        recall_multiplier = params.get("recall_multiplier", 5)

        if not dense_query:
            return self._respond_error("dense_query cannot be empty")

        collection_type = params.get("collection_type", "functions")

        try:
            results = self.engine.hybrid_search(
                dense_vector=dense_query,
                sparse_vector=sparse_query,
                top_k=top_k,
                filter_expr=filter_expr,
                use_rrf=use_rrf,
                rrf_topn=rrf_topn,
                collection_type=collection_type,
                query_text=query_text,
                enable_reranker=enable_reranker,
                reranker_type=reranker_type,
                weighted_params=weighted_params,
                recall_multiplier=recall_multiplier,
            )

            result_list = []
            for doc in results:
                item = {
                    "id": doc.id,
                    "score": doc.score if hasattr(doc, 'score') else 0.0,
                    "fields": doc.fields if hasattr(doc, 'fields') else {},
                }
                result_list.append(item)

            self._respond_ok(
                data={
                    "results": result_list,
                    "count": len(result_list),
                    "search_type": "hybrid_reranked" if enable_reranker else ("hybrid" if sparse_query or query_text else "dense_only"),
                },
            )
        except Exception as e:
            logger.error("Hybrid search failed: %s\n%s", e, traceback.format_exc())
            self._respond_error(f"Hybrid search failed: {e}")

    def handle_init_embedding(self, params: dict):
        """Phase 3: Initialize embedding provider

        params:
            dense_provider: str - "local" | "openai" | "qwen" | "jina" (default "local")
            sparse_provider: str - "bm25_zh" | "bm25_en" | "splade" | "none" (default "none")
            model_name: str - Override default model name
            model_source: str - "huggingface" | "modelscope" (default "huggingface")
            api_key: str - Required for cloud providers
            dimension: int - Override dimension
        """
        try:
            from flashmemory.embedding_provider import EmbeddingProvider
            self.embedding_provider = EmbeddingProvider(config=params)
            self._respond_ok(
                data=self.embedding_provider.get_info(),
                message="Embedding provider initialized",
            )
        except Exception as e:
            logger.error("Embedding init failed: %s\n%s", e, traceback.format_exc())
            self._respond_error(f"Embedding 初始化失败: {e}")

    def handle_embed(self, params: dict):
        """Phase 3: Generate embedding vectors

        params:
            text: str - Text to embed
            texts: list[str] - Batch texts to embed (alternative to text)
            type: str - "dense" | "sparse" | "both" (default "dense")
        """
        if not hasattr(self, "embedding_provider") or self.embedding_provider is None:
            return self._respond_error("Embedding provider 未初始化，请先调用 init_embedding")

        text = params.get("text", "")
        texts = params.get("texts", [])
        embed_type = params.get("type", "dense")

        if not text and not texts:
            return self._respond_error("text 或 texts 参数不能为空")

        try:
            result = {}

            if text:
                # Single text embedding
                if embed_type in ("dense", "both"):
                    result["dense"] = self.embedding_provider.embed_dense(text)
                if embed_type in ("sparse", "both"):
                    sparse = self.embedding_provider.embed_sparse(text)
                    result["sparse"] = sparse
                result["dimension"] = self.embedding_provider.dimension
            elif texts:
                # Batch embedding
                if embed_type in ("dense", "both"):
                    result["dense_batch"] = self.embedding_provider.embed_dense_batch(texts)
                if embed_type in ("sparse", "both"):
                    result["sparse_batch"] = [
                        self.embedding_provider.embed_sparse(t) for t in texts
                    ]
                result["dimension"] = self.embedding_provider.dimension
                result["count"] = len(texts)

            self._respond_ok(data=result, message="ok")
        except Exception as e:
            logger.error("Embedding failed: %s\n%s", e, traceback.format_exc())
            self._respond_error(f"Embedding 失败: {e}")

    def handle_pipeline_search(self, params: dict):
        """Phase 3: Execute full search pipeline (embed → search → rerank)

        params:
            query: str - Natural language query
            top_k: int - Number of results (default 10)
            filter: str - Scalar filter expression (optional)
            language: str - Filter by language (optional)
            package: str - Filter by package (optional)
            search_type: str - "functions" | "modules" (default "functions")
            use_reranker: bool - Override reranker usage (optional)
        """
        if self.engine is None:
            return self._respond_error("引擎未初始化")
        if not hasattr(self, "embedding_provider") or self.embedding_provider is None:
            return self._respond_error("Embedding provider 未初始化")

        query = params.get("query", "")
        if not query:
            return self._respond_error("query 不能为空")

        try:
            from flashmemory.search_pipeline import SearchPipeline

            pipeline_config = {
                "enable_reranker": params.get("use_reranker", False),
                "recall_multiplier": params.get("recall_multiplier", 5),
                "use_rrf": params.get("use_rrf", True),
            }

            pipeline = SearchPipeline(
                engine=self.engine,
                embedding_provider=self.embedding_provider,
                config=pipeline_config,
            )

            # Build filter from convenience params
            language = params.get("language")
            package = params.get("package")

            if language or package:
                results = pipeline.search_with_context(
                    query=query,
                    top_k=params.get("top_k", 10),
                    language=language,
                    package=package,
                )
            else:
                results = pipeline.search(
                    query=query,
                    top_k=params.get("top_k", 10),
                    filter_expr=params.get("filter"),
                    search_type=params.get("search_type", "functions"),
                )

            result_list = [r.to_dict() for r in results]
            self._respond_ok(
                data={
                    "results": result_list,
                    "count": len(result_list),
                    "pipeline_info": pipeline.get_pipeline_info(),
                },
            )
        except Exception as e:
            logger.error("Pipeline search failed: %s\n%s", e, traceback.format_exc())
            self._respond_error(f"Pipeline 搜索失败: {e}")

    # Action 路由表
    ACTION_HANDLERS = {
        "init": handle_init,
        "init_embedding": handle_init_embedding,
        "add_vector": handle_add_vector,
        "add_vectors_batch": handle_add_vectors_batch,
        "add_module_vector": handle_add_module_vector,
        "search": handle_search,
        "hybrid_search": handle_hybrid_search,
        "embed": handle_embed,
        "pipeline_search": handle_pipeline_search,
        "delete": handle_delete,
        "optimize": handle_optimize,
        "stats": handle_stats,
        "close": handle_close,
        "ping": handle_ping,
        "shutdown": handle_shutdown,
    }

    def run(self):
        """主循环：从 stdin 读取 JSON-line 请求，分发到对应 handler"""
        logger.info("ZvecBridge 开始监听 stdin...")

        # 立即发送 ready 信号
        self._respond_ok(message="ready")

        for line in sys.stdin:
            line = line.strip()
            if not line:
                continue

            try:
                request = json.loads(line)
            except json.JSONDecodeError as e:
                self._respond_error(f"JSON 解析错误: {e}")
                continue

            action = request.get("action", "")
            params = request.get("params", {})

            handler = self.ACTION_HANDLERS.get(action)
            if handler is None:
                self._respond_error(f"未知的 action: {action}")
                continue

            try:
                handler(self, params)
            except Exception as e:
                logger.error(
                    "处理 action=%s 时发生未捕获异常: %s\n%s",
                    action, e, traceback.format_exc(),
                )
                self._respond_error(f"内部错误: {e}")

            if not self.running:
                break

        logger.info("ZvecBridge 退出")


def main():
    """入口函数"""
    bridge = ZvecBridge()
    bridge.run()


if __name__ == "__main__":
    main()
