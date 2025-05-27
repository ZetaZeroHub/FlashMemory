

**梳理API接口流程与玩法**

用户想让我根据API文档和代码，梳理这些接口的流程和用法，还希望探讨一些有趣的玩法。接下来，我需要分析各个接口的使用方式，比如/api/search、/api/functions、/api/index、/api/index incremental、/api/index delete。而Python的Faiss服务可能是指faiss_server.py，在FAISSService目录下。这个服务通过HTTP进行健康检查和获取嵌入，是核心部分。探索“玩法”时，我想也许可以尝试不同的接口组合或者对API进行拓展。

已思考 7 秒

下面结合 `/api` 的官方文档和 `main.go`、`back` 包中的核心逻辑，梳理各接口的调用流程与底层实现，并给出一些可玩性和扩展思路。

---

## 一、环境准备 & 服务启动

1. **环境变量**（必须设置）
   ```bash
   export API_USER="your_username"
   export API_PASS="your_password"
   export FAISS_SERVICE_PATH="/path/to/flashmemory/cmd/main/FAISSService"
   export PORT="5532"                # 可选，默认 5532
   ```  
   通过 Basic Auth 保护所有 `/api` 路径接口 citeturn0file2。

2. **启动流程**
   ```bash
   go run main.go
   ```  
  - 启动时会调用 `back.InitFaiss()`：
    1. 在多种路径下寻找 `FAISSService` 目录
    2. 检查 Python 环境，启动 `faiss_server.py`
    3. 轮询 `/health` 直到服务就绪（最多 60 次尝试） citeturn0file1
  - 然后注册中间件 & 路由，监听 `${PORT}`

---

## 二、核心接口 & 调用流程

### 1. 全量构建索引 `POST /api/index`
- **文档**：构建整个项目（或子目录）的索引，并生成嵌入向量 citeturn0file2
- **Go 层面**：`buildIndexHandler` → `back.BuildIndex(projDir, relativeDir, full=true)` citeturn0file0turn0file1
- **内部主要步骤**（`back.indexCode`）：
  1. **InitFaiss**：启动/确认 Faiss 服务
  2. **.gitgo 目录**：创建索引存储目录
  3. **解析代码**：遍历支持的语言文件（.go/.py/.js/...），用 `parser` 提取所有函数信息
  4. **LLM 分析**：`analyzer.NewLLMAnalyzer` 生成每个函数的自然语言描述
  5. **知识图谱**：`graph.BuildGraph` 构建函数依赖图
  6. **数据库保存**：`index.SaveAnalysisToDB(results)` 将分析结果写入 SQLite
  7. **向量化**：对每个描述调用 `search.SimpleEmbedding`，并通过 HTTP Faiss Wrapper 添加到索引
  8. **存盘**：将 Faiss 索引写入 `code_index.faiss`，并保存分支/commit 信息
- **返回**：`{"code":0,"message":"Index built successfully"}`

### 2. 增量更新索引 `POST /api/index/incremental`
- **文档**：基于 Git 分支或指定 `commit`，仅处理变更过的文件 citeturn0file2
- **流程**：`incrementalIndexHandler` → `back.IncrementalUpdate(projDir, branch, commit)`
  - 内部调用 `back.indexCode(..., full=false)`
  - 如果检测到已有索引且在 Git 仓库下，则只针对变更文件列表执行删除旧记录 & 重新索引，否则退化为全量 citeturn0file1
- **返回**：`{"code":0,"message":"Incremental update completed"}`

### 3. 删除索引 `DELETE /api/index`
- **文档**：删除整个 `.gitgo` 或某个子目录索引 citeturn0file2
- **流程**：`deleteIndexHandler` → `back.DeleteIndex(targetDir)`，内部直接 `os.RemoveAll(.gitgo)` citeturn0file1
- **返回**：`{"code":0,"message":"Index deleted successfully"}`

### 4. 列出函数 `POST /api/functions`
- **文档**：列出指定目录下所有函数签名和所在文件 citeturn0file2
- **流程**：`listFunctionsHandler` → `parser.WalkAndParse(root, callback)`
- **返回**：
  ```json
  {
    "code":0,
    "message":"OK",
    "data":[
      {"name":"ParseConfig","package":"config","file":"src/utils/config.go"},…
    ]
  }
  ```

### 5. 深度搜索 `POST /api/search`
- **文档**：按语义/关键词/混合模式，在已有索引中查找最相关的函数 citeturn0file2
- **流程**（仅查询模式，跳过重建索引）：
  1. 验证 `.gitgo` 下 `code_index.db` 和 `code_index.faiss` 存在
  2. 打开 SQLite，构造 `index.NewFaissWrapper` 并 `LoadFromFile`
  3. `engine := &search.SearchEngine{Indexer: idx}` → `engine.Query(query, opts)`
  4. 将 `FunctionResult` 转成 `FuncRes` 并返回
  5. 停掉 Faiss 子进程
- **参数**：`{"project_dir":…,"query":"上传","search_mode":"hybrid","limit":5}`
- **返回**：
  ```json
  {
    "code":0,
    "message":"OK",
    "data":[
      {"name":"UploadFile","package":"handler","file":"service/upload.go","score":0.87,…},…
    ]
  }
  ```

---

## 三、可玩性 & 拓展思路

1. **IDE/Editor 集成**
  - 利用 `/functions` 构建侧边栏函数列表，点击调用 `/search` 跳转到代码片段
  - 在保存或 Git 提交钩子上触发 `/index/incremental`，保持索引实时更新

2. **多分支场景**
  - 不同分支维护独立索引，可同时查询 `feature-x` 与 `develop` 的差异，实现跨分支对比

3. **知识图谱可视化**
  - `graph.BuildGraph` 生成的 JSON 导出后，用 D3.js 或 Cytoscape 可视化调用关系，助力代码审查

4. **混合文档检索**
  - 把项目 Wiki、设计文档也当成“代码”解析，注入至同一 Faiss 服务，统一搜索

5. **ChatOps / Bot**
  - 在 Slack/Teams 中封装一个 Bot，将自然语言问题转成 `/api/search`，实时答复开发者

6. **安全审计 & LLM 评估**
  - 对搜索结果中的函数描述，结合安全规则或自定义提示，自动标注潜在风险

7. **自定义向量索引**
  - 直接调用 Python Faiss 服务的 HTTP 接口（`/add_vector`、`/search_vector`），对非代码文本进行检索

8. **自动化报告**
  - 周期性调用 `/api/index`、导出 `visualize.ComputePackageStats` 数据，生成包体量与函数增删变更报告

通过上述流程梳理和玩法建议，你可以快速上手并基于 FlashMemory 打造多样化的代码智能分析与检索体验。