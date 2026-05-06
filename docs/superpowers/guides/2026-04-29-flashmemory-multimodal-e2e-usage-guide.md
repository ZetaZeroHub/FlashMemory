# FlashMemory 多模态文档图谱 E2E 使用手册

- 日期：2026-04-29
- 状态：usage guide
- 关系：配套 `2026-04-29-flashmemory-multimodal-hierarchical-ingest-spec-v0.1.md`

## 1. 目标

本文给出从“多模态文档摄取”到“文档树查询/邻接查询/解析产物审计”的端到端操作路径。

适用对象：

- `markdown`
- `pdf`
- `pptx`
- `docx`
- `image`（OCR MVP）

## 2. 前置条件

1. 已启动 FlashMemory HTTP 服务（默认端口 `5532`）。
2. 项目目录存在并可写（会创建 `.gitgo/`）。
3. 外部依赖：
   - PDF 抽取：`pdftotext`
   - 图片 OCR：`tesseract`

## 3. 端到端主流程

## 3.1 构建索引（摄取入口）

```bash
curl -X POST http://localhost:5532/api/index \
  -H "Content-Type: application/json" \
  -d '{
    "project_dir": "/abs/path/to/project",
    "relative_dir": "docs",
    "full": true
  }'
```

执行后系统会：

- 解析文档为 `llm_parser` 记录
- 写入 `functions`（兼容旧路径）
- 写入文档图谱：`doc_nodes/doc_edges`
- 写入解析审计：`parse_artifacts`

## 3.2 查询文档层级树（Doc Tree）

```bash
curl -X POST http://localhost:5532/api/substrate/doc/tree \
  -H "Content-Type: application/json" \
  -d '{
    "project_dir": "/abs/path/to/project",
    "source": "docs/guide.md"
  }'
```

可选入参：

- `doc_id`（若已知）
- `source`（常用）

响应核心字段：

- `nodes[]`：文档节点（`document/chapter/section/subsection/chunk`）
- `edges[]`：关系边（`contains/follows/references`）

## 3.3 查询文档邻接（Doc Neighbors）

```bash
curl -X POST http://localhost:5532/api/substrate/doc/neighbors \
  -H "Content-Type: application/json" \
  -d '{
    "project_dir": "/abs/path/to/project",
    "node_id": "node_xxx",
    "direction": "both",
    "edge_type": "references",
    "limit": 20
  }'
```

参数说明：

- `direction`: `in | out | both`
- `edge_type`: `contains | follows | references`（可空，表示不过滤）

## 3.4 查询解析产物审计（Parse Artifacts）

```bash
curl -X POST http://localhost:5532/api/substrate/doc/parse-artifacts \
  -H "Content-Type: application/json" \
  -d '{
    "project_dir": "/abs/path/to/project",
    "status": "degraded",
    "limit": 50
  }'
```

常用过滤：

- `status`: `success | degraded | failed`
- `source`: 某个文档源路径

关键字段：

- `error_code`
- `error_message`
- `fallback_mode`
- `quality_json`

## 4. 数据语义约定

## 4.1 节点语义

- `document`：文档根节点
- `chapter/section/subsection`：标题层级节点
- `chunk`：正文块

## 4.2 关系语义

- `contains`：父子包含
- `follows`：顺序关系
- `references`：引用关系（MVP：markdown 链接规则）

## 4.3 解析状态语义

- `success`：解析成功且入图
- `degraded`：解析降级（依赖缺失或抽取质量不足）
- `failed`：解析失败

## 5. 多模态行为说明

## 5.1 Markdown

- 标题分层（`#`~`######`）映射为层级节点
- 支持基础 `references` 抽取（markdown 链接）

## 5.2 PDF

- 页级抽取 + 标题候选规则（MVP）
- 若标题识别弱，则退化为页内 chunk

## 5.3 PPTX

- slide 级抽取
- 首个非空文本块作为标题（MVP）

## 5.4 DOCX

- 从 `word/document.xml` 抽取文本并切片

## 5.5 Image OCR（MVP）

- 通过 `tesseract` 做文字识别
- 产物源标记：`source::ocr`

## 6. 常见问题排查

1. `doc/tree` 无结果  
检查是否先执行 `/api/index`，以及 `source` 是否与落库路径一致（相对路径）。

2. `parse-artifacts` 全是 `failed`  
优先检查 `pdftotext` / `tesseract` 是否可执行，并确认文件本身可抽取文本。

3. 图片识别内容为空  
当前 OCR 为 MVP，先尝试更清晰图片或更高分辨率，再做语言参数调优。

## 7. 最小验收清单

- 能对 `markdown/pdf/pptx/docx/image` 至少各 1 个样本完成索引。
- `doc/tree` 能返回 `nodes + edges`。
- `doc/neighbors` 能按 `references` 或 `contains` 查到邻接。
- `doc/parse-artifacts` 能查到 `success`，并至少可观察到 1 条失败/降级样本。 
