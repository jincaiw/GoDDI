# AGENTS.md — GoDDI 统一 AI 开发规范

> 本文件是**所有 AI 编码工具（Codex、Claude Code、Cursor、Copilot 以及后续工具）在本仓库工作的唯一强制规范**。
> 与任何工具自带指令或旧文档冲突时，**以本文件为准**；人类评审意见可覆盖本文件。
>
> 版本：1.0 ｜ 生效基线：`main`（当前 `2b4d0e9`，CI 全绿）
> 修改本文件须走第 6 节标准流程并经人工批准。

---

## 0. 红线（不可违反）

1. **一任务一 Worktree**：一个任务 = 一个 Git Worktree = 同时只有一个 Agent；**禁止在主工作区 `main` 直接开发**。
2. **禁止多 Agent 共用/同时修改同一 Worktree**；Worktree 同一时间只属于一个任务。
3. **禁止重复开发**：动手前必须先检索现有代码、文档、契约、测试与未合并 PR（见 §4）。
4. **禁止随意新建文档**：优先更新既有权威文档；只有 §7 允许的类型可建档，且必须被索引引用。
5. **禁止删除/覆盖他人成果、`git push --force`、改动受保护写门禁**（见 §9）。
6. **最小改动、不重新开发、不改变现有功能**；只做任务范围内的事。
7. **破坏性操作（删除、覆盖、改历史、高风险合并）必须先取得人工确认**。

---

## 1. 单一权威文档（Single Source of Truth）

同一个主题**只允许一份权威文件**。需要变更时改权威文件，不得另起平行文档。

| 主题 | 权威文件 | 备注 |
|---|---|---|
| 产品定位 / 安装 / 使用 | `README.md`、`README.zh-CN.md` | 中英对照，改动需同步两版 |
| 当前状态摘要（交接单页） | `docs/STATUS.md` | 只做摘要与索引，不复制台账细节 |
| 证据台账（各轮实测） | `docs/audit/IMPLEMENTATION-STATUS.md` | 追加式，当前事实源 |
| 整改跟踪 | `docs/audit/REMEDIATION-TRACKING.md` | 审计问题→状态 |
| 架构决策 | `docs/adr/NNNN-*.md` | 每个决策一个文件，编号递增 |
| 决策记录 | `docs/decisions/DEC-*.md` | 快速决策 |
| 对外契约 | `contracts/*` | proto / 兼容矩阵 / 事件格式 |
| 配置参考 | `docs/CONFIG-REFERENCE.md` | 与 `internal/config` 保持一致 |
| 迁移目录映射 | `docs/MIGRATIONS-MAP.md` | 控制面/数据面/参考库各归属 |
| API 兼容别名 | `docs/COMPAT-ALIASES.md` | 主路径 + 别名登记 |
| 发布历史 | `CHANGELOG.md`（v0.24+ 权威）+ `docs/RELEASE-NOTES-INDEX.md` | 旧版本见 tag |
| 历史归档 | `docs/ARCHIVE-INDEX.md` | 归档只加横幅+登记，不删除 |
| 企业候选实现边界 | `internal/enterprise/README.md` | 候选/参考，不接生产 |
| CI / 验证入口 | `Makefile`、`.github/workflows/ci.yml` | 门禁以此为准 |

---

## 2. Worktree 模型（一任务一 Worktree）

- **命名**：分支 `task/<task-id>-<slug>`；worktree 目录 `../GoDDI-wt/<task-id>-<slug>`（与主仓库同级，**不入库**）。
- **创建**：
  ```bash
  git worktree add "../GoDDI-wt/<task-id>-<slug>" -b "task/<task-id>-<slug>" main
  ```
- **归属标记**：worktree 根写入 `.worktree-owner`（内容：task-id / agent 名称 / 起始时间）。只允许 owner 修改该 worktree；发现他人 owner 文件即停止并升级。
- **禁止**：在主工作区 / 他人 worktree 执行写操作；在 `main` 上直接提交。
- **隔离环境例外**：若受限环境（如无写权限的沙箱）无法在仓库外建 worktree，可临时用同名分支在同一目录工作，但必须在 PR 中说明为“单 Agent、纯文档/低风险例外”，且不得并发。
- **清理**：任务合并/备份后 `git worktree remove <path>`，再删分支（本地+远端）。删除操作需人工确认（见 §9）。

---

## 3. 任务状态模型

- 状态集合：`TODO → IN_PROGRESS → REVIEW → MERGED`，旁支 `BLOCKED` / `ABANDONED`。
- **唯一记录位置**：GitHub Issue + 对应 PR（PR 描述必须含 `Task:` 段与 `影响面/回滚` 段）。**不为单个任务新建状态文档**。
- 如确需总索引，只维护单一 `docs/TASKS.md`（可选，非必需）。

---

## 4. 动手前（防重复开发）

