# 功能开发提示词模板

> 适用场景：新增功能、新模块开发（涉及 3+ 文件时推荐配合 SDD 流程）
> 参考规范：`.claude/rules/workflow.md` — SDD 三层产物流程

---

## 模板 A：完整 SDD 发起（推荐用于新功能）

```
实现 [功能描述]。

上下文：
- 相关文件：@internal/[package]/[file].go
- 参考已有模式：@internal/[similar_package]/[example].go
- 技术约束：Go 1.23 / Echo / SQLite / Zvec engine

要求：
1. [具体要求 1]
2. [具体要求 2]
3. 所有用户可见字符串使用 common.I18n(zh, en) 包装

验证：
- 运行 `go test ./internal/[package]/...` 确保测试通过
- 运行 `go build -o /tmp/fm_test cmd/main/fm.go` 确保编译成功
- 运行 `go build -o /tmp/fm_http_test cmd/app/fm_http.go` 确保 HTTP 二进制编译成功
```

---

## 模板 B：小功能直接实现（< 3 文件）

```
在 @[file path] 中实现 [功能名称]。

背景：[一句话说明为什么需要这个功能]

要求：
1. [具体要求 1]
2. [具体要求 2]
3. 不引入新的第三方依赖

验证步骤（按序执行）：
1. 先写测试用例（必须先失败）
2. 实现功能代码
3. 运行 `go test -run [TestFuncName] ./[package]/...`
4. 确认测试通过后提交
```

---

## 模板 C：CLI 命令新增

```
为 FlashMemory CLI 新增 `fm [command]` 子命令。

功能描述：[描述命令的作用]

实现要求：
1. 在 `cmd/cli/[command].go` 添加 Cobra 命令定义
2. 在 `cmd/main/fm.go` 添加对应的 flag 处理逻辑
3. 确保两处逻辑一致（IMPORTANT：改 CLI 必须同时更新 Cobra wrapper 和 core flag）
4. 用户可见字符串使用 common.I18n(zh, en)
5. 添加 `-lang` 支持（在 flag.Parse() 之前处理）

验证：
- `go build -o /tmp/fm_test cmd/main/fm.go && /tmp/fm_test [command] --help`
- `go build -o /tmp/fm_cli_test cmd/fm/main.go && /tmp/fm_cli_test [command] --help`
```

---

## 模板 D：HTTP API 端点新增

```
在 `cmd/app/fm_http.go` 新增 API 端点 `[METHOD] /api/[path]`。

功能描述：[描述端点的作用]

要求：
1. 响应格式严格遵循标准 envelope：{ "code": int, "message": string, "data": any }
2. 实现 Basic Auth 校验（复用现有中间件）
3. 参数验证：[列出必填参数及类型]
4. 错误处理：[列出需要处理的错误场景]

验证：
- 运行 `./start_fm_http_dev.sh`
- 使用 `@test_api.sh` 或 curl 测试端点响应
- 运行 `go test ./cmd/app/...`
```
