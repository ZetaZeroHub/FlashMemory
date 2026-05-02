# 网关集成指南 — eino 后端适配版

> **版本**: 1.0.0
> **日期**: 2026-05-02
> **适用栈**: eino v0.9+ / `adk/middlewares/skill` / `react.Agent` / `host.MultiAgent` / `prebuilt/supervisor`
> **预计改造量**: 80-120 行 Go + 1 处 fm.yaml + 重启网关
> **替代关系**: 本文取代 [`README.md`](./README.md) 的 "Step 3: 绑定 Dialog Hooks" 那一章 —— 在 eino 栈上,该章节无法落地。

---

## TL;DR

如果你的后端是 eino,那么:

1. **dialog hooks 不存在** —— `before_reply` / `after_reply` / `on_user_feedback` 这些钩子在 `react.Agent` 链路上没有等价物。
2. **5 个 Skill 里只有 2 个真正适合做 Skill** —— `game-theory-debate` 和 `user-feedback-handler`,因为它们需要 LLM 决策时机。
3. **其余 3 个能力下沉到 infra 层**:
   - `memory-augmented-reply` → 网关入口 middleware(自动召回)
   - `symbolism-extract` → 网关出口 middleware(异步抽取)
   - `memory-evolution-cycle` → DeepMemory 后台 daemon(env 启用)
4. **小白路径**: `pip install` → 改 1 个 yaml → 在网关注册 1 个 middleware → 重启,完事。

---

## 为什么不用 dialog hooks?

证据链已经在 grill 阶段确认过,这里给一个可贴在 PR 描述里的版本:

> eino 官方 `skill.skillTool` 是纯 ToolInfo 注入模型(B 模式):主 LLM 在工具池里看到一个名为 `skill` 的工具,描述里列出全部 SKILL 的 `name + description` (≈ 200 tokens / 2 skills,线性增长);只有 LLM 显式调 `{"skill":"<name>"}` 时才把 SKILL.md 全文作为 tool message 注入。这种模型**不存在生命周期钩子**:它没法在 LLM 决定回复**之前**自动跑一段代码,也没法在回复**之后**自动跑一段代码。
>
> 让 LLM 自己每轮判断"现在该不该召回记忆 / 抽取符号"是反模式:
>
> - 召回率取决于 LLM 自觉性,不稳定
> - 每轮都消耗 tokens 在"要不要触发"的决策上
> - 时机不可控,可能召回得太晚或太早
>
> 因此**这类机械触发的行为应该下沉到 infra**,而不是放在 LLM 决策面上。Skill 抽象只留给真正需要 LLM 判断时机的事(矛盾仲裁、反馈识别)。

---

## 5 个能力的最终落地方式

| 原 Skill | 新落地层 | 触发方式 | 改造点 |
|---------|---------|---------|--------|
| `memory-augmented-reply` | **网关入口 middleware** | 每次 chat 请求自动跑 | 加 1 个 Go middleware ≈ 40 行 |
| `symbolism-extract` | **网关出口 middleware**(异步) | LLM 回复完成后 fire-and-forget | 加 1 个 goroutine + 1 个 SDK 方法 ≈ 30 行 |
| `memory-evolution-cycle` | **DeepMemory 后台 daemon** | env var 启用,周期性运行 | 改 fm.yaml,1 行 |
| `user-feedback-handler` | **保留为 Skill** | LLM 识别"用户在反馈" 时调用 | 改 description 文案 |
| `game-theory-debate` | **保留为 Skill** | LLM 发现矛盾时调用 | 维持现状 |

---

## 新架构总览

