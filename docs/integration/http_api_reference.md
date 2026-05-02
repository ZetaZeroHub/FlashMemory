# DeepMemory + FlashMemory HTTP API 参考

> **版本**: 1.0.0
> **日期**: 2026-05-02
> **定位**: 跨语言、跨进程、跨主机的轻量集成接口
> **替代关系**: 完全可替代 MCP stdio Server。stdio MCP Server 仍然保留,供 Claude Desktop 等 MCP-native 客户端使用;但**网关侧推荐用 HTTP**。

---

## 为什么是 HTTP

| 维度 | MCP stdio | HTTP |
|------|----------|------|
| 网关代码复杂度 | 子进程管理 + JSON-RPC + 握手 ≈ 150 行 | `net/http` 标准客户端 ≈ 50 行 |
| 跨主机部署 | ❌ 必须同机 | ✅ 任意拓扑 |
| 多副本扩缩容 | ❌ 一对一 | ✅ 后端任意横向扩展 |
| 调试 | 拼 JSON-RPC 帧 | `curl` |
| 进程重启 | 网关需感知 | 健康检查 + 重试即可 |
| 鉴权 | 自定义协议 | Basic / Bearer 标准方案 |
| 监控 | stdio 流没法接 metrics | 标准 access log + Prometheus |

**结论**: 除了"在同机以子进程方式拉起"这个唯一优势,HTTP 全方位胜出。
保留 stdio MCP 给 Claude Desktop / IDE 客户端,网关侧用 HTTP。

---

## 总览

| 组件 | 端口(默认) | 启动命令 | 配置入口 |
|------|----------|---------|---------|
| **DeepMemory HTTP** | `:8765` | `python -m deepmemory.http_server` | `DEEPMEMORY_*` env |
| **FlashMemory HTTP**(已有) | `:8766` | `./fm_http` 或 `start_fm_http_dev.sh` | `fm.yaml` |

两者都使用统一的响应信封:

```json
{
  "code": 0,
  "message": "ok",
  "data": { ... }
}
```

| code | 含义 |
|------|------|
| `0` | 成功 |
| `4xx` | 客户端错误(参数缺失、ID 不存在) |
| `5xx` | 服务端错误 |

---

## 鉴权

两个服务统一使用以下三种方案之一:

```
1. 无鉴权(开发模式,默认)
2. Basic Auth   — 设置 DEEPMEMORY_API_USER + DEEPMEMORY_API_PASS
3. Bearer Token — 设置 DEEPMEMORY_API_TOKEN, 客户端发 Authorization: Bearer <token>
```

FlashMemory 同理,使用 `API_USER` / `API_PASS` / `API_TOKEN`。

**生产环境强烈建议 Bearer Token + HTTPS 反向代理**。

---

## DeepMemory HTTP API

Base path: `/api/v1/memory`

### 1. 健康检查

```
GET /api/v1/health
```

**响应**:
```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "version": "1.1.0",
    "store_path": "./artifacts/memory.jsonl",
    "memory_count": 142,
    "evolve_daemon_running": true
  }
}
```

---

### 2. 摄入记忆 — `ingest`

```
POST /api/v1/memory/ingest
Content-Type: application/json
```

**请求体**:
```json
{
  "summary": "user prefers dark mode UI",
  "object_id": "user:preference:theme",
  "object_type": "user_preference",
  "anchor_kind": "config",
  "anchor_locator": "settings.json",
  "template": "task",
  "actor_ref": "user:alice",
  "source_ref": "session-001-turn-005",
  "context": {}
}
```

**响应** — 返回完整 MemoryUnit JSON:
```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "memory_id": "mu_a1b2c3d4",
    "schema_version": "dm.core.memory-unit.v0.2",
    "sign": { "canonical_sign": "...", "sign_variants": [...] },
    "object": { "object_id": "user:preference:theme", ... },
    "interpretant": { "operational_claim": "...", "confidence": 0.78 },
    "social_status": "local_hypothesis",
    "provenance": { ... }
  }
}
```

---

### 3. 召回记忆 — `recall`

```
GET /api/v1/memory/recall?object_id=user:preference:theme&limit=10
```

