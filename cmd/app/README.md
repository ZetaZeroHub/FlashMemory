# FlashMemory HTTP API 接口文档

## 概述

FlashMemory HTTP API 提供了代码索引、搜索、分析和管理功能。服务默认运行在端口 5532 上。

## 认证

如果设置了环境变量 `API_USER` 和 `API_PASS`，则需要使用 Basic Auth 认证。

## 通用响应格式

所有接口都返回统一的 JSON 格式：

```json
{
  "code": 0,
  "message": "OK",
  "data": {}
}
```

- `code`: 状态码，0 表示成功，非 0 表示失败
- `message`: 响应消息
- `data`: 响应数据（可选）

## API 接口列表

### 1. 健康检查

**接口地址**: `GET /api/health`

**描述**: 检查服务健康状态

**请求参数**: 无

**响应示例**:

```json
{
  "code": 0,
  "message": "OK"
}
```

### 2. 代码搜索

**接口地址**: `POST /api/search`

**描述**: 在指定项目中搜索代码函数

**请求参数**:

```json
{
  "project_dir": "/path/to/project",
  "request_id": "req-001",
  "trace_id": "trace-001",
  "query": "search query",
  "search_mode": "auto",
  "limit": 5,
  "faiss": false
}
```


| 参数名      | 类型   | 必填 | 说明                    | 可选值/格式                   | 默认值   |
| ------------- | -------- | ------ | ------------------------- | ------------------------------- | ---------- |
| project_dir | string | 是   | 项目目录绝对路径        | -                             | -        |
| request_id  | string | 否   | 请求 ID（事件关联）     | 任意字符串                    | -        |
| trace_id    | string | 否   | 链路追踪 ID（跨请求）   | 任意字符串                    | `request_id` |
| query       | string | 是   | 搜索关键词              | -                             | -        |
| search_mode | string | 否   | 搜索模式                | `auto`/`semantic`/`keyword`/`hybrid` | `auto` |
| limit       | int    | 否   | 返回结果数量限制        | 正整数                        | 5        |
| faiss       | bool   | 否   | 是否使用 Faiss 语义索引 | `true`/`false`                | `false`  |

**响应示例**:

```json
{
  "code": 0,
  "message": "OK",
  "data": {
    "func_res": [
      {
        "anchor_id": "anc_2a98f530a7920208",
        "name": "functionName",
        "package": "packageName",
        "file": "file/path",
        "source": "docs/design.pdf",
        "page": 3,
        "slide": 0,
        "score": 0.95,
        "description": "function description",
        "code_snippet": "code snippet"
      }
    ],
    "route": {
      "intent": "code_search",
      "search_mode": "semantic",
      "confidence": 0.78,
      "signals": ["函数", "实现"],
      "reason": "intent=code_search, matched_signals=[函数 实现]"
    },
    "llm_route": {
      "provider": "githave",
      "model_id": "auto",
      "context_budget": 4096,
      "reasoning_mode": "balanced",
      "cost_hint": "medium",
      "complexity": 6,
      "signals": ["medium_query", "hybrid_mode"],
      "reason": "selected by query complexity + intent + search controls"
    },
    "tags": ["tag1", "tag2"],
    "funcs": [
      {
        "anchor_id": "anc_2a98f530a7920208",
        "name": "functionName",
        "package": "packageName",
        "file": "file/path",
        "score": 0.95,
        "description": "function description",
        "code_snippet": "code snippet"
      }
    ],
    "modules": [
      {
        "name": "moduleName",
        "package": "packageName",
        "file": "file/path",
        "score": 0.95,
        "description": "module description",
        "code_snippet": "code snippet"
      }
    ]
  }
}
```

### 3. 函数列表

**接口地址**: `POST /api/functions`

**描述**: 获取项目中的函数列表

**请求参数**:

```json
{
  "project_dir": "/path/to/project",
  "scan": false
}
```


| 参数名      | 类型   | 必填 | 说明                   | 可选值/格式    | 默认值  |
| ------------- | -------- | ------ | ------------------------ | ---------------- | --------- |
| project_dir | string | 是   | 项目目录绝对路径       | -              | -       |
| scan        | bool   | 否   | 是否只返回扫描统计信息 | `true`/`false` | `false` |

**响应示例**:

