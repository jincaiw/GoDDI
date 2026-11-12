# 10 · 最终审计结论与生产验收阻塞项

> 基线 commit `8e769d7`（工作树干净）。审计为只读；证据命令：`go build ./...` exit 0、`go vet ./...` exit 0、`go test ./...` 102 包 ok / 0 FAIL。故障注入多数 NOT_VERIFIED（见 `07-failure-test-matrix.csv`）。

## 一、结论（直接回答）
1. **当前架构是否适合企业级 S1 和 H2？**
   - **S1：PARTIAL** —— 功能面较完整、单元测试充分、SQLite `WAL/FULL` 正确；但**未做长期/破坏性验收**，且默认热路径刚从 SQLite 切到 WAL 事件溯源（证据不足），S1 尚不宜宣称"生产就绪"。
   - **H2：PARTIAL/不满足** —— DHCP 单写热备的"ACK 前双端持久化"骨架**真实实现且有 SIGKILL 演练证据**；但**数据面无 mTLS、无 Owner Epoch 写入栅栏、facts 无哈希链、多节点配置发布未实现**，不满足企业级 H2 的强一致/可审计要求。
2. **DNS / DHCP / IPAM 一致性程度**
   - DNS：单节点 config revision+outbox+读本地副本 PASS；AXFR/IXFR/NOTIFY/TSIG 本轮修复并有演练 PASS；**多节点 Prepare/Commit/Apply 与"集群全部应用"NOT_IMPLEMENTED**。
   - DHCP：ACK 前置持久化 PASS（代码+单测+SIGKILL）；防自动接管 PASS；**复制失败默认暂停（与 DEC-006 冲突）FAIL**；无 epoch 栅栏。
   - IPAM：配置权威与租约投影分离 PASS；DHCP→IPAM→DDNS 事务性 outbox（单节点）PASS；**陈旧配置无 freshness 栅栏 PARTIAL/风险**；两节点写协调 NOT_IMPLEMENTED。
3. **是否存在脑裂 / 重复 IP / 已确认租约丢失风险？**
   - **已确认租约丢失**：代码路径与 SIGKILL 演练未发现（PASS 级别证据）；**真实断电 NOT_VERIFIED**。
   - **脑裂写**：当前靠 paused（MayBind=false）+ Confirm 成功抑制，**未发现自动双写**；但**无 Owner Epoch 证明**，且人工接管/旧主回归场景 NOT_VERIFIED → **存在理论窗口（P0）**。
   - **重复 IP**：正常情况下依赖租约按 IP/MAC + 单写者；**并发压测缺失** → 风险 NOT_VERIFIED。
4. **真实实现 vs 设计阶段**
   - 真实实现：DNS 权威/缓存/转发/传送、DHCP 单机+HA 单写热备、IPAM 配置/事实消费、统一事实链、RBAC/MFA/审计、systemd、SQLite 持久化。
   - 仅设计/未实现：多节点集群协调（`cluster` 501 / ADR-0005）、Owner Epoch、facts 哈希链、License 执行、Appliance Agent 运行验证。
5. **必须在正式上线前修复的 P0（详见 08）**
   - D-01 数据面 HA 无 mTLS/防重放 → 必须。
   - D-02 无 Owner Epoch 写栅栏 → 必须。
   - D-03 facts 无哈希链（无篡改/分叉证明）→ 必须（可审计性）。
   - D-04 **已由 ADR-0010 裁定（2026-11-12 批准）**：默认 `paused`（不承诺新/延长租约）；自动 degraded 继续仅当 Owner Epoch 最新 + quorum/witness + mTLS；DEC-006/007 已建立。实施见 09 的 E-1/E-2/E-3。
   - D-05 多节点配置发布未实现（若声称 H2 多节点）→ 必须或明确降级范围。
   - D-06 AXFR 授权链路（已修 API）+ 新 TSIG key 需重启（运行可用性）→ 建议尽快。
6. **最合理的目标架构**
   - 保持"单写者 + 本地副本 + 事件链"：DHCP/DNS 数据面**只从本地持久副本应答**，控制面宕机不停数据面；配置发布走**leader-epoch + 两阶段 + 集群应用视图**；复制通道 **mTLS + epoch + 哈希链**；IPAM/DNS 经**统一事实链 + outbox/watermark** 最终一致；多节点协调落地 ADR-0005。
   - 明确 S1 与 H2 的存储组合矩阵（S1: WAL；H2: primary WAL / standby SQLite 镜像，或统一 WAL）。
7. **修改顺序（最低风险，详见 09）**
   - 先决策（T0）→ 可信身份 mTLS（T1-1）→ Owner Epoch（T1-2）→ facts 哈希链（T1-3）→ 阻断/人工恢复（T2）→ ADR-0005 发布协调（T3）→ IPAM freshness/outbox 故障注入/SQLite 错误处理/备份恢复（T4）→ 性能/清理/热加载（T5）。每阶段独立回滚，先在双容器演练，再谈真机 B8.5。

