# 实现规格 A：Semiotic Memory Unit Schema v0.1

- 日期：2026-04-23
- 状态：implementation spec draft
- 关系：承接 `2026-04-22-memory-02-semiotic-memory-unit.md` 与 `2026-04-22-memory-08-experiments-metrics-roadmap.md`

## 1. 目标与边界

本规格定义 FlashMemory 认知记忆架构的最小可持久化单元：`semiotic memory unit`，简称 `SMU`。本规格的目标不是一次性实现全部 memory system，而是先把后续所有实现都要依赖的“统一对象”写清楚。

本规格解决的问题：

- 一条记忆在系统中究竟长什么样
- 哪些字段是必需字段，哪些是派生字段
- 竞争解释如何表示
- social status 如何先做最小可用建模
- FlashMemory 应如何存、查、组装这些单元

本规格刻意不解决的问题：

- 完整的 `self-play harness trial protocol`
- 完整的 `social_status` 状态转移算法
- 漂移指标的最终计算公式
- UI 或 agent persona 层的交互呈现

## 2. 设计原则

本规格采用以下不可变更的原则：

### 2.1 一个 SMU 表示“一条可行动的解释性主张”

SMU 不是对象本身，也不是原始文本片段。它表示的是：

- 某个符号集
- 针对某个对象集
- 在特定上下文中
- 所形成的一条可行动解释

因此，SMU 的粒度是 **claim-level**，不是 object-level。

### 2.2 竞争解释不共存于同一 SMU 内

对于同一对象的相互冲突解释，不在一个 SMU 内做“多 interpretant 分支”。而是：

- 每条解释各自形成一个独立 SMU
- 通过 `relation = contests` 显式关联

这样做的原因是：

- 更易索引
- 更易审计
- 更易做状态升级与降级
- 更适合后续 debate / harness 计算

### 2.3 未锚定对象的内容不进入持久化 SMU

尚未解析出稳定对象锚点的内容，只能停留在 perceptual buffer，不进入长期持久化 SMU。

原因：

- 本研究的核心是 object-aware memory
- 如果允许未锚定内容直接进入核心记忆层，系统会重新退化为文本仓库

### 2.4 持久化单元采用“主记录 + 派生索引”模型

SMU 的 source of truth 是结构化主记录；向量文本、检索摘要、timeline 片段都属于派生物，不可反向作为权威来源。

## 3. 核心实体模型

### 3.1 `SemioticMemoryUnit`

这是系统中的主记录。

```json
{
  "id": "smu_01HXYZ...",
  "schema_version": "smu.v0.1",
  "canonical_sign": "auth模块必须避免全局单例client",
  "sign_variants": [],
  "object_anchors": [],
  "interpretant": {},
  "context": {},
  "provenance": {},
  "confidence": {},
  "social_status": "local_hypothesis",
  "relations": [],
  "metrics": {},
  "timestamps": {}
}
```

### 3.2 `SignVariant`

用于描述同一主张在不同表面形式中的出现。

```json
{
  "surface": "HTTP client 不能做全局变量",
  "normalized": "http client 不应为全局单例",
  "modality": "discussion_note",
  "language": "zh",
  "source_ref": "review-comment-182",
  "is_canonical": false
}
```

### 3.3 `ObjectAnchor`

用于把 SMU 绑定到真实对象。

```json
{
  "object_type": "code_symbol",
  "object_id": "go://internal/auth/client.go#NewClient",
  "label": "NewClient",
  "role": "primary",
  "confidence": 0.92
}
```

允许的 `object_type` 最小集合：

- `code_symbol`
- `module`
- `document_section`
- `architecture_decision`
- `incident`
- `task`
- `concept`

### 3.4 `Interpretant`

表达当前主张的可行动解释。

```json
{
  "summary": "避免共享状态导致重试链路污染和并发风险",
  "kind": "design_constraint",
  "action_bias": "prefer scoped client or factory pattern",
  "scope": "auth module refactor",
  "validity_window": "2026-Q2"
}
```

允许的 `kind` 最小集合：

