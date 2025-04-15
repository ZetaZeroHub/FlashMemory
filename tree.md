# 代码组织结构：
```
flashmemory/                 (project root)
├─ cmd/
│   └─ tree/               (Tree-sitter 交互式查询工具)
│       ├─ tree.go        (命令行工具入口)
│       └─ README.md      (使用说明)
├─ internal/
│   ├─ parser/
│   │    ├─ parser.go            (定义 Parser 接口和通用结构)
│   │    ├─ tree_sitter_parser.go (Tree-sitter 实现的多语言解析器)
│   │    ├─ tree_sitter_parser_test.go (Tree-sitter 解析器测试)
│   │    ├─ tree_sitter_parser_imports_calls_test.go (导入和调用解析测试)
│   │    ├─ ast_parser.go        (基于 AST 的实现)
│   │    ├─ regex_parser.go      (基于正则表达式的实现)
│   │    └─ llm_parser.go        (基于 LLM 的实现)
│   ├─ analyzer/
│   │    └─ analyzer.go          (分析函数，生成描述)
│   ├─ graph/
│   │    └─ graph.go             (构建依赖图，知识图谱结构)
│   ├─ index/
│   │    ├─ index.go             (处理 SQLite 和 Faiss 存储)
│   │    ├─ faiss_wrapper.go     (Faiss 封装接口)
│   │    ├─ http_faiss_wrapper.go (HTTP 方式的 Faiss 调用)
│   │    └─ memory_faiss_wrapper.go (内存方式的 Faiss 实现)
│   ├─ search/
│   │    └─ search.go            (实现对索引的查询)
│   └─ visualize/
│        └─ visualize.go         (生成统计数据和可能的图表)
├─ pkg/                          (外部使用的包)
│   └─ ... (可选)
├─ .gitgo/                       (输出索引目录，运行时创建)
└─ go.mod                        (模块定义，依赖项配置)
```
