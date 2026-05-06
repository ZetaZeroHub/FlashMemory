# 认知记忆主轴 04：Self-Play Harness

- 日期：2026-04-22
- 状态：research draft
- 关系：基于 `2026-04-22-superpower-blueprint-design.md` 的记忆主轴分篇

## 1. 问题提出

如果记忆系统只在“写入时”被评估，那么它几乎总会看起来不错。真正的问题发生在记忆被行动调用、被不同主体转述、被错误经验污染、被任务压力迫使简化的时候。也就是说：

- 真正的记忆质量，只能在 use-time 暴露。
- 真正的自我校正，只能在失败后比较解释与结果。
- 真正的长期演化，只能在多轮冲突和协作中发生。

这说明，记忆系统需要一个常态化的演化环境，而不仅是离线 benchmark。这个环境就是 `self-play harness`。

## 2. 核心命题

本文主张：`harness` 不应被理解为评测脚手架，而应被设计为 **记忆系统的进化操作系统**。它至少承担四项职责：

- 驱动感知-记忆-推理-行动闭环
- 制造解释冲突和观点竞争
- 对记忆进行奖励、降权、冻结、淘汰
- 将局部成功提炼为可复用的稳定记忆

也就是说，`self-play harness` 的目标不是“测出哪个模型更强”，而是“逼迫记忆系统在复杂任务中暴露缺陷，并通过反思和竞争得到更好的解释结构”。

## 3. 跨学科理论支撑

### 3.1 博弈论：记忆更新本质上是策略选择

在复杂任务里，不同记忆会支持不同策略；不同 interpretant 也会引导不同动作。成功与失败会反过来改变这些记忆的权重。因此，记忆系统不能被理解为只读知识库，而应被视为策略生态的一部分。

### 3.2 Reflexion 与 Self-Refine：语言反馈可以充当轻量强化学习

Reflexion 把 verbal feedback 作为跨 trial 的 episodic memory；Self-Refine 则证明单模型也可以通过自反馈持续改善输出。这两者的共同点是：它们让系统在 **不更新参数** 的前提下，依然能形成稳定改进轨迹。

这启发我们把 `harness` 视作外显的 learning substrate：

- 反馈不必等于 reward scalar
- 反思不必等于长段文字
- 关键是让过去 trial 对未来 decision 有结构化影响

### 3.3 CAMEL 与 Multi-Agent Debate：社会性交互能产生更强纠错机制

多 agent 角色扮演与 debate 的价值，不只是多样性，而是它们迫使系统面对：

- 不同解释项之间的竞争
- 不同目标函数之间的摩擦
- 反驳与辩护中的证据调用

对于记忆演化而言，这种竞争结构正是筛掉脆弱解释、提升稳健共识的发动机。

### 3.4 Generative Agents：反思与规划是长期行为一致性的关键

Generative Agents 将 observation、memory retrieval、reflection、planning 串成闭环，证明长期 believable behavior 需要 memory stream 与 higher-level reflection 的共同支持。它为 `harness` 的行动编排层提供了非常直接的原型。

## 4. 系统映射到 FlashMemory + harness

本文建议的 `self-play harness` 由六个模块组成：

```text
1. Task Generator
2. Role Allocator
3. Memory Arena
4. Debate / Reflection Loop
5. Outcome Judge
6. Consolidation Controller
```

### 4.1 Task Generator

任务来源包括：

- 代码理解
- 设计评审
- incident 复盘
- 多文档综合问答
- 协同规划

目标是生成足够复杂、能诱发记忆冲突的任务，而不是只测单轮问答。

### 4.2 Role Allocator

为不同 agent 分配：

- implementer
- reviewer
- skeptic
- archivist
- planner

不同角色拥有不同 access pattern 和不同记忆偏置，以增加解释竞争的真实度。

### 4.3 Memory Arena

所有 semiotic memory units 在 arena 中具有可观测状态：

- 被谁引用
- 被谁反驳
- 在哪些任务中成功
- 在哪些任务中失效
- 当前 social_status

