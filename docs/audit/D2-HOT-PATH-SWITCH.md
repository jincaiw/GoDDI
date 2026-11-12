# D2 默认 DHCP 热路径切换计划（WAL 事件溯源存储）

> 用户已授权 D2：把默认 DHCP 热路径切到 WAL 事件溯源存储，按「灰度 → 崩溃/断电/回滚演练 → 确认回滚方案」推进。本文件记录分阶段计划与每阶段验收，**未触碰生产、未提升 `ErrNoAuthority` 门禁**。

## 目标与边界
- 目标：默认写入路径从「`*lease.Manager` 同步写 SQLite」迁到「`WALLeaseStore`（MemoryIndex 权威读 + WAL 持久边界 + 异步 SQLite 投影 + facts 同源）」。
- 保留：SQLite 仍是投影/查询库（控制台读）；HA/复制语义不变；standby 不本地写。
- 不宣称：B8.5 物理验收、专家冻结；断电演练仅以 `kill -9`/容器重启等价近似，明确标注边界。

## 阶段

### Stage 1（round 343，已完成）
- `internal/dhcp/lease/wal_store.go`：`WALLeaseStore` 实现 `server.LeaseStore` 全 11 方法（Create/Reserve/Activate/Renew/Release/Quarantine/Expire/Get/GetByMAC/GetHeldByIP/FindAvailableIP），MemoryIndex 权威读 + `WALPipeline.Admit`（WAL 持久 + 异步投影）+ 审计复用 Manager 的 audit helper。
- 启动：`OpenWALLeaseStore` 先 `RecoverSQLiteProjection` 再把 `dhcp_leases`+作用域/保留地址装载进索引。
- `MemoryIndex.Snapshot()`；`WALPipeline.Drain()`。
- 契约：`internal/dhcp/server/lease_store.go` 增加 `var _ LeaseStore = (*lease.WALLeaseStore)(nil)`。
- 验证：`TestWALLeaseStoreMatchesManagerSemantics`（同一 DHCP 生命周期脚本跑 Manager 与 WALLeaseStore，语义行一致、FindAvailableIP 一致）、`TestWALLeaseStoreRestartRebuildsIndex`（重启重建索引并继续变更）。**未切默认**。

### Stage 2（round 344，已完成）：facts 同源 + 配置 + 装配
- WAL 事件携带 `Mutation`（已具备字段）；投影事务内**同时**入队 facts 信封（`SequenceAllocator.NextTx` + `ObservationOutbox.EnqueueTx`），space 由 scope→IPAM 映射在投影时解析；这样 facts 由 WAL 事件派生，不再依赖 `FactsMutationWriter` 在 Manager 上的同步事务。
- 配置 `dhcp.lease_store: sqlite|wal`（默认 sqlite）；`main.go` 用 `NewWithLeaseStore(walStore,...)` + `SetStartupGate`（启动恢复先于开 socket）+ WAL 持久边界；启用 wal 时**不**装配旧的 `SetLeaseFactsMutation`/`SetReleaseFactsMutation`（facts 改由投影发）。
- 守卫：默认仍 sqlite；`TestDefaultProcessDoesNotAssembleWALPipeline` 仍通过（默认不装配）。

