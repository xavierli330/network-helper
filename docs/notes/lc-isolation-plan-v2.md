# 设备隔离变更方案

| 字段 | 值 |
|------|----|
| 目标设备 | CD-GX-0201-G17-H12516AF-LC-01 (h3c) |
| 生成时间 | 2026-03-22 17:17 |
| 互联设备 | 0 台 |
| 影响评估 | ⚠️ SPOF — 移除后 6 台设备受影响 |

## 阶段0: 采集

变更前采集设备状态，建立基线

**注意事项:**

- ⚠️  [SPOF] 该设备是单点故障节点，隔离将直接影响以下设备: GZ-HXY-0203-C05-H12516XAF-QCDR-01, CD-GX-0201-H10-HW12816-QCDR-01, SZ-BH-0701-J04-MX960-QCTIX-02a, CD-GX-0201-D04-H6800QT-LA-01, CD-GX-0402-J20-NE40E-BR-01, GZ-YS-0101-G05-ASR9912-QCSTIX-01

### [cd-gx-0201-g17-h12516af-lc-01] 采集设备当前状态

```
display interface brief
display ip routing-table statistics
display bgp peer
display bgp routing-table statistics
display current-configuration
display version
```

## 阶段1: 预检查

确认变更前各 BGP 邻居组及 LAG 状态符合预期

**注意事项:**

- [ ] BGP 组 QCDR                  角色=downlink    期望 peers=2
- [ ] BGP 组 LA1                   角色=downlink    期望 peers=2
- [ ] BGP 组 LA2~448               角色=downlink    期望 peers=220
- [ ] BGP 组 XGWL                  角色=downlink    期望 peers=8
- [ ] BGP 组 SDN-Controller-Read   角色=management  期望 peers=1
- [ ] BGP 组 SDN-Controller-Write  角色=management  期望 peers=1

## 阶段2: 协议级隔离

按 downlink → uplink → management 顺序逐组执行 BGP peer ignore

### [cd-gx-0201-g17-h12516af-lc-01] BGP 隔离 — QCDR (2 peers, AS 45090)

```
system-view
bgp 65508
peer 10.162.185.53 ignore  # CD-XQ803-0509-C14-H12516AF-QCDR-01
peer 10.162.185.69 ignore  # CD-XQ803-0510-C14-H12516AF-QCDR-02
quit
return
```

### [cd-gx-0201-g17-h12516af-lc-01] >>> 检查点: QCDR 组 peers 应变为 Idle <<<

```
display bgp peer 10.162.185.53
display bgp peer 10.162.185.69
display bgp peer | include Established
display bgp routing-table statistics
```

### [cd-gx-0201-g17-h12516af-lc-01] BGP 隔离 — LA1 (2 peers, AS 1001)

```
system-view
bgp 65508
peer 100.88.244.1 ignore  # CD-GX-0201-R08-H6800QT-LA-01
peer 100.88.244.3 ignore  # CD-GX-0201-R09-H6800QT-LA-01
quit
return
```

### [cd-gx-0201-g17-h12516af-lc-01] >>> 检查点: LA1 组 peers 应变为 Idle <<<

```
display bgp peer 100.88.244.1
display bgp peer 100.88.244.3
display bgp peer | include Established
display bgp routing-table statistics
```

### [cd-gx-0201-g17-h12516af-lc-01] BGP 隔离 — LA2~448 (220 peers, AS 110 ASes)

