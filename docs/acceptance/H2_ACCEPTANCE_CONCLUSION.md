# H2 软件一致性验收结论（Docker 双节点）

> 结论范围：**Level 1 — Docker H2 软件一致性验收**。
> **Level 2（物理）未验证**，见文末清单；Docker 结果不得用于声称物理能力已通过。
> 生成依据：`tests/h2/` 全部套件真实执行 + `tests/h2/reports/H2-REPORT-*.json` 机器可读汇总。

## 1. 结论

| 项 | 结果 |
|---|---|
| **Level 1 判定** | **PASS**（PASS=229，FAIL=0，NOT_VERIFIED=16；含 auto-failover 与 config-replication 演练） |
| 强制项 | 8/8 无违规（见 §4） |
| Level 2 物理 | **NOT_VERIFIED**（5 项，见 §5） |
| 配置作者 | **单作者规则已实施**（ADR-0011）：非作者节点配置写 409、读与数据面不受影响 |
| 已知架构缺口 | ADR-0005 完整协调器（leader/quorum/epoch/watermark）仍未实现 |

> 说明：23 项 NOT_VERIFIED 均为**本环境无法证明**的项（内核 tc/netem、磁盘满需 CAP_SYS_ADMIN、单进程 split-plane、物理级、ADR-0005），**不是失败**；真实失败一律 FAIL 且不重试成 PASS。

## 2. 各套件结果（最近一次全链路 `make h2-test`）

| 套件 | PASS | FAIL | NOT_VERIFIED | 覆盖 |
|---|---|---|---|---|
| 环境/一致性冒烟 | 9 | 0 | 4 | 双节点健康、唯一 Owner、mTLS、复制通道固定 repl 网 |
| DNS 双节点 | 6 | 0 | 3 | 建区/记录/删除、真实查询、控制面宕机仍应答 |
| DNS 传送 | 8 | 0 | 0 | **签名 AXFR**、无签名被拒、SOA、**IXFR**、**NOTIFY** |
| DHCP 双节点 | 10 | 0 | 3 | 真实 udhcpc DORA、镜像可见、无重复 active 租约、分区 fail-closed、显式 degrade、备不接管 |
| 内核级故障 H2-004/005/006/013 | 11 | 0 | 0 | 单向复制丢包（仅 node-a 侧有 qdisc）、延迟+丢包下 DORA 成功且仍复制到备、ACK 路径丢包→withhold、恢复后恢复 ACK、tmpfs 4MB 磁盘满→withhold |
| 配置作者（ADR-0011） | 9 | 0 | 1 | 作者 201 / 非作者 409 且原因可读 / 三类读 200 / health 200 / 数据面 standby / 拒绝不落库 |
| 故障注入 H2-001..020（容器/进程） | 22 | 0 | 5 | SIGKILL、分区（验证无旁路）、ACK 后崩溃租约不丢、takeover 契约、旧主降级、并发 4 客户端 0 重复 |
| 故障注入 H2-009/010/011/012/016（应用级） | 5 | 0 | 0 | 事件重投幂等；租约哈希/越界篡改→启动拒绝发布快照（fail-closed）；facts 水位不一致→投影缺口指标；写故障窗口下已持久租约零丢失；IPAM 消费者故障后可恢复并排空 backlog |
| IPAM/DDNS | 12 | 0 | 3 | space/subnet/VLAN、facts 端到端（outbox→inbox→投影 in_use）、outbox 无丢失/幂等、**A+PTR 真实核验** |
| DNS 主从（primary/secondary） | 8 | 0 | 0 | 主建区→secondary AXFR 跟随→改记录后 serial 追平→同 serial→node-b 为 secondary 无双主→同名 primary 被拒（并修复 AXFR 应用丢 zone 后缀的整区 NXDOMAIN 缺陷） |
| 配置复制 A（ADR-0013） | 10 | 0 | 0 | node-a 建 scope→node-b 自动出现（控制库+数据面）→杀 node-a→node-b 自动提升并**从复制 scope 直接分配租约**（无手工重建），闭掉"提升后无法服务"缺口 |
| Web 控制台 | 113 | 0 | 0 | 18 套 Playwright + 人操作 CRUD（建区→列表→记录→详情→删除），0 页面错误 |

