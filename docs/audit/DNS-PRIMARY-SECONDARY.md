# DNS 双机：标准 DNS 主从（方案 A）

> 用标准 DNS 机制（AXFR/IXFR + NOTIFY + TSIG + XFR-over-TLS）把两个 GoDDI DNS 实例组成主/从：primary 是唯一写者，secondary 通过区传送拉取只读副本并本地应答。**不是**自动选主的 DNS 集群（`/cluster` 仍 501）。

## 1. 组件与代码
- primary 对外传送：`internal/dns/transfer/transfer.go`（`AXFRHandler.HandleAXFR/HandleIXFR`），`main.go` `dnsSrv.SetAXFRHandler(...)`；仅在 TCP 上、需 TSIG 授权（`dns_zone_transfer.tsig_key_name`）。
- secondary 拉取：`internal/dns/transfer/secondary.go`（`SecondarySync.SyncFromPrimary` → `axfrFromPrimary`，`StartPeriodicSync` 周期刷新），过期按 SOA EXPIRE 停止权威应答。
- 变更通知：`internal/dns/transfer/notify.go`（primary `SendNotifyForZone`，目标取自 zone `acl.notify`；secondary 入站 NOTIFY 触发刷新）。
- 协议交互由 `miemg/dns` 实现；TSIG 为强制（无 TSIG 的 AXFR 被判 fail-closed 拒绝）。

## 2. 配置（在控制台/库中）
**Primary 节点**：
1. 建 `type=primary` 的 zone（SOA/NS/A 等记录）。
2. zone ACL：`allow-transfer` 允许 secondary 的源地址；`notify` 填 secondary 的 IP:port（变更时发 NOTIFY）。
3. 建 TSIG key（`dns_tsig_keys`，名称+密钥），并建立 `dns_zone_transfer(zone_id, tsig_key_name, allowed_cidr)` 绑定——AXFR 的授权就是这条绑定。

**Secondary 节点**：
1. 建同名 `type=secondary` 的 zone。
2. `transfer_policy` 填：
   ```
   <primary-host>[:port][|tsig-key名|base64密钥[|hmac-sha256]]
   例：192.0.2.10:53|xfra.key|BASE64SECRET|hmac-sha256
   例(TLS/XFR-over-TLS)：tls://192.0.2.10:853|xfra.key|BASE64SECRET
   ```
   无 TSIG 字段 → 同步 fail-closed 拒绝。
3. `SecondarySync` 周期/收到 NOTIFY 时 AXFR 拉取、校验、落库并 reload；SOA EXPIRE 到期 → 停止权威应答。

**客户端/解析器**：把 primary、secondary 都作为 NS 或上游（多 NS / LB / anycast）实现读容灾。写仍只在 primary。

## 3. 本轮修复的真实缺陷（使方案 A 真正可用）
1. **secondary 的 AXFR 请求未携带 TSIG**：`axfrFromPrimary` 只 `m.SetAxfr()`，而 miekg `Transfer.WriteMsg` 仅在 `m.IsTsig()!=nil` 时签名 → 请求实际未签名，primary 必然拒绝。修复：请求加 `m.SetTsig(canonical key, algo, 300, now)`。
2. **primary 的 AXFR 响应未携带 TSIG**：`serveZoneTransfer` 直接 `WriteMsg(msg)`，而 miekg 服务器只在响应自带 TSIG RR 时签名 → 严格 secondary 报 `dns: no signature found`。修复：每条传送响应 `msg.SetTsig(req 的 key/algo)`，首条之后 `TsigTimersOnly(true)`（RFC 8945 §5.3.1 多消息链）。

## 4. 详细测试（仓内、可复现）
- `internal/dns/transfer/primary_secondary_e2e_test.go`：起一个**真实 TSIG 保护的 primary**（`AXFRHandler` + 真实 `dns.Server`/TCP），用真实 `SecondarySync` 走 TCP 拉取：
  - 初次 AXFR 复制（记录 + serial）；
  - primary 变更后再同步（新记录/新 serial）；
  - NOTIFY 触发刷新；
  - **无 TSIG / 错误密钥 → fail-closed 拒绝**。
- `internal/dns/server/handler_axfr_tsig_test.go`：直接验证产品 `serveZoneTransfer` 对每条传送响应附加 TSIG 且首条后切 timers-only。

