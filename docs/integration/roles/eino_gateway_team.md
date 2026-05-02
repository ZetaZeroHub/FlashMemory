# eino 网关团队 — 你要做什么

> **你是谁**: 维护 eino v0.9 + `react.Agent` / `prebuilt/supervisor` 的 Go 网关后端
> **目标**: 用纯 HTTP(不用 MCP stdio)集成 DeepMemory + FlashMemory,给 AI 加上"长期记忆 + 代码检索"能力
> **预计工作量**: 1-2 个工作日(等 DeepMemory 团队的 HTTP server 上线后)
> **不影响什么**: react.Agent / supervisor / planexecute 这些 prebuilt 链路代码完全不动 —— 只是在外面加 middleware

---

## 你需要交付的 5 件事

| # | 交付物 | 文件 | 工作量 |
|---|-------|------|-------|
| 1 | DeepMemory HTTP client | `gateway/deepmemory/client.go` | 0.5 天 |
| 2 | FlashMemory HTTP client | `gateway/flashmemory/client.go` | 0.5 天 |
| 3 | 入口 middleware:记忆增强 | `gateway/middleware/memory_augmentation.go` | 0.5 天 |
| 4 | 出口 middleware:符号异步 | `gateway/middleware/symbolism_async.go` | 0.25 天 |
| 5 | 注册保留的 2 个 Skill | `configs/skills/{game-theory-debate,user-feedback-handler}/SKILL.md` | 0.25 天 |

---

## 1. DeepMemory HTTP Client

新文件 `gateway/deepmemory/client.go`,纯 `net/http`,无任何 MCP / RPC 依赖:

```go
package deepmemory

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "time"
)

// Client 是 DeepMemory HTTP API 的极简 Go 客户端。
// 只暴露网关需要的 4 个方法 —— 其他 endpoint 网关用不上。
type Client interface {
    Health(ctx context.Context) (*HealthData, error)
    RecallMulti(ctx context.Context, req RecallMultiReq) ([]Record, error)
    IngestAsync(ctx context.Context, req IngestAsyncReq) (taskID string, err error)
    // 以下两个给保留的 Skill 用,但 LLM 通过 tool 调用,不直接走 client
    Promote(ctx context.Context, memoryID, status string) error
    Revise(ctx context.Context, memoryID string, req ReviseReq) error
    Delete(ctx context.Context, memoryID, reason string) error
}

type HTTPClient struct {
    BaseURL string
    Token   string         // Bearer token; 空字符串则不带
    HTTP    *http.Client
}

func NewHTTPClient(baseURL, token string, timeout time.Duration) *HTTPClient {
    return &HTTPClient{
        BaseURL: baseURL,
        Token:   token,
        HTTP: &http.Client{
            Timeout: timeout,
        },
    }
}

// ---- Types ----

type Envelope struct {
    Code    int             `json:"code"`
    Message string          `json:"message"`
    Data    json.RawMessage `json:"data"`
}

type HealthData struct {
    Version            string `json:"version"`
    StorePath          string `json:"store_path"`
    MemoryCount        int    `json:"memory_count"`
    EvolveDaemonRunning bool  `json:"evolve_daemon_running"`
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

type ReviseReq struct {
    OperationalClaim *string `json:"operational_claim,omitempty"`
    Confidence       *float64 `json:"confidence,omitempty"`
}

type Record struct {
    MemoryID         string  `json:"memory_id"`
    CanonicalSign    string  `json:"-"`
    OperationalClaim string  `json:"-"`
    SocialStatus     string  `json:"social_status"`
    Confidence       float64 `json:"-"`
    SourceRef        string  `json:"-"`
    Raw              json.RawMessage `json:"-"`
}

// 自定义 unmarshal 把嵌套字段拍平
func (r *Record) UnmarshalJSON(b []byte) error {
    var raw struct {
        MemoryID    string `json:"memory_id"`
        Sign        struct{ CanonicalSign string `json:"canonical_sign"` } `json:"sign"`
        Interpretant struct {
            OperationalClaim string  `json:"operational_claim"`
            Confidence       float64 `json:"confidence"`
        } `json:"interpretant"`
        Provenance   struct{ SourceRef string `json:"source_ref"` } `json:"provenance"`
        SocialStatus string `json:"social_status"`
    }
    if err := json.Unmarshal(b, &raw); err != nil {
        return err
    }
    r.MemoryID = raw.MemoryID
    r.CanonicalSign = raw.Sign.CanonicalSign
    r.OperationalClaim = raw.Interpretant.OperationalClaim
    r.Confidence = raw.Interpretant.Confidence
    r.SourceRef = raw.Provenance.SourceRef
    r.SocialStatus = raw.SocialStatus
    r.Raw = json.RawMessage(b)
    return nil
}

// ---- Methods ----

func (c *HTTPClient) Health(ctx context.Context) (*HealthData, error) {
    var data HealthData
    return &data, c.doJSON(ctx, "GET", "/api/v1/health", nil, &data)
}

func (c *HTTPClient) RecallMulti(ctx context.Context, req RecallMultiReq) ([]Record, error) {
    var data []Record
    return data, c.doJSON(ctx, "POST", "/api/v1/memory/recall_multi", req, &data)
}

func (c *HTTPClient) IngestAsync(ctx context.Context, req IngestAsyncReq) (string, error) {
    var data struct{ TaskID string `json:"task_id"` }
    err := c.doJSON(ctx, "POST", "/api/v1/memory/ingest_async", req, &data)
    return data.TaskID, err
}

func (c *HTTPClient) Promote(ctx context.Context, memoryID, status string) error {
    path := fmt.Sprintf("/api/v1/memory/%s/promote", url.PathEscape(memoryID))
    return c.doJSON(ctx, "POST", path, map[string]string{"status": status}, nil)
}

func (c *HTTPClient) Revise(ctx context.Context, memoryID string, req ReviseReq) error {
    path := fmt.Sprintf("/api/v1/memory/%s/revise", url.PathEscape(memoryID))
    return c.doJSON(ctx, "POST", path, req, nil)
}

func (c *HTTPClient) Delete(ctx context.Context, memoryID, reason string) error {
    path := fmt.Sprintf("/api/v1/memory/%s?reason=%s",
        url.PathEscape(memoryID), url.QueryEscape(reason))
    return c.doJSON(ctx, "DELETE", path, nil, nil)
}

// ---- Internal ----

func (c *HTTPClient) doJSON(ctx context.Context, method, path string, in, out any) error {
    var body io.Reader
    if in != nil {
        b, err := json.Marshal(in)
        if err != nil {
            return err
        }
        body = bytes.NewReader(b)
    }
    req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
    if err != nil {
        return err
    }
    req.Header.Set("Content-Type", "application/json")
    if c.Token != "" {
        req.Header.Set("Authorization", "Bearer "+c.Token)
    }

    resp, err := c.HTTP.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    raw, _ := io.ReadAll(resp.Body)
    if resp.StatusCode >= 400 {
        return fmt.Errorf("deepmemory %s %s: HTTP %d: %s", method, path, resp.StatusCode, string(raw))
    }
    var env Envelope
    if err := json.Unmarshal(raw, &env); err != nil {
        return err
    }
    if env.Code != 0 {
        return fmt.Errorf("deepmemory %s %s: code=%d, msg=%s", method, path, env.Code, env.Message)
    }
    if out != nil {
        return json.Unmarshal(env.Data, out)
    }
    return nil
}
```

**就这么多。 ≈ 150 行,没有任何 MCP / 子进程 / RPC 依赖。**

---

## 2. FlashMemory HTTP Client

类似的薄包装,新文件 `gateway/flashmemory/client.go`:

```go
package flashmemory

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "time"
)

type Client struct {
    BaseURL string
    Token   string
    HTTP    *http.Client
}

func NewClient(baseURL, token string, timeout time.Duration) *Client {
    return &Client{
        BaseURL: baseURL,
        Token:   token,
        HTTP:    &http.Client{Timeout: timeout},
    }
}

type SearchReq struct {
    Query       string `json:"query"`
    ProjectDir  string `json:"project_dir"`
    TopK        int    `json:"top_k"`
    Language    string `json:"language,omitempty"`
    SearchType  string `json:"search_type,omitempty"` // functions / modules
}

type SearchHit struct {
    FunctionName string  `json:"function_name"`
    FilePath     string  `json:"file_path"`
    LineNumber   int     `json:"line_number"`
    Package      string  `json:"package"`
    Score        float64 `json:"score"`
    Summary      string  `json:"summary"`
    CodeSnippet  string  `json:"code_snippet"`
}

func (c *Client) Search(ctx context.Context, req SearchReq) ([]SearchHit, error) {
    body, _ := json.Marshal(req)
    httpReq, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/api/search", bytes.NewReader(body))
    if err != nil {
        return nil, err
    }
    httpReq.Header.Set("Content-Type", "application/json")
    if c.Token != "" {
        httpReq.Header.Set("Authorization", "Bearer "+c.Token)
    }
    resp, err := c.HTTP.Do(httpReq)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    raw, _ := io.ReadAll(resp.Body)
    if resp.StatusCode >= 400 {
        return nil, fmt.Errorf("flashmemory search: HTTP %d: %s", resp.StatusCode, string(raw))
    }
    var env struct {
        Code int            `json:"code"`
        Data []SearchHit    `json:"data"`
    }
    if err := json.Unmarshal(raw, &env); err != nil {
        return nil, err
    }
    if env.Code != 0 {
        return nil, fmt.Errorf("flashmemory search code=%d", env.Code)
    }
    return env.Data, nil
}
```