```
system-view
bgp 65508
peer 100.88.244.101 ignore  # CD-GX-0201-E19-H6800QT-LA-01
peer 100.88.244.103 ignore  # CD-GX-0201-E20-H6800QT-LA-01
peer 100.88.244.105 ignore  # CD-GX-0201-F01-H6800QT-LA-01
peer 100.88.244.107 ignore  # CD-GX-0201-F02-H6800QT-LA-01
peer 100.88.244.109 ignore  # CD-GX-0201-F04-H6800QT-LA-01
peer 100.88.244.11 ignore  # CD-GX-0201-R16-H6800QT-LA-01
peer 100.88.244.111 ignore  # CD-GX-0201-F05-H6800QT-LA-01
peer 100.88.244.113 ignore  # CD-GX-0201-F06-H6800QT-LA-01
peer 100.88.244.115 ignore  # CD-GX-0201-F07-H6800QT-LA-01
peer 100.88.244.117 ignore  # CD-GX-0201-F09-H6800QT-LA-01
peer 100.88.244.119 ignore  # CD-GX-0201-F10-H6800QT-LA-01
peer 100.88.244.121 ignore  # CD-GX-0201-F11-H6800QT-LA-01
peer 100.88.244.123 ignore  # CD-GX-0201-F12-H6800QT-LA-01
peer 100.88.244.125 ignore  # CD-GX-0201-F14-H6800QT-LA-01
peer 100.88.244.127 ignore  # CD-GX-0201-F15-H6800QT-LA-01
peer 100.88.244.129 ignore  # CD-GX-0201-F16-H6800QT-LA-01
peer 100.88.244.13 ignore  # CD-GX-0201-R18-H6800QT-LA-01
peer 100.88.244.131 ignore  # CD-GX-0201-F17-H6800QT-LA-01
peer 100.88.244.133 ignore  # CD-GX-0201-F19-H6800QT-LA-01
peer 100.88.244.135 ignore  # CD-GX-0201-F20-H6800QT-LA-01
peer 100.88.244.137 ignore  # CD-GX-0201-G01-H6800QT-LA-01
peer 100.88.244.139 ignore  # CD-GX-0201-G02-H6800QT-LA-01
peer 100.88.244.141 ignore  # CD-GX-0201-G04-H6800QT-LA-01
peer 100.88.244.143 ignore  # CD-GX-0201-G05-H6800QT-LA-01
peer 100.88.244.145 ignore  # CD-GX-0201-G06-H6800QT-LA-01
peer 100.88.244.147 ignore  # CD-GX-0201-G07-H6800QT-LA-01
peer 100.88.244.149 ignore  # CD-GX-0201-G09-H6800QT-LA-01
peer 100.88.244.15 ignore  # CD-GX-0201-R19-H6800QT-LA-01
peer 100.88.244.151 ignore  # CD-GX-0201-G10-H6800QT-LA-01
peer 100.88.244.153 ignore  # CD-GX-0201-H13-H6800QT-LA-01
peer 100.88.244.155 ignore  # CD-GX-0201-H14-H6800QT-LA-01
peer 100.88.244.157 ignore  # CD-GX-0201-H16-H6800QT-LA-01
peer 100.88.244.159 ignore  # CD-GX-0201-H17-H6800QT-LA-01
peer 100.88.244.161 ignore  # CD-GX-0201-J01-H6800QT-LA-01
peer 100.88.244.163 ignore  # CD-GX-0201-J02-H6800QT-LA-01
peer 100.88.244.165 ignore  # CD-GX-0201-J04-H6800QT-LA-01
peer 100.88.244.167 ignore  # CD-GX-0201-J05-H6800QT-LA-01
peer 100.88.244.169 ignore  # CD-GX-0201-J06-H6800QT-LA-01
peer 100.88.244.17 ignore  # CD-GX-0201-S08-H6800QT-LA-01
peer 100.88.244.171 ignore  # CD-GX-0201-J07-H6800QT-LA-01
peer 100.88.244.173 ignore  # CD-GX-0201-J09-H6800QT-LA-01
peer 100.88.244.175 ignore  # CD-GX-0201-J10-H6800QT-LA-01
peer 100.88.244.177 ignore  # CD-GX-0201-J11-H6800QT-LA-01
peer 100.88.244.179 ignore  # CD-GX-0201-J12-H6800QT-LA-01
peer 100.88.244.181 ignore  # CD-GX-0201-J14-H6800QT-LA-01
peer 100.88.244.183 ignore  # CD-GX-0201-J15-H6800QT-LA-01
peer 100.88.244.185 ignore  # CD-GX-0201-J16-H6800QT-LA-01
peer 100.88.244.187 ignore  # CD-GX-0201-J17-H6800QT-LA-01
peer 100.88.244.189 ignore  # CD-GX-0201-J19-H6800QT-LA-01
peer 100.88.244.19 ignore  # CD-GX-0201-S09-H6800QT-LA-01
peer 100.88.244.191 ignore  # CD-GX-0201-J20-H6800QT-LA-01
peer 100.88.244.193 ignore  # CD-GX-0201-K01-H6800QT-LA-01
peer 100.88.244.195 ignore  # CD-GX-0201-K02-H6800QT-LA-01
peer 100.88.244.197 ignore  # CD-GX-0201-K04-H6800QT-LA-01
peer 100.88.244.199 ignore  # CD-GX-0201-K05-H6800QT-LA-01
peer 100.88.244.201 ignore  # CD-GX-0201-K06-H6800QT-LA-01
peer 100.88.244.203 ignore  # CD-GX-0201-K07-H6800QT-LA-01
peer 100.88.244.205 ignore  # CD-GX-0201-K09-H6800QT-LA-01
peer 100.88.244.207 ignore  # CD-GX-0201-K10-H6800QT-LA-01
peer 100.88.244.209 ignore  # CD-GX-0201-K11-H6800QT-LA-01
peer 100.88.244.21 ignore  # CD-GX-0201-S11-H6800QT-LA-01
peer 100.88.244.211 ignore  # CD-GX-0201-K12-H6800QT-LA-01
peer 100.88.244.213 ignore  # CD-GX-0201-K14-H6800QT-LA-01
peer 100.88.244.215 ignore  # CD-GX-0201-K15-H6800QT-LA-01
peer 100.88.244.217 ignore  # CD-GX-0201-K16-H6800QT-LA-01
peer 100.88.244.219 ignore  # CD-GX-0201-K17-H6800QT-LA-01
peer 100.88.244.221 ignore  # CD-GX-0201-K19-H6800QT-LA-01
peer 100.88.244.223 ignore  # CD-GX-0201-K20-H6800QT-LA-01
peer 100.88.244.225 ignore  # CD-GX-0307-K01-H6800QT-LA-01
peer 100.88.244.227 ignore  # CD-GX-0307-K02-H6800QT-LA-01
peer 100.88.244.229 ignore  # CD-GX-0307-K04-H6800QT-LA-01
peer 100.88.244.23 ignore  # CD-GX-0201-S12-H6800QT-LA-01
peer 100.88.244.231 ignore  # CD-GX-0307-K05-H6800QT-LA-01
peer 100.88.244.233 ignore  # CD-GX-0307-K06-H6800QT-LA-01
peer 100.88.244.235 ignore  # CD-GX-0307-K07-H6800QT-LA-01
peer 100.88.244.237 ignore  # CD-GX-0307-K09-H6800QT-LA-01
peer 100.88.244.239 ignore  # CD-GX-0307-K10-H6800QT-LA-01
peer 100.88.244.241 ignore  # CD-GX-0307-K11-H6800QT-LA-01
peer 100.88.244.243 ignore  # CD-GX-0307-K12-H6800QT-LA-01
peer 100.88.244.245 ignore  # CD-GX-0307-K14-H6800QT-LA-01
peer 100.88.244.247 ignore  # CD-GX-0307-K15-H6800QT-LA-01
peer 100.88.244.249 ignore  # CD-GX-0307-K16-H6800QT-LA-01
peer 100.88.244.25 ignore  # CD-GX-0201-S13-H6800QT-LA-01
peer 100.88.244.251 ignore  # CD-GX-0307-K17-H6800QT-LA-01
peer 100.88.244.253 ignore  # CD-GX-0307-K19-H6800QT-LA-01
peer 100.88.244.255 ignore  # CD-GX-0307-K20-H6800QT-LA-01
peer 100.88.244.27 ignore  # CD-GX-0201-S14-H6800QT-LA-01
peer 100.88.244.29 ignore  # CD-GX-0201-S16-H6800QT-LA-01
peer 100.88.244.31 ignore  # CD-GX-0201-S17-H6800QT-LA-01
peer 100.88.244.33 ignore  # CD-GX-0201-S18-H6800QT-LA-01
peer 100.88.244.35 ignore  # CD-GX-0201-S19-H6800QT-LA-01
peer 100.88.244.37 ignore  # CD-GX-0201-S21-H6800QT-LA-01
peer 100.88.244.39 ignore  # CD-GX-0201-R21-H6800QT-LA-01
peer 100.88.244.41 ignore  # CD-GX-0201-D01-H6800QT-LA-01
peer 100.88.244.43 ignore  # CD-GX-0201-D02-H6800QT-LA-01
peer 100.88.244.45 ignore  # CD-GX-0201-D04-H6800QT-LA-01
peer 100.88.244.47 ignore  # CD-GX-0201-D05-H6800QT-LA-01
peer 100.88.244.49 ignore  # CD-GX-0201-D06-H6800QT-LA-01
peer 100.88.244.5 ignore  # CD-GX-0201-R13-H6800QT-LA-01
peer 100.88.244.51 ignore  # CD-GX-0201-D07-H6800QT-LA-01
peer 100.88.244.53 ignore  # CD-GX-0201-D09-H6800QT-LA-01
peer 100.88.244.55 ignore  # CD-GX-0201-D10-H6800QT-LA-01
peer 100.88.244.57 ignore  # CD-GX-0201-D13-H6800QT-LA-01
peer 100.88.244.59 ignore  # CD-GX-0201-D14-H6800QT-LA-01
peer 100.88.244.61 ignore  # CD-GX-0201-D16-H6800QT-LA-01
peer 100.88.244.63 ignore  # CD-GX-0201-D17-H6800QT-LA-01
peer 100.88.244.65 ignore  # CD-GX-0201-D18-H6800QT-LA-01
peer 100.88.244.67 ignore  # CD-GX-0201-D19-H6800QT-LA-01
peer 100.88.244.69 ignore  # CD-GX-0201-D21-H6800QT-LA-01
peer 100.88.244.7 ignore  # CD-GX-0201-R14-H6800QT-LA-01
peer 100.88.244.71 ignore  # CD-GX-0201-E21-H6800QT-LA-01
peer 100.88.244.73 ignore  # CD-GX-0201-E01-H6800QT-LA-01
peer 100.88.244.75 ignore  # CD-GX-0201-E02-H6800QT-LA-01
peer 100.88.244.77 ignore  # CD-GX-0201-E04-H6800QT-LA-01
peer 100.88.244.79 ignore  # CD-GX-0201-E05-H6800QT-LA-01
peer 100.88.244.81 ignore  # CD-GX-0201-E06-H6800QT-LA-01
peer 100.88.244.83 ignore  # CD-GX-0201-E07-H6800QT-LA-01
peer 100.88.244.85 ignore  # CD-GX-0201-E09-H6800QT-LA-01
peer 100.88.244.87 ignore  # CD-GX-0201-E10-H6800QT-LA-01
peer 100.88.244.89 ignore  # CD-GX-0201-E11-H6800QT-LA-01
peer 100.88.244.9 ignore  # CD-GX-0201-R15-H6800QT-LA-01
peer 100.88.244.91 ignore  # CD-GX-0201-E12-H6800QT-LA-01
peer 100.88.244.93 ignore  # CD-GX-0201-E14-H6800QT-LA-01
peer 100.88.244.95 ignore  # CD-GX-0201-E15-H6800QT-LA-01
peer 100.88.244.97 ignore  # CD-GX-0201-E16-H6800QT-LA-01
peer 100.88.244.99 ignore  # CD-GX-0201-E17-H6800QT-LA-01
peer 100.88.245.101 ignore  # CD-GX-0307-D17-H6800QT-LA-01
peer 100.88.245.103 ignore  # CD-GX-0307-D18-H6800QT-LA-01
peer 100.88.245.105 ignore  # CD-GX-0307-D20-H6800QT-LA-01
peer 100.88.245.107 ignore  # CD-GX-0307-D21-H6800QT-LA-01
peer 100.88.245.109 ignore  # CD-GX-0307-E02-H6800QT-LA-01
peer 100.88.245.11 ignore  # CD-GX-0201-N09-H6800QT-LA-01
peer 100.88.245.111 ignore  # CD-GX-0307-E03-H6800QT-LA-01
peer 100.88.245.113 ignore  # CD-GX-0307-E05-H6800QT-LA-01
peer 100.88.245.115 ignore  # CD-GX-0307-E06-H6800QT-LA-01
peer 100.88.245.117 ignore  # CD-GX-0307-E14-H6800QT-LA-01
peer 100.88.245.119 ignore  # CD-GX-0307-E15-H6800QT-LA-01
peer 100.88.245.121 ignore  # CD-GX-0307-E17-H6800QT-LA-01
peer 100.88.245.123 ignore  # CD-GX-0307-E18-H6800QT-LA-01
peer 100.88.245.125 ignore  # CD-GX-0307-E20-H6800QT-LA-01
peer 100.88.245.127 ignore  # CD-GX-0307-E21-H6800QT-LA-01
peer 100.88.245.129 ignore  # CD-GX-0307-F01-H6800QT-LA-01
peer 100.88.245.13 ignore  # CD-GX-0201-N13-H6800QT-LA-01
peer 100.88.245.131 ignore  # CD-GX-0307-F02-H6800QT-LA-01
peer 100.88.245.133 ignore  # CD-GX-0307-F04-H6800QT-LA-01
peer 100.88.245.135 ignore  # CD-GX-0307-F05-H6800QT-LA-01
peer 100.88.245.137 ignore  # CD-GX-0307-F16-H6800QT-LA-01
peer 100.88.245.139 ignore  # CD-GX-0307-F17-H6800QT-LA-01
peer 100.88.245.141 ignore  # CD-GX-0307-F19-H6800QT-LA-01
peer 100.88.245.143 ignore  # CD-GX-0307-F20-H6800QT-LA-01
peer 100.88.245.145 ignore  # CD-GX-0307-G01-H6800QT-LA-01
peer 100.88.245.147 ignore  # CD-GX-0307-G02-H6800QT-LA-01
peer 100.88.245.149 ignore  # CD-GX-0307-G04-H6800QT-LA-01
peer 100.88.245.15 ignore  # CD-GX-0201-N14-H6800QT-LA-01
peer 100.88.245.151 ignore  # CD-GX-0307-G05-H6800QT-LA-01
peer 100.88.245.153 ignore  # CD-GX-0307-G06-H6800QT-LA-01
peer 100.88.245.155 ignore  # CD-GX-0307-G07-H6800QT-LA-01
peer 100.88.245.157 ignore  # CD-GX-0307-G09-H6800QT-LA-01
peer 100.88.245.159 ignore  # CD-GX-0307-G10-H6800QT-LA-01
peer 100.88.245.161 ignore  # CD-GX-0307-G11-H6800QT-LA-01
peer 100.88.245.163 ignore  # CD-GX-0307-G12-H6800QT-LA-01
peer 100.88.245.165 ignore  # CD-GX-0307-G14-H6800QT-LA-01
peer 100.88.245.167 ignore  # CD-GX-0307-G15-H6800QT-LA-01
peer 100.88.245.169 ignore  # CD-GX-0307-G16-H6800QT-LA-01
peer 100.88.245.17 ignore  # CD-GX-0201-N15-H6800QT-LA-01
peer 100.88.245.171 ignore  # CD-GX-0307-G17-H6800QT-LA-01
peer 100.88.245.173 ignore  # CD-GX-0307-G19-H6800QT-LA-01
peer 100.88.245.175 ignore  # CD-GX-0307-G20-H6800QT-LA-01
peer 100.88.245.177 ignore  # CD-GX-0307-J01-H6800QT-LA-01
peer 100.88.245.179 ignore  # CD-GX-0307-J02-H6800QT-LA-01
peer 100.88.245.181 ignore  # CD-GX-0307-N07-H6800QT-LA-01
peer 100.88.245.183 ignore  # CD-GX-0307-N08-H6800QT-LA-01
peer 100.88.245.185 ignore  # CD-GX-0307-N10-H6800QT-LA-01
peer 100.88.245.187 ignore  # CD-GX-0307-N11-H6800QT-LA-0
peer 100.88.245.189 ignore  # CD-GX-0307-J01-H6800QT-LA-02
peer 100.88.245.19 ignore  # CD-GX-0201-N16-H6800QT-LA-01
peer 100.88.245.191 ignore  # CD-GX-0307-J02-H6800QT-LA-02
peer 100.88.245.193 ignore  # CD-GX-0307-N07-H6800QT-LA-02
peer 100.88.245.195 ignore  # CD-GX-0307-N08-H6800QT-LA-02
peer 100.88.245.21 ignore  # CD-GX-0201-N18-H6800QT-LA-01
peer 100.88.245.23 ignore  # CD-GX-0201-N19-H6800QT-LA-01
peer 100.88.245.25 ignore  # CD-GX-0201-P08-H6800QT-LA-01
peer 100.88.245.27 ignore  # CD-GX-0201-P09-H6800QT-LA-01
peer 100.88.245.29 ignore  # CD-GX-0201-P11-H6800QT-LA-01
peer 100.88.245.31 ignore  # CD-GX-0201-P12-H6800QT-LA-01
peer 100.88.245.33 ignore  # CD-GX-0201-P13-H6800QT-LA-01
peer 100.88.245.35 ignore  # CD-GX-0201-P14-H6800QT-LA-01
peer 100.88.245.37 ignore  # CD-GX-0201-P16-H6800QT-LA-01
peer 100.88.245.39 ignore  # CD-GX-0201-P17-H6800QT-LA-01
peer 100.88.245.41 ignore  # CD-GX-0201-Q08-H6800QT-LA-01
peer 100.88.245.43 ignore  # CD-GX-0201-Q09-H6800QT-LA-01
peer 100.88.245.45 ignore  # CD-GX-0201-Q11-H6800QT-LA-01
peer 100.88.245.47 ignore  # CD-GX-0201-Q12-H6800QT-LA-01
peer 100.88.245.49 ignore  # CD-GX-0201-Q13-H6800QT-LA-01
peer 100.88.245.51 ignore  # CD-GX-0201-Q14-H6800QT-LA-01
peer 100.88.245.53 ignore  # CD-GX-0201-Q16-H6800QT-LA-01
peer 100.88.245.55 ignore  # CD-GX-0201-Q17-H6800QT-LA-01
peer 100.88.245.57 ignore  # CD-GX-0201-Q18-H6800QT-LA-01
peer 100.88.245.59 ignore  # CD-GX-0201-Q19-H6800QT-LA-01
peer 100.88.245.61 ignore  # CD-GX-0201-Q21-H6800QT-LA-01
peer 100.88.245.63 ignore  # CD-GX-0201-R20-H6800QT-LA-01
peer 100.88.245.65 ignore  # CD-GX-0307-A14-H6800QT-LA-01
peer 100.88.245.67 ignore  # CD-GX-0307-A15-H6800QT-LA-01
peer 100.88.245.69 ignore  # CD-GX-0307-A17-H6800QT-LA-01
peer 100.88.245.71 ignore  # CD-GX-0307-A18-H6800QT-LA-01
peer 100.88.245.73 ignore  # CD-GX-0307-A20-H6800QT-LA-01
peer 100.88.245.75 ignore  # CD-GX-0307-A21-H6800QT-LA-01
peer 100.88.245.77 ignore  # CD-GX-0307-C10-H6800QT-LA-01
peer 100.88.245.79 ignore  # CD-GX-0307-C11-H6800QT-LA-01
peer 100.88.245.81 ignore  # CD-GX-0307-D01-H6800QT-LA-01
peer 100.88.245.83 ignore  # CD-GX-0307-D02-H6800QT-LA-01
peer 100.88.245.85 ignore  # CD-GX-0307-D04-H6800QT-LA-01
peer 100.88.245.87 ignore  # CD-GX-0307-D05-H6800QT-LA-01
peer 100.88.245.89 ignore  # CD-GX-0307-D06-H6800QT-LA-01
peer 100.88.245.9 ignore  # CD-GX-0201-N08-H6800QT-LA-01
peer 100.88.245.91 ignore  # CD-GX-0307-D07-H6800QT-LA-01
peer 100.88.245.93 ignore  # CD-GX-0307-D09-H6800QT-LA-01
peer 100.88.245.95 ignore  # CD-GX-0307-D10-H6800QT-LA-01
peer 100.88.245.97 ignore  # CD-GX-0307-D14-H6800QT-LA-01
peer 100.88.245.99 ignore  # CD-GX-0307-D15-H6800QT-LA-01
quit
return
```

