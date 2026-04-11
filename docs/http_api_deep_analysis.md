# FlashMemory HTTP 模块接口深度分析

> **分析日期**: 2026-03-19 (Updated: 2026-04-11 for Zvec integration)  
> **源文件**: `cmd/app/fm_http.go` (1796行)  
> **框架**: Echo v4 (Go)  
> **默认端口**: 5532 (可通过 `FM_PORT` 环境变量配置)  
> **向量引擎**: Faiss (legacy) / Zvec (recommended, v0.4.0+)

---

## 一、架构总览

### 1.1 分层架构

```
┌─────────────────────────────────────────────────────────┐
│                    HTTP 层 (Echo v4)                     │
│  路由注册 + 认证中间件 (Bearer Token / Basic Auth)        │
├─────────────────────────────────────────────────────────┤
│                    接口层 (Handler)                       │
│  fm_http.go 中定义的 22 个 Handler 函数                   │
├─────────────────────────────────────────────────────────┤
│                    服务层 (Service)                       │
│  internal/back    - 索引管理核心编排                      │
│  internal/search  - 搜索引擎                             │
│  internal/ranking - 函数重要性评分                        │
│  internal/module_analyzer - 异步模块分析                  │
│  internal/embedding - 向量嵌入                           │
│  internal/graph   - 知识图谱                             │
│  internal/parser  - 多语言代码解析                        │
│  config           - 配置管理                             │
├─────────────────────────────────────────────────────────┤
│                    数据层 (Data)                          │
│  SQLite (code_index.db) - 函数/模块/分支索引              │
│  向量索引 (两种模式):                                     │
│    ├─ Zvec (推荐): 进程内HNSW, 支持混合搜索/标量过滤       │
│    └─ Faiss (兼容): HTTP/内存两种模式                     │
│  文件系统 - graph.json / module_graphs/ / exclude.json   │
│  YAML 配置文件 - fm.yaml                                 │
└─────────────────────────────────────────────────────────┘
```

### 1.2 路由组织

| 路由组 | 前缀 | 中间件 | 用途 |
|--------|------|--------|------|
| `api` | `/api` | 认证中间件 | 核心 API 接口 (20个) |
| `c` | `/c` | 认证中间件 | 配置管理接口 (2个) |

### 1.3 认证机制

认证优先级：**Bearer Token > Basic Auth > 无认证（仍提取Token）**

- 环境变量 `API_TOKEN` 或配置文件 `api_token` → Bearer Token 认证
- 环境变量 `API_USER` + `API_PASS` → Basic Auth 认证
- 均未配置 → 无认证，但仍尝试提取请求中的 Bearer Token 注入公共管理器

### 1.4 统一响应格式

```json
{
  "code": 0,        // 0=成功, 1=参数/业务错误, 2=服务端错误, 5=LLM错误
  "message": "OK",
  "data": {}         // 可选
}
```

---

## 二、接口深度分析（按功能域分组）

---

### 🔍 2.1 搜索域

#### 2.1.1 代码搜索 — `POST /api/search`

**Handler**: `searchHandler()`

| 层级 | 调用链路 | 说明 |
|------|---------|------|
| **接口层** | `searchHandler()` | 解析请求，构造 Indexer + SearchEngine，执行查询 |
| **服务层** | `search.SearchEngine.Query()` | 根据 search_mode 分发到不同搜索策略 |
| ↳ | `semanticSearch()` | 语义搜索：向量化查询→Faiss相似度搜索 |
| ↳ | `keywordSearch()` | 关键词搜索：SQL LIKE 模糊匹配 |
| ↳ | `hybridSearch()` | 混合搜索：语义+关键词结果融合排序 |
| ↳ | `embedding.EnsureEmbeddingsBatch()` | Faiss索引不存在时自动生成嵌入向量 |
| ↳ | `embedding.EnsureCodeDescEmbeddingsBatch()` | 模块描述向量初始化 |
| **数据层** | `index.EnsureIndexDB()` → SQLite `code_index.db` | 函数索引数据库 |
| ↳ | `index.FaissWrapper` (HTTP/Memory) | Faiss 向量索引 (`code_index.local` / `code_index.faiss`) |
| ↳ | `module.faiss` | 模块描述向量索引 |