- `design_constraint`
- `causal_explanation`
- `incident_lesson`
- `operational_hint`
- `glossary_mapping`
- `task_strategy`
- `working_hypothesis`

### 3.5 `Provenance`

记录这条主张的来源与演化根。

```json
{
  "source_kind": "team_review",
  "source_refs": ["review-182", "incident-17"],
  "created_by": "agent:architect",
  "parent_smu_ids": [],
  "lineage_note": "derived from incident retro and review consensus"
}
```

### 3.6 `Relation`

表示 SMU 与其他 SMU 的语义关系。

```json
{
  "type": "contests",
  "target_smu_id": "smu_01HABC...",
  "reason": "opposite interpretation on same object anchor"
}
```

允许的 `type` 最小集合：

- `supports`
- `contests`
- `refines`
- `supersedes`
- `aliases`
- `derived_from`

## 4. 字段约束与最小必填要求

### 4.1 必填字段

每个持久化 SMU 必须包含：

- `id`
- `schema_version`
- `canonical_sign`
- 至少 `1` 个 `sign_variant`
- 至少 `1` 个 `object_anchor`
- `interpretant.summary`
- `interpretant.kind`
- `context.task_type`
- `provenance.source_kind`
- `provenance.source_refs`
- `confidence.score`
- `social_status`
- `timestamps.created_at`
- `timestamps.updated_at`

附加约束：

- `canonical_sign` 必须在 `sign_variants` 中显式出现且恰好存在 `1` 个 `is_canonical = true` 的条目。
- `sign_variants[].normalized` 必须用于去重比较，因此同一 SMU 内不允许存在两个完全相同的 `normalized` 值。

### 4.2 初始写入约束

新建 SMU 时，仅允许以下初始状态：

- `private_cue`
- `local_hypothesis`
- `contested_interpretation`

以下状态禁止在首次写入时直接创建：

- `provisional_consensus`
- `institutional_canon`

它们只能经由后续控制器升级。

### 4.3 置信度约束

`confidence.score`：

- 范围：`0.0 - 1.0`
- 默认保留两位小数
- 不允许缺失

最小策略：

- `private_cue` 可低至 `0.20`
- `institutional_canon` 不得低于 `0.80`

### 4.4 锚点约束

- 每个 SMU 必须至少有一个 `primary` anchor
- 可以有多个 `secondary` anchors
- 若多个 primary anchors 指向不同 object cluster，则禁止落库，应拆为多个 SMU

### 4.5 关系约束

- `contests` 关系的两端必须至少共享一个 object anchor cluster
- `supersedes` 关系必须指向同类 `interpretant.kind`
- `aliases` 关系不得用于替代 `sign_variants`

## 5. Canonical Schema v0.1

以下为建议的 canonical JSON shape：

注意：该示例展示的是一个**已经过后续验证和升级**的 SMU，因此 `social_status` 为 `provisional_consensus`；它不是“新建时的默认状态”示例。