这让记忆成为可竞争、可比较、可审计的对象。

### 4.4 Debate / Reflection Loop

每个 trial 至少包含一轮：

- 取回记忆
- 形成解释
- 执行动作
- 接收结果
- 产出反思或反驳

必要时多 agent 进行 debate，以形成 contested interpretation 的竞争。

### 4.5 Outcome Judge

裁决结果不应只有 `正确 / 错误`，还应包含：

- 是否引用了正确对象
- 是否误用了旧规范
- 是否产生了新的 alias confusion
- 是否把局部经验错误提升为全局结论

### 4.6 Consolidation Controller

控制器根据结果更新：

- confidence
- provenance lineage
- social_status
- promotion / demotion
- forgetting / quarantine

## 5. 可验证实验与评测假设

### 假设 A：带 debate 的 harness 比单 agent 反思更能减少错误规范固化

实验：

- 一组只允许单 agent reflection。
- 一组允许 reviewer/skeptic 与 implementer 就同一记忆展开 debate。

指标：

- false canonization rate
- contested interpretation resolution quality
- downstream policy accuracy

### 假设 B：角色异质性比模型异质性更重要

实验：

- 同模型多角色
- 多模型同角色
- 多模型多角色

观察谁更能稳定发现错误记忆和脆弱解释。

### 假设 C：显式 consolidation controller 能减少“每轮都像第一次见面”的短视问题

实验：

- 对比无控制器系统与有控制器系统在长程 coding / planning 任务中的表现。

指标：

- repeated failure count
- memory reuse quality
- task continuity score

## 6. 对项目演进的直接启示

对 FlashMemory 生态来说，`self-play harness` 的价值不在于替代模型，而在于提供一个长期稳定的 memory improvement loop。它让后续实现不必把“自进化”理解为持续训练，而可以先从：

- 记忆结构改进
- 决策反馈结构化
- 多角色对抗
- 状态升级与降级

四条路径做起。

## 对 FlashMemory 的结构性启示

- FlashMemory 需要暴露更适合 `arena` 使用的 object-level memory API。
- provenance 与 timeline 将成为 harness 观察记忆演化的主索引。
- 后续若建设 MCP / tool 平台，harness 可以自然成为 tool orchestration 的上层试验场。

## 对 harness 的实验性启示

- 它应该先做成研究基础设施，而不是产品功能。
- 第一阶段优先支持离线重放、role-based debate 和结果审计。
- 第二阶段再进入在线协作与群体共识实验。

## 下一篇衔接

如果说本文解决的是“记忆如何在冲突和反馈中演化”，下一篇要解决的是“这些演化不是发生在真空里，而是发生在群体语义空间中”。也就是说，下一篇会把 Lotman 的 semiosphere 引入多 agent / 人机团队记忆。

参见：[2026-04-22-memory-05-collective-memory-semiosphere.md](./2026-04-22-memory-05-collective-memory-semiosphere.md)

## 参考文献

- Shinn, N., et al. (2023). [Reflexion: Language Agents with Verbal Reinforcement Learning](https://doi.org/10.48550/arXiv.2303.11366).
- Madaan, A., et al. (2023). [Self-Refine: Iterative Refinement with Self-Feedback](https://doi.org/10.48550/arXiv.2303.17651).
- Li, G., et al. (2023). [CAMEL: Communicative Agents for "Mind" Exploration of Large Scale Language Model Society](https://doi.org/10.48550/arXiv.2303.17760).
- Du, Y., et al. (2023). [Improving Factuality and Reasoning in Language Models through Multiagent Debate](https://doi.org/10.48550/arXiv.2305.14325).
- Park, J. S., et al. (2023). [Generative Agents: Interactive Simulacra of Human Behavior](https://doi.org/10.48550/arXiv.2304.03442).
- Packer, C., et al. (2023). [MemGPT: Towards LLMs as Operating Systems](https://doi.org/10.48550/arXiv.2310.08560).
