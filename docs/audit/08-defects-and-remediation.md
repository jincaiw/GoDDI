# 08 · 缺陷与整改（P0 / P1 / P2）

> 基线 commit `8e769d7`。状态定义：PASS / PARTIAL / FAIL / NOT_IMPLEMENTED / NOT_VERIFIED。仅提出整改，**不修改业务/安全决策**。

## P0（上线前必须修复或由 ADR 明确裁决）

### D-01 数据面 HA 复制无 mTLS / 证书轮换 / 加密防重放
1. 问题/触发：primary↔standby 复制通道为**明文 TCP + 共享 `peer_token`**；任一同网段攻击者可窃听/伪造租约帧；token 泄露即完全接管镜像。
2. 源码：`internal/dhcp/ha/config.go`（`PeerToken`/`ListenAddr`/`PeerAddress`）、`internal/dhcp/ha/protocol.go`（Hello 携带 token，无签名）、`replicator.go:409-535`（session/handshake）；ha 包无 `tls.`。
3. 影响：租约数据泄露、镜像被投毒、脑裂误判、审计不可信。
4. 根因：为降低依赖选择明文+token，未落地 mTLS/证书生命周期。
5. 修复：复用 `internal/tlsutil`（`tlsutil.go:58-89` 已支持 `RequireAndVerifyClientCert`）为复制通道启用 mTLS；帧加 nonce/时间戳+序号防重放；证书轮换与吊销（短期证书 + 根 CA 轮换）。
6. 测试：错误/过期证书拒绝；重放帧拒绝；中间人不可注入；证书轮换不断链。
7. 依赖/顺序：先于任何生产 H2 上线；与 D-02 一起改（fencing 需可信对端身份）。
8. 验收：无有效 mTLS 客户端证书一律拒绝；抓包无法读到明文租约。

> **状态更新（2026-11-12）**：E-2 已实现——复制通道可选 **mTLS**（`ha.LoadTLS` + `Config.serverTLS/clientTLS`，`RequireAndVerifyClientCert`）；hello 增 `Nonce`+`SentAt` 防重放（`validateHelloFreshness`，仅 TLS 路径强制）；配置 `dhcp_ha.tls_{cert,key,ca}_file`/`tls_server_name`（含 env 与 CONFIG-REFERENCE）。测试 `tls_test.go`（mTLS 往返、明文拒绝、错 CA 拒绝）通过。生产环境**必须**配置 TLS（缺省 nil=旧明文，仅为兼容）。

### D-02 无 Owner Epoch（写入栅栏）
1. 问题/触发：HA 用 `ha_role/takeover_at/fenced_at` 表达角色（`internal/dhcp/ha/role.go:28-53`），**没有单调 epoch 令牌**在**每次写**时校验；旧主若未被 fence 且状态误判，可能继续写。
2. 源码：`role.go`（EffectiveRole `:115`、Takeover `:425`、Fence `:584`、Rejoin `:626`）；写路径 `internal/dhcp/server/handler.go:497-580` 未校验 epoch。
3. 影响：并发双写/旧主回归/脑裂的**理论窗口**（当前靠 paused 抑制，但非证明）。
4. 根因：所有权仅节点级角色，未把 epoch 作为持久、单调、写前强校验的围栏令牌。
5. 修复：引入 `owner_epoch`（持久化于 `dataplane_meta`），Takeover 单调 +1；所有权威写在事务内校验 `request.epoch == current_epoch`，落后即拒绝并审计；旧主重连/启动读到更高 epoch 立即进入 fenced。
6. 测试：旧 epoch 写入拒绝；epoch 回退拒绝；并发 takeover 只有一个成功。
7. 依赖：D-01（可信身份）之后。
8. 验收：任何落后 epoch 的写 100% 被拒并产生审计记录。

> **状态更新（2026-11-12）**：E-1 已实现——`dataplane_meta.ha_owner_epoch` 持久化、`Takeover` 事务内单调 +1、握手交换 `Watermarks.Epoch`、见更高 epoch 即 `StateFenced`（`MayBind=false`，不 ACK）；单测 `epoch_test.go`（4 例）通过。剩余：多节点 quorum 证明（E-3）与更细粒度写路径 epoch 参数。

