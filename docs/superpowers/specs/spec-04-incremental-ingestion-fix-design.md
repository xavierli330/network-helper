# Design: Fix Incremental Ingestion Config Truncation

**日期:** 2026-03-22
**状态:** Approved

## 问题

增量采集（watcher）在处理大命令输出（如 `display current-configuration`）时会截断数据。

**根因：** `watch.go` 的 `OnFileChange` 回调从 `last_offset` 读到文件末尾，将新字节传给 `pipeline.Ingest()`。如果一个命令的输出跨越了两次文件增长事件的边界，`Split()` 函数会将截断的输出当作完整块存储。

**实际案例：** LC 设备（H12516AF）的 `display current-configuration` 输出 ~104KB，但只存储了 14KB（接口段），BGP 配置段完全丢失。

## 修复方案：回溯到命令边界

### 变更文件

`internal/cli/watch.go` — 修改 `OnFileChange` 回调中的增量读取逻辑。

### 核心改动

**当前逻辑（有 bug）：**
```
1. offset = db.GetIngestion(path).LastOffset
2. newData = file[offset : fileSize]
3. pipeline.Ingest(path, newData)
4. db.UpsertIngestion(path, fileSize)  // 更新 offset 为当前文件大小
```

**修复后逻辑：**
```
1. offset = db.GetIngestion(path).LastOffset
2. newData = file[offset : fileSize]
3. blocks = pipeline.Ingest(path, newData)
4. 计算"安全偏移量"：最后一个有结束提示符的命令块结束位置
5. db.UpsertIngestion(path, safeOffset)  // 只推进到安全位置
```

### 实现细节

**方式：** 让 `pipeline.Ingest()` 返回已处理的字节数（相对于传入内容的偏移量），而不是总是假设所有传入内容都已完整处理。

具体：

1. **修改 `Split()` 函数**：返回最后一个完整命令块在原始内容中的结束偏移量（即最后一个有后续提示符的块的结束位置）。

2. **修改 `IngestResult`**：增加 `BytesConsumed int` 字段，表示实际被完整处理的字节数。

3. **修改 `watch.go`**：`last_offset` 更新为 `offset + result.BytesConsumed` 而非 `info.Size()`。

4. **对于 `watch ingest`（手动一次性导入）**：行为不变，因为整个文件一次性传入，不存在截断问题。

### Split 返回安全偏移量的逻辑

```go
// 在 Split 中，跟踪每个匹配到的提示符在原始文本中的字节偏移
// 最后一个命令块如果没有结束提示符 → 不包含在 blocks 中
// 返回 (blocks, lastCompleteByteOffset)
```

当 Split 检测到 N 个提示符时：
- 命令块 0..N-2 都有前后提示符界定 → 完整
- 命令块 N-1（最后一个）没有结束提示符 → 可能不完整

**安全偏移量** = 最后一个提示符在原始内容中的字节起始位置。这样下次读取时会从这个提示符重新开始，包含完整的最后一个命令输出。

### 重复检测

回溯意味着同一个命令块可能被处理两次。需要处理去重：
- `storeResult` 中的 UPSERT 语义（`ON CONFLICT DO UPDATE`）自然处理了接口和设备的去重
- 配置快照需要检查是否已存在相同内容（对比 `config_text` 或用 snapshot 去重）
- 最简单的方案：**跳过已见过的命令块**——在 `Split` 返回的 blocks 中，如果某个块的提示符+命令+时间戳与上一次处理的最后一个块相同，则跳过

### 测试

- 单元测试：构造一个在命令输出中间截断的输入，验证 Split 不返回不完整的块
- 单元测试：构造两次增量读取模拟，验证第二次读取时前一次的不完整块被正确重新处理
- 集成测试：用 LC 的真实日志重新采集，验证 BGP 配置段完整存入
