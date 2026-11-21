# 审计整改跟踪（Remediation Tracking）

对应审计报告的问题清单（G/D/C/DB/A/T 系列）。状态：DONE（本分支已完成）/ DEFERRED（需可编译或可执行环境）。

分支：`chore/audit-remediation-20261121`

## P0 — 口径统一（DONE）
| ID | 处理 | 文件 |
|---|---|---|
| D1 | STATUS 证据指针更新至 round 392 | `docs/STATUS.md` |
| D2 | 10 号审计标记为已被取代的历史快照 | `docs/audit/10-final-audit-report.md` |
| D4 | README 中英 DNSSEC 口径与“已实现并测试”对齐 | `README.md`、`README.zh-CN.md` |
| D5 | 页面数统一为 31（业务叶子路由） | `STATUS.md`、`tdesign-migration.md`、`CHANGELOG.md` |
| D6 | README 501 清单删除过期的 DHCP HA 扩展 | `README.md`、`README.zh-CN.md` |
| D12 | CONFIG-REFERENCE 去掉 v0.9.0 过期引用 | `docs/CONFIG-REFERENCE.md` |

## P1 — 重复/死物/未完成（DONE）
| ID | 处理 |
|---|---|
| G1/G2 | 删除 4 个已合并 `codex/*` 与重复 `enterprise-p0-sqlite` 远端分支（远端现仅 `main`） |
| G5 | 移除入库 `dnsprobe` 二进制（根 + tests/h2），并加入 `.gitignore` |
| C3/DB1 | 删除 sqlc 死配置（`sqlc.yaml`、`queries/`、`database/migrations/placeholder.sql`） |
| C5 | 删除 web-admin Soybean 模板 CHANGELOG/README.en_US/soybean.jpg；README 重写；version→0.25.1 |
| T3 | 删除 `e2e/tests/console.spec.ts`、`workflows.spec.ts`（明确 STALE 且被 CI 排除）；清理 config |
| T4 | 删除 `test/browser/tmp-inspect*.cjs` |
| T5 | Makefile/STATUS 套件口径对齐（24 个浏览器/静态套件，本地不进入 CI） |
| D7/D8/D13 | 历史计划/测试报告加“历史归档”横幅 + `docs/ARCHIVE-INDEX.md` |
| D9 | `docs/RELEASE-NOTES-INDEX.md`：声明 `CHANGELOG.md` 为 v0.24+ 权威，登记缺口（保留历史文件，未删除） |
| D10 | PRD 标记历史（见 ARCHIVE-INDEX） |
| D11 | 移除 contracts/P0-G1 的绝对主机路径（改为“外部资料包，未随仓库提供”） |
| C4 | 修正 router.go / extension.go 的“501”陈旧注释（DoT/DoH/DoQ 已实现） |

## P2 — 结构治理
| ID | 状态 | 说明 |
|---|---|---|
| DB2 | DONE | 新增 `docs/MIGRATIONS-MAP.md`（迁移目录→目标库登记） |
| A3 | DONE | 新增 `docs/COMPAT-ALIASES.md`（兼容别名登记） |
| C1 | DONE(文档) | 新增 `internal/enterprise/README.md` 边界声明；长期去留待单独决策 |
| C2 | DEFERRED | `cmd/goddi/main.go`（4054 行）拆分：需可编译环境验证，建议单独 PR 并跑 `go build/test` 后合并 |
| G4 | DEFERRED | `go.mod` 模块路径 `github.com/jasonwa/goddi` vs 远端 `jincaiw/GoDDI`：改模块路径影响全仓 import，需全量编译验证，单独批次处理 |

## 验证说明
- 本沙箱 `go build ./...` 因 Go 构建缓存目录无写权限（`operation not permitted`）无法执行；
  上述 Go 代码仅改动**注释**（router.go/extension.go），不影响编译。
- 待可编译环境补跑：`make verify`、`make verify-e2e`、`make verify-browser`。