```json
{
  "id": "smu_01J0ABCDEF123456789",
  "schema_version": "smu.v0.1",
  "canonical_sign": "auth模块必须避免全局单例client",
  "sign_variants": [
    {
      "surface": "auth模块必须避免全局单例client",
      "normalized": "auth 模块避免全局单例 client",
      "modality": "discussion_note",
      "language": "zh",
      "source_ref": "review-182",
      "is_canonical": true
    },
    {
      "surface": "HTTP client 不能做全局变量",
      "normalized": "http client 不应为全局单例",
      "modality": "incident_note",
      "language": "zh",
      "source_ref": "incident-17",
      "is_canonical": false
    }
  ],
  "object_anchors": [
    {
      "object_type": "module",
      "object_id": "module://internal/auth",
      "label": "internal/auth",
      "role": "primary",
      "confidence": 0.94
    },
    {
      "object_type": "code_symbol",
      "object_id": "go://internal/auth/client.go#NewClient",
      "label": "NewClient",
      "role": "secondary",
      "confidence": 0.92
    }
  ],
  "interpretant": {
    "summary": "避免共享状态导致重试链路污染和并发风险",
    "kind": "design_constraint",
    "action_bias": "prefer scoped client or factory pattern",
    "scope": "auth module refactor",
    "validity_window": "2026-Q2"
  },
  "context": {
    "task_type": "architecture_review",
    "task_ref": "task-auth-refactor",
    "role": "architect",
    "environment": "backend-service",
    "time_scope": "2026-Q2"
  },
  "provenance": {
    "source_kind": "team_review",
    "source_refs": ["review-182", "incident-17"],
    "created_by": "agent:architect",
    "parent_smu_ids": [],
    "lineage_note": "derived from incident retro and architecture review"
  },
  "confidence": {
    "score": 0.84,
    "basis": "multi-source agreement",
    "updated_by": "controller:memory_consolidation"
  },
  "social_status": "provisional_consensus",
  "relations": [
    {
      "type": "supports",
      "target_smu_id": "smu_01J0SUPPORT123",
      "reason": "reinforced by incident lesson"
    }
  ],
  "metrics": {
    "reuse_count": 4,
    "success_count": 3,
    "conflict_count": 1,
    "drift_score": 0.11
  },
  "timestamps": {
    "created_at": "2026-04-23T10:30:00+08:00",
    "updated_at": "2026-04-23T14:20:00+08:00",
    "last_validated_at": "2026-04-23T14:20:00+08:00"
  }
}
```

## 6. 存储与索引约定

### 6.1 主记录存储

v0.1 建议把 SMU 主记录存为结构化行记录或 JSON 文档，但无论实现细节如何，必须满足：

- 可按 `id` 直接读取
- 可按 `object_anchor` 反查
- 可按 `social_status` 过滤
- 可按 `timestamps` 做 timeline 查询

### 6.2 派生搜索文档

每个 SMU 派生出一个用于检索的 `search document`：

```text
canonical_sign
+ sign_variants.normalized
+ interpretant.summary
+ object anchor labels
+ context.task_type
```

该派生文档用于 embedding / keyword / hybrid retrieval，但不是权威源。

### 6.3 图关系

至少需要支持以下图边：

- `SMU -> ObjectAnchor`
- `SMU -> SMU (supports / contests / supersedes / refines)`
- `SMU -> ProvenanceSource`

### 6.4 时间序列

每次以下事件发生时，必须写入 timeline event：

- 新建 SMU
- 更新 interpretant
- 变更 confidence
- 变更 social_status
- 新增 relation

## 7. 写入流程

### 7.1 新建流程

1. 接收候选记忆内容
2. 解析 sign variants
3. 解析并确认 object anchors
4. 生成 interpretant
5. 补齐 context 与 provenance
6. 计算初始 confidence
7. 赋予允许的初始 `social_status`
8. 持久化主记录
9. 生成派生搜索文档
10. 写入 timeline event

### 7.2 更新流程

允许更新的字段：

- `sign_variants`
- `interpretant`
- `confidence`
- `social_status`
- `relations`
- `metrics`
- `timestamps`

禁止直接原地覆盖的字段：

- `id`
- `schema_version`
- 原始 `provenance.source_refs`

如果核心解释发生实质变化，必须：

- 更新 `interpretant`
- 增加 timeline event
- 记录 `parent_smu_id` 或 `supersedes / refines` 关系

## 8. 读取与组装接口

本规格为后续实现预留以下逻辑接口：

### 8.1 `GetSMU(id)`

按 `id` 返回完整主记录。

### 8.2 `FindSMUsByObject(object_id, filters)`

按对象读取所有相关 SMU，并支持：

- `social_status`
- `interpretant.kind`
- 时间窗口

### 8.3 `SearchSMUs(query, mode, filters)`

基于派生搜索文档进行检索，再回填完整 SMU。

### 8.4 `AssembleWorkingSet(task_context, object_scope)`

为 working memory 组装候选 SMU 集合，至少包含：

- 高相关 `provisional_consensus`
- 与当前对象直接关联的 `contested_interpretation`
- 最近验证成功的 `local_hypothesis`

## 9. 最小状态语义

本规格只定义状态含义，不定义完整控制算法。

