以下是基于新版接口的完整 RESTful API 文档，所有请求均需在 JSON Body 中提供 **project_dir**，以支持任意目录下的子项目操作。

---

# 环境准备

在启动 HTTP 服务之前，请先在你的 Shell 中设置以下环境变量：

```bash
# API 鉴权
export API_USER="your_username"
export API_PASS="your_password"

# FAISS 后端服务目录（必须）
# 指向包含 faiss_server.py 的 FAISSService 目录，例如：
export FAISS_SERVICE_PATH="/path/to/flashmemory/cmd/main/FAISSService"

# （可选）HTTP 服务端口，默认为 5532
export PORT="5532"
```

确保上述变量生效后，再运行你的二进制或 `go run main.go` 启动服务。

---

## 全局说明

- **Base URL**：`http://<host>:<port>/api`
- **鉴权**：HTTP Basic Auth，用户名/密码通过环境变量 `API_USER` / `API_PASS` 配置。
  ```
  Authorization: Basic base64(API_USER:API_PASS)
  ```
- **请求/响应格式**：均为 `application/json`
- **统一响应结构**：
  ```jsonc
  {
    "code": 0,           // 0 表示成功，非 0 表示错误
    "message": "OK",     // 提示或错误信息
    "data": {...}        // 成功时返回的数据，可选
  }
  ```
- **通用错误码**：

  | code | HTTP 状态 | 含义                       |
    | ---- | --------- | -------------------------- |
  | 0    | 200       | 成功                       |
  | 1    | 400       | 请求体非法（缺少必填字段或 JSON 解析失败） |
  | 2    | 500       | 服务器内部错误             |
  | 401  | 401       | 认证失败                   |

---

## 1. 深度搜索 `/search`

- **URL**  
  `POST /api/search`
- **功能**  
  在指定项目目录的索引中按语义/关键词/混合模式查询函数。
- **请求参数**（JSON Body）
  ```json
  {
    "project_dir":  "/path/to/project",  // 必填：项目根路径
    "query":        "文件上传",           // 必填：查询关键词
    "search_mode":  "semantic",           // 可选：semantic|keyword|hybrid，默认 semantic
    "limit":        5                     // 可选：结果条数，默认 5
  }
  ```
- **响应示例**
  ```json
  {
    "code": 0,
    "message": "OK",
    "data": [
      {
        "name":        "UploadFile",
        "package":     "handler",
        "file":        "service/upload.go",
        "score":       0.873,
        "description": "上传文件处理函数，支持多部分表单..."
      },
      …
    ]
  }
  ```
- **调用示例**
  ```bash
  curl -u $API_USER:$API_PASS \
    -X POST http://localhost:5532/api/search \
    -H 'Content-Type: application/json' \
    -d '{
      "project_dir": "/Users/me/myproj",
      "query":       "文件上传",
      "search_mode": "hybrid",
      "limit":       3
    }'
  ```

---

## 2. 列出函数 `/functions`

- **URL**  
  `POST /api/functions`
- **功能**  
  列出指定项目（或子目录）中所有函数名称、所属包和文件路径。
- **请求参数**（JSON Body）
  ```json
  {
    "project_dir":  "/path/to/project",  // 必填：项目根路径
    "relative_dir": "src/utils"          // 可选：子目录，相对于 project_dir
  }
  ```
- **响应示例**
  ```json
  {
    "code": 0,
    "message": "OK",
    "data": [
      {
        "name":    "ParseConfig",
        "package": "config",
        "file":    "src/utils/config.go"
      },
      …
    ]
  }
  ```
- **调用示例**
  ```bash
  curl -u $API_USER:$API_PASS \
    -X POST http://localhost:5532/api/functions \
    -H 'Content-Type: application/json' \
    -d '{
      "project_dir":  "/Users/me/myproj",
      "relative_dir": "src/utils"
    }'
  ```

---

## 3. 构建索引 `/index` （全量）

- **URL**  
  `POST /api/index`
- **功能**  
  对整个项目或指定子目录执行全量索引（解析、LLM 分析、向量化、保存）。
- **请求参数**（JSON Body）
  ```json
  {
    "project_dir":  "/path/to/project",  // 必填：项目根路径
    "relative_dir": ""                   // 可选：子目录，默认全项目
  }
  ```
- **响应示例**
  ```json
  {
    "code": 0,
    "message": "Index built successfully"
  }
  ```
- **调用示例**
  ```bash
  curl -u $API_USER:$API_PASS \
    -X POST http://localhost:5532/api/index \
    -H 'Content-Type: application/json' \
    -d '{
      "project_dir": "/Users/me/myproj"
    }'
  ```

---

## 4. 删除索引 `/index` （删除）

- **URL**  
  `DELETE /api/index`
- **功能**  
  删除指定项目（或子目录）下的索引文件（整个 `.gitgo` 目录或子目录索引）。
- **请求参数**（JSON Body）
  ```json
  {
    "project_dir":  "/path/to/project",  // 必填：项目根路径
    "relative_dir": "src/utils"          // 可选：只删除该子目录索引
  }
  ```
- **响应示例**
  ```json
  {
    "code": 0,
    "message": "Index deleted successfully"
  }
  ```
- **调用示例**
  ```bash
  curl -u $API_USER:$API_PASS \
    -X DELETE http://localhost:5532/api/index \
    -H 'Content-Type: application/json' \
    -d '{
      "project_dir":  "/Users/me/myproj",
      "relative_dir": "src/utils"
    }'
  ```

---

## 5. 增量更新索引 `/index/incremental`

- **URL**  
  `POST /api/index/incremental`
- **功能**  
  基于 Git 分支和 Commit 做增量索引，只处理变更文件。
- **请求参数**（JSON Body）
  ```json
  {
    "project_dir": "/path/to/project",  // 必填：项目根路径
    "branch":      "develop",           // 可选：分支名，默认 master
    "commit":      "abc123"             // 可选：commit hash，不填则取 HEAD
  }
  ```
- **响应示例**
  ```json
  {
    "code": 0,
    "message": "Incremental update completed"
  }
  ```
- **调用示例**
  ```bash
  curl -u $API_USER:$API_PASS \
    -X POST http://localhost:5532/api/index/incremental \
    -H 'Content-Type: application/json' \
    -d '{
      "project_dir": "/Users/me/myproj",
      "branch":      "feature-x",
      "commit":      "ff12ab3"
    }'
  ```

---

### 常见错误响应

- **认证失败**
  ```http
  HTTP/1.1 401 Unauthorized
  ```
- **参数非法**（如缺少 `project_dir` 或 JSON 解析失败）
  ```json
  HTTP/1.1 400 Bad Request
  {
    "code": 1,
    "message": "project_dir is required"
  }
  ```
- **服务器内部错误**
  ```json
  HTTP/1.1 500 Internal Server Error
  {
    "code": 2,
    "message": "保存分析到DB失败: <具体错误>"
  }
  ```

---

> **注意**：
> - 调用前请确保环境变量 `API_USER`、`API_PASS`、`FAISS_SERVICE_PATH` 已设置。
> - 各接口返回的 `data` 结构请参照上述示例。
> - FAISS 服务可通过 `utils.StartFaissService` 在 Go 启动时自动拉起，或提前手动启动。
> - 如果未提前手动启动，服务启动后会自动寻找并启动该目录下的 `faiss_server.py`。