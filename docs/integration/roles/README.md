# 角色任务索引

> 三个团队各有一份"你拿到这份文档,直接照做"的任务书。
> 每份都是 self-contained 的:列出交付物 / 代码骨架 / 测试要求 / 完成标准 / 跨团队契约。

---

## 三个团队

| 文档 | 谁看 | 工作量 | 主要交付 |
|------|------|-------|---------|
| [📘 DeepMemory 团队](./deepmemory_team.md) | 维护 DeepMemory Python 包的开发者 | 2-3 天 | HTTP 服务器 + 异步抽取 worker + evolution daemon |
| [📗 FlashMemory 团队](./flashmemory_team.md) | 维护 FlashMemory Go 核心 / `fm_http` 的开发者 | 0.5-1.5 天 | OpenAPI + 字段契约稳定承诺(已有 fm_http) |
| [📕 eino 网关团队](./eino_gateway_team.md) | 维护 eino v0.9 + react.Agent 后端的开发者 | 1-2 天 | HTTP client + 入口/出口 middleware + Skill 注册 |

---

## 顺序与依赖

```
┌─────────────────────────────┐
│ DeepMemory 团队              │ ← 先做(网关阻塞依赖)
│   HTTP server + daemon      │
└──────────────┬──────────────┘
               │  HTTP API 上线
               ▼
┌─────────────────────────────┐
│ FlashMemory 团队              │ ← 并行做
│   OpenAPI + 字段契约          │
└──────────────┬──────────────┘
               │  /api/search 字段冻结
               ▼
┌─────────────────────────────┐
│ eino 网关团队                 │ ← 最后做(消费上面两个的契约)
│   client + middleware       │
└─────────────────────────────┘
```

**关键节点**:
1. DeepMemory v1.1 HTTP server 上线 → 网关团队解锁
2. FlashMemory `/api/search` 字段确认 → 网关团队 client struct 冻结
3. 网关团队完成 → 端到端验证

---

## 各团队互不影响的边界

| | 进程 | 仓库 | 部署 | 重启互相影响? |
|--|------|------|------|------------|
| DeepMemory | Python uvicorn | `deepmemory/` | 独立容器 | 否 |
| FlashMemory | Go `fm_http` 二进制 | `pip-package/` + `cmd/app/` | 独立容器 | 否 |
| eino 网关 | Go gateway 二进制 | 你们自己的仓库 | 独立容器 | 否 |

**任何一个组件死了,网关都应该 graceful degrade**:
- DeepMemory 死 → 召回返回空 → 对话继续(无记忆增强)
- FlashMemory 死 → 代码搜索返回空 → 对话继续(无代码上下文)
- 网关死 → ... 这个就没救了,本身就是用户入口

---

## 跨团队沟通模板

如果某个团队需要找另一个团队推进,用这个模板发到群里:

```
[<role>] 需要 <另一个 role> 配合
- 我在做: ...
- 我卡在: ...
- 我需要: ...(具体到字段名 / 接口 / 配置)
- 期望时间: ...
```

例:
> [eino-gateway] 需要 deepmemory 配合
> - 我在做: 实现 RecallMulti 客户端
> - 我卡在: HTTP server 还没上线
> - 我需要: dev 环境的 base_url + 一个测试 token
> - 期望时间: 本周内

---

## 通用契约(三方都要遵守)

1. **响应信封**: 全部使用 `{ code, message, data }` 三段式 JSON
2. **`code: 0`** 表示成功;非 0 都视为错误
3. **鉴权**: 生产环境强制 Bearer Token + HTTPS 反向代理
4. **超时**: 同步接口 P99 < 500ms;长任务异步化
5. **不绕开 API**: 不直接读对方的数据库/jsonl/SQLite,所有交互走 HTTP

---

## 何时合并

每个角色文档的 §"完成标准 (Definition of Done)" 全部打钩 + [验证脚本](../../../scripts/verify_mcp_integration.py) **25/25 通过** + 端到端 §10 验证(在 eino 网关文档里)全绿,即可合并并对外宣布上线。

---

**索引维护**: 集成组
**疑问反馈**: 各角色文档底部都有问题反馈渠道
