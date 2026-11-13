# H2 Docker 软件一致性验收

> 开发/CI 的 H2 数据一致性验收统一使用 `tests/h2/` 的 Docker Compose 双节点模拟；
> **不降低** DNS/DHCP/IPAM 的一致性、安全、防脑裂与持久化要求。Docker 通过≠物理 GA。

## 环境（tests/h2）
- **node-a**：合法 DHCP Owner（primary）；**node-b**：被动镜像（standby）。
- **client**：真实 DNS/DHCP 客户端容器；**relay**：DHCP Relay 测试组件；**fault-injector**：网络/进程故障注入；**observer**：证据采集。
- 三张独立网络：`mgmt-net`(172.29/24)、`repl-net`(172.28/24，固定 IP)、`client-net`(172.30/24)。
- 复制通道固定绑定 `repl-net`（`dhcp_ha.listen_addr` 为节点 repl IP:9953），不与管理网共享。
- **mTLS**：`tests/h2/scripts/gen-certs.sh` 生成私有 CA + 每节点证书（SAN 绑定 repl IP），配置 `dhcp_ha.tls_{cert,key,ca}_file`。
- 每节点独立 SQLite + Lease WAL + 配置 + 证书目录；Docker Volume `node-a-data`/`node-b-data` 持久化；**无共享 DB/WAL**。
- 镜像由**固定 commit（当前 HEAD）**构建（`make h2-build` 编译 `.ha-goddi-bin`）。

## 统一入口
```
make h2-up                # 生成证书 + 构建 HEAD 镜像 + 启动 6 服务
make h2-test              # 等待健康 + 一致性检查 + 采集证据
make h2-fault-test        # 故障场景（逐步接入，见 tests/h2/scenarios）
make h2-consistency-check # 运行 verify-consistency.sh
make h2-report            # 采集 logs/inspect/HA 状态到 tests/h2/reports/<ts>/
make h2-down              # 停止并清理（含卷）
```

## 验收层级（严格区分）
- **Level 1 — Docker H2 软件一致性**：协议/一致性/防脑裂/复制恢复/应用崩溃/故障注入。由本环境判定 PASS/FAIL/NOT_VERIFIED。
- **Level 2 — 物理部署补充（不可由 Docker 证明，缺证据即 NOT_VERIFIED）**：真实整机断电持久性、SSD/eMMC+磁盘缓存可靠性、两台独立主机硬件故障隔离、真实 NIC/交换机/BMC 隔离、目标硬件性能/功耗/长期稳定性。

## 判定
每个检查输出 PASS / FAIL / NOT_VERIFIED；**真实故障失败不得重试成 PASS**。强制项：无多 Owner、无重复 IP、无已确认租约丢失、无未验证镜像恢复、无未批准自动接管、无不可解释 Revision 冲突、无 Outbox 权威事件丢失、无把未持久化复制误报成功。

## 一键验收与报告
```
make h2-up && make h2-test    # 环境 + 全部套件 + 证据 + 汇总报告
make h2-fault-test            # H2-001..020 故障场景
make h2-fault-kernel          # tc/netem 与磁盘故障（Linux CI 才可验证）
make h2-web-test              # 控制台全页面回归 + 人操作 CRUD 走查
make h2-report                # 只汇总 reports/*-summary-*.json 与 faults-*.jsonl
make h2-down                  # 清理
```
`make h2-report` 产出 `tests/h2/reports/H2-REPORT-<ts>.json`：每套件 PASS/FAIL/NOT_VERIFIED、
故障场景判定、未运行套件（NOT_RUN，不当作 PASS）、以及 **Level 2 物理项清单（固定 NOT_VERIFIED）**。
汇总判定：任一 FAIL → FAIL；全部通过但有未运行 → NOT_VERIFIED；否则 PASS。

## 最终结论（2026-11-13 更新）

- **内核级故障已转 PASS**：`make h2-fault-kernel` 在本机 linux/arm64 运行时真实注入 tc/netem 与 tmpfs 磁盘满 → H2-004/005/006/013 **PASS=11 FAIL=0**。
- **ADR-0011 单作者规则已实施**：`cluster.config_author`；非作者节点配置写 409、读与数据面不受影响 → `make h2-config-author` **PASS=9**。
- 全链路 `make h2-test` → **verdict=PASS，PASS=196 FAIL=0 NOT_VERIFIED=16**。

