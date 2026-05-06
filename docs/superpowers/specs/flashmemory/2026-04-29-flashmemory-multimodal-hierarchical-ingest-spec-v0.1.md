# FlashMemory 多模态层级化解析与依赖管理 Spec v0.1

- 日期：2026-04-29
- 状态：implementation spec draft
- 关系：补充 `2026-04-23-flashmemory-total-system-spec.md` 中 5.1 多模态文档解析模块

## 0. 推进状态（持续更新）

### 已完成

- [x] `doc_nodes/doc_edges/parse_artifacts` 表结构与索引已落地
- [x] `EnsureIndexDB` 已接入文档 schema 自动初始化
- [x] BuildIndex 链路已接入 `PersistDocHierarchyFromResults`
- [x] `POST /api/substrate/doc/tree` 已实现并可查询文档树与边
- [x] `markdown/pdf/pptx/docx` 解析入口可走通并保留 provenance
- [x] image OCR MVP 已接入（`DetectLang=image` + `DocParser(image)` + `tesseract`）

### 本轮收口

- [x] markdown 层级树稳定化（标题层级规则 + 回归测试）
- [x] `parse_artifacts` 成功/降级/失败写入链路（MVP）
- [x] `references` 边自动抽取（MVP：markdown 链接规则）

### 未完成（当前无）

- [x] `POST /api/substrate/doc/neighbors`
- [x] `POST /api/substrate/doc/parse-artifacts`（查询能力）
- [x] pdf 标题候选规则调优与质量门槛统计（MVP 规则 + quality_json）
- [x] pptx 标题块识别增强（MVP：首文本块标题化）

## 1. 目标

为 `pdf/pptx/markdown/image` 增加统一的“章节层级切片 + 关系管理 + 可观测失败输出”能力，使其满足 substrate 层要求：

- 按标题/章节语义分层存储，而非纯扁平 chunk
- 维护文档内部依赖关系（包含关系、引用关系、时序关系）
- 保留可回溯 provenance（source/page/slide/span）
- 解析失败有结构化降级产物，不静默吞掉

## 2. 非目标

- 不在本期引入 DeepMemory 的 social_status 或 interpretant 治理
- 不在本期做跨文档复杂推理图（仅做基础跨文档引用边）
- 不强制替换现有 `FunctionInfo` 索引路径，先兼容并行输出

## 3. 当前差距（as-is）

- `markdown`：已有标题分段，但存储扁平化，无章节树落库
- `pdf`：按页与固定行数切片，无目录级章节层次
- `pptx`：按 slide 切片，无标题-正文层次建模
- `image`：此前无 OCR 入口，不可索引
- 关系管理：无 `node/edge` 级文档依赖图
- 失败处理：调用链上存在“记录日志后跳过”路径，缺少结构化失败 artifact

## 4. 数据模型（to-be）

新增表（建议放在 `.gitgo/code_index.db`）：

### 4.1 `doc_nodes`

- `node_id` TEXT PRIMARY KEY
- `project_dir` TEXT NOT NULL
- `doc_id` TEXT NOT NULL
- `parent_id` TEXT
- `node_type` TEXT NOT NULL
  - `document | chapter | section | subsection | chunk | slide | page`
- `level` INTEGER NOT NULL
- `title` TEXT
- `content` TEXT
- `source` TEXT NOT NULL
- `page` INTEGER DEFAULT 0
- `slide` INTEGER DEFAULT 0
- `start_line` INTEGER DEFAULT 0
- `end_line` INTEGER DEFAULT 0
- `anchor_id` TEXT
- `parse_quality` REAL DEFAULT 0
- `created_at` TEXT NOT NULL
- `updated_at` TEXT NOT NULL

索引：

- `idx_doc_nodes_doc_id_level`
- `idx_doc_nodes_parent_id`
- `idx_doc_nodes_anchor_id`

### 4.2 `doc_edges`

- `edge_id` TEXT PRIMARY KEY
- `project_dir` TEXT NOT NULL
- `doc_id` TEXT NOT NULL
- `from_node_id` TEXT NOT NULL
- `to_node_id` TEXT NOT NULL
- `edge_type` TEXT NOT NULL
  - `contains | references | follows | semantic_related`
- `weight` REAL DEFAULT 1.0
- `evidence` TEXT
- `created_at` TEXT NOT NULL

索引：

- `idx_doc_edges_doc_id_type`
- `idx_doc_edges_from_to`

### 4.3 `parse_artifacts`

- `artifact_id` TEXT PRIMARY KEY
- `project_dir` TEXT NOT NULL
- `source` TEXT NOT NULL
- `mime_type` TEXT
- `status` TEXT NOT NULL
  - `success | degraded | failed`
- `error_code` TEXT
- `error_message` TEXT
- `fallback_mode` TEXT
  - `none | page_chunk | slide_chunk | plain_text_chunk`
- `quality_json` TEXT
- `created_at` TEXT NOT NULL

