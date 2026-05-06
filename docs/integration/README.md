# AI 系统集成指南：FlashMemory + DeepMemory

> **版本**: 1.0.0  
> **日期**: 2026-05-01  
> **定位**: 面向已有 AI 对话系统的零侵入集成方案  
> **Slogan**: 用符号学解构语义、用博弈增强群体记忆

---

## 你将获得什么

集成完成后，你的 AI 系统将具备：

| 能力 | 传统 RAG | 集成后 |
|------|---------|--------|
| 语义表示 | 纯向量（黑盒） | **Sign + Interpretant**（可解释的符号秩序） |
| 决策溯源 | 无 | **Provenance 链**（谁在什么上下文观察到什么） |
| 长期学习 | 不支持 | **社会状态机**（记忆从 PRIVATE_HINT 演化到 CANONICAL） |
| 多观点处理 | 不支持 | **博弈论论证**（多 agent 论证 → 共识投票） |
| 群体协调 | 不支持 | **进化周期**（自动衰减过期观察、淘汰低置信记忆） |
| 代码检索 | 基础关键词 | **混合搜索**（Dense + Sparse + RRF 融合 + 交叉编码器精排） |

**你的系统代码改动量：0 行。** 全部通过 MCP 协议 + Skills 配置实现。

---

## 整体架构

```
┌────────────────────────────────────────────────────────┐
│                  你的 AI 系统（零改动）                   │
│                                                        │
│   对话引擎 ─── 智能体编排 ─── 会话管理 ─── 模型路由     │
│       │              │                                 │
│   ┌───┴──────────────┴───┐                             │
│   │  MCP Client + Skills │  ← 你已有的 MCP/Skills 框架  │
│   └───┬──────────┬───────┘                             │
│       │          │                                     │
├───────┼──────────┼─────────────────────────────────────┤
│   MCP │ 协议  MCP│ 协议   (stdio / HTTP)                │
├───────┼──────────┼─────────────────────────────────────┤
│       ▼          ▼                                     │
│ ┌───────────┐ ┌──────────────┐                         │
│ │FlashMemory│ │  DeepMemory  │                         │
│ │MCP Server │ │  MCP Server  │                         │
│ │ 3 个工具  │ │  7 个工具    │                         │
│ │ 代码检索  │ │ 认知记忆管理 │                         │
│ └───────────┘ └──────────────┘                         │
└────────────────────────────────────────────────────────┘
```

---

## 前置条件

### 环境要求

```bash
Python >= 3.9
pip install flashmemory[mcp]    # FlashMemory SDK + MCP 依赖
pip install deepmemory[mcp]     # DeepMemory SDK + MCP 依赖
```

如果 `[mcp]` extra 尚未发布，手动安装：

```bash
pip install flashmemory
pip install "mcp>=1.2.0,<2"

# DeepMemory（从本地安装）
cd deepmemory && pip install -e ".[mcp]"
```

### 你的系统需要支持

- [x] 注册 MCP Server（stdio 或 HTTP 传输）
- [x] 注册 Skills（SKILL.md 格式或等效的 YAML/JSON）
- [x] 对话生命周期钩子（before_reply / after_reply / on_feedback）

---

## 第一步：注册 MCP Servers

### 1.1 DeepMemory MCP Server

DeepMemory 提供认知记忆管理——记忆的摄入、召回、晋升、修订、删除和自进化。

**注册配置**（添加到你的 MCP 配置文件）：

```json
{
  "mcpServers": {
    "deepmemory": {
      "command": "python",
      "args": ["-m", "deepmemory.mcp_server"],
      "env": {
        "DEEPMEMORY_STORE": "./artifacts/memory.jsonl"
      }
    }
  }
}
```

**可用工具（7个）**：

| 工具 | 用途 | 必需参数 |
|------|------|---------|
| `deepmemory_ingest` | 摄入新记忆 | `summary`, `object_id` |
| `deepmemory_recall` | 按对象召回记忆 | `object_id` |
| `deepmemory_promote` | 社会状态晋升 | `memory_id`, `status` |
| `deepmemory_revise` | 修订记忆内容 | `memory_id` |
| `deepmemory_delete` | 删除记忆 | `memory_id` |
| `deepmemory_evolve` | 运行进化周期 | （无，全可选） |
| `deepmemory_events` | 读取事件日志 | （无，全可选） |

**验证**：

```bash
# 启动服务器并列出工具
echo '{"jsonrpc":"2.0","method":"tools/list","id":1}' | python -m deepmemory.mcp_server
```

### 1.2 FlashMemory MCP Server

FlashMemory 提供代码语义检索——混合搜索、增量索引、引擎诊断。

**方式 A：stdio Server（推荐，无需额外进程）**

