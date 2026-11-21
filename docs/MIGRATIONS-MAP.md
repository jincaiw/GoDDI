# 数据库迁移目录登记表（Migrations Map）

GoDDI 存在多个**相互独立**的迁移集合，分属不同 SQLite 数据库文件。修改前请先确认目标库，
**不要把控制面迁移合并进数据面（或反之）**；跨库存在同名主题（如 `dnssec_keys`、
`retire_legacy_ddns`、`zone_change_caa_fields`、`dns_owner_casefold_index`）属于设计使然。

| 目录 | 目标库 | 驱动/机制 | 编号范围 | 说明 |
|---|---|---|---|---|
| `migrations/` | 控制面主库（`goddi.db`） | goose（`assets.go` 嵌入 `migrations/*.sql`，`DB.RunMigrationsFS`） | 001–039（连续） | 权威业务库：auth/dns/dhcp/ipam/system/facts/config-repl |
| `internal/dataplane/migrations/` | 数据面库（`dnsdata.db`） | 独立 `//go:embed migrations/*.sql`（`internal/dataplane/store.go`） | 001–019（连续） | 只读数据面/复制消费侧所用表 |
| `internal/enterprise/ipamstore/migrations/` | 企业 IPAM 参考库 | 独立 embed + 摘要校验（`internal/enterprise/ipamstore/store.go`） | 001–002 | **参考/候选**，非生产 DDL，不得直接应用于生产库 |

## 已移除

- `internal/database/migrations/`（原仅含 `placeholder.sql`）：未被任何 `go:embed`/goose 使用，
  且 sqlc 死配置已移除，本目录已删除（审计整改）。

## 规则

- 新增控制面迁移：追加到 `migrations/`，编号严格递增、不留空洞（由 `internal/database/migration_numbering_test.go` 守护）。
- 新增数据面迁移：追加到 `internal/dataplane/migrations/`，独立编号。
- 企业参考库迁移：仅用于候选/测试，冻结字节与摘要不得随意变更（见 `contracts/enterprise-ipam-rowid-002.md`）。