```json
{
  "code": 0,
  "message": "OK",
  "data": [
    {
      "name": "functionName",
      "package": "packageName",
      "file": "file/path",
      "scan": true
    }
  ]
}
```

### 4. 构建索引

**接口地址**: `POST /api/index`

**描述**: 为指定项目构建代码索引

**请求参数**:

```json
{
  "project_dir": "/path/to/project",
  "relative_dir": "sub/directory",
  "Faiss": false,
  "exclude": ["pattern1", "pattern2"]
}
```


| 参数名       | 类型     | 必填 | 说明                         | 可选值/格式    | 默认值   |
| -------------- | ---------- | ------ | ------------------------------ | ---------------- | ---------- |
| project_dir  | string   | 是   | 项目目录绝对路径             | -              | -        |
| relative_dir | string   | 否   | 相对目录路径（为空全量索引） | -              | 空字符串 |
| Faiss        | bool     | 否   | 是否使用 Faiss 语义索引      | `true`/`false` | `false`  |
| exclude      | string[] | 否   | 排除模式列表（glob语法）     | 例如`["*.md"]` | 空数组   |

**响应示例**:

```json
{
  "code": 0,
  "message": "Index built successfully"
}
```

### 5. 删除索引

**接口地址**: `DELETE /api/index`

**描述**: 删除指定项目的索引

**请求参数**:

```json
{
  "project_dir": "/path/to/project",
  "relative_dir": "sub/directory"
}
```

- `project_dir` (必需): 项目目录路径
- `relative_dir` (可选): 相对目录路径

**响应示例**:

```json
{
  "code": 0,
  "message": "Index deleted successfully"
}
```

### 6. 删除部分索引

**接口地址**: `DELETE /api/index/some`

**描述**: 删除指定项目的部分索引

**请求参数**:

```json
{
  "project_dir": "/path/to/project",
  "relative_dir": "sub/directory"
}
```

- `project_dir` (必需): 项目目录路径
- `relative_dir` (必需): 相对目录路径

**响应示例**:

```json
{
  "code": 0,
  "message": "Index deleted successfully"
}
```

### 7. 重置索引

**接口地址**: `DELETE /api/index/reset`

**描述**: 重置指定项目的索引

**请求参数**:

```json
{
  "project_dir": "/path/to/project",
  "relative_dir": "sub/directory"
}
```

- `project_dir` (必需): 项目目录路径
- `relative_dir` (可选): 相对目录路径

**响应示例**:

```json
{
  "code": 0,
  "message": "Index deleted successfully"
}
```

### 8. 模块分析任务状态查询

**接口地址**: `POST /api/module-graphs/status`

**描述**: 查询模块分析任务的当前状态和进度信息

**请求参数**:

```json
{
  "task_id": "20250622145417-1234"
  "project_dir": "/path/to/project"
}
```

| 参数名 | 类型 | 必填 | 说明 | 可选值/格式 | 默认值 |
| ------ | ---- | ---- | ---- | ---------- | ------ |
| task_id | string | 否 | 任务ID（从更新接口返回获取）| - | - |
| project_dir | string | 否 | 项目目录 | - | - |

**响应示例**:

```json
{
  "code": 0,
  "message": "Task status retrieved successfully",
  "data": {
    "id": "20250622145417-1234",
    "project_dir": "/path/to/project",
    "status": "running",
    "total_modules": 150,
    "completed_modules": 75,
    "remaining_modules": 75,
    "start_time": "2025-06-22T14:54:17+08:00"
  }
}
```

**任务状态说明**:

| 状态值 | 描述 |
| ------ | ---- |
| pending | 任务已创建但尚未开始执行 |
| running | 任务正在执行中 |
| completed | 任务已成功完成 |
| failed | 任务执行失败（可查看error_message字段） |

**注意**: 模块分析是一个耗时操作，尤其对于大型项目，强烈建议使用异步方式处理。客户端应先调用更新接口获取task_id，然后定期轮询状态接口获取最新进度。


### 9. 检查索引

**接口地址**: `POST /api/index/check`

**描述**: 查询指定子路径或子文件的索引信息

**请求参数**:

```json
{
  "project_dir": "/path/to/project",
  "relative_dir": "src/main"
}
```


