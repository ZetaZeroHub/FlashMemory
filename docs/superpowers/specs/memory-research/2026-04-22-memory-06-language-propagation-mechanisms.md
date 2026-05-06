# 认知记忆主轴 06：语言传播、压缩与漂移机制

- 日期：2026-04-22
- 状态：research draft
- 关系：基于 `2026-04-22-superpower-blueprint-design.md` 的记忆主轴分篇

## 1. 问题提出

记忆系统并不是在真空中运作。几乎所有长期记忆都要经过语言表述、总结、复述、压缩、转译和再传播。也正是在这些过程中，系统会遇到最常见的失败：

- 总结后丢失限定条件
- 术语在复述中获得新的默认含义
- 不同 agent 使用不同 alias 指向相同对象
- 由经验陈述压缩成绝对规则

换言之，长期记忆的最大敌人不一定是忘记，而是 **传播中的语义失真**。

## 2. 核心命题

本文主张：一个自进化记忆系统必须把语言传播视为一条一等公民的记忆管线，而不是外围现象。系统至少要显式处理四类传播机制：

- 扩散 diffusion
- 压缩 compression
- 漂移 drift
- 稳定 stabilization

如果没有这四类机制的显式建模，任何长期记忆层都将不可避免地积累：

- sloganized memory
- stale summaries
- alias fragmentation
- false consensus

## 3. 跨学科理论支撑

### 3.1 社会传播学：重复暴露与冗余传播会改变采纳概率

Centola 的研究表明，行为采纳往往需要社会强化，而非单次接触。对于团队知识而言，真正升级为规范的记忆通常不是“一次看到”，而是：

- 在多处被一致提及
- 由多个可信角色重复表述
- 在不同任务中被反复证成

这说明“共识记忆”形成更接近强化扩散，而不是信息广播。

### 3.2 计算语言学：意义变化可以被向量化和时序化

Hamilton 等人对 diachronic semantic change 的研究说明，词义漂移可通过分布式表示跨时间比较。对于 agent memory：

- 同一术语跨周、跨会话、跨 agent 的 embedding 变化可作为 drift signal
- 同一对象对应的 sign 集合增长速度可作为 alias explosion signal
- 总结前后与原对象锚点的一致性可作为 compression loss signal

### 3.3 符号学：翻译既生成意义，也制造失真

Peirce 视 interpretant 为 translation/development，Lotman 则将边界翻译视为文化生成机制。这共同说明：传播不是把原文搬运到另一个位置，而是把符号改写成另一种语法。于是：

- 记忆强化与记忆失真来自同一过程
- 传播效率与语义精度天然存在张力

### 3.4 LLM/agent 文献：summary chains 天然会带来 drift

无论是 multi-agent debate、reflection 还是 planning，系统都会不断生成中间解释文本。这些文本既是记忆来源，也是记忆污染源。若系统缺少 drift audit，就会把“越来越流畅的说法”误当作“越来越可靠的知识”。

## 4. 系统映射到 FlashMemory + harness

本文建议在 `FlashMemory + harness` 上引入一条独立的语言传播监测链：

```text
Source Memory Unit
    ↓
Retelling / Summary / Translation
    ↓
Propagation Tracker
    ├── diffusion count
    ├── compression loss
    ├── semantic drift
    ├── alias growth
    └── anchor retention
    ↓
Status Update
```

### 4.1 FlashMemory 负责的部分

- 存储原始 sign 与 object anchors
- 追踪 alias 演化
- 比较不同时间点的解释项与对象一致性
- 为每条 memory unit 维护 propagation history

### 4.2 harness 负责的部分

- 人工生成 summary chain
- 构造跨角色复述
- 构造跨模态转译，例如代码说明转 incident note、incident note 转架构规则
- 用长期任务结果反向验证哪些传播保持了可操作意义

## 5. 可验证实验与评测假设

### 假设 A：低 compression loss 的记忆，比高 lexical overlap 的记忆更有价值

实验：

- 比较两个 summary system。
- 一个更忠实于对象锚定，一个更忠实于原表面词汇。

指标：

- anchor retention
- downstream action accuracy
- hallucinated constraint rate

### 假设 B：对 drift 进行早期监控，能减少错误规范传播

实验：

- 一组系统只在失败后回查。
- 一组系统在每次传播后就做 drift audit。

指标：

- stale canon emergence
- repair latency
- false agreement rate

### 假设 C：多角色复述比单角色总结更容易暴露 contested meanings

实验：

- 一组使用单 agent summary chain。
- 一组使用多 agent retelling chain。

指标：

- ambiguity surfacing rate
- contested memory detection
- canon robustness

## 6. 对项目演进的直接启示

如果未来 FlashMemory 要承载长期团队认知，它不能把 summary 当作 cheap cache，而应把 summary 当作高风险传播事件。工程上需要逐步加入：

- propagation lineage
- drift scoring
- anchor retention scoring
- alias explosion detection

这些功能表面上像分析指标，实质上是在为长期记忆系统提供“语义健康监测”。

## 对 FlashMemory 的结构性启示

- 现有 embedding 与索引能力可以直接服务 drift detection。
- 多模态 ingest 若不加入 propagation 视角，很容易沦为资料堆栈。
- 后续 timeline 设计应支持查看一条记忆经由哪些复述链演化。

## 对 harness 的实验性启示

- `harness` 需要把“传播任务”单独作为任务类别。
- 应支持 controlled retelling、controlled summarization 和 controlled translation。
- 最重要的不是看文本是否更短，而是看对象是否仍被正确调用。

## 下一篇衔接

下一篇会把前面这些理论重新接地，明确 FlashMemory 本身在这套架构里究竟扮演什么角色：哪些已有能力可以直接复用，哪些新增能力是实现 semiotic memory 与 harness 的关键依赖。

参见：[2026-04-22-memory-07-flashmemory-as-substrate.md](./2026-04-22-memory-07-flashmemory-as-substrate.md)

## 参考文献

- Centola, D. (2010). [The Spread of Behavior in an Online Social Network Experiment](https://doi.org/10.1126/science.1185231).
- Hamilton, W. L., Leskovec, J., & Jurafsky, D. (2016). [Diachronic Word Embeddings Reveal Statistical Laws of Semantic Change](https://aclanthology.org/P16-1141/).
- Short, T. L. (2021). [Peirce's Theory of Signs](https://plato.stanford.edu/archives/sum2021/entries/peirce-semiotics/).
- Noth, W. (2015). [The Topography of Yuri Lotman's Semiosphere](https://doi.org/10.1177/1367877914528114).
- Shinn, N., et al. (2023). [Reflexion: Language Agents with Verbal Reinforcement Learning](https://doi.org/10.48550/arXiv.2303.11366).
- Du, Y., et al. (2023). [Improving Factuality and Reasoning in Language Models through Multiagent Debate](https://doi.org/10.48550/arXiv.2305.14325).