**Query 参数**:
| 参数 | 必填 | 默认 | 说明 |
|------|------|------|------|
| `object_id` | ✅ | — | 要召回的对象 ID |
| `limit` | ❌ | 10 | 最多返回条数 |
| `min_confidence` | ❌ | 0.0 | 置信下限 |

**响应**: `data` 是数组
```json
{
  "code": 0,
  "data": [
    { "memory_id": "mu_xxx", "operational_claim": "...", "confidence": 0.85, "social_status": "canonical", ... }
  ]
}
```

---

### 4. **批量召回** — `recall_multi`(网关 middleware 用)

```
POST /api/v1/memory/recall_multi
```

**请求体**:
```json
{
  "object_ids": ["session:abc", "user:alice"],
  "limit": 10,
  "min_confidence": 0.35
}
```

**响应**: 多个 object_id 的合并结果,已按 social_status + confidence 排序去重。

---

### 5. 晋升 — `promote`

```
POST /api/v1/memory/{memory_id}/promote
```

**请求体**:
```json
{ "status": "provisional_consensus" }
```

`status` 可选: `private_hint` / `local_hypothesis` / `provisional_consensus` / `canonical` / `contested`

---

### 6. 修订 — `revise`

```
POST /api/v1/memory/{memory_id}/revise
```

**请求体**:
```json
{
  "operational_claim": "user strongly prefers dark mode with OLED-friendly colors",
  "confidence": 0.92,
  "context_patch": {}
}
```

字段全部可选,只更新提供的字段。

---

### 7. 删除 — `delete`

```
DELETE /api/v1/memory/{memory_id}?reason=user_disagreement
```

**响应**:
```json
{ "code": 0, "data": { "deleted": true } }
```

---

### 8. **异步抽取** — `ingest_async`(网关 middleware 用)

```
POST /api/v1/memory/ingest_async
```

**请求体**:
```json
{
  "turn_text": "USER: 我用 uv 不用 pip\n\nASSISTANT: ...",
  "session_id": "sess-001",
  "actor_ref": "user:alice",
  "source_ref": "session-001-turn-005"
}
```

**响应**: 立即返回 task_id,真正的 LLM 抽取在 server 后台执行
```json
{ "code": 0, "data": { "task_id": "tsk_xxx", "queued_at": "2026-05-02T13:00:00Z" } }
```

可选:`GET /api/v1/memory/ingest_async/{task_id}` 查询状态。

---

### 9. 进化 — `evolve`

```
POST /api/v1/memory/evolve
```

**请求体**(全部可选,默认值见下表):
```json
{
  "min_confidence": 0.35,
  "stale_after_hours": 168,
  "decay_confidence_delta": 0.05,
  "max_memories_per_object": 20
}
```

**响应**:
```json
{
  "code": 0,
  "data": {
    "scanned_count": 142,
    "revised_count": 8,
    "deleted_count": 3,
    "actions": [...]
  }
}
```

> 设置 `DEEPMEMORY_EVOLVE_INTERVAL_HOURS=24` 后,server 启动时会自动 spawn daemon 周期性调用此接口。**不需要网关来触发。**

---

### 10. 事件流 — `events`

```
GET /api/v1/memory/events?limit=50&since=2026-05-02T00:00:00Z
```

**响应**: 时间序事件列表
```json
{
  "code": 0,
  "data": [
    { "event_type": "memory.ingested", "memory_id": "mu_xxx", "timestamp": "..." },
    { "event_type": "memory.promoted", "memory_id": "mu_xxx", "timestamp": "..." }
  ]
}
```

---

## FlashMemory HTTP API(已有,这里只是简明 reference)

详见 [`cmd/app/README.md`](../../cmd/app/README.md)。集成相关的接口:

```
POST /api/search       搜索代码(语义/关键词/混合)
GET  /api/functions    列出已索引函数
POST /api/index        建索引
POST /api/index/incremental  增量索引
GET  /api/index/check  索引状态
```

---

## 启动 DeepMemory HTTP Server

### 单机 dev 模式

