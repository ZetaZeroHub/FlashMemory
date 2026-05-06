# 🛠️ 自定义高可用 Skill 完全教程

> **基于 Anthropic 官方《The Complete Guide to Building Skills for Claude》整理**
>
> 本教程将手把手带你从零开始构建一个**高可用**的 Skill，涵盖规划、开发、测试、发布全流程。

---

## 📑 目录

1. [Skill 是什么](#1-skill-是什么)
2. [核心设计原则](#2-核心设计原则)
3. [动手前的规划](#3-动手前的规划)
4. [技术规范详解](#4-技术规范详解)
5. [一步步写出你的 SKILL.md](#5-一步步写出你的-skillmd)
6. [高可用指令编写技巧](#6-高可用指令编写技巧)
7. [五大工作流模式](#7-五大工作流模式)
8. [测试与迭代](#8-测试与迭代)
9. [分发与共享](#9-分发与共享)
10. [故障排除速查表](#10-故障排除速查表)
11. [完整实战案例](#11-完整实战案例)
12. [Quality Checklist — 上线前检查清单](#12-quality-checklist--上线前检查清单)

---

## 1. Skill 是什么

**Skill（技能）** 是一组打包在文件夹里的指令，用来教会 Claude 如何处理特定的任务或工作流程。

**核心价值：** 把你的专业知识、重复工作流程、团队规范"教"给 Claude 一次，之后每次对话都能自动运用，无需反复解释。

**适用场景：**
- 根据设计稿生成前端代码
- 用一致的方法论做调研
- 按团队模板生成文档
- 编排多步骤复杂流程
- 增强 MCP 工具的使用体验

> [!TIP]
> 第一个 Skill 通常 **15-30 分钟**即可完成开发与测试，你可以借助 `skill-creator` 技能来加速。

---

## 2. 核心设计原则

### 2.1 渐进式加载（Progressive Disclosure）

Skill 采用三级加载机制，最大化节省 Token 的同时保持专业能力：

```
┌─────────────────────────────────────────────────────┐
│  第一级：YAML frontmatter（name + description）      │
│  → 始终加载到 Claude 的系统提示中（~100 词）          │
│  → Claude 凭此决定是否激活该 Skill                   │
├─────────────────────────────────────────────────────┤
│  第二级：SKILL.md 正文                               │
│  → 仅当 Claude 认为该 Skill 与当前任务相关时加载      │
│  → 包含完整指令和引导（建议 < 500 行）               │
├─────────────────────────────────────────────────────┤
│  第三级：关联文件（scripts/ references/ assets/）     │
│  → 按需加载，Claude 自行决定何时读取                  │
│  → 无大小限制                                       │
└─────────────────────────────────────────────────────┘
```

### 2.2 可组合性（Composability）

你的 Skill 可能与其他 Skill 同时被加载，因此：
- **不要**假设你的 Skill 是唯一生效的
- **要**设计成能与其他 Skill 协同工作

### 2.3 跨平台性（Portability）

一个 Skill 在 Claude.ai、Claude Code 和 API 上**通用**，无需修改（前提是环境支持所需依赖）。

---

## 3. 动手前的规划

> [!IMPORTANT]
> **不要**上来就写代码。先想清楚 2-3 个具体的使用场景！

### 3.1 定义使用场景

用以下模板梳理你的需求：

```
使用场景：[场景名称]
触发条件：用户说"____"或"____"
步骤：
  1. [第一步做什么]
  2. [第二步做什么]
  3. [第三步做什么]
预期结果：[用户获得的最终产出]
```

**示例：**

```
使用场景：Sprint 冲刺规划
触发条件：用户说"帮我规划冲刺"或"创建 sprint 任务"
步骤：
  1. 通过 MCP 获取当前项目状态
  2. 分析团队速度和产能
  3. 建议任务优先级
  4. 在项目管理工具中创建任务
预期结果：完整的冲刺计划，任务已创建
```

### 3.2 三种常见场景类型

| 类型 | 说明 | 示例 |
|------|------|------|
| **文档/资产创建** | 创建一致、高质量的输出物 | 前端设计、DOCX/PPTX 生成 |
| **工作流自动化** | 多步骤流程，需要一致的方法论 | skill-creator 本身 |
| **MCP 增强** | 为 MCP 工具添加领域知识和最佳实践 | Sentry 代码审查 |

### 3.3 定义"成功"

| 指标类型 | 目标 | 衡量方式 |
|----------|------|----------|
| Skill 触发率 | 90% 相关查询能自动触发 | 跑 10-20 个测试查询 |
| 工作流调用次数 | 在 X 次调用内完成 | 对比有/无 Skill 的情况 |
| API 失败率 | 每次工作流 0 失败 | 监控 MCP 日志 |
| 用户干预频次 | 用户无需主动引导下一步 | 观察测试中的纠正次数 |

---

## 4. 技术规范详解

### 4.1 文件结构

```
your-skill-name/           ← 必须用 kebab-case 命名
├── SKILL.md               ← 必须（核心指令文件）
├── scripts/               ← 可选（可执行脚本）
│   ├── process_data.py
│   └── validate.sh
├── references/            ← 可选（参考文档，按需加载）
│   ├── api-guide.md
│   └── examples/
└── assets/                ← 可选（模板、字体、图标）
    └── report-template.md
```

### 4.2 命名规则

| 规则 | 正确 ✅ | 错误 ❌ |
|------|---------|---------|
| 文件夹用 kebab-case | `notion-project-setup` | `Notion Project Setup` |
| 无空格 | `my-skill` | `my skill` |
| 无下划线 | `data-analysis` | `data_analysis` |
| 无大写 | `api-helper` | `ApiHelper` |
| SKILL.md 精确命名 | `SKILL.md` | `SKILL.MD` / `skill.md` |

> [!CAUTION]
> **不要**在 Skill 文件夹内放 `README.md`！所有文档放在 `SKILL.md` 或 `references/` 中。

### 4.3 YAML Frontmatter 规范

**最小必需格式：**

```yaml
---
name: your-skill-name
description: What it does. Use when user asks to [specific phrases].
---
```

**完整可选字段：**

```yaml
---
name: your-skill-name
description: 做什么 + 什么时候用。包含用户会说的具体短语。
license: MIT                    # 可选：开源许可证
compatibility: 需要 Python 3.9+  # 可选：环境要求
allowed-tools: "Bash(python:*) Bash(npm:*) WebFetch"  # 可选：限制工具
metadata:                       # 可选：自定义元数据
  author: Your Name
  version: 1.0.0
  mcp-server: server-name
  category: productivity
  tags: [automation, workflow]
---
```

**字段说明：**

| 字段 | 必需 | 限制 |
|------|------|------|
| `name` | ✅ | kebab-case，不能含空格或大写 |
| `description` | ✅ | ≤ 1024 字符，不能含 XML 标签（`<>`） |
| `license` | ❌ | 如 MIT、Apache-2.0 |
| `compatibility` | ❌ | 1-500 字符 |
| `metadata` | ❌ | 任意 YAML 键值对 |

> [!WARNING]
> **安全限制：**
> - ❌ 不能在 frontmatter 中使用 XML 尖括号（`<` `>`）
> - ❌ 不能用 "claude" 或 "anthropic" 作为 Skill 名称前缀（保留名）

---

## 5. 一步步写出你的 SKILL.md

### 5.1 编写高效的 description（最关键！）

`description` 是 Claude **决定是否加载你的 Skill** 的唯一依据。它需要回答两个问题：

1. **WHAT** — 这个 Skill 做什么
2. **WHEN** — 什么时候应该触发

**✅ 好的写法：**

```yaml
description: >
  分析 Figma 设计文件并生成开发交接文档。当用户上传 .fig 文件、
  要求"设计规范"、"组件文档"或"设计转代码交接"时使用。
```

```yaml
description: >
  管理 Linear 项目工作流，包括冲刺规划、任务创建和状态跟踪。
  当用户提到"sprint"、"Linear 任务"、"项目规划"或要求"创建工单"时使用。
```

```yaml
description: >
  端到端客户入职工作流。处理账号创建、支付设置和订阅管理。
  当用户说"新客户入职"、"设置订阅"或"创建 PayFlow 账户"时使用。
```

**❌ 坏的写法：**

```yaml
# 太笼统 — Claude 不知道什么时候该用
description: Helps with projects.

# 没有触发条件 — Claude 永远不会主动加载
description: Creates sophisticated multi-page documentation systems.

# 太技术化 — 没有用户视角
description: Implements the Project entity model with hierarchical relationships.
```

> [!TIP]
> **高可用窍门：** Claude 目前有"触发不足"的倾向，所以建议 description 写得稍微"主动"一点。例如加上 "Make sure to use this skill whenever the user mentions dashboards, data visualization..."。

### 5.2 SKILL.md 正文推荐结构

```markdown
---
name: your-skill
description: [见上方写法]
---

# 你的 Skill 名称

## 指令

### 步骤 1：[第一个关键步骤]
清晰说明做什么。

```bash
python scripts/fetch_data.py --project-id PROJECT_ID
```
预期输出：[描述成功的样子]

### 步骤 2：[第二个关键步骤]
...

## 示例

### 示例 1：[常见场景]
用户说："帮我创建一个新的营销活动"
操作：
1. 通过 MCP 获取已有活动列表
2. 用给定参数创建新活动
结果：活动已创建，附带确认链接

## 故障排除

### 错误：[常见错误信息]
原因：[为什么会发生]
解决方案：[怎么修复]
```

---

## 6. 高可用指令编写技巧

### 6.1 指令要具体可执行

```markdown
# ✅ 好的
运行 `python scripts/validate.py --input {filename}` 检查数据格式。
如果验证失败，常见问题包括：
- 缺少必填字段（添加到 CSV 中）
- 日期格式无效（使用 YYYY-MM-DD）

# ❌ 坏的
在继续之前验证数据。
```

### 6.2 包含错误处理

```markdown
## 常见问题

### MCP 连接失败
如果看到 "Connection refused"：
1. 确认 MCP 服务器正在运行：检查 Settings > Extensions
2. 确认 API Key 有效
3. 尝试重新连接：Settings > Extensions > [你的服务] > Reconnect
```

### 6.3 清晰引用打包的资源

```markdown
编写查询前，请参考 `references/api-patterns.md`，了解：
- 限速指南
- 分页模式
- 错误码及处理方式
```

### 6.4 善用渐进式加载

```markdown
# SKILL.md 中只写核心指令
# 详细文档放到 references/ 并指引 Claude

如需了解完整 API 参数，请读取 `references/full-api-docs.md`。
```

### 6.5 解释"为什么"而非堆砌"必须"

```markdown
# ❌ 不推荐
MUST ALWAYS validate the project name. NEVER skip this step.

# ✅ 推荐
在创建项目前验证项目名，因为空名称会导致下游所有任务失败，
而且一旦创建就无法更改名称，返工成本很高。
```

### 6.6 用脚本替代模糊指令

对关键验证，优先打包确定性脚本，而非靠语言描述：

```markdown
# 推荐做法：
运行 `python scripts/pre_check.py` 进行创建前校验。
该脚本会检查：项目名非空、至少分配一名成员、开始日期不在过去。
```

> **代码是确定性的，语言解释不是。**

### 6.7 控制 SKILL.md 体积

| 准则 | 建议 |
|------|------|
| SKILL.md 行数 | < 500 行 |
| SKILL.md 词数 | < 5,000 词 |
| 详细文档 | 移到 `references/` |
| 大文件(>300行) | 加目录索引 |

---

## 7. 五大工作流模式

根据 Anthropic 总结的最佳实践，常用 Skill 可归类为以下五种模式：

### 模式 1：顺序工作流编排

**适用场景：** 用户需要按特定顺序执行的多步骤流程。

```markdown
## 工作流：新客户入职

### 步骤 1：创建账号
调用 MCP 工具：`create_customer`
参数：name, email, company

### 步骤 2：设置支付
调用 MCP 工具：`setup_payment_method`
等待：支付方式验证通过

### 步骤 3：创建订阅
调用 MCP 工具：`create_subscription`
参数：plan_id, customer_id（来自步骤 1）

### 步骤 4：发送欢迎邮件
调用 MCP 工具：`send_email`
模板：welcome_email_template
```

**关键技巧：** 明确步骤顺序 → 步骤间依赖 → 每步校验 → 失败回滚指令

---

### 模式 2：多 MCP 协调

**适用场景：** 工作流跨多个服务。

```markdown
## 阶段 1：设计导出（Figma MCP）
1. 从 Figma 导出设计资源
2. 生成设计规格

## 阶段 2：资源存储（Drive MCP）
1. 在 Drive 创建项目文件夹
2. 上传所有资源

## 阶段 3：任务创建（Linear MCP）
1. 创建开发任务
2. 将资源链接附加到任务

## 阶段 4：通知（Slack MCP）
1. 在 #engineering 频道发布交接摘要
```

**关键技巧：** 清晰的阶段划分 → 阶段间数据传递 → 进入下一阶段前校验 → 集中错误处理

---

### 模式 3：迭代优化

**适用场景：** 输出质量通过迭代提升。

```markdown
## 初稿
1. 通过 MCP 获取数据
2. 生成初稿报告

## 质量检查
1. 运行校验脚本：`scripts/check_report.py`
2. 识别问题（缺失章节、格式不一致、数据错误）

## 优化循环
1. 逐一解决识别的问题
2. 重新生成受影响的章节
3. 重新校验
4. 重复直到达到质量标准

## 定稿
1. 应用最终格式
2. 生成摘要
```

**关键技巧：** 明确质量标准 → 迭代改进 → 校验脚本 → 知道何时停止

---

### 模式 4：上下文感知的工具选择

**适用场景：** 同一目标，根据上下文选择不同工具。

```markdown
## 决策树
1. 检查文件类型和大小
2. 确定最佳存储位置：
   - 大文件 (>10MB)：使用云存储 MCP
   - 协作文档：使用 Notion/Docs MCP
   - 代码文件：使用 GitHub MCP
   - 临时文件：使用本地存储

## 执行存储
根据决策调用相应 MCP 工具 + 应用元数据 + 生成访问链接

## 向用户解释
说明为什么选择了该存储方式
```

---

### 模式 5：领域专业智能

**适用场景：** Skill 需要超越工具访问，嵌入领域知识。

```markdown
## 处理前（合规检查）
1. 通过 MCP 获取交易详情
2. 应用合规规则：制裁名单检查、辖区允许性验证、风险等级评估
3. 记录合规决策

## 处理
IF 合规通过：处理交易
ELSE：标记为需要审查，创建合规案例

## 审计追踪
记录所有合规检查 → 记录处理决策 → 生成审计报告
```

---

## 8. 测试与迭代

### 8.1 三种测试级别

| 级别 | 方式 | 适用场景 |
|------|------|----------|
| **手动测试** | 在 Claude.ai 直接提问观察 | 快速迭代，无需配置 |
| **脚本测试** | 在 Claude Code 中自动化 | 跨版本可重复验证 |
| **编程化测试** | 通过 Skills API 构建评估套件 | 大规模系统化测试 |

### 8.2 三类测试用例

#### ① 触发测试

确保 Skill 在正确的时机加载。

```
应该触发：
- "帮我在 ProjectHub 建一个新工作区"
- "我需要在 ProjectHub 创建一个项目"
- "为 Q4 规划初始化一个 ProjectHub 项目"

不应该触发：
- "今天旧金山天气怎么样？"
- "帮我写 Python 代码"
- "创建一个电子表格"
```

#### ② 功能测试

验证 Skill 产出正确的结果。

```
测试：创建包含 5 个任务的项目
条件：项目名 "Q4 Planning"，5 个任务描述
执行：Skill 执行工作流
预期：
  - 项目在 ProjectHub 中已创建
  - 5 个任务已创建且属性正确
  - 所有任务关联到项目
  - 无 API 错误
```

#### ③ 性能对比

证明 Skill 相比基线有提升。

```
无 Skill：                    有 Skill：
- 用户每次提供指令             - 自动执行工作流
- 15 轮对话                   - 仅 2 个澄清问题
- 3 次 API 调用失败            - 0 次 API 调用失败
- 12,000 tokens               - 6,000 tokens
```

### 8.3 用 skill-creator 加速

```
"Use the skill-creator skill to help me build a skill for [你的使用场景]"
```

skill-creator 能帮你：
- ✅ 从自然语言描述生成 Skill
- ✅ 生成格式正确的 SKILL.md
- ✅ 建议触发短语和结构
- ✅ 标记常见问题（模糊描述、缺少触发条件等）
- ✅ 按用途建议测试用例

### 8.4 迭代信号

| 信号 | 问题 | 解决方案 |
|------|------|----------|
| 用户手动启用 Skill | 触发不足 | 在 description 中加更多关键词和触发短语 |
| Skill 对不相关查询也加载 | 触发过度 | 添加负面触发条件，更具体化 |
| 结果不一致 | 执行问题 | 改进指令，添加错误处理 |

---

## 9. 分发与共享

### 9.1 安装方式

**Claude.ai 用户：** Settings > Capabilities > Skills > Upload skill（上传压缩的文件夹）

**Claude Code 用户：** 放到 Skill 目录即可

**组织级部署：** 管理员可以全工作区部署 Skill，支持自动更新和集中管理。

**通过 Skills CLI 安装：**
```bash
# 搜索技能
npx skills find [关键词]

# 安装技能
npx skills add <owner/repo@skill> -y
```

### 9.2 通过 GitHub 分发

1. 创建公开仓库，附带清晰的 README 和安装说明
2. 在 MCP 文档中链接到 Skill
3. 提供快速入门指南

### 9.3 通过 API 使用

```
/v1/skills 端点管理 Skill
Messages API 的 container.skills 参数添加 Skill
配合 Claude Agent SDK 构建自定义 Agent
```

---

## 10. 故障排除速查表

### Skill 上传失败

| 错误信息 | 原因 | 解决 |
|----------|------|------|
| `Could not find SKILL.md` | 文件名不对 | 重命名为 `SKILL.md`（区分大小写） |
| `Invalid frontmatter` | YAML 格式问题 | 确保有 `---` 分隔符，引号闭合 |
| `Invalid skill name` | 名称有空格或大写 | 改为 kebab-case：`my-cool-skill` |

### Skill 不触发

```
排查清单：
□ description 是否太笼统？（"Helps with projects" 不行）
□ 是否包含用户实际会说的触发短语？
□ 是否提到了相关文件类型？

调试技巧：问 Claude "你什么时候会使用 [Skill名] 技能？"
Claude 会引用 description 作答，据此调整。
```

### Skill 触发过频

```yaml
# 方案 1：添加负面触发
description: >
  CSV 文件的高级数据分析。用于统计建模、回归、聚类。
  不要用于简单的数据探索（请使用 data-viz 技能）。

# 方案 2：更具体
description: 处理 PDF 法律文档以进行合同审查

# 方案 3：明确范围
description: >
  PayFlow 电商支付处理。专门用于在线支付工作流，
  不用于一般的财务查询。
```

### 指令未被遵循

| 原因 | 解决 |
|------|------|
| 指令太冗长 | 用要点和编号列表，详情移到 `references/` |
| 关键指令被埋没 | 关键内容放最前面，用 `## Critical` 标题 |
| 语言含糊 | 用具体的检查清单替代模糊描述 |
| 模型"偷懒" | 加 "Take your time" / "Quality > Speed" 提示 |

### 上下文过大

```
症状：Skill 响应变慢或质量下降
解决：
1. 优化 SKILL.md 体积（< 5,000 词）
2. 将详细文档移到 references/
3. 减少同时启用的 Skill 数量（评估是否超过 20-50 个）
```

---

## 11. 完整实战案例

下面用一个「**Git 提交信息规范化**」Skill 演示完整开发流程。

### Step 1：创建文件夹

```bash
mkdir -p git-commit-style/scripts
mkdir -p git-commit-style/references
```

### Step 2：编写 SKILL.md

```markdown
---
name: git-commit-style
description: >
  自动生成符合 Conventional Commits 规范的 Git 提交信息。
  当用户要求"写提交信息"、"格式化 commit"、"生成 commit message"、
  提交代码变更、或进行 git commit 操作时使用此技能。
metadata:
  author: YourName
  version: 1.0.0
  category: developer-tools
  tags: [git, commit, convention]
---

# Git Commit Style

为代码变更生成规范的 Conventional Commits 格式提交信息。

## 指令

### 步骤 1：分析变更
查看 `git diff --staged` 的输出，理解变更内容。

### 步骤 2：确定类型
根据变更选择合适的 Commit 类型：

| 类型 | 描述 |
|------|------|
| feat | 新功能 |
| fix | 修复 Bug |
| docs | 仅文档变更 |
| style | 不影响逻辑的格式调整 |
| refactor | 既非修复也非新功能的代码重构 |
| perf | 性能优化 |
| test | 添加或修改测试 |
| chore | 构建过程或辅助工具的变动 |

### 步骤 3：生成提交信息
格式：`<type>(<scope>): <subject>`

规则：
- subject 不超过 50 个字符
- 使用祈使语气（add, fix, update，而非 added, fixed）
- 不以句号结尾
- scope 可选，表示影响范围

如果变更复杂，添加 body 说明：
- 空一行后写 body
- body 每行不超过 72 字符
- 解释"为什么"而非"是什么"

## 示例

**示例 1：**
输入：添加了 JWT 用户认证
输出：`feat(auth): implement JWT-based authentication`

**示例 2：**
输入：修复了登录页面密码校验不生效的问题
输出：`fix(login): validate password field before submission`

**示例 3：**
输入：更新了 README 的安装说明
输出：`docs(readme): update installation instructions`

## 故障排除

### 无法确定变更类型
如果一次提交包含多种类型的变更，建议用户拆分为多次提交。
每次提交应只做一件事。

### Scope 不确定
参考项目目录结构确定影响范围。如果变更跨多个模块，
可省略 scope 或使用最主要的模块名。
```

### Step 3：添加参考文档（可选）

```bash
# 创建 references/conventional-commits-spec.md
# 将完整的 Conventional Commits 规范放入其中
```

### Step 4：测试

```
测试查询：
✅ "帮我写一个提交信息，我刚修复了购物车数量计算的 bug"
✅ "format my commit message for adding dark mode"
✅ "我改了三个文件，帮我生成 commit"
❌ "帮我写一段 Python 代码"（不应触发）
❌ "git 是什么？"（不应触发）
```

### Step 5：安装使用

```bash
# 方法 A：直接放到项目技能目录
cp -r git-commit-style/ .agents/skills/

# 方法 B：通过 Claude.ai 上传
zip -r git-commit-style.zip git-commit-style/
# 然后在 Claude.ai > Settings > Skills 上传
```

---

## 12. Quality Checklist — 上线前检查清单

### 开始前

- [ ] 确定了 2-3 个具体使用场景
- [ ] 确定了需要的工具（内置或 MCP）
- [ ] 已参考现有示例 Skill
- [ ] 规划了文件夹结构

### 开发中

- [ ] 文件夹使用 kebab-case 命名
- [ ] `SKILL.md` 文件存在（精确拼写）
- [ ] YAML frontmatter 有 `---` 分隔符
- [ ] `name` 字段：kebab-case，无空格，无大写
- [ ] `description` 包含 WHAT 和 WHEN
- [ ] 无 XML 标签（`<` `>`）
- [ ] 指令清晰可执行
- [ ] 包含错误处理
- [ ] 提供了示例
- [ ] 引用关联文件清晰

### 上传前

- [ ] 测试了明显任务的触发
- [ ] 测试了换种说法的触发
- [ ] 验证了不相关话题不会触发
- [ ] 功能测试通过
- [ ] 工具集成正常（如适用）

### 上线后

- [ ] 在真实对话中测试
- [ ] 监控触发不足/过度情况
- [ ] 收集用户反馈
- [ ] 迭代优化 description 和指令
- [ ] 更新 metadata 中的版本号

---

## 📚 参考资源

| 资源 | 链接 |
|------|------|
| 官方最佳实践指南 | Anthropic Best Practices Guide |
| Skills 文档 | Anthropic Skills Documentation |
| API 参考 | Anthropic API Reference |
| MCP 文档 | MCP Documentation |
| 示例 Skills 仓库 | [github.com/anthropics/skills](https://github.com/anthropics/skills) |
| 技能市场 | [skills.sh](https://skills.sh/) |
| 社区支持 | Claude Developers Discord |
| Bug 报告 | [anthropics/skills/issues](https://github.com/anthropics/skills/issues) |

---

> [!NOTE]
> 本教程基于 Anthropic 官方发布的《The Complete Guide to Building Skills for Claude》编写。
> 随着 Skills 生态的发展，部分内容可能会更新，请以官方文档为准。
