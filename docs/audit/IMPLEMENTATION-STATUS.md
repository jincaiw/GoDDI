# 实施状态与证据台账（隔离工作树）

> 本文件是「根据开发文档，完成全部实施任务」的**完成边界与证据索引**，不是生产验收声明。**一页交接总览见 `docs/STATUS.md`**（当前状态、已完成/候选/外部阻断、一键复现、文档索引）。
> 范围约定（用户口径）：**先单机实现所有功能，双机（HA / 复制 / 多节点协调）从技术架构角度预留，最后实现**；不得触碰生产、不得提升受保护写门禁。
> 所有「实跑」证据均来自 `/tmp/goddi-run` 隔离单机（临时 DB / 配置 / 非生产网卡），**不构成** Linux、真实双主机、物理断电、真实网络分区或生产环境证据。

## 1. 验证矩阵（当前状态）

> 本节是**当前**验证状态，历史增量细节见 §1b–§1m。

| 检查 | 命令 / 入口 | 当前结果 |
|---|---|---|
| Go 构建 | `go build -mod=readonly ./...` | 0 |
| Go 测试 | `go test -mod=readonly ./...` | 0 |
| Go 竞态 | `go test -mod=readonly -race ./...` | 0 |
| Go 静态检查 | `go vet -mod=readonly ./...` | 0 |
| Go 格式 | `gofmt -l cmd internal pkg` | 空 |
| 补丁卫生 | `git diff --check` | 干净 |
| 前端 lint | `pnpm lint`（`web-admin/`） | 0 warning / 0 error |
| 前端类型 | `pnpm typecheck` | 0 |
| 前端构建 | `pnpm build` + 复制 `web/dist` | 0 |
| 纯净构建 | 清空 `web-admin/dist` / `web/dist` / `dist/` 后 `make build` | `BUILD_EXIT=0`（§1i） |
| 浏览器 + 静态契约回归 | `make verify-browser`（`test/browser/run.sh`，**20 项**） | `TOTAL_FAILS=0` / 0 console+page error |
| 生产门禁 | `go test ./internal/enterprise/prodgates/` | PASS（10 个 `ErrNoAuthority` 哨兵恒关闭） |
| 运行期 OpenAPI | 重建二进制后 `GET /api/v1/openapi.json` | 207 个操作、JSON 可解析（§1l） |

**可复现入口（仓内，无需记忆命令）**：
- `make verify` —— Go build/test/vet + gofmt 检查 + 前端 lint/typecheck/build + `git diff --check`。
- `make verify-race` —— 上述后再跑 `go test -race ./...`。
- `make verify-browser` —— 运行中的隔离服务 + Playwright，前置 `GODDI_ADMIN_PASS`；见 `test/browser/README.md`。
- `make build` —— 从源码干净构建 `./dist/goddi` 与 `web/dist`。
- **注**：`pnpm lint` 带 `--fix`，运行后应提交其格式化结果；`verify-browser` 不含浏览器二进制，Chromium 由 `common.cjs` 自动探测或 `GODDI_CHROMIUM` 指定（§1f/§1h/§1i）。

**浏览器与静态检查清单（20 项）**：静态 `docs`（交接文档一致性，§1z）、`contracts`（前端↔后端契约，§1m）、`i18n`（DNSSEC 提示措辞，§1k）；浏览器 `smoke`、`batch2`–`batch16`、`legacy`（`batch16` 为只读角色 UI，§1aw-补）。覆盖登录/空布局(403/404/500)、全部 30 个 TDesign 页面（含独立 DNSSEC 密钥页，§1j）、CRUD 端到端（区/记录/子网/转发器/保留地址/令牌/组/用户/角色/允许规则）、IPAM 弹窗与抽屉、DNS 调试客户端真实查询。

### 1.1 增量索引（供评审导航；各节内容为该轮实测与证据）

- `1b. 分离角色（control + dns）隔离实跑（round 124）`
- `1c. DHCP HA 隔离安全模式实跑（round 125）`
- `1d. B8.5 外部验收准备（round 126）`
- `1e. 独立评审材料（round 128）`
- `1f. 浏览器回归 harness 仓内化（round 129）`
- `1g. 完成边界：还需用户提供什么（round 130）`
- `1h. 浏览器 harness 依赖可复现（round 131）`
- `1i. 纯净构建可复现（round 132）`
- `2. 单机实施任务台账`
- `3. 按开发文档**有意保留**的项目（非未完成）`
- `4. 外部阻断（本沙箱无法闭合，非代码缺陷）`
- `5. 未声明的内容`
- `6. 相关文档索引`
- `1j. 批次 F 前端待办收口：独立 DNSSEC 密钥生命周期页（round 133）`
- `1k. 全仓文档 TODO 复核与 DNSSEC 提示一致性（round 134）`
- `1l. OpenAPI 契约全量补齐与双向一致性门禁（round 135）`
- `1m. 前端↔后端静态契约门禁（round 136）`
- `1n. 证据台账 §1 更新与 RBAC 权限门禁（round 137）`
- `1o. 未测包清理与 SSRF 双层守卫对齐（round 138）`
- `1p. 全量 API 认证覆盖门禁（round 139）`
- `1q. 迁移编号连续性门禁（round 140）`
- `1r. 全量 CSRF 覆盖门禁（round 141）`
- `1s. 授权深度门禁：零权限用户不可读（round 142）`
- `1t. 浏览器页面覆盖门禁（round 143）`
- `1u. 单机优先 / 双机预留门禁（round 144）`
- `1v. 旧快照迁移路径：从「尚未定义」到「候选定义待冻结」（round 145）`
- `1w. 同键 UPSERT 语义变更：实测钉住 + 候选写路径定义（round 146）`
- `1x. 生产门禁清单复核与「DB 行不授予权威」回归（round 147）`
- `1y. 一页交接总览（round 148）`
- `1z. 交接文档防漂移门禁（round 149）`
- `1aa. 全仓竞态复跑与工作区卫生复核（round 150）`
- `1ab. i18n 字面量键全量校验与 10 处缺失修复（round 151）`
- `1ac. 前端 API 调用「路径 + 方法」双向契约（round 152）`
- `1ad. 配置键文档补齐与双向防漂移（round 153）`
- `1ae. 环境变量覆盖全表与防漂移（round 154）`
- `1af. 环境变量表路径准确性回归（round 155）`
- `1ag. 部署产物一致性门禁（round 156）`
- `1ah. 模块依赖卫生：`go mod tidy`（round 157）`
- `1ai. 评审材料数字同步与防漂移（round 158）`
- `1aj. 交付物质量复核与 `verify-tidy` 入口（round 159）`
- `1ak. B8.5 Runbook 结构与状态复核（round 160）`
- `1al. 交叉引用准确性复核（round 161）`
- `1am. 生产配置闸门运行期复核（round 162）`
- `1an. 随仓库 `config.yaml` 端到端启动复核与两处运维体验修正（round 163）`
- `1ao. `migrate` 只建控制库契约的回归钉住（round 164）`
- `1ap. 加密备份/恢复往返与完整性拒绝（round 165）`
- `1aq. W12 灾备演练全套复跑与变异检查缺陷修复（round 166）`
- `1ar. W 系列验证脚本全套复跑与 3 处陈旧变异目标修复（round 167）`
- `1as. 长时 W 脚本复跑：耐久 / 容量 / 三进程分裂演练（round 170）`
- `1at. 剩余 7 个脚本复跑与 d1 变异判据修复 —— 脚本清单全绿（round 171）`
- `1au. e2e fixture 套件复活：针对 TDesign UI 重写 8/8 全绿（round 172）`
- `1av. bare-config e2e 规格：api.spec 复活通过、3 个旧 shell UI 规格处置（round 173）`
- `1aw. 只读角色写拒绝门禁（round 175，接 §1av 的独有缺口）`
- `1ax. 分页跨页语义服务端钉住（round 175，接 §1av 第二块独有缺口）`
- `1ay. 交接总览同步与全量 race 复跑（round 176）`
- `1ba. `config-versions.spec` 复活：3/3 全绿（round 177）`

## 1b. 分离角色（control + dns）隔离实跑（round 124）

用户口径「单机和双机都可以运行」在隔离单机上以两个进程验证（`/tmp/goddi-run/split/control.yaml` 与 `dns.yaml`，共享控制库、各自独立监听端口）：

| 检查 | 结果 |
|---|---|
| `role: control`（18082，仅管理 API）+ `role: dns`（15355，仅 DNS）同时启动 | 两者 `/health` / DNS 监听正常；日志 `management_api=true/false`、`serves_dns=false/true` 正确 |
| 端口隔离 | control 只监听 TCP 18082；dns 只监听 UDP 15355；control 未绑定 DNS、dns 未绑定 HTTP 管理 API |
| 控制面 → 数据面复制 | 经 control API 创建 `split-role.test.` 区 + `www A 192.0.2.177`，约 4s 后 dns 进程权威应答 `192.0.2.177` |
| **控制面停机后 DNS 继续服务** | 杀掉 control 进程，dns 仍从 `dnsdata.db` 应答 `192.0.2.177`；重启 control 后两者均恢复健康 |
| 日志 | 两个进程 0 ERROR / 0 panic |

同时修正了 `cmd/goddi/main.go` 一处**误导性启动告警**：原文称数据面「尚未从本地存储提供服务」，但代码实际 `dnsDataDB = store.DB` / `dhcpStore.DB`，且上表已实测控制面停机不影响应答。改为准确表述「数据面服务路径使用本地存储，控制库仅在启动时用于迁移与复制种子」。**未改变任何运行逻辑**，仅修正日志与注释，并实跑确认新告警文案生效。

## 1c. DHCP HA 隔离安全模式实跑（round 125）

用户为双节点 DHCP HA 选择的「隔离安全模式验证」在隔离环境用两个进程完成（`/tmp/goddi-run/ha-a.yaml` primary on `utun7` / `ha-b.yaml` standby on `lo0`，各自独立控制库与 lease store，dev peer token，**无生产网络、无真实客户端**）：

| 检查 | 结果 |
|---|---|
| 干净角色启动 | 清空早期 `goddi ha takeover` 遗留的持久化 `ha_role=primary`（`EffectiveRole` 有意让运维记录的角色优先于配置，属设计而非缺陷）；重启后 `role_in_force=primary` / `standby`，standby 明确「will not serve clients」 |
| 镜像链路 | primary `state=paused` → 「mirror connected」→「sent a snapshot」→ `state=primary`；standby 「mirror connected to its primary」+「mirror rebuilt from a snapshot」 |
| **失联 fail-closed** | 杀掉 standby → primary 日志「the mirror session ended ... state=paused」；无第二副本时不再承诺绑定 |
| **重连恢复** | 重启 standby → primary 再次「mirror connected」并回到 `state=primary`，standby 重新从快照重建 |
| 错误 peer token | 伪造 standby 用同长度错误 token 拨入 → 只得到 handshake i/o timeout，**未建立 mirror session**，原有镜像链路不受影响；过短 token 直接被配置校验拒绝 |
| 运维动作 fail-closed | `goddi ha status` 正常；`degrade` 不带 `--confirm` → `ErrNoConfirmation`；在 primary 上 `takeover --confirm --old-primary-cannot-write` → `ErrWrongRole` |
| 日志 | 0 ERROR / 0 panic；结束后清空两个隔离 lease store 的 `ha_*` meta，进程全部停止 |

这是**仓内隔离双进程**证据，明确不等价于 B8.5 要求的真实跨主机、网络分区、fencing/witness、断电与真实客户端互操作验收。

## 1d. B8.5 外部验收准备（round 126）

`docs/B8.5-外部验收Runbook.md` 的「当前仓内证据」列此前较稀疏。本轮把本工作树**已实测**的证据逐项归档到 A–E 五行，并新增 §11 说明：
- A：隔离单机四次握手 / option 54 / 续租 / DECLINE / 短租约到期回收与 3 个运行期缺陷修复。
- B：`w14h_split_outage_drill.py` + round 124 两进程 control/dns 复制与停机存活。
- C：WAL `synchronous=FULL`、kill -9 / 存储注入 / 关闭 drain 契约。
- D：HA 状态机与模型测试 + round 125 隔离双进程镜像链路/失联暂停/重连/错误 token/运维动作拒绝。
- E：指标注册与示例规则 + `internal/metrics/alert_rules_test.go` 三项测试实测 PASS（每条规则引用的指标真实注册且有写入者）。
- **所有外部条目状态保持 `未验收` / `BLOCKED`**，明确「仓内单机/单机双进程证据不代表任一 B8.5 条目 PASS」；现场负责人仍须按 §10 模板登记原始证据。复跑 `go test -race ./...`、全量浏览器套件、alert rules 测试与 `git diff --check` 全部通过。

## 1e. 独立评审材料（round 128）

新增 `docs/audit/ENTERPRISE-REVIEW-DOSSIER.md`：把散落在 P0-G1 / 本台账 / 版本兼容合同 / DHCP 租约提案 / B8.5 Runbook 的候选状态汇成单一评审索引 —— 52 个 `internal/enterprise` 包、569 个测试/Fuzz 入口、10 个 `ErrNoAuthority` 哨兵的逐项清单、按域的「证明/不证明」边界、6 条版本轴、可复现验证命令、以及**独立评审必须回答的 7 组问题**（授权模型、schema 冻结、兼容矩阵、老库升级、G1–G5/ACK 语义、B8.5 外部验收、501 契约）。数量与 `prodgates` 回归实测一致。该材料**不新增权限、不改变门禁、不替代原始证据**，目的是把阻塞项「独立评审」准备到可直接开工的程度。

## 1f. 浏览器回归 harness 仓内化（round 129）

此前 15 个 Playwright 套件只存在于 `/tmp/pwsmoke/`，浏览器证据不可复现。本轮把 15 个套件 + `common.cjs` + `reset_testdata.py` + `run.sh` + `README.md` 收进 `test/browser/`，全部硬编码路径（Chromium 可执行文件、BASE URL、凭据、截图目录、测试 DB）改为环境变量，Chromium 支持 ms-playwright 缓存自动探测与系统浏览器回退。`Makefile` 新增 `verify-browser`；`make verify` / `verify-race` / `verify-browser` 三个入口本轮实跑全绿，`TOTAL_FAILS=0`、`MAKE_EXIT=0`。所有套件仍保持创建后自清理与全时间戳随机后缀，`reset_testdata.py` 只删已知测试前缀的行。

## 1g. 完成边界：还需用户提供什么（round 130）

本工作树内**可单方面安全实现与验证的工作已全部收敛**（§1–§1f、§2、§2.1–§2.7）。要真正关闭「完成全部实施任务」，剩余项都需要工作树之外的输入，不能由本会话自行生成或替代：

| # | 需要用户/现场提供 | 对应未闭合项 | 为什么不能在本沙箱完成 |
|---|---|---|---|
| 1 | 两台以上真实主机、可控网络隔离、fencing/witness 设备、真实断电/持久盘、真实 relay 与客户端、真实 Prometheus + `promtool` + Alertmanager | B8.5 A–E、`docs/audit/P0-G1.md` G1–G5 | 用户明确「不得触碰生产」；本环境只有单机与环回网卡，单机写锁/环回 UDP/示例规则文件按 Runbook 明确禁止替代原始证据 |
| 2 | 独立专家评审并**冻结**：生产兼容矩阵、DHCP 租约/发件箱/完整业务审计参考 schema（6 个开放决策项）、旧四/五/六表快照迁移路径、`dnswire` 是否扩展、加固 DDL 与同键 UPSERT 写路径 | `contracts/version-compatibility.md`「尚未完成」、`contracts/enterprise-dhcp-lease.proposal.md`、`docs/audit/ENTERPRISE-REVIEW-DOSSIER.md` §6 | 这些是合同/协议版本决策，需要人工签字；提案明确写「冻结前不实现代码」 |
| 3 | 本机 role/epoch/generation/fencing 的**独立授权证据**，以及可信时间/权威尾来源 | 10 个 `ErrNoAuthority` 生产门禁 | DB 行、config、TLS 身份都不能单独授予写权限；自行提升等于绕过安全模型，且 `prodgates` 回归会拒绝 |
| 4 | 真实遗留生产库升级与历史数据对账、跨主机分区与旧主回归原始证据、ACK 语义迁移评审 | v0.9.0 四条主线、B8.5-D | 需真实遗留数据与现场时间线 |

**本会话未做的、也刻意未做的**：未提升任何 `ErrNoAuthority` 门禁、未把任何行形状/矩阵宣称为已冻结、未把仓内单机证据升级为 B8.5 PASS、未改动 `501` 契约、未触碰生产。评审材料（`docs/audit/ENTERPRISE-REVIEW-DOSSIER.md`）与验收证据模板（`docs/B8.5-外部验收Runbook.md` §10）已备好，可直接在现场开工。

> **状态更新（round 283，用户「详细检查是否有未完成实施任务」复核）**：上表第 2 项中的「DHCP 租约 / 发件箱 / 完整业务审计参考 schema（6 个开放决策项）」已由用户授权自行决策冻结（round 258，提案状态 FROZEN）、`dhcplease` 已实现（§1cb 系列），**不再属于待外部输入项**；第 1/3/4 项与 `501` 契约、B8.5 A–E 物理验收仍开放。round 273–280 已落地统一事实链默认装配与 DNS consumer 迁移（§1ad 系列），round 281–283 完成启动全面检查与缺陷修复（§1ae、§1af）。本表保留原始历史判断，当前状态以 STATUS §1–§3 与本注记为准。

## 1h. 浏览器 harness 依赖可复现（round 131）

`test/browser/` 新增 `package.json`（声明 `playwright ^1.63.0`，与实际测试版本一致）与 `npm test` 脚本；`common.cjs` 在模块加载时 `require.resolve('playwright')`，缺失时打印可执行的安装提示（`cd test/browser && npm install` 或设置 `NODE_PATH`）并 `exit=2`，不再抛出裸 `MODULE_NOT_FOUND`。`test/browser/README.md` 前置条件同步。实测两条路径：无 `NODE_PATH` 时清晰报错 `exit=2`；有 `NODE_PATH` 时自动探测 Chromium 并完整跑通 15 套件 `TOTAL_FAILS=0`、`MAKE_EXIT=0`。

## 1i. 纯净构建可复现（round 132）

此前验证都在已有 `web-admin/dist` / `web/dist` 与临时二进制上进行。本轮删除 `web-admin/dist`、`web/dist`、`dist/` 后从源码执行 `make build`（含 `pnpm install --frozen-lockfile && pnpm build`、复制 `web/dist`、带 ldflags 编译 `./dist/goddi`）→ `BUILD_EXIT=0`，`web/dist` 80 个文件。随后**用这个纯净构建的二进制**（非 `/tmp/goddi-run/goddi`）启动隔离服务：`/health` OK、`/login` 200、`make verify-browser` 15 套件 `TOTAL_FAILS=0`、`MAKE_EXIT=0`。这补齐了「从源码到可服务端到端」的干净构建证据。

## 2. 单机实施任务台账

### 2.1 前端技术栈（用户指定）
- **状态：完成。** Vue 3 + TypeScript + Vite + TDesign Vue Next（Starter 分层）。29 个页面全部纯 TDesign，`src/`、`build/`、`package.json`、`tsconfig.json` 中 `naive-ui` 引用为 0：依赖、`naive-ui/volar` 类型、`NaiveUiResolver()`、`typings/naive-ui.d.ts`、生成的 `N*` 全局声明与 `<N*>` 组件标签均已移除；`App.vue` 简化为 `TConfigProvider` + `<RouterView />`。`@vicons/ionicons5` 仍作为纯图标包被 dashboard 使用，与 Naive UI 无关。
- 证据：`docs/tdesign-migration.md`（批次 2–14）、`web-admin/src/components/RemoteTable.vue`、`web-admin/src/layouts/tdesign-layout/index.vue`、`web-admin/build/plugins/router.ts`。
- 双机预留：`NodeModeBadge.vue`（`single` 固定）、`service/api/goddi/cluster.ts`（纯类型、零请求）、布局中的保留导航注释。

### 2.2 运行期缺陷与功能补齐（审计文档 `docs/audit/RUNTIME-VALIDATION-LEGACY.md`）
| 项 | 状态 | 关键证据 |
|---|---|---|
| DHCP OFFER/ACK 缺 option 54 | 已修 | `internal/dhcp/server/server_identifier_test.go`；实跑 `server-id=198.18.0.1` |
| RENEWING 可认领基础设施地址 | 已修 | `internal/dhcp/server/infra_address_test.go` |
| `ExpireFactsBatch` 单连接自死锁 | 已修 | `internal/dhcp/lease/facts_expiry_deadlock_test.go`；实跑租约到期回收 |
| 第 4 项 AXFR/IXFR ACL 在服务角色失效 | 已修（round 111） | `015_zone_transfer_acl.sql`、`030_...bump_revision.sql`、`zone_transfer_acl_test.go`；真实 `dig AXFR` A/B 日志 |
| 第 6 项 `cache:` 被 DB 默认值遮蔽 | 已修（round 112） | `system.EnsureDefaults(overrides)`、`settings_test.go`；空库 5/5 值等于 config |
| 第 7 项 RFC 2136 UPDATE 返回 NOTIMP | 已修（round 117） | `msgAcceptFunc`、`017_update_policies.sql`、`032_...bump_revision.sql`、`wire_tsig_test.go`；`nsupdate` 签名 UPDATE → NOERROR 且写库可解析 |
| 第 5 项 RENEWING 池外认领 | 已配置化（round 118） | `config.DHCPConfig.RenewOutOfPoolPolicy`、`renew_out_of_pool_test.go`；默认 `allow` 保持历史语义，严格模式为可选 |
| round 119 lint 清理 | 已修 | `theme/shared.ts` 未用导入、`dashboard/index.vue` 未用 `computed`、`zone-detail` 变量前置、尾随空格 |

### 2.3 DNSSEC（`docs/Technitium对标评估与实施方案.md` 批次 F 后端）
- **状态：后端签名链路完成（4 个增量），仍为实验功能。**
- 增量 1 `internal/dns/dnssec/responder.go` + `responder_test.go`：DNSKEY RRset / 正向 RRSIG，5 种算法逐一 `rrsig.Verify` 通过、fail-closed。
- 增量 2 `016_dnssec_keys.sql` + `031_dnssec_keys_bump_revision.sql` + `dnssec_keys_test.go`：密钥复制到数据面存储，脱离控制库签名。
- 增量 3 `internal/dns/server/server.go` / `dnssec_answer_test.go`：按 EDNS DO 位接入 DNSKEY 与正向 RRSIG；真实 `dig +dnssec` 验证。
- 增量 4 `denial.go` / `denial_test.go`：RFC 4034 规范序、NXDOMAIN 覆盖+通配符双 NSEC、NODATA 位图、SOA/NSEC RRSIG；真实 `dig` 得到 2 NSEC + 3 RRSIG。
- 增量 5（round 120）`internal/dns/dnssec/nsec3.go` + `nsec3_test.go`：RFC 5155 哈希名链、最近封闭者三类证明、NODATA 位图、SOA/NSEC3 RRSIG；真实 `dig +dnssec` 得到 NSEC3 记录且 0 条 NSEC。
- **明确边界**：无父区 DS 链式信任、密钥轮换自动化与生产级 KSK 保管未做、NSEC3 opt-out 的未签名委派链裁剪未做；`dns.dnssec.enabled` 默认关闭、`GetDNSSECStatus().signed=false` 保持不变，**不宣称 DNSSEC 生产可用**。

### 2.4 Technitium 对标批次 A–E
- A（Zone 管理）：启用/禁用、queryAccess ACL、per-zone 权限、History、expiryTtl/overwrite、zone-detail 四标签页 —— 已具备。
- B（统计）：范围化 stats、Top 榜、Dashboard —— 已具备。
- C（菜单/安全页）：allowed/blocked 拆分、DNS Client 导入区域 —— 已具备；Apps 管理页属 reserved 501（见 §3）。
- D（加密上游）：`internal/dns/forwarder/upstream_encrypted.go` 含 DoT/DoH/DoQ 实现。
- E（Catalog Zone）：`ZoneTypeCatalog`、成员 PTR、zone-detail 归属编辑 —— 已具备。

### 2.5 双机协议离线增量（round 121）
- `internal/enterprise/pullresponse` 的 `BuildControl` 现支持 4 种控制处置；新增 `SNAPSHOT_REQUIRED` / `DIVERGED` 的**离线编码器**，要求调用方显式提供 `Earliest`（保留窗口最早事件）或 `Conflict`（同位置但摘要不同的事件），并做身份/摘要/epoch/位置校验；`control_snapshot_test.go` 覆盖编码、30 组反例、输入不被修改、无证据时无法构造。
- 这是纯离线序列化，**不授予角色、epoch、fencing 或权威尾证明**，也不改变任何生产门禁；详见 `docs/audit/P0-G1.md` 的 round 121 增量与包内 `README.md`。

### 2.6 双机协议离线增量（round 122）
- `replicaschema.Verify` 现为**版本感知的只读结构校验**：通过 `eventlogguard.InspectTx` 分类并接受 legacy / hardened 两种冻结参考；`eventlogguard.ReferenceDDLs` / `ReferenceMarkerStatements` 作为统一 DDL/marker 真源。原有 PRAGMA / source / 附加库 / TEMP 严格性不变，drift 与篡改仍拒绝。
- 消除了测试期 import cycle 并更新 `snapshotstage` 的过时断言；`eventlogguard/README.md`、`contracts/version-compatibility.md`、`docs/audit/P0-G1.md` 已同步。
- 这是只读识别，**不授予角色、epoch、fencing、版本协商、生产升级或同键 UPSERT 写权限**；所有受保护写入口仍恒 `ErrNoAuthority`。

### 2.7 双机协议离线增量（round 123）
- 新增 `internal/enterprise/compat`：六条独立版本轴的**候选**生产兼容矩阵 + `Negotiate` / `ValidateReadableSchemas` fail-closed 协商原语；7 组测试全绿。`contracts/version-compatibility.md` 已补候选矩阵表。
- **明确标注「尚未冻结」**：需独立评审与对端显式协商；不授予角色/epoch/fencing/写权限，未接入生产 RPC / 快照安装 / 复制读路径。

## 3. 按开发文档**有意保留**的项目（非未完成）

| 项 | 保留依据 |
|---|---|
| `/sso`、`/apps`、`/cluster`、`/dhcp/ha` 固定 501 | `docs/实施计划-v0.9.0-企业级方案复核与生产收敛.md`：「未完成上述证据前，继续保留 `reserved/501`」；`internal/api/openapi_test.go` 的 `TestReservedExtensionsDocumentTheir501Contract` 把 501 契约固定为回归测试 |
| `internal/cluster.ErrNotImplemented` | 双机多节点协调按用户口径「最后实现」 |
| `internal/enterprise/*` 生产写/激活/网络/迁移门恒 `ErrNoAuthority` | 受保护写需独立验证的本机角色/权限、epoch、generation、fencing；DB 行或 config 不能授予权限。`internal/enterprise/prodgates/gates_test.go` 固定 10 个哨兵 |
| `SNAPSHOT_REQUIRED` / `DIVERGED` 离线构造路径 | round 121 已补齐（`control.go` + `control_snapshot_test.go` + `README.md`）：显式 `Earliest`/`Conflict` 证据 + 30 组反例。**生产授权/角色/权威尾证据、快照安装仍关闭**；禁止从数据库错误推断分叉 |
| 父区 DS 链、生产级 KSK 轮换、NSEC3 opt-out 委派裁剪 | 见 §2.3 边界；实验功能默认关闭 |

## 4. 外部阻断（本沙箱无法闭合，非代码缺陷）

依据 `docs/B8.5-外部验收Runbook.md`（状态 `BLOCKED / external-validation-required`）与 `docs/audit/P0-G1.md` 实施进度总表：

- 真实跨主机双节点、物理断电/未知提交、可控网络分区与 fencing/witness、真实 relay 客户端互操作、目标 Linux 平台容量与长稳。
- 真实遗留生产库升级与历史数据对账；生产兼容矩阵与旧四/五/六表快照到八表候选的迁移路径冻结（`contracts/version-compatibility.md`「尚未完成」）。
- Enterprise 版 DHCP 租约/发件箱/完整审计 schema 冻结与独立专家评审、统一事实链默认装配、G2–G5、ACK 语义迁移评审。
- 真实 Prometheus / Alertmanager 告警触发证据。

用户明确要求不得触碰生产；以上均需真实拓扑、设备与原始证据，**不得**用单机 SQLite 写锁、环回 UDP、示例规则文件或代码静态存在替代。

## 5. 未声明的内容

- 未声明「全部实施任务已 100% 完成」：§4 的外部验收与双机 GA 仍未闭合。
- 未声明生产可用、HA GA、DNSSEC 完整、或任何企业版受保护写已启用。
- 未提升任何 `ErrNoAuthority` 门禁；未改动 `501` 契约；未替用户对第 5 项做默认收紧决定。

## 6. 相关文档索引

- `docs/tdesign-migration.md` — 前端迁移全批次与验证
- `docs/audit/RUNTIME-VALIDATION-LEGACY.md` — 运行期缺陷与修复证据
- `docs/audit/P0-G1.md` — Enterprise 候选/原语进度总表
- `docs/Technitium对标评估与实施方案.md` — 对标批次 A–F
- `contracts/version-compatibility.md` — 五个独立版本轴
- `docs/B8.5-外部验收Runbook.md` — 外部验收证据模板

## 1j. 批次 F 前端待办收口：独立 DNSSEC 密钥生命周期页（round 133）

`docs/Technitium对标评估与实施方案.md` 明确把「批次 F 的 UI 密钥生命周期独立页面」列为前端待办；此前只有 `dns/zone-detail/[id].vue` 的 DNSSEC 标签页承载这些操作。本轮补齐，成为第 30 个 TDesign 页面：
- `web-admin/src/views/dns/dnssec/index.vue`：区域选择器（真实 `listDNSZones`）、签名状态与密钥数、启用/禁用 DNSSEC、轮换密钥、生成密钥（KSK/ZSK + 5 种算法）、晋升备用密钥、密钥启停/删除（确认对话框 + RBAC）、密钥表、DS 记录表、NSEC3 参数表单与保存、实验性告警。全部走现有 zone 级 API，**无新增后端端点**，无跨区密钥列表假设。
- 路由/导航/i18n：`build/plugins/router.ts` 的 `ROUTE_META` 新增 `dns_dnssec`（`dns:read`）；布局 NAV + `PATHS`；`zh-cn.ts` / `en-us.ts` 新增 `route.dns_dnssec` 与 `dns.dnssec.*`（28 键）；`src/typings/app.d.ts` 的 `App.I18n.Schema` 同步；elegant-router 生成的 `imports.ts` / `routes.ts` / `transform.ts` 由 `pnpm build` 自动重建。
- 顺带修正两处**过时提示**：`dns.zones.dnssecExperimentalHint` 与 `dnssecNsec3Hint` 仍声称「权威应答尚未携带 RRSIG/DNSKEY」「待真实签名实现后生效」，与 round 113–117 已上线的 NSEC/NSEC3 签名实现矛盾；两种语言现改为「正向应答与否定证明已签名，但无父区 DS 链式信任，请勿视为生产就绪」。
- 验证：`pnpm lint` / `typecheck` / `build` 全绿；新增 `test/browser/batch15.cjs`（TDesign DOM、0 Naive、三个区块、父区 DS 提示、区域下拉由真实 API 填充）PASS；**全量 16 个浏览器套件 `TOTAL_FAILS=0`**；`docs/tdesign-migration.md` 页面数与套件数同步。
- **边界未变**：DNSSEC 仍为实验功能，默认关闭、`GetDNSSECStatus().signed=false` 不变，父区 DS 链、生产 KSK 轮换/保管、NSEC3 opt-out 委派链裁剪仍属外部边界；未授予任何生产权限。

## 1k. 全仓文档 TODO 复核与 DNSSEC 提示一致性（round 134）

按 round 133 的教训，系统扫描 `docs/*.md` / `contracts/*.md` 的「待办 / 仍未 / 尚未 / 未完成」标记并逐项对照当前代码：
- **已确认闭合**（历史计划里的旧陈述，代码早已实现）：IXFR 已有 journal 链与显式 AXFR 回退、zone 导入 `dry_run` 预检、`expiry_ttl` / `overwrite` 记录语义、stats 时间范围与 Top 榜、per-zone 权限与 Zone History、zone-detail 五个标签页、DoT/DoH/DoQ 上游装配、批次 F 独立密钥页。对标方案 §3.2 的「❌/部分」是**评估时基线**，不是当前缺口。
- **发现并修正两处会误导读者的过时文档**：`docs/上线前测试报告-2026-09-08.md` 的 W-6 与 `docs/优化改进建议.md` 的 R1 仍称「权威应答尚未携带 RRSIG/DNSKEY」「不生成任何 RRSIG/DNSKEY」，与已上线的 NSEC/NSEC3 签名矛盾。两处均加「round 134 现状更新」注记，说明签名链路主体已完成、剩余父区 DS 链/生产 KSK 轮换、仍默认关闭且不得生产启用。
- **新增防回归**：`internal/api/handler/dnssec_note_test.go` 钉住 `dnssecExperimentalNote` 必须同时陈述「已签名 / NSEC/NSEC3 / 父区 DS / 实验功能 / 生产就绪」且不得回到旧措辞；`test/browser/i18n.cjs` 静态检查中英文三个提示键不回退到旧措辞并保留父区 DS 边界（不需要浏览器）。两者均做了**负向验证**：注入旧措辞时测试确实失败，恢复后通过。
- 验证：`go test ./internal/api/handler/ -run TestDNSSECExperimentalNote` PASS；`i18n.cjs` PASS / 注入旧措辞 exit=1 / 恢复 PASS；`test/browser/run.sh` 现含 17 个检查（含静态 i18n）。
- **边界未变**：剩余未闭合项仍为 §1g 的外部硬件证据、独立评审冻结与授权输入；未把任何历史计划的「未验证」升级为已验收，未提升任何生产门禁。

## 1l. OpenAPI 契约全量补齐与双向一致性门禁（round 135）

`internal/api/openapi.go` 的注释说明 `routeDocs` 刻意与路由登记放在同一处评审，但此前没有测试强制两者一致。本轮用 `chi.Walk` 枚举真实路由器发现：**207 个 `/api/v1` 操作中 55 个没有文档**（TOTP、groups/users 角色与组关联、TSIG 密钥、blocklist 规则、zone clone/convert、DHCP 写操作、`/system/*` 兼容路径、tasks、login logs、permissions 等；其余 49 个差异只是尾斜杠与参数名不同，归一化后已匹配）。
- `routeDocs` 补齐 **56 条**（207 个操作全覆盖），沿用现有 tag / summary / `Params` 风格；`TestEveryDocumentedPathParameterIsInThePathTemplate` 与 `TestEveryDocumentedOperationHasAUniqueID` 继续 PASS。
- 新增 `internal/api/openapi_coverage_test.go`：`TestEveryRegisteredAPIRouteIsDocumentedInOpenAPI` 用一次性 SQLite 装配真实 `NewRouter`，`chi.Walk` 后做**双向**比对（已登记但未文档化、已文档化但路由已消失都失败）；路径归一化只忽略尾斜杠与 `{param}` 命名，不忽略新路径或新方法。
- **负向验证**：临时删掉一条 `routeDocs` 条目 → 测试确实 `--- FAIL` 并精确报出缺失操作；恢复后 PASS。
- 验证：`go test ./internal/api/` 全量 PASS（含 6 个 OpenAPI 契约测试）、`make verify` / `go build` / `go test ./...` / `go vet` 全绿、浏览器 17 项 `TOTAL_FAILS=0`。**运行期复核**：首次用 round 132 的旧二进制查询 `/api/v1/openapi.json` 仍只有 152 个操作，重新 `go build -o ./dist/goddi` 后为 **207 个操作且 JSON 可解析** —— 这条差异说明增量改动后必须重建二进制再验运行期，已记入教训；随后用新二进制重跑 17 项仍全绿。
- **边界未变**：只补文档与测试，未新增/删除任何路由、未改变任何 handler 行为、未提升任何生产门禁；501 保留端点的文档仍标注其真实 501 响应。

## 1m. 前端↔后端静态契约门禁（round 136）

系统性复核两处此前无强制检查的一致性：
- **i18n 路由键**：36 个 NAV `labelKey`/`ROUTE_META` 键在中英文 `route` 表均有翻译（各 41 键，无缺失、无多余）；35 个 NAV 键中 7 个是分组父节点（`expanded` 中声明，`go()` 只对叶子取 `PATHS`，父节点仅展开不跳转），其余 28 个叶子均有 `PATHS` 条目；32 个 `ROUTE_META` 键全部匹配 elegant-router 生成的路由名。
- **前端 API 字面量**：`web-admin/src/service/api/goddi/*.ts` 的 125 个路径字面量（归一化 `${id}` / `{id}` / 尾斜杠后）全部命中 `internal/api/openapi.go` 的 151 个文档路径；仅 `/login`、`/v1`、`/proxy-default/api/v1` 等非 API 字面量在忽略列表。**两项复核当前均无缺口**。
- 新增 `test/browser/contracts.cjs`（无需浏览器）把上述四类不变量 + **RBAC 权限有效性**固化为门禁，纳入 `make verify-browser`；`i18n.cjs` 继续钉住 DNSSEC 提示措辞。**负向验证**：同时注入「删掉一条路由翻译」+「新增一个未文档化的前端 API 调用」，守卫精确报出两处并 `exit=1`，恢复后 PASS。
- 验证：`make verify`、`go build` / `go test ./...` / `go vet` 全绿；`prodgates` 仍 PASS；浏览器 18 项 `TOTAL_FAILS=0`；`git diff --check` 干净。
- **边界未变**：仅新增静态检查，未改任何路由、handler、页面行为或生产门禁；未把任何仓内证据升级为外部验收。

## 1n. 证据台账 §1 更新与 RBAC 权限门禁（round 137）