### D-03 facts 无哈希链（无篡改/分叉证明）
1. 问题/触发：`facts.Envelope`（`internal/facts/event.go:21-37`）仅 `EventID/Sequence/Source/Generation`，**无 PrevHash/PayloadHash/TipHash/Epoch**；`Watermark` 只检测 sequence 缺口（`event.go:70+`）。
2. 源码：`internal/facts/event.go`、`internal/facts/replica_stage.go`、`internal/ipam/facts_consumer.go`。
3. 影响：无法检测篡改/分叉；不能证明"备覆盖全部已承诺租约"；审计不可证。
4. 根因：事实信封定位为"协议边界"，未纳入不可篡改链。
5. 修复：Envelope 增 `epoch/prev_hash/payload_hash/tip_hash`；出箱时计算并链式串联；消费者校验链与 tip；不匹配进入阻断（对应 DEC-007）。
6. 测试：篡改 payload/断链/tip 不符 → 拒绝并阻断；重放幂等。
7. 依赖：D-02（epoch）先定；与镜像回执改造同批。
8. 验收：任何链断裂/篡改被检测并阻断，不自动恢复。

> **状态更新（2026-11-12，阶段一）**：已实现链**密码学内核与校验器**（`internal/facts/chain.go`：`PayloadDigest/ComputeTip/ChainBuilder.Stamp/VerifyEnvelope/VerifyChain`、`ChainTipStore`（CAS 防分叉）+ `StampTx` 事务内集成点；`Envelope` 增 `Epoch/PrevHash/PayloadHash/TipHash`，向后兼容）。测试 `chain_test.go`（篡改/乱序/丢事件/epoch 回退/tip 竞争/`StampTx` 链）通过。**待接**：把 `StampTx` 接入 lease 生产 outbox、把 `VerifyEnvelope` 接入 IPAM/DNS 消费者并持久化 tip（需 outbox 表增列 + 双库迁移滚动）——属阶段二，未完成前链路不宣称端到端生效。

### D-04 DEC-006 与 ADR-0003/实现冲突（复制失败是否继续服务）
1. 问题/触发：任务书 DEC-006 要求复制中断时原 Owner 继续服务；实现默认 `paused`→不 ACK（`state.go`、`handler.go:491`），ADR-0003 明确"暂停新租约"。
2. 源码：`internal/dhcp/ha/state.go`（`MayBind`/`ServesClients`）、`handler.go:491`、`main.go:273-291`（degrade）。
3. 影响：若按 DEC-006 验收，当前实现**不满足**；若按 ADR-0003，则需修订 DEC-006。悬而未决=验收风险。
4. 根因：两份决策冲突，未收敛为单一 ADR。
5. 修复（决策，不是代码）：提交 ADR 变更，明确"复制中断默认：暂停新承诺（fail-closed）vs 本地持久化后继续（degraded 自动）"；若选继续，则实现**自动进入 primary-degraded**并强审计（而非要求人工 degrade）；若选暂停，则修订 DEC-006 文本。
6. 测试：断链后行为与所选 ADR 一致；状态与审计可观测。
7. 依赖：需用户/架构裁决（本轮不擅改）。
8. 验收：DEC 与 ADR 文本一致，行为有测试钉住。

> **状态更新（2026-11-12，已批准）**：用户批准 **ADR-0010**。D-04 关闭（决策已定）：**默认 `paused`（不承诺新/延长租约）；自动 degraded 继续仅当 Owner Epoch 最新 + quorum/witness + mTLS 齐备**。DEC-006/007 已建立（`docs/decisions/`）；实施任务见 `09` 的 E-1/E-2/E-3。

### D-05 多节点配置发布协调未实现 + "集群全部应用"状态缺失
1. 问题/触发：`cluster` 为 `ErrNotImplemented`（`internal/cluster/cluster.go:14`）；`/cluster` 501（`extension.go:114-124`）；无集群 `applied` 视图。
2. 源码：`docs/adr/0005...`、`internal/configver/service.go`（单节点 revision/outbox）、`dataplane/sync.go`（每节点复制）。
3. 影响：双节点可能短暂/长期不同版本；管理面单节点成功可能被误当集群成功；无 leader fencing。
4. 根因：发布协调协议仅设计。
5. 修复：实现 ADR-0005（leader epoch/fencing/quorum 或明确 witness）；暴露每节点 `applied_revision` 与集群一致判据；管理面区分三态。
6. 测试：发布中断/节点宕机后一致性；分区下拒绝双 leader 写入。
7. 依赖：D-01/D-02 之后。
8. 验收：三态可查；中断不产生两个"已提交"版本。

> **状态更新（2026-11-12）**：ADR-0005 发布协调与 quorum/witness 依赖第三方仲裁，本环境无法验证 → **BLOCKED（external-validation-required）**；不实现自动 degraded，默认 paused 保持不变（ADR-0010）。

