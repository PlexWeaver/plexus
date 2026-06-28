# Bug Report: `internal/boot` 测试失败 (2026-06-27)

## 环境

- **Repo**: `/d/deepseek/Reasonix/src/DeepSeek-Reasonix`
- **Branch**: (当前)
- **Go**: go1.24+
- **运行命令**: `go test ./internal/boot/`

---

## Bug 1: `TestBuildSubagentSkillFailedContinuationPersistsTranscript`

**文件**: `internal/boot/boot_test.go:428`

**症状**:
测试期望 subagent 失败后的 transcript 有精确的 4 条消息结构：
- `msgs[1].Content == "first skill task"`
- `msgs[2].Content == "first skill answer"`
- `msgs[3].Content == "second skill task"`

但实际结果中每条 user 消息前面都被注入了 `<reasoning-language>` 标签块，导致 Content 不匹配。

**实际输出** (消息 1 的内容):
```
<reasoning-language>
Visible reasoning/thinking text preference: use Simplified Chinese when...
</reasoning-language>

first skill task
```

**根因**:
系统提示构建流程 (`internal/boot/boot.go`) 中，`languagePolicy` 被注入到系统提示尾部，同时在 `Compose` 或 `Build` 逻辑中还可能追加了 `<reasoning-language>` 标签到每条 user 消息。subagent 的会话构建链路没有适配这种注入，导致 transcript 与预期格式不符。

**影响**:
subagent 失败续传的持久化 transcript 内容膨胀，下游（如前端或重试逻辑）依赖精确消息结构时可能解析错误。

**建议修复方向**:
1. 在 subagent 会话构建时跳过 `<reasoning-language>` 标签注入，或把这部分逻辑统一到 `control.Compose` 中集中处理。
2. 修改测试断言，容忍或剥离语言标签前缀。

---

## Bug 2: `TestBuildWithoutMemoryLeavesPromptUnchanged`

**文件**: `internal/boot/boot_test.go:1943`

**症状**:
测试配置 `system_prompt = "JUST THE BASE"`，且没有 memory 文件，期望构建后的 system prompt 去掉 `# Skills` 区块和语言政策后等于 `"JUST THE BASE"`。

但实际输出在 `"JUST THE BASE"` 之后附加了以下内容：
1. **"User-owned choices"** 政策段落 — 来自 `UserDecisionPolicy` 的 `ask` 工具说明
2. **`# Memory`** 区块 — 包含 "Persistent context loaded from memory files" 以及 `## Saved memories` 列表
3. **`# Skills — playbooks you can invoke`** 区块（虽然已被 strip 逻辑排除）

**根因**:
测试的 strip 逻辑（第 1935-1942 行）只剥离了 `# Skills` 和语言政策，但系统提示构建逻辑现在已经额外注入了 `UserDecisionPolicy` 和 `Memory` 区块。这些注入在 `Build` 流程中已经硬编码写入，而测试没有忽略它们。

**影响**:
这个测试实际上在阻止 memory 区块和 user-decision policy 的注入。如果这些注入是设计意图（它们很像是故意的特性），那么测试断言需要更新。

**建议修复方向**:
1. **如果这些注入是设计行为**：在测试的 strip 逻辑中添加 `# Memory` 和 user-decision policy 的剥离，使断言仍能验证 base prompt 不变。
2. **如果注入是回归 bug**：检查 `Build` 或 `Compose` 流程，确认是否在没有 memory 文件时也错误地追加了 `# Memory` 区块。

---

## 总结

| Bug | 类型 | 严重程度 | 修复复杂度 |
|-----|------|---------|-----------|
| #1 Subagent transcript 格式不符 | 测试/构建 | 中 | 低（测试适配或统一注入点） |
| #2 Base prompt 被额外内容污染 | 测试/构建 | 中 | 低（更新 strip 逻辑） |

两个 bug 都是**测试断言与系统提示注入逻辑之间的同步问题**，并非运行时功能损坏。系统的新特性（memory 注入、user-decision policy、reasoning-language 标签）已经部署，但对应测试没有同步更新。