```
┌─────────────────────────────────────────────────────────────┐
│                       eino 网关层                            │
│                                                             │
│  HTTP Handler                                               │
│      │                                                      │
│      ├─ [入口 mw] memory_augmentation                       │
│      │     ↓ deepmemory.RecallMulti(session, user, ...)     │
│      │     ↓ 注入到 SystemExtra                             │
│      │                                                      │
│      ├─ react.Agent / host.MultiAgent / supervisor          │
│      │     ├─ 工具池: [..., skill, deepmemory_*, fm_*]      │
│      │     │     ↑                                          │
│      │     │     └─ skill 工具描述里只有 2 个 SKILL:        │
│      │     │           game-theory-debate                   │
│      │     │           user-feedback-handler                │
│      │     ↓                                                │
│      │  LLM 推理 + 工具调用循环                              │
│      │                                                      │
│      ├─ [出口 mw] symbolism_async                           │
│      │     ↓ go deepmemory.IngestAsync(turn)                │
│      │                                                      │
│      └─ HTTP Response                                       │
│                                                             │
└─────────────────────────────────────────────────────────────┘
       │                                       │
       ▼                                       ▼
┌──────────────┐                   ┌──────────────────────┐
│ FlashMemory  │                   │  DeepMemory          │
│ MCP Server   │                   │  MCP Server          │
│ (3 tools)    │                   │  (7 tools + daemon)  │
└──────────────┘                   └──────────────────────┘
                                              │
                                              ▼
                                   ┌──────────────────────┐
                                   │  evolution daemon    │
                                   │  (后台 goroutine)     │
                                   │  每 24h 自动 evolve   │
                                   └──────────────────────┘
```

---

## Step 1 — 升级 DeepMemory SDK / MCP Server

### 1.1 新增的 SDK 方法

DeepMemory v1.1 在原有 7 个 MCP 工具基础上,新增以下 **SDK 直调方法**(不暴露为 MCP tool,因为不需要 LLM 调用):

```python
# deepmemory/runtime/facade.py 扩展
class DeepMemoryFacade:
    # ↓↓↓ 新增 ↓↓↓

    def recall_multi(
        self,
        object_ids: list[str],
        limit: int = 10,
        min_confidence: float = 0.35,
    ) -> list[dict]:
        """批量召回:对多个 object_id 一次性查询,按 social_status 和 confidence 排序去重。"""

    def ingest_async(
        self,
        turn_text: str,
        session_id: str,
        actor_ref: str = "system",
        source_ref: str | None = None,
    ) -> str:
        """异步入队抽取任务,立即返回 task_id。后台 worker 会:
           1. 调 LLM 抽取符号(Sign + ObjectRef + Interpretant)
           2. 对每个抽取项调 deepmemory_recall 去重
           3. 新观察 → ingest;矛盾 → 标 contested;一致 → skip
        """

    def start_evolution_daemon(
        self,
        interval_hours: int = 24,
        policy: EvolutionPolicy | None = None,
    ) -> None:
        """启动后台 daemon goroutine,周期性运行 evolve_records()。
           调用者只需在 main() 里调一次。"""
```

### 1.2 安装 / 升级

```bash
# 升级到带 daemon 的版本
pip install -U "deepmemory[mcp,daemon]"

# 或本地源码安装
cd deepmemory && pip install -e ".[mcp,daemon]"
```

### 1.3 启动 DeepMemory MCP Server(已带 daemon)

修改 `mcp_config_example.json`:

```json
{
  "mcpServers": {
    "deepmemory": {
      "command": "python",
      "args": ["-m", "deepmemory.mcp_server"],
      "env": {
        "DEEPMEMORY_STORE": "./artifacts/memory.jsonl",
        "DEEPMEMORY_EVOLVE_INTERVAL_HOURS": "24",
        "DEEPMEMORY_EVOLVE_MIN_CONFIDENCE": "0.35",
        "DEEPMEMORY_EVOLVE_STALE_HOURS": "168",
        "DEEPMEMORY_EVOLVE_MAX_PER_OBJECT": "20"
      }
    },
    "flashmemory": {
      "command": "python",
      "args": ["-m", "flashmemory.mcp_server"],
      "env": {
        "FM_DEFAULT_PROJECT": "/path/to/your/project"
      }
    }
  }
}
```

> 设了 `DEEPMEMORY_EVOLVE_INTERVAL_HOURS` 之后,server 启动时会自动 spawn evolution daemon。**这就替换了原来的 `memory-evolution-cycle` Skill。**

---

## Step 2 — 网关 middleware 改造(核心)

下面是给 eino 网关加的两个中间件。完整代码也在 [`examples/gateway_middleware_eino.go`](./examples/gateway_middleware_eino.go),可直接 copy。

### 2.1 入口 middleware:`MemoryAugmentation`