### D-06 AXFR 授权链路与 TSIG 热加载（部分已修）
1. 问题/触发：此前应用无路径写 `dns_zone_transfer`（纯 UI 无法授权 AXFR）；TSIG 密钥仅启动加载，运行期新增需重启。
2. 源码：`internal/api/handler/dns_zone_transfer.go`（本轮新增）；`cmd/goddi/main.go:845-869`（一次性 `SetTSIGSecretMap`）。
3. 影响：纯产品配置不可用（已补 API）；新 key 需重启（运维摩擦/易错）。
4. 根因：授权表无管理面；TSIG map 启动快照。
5. 修复：已完成 API；后续实现运行期安全热加载（`dns.Server` 改动态 `TsigProvider` + 并发安全快照）与 UI 页面。
6. 测试：API 建/列/删；新 key 免重启生效；两容器 dig 演练。
7. 依赖：无（API 已入）；热加载可独立。
8. 验收：纯 API 完成授权且 AXFR 成功；新 key 无需重启。

## P1
- **D-08 SQLite 磁盘满/fsync 错误未验证**：`sqlitepolicy` 只设 PRAGMA；需 ENOSPC/fsync 失败注入，要求绑定失败且不 ACK（`handler.go` 路径）。测试：填盘/错误注入。
- **D-09 IPAM 陈旧导致错分配**：**部分已实现（2026-11-12）**：新增 `internal/ipam/projection.go` 的 `ProjectionGaps`（非阻塞缺口检测，启动时告警 scopes 子网无本地 `ipam_subnets` 投影）与 `NewProjectionGuard`（**可选** fail-closed：`dhcp.require_ipam_projection=true` 时分配前拒绝无投影 scope，静默不 ACK）；Server `SetScopeProjectionGuard`，main 接线，配置/env/CONFIG-REFERENCE 已加；测试 `projection_test.go` 通过。**剩余**：真正的"相对控制面滞后"需跨面发布水位（E-3/ADR-0005），本地面无法证明；严格拦截默认关闭以免误伤独立 scope。
- **D-10 Outbox/consumer 故障注入缺失**：**部分已实现（2026-11-12）**：新增 `internal/facts/relay_test.go` 覆盖 ① relay 按序投递、② 重 tick 幂等（不重复）、③ 重启从水位续传不重复、④ inbox 不可用（表被改名模拟）时 tick 失败但**源事件不丢、水位不前进**、恢复后**恰好一次**投递；并断言 relay 从不标记 source 行（copier 非 authority）。既有 `outbox_test`（enqueue 幂等/冲突）、`replica_stage_test`（快照原子/校验失败/断点）、`facts_consumer_test`（gap/失败留箱）覆盖消费端。**仍缺**：显式 DLQ（当前失败留 inbox 重放，无死信队列）；消费者"apply 与 MarkDone 之间崩溃"的端到端故障注入未单独演练（设计上同库同事务，风险低）。
- **D-11 DHCP 报文重放/去重**：无显式事务号去重（依赖 IP/MAC 语义）；**已补租约层幂等/重复/并发测试并修复一处真实缺陷（2026-11-12）**：并发抢同一地址时落败方原返回裸 `UNIQUE constraint` 错误，现映射为 `ErrAddressTaken`（`internal/dhcp/lease/lease.go` `createLeaseWithStatus`），使调用方可按语义重试；测试 `internal/dhcp/lease/idempotency_test.go`（DISCOVER 重传同 ID/单行、异 MAC 拒绝、重复 Activate/Release 幂等、并发同址仅一胜者）全 PASS。
- **D-12 备份/升级恢复演练缺失**：`internal/backup/*` 存在但无恢复演练；systemd 单实例约束需文档化（`deployments/systemd/goddi.service`）。
- **D-13 遗留存储路径复杂度**：`lease_store_shadow.go`/`lease_store_indexed.go` 与 WAL store 并存；建议按灰度收敛计划清理。
- **D-14 DNS LKG/原子性**：本地副本无显式 LKG 版本戳/回滚；`replaceWhole` 多表原子性未验证（`dataplane/sync.go`）。

## P2
- **D-16 文档冲突/缺失**：任务书引用 DEC-001~019 不存在；以 ADR 0001–0009 为准，需补齐决策登记并统一编号。
- **D-17 Appliance Agent/apps 运行时**：`internal/apps/{runtime,sdk}` 未做运行/故障验证。
- **D-18 性能/长跑**：`synchronous=FULL` + 每绑定一次对端确认 → 吞吐/延迟未基准（需压测/soak）。
- **D-19 集群身份**：无 ClusterID / 节点唯一性仲裁（依赖部署约定）。
- **D-20 可观测性**：合成种子下 `/ready` 可能因控制库 FK 显示 degraded（种子产物），需区分真实缺陷与种子噪声。
