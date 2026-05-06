# FlashMemory Superpower Blueprint
## 从代码解析引擎 → 多模态认知知识引擎

**版本**: v0.1-draft  
**日期**: 2026-04-22  
**状态**: 设计探索阶段（待用户确认优先级后进入实现规划）

---

## 一、全局愿景

FlashMemory 的核心差异化能力：

> **AST语法树 × 自注意力评分 × 大小模型协同推理**

在此基础上，本蓝图探索将 FlashMemory 扩展为：

> **任意知识形态（代码 + 文档 + 图片 + PPT）的端侧认知引擎**
>
> 具备：智能意图路由 → 多模态解析 → 分层记忆存储 → 动态工具调度

---

## 子系统 1：多模态文档解析器

### 问题陈述

当前 FlashMemory 只能解析代码文件。用户本地存放大量 PDF 技术报告、PPT 演讲、图片截图，这些"非结构化知识"无法被纳入知识图谱。

### 开源方案对比

| 方案 | Stars | 核心优势 | 短板 | License |
|------|-------|---------|------|---------|
| **Docling** (IBM/LF AI) | 58k | 最全格式支持；内置 MCP Server；TableFormer精准表格提取；DoclingDocument统一数据模型 | 模型较重，GPU加速效果更好 | MIT |
| **marker-pdf** (Datalab) | 34.2k | 速度极快（25页/秒 H100）；支持 `--use_llm` 混合模式；输出 Markdown/JSON/HTML/chunks | GPL-3.0（商业需授权）；PPTX支持较弱 | GPL-3.0 |
| **MegaParse** (QuivrHQ) | ~3k | 专为 RAG 设计；零信息丢失策略；与 Quivr 深度集成 | 生态较小，依赖云端VLM | Apache-2.0 |

### 推荐方案

**双引擎策略**：
- **主引擎**: `Docling` — 格式覆盖最全（PDF/PPTX/XLSX/图片/LaTeX），MCP原生支持，MIT许可无商业风险
- **快速引擎**: `marker-pdf` — 用户只需要快速提取纯文本时的备选，速度提升10x

### FlashMemory 集成设计

```
用户: fm ingest ~/papers/llm_survey.pdf
         ↓
[DocParser Layer]
  Docling → DoclingDocument (JSON)
         ↓
[Chunker]
  按段落/表格/标题层级智能分块
         ↓
[Embedding Pipeline]
  现有 EmbeddingProvider (internal/utils/embedding.go)
         ↓
[Zvec Index]
  文档块与代码块统一索引
         ↓
[Knowledge Graph]
  文档节点 <→ 代码节点 双向关联
```

**新增 CLI 命令**：

```bash
fm ingest <file|dir>          # 解析并索引文档
fm ingest --watch ~/papers/   # 监听目录，自动增量索引
fm search "transformer注意力机制原理"  # 跨代码+文档统一检索
```

**FlashMemory 专属亮点**：文档解析后不是简单向量化——而是提取文档中的**概念节点**（算法、架构、人名、论文引用），与代码知识图谱的**函数节点**建立语义关联。例如：PDF里的"Self-Attention"概念 → 自动关联代码库中的 `attention.py::SelfAttentionLayer`。

---

## 子系统 2：意图路由引擎

### 问题陈述

当用户发出一个查询，系统需要在 10ms 内决定：这是代码搜索？文档问答？还是知识推理？不同管线的成本和延迟差异巨大，用 LLM 做路由判断本身就会引入 2-5 秒的延迟。

### 开源方案对比

| 方案 | Stars | 核心机制 | 延迟 | 特点 |
|------|-------|---------|------|------|
| **Semantic Router** (Aurelio) | 3.4k | 向量空间相似度匹配 utterances | **~100ms** | 确定性路由，支持本地/云端 encoder，多模态 |
| **FlowMind / DSPy Router** | N/A | 基于 DSPy 优化的 LLM 路由 | ~1-3s | 精度更高，速度慢 |
| **自研规则引擎** | — | 关键词 + 向量混合 | ~10ms | 最快，但覆盖率低 |

### 推荐方案

**Semantic Router** — 100ms 延迟，无需 LLM 调用，与现有 EmbeddingProvider 无缝对接。

### FlashMemory 路由设计

```
用户查询
    ↓
[Semantic Router]  ~100ms
    ├── code_search      → Zvec 代码搜索管线
    ├── doc_qa           → Docling 文档问答管线
    ├── knowledge_graph  → 图谱遍历推理
    ├── llm_analyze      → 触发 LLM 深度分析
    └── fallback         → 混合检索兜底
```

**Route 定义示例**（Python SDK 侧）：

