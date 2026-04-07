# FlashMemory

跨语言代码分析与语义搜索系统。

## 功能特性

- **多语言支持**：解析 Go / Python / JavaScript / Java / C++ 代码
- **智能分析**：使用 LLM 生成函数描述和语义理解
- **增量索引**：基于 Git 变更记录的高效更新机制
- **混合搜索**：支持语义搜索、关键词搜索和混合模式
- **知识图谱**：构建函数级别的代码依赖关系图

## 安装

```bash
pip install flashmemory
```

## 使用

```bash
# CLI 工具
fm -dir /path/to/project -query "文件上传处理"

# HTTP 服务
fm_http
```

## 更多信息

- [GitHub](https://github.com/ZetaZeroHub/FlashMemory)
- [API 文档](https://github.com/ZetaZeroHub/FlashMemory/blob/main/cmd/app/README.md)
