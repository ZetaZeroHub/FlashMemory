# FlashMemory 团队 — 你要做什么

> **你是谁**: 维护 FlashMemory Go 核心 + HTTP 服务的开发者
> **目标**: 让现有的 `fm_http` 满足"被外部 AI 网关调用"的契约,补齐缺失的 1-2 项,提供稳定的 OpenAPI 契约
> **预计工作量**: 0.5-1.5 个工作日(大部分能力已经存在)
> **不影响什么**: 现有的核心二进制、CLI、`fm_core` 全部不动

---

## 你已经有的能力(令人欣慰)

按 `cmd/app/fm_http.go` 看,以下接口**已经存在**:

| 接口 | 用途 | 网关是否需要 |
|------|------|-----------|
| `POST /api/search` | 语义/关键词/混合搜索 | ✅ **核心**,memory-augmented-reply 用 |
| `POST /api/index` | 全量索引 | ❌ 网关不调,部署时手动 |
| `POST /api/index/incremental` | 增量索引 | ❌ |
| `GET /api/index/check` | 索引状态 | ❌ |
| `GET /api/functions` | 列函数 | ⚠️ 偶尔有用 |
| `POST /api/ranking` | 自定义排序 | ❌ |
| `GET /c/config` | 读 live config | ❌ |
| Basic Auth (`API_USER`/`API_PASS`) + Bearer (`API_TOKEN`) | 鉴权 | ✅ 必需 |

**结论**: 网关只**强依赖一个接口** —— `POST /api/search`。这意味着你的工作量很小。

---

## 你需要交付的 4 件事

| # | 交付物 | 工作量 |
|---|-------|-------|
| 1 | 健康检查接口(可能已有,确认即可) | 0-2h |
| 2 | OpenAPI 契约 + Swagger UI | 0.5 天 |
| 3 | `/api/search` 响应字段对网关稳定承诺 | 0.5 天(写测试 + 文档) |
| 4 | 一份对外契约文档 | 0.5 天 |

---

## 1. 健康检查接口

如果 `fm_http` 还没有,加一个:

```go
// cmd/app/fm_http.go (片段)

func registerHealthRoute(e *echo.Echo) {
    e.GET("/api/health", func(c echo.Context) error {
        return c.JSON(http.StatusOK, map[string]any{
            "code":    0,
            "message": "ok",
            "data": map[string]any{
                "version":      buildVersion,
                "engine":       runtimeEngine(),         // "zvec" 或 "faiss"
                "indexed_dirs": indexedProjectCount(),
                "uptime_sec":   int(time.Since(startedAt).Seconds()),
            },
        })
    })
}
```

**为什么**: 网关需要在启动前 ping 一下确认你活着;K8s 也用作 liveness probe。

**测试要求**: 请求返回 200 + `{"code":0}` 即可。

---

## 2. OpenAPI 契约 + Swagger UI

eino 网关团队要拿你的 OpenAPI 做 Go codegen,所以你必须暴露规范:

### 2.1 用 swag 生成 Swagger doc

```bash
go install github.com/swaggo/swag/cmd/swag@latest
cd cmd/app && swag init -g fm_http.go -o ./docs
```

### 2.2 在 fm_http 里挂 swagger UI

```go
import (
    echoSwagger "github.com/swaggo/echo-swagger"
    _ "your-module/cmd/app/docs" // swag 生成的
)

e.GET("/swagger/*", echoSwagger.WrapHandler)
```

启动后 `http://localhost:8766/swagger/index.html` 应该可访问。

### 2.3 注解每个 handler

至少给以下 handlers 加 swag 注解(其他可选):

```go
// SearchHandler godoc
// @Summary      Semantic / keyword / hybrid search
// @Description  搜索代码语义片段
// @Tags         search
// @Accept       json
// @Produce      json
// @Param        body body SearchRequest true "search params"
// @Success      200 {object} SearchResponse
// @Failure      400 {object} ErrorResponse
// @Security     BearerAuth
// @Router       /api/search [post]
func SearchHandler(c echo.Context) error { ... }
```

---

## 3. `/api/search` 字段稳定承诺

这是网关唯一强依赖的接口。把字段冻死,写一份字段不变性测试:

### 3.1 当前请求/响应(网关期望的字段)

**请求**:
```json
{
  "query": "user upload handler",
  "project_dir": "/path/to/project",
  "top_k": 5,
  "language": "go",
  "search_type": "functions"
}
```

**响应**(`data` 数组每项):
```json
{
  "function_name": "HandleUpload",
  "file_path": "pkg/upload/handler.go",
  "line_number": 42,
  "package": "upload",
  "score": 0.89,
  "summary": "Handles multipart file upload with size validation",
  "code_snippet": "func HandleUpload(...) { ... }"
}
```

> ⚠️ 如果你**当前的字段名跟上面对不上**,**先告诉网关团队、协商一致**,然后**这是最后一次改字段名** —— 之后契约冻结。

### 3.2 字段不变性测试

新增 `cmd/app/fm_http_contract_test.go`:

```go
//go:build contract

package main

import (
    "encoding/json"
    "testing"
)

// TestSearchResponseFieldStability fails if any expected field is missing or
// renamed. This is the contract the gateway depends on — DO NOT remove
// or rename these fields without coordinating with the gateway team.
func TestSearchResponseFieldStability(t *testing.T) {
    expectedFields := []string{
        "function_name", "file_path", "line_number",
        "package", "score", "summary", "code_snippet",
    }

    sample := callSearchAPI(t, "test query")
    var data []map[string]any
    if err := json.Unmarshal(sample.Data, &data); err != nil {
        t.Fatal(err)
    }
    if len(data) == 0 {
        t.Skip("no results to verify")
    }

    for _, f := range expectedFields {
        if _, ok := data[0][f]; !ok {
            t.Errorf("expected field %q missing from /api/search response", f)
        }
    }
}
```

把它加到 CI:`go test -tags=contract ./cmd/app/...`。

---

## 4. 对外契约文档

写一份 `cmd/app/HTTP_API_CONTRACT.md`(英文 + 中文双语,因为外部团队可能多语言):

```markdown
# FlashMemory HTTP API — Stability Contract

This document defines the stable HTTP surface exposed by `fm_http`. Endpoints
listed here MUST NOT have breaking changes without a deprecation cycle.

## Stable endpoints (locked)

- POST /api/search
- GET  /api/health

## Stable but lightly used (please coordinate before changing)

- POST /api/index/incremental
- GET  /api/index/check
- GET  /api/functions

## Internal / experimental (may change)

- POST /api/ranking
- POST /api/module-graphs/update
- GET  /c/config
- PUT  /c/config

## Auth

- Basic: API_USER + API_PASS (env)
- Bearer: API_TOKEN (env)
- Both can coexist; at least one must be set in production

## Versioning

This API is version 1. Breaking changes will go to /api/v2/* with parallel
support for at least 6 months.
```

---

## 5. 配置默认值

确保 `fm_http` 的以下配置易于网关方使用:

```yaml
# fm.yaml(在被索引项目根目录或 ~/.flashmemory/config.yaml)
http:
  host: "127.0.0.1"
  port: 8766
  read_timeout_sec: 30
  write_timeout_sec: 30
  cors:
    enabled: false      # 网关到 fm_http 是后端到后端,不需要 CORS
search:
  default_top_k: 10
  default_search_type: "functions"
  max_top_k: 50          # 防止网关误调拖垮服务
  query_timeout_ms: 5000
auth:
  bearer_required: true  # 生产环境强制
```

---

## 6. 给网关方的启动指引

把这段贴到 `cmd/app/README.md`:

```bash
# 最简启动(开发,无鉴权)
./fm_http --port 8766 --project-dir /path/to/project

# 生产启动(Bearer + 后台)
API_TOKEN="$(openssl rand -hex 32)" \
FM_DEFAULT_PROJECT=/path/to/project \
nohup ./fm_http --port 8766 > /var/log/fm_http.log 2>&1 &

# 健康检查
curl -s http://localhost:8766/api/health | jq

# 网关侧调用示例
curl -s -X POST http://localhost:8766/api/search \
  -H "Authorization: Bearer $API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "upload handler",
    "project_dir": "/path/to/project",
    "top_k": 5,
    "search_type": "functions"
  }'
```

---

## 7. 测试要求

| 文件 | 覆盖点 |
|------|-------|
| `cmd/app/fm_http_health_test.go` | `/api/health` 返回 200 + `code:0` |
| `cmd/app/fm_http_contract_test.go` | `/api/search` 响应字段不变性(见 §3.2) |
| `cmd/app/fm_http_auth_test.go` | 无 token / 错误 token / Bearer 正确 token 三种情况 |

---

## 8. 完成标准 (Definition of Done)

```
[ ] /api/health 实现(若已有则跳过)
[ ] OpenAPI 通过 swag 生成,/swagger/index.html 可访问
[ ] /api/search 字段稳定测试通过
[ ] HTTP_API_CONTRACT.md 写好并 review
[ ] cmd/app/README.md 更新启动 / 鉴权 / 调用示例
[ ] 通知 eino 网关团队: 拉取最新版后,可按契约调用
[ ] 提供一份 OpenAPI JSON 文件给网关团队做 codegen
```

---

## 9. 跟其他角色的接口契约

| 给谁 | 你保证什么 |
|------|----------|
| **eino 网关团队** | `/api/search` 字段不变性;Bearer 鉴权稳定;响应延迟 P99 < 500ms;CORS 不需要(后端到后端);提供 OpenAPI JSON |
| **DeepMemory 团队** | 互不依赖 |
| **运维 / SRE** | `/api/health` 可作 K8s liveness;日志输出 stdout;支持优雅关闭(SIGTERM 后等所有请求完成) |

---

## 10. 你**不**需要做的事(明确拒绝范围蔓延)

| 请求 | 回应 |
|------|------|
| "网关想要 GraphQL" | 拒绝。HTTP/JSON 已够用,加 GraphQL 是过度设计 |
| "网关想要 SSE 流式搜索" | 缓议。先看 `/api/search` 同步够不够,延迟超 1s 再讨论 |
| "网关想要每次启动自动建索引" | 拒绝。索引是部署期工作,运行时不该重建 —— 让运维做 |
| "DeepMemory 想直接读你的 SQLite" | 拒绝。所有跨组件交互走 HTTP API,不要绕开 |

---

**问题反馈**: 在 issue tracker 用 `[fm-http]` 前缀
**对接人**: eino 网关团队(见 [`eino_gateway_team.md`](./eino_gateway_team.md))
