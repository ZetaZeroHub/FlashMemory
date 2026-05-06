# Bug 修复提示词模板

> 适用场景：定位并修复已知 bug、CI 失败、运行时错误
> 参考规范：`.claude/rules/testing.md` — 先写复现测试，再修复

---

## 模板 A：标准 Bug 修复

```
修复 [问题描述]。

复现步骤：
1. [步骤 1]
2. [步骤 2]
3. 出现错误：[现象]

错误信息（完整日志）：
```
[粘贴完整错误信息或堆栈跟踪]
```

期望行为：[描述正确行为]

修复流程（按序执行）：
1. 找到根因（不要假设，先读代码确认）
2. 写一个能**复现问题**的失败测试
3. 修复根因（解决根本问题，不要用 recover/suppress 掩盖错误）
4. 确认测试从失败变为通过
5. 运行 `go test ./[affected_package]/...` 确认无回归
```

---

## 模板 B：LLM 调用 / 网络错误

```
修复 [LLM调用/网络请求] 相关错误。

错误信息：[粘贴错误]

相关文件：@internal/analyzer/llm_analyzer.go

检查要点：
1. 确认重试逻辑覆盖此错误类型（EOF、connection reset、timeout、5xx）
2. 确认 backoff 策略正确（当前：500ms * 2^retry，上限 8s）
3. 确认 LLM 错误被正确识别为 LLMError（common.IsLLMError）

修复后验证：
- 运行 `go test ./internal/analyzer/...`
- 手动触发同类网络条件验证重试行为
```

---

## 模板 C：SQLite / 数据库错误

```
修复 SQLite 相关错误：[错误描述]

错误信息：[粘贴错误]

相关文件：
- @internal/index/index.go
- @internal/utils/ (DbWriter)

检查要点：
1. PRAGMA busy_timeout 是否已设置（默认 5000ms）
2. PRAGMA journal_mode 是否为 WAL
3. 是否存在未关闭的 rows（rows.Close() 调用）
4. 写操作是否通过 DbWriter 串行化

修复后验证：
- `go test -race ./internal/index/...`（检查 race condition）
- `go test -run TestEnsureIndexDB ./internal/index`
```

---

## 模板 D：CI 失败 / 构建错误

```
修复 CI 失败。

错误输出：
```
[粘贴完整 CI 日志，尤其是 FAIL 和 Error 部分]
```

请：
1. 定位失败的具体测试或构建步骤
2. 在本地复现（给出复现命令）
3. 修复根因
4. 本地确认 `go test ./...` 全部通过
5. 如果是跨平台问题，说明哪个平台受影响及原因
```

---

## 模板 E：Index / Zvec 引擎问题

```
排查并修复 [zvec/faiss] 引擎问题：[描述]

上下文：
- 使用引擎：[zvec / faiss]
- 错误发生阶段：[indexing / querying / embedding]
- 相关文件：@internal/index/zvec_wrapper.go 或 @internal/index/faiss_wrapper.go

检查要点：
1. subprocess JSON-line 协议是否正确（zvec 的 15-action 协议）
2. .gitgo/ 目录是否存在且有写权限
3. Zvec 进程是否正常启动（检查 FAISS_SERVICE_PATH 或 zvec 路径）

修复后验证：
- `go test ./internal/index/...`
- 手动运行 `./fm -dir [test_project] -engine zvec -query "test"`
```
