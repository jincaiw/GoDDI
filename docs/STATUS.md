# GoDDI 实施状态总览（交接单页）

> 本文件是「根据开发文档，完成全部实施任务」的**当前状态索引**，不是生产验收声明。完整证据见 `docs/audit/IMPLEMENTATION-STATUS.md`（§1 当前验证矩阵、§1b 及后续各轮补记（截至 round 392）、§1g 完成边界），独立评审输入见 `docs/audit/ENTERPRISE-REVIEW-DOSSIER.md`。
> 范围约定（用户口径）：**先单机实现所有功能，双机（HA / 复制 / 多节点协调）从技术架构角度预留，最后实现**；不得触碰生产、不得提升受保护写门禁。
> 证据范围：隔离单机（`/tmp/goddi-run` 临时 DB / 配置 / 非生产网卡）+ **容器化双节点环境**（2 Docker 容器独立网络/数据卷——HA/facts 链证据层，round 246–270）；均**不构成** Linux 物理双主机、物理断电、生产网络分区或生产环境证据（B8.5 A–D 未验收）。

## 一句话结论

**单机全部功能已完成并反复复跑；双机以 2 容器双节点演练形成新证据层（round 246–282，25+ 场景：HA 生命周期、SIGKILL 恢复、告警时间线、统一事实链跨节点/跨重启、作用域耗尽、锁压力，非 B8.5 验收）；统一事实链默认装配（data-plane relay + DNS/IPAM 双 consumer + DomainFacts split 复制）与 schema=2 租约表集已落地并经线上 DNS 应答实证（round 258–280）。**round 351 已完成 DNS 双机（方案 A 标准主从：AXFR/IXFR+NOTIFY+TSIG/XFR-over-TLS）并修复主从 TSIG 未签名缺陷（§1au）。** round 338–348 已完成 D2：默认 DHCP 热路径切换到 WAL 事件溯源存储**（`config.yaml` 默认 `dhcp.lease_store: wal`；参考 HA 部署默认 primary=WAL/standby=sqlite），含平价/事实同源/灰度/崩溃重放/回滚全矩阵证据（§1ao–§1as）。剩余均为需工作树之外输入或有意保留的边界**：B8.5 物理真机验收、501 预留扩展契约（6 个，有意保留）、候选未冻结项（兼容矩阵/快照迁移/UPSERT 写路径）、独立专家签字、HA standby 常驻 WAL（不必要，被提升节点靠重启加载）。round 286–287 已按用户「不需要按历史数据兼容」的指令把 legacy 代码、兼容层、数据库表与迁移全部清理，统一事实链成为唯一 DNS 生产者；未宣称外部验收通过、未触碰生产。

## 1. 已完成（有仓内证据）