| 参数名       | 类型   | 必填 | 说明             | 可选值/格式 | 默认值   |
| -------------- | -------- | ------ | ------------------ | ------------- | ---------- |
| project_dir  | string | 是   | 项目目录绝对路径 | -           | -        |
| relative_dir | string | 否   | 子路径           | -           | 空字符串 |

**响应示例**:

```json
{
  "code": 0,
  "message": "查询成功",
  "data": {
    "total_function_count": 150,
    "total_file_count": 25,
    "real_file_count": 170,
    "functions": {
      "src/main/utils.go": [
        {
          "name": "ParseConfig",
          "package": "main",
          "file": "src/main/utils.go",
          "start_line": 10,
          "end_line": 25,
          "description": "解析配置文件"
        }
      ],
      "src/main/handler.go": [
        {
          "name": "HandleRequest",
          "package": "main",
          "file": "src/main/handler.go",
          "start_line": 15,
          "end_line": 30,
          "description": "处理HTTP请求"
        }
      ]
    },
    "modules": {
      "root": [
        {
          "name": "project_root",
          "type": "directory",
          "path": "",
          "parent_path": "",
          "function_count": 150,
          "file_count": 25,
          "description": "项目根目录，包含核心业务逆向和连接层",
          "created_at": "2025-06-20T10:30:45+08:00",
          "updated_at": "2025-06-22T15:45:30+08:00"
        }
      ],
      "src": [
        {
          "name": "main",
          "type": "directory",
          "path": "src/main",
          "parent_path": "src",
          "function_count": 48,
          "file_count": 8,
          "description": "主程序包，包含程序入口点和核心逻辑",
          "created_at": "2025-06-20T12:15:22+08:00",
          "updated_at": "2025-06-22T15:45:30+08:00"
        },
        {
          "name": "utils",
          "type": "directory",
          "path": "src/utils",
          "parent_path": "src",
          "function_count": 35,
          "file_count": 6,
          "description": "工具类包，提供各种通用功能",
          "created_at": "2025-06-20T12:18:45+08:00",
          "updated_at": "2025-06-22T15:45:30+08:00"
        }
      ]
    }
  }
}
```

### 10. 增量索引更新

**接口地址**: `POST /api/index/incremental`

**描述**: 对指定项目进行增量索引更新

**请求参数**:

```json
{
  "project_dir": "/path/to/project",
  "branch": "main",
  "commit": "commit_hash",
  "faiss": false
}
```

- `project_dir` (必需): 项目目录路径
- `branch` (可选): Git 分支名
- `commit` (可选): Git 提交哈希
- `faiss` (可选): 是否使用 Faiss 索引，默认 false


| 参数名      | 类型   | 必填 | 说明                    | 可选值/格式    | 默认值   |
| ------------- | -------- | ------ | ------------------------- | ---------------- | ---------- |
| project_dir | string | 是   | 项目目录绝对路径        | -              | -        |
| branch      | string | 否   | Git 分支名              | -              | 空字符串 |
| commit      | string | 否   | Git 提交哈希            | -              | 空字符串 |
| faiss       | bool   | 否   | 是否使用 Faiss 语义索引 | `true`/`false` | `false`  |

**响应示例**:

```json
{
  "code": 0,
  "message": "Incremental update completed"
}
```

### 11. 列出图谱

**接口地址**: `POST /api/listGraph`

**描述**: 列出项目的代码图谱信息

**请求参数**:

```json
{
  "project_dir": "/path/to/project",
  "sub_path": "sub/path"
}
```


| 参数名      | 类型   | 必填 | 说明             | 可选值/格式 | 默认值   |
| ------------- | -------- | ------ | ------------------ | ------------- | ---------- |
| project_dir | string | 是   | 项目目录绝对路径 | -           | -        |
| sub_path    | string | 否   | 子路径           | -           | 空字符串 |

**响应示例**:

```json
{
  "code": 0,
  "message": "List completed",
  "data": null
}
```

### 12. 设置排除项

**接口地址**: `POST /api/exclude`

**描述**: 设置项目的排除模式

**请求参数**:

```json
{
  "project_dir": "/path/to/project",
  "exclude": ["pattern1", "pattern2"]
}
```


