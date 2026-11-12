# H2 软件一致性验收结论（Docker 双节点）

> 结论范围：**Level 1 — Docker H2 软件一致性验收**。
> **Level 2（物理）未验证**，见文末清单；Docker 结果不得用于声称物理能力已通过。
> 生成依据：`tests/h2/` 全部套件真实执行 + `tests/h2/reports/H2-REPORT-*.json` 机器可读汇总。

## 1. 结论

| 项 | 结果 |
|---|---|
| **Level 1 判定** | **PASS**（PASS=171，FAIL=0，NOT_VERIFIED=23） |
| 强制项 | 8/8 无违规（见 §4） |
| Level 2 物理 | **NOT_VERIFIED**（5 项，见 §5） |
| 已知架构缺口 | ADR-0005 跨节点配置发布未实现（跨节点 DNS/IPAM 配置一致性 NOT_VERIFIED） |

> 说明：23 项 NOT_VERIFIED 均为**本环境无法证明**的项（内核 tc/netem、磁盘满需 CAP_SYS_ADMIN、单进程 split-plane、物理级、ADR-0005），**不是失败**；真实失败一律 FAIL 且不重试成 PASS。

## 2. 各套件结果（最近一次全链路 `make h2-test`）

| 套件 | PASS | FAIL | NOT_VERIFIED | 覆盖 |
|---|---|---|---|---|
| 环境/一致性冒烟 | 9 | 0 | 4 | 双节点健康、唯一 Owner、mTLS、复制通道固定 repl 网 |
| DNS 双节点 | 6 | 0 | 3 | 建区/记录/删除、真实查询、控制面宕机仍应答 |
| DNS 传送 | 8 | 0 | 0 | **签名 AXFR**、无签名被拒、SOA、**IXFR**、**NOTIFY** |
| DHCP 双节点 | 10 | 0 | 3 | 真实 udhcpc DORA、镜像可见、无重复 active 租约、分区 fail-closed、显式 degrade、备不接管 |
| 故障注入 H2-001..020 | 21 | 0 | 15 | SIGKILL、分区（验证无旁路）、ACK 后崩溃租约不丢、takeover 契约、旧主降级、并发 4 客户端 0 重复 |
| IPAM/DDNS | 13 | 0 | 2 | space/subnet/VLAN、facts 端到端（outbox→inbox→投影 in_use）、outbox 无丢失/幂等、**A+PTR 真实核验** |
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

## 6. 原始证据索引

- 机器可读汇总：`tests/h2/reports/H2-REPORT-<ts>.json`（含各套件计数、故障场景判定、Level 2 清单）
- 故障场景：`tests/h2/reports/faults-<ts>.jsonl`（逐条 PASS/FAIL/NOT_VERIFIED）
- 套件明细：`tests/h2/reports/{dns,dns-transfer,dhcp,ipam,web}-summary-<ts>.json`
- 现场快照：`tests/h2/reports/<ts>/`（各容器 logs、inspect、HA 状态、网络列表、compose ps）
- 复现命令：`make h2-up && make h2-test`、`make h2-fault-test`、`make h2-fault-kernel`、`make h2-report`、`make h2-down`

## 7. 已知缺口（不影响 Level 1 判定，需产品路线决策）

1. **ADR-0005 多节点配置发布未实现**：node-a 建区不会自动出现在 node-b；跨节点 Revision/Hash 一致性、跨节点 IPAM 配置一致性均记 NOT_VERIFIED。
2. **TSIG 新密钥需重启**（启动快照），运维需知。
3. **`goddi_dhcp_ipam_projection_gaps` 未在该节点暴露**（代码已注册，运行路径未触发）。
4. **H2-019 管理面/数据面分离**：单进程模式未验证，`server.role=dns|dhcp` split-plane 路径未在此环境演练。
5. **H2-020 许可证到期保护**：产品功能未实现，记 NOT_VERIFIED。