## 早期结论
见 [`H2_ACCEPTANCE_CONCLUSION.md`](H2_ACCEPTANCE_CONCLUSION.md)：Level 1 **PASS**（PASS=171 / FAIL=0 / NOT_VERIFIED=23，8/8 强制项无违规）；Level 2 物理 5 项 **NOT_VERIFIED**；已知缺口（ADR-0005 等）已列明。

## 状态（滚动）
- Round 11（应用级故障注入 H2-009/010/011/012/016）：`app-fault.sh` 真实篡改持久状态验证 fail-closed → **APP-FAULTS PASS=5 FAIL=0 NOT_VERIFIED=0**（事件重投幂等、越界租约→启动拒绝快照、水位不一致→投影缺口指标、写故障零丢失、消费者故障恢复）。另修正 `/metrics` 需鉴权（此前多处 NOT_VERIFIED 实为 401）与 udhcpc 改写客户端网络的问题。全链路 `make h2-test` verdict=PASS（PASS=176）。
- Round 10（六入口验证 + 最终结论）：`make h2-up / h2-test / h2-fault-test / h2-consistency-check / h2-report / h2-down` 全部实测可用、可重复、可清理。修复三处验收基础设施问题（`h2-fault-test` 调错脚本、`h2-down` 后探针丢失需自动重建、DNS 端口就绪竞态）。最终 `make h2-test` verdict=PASS（PASS=171），`make h2-fault-test` PASS=22 FAIL=0。产出 `docs/acceptance/H2_ACCEPTANCE_CONCLUSION.md`。
- Round 9（Web 控制台全功能验收）：启动**最新构建**（HEAD 编译，隔离数据目录/端口），用真实 Chromium 执行 18 套 Playwright 回归 + 人操作 CRUD 走查 → **WEB PASS=113 FAIL=0 PAGE_ERRORS=0**。
  - 人操作路径实测：表单登录 → 仪表盘渲染 → 建区 → **列表 UI 可见** → 加 A 记录 → **区域详情页可见** → 删除 → 无页面错误。
  - 澄清一处"疑似缺陷"：区域详情页正确路由是 `/dns/zone-detail/:id`（列表页链接也指向它），`/dns/zones/:id` 返回 404 属预期；验收脚本已按真实路由修正。
  - 汇总：`make h2-report` → **verdict=PASS，PASS=171 FAIL=0 NOT_VERIFIED=23**（dhcp 10 / dns 6 / dns-transfer 8 / faults 21 / ipam 13 / web 113）。
- Round 8（CI 故障脚本 + 一键报告）：新增 `netem.sh`（tc/netem 延迟/丢包，验证 qdisc 生效才 PASS）、`disk-fault.sh`（tmpfs 磁盘满，无 CAP_SYS_ADMIN 即 NOT_VERIFIED）、`Dockerfile.tools`（带 iproute2 的 CI 工具镜像）、`h2-report.sh`（汇总器）。**`make h2-test` 全链路 verdict=PASS：PASS=58 FAIL=0 NOT_VERIFIED=23**（dhcp 10、dns 6、dns-transfer 8、faults 21、ipam 13）。
  - 修正两处验收脚本缺陷（非产品问题）：① DHCP 重复判定把历史 expired 行当成重复分配，改为只统计**同时 active** 的租约；② DNS 套件不可重复（zone 已存在即失败）且停机验证打错目标，改为幂等复用并按 node-b 自身地址查询。
- Round 6（DNS 传送/TSIG + LKG）：`dns-transfer-acceptance.sh` → **PASS=4 FAIL=1 NOT_VERIFIED=3**。
  - PASS：TSIG key 创建（服务端生成并一次性返回 secret）、AXFR 按 CIDR+key 授权、**无签名 AXFR 被拒（REFUSED）**。
  - **Round 7 澄清并修复**：先前记录的"签名 AXFR 失败"**不是产品缺陷**，是验收脚本自身两处错误——① 目标 DNS IP 硬编码为 `10.77.3.4`，容器重建后实际是 `10.77.3.2`，请求打到了没有 key/授权的 node-b；② `docker restart`（加载新 TSIG key 的必需步骤）被放在了探测**之后**。修正后：**签名 AXFR 真实成功（16 条记录）**、无签名被拒（REFUSED）、SOA 应答、IXFR 应答、**NOTIFY 被确认**。最终 `DNS-TRANSFER PASS=8 FAIL=0 NOT_VERIFIED=0`。
  - 新增：`tests/h2/tools/axfrprobe`（真实 DNS 传送客户端，镜像无 dig）；DNS 域 LKG 指标 `goddi_dataplane_lkg_revision{domain="dns"}` 已接入启动与 30s 刷新。
