# Design: 多 Session 并发 + 数据时效覆盖

**日期:** 2026-03-22
**状态:** Approved

## 问题

nethelper 的 watcher 在多文件并发场景下存在三类数据质量问题：

1. **重复数据**：`InsertNeighbors`、`InsertBGPPeers` 等使用纯 INSERT，同一命令执行两次产生重复行
2. **时间错乱**：所有时间戳使用 ingest time（处理时间）而非 capture time（采集时间），多文件乱序处理时无法判断数据新鲜度
3. **无法查询"当前状态"**：查询必须指定 `snapshot_id`，无法跨 snapshot 获取设备最新数据
4. **配置重复存储**：相同配置文本存多份，浪费空间
5. **全局互斥锁**：所有文件共享一把锁，10+ 文件时序列化瓶颈

## 设计

### 修复 1: Capture Time 传递

**当前：** Pipeline 中 `storeResult` 设置 `iface.LastUpdated = time.Now()`，Device 的 `LastSeen` 也是 `time.Now()`。

**修复：** 将 `CommandBlock.CapturedAt`（从日志时间戳解析的真实采集时间）传递到 store 层。

具体改动：
- `storeResult(deviceID, snapID, parseResult, capturedAt, vendor)` 已经接收 `capturedAt`
- 修改接口/设备更新逻辑：
  - `UpsertInterface`：`last_updated` 使用 `capturedAt`（非零时），且仅当 `capturedAt >= 已有 last_updated` 时才更新
  - `UpsertDevice`：`last_seen` 使用 `capturedAt`（非零时）

**Interfaces UPSERT 增强（条件更新）：**

```sql
INSERT INTO interfaces (...) VALUES (...)
ON CONFLICT(id) DO UPDATE SET
  status = CASE WHEN excluded.last_updated >= interfaces.last_updated
                THEN excluded.status ELSE interfaces.status END,
  ip_address = CASE WHEN excluded.last_updated >= interfaces.last_updated
                    THEN excluded.ip_address ELSE interfaces.ip_address END,
  -- ... 其他字段同理
  last_updated = CASE WHEN excluded.last_updated >= interfaces.last_updated
                      THEN excluded.last_updated ELSE interfaces.last_updated END
```

这样无论文件处理顺序如何，数据库总是保留最新采集的数据。

### 修复 2: Snapshot 级别的数据去重

**当前：** `InsertNeighbors` 等是纯 INSERT，重复 ingest 产生重复行。

**修复策略：** 在每次写入前，删除该设备该 snapshot 类型的旧数据（如果 snapshot 已存在）。但实际上 snapshot 是每次 ingest 新建的，不会重复。真正的重复来源是：同一个命令在同一个 log 中出现两次（用户多次执行），或不同 log 文件包含同一命令。

**实际修复：**
- 对 `protocol_neighbors`：以 `(device_id, protocol, remote_id, remote_address, snapshot_id)` 为自然键，改为 UPSERT
- 对 `bgp_peers`：以 `(device_id, peer_ip, address_family, snapshot_id)` 为自然键，改为 UPSERT
- 对 `mpls_te_tunnels`：以 `(device_id, tunnel_name, snapshot_id)` 为自然键，改为 UPSERT
- 对 `sr_mappings`：以 `(device_id, prefix, sid_index, snapshot_id)` 为自然键，改为 UPSERT

需要为这些表添加 UNIQUE 约束（通过 ALTER TABLE 或 CREATE UNIQUE INDEX）。

### 修复 3: 跨 Snapshot 查询当前状态

**新增 store 方法：**

```go
// GetLatestNeighbors 获取设备最新一次采集的邻居数据
func (db *DB) GetLatestNeighbors(deviceID string) ([]model.NeighborInfo, error)

// GetLatestBGPPeers 获取设备最新一次采集的 BGP peers
func (db *DB) GetLatestBGPPeers(deviceID string) ([]model.BGPPeer, error)
```

内部实现：先 `LatestSnapshotID(deviceID)` 获取最新 snapshot，然后调用已有的 `GetNeighbors(deviceID, snapID)` / `GetBGPPeers(deviceID, snapID)`。

对于 BGP peers 可能在不同 snapshot 上的情况（今天遇到的 bug），使用新的查询：

```sql
-- 获取最新 snapshot 中有 BGP peers 的那个
SELECT snapshot_id FROM bgp_peers
WHERE device_id = ?
ORDER BY snapshot_id DESC
LIMIT 1
```

### 修复 4: Config Hash 去重

在 `storeResult` 的配置存储逻辑中，插入前计算 SHA256：

```go
hash := sha256.Sum256([]byte(cleanedConfig))
hashStr := hex.EncodeToString(hash[:])
```

查询最近一份 config 的 hash，如果相同则跳过插入。

需要在 `config_snapshots` 表增加 `content_hash` 列（ALTER TABLE ADD COLUMN）。

### 修复 5: Per-File Mutex

将 `watch.go` 中的全局 `var ingestMu sync.Mutex` 替换为 per-file mutex：

```go
var fileMutexes sync.Map // map[string]*sync.Mutex

func getFileMutex(path string) *sync.Mutex {
    v, _ := fileMutexes.LoadOrStore(path, &sync.Mutex{})
    return v.(*sync.Mutex)
}
```

`OnFileChange` 中改为：
```go
mu := getFileMutex(path)
mu.Lock()
defer mu.Unlock()
```

不同文件并行处理，同一文件仍然序列化。SQLite WAL 模式支持并发读+单写。

### 代码改动范围

| 文件 | 改动 |
|------|------|
| `internal/store/migrations.go` | 添加 UNIQUE INDEX + content_hash 列 |
| `internal/store/interface_store.go` | UPSERT 条件更新（capture time） |
| `internal/store/device_store.go` | UPSERT 使用 capture time |
| `internal/store/neighbor_store.go` | INSERT → UPSERT |
| `internal/store/bgp_store.go` | INSERT → UPSERT + GetLatestBGPPeers |
| `internal/store/tunnel_store.go` | INSERT → UPSERT |
| `internal/store/sr_store.go` | INSERT → UPSERT |
| `internal/store/config_snapshot_store.go` | Hash 去重 |
| `internal/parser/pipeline.go` | 传递 capturedAt 到 UpsertInterface/UpsertDevice |
| `internal/cli/watch.go` | Per-file mutex |
| `internal/plan/topology.go` | 使用新的 GetLatestBGPPeers |

### 测试

- 单元测试：两次插入相同 neighbor → 只有一行（UPSERT 去重）
- 单元测试：旧 capture time 不覆盖新数据
- 单元测试：GetLatestNeighbors 返回最新 snapshot 的数据
- 单元测试：相同 config 不重复存储
- 集成测试：同一 log 文件 ingest 两次 → 数据不翻倍
