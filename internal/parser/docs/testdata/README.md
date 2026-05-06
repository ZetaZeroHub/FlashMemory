# DocParser 测试 Fixture

> 配套：`docs/specs/20260502-doc-parser-overhaul/`
> 生成方式：运行 `python3 scripts/gen_test_docs.py`（依赖见脚本注释）

## 文件清单

### `md/`
| 文件 | 用途 | 关联测试 |
|------|------|---------|
| `kb_sample.md` | 含 3 条真实 kb_qa 编号的小样本 | MD-06 |
| `nested_headings.md` | H1→H2→H3 嵌套结构 | MD-02 |
| `no_headings.md` | 全文无标题（fallback 测试） | MD-03 |
| `long_section.md` | 单 H2 下塞 5000+ 字符 | MD-04 |
| `intro_before_heading.md` | 标题前有内容 | MD-01 |
| `emoji_titles.md` | 标题含 emoji 与特殊字符 | MD-05 |

### `pdf/`
| 文件 | 用途 | 关联测试 |
|------|------|---------|
| `english_numeric.pdf` | 数字标题（1.2.3 风格） | PDF-01 |
| `chinese_headings.pdf` | 中文章节标题（### N. / 第N章） | PDF-02 |
| `cross_page_qa.pdf` | QA 横跨第 3-4 页 | PDF-04 |
| `flat_page.pdf` | 单页 80+ 行无标题 | PDF-03 |

### `docx/`
| 文件 | 用途 | 关联测试 |
|------|------|---------|
| `headings_en.docx` | 英文版 office Heading 1/2/3 | DOCX-01 |
| `headings_cn.docx` | 中文版 office "标题 1/2/3" | DOCX-05 |
| `with_table.docx` | 含表格 `<w:tbl>` | DOCX-02 |
| `with_image.docx` | 含嵌入图片 | DOCX-03 |
| `no_styles.docx` | 无任何样式（纯段落） | DOCX-04 |

### `pptx/`
| 文件 | 用途 | 关联测试 |
|------|------|---------|
| `multi_slides.pptx` | ≥10 张 slide 测排序 | PPT-03 |
| `with_notes.pptx` | 含演讲者备注 | PPT-05 |

### `images/`
| 文件 | 用途 | 关联测试 |
|------|------|---------|
| `chinese_text.png` | 中文 OCR 测试 | IMG-01 |
| `english_text.png` | 英文基线 | IMG-02 |
| `low_quality.png` | 低质量图片降级测试 | IMG-04 |

## 重新生成

```bash
# 安装依赖
pip install python-docx python-pptx Pillow reportlab

# 系统依赖（可选，用于 PDF 生成）
brew install pandoc poppler  # macOS

# 生成全部 fixture
python3 scripts/gen_test_docs.py
```

## 校验

每次重新生成后，校验和会变化（PDF/DOCX 含时间戳）。测试断言不依赖整体 hash，只断言关键字段（标题数、kb_qa 编号匹配等）。
