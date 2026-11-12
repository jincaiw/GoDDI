# 06 · SQLite / WAL / 事务持久化审计

> 只读审计，基线 commit `8e769d7`。已执行 `go test ./internal/database/ ./internal/sqlitepolicy/ ./internal/dhcp/lease/` 全绿。

## 1. 连接级 PRAGMA（实现证据）
`internal/sqlitepolicy/connection.go`：
- `:83` `busy_timeout = 5000`
- `:84` `foreign_keys = ON`
- `:85` `journal_mode = WAL`
- `:86` `synchronous = FULL`
驱动连接钩子执行这些 PRAGMA（`:83-96`），并有测试断言期望值（`internal/database/sqlite_connection_test.go:23-24`：busy_timeout=5000、foreign_keys=1、journal_mode=wal、synchronous=2(=FULL)）。
- **判断：PASS**——`synchronous=FULL` 意味着每次事务提交在返回前完成 fsync（WAL 下 fsync WAL 文件），满足"提交即持久"的前提。
- 缺口：`sqlitepolicy` 只校验/设置 PRAGMA，未见对 **磁盘满 / fsync 返回错误** 的上层降级测试（NOT_VERIFIED，见 07）。

## 2. WAL 文件与 fsync（DHCP 事件溯源热路径）
- `internal/dhcp/lease/wal.go:144-147` `WALWriter.Sync()` → `file.Sync()`（真实 fsync）。
- `internal/dhcp/lease/wal_file.go:55` 写头部后 `file.Sync()`；`:121` 追加后 `w.writer.Sync()`；`:128-134` `WALFile.Sync()`。
- `internal/dhcp/lease/wal_lifecycle.go:105,111` 提交/关闭边界 `l.wal.Sync()`。
- **判断：PASS**——WAL 追加在返回前 fsync；`WALLeaseStore.admitEvent` 走 WAL durable 边界再异步投影（`internal/dhcp/lease/wal_pipeline.go`、`wal_store.go`）。
- **注意**：异步 SQLite 投影发生在 WAL 落盘之后，因此"WAL 已持久但未投影"是合法中间态；崩溃后由 `wal.Replay`/`RecoverSQLiteProjection` 补投影（`wal_pipeline.go`；演练 `TestWALLeaseStoreReplaysUnprojectedMutationsAfterCrash`）。**PASS（有单测）**。

## 3. 事务边界
- facts 序列分配：`internal/facts/sequence.go` `NextTx` 要求"与 mutation/outbox 同一 SQL 事务"（注释 `:16-18`）。
- IPAM facts 消费：`internal/ipam/facts_consumer.go:85-126` `ProcessOne` 把"投影 + `watermark.AdvanceTx` + `outbox.MarkDoneTx`"放在同一事务提交（注释 `:69-84`）。**PASS（有单测）**。
- DHCP WAL store 投影：`wal_store.go applyEvent` = BeginTx → `ApplyWALEventTx`(lease+watermark) → facts outbox enqueue（同事务）→ Commit。**PASS（有 parity/facts 测试）**。

## 4. ACK 前置持久化
`internal/dhcp/server/handler.go:497-580`：先 `CreateLease/ActivateLease/RenewLease`（提交）→ 复制确认（`Confirm`/`ConfirmFacts`）→ `RequireDurable` gate → 才构建 ACK（`:617`）。SQLite 路径 `synchronous=FULL` 保证提交即 fsync；WAL 路径 `file.Sync`。
**判断：PASS（代码路径 + 单测）；运行级"ACK 后立即断电不丢租约"见 07（部分有 SIGKILL 演练，真实断电 NOT_VERIFIED）**。

## 5. 并发/锁竞争
- `busy_timeout=5000ms`（5s）——高并发写超出会 `SQLITE_BUSY` 超时；`internal/dhcp/lease/async_apply.go` 修复了 `Wait/Drain` 的提前空闲竞态（accepted/applied 计数，回归 `async_apply_wait_test.go`）。**PASS（有回归）**。
- 单库单写者（控制库）；多进程同时写同一 `goddi.db`（如 systemd 单实例 + CLI 维护）未做多写者仲裁（部署约束，非缺陷，但需文档化）。

## 6. 备份/升级
- `internal/backup/*`（snapshot/export/加密）存在，`migrations/` goose 版本化；**恢复/升级运行演练 NOT_VERIFIED**。

## 7. 结论
| 项 | 状态 | 证据 |
|---|---|---|
| PRAGMA WAL/FULL/busy_timeout/FK | PASS | `sqlitepolicy/connection.go:83-96` |
| WAL 追加 fsync | PASS | `wal.go:144-147`、`wal_file.go:55,121,128` |
| 崩溃后 WAL 重放补投影 | PASS（单测+演练） | `wal_pipeline.go`、`TestWALLeaseStoreReplaysUnprojectedMutationsAfterCrash` |
| facts/序列/outbox 同事务 | PASS（单测） | `facts/sequence.go`、`ipam/facts_consumer.go:85-126` |
| 磁盘满/fsync 失败/锁超时故障注入 | **NOT_VERIFIED** | 无对应运行测试 |
| 真实断电（非 SIGKILL）持久性 | **NOT_VERIFIED** | 需物理真机（B8.5 BLOCKED） |