```python
routes = [
    Route(name="code_search", utterances=[
        "找到函数实现", "这个API怎么用", "show me the code for",
        "哪里定义了", "搜索类", "find implementation"
    ]),
    Route(name="doc_qa", utterances=[
        "PDF里说了什么", "这篇论文的结论", "PPT第几页",
        "文档中关于...的解释", "according to the paper"
    ]),
    Route(name="deep_analysis", utterances=[
        "分析这段代码的复杂度", "解释整个模块的架构",
        "为什么这样设计", "优化建议"
    ]),
]
```

**FlashMemory 专属亮点**：路由本身也能学习。当用户对某条路由结果点踩（或切换管线），路由器的 utterance 向量空间会通过 RLHF-lite 方式自动调整，越用越准。

---

## 子系统 3：LLM 智能路由器

### 问题陈述

不同复杂度的查询需要不同规模的模型：
- "这个变量叫什么名字？" → 本地 Qwen2.5-3B 完全够用，0延迟
- "分析整个项目的架构瓶颈并给出重构方案" → 需要 Claude/GPT-5

当前所有请求都走同一个模型端点，造成大量算力浪费或质量不足。

### 开源方案对比

| 方案 | 机构 | 路由策略 | 节省成本 | 集成难度 |
|------|------|---------|---------|---------|
| **RouteLLM** (lm-sys/LMSYS) | UC Berkeley | Matrix Factorization + BERT分类器，基于人类偏好数据训练 | 85%+ | 中（需训练数据） |
| **Semantic Router 多模型** | Aurelio | 向量相似度决策树 | ~60% | 低 |
| **自研置信度评分** | — | AST复杂度 + 查询长度 + 实体密度 → 模型选择 | ~50% | 低（最契合现有架构） |

### 推荐方案（FlashMemory 定制版）

利用 **现有的 FunctionRanker 评分系统** 做天然的复杂度信号：

```
查询复杂度信号 = f(
    query_embedding_entropy,    // 查询语义复杂度
    target_node_rank_score,     // 目标代码节点的重要性评分(Alpha/Beta/Gamma/Delta)
    context_window_size,        // 所需上下文大小
    is_cross_module             // 是否跨模块查询
)

if complexity_score < 0.3:
    → 本地小模型 (Ollama Qwen2.5-3B)   // 0 latency, 0 cost
elif complexity_score < 0.7:
    → 中等模型 (GPT-4o-mini / Gemini Flash)
else:
    → 旗舰模型 (Claude / GPT-5 / Gemini Pro)
```

**与现有架构的连接点**：
- `internal/utils/ollama_call.go` — 本地模型调用（已有）
- `config/config.go` — LLM 配置中心（已有）
- 新增 `internal/router/llm_router.go` — 路由决策引擎

**FlashMemory 专属亮点**：路由决策不仅基于查询语义，还基于**AST节点的 Gamma/Delta 复杂度评分**。被深度依赖的核心函数（高 Fan-In）查询自动路由到更强模型；叶子节点的简单查询路由到本地模型。这是其他通用路由框架完全没有的能力。

---

## 子系统 4：认知记忆架构

### 问题陈述

每次会话结束，上下文全部丢失。用户昨天分析过的架构决策、已知的技术债、团队共识——在下一次会话时需要重新解释。这不是"搜索工具"，而是"有记忆的知识伙伴"。

### 开源方案对比

| 方案 | Stars | 架构特色 | 与 FlashMemory 契合度 |
|------|-------|---------|---------------------|
| **Mem0** (mem0ai) | 53.8k | 三层记忆（User/Session/Agent）；LLM提取结构化记忆；BM25+语义混合检索；2026新算法+20分 | 高 — Python SDK，可直接替换现有会话上下文管理 |
| **Graphiti** (Zep AI) | ~5k | 时序知识图谱；事实有效期窗口；冲突自动解决；MCP Server支持 | 极高 — 时序感知与现有 KnowledgeGraph 架构天然互补 |
| **Letta/MemGPT** (letta-ai) | 22.2k | 虚拟内存分页（主存/外存）；Agent自主管理记忆；skills/subagents | 中 — 功能强大但较重，适合作为长期 Agent 平台参考 |

### 推荐方案

**分层混合架构**，灵感来自 Freud 三层意识模型（本我/自我/超我 → 即时缓冲/工作记忆/长期记忆）：

```
┌─────────────────────────────────────────────────────┐
│  Layer 3: 长期记忆 (Graphiti 时序知识图谱)             │
│  - 项目架构决策、技术债记录、团队约定                  │
│  - 时序感知：知道"上周的结论"已被"昨天的决策"覆盖      │
├─────────────────────────────────────────────────────┤
│  Layer 2: 工作记忆 (Mem0 会话级)                     │
│  - 当前任务上下文、本次会话的用户偏好                  │
│  - 跨会话持久化：下次打开自动恢复                      │
├─────────────────────────────────────────────────────┤
│  Layer 1: 即时缓冲 (现有 SQLite + Zvec)              │
│  - 当前代码分析结果、实时搜索缓存                      │
│  - 现有架构，无需改动                                 │
└─────────────────────────────────────────────────────┘
```