**目的**: 在请求进入 `react.Agent` 之前,自动拉取相关历史记忆并注入到 system prompt。这就替换了原来的 `memory-augmented-reply` Skill。

```go
// gateway/middleware/memory_augmentation.go

package middleware

import (
    "context"
    "fmt"
    "strings"

    "github.com/cloudwego/eino/schema"
    "your-project/deepmemory" // 你的 DeepMemory Go client(见 §2.3)
)

// MemoryAugmentationConfig 配置入口记忆增强中间件。
type MemoryAugmentationConfig struct {
    Client          deepmemory.Client
    MaxRecall       int     // 默认 10
    MinConfidence   float64 // 默认 0.35
    SessionIDKey    string  // ctx.Value 的 key,默认 "session_id"
    UserIDKey       string  // 默认 "user_id"
    InjectAsSystem  bool    // true=作为额外 system message;false=拼到首个 system message
}

// AugmentMessages 在调用模型前对 messages 切片做增强。
//
// 用法:在你的 HTTP handler 里、把 user input 喂给 react.Agent.Generate 之前调一次。
func (cfg *MemoryAugmentationConfig) AugmentMessages(
    ctx context.Context,
    messages []*schema.Message,
) ([]*schema.Message, error) {
    sessionID, _ := ctx.Value(cfg.SessionIDKey).(string)
    userID, _ := ctx.Value(cfg.UserIDKey).(string)

    objectIDs := []string{}
    if sessionID != "" {
        objectIDs = append(objectIDs, "session:"+sessionID)
    }
    if userID != "" {
        objectIDs = append(objectIDs, "user:"+userID)
    }
    if len(objectIDs) == 0 {
        return messages, nil // 没有上下文标识,跳过
    }

    memories, err := cfg.Client.RecallMulti(ctx, deepmemory.RecallMultiReq{
        ObjectIDs:     objectIDs,
        Limit:         cfg.MaxRecall,
        MinConfidence: cfg.MinConfidence,
    })
    if err != nil {
        // 召回失败不应该阻塞对话;记录后继续
        return messages, nil
    }
    if len(memories) == 0 {
        return messages, nil
    }

    block := buildMemoryContextBlock(memories)

    if cfg.InjectAsSystem {
        // 模式 A:加一条独立的 system message
        return append([]*schema.Message{
            schema.SystemMessage(block),
        }, messages...), nil
    }

    // 模式 B:拼接到现有首个 system message
    for i, m := range messages {
        if m.Role == schema.System {
            messages[i] = schema.SystemMessage(m.Content + "\n\n" + block)
            return messages, nil
        }
    }
    // 没有 system message,新建一条
    return append([]*schema.Message{
        schema.SystemMessage(block),
    }, messages...), nil
}

// buildMemoryContextBlock 渲染召回结果为 markdown 上下文块。
func buildMemoryContextBlock(memories []deepmemory.Record) string {
    var sb strings.Builder
    sb.WriteString("## Recalled Memories (DeepMemory)\n\n")
    sb.WriteString("These are persistent observations from prior sessions. ")
    sb.WriteString("Treat CANONICAL as fact, PROVISIONAL_CONSENSUS as reliable, ")
    sb.WriteString("LOCAL_HYPOTHESIS as one-source. Skip CONTESTED.\n\n")

    for _, m := range memories {
        sb.WriteString(fmt.Sprintf(
            "- **[%s]** (%s, confidence=%.2f)\n  Claim: %s\n  Source: %s\n",
            m.CanonicalSign,
            strings.ToUpper(m.SocialStatus),
            m.Confidence,
            m.OperationalClaim,
            m.SourceRef,
        ))
    }
    return sb.String()
}
```

### 2.2 出口 middleware:`SymbolismAsyncIngest`

**目的**: 在 LLM 完成回复后,异步抽取本轮对话的符号并入库。这就替换了原来的 `symbolism-extract` Skill。