> ⚠️ **依赖**: `flashmemory` 团队的 `/api/search` 字段契约 —— 见
> [`flashmemory_team.md` §3](./flashmemory_team.md#3-apisearch-字段稳定承诺)。
> 如果他们改字段名,你要同步更新 struct tag。

---

## 3. 入口 Middleware:`MemoryAugmentation`

**职责**: 在 `react.Agent.Generate` 之前,自动调 DeepMemory `recall_multi`,把
召回的记忆作为 system context 注入。**这就替代了原来的 `memory-augmented-reply` Skill。**

新文件 `gateway/middleware/memory_augmentation.go`:

```go
package middleware

import (
    "context"
    "fmt"
    "log"
    "strings"
    "time"

    "github.com/cloudwego/eino/schema"
    "your-project/gateway/deepmemory"
)

type MemoryAugmentationConfig struct {
    Client          deepmemory.Client
    Enabled         bool
    MaxRecall       int           // 默认 10
    MinConfidence   float64       // 默认 0.35
    SessionIDKey    string        // ctx key,如 "session_id"
    UserIDKey       string        // ctx key,如 "user_id"
    InjectAsSystem  bool          // true=独立 system msg;false=拼接现有
    RecallTimeout   time.Duration // 超时则跳过(默认 200ms)
}

// AugmentMessages 在 react.Agent.Generate 之前调用一次。
// 召回失败 / 超时 时静默通过 —— 不阻塞对话。
func (cfg *MemoryAugmentationConfig) AugmentMessages(
    ctx context.Context,
    messages []*schema.Message,
) ([]*schema.Message, error) {
    if !cfg.Enabled || cfg.Client == nil {
        return messages, nil
    }
    sessionID, _ := ctx.Value(cfg.SessionIDKey).(string)
    userID, _ := ctx.Value(cfg.UserIDKey).(string)

    objectIDs := make([]string, 0, 2)
    if sessionID != "" {
        objectIDs = append(objectIDs, "session:"+sessionID)
    }
    if userID != "" {
        objectIDs = append(objectIDs, "user:"+userID)
    }
    if len(objectIDs) == 0 {
        return messages, nil
    }

    callCtx := ctx
    if cfg.RecallTimeout > 0 {
        var cancel context.CancelFunc
        callCtx, cancel = context.WithTimeout(ctx, cfg.RecallTimeout)
        defer cancel()
    }

    memories, err := cfg.Client.RecallMulti(callCtx, deepmemory.RecallMultiReq{
        ObjectIDs:     objectIDs,
        Limit:         orDefault(cfg.MaxRecall, 10),
        MinConfidence: orDefaultF(cfg.MinConfidence, 0.35),
    })
    if err != nil {
        log.Printf("[memory-aug] recall failed (non-fatal): %v", err)
        return messages, nil
    }
    if len(memories) == 0 {
        return messages, nil
    }

    block := buildContextBlock(memories)

    if cfg.InjectAsSystem {
        return append([]*schema.Message{schema.SystemMessage(block)}, messages...), nil
    }
    for i, m := range messages {
        if m.Role == schema.System {
            messages[i] = schema.SystemMessage(m.Content + "\n\n" + block)
            return messages, nil
        }
    }
    return append([]*schema.Message{schema.SystemMessage(block)}, messages...), nil
}

func buildContextBlock(memories []deepmemory.Record) string {
    var sb strings.Builder
    sb.WriteString("## Recalled Memories\n\n")
    sb.WriteString("Persistent observations from prior sessions. ")
    sb.WriteString("CANONICAL=fact, PROVISIONAL_CONSENSUS=reliable, ")
    sb.WriteString("LOCAL_HYPOTHESIS=single-source, CONTESTED=ignore.\n\n")
    for _, m := range memories {
        sb.WriteString(fmt.Sprintf(
            "- **[%s]** (%s, conf=%.2f) %s — _src: %s_\n",
            m.CanonicalSign,
            strings.ToUpper(m.SocialStatus),
            m.Confidence,
            m.OperationalClaim,
            m.SourceRef,
        ))
    }
    return sb.String()
}

func orDefault(v, def int) int        { if v <= 0 { return def }; return v }
func orDefaultF(v, def float64) float64 { if v <= 0 { return def }; return v }
```

---

## 4. 出口 Middleware:`SymbolismAsyncIngest`

**职责**: 在 `react.Agent.Generate` 完成后,异步把 user 输入 + assistant 回复打包扔给
DeepMemory `ingest_async`。**这就替代了原来的 `symbolism-extract` Skill。**

新文件 `gateway/middleware/symbolism_async.go`:

```go
package middleware

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/cloudwego/eino/schema"
    "your-project/gateway/deepmemory"
)

type SymbolismAsyncConfig struct {
    Client       deepmemory.Client
    Enabled      bool
    SessionIDKey string
    UserIDKey    string
    Timeout      time.Duration // 默认 30s
}

// FireAndForget 立即返回。HTTP 请求在 detached background context 中执行。
func (cfg *SymbolismAsyncConfig) FireAndForget(
    ctx context.Context,
    userMessage string,
    assistantReply *schema.Message,
) {
    if !cfg.Enabled || cfg.Client == nil || assistantReply == nil {
        return
    }
    sessionID, _ := ctx.Value(cfg.SessionIDKey).(string)
    userID, _ := ctx.Value(cfg.UserIDKey).(string)

    timeout := cfg.Timeout
    if timeout == 0 {
        timeout = 30 * time.Second
    }

    payload := deepmemory.IngestAsyncReq{
        TurnText:  fmt.Sprintf("USER: %s\n\nASSISTANT: %s", userMessage, assistantReply.Content),
        SessionID: sessionID,
        ActorRef:  "user:" + userID,
        SourceRef: fmt.Sprintf("session-%s-turn-%d", sessionID, time.Now().UnixNano()),
    }

    // 关键:不要继承 ctx,主请求结束后会被 cancel
    go func() {
        bgCtx, cancel := context.WithTimeout(context.Background(), timeout)
        defer cancel()
        if _, err := cfg.Client.IngestAsync(bgCtx, payload); err != nil {
            log.Printf("[symbolism-async] non-fatal ingest failed: %v", err)
        }
    }()
}
```

---

## 5. 把 middleware 装进 HTTP handler

修改你现有的 chat handler:

```go
// gateway/handler/chat.go

func ChatHandler(
    agent *react.Agent,
    memMW *middleware.MemoryAugmentationConfig,
    symMW *middleware.SymbolismAsyncConfig,
    fmClient *flashmemory.Client,
) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        var req ChatRequest
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            http.Error(w, err.Error(), 400)
            return
        }

        // 注入 ctx
        ctx := r.Context()
        ctx = context.WithValue(ctx, memMW.SessionIDKey, req.SessionID)
        ctx = context.WithValue(ctx, memMW.UserIDKey, req.UserID)
        // FlashMemory 项目目录:从配置或会话级元数据拿
        ctx = context.WithValue(ctx, "fm_project_dir", req.ProjectDir)

        msgs := []*schema.Message{
            schema.SystemMessage(req.SystemPrompt),
            schema.UserMessage(req.UserMessage),
        }

        // ★ 入口:记忆 + 代码上下文注入
        msgs, _ = memMW.AugmentMessages(ctx, msgs)
        // (可选)如果用户消息明显涉及代码,加调一次 fm.Search 注入到 system
        if isCodeQuery(req.UserMessage) {
            hits, _ := fmClient.Search(ctx, flashmemory.SearchReq{
                Query: req.UserMessage, ProjectDir: req.ProjectDir,
                TopK: 5, SearchType: "functions",
            })
            msgs = injectCodeContext(msgs, hits)
        }

        // ★ 调 react.Agent
        reply, err := agent.Generate(ctx, msgs)
        if err != nil {
            http.Error(w, err.Error(), 500)
            return
        }

        // ★ 出口:异步抽取
        symMW.FireAndForget(ctx, req.UserMessage, reply)

        json.NewEncoder(w).Encode(ChatResponse{Reply: reply.Content})
    }
}
```

---

## 6. 注册保留的 2 个 Skill

把这两个 SKILL.md 放在你的 eino skill backend 配的目录下(默认 `configs/skills/`):

```
configs/skills/
├── game-theory-debate/
│   └── SKILL.md
└── user-feedback-handler/
    └── SKILL.md
```

文件内容直接复用:
- [`docs/integration/skills/game-theory-debate.md`](../skills/game-theory-debate.md)
- [`docs/integration/skills/user-feedback-handler.md`](../skills/user-feedback-handler.md)

但要**修改 description 字段**,让 LLM 在纯 B 模式下能准确触发。改后的 description 见
[`gateway_integration_eino.md` §3.1](../gateway_integration_eino.md#31-优化后的-description-文案关键)。

### 6.1 给保留 Skill 提供配套工具

这两个 Skill 的 SKILL.md 里写的是"调 deepmemory_promote / revise / delete"。
LLM 不能直接调 HTTP,需要你把这 3 个操作注册为 eino tool:

```go
// gateway/tools/deepmemory_tools.go

func RegisterDeepMemoryTools(toolPool *tool.Pool, client deepmemory.Client) {
    toolPool.Register(tool.NewFromFunc("deepmemory_promote",
        "Promote a memory's social status. Use when user explicitly confirms a recalled memory.",
        func(ctx context.Context, args struct {
            MemoryID string `json:"memory_id"`
            Status   string `json:"status"`
        }) (string, error) {
            err := client.Promote(ctx, args.MemoryID, args.Status)
            if err != nil { return "", err }
            return "promoted", nil
        }))

    toolPool.Register(tool.NewFromFunc("deepmemory_revise",
        "Revise a memory's claim or confidence. Use when user corrects a recalled memory.",
        func(ctx context.Context, args struct {
            MemoryID         string  `json:"memory_id"`
            OperationalClaim string  `json:"operational_claim"`
            Confidence       float64 `json:"confidence"`
        }) (string, error) {
            // 用 nil-able pointers 跳过未提供的字段
            req := deepmemory.ReviseReq{}
            if args.OperationalClaim != "" {
                req.OperationalClaim = &args.OperationalClaim
            }
            if args.Confidence > 0 {
                req.Confidence = &args.Confidence
            }
            err := client.Revise(ctx, args.MemoryID, req)
            if err != nil { return "", err }
            return "revised", nil
        }))

    toolPool.Register(tool.NewFromFunc("deepmemory_delete",
        "Delete a memory. Use when user disagrees with or requests forgetting a memory.",
        func(ctx context.Context, args struct {
            MemoryID string `json:"memory_id"`
            Reason   string `json:"reason"`
        }) (string, error) {
            err := client.Delete(ctx, args.MemoryID, args.Reason)
            if err != nil { return "", err }
            return "deleted", nil
        }))
}
```

---

## 7. 配置文件

新增 / 修改 `fm.yaml`:

```yaml
deepmemory:
  base_url: "http://127.0.0.1:8765"   # 或生产环境的 https://memory.your-domain.com
  token_env: "DEEPMEMORY_API_TOKEN"   # Bearer token 从环境变量读
  timeout_sec: 5

flashmemory:
  base_url: "http://127.0.0.1:8766"
  token_env: "FM_API_TOKEN"
  default_project_dir: "/path/to/your/project"
  timeout_sec: 5

memory_middleware:
  augmentation:
    enabled: true
    max_recall: 10
    min_confidence: 0.35
    inject_as_system: false   # 拼到现有 system msg
    recall_timeout_ms: 200    # 超时静默跳过
  symbolism_async:
    enabled: true
    timeout_sec: 30
```

启动时读 yaml,把字段塞给两个 middleware 的 config 对象。

---

## 8. 启动顺序 / 依赖图

```
Step 1. 等 DeepMemory 团队的 v1.1 HTTP server 上线
   └ 验证: curl http://deepmemory:8765/api/v1/health 应返回 {"code":0}

Step 2. 等 FlashMemory 团队的 OpenAPI + 字段契约确认
   └ 验证: curl -X POST http://flashmemory:8766/api/search -d '{"query":"test","project_dir":"/x","top_k":1}' 字段名匹配你的 client struct

Step 3. (你这边) 加 client / middleware / handler 接线 / Skill 注册

Step 4. 重启网关
   └ ToolInfo 缓存机制:react.Agent 启动时读取 SKILL.md,所以新加 skill 必须重启或调 /api/v1/skills/refresh

Step 5. 跑 §10 验证
```

---

## 9. 测试要求

| 文件 | 覆盖点 |
|------|-------|
| `gateway/deepmemory/client_test.go` | 用 `httptest.NewServer` mock,覆盖 health/recall_multi/ingest_async happy path + 错误码 |
| `gateway/flashmemory/client_test.go` | 同上 |
| `gateway/middleware/memory_augmentation_test.go` | recall 成功 → 注入 / recall 失败 → 透传 / 超时 → 透传 / no session_id → 透传 |
| `gateway/middleware/symbolism_async_test.go` | FireAndForget 立即返回 / 后台任务调 client / 主 ctx cancel 后任务仍能完成 |
| `gateway/handler/chat_test.go` | 端到端:fake agent + fake client + 验证消息流 |

---

## 10. 验证清单

按顺序跑,每步绿后再下一步:

```bash
# 10.1 后端可达
curl -s http://deepmemory:8765/api/v1/health | jq .code     # 应输出 0
curl -s http://flashmemory:8766/api/health     | jq .code   # 应输出 0

# 10.2 网关启动日志应有
[memory-aug] enabled, base_url=http://deepmemory:8765
[symbolism-async] enabled, timeout=30s
[skills] loaded: game-theory-debate, user-feedback-handler

# 10.3 端到端:发对话,验证记忆注入
# 先手动 ingest 一条
curl -X POST http://deepmemory:8765/api/v1/memory/ingest -H 'Content-Type: application/json' -d '{
  "summary":"user prefers dark mode","object_id":"user:preference:theme",
  "object_type":"user_preference","anchor_kind":"config","anchor_locator":"settings.json",
  "template":"task","actor_ref":"user:test","source_ref":"manual"
}'

# 然后向网关发对话
curl -X POST http://localhost:8080/api/chat -H 'Content-Type: application/json' -d '{
  "session_id":"s1","user_id":"test","user_message":"我刚才说要什么主题来着?"
}'
# AI 回复应提到 "dark mode" → 证明召回 + 注入路径打通

# 10.4 端到端:验证异步抽取
curl -X POST http://localhost:8080/api/chat -H 'Content-Type: application/json' -d '{
  "session_id":"s2","user_id":"test","user_message":"我用 uv 不用 pip"
}'
# 等 5 秒
curl -s "http://deepmemory:8765/api/v1/memory/recall?object_id=user:preference:packaging" | jq
# 应看到一条包含 'uv' 的记录 → 证明异步抽取路径打通

# 10.5 端到端:验证 Skill 触发
# 接 10.3 的对话之后再发
curl -X POST http://localhost:8080/api/chat -H 'Content-Type: application/json' -d '{
  "session_id":"s1","user_id":"test","user_message":"对,主题就是 dark mode"
}'
# 网关日志应看到 [skill] launched: user-feedback-handler
# 然后 deepmemory 那条记忆 social_status 应升到 provisional_consensus

# 10.6 evolution daemon 运转
curl -s http://deepmemory:8765/api/v1/health | jq .data.evolve_daemon_running   # 应输出 true
```

---

## 11. Token 成本(实测口径)

按你之前给出的 eino skill middleware 实测数据外推:

| 场景 | 每轮对话增量 |
|------|------------|
| skill 工具描述(2 个 SKILL) | ~200 tokens |
| 入口 middleware 注入的记忆块 | 200-800 tokens(实际有用) |
| 出口 middleware | 0 tokens(异步,主上下文不污染) |
| evolution daemon | 0 tokens(完全后台) |
| 触发 user-feedback-handler 那一轮 | +1500 tokens(SKILL.md 全文) |
| 触发 game-theory-debate 那一轮 | +3000 tokens(SKILL.md 全文) |

**典型一轮: 400-1000 tokens 增量。** 不是浪费,是有用上下文。

---

## 12. 完成标准 (Definition of Done)

```
[ ] gateway/deepmemory/client.go 实现 + 测试通过
[ ] gateway/flashmemory/client.go 实现 + 测试通过
[ ] gateway/middleware/memory_augmentation.go 实现 + 测试通过
[ ] gateway/middleware/symbolism_async.go 实现 + 测试通过
[ ] gateway/tools/deepmemory_tools.go 注册 promote/revise/delete 三个 tool
[ ] handler 接线 + ctx 注入 session_id/user_id
[ ] configs/skills/game-theory-debate/SKILL.md 加 + description 优化
[ ] configs/skills/user-feedback-handler/SKILL.md 加 + description 优化
[ ] fm.yaml 配置段加好 + 加载逻辑
[ ] §10 6 项验证全绿
[ ] 通知 boss / 客户:5 个能力全部上线
```

---

## 13. 跟其他角色的接口契约

| 找谁要什么 |
|------|
| **DeepMemory 团队** → `recall_multi` / `ingest_async` 字段稳定 + Bearer token 鉴权;P99 延迟承诺;[`deepmemory_team.md`](./deepmemory_team.md) |
| **FlashMemory 团队** → `/api/search` 字段稳定 + OpenAPI;[`flashmemory_team.md`](./flashmemory_team.md) |
| **运维 / SRE** → 部署 deepmemory + fm_http 两个服务;Bearer token 注入;反向代理;Prometheus 抓 /metrics(后续) |

---

## 14. 你**不**需要做的事(明确拒绝范围蔓延)

| 别人的请求 | 回应 |
|----------|------|
| "能不能让 LLM 自己每轮决定要不要召回?" | 拒绝。机械触发的事下沉到 middleware,LLM 决策反而不稳 |
| "能不能直接读 DeepMemory 的 jsonl 文件?" | 拒绝。所有跨组件交互走 HTTP,不绕开契约 |
| "能不能在网关跑 evolution?" | 拒绝。infra 行为属于 DeepMemory 后台 daemon,不是网关职责 |
| "能不能加 5 个 Skill 全部接进来?" | 拒绝。其中 3 个不适合 LLM 决策,见 [`gateway_integration_eino.md`](../gateway_integration_eino.md) |
| "能不能让 fm_http 通过 DeepMemory 调用?" | 拒绝。两个服务并列,不嵌套调用 |

---

**问题反馈**: 在 issue tracker 用 `[gateway-eino]` 前缀
**对接人**: DeepMemory 团队([`deepmemory_team.md`](./deepmemory_team.md))、FlashMemory 团队([`flashmemory_team.md`](./flashmemory_team.md))
