# 认知记忆主轴 02：Semiotic Memory Unit

- 日期：2026-04-22
- 状态：research draft
- 关系：基于 `2026-04-22-superpower-blueprint-design.md` 的记忆主轴分篇

## 1. 问题提出

现有 agent memory 的最小单位通常是：

- chunk
- message
- summary
- embedding item
- key-value memory

这些单位方便实现，却天然丢失两个最重要的层次：

- 这段内容究竟在指向什么对象？
- 当前主体是如何理解这个对象的？

结果是，系统虽然“记住了文本”，却没有真正记住意义。进一步地，当总结链拉长、参与者增多、模态混合时，系统会出现三种常见失败：

- 把同一对象拆成多个不相连碎片
- 把不同对象误当作同一术语的同义项
- 把局部解释误升级为全局事实

因此，认知记忆架构必须重新定义最小记忆单元。

## 2. 核心命题

本文提出：下一代记忆系统的最小单元不应是 chunk，而应是 **semiotic memory unit**，其最小字段为：

```text
memory_unit =
  sign
  + object
  + interpretant
  + context
  + provenance
  + confidence
  + social_status
```

其中：

- `sign`：被表述出来的符号表面，例如函数名、术语、PR 评论、设计图、截图、口头约定。
- `object`：该符号所锚定的对象，例如具体函数、模块、架构决策、故障模式、论文结论。
- `interpretant`：当前主体对 sign-object 关系的理解，可表现为解释、意图、推断、行动建议。
- `context`：此解释成立的任务场景、会话阶段、角色位置、时间窗口。
- `provenance`：记忆来源、生成链、引用历史、修订路径。
- `confidence`：当前解释项的置信度及其校准方式。
- `social_status`：这条记忆在群体中的社会地位。

这里的 `social_status` 是本框架相对于传统 memory system 的关键创新。它建议把记忆区分为：

- `private_cue`：私人线索，只对某个 agent 临时有用
- `local_hypothesis`：局部假设，可行动但未验证
- `contested_interpretation`：存在竞争解释
- `provisional_consensus`：获得阶段性共识
- `institutional_canon`：已升级为团队规范或稳定原则

## 3. 跨学科理论支撑

### 3.1 Peirce：解释项不是注释，而是记忆本体的一部分

Peirce 的 sign-object-interpretant 结构指出，意义不是 sign 与 object 的静态映射，而是通过 interpretant 被不断生成和转译。对 agent systems 而言，这意味着：

- 如果不存 interpretant，系统只能保存表面痕迹。
- 如果只存 sign 与 object，系统仍无法判断“当前为什么要这样理解它”。
- interpretant 本身会成为下一轮推理的新 sign，因此记忆天然具有递归性。

### 3.2 Tulving：情景与语义必须被区分，但又不能断裂

同一对象在不同情景中可能拥有不同解释。Tulving 对 episodic / semantic memory 的区分提示我们：

- `context + provenance` 更接近 episodic trace
- `object + stabilized interpretant` 更接近 semantic abstraction

二者不能合并，也不能完全分离，否则系统要么只剩流水账，要么只剩空洞标签。

### 3.3 Nelson & Narens：置信度必须成为元记忆字段

大多数记忆系统保存“内容”，却不保存“对内容的认识状态”。元记忆研究表明，知道自己知道什么、以及知道自己不确定什么，同样决定系统后续行为。因此 `confidence` 不是评分装饰，而是决定是否升级、是否复核、是否发起辩论的控制字段。

### 3.4 近期 agent memory 研究：现有系统已经逼近，但尚未显式完成

Generative Agents 使用 memory stream、reflection 与 planning 的组合，已经隐含了从 observation 到 higher-level reflection 的链式结构；MemGPT 和 Mem0 则证明分层记忆与长期记忆 consolidation 确实能带来长期一致性收益。但这些系统的最小单位仍偏向自然语言片段或 memory record，尚未把符号表面、对象锚定、解释项和社会地位合并为统一单元。

## 4. 系统映射到 FlashMemory + harness

在 FlashMemory 中，一个 semiotic memory unit 可以这样落地：

