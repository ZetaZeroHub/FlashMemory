# 认知记忆主轴 03：个体认知分层

- 日期：2026-04-22
- 状态：research draft
- 关系：基于 `2026-04-22-superpower-blueprint-design.md` 的记忆主轴分篇

## 1. 问题提出

当人们说“给 agent 加记忆”时，往往默认这是一层统一缓冲区：把重要内容塞进去，之后再取出来即可。但认知心理学长期表明，记忆并不是单层池，而是多层、异速、异功能的系统。将所有记忆统一放入一个存储层，会引发至少四类问题：

- 工作中的局部线索被长期污染，导致上下文拥堵。
- 一次性经历与稳定知识混在一起，系统难以区分例外与规则。
- 系统没有显式“记忆自己不确定什么”的能力。
- 反思与计划无法被建模为独立层次，只能挤在 prompt 里。

因此，semiotic memory unit 还需要被放进一个分层认知框架中。

## 2. 核心命题

本文主张：agent 的记忆架构至少应包含五层，并通过不同的更新速率和访问权限彼此耦合：

```text
1. 感知缓冲 / Perceptual Buffer
2. 工作记忆 / Working Memory
3. 情景记忆 / Episodic Memory
4. 语义记忆 / Semantic Memory
5. 元记忆 / Metamemory
```

这五层并非简单映射人脑，而是为工程系统提供一套最小而足够的认知分层：

- 感知缓冲处理刚进入系统、尚未稳定解释的符号。
- 工作记忆维持当前任务的可操作解释项。
- 情景记忆保存“发生过什么”的时间化轨迹。
- 语义记忆保存“什么通常为真”的稳定抽象。
- 元记忆监控置信度、冲突度、可回忆性和升级条件。

## 3. 跨学科理论支撑

### 3.1 Atkinson-Shiffrin：多层结构是基础，不是实现细节

Atkinson 与 Shiffrin 的贡献不只是提出三层记忆，更重要的是强调结构特征与控制过程的区分。对 agent architecture 来说，这启发我们把：

- memory stores
- routing policies
- consolidation policies
- forgetting policies

明确拆开，而不是把所有控制逻辑藏在单一 prompt orchestration 里。

### 3.2 Baddeley-Hitch：工作记忆是操作台，不是缓存桶

Working memory 的真正作用不是多放几条信息，而是在有限容量内支撑组合、比对、切换和更新。因此 agent 的 working memory 应偏向：

- 当前任务强相关的 interpretant
- 可立即调用的 object anchors
- 与推理链强耦合的暂存解释

而不应成为长期知识仓库。

### 3.3 Tulving：情景与语义的双向转化是系统核心

情景记忆与语义记忆不是平行孤岛。一次 incident、一次设计 review、一次失败修复，都可能先以 episodic trace 的形式进入系统，再在多次重复后被抽象为 semantic rule。反过来，既有语义规则也会塑造系统如何理解新经历。

对 FlashMemory 而言，这意味着：

- 一条 review comment 不应直接变成全局规则。
- 多次相似 incident 经过归纳后，才有资格升级成 architecture canon。

### 3.4 Nelson-Narens：元记忆是控制层，不是旁白

元记忆研究把系统分成 object-level 与 meta-level，这对 agent 尤其重要。没有元记忆，系统无法系统性地决定：

- 哪些记忆需要复核
- 哪些规则已经过时
- 哪些冲突值得发起 debate
- 哪些知识虽然存在，但当前不可可靠调用

因此，元记忆层应拥有单独字段与单独策略，而非只在日志中隐式存在。

### 3.5 近期 agent 研究：reflection 其实就是原始的元记忆操作

Reflexion 与 Self-Refine 已经证明，语言反馈与自我修正可以在不更新模型参数的情况下持续改善表现。它们之所以重要，不是因为“让模型多说几句”，而是因为它们显式引入了：

- 对先前行为的评价
- 对失败原因的语言化解释
- 对下一轮策略的控制

这些都可以被视为元记忆操作在工程系统中的早期实现。

## 4. 系统映射到 FlashMemory + harness

本文建议的五层映射如下：

### 4.1 感知缓冲

输入来源：

- 代码 diff
- 文档新段落
- 图片/PPT 提取结果
- 用户指令
- agent 间消息

功能：

- 临时缓存原始 sign
- 初步抽取 alias、entity、对象候选
- 尚不写入长期层

### 4.2 工作记忆

保存对象：

