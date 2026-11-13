# ADR-0013：多节点配置复制（A）优先，分布式写入授权（B）延后

- 状态：**Implemented（2026-11-13）**——路线 1 已落地：配置复制 A 完成（P0–P5），保留 ADR-0011 静态单作者；B 延后
- 关联：ADR-0005（多节点配置发布协调，未实现）、ADR-0011（单作者规则）
- 范围：B8.7 的落地取舍

## 背景

ADR-0005 把「多节点配置发布」作为一个整体，包含两件性质完全不同的事：

- **A. 配置复制**：把 author 节点的 zone/scope/IPAM 配置**有序复制**到另一节点，使其控制库拥有同一份配置，再经本地 dataplane sync 落到 `dnsdata.db`/`leases.db`。
- **B. 分布式写入授权**：leader 选举、quorum、witness、epoch fencing，保证**任何分区里只有一个可写作者**。

当前只落地了 **ADR-0011 静态单作者**（`cluster.config_author` + API 层 409 fencing）：它管住"谁可以写"，因此两节点**不可能各自发布分叉的 revision**。但配置**不会**从 node-a 自动出现在 node-b。

直接后果（已由 H2 auto-failover 演练实测）：备端提升为 primary 后，本地没有 scope 定义，**无法分配租约**——这是当前最大的功能缺口（auto-failover 唯一 NOT_VERIFIED）。

关键判断：在"2 台 VM、`config_author` 为部署时写死的静态声明"这一形态下，**B 基本不需要**——写端已被 ADR-0011 固定，不存在两个作者。用户价值全部集中在 **A**。

## 决策

### 选择路线 1：实现 A（配置复制），保留 ADR-0011 单作者，B（协调器）延后

**理由**

1. **A 是单写者、只读跟随的流**：只有一个 author，follower 只读、按序应用，与 DNS secondary、DHCP mirror 同理，**不可能产生第二个写入者**，因此不需要 leader 选举/quorum/witness。
2. **风险与工作量最低**：单向复制可做成独立开关（默认关），不改变任何现有运行行为。
3. **直接解决用户痛点**：让 failover 后备端"能立即服务"，把 auto-failover 的 NOT_VERIFIED 转 PASS。
4. **B 只在"作者节点本身也要自动故障切换"时才需要**，且需要 ≥3 故障域与真实分区验收，风险高、周期长，作为独立里程碑。

### 边界（诚实不冒进）

- `internal/cluster.ErrNotImplemented` **保留**，它属于 B。A 不触碰它。
- A 的实现放在独立包 `internal/configrepl`（follower 侧模型）+ 独立开关 `cluster.config_replication`（**默认 false**）。
- A **只宣称**覆盖 ADR-0005 最小原型验收里的**幂等、水位 gap、rollback** 类（见下）；**不宣称**覆盖旧 epoch 拒绝、少数分区拒绝、fence rejoin、leader 故障——那些是 B。

## 设计要点（复用现有接缝）

- **源**：author 控制库已有 `config_revisions` + `config_release_outbox`（`migrations/019_config_revisions.sql`）。`config_release_outbox.id` 为 `INTEGER PRIMARY KEY AUTOINCREMENT`，即**单调序列**，天然作为复制流的 `seq`。
- **传输**：复用通用 relay `facts.NewRelay(producerOutbox, inbox, db)` 与 `appendNewer` + `seq==applied+1` 语义（`internal/dataplane/sync.go`）。
- **应用**：follower 按序经现有 `configver` adapters（`adapters_dhcp.go`、`adapters_dns.go`）写本地控制库资源表；gap 则暂停并请求快照。
- **生效**：follower 的 `DomainDHCP`/`DomainDNS` sync 把新 scope/zone 推到 `dnsdata.db`/`leases.db`；既有 `OnApplied(DomainDHCP) → reloadDHCPIndex` 让 scope 立即进 WAL 索引。

## 分阶段落地

| 阶段 | 内容 | 状态 |
|---|---|---|
| **P0** | `internal/configrepl` follower 模型 + 契约测试（幂等、stale、gap 暂停不跳过、rollback 为新 revision、未知结果可查、key 冲突、资源携带） | ✅ 本轮完成 |
| P1 | 迁移 `036_config_repl_state.sql`（follower 水位） | ✅ 完成 |
| P2 | 快照读取/应用（`internal/configrepl/snapshot.go`：`ReadSnapshot`/`ApplySnapshot`，replace_whole / replace_control_authored；内容指纹作版本）+ HTTP 端点 `GET /internal/cluster/config/snapshot`（共享 token） | ✅ 完成 |
| P3 | follower 拉取器 `internal/configrepl/puller.go`（按指纹变化拉取、整表事务替换、写水位、唤醒数据面） | ✅ 完成 |
| P4 | 装配 `cluster.config_replication*`（默认关）+ main follower 循环 + 唤醒 dns/dhcp runner；dataplane sync 把 scope/zone 落到本地数据面 | ✅ 完成 |
| P5 | H2 验收 `make h2-config-replication`：**PASS=10 FAIL=0** | ✅ 完成 |

### 实现说明（重要取舍）

- **版本 = 内容指纹**（FNV-64 over 表行），而非 `config_release_outbox.id`。因为 DHCP scope 通过控制台 API 直接写表、**不产生 outbox 行**，用 outbox id 会恒为 0，follower 永不拉取——这一点由 H2 实测暴露并修正。
- **快照是整表替换、幂等**：不需要 seq/applied+1 逐条重放；漏掉一次拉取由下一次全量快照自愈。P0 的 `Follower` 顺序重放模型仍保留为协议契约与单元测试基础。
- **自动提升被拒时重试**：`docker kill` 使拨号瞬间 connection-refused，4 个窗口远早于 15s 静默窗，Takeover 被拒；修复为回调返回 bool、被拒则继续重试直到静默窗满足（否则会永久停在 standby）。

### 端到端验收（H2，真实 udhcpc）

node-a 建 scope → **node-b 自动出现在控制库与数据面（无手工）** → node-a 基线 DORA 成功 → `docker kill node-a` → **node-b 自动提升为 primary 并直接从复制来的 scope 分配租约 10.77.3.121（无手工重建 scope）**。这正是"提升后可立即服务"的缺口闭环。

## 验收映射（ADR-0005 最小原型 7 条）

- **A 覆盖**：①同一 publish_id 重试只产生一个 revision（`configrepl` 幂等）；④水位 gap 暂停不跳过（`configrepl` ErrGap）；⑤rollback 生成新 revision（`configrepl`）。
- **B 才覆盖**：②旧 epoch 写入被拒绝；③少数分区不能提交发布；⑥fenced 节点清理后 rejoin；⑦leader 故障后未确认结果可查询且不重复（A 提供"按 key 查询"的一半，leader 故障切换属 B）。

## 后果

- **正面**：failover 后能立即服务；缺口从"无"到"有界"；默认关闭，零行为变化；为将来 B 打好数据通路。
- **保留风险**：A 不解决"作者节点故障时配置不可写"——这需要人工把 `config_author` 移到另一节点（ADR-0011 仍是静态声明）。若未来要求作者自动切换，再启动 B。
- **不宣称**：不做 A 的 P1–P5，就不能说"跨节点配置自动一致"已实现；ADR-0005 的完整协调器结论不变。