- Round 5（IPAM/DDNS 验收）：`ipam-acceptance.sh` → **PASS=13 FAIL=0 NOT_VERIFIED=2**。space/subnet(VLAN 130) 建/复用；真实 DORA 后 **facts 端到端**（producer outbox `done` → control inbox → IPAM `observed_state=in_use`）；outbox 无 failed、序号推进、无重复行（幂等）；**DDNS 真实发布并核验**：`<host>.h2ddns.test A` 与 `x.3.77.10.in-addr.arpa PTR` 经真实 DNS 查询确认。
  - 观察到的环境约束（非产品缺陷）：DORA 后 udhcpc 会改写客户端自身地址/路由，脚本需恢复 fixture 网络；容器内无 `dig`（alpine 源受限），改用 `nslookup`。
  - 未验证：`goddi_dhcp_ipam_projection_gaps` 指标未在此节点暴露；跨节点 IPAM 配置一致性属 ADR-0005（未实现），如实 NOT_VERIFIED。
- Round 4（故障注入 + 第二个真实缺陷修复）：`fault-scenarios.sh` 真实执行 H2-001/002/003/008/014/015/017/018/019 → **PASS=21 FAIL=0 NOT_VERIFIED=15**（tc/netem、磁盘满、license 等记 NOT_VERIFIED）。证据：`tests/h2/reports/faults-*.jsonl` + `collect-evidence.sh`。
  - **修复的真实缺陷（防脑裂）**：旧主宕机、备被 `ha takeover` 提升（epoch+1）后，旧主重启仍以 primary 服务 —— 两侧都是 primary，各自 listen、无人 dial，epoch 比对永不发生，形成双写窗口。修复：① 新增 `FrameStaleEpoch`（已升主节点在握手时告知对端其写入权已失效）；② `Replicator.fenceForStaleEpoch` 持久化降级；③ **启动自检** `ha.ProbePeerEpoch` + `ha.RecordSuperseded`：配置为 primary 但探测到 peer epoch 更高则持久化为 standby 并不服务。实测：旧主回归 `role in force: standby`（日志 `local_epoch:0 peer_epoch:1`），新主保持 primary。
  - `ha takeover` 契约实测正确：primary 水位新鲜时**拒绝**提升，静默 15s 后才接受（需 `--confirm --old-primary-cannot-write --accept-gap`）。
  - 并发：4 个**独立客户端容器**同时 DORA → 4 个不同地址、**0 重复**。
- Round 3（DHCP 一致性 + 缺陷修复）：真实 udhcpc DORA 拿到租约；node-b 镜像可见同一租约；重复租约行=0；**复制双向隔离（blackhole，验证无旁路）**后默认 fail-closed 不 ACK、操作员 `ha degrade` 后本地继续服务、node-b 始终 standby、租约序号推进 → `DHCP PASS=11 FAIL=0 NOT_VERIFIED=2`。
  - **修复的真实缺陷**：通过管理 API 新建/修改的 scope 已写入 lease.db，但 WAL 内存索引不热加载，DHCP 报 `scope ... not found in memory index`，必须重启节点才生效。现由 `dataplane` 的 `OnApplied(DomainDHCP)` 回调 `WALLeaseStore.ReloadFromDB` 重新发布池快照。
  - **Docker 旁路发现**：仅 `docker network disconnect repl-net` 不能隔离（宿主在桥间转发），必须双向 blackhole + 连通性验证，脚本已按此实现并输出 PASS/NOT_VERIFIED。
- Round 2（DNS 验收核心）：`dns-acceptance.sh` 真实 dig：node-a 建区/记录→应答 PASS；node-b 各自控制面应答 PASS；node-a 停机后 node-b DNS 继续应答 PASS；删除撤记录 PASS；**跨节点配置传播 NOT_VERIFIED（ADR-0005 多节点发布未实现）**；AXFR/IXFR/NOTIFY 待补。
- Round 1（已验证）：环境脚手架落地；`make h2-up` 起 6 服务、双节点 healthy；`make h2-test` = **PASS=9 FAIL=0 NOT_VERIFIED=4**（含 mTLS、唯一 Owner、健康）；`make h2-down` 清理干净、可重复。具体 20 个故障场景与 DNS/DHCP/IPAM 验收套件在后续轮次执行。
- macOS OrbStack：tc/netem 与磁盘限制类仅在 Linux CI 执行，macOS 记 NOT_VERIFIED。