**新增能力示例**：

```bash
fm remember "这个项目的 HTTP 客户端不能用全局变量，已经踩坑两次"
fm recall "之前有什么关于并发安全的讨论？"
fm timeline "auth模块的设计演变历史"   # Graphiti 时序查询
```

**FlashMemory 专属亮点**：记忆不只是文本片段——而是**代码感知的结构化记忆**。"这个函数有性能问题"会自动关联到 `internal/ranking/ranking.go::CalculateScore`，下次查询该函数时，历史记忆自动注入上下文。

---

## 子系统 5：Skills/Tool 工具路由平台

### 问题陈述

FlashMemory 目前是单一功能工具。随着能力扩展，需要一个可扩展的工具注册和动态调度机制，让用户和社区能够贡献和组合能力。

### 开源方案对比

| 方案 | 描述 | 契合度 |
|------|------|-------|
| **MCP Protocol** (Anthropic) | 标准化工具接口；FlashMemory 已支持 MCP Client；可暴露为 MCP Server | 极高 — 现成基础设施 |
| **OpenAI Function Calling 格式** | JSON Schema 定义工具；广泛兼容 | 高 — 可作为内部工具描述格式 |
| **Haystack Pipelines** | 声明式 DAG 管线；工具组合 | 中 — 过重，参考其设计思想即可 |

### FlashMemory Tool 平台设计

```
pip-package/flashmemory/tools/
├── builtin/
│   ├── code_search.py       # 现有：代码搜索
│   ├── doc_ingest.py        # 新增：文档解析入库
│   ├── memory_recall.py     # 新增：记忆检索
│   └── llm_analyze.py       # 现有：深度分析
├── community/               # 用户贡献工具目录
│   └── *.tool.yaml          # 声明式工具定义
└── registry.py              # 工具注册中心
```

**工具声明格式**（`*.tool.yaml`）：

```yaml
name: arxiv_search
description: Search and ingest papers from ArXiv into FlashMemory
trigger_utterances:
  - "搜索最新的transformer论文"
  - "find papers about RAG"
input_schema:
  query: string
  max_results: integer
output: knowledge_chunks
```

**FlashMemory 专属亮点**：工具路由由**子系统2的意图路由引擎**驱动，100ms 内完成工具选择。工具输出统一经过**子系统1的文档解析管线**标准化后入库，形成闭环。

---

## 整体架构数据流

```
用户输入
    ↓
[子系统2: 意图路由] ~100ms (Semantic Router)
    ├── 工具路由 → [子系统5: Tool Platform]
    │                   ├── 代码搜索 → Zvec (现有)
    │                   ├── 文档解析 → [子系统1: Docling/Marker]
    │                   └── 记忆检索 → [子系统4: Mem0/Graphiti]
    │
    └── LLM 路由 → [子系统3: LLM Router]
                        ├── 本地模型 (Ollama, 现有)
                        ├── 中等云模型
                        └── 旗舰模型
                             ↓
                    结果 → [子系统4: 记忆写入]
                             ↓
                         用户响应
```

---

## 优先级与实现路线图

### Phase 1 — Quick Win（2-4周）

| 子系统 | 任务 | 依赖 |
|--------|------|------|
| **1** | 集成 Docling，新增 `fm ingest` 命令 | `pip install docling` |
| **2** | 集成 Semantic Router，替换 hardcode 查询分发 | `pip install semantic-router` |

### Phase 2 — 核心差异化（4-8周）

| 子系统 | 任务 | 依赖 |
|--------|------|------|
| **3** | 基于 AST 复杂度的 LLM 路由器 | 新增 `internal/router/llm_router.go` |
| **4** | Mem0 工作记忆层集成 | `pip install mem0ai` |

### Phase 3 — 战略布局（2-3月）

| 子系统 | 任务 | 依赖 |
|--------|------|------|
| **4** | Graphiti 时序知识图谱（扩展现有 KnowledgeGraph） | `pip install graphiti-core` |
| **5** | Community Tool 平台 + `.tool.yaml` 规范 | MCP Server 扩展 |

---

## 技术依赖摘要

```toml
# pip-package/pyproject.toml 新增依赖
[project.optional-dependencies]
multimodal = ["docling", "marker-pdf"]
routing   = ["semantic-router"]
memory    = ["mem0ai"]
memory-full = ["mem0ai", "graphiti-core", "neo4j"]
```

```
# Go 新增组件
internal/router/llm_router.go    # LLM 路由决策
internal/ingest/doc_parser.go    # 文档解析 Go 桥接层
internal/memory/session.go       # 会话记忆管理
```

---

> **注意**: 本文档是设计探索阶段产物，所有子系统均需独立 spec → 实现规划 → 开发 的完整周期。
> **下一步**: 确认 Phase 1 两个子系统的详细设计后，invoke writing-plans 技能。