| 领域 | 状态 |
|---|---|
| 前端 | Vue 3 + TypeScript + **TDesign Vue Next Starter `dashboard/base` devflow 主题**（TDesign Blue `#0052D9`、light side menu、TDesign token 化 shell、生产缓存覆盖；round 142–145）；**31 个 TDesign 页面（业务叶子路由）**（含 round 286 新增 `/dhcp/ha` 只读 HA 状态页）（含 round 133 补齐的独立 DNSSEC 密钥生命周期页 `/dns/dnssec`）；Naive UI 100% 移除；登录、RBAC；24 个浏览器/静态套件（本地 `make verify-browser`，不进入 CI）覆盖全部页面（含只读角色 `batch16`，round 178；11 个跨模块「创建 e2e」步骤，round 206 补齐 option/backup 后无仅渲染的生命周期页）；**round 172–173 复活仓内 Playwright e2e**：fixture 套件 8/8 全绿（针对 TDesign 重写选择器）、`api.spec` 1/1（token 隔离/CSRF/单次令牌/DNS 备份恢复）；3 个旧 shell UI 规格处置为待重写（独有 RBAC 只读写拒绝与分页跨页语义已分别以 §1aw/§1ax 服务端回归钉住） |
| 后端 API | **216 个 `/api/v1` 操作**全部有 OpenAPI 文档（`internal/api/openapi.go`），路由与文档双向一致性由 `TestEveryRegisteredAPIRouteIsDocumentedInOpenAPI` 钉住；**round 357 新增区域传送授权 API（`GET/POST /dns/zones/{id}/transfers`、`DELETE .../{transferId}`）**；**round 285 `GET /dhcp/ha` 由 501 升级为只读 HA 状态 API**（200，复用 CLI 状态数据，6 个 501 端点有意保留——`docs/audit/RESERVED-EXTENSION-DECISION.md`）；行为级回归钉住：RBAC 四象限（无权读 403/有权读 200/无权写 403/有权写 201）、按资源隔离+多角色并集、只读角色 120 变更全拒、501 预留扩展行为、分页跨页语义、auth 全流程（限速 429+解锁闭环、TOTP 状态机+登录强制+恢复码单次、会话吊销、must_change_password 禁闭、禁用即时失效、refresh SEC-05、令牌吊销）（round 196–205；见 `internal/api/auth_http_flow_test.go`、`internal/api/rbac_coverage_test.go`、`internal/api/handler/ipam_pagination_test.go`） |
| 单机运行期 | 四次握手 / option 54 / 续租 / 保留 / DECLINE / 过期回收；AXFR/IXFR ACL、cache 配置覆盖、RFC 2136 TSIG UPDATE、RENEWING 策略、DNSSEC NSEC/NSEC3 签名与否定证明、DoT/DoH/DoQ 上游装配等运行期缺陷已修复并实测 |
| 分离角色 | `role=control` + `role=dns` 两进程实跑：端口隔离、控制面→数据面复制（约 4s 权威应答）、**杀掉控制面后 DNS 仍从 `dnsdata.db` 应答**、重启恢复（§1b） |
| 容器化双节点 HA | **round 246–270（新增证据层，20+ 场景 + 监控/告警/facts 证据链全过）**：含 SIGKILL 单节点/双节点同时崩溃恢复、操作员批准单副本运行（degrade/undo）、作用域耗尽+队列压力真实负载、隔离上游 SSRF 守卫实测、**统一事实链容器层全链路**（facts 写入/单行计数器 sequence/跨重启 replay/跨双节点复制/接管时 facts 水位检查/fence→rejoin 回卷）：2 个 Docker 容器跑真实 DHCP HA——跨容器 TCP 镜像链路、真实 udhcpc DORA 租约**跨容器镜像**、容器死亡→`paused`（fail-closed）、持久水位 resync、网络分区→静默检测路径、**primary 死亡→standby 不自行提升（防脑裂）**、操作员接管（三确认）→角色翻转、fence/rejoin、**SIGKILL 单节点/双节点同时崩溃→WAL 恢复**、操作员批准单副本运行（degrade/undo，paused↔primary-degraded 对照）、**Prometheus 时间线捕获 paused 窗口**、**HA 专属告警（NoSecondCopy/WithholdingLeases）真实触发→恢复完整时间线经 Alertmanager+webhook**、两种断链的可观测语义区分（全节点分区=InstanceDown；仅镜像死亡=HA 告警）。`docker-compose.ha.yml`+`Dockerfile.ha`。定位：介于单机双进程与真实双主机之间，**非 B8.5 验收**（§1ca 系列） |
| DHCP HA 隔离安全模式 | 两进程 primary/standby：镜像链路、失联 `state=paused` fail-closed、重连恢复、错误 peer token 拒绝、运维动作 `ErrNoConfirmation`/`ErrWrongRole` 拒绝（§1c）。**不是**真实跨主机 HA 验收 |
| Enterprise 候选 | **53 个 `internal/enterprise` 包、613 个测试/Fuzz 入口**（round 258 起含 `dhcplease`）；事件链、快照只读校验、DHCP 数据面原型、角色/epoch/fencing 原语、`compat` 候选矩阵、`pullresponse` 4 种处置、版本感知 `replicaschema`；**round 258–263 schema=2 租约表集落地**（§1cb 系列）；**round 273–280 统一事实链默认装配落地**（data-plane relay + DNS/IPAM 双 facts consumer + DomainFacts split 复制 + 端到端实证含线上 DNS 应答——§1ad 系列） |
| 生产门禁 | **10 个 `ErrNoAuthority` 哨兵**恒关闭；`TestProductionWriteGatesRemainClosed`（21 项）、`TestAuthoritySentinelsAreDistinct`，以及 round 147 新增「完整 `writer_authority` 行也不授予写权限」回归 |
| 验证可复现 | `make verify` / `verify-race` / `verify-browser` / `build`；`test/browser/` 24 项检查（含 `docs` 交接文档守卫、`contracts` 7 类静态契约、`i18n`、`theme` devflow 主题守卫（品牌 #0052D9 / 默认隐藏项 / 全局响应式与视觉 token 规则）、`h2-report-ref` 报告引用守卫、19 个真实 Chromium 套件，含 `route-audit` 全路由文本守卫与只读角色 `batch16`、5 个静态守卫）；纯净构建 + 重新部署后端到端跑通（§1i） |
| 文档/评审材料 | 证据台账、评审 dossier、B8.5 Runbook、TDesign 迁移记录、对标方案（批次 A–F 全部有实测状态标记）、旧快照迁移路径候选定义、UPSERT 写路径候选定义、版本兼容合同；**round 210–216 全部 10 份引用/历史文档逐一审计**（量化声明实测、失效引用修正、过时差距行修正、取代注记），死路径引用机械扫描清零 |

