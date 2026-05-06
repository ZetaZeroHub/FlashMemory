# 代码审查提示词模板

> 适用场景：PR 提交前自查、模块质量审查、安全性审计
> 参考规范：`.claude/rules/security.md`、`.claude/rules/testing.md`

---

## 模板 A：PR 提交前全面审查

```
审查 @[file or directory] 的以下维度，给出结构化报告：

1. **安全性**
   - 是否存在注入风险（SQL、命令注入）
   - 是否存在硬编码的密钥或凭证
   - 是否存在未授权的文件读写（特别是 .gitgo/、.env）

2. **正确性**
   - 边界情况处理（nil、空切片、零值、路径不存在）
   - 错误路径是否都有 return/处理（不能静默忽略）
   - 并发安全（锁、channel、goroutine 泄漏）

3. **一致性**
   - 是否遵循项目约定（common.I18n、DbWriter、FaissWrapper 接口）
   - 文件路径处理是否使用 filepath.ToSlash / filepath.FromSlash
   - 日志使用 logs.Infof/Warnf/Errorf 而非 fmt.Println

4. **性能**
   - 是否有 N+1 查询（循环内数据库调用）
   - 大文件/大 prompt 是否有截断保护（cfg.CodeLimit）

对每个问题给出：
- 严重度：Critical / Warning / Suggestion
- 具体位置：文件名 + 行号
- 修复建议（代码示例优先）
```

---

## 模板 B：LLM 提示词质量审查

```
审查 @[file].go 中的 LLM 提示词构建逻辑（AnalyzeFunction / 相关函数）。

检查维度：

1. **结构清晰度**：提示词各段落是否有明确分区标识，模型是否能快速定位指令
2. **长度控制**：是否有 cfg.CodeLimit 截断保护，prompt 超限是否有降级策略
3. **JSON 输出校验**：是否验证 description 和 process 字段存在
4. **token 清理**：是否去除 ``` / ```json 等 markdown 包裹符号
5. **重试逻辑**：EOF / timeout / 5xx 是否覆盖，backoff 是否合理

输出格式：逐项给出 OK / WARNING / CRITICAL + 说明
```

---

## 模板 C：新 Parser 实现审查

```
审查新增的 parser 实现 @internal/parser/[xxx_parser.go]。

检查要点：

1. 是否实现了 FunctionInfo 的所有必填字段
   （Name, File, StartLine, EndLine, Package, Calls, Imports, FunctionType）
2. 跨平台路径处理：filepath.ToSlash / filepath.FromSlash
3. 是否有 fallback 机制（失败时降级到 regex_parser）
4. Tree-sitter 语言绑定是否正确加载（避免 nil panic）
5. 是否覆盖了以下测试用例：
   - 空文件
   - 只有注释的文件
   - 嵌套函数 / 匿名函数
   - Unicode 文件名

审查后给出：通过 / 需修改 + 具体改动点
```

---

## 模板 D：HTTP Handler 安全审查

```
审查 HTTP handler @cmd/app/fm_http.go 中的 [handler 名称] 函数。

安全检查清单：
- [ ] 输入参数有边界校验（长度、类型、非空）
- [ ] 路径参数不存在路径穿越风险（`..` 过滤）
- [ ] 数据库查询使用参数化（无字符串拼接 SQL）
- [ ] 响应不泄漏内部路径或堆栈信息
- [ ] Auth 中间件在 handler 前执行（非绕过）
- [ ] 大请求体有 size limit（Echo BodyLimit 中间件）

输出：逐项 ✅ / ❌ + 对应代码行号和修复建议
```

---

## 模板 E：快速 Diff 审查（PR 前）

```
审查当前分支相对于 main 的所有变更：

! git diff main --name-only

对每个变更文件检查：
1. 是否有测试覆盖新增的代码路径
2. 是否修改了不应该改动的文件（迁移文件、*_test.go、.env 相关）
3. 是否有遗漏的 i18n 字符串（硬编码中文或英文 UI 文本）

输出：
- 变更文件列表 + 每个文件的审查结论
- 需要补充测试的函数列表
- 可以合并 / 需要修改后合并 的最终建议
```