```json
{
  "sign": {
    "surface": "auth模块必须避免全局单例client",
    "modality": "discussion_note",
    "aliases": ["HTTP client 不能做全局变量", "auth client singleton anti-pattern"]
  },
  "object": {
    "type": "architecture_constraint",
    "anchors": [
      "internal/auth/client.go",
      "docs/incident/auth-client-retry.md"
    ]
  },
  "interpretant": {
    "summary": "避免共享状态导致重试链路污染和并发读写冲突",
    "action_bias": "favor per-request factory or scoped client pool"
  },
  "context": {
    "task": "auth refactor",
    "time_scope": "2026-Q2",
    "role": "system_designer"
  },
  "provenance": {
    "source": "team review",
    "lineage": ["incident-17", "retro-3", "design-note-12"]
  },
  "confidence": 0.84,
  "social_status": "provisional_consensus"
}
```

FlashMemory 在其中负责：

- 维护 object anchors
- 存储 sign aliases 与 cross-modal references
- 将 interpretant 与对象图谱相连
- 保留 provenance 和时序信息

`harness` 在其中负责：

- 制造对同一对象的竞争 sign
- 让不同 agent 生成不同 interpretant
- 通过任务结果调整 `confidence`
- 根据群体行为更新 `social_status`

## 5. 可验证实验与评测假设

### 假设 A：显式 object anchoring 能减少术语漂移

实验：

- 构造多个 agent 反复总结同一设计约束。
- 一组使用 plain text memory，一组使用 object-anchored memory unit。
- 观察五轮传播后术语是否仍指向同一对象。

指标：

- anchor consistency
- alias explosion rate
- false merge rate

### 假设 B：存储 interpretant 能提升行动正确性

实验：

- 给系统同一条 sign，但省略 interpretant。
- 比较系统在后续 refactor、代码 review、incident triage 中的决策质量。

指标：

- action success rate
- unnecessary clarification turns
- policy misuse frequency

### 假设 C：引入 social_status 能改善共识升级逻辑

实验：

- 对比“所有记忆平权写入”与“带社会状态升级”的记忆系统。
- 观察错误经验是否更容易被及时降级，稳定规范是否更容易被复用。

指标：

- canon precision
- stale memory persistence
- contested memory resolution time

## 6. 对项目演进的直接启示

本文把 FlashMemory 的下一步工作从“做更强检索”转成“做更强记忆单元”。它意味着后续系统设计至少应新增四类字段：

- object anchors
- interpretant representations
- provenance lineage
- social status state machine

如果未来要把认知记忆架构从论文概念推进到工程实现，最先需要被设计清楚的不是更大的模型，而是：

- semiotic memory unit 的 schema
- anchor resolution 机制
- interpretant 的抽取与更新规则
- social_status 的状态转移条件

## 对 FlashMemory 的结构性启示

- 现有代码图谱适合承担 `object` 层的第一版实现。
- 文档解析、多模态 ingest 与知识图谱能力将成为 `sign` 与 `object` 之间的桥梁。
- 若不显式增加 provenance 与 social status，FlashMemory 很难从“知识工具”进化到“集体认知底座”。

## 对 harness 的实验性启示

- `harness` 可以围绕 memory unit 做 controlled perturbation：替换 sign、替换 interpretant、隐藏 provenance、打乱 social status。
- 通过这种 ablation，可以分离不同字段对长期任务表现的贡献。
- 这让记忆研究从 end-to-end anecdote 转向可重复实验。

## 下一篇衔接

下一篇将从个体认知角度展开，回答 semiotic memory unit 如何进入 agent 内部结构：哪些进入工作记忆，哪些沉淀为情景记忆，哪些上升为语义记忆，哪些被元记忆层监控。

参见：[2026-04-22-memory-03-cognitive-layers.md](./2026-04-22-memory-03-cognitive-layers.md)

## 参考文献

- Short, T. L. (2021). [Peirce's Theory of Signs](https://plato.stanford.edu/archives/sum2021/entries/peirce-semiotics/).
- Tulving, E. (1972). [Episodic and Semantic Memory](https://cir.nii.ac.jp/crid/1574231874408386176?lang=en).
- Nelson, T. O., & Narens, L. (1990). [Metamemory: A Theoretical Framework and New Findings](https://doi.org/10.1016/S0079-7421(08)60053-5).
- Park, J. S., et al. (2023). [Generative Agents: Interactive Simulacra of Human Behavior](https://doi.org/10.48550/arXiv.2304.03442).
- Packer, C., et al. (2023). [MemGPT: Towards LLMs as Operating Systems](https://doi.org/10.48550/arXiv.2310.08560).
- Chhikara, P., et al. (2025). [Mem0: Building Production-Ready AI Agents with Scalable Long-Term Memory](https://doi.org/10.48550/arXiv.2504.19413).
