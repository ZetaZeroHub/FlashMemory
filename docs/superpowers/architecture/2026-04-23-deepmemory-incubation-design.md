# DeepMemory 孵化立项设计

- 日期：2026-04-23
- 状态：design draft
- 关系：基于 `FlashMemory` 仓库内已有记忆研究包与 `DeepMemory` 孵化讨论整理而成
- 目标目录：`/Users/apple/Public/openProject/flashmemory/deepmemory`

## 1. 项目定位

`DeepMemory` 的目标不是再做一个“更强一点的工具层”，而是面向 AI-native 软件系统构建一层可演化的认知基础设施。

项目 slogan 建议固定为：

> 我们不只生产工具，我们致力于成为 AI 认知基础设施。  
> 编程语言成为人工智能的手和脚，符号学成为人工智能的认知引擎。

在这个定位下：

- `FlashMemory` 更偏对象层、图谱层、检索层和代码智能 substrate。
- `DeepMemory` 更偏记忆层、解释层、演化层和认知运行层。

两者不是替代关系，而是上下协作关系：

- `FlashMemory` 负责帮助系统“看见对象”。
- `DeepMemory` 负责帮助系统“形成、维护、修正和传播意义”。

`DeepMemory` 虽然在第一阶段贴近 `AI Coding` 场景工程化落地，但其核心抽象不能被 coding 场景锁死。它必须被设计成一套 `general cognitive infrastructure`，并通过 `domain pack` 机制去吸收不同领域语义。

## 2. 核心用户与市场切口

### 2.1 核心用户

`DeepMemory` 的直接核心用户不是终端开发者，也不是单个 AI agent 使用者，而是：

> 正在构建 AI-native agent 系统、AI coding workspace、agent runtime 和人机协作平台的产品团队与基础设施团队。

这是一个典型的 `platform buyer` / `infra builder` 用户群，而不是单点工具用户群。

### 2.2 用户优先级

建议明确以下优先级：

1. `AI coding platform / coding agent runtime 团队`
2. `Research agent 平台团队`
3. `Enterprise knowledge / copilot 平台团队`

### 2.3 第一阶段切入任务

第一阶段围绕 `AI Coding` 的长期项目记忆来落地，核心 job-to-be-done 为：

> 让 AI coding 系统跨会话、跨任务、跨角色地持续保留项目级认知记忆，而不是每次重新依赖即时上下文拼装。

第一版不把通用多场景全部做全，而是：

- 产品形态上优先服务 `AI Coding`
- 架构抽象上保留通用认知基础设施能力

## 3. 总体架构原则

### 3.1 架构路线选择

本项目采用“双层架构 + 运行时分层”的思路，而不是：

- 直接做一个 AI Coding 垂直产品
- 或一开始就追求过度抽象的纯通用认知框架

推荐路线为：

- `DeepMemory Core`：通用认知内核
- `DeepMemory Domain Packs`：场景适配层
- `DeepMemory Runtime`：生命周期编排层
- `Compatibility Layer`：与 FlashMemory 和其他 substrate 对接
- `Deployment Surface`：本地、sidecar、服务化部署形态

### 3.2 核心架构纪律

必须坚持一条硬规则：

> 场景特化能力只能出现在 `domain pack` / `adapter` 层，不能污染 `core abstraction`。

这条规则决定：

- `DeepMemory` 能否从 AI Coding 走向通用认知基础设施
- `DeepMemory` 能否未来平滑拆仓
- `FlashMemory` 兼容是否会演变成耦合依附

## 4. 系统分层设计

建议采用五层分层：

### Layer 1: DeepMemory Core

定义最稳定、最通用的认知记忆抽象：

- `ObjectRef`
- `Sign`
- `Interpretant`
- `MemoryUnit`
- `Provenance`
- `SocialStatus`
- `Relation`
- `Memory Lifecycle Hooks`

### Layer 2: Runtime

负责运行时调度与记忆生命周期编排：

