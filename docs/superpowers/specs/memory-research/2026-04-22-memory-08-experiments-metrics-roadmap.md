# 认知记忆主轴 08：实验设计、评测指标与研究路线图

- 日期：2026-04-22
- 状态：research draft
- 关系：基于 `2026-04-22-superpower-blueprint-design.md` 的记忆主轴分篇

## 1. 问题提出

如果前七篇只停留在理论和架构提案，这组文章就仍然只是富有想象力的方向稿。真正使其成为研究包的关键，在于把它压成一组可执行问题：

- 先验证什么？
- 用什么指标验证？
- 哪些失败意味着理论有问题，哪些失败只是实现太弱？
- 哪些原型可以在 FlashMemory 当前能力上最先启动？

因此，本篇把前述框架收束为实验矩阵与阶段化路线图。

## 2. 核心命题

本文主张：这套认知记忆架构的推进方式不应是“一次性做出完美 agent”，而应是沿着三条研究主线递进：

- **记忆单元验证**：证明 semiotic memory unit 优于 plain chunk memory
- **演化机制验证**：证明 harness 中的自博弈、自反思和社会传播机制确实改善长期表现
- **制度化验证**：证明系统不仅能记住，还能形成更稳定的团队级公共记忆

对应地，研究路线应按 `unit -> layer -> arena -> society -> canon` 五个层次推进。

## 3. 跨学科理论支撑

### 3.1 认知心理学提供分层假设

Atkinson-Shiffrin、Baddeley-Hitch、Tulving、Nelson-Narens 共同给出一个可验证前提：不同功能层应对应不同记忆表征、不同更新速率和不同控制逻辑。

### 3.2 符号学提供结构假设

Peirce 和 Lotman 提供的不是文学修辞，而是可被工程化的结构主张：

- 记忆单元必须显式区分 sign / object / interpretant
- 群体记忆必须显式建模语义边界与翻译过程

### 3.3 Agent 研究提供机制假设

Reflexion、Self-Refine、CAMEL、Generative Agents、MemGPT、Mem0 共同说明：

- 长期表现提升可以在 test-time / system-time 发生
- 反思、角色分化和分层记忆确实是有效机制
- 但它们尚未被统一到“符号秩序演化”的单一框架中

因此，本研究的增量并不在于再发明单个技巧，而在于把这些技巧整合成一个可解释、可实验、可迁移的理论-工程闭环。

## 4. 系统映射到 FlashMemory + harness

本文建议构造一个四象限实验矩阵：

```text
Axis A: Memory Unit
- Plain chunk
- Object-anchored chunk
- Semiotic memory unit

Axis B: Memory Layers
- Flat memory
- Layered memory

Axis C: Evolution Mechanism
- No reflection
- Single-agent reflection
- Multi-agent debate
- Full self-play harness

Axis D: Social Structure
- Single agent
- Human-agent pair
- Multi-agent cluster
- Multi-region semiosphere
```

在 FlashMemory 上的实现建议分三期：

### Phase 1：最小研究原型

- object anchors
- provenance fields
- semiotic memory unit 的最小 schema
- basic harness replay

### Phase 2：演化与冲突

- reflection / debate loops
- confidence 更新
- contested interpretation 管理
- drift audit

### Phase 3：群体记忆与制度化

- region-aware memory spaces
- social_status 状态机
- canonization pipeline
- semiosphere-level experiments

## 5. 可验证实验与评测假设

以下指标构成整套研究的核心 metric family：

### 5.1 Semiotic Stability

定义：

- 同一 memory unit 在跨轮次、跨主体、跨模态传播后，是否仍保持 sign-object-interpretant 的可操作一致性。

可测代理指标：

- anchor retention
- interpretant variance
- alias divergence

### 5.2 Interpretive Divergence

定义：

- 多个主体面对同一对象时，其解释项之间的分歧程度。

意义：

- 分歧不是坏事，但不可见的高分歧会导致系统性协作失败。

### 5.3 Symbol Grounding Strength

定义：

- 一条记忆中的符号是否稳定锚定到可验证对象，而不是停留在 slogan 层。

代理指标：

- object resolution success
- actionable retrieval rate
- false abstraction rate

### 5.4 Canon Quality

定义：