## 2. 候选 / 已定义但**待独立评审冻结**（不是完成）

| 项 | 状态 | 文档 |
|---|---|---|
| 生产兼容矩阵（六轴） | 候选实现 + fail-closed 协商原语，**未冻结** | `contracts/version-compatibility.md`、`internal/enterprise/compat` |
| 旧四/五/六表快照 → 八表候选迁移路径 | **候选定义**（round 145），行为回归已钉住，**未冻结** | `docs/audit/SNAPSHOT-MIGRATION-PATH.md` |
| 加固 DDL 版本迁移 + 同键 UPSERT 写路径兼容 | **候选定义**（round 146），语义已实测钉住，**未集成/未冻结** | `docs/audit/UPSERT-WRITE-PATH-COMPAT.md` |
| DHCP 租约 / 发件箱 / 完整业务审计参考 schema | **已 FROZEN**（round 258 用户明确授权「从合理角度评估决策直接实施」，6 项决定记入提案 §6）+ **`internal/enterprise/dhcplease` 已实现**（29 测试、schema=2 十二表校验链，§1cb 系列）；仍非生产 DDL / 迁移 / 验收 | `contracts/enterprise-dhcp-lease.proposal.md` |
| 本机 DDL 加固参考结构 | 只读识别已完成（round 122），**生产升级未集成** | `contracts/version-compatibility.md` |

## 3. 外部阻断（不能在本沙箱替代）

| # | 需要用户/现场提供 | 为什么不能在此完成 |
|---|---|---|
| 1 | 两台以上真实主机、可控网络隔离、fencing/witness、真实断电/持久盘、真实 relay 与客户端、真实 Prometheus + `promtool` + Alertmanager 原始时间线 | B8.5 A–E / G1–G5；用户明确不得触碰生产；单机写锁、环回 UDP、示例规则文件按 Runbook 明确禁止替代 |
| 2 | 独立专家评审并冻结：兼容矩阵、旧快照迁移路径、UPSERT 写路径、`dnswire` 是否扩展（DHCP 租约 schema 已由用户授权冻结，不在此列） | 合同/版本决策需人工签字；候选定义明令冻结前不集成 |
| 3 | 本机 role/epoch/generation/fencing 的独立授权证据、可信时间/权威尾来源 | DB 行、config、TLS 身份都不能单独授予写权限；自行提升等于绕过安全模型且 `prodgates` 会拒绝 |
| 4 | 真实遗留生产库升级与历史对账、跨主机分区/旧主回归、ACK 语义迁移评审 | 需真实遗留数据与现场时间线 |
| 5 | `/cluster`、`/sso`、`/apps` 共 **6 个 501 预留端点**的契约 | 用户已明确「不做物理真机验收、独立专家冻结签字」→ 501 不再有"等证据齐备"的前提；**决策为有意保留为产品边界**（无产品规格/无现场需求），重新评估条件 = 出现明确产品规格。逐项决策记录见 `docs/audit/RESERVED-EXTENSION-DECISION.md`。**`GET /dhcp/ha` 已于 round 285 升级为只读状态 API（200），不再属于 501 契约** |