- ingest routing
- retrieval orchestration
- reflection loop
- consolidation
- promotion / demotion
- repair handling
- session lifecycle
- hook dispatch

### Layer 3: Domain Packs

负责将场景世界翻译为 `Core` 可处理的对象与记忆：

- `coding pack` 作为第一套 pack
- 后续可扩展 `research pack`
- 后续可扩展 `enterprise pack`

### Layer 4: Compatibility / Adapter Layer

负责与外部 substrate 或运行环境进行适配：

- `FlashMemory adapter`
- future code graph adapter
- future research substrate adapter
- future enterprise knowledge adapter

### Layer 5: Deployment Surface

负责部署形态与接入边界：

- embedded mode
- sidecar mode
- service mode
- CLI / HTTP / SDK / MCP 等外部接入界面

## 5. DeepMemory Core 的最小稳定抽象

`Core` 的职责不是理解“代码仓库”，而是定义“什么是可演化的记忆”。

### 5.1 ObjectRef

表示被记忆锚定的对象引用，而不是业务对象本身。  
`Core` 只关心对象被引用与锚定，不关心对象属于哪个领域。

建议最小字段：

- `object_id`
- `object_type`
- `anchor_kind`
- `anchor_locator`

### 5.2 Sign

表示记忆的符号表面，包括自然语言、代码片段、标识符、术语或其他符号形式。

建议最小字段：

- `canonical_sign`
- `sign_variants`
- `sign_modality`

### 5.3 Interpretant

表示某个主体在某个语境下形成的“可行动理解”。  
这是 `DeepMemory` 区别于普通知识库存储的关键抽象。

建议最小字段：

- `actor_ref`
- `intent`
- `operational_claim`
- `applicability_scope`
- `confidence`

### 5.4 MemoryUnit

`MemoryUnit` 是工程命名上的核心原子，与研究稿中的 `SMU` 一一映射。

建议组成：

- `sign`
- `object_ref`
- `interpretant`
- `context`
- `provenance`
- `confidence`
- `social_status`
- `relations`
- `timestamps`

### 5.5 Provenance

`Provenance` 必须是一等公民，不能退化为散乱 metadata。

它支撑：

- 审计
- 回溯
- 争议修复
- 记忆晋升依据
- 长期演化分析

### 5.6 SocialStatus

建议做成明确状态机，而不是自由文本。

初始推荐状态：

- `private_hint`
- `local_hypothesis`
- `contested`
- `provisional_consensus`
- `canonical`

### 5.7 Relation

记忆之间的关系必须显式化。

首批关系类型建议：

- `supports`
- `contests`
- `refines`
- `supersedes`
- `derives_from`
- `aliases`
- `targets`

### 5.8 Memory Lifecycle Hooks

Core 需要暴露生命周期 hook，但不写死场景策略。

建议最小 hook：

- `ingest`
- `retrieve`
- `reflect`
- `consolidate`
- `promote`
- `demote`
- `deprecate`
- `repair`

## 6. 明确禁止进入 Core 的内容

为了防止 `AI Coding` 污染通用内核，需要明确下列内容禁止进入 `Core`：

### 6.1 禁止进入 Core 的领域对象

- `repo`
- `file`
- `commit`
- `branch`
- `PR`
- `issue`
- `workspace`
- `IDE`
- `notebook`
- `paper`
- `slack message`

这些都只能存在于 `domain pack` 层。

### 6.2 禁止进入 Core 的技术绑定

- `FlashMemory` 内部数据结构
- 特定 vector engine
- 特定 embedding provider
- 特定 storage backend
- 特定 transport protocol
- 特定 deployment topology

### 6.3 禁止进入 Core 的产品功能

- UI
- 权限系统
- 多租户系统
- 账单系统
- 团队管理面

## 7. DeepMemory Coding Pack 设计

`Coding Pack` 是 `DeepMemory` 的第一个 `domain pack`，它的职责不是重写 `Core`，而是将 `AI Coding` 世界翻译为 `Core` 能理解的对象和记忆。