**请求参数**:

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `project_dir` | string | ✅ | - | 项目目录绝对路径 |
| `query` | string | ✅ | - | 搜索关键词 |
| `search_mode` | string | ❌ | `hybrid` | `semantic` / `keyword` / `hybrid` |
| `limit` | int | ❌ | `5` | 结果数量限制 |
| `faiss` | bool | ❌ | `false` | 是否使用 Faiss 远程索引 |

**响应数据**: 包含 `func_res`（合并结果）、`tags`（关键词标签）、`funcs`（纯函数结果）、`modules`（模块结果），按 Score 降序排序。

**核心流程**:
1. 打开 `.gitgo/code_index.db` 数据库
2. 创建函数级 FaissWrapper（128维），若 `.faiss`/`.local` 文件不存在则自动生成
3. 加载 Faiss 索引，执行函数搜索
4. 创建模块级 FaissWrapper，加载 `module.faiss`，执行模块搜索
5. 合并函数+模块结果，按 Score 降序排序返回

---

#### 2.1.2 函数信息查询 — `POST /api/function-info`

**Handler**: `getFunctionInfoHandler()`

| 层级 | 调用链路 | 说明 |
|------|---------|------|
| **接口层** | `getFunctionInfoHandler()` | 解析请求，读取 graph.json，过滤匹配函数 |
| **服务层** | 无独立服务层 | 直接在 Handler 内处理 |
| **数据层** | 文件系统读取 `.gitgo/graph.json` | 知识图谱 JSON 文件 |

**请求参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `project_dir` | string | ✅ | 项目目录 |
| `file_path` | string | ✅ | 文件相对路径（模糊匹配） |
| `package` | string | ❌ | 包名过滤 |
| `function_name` | string | ❌ | 函数名过滤 |

**响应数据**: 匹配的函数详情列表，包含 name, package, file, imports, calls, start_line, end_line, lines, function_type, description, fan_in, fan_out, complexity, depth, score。

---

### 📦 2.2 索引管理域

#### 2.2.1 构建索引 — `POST /api/index`

**Handler**: `buildIndexHandler()`

| 层级 | 调用链路 | 说明 |
|------|---------|------|
| **接口层** | `buildIndexHandler()` | 解析请求，处理排除项，调用构建流程 |
| **服务层** | `back.MakeExcludeFile()` | 生成排除文件 |
| ↳ | `back.RefreshFaiss()` | 清理旧的 Faiss 索引缓存 |
| ↳ | `back.BuildIndex()` | **核心**: 初始化 FaissManager，执行索引构建 |
| ↳ | `back.indexCodeWithManager()` | 内部索引逻辑（支持增量/全量） |
| ↳ | `parser.WalkAndParse()` | 遍历文件，解析函数 |
| ↳ | `analyzer.AnalyzeBatch()` | LLM 批量分析 |
| ↳ | `graph.BuildGraph()` | 构建知识图谱 |
| ↳ | `embedding.EnsureEmbeddingsBatch()` | 生成嵌入向量 |
| ↳ | `visualize.SaveD3JSON()` | 导出图谱可视化 |
| **数据层** | SQLite `code_index.db` (functions/calls/externals/branch_index 表) |
| ↳ | Faiss 向量索引文件 |
| ↳ | `.gitgo/graph.json` 知识图谱 |
| ↳ | `.gitgo/info.json` 构建历史 |

**请求参数**:

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `project_dir` | string | ✅ | - | 项目目录 |
| `relative_dir` | string | ❌ | `""` (全量) | 子目录路径 |
| `Faiss` | bool | ❌ | `false` | 使用 Faiss 远程索引 |
| `exclude` | string[] | ❌ | `[]` | glob 排除模式 |

**核心流程**:
1. 自动生成 `.gitignore`（忽略 `.gitgo/` 和 `*.zip`）
2. 清理旧 Faiss 索引
3. 初始化 FaissManager（本地/HTTP两种模式）
4. 根据 Git 信息判断增量/全量模式
5. 遍历代码文件 → 解析函数 → LLM 分析 → 存入 DB → 构建图谱 → 生成向量

---

