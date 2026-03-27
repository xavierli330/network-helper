# Design: `plan upgrade` — 设备软件升级变更方案

**日期:** 2026-03-22
**状态:** Approved

## 命令接口

```bash
nethelper plan upgrade <device-id> --version <target-version> --file <firmware-file>
```

参数：
- `--version`：目标软件版本号（如 `V200R021C10SPC600`）
- `--file`：固件文件名（如 `NE40E-V800R021C10SPC600.cc`）
- `--format`、`--output`：同 plan isolate

## 阶段设计

`plan upgrade` = plan isolate + 升级步骤 + 恢复。共 8 个阶段：

| 阶段 | 名称 | 来源 |
|------|------|------|
| 0 | 采集 | 复用 isolate（+ 加 display version） |
| 1 | 预检查 | 复用 isolate（+ 检查磁盘空间、当前版本） |
| 2 | 协议隔离 | 复用 isolate（IGP→LDP→BGP） |
| 3 | 接口隔离 | 复用 isolate（LAG→物理口） |
| **4** | **升级执行** | **新增：上传+激活+重启** |
| **5** | **升级验证** | **新增：版本确认+基本健康检查** |
| 6 | 恢复 | isolate 回退的逆序（接口→协议） |
| 7 | 变更后检查 | 复用 isolate 的 post-check |

### 阶段 4: 升级执行（厂商差异最大的阶段）

**Huawei VRP：**
```
system-view
startup system-software <file>
quit
reboot
# 确认: Y
```

**H3C Comware：**
```
boot-loader file <file> slot <slot> main
# 确认: Y
reboot
```

**Cisco IOS-XR：**
```
install add source <path> <file>
install activate <package>
install commit
```

**Juniper JUNOS：**
```
request system software add <file>
request system reboot
```

### 阶段 5: 升级验证

```
display version           # 确认版本号匹配 --version 参数
display device            # 确认硬件正常
display alarm active      # 确认无告警
display interface brief   # 确认接口状态
```

## 实现

### 代码结构

| 文件 | 改动 |
|------|------|
| `internal/plan/commands_upgrade.go` | 新建：升级执行+验证命令（按厂商） |
| `internal/plan/commands_upgrade_test.go` | 新建：测试 |
| `internal/plan/upgrade.go` | 新建：`GenerateUpgradePlan(topo, version, file)` 编排 8 阶段 |
| `internal/plan/upgrade_test.go` | 新建：测试 |
| `internal/cli/plan.go` | 修改：添加 `plan upgrade` 子命令 |

### 复用策略

`GenerateUpgradePlan` 不重新实现 isolate 逻辑——直接调用 isolate_v2.go 中的 `buildCollectionPhase`、`buildPreCheckPhase`、`buildProtocolIsolationPhase`、`buildInterfaceIsolationPhase` 等已有函数（需要将它们从 `GenerateIsolationPlanV2` 内部提取为 exported 或至少 package-level 函数）。

### 升级参数数据结构

```go
type UpgradeParams struct {
    TargetVersion string // "V200R021C10SPC600"
    FirmwareFile  string // "NE40E-V800R021C10SPC600.cc"
}
```