### [cd-gx-0201-g17-h12516af-lc-01] >>> 检查点: LA2~448 组 peers 应变为 Idle <<<

```
display bgp peer 100.88.244.101
display bgp peer 100.88.244.103
display bgp peer 100.88.244.105
display bgp peer 100.88.244.107
display bgp peer 100.88.244.109
display bgp peer 100.88.244.11
display bgp peer 100.88.244.111
display bgp peer 100.88.244.113
display bgp peer 100.88.244.115
display bgp peer 100.88.244.117
display bgp peer 100.88.244.119
display bgp peer 100.88.244.121
display bgp peer 100.88.244.123
display bgp peer 100.88.244.125
display bgp peer 100.88.244.127
display bgp peer 100.88.244.129
display bgp peer 100.88.244.13
display bgp peer 100.88.244.131
display bgp peer 100.88.244.133
display bgp peer 100.88.244.135
display bgp peer 100.88.244.137
display bgp peer 100.88.244.139
display bgp peer 100.88.244.141
display bgp peer 100.88.244.143
display bgp peer 100.88.244.145
display bgp peer 100.88.244.147
display bgp peer 100.88.244.149
display bgp peer 100.88.244.15
display bgp peer 100.88.244.151
display bgp peer 100.88.244.153
display bgp peer 100.88.244.155
display bgp peer 100.88.244.157
display bgp peer 100.88.244.159
display bgp peer 100.88.244.161
display bgp peer 100.88.244.163
display bgp peer 100.88.244.165
display bgp peer 100.88.244.167
display bgp peer 100.88.244.169
display bgp peer 100.88.244.17
display bgp peer 100.88.244.171
display bgp peer 100.88.244.173
display bgp peer 100.88.244.175
display bgp peer 100.88.244.177
display bgp peer 100.88.244.179
display bgp peer 100.88.244.181
display bgp peer 100.88.244.183
display bgp peer 100.88.244.185
display bgp peer 100.88.244.187
display bgp peer 100.88.244.189
display bgp peer 100.88.244.19
display bgp peer 100.88.244.191
display bgp peer 100.88.244.193
display bgp peer 100.88.244.195
display bgp peer 100.88.244.197
display bgp peer 100.88.244.199
display bgp peer 100.88.244.201
display bgp peer 100.88.244.203
display bgp peer 100.88.244.205
display bgp peer 100.88.244.207
display bgp peer 100.88.244.209
display bgp peer 100.88.244.21
display bgp peer 100.88.244.211
display bgp peer 100.88.244.213
display bgp peer 100.88.244.215
display bgp peer 100.88.244.217
display bgp peer 100.88.244.219
display bgp peer 100.88.244.221
display bgp peer 100.88.244.223
display bgp peer 100.88.244.225
display bgp peer 100.88.244.227
display bgp peer 100.88.244.229
display bgp peer 100.88.244.23
display bgp peer 100.88.244.231
display bgp peer 100.88.244.233
display bgp peer 100.88.244.235
display bgp peer 100.88.244.237
display bgp peer 100.88.244.239
display bgp peer 100.88.244.241
display bgp peer 100.88.244.243
display bgp peer 100.88.244.245
display bgp peer 100.88.244.247
display bgp peer 100.88.244.249
display bgp peer 100.88.244.25
display bgp peer 100.88.244.251
display bgp peer 100.88.244.253
display bgp peer 100.88.244.255
display bgp peer 100.88.244.27
display bgp peer 100.88.244.29
display bgp peer 100.88.244.31
display bgp peer 100.88.244.33
display bgp peer 100.88.244.35
display bgp peer 100.88.244.37
display bgp peer 100.88.244.39
display bgp peer 100.88.244.41
display bgp peer 100.88.244.43
display bgp peer 100.88.244.45
display bgp peer 100.88.244.47
display bgp peer 100.88.244.49
display bgp peer 100.88.244.5
display bgp peer 100.88.244.51
display bgp peer 100.88.244.53
display bgp peer 100.88.244.55
display bgp peer 100.88.244.57
display bgp peer 100.88.244.59
display bgp peer 100.88.244.61
display bgp peer 100.88.244.63
display bgp peer 100.88.244.65
display bgp peer 100.88.244.67
display bgp peer 100.88.244.69
display bgp peer 100.88.244.7
display bgp peer 100.88.244.71
display bgp peer 100.88.244.73
display bgp peer 100.88.244.75
display bgp peer 100.88.244.77
display bgp peer 100.88.244.79
display bgp peer 100.88.244.81
display bgp peer 100.88.244.83
display bgp peer 100.88.244.85
display bgp peer 100.88.244.87
display bgp peer 100.88.244.89
display bgp peer 100.88.244.9
display bgp peer 100.88.244.91
display bgp peer 100.88.244.93
display bgp peer 100.88.244.95
display bgp peer 100.88.244.97
display bgp peer 100.88.244.99
display bgp peer 100.88.245.101
display bgp peer 100.88.245.103
display bgp peer 100.88.245.105
display bgp peer 100.88.245.107
display bgp peer 100.88.245.109
display bgp peer 100.88.245.11
display bgp peer 100.88.245.111
display bgp peer 100.88.245.113
display bgp peer 100.88.245.115
display bgp peer 100.88.245.117
display bgp peer 100.88.245.119
display bgp peer 100.88.245.121
display bgp peer 100.88.245.123
display bgp peer 100.88.245.125
display bgp peer 100.88.245.127
display bgp peer 100.88.245.129
display bgp peer 100.88.245.13
display bgp peer 100.88.245.131
display bgp peer 100.88.245.133
display bgp peer 100.88.245.135
display bgp peer 100.88.245.137
display bgp peer 100.88.245.139
display bgp peer 100.88.245.141
display bgp peer 100.88.245.143
display bgp peer 100.88.245.145
display bgp peer 100.88.245.147
display bgp peer 100.88.245.149
display bgp peer 100.88.245.15
display bgp peer 100.88.245.151
display bgp peer 100.88.245.153
display bgp peer 100.88.245.155
display bgp peer 100.88.245.157
display bgp peer 100.88.245.159
display bgp peer 100.88.245.161
display bgp peer 100.88.245.163
display bgp peer 100.88.245.165
display bgp peer 100.88.245.167
display bgp peer 100.88.245.169
display bgp peer 100.88.245.17
display bgp peer 100.88.245.171
display bgp peer 100.88.245.173
display bgp peer 100.88.245.175
display bgp peer 100.88.245.177
display bgp peer 100.88.245.179
display bgp peer 100.88.245.181
display bgp peer 100.88.245.183
display bgp peer 100.88.245.185
display bgp peer 100.88.245.187
display bgp peer 100.88.245.189
display bgp peer 100.88.245.19
display bgp peer 100.88.245.191
display bgp peer 100.88.245.193
display bgp peer 100.88.245.195
display bgp peer 100.88.245.21
display bgp peer 100.88.245.23
display bgp peer 100.88.245.25
display bgp peer 100.88.245.27
display bgp peer 100.88.245.29
display bgp peer 100.88.245.31
display bgp peer 100.88.245.33
display bgp peer 100.88.245.35
display bgp peer 100.88.245.37
display bgp peer 100.88.245.39
display bgp peer 100.88.245.41
display bgp peer 100.88.245.43
display bgp peer 100.88.245.45
display bgp peer 100.88.245.47
display bgp peer 100.88.245.49
display bgp peer 100.88.245.51
display bgp peer 100.88.245.53
display bgp peer 100.88.245.55
display bgp peer 100.88.245.57
display bgp peer 100.88.245.59
display bgp peer 100.88.245.61
display bgp peer 100.88.245.63
display bgp peer 100.88.245.65
display bgp peer 100.88.245.67
display bgp peer 100.88.245.69
display bgp peer 100.88.245.71
display bgp peer 100.88.245.73
display bgp peer 100.88.245.75
display bgp peer 100.88.245.77
display bgp peer 100.88.245.79
display bgp peer 100.88.245.81
display bgp peer 100.88.245.83
display bgp peer 100.88.245.85
display bgp peer 100.88.245.87
display bgp peer 100.88.245.89
display bgp peer 100.88.245.9
display bgp peer 100.88.245.91
display bgp peer 100.88.245.93
display bgp peer 100.88.245.95
display bgp peer 100.88.245.97
display bgp peer 100.88.245.99
display bgp peer | include Established
display bgp routing-table statistics
```

### [cd-gx-0201-g17-h12516af-lc-01] BGP 隔离 — XGWL (8 peers, AS 4 ASes)

