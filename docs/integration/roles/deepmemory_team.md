# DeepMemory 团队 — 你要做什么

> **你是谁**: 维护 DeepMemory Python 包的开发者
> **目标**: 给 DeepMemory 加一个 HTTP 服务面 + 后台 daemon + 异步抽取 worker,让外部网关用 `net/http` 就能集成
> **预计工作量**: 2-3 个工作日
> **不影响什么**: 现有的 stdio MCP Server (`deepmemory.mcp_server`) 完全保留,Claude Desktop 等 MCP 客户端继续可用

---

## 你需要交付的 4 件事

| # | 交付物 | 文件 | 工作量 |
|---|-------|------|-------|
| 1 | HTTP 服务器 | `deepmemory/http_server.py` | 1 天 |
| 2 | Facade 新方法 | `deepmemory/runtime/facade.py` 扩展 | 0.5 天 |
| 3 | 异步抽取 worker | `deepmemory/runtime/async_extractor.py` | 0.5-1 天 |
| 4 | Evolution daemon | `deepmemory/runtime/evolution_daemon.py` | 0.5 天 |

---

## 1. HTTP 服务器

技术选型: **FastAPI**(自带 OpenAPI / Swagger UI / Pydantic 校验,代码量最少)。

### 1.1 安装依赖

修改 `deepmemory/pyproject.toml`:

```toml
[project.optional-dependencies]
mcp    = ["mcp>=1.2.0,<2"]
http   = [
    "fastapi>=0.110,<1",
    "uvicorn[standard]>=0.27,<1",
    "pydantic>=2.5,<3",
]
daemon = []  # 用 asyncio,无额外依赖
```

### 1.2 文件骨架 — `deepmemory/http_server.py`

完全 1:1 镜像 MCP server 的 7 个工具,但用 REST 表达:

```python
"""DeepMemory HTTP API server (FastAPI).

Mirrors the 7 MCP tools as HTTP endpoints, plus 2 batch endpoints
(recall_multi, ingest_async) used by gateway middlewares.
"""
import os
from contextlib import asynccontextmanager
from typing import Any, Optional

from fastapi import FastAPI, HTTPException, Header, Path, Query
from pydantic import BaseModel, Field

from deepmemory.core.enums import SocialStatus
from deepmemory.core.schema import memory_unit_to_record
from deepmemory.packs.coding.templates import CodingTemplateKind
from deepmemory.runtime.evolution import EvolutionPolicy
from deepmemory.runtime.facade import DeepMemoryFacade
from deepmemory.runtime.async_extractor import AsyncExtractor
from deepmemory.runtime.evolution_daemon import EvolutionDaemon


# ---------------------------------------------------------------------------
# Pydantic 请求/响应模型
# ---------------------------------------------------------------------------

class IngestReq(BaseModel):
    summary: str
    object_id: str
    object_type: str = "concept"
    anchor_kind: str = "symbol"
    anchor_locator: str
    template: str = "task"
    actor_ref: str = "http-client"
    source_ref: str = "http-session"
    context: dict[str, Any] = Field(default_factory=dict)


class RecallMultiReq(BaseModel):
    object_ids: list[str]
    limit: int = 10
    min_confidence: float = 0.0


class IngestAsyncReq(BaseModel):
    turn_text: str
    session_id: str
    actor_ref: str = "system"
    source_ref: str | None = None


class PromoteReq(BaseModel):
    status: str  # 必须是 SocialStatus 枚举字符串


class ReviseReq(BaseModel):
    operational_claim: str | None = None
    confidence: float | None = None
    context_patch: dict[str, Any] | None = None


class EvolveReq(BaseModel):
    min_confidence: float = 0.35
    stale_after_hours: int = 168
    decay_confidence_delta: float = 0.05
    max_memories_per_object: int = 20


class Envelope(BaseModel):
    code: int = 0
    message: str = "ok"
    data: Any = None


# ---------------------------------------------------------------------------
# 启停 lifecycle
# ---------------------------------------------------------------------------

state: dict = {}

@asynccontextmanager
async def lifespan(app: FastAPI):
    store_path = os.environ.get("DEEPMEMORY_STORE", "./artifacts/memory.jsonl")
    actor_ref = os.environ.get("DEEPMEMORY_DEFAULT_ACTOR", "http-server")
    state["facade"] = DeepMemoryFacade(
        store_path=store_path,
        event_path=store_path.replace(".jsonl", "_events.jsonl"),
        actor_ref=actor_ref,
    )

    # 启动异步抽取 worker
    state["extractor"] = AsyncExtractor(facade=state["facade"])
    await state["extractor"].start()

    # 启动 evolution daemon
    interval_h = float(os.environ.get("DEEPMEMORY_EVOLVE_INTERVAL_HOURS", "0"))
    if interval_h > 0:
        state["daemon"] = EvolutionDaemon(
            facade=state["facade"],
            interval_seconds=int(interval_h * 3600),
            policy=EvolutionPolicy(
                min_confidence=float(os.environ.get("DEEPMEMORY_EVOLVE_MIN_CONFIDENCE", "0.35")),
                stale_after_hours=int(os.environ.get("DEEPMEMORY_EVOLVE_STALE_HOURS", "168")),
                max_memories_per_object=int(os.environ.get("DEEPMEMORY_EVOLVE_MAX_PER_OBJECT", "20")),
            ),
        )
        await state["daemon"].start()
        print(f"[deepmemory] evolution daemon started: interval={interval_h}h")

    yield

    # 关闭
    if "daemon" in state:
        await state["daemon"].stop()
    await state["extractor"].stop()


app = FastAPI(
    title="DeepMemory HTTP API",
    version="1.1.0",
    description="Cognitive memory management — REST surface for gateway integration.",
    lifespan=lifespan,
)


# ---------------------------------------------------------------------------
# 鉴权依赖(可选)
# ---------------------------------------------------------------------------

def auth(authorization: Optional[str] = Header(default=None)) -> None:
    expected = os.environ.get("DEEPMEMORY_API_TOKEN")
    if not expected:
        return  # 无鉴权模式
    if not authorization or not authorization.startswith("Bearer "):
        raise HTTPException(401, "missing bearer token")
    if authorization[7:] != expected:
        raise HTTPException(403, "invalid token")


# ---------------------------------------------------------------------------
# Endpoints
# ---------------------------------------------------------------------------

@app.get("/api/v1/health")
async def health() -> Envelope:
    facade: DeepMemoryFacade = state["facade"]
    return Envelope(data={
        "version": "1.1.0",
        "store_path": facade.store_path,
        "memory_count": facade.count(),
        "evolve_daemon_running": "daemon" in state,
    })


@app.post("/api/v1/memory/ingest", dependencies=[])
async def ingest(req: IngestReq) -> Envelope:
    facade: DeepMemoryFacade = state["facade"]
    try:
        tpl = CodingTemplateKind(req.template)
    except ValueError:
        tpl = CodingTemplateKind.TASK
    memory = facade.ingest_coding(
        template=tpl,
        actor_ref=req.actor_ref,
        summary=req.summary,
        object_id=req.object_id,
        object_type=req.object_type,
        anchor_kind=req.anchor_kind,
        anchor_locator=req.anchor_locator,
        source_ref=req.source_ref,
        context=req.context,
    )
    return Envelope(data=memory_unit_to_record(memory))


@app.get("/api/v1/memory/recall")
async def recall(
    object_id: str = Query(...),
    limit: int = 10,
    min_confidence: float = 0.0,
) -> Envelope:
    facade: DeepMemoryFacade = state["facade"]
    records = facade.recall_records(object_id=object_id)
    records = [r for r in records if r.get("interpretant", {}).get("confidence", 0) >= min_confidence]
    return Envelope(data=records[:limit])


@app.post("/api/v1/memory/recall_multi")
async def recall_multi(req: RecallMultiReq) -> Envelope:
    facade: DeepMemoryFacade = state["facade"]
    seen: set[str] = set()
    merged: list[dict] = []
    for oid in req.object_ids:
        for rec in facade.recall_records(object_id=oid):
            mid = rec.get("memory_id")
            if mid and mid not in seen:
                if rec.get("interpretant", {}).get("confidence", 0) >= req.min_confidence:
                    seen.add(mid)
                    merged.append(rec)
    # 按 social_status 优先级 + confidence 排序
    status_order = {
        "canonical": 0, "provisional_consensus": 1, "local_hypothesis": 2,
        "private_hint": 3, "contested": 4,
    }
    merged.sort(key=lambda r: (
        status_order.get(r.get("social_status", "private_hint"), 99),
        -r.get("interpretant", {}).get("confidence", 0),
    ))
    return Envelope(data=merged[:req.limit])


@app.post("/api/v1/memory/{memory_id}/promote")
async def promote(memory_id: str = Path(...), req: PromoteReq = None) -> Envelope:
    facade: DeepMemoryFacade = state["facade"]
    record = facade.promote_record(memory_id=memory_id, status=SocialStatus(req.status))
    return Envelope(data=record)


@app.post("/api/v1/memory/{memory_id}/revise")
async def revise(memory_id: str = Path(...), req: ReviseReq = None) -> Envelope:
    facade: DeepMemoryFacade = state["facade"]
    record = facade.revise_record(
        memory_id=memory_id,
        operational_claim=req.operational_claim,
        confidence=req.confidence,
        context_patch=req.context_patch,
    )
    return Envelope(data=record)


@app.delete("/api/v1/memory/{memory_id}")
async def delete(memory_id: str = Path(...), reason: str = Query("manual")) -> Envelope:
    facade: DeepMemoryFacade = state["facade"]
    ok = facade.delete_record(memory_id=memory_id, reason=reason)
    return Envelope(data={"deleted": ok})


@app.post("/api/v1/memory/ingest_async")
async def ingest_async(req: IngestAsyncReq) -> Envelope:
    extractor: AsyncExtractor = state["extractor"]
    task_id = await extractor.enqueue(
        turn_text=req.turn_text,
        session_id=req.session_id,
        actor_ref=req.actor_ref,
        source_ref=req.source_ref,
    )
    return Envelope(data={"task_id": task_id, "queued_at": extractor.now_iso()})


@app.get("/api/v1/memory/ingest_async/{task_id}")
async def ingest_async_status(task_id: str = Path(...)) -> Envelope:
    extractor: AsyncExtractor = state["extractor"]
    return Envelope(data=extractor.status(task_id))


@app.post("/api/v1/memory/evolve")
async def evolve(req: EvolveReq = None) -> Envelope:
    facade: DeepMemoryFacade = state["facade"]
    policy = EvolutionPolicy(
        min_confidence=req.min_confidence,
        stale_after_hours=req.stale_after_hours,
        decay_confidence_delta=req.decay_confidence_delta,
        max_memories_per_object=req.max_memories_per_object,
    )
    report = facade.evolve_records(policy=policy)
    return Envelope(data=report)


@app.get("/api/v1/memory/events")
async def events(limit: int = 50, since: Optional[str] = None) -> Envelope:
    facade: DeepMemoryFacade = state["facade"]
    return Envelope(data=facade.read_events(limit=limit, since_iso=since))


# ---------------------------------------------------------------------------
# 入口
# ---------------------------------------------------------------------------

def main():
    import uvicorn
    host = os.environ.get("DEEPMEMORY_HTTP_HOST", "127.0.0.1")
    port = int(os.environ.get("DEEPMEMORY_HTTP_PORT", "8765"))
    uvicorn.run("deepmemory.http_server:app", host=host, port=port, log_level="info")


if __name__ == "__main__":
    main()
```