## 5. 边界（诚实）
- 写单点（primary）；secondary 只读；切换在 DNS 解析层（多 NS/LB），**无自动选主/quorum**；`/cluster` 恒 501。
- secondary 客户端同步当前用 AXFR（`axfrFromPrimary`）；IXFR 服务端能力在 `HandleIXFR`（有 journal），但客户端侧未走 IXFR 增量。
- 需现场正确配置 NS/TSIG/ACL/notify；容器/主机双实例的端口与网络由部署决定。

## 6. 一键演练脚本
`scripts/dns_primary_secondary_drill.sh`：脚本化运行上面的 loopback 演练（`TestPrimarySecondary*`），输出 `PASS | ...` 各行与最终 `DRILL: OK`。覆盖：初始 AXFR、变更再同步、NOTIFY 刷新、无 TSIG/错密钥 fail-closed、**停掉 primary 后 secondary 仍应答（读容灾）**。

## 7. 两容器 + dig 现场版
`scripts/dns_primary_secondary_container_drill.sh`：构建 `goddi:dnsdrill` 轻量镜像，起 **primary、secondary 两个真实 DNS 容器**（复用产品 `transfer.AXFRHandler` / `transfer.SecondarySync`）加一个 `dig` 客户端容器，实测：
```
PASS | primary (dig) answers www.drill.test. A = 192.0.2.10
PASS | secondary (dig) mirrors www.drill.test. A = 192.0.2.10 (TSIG AXFR)
PASS | primary refuses unsigned AXFR (TSIG fail-closed)
PASS | primary down: primary no longer answers
PASS | secondary still answers after primary down (read failover)
PASS=5 FAIL=0
DRILL: OK
```
运行：`bash scripts/dns_primary_secondary_container_drill.sh`（需要 Docker）。
CI 侧的轻量版本：`make verify-dns-drill`（loopback，无需 Docker/外部 dig）。

### 已知产品缺口（诚实）
代码里**没有任何应用路径写入 `dns_zone_transfer`**（AXFR 的 TSIG 授权绑定表）。也就是说只靠管理台/API 建不出"zone ↔ TSIG key"的传送授权，`HandleAXFR` 的 `JOIN dns_zone_transfer ... tsig_key_name` 会一直不匹配。因此本容器演练用 harness 直接写入该绑定（真实产品行为仍受此限）。建议后续补一个 `dns_zone_transfer` 的管理 API/UI（允许按 zone 绑定 TSIG key + 允许来源），否则方案 A 在纯 UI 部署下不可用。完整 `goddi serve` 双容器版还受控制→DNS 复制、TSIG 密封等编排影响，本演练用 harness 规避这些以便稳定复现。

## 8. 产品化传送授权 API（dns_zone_transfer）
补齐了此前"应用代码不写 `dns_zone_transfer`"的缺口，新增管理 API（写控制库，随数据面复制到 DNS 平面）：
- `GET    /api/v1/dns/zones/{id}/transfers` 列出授权；
- `POST   /api/v1/dns/zones/{id}/transfers` `{allowed_cidr, tsig_key_name}` 新增（CIDR 校验；裸地址自动补 `/32`|`/128`；`tsig_key_name` 必须存在）；
- `DELETE /api/v1/dns/zones/{id}/transfers/{transferId}` 删除。
路由在 `internal/api/router.go`（RBAC dns read/write/delete + 区域权限 view/modify/delete），处理在 `internal/api/handler/dns_zone_transfer.go`，OpenAPI 目录已登记。

**实机验证（18080 真实产品）**：登录 → 建 `prodpath2.test.` 区 → 建 TSIG key（回传 secret）→ `POST /transfers` 绑定 `127.0.0.1/32`+key（201）→ 建 A 记录 `www=192.0.2.77` → 用产品 `SecondarySync` 从 `127.0.0.1:15353` 拉取成功：`PRODUCT_SYNC_OK records=3 sampleA="192.0.2.77"`。

**运维注意（已知限制）**：DNS 的 TSIG 密钥映射在**进程启动时加载一次**（`main.go` 只调一次 `SetTSIGSecretMap`，运行中的 `dns.Server` 持有启动快照）。因此**新增 TSIG key 后需重启 DNS 进程（或重启 goddi）才生效**；本验证即"建 key→重启→AXFR 成功"。同类产品通常也需 reload。后续可选增强：实现运行期安全热加载（需把 `dns.Server` 改为使用动态 `TsigProvider` 并加并发安全的快照刷新）。
**UI**：本轮交付的是 API（管理台后续可接入该三端点；未新增 Vue 页面）。