```
system-view
bgp 65508
peer 100.88.247.129 ignore  # CD-GX-0201-F21-H6800QT-XGWL-01
peer 100.88.247.131 ignore  # CD-GX-0201-H18-H6800QT-XGWL-01
peer 100.88.247.133 ignore  # CD-GX-0201-H01-H6800QT-XGWL-01
peer 100.88.247.135 ignore  # CD-GX-0201-J21-H6800QT-XGWL-01
peer 100.88.247.137 ignore  # CD-GX-0201-H20-H6800QT-XGWL-01
peer 100.88.247.139 ignore  # CD-GX-0201-P20-H6800QT-XGWL-01
peer 100.88.247.141 ignore  # CD-GX-0201-N20-H6800QT-XGWL-01
peer 100.88.247.143 ignore  # CD-GX-0201-P18-H6800QT-XGWL-01
quit
return
```

### [cd-gx-0201-g17-h12516af-lc-01] >>> 检查点: XGWL 组 peers 应变为 Idle <<<

```
display bgp peer 100.88.247.129
display bgp peer 100.88.247.131
display bgp peer 100.88.247.133
display bgp peer 100.88.247.135
display bgp peer 100.88.247.137
display bgp peer 100.88.247.139
display bgp peer 100.88.247.141
display bgp peer 100.88.247.143
display bgp peer | include Established
display bgp routing-table statistics
```

### [cd-gx-0201-g17-h12516af-lc-01] BGP 隔离 — SDN-Controller-Read (1 peers, AS 65508)

```
system-view
bgp 65508
peer 9.230.220.65 ignore  # SDN-Controller-Read
quit
return
```

### [cd-gx-0201-g17-h12516af-lc-01] >>> 检查点: SDN-Controller-Read 组 peers 应变为 Idle <<<

```
display bgp peer 9.230.220.65
display bgp peer | include Established
display bgp routing-table statistics
```

### [cd-gx-0201-g17-h12516af-lc-01] BGP 隔离 — SDN-Controller-Write (1 peers, AS 65508)

```
system-view
bgp 65508
peer 9.230.220.74 ignore  # SDN-Controller-Write
quit
return
```

### [cd-gx-0201-g17-h12516af-lc-01] >>> 检查点: SDN-Controller-Write 组 peers 应变为 Idle <<<

```
display bgp peer 9.230.220.74
display bgp peer | include Established
display bgp routing-table statistics
```

## 阶段3: 接口级隔离

关闭 LAG 上联接口，再关闭物理下联接口

### [cd-gx-0201-g17-h12516af-lc-01] 接口隔离 — 关闭 233 个物理下联

```
system-view
interface FortyGigE10/0/1  # CD-GX-0307-N07-H6800QT-LA-02-FG1/0/50
shutdown
quit
interface FortyGigE10/0/2  # CD-GX-0307-N08-H6800QT-LA-02-FG1/0/50
shutdown
quit
interface FortyGigE2/0/1  # CD-GX-0201-R08-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE2/0/10  # CD-GX-0201-S08-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE2/0/11  # CD-GX-0201-S09-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE2/0/12  # CD-GX-0201-H18-H6800QT-XGWL-01-FG1/0/50
shutdown
quit
interface FortyGigE2/0/13  # CD-GX-0201-S11-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE2/0/14  # CD-GX-0201-S12-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE2/0/15  # CD-GX-0201-S13-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE2/0/16  # CD-GX-0201-S14-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE2/0/17  # CD-GX-0201-S16-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE2/0/19  # CD-GX-0201-S17-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE2/0/2  # CD-GX-0201-R09-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE2/0/20  # CD-GX-0201-S18-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE2/0/21  # CD-GX-0201-S19-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE2/0/22  # CD-GX-0201-S21-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE2/0/23  # CD-GX-0201-R21-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE2/0/25  # CD-GX-0201-D01-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE2/0/26  # CD-GX-0201-D02-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE2/0/27  # CD-GX-0201-D04-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE2/0/28  # CD-GX-0201-D05-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE2/0/29  # CD-GX-0201-D06-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE2/0/3  # CD-GX-0201-R13-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE2/0/31  # CD-GX-0201-D07-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE2/0/32  # CD-GX-0201-D09-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE2/0/33  # CD-GX-0201-D10-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE2/0/4  # CD-GX-0201-R14-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE2/0/5  # CD-GX-0201-R15-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE2/0/6  # CD-GX-0201-F21-H6800QT-XGWL-01-FG1/0/50
shutdown
quit
interface FortyGigE2/0/7  # CD-GX-0201-R16-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE2/0/8  # CD-GX-0201-R18-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE2/0/9  # CD-GX-0201-R19-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE3/0/1  # CD-GX-0201-D13-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE3/0/10  # CD-GX-0201-E01-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE3/0/11  # CD-GX-0201-E02-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE3/0/12  # CD-GX-0201-J21-H6800QT-XGWL-01-FG1/0/50
shutdown
quit
interface FortyGigE3/0/13  # CD-GX-0201-E04-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE3/0/14  # CD-GX-0201-E05-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE3/0/15  # CD-GX-0201-E06-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE3/0/16  # CD-GX-0201-E07-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE3/0/17  # CD-GX-0201-E09-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE3/0/19  # CD-GX-0201-E10-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE3/0/2  # CD-GX-0201-D14-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE3/0/20  # CD-GX-0201-E11-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE3/0/21  # CD-GX-0201-E12-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE3/0/22  # CD-GX-0201-E14-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE3/0/23  # CD-GX-0201-E15-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE3/0/25  # CD-GX-0201-E16-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE3/0/26  # CD-GX-0201-E17-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE3/0/27  # CD-GX-0201-E19-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE3/0/28  # CD-GX-0201-E20-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE3/0/29  # CD-GX-0201-F01-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE3/0/3  # CD-GX-0201-D16-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE3/0/31  # CD-GX-0201-F02-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE3/0/32  # CD-GX-0201-F04-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE3/0/33  # CD-GX-0201-F05-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE3/0/4  # CD-GX-0201-D17-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE3/0/5  # CD-GX-0201-D18-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE3/0/6  # CD-GX-0201-H01-H6800QT-XGWL-01-FG1/0/50
shutdown
quit
interface FortyGigE3/0/7  # CD-GX-0201-D19-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE3/0/8  # CD-GX-0201-D21-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE3/0/9  # CD-GX-0201-E21-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE4/0/1  # CD-GX-0201-F06-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE4/0/10  # CD-GX-0201-F16-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE4/0/11  # CD-GX-0201-F17-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE4/0/12  # CD-GX-0201-P20-H6800QT-XGWL-01-FG1/0/50
shutdown
quit
interface FortyGigE4/0/13  # CD-GX-0201-F19-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE4/0/14  # CD-GX-0201-F20-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE4/0/15  # CD-GX-0201-G01-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE4/0/16  # CD-GX-0201-G02-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE4/0/17  # CD-GX-0201-G04-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE4/0/19  # CD-GX-0201-G05-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE4/0/2  # CD-GX-0201-F07-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE4/0/20  # CD-GX-0201-G06-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE4/0/21  # CD-GX-0201-G07-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE4/0/22  # CD-GX-0201-G09-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE4/0/23  # CD-GX-0201-G10-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE4/0/25  # CD-GX-0201-H13-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE4/0/26  # CD-GX-0201-H14-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE4/0/27  # CD-GX-0201-H16-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE4/0/28  # CD-GX-0201-H17-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE4/0/29  # CD-GX-0201-J01-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE4/0/3  # CD-GX-0201-F09-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE4/0/31  # CD-GX-0201-J02-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE4/0/32  # CD-GX-0201-J04-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE4/0/33  # CD-GX-0201-J05-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE4/0/4  # CD-GX-0201-F10-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE4/0/5  # CD-GX-0201-F11-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE4/0/6  # CD-GX-0201-H20-H6800QT-XGWL-01-FG1/0/50
shutdown
quit
interface FortyGigE4/0/7  # CD-GX-0201-F12-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE4/0/8  # CD-GX-0201-F14-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE4/0/9  # CD-GX-0201-F15-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE5/0/1  # CD-GX-0201-J06-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE5/0/10  # CD-GX-0201-J16-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE5/0/11  # CD-GX-0201-J17-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE5/0/12  # CD-GX-0201-P18-H6800QT-XGWL-01-FG1/0/50
shutdown
quit
interface FortyGigE5/0/13  # CD-GX-0201-J19-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE5/0/14  # CD-GX-0201-J20-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE5/0/15  # CD-GX-0201-K01-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE5/0/16  # CD-GX-0201-K02-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE5/0/17  # CD-GX-0201-K04-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE5/0/19  # CD-GX-0201-K05-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE5/0/2  # CD-GX-0201-J07-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE5/0/20  # CD-GX-0201-K06-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE5/0/21  # CD-GX-0201-K07-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE5/0/22  # CD-GX-0201-K09-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE5/0/23  # CD-GX-0201-K10-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE5/0/25  # CD-GX-0201-K11-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE5/0/26  # CD-GX-0201-K12-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE5/0/27  # CD-GX-0201-K14-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE5/0/28  # CD-GX-0201-K15-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE5/0/29  # CD-GX-0201-K16-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE5/0/3  # CD-GX-0201-J09-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE5/0/31  # CD-GX-0201-K17-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE5/0/32  # CD-GX-0201-K19-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE5/0/33  # CD-GX-0201-K20-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE5/0/4  # CD-GX-0201-J10-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE5/0/5  # CD-GX-0201-J11-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE5/0/6  # CD-GX-0201-N20-H6800QT-XGWL-01-FG1/0/50
shutdown
quit
interface FortyGigE5/0/7  # CD-GX-0201-J12-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE5/0/8  # CD-GX-0201-J14-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE5/0/9  # CD-GX-0201-J15-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE6/0/1  # CD-GX-0307-K01-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE6/0/10  # CD-GX-0307-K11-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE6/0/11  # CD-GX-0307-K12-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE6/0/13  # CD-GX-0307-K14-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE6/0/14  # CD-GX-0307-K15-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE6/0/15  # CD-GX-0307-K16-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE6/0/16  # CD-GX-0307-K17-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE6/0/17  # CD-GX-0307-K19-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE6/0/19  # CD-GX-0307-K20-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE6/0/2  # CD-GX-0307-K02-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE6/0/25  # CD-GX-0201-N08-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE6/0/26  # CD-GX-0201-N09-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE6/0/27  # CD-GX-0201-N13-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE6/0/28  # CD-GX-0201-N14-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE6/0/29  # CD-GX-0201-N15-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE6/0/3  # CD-GX-0307-K04-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE6/0/31  # CD-GX-0201-N16-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE6/0/32  # CD-GX-0201-N18-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE6/0/33  # CD-GX-0201-N19-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE6/0/4  # CD-GX-0307-K05-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE6/0/5  # CD-GX-0307-K06-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE6/0/7  # CD-GX-0307-K07-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE6/0/8  # CD-GX-0307-K09-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE6/0/9  # CD-GX-0307-K10-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE7/0/1  # CD-GX-0201-P08-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE7/0/10  # CD-GX-0201-Q08-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE7/0/11  # CD-GX-0201-Q09-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE7/0/13  # CD-GX-0201-Q11-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE7/0/14  # CD-GX-0201-Q12-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE7/0/15  # CD-GX-0201-Q13-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE7/0/16  # CD-GX-0201-Q14-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE7/0/17  # CD-GX-0201-Q16-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE7/0/19  # CD-GX-0201-Q17-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE7/0/2  # CD-GX-0201-P09-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE7/0/20  # CD-GX-0201-Q18-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE7/0/21  # CD-GX-0201-Q19-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE7/0/22  # CD-GX-0201-Q21-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE7/0/23  # CD-GX-0201-R20-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE7/0/25  # CD-GX-0307-A14-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE7/0/26  # CD-GX-0307-A15-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE7/0/27  # CD-GX-0307-A17-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE7/0/28  # CD-GX-0307-A18-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE7/0/29  # CD-GX-0307-A20-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE7/0/3  # CD-GX-0201-P11-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE7/0/31  # CD-GX-0307-A21-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE7/0/32  # CD-GX-0307-C10-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE7/0/33  # CD-GX-0307-C11-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE7/0/4  # CD-GX-0201-P12-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE7/0/5  # CD-GX-0201-P13-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE7/0/7  # CD-GX-0201-P14-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE7/0/8  # CD-GX-0201-P16-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE7/0/9  # CD-GX-0201-P17-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE8/0/1  # CD-GX-0307-D01-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE8/0/10  # CD-GX-0307-D14-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE8/0/11  # CD-GX-0307-D15-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE8/0/13  # CD-GX-0307-D17-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE8/0/14  # CD-GX-0307-D18-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE8/0/15  # CD-GX-0307-D20-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE8/0/16  # CD-GX-0307-D21-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE8/0/17  # CD-GX-0307-E02-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE8/0/19  # CD-GX-0307-E03-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE8/0/2  # CD-GX-0307-D02-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE8/0/20  # CD-GX-0307-E05-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE8/0/21  # CD-GX-0307-E06-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE8/0/22  # CD-GX-0307-E14-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE8/0/23  # CD-GX-0307-E15-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE8/0/25  # CD-GX-0307-E17-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE8/0/26  # CD-GX-0307-E18-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE8/0/27  # CD-GX-0307-E20-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE8/0/28  # CD-GX-0307-E21-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE8/0/29  # CD-GX-0307-F01-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE8/0/3  # CD-GX-0307-D04-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE8/0/31  # CD-GX-0307-F02-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE8/0/32  # CD-GX-0307-F04-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE8/0/33  # CD-GX-0307-F05-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE8/0/4  # CD-GX-0307-D05-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE8/0/5  # CD-GX-0307-D06-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE8/0/7  # CD-GX-0307-D07-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE8/0/8  # CD-GX-0307-D09-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE8/0/9  # CD-GX-0307-D10-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE9/0/1  # CD-GX-0307-F16-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE9/0/10  # CD-GX-0307-G06-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE9/0/11  # CD-GX-0307-G07-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE9/0/13  # CD-GX-0307-G09-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE9/0/14  # CD-GX-0307-G10-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE9/0/15  # CD-GX-0307-G11-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE9/0/16  # CD-GX-0307-G12-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE9/0/17  # CD-GX-0307-G14-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE9/0/19  # CD-GX-0307-G15-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE9/0/2  # CD-GX-0307-F17-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE9/0/20  # CD-GX-0307-G16-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE9/0/21  # CD-GX-0307-G17-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE9/0/22  # CD-GX-0307-G19-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE9/0/23  # CD-GX-0307-G20-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE9/0/25  # CD-GX-0307-J01-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE9/0/26  # CD-GX-0307-J02-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE9/0/27  # CD-GX-0307-N07-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE9/0/28  # CD-GX-0307-N08-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE9/0/29  # CD-GX-0307-N10-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE9/0/3  # CD-GX-0307-F19-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE9/0/31  # CD-GX-0307-N11-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE9/0/32  # CD-GX-0307-J01-H6800QT-LA-02-FG1/0/50
shutdown
quit
interface FortyGigE9/0/33  # CD-GX-0307-J02-H6800QT-LA-02-FG1/0/50
shutdown
quit
interface FortyGigE9/0/4  # CD-GX-0307-F20-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE9/0/5  # CD-GX-0307-G01-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE9/0/7  # CD-GX-0307-G02-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE9/0/8  # CD-GX-0307-G04-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface FortyGigE9/0/9  # CD-GX-0307-G05-H6800QT-LA-01-FG1/0/50
shutdown
quit
interface M-GigabitEthernet0/0/2  # Out-Of-Band-Management
shutdown
quit
interface Route-Aggregation1  # CD-GX-0201-H10-HW12816-QCMAN-01
shutdown
quit
interface Route-Aggregation2  # CD-GX-0201-G13-HW12816-QCMAN-02
shutdown
quit
return
```