### 1.3 完整 OpenAPI 契约

完整接口在 [`docs/integration/http_api_reference.md`](../http_api_reference.md)。
你的 FastAPI server 启动后会自动暴露 `/openapi.json`、`/docs`、`/redoc` —— 直接拷贝 spec 给前端 / Go 客户端做 codegen。

---

## 2. Facade 扩展

修改 `deepmemory/runtime/facade.py`,加 3 个方法:

```python
class DeepMemoryFacade:
    # ... 现有 9 个方法不动 ...

    # ↓↓↓ 新增 ↓↓↓

    def count(self) -> int:
        """供 health endpoint 使用。"""
        return sum(1 for _ in self._iter_records())

    # recall_multi 已在 http_server.py 的 endpoint 里组合了 recall_records,
    # 不需要新方法。但建议你抽到 facade 里好测试:
    def recall_multi(
        self,
        object_ids: list[str],
        limit: int = 10,
        min_confidence: float = 0.0,
    ) -> list[dict]:
        """批量召回 + 按 social_status/confidence 排序去重。"""
        # 实现见上面 endpoint 的逻辑

    # ingest_async 不需要 facade 方法,直接由 AsyncExtractor 处理。
    # 见 §3。
```

**配套测试**:`tests/test_facade_recall_multi.py`,覆盖
- 多 object_id 合并去重
- min_confidence 过滤
- social_status 排序优先级