#### 2.2.2 删除索引 — `DELETE /api/index`

**Handler**: `deleteIndexHandler()`

| 层级 | 调用链路 | 说明 |
|------|---------|------|
| **接口层** | `deleteIndexHandler()` | 解析请求 |
| **服务层** | `back.DeleteIndex()` | 删除 DB 记录 + 重置索引文件 |
| **数据层** | `DELETE FROM functions/calls/externals` | 清空索引表 |
| ↳ | `back.ResetIndex()` | 删除 `.faiss`/`.local`/`graph.json` 等文件 |

---

#### 2.2.3 删除部分索引 — `DELETE /api/index/some`

**Handler**: `deleteSomeIndexHandler()`

| 层级 | 调用链路 | 说明 |
|------|---------|------|
| **接口层** | `deleteSomeIndexHandler()` | 解析请求，校验 relative_dir |
| **服务层** | `back.DeleteSomeIndex()` | 按子路径删除索引记录 |
| **数据层** | `DELETE FROM functions WHERE file LIKE ?` / `file = ?` | 目录用 LIKE，文件用精确匹配 |

**两个必填参数**: `project_dir` + `relative_dir`

---

#### 2.2.4 重置索引 — `DELETE /api/index/reset`

**Handler**: `resetIndexHandler()`

| 层级 | 调用链路 | 说明 |
|------|---------|------|
| **接口层** | `resetIndexHandler()` | 解析请求 |
| **服务层** | `back.ResetIndex()` | 重置索引（删除所有索引文件，保留 DB 结构） |
| **数据层** | 删除文件: `code_index.faiss`, `code_index.local`, `graph.json`, `indexing.temp`, `module.faiss` 等 |
| ↳ | HTTP 调用 Faiss 服务 `/delete_index` 清除缓存 |

---

#### 2.2.5 刷新 Faiss 索引 — `POST /api/index/refresh`

**Handler**: `refreshFaissHandler()`

| 层级 | 调用链路 | 说明 |
|------|---------|------|
| **接口层** | `refreshFaissHandler()` | 解析请求 |
| **服务层** | `back.RefreshFaiss()` | 清除 Faiss 缓存和索引文件 |
| **数据层** | 删除 `.faiss`/`.local`/`module.faiss` 文件 + HTTP 调用 Faiss 服务清除缓存 |

---

#### 2.2.6 检查索引 — `POST /api/index/check`

**Handler**: `checkIndexHandler()`

| 层级 | 调用链路 | 说明 |
|------|---------|------|
| **接口层** | `checkIndexHandler()` | 直接查询 DB，无独立服务层 |
| **服务层** | 无（Handler 内直接操作） | — |
| **数据层** | SQLite 查询 `functions` 表（按文件分组） + `code_desc` 表（模块信息） |
| ↳ | `filepath.Walk()` 遍历统计实际文件数 |

**请求参数**: `project_dir` (必填) + `relative_dir` (可选，支持目录/文件路径)

**响应数据**: `total_function_count`, `total_file_count`, `real_file_count`, `functions` (按文件分组的函数列表), `modules` (按路径分组的模块列表)

**SQL 查询逻辑**:
- 有 `relative_dir` → LIKE 模糊匹配（目录加 `/%`，文件精确匹配）
- 无 `relative_dir` → 查询全部 + 只返回根模块

---

#### 2.2.7 增量索引更新 — `POST /api/index/incremental`

**Handler**: `incrementalIndexHandler()`

| 层级 | 调用链路 | 说明 |
|------|---------|------|
| **接口层** | `incrementalIndexHandler()` | 解析请求 |
| **服务层** | `back.IncrementalUpdate()` | 初始化 FaissManager → 增量索引 |
| ↳ | `back.indexCodeWithManager()` | **Git diff** 检测变更文件 → 仅处理变更 |
| **数据层** | Git 操作获取变更文件列表 → 删除旧记录 → 重新索引 |

**请求参数**: `project_dir` (必填), `branch`, `commit`, `faiss` (均可选)

**增量策略**: 
1. 通过 `branch_index` 表获取上次索引的 commit
2. 通过 `git diff` 获取两次 commit 间的变更文件
3. 删除变更文件的旧索引记录，仅重新索引变更文件