**Stage 2 落地细节（round 344）**：
- `WALEvent` 增加 `Before`/`SpaceID`；`WALPipelineConfig.Apply` 允许自定义投影 applier；`WALLeaseStore` 在投影事务内（`ApplyWALEventTx` 同事务）用 `SequenceAllocator.NextTx` + `ObservationOutbox.EnqueueTx` 派生 facts 信封（复用 `MutationCommand.FactEnvelope`，OFFER 不发；space 在 admit 时解析，ErrNoRows→不发事实，其它错误→请求失败）。
- 配置 `dhcp.lease_store: sqlite|wal` + `dhcp.lease_wal_path`（默认 `<data_dir>/leases.wal`），并补 `GODDI_DHCP_LEASE_STORE`/`GODDI_DHCP_LEASE_WAL_PATH` 环境映射。
- `main.go`：`lease_store=wal` 时 `OpenWALLeaseStore` + `NewWithLeaseStore`，**不装配**旧同步 facts seam/observer（facts 由 WAL 派生）；HA 下暂拒绝 wal（守卫）。
- 测试：`TestWALLeaseStoreDerivesFactsFromTheSameWALEvent`/`...OmitsFactWithoutSpaceMapping`/`...FactsAreIdempotentAcrossRestart`（store 级）+ `TestWALLeaseStoreServesDORAAndEmitsFacts`（Server 级真实 DISCOVER/REQUEST→ACK + fact）。
- 冒烟：`lease_store=wal` 进程 boot 正常（日志 `event-sourced WAL`、WAL 文件生成、/ready ok）；默认 sqlite 路径仍健康（顺带修复 `source` 变量遮蔽）。

### Stage 3（round 345，已完成）：灰度（独立非 HA 节点先切 `wal`）
- 容器双节点：primary 切 `wal`、standby 保持 `sqlite`（或反之），观察 DORA→facts→DNS、`/ready`、复制、水位；对照 SQLite 节点行为一致。

### Stage 4（round 345，已完成）：崩溃(SIGKILL)/重启与回滚演练
- `docker kill` primary 后重启：WAL replay 使投影追平，租约不丢、facts 不丢、DNS 记录正确。
- 回滚：先 `Drain` 保证投影追平 → 配置切回 `sqlite` → 重启；验证租约/事实/控制台一致，无重复/丢失。

## 回滚方案
1. 配置 `dhcp.lease_store` 改回 `sqlite`；2. 重启前确保 WAL 已 drain（投影追平，`WALPipeline.Drain`/Close 顺序）；3. 用 `*Manager` 直接读 SQLite 投影——因为它已是 WAL 的最新快照，回滚不丢数据；4. 保留 WAL 文件以备再切。

## 当前进度
- **Stage 1–4 与默认切换均已完成（round 343–348）**：默认 `dhcp.lease_store=wal`；参考 HA 默认 primary=WAL/standby=sqlite；回滚=`sqlite`。详见上文各 Stage 与「默认切换」节。

## Stage 3/4 实测（round 345）

可复现脚本：`scripts/d2_wal_canary_drill.sh`（`PASS=9 FAIL=0 DRILL: OK`）。做法：独立**非 HA** 容器节点跑 `lease_store=wal`，同网段真实 `udhcpc` 客户端；生产 HA 对保持 sqlite（HA+wal 仍拒绝，见守卫）。

- **灰度（wal 节点真实 DORA）**：客户端拿到 192.168.118.240；`event-sourced WAL` 已装配；WAL 文件生成。
- **facts 同源**：确定性证据见 Go 测试 `TestWALLeaseStoreDerivesFactsFromTheSameWALEvent`、`TestWALLeaseStoreServesDORAAndEmitsFacts`（bind/activate→renew→release 产对应 facts，OFFER 不发，Server 级 DORA→ACK+fact）；容器演练中也观察到 facts→DNS 记录（异步 relay，脚本列观察项）。
- **崩溃（SIGKILL）**：`docker kill` 后重启，节点恢复健康、DORA 继续被服务（WAL replay 生效）；Go 级 `TestWALLeaseStoreReplaysUnprojectedMutationsAfterCrash` 证明"未投影的 durable 事件重启后既补投影租约、又补发 facts，且不重复"。
- **回滚到 sqlite（同一数据目录）**：`lease_store=sqlite` 重启后健康、WAL 分支未装配、DORA 继续服务；Go 级 `TestWALLeaseStoreRollbackToSQLiteManagerSeesTheSameData` 证明回滚后 `*Manager` 读到同样租约。