### [cd-gx-0201-g17-h12516af-lc-01] 验证接口隔离后设备状态

```
display interface brief
display ip routing-table statistics
```

## 阶段4: 变更后检查

确认设备已完全隔离，各协议邻居已中断

### [cd-gx-0201-g17-h12516af-lc-01] 确认设备已完全隔离

```
display interface brief
display ip routing-table statistics
display bgp peer
display bgp peer | include Established
```

## 阶段5: 回退方案

按 management → uplink → downlink 顺序撤销 BGP 隔离，再恢复接口

### [cd-gx-0201-g17-h12516af-lc-01] BGP 回退 — 恢复 SDN-Controller-Write (1 peers)

```
system-view
bgp 65508
undo peer 9.230.220.74 ignore  # SDN-Controller-Write
quit
return
```

### [cd-gx-0201-g17-h12516af-lc-01] BGP 回退 — 恢复 SDN-Controller-Read (1 peers)

```
system-view
bgp 65508
undo peer 9.230.220.65 ignore  # SDN-Controller-Read
quit
return
```

### [cd-gx-0201-g17-h12516af-lc-01] BGP 回退 — 恢复 XGWL (8 peers)

```
system-view
bgp 65508
undo peer 100.88.247.129 ignore  # CD-GX-0201-F21-H6800QT-XGWL-01
undo peer 100.88.247.131 ignore  # CD-GX-0201-H18-H6800QT-XGWL-01
undo peer 100.88.247.133 ignore  # CD-GX-0201-H01-H6800QT-XGWL-01
undo peer 100.88.247.135 ignore  # CD-GX-0201-J21-H6800QT-XGWL-01
undo peer 100.88.247.137 ignore  # CD-GX-0201-H20-H6800QT-XGWL-01
undo peer 100.88.247.139 ignore  # CD-GX-0201-P20-H6800QT-XGWL-01
undo peer 100.88.247.141 ignore  # CD-GX-0201-N20-H6800QT-XGWL-01
undo peer 100.88.247.143 ignore  # CD-GX-0201-P18-H6800QT-XGWL-01
quit
return
```

### [cd-gx-0201-g17-h12516af-lc-01] BGP 回退 — 恢复 LA2~448 (220 peers)