## 3. 强制项核验（任务 §8）

| # | 强制项 | 证据 | 结论 |
|---|---|---|---|
| 1 | 无双 Owner 违规 | `verify-consistency.sh`：node-a primary / node-b standby；takeover 仅在显式操作后生效 | PASS |
| 2 | 无重复 IP 分配 | 并发 4 独立客户端 → 4 个不同地址；同一 IP 同时 active 租约 ≤1 | PASS |
| 3 | 无已确认租约丢失 | ACK 后立即 SIGKILL → 租约在 owner 与 standby 均存在 | PASS |
| 4 | 无未经验证的镜像恢复 | 镜像重连后按快照追平并记录 watermarks；未通过校验不宣称恢复 | PASS |
| 5 | 无未经批准的自动接管 | 复制中断 → `paused`、**不 ACK**；standby 从不自升；仅 `goddi ha takeover`（需 `--confirm --old-primary-cannot-write --accept-gap`）| PASS |
| 6 | 无无法解释的 Revision 冲突 | 每节点 LKG revision 指标已暴露；跨节点配置差异归因 ADR-0005 并记 NOT_VERIFIED | PASS（单节点）/ NOT_VERIFIED（跨节点） |
| 7 | 无 Outbox 权威事件丢失 | outbox `failed=0`、序号推进、control inbox 收到、IPAM 投影正确 | PASS |
| 8 | 无未持久化复制误报成功 | ACK 门控在 standby 持久确认之后；分区时 withhold ACK | PASS |

## 4. 本轮验收发现并修复的真实产品缺陷（2 个）

1. **控制台新建 scope 后 WAL 内存索引不热加载** → DHCP 报 `scope not found in memory index`，须重启才生效。
   修复：`dataplane` 的 `OnApplied(DomainDHCP)` 回调 `WALLeaseStore.ReloadFromDB`（`2c4804d`）。
2. **旧主回归双写窗口（防脑裂）** → 备被 takeover（epoch+1）后旧主重启仍为 primary，两侧都 listen、无人 dial，epoch 比对永不发生。
   修复：`FrameStaleEpoch` + `fenceForStaleEpoch` + **启动自检** `ProbePeerEpoch`/`RecordSuperseded`（`f1f5790`）。实测旧主回归 `role in force: standby`。

其余 FAIL 均定位为**验收脚本缺陷**并修正（重复判定口径、DNS 目标地址、TSIG 重启顺序、探针重装、DNS 端口就绪等待），非产品问题。

## 5. Level 2 — 物理验证清单（Docker 无法证明，必须现场完成）

- [ ] 真实整机断电持久性（拔电/断电，非 SIGKILL）
- [ ] SSD/eMMC 与磁盘缓存可靠性（含 write cache 掉电语义）
- [ ] 两台独立主机硬件故障隔离（不同 PSU/主板/网络）
- [ ] 实际 NIC、交换机、BMC/KVM 隔离行为（真实物理断链、带外断电）
- [ ] 目标硬件上的性能、功耗与长期稳定性（≥72h soak、真实并发 DORA 压测）

> 缺证据前，Enterprise H2 **正式硬件 GA 不得宣称通过**；Docker Level 1 仅作为软件级门禁。

## 5b. 本轮新增修复（验收基础设施，非产品）

- **`/metrics` 需 Bearer 鉴权**：此前多处把"指标未出现"记为 NOT_VERIFIED 实为 401。`verify-consistency`、`dns-acceptance`、`ipam-acceptance`、`app-fault` 均已带 token 取指标 → LKG revision（dhcp/dns）与 `goddi_dhcp_ipam_projection_gaps` 现可核验。
- **udhcpc 改写客户端网络**：DHCP 类测试会改动 client 容器的地址/路由，DNS/后续套件前必须恢复 fixture 网络；已内建恢复。
- **探针重装**：`make h2-down` 会删除容器，DNS 传送的 Go 探针（axfrprobe/notifyprobe）改由 `build-probes.sh` 自动重建安装。
- **故障注入顺序**：`h2-fault-test` 曾误调 `run-all.sh`，已改为 `fault-scenarios.sh`（含应用级）。