```bash
# 安装
pip install -U "deepmemory[http,daemon]"

# 启动(开发,无鉴权)
python -m deepmemory.http_server \
    --host 0.0.0.0 \
    --port 8765 \
    --store-path ./artifacts/memory.jsonl

# 后台 + daemon
DEEPMEMORY_STORE=./artifacts/memory.jsonl \
DEEPMEMORY_EVOLVE_INTERVAL_HOURS=24 \
nohup python -m deepmemory.http_server > /var/log/deepmemory.log 2>&1 &
```

### 生产模式 — Bearer + 反向代理

```bash
# 启动后端(只监听 127.0.0.1,前面挂 nginx)
DEEPMEMORY_API_TOKEN="$(openssl rand -hex 32)" \
DEEPMEMORY_STORE=/var/lib/deepmemory/memory.jsonl \
DEEPMEMORY_EVOLVE_INTERVAL_HOURS=24 \
python -m deepmemory.http_server --host 127.0.0.1 --port 8765
```

```nginx
# /etc/nginx/sites-available/deepmemory
server {
    listen 443 ssl http2;
    server_name memory.your-domain.com;
    ssl_certificate     /etc/letsencrypt/live/.../fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/.../privkey.pem;

    location /api/ {
        proxy_pass http://127.0.0.1:8765;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_read_timeout 60s;
    }
}
```

### Docker

```dockerfile
FROM python:3.11-slim
RUN pip install --no-cache-dir "deepmemory[http,daemon]"
ENV DEEPMEMORY_STORE=/data/memory.jsonl \
    DEEPMEMORY_EVOLVE_INTERVAL_HOURS=24 \
    DEEPMEMORY_HTTP_HOST=0.0.0.0 \
    DEEPMEMORY_HTTP_PORT=8765
VOLUME /data
EXPOSE 8765
CMD ["python", "-m", "deepmemory.http_server"]
```

```bash
docker build -t deepmemory:1.1.0 .
docker run -d -p 8765:8765 -v deepmemory_data:/data \
  -e DEEPMEMORY_API_TOKEN="$(openssl rand -hex 32)" \
  deepmemory:1.1.0
```

---

## curl 速查 — 验证服务可达

```bash
TOKEN="your-bearer-token"
BASE="https://memory.your-domain.com/api/v1"

# 健康检查
curl -s "$BASE/health" | jq

# 摄入
curl -s -X POST "$BASE/memory/ingest" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"summary":"test","object_id":"test:obj:1","object_type":"concept","anchor_kind":"symbol","anchor_locator":"x","template":"task","actor_ref":"curl","source_ref":"manual"}' \
  | jq

# 召回
curl -s "$BASE/memory/recall?object_id=test:obj:1" \
  -H "Authorization: Bearer $TOKEN" | jq

# 批量召回(网关入口 middleware 用的就是这个)
curl -s -X POST "$BASE/memory/recall_multi" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"object_ids":["session:abc","user:alice"],"limit":10,"min_confidence":0.35}' \
  | jq
```

---

## OpenAPI 规范

DeepMemory HTTP server 启动后会自动暴露:

- `GET /openapi.json` — OpenAPI 3.0 规范
- `GET /docs`        — Swagger UI
- `GET /redoc`       — ReDoc UI

可直接用这个生成各种语言的 client(`openapi-generator`),不需要手写。

---

## MCP stdio 用户的迁移路径

如果你之前已经按 [`mcp_config_example.json`](./mcp_config_example.json) 配过 stdio MCP:

```diff
  "mcpServers": {
-   "deepmemory": {
-     "command": "python",
-     "args": ["-m", "deepmemory.mcp_server"],
-     "env": { "DEEPMEMORY_STORE": "./artifacts/memory.jsonl" }
-   }
+   "deepmemory_http": {  // 仅 Claude Desktop 等 MCP 原生客户端需要
+     "url": "http://localhost:8765",
+     "transport": "http+sse",
+     "headers": { "Authorization": "Bearer ${DEEPMEMORY_TOKEN}" }
+   }
  }
```

**网关侧直接删掉 MCP 客户端代码**,改用 `net/http`。完整示例见
[`gateway_integration_eino.md`](./gateway_integration_eino.md) Step 2。

---

**作者**: FlashMemory + DeepMemory 集成组