- 被升级为团队规范的记忆，是否真的高复用、高稳定、低误伤。

代理指标：

- canon precision
- stale canon persistence
- downstream policy utility

### 5.5 Repair Efficiency

定义：

- 一旦记忆漂移或错误制度化，系统恢复到健康状态的速度和代价。

代理指标：

- correction latency
- conflict resolution rounds
- repeated failure suppression

## 6. 对项目演进的直接启示

这份路线图要求 FlashMemory 后续规划遵循一个原则：**先让研究问题可测，再让产品功能可见。** 更具体地说：

- 不要先承诺“永远记住一切”，而要先回答“哪种记忆最值得被长期保留”。
- 不要先把所有精力投到 UI/agent persona，而要先把 object anchoring、timeline、provenance 与 drift audit 做成可靠底层。
- 不要把 benchmark 只设为一次性问答正确率，而要引入长期、群体、传播和制度化维度。

## 对 FlashMemory 的结构性启示

- 原蓝图中的认知记忆架构，可以在本路线图下拆为一条更长的研究链。
- 原蓝图中的图谱、向量检索、多模态 ingest 和路由，都能在这里找到更清晰的实验位置。
- 这让项目既保留工程实现感，又拥有足够强的研究叙事。

## 对 harness 的实验性启示

- `harness` 首先是研究基础设施，其次才是产品能力。
- 它需要服务于 controlled experiments、ablation、回放和解释，而不只是“自动跑任务”。
- 最后形成的不是一个单 benchmark，而是一套持续演化的 memory research harness。

## 下一篇衔接

本篇是本组文档的收束篇。下一步如果要继续推进，不应直接跳到全面实现，而应从写更细的 implementation spec 开始，优先锁定：

- semiotic memory unit schema
- harness trial protocol
- social_status 状态机
- drift audit 计算方法

## 参考文献

- Atkinson, R. C., & Shiffrin, R. M. (1968). [Human Memory: A Proposed System and its Control Processes](https://doi.org/10.1016/S0079-7421(08)60422-3).
- Baddeley, A. D., & Hitch, G. J. (1974). Working Memory. In *Psychology of Learning and Motivation*.
- Tulving, E. (1972). [Episodic and Semantic Memory](https://cir.nii.ac.jp/crid/1574231874408386176?lang=en).
- Nelson, T. O., & Narens, L. (1990). [Metamemory: A Theoretical Framework and New Findings](https://doi.org/10.1016/S0079-7421(08)60053-5).
- Short, T. L. (2021). [Peirce's Theory of Signs](https://plato.stanford.edu/archives/sum2021/entries/peirce-semiotics/).
- Noth, W. (2015). [The Topography of Yuri Lotman's Semiosphere](https://doi.org/10.1177/1367877914528114).
- Centola, D. (2010). [The Spread of Behavior in an Online Social Network Experiment](https://doi.org/10.1126/science.1185231).
- Hamilton, W. L., Leskovec, J., & Jurafsky, D. (2016). [Diachronic Word Embeddings Reveal Statistical Laws of Semantic Change](https://aclanthology.org/P16-1141/).
- Park, J. S., et al. (2023). [Generative Agents: Interactive Simulacra of Human Behavior](https://doi.org/10.48550/arXiv.2304.03442).
- Shinn, N., et al. (2023). [Reflexion: Language Agents with Verbal Reinforcement Learning](https://doi.org/10.48550/arXiv.2303.11366).
- Madaan, A., et al. (2023). [Self-Refine: Iterative Refinement with Self-Feedback](https://doi.org/10.48550/arXiv.2303.17651).
- Li, G., et al. (2023). [CAMEL: Communicative Agents for "Mind" Exploration of Large Scale Language Model Society](https://doi.org/10.48550/arXiv.2303.17760).
- Du, Y., et al. (2023). [Improving Factuality and Reasoning in Language Models through Multiagent Debate](https://doi.org/10.48550/arXiv.2305.14325).
- Packer, C., et al. (2023). [MemGPT: Towards LLMs as Operating Systems](https://doi.org/10.48550/arXiv.2310.08560).
- Chhikara, P., et al. (2025). [Mem0: Building Production-Ready AI Agents with Scalable Long-Term Memory](https://doi.org/10.48550/arXiv.2504.19413).