详见 `docs/audit/IMPLEMENTATION-STATUS.md` §1g 与 `docs/B8.5-外部验收Runbook.md` §10（现场证据模板）。

## 4. 一键复现

```bash
make verify          # Go build/test/vet + gofmt + 前端 lint/typecheck/build + git diff --check
make verify-race     # 上述 + go test -race ./...
make build           # 清空 dist 后从源码构建 ./dist/goddi 与 web/dist
make verify-browser  # 需运行中的隔离服务 + GODDI_ADMIN_PASS；见 test/browser/README.md
make verify-tidy     # go mod tidy -diff：零副作用校验 go.mod / go.sum 未漂移
make verify-e2e      # 一键跑完 CI 的两个 Playwright 套件（bare + 种子 fixture；自建实例）
```

`pnpm lint` 带 `--fix`，运行后应提交其格式化结果；`verify-browser` 的 Chromium 由 `test/browser/common.cjs` 自动探测或 `GODDI_CHROMIUM` 指定；`verify-tidy` 在模块缓存未预热时需要联网计算 tidy 图。

> **顺序提醒**：`make verify` 会 `pnpm build` 重建 `web-admin/dist`（文件名带哈希），但**不会**同步 `web/dist`；而服务端从 `./web/dist` 提供前端，`docs.cjs` 守卫会逐字节比对两者。因此跑完 `make verify` 后需执行 `make build`（或手动 `cp -r web-admin/dist/. web/dist/`）再跑 `make verify-browser`，否则守卫会正确报「deployed asset ... differs from the built bundle」。

## 5. 本会话刻意未做的事

未提升任何 `ErrNoAuthority` 门禁、除用户明确授权冻结的 DHCP 租约 schema 外未把任何行形状/矩阵宣称为已冻结、未把仓内证据升级为 B8.5 PASS、**除用户授权把 `GET /dhcp/ha` 升级为只读状态 API 外未改动其余 6 个 501 契约**、未实现双机生产协调、未扫描或改造现存生产 UPSERT 调用点、**未触碰生产**。

## 6. 文档索引

| 文档 | 用途 |
|---|---|
| `docs/audit/IMPLEMENTATION-STATUS.md` | 完成边界与证据台账（§1 当前矩阵、§1g 完成边界、§1b–§1aw 补记的增量） |
| `docs/audit/ENTERPRISE-REVIEW-DOSSIER.md` | 独立评审输入：52 包、10 门禁、评审必答问题 |
| `docs/B8.5-外部验收Runbook.md` | 外部验收 A–E、故障矩阵、§10 证据模板、§11 仓内准备 |
| `docs/audit/RUNTIME-VALIDATION-LEGACY.md` | 单机与分离角色运行期验证 |
| `docs/tdesign-migration.md` | 前端迁移与 30 页清单 |
| `docs/Technitium对标评估与实施方案.md` | 对标批次 A–F、DNSSEC 增量 1–5 |
| `contracts/version-compatibility.md` | 六轴版本规则、候选矩阵、剩余未冻结项 |
| `docs/audit/SNAPSHOT-MIGRATION-PATH.md` | 旧快照迁移路径候选定义 |
| `docs/audit/UPSERT-WRITE-PATH-COMPAT.md` | 同键 UPSERT 写路径兼容候选定义 |
| `docs/CONFIG-REFERENCE.md` | `config.yaml` 未列出的 36 个可选配置键、默认值、环境变量与保留占位说明 |
| `test/browser/README.md` | 浏览器回归运行方式与测试数据治理 |