### `private_cue`

- 私人、局部、弱复用
- 可以被单次任务消费
- 不得直接作为团队规则检索默认项

### `local_hypothesis`

- 可行动但尚未稳定
- 可进入 working set
- 需要后续验证或辩论

### `contested_interpretation`

- 明确存在竞争解释
- 默认必须和至少一个 `contests` 关系同时出现

### `provisional_consensus`

- 已获阶段性共识
- 可作为默认建议被检索
- 仍可能被降级

### `institutional_canon`

- 已形成稳定规范
- 进入高优先级语义记忆层
- 仅在后续状态机中允许严格条件升级

## 10. 失败模式与拒绝写入条件

以下情况必须拒绝形成持久化 SMU：

- 没有稳定 object anchor
- 同时锚定多个无关 primary objects
- interpretant 仅重复原 sign，没有新增可行动意义
- provenance 缺失，无法追溯来源
- social_status 试图越级创建为 `institutional_canon`

以下情况允许写入，但必须标记高风险：

- 只有单一来源
- confidence 低于 `0.40`
- 来源之间语义冲突明显
- sign 过于抽象，存在 slogan 化风险

## 11. 验收场景

### 场景 A：代码评审约束

输入：

- review comment
- 相关模块
- 相关函数

期望：

- 成功生成一个 `design_constraint` 类型 SMU
- 至少一个 module anchor
- 初始状态不高于 `local_hypothesis`

### 场景 B：incident lesson 升级

输入：

- incident 记录
- retro note
- 后续成功修复案例

期望：

- 原有 `local_hypothesis` 能升级为 `provisional_consensus`
- timeline 中可见升级事件

### 场景 C：竞争解释并存

输入：

- 同一对象上的两条相反结论

期望：

- 形成两个独立 SMU
- 通过 `contests` 关联
- 不允许塞进同一记录的多 interpretant 分支

### 场景 D：未锚定草稿

输入：

- 含糊的团队口号，没有明确对象

期望：

- 拒绝形成持久化 SMU
- 保留在 perceptual buffer 或候选区，而不是写入长期层

## 12. 后续依赖

本规格冻结后，下一层规格应按以下顺序展开：

1. `Self-Play Harness Trial Protocol v0.1`
2. `Social Status State Machine v0.1`
3. `Drift Audit Metrics and Computation v0.1`

这三者都必须以本规格中的：

- `id`
- `object_anchors`
- `interpretant`
- `social_status`
- `relations`
- `timeline events`

作为统一操作对象。

## 对 FlashMemory 的结构性启示

- 当前 graph/index/embedding 资产可以直接承接 `object_anchors` 与 `search document`。
- 若不先固定 SMU schema，后续多模态 ingest、memory recall 与 harness 都会缺少同一语义底座。
- v0.1 最重要的不是字段多，而是边界清晰：什么能入长期层，什么不能。

## 对 harness 的实验性启示

- `harness` 后续要操作的是 claim-level unit，而不是裸文本片段。
- `contests`、`supports`、`supersedes` 这类关系，是后续 trial protocol 的直接输入。
- 若 schema 清晰，harness 的 debate、promotion、demotion 才有统一对象。

## 下一篇衔接

下一篇实现规格应进入 `Self-Play Harness Trial Protocol v0.1`，明确一个 trial 怎样组织角色、怎样组装 working set、怎样记录 outcome、怎样把结果回写到 SMU。

## 参考文献

- [2026-04-22-memory-02-semiotic-memory-unit.md](./2026-04-22-memory-02-semiotic-memory-unit.md)
- [2026-04-22-memory-03-cognitive-layers.md](./2026-04-22-memory-03-cognitive-layers.md)
- [2026-04-22-memory-04-self-play-harness.md](./2026-04-22-memory-04-self-play-harness.md)
- [2026-04-22-memory-07-flashmemory-as-substrate.md](./2026-04-22-memory-07-flashmemory-as-substrate.md)
- [2026-04-22-memory-08-experiments-metrics-roadmap.md](./2026-04-22-memory-08-experiments-metrics-roadmap.md)