---

## 3. 异步抽取 worker

新文件 `deepmemory/runtime/async_extractor.py`:

```python
"""Async LLM-based symbol extraction worker.

Receives raw turn text, calls an LLM to extract Sign + ObjectRef + Interpretant
triples, then ingests each via the facade. Runs as a background asyncio task.
"""
import asyncio
import json
import uuid
from datetime import datetime
from typing import Any, Optional

from deepmemory.runtime.facade import DeepMemoryFacade
from deepmemory.packs.coding.templates import CodingTemplateKind


EXTRACTION_PROMPT = """You are a symbolic deconstruction engine. Read the dialog turn
below and extract any DECISIONS, PREFERENCES, or OBSERVATIONS as structured
triples.

For each triple, output JSON:
{
  "canonical_sign": "snake_case_concept",
  "object_id": "user:preference:xxx | module:yyy:zzz | concept:aaa",
  "object_type": "user_preference | decision | bug | feature | concept",
  "anchor_kind": "symbol | locator | config",
  "anchor_locator": "filename or logical anchor",
  "operational_claim": "one-sentence claim in original language",
  "confidence": 0.0-1.0
}

Only emit triples for genuine new information. Skip greetings, filler, or
ambiguous statements. Output a JSON array, possibly empty.

Dialog turn:
---
{turn_text}
---
"""


class AsyncExtractor:
    def __init__(self, facade: DeepMemoryFacade, llm_client=None):
        self.facade = facade
        self.llm = llm_client or _default_llm_from_env()
        self._tasks: dict[str, dict] = {}
        self._queue: asyncio.Queue = asyncio.Queue(maxsize=1000)
        self._worker: Optional[asyncio.Task] = None

    async def start(self):
        self._worker = asyncio.create_task(self._run())

    async def stop(self):
        if self._worker:
            self._worker.cancel()
            try:
                await self._worker
            except asyncio.CancelledError:
                pass

    async def enqueue(
        self,
        turn_text: str,
        session_id: str,
        actor_ref: str = "system",
        source_ref: Optional[str] = None,
    ) -> str:
        task_id = f"tsk_{uuid.uuid4().hex[:12]}"
        self._tasks[task_id] = {
            "task_id": task_id,
            "status": "queued",
            "queued_at": self.now_iso(),
            "params": {
                "turn_text": turn_text,
                "session_id": session_id,
                "actor_ref": actor_ref,
                "source_ref": source_ref or f"async-{task_id}",
            },
        }
        await self._queue.put(task_id)
        return task_id

    def status(self, task_id: str) -> dict:
        return self._tasks.get(task_id, {"task_id": task_id, "status": "unknown"})

    @staticmethod
    def now_iso() -> str:
        return datetime.utcnow().isoformat() + "Z"

    async def _run(self):
        while True:
            task_id = await self._queue.get()
            task = self._tasks[task_id]
            task["status"] = "running"
            task["started_at"] = self.now_iso()
            try:
                triples = await self._extract(task["params"]["turn_text"])
                ingested = []
                for t in triples:
                    memory = self.facade.ingest_coding(
                        template=CodingTemplateKind.TASK,
                        actor_ref=task["params"]["actor_ref"],
                        summary=t.get("operational_claim", ""),
                        object_id=t["object_id"],
                        object_type=t.get("object_type", "concept"),
                        anchor_kind=t.get("anchor_kind", "symbol"),
                        anchor_locator=t.get("anchor_locator", "n/a"),
                        source_ref=task["params"]["source_ref"],
                        context={"session_id": task["params"]["session_id"]},
                    )
                    ingested.append(memory.memory_id)
                task["status"] = "done"
                task["ingested"] = ingested
            except Exception as e:
                task["status"] = "failed"
                task["error"] = str(e)
            task["finished_at"] = self.now_iso()

    async def _extract(self, turn_text: str) -> list[dict]:
        # 调用 LLM,要求返回 JSON 数组
        response = await self.llm.acomplete(
            EXTRACTION_PROMPT.format(turn_text=turn_text[:4000]),
            response_format={"type": "json_object"},
        )
        try:
            payload = json.loads(response.content)
            if isinstance(payload, list):
                return payload
            if isinstance(payload, dict) and "items" in payload:
                return payload["items"]
        except json.JSONDecodeError:
            pass
        return []


def _default_llm_from_env():
    """从环境变量构造一个最小 LLM 客户端。
    支持 DEEPMEMORY_LLM_PROVIDER=openai|anthropic|ollama,详见你团队的 LLM 抽象。"""
    # 实现略 —— 复用你已有的 LLM client wrapper
    raise NotImplementedError("wire to your team's LLM abstraction")
```