### 7.1 Coding Pack 的领域对象

建议识别四类 coding 世界对象：

- `RepoObject`
- `TaskObject`
- `ProcessObject`
- `CollaborationObject`

这些对象进入 Core 前，必须被翻译为：

- `ObjectRef`
- `Sign`
- `Interpretant`
- `MemoryUnit`
- `Relation`

### 7.2 v1 高价值记忆类型

第一版建议聚焦四类 coding memory：

- `Architecture Memory`
- `Task Memory`
- `Repair Memory`
- `Collaboration Memory`

### 7.3 Coding Pack 的职责边界

建议只承担以下五类职责：

- `Domain Ingestion`
- `Domain Interpretation`
- `Domain Retrieval Policy`
- `Domain Promotion Policy`
- `Domain-Specific Evaluators`

### 7.4 核心 anti-corruption rule

必须坚持：

> `Coding Pack` 可以定义 coding ontology，但不能改写 `Core ontology`。

这意味着：

- pack 可以理解 repo、file、diff、review、test failure
- core 不可以因此退化为 repo 专用记忆系统

## 8. 产品形态与运行时策略

### 8.1 v1 推荐产品形态

建议采取：

> `Python SDK first, local runtime included, service form reserved`

原因：

- AI Coding 初期大量场景是本地 agent、CLI、IDE workflow
- 进程内嵌入式认知能力更利于早期验证
- 避免过早引入多租户、权限、服务治理等非核心问题

### 8.2 v1 三种运行形态

从设计上承认三种形态，但只把第一种做全：

1. `Embedded Mode`
2. `Sidecar Mode`
3. `Service Mode`

建议：

- `v1` 主做 `Embedded Mode`
- `v1.5` 预留 `Sidecar Mode`
- `v2+` 再推进 `Service Mode`

### 8.3 v1 暴露的入口

建议首版暴露三类入口：

- `Python SDK`
- `Local CLI`
- `Protocol-ready Runtime Interface`

## 9. Monorepo 中的 DeepMemory 目录结构

建议在当前仓库中的 `deepmemory/` 下按未来独立仓库的标准组织：

```text
deepmemory/
  README.md
  pyproject.toml
  deepmemory/
    core/
    runtime/
    packs/
      coding/
    adapters/
      flashmemory/
    storage/
    interfaces/
    policies/
    telemetry/
    cli/
  tests/
  examples/
  docs/
```

### 9.1 目录职责

- `core/`：通用认知内核
- `runtime/`：生命周期编排
- `packs/coding/`：AI Coding 场景翻译层
- `adapters/flashmemory/`：FlashMemory 适配层
- `storage/`：本地持久化与可替换 backend
- `interfaces/`：稳定 contract
- `policies/`：retrieval / promotion / repair 策略
- `telemetry/`：事件、审计、追踪
- `cli/`：本地运维与调试入口

## 10. FlashMemory Compatibility Layer

这一层的目标不是让 `DeepMemory` 成为 `FlashMemory` 子模块，而是在充分复用 `FlashMemory` 的同时保有独立演化权。

### 10.1 建议消费的能力类型

`DeepMemory` 应该只消费 `FlashMemory` 的稳定能力：

- `Object Resolution`
- `Graph Substrate`
- `Search Substrate`
- `Change Signals`
- `Project Metadata`

### 10.2 禁止消费的内容

`DeepMemory` 不应直接依赖：

- FlashMemory 内部存储实现
- 内部包路径
- 内部数据表结构
- 某个具体向量引擎
- 某个具体 bridge 协议
- 某个具体 CLI 内部细节

### 10.3 推荐适配方式

建议采用三段式：

1. `DeepMemory` 定义 substrate contract
2. `FlashMemory adapter` 实现 contract
3. 未来允许 alternate adapters 存在

这意味着：