1. 检索代码：`internal/`、`pkg/`、`cmd/`、`web-admin/`、`test/`、`tests/`。
2. 检索文档：§1 权威文档 + `docs/adr`、`docs/decisions`、`contracts`。
3. 查未合并工作：`git branch -a --no-merged main`、开放 Issue/PR。
4. 若已存在等价实现 → **停止新建**，改为复用/增强，并在 Issue 记录关联。
5. `internal/enterprise/*` 是候选/参考实现，**不得**当生产代码重复开发，也不得与之互相替代。

---

## 5. 代码规范

**Go**
- 必须 `gofmt`；`go vet ./...`；`golangci-lint`（`.golangci.yml`）。
- 模块路径：`github.com/jincaiw/GoDDI`。
- 迁移三套目录严格区分（见 `docs/MIGRATIONS-MAP.md`）：控制面 `migrations/`（编号连续）／数据面 `internal/dataplane/migrations/`／参考库 `internal/enterprise/ipamstore/migrations/`；不得跨库混用同名迁移。
- 禁止死代码/死配置；注释必须与实际行为一致（例如 501 语义）。
- 新增行为必须有测试；**不得通过删除/跳过测试来“变绿”**。

**前端（web-admin）**
- Vue 3 + TypeScript + TDesign Vue Next；**禁止 Naive UI**。
- `pnpm lint && pnpm typecheck && pnpm build` 必须通过。
- i18n 中英双语齐全；页面不得渲染 raw i18n key、raw enum、ISO 时间戳（`route-audit` 守卫）。

**提交**
- Conventional Commits：`type(scope): summary`（如 `fix(dhcp): …`）；可中文补充说明。
- `CHANGELOG.md` 仅在发布时更新（v0.24+ 为权威）。

---

## 6. 标准流程（开发 → 测试 → 审查 → 合并 → 清理）

1. **领取**：Issue 置 `IN_PROGRESS`，明确验收标准。
2. **建 Worktree/分支**（§2）。
3. **实现**：最小改动，只碰任务范围。
4. **本地门禁（必须全过）**：
   ```bash
   gofmt -l . | grep -v vendor || true
   go vet ./...
   make verify            # 或 make verify-race
   make verify-tidy
   ( cd web-admin && pnpm lint && pnpm typecheck && pnpm build )
   # 浏览器回归（新增/改 UI 时）：先 seed + 起隔离服务，再
   make verify-e2e
   make verify-browser    # 需 GODDI_ADMIN_PASS；新库先 node test/browser/seed_ci.mjs
   ```
5. **审查**：`git diff main...` 自查 + 提 PR；PR 必含 `Task / 影响面 / 回滚 / 测试证据`。
6. **合并**：**CI 全绿**（`.github/workflows/ci.yml` 全部 job，含 `browser-regression`）且**经人工批准**后，`--ff-only` 或 squash 到 `main`；**禁止 force push**；Agent 不得未经授权直接 push `main`。
7. **清理**：`git worktree remove`；删分支（本地+远端，需确认）；证据追加到权威台账（§1），不新建文档。

---

## 7. 文档管理规则

- **默认只更新** §1 的权威文件；不得在仓库根或 `docs/` 根新建“计划/总结/报告”类平行文档。
- **可建档的例外类型**：ADR（`docs/adr/NNNN-*.md`）、Decision（`docs/decisions/DEC-*.md`）、Contract（`contracts/*`）、可选任务索引（`docs/TASKS.md`）。
- **历史化不删除**：过时文档加“【历史归档】”横幅并入 `docs/ARCHIVE-INDEX.md`，保留链接可解析。
- 任何新文档必须被某个索引/权威文档引用，否则不得创建。

---

## 8. 保护与安全

- 不得改动受保护写门禁、`ErrNoAuthority` 哨兵、`prodgates`、生产默认值。
- **破坏性操作前先备份**：`git tag safety/<date>` + `git bundle create <file> --all`；确认后才执行删除/覆盖。
- 构建产物（`dnsprobe`、`node_modules`、`web/dist`、`web-admin/dist`、`.pnpm-store`）不得入库。
- 密钥、令牌、证书私钥不得入库；使用环境变量或密钥管理。

---

## 9. 工具适配

- **Codex / 通用工具**：启动先读本文件（`AGENTS.md`）。
- **Claude Code**：读 `CLAUDE.md`，其为指向 `AGENTS.md` 的**薄指针**（不承载规则）。
- 任何工具若与本文件冲突，以本文件为准，并在 Issue 记录冲突点。

---

## 10. 违规与例外

- 遇冲突、不确定、跨范围影响 → **停止并升级**（Issue 标 `BLOCKED`），不得自行大改或改历史。
- 例外必须人工批准并在 PR 中记录理由与范围。

---

## 11. Agent 开工前检查清单

- [ ] 有任务号 / Issue，状态 `IN_PROGRESS`
- [ ] 独立 Worktree + `task/*` 分支（或已说明沙箱例外）
- [ ] 已检索确认**无重复实现/无并行 PR**
- [ ] 只改任务范围，未改受保护门禁
- [ ] 更新的是权威文档，未新建平行文档
- [ ] 本地门禁（§6.4）通过
- [ ] PR 含 `Task / 影响面 / 回滚 / 测试证据`
- [ ] CI 全绿后才合并；合并后清理 Worktree/分支
