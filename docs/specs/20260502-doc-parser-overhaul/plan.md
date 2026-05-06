# 文档解析器重构 SDD · plan.md

> 配套 spec.md v1.0
> 技术栈：Go 1.23 / SQLite / Zvec / 外部依赖：pandoc, pdftotext, tesseract

---

## 1. 整体架构

### 1.1 重构后的目录结构

```
internal/parser/
├── doc_parser.go                  # 顶层 DocParser，根据 lang 派发
├── doc_parser_test.go             # 通用测试
├── docs/                          # ← 新增子包
│   ├── markdown.go                # MD 切片（增强自然段二次切）
│   ├── markdown_test.go
│   ├── pdf.go                     # PDF 切片（增强中文标题识别）
│   ├── pdf_test.go
│   ├── docx.go                    # DOCX 切片（新增样式感知）
│   ├── docx_test.go
│   ├── pptx.go                    # PPTX 切片（增加 notes 支持）
│   ├── pptx_test.go
│   ├── image.go                   # OCR（中英双语）
│   ├── image_test.go
│   ├── pandoc.go                  # ← 新增：Pandoc fallback
│   ├── pandoc_test.go
│   ├── chunking.go                # 共享：自然段切分、字符限制
│   └── chunking_test.go
└── testdata/                      # ← 新增：测试 fixture
    ├── md/
    ├── pdf/
    ├── docx/
    ├── pptx/
    └── images/

internal/analyzer/
└── llm_analyzer.go                # 函数级保持不变（已隔离）

internal/module_analyzer/
├── module_analyzer.go             # 修改：按 doc_ratio 选择 prompt
└── prompt_selector.go             # ← 新增：prompt 选择逻辑

resource/
└── fm.yaml                        # 新增 doc_file_analyzer_prompts / doc_module_analyzer_prompts

config/
└── config.go                      # 新增 DocParserConfig 节
```

### 1.2 关键调用链

```
ParseFile(path)
  └─ extractDocumentSections(path, lang)
      ├─ [pandoc_fallback == "force"]
      │    └─ pandoc.ConvertToMarkdown(path) → splitMarkdownSections
      ├─ [lang dispatch]
      │    ├─ markdown   → docs.SplitMarkdown
      │    ├─ pdf        → docs.SplitPDF       (新增中文标题识别)
      │    ├─ docx       → docs.SplitDOCX      (新增样式感知)
      │    ├─ pptx       → docs.SplitPPTX      (新增 notes)
      │    └─ image      → docs.SplitImage     (中英双语 OCR)
      └─ [if 0 sections OR low_quality]
           └─ pandoc fallback (auto 模式)
```

---

## 2. 核心数据模型

### 2.1 docSection 扩展

```go
type docSection struct {
    title      string
    startLine  int
    endLine    int
    content    string
    source     string
    page       int
    slide      int

    // 新增字段
    headingLevel int    // 1-6, 0 表示非标题段
    parentTitle  string // 父级章节标题（嵌套时用）
    quality      string // "high"/"medium"/"low" — 标记 fallback 候选
    kbIDs        []string // 提取到的 kb_qa_xxx 列表（赛题专用，可选启用）
}
```

### 2.2 配置节

```go
// config/config.go 新增
type DocParserConfig struct {
    PandocFallback     string `yaml:"pandoc_fallback"`      // "off" | "auto" | "force"
    PandocCacheDir     string `yaml:"pandoc_cache_dir"`     // 默认 .gitgo/pandoc_cache
    PandocBin          string `yaml:"pandoc_bin"`           // 默认 "pandoc"
    DocChunkLines      int    `yaml:"doc_chunk_lines"`      // 默认 80
    MaxSectionChars    int    `yaml:"max_section_chars"`    // 默认 4000，超过触发自然段二次切
    MinChunkChars      int    `yaml:"min_chunk_chars"`      // 默认 100
    OCRLangs           string `yaml:"ocr_langs"`            // 默认 "chi_sim+eng"
    ExtractKBIDs       bool   `yaml:"extract_kb_ids"`       // 提取 kb_qa_xxx 编号到字段
}
```

### 2.3 fm.yaml 新增提示词