---

### 📊 2.3 图谱域

#### 2.3.1 列出图谱 — `POST /api/listGraph`

**Handler**: `listGraphHandler()`

| 层级 | 调用链路 | 说明 |
|------|---------|------|
| **接口层** | `listGraphHandler()` | 解析请求，自动创建 .gitignore |
| **服务层** | `back.ListGraph()` | 构建/更新代码图谱 |
| ↳ | `parser.WalkAndParse()` | 遍历解析代码 |
| ↳ | `analyzer.AnalyzeBatch()` | LLM 分析 |
| ↳ | `graph.BuildGraph()` → `SaveGraphJSON()` | 构建并保存知识图谱 |
| ↳ | `visualize.SaveD3JSON()` | 导出 D3 可视化数据 |
| **数据层** | `.gitgo/graph.json` + SQLite + Faiss |

---

#### 2.3.2 获取模块图谱 — `POST /api/module-graphs`

**Handler**: `getModuleGraphsHandler()`

| 层级 | 调用链路 | 说明 |
|------|---------|------|
| **接口层** | `getModuleGraphsHandler()` | 读取图谱 JSON 文件 |
| **服务层** | 无（直接文件读取） | — |
| **数据层** | `.gitgo/module_graphs/` 目录下四种图谱文件 |

**四种图谱类型**:

| 类型 | 文件 | 用途 |
|------|------|------|
| `hierarchical` | `hierarchical_tree.json` | 树形层级结构 |
| `network` | `network_graph.json` | 节点+边的关系网络 |
| `sunburst` | `sunburst_chart.json` | 旭日图（环形嵌套） |
| `flat` | `flat_nodes.json` | 扁平化节点列表 |

**请求参数**: `project_dir` (必填), `graph_type` (可选，指定只返回某种类型)

---

#### 2.3.3 更新模块图谱 (异步) — `POST /api/module-graphs/update`

**Handler**: `updateModuleGraphsHandler()`

| 层级 | 调用链路 | 说明 |
|------|---------|------|
| **接口层** | `updateModuleGraphsHandler()` | 防重复提交检查，启动异步任务 |
| **服务层** | `module_analyzer.AnalyzeAllModules()` | **异步**分析所有模块 |
| ↳ | `ModuleAnalyzer.AnalyzeModules()` | 组织文件→构建目录树→生成描述 |
| ↳ | `organizeByFile()` | 按文件组织模块 |
| ↳ | `buildDirectoryTree()` | 构建目录树 |
| ↳ | `generateDescriptionsWithConcurrency()` | 并发调用 LLM 生成模块描述 |
| ↳ | `saveToFile()` | 保存四种图谱 JSON |
| ↳ | `batchInsertModules()` / `saveToDatabase()` | 存入 code_desc 表 |
| **数据层** | SQLite `code_desc` 表 + 四种图谱 JSON 文件 |
| ↳ | `.gitgo/graph.json` (读取知识图谱) |
| ↳ | LLM API 远程调用 |

**请求参数**: `project_dir` (必填), `skip_llm` (可选，跳过 LLM 描述生成)

**任务管理**: 通过 `module_analyzer` 包内的全局 task map 管理异步任务状态 (pending → running → completed/failed)

---

#### 2.3.4 模块分析任务状态 — `POST /api/module-graphs/status`

**Handler**: `moduleGraphsStatusHandler()`

| 层级 | 调用链路 | 说明 |
|------|---------|------|
| **接口层** | `moduleGraphsStatusHandler()` | 查询任务状态 |
| **服务层** | `module_analyzer.GetTask()` / `GetTaskByProjDir()` | 从内存 map 中查询 |
| **数据层** | 内存中的 `sync.Map` (任务注册表) |

**请求参数**: `task_id` 或 `project_dir` (至少一个)

**任务状态**: `pending` → `running` → `completed` / `failed`

---

#### 2.3.5 删除模块分析记录 — `POST /api/module-graphs/delete`

**Handler**: `deleteModuleDescHandler()`