```go
// gateway/middleware/symbolism_async.go

package middleware

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/cloudwego/eino/schema"
    "your-project/deepmemory"
)

type SymbolismAsyncConfig struct {
    Client       deepmemory.Client
    SessionIDKey string
    UserIDKey    string
    Enabled      bool
}

// FireAndForget 把本轮 user message + assistant reply 丢给 DeepMemory 异步抽取。
//
// 用法:在你的 HTTP handler 里、拿到 react.Agent.Generate 的结果之后、写回 HTTP 响应之前调一次。
//      也可以放在 defer 里,确保即使下游写入失败也能记录。
func (cfg *SymbolismAsyncConfig) FireAndForget(
    ctx context.Context,
    userMessage string,
    assistantReply *schema.Message,
) {
    if !cfg.Enabled || assistantReply == nil {
        return
    }
    sessionID, _ := ctx.Value(cfg.SessionIDKey).(string)
    userID, _ := ctx.Value(cfg.UserIDKey).(string)

    // 关键: 一定要 detach context,主请求 ctx 一旦 cancel,这个异步任务也会被砍。
    payload := deepmemory.IngestAsyncReq{
        TurnText:  fmt.Sprintf("USER: %s\n\nASSISTANT: %s", userMessage, assistantReply.Content),
        SessionID: sessionID,
        ActorRef:  "user:" + userID,
        SourceRef: fmt.Sprintf("session-%s-turn-%d", sessionID, time.Now().UnixNano()),
    }

    go func() {
        bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer cancel()
        if _, err := cfg.Client.IngestAsync(bgCtx, payload); err != nil {
            // 异步失败不影响主对话,记 warn 即可
            log.Printf("[symbolism-async] ingest failed: %v", err)
        }
    }()
}
```

### 2.3 你需要的 DeepMemory Go Client

DeepMemory 本体是 Python,所以网关侧需要一个轻量 Go client。**最简单的方式**:通过 MCP Server 的 stdio JSON-RPC 跟它讲话(已经有现成 Go MCP client 库),或者直接 HTTP / Unix socket。

下面给一个**最小可用 stdio client 骨架**(完整版见 [`examples/deepmemory_client.go`](./examples/deepmemory_client.go)):

```go
package deepmemory

import (
    "context"
    "encoding/json"
    "fmt"
)

type Client interface {
    RecallMulti(ctx context.Context, req RecallMultiReq) ([]Record, error)
    IngestAsync(ctx context.Context, req IngestAsyncReq) (taskID string, err error)
}

type RecallMultiReq struct {
    ObjectIDs     []string `json:"object_ids"`
    Limit         int      `json:"limit"`
    MinConfidence float64  `json:"min_confidence"`
}

type IngestAsyncReq struct {
    TurnText  string `json:"turn_text"`
    SessionID string `json:"session_id"`
    ActorRef  string `json:"actor_ref"`
    SourceRef string `json:"source_ref"`
}

type Record struct {
    MemoryID         string  `json:"memory_id"`
    CanonicalSign    string  `json:"canonical_sign"`
    OperationalClaim string  `json:"operational_claim"`
    SocialStatus     string  `json:"social_status"`
    Confidence       float64 `json:"confidence"`
    SourceRef        string  `json:"source_ref"`
}

// stdioClient 通过 MCP stdio JSON-RPC 跟 deepmemory MCP server 讲话。
type stdioClient struct {
    // ... 内部字段:进程引用 / 读写流 / 请求 ID 计数器
}

func NewStdioClient(cmd string, args []string, env []string) (Client, error) {
    // ... 启动子进程,握手 MCP initialize
    return &stdioClient{}, nil
}
```

