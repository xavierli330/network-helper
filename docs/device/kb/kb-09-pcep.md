# 第九章 路径控制 PCEP

### 华为 NE40E

```
# PCE Server (算路中心)
pce
  pce-server
    ip-address <ip-address>
    peer-id <peer-id>

# PCE Client (路由器端)
pce
  pce-client
    connect-server <pce-server-ip>
    delegate { sr-te-policy | rsvp-te-tunnel }
    report { sr-te-policy | rsvp-te-tunnel }

# PCEP关联SR-TE Policy
segment-routing te
  pce-client
    connect-server <pce-ip>
    delegate sr-te-policy [ <name> ]

# PCEP关联SRv6-TE Policy
segment-routing ipv6 te
  pce-client
    connect-server <pce-ip>
    delegate srv6-te-policy [ <name> ]

# PCEP自动带宽调整
segment-routing te
  policy <name>
    auto-bandwidth
      threshold <percentage>
      max-bandwidth <bw-kbps>
      min-bandwidth <bw-kbps>
      adjust-interval <seconds>
```

### Cisco IOS-XR PCEP

```
# PCE Client (PCC)
segment-routing
  traffic-eng
    pcc
      source-address ipv4 <ip>
      pce address ipv4 <pce-ip>
        precedence <value>                          # 多PCE优先级
      report-all                                    # 上报所有SR-TE Policy
      autoroute include all

# PCE Server
pce
  address ipv4 <ip>
  segment-routing
    traffic-eng
      p2p
        policy <name>
          color <color> end-point ipv4 <ip>
          candidate-paths
            preference <pref>
              dynamic
                metric type { igp | te | latency }

# PCEP相关Show命令
Cisco: show pce ipv4 peer [ detail ]                # PCEP对等 ★
Cisco: show pce lsp [ detail | summary ]             # PCE管理的LSP
Cisco: show pce ipv4 topology [ summary ]            # PCE拓扑
Cisco: show segment-routing traffic-eng pcc ipv4 peer
```

### Juniper Junos PCEP

```
[edit protocols pcep]
pce <pce-name> {
    local-address <ip>;
    destination-ipv4-address <pce-ip>;
    destination-port <port>;                        # 默认4189
    pce-type { active | stateful };
    delegation-cleanup-timeout <seconds>;
    lsp-provisioning;                               # 允许PCE创建LSP
    spring-capability;                              # SR-TE能力
    pce-initiated-lsp;
    max-unknown-requests <count>;
    max-unknown-messages <count>;
}

# 全局PCEP选项
[edit protocols pcep]
disable-tlv { no-path-binding-tlv; }
```

```
华为: display pce peer [ <ip> ] [ verbose ]
华为: display pce lsp [ detail ]
华为: display pce delegation

Cisco: show pce ipv4 peer [ detail ]
Cisco: show pce lsp [ detail ]
Cisco: show pce ipv4 topology [ summary ]

Juniper: show path-computation-client active-pce    # 活动PCE ★
Juniper: show path-computation-client lsp           # PCE管理的LSP
Juniper: show path-computation-client statistics    # PCEP统计
```
