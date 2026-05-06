# 认知记忆主轴：总目录与导读

- 日期：2026-04-23
- 状态：research package index
- 关系：汇总 `2026-04-22-superpower-blueprint-design.md` 同级目录下的记忆主轴研究稿与后续实现规格

## 1. 这组文档在解决什么问题

这组文档试图把 FlashMemory 从“代码解析引擎 / 检索系统”推进为一个更完整的 **认知记忆基座**。它的核心判断是：

> 记忆系统真正要解决的，不是如何保存更多文本，而是如何让符号在多主体、多模态、多轮传播和持续行动中，维持可操作的意义稳定性。

因此，这组文档不是普通的 memory feature 说明，而是一条连续的研究-设计链：

- 先重写“记忆是什么”
- 再定义最小记忆单元
- 再给出个体认知分层
- 再引入 self-play harness
- 再把群体传播与 semiosphere 带进来
- 最后落到 FlashMemory 作为 substrate 的工程位置与实验路线图

## 2. 建议阅读路径

### 路径 A：理论全景

适合第一次进入这组文档、希望先把世界观看清的人。

1. [核心命题与总框架](../specs/memory-research/2026-04-22-memory-01-core-thesis.md)
2. [Semiotic Memory Unit](../specs/memory-research/2026-04-22-memory-02-semiotic-memory-unit.md)
3. [个体认知分层](../specs/memory-research/2026-04-22-memory-03-cognitive-layers.md)
4. [Self-Play Harness](../specs/memory-research/2026-04-22-memory-04-self-play-harness.md)
5. [群体记忆与 Semiosphere](../specs/memory-research/2026-04-22-memory-05-collective-memory-semiosphere.md)
6. [语言传播、压缩与漂移机制](../specs/memory-research/2026-04-22-memory-06-language-propagation-mechanisms.md)
7. [FlashMemory 作为认知基座](../specs/memory-research/2026-04-22-memory-07-flashmemory-as-substrate.md)
8. [实验设计、评测指标与研究路线图](../specs/memory-research/2026-04-22-memory-08-experiments-metrics-roadmap.md)

### 路径 B：实现导向

适合希望尽快进入后续 spec / implementation plan 的人。

1. [核心命题与总框架](../specs/memory-research/2026-04-22-memory-01-core-thesis.md)
2. [Semiotic Memory Unit](../specs/memory-research/2026-04-22-memory-02-semiotic-memory-unit.md)
3. [FlashMemory 作为认知基座](../specs/memory-research/2026-04-22-memory-07-flashmemory-as-substrate.md)
4. [实验设计、评测指标与研究路线图](../specs/memory-research/2026-04-22-memory-08-experiments-metrics-roadmap.md)
5. [实现规格 A：Semiotic Memory Unit Schema v0.1](../specs/memory-research/2026-04-23-semiotic-memory-unit-schema-spec.md)

### 路径 C：群体认知与演化

适合关注 multi-agent society、人机团队协作与共识形成的人。

1. [核心命题与总框架](../specs/memory-research/2026-04-22-memory-01-core-thesis.md)
2. [Self-Play Harness](../specs/memory-research/2026-04-22-memory-04-self-play-harness.md)
3. [群体记忆与 Semiosphere](../specs/memory-research/2026-04-22-memory-05-collective-memory-semiosphere.md)
4. [语言传播、压缩与漂移机制](../specs/memory-research/2026-04-22-memory-06-language-propagation-mechanisms.md)
5. [实验设计、评测指标与研究路线图](../specs/memory-research/2026-04-22-memory-08-experiments-metrics-roadmap.md)

## 3. 八篇研究稿的最短摘要

### 01. 核心命题与总框架

把“记忆”重写为 **符号秩序的演化系统**，提出 `FlashMemory = substrate`、`harness = evolution layer` 的总结构。

### 02. Semiotic Memory Unit

把最小记忆单元从 `chunk` 改写为：

`sign + object + interpretant + context + provenance + confidence + social_status`

### 03. 个体认知分层

给出五层认知结构：