## 5. 解析与关系构建策略

## 5.1 Markdown

- 解析标题层级：`#` 到 `######` 映射 `level=1..6`
- 构建树：标题节点为父，正文 chunk 为子
- 边：
  - `contains`：文档 -> 标题 -> chunk
  - `follows`：同级节点顺序边
  - `references`：`[text](#anchor)` 或显式链接

## 5.2 PDF

- 第一阶段（MVP）：
  - 按页抽取（保留 `page`）
  - 页内弱标题检测（短行、编号模式、全大写、冒号结尾）
  - 构建 `page -> section_candidate -> chunk`
- 失败降级：
  - 若标题检测失败：`page -> chunk`
  - 写入 `parse_artifacts.status=degraded`

## 5.3 PPTX

- 以 slide 为一级节点（`node_type=slide`）
- 通过 XML 文本顺序与简规则识别标题块（首个高置信文本块）
- 构建 `slide -> title_chunk/body_chunk`
- 边：
  - `contains`：slide -> chunk
  - `follows`：slide_n -> slide_n+1

## 5.4 Image OCR（MVP）

- 第一阶段（MVP）：
  - 使用 `tesseract` 抽取图片文本
  - 输出 `source::ocr` chunk，进入统一索引链路
  - 与现有 `FunctionInfo(llm_parser)` 兼容，不破坏旧逻辑
- 失败降级：
  - OCR 失败或输出为空时返回结构化错误（后续写入 `parse_artifacts.failed`）
- 边界：
  - 本期仅做基础文字识别，不做图片空间 Transformer 高精识别

## 6. 接口改造点

在 `cmd/app/fm_http.go` 增加：

- `POST /api/substrate/doc/tree`
  - 入参：`project_dir, source|doc_id`
  - 出参：节点树 + 邻接边
- `POST /api/substrate/doc/neighbors`
  - 入参：`project_dir, node_id, edge_type, direction`
  - 出参：邻居节点
- `POST /api/substrate/doc/parse-artifacts`
  - 入参：`project_dir, source?, status?, limit`
  - 出参：结构化解析产物（含降级/失败）

兼容性要求：

- 不破坏现有 `/api/search`、`/api/functions` 路径
- 新能力作为增量 substrate contract 暴露给 DeepMemory adapter

## 7. 代码改造点

- `internal/parser/doc_parser.go`
  - 从“只返回扁平 sections”扩展为“sections + hierarchy metadata”
  - 新增 `image` OCR 抽取路径（`tesseract`）
- `internal/back/backwork.go`
  - 在 BuildIndex/Incremental 中写入 `doc_nodes/doc_edges/parse_artifacts`
- `internal/index/*`
  - 增加 doc graph DAO（增删改查）
- `cmd/app/fm_http.go`
  - 新增 doc tree / neighbors / parse-artifacts API
- `cmd/app/README.md`
  - 补齐接口文档和示例

## 8. 测试计划

单元测试：

- markdown 层级树构建正确（父子层级、顺序边）
- pdf 降级路径可触发且有 `parse_artifact`
- pptx slide 层级与 source/slide provenance 保留
- image OCR 入口可走通且保留 `source::ocr`

集成测试：

- 构建索引后可查询 `doc/tree`
- `doc/neighbors` 返回 `contains/follows/references`
- 失败解析可在 `doc/parse-artifacts` 查询

验收门槛：

- `go test ./internal/parser ./cmd/app` 相关新增测试全绿
- 对 3 份 markdown、3 份 pdf、3 份 pptx、3 份 image 样例：
  - 层级完整率 >= 0.85
  - 孤儿节点率 <= 0.05
  - provenance 覆盖率 = 1.0（source 必填；pdf page/ppt slide 可用时必填）

## 9. 里程碑与工期评估

M1（约 3-4 人日）：

- 数据库 schema + DAO + markdown 层级树
- `doc/tree` 基础查询接口

M2（约 3-4 人日）：

- pdf/pptx 层级化与降级 artifact
- `doc/parse-artifacts` 接口
- image OCR 失败产物结构化记录

M3（约 2-3 人日）：

- 关系边与 `doc/neighbors` 接口
- 集成测试、README、回归修复

总计：

- MVP：`7-10` 人日
- v1（含质量指标与更多样例集）：额外 `2-3` 周

## 10. 风险与缓解

- PDF 标题检测不稳定：
  - 先用可解释规则 + 降级标记，避免伪精确
- 旧路径兼容风险：
  - 保持并行存储，不替换旧表
- 性能开销：
  - 批量写入 + 索引延迟构建

## 11. 对齐判定（Done Definition）

当以下条件同时满足，视为“多模态层级化解析能力达标”：

- `markdown/pdf/pptx/image` 均可落地为 `doc_nodes`
- 至少支持 `contains/follows/references` 三类边
- 任一解析失败均可在 `parse_artifacts` 查询到结构化记录
- DeepMemory 可通过 substrate API 读取文档树与依赖关系