```
system-view
bgp 65508
undo peer 100.88.244.101 ignore  # CD-GX-0201-E19-H6800QT-LA-01
undo peer 100.88.244.103 ignore  # CD-GX-0201-E20-H6800QT-LA-01
undo peer 100.88.244.105 ignore  # CD-GX-0201-F01-H6800QT-LA-01
undo peer 100.88.244.107 ignore  # CD-GX-0201-F02-H6800QT-LA-01
undo peer 100.88.244.109 ignore  # CD-GX-0201-F04-H6800QT-LA-01
undo peer 100.88.244.11 ignore  # CD-GX-0201-R16-H6800QT-LA-01
undo peer 100.88.244.111 ignore  # CD-GX-0201-F05-H6800QT-LA-01
undo peer 100.88.244.113 ignore  # CD-GX-0201-F06-H6800QT-LA-01
undo peer 100.88.244.115 ignore  # CD-GX-0201-F07-H6800QT-LA-01
undo peer 100.88.244.117 ignore  # CD-GX-0201-F09-H6800QT-LA-01
undo peer 100.88.244.119 ignore  # CD-GX-0201-F10-H6800QT-LA-01
undo peer 100.88.244.121 ignore  # CD-GX-0201-F11-H6800QT-LA-01
undo peer 100.88.244.123 ignore  # CD-GX-0201-F12-H6800QT-LA-01
undo peer 100.88.244.125 ignore  # CD-GX-0201-F14-H6800QT-LA-01
undo peer 100.88.244.127 ignore  # CD-GX-0201-F15-H6800QT-LA-01
undo peer 100.88.244.129 ignore  # CD-GX-0201-F16-H6800QT-LA-01
undo peer 100.88.244.13 ignore  # CD-GX-0201-R18-H6800QT-LA-01
undo peer 100.88.244.131 ignore  # CD-GX-0201-F17-H6800QT-LA-01
undo peer 100.88.244.133 ignore  # CD-GX-0201-F19-H6800QT-LA-01
undo peer 100.88.244.135 ignore  # CD-GX-0201-F20-H6800QT-LA-01
undo peer 100.88.244.137 ignore  # CD-GX-0201-G01-H6800QT-LA-01
undo peer 100.88.244.139 ignore  # CD-GX-0201-G02-H6800QT-LA-01
undo peer 100.88.244.141 ignore  # CD-GX-0201-G04-H6800QT-LA-01
undo peer 100.88.244.143 ignore  # CD-GX-0201-G05-H6800QT-LA-01
undo peer 100.88.244.145 ignore  # CD-GX-0201-G06-H6800QT-LA-01
undo peer 100.88.244.147 ignore  # CD-GX-0201-G07-H6800QT-LA-01
undo peer 100.88.244.149 ignore  # CD-GX-0201-G09-H6800QT-LA-01
undo peer 100.88.244.15 ignore  # CD-GX-0201-R19-H6800QT-LA-01
undo peer 100.88.244.151 ignore  # CD-GX-0201-G10-H6800QT-LA-01
undo peer 100.88.244.153 ignore  # CD-GX-0201-H13-H6800QT-LA-01
undo peer 100.88.244.155 ignore  # CD-GX-0201-H14-H6800QT-LA-01
undo peer 100.88.244.157 ignore  # CD-GX-0201-H16-H6800QT-LA-01
undo peer 100.88.244.159 ignore  # CD-GX-0201-H17-H6800QT-LA-01
undo peer 100.88.244.161 ignore  # CD-GX-0201-J01-H6800QT-LA-01
undo peer 100.88.244.163 ignore  # CD-GX-0201-J02-H6800QT-LA-01
undo peer 100.88.244.165 ignore  # CD-GX-0201-J04-H6800QT-LA-01
undo peer 100.88.244.167 ignore  # CD-GX-0201-J05-H6800QT-LA-01
undo peer 100.88.244.169 ignore  # CD-GX-0201-J06-H6800QT-LA-01
undo peer 100.88.244.17 ignore  # CD-GX-0201-S08-H6800QT-LA-01
undo peer 100.88.244.171 ignore  # CD-GX-0201-J07-H6800QT-LA-01
undo peer 100.88.244.173 ignore  # CD-GX-0201-J09-H6800QT-LA-01
undo peer 100.88.244.175 ignore  # CD-GX-0201-J10-H6800QT-LA-01
undo peer 100.88.244.177 ignore  # CD-GX-0201-J11-H6800QT-LA-01
undo peer 100.88.244.179 ignore  # CD-GX-0201-J12-H6800QT-LA-01
undo peer 100.88.244.181 ignore  # CD-GX-0201-J14-H6800QT-LA-01
undo peer 100.88.244.183 ignore  # CD-GX-0201-J15-H6800QT-LA-01
undo peer 100.88.244.185 ignore  # CD-GX-0201-J16-H6800QT-LA-01
undo peer 100.88.244.187 ignore  # CD-GX-0201-J17-H6800QT-LA-01
undo peer 100.88.244.189 ignore  # CD-GX-0201-J19-H6800QT-LA-01
undo peer 100.88.244.19 ignore  # CD-GX-0201-S09-H6800QT-LA-01
undo peer 100.88.244.191 ignore  # CD-GX-0201-J20-H6800QT-LA-01
undo peer 100.88.244.193 ignore  # CD-GX-0201-K01-H6800QT-LA-01
undo peer 100.88.244.195 ignore  # CD-GX-0201-K02-H6800QT-LA-01
undo peer 100.88.244.197 ignore  # CD-GX-0201-K04-H6800QT-LA-01
undo peer 100.88.244.199 ignore  # CD-GX-0201-K05-H6800QT-LA-01
undo peer 100.88.244.201 ignore  # CD-GX-0201-K06-H6800QT-LA-01
undo peer 100.88.244.203 ignore  # CD-GX-0201-K07-H6800QT-LA-01
undo peer 100.88.244.205 ignore  # CD-GX-0201-K09-H6800QT-LA-01
undo peer 100.88.244.207 ignore  # CD-GX-0201-K10-H6800QT-LA-01
undo peer 100.88.244.209 ignore  # CD-GX-0201-K11-H6800QT-LA-01
undo peer 100.88.244.21 ignore  # CD-GX-0201-S11-H6800QT-LA-01
undo peer 100.88.244.211 ignore  # CD-GX-0201-K12-H6800QT-LA-01
undo peer 100.88.244.213 ignore  # CD-GX-0201-K14-H6800QT-LA-01
undo peer 100.88.244.215 ignore  # CD-GX-0201-K15-H6800QT-LA-01
undo peer 100.88.244.217 ignore  # CD-GX-0201-K16-H6800QT-LA-01
undo peer 100.88.244.219 ignore  # CD-GX-0201-K17-H6800QT-LA-01
undo peer 100.88.244.221 ignore  # CD-GX-0201-K19-H6800QT-LA-01
undo peer 100.88.244.223 ignore  # CD-GX-0201-K20-H6800QT-LA-01
undo peer 100.88.244.225 ignore  # CD-GX-0307-K01-H6800QT-LA-01
undo peer 100.88.244.227 ignore  # CD-GX-0307-K02-H6800QT-LA-01
undo peer 100.88.244.229 ignore  # CD-GX-0307-K04-H6800QT-LA-01
undo peer 100.88.244.23 ignore  # CD-GX-0201-S12-H6800QT-LA-01
undo peer 100.88.244.231 ignore  # CD-GX-0307-K05-H6800QT-LA-01
undo peer 100.88.244.233 ignore  # CD-GX-0307-K06-H6800QT-LA-01
undo peer 100.88.244.235 ignore  # CD-GX-0307-K07-H6800QT-LA-01
undo peer 100.88.244.237 ignore  # CD-GX-0307-K09-H6800QT-LA-01
undo peer 100.88.244.239 ignore  # CD-GX-0307-K10-H6800QT-LA-01
undo peer 100.88.244.241 ignore  # CD-GX-0307-K11-H6800QT-LA-01
undo peer 100.88.244.243 ignore  # CD-GX-0307-K12-H6800QT-LA-01
undo peer 100.88.244.245 ignore  # CD-GX-0307-K14-H6800QT-LA-01
undo peer 100.88.244.247 ignore  # CD-GX-0307-K15-H6800QT-LA-01
undo peer 100.88.244.249 ignore  # CD-GX-0307-K16-H6800QT-LA-01
undo peer 100.88.244.25 ignore  # CD-GX-0201-S13-H6800QT-LA-01
undo peer 100.88.244.251 ignore  # CD-GX-0307-K17-H6800QT-LA-01
undo peer 100.88.244.253 ignore  # CD-GX-0307-K19-H6800QT-LA-01
undo peer 100.88.244.255 ignore  # CD-GX-0307-K20-H6800QT-LA-01
undo peer 100.88.244.27 ignore  # CD-GX-0201-S14-H6800QT-LA-01
undo peer 100.88.244.29 ignore  # CD-GX-0201-S16-H6800QT-LA-01
undo peer 100.88.244.31 ignore  # CD-GX-0201-S17-H6800QT-LA-01
undo peer 100.88.244.33 ignore  # CD-GX-0201-S18-H6800QT-LA-01
undo peer 100.88.244.35 ignore  # CD-GX-0201-S19-H6800QT-LA-01
undo peer 100.88.244.37 ignore  # CD-GX-0201-S21-H6800QT-LA-01
undo peer 100.88.244.39 ignore  # CD-GX-0201-R21-H6800QT-LA-01
undo peer 100.88.244.41 ignore  # CD-GX-0201-D01-H6800QT-LA-01
undo peer 100.88.244.43 ignore  # CD-GX-0201-D02-H6800QT-LA-01
undo peer 100.88.244.45 ignore  # CD-GX-0201-D04-H6800QT-LA-01
undo peer 100.88.244.47 ignore  # CD-GX-0201-D05-H6800QT-LA-01
undo peer 100.88.244.49 ignore  # CD-GX-0201-D06-H6800QT-LA-01
undo peer 100.88.244.5 ignore  # CD-GX-0201-R13-H6800QT-LA-01
undo peer 100.88.244.51 ignore  # CD-GX-0201-D07-H6800QT-LA-01
undo peer 100.88.244.53 ignore  # CD-GX-0201-D09-H6800QT-LA-01
undo peer 100.88.244.55 ignore  # CD-GX-0201-D10-H6800QT-LA-01
undo peer 100.88.244.57 ignore  # CD-GX-0201-D13-H6800QT-LA-01
undo peer 100.88.244.59 ignore  # CD-GX-0201-D14-H6800QT-LA-01
undo peer 100.88.244.61 ignore  # CD-GX-0201-D16-H6800QT-LA-01
undo peer 100.88.244.63 ignore  # CD-GX-0201-D17-H6800QT-LA-01
undo peer 100.88.244.65 ignore  # CD-GX-0201-D18-H6800QT-LA-01
undo peer 100.88.244.67 ignore  # CD-GX-0201-D19-H6800QT-LA-01
undo peer 100.88.244.69 ignore  # CD-GX-0201-D21-H6800QT-LA-01
undo peer 100.88.244.7 ignore  # CD-GX-0201-R14-H6800QT-LA-01
undo peer 100.88.244.71 ignore  # CD-GX-0201-E21-H6800QT-LA-01
undo peer 100.88.244.73 ignore  # CD-GX-0201-E01-H6800QT-LA-01
undo peer 100.88.244.75 ignore  # CD-GX-0201-E02-H6800QT-LA-01
undo peer 100.88.244.77 ignore  # CD-GX-0201-E04-H6800QT-LA-01
undo peer 100.88.244.79 ignore  # CD-GX-0201-E05-H6800QT-LA-01
undo peer 100.88.244.81 ignore  # CD-GX-0201-E06-H6800QT-LA-01
undo peer 100.88.244.83 ignore  # CD-GX-0201-E07-H6800QT-LA-01
undo peer 100.88.244.85 ignore  # CD-GX-0201-E09-H6800QT-LA-01
undo peer 100.88.244.87 ignore  # CD-GX-0201-E10-H6800QT-LA-01
undo peer 100.88.244.89 ignore  # CD-GX-0201-E11-H6800QT-LA-01
undo peer 100.88.244.9 ignore  # CD-GX-0201-R15-H6800QT-LA-01
undo peer 100.88.244.91 ignore  # CD-GX-0201-E12-H6800QT-LA-01
undo peer 100.88.244.93 ignore  # CD-GX-0201-E14-H6800QT-LA-01
undo peer 100.88.244.95 ignore  # CD-GX-0201-E15-H6800QT-LA-01
undo peer 100.88.244.97 ignore  # CD-GX-0201-E16-H6800QT-LA-01
undo peer 100.88.244.99 ignore  # CD-GX-0201-E17-H6800QT-LA-01
undo peer 100.88.245.101 ignore  # CD-GX-0307-D17-H6800QT-LA-01
undo peer 100.88.245.103 ignore  # CD-GX-0307-D18-H6800QT-LA-01
undo peer 100.88.245.105 ignore  # CD-GX-0307-D20-H6800QT-LA-01
undo peer 100.88.245.107 ignore  # CD-GX-0307-D21-H6800QT-LA-01
undo peer 100.88.245.109 ignore  # CD-GX-0307-E02-H6800QT-LA-01
undo peer 100.88.245.11 ignore  # CD-GX-0201-N09-H6800QT-LA-01
undo peer 100.88.245.111 ignore  # CD-GX-0307-E03-H6800QT-LA-01
undo peer 100.88.245.113 ignore  # CD-GX-0307-E05-H6800QT-LA-01
undo peer 100.88.245.115 ignore  # CD-GX-0307-E06-H6800QT-LA-01
undo peer 100.88.245.117 ignore  # CD-GX-0307-E14-H6800QT-LA-01
undo peer 100.88.245.119 ignore  # CD-GX-0307-E15-H6800QT-LA-01
undo peer 100.88.245.121 ignore  # CD-GX-0307-E17-H6800QT-LA-01
undo peer 100.88.245.123 ignore  # CD-GX-0307-E18-H6800QT-LA-01
undo peer 100.88.245.125 ignore  # CD-GX-0307-E20-H6800QT-LA-01
undo peer 100.88.245.127 ignore  # CD-GX-0307-E21-H6800QT-LA-01
undo peer 100.88.245.129 ignore  # CD-GX-0307-F01-H6800QT-LA-01
undo peer 100.88.245.13 ignore  # CD-GX-0201-N13-H6800QT-LA-01
undo peer 100.88.245.131 ignore  # CD-GX-0307-F02-H6800QT-LA-01
undo peer 100.88.245.133 ignore  # CD-GX-0307-F04-H6800QT-LA-01
undo peer 100.88.245.135 ignore  # CD-GX-0307-F05-H6800QT-LA-01
undo peer 100.88.245.137 ignore  # CD-GX-0307-F16-H6800QT-LA-01
undo peer 100.88.245.139 ignore  # CD-GX-0307-F17-H6800QT-LA-01
undo peer 100.88.245.141 ignore  # CD-GX-0307-F19-H6800QT-LA-01
undo peer 100.88.245.143 ignore  # CD-GX-0307-F20-H6800QT-LA-01
undo peer 100.88.245.145 ignore  # CD-GX-0307-G01-H6800QT-LA-01
undo peer 100.88.245.147 ignore  # CD-GX-0307-G02-H6800QT-LA-01
undo peer 100.88.245.149 ignore  # CD-GX-0307-G04-H6800QT-LA-01
undo peer 100.88.245.15 ignore  # CD-GX-0201-N14-H6800QT-LA-01
undo peer 100.88.245.151 ignore  # CD-GX-0307-G05-H6800QT-LA-01
undo peer 100.88.245.153 ignore  # CD-GX-0307-G06-H6800QT-LA-01
undo peer 100.88.245.155 ignore  # CD-GX-0307-G07-H6800QT-LA-01
undo peer 100.88.245.157 ignore  # CD-GX-0307-G09-H6800QT-LA-01
undo peer 100.88.245.159 ignore  # CD-GX-0307-G10-H6800QT-LA-01
undo peer 100.88.245.161 ignore  # CD-GX-0307-G11-H6800QT-LA-01
undo peer 100.88.245.163 ignore  # CD-GX-0307-G12-H6800QT-LA-01
undo peer 100.88.245.165 ignore  # CD-GX-0307-G14-H6800QT-LA-01
undo peer 100.88.245.167 ignore  # CD-GX-0307-G15-H6800QT-LA-01
undo peer 100.88.245.169 ignore  # CD-GX-0307-G16-H6800QT-LA-01
undo peer 100.88.245.17 ignore  # CD-GX-0201-N15-H6800QT-LA-01
undo peer 100.88.245.171 ignore  # CD-GX-0307-G17-H6800QT-LA-01
undo peer 100.88.245.173 ignore  # CD-GX-0307-G19-H6800QT-LA-01
undo peer 100.88.245.175 ignore  # CD-GX-0307-G20-H6800QT-LA-01
undo peer 100.88.245.177 ignore  # CD-GX-0307-J01-H6800QT-LA-01
undo peer 100.88.245.179 ignore  # CD-GX-0307-J02-H6800QT-LA-01
undo peer 100.88.245.181 ignore  # CD-GX-0307-N07-H6800QT-LA-01
undo peer 100.88.245.183 ignore  # CD-GX-0307-N08-H6800QT-LA-01
undo peer 100.88.245.185 ignore  # CD-GX-0307-N10-H6800QT-LA-01
undo peer 100.88.245.187 ignore  # CD-GX-0307-N11-H6800QT-LA-0
undo peer 100.88.245.189 ignore  # CD-GX-0307-J01-H6800QT-LA-02
undo peer 100.88.245.19 ignore  # CD-GX-0201-N16-H6800QT-LA-01
undo peer 100.88.245.191 ignore  # CD-GX-0307-J02-H6800QT-LA-02
undo peer 100.88.245.193 ignore  # CD-GX-0307-N07-H6800QT-LA-02
undo peer 100.88.245.195 ignore  # CD-GX-0307-N08-H6800QT-LA-02
undo peer 100.88.245.21 ignore  # CD-GX-0201-N18-H6800QT-LA-01
undo peer 100.88.245.23 ignore  # CD-GX-0201-N19-H6800QT-LA-01
undo peer 100.88.245.25 ignore  # CD-GX-0201-P08-H6800QT-LA-01
undo peer 100.88.245.27 ignore  # CD-GX-0201-P09-H6800QT-LA-01
undo peer 100.88.245.29 ignore  # CD-GX-0201-P11-H6800QT-LA-01
undo peer 100.88.245.31 ignore  # CD-GX-0201-P12-H6800QT-LA-01
undo peer 100.88.245.33 ignore  # CD-GX-0201-P13-H6800QT-LA-01
undo peer 100.88.245.35 ignore  # CD-GX-0201-P14-H6800QT-LA-01
undo peer 100.88.245.37 ignore  # CD-GX-0201-P16-H6800QT-LA-01
undo peer 100.88.245.39 ignore  # CD-GX-0201-P17-H6800QT-LA-01
undo peer 100.88.245.41 ignore  # CD-GX-0201-Q08-H6800QT-LA-01
undo peer 100.88.245.43 ignore  # CD-GX-0201-Q09-H6800QT-LA-01
undo peer 100.88.245.45 ignore  # CD-GX-0201-Q11-H6800QT-LA-01
undo peer 100.88.245.47 ignore  # CD-GX-0201-Q12-H6800QT-LA-01
undo peer 100.88.245.49 ignore  # CD-GX-0201-Q13-H6800QT-LA-01
undo peer 100.88.245.51 ignore  # CD-GX-0201-Q14-H6800QT-LA-01
undo peer 100.88.245.53 ignore  # CD-GX-0201-Q16-H6800QT-LA-01
undo peer 100.88.245.55 ignore  # CD-GX-0201-Q17-H6800QT-LA-01
undo peer 100.88.245.57 ignore  # CD-GX-0201-Q18-H6800QT-LA-01
undo peer 100.88.245.59 ignore  # CD-GX-0201-Q19-H6800QT-LA-01
undo peer 100.88.245.61 ignore  # CD-GX-0201-Q21-H6800QT-LA-01
undo peer 100.88.245.63 ignore  # CD-GX-0201-R20-H6800QT-LA-01
undo peer 100.88.245.65 ignore  # CD-GX-0307-A14-H6800QT-LA-01
undo peer 100.88.245.67 ignore  # CD-GX-0307-A15-H6800QT-LA-01
undo peer 100.88.245.69 ignore  # CD-GX-0307-A17-H6800QT-LA-01
undo peer 100.88.245.71 ignore  # CD-GX-0307-A18-H6800QT-LA-01
undo peer 100.88.245.73 ignore  # CD-GX-0307-A20-H6800QT-LA-01
undo peer 100.88.245.75 ignore  # CD-GX-0307-A21-H6800QT-LA-01
undo peer 100.88.245.77 ignore  # CD-GX-0307-C10-H6800QT-LA-01
undo peer 100.88.245.79 ignore  # CD-GX-0307-C11-H6800QT-LA-01
undo peer 100.88.245.81 ignore  # CD-GX-0307-D01-H6800QT-LA-01
undo peer 100.88.245.83 ignore  # CD-GX-0307-D02-H6800QT-LA-01
undo peer 100.88.245.85 ignore  # CD-GX-0307-D04-H6800QT-LA-01
undo peer 100.88.245.87 ignore  # CD-GX-0307-D05-H6800QT-LA-01
undo peer 100.88.245.89 ignore  # CD-GX-0307-D06-H6800QT-LA-01
undo peer 100.88.245.9 ignore  # CD-GX-0201-N08-H6800QT-LA-01
undo peer 100.88.245.91 ignore  # CD-GX-0307-D07-H6800QT-LA-01
undo peer 100.88.245.93 ignore  # CD-GX-0307-D09-H6800QT-LA-01
undo peer 100.88.245.95 ignore  # CD-GX-0307-D10-H6800QT-LA-01
undo peer 100.88.245.97 ignore  # CD-GX-0307-D14-H6800QT-LA-01
undo peer 100.88.245.99 ignore  # CD-GX-0307-D15-H6800QT-LA-01
quit
return
```

