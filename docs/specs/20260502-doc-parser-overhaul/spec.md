# 文档解析器重构 SDD · spec.md

> 版本：v1.1 · 创建日期：2026-05-02
> 范围：`internal/parser/doc_parser.go` 及相关切片/索引/LLM 链路
> 目标：让 MD / DOCX / PDF / PPTX / Image 五种格式均达到"按结构层级 + 自然段 + 字数控制"的完美切片

---

## 0. Bug Catalog（按严重度排序）

本节集中列出本次 SDD 必须修复的所有 bug。每个 bug 必须有对应的测试用例与实施任务，全部在 §4 测试矩阵和 tasks.md 中可追溯。

### BUG-001【严重 · P0】文档段被错误注入"代码导向"LLM 提示词
**位置**：[`internal/module_analyzer/module_analyzer.go:1620-1666`](internal/module_analyzer/module_analyzer.go#L1620) → `generateFileDescription`
**根因**：函数级 LLM 调用确实通过 [`llm_analyzer.go:96-102`](internal/analyzer/llm_analyzer.go#L96) 跳过了 `FunctionType == "llm_parser"`，但**文件级模块描述生成时**会把当前文件下前 20 个 `FunctionInfo`（含 `llm_parser` 文档段）拼入 [`FileAnalyzerPrompts`](resource/fm.yaml:58)（"文件中包含的函数/方法及其描述"）。目录级同问题（[`ModuleAnalyzerPrompts`](resource/fm.yaml:174)）。
**实际影响**：
1. **质量退化**：LLM 把 PDF/DOCX 的章节标题（如"信用卡境外使用"）当成函数名总结，输出"该文件实现了一个名为 X 的方法..."这种错误描述
2. **prompt 注入风险**：知识库中若含"忽略以上指令"等内容会污染上层 prompt（黑客松陷阱题 prompt_injection 子类型直接利用此漏洞）
3. **检索退化**：`code_desc` 表存储的模块描述被错误总结后，混合检索的 description 字段语义偏差
**修复任务**：T029-T033（PromptSelector + 沙箱化 + 双 prompt）
**关联测试**：LLM-01/02/03/04
**验收**：
- 文件下 ≥ 95% 段为 `llm_parser` → 走 `DocFileAnalyzerPrompts`，prompt 中不出现"函数/方法/类"字样
- 混合文件 → prompt 显式分组"代码单元:" / "文档章节:"
- 文档内容用三反引号包裹（沙箱化）

### BUG-002【严重 · P0】DOCX 完全没有标题层级感知
**位置**：[`internal/parser/doc_parser.go:249-270`](internal/parser/doc_parser.go#L249) → `extractSectionsFromDOCX`
**根因**：只通过 `<w:t>` 文本节点提取，**完全忽略 `<w:pStyle>` 样式属性**（Heading 1/2/3 / 中文版"标题 1"），最终走 80 行/块固定窗口切。
**实际影响**：3 个 DOCX 文件（账户与转账.docx 107 条 QA、咨询与投诉.docx 124 条、综合咨询.docx）的 QA 边界被切碎，`kb_qa_xxx` 编号可能横跨多个 section。
**修复任务**：T007-T010
**关联测试**：DOCX-01/02/03/04/05/06

### BUG-003【中 · P0】PDF 中文标题不命中正则
**位置**：[`internal/parser/doc_parser.go:24`](internal/parser/doc_parser.go#L24) → `pdfHeadingPattern`
**根因**：正则 `^(\d+(\.\d+){0,3}|[A-Z][A-Z0-9 _-]{3,}|.{1,40}:)$` 仅覆盖纯数字、全大写英文、英文冒号结尾，对中文标题（"### 1. 我想申诉"、"第一章总则"）不命中 → fallback 到 80 行/块。
**实际影响**：2 个中文 PDF（话费账单与缴费.pdf 76 条 QA、店铺账户与支付.pdf 101 条 QA）切片质量低。
**修复任务**：T011-T014
**关联测试**：PDF-01/02/03/04/05/06

### BUG-004【中 · P0】单 section 超长被尾部截断
**位置**：[`internal/parser/doc_parser.go:69-77`](internal/parser/doc_parser.go#L69)
**根因**：`maxDocSnippetChars = 4000`，超过直接 `snippet[:4000] + "..."`，丢失后续内容（含可能的 `kb_qa_xxx` 编号）。
**实际影响**：长 QA 信息丢失，可追溯性下降。
**修复任务**：T003-T006（自然段二次切代替截断）
**关联测试**：MD-04/06

### BUG-005【中 · P1】Embedding 切片粒度与 token 估算不一致
**位置**：[`internal/embedding/embedding.go:21-41`](internal/embedding/embedding.go#L21) → `splitTextByToken`
**根因**：`estimateTokens` 按 1 rune = 1 token 估算，但 `splitTextByToken` 实际按 `chunkTokenMax * 2 = 600` runes 切。导致 chunk 实际 token 数（中文）逼近 BGE-512 上限。
**实际影响**：偶发 413 错误，多 chunk 平均向量包含半截语义。
**修复任务**：T025-T028
**关联测试**：EMB-01/02/03

### BUG-006【低 · P1】Image OCR 写死英文不识别中文
**位置**：[`internal/parser/doc_parser.go:134`](internal/parser/doc_parser.go#L134) → `tesseract -l eng`
**根因**：硬编码语言参数。
**实际影响**：中文截图文档解析为空（黑客松不涉及，但生产环境需要）。
**修复任务**：T023-T024
**关联测试**：IMG-01/02/03/04

### BUG-007【低 · P2】PPTX slide 字典序排序错乱
**位置**：[`internal/parser/doc_parser.go:222`](internal/parser/doc_parser.go#L222) → `sort.Strings(slideNames)`
**根因**：字典序导致 `slide10.xml` 排在 `slide2.xml` 前面。
**实际影响**：超过 9 张 slide 的 PPTX 顺序错乱。
**修复任务**：T021-T022
**关联测试**：PPT-03/05

### Bug 严重度与黑客松收益矩阵

| Bug ID | 严重度 | 黑客松直接收益 | 修复优先级 |
|--------|--------|----------------|-----------|
| BUG-001 | P0 | 拒答可靠性 + 防 prompt 注入（陷阱 25%） | 必修 |
| BUG-002 | P0 | DOCX 231 条 QA 召回率（追溯 20%） | 必修 |
| BUG-003 | P0 | PDF 177 条 QA 召回率（追溯 20%） | 必修 |
| BUG-004 | P0 | 长 QA 完整性（准确性 50%） | 必修 |
| BUG-005 | P1 | 检索语义稳定性 | 应修 |
| BUG-006 | P1 | 多模态扩展性 | 应修 |
| BUG-007 | P2 | 长 PPTX 顺序 | 可修 |

---

## 1. 背景与问题

FlashMemory 当前的多模态文档解析存在以下被代码审计确认的问题：

### 1.1 切片层面的真实缺陷

| 格式 | 现状 | 风险 |
|------|------|------|
| **MD** | 按 H1-H6 切片 ✅ | 子层级嵌套时单 section 可能极长（H2 下塞 1 万字） |
| **PDF** | 按 `\f` 分页 + 伪标题正则 | 中文标题不命中正则 → fallback 到 80 行/块；表格/多列布局会丢结构 |
| **DOCX** | **完全没有标题感知**，只抓 `<w:t>` 文本节点后 80 行/块 | QA 边界被切碎；标题样式信息（Heading 1/2/3）被丢弃 |
| **PPTX** | 第一行为 title + 剩余 80 行/块 | 单 slide 通常足够，问题不大 |
| **Image** | `tesseract -l eng` 写死 | 中文图片无法识别 |
| **共性** | 单 section 字符硬上限 4000，超长直接尾部截断 | 长 QA 信息丢失 |

### 1.2 Embedding 层潜在问题

- `splitTextByToken` 实际切分粒度是 `chunkTokenMax * 2 = 600` runes，但 `estimateTokens` 按 1 rune = 1 token 估算 → 可能逼近 BGE-512 上限
- 多 chunk 向量按维度均值聚合，失去段内相对位置信息

### 1.3 LLM 提示词共享风险（隐蔽 bug）

经代码审计：

- ✅ **函数级安全**：`AnalyzeFunction` 在 `FunctionType == "llm_parser"` 时直接 return，文档段不会被喂入 [`AnaPrompts`](resource/fm.yaml:1)（"你是一个专业的架构师"）
- ⚠️ **文件级有风险**：`module_analyzer.go:1620-1666` 的 `generateFileDescription` 会将该文件下前 20 个 `FunctionInfo`（包含 `llm_parser` 类型的文档段）拼入提示词，使用 [`FileAnalyzerPrompts`](resource/fm.yaml:58)（"文件中包含的函数/方法及其描述"）—— 这个 prompt 假设输入是函数/方法，把"文档章节"当成"函数"喂入 LLM，**会产生：**
  1. 模块描述质量退化（LLM 把章节标题当成函数名总结）
  2. 跨语义注入风险（用户文档内容若含"忽略以上指令"等会污染上层 prompt）
  3. 黑客松 KB 客服文档被错误总结为"代码模块"，影响检索 description
- ⚠️ **目录级同问题**：`generateDirectoryDescription` 调用 [`ModuleAnalyzerPrompts`](resource/fm.yaml:174)，同样假设子模块是代码

---

## 2. 目标用户故事

### US-1：作为 RAG 系统使用者，我希望知识库文档被精确切到 QA 粒度
**验收标准**：
- 银行/电信/电商三类 KB 文档（10 个文件，1379 条 QA）索引后，每个 `kb_qa_xxxxxxxxxx` 编号能被检索定位到独立的 docSection（一个 section 至多含 1 条 QA）
- DOCX 文件的 H1/H2/H3 标题样式被正确识别并作为切片边界
- PDF 文件中的中文标题（"### N. 问题..."）被正确识别为切片边界

### US-2：作为系统集成方，我希望长文档不会因为字数超限丢失尾部信息
**验收标准**：
- 单 section 超过 4000 字符时不再"截断 + ..."，改为按自然段（空行/句号）二次切分
- 切分后每个子 section 保留 `kb_qa_xxx` 编号完整性（不允许编号被切到两个 section 中间）

### US-3：作为运维者，我希望对解析失败/低质量的文件可以自动 fallback 到 Pandoc
**验收标准**：
- 提供配置开关 `doc_parser.pandoc_fallback`（默认 `auto`）
- 三种模式：
  - `off`：禁用 fallback，原生解析失败时报错
  - `auto`：原生解析产生 0 sections **或** 检测到"低质量 fallback"（PDF 中文标题不命中、DOCX 无标题感知等）时自动转 MD
  - `force`：所有非 MD 文档强制走 Pandoc → MD → MD 解析器
- Pandoc 不可用时优雅降级（记日志，不阻塞索引）
- 转换产物缓存到 `.gitgo/pandoc_cache/<sha256>.md`，下次相同文件免转

### US-4：作为系统设计者，我希望文档段不会被错误地灌入"代码模块"的 LLM prompt
**验收标准**：
- 文件级模块描述：当文件下所有 FunctionInfo 均为 `llm_parser` 类型时，使用专用的 `DocFileAnalyzerPrompts`（"该文件是一份知识库文档..."），而非代码导向的 prompt
- 混合文件（既有代码也有文档）：在 prompt 中明确分组（"代码单元"/"文档章节"）
- 目录级：当目录下文件多数为文档时，触发 `DocModuleAnalyzerPrompts`（阈值 `doc_ratio >= 0.7`）

### US-5：作为开发者，我希望所有边界条件都有单元测试覆盖
**验收标准**：详见 §4

---

## 3. 非目标（Out of Scope）

- ❌ 不重写 zvec/FAISS 引擎层
- ❌ 不引入新的 embedding 模型
- ❌ 不修改 MCP Server 接口（保持向后兼容）
- ❌ 不处理扫描版 PDF（OCR 到 PDF 文字层属未来扩展）
- ❌ 不实现 Excel/HTML 解析（赛题不需要）

---

## 4. 单元测试矩阵（必覆盖的边界条件）

每个测试用例都列在 `internal/parser/doc_parser_test.go`，按格式分组：

### 4.1 Markdown 边界

| TC | 场景 | 期望 |
|----|------|------|
| MD-01 | 标题前有内容 | 第 1 个 section 标题为 `document_intro` |
| MD-02 | H1 → H2 → H3 嵌套 | 每级标题独立切片，父级 section 不包含子级内容 |
| MD-03 | 全文无任何标题 | fallback 到 80 行/块切片 |
| MD-04 | 单 section 超 4000 字符 | **新行为**：按自然段（连续两个换行）二次切分，不再截断 |
| MD-05 | 标题中含 emoji/特殊字符 | `normalizeSectionTitle` 正确清洗 |
| MD-06 | `kb_qa_xxx` 编号刚好在 section 末尾 | 编号完整保留 |

### 4.2 PDF 边界

| TC | 场景 | 期望 |
|----|------|------|
| PDF-01 | 纯英文 PDF 含数字标题（`1.2.3`） | 按伪标题切分 |
| PDF-02 | **中文 PDF 含 `### N. 问题`** | **新行为**：识别为标题，不再 fallback 到 80 行/块 |
| PDF-03 | 单页超 80 行无任何标题 | 兜底切片 |
| PDF-04 | 跨页内容（QA 横跨第 3-4 页） | section 包含跨页标记，但优先按页边界切 |
| PDF-05 | `pdftotext` 不可用 | 触发 Pandoc fallback（若开启）/ 报错（若关闭） |
| PDF-06 | 加密 PDF | 报错并提示 |

### 4.3 DOCX 边界

| TC | 场景 | 期望 |
|----|------|------|
| DOCX-01 | **含 Heading 1/2/3 样式** | **新行为**：按 `<w:pStyle w:val="Heading*"/>` 切片 |
| DOCX-02 | 含表格 `<w:tbl>` | 表格内容提取，section 中标记 `[TABLE]` |
| DOCX-03 | 含图片（嵌入 word/media/） | 跳过图片，文字段不丢失 |
| DOCX-04 | 无任何样式（纯段落） | fallback 到自然段切片（按空段切） |
| DOCX-05 | 标题样式为非英文（"标题 1"中文样式名） | 兼容识别（office 中文版会用 `标题 1` 而非 `Heading 1`） |
| DOCX-06 | 嵌入 SmartArt/EquationXML | 跳过且不报错 |

### 4.4 PPTX 边界

| TC | 场景 | 期望 |
|----|------|------|
| PPT-01 | 单 slide 仅含标题 | section 数 = 1，content = title |
| PPT-02 | slide 含 SmartArt 文字 | 提取 SmartArt 文字 |
| PPT-03 | slide 顺序乱（slide10.xml 在 slide2.xml 前） | 按数字排序而非字典序 |
| PPT-04 | 无 slide xml 文件 | 报错 |
| PPT-05 | slide 含演讲者备注（notesSlide） | 备注独立成 section（标记 `slide_N_notes`） |

### 4.5 Image 边界

| TC | 场景 | 期望 |
|----|------|------|
| IMG-01 | **中文图片** | **新行为**：使用 `tesseract -l chi_sim+eng` |
| IMG-02 | tesseract 未安装 | 报错并提示安装命令 |
| IMG-03 | OCR 输出空 | 报错，不写空 section |
| IMG-04 | 倾斜/低质量图片 | 解析降级，不崩溃 |

### 4.6 Pandoc Fallback 边界

| TC | 场景 | 期望 |
|----|------|------|
| FB-01 | 配置 `pandoc_fallback=off` 且原生解析失败 | 直接报错 |
| FB-02 | `auto` 模式 + 原生 0 section | 自动 fallback |
| FB-03 | `auto` 模式 + 原生低质量（DOCX 无标题样式） | 自动 fallback |
| FB-04 | `force` 模式 + DOCX | 走 Pandoc 不走原生 |
| FB-05 | Pandoc 不可用且 `force` | 优雅降级到原生（记 warn） |
| FB-06 | 缓存命中 | 跳过转换，直接读 `.gitgo/pandoc_cache/<sha256>.md` |
| FB-07 | 同一文件被并发索引 | 缓存写入有锁，不损坏 |

### 4.7 Embedding 层超限

| TC | 场景 | 期望 |
|----|------|------|
| EMB-01 | section 含 800 中文字符 | 触发 splitTextByToken，产生 2-3 chunks |
| EMB-02 | chunkMax 修正为按 rune 数（取消 ×2） | 切片粒度与估算一致 |
| EMB-03 | 多 chunk 向量平均后语义保持 | 抽样测试余弦相似度 ≥ 阈值 |

### 4.8 LLM 提示词隔离

| TC | 场景 | 期望 |
|----|------|------|
| LLM-01 | 文件下全部 FunctionInfo 为 `llm_parser` | 使用 `DocFileAnalyzerPrompts` |
| LLM-02 | 混合文件（5 函数 + 5 文档段） | prompt 分组明确 |
| LLM-03 | 目录下 doc_ratio >= 0.7 | 使用 `DocModuleAnalyzerPrompts` |
| LLM-04 | 文档段含 "忽略以上指令" | prompt 拼装时转义/沙箱化 |

### 4.9 端到端集成（assets/ 数据）

| TC | 场景 | 期望 |
|----|------|------|
| E2E-01 | 索引完整 KB 后，1379 条 `kb_qa_xxx` 全部可检索 | recall@3 ≥ 95% |
| E2E-02 | 公开评测 295 条样本，检索召回 | F1@3 ≥ 0.6（基线） |

---

## 5. 验收门 (Definition of Done)

**P0 必须通过：**
- [ ] 全部 TC 通过（go test ./internal/parser/... -race）
- [ ] `go test ./...` 整库回归通过
- [ ] 索引 `assets/data/knowledge_base/` 后，1379 条 kb_qa 编号全部可检索
- [ ] 配置项 `doc_parser.pandoc_fallback` 文档化在 `fm.yaml.bak`

**P1 应该通过：**
- [ ] DOCX 标题样式识别覆盖 office 中英文版本
- [ ] PDF 中文标题识别覆盖三种常见格式（数字开头/章节符/中文标题）
- [ ] LLM 提示词隔离对应的 `code_desc` 表内容质量目检通过

**P2 可以延后：**
- [ ] HTML / Excel 解析支持（独立 spec）
- [ ] 扫描版 PDF OCR（独立 spec）