| 参数名      | 类型     | 必填 | 说明                 | 可选值/格式    | 默认值 |
| ------------- | ---------- | ------ | ---------------------- | ---------------- | -------- |
| project_dir | string   | 是   | 项目目录绝对路径     | -              | -      |
| exclude     | string[] | 是   | 排除模式列表（glob） | 例如`["*.md"]` | -      |

**响应示例**:

```json
{
  "code": 0,
  "message": "Exclude completed",
  "data": null
}
```

### 13. 读取排除项

**接口地址**: `POST /api/exclude/read`

**描述**: 读取项目的排除模式列表

**请求参数**:

```json
{
  "project_dir": "/path/to/project"
}
```

- `project_dir` (必需): 项目目录路径

**响应示例**:

```json
{
  "code": 0,
  "message": "List completed",
  "data": ["pattern1", "pattern2"]
}
```

### 14. LLM 分析器

**接口地址**: `POST /api/llm/analyzer`

**描述**: 使用 LLM 进行代码分析（待实现）

**请求参数**:

```json
{
  "project_dir": "/path/to/project",
  "relative_dir": "sub/directory"
}
```

- `project_dir` (必需): 项目目录路径
- `relative_dir` (可选): 相对目录路径

**响应示例**:

```json
{
  "code": 0,
  "message": "LLM SUCCESS",
  "data": null
}
```

### 15. 函数重要性评级

**接口地址**: `POST /api/ranking`

**描述**: 计算项目中函数的重要性评分

**请求参数**:

```json
{
  "project_dir": "/path/to/project",
  "config": {
    "Alpha": 0.4,
    "Beta": 0.2,
    "Gamma": 0.2,
    "Delta": 0.2
  }
}
```


| 参数名      | 类型               | 必填 | 说明             | 可选值/格式 | 默认值 |
| ------------- | -------------------- | ------ | ------------------ | ------------- | -------- |
| project_dir | string             | 是   | 项目目录绝对路径 | -           | -      |
| config      | object（权重配置） | 否   | 见下表           | 见下表      | 见下表 |

**config 权重配置对象说明：**


| 字段  | 类型  | 说明            | 默认值 |
| ------- | ------- | ----------------- | -------- |
| Alpha | float | FanIn 权重      | 0.4    |
| Beta  | float | FanOut 权重     | 0.2    |
| Gamma | float | Depth 权重      | 0.2    |
| Delta | float | Complexity 权重 | 0.2    |

**响应示例**:

```json
{
  "code": 0,
  "message": "Function importance scores calculated and saved successfully",
  "data": {
    "total_functions": 100,
    "config": {
      "Alpha": 0.4,
      "Beta": 0.2,
      "Gamma": 0.2,
      "Delta": 0.2
    },
    "scores": {
      "functionName1": 0.85,
      "functionName2": 0.72
    }
  }
}
```

### 16. 获取模块图谱

**接口地址**: `POST /api/module-graphs`

**描述**: 获取指定项目的所有模块图谱数据

**请求参数**:

```json
{
  "project_dir": "/path/to/project"
}
```

| 参数名      | 类型   | 必填 | 说明             | 可选值/格式 | 默认值 |
| ------------- | -------- | ------ | ------------------ | ------------- | -------- |
| project_dir | string | 是   | 项目目录绝对路径 | -           | -      |
| graph_type  | string | 否   | 图谱类型         | "flat"/"hierarchical"/"network"/"sunburst" | -      |

**响应示例**:

```json
{
  "code": 0,
  "message": "Module graphs retrieved successfully",
  "data": {
    "flat": {},
    "hierarchical": {},
    "network": {},
    "sunburst": {}
  }
}
```

**模块图谱类型说明**:

响应中包含四种不同类型的模块图谱数据，每种图谱适用于不同的前端可视化场景：

| 图谱类型 | 文件名 | 结构特点 | 前端渲染建议 | 适用场景 |
| -------- | ------ | -------- | ------------ | -------- |
| hierarchical | hierarchical_tree.json | 树形嵌套结构，包含父子层级关系 | 树状图、文件夹导航、层级结构图 | 项目结构梳理、模块层级导航 |
| network | network_graph.json | 节点和边的关系图，描述模块间引用依赖 | 力导向图、关系网络图 | 依赖关系分析、调用关系可视化 |
| sunburst | sunburst_chart.json | 多层嵌套环形结构，从中心向外扩展 | 旭日图 | 全局结构鸟瞰、模块复杂度分布 |
| flat | flat_nodes.json | 扁平化的模块节点数组 | 表格、列表、搜索/筛选组件 | 模块一览、批量操作、属性筛选 |

