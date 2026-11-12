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

## 状态（滚动）
- Round 1（已验证）：环境脚手架落地；`make h2-up` 起 6 服务、双节点 healthy；`make h2-test` = **PASS=9 FAIL=0 NOT_VERIFIED=4**（含 mTLS、唯一 Owner、健康）；`make h2-down` 清理干净、可重复。具体 20 个故障场景与 DNS/DHCP/IPAM 验收套件在后续轮次执行。
- macOS OrbStack：tc/netem 与磁盘限制类仅在 Linux CI 执行，macOS 记 NOT_VERIFIED。