- **修正过时主表**：§1 仍标注「round 119 全量复跑」、浏览器回归写「`/tmp/pwsmoke/` 15 个套件」、套件清单只列到 `batch14`。现重写为**当前状态**：13 项检查（含纯净构建、生产门禁、运行期 OpenAPI 207 操作）、4 个可复现 `make` 入口、18 项浏览器/静态检查清单（含 `contracts`/`i18n`/`batch15`），并去掉与文件头重复的免责声明。历史增量仍保留在 §1b–§1m，不改写历史。
- **补齐第 5 类契约不变量**：复核 `build/plugins/router.ts` 的 25 处 `permission` 声明全部命中 `internal/rbac.PredefinedPermissions`（26 项），无无效权限；把该校验加入 `test/browser/contracts.cjs`。**负向验证**：注入 `nonexistent:read` → 守卫精确报出并 `exit=1`，恢复后 PASS。
- 验证：`make verify`、`go build` / `go test ./...` / `go vet` 全绿；`prodgates` 仍 PASS；浏览器 18 项 `TOTAL_FAILS=0`；`git diff --check` 干净。
- **边界未变**：仅文档更新与静态检查，未改任何权限、路由、页面行为或生产门禁。

## 1o. 未测包清理与 SSRF 双层守卫对齐（round 138）

扫描 `internal/` 发现 7 个无测试的生产包：`api/response`(277 行)、`dns/client`(211 行) 有实质逻辑；`dns/authoritative`、`dns/resolver`（各 3 行占位）、`apps/runtime`、`apps/sdk`（501 哨兵）、`log`（日志装配）无值得单测的行为。
- **`internal/api/response/response_test.go`（7 组）**：成功/错误 helper 的状态码、`code`/`message`、安全头（JSON + `nosniff`）、错误响应不带 `data`、携带 payload 的 400/409 保留 `data`、`OKPaginated` 的页数向上取整 / `pageSize<1` 钳位 / nil 与 typed-nil 切片渲染为 `[]`、`ParsePagination` 默认值与 100 上限、`InternalErrorWithLog` / `SanitizeError` 不泄漏底层错误文本。
- **`internal/dns/client/client_test.go`（3 组）**：`validateUpstreamAddress` 的 loopback / link-local / 私有 / ULA 各种拼写（含端口、尾点、IPv6 括号）、`NewDNSClient` 默认超时、`rrToEntry` 对 A/AAAA/CNAME/NS/PTR/MX/TXT/SRV/CAA/SOA 提取 RDATA 与未知类型回退。
- **发现并修复一个真实 SSRF 缺口**：`internal/dns/client.validateUpstreamAddress` 是 `dns.allow_private_upstream=true`（默认）下唯一仍生效的字面量守卫，但它只检查 `IsLoopback/IsLinkLocalUnicast/IsPrivate`，**漏掉 `0.0.0.0`/`::`（unspecified）与 link-local multicast（224.0.0.0/24、ff02::/16）**，而更严格的 `handler.isPrivateIP` 明确拒绝这两类。现补齐 `IsUnspecified()` 与 `IsLinkLocalMulticast()`，并把守卫意图写进注释。**只收紧、未放宽任何地址**。
- **`internal/api/handler/dns_client_test.go`（4 组）**：`isPrivateIP` 全类别表、`validateDNSName` 沿用既有测试不重复、`mustParseCIDR` 非法输入 panic、以及**跨层一致性**测试 `TestClientLiteralGuardIsNotWeakerThanHandlerGuard` —— 对 8 个被拒地址同时断言 handler 拒绝且 `client.Query` 在任意网络 I/O 前以 `blocked address range` 返回；另有 guard 开启时拒绝私有字面量、放行公网的测试（保存/恢复 `DNSServices` 全局）。
- **运行期复核**：重建二进制后 `POST /api/v1/dns/client` 用 `0.0.0.0:53` 被拒、`8.8.8.8:53` 仍正常返回 NOERROR。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；三个改动包 `go test -race` 全绿；`make verify` 全绿；浏览器 18 项 `TOTAL_FAILS=0`；`prodgates` 仍 PASS。
- **边界未变**：仅新增测试与收紧 SSRF 字面量守卫，未改任何路由、写路径或生产门禁；`dns.allow_private_upstream` 的配置语义与默认未改。

## 1p. 全量 API 认证覆盖门禁（round 139）

按 round 135/138 的思路继续做系统性安全复核：用真实 `NewRouter` 对 `routeDocs` 中 **204 个 `Secure: true` 操作**逐一发起无凭据请求。
- 结果：204 个全部返回 **401**，0 个泄漏；3 个 `Secure: false`（`/auth/init` 400、`/auth/login` 400、`/auth/refresh` 401 属无 refresh token 的正常表现）均已注册。
- 新增 `internal/api/auth_coverage_test.go`：`TestEverySecureOperationRefusesUnauthenticatedRequests`（只允许 401/403）、`TestPublicOperationsAreRegistered`（不允许 404/403）。`{param}` 用合法哑值替换，确保请求抵达路由而认证先行。
- **负向验证**：把 `/auth/login` 临时标成 `Secure: true` → 测试 FAIL/`exit=1`，恢复后 PASS。另做了一次更有意思的实验：**临时删掉认证组的 `r.Use(middleware.Authentication(...))` 后，所有安全路由仍返回 401** —— 因为 per-route `rbac.RequirePermission` 与读取上下文身份的 handler 构成第二、三层防线，移除单层不构成绕过。该结论已写入测试文件注释，测试钉的是**可观察的认证边界**而非某个特定中间件。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`prodgates` 仍 PASS；浏览器 18 项 `TOTAL_FAILS=0`；`git diff --check` 干净。
- **边界未变**：仅新增测试与注释，未改任何路由、中间件或生产门禁。

## 1q. 迁移编号连续性门禁（round 140）

继续系统性复核两类尚未有强制检查的不变量：
- **SQL 构造审计**：扫描全部 `Exec/Query/QueryRow/Prepare` 与动态 `ORDER BY`/表名拼接。所有动态片段均来自内部常量（硬编码表名、固定列清单、由固定条件 + `?` 占位符组成的 `whereClause`、goose 备份表清单、dataplane 列定义），**未发现用户值直接插入 SQL**；`ListScopes` 等筛选走参数化。无注入缺口。
- **迁移编号**：四组迁移目录中 `internal/database/migrations` 只是 sqlc 占位文件；其余三组（根 `migrations/` 001–032、dataplane 001–017、ipamstore 001–002）此前无编号一致性测试，而 goose 对**重复编号会静默替换**、对编号跳空会让旧库永远到不了已知版本。新增 `internal/database/migration_numbering_test.go` 钉住：文件名 `NNN_name.sql`、编号从 001 起严格连续且无重复、文件非空、goose 目录每个文件同时含 `-- +goose Up` 与 `-- +goose Down`（ipamstore 为纯 SQL 不适用）。实测 32/17/2 个文件全部通过。
- **负向验证**：临时加入 `032_probe_duplicate.sql` → 测试精确报出「duplicate migration number 032」并 FAIL，删除后 PASS。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`prodgates` 仍 PASS；浏览器 18 项 `TOTAL_FAILS=0`；`git diff --check` 干净。
- **边界未变**：仅新增测试，未改任何迁移内容、SQL 或生产门禁。

## 1r. 全量 CSRF 覆盖门禁（round 141）

继 §1p 的认证覆盖后，补齐另一半：**有效会话令牌本身不足以改写状态**。
- 用真实 `NewRouter` 通过 `/auth/init` + `/auth/login` 建立浏览器式会话（JWT + 会话绑定的 HMAC `csrf_token`），对 `routeDocs` 中 **122 个变更型 `Secure` 操作**（POST/PUT/DELETE/PATCH）逐一发起「有 Bearer、无 `X-CSRF-Token`」请求 → 全部被 `CSRFProtection` 以 401 拒绝，0 个泄漏。
- 新增 `internal/api/csrf_coverage_test.go`：`TestEveryMutatingSecureOperationRequiresCSRF` 先断言「已认证的 `GET /users` 返回 200」证明会话有效，再遍历全部变更操作；**双向**补一句「带合法 CSRF 的 `POST /users {}` 不再被 401/403 拒绝」，避免把「会话根本无效」误判成 CSRF 生效。
- 两个**测试自身陷阱**已修正并写入注释：`/auth/logout` 成功会注销会话，若排在遍历前面会让后续请求全部因会话失效返回 401 从而**掩盖真实泄漏** —— 现把会注销会话的两个端点放到最后、各用**全新会话**单独断言；`auth/init` 重复调用返回 409 属正常，允许 200/409。另一陷阱是 router-only fixture 不初始化 `DNSServices`，遍历所有 handler 会触发与 CSRF 无关的 500/panic 噪音，因此补集只用一个无需容器的端点。
- **负向验证（修复后）**：临时删掉 `r.Use(middleware.CSRFProtection(cfg))` → 测试精确报出 **122 个**操作全部绕过（400/404/500/501/503/200），FAIL；恢复后 PASS。修复前该负向实验只报出 1 个泄漏，正是上述会话注销掩盖问题，本轮已消除。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`prodgates` 仍 PASS；浏览器 18 项 `TOTAL_FAILS=0`；`git diff --check` 干净。
- **边界未变**：仅新增测试，未改任何中间件、路由或生产门禁。

## 1s. 授权深度门禁：零权限用户不可读（round 142）

继 §1p（认证）、§1r（CSRF）后补齐第三层：**登录 ≠ 有权读配置**。
- 用真实 `NewRouter` 建管理员 → 创建一个**无角色、无权限**的用户（`must_change_password=false`）→ 以该用户登录，对 `routeDocs` 的 82 个 `Secure` GET 操作逐一请求。结果：**80 个返回 403**，仅 `/auth/me`、`/auth/sessions` 返回 200 —— 两者是账户自助端点，且 `ListSessions` 用 `ListUserSessions(userID)` 限定为调用者本人（已核实源码）。**无越权缺口**。
- 新增 `internal/api/rbac_coverage_test.go`：
  - `TestZeroPermissionUserIsRefusedBySecureReadEndpoints`：除自助白名单外，任何 `<400` 的响应都判为越权泄漏；并带**正向对照**「管理员 `GET /users` 必须 200」，确保 403 是授权决策而非路由损坏。
  - `TestSelfServiceReadsAreScopedToTheCaller`：`/auth/me` 返回的 `id` 必须是调用者本人；`/auth/sessions` 非空且每条 `user_id` 都等于调用者，防止会话列表跨用户泄漏。
- **负向验证**：临时移除 `/users` 的 `r.With(rbac.RequirePermission(...))` → 测试精确报出「1 secure reads ... `GET /api/v1/users -> 200`」并 FAIL；恢复后 PASS。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`prodgates` 仍 PASS；浏览器 18 项 `TOTAL_FAILS=0`；`git diff --check` 干净。
- **边界未变**：仅新增测试，未改任何 RBAC、路由、handler 或生产门禁；白名单是既有产品行为，不是新放宽。

## 1t. 浏览器页面覆盖门禁（round 143）

继续系统复核，这次查**页面覆盖**：`web-admin/src/views` 下 29 个管理视图（+ `/login` = 30 个 TDesign 页面）逐一比对 `test/browser/*.cjs` 的路径引用 —— **全部至少被 1 个套件访问，无盲区**（`dns/zone-detail/[id].vue` 按目录前缀 `/dns/zone-detail` 匹配）。
- 给 `test/browser/contracts.cjs` 加第 6 类不变量：扫描 `views/` 下所有 `.vue`，`index.vue` 归一化为目录路由、`[id].vue` 归一化为父目录前缀、`_builtin/*` 排除但单独断言 `/login`；只扫描 `smoke.cjs` / `batch*.cjs` / `legacy.cjs`，避免把守卫自身源码算进覆盖（自证循环）。新增页面若没有浏览器套件即 FAIL。
- **负向验证**：临时加入 `views/dns/uncovered-probe/index.vue` → 精确报出「view ... has no browser suite」并 `exit=1`；删除后 PASS。
- 至此 `contracts.cjs` 固化 6 类前后端契约：双语 i18n 键、NAV→PATHS、ROUTE_META→生成路由、ROUTE_META 权限→RBAC、前端 API 字面量→OpenAPI、视图→浏览器套件。
- 验证：`make verify`、`go build` / `go test ./...` / `go vet` 全绿；`prodgates` 仍 PASS；浏览器 18 项 `TOTAL_FAILS=0`；`git diff --check` 干净。
- **边界未变**：仅新增静态检查与文档，未改任何页面、路由或生产门禁。

## 1u. 单机优先 / 双机预留门禁（round 144）

直接核对用户口径「**先单机实现所有功能，双机部分从技术架构角度预留，最后实现**」的预留点是否仍然完整：
- `NodeModeBadge.vue` 默认 `mode: 'single'`、类型预定义 4 种 HA 模式但无 HA 状态；`cluster.ts` 是纯类型预留（无 `get/post/put/del/fetch` 调用、保留 `CLUSTER_API_RESERVED`）；布局中 `<NodeModeBadge mode="single" />` 无动态绑定；NAV 只有 STAGE 3 注释、无 `cluster` 组；后端 `/cluster`/`/sso`/`/apps`/`/dhcp/ha` 仍为 501，由 `TestReservedExtensionsDocumentTheir501Contract` 钉住。**全部符合，无越界**。
- 给 `test/browser/contracts.cjs` 加**第 7 类不变量**固化该边界：badge 默认单节点且仍渲染 `Single-Node` 标签、`cluster.ts` 不得发出任何请求、不得注册 `cluster` 导航组、badge 不得绑定 `:mode` 动态 HA 状态。正则已放宽到允许格式化重排，避免重排造成误报。
- **负向验证**：把布局临时改成 `<NodeModeBadge :mode="clusterMode" />` → 精确报「bound to dynamic HA state」并 `exit=1`；恢复后 PASS。
- 至此 `contracts.cjs` 固化 7 类不变量，覆盖前后端契约与单机/双机阶段边界。
- 验证：`make verify`、`go build` / `go test ./...` / `go vet` 全绿；`prodgates` 仍 PASS；浏览器 18 项 `TOTAL_FAILS=0`；`git diff --check` 干净。
- **边界未变**：仅新增静态检查与文档，未引入任何 HA 状态、请求、导航或后端能力；未触碰生产。

## 1v. 旧快照迁移路径：从「尚未定义」到「候选定义待冻结」（round 145）

`contracts/version-compatibility.md` 的「旧四/五/六表快照到八表候选的迁移路径尚未定义」是本轮推进的明确缺口。
- **先核实当前强制形状**：`snapshotipam` 冻结 8 表有序集合；`snapshotpipeline.New` 要求 `len(schema)==len(m.Tables)` 且逐项同名同序，白名单只从本地 schema 派生；`TestNewRejectsManifestTableSetAndOrder` 已钉住 missing/duplicate/unknown/reordered。**结论：不完整表集在管道入口即被 `ErrInvalid` 拒绝，不会被当作空表接受。**
- **新增 `docs/audit/SNAPSHOT-MIGRATION-PATH.md`（候选定义，未冻结）**：说明为何不能就地转换（旧格式彼此隔离、都标 `schema=1`、缺表不是零行、行形状已不在代码中）；给出 6 步 fail-closed 路径（检测而非猜测 → 独立格式版本协商 → 从可信源重新抽取全新八表快照 → 独立只读校验 → 不安装 → 迁移事件可审计）；列出冻结前独立评审必答的 5 个问题；明确**不实现任何转换代码、不发明旧格式、不提升门禁、不宣称向后兼容或生产可用**。
- **新增 `internal/enterprise/snapshotpipeline/old_table_set_test.go`**：`TestOldFourFiveSixTableSetsAreRefused`（4/5/6 表 manifest 对 8 表 schema 全部被拒）+ `TestCompleteEightTableSetIsAcceptedByNew`（完整 8 表含零行表被接受，作为正向对照，证明拒绝来自表集而非坏夹具）。
- 更新 `contracts/version-compatibility.md` 的缺口条目与 `docs/audit/ENTERPRISE-REVIEW-DOSSIER.md` §6 的评审问题，标注「候选定义，待冻结」。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`prodgates` 仍 PASS（10 个哨兵未动）；浏览器 18 项 `TOTAL_FAILS=0`；`git diff --check` 干净。
- **边界未变**：只推进文档定义与只读回归测试；`snapshotstage.Import/Install`、`generationpointer.*`、`eventlogguard.Migrate` 仍恒 `ErrNoAuthority`；未生成任何「旧格式参考 schema」代码。

## 1w. 同键 UPSERT 语义变更：实测钉住 + 候选写路径定义（round 146）

推进 `contracts/version-compatibility.md` 最后一条功能性缺口「加固参考结构的同键 UPSERT 语义变更尚未集成」。先**实测**再定义，不猜语义：
- 在 `ipamstore.Migrate` 的加固（v2）库上逐项实测：同键 `INSERT OR REPLACE` 与 `ON CONFLICT DO UPDATE` 均被 `IPAM_ID_ALREADY_USED` 拒绝；不推进 revision 的 `UPDATE` 被 `IPAM_REVISION_INVALID` 拒绝；`UPDATE ... revision=revision+1` 与新键 `INSERT OR REPLACE` 成功；被拒 UPSERT 后原行不变。既有测试另钉住 `IPAM_ROWID_CONFLICT` / `IPAM_ROWID_IMMUTABLE` / `IPAM_ADDRESS_OCCUPIED`。
- 新增 `internal/enterprise/ipamstore/upsert_semantics_test.go`（3 组，含正向对照）把该语义固化为回归，防止未来集成悄悄改回 `INSERT OR REPLACE`。
- 新增 `docs/audit/UPSERT-WRITE-PATH-COMPAT.md`（**候选定义，未冻结**）：解释「替换一行 = 替换历史」的设计意图、v1/v2 差异必须经 `InspectTx` 分类 + 显式版本协商（不得因同为 1 而推断）、6 条 fail-closed 写路径要求（禁止 UPSERT 表达更新、显式插入/更新二分且同事务、同事务事实与审计、失败即丢弃、版本感知装配、不安装不授权）、冻结前 5 个评审问题；明确**未**接入任何生产写路径、**未**改守卫定义、**未**扫描现存生产 UPSERT 调用点、**未**提升门禁。
- `contracts/version-compatibility.md` 与 `docs/audit/ENTERPRISE-REVIEW-DOSSIER.md` §6 同步为「候选定义，待冻结」。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`prodgates` 仍 PASS（10 个哨兵未动）；浏览器 18 项 `TOTAL_FAILS=0`；`git diff --check` 干净。
- **边界未变**：`eventlogguard.Migrate`、快照安装、复制与受保护写仍恒 `ErrNoAuthority`；默认进程仍用现有 SQLite 热路径，v0.9.0 四条主线未动。

## 1x. 生产门禁清单复核与「DB 行不授予权威」回归（round 147）

复核 `prodgates` 的门禁覆盖是否完整：全部 10 个 `ErrNoAuthority` 哨兵变量、21 个公开写/激活/网络/迁移入口已被 `TestProductionWriteGatesRemainClosed` 与 `TestAuthoritySentinelsAreDistinct` 钉住；额外出现的 `ErrNoAuthority` 返回点经核实是 `dnstransport.newFixture` 的输入校验（实验室夹具）与 `eventlog.CurrentAuthority` 缺行时的 fail-closed 读守卫，**无遗漏的生产写门禁**。
- 新增 `TestADatabaseAuthorityRowAloneDoesNotGrantWriteAuthority`（`internal/enterprise/prodgates/gates_test.go`）：在真实 SQLite 上 `eventlog.Init` 建表、插入一条**完整合法**的 `writer_authority` 行（`cluster_id`/`holder_id`/`epoch`/`db_generation`/`grant_digest`/`granted_at_unix_ms` 全满足 CHECK），用 `SELECT count(*)=1` 作正向对照证明夹具确有该行，然后断言 `Log.Begin` 与 `Log.RecordApplied` **仍恒 `ErrNoAuthority`**。这把文档反复强调的「DB 行 / config / TLS 身份都不能单独授予写权限，另一半是独立本机 role log / epoch / generation / fencing」从注释变成可执行回归。
- 同一测试还钉住读侧：`CurrentAuthority` 在空表上必须返回 `ErrNoAuthority`（而非泛化的 no-rows 错误被误当作良性空结果）。
- **负向验证**：临时把 `Log.Begin` 改成 `return nil, nil`（模拟门禁被打开）→ 既有 `TestProductionWriteGatesRemainClosed` 与新增测试**同时**报「gate is open」并 FAIL；恢复后 PASS。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`prodgates` 3 组全 PASS；浏览器 18 项 `TOTAL_FAILS=0`；`git diff --check` 干净。
- **边界未变**：仅新增测试，未改任何生产逻辑、未提升任何门禁；`logstore.go` 负向改动已按字节恢复。

## 1y. 一页交接总览（round 148）

证据台账已累积到 §1x（23 个附录），加上评审 dossier、B8.5 Runbook、两份候选定义、对标方案与 TDesign 迁移记录，读者需跨 8 份文档才能拼出全貌。本轮新增 `docs/STATUS.md` 作为**单页交接索引**：一句话结论、6 类已完成事项（含当前量化：30 个 TDesign 页面、207 个 API 操作、52 个 Enterprise 包 / 575 个测试入口、10 个门禁哨兵、18 项验证检查）、5 项「候选/已定义但待冻结」、5 类外部阻断、4 个一键复现命令、5 条刻意未做的事、10 份文档索引。台账头部与评审 dossier 头部加入互链。
- **边界未变**：纯文档索引，未新增任何能力、未提升门禁、未把任何证据升级为已验收。
- 验证：`make verify`、`go build` / `go test ./...` / `go vet` 全绿；`prodgates` 仍 PASS；浏览器 18 项 `TOTAL_FAILS=0`；`git diff --check` 干净。

## 1z. 交接文档防漂移门禁（round 149）

round 148 的 `docs/STATUS.md` 是评审/现场人员的唯一入口页，若其中引用的路径被移动、或引用的量化数字随仓库演进而过时，交接材料会**静默失真** —— 比没有总览更危险。本轮新增 `test/browser/docs.cjs`（`make verify-browser` 现 **19 项检查**）固化三类不变量：
- **链接完整性**：`docs/STATUS.md` 中所有仓库路径引用（13 个 `.md/.go/.ts/...`）必须存在；
- **量化一致性**：从仓库实时重算并比对 30 个 TDesign 页面、207 个 `/api/v1` 操作、52 个 `internal/enterprise` 包、575 个测试/Fuzz 入口、10 个 `ErrNoAuthority` 哨兵、19 项验证检查；
- **边界语句**：必须保留「刻意未做的事」「外部阻断」「待独立评审冻结」「不构成」等免责声明，防止读者把候选当验收。
- **负向验证**：同时注入「19 项检查 → 18 项检查」与一个不存在的文档链接 → 守卫精确报出两处并 `exit=1`；恢复后 PASS。
- 同步更新 `docs/STATUS.md`、`test/browser/README.md` 与台账 §1 当前矩阵为 19 项；**历史附录中当时的 18 项记录保持原样，不改写历史**。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`prodgates` 仍 PASS；浏览器 19 项 `TOTAL_FAILS=0`；`git diff --check` 干净。
- **边界未变**：仅新增静态检查与文档更新，未改任何能力、路由或生产门禁。

## 1aa. 全仓竞态复跑与工作区卫生复核（round 150）

上次全仓 `go test -race ./...` 在 round 126，此后累积了 24 轮的测试与门禁（auth/CSRF/RBAC 覆盖、OpenAPI 契约、迁移编号、SSRF、UPSERT 语义、门禁清单、静态契约守卫等）。本轮对**当前全量**重跑竞态检测并复核工作区：
- `go test -mod=readonly -race ./...` → **RACE_EXIT=0**，109 个包/行通过，0 `DATA RACE`、0 FAIL。
- 复核并确认 4 个 `*probe*` 文件是合法内容而非遗留垃圾：`internal/dataplane/probe.go` / `probe_test.go` / `probe_log_test.go` 本就随基线提交，`internal/enterprise/snapshotstage/rowid_probe_test.go` 是早期隔离夹具测试（记录 eventlog DDL 的 rowid 替换限制），均编译通过、无需删除；用户要求保留的未提交文件保持原样。
- 确认历次负向实验的临时改动均已按字节恢复：`CSRFProtection` 中间件在 `router.go:124`、`Log.Begin` 返回 `ErrNoAuthority`、布局 `<NodeModeBadge mode="single" />`；`gofmt -l cmd internal pkg` 为空、`git diff --check` 干净。
- `make verify` → `VERIFY_EXIT=0`；重建二进制后浏览器 **19 项 `TOTAL_FAILS=0`**；`prodgates` 仍 PASS；无残留服务进程。
- **边界未变**：纯验证轮，未新增任何能力、未改任何生产逻辑、未提升门禁。

## 1ab. i18n 字面量键全量校验与 10 处缺失修复（round 151）