### 18. 更新模块图谱 (异步)

**接口地址**: `POST /api/module-graphs/update`

**描述**: 异步触发模块图谱的更新

**请求参数**:

```json
{
  "project_dir": "/path/to/project",
  "skip_llm": false
}
```

| 参数名      | 类型   | 必填 | 说明             | 可选值/格式 | 默认值 |
| ------------- | -------- | ------ | ------------------ | ------------- | -------- |
| project_dir | string | 是   | 项目目录绝对路径 | -           | -      |
| skip_llm    | bool   | 否   | 是否跳过 LLM 生成描述 | `true`/`false` | `false` |

**响应示例**:

```json
{
  "code": 0,
  "message": "图谱更新任务已启动",
  "data": {
    "task_id": "20250622145417-1234",
    "status": "pending",
    "project_dir": "/path/to/project"
  }
}
```

**说明**: 该接口会异步触发模块分析过程，生成上述四种类型的模块图谱文件，并保存在项目的 `.gitgo/module_graphs` 目录下。生成过程可能需要一些时间，接口会立即返回并在后台继续处理。

### 19. 删除模块分析记录

**接口地址**: `POST /api/module-graphs/delete`

**描述**: 删除指定项目的模块分析记录

**请求参数**:

```json
{
  "project_dir": "/path/to/project"
}
```

- `project_dir` (必需): 项目目录路径

**响应示例**:

```json
{
  "code": 0,
  "message": "模块分析记录删除成功"
}
```

### 20. 重置模块分析记录

**接口地址**: `POST /api/module-graphs/reset`

**描述**: 重置指定项目的模块分析记录

**请求参数**:

```json
{
  "project_dir": "/path/to/project"
}
```

- `project_dir` (必需): 项目目录路径

**响应示例**:

```json
{
  "code": 0,
  "message": "模块分析记录重置成功"
}
```

### 21. 刷新 Faiss 索引

**接口地址**: `POST /api/index/refresh`

**描述**: 刷新 Faiss 索引

**请求参数**:

```json
{
  "project_dir": "/path/to/project"
}
```

- `project_dir` (必需): 项目目录路径

**响应示例**:

```json
{
  "code": 0,
  "message": "Faiss index refreshed successfully"
}
```


## 工具平台接口

### 22. 列出工具

**接口地址**: `POST /api/tools/list`

**描述**: 列出当前已注册的工具能力（可按 `category` 过滤）。

**请求参数**:

```json
{
  "category": "routing"
}
```

**响应示例**:

```json
{
  "code": 0,
  "message": "OK",
  "data": [
    {
      "name": "fm.intent.route",
      "title": "Intent Router",
      "description": "对 query 进行意图路由，输出 intent 与 search_mode。",
      "version": "v1",
      "category": "routing"
    }
  ]
}
```

### 23. 调用工具

**接口地址**: `POST /api/tools/invoke`

**描述**: 调用单个已注册工具，并将调用事件记录到 `.gitgo/events.jsonl`（若传入 `project_dir`）。

**请求参数**:

```json
{
  "project_dir": "/path/to/project",
  "request_id": "req-001",
  "trace_id": "trace-001",
  "tool": "fm.search.plan",
  "input": {
    "query": "分析 auth 与 payment 的依赖关系",
    "search_mode": "auto",
    "strict": true,
    "enable_reranker": true,
    "limit": 10
  }
}
```

**响应示例**:

```json
{
  "code": 0,
  "message": "OK",
  "data": {
    "tool": "fm.search.plan",
    "success": true,
    "duration_ms": 2,
    "output": {
      "route": {
        "intent": "graph_reasoning"
      },
      "llm_route": {
        "model_id": "auto"
      }
    }
  }
}
```

### 24. 导出 MCP/OpenAI 工具声明

**接口地址**: `GET /api/tools/mcp`

**描述**: 导出 OpenAI function-call 风格的工具声明，便于 agent/MCP 直接消费。

**响应示例**:

```json
{
  "code": 0,
  "message": "OK",
  "data": {
    "tools": [
      {
        "type": "function",
        "function": {
          "name": "fm.intent.route",
          "description": "对 query 进行意图路由，输出 intent 与 search_mode。",
          "parameters": {
            "type": "object",
            "properties": {
              "query": {"type": "string"}
            },
            "required": ["query"]
          }
        }
      }
    ]
  }
}
```

### 25. 查询事件审计日志

**接口地址**: `POST /api/events/query`

**描述**: 查询项目 `.gitgo/events.jsonl` 中的结构化事件，支持按 `event_type` / `request_id` / `trace_id` 过滤。

**事件保留字段**: 每条事件包含 `schema_version`、`trace_id`，并可选包含 `cost`（例如 `hint`、`estimated_tokens`）以支持后续成本审计。

**请求参数**:

```json
{
  "project_dir": "/path/to/project",
  "limit": 50,
  "event_type": "tool_invoke",
  "request_id": "req-001",
  "trace_id": "trace-001"
}
```

**响应示例**:

```json
{
  "code": 0,
  "message": "OK",
  "data": {
    "count": 1,
    "events": [
      {
        "schema_version": "v1",
        "event_type": "tool_invoke",
        "request_id": "req-001",
        "trace_id": "trace-001",
        "status": "success"
      }
    ]
  }
}
```

### 26. Substrate 对象解析（DeepMemory Contract）

**接口地址**: `POST /api/substrate/object/resolve`

**描述**: 根据 `locator`（可传 `anchor_id`、`package.func`、函数名、文件线索）解析为稳定对象引用。

**请求参数**:

```json
{
  "project_dir": "/path/to/project",
  "locator": "pkg.AuthService.Login"
}
```

### 27. Substrate 邻接遍历（GraphTraversal）

**接口地址**: `POST /api/substrate/graph/neighbors`

**描述**: 查询目标对象的调用邻居，支持 `direction=in/out/both`。

**请求参数**:

```json
{
  "project_dir": "/path/to/project",
  "locator": "pkg.AuthService.Login",
  "direction": "both",
  "limit": 20
}
```

### 28. Substrate 变更信号（ChangeSignal）

**接口地址**: `POST /api/substrate/changes`

**描述**: 返回工作区变更信号；可选 `since`（commit/revision）执行区间对比。

**请求参数**:

```json
{
  "project_dir": "/path/to/project",
  "since": "HEAD~3"
}
```

### 29. Substrate 项目元信息（ProjectMetadata）

**接口地址**: `POST /api/substrate/project/meta`

**描述**: 返回 substrate 就绪度（`gitgo/index/graph/events`）和当前 `schema_version`。

### 30. Substrate 工具调用记录（ToolInvocationRecord）

**接口地址**: `POST /api/substrate/tool-records`

**描述**: 查询工具调用事件（`tool_invoke`），支持 `request_id/trace_id/status` 过滤。

**请求参数**:

```json
{
  "project_dir": "/path/to/project",
  "trace_id": "trace-001",
  "status": "success",
  "limit": 50
}
```

### 31. Substrate 文档层级树（Doc Tree）

**接口地址**: `POST /api/substrate/doc/tree`

**描述**: 查询文档的层级节点与依赖边（M1 支持 `contains/follows` 基础边）。

**请求参数**:

```json
{
  "project_dir": "/path/to/project",
  "source": "docs/guide.md"
}
```

### 32. Substrate 解析产物查询（Parse Artifacts）

**接口地址**: `POST /api/substrate/doc/parse-artifacts`

**描述**: 查询结构化解析产物，支持按 `source/status` 过滤。

**请求参数**:

```json
{
  "project_dir": "/path/to/project",
  "status": "failed",
  "limit": 50
}
```

### 33. Substrate 文档邻接查询（Doc Neighbors）

**接口地址**: `POST /api/substrate/doc/neighbors`

**描述**: 按 `node_id` 查询文档图邻接边，支持 `direction` 与 `edge_type` 过滤。

**请求参数**:

```json
{
  "project_dir": "/path/to/project",
  "node_id": "node_xxx",
  "direction": "both",
  "edge_type": "references",
  "limit": 20
}
```

## 配置管理接口

### 34. 获取配置

**接口地址**: `GET /c/config`

**描述**: 获取系统配置

**请求参数**: 无

### 35. 更新配置

**接口地址**: `PUT /c/config`