**关键决策**:
- 用 `asyncio.Queue` 而不是外部 MQ —— 简单,够用
- 队列上限 1000,溢出时直接拒绝(`enqueue` 阻塞 → 客户端超时);如果生产流量需要,改用 Redis Stream
- LLM 失败的任务标 `status=failed`,**不重试** —— 抽取丢一两条没事,记忆库本来就是有损的
- task_id TTL: 内存里保留 24h,超过后清理

---

## 4. Evolution Daemon

新文件 `deepmemory/runtime/evolution_daemon.py`:

```python
"""Background daemon that periodically calls evolve_records().

Started by http_server lifespan when DEEPMEMORY_EVOLVE_INTERVAL_HOURS > 0.
"""
import asyncio
import logging
from typing import Optional

from deepmemory.runtime.evolution import EvolutionPolicy
from deepmemory.runtime.facade import DeepMemoryFacade

log = logging.getLogger("deepmemory.daemon")


class EvolutionDaemon:
    def __init__(
        self,
        facade: DeepMemoryFacade,
        interval_seconds: int,
        policy: EvolutionPolicy,
    ):
        self.facade = facade
        self.interval = interval_seconds
        self.policy = policy
        self._task: Optional[asyncio.Task] = None

    async def start(self):
        self._task = asyncio.create_task(self._run())

    async def stop(self):
        if self._task:
            self._task.cancel()
            try:
                await self._task
            except asyncio.CancelledError:
                pass

    async def _run(self):
        # 启动后立即跑一次,然后周期性跑
        while True:
            try:
                report = self.facade.evolve_records(policy=self.policy)
                log.info(
                    "evolution cycle: scanned=%d, revised=%d, deleted=%d",
                    report.get("scanned_count", 0),
                    report.get("revised_count", 0),
                    report.get("deleted_count", 0),
                )
            except Exception as e:
                log.warning("evolution cycle failed: %s", e)
            await asyncio.sleep(self.interval)
```

---

## 5. 启动脚本 / 包入口

修改 `deepmemory/pyproject.toml`:

```toml
[project.scripts]
deepmemory-mcp  = "deepmemory.mcp_server:main"
deepmemory-http = "deepmemory.http_server:main"
```

这样用户可以直接:
```bash
deepmemory-http  # 而不是 python -m deepmemory.http_server
```

---

## 6. 测试要求