- perceptual buffer
- working memory
- episodic memory
- semantic memory
- metamemory

### 04. Self-Play Harness

把 `harness` 从 benchmark runner 升级为记忆进化环境，用任务、冲突、反思和辩论来筛选记忆。

### 05. 群体记忆与 Semiosphere

引入 Lotman，把多 agent / 人机团队记忆视为带边界、带翻译的群体语义空间。

### 06. 语言传播、压缩与漂移机制

解释记忆如何在总结、复述、转译和扩散中失真，并提出 drift audit 的必要性。

### 07. FlashMemory 作为认知基座

明确 FlashMemory 的战略角色是 object-aware memory substrate，而不是过早变成通用 agent 平台。

### 08. 实验设计、评测指标与研究路线图

给出 `unit -> layer -> arena -> society -> canon` 的推进顺序，以及 semiotic stability、interpretive divergence 等核心指标。

## 4. 概念依赖图

```text
01 核心命题
  ↓
02 memory unit
  ↓
03 cognitive layers
  ↓
04 self-play harness
  ↓
05 semiosphere
  ↓
06 propagation / drift
  ↓
07 FlashMemory substrate
  ↓
08 experiments / roadmap
```

如果从实现视角看，真正的依赖链更接近：

```text
02 semiotic memory unit
  ↓
07 FlashMemory substrate
  ↓
04 harness
  ↓
06 drift audit
  ↓
05 semiosphere + 08 roadmap
```

## 5. 这组文档已经达成的共识

截至目前，这组研究稿已经明确了以下强主张：

- 记忆系统的核心不是 recall，而是 meaning stability。
- 符号学不是附录，而是整套架构的中轴。
- `Peirce + Lotman` 是当前最合适的理论主线。
- `harness` 是“自博弈评测层 + 行动编排层”的混合体。
- FlashMemory 的最佳战略位置是 substrate，而不是先做成完整 agent 成品。

## 6. 下一层实现规格的拆分建议

基于第 08 篇路线图，下一层实现规格建议按以下顺序推进：

1. [实现规格 A：Semiotic Memory Unit Schema v0.1](../specs/memory-research/2026-04-23-semiotic-memory-unit-schema-spec.md)
2. `Self-Play Harness Trial Protocol v0.1`
3. `Social Status State Machine v0.1`
4. `Drift Audit Metrics and Computation v0.1`

当前优先级的理由很简单：没有稳定 schema，后面的 trial protocol、状态机和指标都没有统一操作对象。

## 7. 两个总 spec

在研究稿和实现规格之间，还需要两篇总 spec 来把 FlashMemory 与 DeepMemory 的系统边界正式定清楚：

1. [FlashMemory 总体系统规格 v0.1](../architecture/2026-04-23-flashmemory-total-system-spec.md)
2. [DeepMemory 总体系统规格 v0.1](../../../deepmemory/docs/2026-04-23-deepmemory-total-system-spec.md)

这两篇文档的作用不是重复研究稿，而是把各自系统的模块归属、公开 contract、目录映射和演进阶段写成可执行规范。它们与本导读页的关系是：

- 本导读页回答“记忆主轴在研究上是什么”
- 总 spec 回答“这条主轴如何分别落到 FlashMemory 与 DeepMemory”

## 8. 与原蓝图的关系

这组文章与 [2026-04-22-superpower-blueprint-design.md](../architecture/2026-04-22-superpower-blueprint-design.md) 的关系是：

- 原蓝图仍然是总蓝图，不做替换。
- 本组文档只展开其中的“认知记忆主轴”。
- 它们为后续独立实现 spec 和 implementation plan 提供理论与结构前提。

## 9. 读完之后应该进入哪里

如果目标是继续研究与设计，下一站应进入：

- [实现规格 A：Semiotic Memory Unit Schema v0.1](../specs/memory-research/2026-04-23-semiotic-memory-unit-schema-spec.md)

如果目标是回到产品总蓝图，下一站应返回：

- [FlashMemory Superpower Blueprint](../architecture/2026-04-22-superpower-blueprint-design.md)