**描述**: 更新系统配置

**请求参数**: 根据配置结构定义

## 错误码说明

- `0`: 成功
- `1`: 请求参数错误或业务逻辑错误
- `2`: 服务器内部错误

## 使用示例

### cURL 示例

```bash
# 健康检查
curl -X GET http://localhost:5532/api/health

# 搜索代码
curl -X POST http://localhost:5532/api/search \
  -H "Content-Type: application/json" \
  -d '{
    "project_dir": "/path/to/project",
    "request_id": "req-001",
    "trace_id": "trace-001",
    "query": "function name",
    "search_mode": "auto",
    "limit": 10
  }'

# 列出工具
curl -X POST http://localhost:5532/api/tools/list \
  -H "Content-Type: application/json" \
  -d '{"category":"routing"}'

# 调用工具（搜索规划）
curl -X POST http://localhost:5532/api/tools/invoke \
  -H "Content-Type: application/json" \
  -d '{
    "project_dir": "/path/to/project",
    "request_id": "req-001",
    "trace_id": "trace-001",
    "tool": "fm.search.plan",
    "input": {
      "query": "分析 auth 与 payment 的依赖关系",
      "search_mode": "auto"
    }
  }'

# 查询事件日志
curl -X POST http://localhost:5532/api/events/query \
  -H "Content-Type: application/json" \
  -d '{
    "project_dir": "/path/to/project",
    "event_type": "tool_invoke",
    "trace_id": "trace-001",
    "limit": 20
  }'

# Substrate 对象解析
curl -X POST http://localhost:5532/api/substrate/object/resolve \
  -H "Content-Type: application/json" \
  -d '{
    "project_dir": "/path/to/project",
    "locator": "pkg.AuthService.Login"
  }'

# Substrate 工具记录
curl -X POST http://localhost:5532/api/substrate/tool-records \
  -H "Content-Type: application/json" \
  -d '{
    "project_dir": "/path/to/project",
    "trace_id": "trace-001",
    "status": "success",
    "limit": 20
  }'

# 构建索引
curl -X POST http://localhost:5532/api/index \
  -H "Content-Type: application/json" \
  -d '{
    "project_dir": "/path/to/project",
    "Faiss": true
  }'

# 检查索引
curl -X POST http://localhost:5532/api/index/check \
  -H "Content-Type: application/json" \
  -d '{
    "project_dir": "/path/to/project",
    "relative_dir": "src/main"
  }'

# 函数重要性评级
curl -X POST http://localhost:5532/api/ranking \
  -H "Content-Type: application/json" \
  -d '{
    "project_dir": "/path/to/project",
    "config": {
      "Alpha": 0.5,
      "Beta": 0.2,
      "Gamma": 0.2,
      "Delta": 0.1
    }
  }'
```

### JavaScript 示例

```javascript
// 搜索代码
const searchCode = async (projectDir, query) => {
  const response = await fetch('http://localhost:5532/api/search', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      project_dir: projectDir,
      query: query,
      search_mode: 'auto',
      limit: 10
    })
  });
  
  const result = await response.json();
  return result;
};

// 构建索引
const buildIndex = async (projectDir) => {
  const response = await fetch('http://localhost:5532/api/index', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      project_dir: projectDir,
      Faiss: true
    })
  });
  
  const result = await response.json();
  return result;
};

// 检查索引
const checkIndex = async (projectDir, relative_dir) => {
  const response = await fetch('http://localhost:5532/api/index/check', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      project_dir: projectDir,
      relative_dir: relative_dir
    })
  });
  
  const result = await response.json();
  return result;
};
```

## 注意事项

1. 所有涉及文件路径的参数都应使用绝对路径
2. 在使用搜索功能前，需要先构建索引
3. Faiss 索引提供更好的语义搜索能力，但需要额外的计算资源
4. 函数重要性评级功能需要先生成代码图谱（graph.json）
5. 排除模式支持 glob 语法
6. 服务启动时会自动初始化 Faiss 服务（如果配置了 FAISS_SERVICE_PATH）

## 环境变量

- `FM_PORT`: 服务端口，默认 5532
- `API_USER`: Basic Auth 用户名（可选）
- `API_PASS`: Basic Auth 密码（可选）
- `FAISS_SERVICE_PATH`: Faiss 服务路径（可选）