| 层级 | 调用链路 | 说明 |
|------|---------|------|
| **服务层** | `back.DeleteModuleDesc()` | 清空 code_desc 表 + 重置图谱文件 |
| **数据层** | `DELETE FROM code_desc` + 删除 `module_graphs/` 目录和 `module_analyzer.temp` |

---

#### 2.3.6 重置模块分析记录 — `POST /api/module-graphs/reset`

**Handler**: `resetModuleDescHandler()`

| 层级 | 调用链路 | 说明 |
|------|---------|------|
| **服务层** | `back.ResetModuleDesc()` | 仅删除图谱目录和临时文件（不清 DB） |
| **数据层** | 删除 `module_graphs/` 目录 + `module_analyzer.temp` 文件 |

---

### 📋 2.4 函数管理域

#### 2.4.1 函数列表 — `POST /api/functions`

**Handler**: `listFunctionsHandler()`

| 层级 | 调用链路 | 说明 |
|------|---------|------|
| **接口层** | `listFunctionsHandler()` | 解析请求，scan 模式仅返回计数 |
| **服务层** | `parser.WalkAndParse()` | 遍历项目文件，解析所有函数 |
| **数据层** | SQLite `SELECT COUNT(*) FROM functions` (scan 模式) |
| ↳ | 文件系统遍历 + 多语言 AST 解析 (非 scan 模式) |

**请求参数**: `project_dir` (必填), `scan` (可选, `true` 仅返回索引计数)

**两种模式**:
- `scan=true`: 快速返回 DB 中已索引的函数数量
- `scan=false`: 遍历整个项目解析全部函数返回列表

---

#### 2.4.2 函数重要性评级 — `POST /api/ranking`

**Handler**: `functionRankingHandler()`

| 层级 | 调用链路 | 说明 |
|------|---------|------|
| **接口层** | `functionRankingHandler()` | 解析请求，读取 graph.json |
| **服务层** | `ranking.CalculateImportanceScores()` | 计算四维重要性评分 |
| ↳ | `FunctionRanker.buildCallGraph()` | 构建调用图 |
| ↳ | `FunctionRanker.calculateMetrics()` | 计算 FanIn/FanOut/Depth/Complexity |
| ↳ | `FunctionRanker.calculateScore()` | 加权评分 |
| **数据层** | 读取 `.gitgo/graph.json` → 计算评分 → 写回 `graph.json` |

**评分权重配置（默认均衡）**:

| 维度 | 权重字段 | 默认值 | 说明 |
|------|---------|--------|------|
| FanIn | Alpha | 0.4 | 被调用次数（越高越核心） |
| FanOut | Beta | 0.2 | 调用其他函数次数 |
| Depth | Gamma | 0.2 | 调用链深度 |
| Complexity | Delta | 0.2 | 代码复杂度 |

---

### 🚫 2.5 排除项管理域

#### 2.5.1 设置排除项 — `POST /api/exclude`

**Handler**: `excludeHandler()`

| 层级 | 调用链路 | 说明 |
|------|---------|------|
| **服务层** | `back.MakeExcludeFile()` | 写入排除配置 |
| **数据层** | 写入 `.gitgo/exclude.json` (JSON 数组) |

---

#### 2.5.2 读取排除项 — `POST /api/exclude/read`

**Handler**: `excludeReadHandler()`

| 层级 | 调用链路 | 说明 |
|------|---------|------|
| **服务层** | `utils.ReadJSONArrayFile()` | 读取 JSON 数组文件 |
| **数据层** | 读取 `.gitgo/exclude.json` |

---

### 🤖 2.6 LLM 分析域

#### 2.6.1 LLM 分析器 — `POST /api/llm/analyzer`

**Handler**: `llmAnalyzerHandler()`

| 层级 | 调用链路 | 说明 |
|------|---------|------|
| **接口层** | `llmAnalyzerHandler()` | **TODO: 待实现**，当前直接返回成功 |

---

### ❤️ 2.7 系统域

#### 2.7.1 健康检查 — `GET /api/health`

**Handler**: `healthCheckHandler()`

| 层级 | 调用链路 | 说明 |
|------|---------|------|
| **接口层** | 直接返回 `{"code": 0, "message": "OK"}` | 无服务层/数据层调用 |

---

### ⚙️ 2.8 配置管理域