```yaml
doc_file_analyzer_prompts:
    header: "请为以下知识库文档生成一个简洁的内容摘要。文档路径:\n"
    sub_module_header: "文档中包含的章节及其内容摘要:"
    footer: |
        请基于以上章节内容，生成一个简洁的文档级摘要，包括：
        1. 该文档涵盖的主要主题/业务领域
        2. 文档结构（多少章节、覆盖哪些子主题）
        3. 该文档对回答用户问题的参考价值
        注意：这是一份业务文档（非代码），请勿使用"模块/类/函数"等代码术语。

doc_module_analyzer_prompts:
    header: "请为以下文档目录生成一个全面的目录级摘要。目录路径:\n"
    sub_module_header: "该目录包含以下文档:"
    footer: |
        请基于以上文档摘要，生成一个简洁的目录级描述。
        注意：这是文档资料（非代码模块），描述应聚焦于"业务领域、主题分类、文档用途"。
```

---

## 3. 接口定义

### 3.1 公共接口（保持向后兼容）

```go
// 不变
func (p *DocParser) ParseFile(path string) ([]FunctionInfo, error)
```

### 3.2 内部新接口

```go
// internal/parser/docs/chunking.go
package docs

// 共享：当 section.content 超过 maxChars 时按自然段二次切
func RechunkBySection(s docSection, maxChars int) []docSection

// 共享：识别"低质量"切片（用于 fallback 决策）
func IsLowQuality(sections []docSection, lang string) bool

// internal/parser/docs/pandoc.go
package docs

type PandocConverter struct {
    Bin      string
    CacheDir string
}

func (c *PandocConverter) ConvertToMarkdown(path string) (string, error)
func (c *PandocConverter) IsAvailable() bool

// internal/parser/docs/docx.go — 新增样式感知
type docxStyleParser struct {
    headingStyles map[string]int // "Heading1" → 1, "标题 1" → 1
}

// internal/module_analyzer/prompt_selector.go
package module_analyzer

type PromptSelector struct {
    cfg *config.Config
}

// 根据文件下 FunctionInfo 类型分布选择 prompt
func (s *PromptSelector) SelectFilePrompt(funcs []LLMAnalysisResult) PromptSet
func (s *PromptSelector) SelectModulePrompt(subDescs []SubModuleDesc) PromptSet
```

### 3.3 测试 Helper

```go
// internal/parser/docs/testhelper.go
func mustReadFixture(t *testing.T, name string) string
func assertSectionsContain(t *testing.T, sections []docSection, kbIDs ...string)
func assertNoSectionTruncated(t *testing.T, sections []docSection)
```

---

## 4. 实施阶段

### 阶段一：测试 fixture 准备（独立 PR）
- 从 `assets/data/knowledge_base/` 中提取小样本作为 testdata
- 构造一份带 Heading 样式的测试 DOCX（用 python-docx 生成）
- 构造一份中文标题 PDF（用 pandoc 生成）
- 构造一份多语言图片测试样本

### 阶段二：MD 增强（最小变更）
- 实现 `RechunkBySection`：单 section > 4000 字符时按 `\n\n` 切，再不行按 `。`
- 在 `SplitMarkdown` 出口处调用
- 配置 `extract_kb_ids` 时填充 `kbIDs` 字段
- 添加测试

### 阶段三：DOCX 样式感知（核心改动）
- 替换 `extractTextNodesFromXML`：使用 `mholt/archives` 或自实现的 streaming XML parser
- 解析 `<w:p>` 段落，读取 `<w:pStyle w:val="..."/>`
- 维护标题样式映射表（兼容中英版 office）
- 标题段触发新 section，正文段累积
- 表格 `<w:tbl>` 提取后插入 `[TABLE]\n...` 标记
- 添加 6 个 TC

### 阶段四：PDF 中文标题识别
- 扩展 `pdfHeadingPattern`：
  ```regex
  ^(\d+(\.\d+){0,3}        # 1, 1.2, 1.2.3
   |\d+\.\s*[一-龥] # 1. 中文标题
   |#{1,6}\s+               # markdown 风格（pdftotext -layout 偶尔保留）
   |第[一二三四五六七八九十百0-9]+[章节条]
   |[A-Z][A-Z0-9 _-]{3,}
   |.{1,40}:)$
  ```