新增以下测试:

| 文件 | 覆盖点 |
|------|-------|
| `tests/test_http_server.py` | 用 FastAPI TestClient 跑 7 个 endpoint 的 happy path + 4xx |
| `tests/test_async_extractor.py` | mock LLM,验证 enqueue → done 流程 + 失败处理 |
| `tests/test_evolution_daemon.py` | 启动 daemon,5s interval,验证 evolve 被调多次 |
| `tests/test_recall_multi.py` | 多 object_id 合并去重 + 排序顺序 |

最低覆盖率: 每个新文件 ≥ 80%。

---

## 7. 文档更新

完成实现后,更新这几份文档:

- [ ] `deepmemory/README.md` 顶部加 "HTTP server" 一节,贴启动命令
- [ ] [`docs/integration/http_api_reference.md`](../http_api_reference.md) **核对每个 endpoint 的字段名和示例** —— 实现完成后必查
- [ ] [`docs/integration/roles/eino_gateway_team.md`](./eino_gateway_team.md) 的 client 示例和你的 endpoint 完全一致

---

## 8. 验证(对网关团队的承诺)

发布前,你要保证以下 curl 命令全部返回 `{"code": 0, ...}`:

```bash
BASE="http://localhost:8765/api/v1"

# 1. 健康
curl -s "$BASE/health"

# 2. 摄入
curl -s -X POST "$BASE/memory/ingest" -H 'Content-Type: application/json' -d '{
  "summary":"test","object_id":"test:1","object_type":"concept",
  "anchor_kind":"symbol","anchor_locator":"x","template":"task",
  "actor_ref":"curl","source_ref":"manual"
}'

# 3. 召回单个
curl -s "$BASE/memory/recall?object_id=test:1"

# 4. 批量召回(网关 middleware 入口)
curl -s -X POST "$BASE/memory/recall_multi" -H 'Content-Type: application/json' -d '{
  "object_ids":["test:1","test:2"],"limit":5,"min_confidence":0.0
}'

# 5. 异步抽取(网关 middleware 出口)
curl -s -X POST "$BASE/memory/ingest_async" -H 'Content-Type: application/json' -d '{
  "turn_text":"USER: 我用 uv\nASSISTANT: ok","session_id":"s1","actor_ref":"user:test"
}'

# 6. evolve
curl -s -X POST "$BASE/memory/evolve" -H 'Content-Type: application/json' -d '{}'

# 7. events
curl -s "$BASE/memory/events?limit=10"
```

---

## 9. 完成标准 (Definition of Done)

```
[ ] http_server.py 实现 + 测试通过
[ ] async_extractor.py 实现 + 测试通过(mock LLM)
[ ] evolution_daemon.py 实现 + 测试通过
[ ] facade.recall_multi() / facade.count() 加好 + 测试通过
[ ] pyproject.toml 加 [http,daemon] extras + scripts
[ ] OpenAPI 暴露在 /openapi.json,Swagger UI 在 /docs
[ ] Bearer auth 通过环境变量启用
[ ] §8 的 7 条 curl 全部 {"code":0}
[ ] Docker 镜像构建通过
[ ] README 更新
[ ] 通知网关团队:DeepMemory v1.1 HTTP API 可用
```

---

## 10. 跟其他角色的接口契约

| 给谁 | 你保证什么 |
|------|----------|
| **eino 网关团队** | `POST /api/v1/memory/recall_multi` 和 `POST /api/v1/memory/ingest_async` 这两个接口字段名和返回结构稳定;延迟 P99 < 200ms(recall) / 立即返回(ingest_async) |
| **FlashMemory 团队** | 不依赖 FlashMemory(各跑各的) |
| **运维** | Bearer Token 鉴权;`/api/v1/health` 可用作 K8s liveness probe;日志输出到 stdout 给容器收集 |

---

**问题反馈**: 在 issue tracker 用 `[deepmemory-http]` 前缀
**对接人**: eino 网关团队的实施同学(见 [`eino_gateway_team.md`](./eino_gateway_team.md))