## 二、生产验收阻塞项（Go/No-Go）
**No-Go（当前不可宣称企业级生产就绪）**，阻塞项：
1. **B8.5 物理真机验收 BLOCKED**（真实主机/断电/网络隔离/fencing/真实 relay/监控栈原始时间线）——未执行。
2. **H2 安全**：数据面 mTLS/证书/防重放缺失（D-01）。
3. **H2 一致性证明**：无 Owner Epoch（D-02）、facts 无哈希链（D-03）。
4. ~~决策冲突未收敛~~ **已解决**：ADR-0010 批准，DEC-006/007 建立（D-04 关闭）。
5. **多节点配置发布未实现**（D-05），若对外声称 H2 多节点一致性则不成立。
6. **故障注入矩阵多数 NOT_VERIFIED**（磁盘满/WAL 损坏/真实断电/分区恢复/并发压测）。

## 三、可复现证据索引
- 已执行：`go build/vet/test ./...`（102 ok）；`scripts/d2_wal_canary_drill.sh`（9/0）；`scripts/d2_ha_wal_canary_drill.sh`（17/0）；`scripts/dns_primary_secondary_drill.sh`（DRILL OK）；`scripts/dns_primary_secondary_container_drill.sh`（PASS=5 FAIL=0）；`make verify-dns-drill`。
- 未执行（NOT_VERIFIED）：真实断电、磁盘满、WAL 损坏、网络分区恢复、旧主回归、并发双写、高频续租压测、备份恢复。
- 文档冲突：任务书 DEC-001~019 不存在；以 `docs/adr/0001–0009` 为准。

## 四、一句话
**骨架真实、证据分层清楚**：单机可试运行但未达生产就绪；双节点"ACK 前双端持久化"可信，但**安全身份、Owner Epoch、事实哈希链、多节点配置发布与物理验收**是四到五个必须在正式上线前闭合的 P0/阻塞项。审计可按 09 阶段顺序推进至可验收。

---

## 附：实施进展与完成度（2026-11-12 滚动更新）

> 本审计后续已按「遇决策→合理性评估→按推荐推进」执行了一轮实施，详见 `11-completion-status.md` 与 `IMPLEMENTATION-STATUS.md`。要点：

### 已实现并有测试证据（默认安全、可回滚）
- **D-01** 复制通道可选 mTLS + hello 防重放（`ha.LoadTLS`/serverTLS/clientTLS、`tls_test.go`）。
- **D-02** Owner Epoch 写栅栏（`ha_owner_epoch`、Takeover 单调+1、握手交换、`epoch_test.go`）。
- **D-04** DEC-006/ADR-0003 冲突由 **ADR-0010** 裁定（默认 paused；自动 degraded 需 epoch+quorum+mTLS）；DEC-002/006/007/008 建档。
- **D-03** facts 哈希链：内核+校验器+tip store+StampTx；双库迁移增列；`facts_hash_chain_enabled` 开关；生产打戳；**IPAM+DNS 消费者 VerifyEnvelope + 断链 blocked**（flag 开启端到端）。
- **D-06** `dns_zone_transfer` 授权 API（列/建/删）+ OpenAPI + 实机 AXFR 验证；TSIG 运行期热加载仍需重启（未做，作为运维注意）。
- **D-08** SQLite FULL/IO/BUSY/ENOSPC/EIO 错误分类并接入 lease 写路径（`sqlitepolicy/errors.go`）。
- **D-09** IPAM 投影缺口检测 + 可选严格栅栏（`ipam/projection.go`、`dhcp.require_ipam_projection`）。
- **D-10** relay 中断/重启续传/重复幂等（`facts/relay_test.go`）。
- **D-11** 租约幂等/并发 + 修复并发抢址裸 UNIQUE 错误→`ErrAddressTaken`。
- **D-12** full 备份删后恢复 DNS/DHCP/IPAM 集成测试；旧基线→当前升级演练 `TestLegacyBaselineUpgradeKeepsTheData` 通过（升级路径有测试）。
- **D-14** dataplane LKG 版本戳同事务写入 + `dataplane.LKG` + 指标。
- **D-18/D-20** 基准脚本 `make bench`、Prometheus 指标（LKG/投影缺口，30s 刷新）。

### 仍为外部阻断 BLOCKED（沙箱不可完成）
- D-05 多节点配置发布/quorum（ADR-0005，需第三仲裁节点/见证）；B8.5 物理真机验收；真实填盘 ENOSPC/fsync 注入；证书轮换/吊销编排（需现场 PKI）。

### 仍为 NOT_VERIFIED（需真机/长期环境）
- 真实断电（仅 SIGKILL 演练）、WAL 磁盘损坏注入、网络分区恢复并发双写、并发 DORA/高频续租压测/72h soak、升级大库/停机窗口恢复、Appliance agent（D-17）运行验证、集群 ClusterID（D-19，501 产品边界）。

### 回归证据
- `go build ./...` OK；`go test ./...` **102 包 ok / 0 FAIL**（本轮）。
- 演练：D2 独立 9/0、D2 HA 17/0、DNS loopback `DRILL OK`、DNS 两容器+dig `PASS=5 FAIL=0`。

### 完成度结论
- **可安全实施且可验证的 P0/P1 缺陷已全部落地**（默认关闭/可回滚的项均保持既有行为）；**D-03 端到端哈希链在启用开关时闭合**。
- **剩余项均为外部阻断、需真机/长时间环境或不改变产品的边界**——不属本沙箱可完成范围。
- **S1：接近就绪（待真机与长跑）**；**H2：核心安全要素（单写/ACK 前双端持久化/Owner Epoch/mTLS/不自动提升/哈希链可选）到位；自动 degraded 与 quorum 仍 BLOCKED**，故 H2 仍 PARTIAL。