- 跨页 section 合并：检测页边界处是否在 section 中间，是则合并
- 添加 6 个 TC

### 阶段五：Pandoc Fallback
- 实现 `PandocConverter`：
  - `IsAvailable()` 检查 `exec.LookPath("pandoc")`
  - `ConvertToMarkdown(path)`：基于 sha256 缓存，命中则直接读
  - 调用：`pandoc -f auto -t gfm --wrap=none -o cache.md path`
- 在 `extractDocumentSections` 入口加分支：
  ```go
  if cfg.PandocFallback == "force" && lang != "markdown" && lang != "text" {
      return tryPandoc(path)
  }
  // ... 原生解析 ...
  if cfg.PandocFallback == "auto" && (len(sections) == 0 || IsLowQuality(sections, lang)) {
      return tryPandoc(path)
  }
  ```
- 添加 7 个 TC

### 阶段六：PPTX & Image 完善
- PPTX：解析 `ppt/notesSlides/notesSlide*.xml`，独立成 section（标记 `slide_N_notes`）
- PPTX：按数字排序而非字典序
- Image：OCR 改用 `cfg.OCRLangs`，默认 `chi_sim+eng`
- 添加 9 个 TC

### 阶段七：Embedding 层修正
- 修复 `splitTextByToken`：去掉 `*2`，让切分粒度与估算一致
- 添加 3 个 TC（含余弦相似度回归）

### 阶段八：LLM 提示词隔离
- 实现 `PromptSelector`
- 修改 `generateFileDescription`：调用 `selector.SelectFilePrompt`，文档段独立分组
- 修改 `generateDirectoryDescription`：根据 `doc_ratio` 选择
- 在 prompt 拼装前对文档内容做"沙箱化"：用 ``` ``` ``` 三反引号包裹
- 添加 4 个 TC

### 阶段九：端到端验证
- 全量索引 `assets/data/knowledge_base/`
- 运行 1379 条 kb_qa 编号召回测试
- 跑公开评测集 295 条，统计 recall@3
- 输出对比报告（重构前 vs 重构后）

---

## 5. 风险评估

| 风险 | 影响 | 缓解 |
|------|------|------|
| DOCX XML streaming 解析复杂 | 阶段三延期 | 先用 `unidoc/unioffice` 第三方库 prototype，验证后再决定是否自实现 |
| Pandoc 在 CI 环境缺失 | 测试失败 | 测试中检测 `pandoc` 不可用时 SKIP 而非 FAIL |
| 兼容老索引 | 已索引项目重新解析时偏移 | 检测 `code_index.db` schema_version，触发重建提示 |
| 中文 OCR 语言包大 | 二进制体积膨胀 | tesseract 语言包不打包进 fm 二进制，运行时检测，缺失则报错 |
| LLM prompt 改动影响现有项目 | 已索引项目模块描述质量变化 | 仅对 `doc_ratio >= 0.7` 文件触发，代码项目不受影响 |
| Embedding 层 ×2 bug 修复 | 已索引向量与新切片不一致 | 提供 `--rebuild-embeddings` 命令，但默认不强制重建 |

---

## 6. 性能预算

- DOCX 样式感知解析：相比当前实现 +30% 解析时间（XML 全解析 vs 仅 `<w:t>`）
- Pandoc fallback（首次）：~500ms/MB（pandoc 启动开销）
- Pandoc fallback（缓存命中）：~10ms（仅文件读）
- 整体索引时间增量预算：≤ +20%（CI 验证）

---

## 7. 兼容性策略

- **数据库 schema**：不改 `functions` 表结构，新增字段（headingLevel/parentTitle/quality）只在内存中使用
- **MCP 接口**：`flashmemory_search` / `flashmemory_index` 参数完全不变
- **配置文件**：`fm.yaml` 新增节默认值合理，老配置文件不需要改动也能跑
- **向后回滚**：保留原 `splitMarkdownSections` 等函数为 `*Legacy`，通过 feature flag 切换