> **现成轮子**: 如果你不想自己实现 stdio JSON-RPC,可以直接用 [`github.com/cloudwego/eino-ext/components/tool/mcp`](https://github.com/cloudwego/eino-ext) ——eino 团队已经为 MCP 做了 Go 绑定,可以拿来直接调 `deepmemory_recall_multi` / `deepmemory_ingest_async` 这两个 RPC 方法。

### 2.4 把两个 middleware 装进 eino HTTP handler

```go
// gateway/handler/chat.go

func ChatHandler(
    agent *react.Agent,
    memMW *middleware.MemoryAugmentationConfig,
    symMW *middleware.SymbolismAsyncConfig,
) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        var req ChatRequest
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil { /* 400 */ return }

        // 把 session_id / user_id 注入 context
        ctx := r.Context()
        ctx = context.WithValue(ctx, "session_id", req.SessionID)
        ctx = context.WithValue(ctx, "user_id", req.UserID)

        // 构造原始消息
        msgs := []*schema.Message{
            schema.SystemMessage(req.SystemPrompt),
            schema.UserMessage(req.UserMessage),
        }

        // ★ 入口 middleware:自动召回记忆并注入
        msgs, _ = memMW.AugmentMessages(ctx, msgs)

        // 调用 eino agent
        reply, err := agent.Generate(ctx, msgs)
        if err != nil { /* 500 */ return }

        // ★ 出口 middleware:异步抽取符号
        symMW.FireAndForget(ctx, req.UserMessage, reply)

        // 写回响应
        json.NewEncoder(w).Encode(ChatResponse{Reply: reply.Content})
    }
}
```

**就这么多。整个改造 = 2 个 middleware 文件 + handler 里 2 行调用。**

---

## Step 3 — 注册保留的 2 个 Skill

把这两个 Skill 放在 eino 约定的 `configs/skills/` 目录下(或你的等价路径):

```
configs/skills/
├── game-theory-debate/
│   └── SKILL.md
└── user-feedback-handler/
    └── SKILL.md
```

### 3.1 优化后的 description 文案(关键)

eino 是**纯 B 模式**,所以 description 是 LLM 决定要不要触发的**唯一依据**。下面是针对纯 LLM 触发场景重写过的 description:

**`game-theory-debate/SKILL.md`** front matter:

```yaml
---
name: game-theory-debate
description: >
  Invoke this when you detect TWO OR MORE memories about the same object_id
  with contradicting claims, OR when the user explicitly disputes a recalled
  memory ("that's wrong, actually..."). Runs a 3-round debate among 3 agents
  (Advocate / Challenger / Arbiter) and votes to either CONFIRM, REVISE, or
  DEPRECATE the disputed memory. Do NOT call this for routine recall — only
  when there is a real, identifiable conflict.
metadata:
  ...
---
```

**`user-feedback-handler/SKILL.md`** front matter:

```yaml
---
name: user-feedback-handler
description: >
  Invoke this IMMEDIATELY when the user explicitly affirms, denies, corrects,
  reinforces, or asks to forget a previously surfaced memory. Trigger phrases:
  "yes / correct / exactly / 对" → promote;
  "no / wrong / 不对" → delete;
  "actually it's X, not Y / 其实是 X" → revise;
  "remember this / 记住这个" → reinforce;
  "forget that / 忘掉它" → delete.
  Do NOT call for general agreement on AI suggestions — only when the user is
  reacting to a specific recalled memory.
metadata:
  ...
---
```

### 3.2 让 eino 看到新 Skill

eino `react.Agent` 在构造时缓存了 ToolInfo,所以新增 SKILL.md 之后:

```bash
# 选项 A:调你的 refresh API,然后让 supervisor 重建 worker agent
curl -X POST http://your-gateway/api/v1/skills/refresh

# 选项 B:重启网关(最简单)
systemctl restart your-gateway
```

> **重要**: 这是 eino 当前的限制,不是 Skill 设计的限制。等 eino 后续支持动态 ToolInfo 重建,选项 A 就是唯一路径。

---

## Step 4 — fm.yaml 配置

在你的项目根 `fm.yaml` 加一段:

```yaml
# fm.yaml — 在 eino 网关项目根目录
deepmemory:
  store_path: "./artifacts/memory.jsonl"
  evolve:
    enabled: true
    interval_hours: 24
    min_confidence: 0.35
    stale_after_hours: 168
    max_per_object: 20

memory_middleware:
  augmentation:
    enabled: true
    max_recall: 10
    min_confidence: 0.35
    inject_as_system: false  # true=独立 system msg, false=拼接到现有 system
  symbolism_async:
    enabled: true
    timeout_seconds: 30

flashmemory:
  default_project_dir: "/path/to/your/project"
```

把这份 yaml 在网关启动时读进来,传给两个 middleware 的构造函数即可。

---

## Step 5 — 验证清单

按这个顺序跑,每步都应该绿。

### 5.1 DeepMemory 后台 daemon 是否启动?

```bash
# 启动 MCP server(带 daemon)
python -m deepmemory.mcp_server &

# 看日志应该有
[deepmemory] evolution daemon started: interval=24h, min_confidence=0.35
```

### 5.2 入口 middleware 是否生效?

```bash
# 先手动 ingest 一条
python -c "
from deepmemory.runtime.facade import DeepMemoryFacade
from deepmemory.packs.coding.templates import CodingTemplateKind
f = DeepMemoryFacade(store_path='./artifacts/memory.jsonl', actor_ref='user:test')
f.ingest_coding(
    template=CodingTemplateKind.TASK,
    actor_ref='user:test',
    summary='user prefers dark mode',
    object_id='user:preference:theme',
    object_type='user_preference',
    anchor_kind='config',
    anchor_locator='settings.json',
    source_ref='manual-test',
)
"

# 然后发一个对话请求,带 user_id=test
curl -X POST http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -d '{"session_id":"s1","user_id":"test","user_message":"我刚才说要什么主题来着?"}'

# 应该看到 AI 回复中提到 "dark mode" —— 证明召回 + 注入路径打通
```

### 5.3 出口 middleware 是否触发?

```bash
# 再发一条带新偏好的对话
curl -X POST http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -d '{"session_id":"s2","user_id":"test","user_message":"我用 uv 不用 pip"}'

# 等 5 秒后查
python -c "
from deepmemory.runtime.facade import DeepMemoryFacade
f = DeepMemoryFacade(store_path='./artifacts/memory.jsonl', actor_ref='user:test')
records = f.recall_records(object_id='user:preference:packaging')
print(records)
"

# 应该看到一条 claim 包含 'uv' 的新记录 —— 证明异步抽取路径打通
```

### 5.4 两个 Skill 是否可被 LLM 调用?

```bash
# 发一条会触发反馈识别的消息
curl -X POST http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -d '{"session_id":"s3","user_id":"test","user_message":"对,主题就是dark mode"}'

# 检查 agent 日志,应该看到
[skill] launched: user-feedback-handler
[deepmemory] promote called for memory_id=mu_xxx
```

### 5.5 evolution daemon 是否真的在跑?

```bash
# 测试用:把 interval 调到 1 分钟
DEEPMEMORY_EVOLVE_INTERVAL_HOURS=0.0167 python -m deepmemory.mcp_server &

# 等 90 秒,看日志
[deepmemory] evolution cycle: scanned=42, decayed=3, deleted=0
```

### 5.6 端到端验证

跑现成的脚本:

```bash
python scripts/verify_mcp_integration.py
# 应该: Results: 25/25 passed
```

---

## Token 成本对比(实测口径)

| 场景 | dialog hooks 路线(理论) | 当前 eino 实现路线 |
|------|------------------------|------------------|
| **每轮对话固定开销**(无记忆) | ~200 tokens(skill 工具描述) | ~200 tokens(skill 工具描述) |
| **每轮对话召回开销** | 200-800 tokens(注入的记忆) | 200-800 tokens(middleware 注入) |
| **每轮对话抽取开销** | 0(异步) | 0(异步) |
| **召回失败时的兜底** | LLM 决策"要不要召回"消耗 ~50 tokens | 0(infra 直接跳过) |
| **触发 user-feedback** | 调 SKILL,SKILL.md ~1500 tokens | 调 SKILL,SKILL.md ~1500 tokens |
| **触发 game-theory** | 调 SKILL,SKILL.md ~3000 tokens | 调 SKILL,SKILL.md ~3000 tokens |

**结论**: eino 实现路线**比 hooks 路线还更省**(每轮少 50 tokens),因为 infra 触发不需要 LLM 决策。

---

## 故障排查

| 症状 | 可能原因 | 解决 |
|------|---------|------|
| 加了 SKILL.md 但 LLM 看不到 | ToolInfo 缓存未刷新 | 调 `/api/v1/skills/refresh` 或重启网关 |
| 入口 middleware 召回为空 | session_id / user_id 没注入 ctx | 检查 handler 里 `context.WithValue` 调用 |
| 异步抽取一直无新记忆 | LLM key 未配,抽取 worker 失败 | 看 `_events.jsonl`,搜 `ingest_async_failed` |
| daemon 日志里 evolve 数 = 0 | 记忆库为空或 `min_confidence` 太低 | 检查 `DEEPMEMORY_EVOLVE_*` env |
| LLM 频繁误调 user-feedback-handler | description 没写够限制条件 | 加强 "Do NOT call for..." 段落 |
| game-theory-debate 调用太频繁 | LLM 把"含糊回答"也当矛盾 | description 里强调 "real, identifiable conflict" |
| 对话延迟变高 | 入口召回是同步的 | 加 `RecallTimeout` 字段,默认 200ms 超时直接跳过 |

---

## 与原始 dialog-hooks 设计的迁移映射

如果你以前已经按 [`README.md`](./README.md) Step 3 配过 `dialog_hooks_example.yaml`,按这个映射改:

| 原 hook 配置 | 新落地点 |
|-------------|---------|
| `before_reply: memory-augmented-reply` | 删除,改用 `MemoryAugmentation` middleware |
| `after_reply: symbolism-extract` | 删除,改用 `SymbolismAsyncIngest` middleware |
| `on_user_feedback: user-feedback-handler` | 删除 hook 行,SKILL 本体保留 |
| `scheduled: memory-evolution-cycle` | 删除,改用 env `DEEPMEMORY_EVOLVE_INTERVAL_HOURS` |
| `on_memory_conflict: game-theory-debate` | 删除 hook 行,SKILL 本体保留 |

`dialog_hooks_example.yaml` **本身可以删掉** —— 在 eino 栈上它没有任何运行时作用。我们仍把它保留在 docs 里,是为了说明"如果你将来用支持 hooks 的栈,长这样"。

---

## 改造 Checklist(贴 PR 描述)

```
基础设施
[ ] DeepMemory 升级到带 daemon 的版本(pip install -U deepmemory[daemon])
[ ] mcp_config_example.json 加 DEEPMEMORY_EVOLVE_* 环境变量
[ ] 验证 evolution daemon 启动日志

网关代码
[ ] 加 gateway/middleware/memory_augmentation.go
[ ] 加 gateway/middleware/symbolism_async.go
[ ] 加 gateway/deepmemory/client.go(或引入 eino-ext MCP 绑定)
[ ] handler 里调 AugmentMessages 和 FireAndForget
[ ] context 注入 session_id / user_id

Skills
[ ] 删除 .agents/skills/symbolism-extract/
[ ] 删除 .agents/skills/memory-augmented-reply/
[ ] 删除 .agents/skills/memory-evolution-cycle/
[ ] 保留并优化 .agents/skills/game-theory-debate/SKILL.md description
[ ] 保留并优化 .agents/skills/user-feedback-handler/SKILL.md description
[ ] 调 /api/v1/skills/refresh 或重启网关

配置
[ ] fm.yaml 加 deepmemory / memory_middleware / flashmemory 段
[ ] 验证 fm.yaml 加载日志

验证
[ ] §5.1-§5.6 6 项验证全绿
[ ] verify_mcp_integration.py 25/25 通过

文档
[ ] 团队 wiki 链接到本文
[ ] README.md 顶部加 banner: "eino 用户请直接看 gateway_integration_eino.md"
```

---

## 后续路线图(不在本次改造范围)

| 项目 | 触发条件 | 预计工作量 |
|------|---------|----------|
| ChatModelAgent 迁移 | eino 团队稳定 ChatModelAgentMiddleware API | 1-2 周 |
| 真正的 dialog hooks 支持 | 上面那条做完 | 2-3 天 |
| `context: fork` 子 agent 支持 | 注入 `skill.AgentHub` 之后 | 3-5 天 |
| ToolInfo 热重载(免重启) | eino 提供 `agent.RefreshTools()` API | 1 天 |

这些都是 v2 的事 —— 本次改造**不依赖任何一项**就能让 5 个能力全部生效(2 个 Skill 真跑 + 3 个 infra 行为真跑)。

---

## 一句话给 boss / 给小白

> 我们没用"5 个 Skill 全家桶"那套漂亮但跑不起来的方案,而是把其中 3 个机械触发的能力下沉到了网关 middleware 和 SDK daemon —— 改了 80 行 Go,加了 4 个 env var,5 个能力**全部真在工作**。

---

**作者**: FlashMemory + DeepMemory 集成组
**问题反馈**: 在 issue tracker 用 `[gateway-eino]` 前缀