#### 2.8.1 获取配置 — `GET /c/config`

| 层级 | 调用链路 | 说明 |
|------|---------|------|
| **接口层** | 匿名函数 → `cfgHandler.GetConfig()` | Echo → net/http 适配 |
| **服务层** | `config.Handler.GetConfig()` → `config.GetConfig()` | 读取 YAML 配置文件 |
| **数据层** | 读取 `fm.yaml` 配置文件 |

#### 2.8.2 更新配置 — `PUT /c/config`

| 层级 | 调用链路 | 说明 |
|------|---------|------|
| **接口层** | 匿名函数 → `cfgHandler.UpdateConfig()` | Echo → net/http 适配 |
| **服务层** | `config.Handler.UpdateConfig()` → `config.UpdateConfig()` | 深度合并更新 |
| **数据层** | 读取 YAML → JSON 解码 → 深度合并 → 写回 YAML |

---

## 三、数据层详解

### 3.1 SQLite 数据库 (`code_index.db`)

| 表名 | 用途 | 关键字段 |
|------|------|---------|
| `functions` | 函数索引 | name, package, file, start_line, end_line, description, embedding |
| `calls` | 函数调用关系 | caller, callee |
| `externals` | 外部依赖 | function_name, external_lib |
| `branch_index` | 分支索引信息 | branch_name, commit_hash, indexed_files |
| `code_desc` | 模块描述 | name, type, path, parent_path, function_count, file_count, description |

### 3.2 向量索引

#### 3.2.1 Zvec 向量索引 (v0.4.0+, 推荐)

| 路径 | 用途 | 维度 | 索引类型 |
|------|------|------|---------|
| `.gitgo/zvec_collections/functions/` | 函数级向量 (Dense+Sparse) | 384 | HNSW + Sparse |
| `.gitgo/zvec_collections/modules/` | 模块级向量 (Dense) | 384 | HNSW |

**ZvecWrapper** (通过 subprocess JSON-line 协议与 Python Bridge 通信):
- 支持 Dense + Sparse 混合搜索 (RRF fusion)
- 支持标量过滤 (language, package, file_path 等)
- 通过 `-engine zvec` CLI 参数启用

#### 3.2.2 Faiss 向量索引 (兼容模式)

| 文件 | 用途 | 维度 | 创建方式 |
|------|------|------|---------|
| `code_index.local` | 本地内存索引（MemoryFaissWrapper） | 128 | 本地计算 |
| `code_index.faiss` | 远程 HTTP Faiss 索引 | 128 | 通过 Faiss HTTP 服务 |
| `module.faiss` | 模块描述向量索引 | 128 | LLM 嵌入 |

**三种 Faiss Wrapper**:
- `MemoryFaissWrapper`: 纯内存实现，自动 L2 距离/余弦相似度
- `HTTPFaissWrapper`: 通过 HTTP 调用外部 Faiss 服务 (`DefaultFaissServerURL`)
- `ZvecWrapper`: 进程内 Zvec 引擎 (通过 subprocess bridge，实现 FaissWrapper 接口)

### 3.3 文件系统

| 路径 | 用途 |
|------|------|
| `.gitgo/graph.json` | 知识图谱（函数+调用关系+包+外部依赖） |
| `.gitgo/info.json` | 构建历史（git分支、commit、时间戳） |
| `.gitgo/exclude.json` | 排除模式列表 |
| `.gitgo/indexing.temp` | 索引进行中锁文件（防并发） |
| `.gitgo/module_analyzer.temp` | 模块分析进行中锁文件 |
| `.gitgo/module_graphs/` | 四种模块图谱 JSON 文件 |

---

## 四、接口一览表