### [cd-gx-0201-g17-h12516af-lc-01] BGP 回退 — 恢复 LA1 (2 peers)

```
system-view
bgp 65508
undo peer 100.88.244.1 ignore  # CD-GX-0201-R08-H6800QT-LA-01
undo peer 100.88.244.3 ignore  # CD-GX-0201-R09-H6800QT-LA-01
quit
return
```

### [cd-gx-0201-g17-h12516af-lc-01] BGP 回退 — 恢复 QCDR (2 peers)

```
system-view
bgp 65508
undo peer 10.162.185.53 ignore  # CD-XQ803-0509-C14-H12516AF-QCDR-01
undo peer 10.162.185.69 ignore  # CD-XQ803-0510-C14-H12516AF-QCDR-02
quit
return
```

### [cd-gx-0201-g17-h12516af-lc-01] 接口恢复 — 开启 233 个物理下联

```
system-view
interface FortyGigE10/0/1
undo shutdown
quit
interface FortyGigE10/0/2
undo shutdown
quit
interface FortyGigE2/0/1
undo shutdown
quit
interface FortyGigE2/0/10
undo shutdown
quit
interface FortyGigE2/0/11
undo shutdown
quit
interface FortyGigE2/0/12
undo shutdown
quit
interface FortyGigE2/0/13
undo shutdown
quit
interface FortyGigE2/0/14
undo shutdown
quit
interface FortyGigE2/0/15
undo shutdown
quit
interface FortyGigE2/0/16
undo shutdown
quit
interface FortyGigE2/0/17
undo shutdown
quit
interface FortyGigE2/0/19
undo shutdown
quit
interface FortyGigE2/0/2
undo shutdown
quit
interface FortyGigE2/0/20
undo shutdown
quit
interface FortyGigE2/0/21
undo shutdown
quit
interface FortyGigE2/0/22
undo shutdown
quit
interface FortyGigE2/0/23
undo shutdown
quit
interface FortyGigE2/0/25
undo shutdown
quit
interface FortyGigE2/0/26
undo shutdown
quit
interface FortyGigE2/0/27
undo shutdown
quit
interface FortyGigE2/0/28
undo shutdown
quit
interface FortyGigE2/0/29
undo shutdown
quit
interface FortyGigE2/0/3
undo shutdown
quit
interface FortyGigE2/0/31
undo shutdown
quit
interface FortyGigE2/0/32
undo shutdown
quit
interface FortyGigE2/0/33
undo shutdown
quit
interface FortyGigE2/0/4
undo shutdown
quit
interface FortyGigE2/0/5
undo shutdown
quit
interface FortyGigE2/0/6
undo shutdown
quit
interface FortyGigE2/0/7
undo shutdown
quit
interface FortyGigE2/0/8
undo shutdown
quit
interface FortyGigE2/0/9
undo shutdown
quit
interface FortyGigE3/0/1
undo shutdown
quit
interface FortyGigE3/0/10
undo shutdown
quit
interface FortyGigE3/0/11
undo shutdown
quit
interface FortyGigE3/0/12
undo shutdown
quit
interface FortyGigE3/0/13
undo shutdown
quit
interface FortyGigE3/0/14
undo shutdown
quit
interface FortyGigE3/0/15
undo shutdown
quit
interface FortyGigE3/0/16
undo shutdown
quit
interface FortyGigE3/0/17
undo shutdown
quit
interface FortyGigE3/0/19
undo shutdown
quit
interface FortyGigE3/0/2
undo shutdown
quit
interface FortyGigE3/0/20
undo shutdown
quit
interface FortyGigE3/0/21
undo shutdown
quit
interface FortyGigE3/0/22
undo shutdown
quit
interface FortyGigE3/0/23
undo shutdown
quit
interface FortyGigE3/0/25
undo shutdown
quit
interface FortyGigE3/0/26
undo shutdown
quit
interface FortyGigE3/0/27
undo shutdown
quit
interface FortyGigE3/0/28
undo shutdown
quit
interface FortyGigE3/0/29
undo shutdown
quit
interface FortyGigE3/0/3
undo shutdown
quit
interface FortyGigE3/0/31
undo shutdown
quit
interface FortyGigE3/0/32
undo shutdown
quit
interface FortyGigE3/0/33
undo shutdown
quit
interface FortyGigE3/0/4
undo shutdown
quit
interface FortyGigE3/0/5
undo shutdown
quit
interface FortyGigE3/0/6
undo shutdown
quit
interface FortyGigE3/0/7
undo shutdown
quit
interface FortyGigE3/0/8
undo shutdown
quit
interface FortyGigE3/0/9
undo shutdown
quit
interface FortyGigE4/0/1
undo shutdown
quit
interface FortyGigE4/0/10
undo shutdown
quit
interface FortyGigE4/0/11
undo shutdown
quit
interface FortyGigE4/0/12
undo shutdown
quit
interface FortyGigE4/0/13
undo shutdown
quit
interface FortyGigE4/0/14
undo shutdown
quit
interface FortyGigE4/0/15
undo shutdown
quit
interface FortyGigE4/0/16
undo shutdown
quit
interface FortyGigE4/0/17
undo shutdown
quit
interface FortyGigE4/0/19
undo shutdown
quit
interface FortyGigE4/0/2
undo shutdown
quit
interface FortyGigE4/0/20
undo shutdown
quit
interface FortyGigE4/0/21
undo shutdown
quit
interface FortyGigE4/0/22
undo shutdown
quit
interface FortyGigE4/0/23
undo shutdown
quit
interface FortyGigE4/0/25
undo shutdown
quit
interface FortyGigE4/0/26
undo shutdown
quit
interface FortyGigE4/0/27
undo shutdown
quit
interface FortyGigE4/0/28
undo shutdown
quit
interface FortyGigE4/0/29
undo shutdown
quit
interface FortyGigE4/0/3
undo shutdown
quit
interface FortyGigE4/0/31
undo shutdown
quit
interface FortyGigE4/0/32
undo shutdown
quit
interface FortyGigE4/0/33
undo shutdown
quit
interface FortyGigE4/0/4
undo shutdown
quit
interface FortyGigE4/0/5
undo shutdown
quit
interface FortyGigE4/0/6
undo shutdown
quit
interface FortyGigE4/0/7
undo shutdown
quit
interface FortyGigE4/0/8
undo shutdown
quit
interface FortyGigE4/0/9
undo shutdown
quit
interface FortyGigE5/0/1
undo shutdown
quit
interface FortyGigE5/0/10
undo shutdown
quit
interface FortyGigE5/0/11
undo shutdown
quit
interface FortyGigE5/0/12
undo shutdown
quit
interface FortyGigE5/0/13
undo shutdown
quit
interface FortyGigE5/0/14
undo shutdown
quit
interface FortyGigE5/0/15
undo shutdown
quit
interface FortyGigE5/0/16
undo shutdown
quit
interface FortyGigE5/0/17
undo shutdown
quit
interface FortyGigE5/0/19
undo shutdown
quit
interface FortyGigE5/0/2
undo shutdown
quit
interface FortyGigE5/0/20
undo shutdown
quit
interface FortyGigE5/0/21
undo shutdown
quit
interface FortyGigE5/0/22
undo shutdown
quit
interface FortyGigE5/0/23
undo shutdown
quit
interface FortyGigE5/0/25
undo shutdown
quit
interface FortyGigE5/0/26
undo shutdown
quit
interface FortyGigE5/0/27
undo shutdown
quit
interface FortyGigE5/0/28
undo shutdown
quit
interface FortyGigE5/0/29
undo shutdown
quit
interface FortyGigE5/0/3
undo shutdown
quit
interface FortyGigE5/0/31
undo shutdown
quit
interface FortyGigE5/0/32
undo shutdown
quit
interface FortyGigE5/0/33
undo shutdown
quit
interface FortyGigE5/0/4
undo shutdown
quit
interface FortyGigE5/0/5
undo shutdown
quit
interface FortyGigE5/0/6
undo shutdown
quit
interface FortyGigE5/0/7
undo shutdown
quit
interface FortyGigE5/0/8
undo shutdown
quit
interface FortyGigE5/0/9
undo shutdown
quit
interface FortyGigE6/0/1
undo shutdown
quit
interface FortyGigE6/0/10
undo shutdown
quit
interface FortyGigE6/0/11
undo shutdown
quit
interface FortyGigE6/0/13
undo shutdown
quit
interface FortyGigE6/0/14
undo shutdown
quit
interface FortyGigE6/0/15
undo shutdown
quit
interface FortyGigE6/0/16
undo shutdown
quit
interface FortyGigE6/0/17
undo shutdown
quit
interface FortyGigE6/0/19
undo shutdown
quit
interface FortyGigE6/0/2
undo shutdown
quit
interface FortyGigE6/0/25
undo shutdown
quit
interface FortyGigE6/0/26
undo shutdown
quit
interface FortyGigE6/0/27
undo shutdown
quit
interface FortyGigE6/0/28
undo shutdown
quit
interface FortyGigE6/0/29
undo shutdown
quit
interface FortyGigE6/0/3
undo shutdown
quit
interface FortyGigE6/0/31
undo shutdown
quit
interface FortyGigE6/0/32
undo shutdown
quit
interface FortyGigE6/0/33
undo shutdown
quit
interface FortyGigE6/0/4
undo shutdown
quit
interface FortyGigE6/0/5
undo shutdown
quit
interface FortyGigE6/0/7
undo shutdown
quit
interface FortyGigE6/0/8
undo shutdown
quit
interface FortyGigE6/0/9
undo shutdown
quit
interface FortyGigE7/0/1
undo shutdown
quit
interface FortyGigE7/0/10
undo shutdown
quit
interface FortyGigE7/0/11
undo shutdown
quit
interface FortyGigE7/0/13
undo shutdown
quit
interface FortyGigE7/0/14
undo shutdown
quit
interface FortyGigE7/0/15
undo shutdown
quit
interface FortyGigE7/0/16
undo shutdown
quit
interface FortyGigE7/0/17
undo shutdown
quit
interface FortyGigE7/0/19
undo shutdown
quit
interface FortyGigE7/0/2
undo shutdown
quit
interface FortyGigE7/0/20
undo shutdown
quit
interface FortyGigE7/0/21
undo shutdown
quit
interface FortyGigE7/0/22
undo shutdown
quit
interface FortyGigE7/0/23
undo shutdown
quit
interface FortyGigE7/0/25
undo shutdown
quit
interface FortyGigE7/0/26
undo shutdown
quit
interface FortyGigE7/0/27
undo shutdown
quit
interface FortyGigE7/0/28
undo shutdown
quit
interface FortyGigE7/0/29
undo shutdown
quit
interface FortyGigE7/0/3
undo shutdown
quit
interface FortyGigE7/0/31
undo shutdown
quit
interface FortyGigE7/0/32
undo shutdown
quit
interface FortyGigE7/0/33
undo shutdown
quit
interface FortyGigE7/0/4
undo shutdown
quit
interface FortyGigE7/0/5
undo shutdown
quit
interface FortyGigE7/0/7
undo shutdown
quit
interface FortyGigE7/0/8
undo shutdown
quit
interface FortyGigE7/0/9
undo shutdown
quit
interface FortyGigE8/0/1
undo shutdown
quit
interface FortyGigE8/0/10
undo shutdown
quit
interface FortyGigE8/0/11
undo shutdown
quit
interface FortyGigE8/0/13
undo shutdown
quit
interface FortyGigE8/0/14
undo shutdown
quit
interface FortyGigE8/0/15
undo shutdown
quit
interface FortyGigE8/0/16
undo shutdown
quit
interface FortyGigE8/0/17
undo shutdown
quit
interface FortyGigE8/0/19
undo shutdown
quit
interface FortyGigE8/0/2
undo shutdown
quit
interface FortyGigE8/0/20
undo shutdown
quit
interface FortyGigE8/0/21
undo shutdown
quit
interface FortyGigE8/0/22
undo shutdown
quit
interface FortyGigE8/0/23
undo shutdown
quit
interface FortyGigE8/0/25
undo shutdown
quit
interface FortyGigE8/0/26
undo shutdown
quit
interface FortyGigE8/0/27
undo shutdown
quit
interface FortyGigE8/0/28
undo shutdown
quit
interface FortyGigE8/0/29
undo shutdown
quit
interface FortyGigE8/0/3
undo shutdown
quit
interface FortyGigE8/0/31
undo shutdown
quit
interface FortyGigE8/0/32
undo shutdown
quit
interface FortyGigE8/0/33
undo shutdown
quit
interface FortyGigE8/0/4
undo shutdown
quit
interface FortyGigE8/0/5
undo shutdown
quit
interface FortyGigE8/0/7
undo shutdown
quit
interface FortyGigE8/0/8
undo shutdown
quit
interface FortyGigE8/0/9
undo shutdown
quit
interface FortyGigE9/0/1
undo shutdown
quit
interface FortyGigE9/0/10
undo shutdown
quit
interface FortyGigE9/0/11
undo shutdown
quit
interface FortyGigE9/0/13
undo shutdown
quit
interface FortyGigE9/0/14
undo shutdown
quit
interface FortyGigE9/0/15
undo shutdown
quit
interface FortyGigE9/0/16
undo shutdown
quit
interface FortyGigE9/0/17
undo shutdown
quit
interface FortyGigE9/0/19
undo shutdown
quit
interface FortyGigE9/0/2
undo shutdown
quit
interface FortyGigE9/0/20
undo shutdown
quit
interface FortyGigE9/0/21
undo shutdown
quit
interface FortyGigE9/0/22
undo shutdown
quit
interface FortyGigE9/0/23
undo shutdown
quit
interface FortyGigE9/0/25
undo shutdown
quit
interface FortyGigE9/0/26
undo shutdown
quit
interface FortyGigE9/0/27
undo shutdown
quit
interface FortyGigE9/0/28
undo shutdown
quit
interface FortyGigE9/0/29
undo shutdown
quit
interface FortyGigE9/0/3
undo shutdown
quit
interface FortyGigE9/0/31
undo shutdown
quit
interface FortyGigE9/0/32
undo shutdown
quit
interface FortyGigE9/0/33
undo shutdown
quit
interface FortyGigE9/0/4
undo shutdown
quit
interface FortyGigE9/0/5
undo shutdown
quit
interface FortyGigE9/0/7
undo shutdown
quit
interface FortyGigE9/0/8
undo shutdown
quit
interface FortyGigE9/0/9
undo shutdown
quit
interface M-GigabitEthernet0/0/2
undo shutdown
quit
interface Route-Aggregation1
undo shutdown
quit
interface Route-Aggregation2
undo shutdown
quit
return
```