边界（诚实）：容器为独立非 HA 灰度节点，`/ready` 在合成种子环境可能因控制库复制 FK 显示 degraded（种子产物，非 WAL 缺陷）；断电以 SIGKILL/容器重启近似，非物理断电。HA 主备灰度已打通（见 round 347）：primary 跑 WAL、standby 常驻为 sqlite 镜像（standsAside），被提升节点靠重启从已镜像 SQLite 快照装载 WAL 索引。

## Stage 3/4 扩展：HA 主备灰度（round 347）

脚本 `scripts/d2_ha_wal_canary_drill.sh`（`PASS=17 FAIL=0 DRILL: OK`）+ `docker-compose.d2-canary.yml`（仅把 **primary** 切 wal，standby 保持 sqlite）。复用 HA 演练卷的真实 scope(ha-bridge 192.168.117.0/24)/zone(ha.test)/IPAM(ha-net-space)。

- **一节点切 wal**：primary 日志 `event-sourced WAL`、standby 无；两节点 `/ready` ok。
- **真实 DORA**：客户端从 wal primary 拿到租约；primary 有 lease+fact(载荷含该 IP)+DNS 记录；**standby(sqlite) 镜像到同一租约**。
- **SIGKILL primary**：standby 保持 standby（不自动提升，fail-closed）；停机期间客户端拿不到租约。
- **重启 wal primary**：`/ready` ok、WAL replay 保留租约（含被杀前的）、新 DORA 正常。
- **回滚 sqlite（同卷）**：primary 健康、WAL 分支未装配、租约数不变、新 DORA 正常。
- 说明：HA 下 standby `standsAside` 不构造 DHCP server（镜像直写其 SQLite 库）；takeover 为重启式，promoted 节点启动时由 WAL store 从已镜像的 SQLite 快照装载索引。HA 主备灰度因此无需连续索引同步。

## 默认切换（round 348）

- **shipped 默认**：`config.yaml` 的 `dhcp.lease_store` 默认设为 **`wal`**（注释说明可设 `sqlite` 回退）。因此参考 HA 部署（`docker-compose.ha.yml`）默认 **primary 跑 WAL store**，standby 处于 standby（`standsAside`，不构造 server，镜像直写其 SQLite 库）。
- **回滚开关**：任一处可设 `dhcp.lease_store: sqlite`（或 `GODDI_DHCP_LEASE_STORE=sqlite`）即回到同步 `*Manager`；演练已验证同卷回滚不丢数据。
- **CI 稳定**：e2e 前端实例显式钉 `GODDI_DHCP_LEASE_STORE=sqlite`（该套件只测 API/UI，不测租约后端；WAL 热路径由 D2 演练与参考 HA 默认覆盖）。
- **全矩阵（默认 wal 下）**：`make verify` OK；`verify-e2e` 4+8；`verify-browser` 20 套件 0 失败；全页面巡检 41/41；`verify-race` OK（0 DATA RACE）。参考 HA 默认（primary wal）实测真实 DORA→lease+fact(<ip>)+DNS+standby 镜像、双节点 `/ready` ok。

## standby-wal 就绪（round 349）

- `WALLeaseStore.ReloadFromDB(ctx)`：从 SQLite（standby 由 HA 镜像写入的库）重新装载索引后服务。HA 的 standby 处于 `standsAside`、不构造 DHCP server，其 SQLite 租约库由镜像保持最新；被提升时（重启式）新进程由 `OpenWALLeaseStore` 从该库装载快照即可服务，`ReloadFromDB` 亦可用于非重启的现场提升。
- 就绪证据：`TestWALLeaseStoreReloadFromDBServesMirrorPopulatedRows`（镜像直写 SQLite 的租约 → `ReloadFromDB` 后 `GetHeldLeaseByIP` 命中、`FindAvailableIP` 跳过、可继续分配）。
- 边界：standby 常驻仍不运行 DHCP server（不需要）；常驻 WAL 写的 standby 未实现，也不需要——提升靠重启/Reload 从已镜像快照装载。