| 序号 | 方法 | 路由 | 功能 | 所属域 |
|------|------|------|------|--------|
| 1 | GET | `/api/health` | 健康检查 | 系统 |
| 2 | POST | `/api/search` | 代码搜索 | 搜索 |
| 3 | POST | `/api/functions` | 函数列表 | 函数管理 |
| 4 | POST | `/api/index` | 构建索引 | 索引管理 |
| 5 | DELETE | `/api/index` | 删除索引 | 索引管理 |
| 6 | DELETE | `/api/index/some` | 删除部分索引 | 索引管理 |
| 7 | DELETE | `/api/index/reset` | 重置索引 | 索引管理 |
| 8 | POST | `/api/index/refresh` | 刷新Faiss索引 | 索引管理 |
| 9 | POST | `/api/index/check` | 检查索引 | 索引管理 |
| 10 | POST | `/api/index/incremental` | 增量索引更新 | 索引管理 |
| 11 | POST | `/api/listGraph` | 列出/构建图谱 | 图谱 |
| 12 | POST | `/api/module-graphs` | 获取模块图谱 | 图谱 |
| 13 | POST | `/api/module-graphs/update` | 更新模块图谱(异步) | 图谱 |
| 14 | POST | `/api/module-graphs/status` | 模块分析状态查询 | 图谱 |
| 15 | POST | `/api/module-graphs/delete` | 删除模块分析记录 | 图谱 |
| 16 | POST | `/api/module-graphs/reset` | 重置模块分析记录 | 图谱 |
| 17 | POST | `/api/exclude` | 设置排除项 | 排除项 |
| 18 | POST | `/api/exclude/read` | 读取排除项 | 排除项 |
| 19 | POST | `/api/llm/analyzer` | LLM分析器(待实现) | LLM分析 |
| 20 | POST | `/api/ranking` | 函数重要性评级 | 函数管理 |
| 21 | POST | `/api/function-info` | 获取函数详情 | 搜索 |
| 22 | GET | `/c/config` | 获取配置 | 配置管理 |
| 23 | PUT | `/c/config` | 更新配置 | 配置管理 |

---

## 五、依赖包关系

```
fm_http.go (接口层)
├── internal/back (服务编排层)
│   ├── internal/parser         - 多语言代码解析 (Go AST, Tree-sitter, Regex)
│   ├── internal/analyzer       - LLM 代码分析
│   ├── internal/graph          - 知识图谱构建
│   ├── internal/index          - 索引管理 (SQLite + Faiss)
│   ├── internal/embedding      - 嵌入向量生成
│   ├── internal/visualize      - D3 可视化导出
│   ├── internal/module_analyzer- 异步模块分析
│   └── internal/utils          - 工具函数 (git操作/文件操作/日志)
├── internal/search (搜索引擎)
│   ├── internal/index          - 向量搜索
│   └── internal/embedding      - 查询向量化
├── internal/ranking (函数评分)
│   └── internal/parser         - 函数信息结构
├── internal/module_analyzer (模块分析)
│   ├── internal/analyzer       - LLM 分析
│   └── config                  - 配置管理
├── config (配置管理)
│   └── YAML 文件读写
└── cmd/common (公共管理器)
    └── Token/URL/Model 全局管理
```

---

## 六、启动初始化流程

### 6.1 Faiss 模式（默认）

```
main()
  ├── config.Init()                    // 确定配置文件路径
  ├── config.LoadConfig()              // 加载 fm.yaml
  ├── 初始化参数 (Token/URL/Model)       // 环境变量优先，配置文件回退
  ├── back.InitFaiss()                 // 启动 Faiss Python 服务
  ├── back.NewFaissMonitor()           // 启动 Faiss 健康监控 (5秒轮询)
  ├── echo.New() + 中间件              // Recover + CORS + 认证
  ├── 注册路由 (/api/* + /c/*)
  └── e.Start(":5532")                // 启动 HTTP 服务
```

### 6.2 Zvec 模式（`-engine zvec`）

```
main()
  ├── config.Init()                    // 确定配置文件路径
  ├── config.LoadConfig()              // 加载 fm.yaml
  ├── 初始化参数 (Token/URL/Model)       // 环境变量优先，配置文件回退
  ├── NewFaissWrapperByEngine("zvec")  // 创建 ZvecWrapper (subprocess bridge)
  │   ├── 启动 zvec_bridge.py 子进程
  │   ├── 初始化 Zvec Collection (HNSW + Sparse)
  │   └── 跳过 Faiss HTTP 服务启动
  ├── echo.New() + 中间件              // Recover + CORS + 认证
  ├── 注册路由 (/api/* + /c/*)
  └── e.Start(":5532")                // 启动 HTTP 服务
```