- 当前任务相关的 interpretant
- 当前链路下最重要的 5-20 个 memory units
- 正在辩论或等待验证的 contested items

功能：

- 支撑 reasoning
- 支撑 plan generation
- 支撑 action selection

### 4.3 情景记忆

保存对象：

- 事件序列
- 任务执行结果
- agent 对失败和成功的叙述
- 时间戳、角色、环境状态

功能：

- 回答“之前发生过什么”
- 为反思提供素材
- 为未来归纳提供原始样本

### 4.4 语义记忆

保存对象：

- 稳定概念
- 代码结构知识
- 团队规范
- 高复用架构原则

功能：

- 回答“通常应该怎样做”
- 为新事件提供先验结构

### 4.5 元记忆

保存对象：

- confidence calibration
- retrieval reliability
- conflict score
- freshness/decay status
- promotion / demotion recommendations

功能：

- 控制 consolidation
- 控制 forgetting
- 控制 debate trigger
- 控制 canonization threshold

## 5. 可验证实验与评测假设

### 假设 A：五层分离优于“三层外加 prompt 拼装”

实验：

- 对比简化层次系统与显式五层系统。
- 在多轮编程代理任务中测量长期一致性和错误恢复能力。

指标：

- long-horizon task success
- context overflow rate
- false canonization rate

### 假设 B：把 reflection 写入元记忆层，优于把 reflection 只写入情景记忆

实验：

- 一组系统只保留反思文本。
- 一组系统将反思拆解为 confidence、conflict、trigger 等 meta fields。

指标：

- failure recurrence rate
- corrective action latency
- unnecessary re-analysis frequency

### 假设 C：情景到语义的升级阈值，决定系统是否会变成“过拟合团队传说”

实验：

- 改变从 episodic memory 升级为 semantic rule 的阈值。
- 观察规则过早固化和经验难以复用两类失衡。

指标：

- rule usefulness
- stale rule persistence
- incident generalization quality

## 6. 对项目演进的直接启示

要把认知记忆主轴推进为工程设计，FlashMemory 未来至少需要一套五层接口：

- ingest buffer API
- working set assembly API
- episodic timeline API
- semantic canon API
- metamemory control API

这套接口未必要一开始全部实现，但设计上必须显式区分，否则后续任何长期 memory 功能都容易在一个统一存储层里退化。

## 对 FlashMemory 的结构性启示

- 当前的向量检索和图谱能力更适合作为情景与语义之间的桥梁，而不是直接承担全部层次。
- 若未来加入时序图谱，最先受益的是情景记忆层。
- 代码级 object anchors 将成为情景与语义之间的最稳定纽带。

## 对 harness 的实验性启示

- `harness` 可以分别攻击五层，例如只扰动工作记忆、只污染情景层、只延迟元记忆更新。
- 这有助于确定系统失败究竟是 recall 问题、归纳问题，还是 control 问题。
- 分层实验比整体成败更有解释力，也更利于后续实现规划。

## 下一篇衔接

下一篇将进入真正的演化引擎：如果说本文解决的是“记忆如何在单体 agent 内部分层”，那么下一篇会解决“这些记忆如何在任务、冲突和反馈中被筛选、修订和升级”，也就是 `self-play harness` 的设计。

参见：[2026-04-22-memory-04-self-play-harness.md](./2026-04-22-memory-04-self-play-harness.md)

## 参考文献

- Atkinson, R. C., & Shiffrin, R. M. (1968). [Human Memory: A Proposed System and its Control Processes](https://doi.org/10.1016/S0079-7421(08)60422-3).
- Baddeley, A. D., & Hitch, G. J. (1974). Working Memory. In *Psychology of Learning and Motivation*.
- Tulving, E. (1972). [Episodic and Semantic Memory](https://cir.nii.ac.jp/crid/1574231874408386176?lang=en).
- Greenberg, D. L., & Verfaellie, M. (2010). [Interdependence of Episodic and Semantic Memory](https://pmc.ncbi.nlm.nih.gov/articles/PMC2952732/).
- Nelson, T. O., & Narens, L. (1990). [Metamemory: A Theoretical Framework and New Findings](https://doi.org/10.1016/S0079-7421(08)60053-5).
- Shinn, N., et al. (2023). [Reflexion: Language Agents with Verbal Reinforcement Learning](https://doi.org/10.48550/arXiv.2303.11366).
- Madaan, A., et al. (2023). [Self-Refine: Iterative Refinement with Self-Feedback](https://doi.org/10.48550/arXiv.2303.17651).
