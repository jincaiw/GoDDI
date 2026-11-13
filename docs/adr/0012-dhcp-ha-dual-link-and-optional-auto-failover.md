# ADR-0012：DHCP 双机热备——双心跳链路、优先级/抢占、默认关闭的自动提升

- 状态：**Accepted（2026-11-13）**，已实现（自动提升默认关闭）
- 关联：ADR-0003（DHCP HA 契约）、ADR-0010（复制丢失服务策略）

## 背景

ADR-0003/ADR-0010 选择的是**单写者 + fail-closed**：主失去第二副本即 paused（不 ACK），
备永不自我提升，提升只能是运维动作。这是双节点、单链路下唯一正确的解，因为
单链路无法区分"对端宕机"与"网络分区"。

前提变化：两台虚拟主机由**两条独立网线**连接。两条同时断而本机仍存活，概率远低于
"对端真的宕机"，于是"对端不可达"可作为"对端已死"的可靠代理——这正是主流硬件防火墙
双机热备（VRRP/HRP、Cisco ASA failover、Fortinet FGCP、Palo Alto HA）采用的前提。

## 决策

在不削弱默认安全的前提下，补齐防火墙式 HA 的三块能力，**自动提升默认关闭**。

### 1. 双心跳链路
- 新增 `dhcp_ha.listen_addr_backup`（主端第二监听）、`dhcp_ha.peer_address_backup`（备端第二路径）。
- **任一链路可达即视为对端存活**；只有**所有**链路都不可达才计入失败。单线/NIC 故障不误判断线。
- 主端多监听但仍是**单会话**（第二监听是第二根网线，不是第二个对端）。

### 2. 优先级与抢占
- `dhcp_ha.priority`（默认 0，大者优先）：仅用于自动提升的仲裁。
- `dhcp_ha.preempt`（默认 **false**）：重新加入的高优先级节点默认**不**抢回主，避免链路抖动
  引发接管风暴。即使开启，抢回仍需通过 epoch/fencing 检查。

### 3. 自动提升（默认关闭）
- `dhcp_ha.auto_failover`（默认 **false**）+ `dhcp_ha.auto_failover_windows`（默认 3）。
- 开启后：备在**所有**路径连续失败达 N 个窗口后，调用与运维相同的 `Operator.Takeover`
  （epoch+1，接受已知 gap），**提升持久化**，进程退出，由 supervisor（systemd/容器 restart 策略）
  重启后以 primary 生效。
- 关闭时：完全保持 ADR-0003/ADR-0010 的 fail-closed——备永不自我提升。**默认行为零变化**。

## 为什么这样是安全的

- 主侧在失去备时已经 paused（不 ACK）——这是现有 fail-closed 设计。
- 因此"双链路全断 → 主 paused + 备提升"时，同一时刻**只有一个节点在服务**，不产生双写。
- 备提升后 epoch+1；旧主重连时被现有 `FrameStaleEpoch` + 启动 `ProbePeerEpoch` 降为 standby。
- 自动提升**默认关闭**：未明确选择该权衡的部署保持最保守语义。

## 残余风险（写入运维须知）

- 两条线同时被拔/交换机同时重启/宿主同时挂起仍会造成"主 paused + 备提升"；若主其实在服务
  旧连接，会有短暂不一致。缓解：双线分交换机/直连；生产启用 `preempt` 时配 `preempt_delay`；
  审计每次提升原因。
- 自动提升需 supervisor 重启进程才能生效（与既有"takeover 下次启动生效"一致）。

## 验收证据

- 单元/集成（真实 TCP）：`TestPeerAddrsListsEveryConfiguredPath`、`TestAutoFailoverDefaultWindows`、
  `TestDialPeerUsesBackupWhenPrimaryPathDown`、`TestAutoFailoverFiresOnlyWhenEnabled`、
  `TestDualListenAcceptsOnBothPaths`。
- 默认行为不变：现有 ha 全包 + 全量 `go test ./...` 通过；H2 套件保持 PASS。