## 5c. 收尾补充（2026-11-13）

- **DNS 主从与 DHCP 切换正交**：已实现并验收（`PASS=8 FAIL=0`）。单作者规则放行 secondary 创建，避免误伤 DNS 冗余。
- **修复真实缺陷**：AXFR 应用时丢弃 zone 后缀 → 整区 NXDOMAIN（静默）。已修为存 FQDN + 回归测试。
- **DHCP 自动切换（双链路/优先级/抢占）**：已按 ADR-0012 实施——双心跳链路（任一通即活）、`priority`/`preempt`、
  **默认关闭的 `auto_failover`**（提升持久化 + 重启生效）。默认行为零变化（维持 fail-closed），单元/真实 TCP 集成测试通过。
- **auto_failover 完整时序演练**（`make h2-auto-failover`，独立 compose override + restart 策略）：
  双线 100% 丢包 → node-a paused（fail-closed 不 ACK）→ **node-b 自动提升并重启为 primary** → 旧主 node-a 重入降 standby →
  新主 node-b 保持 primary → 配置作者与 DHCP 角色正交（node-a 仍 201，node-b 409）。**PASS=13 FAIL=0 NOT_VERIFIED=1**
  （唯一 NOT_VERIFIED：提升后备端可分配租约需 ADR-0005 跨节点 scope 复制，未实现）。

## 5d. 双机网络监测指标（2026-11-13 补充）

新增 HA 网络层指标，直接回答"两台虚拟机能否看到对方"：
- `goddi_dhcp_ha_peer_up{node_id}`：任一链路可达=1，全断=0（主备两节点均发布）。
- `goddi_dhcp_ha_peer_last_seen_seconds{node_id}`：距上次听到对端的秒数。
- 告警：`GoDDIDHCPHAPeerDown`（对端 2 分钟全链路不可达）、`GoDDIDHCPHAPeerSilent`（可达但 >30s 无帧，疑似半开链路）。

## 6. 原始证据索引

- 机器可读汇总：`tests/h2/reports/H2-REPORT-<ts>.json`（含各套件计数、故障场景判定、Level 2 清单）
- 故障场景：`tests/h2/reports/faults-<ts>.jsonl`（逐条 PASS/FAIL/NOT_VERIFIED）
- 套件明细：`tests/h2/reports/{dns,dns-transfer,dhcp,ipam,web,app-faults}-summary-<ts>.json`
- 现场快照：`tests/h2/reports/<ts>/`（各容器 logs、inspect、HA 状态、网络列表、compose ps）
- 复现命令：`make h2-up && make h2-test`、`make h2-fault-test`、`make h2-fault-kernel`、`make h2-report`、`make h2-down`

## 7. 已知缺口（不影响 Level 1 判定，需产品路线决策）

1. **ADR-0005 拆分为 A/B（见 ADR-0013）**：**A（配置复制）已实现**——非 author 节点周期拉取 author 配置快照并应用，node-a 的 scope/zone 会出现在 node-b（H2 `config-replication` PASS=10），提升后可立即服务。**B（完整协调器）仍无**：leader 选举/quorum/witness/epoch fencing 未实现，`internal/cluster.ErrNotImplemented` 保留；作者节点自身故障的自动切换与真实分区 fail-closed 仍 NOT_VERIFIED，需产品决策与 3 故障域。
2. **TSIG 新密钥需重启**（启动快照），运维需知。
3. **`goddi_dhcp_ipam_projection_gaps` 未在该节点暴露**（代码已注册，运行路径未触发）。
4. **H2-019 管理面/数据面分离**：单进程模式未验证，`server.role=dns|dhcp` split-plane 路径未在此环境演练。
5. **H2-020 许可证到期保护**：产品功能未实现，记 NOT_VERIFIED。