静态复核发现一个**真实 UI 缺陷**：`vue-tsc` 不检查 `t('...')` 字符串字面量，前端源码共 548 个字面量键中有 **10 个在两种语言里都缺失**（20 处缺失计数），运行时会直接把 `dashboard.day`、`dns.dnssec.digestType` 等原始键渲染给用户：
- `dashboard`：`hour` / `day` / `week` / `month` / `year`（范围选择器标签）、`dnsQueryTrend` / `dnsStats` / `rcodeDistribution`（卡片标题）；
- `dns.dnssec`：`digest` / `digestType`（round 133 新增页面的 DS 表格列头 —— 本会话自己引入的遗漏）。
- 已在 `zh-cn.ts` / `en-us.ts` / `src/typings/app.d.ts` 的 `App.I18n.Schema` 三处补齐全部 10 个键，`pnpm typecheck` 通过；重扫后 **548 个字面量键在中英文均为 0 缺失**。
- 扩展 `test/browser/i18n.cjs`：除原有 DNSSEC 提示措辞检查外，新增「扫描 `web-admin/src/**/*.{vue,ts}` 中所有字面量 `t('...')` 键（排除动态键）→ 逐个在中英文 locale 中解析 leaf 定义」，任一缺失即 FAIL。保留原有负向验证能力并新增：临时删掉 `dashboard.dnsQueryTrend` → 精确报出并 `exit=1`，恢复后 PASS。
- `batch14.cjs`（dashboard）与 `batch15.cjs`（DNSSEC 页）新增运行时断言「body 中不得出现 `dashboard.*` / `dns.dnssec.*` 原始键」，作为静态门禁之外的真实渲染证据。
- 验证：`pnpm lint` / `typecheck` / `build` 全绿；`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`prodgates` 仍 PASS；重建二进制并重新部署后浏览器 **19 项 `TOTAL_FAILS=0`**（含 i18n 静态门禁与两处运行时断言）；`git diff --check` 干净。
- **运行期插曲（环境，非代码）**：本轮首次 `make verify` / `prodgates` 曾报大量「package crypto/internal/... is not in std」「go-build cache 文件不存在」—— 是 Go 构建缓存条目损坏（前几轮 `-race ./...`、`go build`、`make verify` 并行导致缓存竞争/清理），**不是代码回归**。`go clean -cache` 后 `go build` / `prodgates` / `make verify` / `go test ./...` / `go vet` 全部恢复全绿，并已重建 `./dist/goddi` 后重跑浏览器 19 项仍 `TOTAL_FAILS=0`。教训已记入：并行 Go 构建后若出现标准库缓存缺失，先 `go clean -cache` 再复验，不要误判为代码缺陷。
- **边界未变**：仅修复 UI 文案与新增静态/运行时检查，未改任何后端逻辑、路由或生产门禁。

## 1ac. 前端 API 调用「路径 + 方法」双向契约（round 152）

`contracts.cjs` 的第 4 类不变量此前只校验**路径**字面量存在，不校验调用所用的 HTTP 方法 —— `put('/users')` 这类方法错配仍会通过。本轮补齐：
- 从 `web-admin/src/service/api/goddi/*.ts` 提取 `get/getList/getRaw/post/postRaw/put/del/patch` 各 wrapper 的字面量路径，按 wrapper 映射到 HTTP 方法，与 `internal/api/openapi.go` 的 `(method, path)` 集合逐个比对；`${id}` / `{id}` / 尾斜杠沿用既有归一化。含查询串表达式的模板字面量（1 处 `.../import${overwrite ? ...}`）跳过方法比对但仍由路径覆盖，避免误报。
- 实测 **168 个 wrapper 调用中 167 个通过方法+路径双向校验，0 个错配**；修正了初版 `prefix()` 误给裸路径加 `/api/v1` 前缀导致的全量误报（已恢复为原有归一化语义）。
- **负向验证**：把 `listUsers` 的 `getList` 临时改成 `put` → 精确报「uses PUT on /users, but no such documented operation」并 `exit=1`；恢复后 PASS。
- `test/browser/README.md` 同步。验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`prodgates` 仍 PASS；浏览器 19 项 `TOTAL_FAILS=0`；`git diff --check` 干净。
- **运行期插曲（环境，非代码）**：首次 `make verify` 报 corepack 下载 `pnpm@10.15.0` 时 `ECONNRESET`（registry.npmjs.org TLS 建连被重置）—— 是本机 corepack 缓存未预热 + 瞬时网络失败，不是代码缺陷。手动 `cd web-admin && pnpm --version` 完成缓存预热后重跑 `make verify` → `VERIFY_EXIT=0`。连同 round 151 的 Go 构建缓存损坏，两条教训一并记入：**并行构建/离线环境下先确认工具链缓存可用，出现标准库或 corepack 下载类报错先复验工具链，不误判为代码回归**。
- **边界未变**：仅增强静态契约检查，未改任何前端调用、后端路由或生产门禁。

## 1ad. 配置键文档补齐与双向防漂移（round 153）

复核配置可发现性：`config.yaml` 是最小可运行样例，仅列 79 个键；`internal/config` 共有 115 个 `yaml` 标签，**36 个未列入样例**，其中 **13 个在全仓文档中完全未出现**（`client_qps`/`client_burst`/`rrl_threshold`/`tsig_keys` 等生效项，以及 `sso`/`cluster`/`plugin` 保留段字段）。
- **反向核查无缺陷**：`config.yaml` 的 79 个键**全部**有对应 `yaml` 标签（0 个未知键），严格解析 `yaml.UnmarshalStrict` 不会因样例配置报错。
- 新增 `docs/CONFIG-REFERENCE.md`：按「生效可选项 / `dhcp_ha`（隔离验证过但真实 HA 仍属 B8.5）/ 保留占位（SSO、cluster、plugin，配置可解析但功能未实现、API 仍 501）」三段列全 36 个键，含类型、默认值、环境变量与 `production` 收紧规则；明确「任何配置项都不授予角色/epoch/fencing/写权限」。`docs/STATUS.md` 文档索引加入互链。
- 扩展 `test/browser/docs.cjs` 第 4 类不变量：收集全部 `yaml` 标签 → 断言 `config.yaml` 每个键都有标签（防止样例配置漂移导致启动失败）→ 断言每个**未列入样例**的标签都在 `docs/CONFIG-REFERENCE.md` 中被提及（防止新增可选键无人文档化）。
- **负向验证**：同时注入「`config.yaml` 加一个无标签的 `bogus_stale_key`」+「从参考文档删掉 `client_ca_file`」→ 守卫精确报出两处并 `exit=1`；恢复后 PASS。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`prodgates` 仍 PASS；浏览器 19 项 `TOTAL_FAILS=0`；`git diff --check` 干净。
- **边界未变**：纯文档与静态检查，未改任何配置解析逻辑、默认值、环境变量或生产门禁；`config.yaml` 按字节恢复。

## 1ae. 环境变量覆盖全表与防漂移（round 154）

继 §1ad 补齐 yaml 键后，继续复核**环境变量可发现性**：`internal/config/config.go` 的 `applyEnvOverrides` 共注册 **49 个 `GODDI_*` 覆盖**，其中 **31 个在样例配置与全仓文档中完全未出现**（`GODDI_DATABASE_DSN`、`GODDI_SERVER_TLS_CERT_FILE`、`GODDI_LOG_RETENTION_DAYS`、`GODDI_PROXY_TYPE` 等）。环境变量优先于 YAML，运维无法发现的变量等同不可用，且布尔/整数解析失败会直接启动报错。
- 在 `docs/CONFIG-REFERENCE.md` 新增 §6「环境变量覆盖全表（49 项）」：按环境变量 / 类型 / 对应配置路径逐项列出（路径由 Go 目标字段自动转换为 snake_case 并与 yaml 标签对齐，如 `cfg.DNS.Listeners.UDP.Address` → `dns.listeners.udp.address`）；另附密钥类（`JWT_SECRET` / `ENCRYPTION_KEY` / `DHCP_HA_PEER_TOKEN` 建议只从环境注入）与保留占位（`DNS_DNSSEC_ENABLED` 实验性、`DHCP_HA_*` 隔离验证）说明；明确环境变量不授予角色/epoch/fencing/写权限。
- 扩展 `test/browser/docs.cjs` 第 5 类不变量：收集 `setEnvString/setEnvBool/setEnvInt` 的全部 `GODDI_*` 名称并断言每个都在参考文档中出现；数量为 0 时同样 FAIL（防止正则失效造成假绿）。
- **负向验证**：临时删掉 `GODDI_LOG_RETENTION_DAYS` 一行 → 精确报「env override ... is not documented」并 `exit=1`；恢复后 PASS。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`prodgates` 仍 PASS；浏览器 19 项 `TOTAL_FAILS=0`；`git diff --check` 干净。
- **边界未变**：纯文档与静态检查，未改任何配置解析逻辑、默认值、环境变量或生产门禁；`docs/CONFIG-REFERENCE.md` 负向改动已按字节恢复。

## 1af. 环境变量表路径准确性回归（round 155）

复核 round 154 自动生成的 49 条配置路径，发现 **1 处真实错误**：`GODDI_SERVER_EXPOSE_OPENAPI` 由 Go 字段 `ExposeOpenAPI` 转成 `server.expose_open_api`，而实际 yaml 标签是 `expose_openapi` —— 运维若照抄会设错键。已修正为 `server.expose_openapi`。
- 给 `test/browser/docs.cjs` 加**第 6 类不变量**：解析 §6 环境变量表的每一行路径，断言其**每个点分段**都是 `internal/config` 中真实存在的 yaml 标签；表为空或格式变化也 FAIL（防正则失效假绿）。修正后 49 条路径全部通过。
- **负向验证**：把该行改回 `server.expose_open_api` → 精确报「maps ... but "expose_open_api" is not a yaml tag」并 `exit=1`；恢复后 PASS。`docs.cjs` 现共 6 类校验（文档链接、量化数字、边界语句、yaml 键双向、env 覆盖完整性、env 表路径准确性）。
- `test/browser/README.md` 同步。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`prodgates` 仍 PASS；浏览器 19 项 `TOTAL_FAILS=0`；`git diff --check` 干净。
- **边界未变**：纯文档修正与静态检查，未改任何配置解析逻辑或生产门禁；负向改动已按字节恢复。

## 1ag. 部署产物一致性门禁（round 156）

发现一处**真实部署风险**：`make verify` 会 `pnpm build` 重建 `web-admin/dist`，但**不会**同步 `web/dist`；而服务端从 `./web/dist` 提供前端。若有人改了前端源码后只跑 `make verify` 就发布，`web/dist` 仍是旧构建，验证却全绿。
- 给 `test/browser/docs.cjs` 加**第 7 类不变量**：当 `web/dist` 存在时，递归计算 `web-admin/dist` 与 `web/dist` 中每个文件的 SHA-256，断言文件集合与哈希**逐字节一致**；缺文件、多文件、内容漂移、构建目录为空都 FAIL。`web/dist` 不存在时跳过（全新检出尚未部署，`make build` 会创建）。
- **负向验证**：往 `web/dist/index.html` 追加一行注释 → 精确报「deployed asset index.html differs from the built bundle」并 `exit=1`；恢复后 PASS，`index.html` 哈希与构建产物一致。修正了初版 `path.relpath`（Node 应为 `path.relative`）与参数顺序导致的两处报错/错误信息不准。
- `test/browser/README.md` 同步。验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`prodgates` 仍 PASS；浏览器 19 项 `TOTAL_FAILS=0`；`git diff --check` 干净。
- **边界未变**：仅新增静态检查，未改任何构建产物、部署逻辑或生产门禁；负向改动已按字节恢复。

## 1ah. 模块依赖卫生：`go mod tidy`（round 157）

用非破坏性的 `go mod tidy -diff` 复核模块文件，发现真实漂移：`go.mod` 中 `golang.org/x/sync` 被列为直接依赖而 `golang.org/x/sys` 被列为间接（实际相反），`google.golang.org/grpc`/`protobuf` 分类也不准；`go.sum` 缺少 OTel、gonum、grpc 等传递/测试依赖的校验和。当前 `-mod=readonly` 构建虽仍通过，但在干净模块缓存或 CI 中 `go mod tidy` 检查会失败。
- 执行 `go mod tidy`（已先备份 `go.mod`/`go.sum`）：`go.mod` 仅做**直接/间接重分类 + 补回 grpc/protobuf 直接项**，**没有任何版本变更**；`go.sum` 净增 24 行校验和、**未删除任何条目**。执行后 `go mod tidy -diff` → **exit 0（无差异）**、`go mod verify` → all modules verified。
- 验证：`go build -mod=readonly ./...` / `go test -mod=readonly ./...` / `go vet -mod=readonly ./...` / `gofmt -l` 全绿；`make verify` → `VERIFY_EXIT=0`；`prodgates` 仍 PASS；重建二进制后浏览器 19 项 `TOTAL_FAILS=0`；全仓 `go test -race ./...` 复跑（后台任务，见下）；`git diff --check` 干净。
- **边界未变**：仅整理模块元数据，未改任何源码、依赖版本或生产门禁。教训：`go mod tidy -diff` 是零副作用的标准 CI 检查，应定期执行；首次需联网预热模块缓存，之后可离线。

### 1ah-补：全量 race 复跑发现并修复一处测试时序脆弱（round 157 续）

`go test -race ./...` 全量复跑首次报 `internal/enterprise/replicarpc.TestConcurrentRequestsAndCancellation` 13 个 `DeadlineExceeded`。定位：
- 该测试并发 32 个 Pull，但 fixture 只提供 8 个 slot（`fixtureConcurrency`）且每个请求被包在自己的 `fixtureTimeout=15s` 上下文里；在整机跑 race 套件的高负载下，后排请求的 15s fixture 截止时间会先于客户端 20s 到期 —— `DeadlineExceeded` 与 `ResourceExhausted` 一样是**夹具设计内的背压结果**，不是数据竞争或产品缺陷。隔离重跑该用例 `-race -count=3` 全部 PASS，证实为时序脆弱。
- 修复 `internal/enterprise/replicarpc/replicarpc_test.go`：把 `DeadlineExceeded` 与 `ResourceExhausted` 并列为可接受的背压结果，同时用 `atomic.Int32` 统计成功响应并断言 `succeeded > 0` —— 既消除 flake，又保证「服务器完全挂死」不能静默通过；取消语义断言保持不变。**仅改测试**，未动 fixture 并发/超时常量或任何生产代码。
- 验证：该用例 `-race -count=3` PASS；`go test -race ./internal/enterprise/replicarpc/` 全包 PASS；全仓 `go test -race ./...` 二次复跑（后台任务）；`go test ./...` / `go build` / `go vet` / `gofmt -l` / `make verify` / `go mod tidy -diff` / `prodgates` 全绿；浏览器 19 项 `TOTAL_FAILS=0`；`git diff --check` 干净。

## 1ai. 评审材料数字同步与防漂移（round 158）

`docs/audit/ENTERPRISE-REVIEW-DOSSIER.md` 写于 round 128，此后累积 30 轮改动，出现多处静默失真：企业测试数 `569` → 实际 `575`；`snapshotpipeline` 9→11、`ipamstore` 17→20、`dnswire`/`dhcpwire`/`hasign`/`httpcontract`/`hapermit` 各 +1、`prodgates` 新增第 3 组测试；§5 仍写「15 个 Chromium 套件 / 2 个验证入口」（实际 16 套件 + 3 静态守卫 = 19 项检查，4 个 make 入口）。评审人若照旧数字判断会得出错误结论。
- 更新 dossier §1/§2/§3/§5 全部数字与描述：575 个测试入口、4 个可复现入口、3 组 prodgates 回归（含「完整 `writer_authority` 行不授予写权限」）、19 项检查构成、逐包测试数；复核「14 个含 Fuzz/属性/篡改测试的域」仍准确。
- 给 `test/browser/docs.cjs` 加**第 8 类不变量**：从仓库实时重算并钉住 dossier 的「企业测试数」「验证入口数」「浏览器检查数」「10 个门禁哨兵」四项标题数字。`docs.cjs` 现共 8 类校验。
- **负向验证**：把标题数改回 569 → 精确报「review dossier no longer states the current enterprise test count」并 `exit=1`；恢复后 PASS。
- `test/browser/README.md` 同步。验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`prodgates` 仍 PASS；`go mod tidy -diff` 干净；浏览器 19 项 `TOTAL_FAILS=0`；`git diff --check` 干净。
- **边界未变**：纯文档同步与静态检查，未改任何代码、测试数量或生产门禁；负向改动已按字节恢复。

## 1aj. 交付物质量复核与 `verify-tidy` 入口（round 159）

本轮不再新增守卫，改为**复核已累积交付物的卫生**，并把 round 157 的模块检查变成可复现入口：
- **质量复核结论（干净）**：新增源码中 0 个 `TODO/FIXME/HACK`、0 个临时备份/补丁文件；`/tmp/goddi-run` 仅出现在 `test/browser/reset_testdata.py` 的默认说明与 README 示例（可经 `GODDI_TEST_DB` 覆盖）；`AdminPass-0123456789` 仅存在于 Go 测试的隔离内存库夹具，非生产凭据；`console.log` 均为 harness 的 PASS/FAIL 输出；4 个 `*probe*` 文件已确认为基线提交或早期隔离夹具测试。另核查全部 `t.Skip`：唯一实际触发的是 `dnstransport.TestDeniedSourceAndOversize`（本机缺 loopback 别名，理由明确），其余「文件不在树中」的跳过条件在本工作树**均未触发**，无静默覆盖缺口。
- `Makefile` 新增 **`verify-tidy`**（`go mod tidy -diff`，零副作用、漂移即非零退出；冷缓存需联网），加入 `.PHONY` 与 `make help`；`docs/STATUS.md` §4、评审 dossier §1/§5 同步为 **5 个可复现入口**，`docs.cjs` 第 8 类不变量的「验证入口数」钉为 5。
- **负向验证**：把 dossier 的入口数改回 4 → 精确报「no longer states the current verification entry count」并 `exit=1`；恢复后 PASS。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`make verify-tidy` → `TIDY_EXIT=0`；`prodgates` 仍 PASS；浏览器 19 项 `TOTAL_FAILS=0`；`git diff --check` 干净。
- **边界未变**：纯复核、文档同步与一个只读 Makefile 目标，未改任何源码或生产门禁；负向改动已按字节恢复。

## 1ak. B8.5 Runbook 结构与状态复核（round 160）

复核现场工程师的主文档 `docs/B8.5-外部验收Runbook.md`，发现两处真实缺陷：
- **重复的 `## 11.` 章节号**：`## 11. 当前项目对应结论`（2026-09-15 历史快照）与 `## 11. 仓内验收准备进度（round 126）` 并存，交叉引用（STATUS.md §6、dossier、台账 §1d 均指向 §11 的仓内进度）产生歧义。现把历史快照降为 `### 10.1 当前项目对应结论（历史快照，2026-09-15；以下 §11 为最新进度）` 并加注「当前状态以 §11 与 `docs/STATUS.md` 为准」，**§11 恢复唯一**，所有既有交叉引用继续有效。
- **过时的验证数字**：§11 仍写「15 个真实 Chromium 浏览器套件」。改为 **16 个真实 Chromium 套件 + 3 个静态守卫 = 19 项检查**，并补齐 `go mod tidy -diff` 与 5 个 `make` 一键入口、`docs/STATUS.md` 指引。复核 A–E 五行「当前状态」仍全部为 `未验收`，无被误升级项。
- 给 `test/browser/docs.cjs` 加**第 9 类不变量**：5 份面向现场/评审的文档中 `## N.` 编号不得重复；B8.5 runbook 必须保留 ≥5 处「未验收」且写明当前 19 项检查。`docs.cjs` 现共 9 类校验。
- **负向验证**：把历史标题改回 `## 11.` → 精确报「duplicate "## 11." heading」并 `exit=1`；恢复后 PASS。
- `test/browser/README.md` 同步。验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`make verify-tidy` 全绿；`prodgates` 仍 PASS；浏览器 19 项 `TOTAL_FAILS=0`；`git diff --check` 干净。
- **边界未变**：纯文档修正与静态检查，未改任何代码或生产门禁；未把任何 B8.5 条目升级为 PASS；负向改动已按字节恢复。

## 1al. 交叉引用准确性复核（round 161）

复核所有指向 `docs/B8.5-外部验收Runbook.md` 的章节引用（round 160 刚调整过编号），发现两处**指向错误章节**：
- `docs/audit/ENTERPRISE-REVIEW-DOSSIER.md` §5 写「B8.5 **仓内证据归档**见 §11」，但 §11 是「仓内验收准备进度」，真正的证据模板在 §8/§10。改为「仓内准备进度见 §11，现场原始证据模板见 §10」。
- `docs/tdesign-migration.md` §3.3 写「§11 明确：真实双主机…未执行、`internal/cluster` 未实现」，但该结论实际来自 §1 验收矩阵（A–E 全部 `未验收`）与 §9/§10.1 设计结论。改为指向 §1 + `docs/STATUS.md` §3 + Runbook §9/§10.1。
- 复核其余引用（STATUS.md §3/§6、SNAPSHOT-MIGRATION-PATH.md、UPSERT-WRITE-PATH-COMPAT.md、台账 §1g/§1d 等）均指向正确的 §10 模板或 §11 进度。
- 给 `test/browser/docs.cjs` 加**第 10 类不变量**：扫描 `docs/` 与 `contracts/` 下所有 `.md`，把 `B8.5-外部验收Runbook.md §N` 的 N 与 runbook 中真实 `## N.` 编号比对（支持 `§10.1` 形式取主编号），任一悬空引用即 FAIL。`docs.cjs` 现共 10 类校验。
- **负向验证**：把 STATUS.md 的 `§10` 改成 `§99` → 精确报「references B8.5 runbook §99, which does not exist」并 `exit=1`；恢复后 PASS。
- `test/browser/README.md` 同步。验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`make verify-tidy` 全绿；`prodgates` 仍 PASS；浏览器 19 项 `TOTAL_FAILS=0`；`git diff --check` 干净。
- **边界未变**：纯文档修正与静态检查，未改任何代码或生产门禁；负向改动已按字节恢复。

## 1am. 生产配置闸门运行期复核（round 162）

补一类此前只由单测覆盖、未在真实启动路径验证过的安全边界：`internal/config` 的 `env: production` 收紧规则。用重建后的 `./dist/goddi` 与 4 份隔离配置（`/tmp/goddi-run/prod162/`，独立 DB 与端口 18086）实测：
- 缺失 `security.encryption_key` → 启动拒绝 `EXIT=1`；
- 独立 32 字符 `encryption_key` → 启动成功，`/health` / `/ready` 均 `{"status":"ok"}`、`/login` 200、日志 0 ERROR；
- `encryption_key == jwt_secret` → 拒绝；
- `encryption_key` 长度 5 → 拒绝（要求 ≥16）。
- 单测 `internal/config/secrets_test.go` 已覆盖 4 条规则；本轮提供**二进制级端到端证据**，确认校验先于监听/数据库打开执行，不只是 `Validate()` 的单元行为。详见 `docs/audit/RUNTIME-VALIDATION-LEGACY.md` §7。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`make verify-tidy` 全绿；`prodgates` 仍 PASS；浏览器 19 项 `TOTAL_FAILS=0`；`git diff --check` 干净；测试用进程全部停止。
- **边界未变**：纯运行期复核，未改任何配置逻辑、代码或生产门禁；测试配置均在 `/tmp` 隔离目录，未触碰主隔离库与生产。

## 1an. 随仓库 `config.yaml` 端到端启动复核与两处运维体验修正（round 163）

此前浏览器回归都用 `/tmp/goddi-run/node1-all.yaml`；本轮首次用**随仓库发布的 `config.yaml`** 配合文档要求的环境变量（`GODDI_SECURITY_JWT_SECRET`、隔离 `GODDI_SERVER_DATA_DIR` / `GODDI_DATABASE_DSN` / `GODDI_SERVER_HTTP_ADDR`）走完整流程，发现并修正两处真实运维体验问题：
- **`migrate --check` 提示误导**：`goddi migrate` 按设计（`docs/实施计划-v0.6.0.md` W12-b）**只准备控制库**，`--status/--check` 必须能在不创建任何库的前提下检查部署；数据面 `dnsdata`/`leases` 由首次 `serve` 创建并迁移。但全新部署上 `--check` 报「有 34 个迁移待应用；请运行 goddi migrate 应用它们」，而再跑 `migrate` 无法消除这 34 项 —— 把运维引向死循环。现补充提示「若待应用项来自 dnsdata/leases 数据面库，它们由首次启动 `goddi serve` 创建并迁移（migrate 不会创建数据面库）」。**语义未改**：`migrate` 仍不创建数据面库，`--status` 仍只读。实测 `migrate` → `--check` 失败（含新提示）→ 首次 `serve` → `--check` 零退出（三库 0 待应用）。
- **健康部署被记为 ERROR**：`config.yaml` 的 `dhcp.interfaces: []`（自动探测）在本机跳过 22 个无 IPv4 的网卡，原代码用 `slog.Error` 逐条记录，使 `/health` 正常的服务在日志里像启动失败。改为 `slog.Warn("...skipping interface without a usable IPv4 address")`，只有真正无法 bind/listen 才返回错误。重建二进制后随仓库 `config.yaml` 启动：**0 条 ERROR、22 条预期 WARN**、`/health` `{"status":"ok"}`。
- 验证：`go test ./internal/dhcp/server/` PASS；`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`make verify-tidy` 全绿；`prodgates` 仍 PASS；浏览器 19 项 `TOTAL_FAILS=0`；`git diff --check` 干净；所有测试进程停止。全部测试配置在 `/tmp/goddi-run/cfg163/`，未改仓库 `config.yaml`、未触碰主隔离库与生产。
- **边界未变**：一处错误信息补充 + 一处日志级别修正，未改任何迁移/启动语义、DHCP 行为或生产门禁。

## 1ao. `migrate` 只建控制库契约的回归钉住（round 164）

复核 round 163 的两处修正是否只是「改了提示」而无回归保护。先确认 `docs/runbook/灾备恢复.md` 步骤 1 已明确写出「`dnsdata`/`leases` 由服务装配时创建，不是 `migrate` 建的，只跑 `migrate` 归档会缺库」，与 W12-b 契约一致 —— **无文档冲突**。
- 新增 `cmd/goddi/migrate_dataplane_test.go`（2 组）：
  - `TestMigratePreparesOnlyTheControlStore`：隔离配置下执行 `runMigrations`，断言 `control.db` 已建而 `data/dnsdata.db`、`data/leases.db` **仍不存在** —— 钉住「`--status` 检查不得意外创建数据面库」这一 W12-b 核心不变式。
  - `TestMigrationCheckExplainsWhereDataPlaneStoresComeFrom`：`migrate` 后调用 `runMigrationStatus(path, true)`，断言错误文本同时包含 `goddi serve`、`不会创建数据面库`、`dnsdata/leases` —— 防止 round 163 补充的提示被无声改回误导性文案。
- **负向验证**：把 `--check` 错误文案临时改回旧的「请运行 goddi migrate 应用它们」→ 测试精确报「does not explain "goddi serve"」并 FAIL；恢复后 PASS。
- 验证：`go test ./cmd/goddi/` 全绿；`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`make verify-tidy` 全绿；`prodgates` 仍 PASS；浏览器 19 项 `TOTAL_FAILS=0`；`git diff --check` 干净。
- **边界未变**：仅新增回归测试，未改任何迁移/启动语义或生产门禁；负向改动已按字节恢复。

## 1ap. 加密备份/恢复往返与完整性拒绝（round 165）

用当前二进制 + 随仓库 `config.yaml` + round 163 已起过一次的三库实例（独立 `/tmp/goddi-run/cfg163/`、专用备份加密密钥）复跑灾备链路：
- `goddi backup` → 57,653 字节归档，含 control(schema 32/62 表)、dnsdata(17/31)、leases(17/31)、配置、密钥盘点与两份密钥指纹，`EXIT=0`；
- `restore --verify` → 「归档完整，且与本机密钥材料一致。未写入任何文件」，`EXIT=0`；
- `restore --into` → 解出 3 库 + 配置，`EXIT=0`；三库 `PRAGMA integrity_check` 全 `ok`；
- **负向验证**：篡改归档 1 字节 → `EXIT=1`（`chunk 0 does not authenticate`，AEAD）；换错 `encryption_key` → 同样 `EXIT=1`。证明归档认证确实生效，而非只做长度/格式检查。
- 详见 `docs/audit/RUNTIME-VALIDATION-LEGACY.md` §8。未执行 `--in-place`，未触碰主隔离库与生产；**不构成** B8.5-C 真实断电/持久盘验收。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`make verify-tidy` 全绿；`prodgates` 仍 PASS；浏览器 19 项 `TOTAL_FAILS=0`；`git diff --check` 干净。
- **边界未变**：纯运行期复核，未改任何代码或生产门禁。

### 1ap-补：batch10 子网 CIDR 碰撞 flake 修复（round 165 续）

本轮全量浏览器回归首次出现 `batch10 | FAIL | subnet created end-to-end` 并伴随 1 条 500。查服务端日志定位为**测试夹具碰撞，非产品缺陷**：`batch9` 以随机八位组 `70..149` 建 `td-seed-subnet-<ts>` 且不删除，`batch10` 从 `120..179` 随机取八位组，本次都落到 `10.132.0.0/24`，触发 `CIDR overlaps with existing subnet`（500）。`reset_testdata.py` 只在每轮**开始前**清理，无法避免同一轮内 batch9→batch10 的累积。
- `test/browser/batch10.cjs`：创建前用 `api('GET','/ipam/subnets?page_size=500')` 收集已占用八位组，从 **200..239**（与 batch9 的 70..149 完全不相交）挑第一个空闲值；套件末尾自删新建子网（`createdSubnetName` 提升到外层作用域，避免块级作用域导致清理落空）。
- `test/browser/batch9.cjs`：同样改为查询已占用八位组后从 70..149 挑空闲值，使**不重置**的连续多轮也不会与自身遗留种子碰撞（该种子因已分配地址不能自删）。
- **压力验证**：`GODDI_SKIP_RESET=1` 连续两轮全量 19 项 + 一轮带 reset 全量，`TOTAL_FAILS` 全部为 0（共 3 轮）；`node --check` 两个脚本通过。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`make verify-tidy` 全绿；`prodgates` 仍 PASS；`git diff --check` 干净；所有服务进程停止、后台任务已收集。
- **边界未变**：仅修测试夹具，未改任何产品代码或生产门禁。

### 1ap-补二：部署一致性守卫在实战中捕获真实漂移（round 165 续）

收尾验证时，`docs.cjs` 第 7 类守卫**自行报出** 44 项「deployed asset ... differs from the built bundle」+ 44 项「built asset ... is missing from web/dist」。定位为 round 156 预判的真实场景：`make verify` 内部 `pnpm build` 用新的内容哈希重建了 `web-admin/dist`，但**没有**同步 `web/dist`（服务端实际提供的目录）。这不是产品缺陷，而是构建顺序问题 —— 守卫按设计生效。
- 重新部署 `web/dist`（`cp -r web-admin/dist/. web/dist/`）后三个静态守卫全部 PASS；未改守卫逻辑来「放行」。
- 在 `docs/STATUS.md` §4 与 `test/browser/README.md` 加**顺序提醒**：跑完 `make verify` 后须执行 `make build`（或手动同步）再跑 `make verify-browser`，并说明守卫会逐字节比对两棵树。
- 新提醒里出现的 `docs.cjs` 触发了 `docs.cjs` 第 1 类链接检查的误报（它只解析仓库根 / `docs/` / `docs/audit/` 下的相对路径）。给候选路径补上 `test/browser/`，并用负向验证（注入 `test/browser/NO-SUCH-FILE.md`）确认仍会精确报出悬空链接。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` / `make verify-tidy` 全绿；`prodgates` 仍 PASS；浏览器 19 项 `TOTAL_FAILS=0`（含 3 轮压力）；`git diff --check` 干净。
- **边界未变**：仅补文档顺序说明与链接解析候选路径，未改任何产品代码或生产门禁；负向改动已按字节恢复。

## 1aq. W12 灾备演练全套复跑与变异检查缺陷修复（round 166）

继 §1ap 的备份/恢复往返后，复跑 `docs/runbook/灾备恢复.md` 配套的**全套可执行演练**（均从仓库根目录、用当前代码构建二进制）：
- `scripts/w12c_restore_drill.py` → **EXIT=0**，「一台什么都没有的主机变成了可用实例」。覆盖：源实例**真的删除**、目标在不同绝对路径且配置从未指向源、目标**不跑 `migrate`** 直接 `restore --in-place`、三库与归档成员**逐字节相等**且不等于源活文件、`PRAGMA integrity_check` ok、无残留日志/共享内存/副本、恢复库无需迁移、schema 等级 32==32、源标记行仍在、恢复实例启动后 `/health` 与 `/ready` 均 200、干净关闭无 panic、数据面自报 current。
- `scripts/w12_snapshot_smoke_check.py` → **EXIT=0**，「every snapshot and restore assertion held」（含归档版本拒绝、schema 前后关系、回滚等断言）。
- `scripts/w12_mutation_check.py` → 初次 **EXIT=1**，报「1 项变异未被捕获」：`migrate --status 直接打开数据库，从而创建它` 因替换目标 `if _, err := os.Stat(path)... if os.IsNotExist(err) {` 在 `cmd/goddi/main.go` 中**出现 2 次**（另一处在 DHCP HA status 路径）被标记 `SKIPPED-AMBIGUOUS`。核实 **base 提交即为 2 次**，是脚本自身遗留缺陷 —— 该变异从未真正被测试过。修复：把替换目标锚定到 `migrationStatusOfStore` 特有的注释行 `// Never opened: opening a SQLite path creates the file, and a`，使目标唯一；重跑 → **EXIT=0**，「每处变异都被相应的检查捕获」（6/6），且脚本临时改动的 `main.go` 经 `cmp` 确认**逐字节还原**。
- 同步 `docs/B8.5-外部验收Runbook.md` §1 的 B8.5-C「当前仓内证据」列，补入 round 165/166 的往返、篡改/错密钥拒绝与全套演练结果；该行状态仍为 `未验收`（真实断电/持久盘/恢复前后校验仍需现场）。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`make verify-tidy` 全绿；`prodgates` 仍 PASS；重新部署 `web/dist` 后浏览器 19 项 `TOTAL_FAILS=0`；`git diff --check` 干净；所有演练进程停止、无残留临时目录。
- **边界未变**：仅复跑演练 + 修一个测试脚本的定位缺陷 + 文档证据更新，未改任何产品代码或生产门禁；演练全程在脚本自建的临时目录，未触碰主隔离库与生产。**不构成** B8.5-C 真实断电验收。

## 1ar. W 系列验证脚本全套复跑与 3 处陈旧变异目标修复（round 167）

对 `scripts/w*.py` 中 13 个可快速执行的验证/变异脚本做了整套复跑（每个脚本单独超时 300s）。首轮：**8 绿 / 5 红**，5 个红全部是 `SKIPPED-NOT-FOUND`（变异目标在源码中找不到），且这些脚本里其余变异均仍被捕获 —— **没有守护机制回归，是脚本目标随历史重构漂移**。核实 base 提交即已存在同类问题（`docs/实施计划-v0.6.0.md` 自身记载 R16/R17「因机制不复存在而退役」的先例）。
- **已修复 3 个脚本**（均做到目标唯一、变异真实注入、检查确实失败、`TREE-CLEAN`、被改动源码 `cmp` 逐字节还原）：
  - `scripts/w09_ha_gap_mutation_check.py`：`GATED` 目标缺了后加的 `FactsGap` 双水位计算，补回后 2 处变异不再空转 → **3/3 捕获**。
  - `scripts/w14_durability_mutation_check.py`：`synchronous` 设置已从 `dataplane/store.go` 迁到 `internal/sqlitepolicy/connection.go`；把变异改为同时改「施加的 pragma」与「校验期望值」两行，使连接仍能打开（`synchronous=OFF`），从而保住「崩溃演练在掉电未验证下仍通过」这一**预期存活**断言 → **4/4 捕获 + 1 处预期存活**。
  - `scripts/w14i_config_version_mutation_check.py`：`adapters_records.go` 改为先算 `name := zone.NormalizeRecordName(...)` 再 INSERT；把变异改为禁用该归一化并保留 `zoneName` 引用以维持可编译 → **4/4 捕获**。
- **8 个原本全绿**：`w05_revert_check`、`w07_role_smoke_check`、`w13_perm_consistency_mutation_check`、`w14_authz_mutation_check`、`w14_branch_mutation_check`(10/10)、`w14_capacity_mutation_check`(5/5)、`w14_protocol_mutation_check`、`w14g_push_isolation_mutation_check`(8/8)。
- **仍有 2 个历史脚本存在陈旧目标（本轮未改，明确列为后续）**：`w06_revert_check.py` 3 处（R4/R5 的 IPAM 观察-审计写入路径、R9 的租约过期 sweep 上报，均已重构）、`w07_revert_check.py` 5 处（R1–R3 的 WAL/FULL/foreign_keys 已统一到 `internal/sqlitepolicy`，R20/R25 的控制台删除与下行同步 serial 恢复位置已变）。它们**仍会 FAIL 而非假绿**，需要把每个已退役/迁移机制映射到当前代码位置后重新锚定；在此之前不能当作绿色门禁。
- 本轮另已复跑 `w12c_restore_drill.py`、`w12_snapshot_smoke_check.py`、`w12_mutation_check.py`（见 §1aq），长时容量的 `w14_capacity_profile.py` / `w14_durability_check.py` / `w14h_split_outage_drill.py` 等未在本轮重复。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`make verify-tidy` 全绿；`prodgates` 仍 PASS；重新部署 `web/dist` 后浏览器 19 项 `TOTAL_FAILS=0`；`git diff --check` 干净；所有变异脚本临时改动均已还原（逐文件 `cmp` 一致）。
- **边界未变**：仅修测试脚本的变异目标定位，未改任何产品代码或生产门禁；未把 `SKIPPED-NOT-FOUND` 放宽为通过。`docs/关键分支覆盖-w14.md` 头部加注本轮复跑结论与漂移修复说明。

### 1ar-补：`w06_revert_check.py` 陈旧目标修复，W 系列 13/13 全绿（round 168 续）

§1ar 遗留的最后一个脚本 `w06_revert_check.py`（3 处 `SKIPPED-NOT-FOUND`）本轮修复完毕。先核实 `base` 提交中 `internal/ipam/address/address.go` 的观察路径即已改为「`if !changed` 早退 + 单语句 history 行」，`internal/dhcp/server/server.go` 的过期 sweep 中 `observeLease` 与状态判断也不再相邻 —— 三个目标**在 base 就已失配**，是脚本漂移而非守护机制回归。
- **R4**「observation 不再移动分配状态」：目标改为 `if !changed { return false, nil }` 早退守卫，变异为 `if true || !changed`（保持 `changed` 被引用，避免 INVALID「编译失败」假阳性 —— 初版 `if true {` 正是这样被脚本自己的 INVALID 判据拦下的）。
- **R5**「observation 不再写审计行」：目标改为现行单语句 `insertHistoryTx(... Action: "observe" ...)` 整块，变异为 `_ = reason`（顺带吸收 `reason` 失去唯一使用点后的编译错误）。
- **R9**「过期 sweep 不再上报被清扫的租约」：目标改为单独的 `s.observeLease(LeaseObservedExpire, l)` 行（现在与状态判断之间隔了注释和 replicateLeaseState）。
- **全量 13 脚本复跑**：`w05` 9/9、`w06` **12/12**、`w07` **44/44**、`w07_role_smoke` 4 角色、`w09_ha_gap` **3/3**、`w13_perm_consistency` 全捕获、`w14_authz` / `w14_protocol` 全捕获、`w14_branch` **10/10**、`w14_capacity` **5/5**、`w14_durability` **4/4 + 1 预期存活**、`w14g_push_isolation` **8/8**、`w14i_config_version` **4/4** —— **13/13 全绿**，所有脚本 `TREE-CLEAN`，被改动源码 `cmp` 逐字节还原。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`make verify-tidy` 全绿；`prodgates` 仍 PASS；浏览器 19 项 `TOTAL_FAILS=0`；`git diff --check` 干净。
- **边界未变**：仅修测试脚本的变异目标定位，未改任何产品代码或生产门禁；未把 `SKIPPED-NOT-FOUND` 放宽为通过。

## 1as. 长时 W 脚本复跑：耐久 / 容量 / 三进程分裂演练（round 170）

§1ar/§1aq 覆盖了 13 个快速 W 脚本；本轮补跑剩余三个**长时进程级演练**（各自自建实例与配置，用当前代码构建的二进制）：
- `scripts/w14_durability_check.py` → **EXIT=0**：`kill -9` 崩溃一致性演练 A/B/C 三节全部成立（真子进程 + 真 SIGKILL；提交绑定落盘、ACK 集合对账、DRAIN 关闭契约；文末「不能声称的」明确 kill -9 不等于掉电）。
- `scripts/w14_capacity_profile.py` → **EXIT=0**：容量画像 A/B/C 三节完成，租约唯一合法写者是 `lease.Manager`，每项上界成立。
- `scripts/w14h_split_outage_drill.py` → **EXIT=0**：三进程分裂/停机/恢复演练 **C 节 18/18 条断言通过**（控制进程重启后追平、DNS 面最终一致、旧主隔离等）。
- 至此 `scripts/w*.py` 全部 22 个脚本在当前工作树**全部实测通过**（13 快速 + 1 变异全套 + 3 长时 + `w12` 三件套在 §1aq/§1ar），`SKIPPED-*` 与「未捕获」均为 0。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`make verify-tidy` 全绿；`prodgates` 仍 PASS；浏览器 19 项 `TOTAL_FAILS=0`；`git diff --check` 干净；所有演练进程/临时目录清理完毕。
- **边界未变**：纯运行期复跑，未改任何代码或生产门禁；这些演练均为单机多进程证据，**不构成** B8.5 的跨主机 / 掉电 / 真实 relay 验收。

## 1at. 剩余 7 个脚本复跑与 d1 变异判据修复 —— 脚本清单全绿（round 171）

§1ar/§1as/§1aq 覆盖了 22 个 W 系列脚本；本轮补跑其余 7 个，并修复其中 1 个空转的变异判据：
- `b5_revert_check.py` **10/10**、`b5_surface_smoke_check.py` 管理面冒烟通过、`w08_ha_smoke_check.py` 4 项 HA 行为成立、`w09_ha_operator_smoke_check.py` 6 项运维行为成立、`release_smoke.sh` **PASS**（v0.24.1：干净库启动、内嵌 UI、登录、重启后 zone 持久）。
- `w13_frontend_smoke.py` 是实例管理器（`up`/`down` 子命令，供 e2e fixture 套件用），首轮 sweep 无参调用才报 exit 2 —— 本轮实测 `up`（建三库实例、种子 /24、稀疏 /15、带 DNS 记录的地址、打印状态 JSON）与 `down` 均 **exit 0**。
- **`d1_ddns_latency_check.py` 变异判据修复（真实缺陷）**：唤醒链现已是**两跳**——① `SetOutboxWake(dhcpRunner.Wake)`、② DNS runner `OnApplied` → `dnsConsumer.Wake()`（后者为后续轮次补的「DDNS 应用后唤醒时延」）。脚本原变异只撤 ①，② 单独就把等待压回一段，变异后实测最慢一轮 1.001s < 2.25s 失败线，门禁**空转**（首轮 sweep 报「撤掉唤醒后连跑两次都通过」的根源）。修复：变异同时撤两跳，使路径回到文档记录的两段轮询；并发现本机相位锁定使两段轮询稳定为 **2.0s < 2.25s**，「变异后用例失败」这一旧判据在本机不可满足——改用等价且更直接的判据：**撤掉唤醒后最慢一轮必须比基线慢 ≥0.5s（半个轮询间隔）**（实测恰为 +0.998s = 整整一段；三轮抖动仅 ~6ms）。修复后实测：基线 1.001s、变异 1.999s、**EXIT=0**、树逐字节还原。
- **至此 `scripts/` 下全部 29 个验证/演练脚本在当前工作树实测通过**（13 快速 + 3 长时 + `w12` 三件套 + 本轮 7 个），0 `SKIPPED-*`、0 `NOT-VERIFIED`、0 未捕获变异。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`make verify-tidy` 全绿；`prodgates` 仍 PASS；浏览器 19 项 `TOTAL_FAILS=0`；`git diff --check` 干净；被变异文件 `cmp` 逐字节还原；`w13` 实例已 `down`，无残留进程。
- **边界未变**：仅修一个测试脚本的变异范围与判据 + 运行期复跑，未改任何产品代码或生产门禁；全部为单机证据，不构成 B8.5 外部验收。

## 1au. e2e fixture 套件复活：针对 TDesign UI 重写 8/8 全绿（round 172）

`上线前测试报告` 的 W-5「`e2e/` Playwright 套件本环境不可用」与本会话从未跑过的缺口，本轮收口：
- **依赖**：`e2e/` 的 `@playwright/test ^1.63.0` 经 `npm install` 装齐（4s，与 `test/browser/` 的 playwright 1.63.0 同代）；chromium 用既有 ms-playwright 缓存。
- **实例**：`scripts/w13_frontend_smoke.py up/down` 实测 exit 0（该脚本原 `上线前测试报告` 记为本环境不可用，实为其角色是**实例管理器**而非检查器，必须带 `up`/`down` 子命令调用；首轮 sweep 无参调用才报 exit 2）。
- **失败根因（两处，均为测试套件自身漂移）**：① 规格「**撤掉唤醒后连跑两次都通过**」见 §1at —— 唤醒链已变两跳、本机相位锁定的失败线不可满足，已改用时长差判据修复。② fixture 规格选择器全部是 pre-TDesign 的：`.n-modal`/`.n-drawer`/`.n-radio`/`.n-form-item`、行操作按 `button` role 找（TDesign `TLink` 渲染为**无 href 的 `<a>`，无隐式 link role**，须按精确文本匹配）、且不知道关闭后的 `.t-dialog` DOM 会保留（隐藏而非卸载）。
- **重写**：`e2e/tests/ipam-console.fixture.spec.ts` 491 行中 22 处结构选择器更新（`.n-*` → `.t-*`、行操作 `button` → `getByText(exact)`、关闭断言 `toHaveCount(0)` → `toBeHidden()`），**全部行为断言与注释原样保留**；文首加 TDesign 选择器说明。语言前提仍成立：console 的语言键仍是 `GODDI_lang`（`VITE_STORAGE_PREFIX=GODDI_` + `lang`、JSON 引号值），规格的 `addInitScript` 使 UI 渲染 en-US，其英文文案断言与 `en-us.ts` 现值逐一对上（`Import addresses`/`Preview — nothing has been written yet`/`Import result`/`Problems that stop the import`/`IP detail · <ip>`/`DHCP scope coverage` 等）。
- **结果**：`npx playwright test --config playwright.fixture.config.ts` → **8 passed (21.6s)**，EXIT=0。实例 up/down 干净，`w13` 进程停止。
- 其余 4 个 bare-config 规格（`api`/`console`/`workflows`/`config-versions`）**尚未重跑**：`api.spec` 为纯 API（另需 `GODDI_TEST_DNS_PORT` 的 dig），三个 UI 规格的标签/流程待逐一对齐；它们按 CI 设计跑在**另一个未种子的实例**上，本轮未重复。已列为后续。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`make verify-tidy` 全绿；`prodgates` 仍 PASS；`test/browser/` 19 项 `TOTAL_FAILS=0`；`git diff --check` 干净；`e2e/node_modules`、`test-results/` 被 `e2e/.gitignore` 覆盖，工作树只余规格本身的修改。
- **边界未变**：仅重写测试套件选择器，未改任何产品代码或生产门禁。

## 1av. bare-config e2e 规格：api.spec 复活通过、3 个旧 shell UI 规格处置（round 173）

§1au 只复活了 fixture 套件；本轮补跑 bare 侧：
- **`api.spec.ts` → 1 passed (2.2s)**：对 DNS-enabled 隔离实例（`/tmp/goddi-run/e2e173/`，HTTP 18088、DNS 15360）实测 **token 隔离、CSRF 拒绝、scope 限制（403）、单次使用令牌（第二次 401）、DNS 备份恢复往返 + dig 解析**全部成立。该规格零 UI 选择器，TDesign 迁移不涉。
- **3 个旧 shell UI 规格（`console`/`workflows`/`config-versions`，共 1082 行）处置为「旧 shell 遗物，待重写」**：其选择器绑定 pre-TDesign 的 soybean/Naive 外壳（`[aria-label="Switch language"]`、`.n-spin-body`、`[class*="layout-mobile-sider"]`、`.n-modal`），外壳已整体更换，这些断言测的对象**已不存在**。其行为内容与现有覆盖的映射：
  - zone/forwarder/scope/backup/user/token/blocklist/option 生命周期 → `test/browser/batch3–batch8` 已覆盖同页面 CRUD；
  - config-versions 三测（含「未过滤表回滚引用行自身 revision」语义）→ `batch13` 渲染 + 服务端 `configver` 单测；
  - **两块独有且未覆盖**：`console.spec` 的「read-only 角色跨模块打不开写操作」（RBAC UI 强制，`test/browser` 未测只读角色）与 `workflows.spec` 的「分页显示下一页而非二次切片」——列为重写时的**必保项**，避免丢失。
- 复跑方式：裸实例 = `/tmp/goddi-run/e2e173/plain.yaml`（独立 DB/端口，DNS enabled）；`w13_frontend_smoke.py` 的种子实例与 bare 套件按 CI 设计**分实例运行**，不可混用（fixture 套件写而不清理，bare 套件假设表未被他人动过）。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`make verify-tidy` 全绿；`prodgates` 仍 PASS；`test/browser/` 19 项 `TOTAL_FAILS=0`；全仓 race 复跑 `RACE_EXIT=0`（本轮后台任务）；`git diff --check` 干净；实例停止。
- **边界未变**：仅运行期复核与文档处置，未改任何产品代码或生产门禁；api.spec 的通过不改变 `501` 契约与受保护写门禁。

## 1aw. 只读角色写拒绝门禁（round 175，接 §1av 的独有缺口）

§1av 列出的第一块独有缺口——旧 e2e `console.spec` 的「read-only 角色跨模块打不开写操作」——本轮以**服务端回归**钉住：
- 新增 `TestReadOnlyRoleIsRefusedByEveryMutatingOperation`（`internal/api/rbac_coverage_test.go`）：通过真实 API 建立「持有全部 `:read` 权限的角色 + 仅持该角色的用户」（`/permissions` 目录收集 read、`/roles` 建角色、`/roles/{id}/permissions` 整集授予、`/users` 建用户、`/users/{id}/roles` 分配），然后对该用户逐个发起 **120 个变更型 Secure 操作**（全部带其自身 CSRF token）。结果：**114 个 403**，6 个按设计可达者全部核实为合法——账户自助（`/auth/toto/*`、`/auth/sessions/{id}` 自己的会话、`/auth/logout`、`/auth/change-password`）与只读语义操作（`/dns/client` 受 `dns:read` + SSRF 守卫、`/ipam/import/preview` 受 `ipam:read` 且**设计上不写任何行**，服务端另有 `TestAPreviewWritesNothing` 钉住）。
- 正向对照：该角色的 `GET /users` 返回 200（角色确实持有其读权限；固定用 `/users` 因 router-only fixture 的 DNS 容器为 nil，DNS handler 会因环境而非权限 500）。
- 过程中排除两个假阳性根源并写进注释：不带 CSRF 的变更请求全部 401（CSRF 层先于 RBAC 拒绝），测试因此**必须携带读者自身的 CSRF token** 才能让权限层真正裁决；`/ipam/import/preview` 的 500 是 fixture nil 容器。
- **负向验证**：临时摘掉 `POST /ipam/spaces` 的 `ipam:write` 闸门 → 测试精确报「1 mutations: POST /api/v1/ipam/spaces -> 500」（到达 handler 而非 403）并 FAIL；恢复后 PASS。`router.go` 按字节还原。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`make verify-tidy` 全绿；`prodgates` 仍 PASS；`git diff --check` 干净。
- **边界未变**：仅新增回归测试，未改任何路由、权限或生产门禁；allowlist 逐项对照路由注册核实过，是既有产品设计而非新放宽。

## 1ax. 分页跨页语义服务端钉住（round 175，接 §1av 第二块独有缺口）

§1av 列出的第二块独有缺口——旧 e2e `workflows.spec` 的「分页显示下一页而非二次切片」——本轮以**服务端回归**钉住（该缺陷的形态在 SQL 而非控制台：对已限定的子查询再套 LIMIT/OFFSET，第 2 页返回的仍是第 1 页的行，表尾不可达）。
- 新增 `internal/api/handler/ipam_pagination_test.go`：真实迁移 schema 内存库 + `withIPAMServices`，经 `CreateIPAMSpace` 建 11 个空间，经 `ListIPAMSpaces` 分别取 `page=1` / `page=2` / `page=3`（`page_size=10`），断言：第 1 页恰 10 行、**第 2 页恰 1 行**（二次切片会是 10 行）、两页无重复且并集覆盖全部 11 个名字、第 3 页为空、meta `total=11 pages=2`。
- **负向验证**：把 `ListSpaces` 的 `offset := (page-1)*pageSize` 临时改为 `offset := 0`（二次切片形态）→ 测试精确报「page 2 has 10 rows, want exactly the 1 remaining row (a re-slice of page 1 is the old bug)」并 FAIL；还原后 PASS，`space.go` `cmp` 逐字节一致。
- 该钉住与其余已覆盖项一起，使 §1av 两块「独有且未覆盖」缺口全部闭合（上一轮的只读角色写拒绝 + 本轮分页语义）；旧 bare UI 套件剩余的 shell-chrome 断言（语言切换下拉、移动端抽屉、spinner）测的是已更换的外壳本身。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`make verify-tidy` 全绿；`prodgates` 仍 PASS；`git diff --check` 干净。
- **边界未变**：仅新增回归测试，未改任何 SQL、handler 或生产门禁；负向改动已逐字节还原。

## 1ay. 交接总览同步与全量 race 复跑（round 176）

- `docs/STATUS.md` §1 前端行补记 round 172–173 的 e2e 复活结论（fixture 8/8、api.spec 1/1、3 个旧 shell 规格处置、两块独有语义已由 §1aw/§1ax 服务端钉住）——交接页此前只提 `test/browser/`，读者会以为 Playwright e2e 已死。
- 全仓 `go test -race ./...` 复跑 → **RACE_EXIT=0**，0 FAIL / 0 DATA RACE（覆盖 round 175 新增的只读角色写拒绝与分页跨页两个测试文件）。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`make verify-tidy` 全绿；`prodgates` 仍 PASS；`git diff --check` 干净。
- **边界未变**：纯文档同步与复跑，未改任何代码或生产门禁。

## 1ba. `config-versions.spec` 复活：3/3 全绿（round 177）

§1av 处置的 3 个旧 shell UI 规格中，`config-versions.spec.ts`（282 行、3 测）本轮重写完毕并实测通过：
- 失败根因与 fixture 规格相同（`.n-data-table`/`.n-spin-body`/`.n-base-selection`/`.n-base-select-option`、`data-col-key` 属性、`getByRole('button')` 找 TLink 行操作、dialog header 非 heading role）。重写点：revision 表按「行内含 Rollback 操作」识别（页面上的资源类型卡不是数据表）、类型/资源列改为**位置单元格**（TDesign 渲染无 `data-col-key`；列序 revision/status/type/resource-id/…）、行操作按精确文本匹配、回滚对话框按 `.t-dialog` hasText 识别。**全部行为断言与注释原样保留**——含「回滚是发布而非撤销」「未过滤表的基线按资源自身取，行号不是基线」这两个回归语义，以及 `dns_zone`/`dns_records` 共用 resource id 的过滤陷阱说明。
- 语言前提仍成立（`GODDI_lang` → en-US），en-US 文案与 `en-us.ts` 现值一致（`Roll back to an earlier revision`/`Publish as new revision`/`Rollback published`/`was modified concurrently`）。
- 结果：`npx playwright test tests/config-versions.spec.ts` → **3 passed (8.7s)**，EXIT=0，实例 down 干净。该规格的三项语义（页面接线真实类型/历史、回滚即新发布且基线按行自身资源、未过滤表回滚引用行自身 revision）在迁移后的 UI 上继续被钉住。
- 剩余：`console.spec`（4 测）与 `workflows.spec`（11 测）仍为旧 shell 遗物待重写；其生命周期行为已由 `test/browser/batch3–8` 覆盖，两块独有语义已由 §1aw/§1ax 服务端钉住。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`make verify-tidy` 全绿；`prodgates` 仍 PASS；`git diff --check` 干净；`w13` 实例已 down。
- **边界未变**：仅重写测试套件选择器，未改任何产品代码或生产门禁。

### 1aw-补：只读角色的 UI 半边钉住（round 178，接 §1aw）

§1aw 钉了服务端半边；本轮以**真实浏览器**钉 UI 半边，新增 `test/browser/batch16.cjs`（13 断言，0 error，已入 `make verify-browser`）：
- 经真实 API 建「全部 `:read` 权限的角色 + 仅持该角色的用户」（复刻 §1aw 的矩阵），再以该用户**经 UI 登录**。
- 断言：zones/ipam/subnets/admin/users 三页正常渲染（角色确实持有全部读权限）；三页的**新建按钮全部缺席**（`v-if="perm.canWrite(...)"` 直接不渲染）；以读者点击 zones 行内 `删除` → **不弹出确认对话框**（点击处理器 `confirmDelete` 自身再查权限后直接返回）。
- **一个真实的 UX 观察（如实记录，未改）**：TDesign `TLink` 的 `disabled` prop 在这些行操作链接上**不体现为 DOM 的 disabled 类**（读者与管理员渲染出相同 class），链接看起来可点——但点击后由处理器权限检查与服务器 403 双重拦住，无安全洞。安全边界由「按钮缺席 + 处理器检查 + 服务端 403」三层构成，均已分别钉住。
- 验证：`node --check` 通过；对运行中实例实测 13 断言全 PASS；`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`make verify-tidy` 全绿；`prodgates` 仍 PASS；`git diff --check` 干净；清理阶段真实删除了所建角色与用户（admin 会话）。
- `docs` 守卫当场验证了自己的价值：batch16 入列后总检查数变 20，STATUS/dossier 仍写 19 → 守卫精确报出两处漂移并 FAIL；同步四处文档（STATUS、dossier、README、Makefile）与台账 §1 后 PASS。此为该守卫第二次实战捕获（第一次见 §1ap-补二）。
- **边界未变**：仅新增浏览器回归与文档数字同步，未改任何产品代码或生产门禁；allowlist 行为为既有产品设计。

### 1ba-补：复活 e2e 套件二轮稳定性确认与遗留规格弃用标注（round 179）

- **二轮复跑全部通过**：fixture 套件 **8 passed (21.8s)**、`config-versions.spec` **3 passed (9.5s)**（新一轮独立实例）、`api.spec` **1 passed (4.2s)**（全新 DNS-enabled 实例 `/tmp/goddi-run/e2e179/`）—— 复活不是一次性的运气，二轮在全新实例上同样全绿。
- **两个旧 shell UI 规格（`console.spec`/`workflows.spec`）加 STALE 弃用头**：文首注明「pre-TDesign 编写、选择器不再匹配、非 CI 绿色路径」，并逐项列出其覆盖内容的**去向**（路由渲染 → batch2–15；只读角色写拒绝 → rbac_coverage_test.go + batch16；生命周期 CRUD → batch3–8；分页语义 → ipam_pagination_test.go；旧外壳移动端/视觉态随外壳淘汰）。未来开发者不会再对着整页失败猜测原因，也不会误以为这些行为失去了覆盖。
- `node --check` 不适用（TS 类型标注），用 Playwright 同源转换与同语法兄弟规格的实际运行证明文件完好；弃用头为纯 JS 注释，不影响任何解析。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`make verify-tidy` 全绿；`prodgates` 仍 PASS；`git diff --check` 干净；`w13` 实例与 e2e179 实例均停止。
- **边界未变**：仅测试套件标注与稳定性复跑，未改任何产品代码或生产门禁。

### 1ba-补二：`上线前测试报告` W-5 现状注记（round 180）

round 179 的 e2e 收尾后，复核发现该报告的 W-5（「`e2e/` 套件依赖前端依赖与浏览器，本环境不可用」）与 W-6 一样属于**对当前状态的过时断言**。已加「round 179 现状更新」注记：依赖已装齐、chromium 可用、fixture 8 测与 config-versions 3 测针对 TDesign 重写后实测全绿（二轮亦过）、api.spec 实测通过、`w13_frontend_smoke.py` up/down 正常；`console.spec`/`workflows.spec` 仍为 pre-TDesign 选择器（已加 STALE 头并注明覆盖去向）。其余历史报告（`上线前测试报告.md` 的「Playwright E2E 17/17」等）为**当时**快照，无对当前状态的虚假断言，不改写。
- **边界未变**：纯文档注记，未改任何代码或生产门禁。

### 1ba-补三：d1 脚本头部文档同步（round 180）

round 171 改了 `d1_ddns_latency_check.py` 的判据，但脚本文件头的说明仍写旧判据（「撤掉唤醒后确实失败」）。本轮同步：文件头现明确记录两跳变异、本机相位锁定为两段轮询 ≈2.0s、失败线 2.25s 不可满足、以及现行的**时长差判据**（变异后最慢一轮须比基线慢 ≥0.5s）。复跑 → **EXIT=0**（基线 1.001s、变异 2.004s、+1.003s），被变异测试文件 `cmp` 逐字节还原。

### 1ba-补四：`make verify-race` 全量复跑与 W14 文档一致性复核（round 182）

- `make verify-race`（verify 全矩阵 + `go test -race ./...`）→ **VERIFY_RACE_EXIT=0**，race 日志 0 FAIL / 0 DATA RACE。自 round 176 上次全量 race 后，round 177–181 未改任何 Go 文件（仅 e2e 规格、脚本判据文档与本轮核对），race 结果覆盖全部现行 Go 测试。
- 复核 `docs/耐久与容量-w14.md` 对 round 167 重新锚定后的脚本描述：其「4/4 捕获；降 synchronous 到 0 时崩溃演练照样通过（唯一存活项）」的表述在重锚后**仍然成立**（变异只是换到了 `internal/sqlitepolicy/connection.go` 的同一 pragma 块，行为判据不变）；`docs/关键分支覆盖-w14.md` 的 round 168 注记位于文首引言之后、首个标题之前，位置正确。两份文档无需修改。
- 验证：`make verify` 全绿（同一次运行内已含 build/test/vet/gofmt/前端/补丁卫生）；`prodgates` 仍 PASS；`git diff --check` 干净。
- **边界未变**：纯复跑与文档一致性核对，未改任何代码或生产门禁。

### 1aw-补二：按资源隔离读权限 + 多角色并集钉住（round 183，接 §1aw 的第三块独有覆盖）

旧 e2e RBAC 矩阵的第三块独有内容——**按资源隔离**（单资源角色的读只开自己的资源）与**多角色并集**——本轮以服务端回归钉住，新增 `TestScopedRoleGetsResourceIsolationOnReads`（`internal/api/rbac_coverage_test.go`）：
- 经真实 API 建「仅 user:read 的角色 + 仅持该角色的用户」：正向对照 `GET /users = 200`（角色确实持有其读），其余 8 个资源的列表读（dns/dhcp/ipam/settings/backup/tokens/roles/groups）**全部 403**。
- **并集**：第二个用户持 user:read + group:read 两个角色 → `/users` 200（第一个角色仍持有）且 `/groups` 200（第二个角色授予）且 `/dns/zones` 403（从未授予）——多角色并集而非替换。
- 过程中排除一个测试自身陷阱并写进注释：`authedRequest` 接收**完整路径**，漏掉 `/api/v1` 前缀会打到控制台静态处理器（200）而非 API——首轮的「全部 200」是该陷阱的假象，修正后 8 资源隔离与并集全部按预期通过；RBAC 数据侧（`/auth/me` 仅 user:read）始终正确。
- **负向验证**：临时摘掉 `/groups` GET 的 `group:read` 闸门 → 测试精确报「user-scoped leaks: [GET /groups (group) -> 200]」并 FAIL；恢复后 PASS，`router.go` `cmp` 逐字节一致。
- 至此旧 e2e RBAC 矩阵三块独有覆盖全部服务端钉住：只读角色写拒绝（§1aw）、UI 半边（§1aw-补）、按资源隔离+并集（本轮）。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`make verify-tidy` 全绿；`prodgates` 仍 PASS；`git diff --check` 干净。
- **边界未变**：仅新增回归测试，未改任何路由、权限或生产门禁；负向改动已逐字节还原。

### 1aw-补三：RBAC 矩阵第四象限——授予权即生效（round 184）

四象限补齐最后一格：新增 `TestUserWriterRoleCanManageUsers`——**授予 user:read + user:write 的角色真的能管理用户**（writer 经 UI 之外的真实 API 建 `created_by_writer` → 201），同时验证授予**不外溢**：writer 不能删除用户（user:delete 未授予 → 403）、不能读 dns（未授予 → 非 200）。至此四象限：无权读 403（zero-permission）、有权读 200（isolation）、无权写 403（read-only）、**有权写 201（本轮）**。
- **负向验证**：把 POST /users 的闸门从 `user:write` 临时换成 `user:delete`（错误动作）→ writer 的创建被 403，测试精确报「writer POST /users = 403, want 201 (the grant must work)」并 FAIL；恢复后 PASS，`router.go` `cmp` 逐字节一致。（先前「直接摘掉闸门」的负向不会 FAIL——摘闸门只会让更多操作成功，而本测试断言成功；正确负向是把闸门换成错误动作，使被授予者反而被拒。两种负向设计差异已记录。）
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`make verify-tidy` 全绿；`prodgates` 仍 PASS；`git diff --check` 干净。
- **边界未变**：仅新增回归测试，未改任何路由、权限或生产门禁；负向改动已逐字节还原。

### 1aw-补四：`上线前测试报告` W-1 现状注记（round 185）

W-1（覆盖率偏低）与 W-5/W-6 一样属于对当前状态的过时断言，已加「round 185 现状更新」注记：实测（`-coverprofile` 按包聚合的函数平均粗口径）`handler` 2.2→15.2%、`transfer` 4.7→35.6%、`server` 9.4→26.7%、`forwarder` 8.7→34.5%、`zone` 15.2→53.7%、`middleware` →83.8%、`cache` →77.9%；并注明项目口径是**关键分支覆盖优先**（W14 两册 + 29 个变异脚本全绿），不以平均值替代。其余 W 项（W-2 24h 常驻观察、W-3 迁移无 Down、W-4 SSRF 极窄 TOCTOU）仍为**当前有效**的风险记录，不改写。测量命令与口径写进注记，避免误读为语句加权覆盖率。

### 1aw-补五：`优化改进建议` 全表复核注记（round 186）

该评审快照此前只有 R1 有现状注记；本轮**逐项对照当前代码**验证其余 12 项建议的实现状态，并在文首加统一注记：R2（unlock 路由）、R3（连接池+busy_timeout）、R4（DHCP recover）、R5（metrics.Shutdown）、C1（32 分片缓存）、C2（PopularEntries top-N 堆）、C3（命中率 metrics）、U1（Client 复用）、U2（上游私网过滤）、U3（健康判定+摘除）、G1（生产 TLS 门禁）、G2（CSRF 绑定会话）、G3（openapi 门控）、G4（部署文档）——全部已在后续轮次落地，注记逐项给出对应代码位置。`docs` 守卫 PASS。

### 1aw-补六：全量 verify-race 复跑与遗留文档标记扫描（round 187）

- `make verify-race` → **VERIFY_RACE_EXIT=0**，race 日志 0 FAIL / 0 DATA RACE —— 覆盖 round 183–184 新增的两个 RBAC 回归（按资源隔离+并集、writer 象限）与全部现行 Go 测试。
- 全 `docs/*.md` 的「待办/尚未/未完成」标记扫描：剩余命中全部**当前准确**——Technitium 对标表的「真实 Prometheus/promtool 接入未闭合」与 `STATUS.md` §3 外部清单一致（round 113 的「尚未接入应答路径」为**当时**增量日志，文档现行状态节已澄清签名已挂应答路径）；无新增虚假断言。
- 验证：`make verify` 全绿（race 运行内已含）；`prodgates` 仍 PASS；`git diff --check` 干净。
- **边界未变**：纯复跑与扫描核对，未改任何代码或生产门禁。

### 1aw-补七：交接单页全文审计与两处范围引用修正（round 188）

对 `docs/STATUS.md`（主交接单页，79 行）逐节审计：一句话结论、§1 已完成表、§2 候选、§3 外部阻断、§4 一键复现、§5 刻意未做、§6 文档索引——内容全部与当前仓库一致，仅两处**范围引用过时**：① 页首与 §6 的「§1b–§1x 历史增量」——台账增量实际已到 §1aw 补记；② §1 前端行的「16 个浏览器套件」——batch16 入列后为 17。两处已同步，`docs` 守卫 PASS。其余行（207 操作、20 项检查、52 包/575 入口、10 门禁、5 条外部阻断、构建顺序提醒）逐项核对无误。

### 1aw-补八：评审 dossier 全文审计（round 189）

对 `docs/audit/ENTERPRISE-REVIEW-DOSSIER.md`（独立评审输入，92 行）逐节审计：§1 量化（52 包/575 测试入口——`ls -d` 与 `grep -c` 实测**逐字吻合**）；§2 门禁清单（10 哨兵 + 理由 + prodgates 3 组回归名与现行一致）；§3 分组清单；§4 版本轴；§5 可复现验证（唯一过时处：代码块注释「19 项检查」——batch16 入列后为 20，正文已写 20，本轮同步）；§6 七个评审必答问题（授权模型/schema 冻结/兼容矩阵/老库升级/G1–G5/B8.5/生产边界，与 STATUS §3 一致）；§7 作用与限制声明。修正 1 处后守卫 PASS。

### 1aw-补九：TDesign 迁移文档的 30 页清单机械核验（round 190）

对 `docs/tdesign-migration.md` 的可机械验证声明逐项实测：① 「当前 TDesign 页面共 30 个」——文档列出的 30 个页面**全部存在**（`dns/zone-detail` 为动态路由 `[id].vue`，其余为 `index.vue`；29 个非 builtin + `_builtin/login` = 30）；② 「0 处 naive-ui 引用」——`grep -rl naive-ui web-admin/src/views/ web-admin/src/components/` 与全 src 的 import 均 **0 命中**；③ HA 预留表（NodeModeBadge 硬编码 single、`cluster.ts` 仅类型零请求、NAV 集群注释占位不渲染）与 round 149/150 的实测一致。文档无需修改。

### 1aw-补十：`RUNTIME-VALIDATION-LEGACY.md` 全文审计（round 190）

对运行验证文档（90 行）逐节审计：§一 已修复缺陷表（历史记录）；§二 已验证可运行（唯一过时处：迁移数 29 → 现为 32 控制面 + 17 数据面，已加 round 190 注记并链接 round 163 的「migrate 不创建数据面库」边界）；§三 已定位问题 4/5/6/7 的修复记录与代码位置逐项核对（zone_transfer ACL、RENEWING 策略配置项、cache 覆盖、RFC 2136 三层修复）；§四 未完成/阻断与 STATUS §3 一致；§五–八 分离角色/HA/fail-closed/备份加密为 round 124/125/162/165 的补充实跑记录。其余无需修改，`docs` 守卫 PASS。

### 1aw-补十一：`verify-race` 时延测试 flake 定位与 race-aware 松量修复（round 192）

- 本轮全量 `make verify-race` 中 `TestTheTimeFromAckToResolvableIsWithinTheBound` 偶发 FAIL（一轮 3.665s > 2.25s 界）；**重跑即 PASS**（9.3s，~1s 量级）——定位为 race 探针插桩放大调度成本 + 本机负载的计时 flake，非产品缺陷（产品路径的缺陷形态是 d1 变异脚本钉的「撤掉唤醒后时长差 ≥0.5s」，不受此影响）。
- 修复（**仅测试**，build-tag 区分）：新增 `race_enabled.go`/`race_disabled.go`（`raceDetectorEnabled` 常量），`latencySlack` 由 const 改为按构建取值——非 race **250ms 不变**，race 下 **2s**（实测 race 轮次 ~3.7s 上界，正常轮次 ~1s 仍远低于界，非 race 语义与失败线完全不变）。两种构建均实测 PASS；`d1_ddns_latency_check.py` 复跑 EXIT=0（基线 1.003s、变异 2.002s、+0.999s ≥0.5s），被变异测试文件逐字节还原——变异判据为**时长差**而非绝对界，本修复不影响门禁效力。

### 1aw-补十一（续）：修复后全量 verify-race 复跑（round 193）

round 192 的 race-aware 松量修复后，全量 `make verify-race` 复跑 → **VERIFY_RACE_EXIT=0**，0 FAIL / 0 DATA RACE —— 时延测试在 race 插桩下不再偶发越界，flake 收口。

### 1aw-补十二：W 系列脚本全量 sweep 复跑（round 194）

round 192 修改 `ddns_latency_test.go`（race-aware 松量）后，对全部 23 个脚本重新 sweep：**22/23 PASS**；唯一非零退出为 `w13_frontend_smoke.py`（rc=2）——该脚本角色是**实例管理器**，无子命令调用即 exit 2（§1au 已记录的既有行为），带 `up`/`down` 实测 exit 0（本轮再证：up=0、down=0）。变异残留检查 0 命中；工作树修改文件均为历史轮次的**有意未提交**变更；`git diff --check` 干净。

### 1aw-补十三：501 预留扩展契约的行为级钉住（round 195）

501 契约此前只有 OpenAPI 文档层钉住（`TestReservedExtensionsDocumentTheir501Contract`）——路由可以悄悄改接其它 handler 而文档仍承诺 501。本轮新增行为级回归 `TestReservedExtensionEndpointsAnswer501`：经真实路由对 `/sso`(GET/PUT)、`/cluster`(GET/POST)、`/apps`(GET)、`/apps/{id}/install`(POST)、`/dhcp/ha`(GET) 逐个发起请求，断言**全部 501**（变更型请求带 admin 会话 CSRF，否则 CSRF 层先答 401——该陷阱已写进注释）。
- **负向验证**：把 `GetSSOSettings` 临时改接 `response.OK`（200）→ 测试精确报「GET /sso = 200, want 501」并 FAIL；还原后 PASS，`extension.go` `cmp` 逐字节一致。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`prodgates` 仍 PASS；`git diff --check` 干净。
- **边界未变**：501 契约保持不变（v0.9.0 计划要求），本测试正是钉住该不变性；负向改动已逐字节还原。

### 1aw-补十四：三个 auth 行为的 HTTP 级钉住（round 196）

internal/auth 的单元件各有单测，但三条**端到端路由行为**此前只靠旧 e2e（已弃用）覆盖——本轮新增 `internal/api/auth_http_flow_test.go` 三个测试：
- **登录限速 429**：自定义 fixture（`LoginRateLimit=2`）下同用户名+源 IP 两次 401 后第三次 **429**；**并钉住键控粒度**——另一用户名同源 IP 不受影响（per-username 键控）。
- **TOTP 状态机全流程**：setup 发 secret → 错误码 400 不启用 → 正确码 200 且带 recovery_codes → 启用态下 re-setup/re-verify 均 **409** → disable 错误密码 **401** → 正确密码 200 → 未启用再 disable **400** → 禁用后可重新 enroll。
- **会话吊销精确性**：第二次登录产生新会话；吊销它后其 token `/auth/me` = **401**、吊销者会话仍 **200**、列表中该会话消失——吊销只杀死目标会话。
- **负向验证**：临时绕过 TOTP disable 的密码校验 → 测试精确报「disable(wrong password) = 200, want 401」并 FAIL；还原后 PASS，`auth.go` `cmp` 逐字节一致。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`prodgates` 仍 PASS；`git diff --check` 干净。
- **边界未变**：仅新增回归测试，未改任何产品代码或生产门禁；负向改动已逐字节还原。

### 1aw-补十五：must_change_password 禁闭的 HTTP 级钉住（round 197）

confinement 中间件（会话欠密码修改时只放行账户安全端点）此前**没有任何 HTTP 级测试**。新增 `TestMustChangePasswordConfinementLiftsAfterTheChange`：建 `must_change_password=true` 的无角色用户 → 登录成功（欠改密不是登录失败）→ `/auth/me` 200（账户安全放行）→ `/users` 403 且带专属 code **`password_change_required`**（与 RBAC 拒绝可区分）→ 改密成功后**立即解禁**（同一端点变为 RBAC 数字码拒绝、不再带禁闭码）、当前会话存活、旧密码登录 401 / 新密码登录 200。禁闭与 RBAC 两种 403 通过响应 code 字段区分是本测试的关键设计。
- **负向验证**：临时禁用中间件的禁闭分支 → 测试精确报「confined GET /users did not carry the confinement code」并 FAIL；还原后 PASS，`middleware.go` `cmp` 逐字节一致。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`prodgates` 仍 PASS；`git diff --check` 干净。
- **边界未变**：仅新增回归测试，未改任何产品代码或生产门禁；负向改动已逐字节还原。

### 1aw-补十六：禁用账户即时失效的 HTTP 级钉住（round 198）

中间件注释自陈的关键设计——「会话存活数日，禁用账户必须在**下一次请求**即生效（每次请求重读账户行），否则禁用不生效」——此前无任何测试。新增 `TestDisablingAUserKillsTheirLiveSessionsImmediately`：建用户 → 登录且会话工作 → admin 禁用 → **存活会话下一次请求即 401**、新登录被拒（统一错误文案，不区分禁用/密码错——该防枚举设计一并被 401 断言隐式钉住）→ 重新启用后**旧会话复活**（账户行重读而非删除——文档语义，按文档钉住）。
- **负向验证**：临时绕过中间件的 `UserEnabled` 每请求检查 → 测试精确报「victim session after disable = 200, want 401」并 FAIL；还原后 PASS，`middleware.go` `cmp` 逐字节一致。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`prodgates` 仍 PASS；`git diff --check` 干净。
- **边界未变**：仅新增回归测试，未改任何产品代码或生产门禁；负向改动已逐字节还原。

### 1aw-补十七：`/auth/refresh` 安全流程的 HTTP 级钉住（round 199）

RefreshToken 的三个安全性质此前无 HTTP 级测试（仅 auth_coverage 提及 401 边界）。新增 `TestAuthRefreshFlowIssuesAWorkingPairAndHonorsRevocation`：
- 有效 refresh **重签发可用的 JWT+CSRF 对**（对上执行变更操作 → 201，证明对可用于变更）；
- **SEC-05**：吊销会话后（refresh 用户自吊销自己的会话），原 token 的 refresh → **401**——会话已不存在；
- **禁用账户** refresh → **401**（refresh 也重读账户行——与中间件每请求重读同源的设计）；
- 无 token → 401。
- 过程中的两个测试设计发现已写进注释：① refresh 重签发的 JWT/CSRF **字符串可等同**（JWT iat 秒级粒度 + CSRF 是绑定会话的确定性 HMAC——**设计使然**），测试断言的是功能性质（重签发的对可用于变更）而非字符串差异；② `/auth/sessions` 只列**调用者自己**的会话——自吊销必须由 refresh 用户本人执行，不能用 admin 的列表（首个版本因此误删 admin 会话）。
- **负向验证**：临时绕过 SEC-05 会话存在性检查 → 精确报「refresh after session revocation = 200, want 401 (SEC-05)」并 FAIL；还原后 PASS，`auth.go` `cmp` 逐字节一致。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`prodgates` 仍 PASS；`git diff --check` 干净。
- **边界未变**：仅新增回归测试，未改任何产品代码或生产门禁；负向改动已逐字节还原。

### 1aw-补十八：启用 TOTP 后登录强制与恢复码单次使用的 HTTP 级钉住（round 200）

登录路径的 TOTP 强制此前无 HTTP 级测试。新增 `TestLoginRequiresTOTPAfterEnablingAndRecoveryCodesAreSingleUse`：启用后 **无 code 登录 → 401**（且计入限速计数——防「猜中密码者无限重试」，代码注释自陈）、**错码 → 401**、**正确 TOTP 码 → 200**、**恢复码可用且严格单次使用**（同一恢复码第二次 → 401，下一个恢复码仍可用）。补全了 §1aw-补十四~十七 之后的最后一块 auth 行为。
- **负向验证**：临时跳过登录路径的 TOTP 强制分支 → 精确报「login without TOTP code = 200, want 401」并 FAIL；还原后 PASS，`auth.go` `cmp` 逐字节一致。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`prodgates` 仍 PASS；`git diff --check` 干净。
- **边界未变**：仅新增回归测试，未改任何产品代码或生产门禁；负向改动已逐字节还原。

### 1aw-补十九：auth 行为钉住的 race 复跑（round 201）

round 196–200 新增的 7 个 auth HTTP 流程测试（限速 429、TOTP 状态机、会话吊销、禁闭、禁用失效、refresh、登录强制+恢复码单次使用）经 `-race` 全包复跑通过（50.1s，0 race/0 fail）——计时敏感测试在这些新增路径上无抖动。DNSSEC 侧复核：密码学核心由 `internal/dns/dnssec/` 单测覆盖（denial/nsec3/responder）、UI 由 batch15 覆盖、HTTP handler 为薄包装——无需再建 HTTP 生命周期钉（router-only fixture 下 DNS 容器为 nil，强行建会引入容器初始化的复杂度而无新增保障）。

### 1aw-补二十：第三轮 e2e 稳定性 + 全量 verify-race（round 202）

- e2e 第三轮稳定性确认：fixture 套件 **8 passed (23.3s)**、`config-versions.spec` **3 passed (11.2s)**（全新 w13 实例，up/down 干净）——复活套件三轮全绿。
- 全量 `make verify-race` → **EXIT=0**，0 FAIL / 0 DATA RACE（含 round 196–200 全部新增 auth 流程测试与既有全量）。
- 验证：`git diff --check` 干净；无残留进程。
- **边界未变**：纯复跑，未改任何代码或生产门禁。

### 1av-补：STALE 头的覆盖映射精确化（round 203）

复核 §1av 建立的 STALE 弃用头的覆盖映射，发现两处**过度承诺**并已精确化（console.spec 与 workflows.spec 两个文件的头部同步修正）：① 「option lifecycles → batch3–8」——batch4 对 `/dhcp/options` 仅**渲染检查**，无创建 e2e；其生命周期实际由 `internal/dhcp/option` 的 **11 个 Go 测试**钉住（CRUD/分层优先级/预留绑定/wire 编码）；② backup——batch4 同样仅渲染检查，备份/恢复生命周期由 `api.spec`（DNS 备份恢复往返）+ `release_flow_test.go` 覆盖。其余映射（zones/forwarders/scopes/groups/users/roles/tokens/reservations/security policies/allow rules 均有 batch「created end-to-end」步骤）逐项实测吻合。历史台账 §1av 原文保留，本条为修正记录。

### 1aw-补二十一：锁定→解锁闭环的 HTTP 级钉住（round 204）

R2 修复（登录锁定 + `POST /auth/unlock`）此前只有 rate limiter 单测——「锁死 → admin 解锁 → 可再登录」的**端到端闭环**无测试。新增 `TestAdminUnlockReleasesALockedAccount`（`LoginRateLimit=2` fixture）：2 次尝试后第 3 次 **429**（锁死）→ admin `POST /auth/unlock` **200** → 解锁后同用户名登录为 **401**（限速器已重置，仅因用户不存在）——并反向再证：**重新锁 2 次后第 3 次又 429**（解锁确被消费、限速器继续计数）。非 admin 调 unlock 的拒绝由 RBAC 矩阵钉住（user:write 门控）。
- **负向验证**：把 `ResetUserAttempts` 临时改成 no-op → 测试 FAIL（解锁后仍 429）；还原后 PASS，`rate_limiter.go` `cmp` 逐字节一致。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`prodgates` 仍 PASS；`git diff --check` 干净。
- **边界未变**：仅新增回归测试，未改任何产品代码或生产门禁；负向改动已逐字节还原。

### 1aw-补二十二：API 令牌吊销即时生效的 HTTP 级钉住（round 205）

api.spec 覆盖令牌创建/scope/单次使用，但**吊销闭环**无仓内 HTTP 级测试（旧 e2e 的「copy and revocation」已弃用）。新增 `TestAPITokenRevocationKillsTheTokenImmediately`：创建 dns:read scope 令牌 → 通过鉴权与 dns:read 门（router-only fixture 下 DNS handler 因 nil 容器 500——**越过 RBAC 即证明授权成立**；200 由 api.spec 对完整实例证明）→ 越界 `/users` **403** → admin `DELETE /tokens/{id}` → **原令牌立即 401**。
- **负向验证**：把 `RevokeToken` 临时改成 no-op → 「revoked token = 500, want 401」FAIL（吊销无效则令牌仍越过 RBAC 到达 handler）；还原后 PASS，`token.go` `cmp` 逐字节一致。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`prodgates` 仍 PASS；`git diff --check` 干净。
- **边界未变**：仅新增回归测试，未改任何产品代码或生产门禁；负向改动已逐字节还原。

### 1av-补二：batch4 补齐 option 与 backup 创建 e2e（round 205-206）

round 203 发现 `/dhcp/options` 与 `/settings/backup` 在浏览器侧仅渲染检查。本轮 batch4 补齐两个创建 e2e（含自清理，沿用该套件的唯一后缀+cleanup 治理）：
- **option created end-to-end**：对话框填 code/value + **必选 作用域 select**（首轮因漏选 scope 而失败——探测发现对话框为 code/value/优先级/作用域四项）→ 断言 body 含唯一 value → API 清理；
- **backup created end-to-end**：创建按钮**无对话框**直接以自动名 `manual-<时间戳>` 创建（前端 handleCreate 直调 createBackup）→ 断言列表出现 → 快照差分清理（仅删本轮新增行；列表行字段是 `description` 而非 `name`——batch5 会话裸数组之后的又一个响应形状陷阱）。
- 全部 17 步 PASS（0 error）。`test/browser/README.md` 同步。STALE 头映射（round 203 精确化版）随之更新为「option/backup 创建 e2e 已由 batch4 覆盖」。

### 1av-补三：round 196–205 新增测试 race 复跑 + 浏览器 e2e 覆盖总览（round 207）

- round 196–205 新增的 10 个 auth/token HTTP 流程测试经 `-race` 全包复跑通过（52.2s，0 race/0 fail）。
- 浏览器「创建 e2e」步骤总览：跨 8 个套件共 **11 个 created-end-to-end 步骤**（batch3 安全策略、batch4 预约/选项/备份、batch5 作用域组/允许规则、batch6 用户/角色、batch7 转发器、batch8 区域、batch10/12 各 1）——round 206 补齐 option/backup 后**无仅渲染的生命周期页**；`docs/STATUS.md` §1 前端行同步。
- 验证：`go test ./...` / `go build` / `go vet` / `gofmt -l` 全绿；`make verify` 全绿；`prodgates` 仍 PASS；`git diff --check` 干净。
- **边界未变**：纯复跑与文档同步，未改任何代码或生产门禁。

### 1av-补四：bare 配置排除两个 STALE 规格（round 208——CI 一致性修正）

发现 `e2e/playwright.config.ts`（bare 套件，即 CI「Browser and API regression」job 与开发者裸跑 `npx playwright test` 的入口）的 `testIgnore` 只排除了 fixture 规格——**两个 STALE 规格（console/workflows，pre-TDesign 选择器）仍会被 CI 拉起并对迁移后的 UI 整页失败**，与其头部「不跑在 CI 绿色路径」的声明矛盾（ci.yml 早于 TDesign 迁移编写，迁移后无人能在本沙箱观察到 CI 结果）。修正：bare 配置的 `testIgnore` 同时排除两个 STALE 规格（注释指向其头部与 §1av/§1aw 映射）。`npx playwright test --list` 现为 **4 tests in 2 files**（api.spec + config-versions），并对全新 plain 实例（18090/15362）实测 **4 passed (12.5s)**——CI bare job 的两个前提（api.spec 的 dig 在无 `GODDI_TEST_DNS_PORT` 时自动跳过 ✓、config-versions 自建数据 ✓）均成立。

### 1av-补五：CI 工作流与 e2e 前提的一致性复核（round 209）

round 208 修正后，对 `.github/workflows/ci.yml` 两个 Playwright job 的全部环境前提逐项复核：① bare job（DNS disabled）——api.spec 的 dig 段在无 `GODDI_TEST_DNS_PORT` 时**自动跳过**（`if (process.env.GODDI_TEST_DNS_PORT)` 守卫）、console/workflows 已排除、config-versions 自建数据 ✓；② fixture job——`GODDI_SMOKE_PORT=16092` 为 `w13_frontend_smoke.py` 支持的变量（line 49）✓、种子凭据 admin/`Admin@123456` 与 fixture 规格默认一致 ✓、fixture 配置的 `baseURL` 读 `GODDI_TEST_BASE_URL` ✓、`up`/`down` 与失败时打印实例日志的处理 ✓。CI 与本仓测试资产现已一致；本沙箱无法执行 GitHub Actions，以上为**配置级**一致性证明（bare 集合 4/4 已对等价本地实例实测）。

### 1aw-补二十三：`P0-G1.md` 审计（round 210）

最后一份未审计的引用文档（74 行）逐节审计：头部指向 dossier 的量化（569 → **575**，round 190 实测值，已修正）；基线与范围声明准确；「已实现的局部能力」逐包描述与边界警示（候选/夹具/未接入/不证明什么）与台账各轮一致；「验证及未关闭事项」的 Go 1.27.1 与当前环境实测一致（go1.27.1 darwin/arm64）；G1 未完成清单与 STATUS §3 外部阻断一致；实施进度总表的 10 行状态准确。修正 1 处后守卫 PASS。至此 `docs/audit/` 全部引用文档均经审计。

### 1aw-补二十四：`contracts/version-compatibility.md` 六轴逐项核验（round 211）

对版本合同文档的六轴表逐项对照代码实测：① `eventlog.SchemaVersionV1=1`（eventlog.go:31）✓；② `snapshotverify.EncodingNDJSONV1=1` ✓；③ 快照行 schema（`SnapshotManifest.schema_version`/`Limits.Schema`）✓；④ proto `package goddi.replica.v1` ✓；⑤ `eventlogguard.Version` 枚举 Legacy/Hardened ✓；⑥ `ipamstore.MigrationPaths()` 001+002 ✓。且 `compat` 包测试钉住常量相等与「同为 1 互不替代」（`Controls` 两两不同）。「尚未完成」两条（候选矩阵未冻结、旧快照迁移路径未冻结）与 STATUS §2/§3 一致。文档零修改——六轴与代码完全一致。

### 1aw-补二十五：DHCP 租约提案文档只读审计（round 212）

`contracts/enterprise-dhcp-lease.proposal.md`（冻结前提案，明令不写码）只读核验：状态行仍为「PROPOSAL（待评审冻结）」；§6 开放问题恰为 **6 项**（地址表示/family、状态枚举映射、永久租约表示与导入不重算、部分唯一索引与删除守卫、是否并入八表快照、总体快照版本号）——与 STATUS §2「6 个开放决策项未定」一致；「冻结前不实现 `internal/enterprise/dhcplease` 代码」的禁令仍被遵守（`internal/enterprise/` 下无 dhcplease 包）；`snapshotstage` 公开导入/安装入口仍由 `prodgates` 钉住恒拒绝。文档零修改。

### 1aw-补二十六：W-1 handler 覆盖率口径修正（round 213）

§1aw-补四 实测的 `handler` 15.2% 存在**跨包归一盲区**：`go test -coverprofile ./internal/api/...` 只把归属到被测包自身的执行计入，经 `internal/api`（release_flow/rbac_coverage/auth_http_flow 等）驱动 handler 的执行**不计入** handler 的覆盖。用 `-coverpkg=./internal/api/handler -count=1` 合并全部测试二进制后，handler 的语句覆盖为 **30.1%**（275 个函数按函数平均 31.5%）。已在 W-1 注记中原位修正并注明口径；其余包不受该盲区影响（其测试就在本包内）。教训：多包 coverage 的归一盲区会系统性低估被其它包测试驱动的包。

### 1aw-补二十七：Enterprise 覆盖率并集口径复测（round 214）

round 213 的归一盲区教训同样适用于 P0-G1 的「各包语句覆盖均 ≥70%」。用 `-coverpkg=./internal/enterprise/... -count=1` 合并 52 个测试二进制并**按块取并集**（合并 profile 中每块出现 52 次，须去重取 max——round 214 排查中两个解析陷阱已修正）后：**总语句覆盖 86.1%**，唯一低于 70% 的包是 `replicaschema`（67.8%，单包口径 70.4%——差值来自其它包测试不驱动它），`ipamstore` 75.8%、`dhcpholds` 75.5%。已在 P0-G1 原位修正口径与例外；其未覆盖行为防御性分支的既有核查不变。

### 1aw-补二十八：P0-G1 失效引用修正（round 216）

round 210 审计 P0-G1 时漏掉一处失效引用：`tools/check_contracts.py` 在仓库中不存在（`find`/`git log --all` 均无——从未提交或已移除）。P0-G1 的合同检查实由 `test/browser/contracts.cjs` 承担（`make verify-browser` 静态守卫），引用已修正并注明失效原因。

### 1aw-补二十九：全文档死路径引用扫描（round 216）

机械扫描当前六份交接文档中**带目录前缀**的 271 个路径引用（裸文件名/历史文档/外部仓库路径无法机械验证，排除）。初报 60 个「未命中」，逐一归类后**全部为三类非问题**：① `tdesign-migration.md` 的路径按**惯例相对 `web-admin/`**（如 `views/admin/users/index.vue` → `web-admin/src/views/...`，实测存在）；② `IMPLEMENTATION-STATUS.md` 的 4 个仓库根路径（`internal/enterprise/eventlogguard/README.md`、`internal/dataplane/store.go` 等）**实测全部存在**；③ `test/browser/NO-SUCH-FILE.md` 是 docs 守卫负向验证的**故意不存在的注入物**（台账在案的测试记录）。**唯一真实死引用即 `tools/check_contracts.py`（round 216 已修正）**。扫描脚本与方法记录在案；`docs` 守卫 PASS。

### 1av-补六：batch3 补齐系统设置保存 e2e（round 217）

最后一个仅渲染的写路径页——`/settings/system`（batch3 只数保存按钮）——补齐 e2e：`dns_cache_min_ttl`（int、可安全微调、缓存自钳制）经 **UI 输入框 + 行内保存按钮** 修改（67→95）→ API 复读确认持久 → 经同一 UI 路径还原（回 67）。至此全部写路径页均有浏览器创建/保存 e2e。`test/browser/README.md` 同步。

### 1aw-补三十：全量 verify-race 复跑（round 218）

全量 `make verify-race` → **EXIT=0**，0 FAIL / 0 DATA RACE —— 覆盖 round 205–217 的全部新增（auth/token 流程、501 行为钉、分页/隔离钉、batch3/4 浏览器扩展不改 Go）与既有全量测试。

### 1aw-补三十一：v0.9.0 方案复核文档的状态标记审计（round 219）

`docs/实施计划-v0.9.0-企业级方案复核与生产收敛.md` 的「四条主线仍未完成」（默认 DHCP 热路径迁移、WAL durable ACK 接管、DHCP-DNS-IPAM 统一事实链、生产级多节点+外部验收）**逐条与当前状态吻合**：每条都有对应的边界守卫或 501 契约钉住（production_boundary_test/wal_lifecycle_boundary_test/`TestReservedExtensionEndpointsAnswer501`）；§2.1 implemented 清单 9 项与既有验证一致；§2.2 experimental/incomplete 与 STATUS §2 候选表一致。文档零修改——其状态标记在迁移后仍当前准确。

### 1aw-补三十二：Technitium 对标表修正一处过时差距（round 220）

对标表「Zone History」行称 GoDDI「无查询 API/页面」——**已过时**：`GET /dns/zones/{id}/history` 已存在（OpenAPI 已登记、`GetZoneHistory` handler、zone-detail 页历史表）。已在该行原位修正并注明快照时点。其余抽查（Prometheus 未闭合=外部、per-zone 权限=仅全局 RBAC、DNSSEC 行的批次 F 收敛注记）均准确。

### 1aw-补三十三：Technitium 对标表再修正两行（round 221）

续 round 220 的抽查，发现对标表另有两行过时并已修正：①「启用/禁用 Zone ❌」——`POST /dns/zones/{id}/enable|disable` 已存在（OpenAPI 已登记）；②「per-zone 权限 ❌ 仅全局 RBAC」——`GET/PUT /dns/zones/{id}/permissions` + zone-detail 页权限编辑已存在，路由中间件 `RequireZonePermission` 按 View/Modify/Delete 分级。两行差距等级同步降为「低」。其余行（Zone 类型无 Catalog、记录类型 28 种、Import/Export 基础版）抽查准确。

### 1aw-补三十四：Technitium 对标表第三批修正四行（round 222）

续 round 220/221 的抽查，系统核对剩余 ❌/⚠️ 行并修正四行：① expiryTtl——`expiry_ttl` 已在 API 与记录表单暴露；② Dashboard Zones 行——启用/禁用、per-zone 权限、Zone History 已上线（round 220/221 修正），仅 Zone Options 完整面板保留「部分」；③ Allowed/Blocked——导入/导出/清空/flush 均已有（batch5/7 e2e），保留「布局不按 T 拆分」差异；④ Apps——更新为「501 预留契约（有意保留）」。其余 ❌/⚠️ 行核对准确（stats Top-N 缺失、record types 28 种等）。

### 1aw-补三十五：Technitium 对标表第四批修正三行（round 223）

§3.4 认证/权限/集群表修正三行：① **API Token**——single-use 已实现（`is_single_use`、第二次使用 401 由 api.spec 实测、batch4 实测一次性值展示），原「缺 single-use」过时；② **集群**——`/cluster` 为 501 预留契约而非「成熟度待核实」；③ **SSO OIDC**——原标「✅ /sso」不准确，实为 501 预留契约（两者均由 `TestReservedExtensionEndpointsAnswer501` 钉住，v0.9.0 计划有意保留）。三行修正后 §3.4 无过时项。

### 1aw-补三十六：对标方案批次 A/D/E 加状态标记（round 224）

对标方案的批次表无状态标记——读者无法得知哪些已落地。实测后加三处头部状态：**批次 A** 部分（A1/A3/A4/A5 已实现，A2/A6 部分）；**批次 D 全部已实现**（`upstream_encrypted.go`：DoT tcp-tls、DoH RFC 8484、DoQ quic-go/RFC 9250 + Priority 排序）；**批次 E 未实现**（无 Catalog zone 类型，与 §3.2 一致）。批次 F 已有增量注记。原方案文字保留。

### 1aw-补三十七：对标方案批次 B/C 状态标记（round 225）

补齐批次 B（统计对标——未实现，stats 仍缺时间范围参数与 Top 榜）与批次 C（菜单拆分/Apps 页/DNS Client 增强——未实现，与 501 契约及 §3.2/§3.3 行一致）的状态标记。至此批次 A–F 全部有实测状态，对标方案文档现状完整。

### 1aw-补三十八：soybean 迁移方案加「已被取代」注记（round 226）

`docs/soybean-admin-migration-plan.md` 描述的 soybean-admin/Naive UI 方案未被执行（实际落地为 TDesign Starter）。文首加「已被取代」注记并指向 `docs/tdesign-migration.md`，明确其中文件路径与组件引用不反映当前代码——防止读者按旧方案动手。

### 1aw-补三十九：v1.2 方案复核文档发布声明核验（round 227）

`docs/GoDDI-v1.2-方案复核与实施计划.md` 的「发布后状态」核验：v0.23.0/v0.24.0/v0.24.1 标签**均存在**（`git tag -l | sort -V`；初查被字母排序误导）、HEAD 即 PR #5 合并提交、PR #3 更新过 v0.23 发布状态、`docs/release-notes-v0.20.0–v0.22.0.md` 在案（v0.23 校验和产物在工作流产物而非 docs/，与原文「发布工作流…产物均成功」一致）。文档声明准确，零修改。

### 1aw-补四十：STATUS 后端行同步（round 228）

`docs/STATUS.md` §1 后端 API 行补记 round 196–205 的行为级回归钉（RBAC 四象限/隔离+并集/只读全拒/501 行为/分页跨页/auth 全流程九项）。过程中触发 docs 守卫的路径校验（裸文件名 `rbac_coverage_test.go`/`ipam_pagination_test.go` 无法解析——守卫要求目录限定路径），改用完整路径后 PASS。守卫的路径校验在此再次证明了其价值。

### 1av-补七：`make verify-e2e` 本地一键入口（round 229）

CI 的两个 Playwright job 此前只能逐个手写命令复现。新增 `scripts/e2e_local.sh` + `make verify-e2e`：一条命令跑完 **bare 套件**（临时 plain 实例 18090、DNS 15362、shipped config + `GODDI_*` env 覆盖——与 CI 同模式）与 **fixture 套件**（`GODDI_SMOKE_PORT=16092` 的 w13 种子实例），并如实汇总退出码。实测 **bare 4 passed + fixture 8 passed**。
- 排查中修正三处脚本自身问题：① 生成的最小 YAML 键不合法（改用 shipped config + env 覆盖）；② DNS 监听 env 名（`GODDI_DNS_LISTEN_ADDRESS` 不存在，正确为 `GODDI_DNS_LISTENERS_UDP_ADDR`/`_TCP_ADDR`——CONFIG-REFERENCE §6 表）；③ 状态累加 bug（fixture 块重置 status 吞掉 bare 失败——已改为取更差者）。e2e 节点缺失时以明确报错退出（exit 2）。
- `Makefile` 加 target 与帮助行；STATUS §4 一键复现清单更新。

### 1av-补八：test/browser/README 补 Playwright e2e 一节（round 230）

`test/browser/README.md` 新增「Playwright e2e suites」节：`make verify-e2e` 一条命令复现 CI 两个 Playwright job（bare 18090+15362 / 种子 fixture 16092）、前置 `cd e2e && npm ci`、以及两个 STALE 规格的排除与处置指针。

### 1av-补九：verify-e2e 临时目录清理（round 231）

`e2e_local.sh` 加退出清理：全部通过时删除 plain 实例临时目录，失败时保留（serve.log 是最有用的排障产物）。实测通过后自清理；早前失败轮次遗留的两个目录已手工清除。

### 1aw-补四十一：STATUS 文档行同步（round 232）

`docs/STATUS.md` §1 文档/评审材料行补记 round 210–216 的文档审计完成（10 份引用/历史文档逐一审计、量化声明实测、失效引用与过时差距行修正、取代注记、死路径扫描清零）。

### 1.2 台账结构说明（round 233）

台账现 927+ 行 / 57 个标题节。§1.1 为全部增量节的**导航索引**（每节含该轮实测与证据，按主题聚类：1b–1e 运行期、1f–1z 前端与契约守卫、1aa–1at 验证脚本与运行期复核、1av–1ba e2e 复活、1aw 系列 RBAC/auth 行为钉、1aw-补 系列 CI/覆盖口径/文档审计收口）。

### 1aw-补四十二：run.sh 头部注释的套件数修正（round 234）

`test/browser/run.sh` 头部注释仍写「15 browser regression suites」——batch16 入列后实际为 **20**（3 静态守卫 + 17 Chromium）。已修正；grep 确认 STATUS/README/台账无同类残留。

### 1aw-补四十三：runbook 检查数与守卫自更新（round 235）

B8.5 runbook §11 仍写「16 个 Chromium + 3 静态 = **19 项检查**」（batch16 入列后为 20）——且 **docs.cjs 守卫硬编码检查字符串「19 项检查」**，导致该漂移两次漏检（STATUS/dossier 已更新为 20，runbook 漏更而守卫仍绿）。双重修正：① runbook 更新为「17 个真实 Chromium 套件（含 batch16）+ 3 个静态守卫 = **20 项检查**」并加 `verify-e2e` 入口；② **守卫改为自更新**——从 `run.sh` 的默认套件列表解析实际数量，要求 runbook 陈述该数字——套件增删时守卫自动跟踪，永不过期。

### 1aw-补四十四：docs.cjs 页面数断言自更新化（round 236）

round 235 的守卫自更新化扩展到页面数：`'30 个 TDesign 页面'` 的硬编码 needle 改为**从 `web-admin/src/views` 递归数 .vue 文件**派生（每文件一路由 + login；排除 `_builtin`）。新增页面时 needle 自动变为新值，STATUS 必须同步——守卫永不过期。实测派生值 = 30 与 STATUS 现值吻合。其余 needle（API 操作数、enterprise 包/测试数、browserChecks）此前已是派生式。

### 1aw-补四十五：docs.cjs 哨兵数断言自更新化（round 237）

最后一个硬编码 needle——`'10 个 ErrNoAuthority 哨兵'`——改为**从代码派生**：数 `internal/enterprise` 下定义 `ErrNoAuthority = errors.New` 的非测试包数。实测派生值 = 10 与 STATUS 吻合；新增哨兵包时 needle 自动更新。至此 docs.cjs 全部 6 个 needle 均为派生式（页面数/API 操作数/enterprise 包数/测试数/哨兵数/browserChecks），无硬编码计数残留。

### 1aw-补四十六：dossier 守卫 needle 派生化（round 238）

dossier 守卫的两处硬编码 needle 派生化：①「可复现验证入口」从 Makefile 解析 6 个验证 target（verify/verify-race/verify-browser/verify-e2e/verify-tidy/build）——round 229 加 verify-e2e 后原硬编码「5」漏检了 dossier 的「5」→ 实际 6；② 门禁数 needle 用 `authoritySentinels()` 派生替代硬编码「10」。dossier §1 量化表与 §5 命令块同步加 verify-e2e。负向模拟确认：移除 verify-e2e target → 派生值 5 ≠ dossier 6 → 守卫 FAIL。

### 1aw-补四十七：release 工作流一致性复核（round 239）

`.github/workflows/release.yml` 与当前构建流程一致：pnpm frozen-lockfile install + build → **web/dist 同步**（正是 docs 守卫要求的顺序）→ `go test ./...` → amd64/arm64 交叉编译 → `release_smoke.sh`（round 155–156 已对产物实测通过，`bash -n` 现复核语法）。无需修改。

### 1aw-补四十八：ADR 目录审计（round 240）

`docs/adr/`（9 份，0001–0009）审计：状态行准确且与 STATUS §2/§3 一致（0001/0002/0003/0006/0008 已接受；0004「分阶段契约继续收敛，默认生产实现未切换」、0005「设计完成，生产多节点协调未实现」、0007「迁移期协议契约已建立，生产事实源尚未统一」、0009「协议与代码路径已实现；部署验收未完成」——边界表述与候选/预留口径一致）；引用的 **13 个 Go 文件与全部 migrations/*.sql 实测存在，零缺失**。无需修改。

### 1aw-补四十九：contracts 目录只读审计（round 241）

`contracts/` 剩余文档只读核验：`replication-generation.md` 钉住的 `replication.proto` SHA-256（`96c8fd05…`）**三重验证一致**——实际文件哈希、文档声明、`internal/gen/replica/v1/descriptor_test.go` 的代码级钉；工具版本（protoc 36.2 / protoc-gen-go v1.36.11 / grpc v1.80.0 与 goose 依赖关系说明）与 go.mod 一致口径。`enterprise-ipam-occupancy-v1.md`、`enterprise-receipt-v1.md`、`enterprise-static-audit-v1.md`、`enterprise-takeover-permit-v1.md`、`enterprise-ipam-rowid-002.md` 均为候选 schema 合同（对应 `internal/enterprise` 包已实测）。零修改。

### 1aw-补五十：W/b/d 系列脚本全量 sweep 复跑（round 242）

round 194 之后的第二次全量 sweep（24 个脚本：w 系列 + d1 + b 系列，排除参数驱动的 `w13_frontend_smoke.py` 实例管理器）→ **24/24 PASS**，变异残留 0 命中，`git diff --check` 干净——round 205–241 的全部改动（文档、测试、守卫、编排脚本）未引入任何脚本漂移。

（补：上条 grep 命中的 3 处均为 `scripts/b5_revert_check.py` 自身的变异定义字符串——脚本先应用变异、跑测试、再逐字节还原，非产品代码残留；`internal/`/`cmd/`/`web-admin/src/` 的产品树 grep **0 命中**。工作树的 `M` 文件为历史轮次的有意未提交变更，24 个脚本全部逐字节还原验证通过。）

### 1aw-补五十一：未提交工作清单（round 243）

`git status` 未跟踪文件 140 个——按用户既定口径（保留未提交工作）为本会话全部产出的预期状态，分类实测：`web/` 80 个（部署产物 = 构建后的前端，服务端从 ./web/dist 提供）、`web-admin/` 6 个（TDesign 新增源文件：NodeModeBadge/RemoteTable/tdesign-layout/plugins/cluster.ts）、`internal/` 28 测试 + 13 代码（auth/RBAC/CSRF/分页/501 等行为钉与 501 处理器）、`cmd/` 3 测试（迁移/edition 门）、`migrations/030–032` 3 个（控制面 bump 迁移）、`docs/` 4、`contracts/` 1、`test/browser/` 1、`scripts/` 1、`e2e/` 若干。修改的已跟踪文件（sync.go/server.go/main.go/config/openapi.go/Makefile/middleware 等）为历史轮次的有意变更。两者均非生产提交、不影响门禁。

### 1aw-补五十二：构建顺序守卫再次实战捕获（round 243）

round 243 的顺序（先跑浏览器回归、后跑 `make verify`）触发了 **docs 守卫的部署产物一致性检查**：`make verify` 重建了 `web-admin/dist`（新哈希）而未同步 `web/dist` → 守卫精确报出全部资产漂移并 FAIL。按 STATUS §4 的既定顺序重新同步后 PASS。此为该守卫第三次实战捕获（前两次见 §1ap-补二、§1aw-补四十二前的 batch16 轮）——顺序要求不是形式，守卫确实拦住了「验证过的树与部署树不一致」。

### 1aw-补五十三：文档既定顺序的全矩阵通过（round 244）

按 STATUS §4 的**文档既定顺序**一次跑通全矩阵：`make verify`（build/test/vet/gofmt/前端/补丁卫生）→ **web/dist 同步** → 重建二进制 + 起隔离实例 → `make verify-browser`（20 项 TOTAL_FAILS=0）→ 实例停止。无顺序违规、无守卫漂移。该顺序即为向评审人推荐的复现路径。

### 1aw-补五十四：沙箱内工作收口确认（round 245）

round 241–245 的复核确认：**沙箱内可完成的工作已全部完成并复跑**——全量验证矩阵（verify/race/browser/e2e/W 系列脚本）全绿、10+ 份文档全部审计或注记、auth/RBAC/分页/501 行为全部钉住、CI/release/e2e 编排一致、守卫全部派生式自更新。STATUS §3 / 台账 §1g 的 5 类剩余项**全部需要沙箱之外输入**（真实双主机与断电/分区硬件、独立专家评审冻结、本机 role/epoch/fencing 授权、真实遗留库、Prometheus 原始时间线），该条件自 round 24 起持续不变。目标转入 blocked 状态：等待上述外部输入。

## 1ca. 容器化双节点 HA 演练（round 246–247；用户指令：2 个 Docker 模拟双机）

**定位（诚实边界）**：这是介于「单机双进程」（§1c）与「真实双主机」（§1g 外部）之间的**新证据层**——两个容器各有独立网络命名空间（独立 IP/loopback）、独立数据卷、独立生命周期，镜像链路为真实跨容器 TCP。**不是 B8.5 验收**：同宿主内核与磁盘、无物理断电、无 fencing/witness 信任域、无生产级 DHCP 广播拓扑。

**编排**：`docker-compose.ha.yml`（新增）——`goddi-primary`（role=primary，18081→6080，15354→53/udp）+ `goddi-standby`（role=standby，18082→6080，15355→53/udp）+ `ha-dhclient`（alpine+udhcpc，真 DHCP 客户端），bridge 网络 `ha-net`，各自独立数据卷；全部 HA 参数经 `GODDI_DHCP_HA_*` env 注入（peer_token≥16 字符、confirm 1s < heartbeat 2s < stale 5s）。镜像 `Dockerfile.ha`（新增）：复用**本地验证过的二进制与 web bundle**（多阶段 Dockerfile 的容器内 pnpm install 因 registry 网络抖动卡死，`.dockerignore` 排除 dist/ 需暂存二进制；宿主为 macOS 需交叉编译 `GOOS=linux GOARCH=arm64 CGO_ENABLED=0`——modernc.org/sqlite 纯 Go 无 CGO）。

**实测时序（全部真实日志/指标）**：
1. **握手+快照**：primary「HA: mirror connected → sent a snapshot to the mirror」/ standby「mirror rebuilt from a snapshot」（跨容器 TCP）。
2. **真实 DORA + 确认 + 镜像**：udhcpc 广播 DISCOVER → primary 从作用域 `ha-bridge`（192.168.117.0/24，池 .240–.250）**两次真实 DORA** 发放 .240/.241 → `goddi_dhcp_ha_sequence=4=acknowledged=peer_applied`、`promising=1`、`redundant=1`、`lag=0`；**standby 租约表出现同样 2 条 active 租约（generation 1）**——跨容器镜像实证。lease facts 按设计被保守省略（本地 IPAM 无该地址空间映射——不猜测，见日志 WARN）。
3. **故障① 容器死亡**：`docker stop goddi-standby` → primary「mirror session ended (EOF)」→ **`state: paused`**、`promising 1→0`、`redundant 1→0`、`/ready=degraded`——fail-closed（不再承诺地址），管理面/DNS 继续应答。
4. **故障② 容器重启**：standby `applied_seq=4` **持久生效**（重启从 4 继续）→ primary 重发快照（seq=4, rows=2）→ 指标全部恢复、`/ready=ok`。
5. **故障③ 网络分区**：`docker network disconnect`（链路静默而非进程死亡）→ primary 走 **ticker 静默检测路径**：「the mirror has been silent for more than 5s」→ paused；standby 连 DNS 解析都失败（真实断网）→ paused 指标同上。
6. **分区恢复**：`network connect` → 快照重发 → 全指标恢复、standby 租约表完好（2 条 active）。

**结论**：镜像链路心跳/静默判定、fail-closed 状态机、快照 resync、持久化水位在**跨容器真实 TCP** 上全部按设计工作；两条检测路径（EOF 与静默超时）均被触发。仍缺：物理断电/掉电（同宿主磁盘）、fencing/witness 信任域、Catalog zones（批次 E 未实现）、真实 relay——B8.5 验收项不变。

### 1ca-补：primary 死亡时 standby 不自行提升——防脑裂设计实证（round 247）

`docker stop goddi-primary` 后观察 standby 60 秒：日志**仅**「lost its primary; reconnecting」（EOF → DNS lookup 失败，持续重试），**takeover/promot 相关日志 0 条**；`/ready=degraded`；无 primary 视角 HA 指标（HA gauges 从 primary 视角注册——standby 的状态面是 `/ready`）。重启 primary → **双侧同时重连**、快照重发（seq=4, rows=2）、`promising/redundant` 恢复 1、`/ready=ok`。**防脑裂设计（standby 永不自行决定成为 primary，`mirror.go` 的 job description 明文）在容器化环境实证成立**。

### 1ca-补二：操作员接管 + fence/rejoin 全流程实证（round 247–248；角色翻转）

续 §1ca 的自动行为验证后，**操作员驱动的接管与故障回卷全流程**在容器化双节点上实测（`goddi ha takeover/fence/rejoin/status` CLI）：
1. **接管前提**：`docker stop goddi-primary` → standby `ha status` 报 `shortfall: 0`（applied=primary=4）→ 无缺口。
2. **接管**：standby 上 `goddi ha takeover --confirm --old-primary-cannot-write --accept-gap 0` → 「ha-node-b is now the primary; sequence resumes 4」；`role in force: primary`（**meta 持久角色覆盖 env 的 role=standby**）、`state: primary-degraded`（诚实：尚无镜像副本）。
3. **提升在下次启动生效**：重启 standby 容器 → `role_in_force: primary`、**「DHCP server started」（原 standby 开始 DHCP 服务）**、`primary is listening for its mirror`。
4. **真实发现（记录为边界）**：接管后的节点收到 DISCOVER 报「no matching scope」——**作用域配置不在 HA 镜像范围内**（镜像只复制租约事实；scope 配置复制属 ADR-0005 未实现的多节点配置协调）。在提升节点上补建作用域后，新 DORA 由提升节点应答（.240）——**服务连续性成立**（需一次手工作用域步骤，如实记录）。
5. **fence**：旧 primary 容器处于停止态，在其数据卷上 `goddi ha fence --confirm --reason`（**需与容器一致的 env**：`GODDI_SERVER_DATA_DIR`/`GODDI_DHCP_HA_*`——漏 data_dir 时报「never opened its DHCP lease store」fail-closed）→ 「ha-node-a is taken out of service」。重启旧 primary → **以 `fenced` 起来**（「will not serve clients and will not mirror」）——没有以第二个 primary 回来。
6. **rejoin**：`goddi ha rejoin --confirm` → 重启 → **回到 standby**，连接新 primary（goddi-standby:9953）、「mirror rebuilt from a snapshot seq=6」、applied=8、`/ready=ok`；新 primary 侧 promising=1、redundant=1、sequence=6=acknowledged=6、新 DORA（.241）由其应答。
7. **角色翻转后的配对**：goddi-standby 容器=primary（服务中）、goddi-primary 容器=standby（镜像中）——**容器名与角色完全翻转而配对健康**。

**边界**：接管需操作员显式三确认（--confirm/--old-primary-cannot-write/--accept-gap 精确缺口）——无自动 failover（设计如此，ADR-0003 决策 4：peer_stale_after 只降级永不提升）；scope 配置复制缺失是 ADR-0005 边界。仍非 B8.5 验收（同宿主、无物理断电、无 fencing/witness）。

### 1ca-补三：takeover CLI 帮助补运维提示（round 248）

§1ca-补二 发现的「接管后作用域不随镜像复制」边界落到运维文档：`goddi ha takeover` 的 Long 帮助补一段——提升后的节点用自己的配置服务客户端，**DHCP scope 定义不随租约镜像复制**（跨节点配置协调是 ADR-0005，未实现），上客户端路径前需在本节点重建作用域，并引用容器演练 round 247 的实际 DISCOVER 现象。纯帮助文本，无行为变更。

### 1ca-补四：租约释放镜像的边界说明（round 249）

尝试经管理 API 释放租约以测「释放事件的跨容器镜像」→ primary 返回 **500「lease mutation owner is unavailable」**——这是 **round 149 生产边界守卫的设计性行为**：管理释放路由（`ReleaseLeaseFromManagement`）与包路径（HandleRelease）必须使用**同一 mutation owner**（server.go 注释明文「must not receive a different mutation owner」），而统一 owner 属 v0.9.0 主线 1（默认进程未装配）→ 管理释放 fail-closed。DORA 创建路径不受影响（已实证镜像）。
- 释放镜像本身走与创建相同的 ops 帧路径（按序租约变更），创建镜像已实证即覆盖该机制；真实 DHCPRELEASE 报文级测试需要一个能构造裸 DHCP 包的客户端（udhcpc 无 release 语义）——列为后续，不作为缺口宣称。
- 管理释放 500 不是缺陷：**与生产边界测试钉住的行为一致**；运维释放租约的现实路径是包路径或等 future 统一 owner 装配。

### 1ca-补五：提升后 primary 自身重启的持久性（round 250，容器层最后场景）

`docker restart` **提升后的 primary**：重启后直接以 primary 起来（seq=8 **持久化水位**）、**DHCP 服务立即恢复**（重新绑定 eth0=.2）、作用域与 2 条租约**全部存活**、standby（旧 primary）`peer_applied_seq=8` **立即追平**、快照重发、`/ready=ok`。至此容器化双节点演练 **11 个场景全部收口**：握手/快照、真实 DORA+镜像、standby 死亡→paused、standby 重启→持久 resync、分区→静默检测、分区恢复、primary 死亡→防脑裂、操作员接管、提升生效+服务连续性、fence/rejoin 角色翻转、提升后自重启持久性。

### 1ca-补六：STATUS 容器化行刷新为 11 场景全览（round 251）

`docs/STATUS.md` §1 的「容器化双节点 HA」行刷新为 11 场景全览（含接管/fence/rejoin/角色翻转/自重启持久性的最终结果）。

### 1aw-补五十五：终态全矩阵确认（round 252）

round 248 的 main.go 帮助文本变更后，按文档既定顺序的**终态全矩阵**复跑：`make verify`（EXIT=0）→ web/dist 同步 → 重建二进制 → `make verify-browser`（20 项 TOTAL_FAILS=0）→ 实例停止。最终工作树全绿确认；无遗留顺序违规。

### 1av-补十：终态 e2e 确认（round 253）

终态全矩阵的最后一块：`make verify-e2e` → bare **4 passed**（临时 plain 实例）+ fixture **8 passed**（w13 种子实例），EXIT=0——终态工作树的 Playwright 套件全绿确认（round 252 已确认 Go/browser 侧）。

### 1ca-补七：全部运行服务健康总览（round 254）

- **18080 单机实例**：在 round 252 的 `pkill` 中被误杀（验证脚本会清掉所有 `goddi serve` 进程），已为用户重启（health ok、web 200）——该 `pkill` 副作用记入运维注意。
- **容器 HA 对（30+ 分钟长稳）**：primary（18082, ha-node-b）`sequence=9=acknowledged=peer_applied`、`promising=1`、`redundant=1`、`lag=0`、`/ready=ok`；standby（18081, ha-node-a）`/ready=ok`、无 primary 视角 HA gauges（状态面=/ready）。角色翻转后的稳定态持续。
- **DNS**：primary 容器 :53（宿主映射 15354）权威应答正常。

### 1ca-补八：STATUS 一句话结论纳入容器化证据层（round 255）

`docs/STATUS.md` 的一句话结论更新：双机部分从「架构预留与离线候选/原语已完成」扩为「**并经 2 容器双节点演练实证（11 个场景…非 B8.5 验收）**」——交接读者从此一眼看到容器化证据层的存在与定位。

## 1ca-补九（终）：会话收尾状态（round 256，自主运行最后一轮）

**终态核验全绿**：`go test ./...` / `go vet` / `gofmt` / `git diff --check`、`docs`+`contracts` 守卫、三服务健康（18080 单机、18081/18082 容器 HA 对）、双容器 healthy。自主运行在本轮结束：目标保持未完成（B8.5 外部验收、专家冻结、本机授权、遗留库升级需工作树之外输入），转入 blocked 状态等待外部输入；恢复入口见 STATUS §4 一键复现与本台账各节证据。

## 1cb. B8.5-E 实测：真实 Prometheus + promtool + Alertmanager 告警链（round 256–257；用户授权容器环境）

**定位**：以真实监控工具在容器环境闭合 B8.5-E 的核心要求（真实 Prometheus 抓取、promtool 校验、规则评估、Alertmanager 路由、告警触发/恢复原始时间线）。仍非物理真机上的长期运行验收。

**实测链（全部真实工具与时间线）**：
1. **认证抓取**：三个 goddi 实例（18080/18081/18082）各建 `scope="*"` 的抓取 API 令牌（`/metrics` 走 `Authentication`——未认证 401 契约保持）；Prometheus v2.53.0 以 `authorization.credentials` 抓取，**三 target 全部 `health=up`**。
2. **promtool**：`check config` SUCCESS；`check rules` **SUCCESS: 30 rules found**（`docs/prometheus-alerts.yml` 的规则在真实 promtool 下全部合法）。
3. **告警触发**：`pkill` 单机实例 → 目标 `up=0` → 规则 `GoDDIInstanceDown`（`up{job="goddi"}==0 for 1m`）→ **FIRING** → Alertmanager v0.27.0 收到 `active` → webhook 接收器捕获 **`21:35:24 | firing | GoDDIInstanceDown | goddi`**。
4. **恢复**：重启实例 → **`21:36:04 | resolved | GoDDIInstanceDown | goddi`**——firing→resolved 完整时间线在案。
5. **HA 指标在抓取中**：`goddi_dhcp_ha_*` 9 个序列经真实 Prometheus 抓取（sequence=9=acknowledged=peer_applied、lag=0）。

**过程中修正的三个配置要点（对后续部署有参考价值）**：① 三个实例需**不同的抓取令牌**（各实例独立 DB）；② 告警规则匹配 `job="goddi"`——多实例抓取须用 `relabel_configs` 把 job 统一改写为 `goddi`（instance 标签区分节点）；③ 双网络（host+bridge）下容器名 DNS 不解析——Alertmanager 地址用 IP。

（补：B8.5-E 行的**状态单元格保持「未验收」**——容器演练是部分闭合，物理真机长期验收仍开放；守卫的 ≥5 未验收行断言继续成立，防止外部项被演练证据误升级。）

## 1cb-补：DHCP 租约 schema 冻结与 dhcplease 包实现（round 256–258）

用户授权（「从合理角度评估决策直接实施，不要叫我决策」）下，6 个开放决策项冻结并实现：
- **合同冻结**：`contracts/enterprise-dhcp-lease.proposal.md` 状态 PROPOSAL→**FROZEN**，§6 逐项记录决定：①地址 `BLOB(16)` 网络序 + `family (4,6)`（与 IPAM 同族编码）；②状态枚举 `OFFER/BOUND/RELEASED/EXPIRED/DECLINED`（旧词表映射 active→BOUND、offered→OFFER、conflict→DECLINED）；③`expires_ms` 可 NULL=永久保留、导入不重算；④活跃唯一改部分唯一索引（`WHERE kind IN ('OFFER','BOUND')`）+ `dhcp_declined` 隔离表双向互斥；⑤**并入八表快照为追加表集，schema 升 2**；⑥行编码沿用既有规则。
- **实现**：新增 `internal/enterprise/dhcplease`——冻结 DDL（`FrozenDDL`）+ `Verify`（family↔16 字节前缀一致性、活跃↔DECLINED 双向互斥、outbox 正序列、receipt kind 冻结枚举）+ **10 个测试全过**（部分唯一索引拒绝同客户端双活跃/允许终态、NULL 永久租约、DDL CHECK 拒绝 expires<issued、family 错配由 Verify 捕获——DDL CHECK 只能校验枚举，已在测试名中写明该边界）。
- **边界**：`Verify` 通过证明候选内部自洽，不证明源库真实；snapshotstage 的 schema=2 组合（八表+租约表集）待实现；公开安装/激活入口仍恒拒绝（prodgates 不变）。

### 1cb-补二（续）：StageLeaseRows 暂存原语（round 258 续）

`dhcplease` 再加 `StageLeaseRows(tx, rows)`：装冻结 DDL → 按冻结列序应用候选行（INTEGER 规范十进制/BLOB base64url 解码、未知表/未知列/非十进制整数拒绝）→ `Verify` 全校验。**16 个测试全过**：两租约暂存、永久租约+隔离表+收据组合、未知表/列/整数拒绝、活跃↔DECLINED 重叠被 Verify 拒。过程中三个测试自身问题已修正（同 client_key 双活跃被部分唯一索引正确拒绝——测试行改用不同客户端；fixture 拆分为 openFrozen/openFrozenNoInstall 供 staging 自装 DDL）。**边界**：snapshotstage 级别的 guarded fixture 组合（八表+租约表集 12 表快照）待下一轮；公开安装入口仍恒拒绝。

### 1cb-补三（续）：snapshotstage schema=2 组合落地（round 259）

**十二表候选**在 snapshotstage 组合落地：guarded 八表 fixture（hardened 结构校验通过）之后，`afterVerify` 钩子（新增于 fixture_test.go——**租约暂存必须在 hardened 结构校验之后**，因为 eventlogguard 的参考匹配对额外表严格，先加租约表会让「schema does not match one reference」拒绝整个候选——round 259 实测该顺序边界）经 `seePath` 打开候选 DB，`dhcplease.StageLeaseRows` 应用租约行（space "s"、地址在池 192.0.2.100–110 内）+ `dhcplease.Verify`。
- **两个组合测试**：① 十二表组合成功（八表完好 + dhcp_lease 2 行 + dhcp_declined 空）；② 活跃租约↔DECLINED 重叠**跨包被拒**（dhcplease.Verify 在组合层生效）。
- **顺序边界记入**：schema=2 的生产实现必须把租约表暂存放进 eventlogguard 结构校验之后的步骤（或让 hardened 参考吸收租约表集——后者需 ADR-0002/0007 演进）。

## 1cc. 合成遗留库升级演练（round 259；用户授权容器/合成环境）

P0-G1 的「真实上游升级」需真实遗留数据——本轮以**合成遗留库**演练升级路径（如实定位：合成 fixture 不是真实遗留数据的对账，真实对账仍是外部项）：
- 新增 `cmd/goddi/legacy_upgrade_drill_test.go`：**25 迁移基线**（v0.8.2/v0.9.0 前沿 001–025，与 git 标签实测一致）建库 → 插入遗留形状数据（dns_zone/dns_record/user）→ 当前二进制的**嵌入迁移集**应用 026–032 → goose 版本达 32 → **遗留数据逐列存活**（zone 名、record 值 192.0.2.77、user enabled）→ 新迁移对象（dns_zone_transfer/dns_dynamic_update_policies）存在。
- 演练钉住：26–032 迁移在 25 基线之上**干净应用**且数据保留——这是真实站点升级路径的机械复现（合成数据版）。真实遗留数据的字段级对账仍需真实遗留库。

### 1cb-补四（续）：租约表集 NDJSON 行校验器（round 260 续二）

`dhcplease` 新增行校验器（`rows.go`，镜像 snapshotipam 的模式）：**冻结列序 definitions**、BLOB base64url / INTEGER 规范十进制 / TEXT 有界 UTF-8 无 NUL、kind 冻结枚举、expires_ms 可 null、address 必须 16 字节、非负整数检查——覆盖全部四张租约表。**8 个新测试**：合法 BOUND/永久/四表全行通过；字段数不符、16 字节违反、alien kind、负整数、NUL 文本拒绝。过程中修正测试 fixture 的非法 base64url（client_key 过短——字段按冻结列序校验，client_key 先于 kind）。
- **边界**：这是 NDJSON 行级校验；快照导出侧的行生成与 PK 顺序断言待接线到 snapshotpipeline 的 feed。

### 1cb-补五（续）：ExportLeaseRows 导出侧 + PK 顺序断言（round 260 续三）

`dhcplease` 新增 `ExportLeaseRows(tx)`：四张租约表按**主键 BINARY 严格递增序**导出（SELECT ORDER BY pk——SQLite 的 BLOB 字节序/TEXT BINARY 序正是冻结行序），行形状与 validateRow **互为逆操作**，并带**严格递增断言**（序不匹配即报错——防止 collation 漂移静默破坏行序）。
- **往返测试**：staged → export → 每行过 validateRow → re-stage 到新库 → **行数与地址逐字节一致**（hex(address) 相等）。**26/26 测试全过**。
- **本轮修掉 export.go 的一个真 bug**：内层 `pk := make([]string, 0, len(pk))` **遮蔽**了 pkColumns[table]，`for _, c := range pk` 遍历空切片 → PK 永远为空 → 正常数据被误判「非递增」；同时清理了误导性的 containsPK(pk,"") 残块。修复后正常数据通过、断言保真。

### 1cb-补六（续）：SchemaTables feed 接线件（round 260 续四）

`dhcplease` 新增 `SchemaTables()`：**租约表集以 snapshotpipeline.SchemaTable 形式暴露**（frozen 顺序 4 张表、各带行校验器）——与 `snapshotipam.New()` 的 8 表条目组合即得 schema=2 快照的**统一校验流**（manifest 列 12 表、每行过冻结形状校验；snapshotpipeline.New 拒绝 schema/manifest 失配）。2 个测试：frozen 顺序与校验器存在、好行通过/alien kind 经同一 feed 面拒绝。至此 schema=2 的三件套闭环：行校验（validateRow）、DB 暂存（StageLeaseRows）、导出（ExportLeaseRows）、**feed 接线（SchemaTables）**。

### 1cb-补七（续）：12 表 schema=2 流水线端到端测试（round 260 续五）

`snapshotstage/schema2_pipeline_test.go`：**十二表 NDJSON 流**（guarded 八表流 + 真实租约行）流经**统一 snapshotpipeline**（schema = snapshotipam 8 条 + `dhcplease.SchemaTables()` 4 条；manifest `Schema: 2`、12 表精确匹配——`snapshotpipeline.New` 强制 schema/manifest 对齐）：
- **通过路径**：feed 两个 chunk → Finish → `StructureAndCountsValid` + `AllRowCallbacksPassed`——十二表流在真实流水线校验全过；
- **拒绝路径**：alien kind（NAK）行在 **Feed 时即被拒**（行回调在 feed 期运行，拒绝即路径本身）；
- `BusinessComplete=false` 语义再次钉住（行回调不宣称业务完备）。
- 演练中修正 fixture 两处（dhcp_declined 地址也须 16 字节形式；alien 行在 Feed 期即拒绝、非 Finish 期）。

### 1ca-补十：SIGKILL 崩溃恢复演练（round 260；容器层最接近掉电的模拟）

`docker kill`（SIGKILL，exit 137——无优雅关闭）击杀**镜像侧节点**（goddi-primary 容器，当前角色=standby/镜像）→ `docker start` 重启：
- **WAL 恢复**：租约数据完好（.241 active / .240 expired——与 pre-kill 状态逐条一致，包括已过期项的过期语义）；
- **水位持久**：`applied_seq=11` 重启即恢复（非猜测）→ 连接 primary → 「mirror rebuilt from a snapshot (seq=11, rows=2)」→ 全指标恢复（sequence=acknowledged=peer_applied=11、promising=1、redundant=1、/ready=ok）；
- **恢复后继续服务**：新 udhcpc DORA 获得 **192.168.117.242**（新地址，序列继续推进）——配对在硬崩溃后继续服务。
- **边界**：SIGKILL 无优雅关闭但仍是同一宿主内核/磁盘——物理断电（磁盘页缓存丢失）与双节点同时掉电仍是外部项。`ha-dhclient` 容器一次性退出后需 `docker start` 重启（已在演练中处理）。

### 1ca-补十一：SIGKILL 窗口的 Prometheus 时间线证据（round 261）

round 260 的 SIGKILL 恢复演练与监控证据闭环：`goddi_dhcp_ha_redundant`（ha-node-b，primary 视角）的 Prometheus **range 时间线**（40 分钟 161 个 5s 样本）中恰好 **1 个 zero**——SIGKILL 窗口内 primary 镜像链路死亡 → paused 被**真实抓取**，随后恢复为 1（first=1, last=1）。`GoDDIDHCPHANoSecondCopy`（`==0 for 5m`）正确地**未触发**——17 秒窗口低于告警阈值是设计（短暂镜像丢失不应 page；恢复后自愈）。**fail-closed 状态机的行为变化被监控时间线独立捕获**——崩溃演练与监控证据相互印证。

### 1ca-补十二：双节点同时 SIGKILL——同时崩溃恢复（round 262；容器层第 13 场景）

`docker kill goddi-primary goddi-standby`（**同时** SIGKILL，双容器 exit 137）→ 双双重启：
- **角色持久**：ha-node-b 直接回 primary（DHCP server started，服务 bridge）、ha-node-a 直接回 standby（「will not serve clients」）——翻转后的角色经持久 meta 存活；
- **水位持久**：双侧 `applied_seq=16` 重启即恢复；primary「mirror connected, peer_applied_seq=16」→ 快照重发（**rows=4 双侧一致**）；
- **数据存活**：租约表双侧一致（.242 active、.241 expired、.240 等四行——含过期语义）；
- 指标全恢复（16=16=16、promising=1、redundant=1）、双 `/ready=ok`、双容器 healthy。
- **边界**：同时 SIGKILL ≠ 物理断电（同一宿主内核/磁盘、页缓存仍在）；真实「双节点同时掉电」仍是 B8.5 外部项。

### 1ca-补十三：HA 专属告警真实触发→恢复完整生命周期（round 263）

**区分两种断链的可观测语义**（本轮关键发现）：
- **全节点分区**（standby 断开唯一网络）→ 其管理面/metrics 不可达 → **InstanceDown 告警**（22:23:25 FIRING）——分区节点无法报告自身状态，InstanceDown 是正确告警；HA 专属告警因序列 stale 而无法评估（诚实行为）。
- **仅镜像死亡、primary 可达**（`docker stop goddi-primary`）→ primary 30 秒内检测（PeerStaleAfter=5s）→ `paused`（promising=0、redundant=0）→ **5 分钟整两个 HA 专属告警同时 FIRING**：`GoDDIDHCPHANoSecondCopy` + `GoDDIDHCPHAWithholdingLeases`（node=ha-node-b）→ Alertmanager active → webhook 捕获（22:34:24 firing×2）。
- **恢复**：`docker start goddi-primary` → mirror 重连（applied_seq=16 持久）→ 快照重建（rows=4）→ promising/redundant 回 1 → **22:35:54 两条 HA 告警同时 resolved**（webhook 捕获 resolved×2）→ /ready=ok。
- **完整生命周期在案**：firing→resolved 原始时间线覆盖全部四种告警（InstanceDown/LowCacheHitRate/NoSecondCopy/WithholdingLeases）。
- **运维提示已在告警描述中**：NoSecondCopy 的 description 说明三种状态（paused/primary-degraded/fenced）都读 0、重启无效、要看 /ready 的 ha_state——与 CLI 帮助的边界一致。

### 1ca-补十四：操作员批准的单副本运行模式（round 263；容器层第 14 场景）

镜像死亡 → primary `paused`（promising=0—— withheld ACK）→ **`goddi ha degrade --confirm --reason`**（首次无 --confirm 被 fail-closed 拒绝：「this action changes what this node serves and requires explicit confirmation」——设计如此）→ 授权记录入 lease store（"mirror had already confirmed everything: 16 of 16"）→ **运行服务一个心跳内拾取**：`promising 0→1`（单副本 ACK 恢复）、redundant=0、/ready=degraded——**primary-degraded 状态**（操作员批准的单副本运行）。
- **恢复镜像** → 双侧重连（applied_seq=16 持久）→ 快照重建 → **许可自动清除**（"clears itself as soon as the mirror has caught up"）→ promising=1、redundant=1、`/ready=ok`、`ha status: state=primary`——全 HA 恢复。
- **审计轨迹**：授权理由（maintenance window…）按设计入 lease store/审计。
- **对照**：paused（无批准，withheld ACK，promising=0）vs primary-degraded（有批准，单副本 ACK，promising=1）——NoSecondCopy 告警的 description 指引（看 /ready 的 ha_state）与之吻合。

### 1ca-补十五：`ha degrade --undo` 立即撤回（round 264；容器层第 15 场景）

**授予→拾取→撤回**闭环实测：① 重新授权（mirror 已死）→ promising **25 秒后**拾取为 1（拾取窗口比首测的 ~8s 长——拾取经运行服务轮询，间隔非固定秒数）；② `ha degrade --undo` **也要求 --confirm**（撤回同样改变服务行为：从单副本 ACK 回到 withheld ACK——一致的 fail-closed；首次无 confirm 报错）→ 确认后「ha-node-b may no longer run without a second copy」→ **promising 1→0**（12 秒内拾取）→ /ready=degraded。
- **拾取窗口非固定秒数的说明**：首次授权 ~8s、本次 ~25s——运行服务按其内部周期拾取，帮助文本的「within one heartbeat interval」指的是 HA 心跳周期（2s）加上服务评估周期，不是精确的秒级承诺。
- **双路径确认语义**：grant 与 undo 都要求 --confirm——与 takeover/fence/rejoin 一致（所有改变服务行为的操作都需要显式确认）。

### 1ca-补十六：undo 演练后的配对恢复（round 265）

round 264 的 undo 演练后（withheld ACK 态），启动镜像容器 → standby 重连（mirror_applied_seq=16 持久）→ 快照重建（rows=4）→ primary 的 **promising 回 1**（withheld ACK 解除——镜像确认恢复）、redundant=1、/ready=ok——配对回到全 HA。**15 个场景 + 两次恢复回路全部收口**。

### 1cb-补八（续）：dhcplease 使计数 53 包/607 测试（round 264 续）

round 258 新增的 `dhcplease` 包使 enterprise 计数变为 **53 包 / 607 测试入口**——**派生式守卫自动派生出正确值并要求 STATUS/dossier/P0-G1 同步**（本轮同步四处）。P0-G1 的 569 处已在 round 210 修正为 575，本轮再同步到 607。计数守卫的自更新化（round 236–238）在此证明其价值：新增包当轮即被要求同步文档。

### 1ca-补十七：作用域耗尽 + 利用率告警真实触发（round 264–265；runbook §7.2 两场景）

**DHCP 队列压力**：10 个并发 DHCP 客户端容器（独立 MAC）同时 DORA → `queue_depth=0`、`dropped=0`（容量 256 吸收突发）→ **11/11 active 租约**（.240–.250 全部分配，含真实 MAC）→ `goddi_dhcp_scope_usage_ratio{scope}=1`。
- **GoDDIDHCPScopeExhausted**（`>=1 for 5m`）→ **FIRING**（22:52:24 经 Alertmanager→webhook）✓
- **GoDDIDHCPScopeNearlyFull**（`>0.9 for 15m`）→ **FIRING**（activeAt 14:47:09Z + 15m 整点触发——for 计时精确）✓
- 利用率 provider 三指标在案（`scrape_error=0`、`last_success_timestamp` 有值、`usage_ratio=1`）——round 149 的利用率 provider 在真实负载下工作。
- **B8.5-E 判据对照**：「真实客户端并发请求 → queue depth 变化」✓、「指标变化、规则评估结果和告警接收记录」✓（webhook firing 时间线在案）。
- **清理路径**：11 条租约 1h 后自然过期；客户端容器按需移除。

### 1ca-补十八：隔离上游失败演练（round 264；runbook §7.2「上游 DNS 失败」场景）

创建指向 TEST-NET-3 黑洞地址（203.0.113.1:53）的转发器——**SSRF 守卫实测**：先尝试私网地址 127.0.0.1:1 与 10.255.255.1:53 被「无效的上游地址」拒绝（allow_private_upstream 默认关闭的 SSRF 保护真实生效）；公开的 TEST-NET-3 接受 ✓（真正的「隔离上游」）。
- **指标变化**：`goddi_dns_upstream_consecutive_failures{dead-upstream}=1`（健康探针失败计数）——runbook 要求的「对应指标变化」满足。
- **递归行为**：非权威名查询 → 死上游失败 → **故障转移到健康上游** → 公开根对保留 TLD `.test` 应 NXDOMAIN——解析器的 failover 按设计工作（SERVFAIL 需全部上游失败，属更深故障注入）。
- 清理：死转发器已删除，配对健康。

### 1ca-补十九：控制库只读演练的完整结论（round 264–265；三个 fail-closed 层）

「控制库不可写」演练揭示**三层 fail-closed**（全部实测）：
1. **WAL 惯性**：仅主库文件只读时，WAL 模式下写仍经 `-wal` 文件成功（checkpoint 才碰主文件）——「只读 DB 文件」不等于「只读数据库」；
2. **非 root 用户 + 全目录只读**（生产一致的 USER goddi 镜像，`Dockerfile.ha` 补 `USER goddi` 后重建）→ sqlitepolicy 连接钩子「attempt to write a readonly database (1544)」→ **应用拒绝启动**——P0-G1「不合格即拒绝连接」在容器环境验证 ✓；
3. **恢复路径的运维细节**：权限恢复必须 `chown goddi:goddi`（容器以非 root 运行；alpine 工具容器无该用户，须用 goddi:ha 镜像+覆盖 entrypoint）+ `docker start`——双容器双双恢复 healthy、/ready=ok。
- **演练中曾出现双 primary 危险态**（旧 primary 未 fence 直接重启）——本轮以 fence 流程纠正并记录：未 fence 的旧 primary 重启=双 primary，必须先 fence。

### 1ca-补二十：ADR-0003 附录（round 264–265 演练发现入合同文档，round 266）

容器演练的 6 点发现（实证清单、双 primary 危险态、控制库只读三层语义、degrade 确认语义、无自动 promote 再次确认）记入 ADR-0003 附录——合同文档与演练证据同步。`docs` 守卫 PASS。

### 1ca-补二十一：B8.5-A/B–D 行的容器演练注记（round 266 续）

B8.5-A 行补记容器化双节点演练（15 场景含真实 udhcpc DORA 镜像与 fail-closed 状态机——非 B8.5 验收）；B8.5-E 行此前已注记；B8.5-B/C/D 行保持原状（B 已有 round 124 双进程实跑、C 已有 round 165/166 演练、D 已有 round 125 实跑+本轮容器演练补充）。**5 个 未验收 行保持不变**——外部验收状态不被容器演练升级。`docs` 守卫 PASS。

### 1cb-补九（续）：guarded schema-2 staging 落地 + StageLeaseRows 契约拆分（round 261 续）

**guarded schema-2 staging 落地**：`fixtureHooks` 加 `schema2`/`LeaseSinkRows`/`leaseInsert`——guarded 流水线 schema 条目 12 个（snapshotipam 8 + `dhcplease.SchemaTables()` 4）、`Schema: 2`、行经流内捕获、**租约行在主事务内经 `dhcplease.InsertLeaseRow` 应用**（计数断言覆盖 12 表）、eventlogguard 校验后 afterVerify 钩子做十二表 Verify。**两个 guarded 测试**：十二表组合成功（IPAM 完好+dhcp_lease 2 行+outbox/receipt/declined 各 1）；alien kind 拒绝。
- **StageLeaseRows 契约拆分**：InstallFrozenDDL/InsertLeaseRow 导出、StageLeaseRows=组合（不再自装 DDL）——schema-2 组合需要 DDL 与行分离（DDL 在主事务前装、行随主事务、Verify 在结构校验后）。拆分后 dhcplease 28 测试+snapshotstage 全部测试通过。
- **过程中一次 fixture_test.go 编辑损坏已修复**（syntax 断行；git 未跟踪该文件，手工修复）。

### 1cb-补十（续）：导出即有效候选流——导出/校验闭环（round 262 续）

`dhcplease` 新增 `TestExportedRowsPassTheFeedValidators`：staged 12 表 DB 经 `ExportLeaseRows` 导出的每一行**逐行通过 `SchemaTables()` 的 feed 校验器**——导出（writer 侧）与 feed（reader 侧）是同一合同的两面，实证成立。**dhcplease 29/29 测试**。schema=2 快照的校验链至此完整：行校验、DB 暂存、导出、feed 接线、guarded staging、导出/校验闭环。

### 1cb-补十一（续）：dossier 3.2 纳入 dhcplease（round 262 续二；计数 607→610）

ENTERPRISE-REVIEW-DOSSIER §3.2 更新：`dhcplease`(29) 纳入快照候选包列表；「证明」补 round 258–262 schema=2 租约表集成果；「不证明」移除已被 dhcplease 覆盖的租约行（保留安装/激活、来源可信等）；提案状态更新为 **FROZEN**（round 258，用户授权决策）。计数守卫再次自更新化（607→610，新增测试自动派生）——STATUS/dossier/P0-G1 三处同步，守卫 PASS。

### 1cb-补十二（续）：snapshotipam.ExportRows——八表导出侧 + 导出/校验闭环（round 263）

`snapshotipam` 新增 `ExportRows(ctx, Querier)`：**八表候选导出**——列类型/主键列序**从 staged schema 自身推导**（PRAGMA table_info/index_list：BLOB→base64url、INTEGER→十进制串、TEXT 原文、NULL 保留），ORDER BY 主键列（SQLite BLOB 字节序/TEXT BINARY 序=冻结行序）+ **严格递增断言**（防 collation 漂移）。
- **导出/校验闭环测试**（snapshotstage/export_feed_test.go）：guarded staged 候选 → ExportRows → 每行过 `snapshotipam.New().Validate`（其内部 PK 顺序校验同时证明导出序=冻结序）——**一次通过**。
- **快照再抽取原语至此完整**：八表（snapshotipam.ExportRows）+ 四租约表（dhcplease.ExportLeaseRows）均可从 staged DB 导出为合法候选流——schema=2 候选的「从可信源重新抽取」步骤的原语齐备。

### 1cb-补十三（续）：流水线级再抽取演练 + ExportLeaseRows 表序修复（round 263 续二）

**流水线级再抽取演练**（snapshotstage/schema2_reextract_test.go）：组合 12 表候选 → 导出双半（snapshotipam.ExportRows 八表 + dhcplease.ExportLeaseRows 四租约表）→ 重建 NDJSON 流与 manifest（Schema 2、12 表精确计数）→ **全新流水线**（无需原 DB）feed 两 chunk → Finish → `StructureAndCountsValid` + `AllRowCallbacksPassed` ✓——快照迁移路径「从可信源重新抽取全新候选快照 → 独立只读校验」的端到端实证（不含安装）。
- **修掉一个真 bug**：ExportLeaseRows 按 definitions **map 迭代（随机表序）**——snapshotrows 的流内表序非递减检查拒绝（`i < v.last`）。加 `exportOrder` 冻结顺序切片修复。**教训**：导出侧必须与冻结表序一致，map 迭代不可用于流生成。

### 1cb-补十四（续）：STATUS Enterprise 行补 schema=2 租约表集（round 263 续三）

STATUS.md Enterprise 候选行补记 round 258–263 的 schema=2 租约表集落地（提案 FROZEN 后的完整校验链）——一句话结论与新工作同步。`docs` 守卫 PASS。

### 1cb-补十五（续）：5000 租约规模演练（round 264）

`dhcplease` 新增 `TestExportHandlesALargeLeaseSet`：**5000 条租约** staged → ExportLeaseRows（严格递增断言在规模下成立）→ 每行过 feed 校验器——新导出原语的规模健全性实测（0.13s）。

### 1cc-补二：标准矩阵全绿（round 265；verify + browser 20 套件 + e2e）

本轮改动后的全树健康检查：**`make verify` PASS**（build/tests/vet/gofmt/前端 lint+typecheck+build/补丁卫生）；**`make verify-browser` TOTAL_FAILS=0（20 套件全过）**；**`make verify-e2e` PASS**（bare + fixture 双套件，fixture 8/8）。
- **过程中修复两个环境级问题**：① `make verify` 的前端构建使 web-admin/dist 与已部署 web/dist 哈希漂移 → 按 Makefile web-build 步骤重新部署；② 运行中的旧二进制内嵌旧 index.html 引用旧哈希 → 资产 404 → 登录页 0 输入框 → 浏览器套件超时——**重建+重启隔离服务器**后消除（html/资产一致性由 docs 守卫的 hashTree 对比钉住——守卫在先捕获了漂移 ✓）。
- playwright 解析：NODE_PATH 指向主 checkout 的 e2e/node_modules（worktree 无本地安装；README 记载了两种方式）。

### 1aw-补五十六：统一事实链 lag/gap 观测接入 /ready——dhcp_facts 生产侧 plane（round 266）

计划第 3 主线「完整 lag/gap 观测」的 /ready 侧缺口闭合：新增 `handler.DHCPFactsPlane`（outbox 状态→就绪平面：**failed 事件=Failing**（不把失败伪装成健康——计划诚实原则）、**pending backlog=Degraded**、空=OK）+ main.go 注册 `dhcp_facts` plane（observation 启用时=primary/standalone 门控内）。
- **映射单测 4/4**（空/积压/失败/状态错误）；生产装配现状核实：**FactsMutationWriter seam 已在生产装配**（`dhcpSrv.SetLeaseFactsMutation`，DNS sink+post-commit wake+space resolver 齐备）、22 个 seam 测试覆盖 Activate/Bind/Renew/Decline/Release/Expire 的 Before/After+回滚恢复 lease+sequence+旧 sink 同事务+调用方回滚——计划「最小安全下一步」**已完整落地**。
- **剩余（计划自身门控）**：默认进程装配（迁移评审项）、DNS consumer 迁移、跨重启生产 replay—— expert-frozen/生产迁移项，容器层可演练。

### 1aw-补五十七：统一事实链容器跨重启 replay 实证（round 267；计划第 3 主线剩余项的容器层证据）

**容器层统一事实链全链路实证**（镜像从当前树重建后——旧镜像早于 facts wiring）：
1. **scope→space 诚实 fail-closed**：容器 IPAM 无 space 映射时 facts 被省略（`resolveLeaseFactSpace` ErrNoRows→omit+legacy observer——设计如此）；经 API 补齐 space/subnet（ha-net-space + 192.168.117.0/24）后 facts 写入开始。
2. **facts 事件真实写入**：DORA（.247）→ `dhcp_ipam_observation_events` 1 事件 sequence=1（**单行计数器原子分配** ✓——非 MAX+1）。
3. **跨重启 replay**：`docker restart` primary → 事件/水位持久（sequence=1）、角色持久（role_in_force=primary）、observation enabled → **重启后新 DORA → sequence=2 延续**（非重新开始）——统一事实链跨重启 replay 在容器层实证 ✓。
- **定位**：容器层证据（非生产 replay）；剩余「默认进程装配、DNS consumer 迁移」仍是计划自身的生产迁移评审项。
- 计划第 3 主线的「跨重启生产 replay 尚未闭合」在容器层取得直接证据（生产级仍待评审）。

### 1aw-补五十八：统一事实链跨双节点 LIVE（round 268；容器层重大证据）

**统一事实链在容器双节点上全链路 LIVE**：
1. **dhcp_facts plane LIVE**：显式重建镜像（compose 缓存曾跳过构建——旧镜像缺 plane 代码，`strings` 检查捕获）后，容器 primary 的 `/api/v1/system/dataplane` 出现 **dhcp_facts → ok** ✓（outbox 已清空）；
2. **outbox 消费（done×2）**：primary 的 REPLICATOR 把 facts 随 HA 复制发给镜像（「sent a snapshot, seq=94, **facts_seq=2**」）——outbox 清空 → plane=ok **正确**；
3. **跨双节点 facts 复制**：standby 侧（goddi-primary 容器）的 facts store **2 事件 sequence=2** ✓（「mirror rebuilt from a snapshot, facts_seq=2」）——**DHCP 租约突变 → facts outbox（单行计数器）→ HA 复制 → 镜像侧 facts store** 全链路在容器双节点上运行。
- **定位**：容器层统一事实链证据（生产级默认装配/DNS consumer 迁移仍是计划评审项）；跨重启 replay（round 267）+ 跨节点复制（本轮）= 计划第 3 主线的容器层证据基本齐备。

### 1aw-补五十九：接管时的 facts 水位连续性——takeover 一等公民检查（round 268 续）

**takeover 输出明确报告 facts 水位连续性**（统一事实链成为接管的 First-class 检查项）：
```
goddi ha takeover --confirm --old-primary-cannot-write
→ facts watermark: 2 of 2, shortfall 0
→ sequence resumes: 94
```
接管机制**显式校验并报告** facts 水位（2 of 2, shortfall 0）——统一事实链的 failover 语义在接管流程中成立 ✓。提升后 primary-degraded（诚实：尚无镜像副本）、提升在下次启动生效（round 247 已记录边界：DHCP 监听需重启开启 ✓ 本轮重现）、scope 配置不随镜像复制（ADR-0005 边界 ✓ 本轮重现——容器双方均有 scope 配置故本轮无需手工补建）。
- **池压力如实记录**：接管后 DORA 暂时失败非缺陷——9/11 地址被合法续租占用（drill 客户端容器 16:47 UTC 才退出、T1 续租至 17:38+），随机分配器 5 次尝试在 82% 占用下命中率低；租约 17:38+ 自然到期后恢复。sequence 延续（3）的下一次 DORA 证据待池释放后补充。

### 1aw-补六十：接管→fence→rejoin 完整回卷循环（facts 链贯穿；round 268 续二）

round 33 接管后的完整回卷在容器层闭环（第二次全流程，本轮含 facts 链）：
1. **fence**：旧 primary（goddi-standby 容器）数据卷上 `goddi ha fence --confirm`（env：`GODDI_DHCP_HA_ENABLED=true` 必须显式——漏报「dhcp_ha.enabled is false」fail-closed）→ 重启以 **fenced** 起来（"will not serve clients and will not mirror"）——**没有以第二个 primary 回来**（防脑裂 ✓）；
2. **rejoin**：`goddi ha rejoin --confirm` → 重启 → 回 standby、mirror connected、**rebuilt from a snapshot (seq=94, facts_seq=2, rows=23)**——**facts 链随镜像复制到回归节点** ✓；
3. **配对恢复**：Prometheus `goddi_dhcp_ha_promising{ha-node-a}=1` ✓、双容器 healthy。
- **第二次全流程**（round 247-249 第一次）+ facts 链贯穿 = 容器层 HA 生命周期证据完备（接管/fence/rejoin/崩溃/分区/ facts 连续性/跨重启 replay/跨节点复制）。

### 1cc-补三：计划文档进展注记 + 全树 race 复跑（round 269）

- **计划文档（实施计划-v0.9.0）第 3 主线补进展注记**（不改状态判断——「默认进程装配、DNS consumer 迁移」与生产级 replay 仍按计划门控）：事务 seam 22 测试完整实现、lag/gap 接入 /ready、DHCP mutation 真实写入/单行计数器 sequence/跨重启 replay/跨节点 facts 复制均取得容器层直接证据（指向 §1aw-补33～37）。
- **全树 `make verify-race` EXIT=0**（0 FAIL / 0 DATA RACE）——覆盖本会话全部新增（dhcplease 29 测试、schema=2 guarded staging、facts plane、导出原语）。
- 过程中发现：并发的 verify-race 前端构建再次造成 web/dist 哈希漂移——docs 守卫 hashTree 对比捕获 ✓，重新部署后恢复绿（第二次证明守卫价值）。

### 1cb-补十六（续）：台账编号冲突修复（round 270）

本轮新章节误用了 1aw-补33～37（与 rounds 218-227 的 Technitium 对标系列编号冲突）——重编号为 **1aw-补五十六～六十**（接续 round 252 的 补五十五）。编号冲突由增量索引核对发现。

### 1aw-补六十一：双节点 facts 链条状态可见性核对（round 270 续）

接管后双节点的 facts 状态核对：primary 侧 outbox **done×2**（REPLICATOR 已消费并发送）、standby 侧 **2 事件 sequence=2**（mirror rebuilt 接收 ✓）；`facts_projection_watermark` 双节点均空（IPAM 投影消费者在 HA 拓扑按设计未运行——事件已复制、投影待 DNS consumer 迁移/生产装配，诚实状态）。
- **分配器顺序扫描边界实测**：接管后 DORA 连续失败——分配器从池首顺序尝试 5 次（.240-.244 全被合法续租占用→确定性失败）；池 17:38+ 自然释放后恢复（后台任务跟踪中）。

### 1ca-补二十二：数据库锁压力演练（round 271；runbook §7.2 场景闭合）

**24 个并行 zone 写入**（curl 并发、无客户端节流）→ **24/24 全部 201 Created**，零 SQLITE_BUSY/锁错误暴露给客户端——WAL busy_timeout 串行化按设计吸收全部并发（sqlitepolicy 的连接钩子+busy_timeout）。runbook §7.2「数据库锁压力」场景闭合；演练区已清理。

### 1cc-补四：STATUS 证据范围声明更新（round 271 续）

STATUS 的「证据均来自隔离单机」声明更新为如实范围：**隔离单机 + 容器化双节点环境**（HA/facts 链证据层）——均不构成物理双主机/断电/生产证据（B8.5 A–D 未验收不变）。
- **docs 守卫的第三次漂移捕获**：后台 verify 的前端构建使 web-admin/dist 与已部署 web/dist 瞬时失配（web/dist 自洽 → 后台浏览器套件不受影响）；任务完成后按 round 35 模式重新部署恢复。

### 1aw-补六十二：容器端口映射澄清（round 271 续二；当前角色-端口对照）

**端口映射（compose 固定，round 246 §1ca-补一已记录）**：`goddi-primary` 容器 → **18081**；`goddi-standby` 容器 → **18082**。round 30–34 部分叙述中的「18082=primary」为**叙述端口假设错误**（round 246 文档正确）；各节的行为结论以 **node_id** 为准，均不受影响。
- **当前角色-端口对照（round 33 接管后）**：**18081 = goddi-primary 容器 = ha-node-a = primary**（facts 生产侧、`dhcp_facts` plane LIVE ✓）；**18082 = goddi-standby 容器 = ha-node-b = standby**（facts 接收侧 2 事件、无 dhcp_facts plane——门控正确跳过 ✓）。
- plane 分布严格跟随角色：primary 有 `dhcp_facts`、standby 无——**门控语义验证成立**（叙述错误排查过程中的额外正面证据）。

### 1cc-补五：全矩阵复跑收口（round 272；facts plane 变更后）

facts plane 变更（round 30）后的全矩阵复跑：**`make verify` OK** ✓；**`make verify-browser` 完成**（20 套件；`docs` 守卫重新部署后恢复绿）。

### 1aw-补六十三：接管后 facts sequence 延续（2→3）——连续性证据完整闭环（round 272 续）

接管后的第一次 DORA（.241，由新 primary ha-node-a 应答）→ **facts sequence=3**（延续，非重置）✓。接管 facts 连续性证据链完整：① takeover 显式校验并报告水位（2 of 2, shortfall 0）；② 提升后新 primary 的 facts sequence 从复制水位**延续**；③ fence→rejoin 回卷节点同样接收 facts（facts_seq=2 随镜像）。
- **诚实边界重现并记录**：新 primary 的 control DB 缺 space/subnet 映射（旧 primary 经 API 创建、不随 HA 复制——ADR-0005 多节点配置协调边界 ✓）→ facts 首次被省略（legacy observer 路径，租约正常发放）；经 18081 API 补齐映射后 facts 恢复写入 sequence=3。**接管运行手册需要「在新 primary 上补建 IPAM 映射」步骤**（或等 ADR-0005 配置协调）。
- 浏览器矩阵在服务器二进制与 web/dist 重建后 **TOTAL_FAILS=0**（20/20）✓。

## 1ad. 统一事实链默认进程装配落地——data-plane relay 实现与端到端实证（round 273–275；用户授权生产迁移实施）

计划第 3 主线的核心剩余项「默认进程装配」落地（用户 round 273 指令「不能提供真实物理设备，继续实施剩余工作任务」授权生产迁移实施）：

**1. 发现装配缺口**：单节点/HA 装配的 consumer 读的是 control DB 上的空 inbox（无人写入）——生产者事件在 **lease DB 的 outbox**；FactsConsumer 明确要求 inbox 与投影**同库**（ProcessOne 原子提交）——设计中的 **data-plane relay 组件不存在**。

**2. 实现 data-plane relay**（`internal/facts/relay.go`，新增）：lease outbox → control inbox 的跨库复制器——**严格 sequence 顺序**（watermark AdvanceTx 的 +1 语义天然防跳/防重）、**每事件 insert+水位 advance 同事务**（崩溃幂等重放）、不改动 payload、不标记源行（HA 确认权归生产者生命周期）。`ObservationOutbox.After`（新增读法：按 sequence 后取、不分状态）。3 个 relay 测试（顺序复制/幂等重放/异库拒绝）全过。

**3. Pipeline 集成**：FactsPipelineOptions.Relay + Start/Stop 的 relay goroutine 生命周期（1s tick、tick 错误 durable-safe 重试）。**main.go 装配重写**：门控从 `controlPlane && !haEnabled` 改为 `controlPlane && dhcpStore != nil && (!haEnabled || primary)`——**HA primary 也消费**（默认装配完成）。

**4. 消费者静默失败修复**：finishLifecycle 增加 slog（失败不静默——可观测性缺口修复）。

**5. 容器端到端实证**：
- relay 启动即复制历史 5 事件（facts_relay 水位=5）；
- consumer 原子投影全部 5 事件（**ipam 水位=5**）→ IPAM 地址 observed_state=in_use（.240/.241）+ 254 地址物化；
- **实时流转**：新 DORA → lease outbox seq=6 → relay → inbox done → **IPAM 投影 MAC 实时更新**（7e:14:52:ed:82:8e）——**DHCP 租约突变 → facts outbox → relay（跨库）→ consumer → IPAM 观测状态** 全链路实时运行。
- 全部测试+全树构建绿。

**6. 装配形状守卫同步**：wal_lifecycle_boundary_test 的装配钉住更新为新形状（relay 接线 + HA primary 全管线门控）——守卫目的不变（防意外丢失装配），形状随有意设计演进。

### 1ad-补二：DNS consumer 迁移第一步——DNS facts consumer + DNS 侧 relay 装配（round 274）

计划「DNS consumer 迁移」的第一步落地：
- **`internal/dhcp/dns_facts_consumer.go`（新增）**：`DNSFactsConsumer` 从统一 facts inbox 消费租约事实 → 派生 DNS 命令（bind/renew/activate→Create；release/expire/decline→Delete——与 legacy scope-aware sink 同策略）→ 复用 **同一 DNSLink.ApplyOne**（幂等、generation 宽容）→ inbox MarkDone + facts 水位（domain "dns"）同事务推进。fact payload 已含 scope_id ✓（无需扩展）。
- **main.go 装配**：servesDNS + 生产者 outbox 存在时——DNS inbox（dnsStore.DB）+ **relay #2**（lease outbox → DNS inbox，独立水位）+ DNS facts consumer（1s Replay 循环）；与 legacy dnsConsumer **双队列并存**（迁移期设计形态）。
- **装配守卫同步**：TestDefaultProcessKeepsLegacyDNSConsumer 从「禁止 facts DNS hook」更新为**钉住双队列共存**（legacy 不得消失、facts 不得缺失、DNS relay 不得缺失）——迁移安全性守卫。
- split 角色部署的跨进程 facts 复制（ddns 式 inbox 同步）为后续项（all-in-one 容器拓扑为本轮验证形态）。

### 1ad-补三：DNS facts 链端到端实证——A 记录由统一事实链发布（round 275）

**DNS consumer 迁移端到端实证完整**：
1. **generation 修复**：facts payload 补 `generation` 字段（writer/IPAM/DNS 三处——applyCreate 的世代拒绝需要正确 generation；旧 payload 解码为 0、消费者保守处理 ✓）；
2. **客户端 hostname**：udhcpc 需 `-x hostname:` 显式发送 option 12（-H 旗标在该 busybox 版本报 usage）——租约 hostname 记录 ✓；
3. **实时流转**：DORA（probe-host → .244）→ facts seq=10（bind 事实）→ relay=10 → **dns 水位=10（消费者应用）** → **A 记录发布：probe-host.ha.test. → 192.168.117.244 enabled=1** ✓✓✓。
- **验证条件链**：scope.domain_name=ha.test + DNS zone ha.test + scope.dns_updates — 服务端配置就绪后 facts 路径自动发布（此前 no-op 为诚实行为：无 zone/无 hostname=无可发布）。
- **DNS consumer 迁移核心闭环达成**：统一事实链驱动 DNS 记录发布，与 legacy 双队列并存（迁移期）。

### 1ad-补四：RELEASE facts seam 接线 + DNS 撤回路径端到端实证（round 276）

**发现并修复 RELEASE 的 facts 缺口**：`SetReleaseFactsMutation` seam 存在（server.go:1037）但 main.go **从未接线**——客户端 RELEASE 走 legacy lease 路径、facts 流从未得知租约结束（DNS 记录不撤回）。接线：`ReleaseFactsMutationConfig{Caller: writer, Source, ResolveSpaceID}`（与 bind seam 同 writer/源/解析器）——**租约释放与 durable fact 同事务**。

**DNS 撤回端到端实证**（容器）：
1. DORA（release-host）→ bind 事实 seq=13 → relay → dns 水位 → **A 记录发布**（release-host.ha.test. → .247）✓
2. 客户端 USR2 RELEASE → **facts seq=14 action=release** ✓（RELEASE seam 生效）→ relay=14 → dns 水位=14 → **A 记录删除 COUNT=0** ✓✓✓
- **DNS facts 链发布+撤回双路径完整闭环**——统一事实链驱动的 DNS 记录生命周期（发布与撤回）在容器内全部实时验证。

### 1ad-补五：主线 2 状态核对——统一 mutation 覆盖完整（round 277）

计划主线 2（WAL durable ACK）的「统一 mutation」核对结论：**全部六类租约突变**（bind/renew/activate/release/expire/decline）经 FactsMutationWriter 与 durable fact 同事务提交——RELEASE seam（round 276 接线）+ ExpireFactsBatch 路由（server.go:238 ✓）+ bind/renew/activate/decline（LeaseFactsMutationCaller ✓）。projection watermark：facts consumer 的 ipam/dns 双 domain 水位 ✓。重启 replay/关闭收敛：容器实证（水位持久+延续、pipeline.Stop）✓。管理面旁路：admin release fail-closed（round 149 设计）✓ 如实记录。
- **WALDurableGate 组件定位**：分阶段迁移的模型产物（生产用事务持久性——gate=nil 的既有语义 ✓ 保留+边界测试钉住）。「正式接管」剩余=WAL 事件流/legacy 复制流的**退役决策**（生产迁移评审门控）。
- 计划文档主线 2 补进展注记（不改状态判断，与主线 3 注记同模式）。台账本节为核对记录。

### 1ad-补六：DomainFacts 复制域——split 角色的 facts 复制装配（round 278）

**split 角色部署的 facts 复制落地**：
- **`DomainFacts`（"facts"）**（internal/dataplane/sync.go）：facts inbox 的复制域——`factsTables`（appendNewer 模式：**仅 producer-owned payload 旅行**，status/attempts/next_attempt_at/last_error 是消费者自有状态——ddns 模式的同款语义）；TablesFor/HasReplica 全接入。
- **控制迁移 033_facts_domain.sql**：`trg_dp_rev_facts_inbox_ins`（relay 的每次 inbox INSERT 触发 dataplane_revision('facts') +1——DNS 同步无需轮询全表）+ 'facts' 域种子行。goose 的 trigger 需要 StatementBegin/End 标记（022 模式）。
- **main.go 装配门控细化（relay/consumer 分离）**：relay 跑在**持有 lease DB 的进程**（all-in-one/DHCP plane，写 control inbox）；IPAM consumer 跑在**持有 control DB 的进程**（all-in-one/control plane）——split 部署各半进程各跑一半，facts revision 触发器保持 DNS 平面副本更新；DNS facts consumer **始终装配**（servesDNS——消费本地 inbox 副本，relay 进程内或复制填充均可）。
- 装配守卫/迁移计数守卫同步（33 迁移=25 legacy+8 current；boundary needles 更新）。

### 1ad-补七：统一事实链端到端终极实证——线上 DNS 应答（round 279）

**DHCP DORA → 统一事实链 → IPAM 投影 → DNS 记录 → 线上协议应答**，容器内全链路真实运行：
- DORA（verify-host → .247）→ lease outbox seq=15 → relay → control inbox 15 done → **IPAM 投影**（ipam 水位=15，8 地址 observed_state=in_use 逐地址实时）→ **A 记录**（verify-host.ha.test. → .247）→ **nslookup verify-host.ha.test 经 DNS 服务器真实应答 192.168.117.247** ✓✓✓
- relay/consumer 分离门控（round 278）重启后行为不变 ✓。
- **计划第 3 主线的核心闭环（DHCP mutation 真实写入 → 统一 sequence → 跨平面投影 → 线上服务）至此在容器内完整实证**；剩余「默认进程装配」语义=该装配即默认（无旗标、primary/standalone 自动）✓。

### 1ad-补八：主线 1 门控评估（round 280）

主线 1（默认 DHCP 热路径迁移）的完成前置=**WAL-backed LeaseStore 的生产接线**——ClaimAvailable 的组件契约自我声明该门（memory_index.go:296-302）；前置是事件溯源租约存储的架构级建设（持久化/replay/Coherence 三重保证），非迁移步骤。当前默认路径经验证正确（容器 25+ 场景）。**主线的完成前置保持，热路径迁移不在本轮实施**（计划文档主线 1 注记同模式）。台账本节为评估记录。

### 1ae. 启动全面检查与问题修复（round 281；用户「启动软件，全面检查，问题修复」）

**检查范围**：单机实例 18080（启动）+ 容器 HA 对 18081/18082 + Prometheus/Alertmanager。发现并修复 **1 个真实缺陷**。

**缺陷 1（已修复）：重新分配的地址被吊销的旧持有者永久阻塞上推**
- **现象**：容器 primary `ready=degraded`、`dhcp` 平面 reason=`change_refused`、`refused_rows=2`、`dhcp_lease_dirty` 中 2 行 `UNIQUE constraint failed: dhcp_leases.scope_id, dhcp_leases.ip_address`。
- **根因**：`internal/dataplane/transfer.go` 的 `Push` 用 `INSERT ... ON CONFLICT(id) DO UPDATE` 上推租约。当 control 投影中同一 `(scope_id, ip_address)` 已有**另一 id 的 active 行**（地址被释放/过期后重新分配、或历史代残留），插入命中 partial unique index `idx_dhcp_leases_scope_ip_held` → 被拒 → 按 5 分钟退避永久重试。投影永远显示旧持有者、新租约永不上推（控制台漂移 + 永久 degraded）。
- **修复**：上推 active/offered 租约前，在同一 SAVEPOINT 内先 `UPDATE dhcp_leases SET status='expired' WHERE scope_id=? AND ip_address=? AND id<>? AND status IN ('active','offered')`（取代吊销的旧持有者），再 upsert；任一步失败则 `ROLLBACK TO` savepoint（避免"地址在投影中显示空闲而 lease store 仍持有"）。lease store 是权威，投影不持有其未持有的 active 行。
- **回归测试**：`internal/dataplane/push_retry_test.go::TestReissuedAddressSupersedesTheStaleHolder`——旧 active 行 + 新 id 同地址 → Push 成功、旧行 expired、恰好 1 个 held 行、marker 清除。修复前该测试失败（Refused=1）。
- **容器验证（自愈）**：重建镜像重启后，.241 在退避到期后自动取代旧行（723613cb→expired，b779791d active）；.240 清退避后同样自愈（86d90625→expired，4dbe8c38 active）；`dhcp_lease_dirty` 清零、`18081 ready=ok`、全部 5 个 plane ok。

**全面检查结论（其余均正常）**：`go test ./...` 0；`-race ./internal/dataplane` 0；20 浏览器套件 TOTAL_FAILS=0；`verify-e2e` 4+8 全过；API 读/写（DNS zone+record CRUD）200/created；Web UI + 资产 200；metrics 364 行；HA promising=1/redundant=1；Prometheus 3 target up；容器 DORA→facts(16)→DNS A 记录→线上 `nslookup` 应答正常；三服务日志无 ERROR（仅 dev 模式 encryption_key 两条预期 WARN）。

### 1ae-补：全面检查深化（round 282）——无新增缺陷

对 round 281 之后的服务做更深层功能核验，**未发现新缺陷**：
- **API 全面探测**：真实端点（dns zones/records、dhcp scopes/reservations/options/leases、ipam spaces/subnets/addresses+allocate+stats、users、settings、audit-logs、logs/audit、logs/login、system/backups、dns cache/forwarders/blocklists/allowlists/policies/tsig）全部 200。
- **写路径**：IPAM allocate（subnet stats 254 total/253 available）；备份 `full` 创建 46MB completed；settings 更新；user 创建→读→更新→删除全通过；DHCP scope 创建→读→删除全通过。
- **负向用例**：无 token 401 / 坏 token 401 / 错密码 401 / 畸形 JSON 400 / 缺 CSRF 401（提示"缺少CSRF令牌"）——均正确。
- **HA 一致性**：sequence 123=123、facts 16/16、shortfall=0；active leases 9=9（双节点一致）。
- **文档化边界复核**：`GET /api/v1/dhcp/ha` 返回 501（openapi 标注"DHCP HA 配置 API（当前版本未实现）"——预留扩展，非缺陷）；`/dns/listeners` 仅 PUT（无 GET，404 正常）。
- **自带 config.yaml 默认形态启动验证**：role=all、DHCP+DNS 启用——隔离数据目录+端口启动后 health/ready=ok、5 个 plane 全 ok、DHCP 监听 UDP 67、DNS 监听 UDP/TCP 53、真实 `dig www.ship-test.test @127.0.0.1:53` 应答 192.0.2.123、0 ERROR 日志；验证后关闭临时实例（端口已释放）。
- **审计与日志**：audit-logs 记录 dns create / dhcp create/delete；三服务 0 ERROR（仅 dev encryption_key 预期 WARN）。

## 1af. 全页面真人式巡检、会话分页缺陷修复与未完成任务详单（round 283–284）

### 1af.1 全页面真人式巡检（模拟人操作所有页面）
用 Playwright 以真人方式登录 → 遍历 **30 个真实页面**（dashboard / dns 9 页 / dhcp 4 页 / ipam 3 页 / admin 5 页 / logs 3 页 / settings 3 页 / tools / 3 个错误页）→ 每页检查渲染、console/pageerror、关键组件、截图；再对 7 个 CRUD 页点击「新建」打开对话框检查表单。**40 项检查中 36 项 PASS**，4 项 FAIL 逐一核实：
- `/403` `/404` `/500`：**巡检误报**——错误页是插画图片（文字内嵌在图片里，`innerText` 只有"返回首页"4 字），截图确认「Page not found / ERROR 404」+ 返回按钮完整渲染；修正巡检判定（按 img/svg 存在判定）后 3 项转为 PASS。
- `/dns/apps`：**已知产品缺口非缺陷**——`/api/v1/apps` 是文档化 501 预留端点（openapi 标注「应用市场（当前版本未实现）」），页面 `onMounted` 捕获异常并渲染明确「501 Not Implemented」空状态（`web-admin/src/views/dns/apps/index.vue` 注释+文档已记录）；console 的 501 是浏览器对 HTTP 5xx 的标准记录，非 JS 异常。
- **`/admin/sessions` 发现真实缺陷**（见 1af.2）。

### 1af.2 修复：会话管理页不分页（真人打开会卡顿）
- **现象**：巡检中 `/admin/sessions` 页面 `innerText` 达 **263248 字符**（其余页数百至数千）；`GET /api/v1/auth/sessions?page_size=500` 返回 **2798 行 / 1.1MB**——分页参数完全被忽略。
- **根因**：后端 `ListSessions`（`internal/api/handler/auth.go`）直接 `ListUserSessions(userID)` 返回该用户**全部未过期会话**（每次登录都留一条，2 小时窗口内累积数千条），`response.OK` 返回裸数组；前端 `listSessions()` 用 `get` 拿全量数组，`fetchSessions` 忽略 RemoteTable 传入的 `{page, page_size}` 并把 `meta.total=data.length` 伪装成分页，且页面给 RemoteTable 传了 `:show-pagination="false"`——全量数据一次性交给 TDesign 表格渲染，且用户**看不到翻页控件**。
- **修复（后端分页 + 前端完整翻页）**：
  1. `internal/auth/session.go` 新增 `ListUserSessionsPaged(userID, limit, offset) ([]Session, int64, error)`——同一过滤条件下 `COUNT(*)` + `LIMIT/OFFSET`，抽出 `scanSessions` 复用；保留原 `ListUserSessions` 给其他调用者。
  2. `ListSessions` 用 `response.ParsePagination`（默认 20、上限 100）+ `response.OKPaginated` 返回**当前页数组 + meta.total**（`data` 仍为数组——`auth_http_flow_test.go` 的形状断言不变）。
  3. 前端 `listSessions(params)` 改用项目既有 `getList` 分页契约；`fetchSessions(params)` 透传分页参数并使用 `result.meta.total`；去掉 `:show-pagination="false"` 让 pager 渲染；「撤销其他会话」改为循环翻页拉取全部会话（保持「撤销所有其他会话」语义，不被分页截断）。
  4. 回归测试 `internal/auth/session_paging_test.go::TestListUserSessionsPagedReturnsOnePageAndTheTotal`（7 会话 + 其他用户 + 过期行：total 只数本用户未过期、分页乱序正确、末页/越界页、token_hash 不外泄）。
- **修复验证**：API 从 1.1MB/2798 行 → **1.8KB/5 行 + meta.total=2799**；容器内 3 行 + total=34；真人式翻页：pager 显示「共 2805 条数据 / 141 页」，点击下一页后 20 行数据 ID 确实更换、0 JS 错误、页面字符数 263248 → **2235**；20 个浏览器套件 TOTAL_FAILS=0；Go 全量 + `verify-race` 全绿。

### 1af.3 详细检查：未完成实施任务详单（用户目标第 3 项）
系统盘点（STATUS §2/§3、计划 4 主线、B8.5 Runbook、openapi 501、contracts、代码 TODO、生产门禁），结论：**无物理设备前提下可安全实施的任务已全部完成**；剩余全部为有意保留的边界或需工作树之外输入，逐类列明：

| 类别 | 具体项 | 为何未完成 / 是否可在本环境实施 |
|---|---|---|
| 501 预留扩展（7 个端点） | `/sso` GET/PUT、`/cluster` GET/POST、`/apps` GET、`/apps/{id}/install` POST、`/dhcp/ha` GET | openapi 明确标注「当前版本未实现」、`TestReservedExtensionEndpointsAnswer501` 钉住、v0.9.0 计划要求保留至外部证据齐备；属产品/架构决策，非代码疏漏。**`/dns/apps` 页面已如实展示 501 空状态**。 |
| 候选未冻结（4 项） | 六轴兼容矩阵、旧四/五/六表→八表快照迁移路径、加固 DDL+同键 UPSERT 写路径、本机 DDL 加固参考结构生产升级 | 候选定义+行为回归已钉住，合同/版本决策需独立专家签字；`prodgates` 拒绝自行提升。 |
| B8.5 外部验收（5 行 A–E） | 真实 relay/Option 82、跨主机 DHCP→DNS、物理断电恢复、双主机分区+fencing/witness、生产 Prometheus 原始时间线 | 用户明确「不能提供真实物理设备、不得触碰生产」；容器演练为新证据层**非验收**，5 行状态保持「未验收」。 |
| 生产迁移评审门控 | 主线 1 默认 DHCP 热路径（WAL-backed LeaseStore 前置）、主线 2 WAL 事件流/legacy 复制流退役决策、主线 4 生产级多节点配置协调（transport/leader/quorum） | 计划注记明确：前者是事件溯源租约存储架构级建设、后者需产品决策与现场评审；容器证据不改变门。 |
| 独立授权/证据 | role/epoch/generation/fencing 独立授权、可信时间/权威尾、真实遗留库升级对账、ACK 语义迁移评审 | 需工作树之外输入；10 个 `ErrNoAuthority` 哨兵恒关闭（有意）。 |

**代码层无 TODO/FIXME/XXX**（Go 0 处；前端仅 `cluster.ts` 一处如实说明 `internal/cluster` 未实现）。

### 1af.4 文档状态漂移修正（本次检查 discovered）
发现并修正 6 处过时表述：STATUS 一句话结论（仍写「11 个场景 / 剩余全部外部输入」）、STATUS §2（DHCP 租约 schema 仍标 PROPOSAL「冻结前不写码」——实际已 FROZEN+实现）、STATUS §3 第 2 项（仍把 DHCP schema 列为待专家冻结）、STATUS §5 措辞、计划文档主线 3 注记（仍写「默认装配/DNS consumer 迁移按门控」+ 引用旧编号 §1aw-补33～37）、dossier §6 与台账 §1g（同类的待冻结表述）——全部改为「保留历史判断 + 当前状态注记」模式；`docs` 守卫 PASS。

### 1af.5 前端资产一致性回归（round 284 续）
`make verify` 的前端构建会刷新 `web-admin/dist` 的资产哈希；若只重新部署 `web/dist` 而不重建内嵌 `index.html` 的 Go 二进制，页面会引用旧 JS 而白屏（round 29 曾出现）。本轮 verify 后实测到该失配（二进制内嵌 `index-D2QmPq0x.js` vs 磁盘 `index-C-y4jdYz.js`）——**重建二进制 + 重启 18080** 后 html 引用与磁盘资产一致、HTTP 200；容器镜像内部自洽（18081/18082 的 html 资产 HTTP 200）。`docs` 守卫的 hashTree 对比在先捕获漂移（第三次证明其价值）。

## 1ag. 退役决策落地与 501 端点产品决策（round 285；用户「按建议执行」）

用户明确「不做物理真机验收、独立专家冻结签字」后，对上轮详单中两类剩余任务作出具体收口（B8.5 五行保持未验收、候选四项保持未冻结，均不动）。

### 1ag.1 生产迁移退役决策：可回滚开关 + 容器证据 + WAL 归档
- **开关落地**：`dhcp.legacy_dual_write`（`config.yaml`、`internal/config/config.go` 的 `DHCPConfig.LegacyDualWrite` + `DefaultConfig()` 默认 `true` + `GODDI_DHCP_LEGACY_DUAL_WRITE` env 覆盖）。`cmd/goddi/main.go` 三处条件接线：`writer.WithDNSSink(...)`（legacy 队列同事务写入）、`go dnsConsumer.Run(...)`（legacy 消费）；`DNSOutboxAtomic` 保持恒定 `true`——其语义是「facts 路径已负责 DNS 变更、handler 不得重复入 legacy 队列」，与 legacy 队列是否写入无关。**默认行为完全不变**（旧配置文件不含该键时由 DefaultConfig 兜底为 true）。
- **过程中修正一处语义误解**：初版把 `DNSOutboxAtomic` 也设为跟随开关，导致 facts-only 下 `factsCommitted=false`、handler 反而回退写 legacy 队列（实测 +1）。读 `handler.go:500-535/643/775` 后改回恒定 true 并加注释；重建验证后 legacy 队列零增长。
- **测试**：`internal/config/legacy_dual_write_test.go` 4 项（默认 true、旧配置文件无该键仍 true、env false→false / true→true 回滚、yaml 显式 false）。
- **容器双节点证据**（compose 加 `GODDI_DHCP_LEGACY_DUAL_WRITE: "${GODDI_DHCP_LEGACY_DUAL_WRITE:-true}"`）：
  - `false`（facts-only）：启动日志「legacy DNS outbox queue retired for this run」；DORA（fo2-host → .242）→ facts seq=30 → dns 水位=30 → A 记录 + **线上 `nslookup` 应答**；**legacy `dhcp_dns_events` 队列零增长（24→24）**；`/ready=ok`、5 个平面全 ok；HA 水位跨重启持久。
  - 回滚 `true`：无 retired 日志、legacy 队列恢复写入（24→25）、facts seq=31、dns 水位=31、A 记录 + 线上应答、ipam 水位跨重启持久（30）。
- **WAL 事件流归档决策**（计划主线 2 注记）：`internal/dhcp/lease/wal*.go` 17 个文件在生产**从未接线**（`SetDurableLeaseGate` 无调用者），决策为**保留为迁移模型产物、永不接线、不删除**；`gate=nil` 的「主写入已完成」语义恒定，ACK 持久性由 SQLite 事务提交承担。**暂不删除 legacy 代码**——删除不可逆，须待 soak 期证据留存后另定。

### 1ag.2 7 个 501 预留端点的产品决策
- **决策**（`docs/audit/RESERVED-EXTENSION-DECISION.md`）：`GET /dhcp/ha` **实施为只读状态 API**；其余 6 个（SSO×2、cluster×2、apps×2）**有意保留 501 为产品边界**，逐项写明不做的理由与重新评估条件（= 出现明确产品规格）。用户已排除物理验收/专家签字 → 「保留至证据齐备」的旧前提消失，清单从"悬着待办"变为"已决策边界"。
- **`/dhcp/ha` 只读 API**：`internal/api/handler/extension.go` 用带 `sync.RWMutex` 的 provider 注册（HA 未启用返回 `ha_enabled=false, role=standalone`）；`cmd/goddi/main.go` 在 `haEnabled` 时注册，返回 `role_in_force/state/redundant/promising/sequence/acknowledged_sequence/peer_applied_sequence/peer_applied_facts_sequence/applied_sequence` 等（**只读、不暴露 peer token**）；openapi 501→200。
- **三处守卫同步**（缺一不可，本轮实测）：`TestReservedExtensionEndpointsAnswer501`（7→6 项 + 新增 `TestDHCPHAStatusIsReadOnly` 的 200/只读断言）、`TestReservedExtensionsDocumentTheir501Contract`（openapi_test.go 的 501 描述表 7→6 + 新增 200 只读断言）、`TestEveryRegisteredAPIRouteIsDocumentedInOpenAPI`（路由/文档双向一致）。
- **实时验证**：18080（HA 关闭）返回 `{ha_enabled:false, role:standalone, state:disabled}`、PUT 405；容器 primary（18081）返回 `sequence=142=acknowledged=peer_applied, redundant=true, promising=true, state=primary`；standby（18082）返回 `applied_sequence=142, promising=false, state=standby, peer_last_seen`。
- **文档同步**：STATUS §1 后端行 / §3 第 5 项 / §5 措辞、dossier §6 第 7 项、计划主线 4 注记、`docs/CONFIG-REFERENCE.md`（配置键表 + env 表）。`docs` 守卫派生式要求新决策记录文件与 env 登记（第三次证明其价值），补齐后 PASS；`contracts` 守卫仍钉住 `cluster.ts` 不得发请求（dual-node 协调保留）。

### 1ag.3 定位
本轮全部为**沙箱内可回滚的收口**：开关默认 true（行为不变）、501 仅升级 1 个只读端点、未删除任何 legacy/WAL 代码、未提升任何 `ErrNoAuthority` 门禁、未触碰生产、未宣称 B8.5 验收。剩余未完成任务详单见 §1af.3，其中「生产迁移退役决策」与「501 是否实施」两类已按本节收口。

### 1ag.4 最终回归与资产一致性（round 285 收尾）
- **全矩阵**：`go test -mod=readonly ./...` 0；`make verify` OK；`make verify-race` OK（0 FAIL / 0 DATA RACE）；`make verify-e2e` bare 4 + fixture 8 全过；`make verify-browser` **20 套件 TOTAL_FAILS=0**；`docs`/`contracts`/`prodgates` 守卫 PASS；`git diff --check` 干净。
- **守卫第三次/第四次派生式捕获**：docs 守卫要求新决策记录文件存在、`GODDI_DHCP_LEGACY_DUAL_WRITE` 在 CONFIG-REFERENCE.md 登记（补齐后 PASS）；`TestReservedExtensionsDocumentTheir501Contract`（openapi_test.go）钉住 501 描述表，随 `/dhcp/ha` 升级同步为 6 项 + 200 只读断言（第三次 501 守卫，此前 rbac_coverage_test.go 已同步）。
- **资产一致性第 4 次**：`make verify-race: verify` 会重跑 `make verify` 并刷新 `web-admin/dist` 资产哈希；浏览器套件的 `docs` 守卫因此一次报 95 项漂移（其余 19 套件全 0）。收口顺序固定为「所有会重建前端的目标跑完 → `cp web-admin/dist/. web/dist/` → `go build` 二进制（内嵌 index.html）→ 重启 18080 → 重跑浏览器套件」；宿主与容器镜像最终均为 `index-CpUz3NeQ.js`（HTTP 200）。容器镜像亦按此重建，双节点 `ready=ok`、`/dhcp/ha` 仍返回 `ha_enabled=true, role=primary, state=primary`。

## 1ah. legacy 代码删除与前端 HA 状态页（round 286；用户「以下两项都做」）

### 1ah.1 ① 删除 legacy DNS 双轨代码（facts 链成为唯一 DNS 生产者）
用户授权执行上一轮特意保留的不可逆动作。**边界**：删除全部**生产写入/消费/推送代码路径与实现文件**；数据库表、迁移、marker 声明与 `DomainDDNS` 复制域常量**保留**（现有数据库的历史数据兼容、schema 守卫 `TestEveryMarkerTableIsInTheUpwardQueueList` 要求列表与表一致），并明确标注退役。
- **删除的实现文件**：`internal/dhcp/dns_outbox.go`（DNSOutbox/ScopeAwareDNSMutationSink）、`internal/dhcp/dns_consumer.go`（DNSConsumer）、`internal/dhcp/dns_outbox_test.go`；`DNSEvent`/`DNSEventAction` 类型提取到新文件 `internal/dhcp/dns_event.go`（`DNSLink.ApplyOne` 与 `DNSFactsConsumer` 仍依赖）。
- **删除的生产路径**：`server.go` 的 `outbox`/`outboxWake` 字段、`enqueueDNSEvent`、`SetOutboxWake`、`wakeOutbox`、management release / expire sweep 的 DNS teardown；`handler.go` 的 `factsCommitted` 变量与 3 处 legacy enqueue 块、`LeaseFactsMutationConfig.DNSOutboxAtomic` 字段；`facts_mutation.go` 的 `DNSMutationSink` 接口/`WithDNSSink`/同事务 legacy 入队；`main.go` 的 `dnsConsumer` 装配、`DomainDDNS` runner 域、`SetOutboxWake`、`legacy_dual_write` 开关与 `WithDNSSink`；`dataplane` 的 `PushEvents` 实现/调用与 `eventPushColumns`。
- **删除开关**：`config.yaml`/`config.go`/`CONFIG-REFERENCE.md`/`docker-compose.ha.yml` 的 `GODDI_DHCP_LEGACY_DUAL_WRITE` 全部移除（facts-only 恒定，无回滚开关——退役已完成）；`legacy_dual_write_test.go` 删除。
- **测试清理**：删除 4 个纯 legacy 测试文件（ddns_latency/ddns_linkage/outbox_wake/legacy_dual_write）；共享 helper 提取到 `linkage_helpers_test.go`；删除/改写引用已删符号的测试（authorship 2 个 PushEvents 测试、push_retry 1 个 case、lease_store 2 个 outbox 测试、ipam_linkage 的 WithDNSSink/DNSOutboxAtomic 断言改为 facts 断言、facts_mutation 的 2 个 legacy sink 测试 + fail 注入改为 caller 注入）；装配守卫 `TestDefaultProcessUsesFactsOnlyDNSConsumer` 钉住「无 NewDNSConsumer/WithDNSSink/SetOutboxWake/LegacyDualWrite/DomainDDNS」。
- **退役语义注释**：`sync.go`（DomainDDNS/ddnsTables）、`runner.go`（PendingDNSEvents）、`dns_facts_consumer.go`、`dns_event.go`、`facts/outbox.go` 均标注 round 286 退役。

### 1ah.2 ② 前端 HA 状态页 `/dhcp/ha`
- **页面**：`web-admin/src/views/dhcp/ha/index.vue` —— TDesign 卡片展示 `state/redundant/promising`（TTag 语义配色）、`node_id/configured_role/role_in_force/sequence/acknowledged_sequence/peer_applied_sequence/peer_applied_facts_sequence/peer_last_seen`；HA 未启用显示单机空状态；5 秒自动刷新（TSwitch 可关）+ 手动刷新；底部标注只读来源 `GET /api/v1/dhcp/ha`。
- **接线**：`service/api/goddi/ha.ts`（`getHAStatus` + 类型）；routes.ts / transform.ts / typings/elegant-router.d.ts / imports.ts / build/plugins/router.ts / layout 菜单与 PATHS / zh-cn+en-us 的 `route.dhcp_ha` 与 `haStatus.*` / app.d.ts 的 `Schema.haStatus` 全部同步（守卫派生：`31 个 TDesign 页面`、contracts 的 nav/meta/PATHS/浏览器套件覆盖）。
- **浏览器覆盖**：`batch5.cjs` 新增 3 项真实检查（页面渲染、只读 API 200 + `ha_enabled`、刷新按钮）——实测 PASS；全页面巡检 41/41（含 `/dhcp/ha`，`/dns/apps` 的 501 console 噪声按文档化预留端点过滤）。

### 1ah.3 验证
- `go test -mod=readonly ./...` 0；`make verify` OK；`make verify-race` OK（0 DATA RACE）；`make verify-e2e` 4+8；`make verify-browser` **20 套件 TOTAL_FAILS=0**；`docs`/`contracts`/`i18n`/`prodgates` 守卫 PASS；`git diff --check` 干净。
- **容器端到端**：重建镜像后 DORA（hc286-host → .241）→ facts seq=57 → dns 水位=57 → A 记录 + 线上 `nslookup` 应答；**legacy `dhcp_dns_events` 零增长（50→50）** —— 代码删除后 facts 链是唯一 DNS 生产者且行为不变；双节点 `/dhcp/ha` 返回 `primary/standby, seq=169`；容器 HA 页真实渲染（标题/primary/序列/来源，0 JS 错误）。
- **资产一致性**：`make verify-race: verify` 第 5 次刷新前端哈希，按固定顺序收口（部署 web/dist → 重建二进制 → 重启 18080 → 重跑浏览器套件）；宿主与容器镜像最终均为 `index-Sg3Fa-wh.js`（HTTP 200）。
- 三服务 health/ready ok、Prometheus/Alertmanager 200。

### 1ah.4 定位
退役决策（§1ag）中的「暂不删除 legacy 代码」与「前端 HA 页待产品决策」两项已按用户指令完成；**未触碰生产、未提升任何 `ErrNoAuthority` 门禁、未宣称 B8.5 验收**。数据库表与迁移保留为历史数据兼容，`DomainDDNS` 常量仅作 schema 兼容声明——如需彻底删表须另定迁移（影响现有部署的历史数据）。

## 1ai. 按新架构彻底重构：移除历史数据兼容层（round 287；用户「不需要按历史数据兼容的要求推进」）

在 §1ah（删除 legacy 生产代码）基础上，用户明确不再保留任何历史数据兼容层，按 facts-only 新架构把残留全部清理，直到代码库、schema、验证矩阵都不再含 legacy 概念。

### 1ai.1 代码层（Go）
- 删除 `DomainDDNS` 常量、`ddnsTables`、`TablesFor`/`HasReplica` 的 DDNS case、`PendingDNSEvents` 字段、`PendingByQueue` 的 `dns_events` 项、`PushEvents` 的 `TakeOverLeases` 表项、`upwardQueues` 的 `dhcp_dns_event_dirty` 声明、`PendingEvents()` 方法、`eventPushColumns`（§1ah 已删）与全部 "RETIRED / schema 兼容" 注释；`quota.go`/`facts/outbox.go`/`dns_facts_consumer.go`/`dns_event.go` 的措辞同步为"唯一 DNS 生产者"。生产代码里 `DomainDDNS`/`dhcp_dns_events` 残留 = 0。

### 1ai.2 schema 层（迁移链）
- **旧迁移移除 legacy 对象**：控制库 `018`（`dhcp_dns_events` 表/索引）、`022`（ddns 计数器种子、`trg_dp_rev_ddns_events_ins`、Down 的 DELETE）、`019` 注释；数据面 `001`（表/索引）、`002`（`dhcp_dns_event_dirty` 表 + 2 个触发器）、`005`（marker 的 3 个 ALTER）、`007` 注释。新库从零迁移**不再创建任何 legacy 对象**。
- **新增 DROP 迁移清理已有库**：控制库 `034_retire_legacy_ddns.sql`、数据面 `018_retire_legacy_ddns.sql`（`DROP TRIGGER/INDEX/TABLE IF EXISTS` + 删除 `dataplane_revision` 的 ddns 行）。两份文件带 `-- migration-contract: retire-allowed` 标记；`TestMigrationsFollowExpandContract` 增加**显式退役豁免**（只对 R2/R3 生效，普通迁移的破坏性语句仍被拒绝，负向用例 `TestTheContractCheckRejectsEachRuleBreakingMigration` 不变）。
- **实测**：全新三库（DHCP+DNS 启用）从零迁移后控制库 v34 / 数据面 v18，`%dns_event%` 表 = 0；已有库（18080 数据 + 容器卷）重启后控制库/dnsdata/leases 三库 legacy 表全部清空。

### 1ai.3 测试与验证矩阵同步
- 删除/改写所有依赖 legacy 表或 DomainDDNS 的测试：`authorship_test.go`（2 个 DDNS 复制测试 + helper、HasAnyReplica/TakeOverLeases 的域列表）、`sync_test.go`（takeover 期望从 2 行改为 1 行）、`store_test.go`/`runner_test.go`/`quota_test.go`（表列表与队列数 7→6→5 期望）、`legacy_upgrade_drill_test.go`（33→34 迁移）。
- **W 系列脚本按新架构重构**：`w14h_split_outage_drill.py` 的 B/C 节从 legacy DNS 事件改为**统一 facts 事件**（`dhcp_ipam_observation_events` + `dhcp_ipam_observation_event_dirty`，动态 sequence 避免 watermark gap），52/52 断言全过；`w07_revert_check.py` 移除 3 个指向已删代码的变异用例（41/41 全过）；`w14g_push_isolation_mutation_check.py` 的 2 个注入锚点重锚到 round 281 supersede 后的 Push 形状（8/8 全过）；`d1_ddns_latency_check.py`（依赖已删的 `ddns_latency_test.go` 与 `SetOutboxWake`）删除。全量 sweep 21+3 项收敛为全绿。

### 1ai.4 过程中发现并修复的 3 个真实缺陷（新架构下暴露）
1. **`DNSFactsConsumer` 发布记录后不 reload 内存 zone store** → DNS 记录写入 DB 但 UDP 查询 NXDOMAIN（`w14h` C 节实测 rcode=3）。`Replay` 结束时批量 `link.ReloadZones()`。
2. **`Replay` 把"记录未变"误当"队列已空"** → 一个幂等 no-op fact 会中断后续消费。`Tick` 改为返回 `(consumed, changed, err)`，`Replay` 用 consumed 判断继续、累计 changed 后一次 reload。
3. **split 部署 DNS 进程缺 `DomainFacts` 复制域**（facts 无法从控制库到 DNS 副本）→ 补上；但 all-in-one 进程会与 relay 双写同一 sequence（实测 `UNIQUE constraint failed: dhcp_ipam_observation_events.sequence`）→ 最终按 `servesDHCP` 门控：**仅 split DNS 进程**装配 DomainFacts，all-in-one 走 relay 单路径。

### 1ai.5 文档同步
计划 v0.9.0 主线 2 注记（原"暂不删除 legacy 代码"）、ADR-0007 状态与相关项、本节台账；历史计划 v0.6.0/v0.7.0 保留当时的历史判断（不重写历史），当前状态以本台账为准。

### 1ai.6 验证（全绿）
`go test -mod=readonly ./...` 0；W/d/b 脚本 sweep 全绿（含 w14h 52/52、w07 41/41、w14g 8/8）；`docs`/`contracts`/`i18n`/`prodgates` 守卫 PASS；`git diff --check` 干净；容器 all-in-one facts 链实测 DORA → facts seq=72 → ipam/dns 水位=72 → A 记录 + 线上 `nslookup` 应答；三服务 health/ready ok。**未触碰生产、未提升 `ErrNoAuthority` 门禁、未宣称 B8.5 验收**。

## 1aj. N5 P1 关闭/恢复装配收敛 + AsyncApplier drain 竞态修复（round 338）

按实施计划 §6.1 的推荐顺序 `P1 → P2 → P3 → P4 → P5`，本轮落地 **P1（N5 关闭/恢复装配收敛）**。默认 DHCP 仍为 SQLite（未切换），WAL/Facts 仍 opt-in，cluster 保持 501，未触碰任何 `ErrNoAuthority` 门禁。

### 1aj.1 新增：`WALPipeline` 外层装配契约（`internal/dhcp/lease/wal_pipeline.go`）
P1 之前，`OpenWAL` / `RecoverSQLiteProjection` / `NewSQLiteProjectionApplier` / `NewWALLifecycle` / `MemoryWriteBridge` 各自存在且单独测试，但没有一个把它们按正确顺序组装成 Start/Stop 单元的"外层装配契约"。本轮补齐：
- `OpenWALPipeline(ctx, cfg)` 的固定顺序：打开 WAL → **先**把 WAL 中未投影的 durable 事件重放回 SQLite 投影（fail-closed，返回 nil 前完成）→ 启动有序异步投影 applier → 构建 lifecycle 与写桥。任何一步失败即关闭资源并返回错误，调用方保持数据面未就绪。
- `Admit(ctx, Lease)`：内存先行 → WAL durable（`lifecycleSink` 让 `WALLifecycle.AppendDurable` 同时落 WAL 并提交 applier，规避双路径）→ 异步投影；durability 失败回滚内存且不提交。
- `Close(ctx)` = `WALLifecycle.Close`：admission 截止 → WAL Sync → applier DrainAndClose → WAL Sync → 关闭 WAL；超时/取消返回 ctx 错误并保留可重启重放的 durable 资源。
- 验收测试（`wal_pipeline_test.go`）：崩溃遗留的 durable 事件在 `OpenWALPipeline` 返回前被投影（启动恢复）；admit 后 durable 且投影；Close 排空已接受队列（10 条全投影、applied=10）；Close 后拒绝新 admit 且二次 Close 返回 `ErrWALLifecycleClosed`；坏配置 fail-closed；`cmd/goddi` 静态守卫 `TestDefaultProcessDoesNotAssembleWALPipeline` 证明默认进程不装配。

### 1aj.2 修复真实并发缺陷：`AsyncApplier.Wait` 提前误报排空
P1 验收测试（全量包连续运行）稳定复现了一个竞态：`Wait` 用 `len(queue)==0 && !inflight` 判断空闲，但 worker "已从 channel 取走事件、尚未在锁内置 `inflight=true`" 之间存在窗口，负载下 `Wait`/`DrainAndClose` 会在事件仍在 apply 时返回 nil——即 **drain 假报成功、Close 丢掉已接受事件的投影机会**（正是 P1 要钉的"accepted queue 不丢失 / drain 超时不假报成功"）。
- 修复：`AsyncApplier` 增加受锁保护的 `accepted`（已提交的最高序列），空闲判据改为 `applied >= accepted`；`Submit` 在入队前记录 `accepted`（队列满则回滚），`Wait` 在 `applied < accepted` 时继续等待，`closed` 且仍有未投影事件时返回 `ErrApplierClosed`（留给启动重放）。
- 回归测试：`async_apply_wait_test.go::TestAsyncApplierWaitWaitsForInFlightApply`（in-flight apply 期间有界 Wait 必须 DeadlineExceeded，放行后 Wait=nil、applied=1）；全量包 `-count=5` 连续通过，不再偶发失败。

### 1aj.3 定位与边界
- 变更全部为**新增 opt-in 组件 + 一处内部并发修复**，默认进程与 SQLite 热路径行为不变；`go test ./...` 0；WAL 相关包测试全绿。
- 剩余 P2–P5（旁路矩阵、N6 mutation 前置、灰度装配评审、N7/N8 验收包）与默认 DHCP 切换（计划 D2 门禁）仍保持未实施；真机/专家冻结等外部项不变。

## 1ak. N5 P2 默认旁路与 durable ACK 评审（round 339）

按计划 §6.1 顺序，P1 之后落地 **P2（N5 默认旁路与 durable ACK 评审）**：把每个改动租约状态或发送 ACK 的入口，逐一登记 owner / 事务边界 / facts 覆盖 / ACK durable 边界 / 失败语义，并用静态守卫把矩阵钉在源码上。默认 DHCP 仍 SQLite、WAL opt-in、cluster 501、门禁未动。

### 1ak.1 产物
- `docs/audit/N5-BYPASS-MATRIX.md`：三张矩阵（DHCP 报文路径、管理面路径、复制/投影路径）+ 只读路径 + 受控旁路与缺口结论 + 直写白名单。
- `internal/dhcp/server/lease_write_boundary_test.go` 两个守卫：
  - `TestLeaseWritesAreConfinedToTheDocumentedOwners`：扫描全仓非测试 `.go`，`INSERT/UPDATE/DELETE dhcp_leases` 直写只允许出现在 8 个已登记文件（权威 `lease.go`/`facts_mutation.go`/`facts_expiry_batch.go`/`wal_projection.go`、镜像 `ha/mirror.go`、受控级联 `scope/scope.go`、投影 `dataplane/sync.go`/`transfer.go`）；任何新增越界直写即失败，必须先在矩阵登记。
  - `TestDocumentedBypassMatrixStaysAccurate`：矩阵必须点名全部白名单文件，并保留「受控旁路」与「durableGate=nil」两条声明，防止文档与代码漂移。

### 1ak.2 评审发现
- **唯一非权威/非投影直写**是 `scope.DeleteScope` 的级联 `DELETE FROM dhcp_leases WHERE scope_id=?`：同一事务内先 `SELECT COUNT(*) ... status='active' AND 未到期`，>0 直接拒绝；只删非活跃历史行，故不产生 facts 是正确的受控旁路（owner=`scope.Manager`，失败整体回滚）。已登记而非静默。
- 其余直写均为权威写入者（facts seam）或复制/镜像/投影（非本地权威）。
- **durable ACK 缺口**如实登记：默认 `durableGate=nil`，ACK 持久性由 SQLite 事务提交承担；WAL durable gate 尚未接管默认装配（计划 D2 门禁）。不是遗漏。

### 1ak.3 验证
`go test ./...`（含两守卫）0；`make verify` / `make verify-race` 通过；`docs`/`contracts`/`i18n`/`prodgates` 守卫 PASS；`git diff --check` 干净。本轮仅新增测试与文档，运行时行为不变。

## 1al. N6 默认 mutation 接入前置（round 340）

按计划 §6.1 顺序，P2 之后落地 **P3（N6 默认 mutation 接入前置）**：把 7 类租约 mutation 的 facts 覆盖、EventID 身份来源、producer/consumer 存储归属、重放语义写清并用测试钉住。默认 DHCP 仍 SQLite、事实链仍 opt-in、门禁未动。

### 1al.1 产物
- `docs/audit/N6-MUTATION-PREREQUISITES.md`：7 类 mutation 覆盖矩阵（Offer/Bind/Activate/Renew/Release/Decline/Expire 的 To/DNS/IPAM/Generation/RequiresDurable/facts writer/默认是否发事实）、EventID 身份来源、producer/consumer 存储归属图（不宣称跨库单事务）、重放语义与既有测试索引、结论与门禁。
- `internal/dhcp/lease/mutation_coverage_test.go`：
  - `TestEveryMutationKindHasAContract`（7 类均有契约，未知 kind 拒绝）；
  - `TestOfferMutationPublishesNoFact`（OFFER 契约 DNS/IPAM=none、RequiresDurable=false → 默认 ReserveAddress 不发事实是正确的）；
  - `TestMappedMutationsRequireDurable`（Bind/Activate/Renew/Release/Decline/Expire 均 RequiresDurable=true）；
  - `TestFactIdentityIsRetryStableAndGenerationScoped`（同输入同 EventID、跨 generation/kind 区分、空 source/space 拒绝）；
  - `TestDuplicateFactEnqueueIsIdempotentAcrossRestart`（同信封重复入队幂等、同 id 冲突信封报错）。

### 1al.2 结论
- 6 类需持久 mutation 均有 facts writer 且 `RequiresDurable=true`；OFFER 有意不发事实（offer 非 binding）。
- 存储归属：lease 库生产者 outbox → relay 幂等上行 → 控制库 inbox → ipam/dns consumer + 双 domain watermark；**跨库不宣称单事务**，整体最终一致 + gap/lag 观测。
- 重放：重复幂等、乱序/缺口 fail-closed、重启从 watermark replay——均有既有测试 + 本轮新增幂等用例。

### 1al.3 验证
`go test ./...` 0；`make verify` / `make verify-race` 通过；`docs`/`contracts`/`i18n`/`prodgates` PASS；`git diff --check` 干净。本轮仅新增测试与文档，运行时行为不变。

## 1am. N6 统一事实链灰度装配评审（round 341）

按计划 §6.1 顺序，P3 之后落地 **P4（N6 统一事实链灰度装配评审）**：把默认进程的事实链装配图、opt-in/回滚开关、失败 consumer 的 Stop→重建→Start 恢复闭环、readiness lag/gap 观测逐项钉住，并映射到代码与测试。默认 DHCP 仍 SQLite、stage 组件仍 opt-in、门禁未动。

### 1am.1 产物
- `docs/audit/N6-GRAY-ASSEMBLY-REVIEW.md`：生产者 outbox → relay → 控制库 inbox → ipam FactsPipeline → DNS facts consumer 的装配图；opt-in/回滚表（pipeline Enabled／primary-only seam／WAL+gate 默认不装配／DNS 分支）；失败 consumer Stop→重建→Start 闭环与 Stop 超时不假报成功；readiness 三平面 lag/gap；P4 判据→测试映射表。
- `cmd/goddi` 源码守卫 `TestDefaultFactsChainAssemblyMatchesGrayReview`：要求 main.go 保留 facts 链装配标记（`ipam.NewFactsPipeline`、`NewDNSFactsConsumer`、DNS relay、Start/Stop、`SetLeaseFactsMutation`/`SetReleaseFactsMutation`），并禁止装配 `OpenWALPipeline`/`NewWALLeaseStore`/`SetDurableLeaseGate(`，保证文档装配描述与源码一致。

### 1am.2 结论
- 事实链 **consumer/pipeline** 在控制面默认装配；仍 opt-in 的是 **stage 组件**（WAL store / durable gate），默认不装配（已有守卫）。
- 失败 consumer 恢复闭环、Stop 超时不误报、lag/gap readiness 均已在 `internal/ipam/facts_pipeline.go` 实现并有既有测试（`TestFactsPipelineRestartsAfterConsumerFailure`、`...ConcurrentStopIsSafe`、`...ReportsLagAsDegradedWhileRunning`、`...ReportsGapAndFailedBacklogAsFailing`）。
- 默认生产接入（durable gate 接管 / WAL 热路径）仍属计划 D2 门禁，未做。

### 1am.3 验证
`go test ./...` 0；`make verify` / `make verify-race` 通过；`docs`/`contracts`/`i18n`/`prodgates` PASS；`git diff --check` 干净。本轮仅新增测试与文档，运行时行为不变。

## 1an. N7 协调协议评审 + N8 验收包（P5，round 342）

按计划 §6.1 顺序，P4 之后落地 **P5（N7 协调协议评审 + N8 验收包）**。N7 故障矩阵与 N8 证据归档模板此前已在 `docs/B8.5-外部验收Runbook.md` §9–§11 存在；本轮补齐**包边界纯契约守卫**并刷新进度，不改变任何 BLOCKED 结论。

### 1an.1 产物
- `internal/cluster/purity_test.go::TestProtocolModelIsAPureContract`：断言 `internal/cluster` 非测试源码无 `net`/`net/http`/`net/rpc`/`os/exec`/`database/sql`/`syscall`/`grpc` 导入，无 `http.Handler`/`RegisterRoutes`/`grpc.NewServer`/`Serve(` 服务入口，且 `ErrNotImplemented` 仍在——防止把进程内协议模型误接成生产 transport/HA（B8.5 §9.3 边界）。
- `docs/B8.5-外部验收Runbook.md`：§9.2 增加「包边界纯契约」一行映射到上述守卫；新增 §12「round 338–342 更新」，串联 P1–P5 台账章节，并重申 §9.1/§10 的 BLOCKED 结论不变。

### 1an.2 结论
- N7 协议模型的 fail-closed 契约（stale epoch/minority/fenced leader/publish 幂等/unknown 查询/watermark gap/fence→clean→rejoin）已有 8 个模型测试 + 本轮包边界守卫；仍只归类 `experimental/incomplete`，**不是**生产 HA。
- cluster 通用 API 保持 `501`（`ErrNotImplemented` + `TestReservedExtensionEndpointsAnswer501` + `contracts.cjs` cluster.ts 不发请求）。
- N8 验收包模板（§10）与 BLOCKED 判据保留；真机/专家冻结等外部项不变。

### 1an.3 验证
`go test ./...`（含 cluster 守卫）0；`make verify` / `make verify-race` 通过；`docs`/`contracts`/`i18n`/`prodgates` PASS；`git diff --check` 干净。本轮仅新增测试与文档，运行时行为不变。

## 1ao. D2 热路径切换 Stage 1：WALLeaseStore 全语义 + 平价（round 343）

用户授权 D2 后，按 `docs/audit/D2-HOT-PATH-SWITCH.md` 分阶段推进。本轮完成 **Stage 1**（存储本体 + 平价，未切默认）。

### 1ao.1 产物
- `internal/dhcp/lease/wal_store.go`：`WALLeaseStore` 实现 `server.LeaseStore` 全 11 方法——MemoryIndex 权威读，`WALPipeline.Admit` 做 WAL 持久 + 异步 SQLite 投影，审计复用 Manager 的 `auditBind/Release/Decline/Expire`；`OpenWALLeaseStore` 先 `RecoverSQLiteProjection` 再装载 `dhcp_leases` + 作用域/保留地址到索引；`Close`/`Drain`/`Index`。
- `internal/dhcp/lease/memory_index.go`：新增 `Snapshot()`。`internal/dhcp/lease/wal_pipeline.go`：新增 `Drain(ctx)`。
- `internal/dhcp/server/lease_store.go`：`var _ LeaseStore = (*lease.WALLeaseStore)(nil)` 编译期契约。
- 测试 `wal_store_parity_test.go`：`TestWALLeaseStoreMatchesManagerSemantics`（Reserve→Activate→Renew→Reserve→Release→Quarantine→Create→Expire→FindAvailableIP 全流程，Manager 与 WALLeaseStore 语义行逐行相等）、`TestWALLeaseStoreRestartRebuildsIndex`（关闭重开重建索引并继续变更）。

### 1ao.2 结论/边界
- Stage 1 只是存储本体与平价，**默认仍 SQLite**；facts 同源、config 灰度、装配、崩溃/回滚演练属 Stage 2–4，尚未做。
- 未触碰生产、未提升门禁。

### 1ao.3 验证
`go test -mod=readonly ./...` 0；`TestWALLeaseStore*` `-count=3` 稳定；lease/server 包绿。

## 1ap. D2 热路径切换 Stage 2：facts 同源 + 配置 + 装配（round 344）

### 1ap.1 产物
- `WALEvent` 增加 `Before`/`SpaceID`；`WALPipelineConfig.Apply`（自定义投影 applier）；`WALLeaseStore` 投影事务内用 `SequenceAllocator.NextTx` + `ObservationOutbox.EnqueueTx` 派生 facts（复用 `MutationCommand.FactEnvelope`；OFFER 不发；space 在 admit 解析，ErrNoRows 不发事实、其它错误请求失败）。
- 配置 `dhcp.lease_store: sqlite|wal` + `dhcp.lease_wal_path`（默认 `<data_dir>/leases.wal`）+ `GODDI_DHCP_LEASE_STORE`/`GODDI_DHCP_LEASE_WAL_PATH` 环境映射。
- `main.go`：wal 分支 `OpenWALLeaseStore` + `NewWithLeaseStore`，关闭旧同步 facts seam/observer；HA+wal 暂拒绝（守卫 error）。
- 测试：store 级 facts 三例 + Server 级 `TestWALLeaseStoreServesDORAAndEmitsFacts`（真实 DISCOVER/REQUEST→ACK、投影 active、fact `request_activate/node-a`）。
- 冒烟：wal 进程 boot ok（`event-sourced WAL`、WAL 文件、ready ok）；顺带修复 sqlite 路径 `source` 变量遮蔽 bug（曾致启动期 expiry 报 facts 配置不全）。

### 1ap.2 边界
- 默认仍 `sqlite`；`wal` 为显式 opt-in，HA 下拒绝（HA 交互/灰度/崩溃/回滚演练属 Stage 3–4）。未提升门禁、未触碰生产。

### 1ap.3 验证
`go build ./...` / `go test ./...` 0；wal/sqlite 两种进程 boot 冒烟健康。

## 1aq. D2 热路径切换 Stage 3/4：灰度 + 崩溃/回滚演练（round 345）

### 1aq.1 产物
- `scripts/d2_wal_canary_drill.sh`：可复现的灰度+崩溃+回滚演练（独立非 HA 容器节点跑 `lease_store=wal`，真实 `udhcpc` DORA；SIGKILL 重启；同数据目录回滚 sqlite）。当前 `PASS=9 FAIL=0 DRILL: OK`。
- Go 级确定性演练：`wal_store_drill_test.go::TestWALLeaseStoreReplaysUnprojectedMutationsAfterCrash`（崩溃后未投影事件补投影租约+补发 facts 且不重复）、`TestWALLeaseStoreRollbackToSQLiteManagerSeesTheSameData`（回滚 `*Manager` 读到同样租约）。
- 修复：`OpenWALPipeline` 在有自定义 `Apply` 时，启动恢复改为经 `wal.Replay(applied, cfg.Apply)`，使崩溃后未投影事件的 facts 也能补发（此前 recovery 只投影租约）。

### 1aq.2 结论/边界
- 灰度：独立非 HA wal 节点真实 DORA→facts→DNS 可用；崩溃(SIGKILL)重启后仍服务且 WAL replay 生效；同数据目录回滚 sqlite 后仍服务、WAL 分支不装配。
- **未完成**：HA+wal（standby 索引同步未实现，故 HA 下仍拒绝 wal）；全局默认未切到 wal（当前默认 sqlite，靠 `dhcp.lease_store` 逐节点灰度）；断电仅 SIGKILL 近似。
- 未提升门禁、未触碰生产。

### 1aq.3 验证
`go build ./...` / `go test ./...` 0；`make verify-race` OK（0 DATA RACE）；`scripts/d2_wal_canary_drill.sh` DRILL: OK。

## 1ar. D2 HA 主备灰度：primary=wal / standby=sqlite（round 347）

### 1ar.1 产物
- `docker-compose.d2-canary.yml`：仅把 HA **primary** 置 `GODDI_DHCP_LEASE_STORE=wal`，standby 保持 sqlite。
- `scripts/d2_ha_wal_canary_drill.sh`：可复现 HA 灰度+崩溃+回滚演练，当前 `PASS=17 FAIL=0 DRILL: OK`。
- `cmd/goddi/main.go`：移除"HA 下拒绝 wal"的硬守卫——standby `standsAside` 不构造 DHCP server（镜像直写其 SQLite 库），primary 可跑 WAL store；takeover 重启式，promoted 节点由 WAL store 从已镜像 SQLite 快照装载索引。

### 1ar.2 结论
- 一节点切 wal：primary 真实 DORA→lease+fact+DNS、standby sqlite 镜像同租约、双节点 `/ready` ok。
- SIGKILL primary：standby 不自动提升（fail-closed）、停机无租约；重启 wal primary 后 WAL replay 保留租约、新 DORA 正常。
- 回滚 sqlite 同卷：健康、WAL 未装配、租约数不变、新 DORA 正常。
- 边界：仍为容器双节点（非物理 HA/断电）；`/ready` 在合成/演练环境受控制库复制影响可能降级（演练已验证关键行为）。全局默认仍 sqlite（`dhcp.lease_store` 逐节点灰度），默认切换与全矩阵回归留待下一步。

### 1ar.3 验证
`go test ./...` 0；`make verify-race` OK（0 DATA RACE）；`scripts/d2_ha_wal_canary_drill.sh` DRILL: OK；`scripts/d2_wal_canary_drill.sh` DRILL: OK。未提升门禁、未触碰生产。

## 1as. D2 默认切换：shipped 默认 lease_store=wal + 全矩阵（round 348）

### 1as.1 变更
- `config.yaml`：`dhcp.lease_store` 默认设为 **`wal`**（注释给出 `sqlite` 回退）。参考 HA 部署（`docker-compose.ha.yml`）默认 **primary=WAL / standby=sqlite**。
- `scripts/e2e_local.sh`：e2e 前端实例显式 `GODDI_DHCP_LEASE_STORE=sqlite`（该套件只测 API/UI，不测租约后端；WAL 由 D2 演练+参考 HA 默认覆盖），保证 CI 稳定。
- 回滚开关：`dhcp.lease_store: sqlite` 或 `GODDI_DHCP_LEASE_STORE=sqlite`。

### 1as.2 证据
- 参考 HA 默认（primary wal）实测：真实 udhcpc DORA→ lease active + fact(`request_activate@102`, 载荷含该 IP) + DNS `d2def.ha.test` + standby(sqlite) 镜像同租约；双节点 `/ready` ok，primary `event-sourced WAL`=1、standby=0。
- **全矩阵（默认 wal 下）**：`make verify` OK；`verify-e2e` 4+8；`verify-browser` 20 套件 0 失败；全页面巡检 41/41；`verify-race` OK（0 DATA RACE）；`docs`/`contracts`/`i18n` PASS；`git diff --check` 干净；宿主与容器资产统一。

### 1as.3 结论
默认热路径已切到 WAL 事件溯源存储；灰度（独立节点 + HA 主备单节点）、SIGKILL 崩溃重放、同卷回滚均有可复现演练（`scripts/d2_wal_canary_drill.sh`、`scripts/d2_ha_wal_canary_drill.sh`）；未触碰生产、未提升 `ErrNoAuthority` 门禁。边界：断电为 SIGKILL/容器重启近似（非物理断电）；HA 演练为容器双节点（非物理机）；standby 常驻为 sqlite 镜像，promoted 节点靠重启加载 WAL。

## 1at. 提交 + verify-tidy + B8.5 现场验收包 + standby-wal 就绪（round 349）

### 1at.1 提交与模块卫生
- `make verify-tidy`：**OK**（`go mod tidy -diff` 无漂移）。
- 把本会话全部源码/测试/文档/迁移/前端改动提交为 **`3c385b2`**（754 files, +66693/-19841）；构建产物 `.ha-goddi-bin`/`.ha-web-dist/` 加入 `.gitignore`，提交后工作树干净。

### 1at.2 B8.5 现场验收包
- `docs/B8.5-现场验收包/`：`README.md`（使用步骤+场景/判据摘要）、`evidence-template.txt`（逐场景登记模板）、`RESULTS.md`（结果登记表，预填 BLOCKED）。
- `scripts/b85_collect_evidence.sh`：`baseline` 采集版本/提交/二进制 sha256/配置；`collect <scenario>` 采集 `/metrics`、`/ready`、并提示 pcap 采集命令。**不启停任何服务**。
- 仍不替代真机证据：B8.5 A–E 未执行前保持 BLOCKED。

### 1at.3 standby-wal 就绪
- `WALLeaseStore.ReloadFromDB(ctx)`（从镜像已写的 SQLite 重载索引）+ 测试 `TestWALLeaseStoreReloadFromDBServesMirrorPopulatedRows`。
- 语义：standby 常驻不跑 DHCP server（`standsAside`），被提升（重启式）由 `OpenWALLeaseStore` 从已镜像快照装载；`ReloadFromDB` 供现场非重启提升。常驻 WAL 写 standby 未实现且不需要。

### 1at.4 验证
`go test ./...` 0；`make verify-tidy` OK；`docs`/`contracts`/`i18n`/`prodgates` PASS；`git diff --check` 干净。

## 1au. DNS 双机（方案 A：标准主从）实现与详细测试（round 351）

### 1au.1 修复的真实缺陷（使主从真正可用）
- `internal/dns/transfer/secondary.go` `axfrFromPrimary`：AXFR 请求未加 TSIG RR → miekg 不签名 → 实际未认证。修复：`m.SetTsig(dns.Fqdn(cfg.tsigName), algo, 300, now)`。
- `internal/dns/server/handler.go` `serveZoneTransfer`：AXFR/IXFR 响应未加 TSIG RR → miekg 不签名 → 严格 secondary 报 `dns: no signature found`。修复：每条响应 `SetTsig(req 的 key/algo)`，首条后 `TsigTimersOnly(true)`。

### 1au.2 测试
- `internal/dns/transfer/primary_secondary_e2e_test.go`：真实 loopback TCP + TSIG 的 primary（`AXFRHandler`+`dns.Server`）+ 真实 `SecondarySync`，覆盖初始 AXFR、变更再同步、NOTIFY 刷新、无 TSIG/错密钥 fail-closed。
- `internal/dns/server/handler_axfr_tsig_test.go`：产品 `serveZoneTransfer` 每条响应带 TSIG、首条后 timers-only。
- `go test ./internal/dns/...` 全绿。

### 1au.3 文档/边界
`docs/audit/DNS-PRIMARY-SECONDARY.md`（配置步骤/transfer_policy 格式/修复/测试/边界）。仍为方案 A：写单点、无自动选主、`/cluster` 501、secondary 客户端同步走 AXFR。

## 1av. DNS 方案A：CI 演练目标 + 两容器 dig 现场版（round 355）

- `make verify-dns-drill`：跑 loopback 演练（`TestPrimarySecondary*`），无需 Docker/外部 dig。
- `scripts/dns_primary_secondary_container_drill.sh`：两真实 DNS 容器（复用产品 `AXFRHandler`/`SecondarySync`）+ `dig` 客户端容器，实测 TSIG AXFR 复制、无签名 AXFR fail-closed、停 primary 后 secondary 仍应答；`PASS=5 FAIL=0 DRILL: OK`。
- 已知缺口：应用代码无写入 `dns_zone_transfer` 的路径（AXFR TSIG 授权绑定），纯 UI 部署无法授权 AXFR；建议后续补管理 API/UI。演练 harness 直接写入该绑定。

## 1aw. DNS 方案A：产品化传送授权 API + CI 容器目标（round 357）

- 新增 `dns_zone_transfer` 管理 API：`GET/POST /api/v1/dns/zones/{id}/transfers`、`DELETE .../{transferId}`（`internal/api/handler/dns_zone_transfer.go` + 路由 + OpenAPI）。校验 CIDR（裸地址补 /32|/128）、TSIG key 存在；写控制库并随数据面复制到 DNS 平面。
- 单测 `dns_zone_transfer_test.go`（建/列/删 + 负例）通过；OpenAPI 路由覆盖测试通过。
- 实机（18080）：API 建区/TSIG/授权后，产品 `SecondarySync` 从 127.0.0.1:15353 AXFR 成功（records=3, sampleA=192.0.2.77）。
- 已知限制：TSIG 密钥启动时加载一次，新增 key 需重启 DNS 进程才生效（文档已记）；UI 未加（API 就绪）。
- `make verify-dns-drill-container`：容器版演练入口（需 Docker）。

## 1ax. ADR-0010 落地 E-1：Owner Epoch 写栅栏（round 358）

### 1ax.1 变更（代码）
- `internal/dhcp/ha/protocol.go`：`Watermarks` 增 `Epoch`；新增 `ErrStaleEpoch`。
- `internal/dhcp/ha/role.go`：新增 `metaOwnerEpoch="ha_owner_epoch"` 与 `OwnerEpoch(store)`；`Takeover` 在同一事务内单调 `+1` 并写入 `TakeoverOutcome.Epoch`（审计含 epoch）。
- `internal/dhcp/ha/replicator.go`：加载/上报 epoch；`observeStandbyEpoch` 见更高 epoch → `superseded` → `StateFenced`/`MayBind=false`；握手交换 epoch。
- `internal/dhcp/ha/mirror.go`：加载/上报 epoch；`observePrimaryEpoch` 拒绝更低（陈旧）primary、采纳更高 epoch 并持久化。

### 1ax.2 测试
`internal/dhcp/ha/epoch_test.go`：4 例（takeover 单调递增、replicator 被更高 epoch 围栏、忽略更低 epoch、mirror 拒绝陈旧/采纳更高）。`go test ./internal/dhcp/...` 全绿。

### 1ax.3 边界
E-1 为**单进程内**围栏（写路径经 `MayBind` 抑制）；真正的"跨节点证明唯一所有权/自动 degraded"仍需 E-2（mTLS）与 E-3（quorum/witness）。

## 1ay. ADR-0010 落地 E-2：复制通道 mTLS + 防重放（round 359）

### 1ay.1 变更（代码）
- `internal/config/extensions.go`：`DHCPHAConfig` 增 `tls_cert_file/tls_key_file/tls_ca_file/tls_server_name`；`config.go` 增对应 `GODDI_DHCP_HA_TLS_*` env；`dhcpha.go` 校验 TLS 三项 all-or-none。
- `internal/dhcp/ha/config.go`：`Config` 增 `TLS *tls.Config`、`TLSServerName`。
- `internal/dhcp/ha/transport.go`：`LoadTLS`（证书+CA roots）、`serverTLS`（`RequireAndVerifyClientCert`）、`clientTLS`（ServerName 回退 peer host）。
- `replicator.go`/`mirror.go`：监听 `tls.NewListener`、拨号 `tls.Dialer`；hello 增 `Nonce/SentAt`，TLS 路径校验新鲜度（`validateHelloFreshness`）。
- `cmd/goddi/main.go`：HA 启用且 `tls_cert_file` 非空时 `ha.LoadTLS` 注入 `haCfg.TLS`。

### 1ay.2 测试
`internal/dhcp/ha/tls_test.go`：mTLS 往返成功、明文客户端被拒、错 CA 被拒（2 例）。`go test ./internal/dhcp/ha ./internal/config ./internal/dhcp/server` 全绿；`docs` 守卫 PASS。

### 1ay.3 边界
TLS 为**可选**（nil=旧明文+token，兼容既有部署）；生产环境必须配置 `dhcp_ha.tls_*`。证书**自动轮换/吊销编排**仍需现场 PKI（未实现，NOT_VERIFIED）。

## 1az. E-3 quorum/witness：合理性评估（BLOCKED，round 360）

- **结论**：真实 quorum/witness 需要第三仲裁节点/外部见证；本沙箱无此环境，无法实现并验证。为避免"伪见证"造成的虚假安全感（违反不得伪造证据），**不实现自动 degraded**；默认 `paused` 保持（符合 ADR-0010）。
- **前置已完成**：E-1（Owner Epoch 写栅栏）、E-2（mTLS+防重放）已实现并有单测；E-3 一旦有见证环境，可在此之上实现"自动 degraded 仅当 epoch 最新 + witness 证明"。
- **状态**：BLOCKED（external-validation-required），实施步骤与判据见 09 计划 E-3。

## 1ba. D-03 facts 哈希链：阶段一（内核+校验器）round 361

- `internal/facts/event.go`：`Envelope` 增 `Epoch/PrevHash/PayloadHash/TipHash`（`HasChain()`），向后兼容（旧事件空值不强制）。
- `internal/facts/chain.go`：`PayloadDigest`、`ComputeTip(prev,env)`（绑定 prev/epoch/eventID/sequence/payloadHash）、`ChainBuilder.Stamp`、`VerifyEnvelope`/`VerifyChain`（检测篡改/乱序/丢事件/epoch 回退）、`ChainTipStore`（`facts_chain_tip` 表，CAS `AdvanceTx` 防并发分叉）+ `StampTx`（事务内 load-stamp-advance）。
- `internal/facts/chain_test.go`：4+1 例全绿。
- 阶段二（未完成）：生产 outbox `EnqueueTx` 同事务调用 `StampTx`、IPAM/DNS 消费者调用 `VerifyEnvelope` 并在不一致时进入 `blocked`；需 outbox 表增列（hash 字段）与双库迁移滚动。完成前不宣称端到端哈希链生效。

## 1bb. D-11 DHCP 租约幂等/重复/并发（round 362）

- 新增 `internal/dhcp/lease/idempotency_test.go`：DISCOVER 重传幂等（同 ID/单行）、异 MAC 抢占返回 `ErrAddressTaken`、重复 Activate/Release 幂等、并发同址仅一胜者。
- **修复真实缺陷**：`internal/dhcp/lease/lease.go` `createLeaseWithStatus` 把并发插入触发的 `UNIQUE constraint failed: dhcp_leases.scope_id, dhcp_leases.ip_address` 映射为 `ErrAddressTaken`（原为裸错误，调用方无法按"地址被占"语义重试）。
- `go test ./internal/dhcp/lease ./internal/dhcp/server` 全绿；`go build ./...` OK。
- D-10（outbox 故障注入）：现有覆盖已含 enqueue 幂等/冲突、replica 快照原子替换/校验失败/断点、consumer gap/失败留箱；relay 中断与 DLQ 仍 NOT_VERIFIED。

## 1bc. D-09 IPAM 投影缺口检测 + 可选严格栅栏（round 363）

- `internal/ipam/projection.go`：`ProjectionGaps(ctx,db)`（enabled scope 子网无 `ipam_subnets.cidr` 即缺口；非阻塞）、`NewProjectionGuard(db)`（scope 子网无投影 → `ErrProjectionMissing`）。
- `internal/dhcp/server`：`SetScopeProjectionGuard`；handler 在 MayBind 后、提交前调用（仅当注入），失败静默不 ACK。
- 配置：`dhcp.require_ipam_projection`（默认 false）+ `GODDI_DHCP_REQUIRE_IPAM_PROJECTION`；CONFIG-REFERENCE 已加。main：启用时注入 guard；始终启动打印缺口计数告警（非阻塞）。
- 测试：`internal/ipam/projection_test.go`（缺口检测仅列未投影且启用 scope；guard 拒绝未投影、放行已投影）。`go test ./internal/ipam ./internal/dhcp/server ./internal/config` 全绿。
- 边界：跨面"相对控制面滞后"需 E-3/ADR-0005 水位，本地面不能证明；严格拦截默认关闭以免误伤独立 scope。

## 1bd. D-10 relay 故障注入/幂等（round 364）

- 新增 `internal/facts/relay_test.go`：relay 按序投递、重 tick 幂等、重启从水位续传不重复、inbox 不可用时 tick 失败但源不丢/水位不动、恢复后恰好一次；并断言 relay 不修改 source 行状态。
- 既有 outbox/replica/consumer 测试覆盖 enqueue 幂等/冲突、快照原子、gap、失败留箱。
- `go test ./internal/facts ./internal/ipam` 全绿；`go build ./...` OK。
- 仍缺：显式 DLQ（失败留 inbox 重放）；consumer apply 与 MarkDone 间崩溃的端到端演练（同库同事务，风险低）。

## 1be. D-08 SQLite 存储/IO/锁错误分类（round 365）

- `internal/sqlitepolicy/errors.go`：`ClassifyError(err)` 把 `Code()==13/10/5/6`、`syscall.ENOSPC/EIO`、常见文本映射为 `ErrStorageFull/ErrStorageIO/ErrBusy`（未识别原样返回）。
- `internal/dhcp/lease/lease.go` `createLeaseWithStatus`：非 UNIQUE 写错误经 `ClassifyError` 包装；写失败返回错误 → handler 不构建 ACK。
- 测试 `internal/sqlitepolicy/errors_test.go`（8 类 + nil/透传）通过；`go test ./internal/sqlitepolicy ./internal/dhcp/lease ./internal/dhcp/server` 全绿。
- 边界：真实填盘 ENOSPC / fsync 失败注入与 WAL Sync 错误路径仍 NOT_VERIFIED（需隔离盘/故障注入）。

## 1bf. D-14 dataplane LKG 版本戳 + 原子快照（round 366）

- `internal/dataplane/sync.go` `apply`：在配置行与 `dataplane_sync_state` 同一事务内调用 `writeLKG`，写 `dataplane_meta` 键 `lkg_revision:<domain>`、`lkg_applied_at:<domain>`。
- `internal/dataplane/lkg.go`：`writeLKG(tx,domain,revision)`、`LKG(db,domain)`（读取 revision/时间/是否已应用）。
- 测试 `internal/dataplane/lkg_test.go`：同步后 LKG revision == 控制 `dataplane_revision`、时间非零；无变更同步不改变戳。
- 原子性：`apply` 单事务覆盖各表 + sync_state + LKG + Commit。
- 边界：显式"回滚到指定 LKG revision"未实现；"集群全部应用"三态仍 NOT_IMPLEMENTED（D-05）。`go test ./internal/dataplane` 全绿。

## 1bg. D-20 指标暴露 + D-18 基准脚本（round 367）

- D-20：`internal/metrics/metrics.go` 增 `DataPlaneLKGRevision{domain}`、`DHCPIPAMProjectionGaps` gauge 并注册；`cmd/goddi/main.go` 启动时按 `dataplane.LKG` 与 `ipam.ProjectionGaps` 置值（缺口即使为 0 也设置）。
- D-18：`scripts/bench.sh`（`go test -run '^$' -bench . -benchmem ./internal/...` 输出时间戳报告）+ `make bench`；已烟测 `internal/dns/zone` benchmark 可运行（`BenchmarkStoreLookup_*`）。
- 回归：`go build ./...` OK；`go test ./internal/metrics ./internal/dataplane ./internal/dhcp/server` 全绿；docs 守卫 PASS。
- 边界：指标为启动快照（非周期/实时）；soak/并发压测未执行（NOT_VERIFIED）。

## 1bh. D-12 全量恢复演练 + D-20 周期刷新（round 368）

- D-12：`internal/backup/restore_full_test.go`——full 备份→删除 DNS/DHCP/IPAM 行→`RestoreBackup`→断言六表恢复（dns_zones/dns_records/dhcp_scopes/dhcp_reservations/ipam_spaces/ipam_subnets）。既有 dns/security 恢复测试。升级路径/大库/停机演练仍 NOT_VERIFIED。
- D-20：`cmd/goddi/main.go` 在 `backgroundCtx` 下每 30s 刷新 `goddi_dataplane_lkg_revision` 与 `goddi_dhcp_ipam_projection_gaps`（启动亦置值）。
- 回归：`go build ./...` OK；`go test ./internal/backup ./internal/metrics` 全绿；docs 守卫 PASS。

## 1bi. D-03 阶段二第 1 步：双库迁移 + outbox/replica 透传（round 369）

- 迁移：`internal/dataplane/migrations/019_facts_hash_chain.sql`、`migrations/035_facts_hash_chain.sql` 为 `dhcp_ipam_observation_events` 增列 `epoch/prev_hash/payload_hash/tip_hash`（默认 0/''），Down 可回滚。
- `internal/facts/outbox.go`：Enqueue 写入四列；重复冲突比较纳入四列；`Pending`/`After` 读取四列到 Envelope。`replica.go` 读取四列。
- 测试 schema（`outbox_test.go` newFactsOutboxDB）同步增列。
- 默认空值→现有行为不变；`go test ./internal/facts ./internal/dataplane ./internal/database` 全绿；`go build ./...` OK。
- 待做（第 2/3 步）：`facts.hash_chain_enabled` 开关 + 生产 outbox `StampTx` 打戳；IPAM/DNS 消费者 `VerifyEnvelope` + 断链 `blocked`。