> `DeepMemory` 定义自己需要什么，而不是由 `FlashMemory` 决定 `DeepMemory` 能成为什么。

### 10.4 正确理解“无缝兼容”

“无缝兼容 FlashMemory” 的正确含义是：

- 使用 FlashMemory 的团队接入 DeepMemory 的心智成本低
- 现有代码图谱、对象解析、检索能力可以直接复用
- 从用户体验上看像一体
- 从架构边界上看仍然是解耦的

## 11. Monorepo 孵化与未来拆仓策略

`DeepMemory` 从第一天就应该按 `born-separable product` 来孵化。

### 11.1 必须在 `deepmemory/` 内自洽的内容

未来可拆仓所必需的内容都应在 `deepmemory/` 内闭环：

- `pyproject.toml`
- `deepmemory/` 源码
- `tests/`
- `docs/`
- `examples/`
- `README.md`

### 11.2 可以暂时复用 monorepo 的内容

初期允许复用：

- 仓库级 CI
- 统一代码规范工具
- 部分开发脚本
- FlashMemory substrate 能力
- 主仓 release / docs 节奏

但这些复用必须满足：

> 复用可以是便捷性，不可以是生存依赖。

### 11.3 拆仓判据

建议满足以下条件后再拆仓：

- `产品边界稳定`
- `依赖关系可控`
- `用户叙事独立`
- `发布节奏独立`
- `团队关注点独立`

### 11.4 需要提前规避的孵化坑

- 共享 utils 泛滥
- 文档叙事混写
- 发布版本绑定
- adapter 失守为内部强耦合

### 11.5 孵化策略一句话原则

> 同仓开发，异核演化；先共享工程壳，后独立产品壳。

## 12. v1 MVP 范围

建议 `DeepMemory v1` 的最小可交付物仅包含：

1. `DeepMemory Core`
2. `Coding Pack`
3. `FlashMemory Adapter`
4. `Embedded Python SDK`
5. `Local Persistence + Audit`

只要这五项成立，`DeepMemory` 就已经是一个真正可研发、可演示、可迁移的产品雏形。

## 13. 非目标

为避免首版失焦，以下内容明确不属于 `v1` 目标：

- 完整 IDE 产品能力
- 通用 agent orchestration 平台
- 完整多租户后台
- 复杂权限系统
- 成熟团队管理面
- 全场景 pack 一次性覆盖
- 重做 FlashMemory 已经擅长的代码索引能力

## 14. 阶段演进建议

### Phase 1

在 monorepo 内完成：

- `Core`
- `Coding Pack`
- `FlashMemory adapter`
- `Embedded Python SDK`
- 本地持久化

### Phase 2

逐步让 `DeepMemory` 的：

- 文档
- 测试
- 示例
- 版本

开始独立化。

### Phase 3

让外部集成逐步面向 `deepmemory` 包，而不是通过 `flashmemory` 间接进入。

### Phase 4

在 adapter contract 稳定后，再正式拆仓。

## 15. 关键结论

本设计稿的核心判断如下：

1. `DeepMemory` 应作为独立产品在 monorepo 内孵化，而不是作为 FlashMemory 的子功能继续内嵌。
2. `AI Coding` 是第一落地场景，但不是产品本体边界。
3. `DeepMemory Core` 必须保持通用认知抽象，不得被 repo / commit / PR 等对象污染。
4. `Coding Pack` 是 `AI Coding` 世界翻译为通用认知语言的第一套 domain pack。
5. `FlashMemory Compatibility Layer` 的目标是协作复用，而不是架构依附。
6. 当前的 `deepmemory/` 目录应被视为未来独立仓库前身，而非临时占位目录。

## 16. 下一步

在本 spec 获得确认后，下一份文档建议直接进入实施规划层，主题可为：

- `DeepMemory implementation plan v0.1`
- `DeepMemory core schema and contracts`
- `DeepMemory coding pack MVP plan`
- `DeepMemory monorepo bootstrap spec`
