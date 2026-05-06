# FlashMemory 文档总目录（Index）

- 更新时间：2026-05-05
- 当前版本：v0.4.5（pip-package/pyproject.toml 为准）
- 范围：`/Users/apple/Public/openProject/flashmemory/docs` 与 `/Users/apple/Public/openProject/flashmemory/deepmemory/docs`
- 目标：按"接口 / SPEC / 计划 / 指南 / 报告 / 论文 / 资产"统一分层，并给出可执行导航

> **近期重大变动（2026-05-03 → 05-04，Zvec 桥进程可靠性硬化）**
> - `internal/back/faiss.go`：`FaissManager.Free()` 现在释放 `allWrappers` 列表里**所有**曾经持有的 wrapper（含 `Reset()` 换出的旧 wrapper），幂等且并发安全。
> - `internal/back/backwork.go`：`BuildIndex` / `IncrementalUpdate` 入口 `defer fm.Free()`，索引完成后不再残留桥进程持锁。
> - `internal/index/zvec_wrapper.go`：新增包级 `activeZvecWrappers sync.Map` 与 `FreeAllActiveWrappers()`，提供进程退出兜底路径。
> - `pip-package/flashmemory/zvec_engine.py`：`_try_open_collection` 升级为三层自愈（open → 清 LOCK → rebuild），匹配模式覆盖 `lock` / `recovery` / `corrupt` / `manifest` / `segment` / `idmap` / `checksum` / `no such file`。
> - `cmd/app/fm_http.go`：信号处理路径改为 `e.Shutdown(ctx, 30s)` + `FreeAllActiveWrappers`，替换原 `os.Exit(0)` 的硬切。
> 详见 [Zvec 集成指南 §11 可靠性](guides/zvec_integration_guide_cn.md#11-%E5%8F%AF%E9%9D%A0%E6%80%A7%E4%B8%8E%E6%A1%A5%E8%BF%9B%E7%A8%8B%E7%94%9F%E5%91%BD%E5%91%A8%E6%9C%9F)。

## 1. FlashMemory 主文档集（`docs/`）

### 1.1 总入口

- [Superpowers Index](/Users/apple/Public/openProject/flashmemory/docs/superpowers/INDEX.md)

### 1.2 接口文档（Interfaces）

- [HTTP API 深度分析](/Users/apple/Public/openProject/flashmemory/docs/interfaces/http_api_deep_analysis.md)

### 1.3 规格与计划（Specs）

- [FlashMemory 多模态层级化解析与依赖管理 Spec v0.1](/Users/apple/Public/openProject/flashmemory/docs/superpowers/specs/flashmemory/2026-04-29-flashmemory-multimodal-hierarchical-ingest-spec-v0.1.md)

### 1.4 使用指南（Guides）

- [Release Guide](/Users/apple/Public/openProject/flashmemory/docs/guides/release_guide.md)
- [zvec Integration Guide](/Users/apple/Public/openProject/flashmemory/docs/guides/zvec_integration_guide.md)
- [zvec Integration Guide（中文）](/Users/apple/Public/openProject/flashmemory/docs/guides/zvec_integration_guide_cn.md)
- [自定义高可用 Skill 完全教程](/Users/apple/Public/openProject/flashmemory/docs/guides/自定义高可用Skill完全教程.md)
- [认知记忆主轴导读](/Users/apple/Public/openProject/flashmemory/docs/superpowers/guides/2026-04-23-memory-index-guide.md)
- [多模态文档图谱 E2E 使用手册](/Users/apple/Public/openProject/flashmemory/docs/superpowers/guides/2026-04-29-flashmemory-multimodal-e2e-usage-guide.md)

### 1.5 AI 编程规范与提示词模板（AI Dev Spec）

> 面向 Claude Code 的结构化开发规范，配合 `.claude/rules/` 使用。

**规则文件（`.claude/rules/`，会话启动时自动加载）：**
- `architecture.md` — 引擎选择、HTTP API、i18n 约定、.gitgo 安全、二进制命名
- `testing.md` — TDD 防幻觉规则、测试文件保护、覆盖率要求
- `workflow.md` — SDD 三层产物流程、Commit 规范、PR 流程、压缩指令
- `security.md` — 环境变量保护、迁移文件保护、网络操作规范

**提示词模板（`docs/templates/`）：**
- [功能开发模板](templates/feature-spec-template.md) — SDD 发起 / 小功能 / CLI 命令 / HTTP 端点
- [Bug 修复模板](templates/bug-fix-template.md) — 标准修复 / LLM 错误 / SQLite 错误 / CI 失败 / 引擎问题
- [代码审查模板](templates/code-review-template.md) — PR 前审查 / 安全审计 / Parser 审查 / Diff 审查

**SDD 规格文档（`docs/specs/`，按需创建）：**

```
docs/specs/<YYYYMMDD>-<feature-name>/
├── spec.md    # WHAT：用户故事、验收标准、边界情况
├── plan.md    # HOW：架构、数据模型、接口定义
└── tasks.md   # DO：原子任务列表、TDD 顺序
```

### 1.6 架构 / 论文 / 研究资产

- 架构文档：`docs/superpowers/architecture/`
- 研究分篇：`docs/superpowers/specs/memory-research/`
- 论文稿件：`docs/superpowers/papers/`
- 图像与构建产物：`docs/superpowers/assets/`

### 1.7 运维与日志

- 运行日志：`docs/operations/logs/`
- 历史归档：`docs/operations/archive/`

## 2. DeepMemory 文档集（`deepmemory/docs/`）

- [DeepMemory Docs Index](/Users/apple/Public/openProject/flashmemory/deepmemory/docs/INDEX.md)

## 3. 状态判定规则（统一口径）

- `interfaces/`：对接合同与稳定接口
- `specs/`：实现规格与验收标准（可能进行中）
- `planning/`：待执行与里程碑计划
- `reports/`：已完成阶段的验证证据
- `guides/`：上手、部署、二开、FAQ
- `architecture/`：顶层设计与边界原则
- `papers/`：研究论文与投稿草稿

