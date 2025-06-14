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
  "query": "search query",
  "search_mode": "hybrid",
  "limit": 5,
  "faiss": false
}
```

- `project_dir` (必需): 项目目录路径
- `query` (必需): 搜索查询
- `search_mode` (可选): 搜索模式，可选值：`semantic`、`keyword`、`hybrid`，默认 `hybrid`
- `limit` (可选): 返回结果数量限制，默认 5
- `faiss` (可选): 是否使用 Faiss 索引，默认 false

**响应示例**:
```json
{
  "code": 0,
  "message": "OK",
  "data": {
    "func_res": [
      {
        "name": "functionName",
        "package": "packageName",
        "file": "file/path",
        "score": 0.95,
        "description": "function description",
        "code_snippet": "code snippet"
      }
    ],
    "tags": ["tag1", "tag2"]
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

- `project_dir` (必需): 项目目录路径
- `scan` (可选): 是否只返回扫描统计，默认 false

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

- `project_dir` (必需): 项目目录路径
- `relative_dir` (可选): 相对目录路径，为空则构建全量索引
- `Faiss` (可选): 是否使用 Faiss 索引，默认 false
- `exclude` (可选): 排除模式列表

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

### 6. 重置索引

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

### 7. 增量索引更新

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

**响应示例**:
```json
{
  "code": 0,
  "message": "Incremental update completed"
}
```

### 8. 列出图谱

**接口地址**: `POST /api/listGraph`

**描述**: 列出项目的代码图谱信息

**请求参数**:
```json
{
  "project_dir": "/path/to/project",
  "sub_path": "sub/path"
}
```

- `project_dir` (必需): 项目目录路径
- `sub_path` (可选): 子路径

**响应示例**:
```json
{
  "code": 0,
  "message": "List completed",
  "data": null
}
```

### 9. 设置排除项

**接口地址**: `POST /api/exclude`

**描述**: 设置项目的排除模式

**请求参数**:
```json
{
  "project_dir": "/path/to/project",
  "exclude": ["pattern1", "pattern2"]
}
```

- `project_dir` (必需): 项目目录路径
- `exclude` (必需): 排除模式列表

**响应示例**:
```json
{
  "code": 0,
  "message": "Exclude completed",
  "data": null
}
```

### 10. 读取排除项

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

### 11. LLM 分析器

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

### 12. 函数重要性评级

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

- `project_dir` (必需): 项目目录路径
- `config` (可选): 权重配置
  - `Alpha`: FanIn 权重（默认 0.4）
  - `Beta`: FanOut 权重（默认 0.2）
  - `Gamma`: Depth 权重（默认 0.2）
  - `Delta`: Complexity 权重（默认 0.2）

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

## 配置管理接口

### 13. 获取配置

**接口地址**: `GET /c/config`

**描述**: 获取系统配置

**请求参数**: 无

### 14. 更新配置

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
    "query": "function name",
    "search_mode": "hybrid",
    "limit": 10
  }'

# 构建索引
curl -X POST http://localhost:5532/api/index \
  -H "Content-Type: application/json" \
  -d '{
    "project_dir": "/path/to/project",
    "Faiss": true
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
      search_mode: 'hybrid',
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