```json
{
  "mcpServers": {
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

**方式 B：Go HTTP 服务（如果已在运行）**

```bash
# 启动 HTTP 服务
fm serve --port 5532

# 你的系统通过 HTTP 调用
curl http://localhost:5532/tools/mcp   # 获取工具定义
curl -X POST http://localhost:5532/tools/invoke \
  -d '{"tool":"fm.search.plan","input":{"query":"auth middleware"}}'
```

**可用工具（3个）**：

| 工具 | 用途 | 必需参数 |
|------|------|---------|
| `flashmemory_search` | 代码/文档混合搜索 | `query` |
| `flashmemory_index` | 添加函数到索引 | `func_id`, `text` |
| `flashmemory_info` | 引擎状态诊断 | （无） |

### 1.3 完整 MCP 配置（两个 Server 合并）

```json
{
  "mcpServers": {
    "deepmemory": {
      "command": "python",
      "args": ["-m", "deepmemory.mcp_server"],
      "env": {
        "DEEPMEMORY_STORE": "./artifacts/memory.jsonl"
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

---

## 第二步：注册 Skills

Skills 是声明式的编排层——用提示词 + MCP 工具组合定义高级行为，不需要写代码。

### 2.1 复制 Skill 文件

将以下 5 个 Skill 目录复制到你的系统的 Skills 注册目录：

```bash
cp -r skills/symbolism-extract      your_app/skills/
cp -r skills/memory-augmented-reply  your_app/skills/
cp -r skills/game-theory-debate      your_app/skills/
cp -r skills/memory-evolution-cycle  your_app/skills/
cp -r skills/user-feedback-handler   your_app/skills/
```

每个目录包含一个 `SKILL.md` 文件，格式为 YAML front matter + Markdown 指令。

### 2.2 Skill 功能概览

#### `symbolism-extract` — 符号学解构引擎

**触发时机**：每轮对话后（after_reply）

**做什么**：分析对话消息，提取 Sign（关键概念）+ ObjectRef（对象引用）+ Interpretant（解释立场），调用 `deepmemory_ingest` 持久化。

**工作流**：
```
对话消息
  → 提取关键概念（Sign: packaging_tool_preference）
  → 锚定到对象（ObjectRef: user:preference:packaging）
  → 记录解释（Interpretant: "用户偏好uv", confidence=0.85）
  → deepmemory_recall 检查去重
  → deepmemory_ingest 写入新记忆
```

**这就是「用符号学解构语义」的具体落地。**

#### `memory-augmented-reply` — 记忆增强回复

**触发时机**：每轮对话前（before_reply）

**做什么**：从 DeepMemory 召回历史决策和用户偏好，从 FlashMemory 检索相关代码，合并注入到系统提示中。

**工作流**：
```
用户消息
  → deepmemory_recall（会话记忆 + 用户偏好 + 主题记忆）
  → flashmemory_search（相关代码函数）
  → 格式化为增强上下文
  → 注入系统提示
```

#### `game-theory-debate` — 博弈论记忆论证

**触发时机**：检测到记忆冲突时 / 手动触发

**做什么**：围绕一条争议记忆，发起多 agent 论证（3 轮），投票决定晋升/修订/删除。

**工作流**：
```
争议记忆
  → deepmemory_recall 获取相关记忆
  → 3 个 agent 分别阐述立场（Advocate / Challenger / Arbiter）
  → 3 轮论证（开场 → 交叉质询 → 终陈述）
  → 投票（CONFIRM / REVISE / DEPRECATE）
  → deepmemory_promote 或 deepmemory_revise 或 deepmemory_delete
```

**这就是「用博弈增强群体记忆」的具体落地。**

#### `memory-evolution-cycle` — 记忆进化周期

**触发时机**：每 10 轮对话 / 每日定时

**做什么**：调用 `deepmemory_evolve` 自动衰减过期观察、淘汰低置信记忆、精简单对象记忆数。

#### `user-feedback-handler` — 用户反馈处理

**触发时机**：用户给出反馈时（同意/否定/修正）

**做什么**：将用户反馈映射为 DeepMemory 操作——同意 → promote，否定 → delete，修正 → revise。

---

## 第三步：绑定对话钩子

将 Skills 绑定到你的对话引擎的生命周期钩子。

### 3.1 配置示例

```yaml
dialog_hooks:
  # 回复前：注入记忆 + 代码上下文
  before_reply:
    - skill: memory-augmented-reply
      config:
        recall_dimensions:
          - "session:{session_id}"
          - "user:{user_id}"
        code_search_enabled: true
        max_memories: 10
        max_code_results: 5

  # 回复后：提取符号
  after_reply:
    - skill: symbolism-extract
      config:
        extract_from: both        # user + ai 两侧都提取
        min_confidence: 0.40
        dedup_check: true

  # 用户反馈
  on_user_feedback:
    - skill: user-feedback-handler

  # 定时进化
  scheduled:
    - skill: memory-evolution-cycle
      cron: "0 2 * * *"           # 每天凌晨 2 点
      config:
        min_confidence: 0.35
        stale_after_hours: 168
        max_memories_per_object: 20

  # 冲突检测（可选）
  on_memory_conflict:
    - skill: game-theory-debate
      config:
        agents: [analyst, coder, reviewer]
        max_rounds: 3
```

### 3.2 钩子的最小实现

如果你的系统还没有钩子框架，最简形式是在对话循环中手动调用：

```python
# 伪代码 — 展示调用链路，不是需要复制的代码
async def chat_turn(user_message, session_id, user_id):
    
    # ① before_reply: 召回记忆 + 检索代码
    memories = await mcp_call("deepmemory_recall", {
        "object_id": f"session:{session_id}"
    })
    user_prefs = await mcp_call("deepmemory_recall", {
        "object_id": f"user:{user_id}"
    })
    code_results = await mcp_call("flashmemory_search", {
        "query": user_message, "top_k": 5
    })
    
    # 注入上下文
    context = format_context(memories, user_prefs, code_results)
    
    # ② 生成回复
    ai_reply = await llm.generate(
        user_message=user_message,
        system_prompt=base_prompt + context
    )
    
    # ③ after_reply: 提取符号并记录
    # （由 symbolism-extract Skill 处理）
    await invoke_skill("symbolism-extract", {
        "message": ai_reply,
        "actor_ref": "ai-agent",
        "session_id": session_id
    })
    
    return ai_reply
```

---

## 第四步：验证集成

### 4.1 运行验证脚本

```bash
python scripts/verify_mcp_integration.py
```

预期输出：25/25 全部通过。

### 4.2 手动验证流程

**测试 1：记忆摄入与召回**

```bash
# 1. 摄入一条记忆
echo '{"method":"tools/call","params":{"name":"deepmemory_ingest","arguments":{"summary":"用户偏好暗色主题","object_id":"user:preference:theme","object_type":"user_preference","anchor_locator":"settings.json"}}}' \
  | python -m deepmemory.mcp_server

# 2. 召回
echo '{"method":"tools/call","params":{"name":"deepmemory_recall","arguments":{"object_id":"user:preference:theme"}}}' \
  | python -m deepmemory.mcp_server
```

**测试 2：代码搜索**

```bash
# 确保项目已索引
fm index /path/to/your/project

# 搜索
echo '{"method":"tools/call","params":{"name":"flashmemory_search","arguments":{"query":"用户认证中间件","project_dir":"/path/to/your/project"}}}' \
  | python -m flashmemory.mcp_server
```

**测试 3：完整对话流**

1. 发送消息 "我喜欢用 TypeScript 而不是 JavaScript"
2. 验证 `symbolism-extract` 是否记录了 `user:preference:language` 记忆
3. 在下一轮对话中验证 `memory-augmented-reply` 是否召回了这条偏好
4. 发送 "不对，两个都用" 验证 `user-feedback-handler` 是否修订了记忆

---

## 配置参考

### 环境变量

| 变量 | 用途 | 默认值 |
|------|------|--------|
| `DEEPMEMORY_STORE` | 记忆存储文件路径 | `./artifacts/memory.jsonl` |
| `DEEPMEMORY_DEBUG` | 启用调试日志（设为任意非空值） | 空 |
| `FM_DEFAULT_PROJECT` | FlashMemory 默认项目路径 | 无（必须指定） |
| `FM_DEBUG` | 启用调试日志 | 空 |

### DeepMemory 社会状态机

记忆从诞生到权威的生命周期：

```
PRIVATE_HINT         创建时默认状态，未经验证的初步观察
    │
    ▼
LOCAL_HYPOTHESIS     单一来源的假设（coding 模板默认起点）
    │
    ├─────────────────────────────┐
    ▼                             ▼
PROVISIONAL_CONSENSUS          CONTESTED
  多方确认的共识                  有争议，需要论证
    │                             │
    ▼                             │
CANONICAL ◄───────────────────────┘
  权威共识，高度可信
```

### FlashMemory 搜索模式

| 模式 | 策略 | 适用场景 |
|------|------|---------|
| `semantic` | 纯向量相似度 | 概念性查询（"认证逻辑在哪里"） |
| `keyword` | BM25 关键词 | 精确标识符搜索（"handleLogin"） |
| `hybrid` | Dense + Sparse + RRF 融合 | 通用场景（推荐） |

---

## 数据存储

### DeepMemory 存储

```
artifacts/
├── memory.jsonl          # 记忆存储（append-only JSONL）
└── memory_events.jsonl   # 事件日志（审计用）
```

每条记忆的 JSON 结构：

```json
{
  "schema_version": "dm.core.memory-unit.v0.2",
  "memory_id": "mu_abc123...",
  "object_ref": {
    "object_id": "user:preference:packaging",
    "object_type": "user_preference",
    "anchor_kind": "config",
    "anchor_locator": "pyproject.toml"
  },
  "sign": {
    "canonical_sign": "user prefers uv over pip",
    "sign_variants": ["user prefers uv over pip"],
    "sign_modality": "natural_language"
  },
  "interpretant": {
    "actor_ref": "user:alice",
    "intent": "continue_task",
    "operational_claim": "user prefers uv over pip",
    "applicability_scope": "coding",
    "confidence": 0.85
  },
  "social_status": "local_hypothesis",
  "timestamps": {
    "created_at": "2026-05-01T10:30:00+00:00",
    "updated_at": "2026-05-01T10:30:00+00:00"
  }
}
```

### FlashMemory 存储

```
your_project/
└── .gitgo/
    ├── code_index.db           # SQLite（函数元数据 + 调用图）
    └── zvec_collections/       # Zvec 向量索引
        ├── functions/          # 函数级向量
        └── modules/            # 模块级向量
```

---

## 进阶：自定义 object_id 命名规范

`object_id` 是记忆锚定的核心标识。建议使用分层命名：

```
user:{user_id}                         # 用户级偏好
user:{user_id}:preference:{topic}      # 具体偏好
session:{session_id}                   # 会话级上下文
project:{project_name}                 # 项目级决策
module:{module_path}                   # 代码模块级
team:{team_name}:policy:{policy}       # 团队策略
```

示例：
```
user:alice:preference:packaging    → "偏好 uv 管理依赖"
project:myapp:tech:framework       → "使用 FastAPI"
module:auth:middleware              → "JWT + Redis 方案"
team:backend:policy:testing         → "90% 覆盖率目标"
```

---

## 进阶：博弈论论证的触发条件

`game-theory-debate` Skill 何时触发？推荐设置：

| 条件 | 触发方式 | 说明 |
|------|---------|------|
| 同一对象有 ≥2 条矛盾记忆 | `symbolism-extract` 检测到矛盾后触发 | 自动 |
| 记忆被标记为 CONTESTED | `deepmemory_promote` 转到 CONTESTED 时 | 自动 |
| 用户主动发起 | 用户说"这两个说法哪个对？" | 手动 |
| 定期审查 | cron 扫描所有 CONTESTED 状态记忆 | 定时 |

---

## 卸载

如果需要移除集成，只需：

1. 从 MCP 配置中删除 `deepmemory` 和 `flashmemory` 两个 Server
2. 从 Skills 目录中删除 5 个 Skill 文件夹
3. 从对话钩子配置中删除相关绑定

**不需要回滚任何代码改动**，因为没有代码改动。

---

## 故障排查

| 症状 | 原因 | 解决方案 |
|------|------|---------|
| `No module named 'mcp'` | mcp 包未安装 | `pip install "mcp>=1.2.0,<2"` |
| `deepmemory_recall` 返回空 | store 文件不存在或路径错误 | 检查 `DEEPMEMORY_STORE` 环境变量 |
| `flashmemory_search` 返回 error | 项目未索引 | 先运行 `fm index /path/to/project` |
| 记忆重复 | `symbolism-extract` 未做去重检查 | 确保 Skill 配置 `dedup_check: true` |
| 进化删除过多 | `min_confidence` 阈值过高 | 调低到 0.20（保守模式） |
| MCP Server 启动失败 | Python 路径问题 | 确认 `python -c "import deepmemory"` 成功 |

---

## 文件清单

```
# MCP Servers（核心代码）
deepmemory/deepmemory/mcp_server.py      # DeepMemory MCP Server (7 tools)
pip-package/flashmemory/mcp_server.py     # FlashMemory MCP Server (3 tools)

# Skills（编排层 — 运行时加载路径）
.agents/skills/symbolism-extract/SKILL.md
.agents/skills/memory-augmented-reply/SKILL.md
.agents/skills/game-theory-debate/SKILL.md
.agents/skills/memory-evolution-cycle/SKILL.md
.agents/skills/user-feedback-handler/SKILL.md

# Skills 文档副本（与上方运行时文件保持同步）
docs/integration/skills/README.md                  # Skills 总览索引
docs/integration/skills/symbolism-extract.md
docs/integration/skills/memory-augmented-reply.md
docs/integration/skills/game-theory-debate.md
docs/integration/skills/memory-evolution-cycle.md
docs/integration/skills/user-feedback-handler.md

# 配置示例
docs/integration/mcp_config_example.json
docs/integration/dialog_hooks_example.yaml

# 验证脚本
scripts/verify_mcp_integration.py         # 25 项端到端测试
```
