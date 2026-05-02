# Skills 文档索引

本目录是集成方案中 5 个 Skills 的完整定义文档（YAML front matter + Markdown 指令），
与运行时实际加载的 `.agents/skills/*/SKILL.md` 内容一致，便于阅读和审阅。

> 实际运行时使用 `.agents/skills/<skill-name>/SKILL.md` 这一路径；
> 本目录下的同名 `.md` 文件为同步的文档副本，方便在 docs 中检索与跳转。

## Skills 总览

| Skill | 触发时机 | 核心 MCP 工具 | 职责 |
|-------|---------|---------------|------|
| [symbolism-extract](./symbolism-extract.md) | `after_reply` / `after_user_message` | `deepmemory_ingest`, `deepmemory_recall` | 符号学解构 — 从对话中抽取 Sign + ObjectRef + Interpretant |
| [memory-augmented-reply](./memory-augmented-reply.md) | `before_reply` | `deepmemory_recall`, `flashmemory_search` | 记忆 + RAG 上下文注入 |
| [game-theory-debate](./game-theory-debate.md) | `on_memory_conflict` / 手动 | `deepmemory_recall`, `deepmemory_promote/revise/delete` | 多 agent 论证 + 投票决出共识 |
| [memory-evolution-cycle](./memory-evolution-cycle.md) | 每 10 轮 / 每日 cron | `deepmemory_evolve`, `deepmemory_events` | 记忆衰减 / 淘汰 / 修剪 |
| [user-feedback-handler](./user-feedback-handler.md) | `on_user_feedback` | `deepmemory_promote/revise/delete` | 把用户反馈翻译成记忆治理动作 |

## 生命周期视图

```
用户输入
   │
   ▼
[before_reply] ── memory-augmented-reply ──▶ 注入历史记忆 + 相关代码
   │
   ▼
AI 生成回复
   │
   ▼
[after_reply]  ── symbolism-extract ───────▶ 抽取符号 → 写入 DeepMemory
   │
   ▼
用户给出反馈
   │
   ▼
[on_user_feedback] ── user-feedback-handler ──▶ 晋升 / 修订 / 删除

(并行旁路)
[on_memory_conflict] ── game-theory-debate ──▶ 多 agent 论证
[scheduled cron]    ── memory-evolution-cycle ─▶ 自动衰减
```

## Slogan 映射

> **「用符号学解构语义、用博弈增强群体记忆」**

| Slogan 拆解 | 对应 Skills |
|-------------|------------|
| **符号学解构语义** | `symbolism-extract`（抽取符号秩序）+ `memory-augmented-reply`（按符号召回） |
| **博弈增强群体记忆** | `game-theory-debate`（多 agent 投票）+ `user-feedback-handler`（人类作为最强 agent）+ `memory-evolution-cycle`（时间作为裁判） |

## 配置示例

将 5 个 Skills 全部接入 dialog hooks，参考：
[`docs/integration/dialog_hooks_example.yaml`](../dialog_hooks_example.yaml)

主集成文档：[`docs/integration/README.md`](../README.md)
