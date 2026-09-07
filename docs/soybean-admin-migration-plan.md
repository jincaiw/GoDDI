# GoDDI 前端迁移 soybean-admin 实施方案

> 版本：v1.1 ｜ 日期：2026-09-07 ｜ 模板：[soybeanjs/soybean-admin](https://github.com/soybeanjs/soybean-admin) v2.2.0（MIT）
> 结论：技术栈高度兼容（Vue3 + Vite8 + TS + Pinia + Naive UI），可整体替换，预估 2–4 个工作日完成主体迁移。
> v1.1 修订：经对 v2.2.0 实际源码逐项核验后修正——新增「版本差异表」；纠正动态路由命名约定；请求层适配策略由「复用模板请求实例」改为「原生 axios 封装整体替换」；proxy 配置方式修正为 .env 驱动。

### 1.0 模板实际版本核验（2026-09-07，GitHub main @ v2.2.0）

| 依赖 | soybean v2.2.0 | GoDDI 现状 | 差异影响 |
|---|---|---|---|
| vue | 3.5.34 | ^3.5.34 | ✅ 完全一致 |
| naive-ui | 2.44.1 | ^2.44.1 | ✅ 完全一致 |
| pinia | 3.0.4 | ^3.0.4 | ✅ 完全一致 |
| echarts | 6.0.0 | ^6.1.0 | ✅ 同主版本 |
| **vue-router** | **5.0.7** | ^4.6.4 | ⚠️ 主版本升级（见风险 R-1） |
| **vue-i18n** | **11.4.2** | ^9.14.5 | ⚠️ 跨两个主版本（见风险 R-2） |
| **typescript** | **6.0.3** | ~5.6.0 | ⚠️ 主版本升级（见风险 R-3） |
| vite | 8.0.12 | ^8.0.16 | ✅ 同主版本 |
| engines | node ≥20.19 / pnpm ≥10.5 | — | Makefile/Dockerfile 需满足 |
| build 命令 | `vite build --mode prod`，outDir 默认 `dist` | — | ✅ 与方案构建链假设一致 |

模板源码目录已核实：`src/{router/{elegant,guard,routes},service/{api,request},store,locales,layouts,views,hooks}`，与方案目标路径一致。

---

## 1. 迁移范围与不变量

### 1.1 必须保持不变（迁移红线）

| 项目 | 说明 |
|---|---|
| Go embed 契约 | `assets.go` 使用 `//go:embed all:web/dist`，**最终产物必须落在 `web/dist`**，assets.go 不改 |
| 认证机制 | JWT（token/refresh_token/csrf_token）存 **sessionStorage**，见 `stores/auth.ts` 安全注释 |
| 权限模型 | `resource + action` 二元组（RBAC），`hasPermission(resource, action)`，后端 `/users/me` 下发 permissions |
| API 层 | `src/api/*` 9 个模块 + `api/client.ts`（axios 拦截器、401 区分逻辑）基本原样保留 |
| i18n 语言 | zh-CN / en-US 两种语言，key 结构可重构但语料必须完整迁移 |
| e2e 测试 | `@playwright/test` 保留，选择器按新布局更新 |

### 1.2 明确放弃的旧实现

| 旧文件 | 处置 |
|---|---|
| `components/AppLayout.vue` | 删除，由 soybean 布局体系（Layout + Header + Sider + Tab）替代 |
| `components/PageHeader.vue` | 删除，改用 soybean 页头/面包屑 |
| `router/index.ts` + `guards.ts` | 删除，改 Elegant Router 文件路由 + soybean 守卫链 |
| `theme.ts` | 删除，改 soybean 主题配置（含暗色模式） |
| `i18n/index.ts` 大文件 | 拆分为按模块 locale |

---

## 2. 工程结构决策

### 2.1 采纳 pnpm monorepo（推荐）

直接采用 soybean 的 monorepo 结构以保持可持续升级；构建产物通过 Makefile/Dockerfile 拷贝回 `web/dist`。

```
GoDDI/
├── web-admin/                 # ← soybean-admin 模板根（monorepo）
│   ├── src/                   # 主应用（本方案所有改造在此）
│   ├── packages/              # 模板组件子包（不改）
│   ├── pnpm-workspace.yaml
│   └── uno.config.ts
├── web/                       # 保留：仅作为 dist 输出目录（构建后生成）
│   └── dist/                  # go:embed 目标
├── assets.go                  # 不改
├── Makefile                   # web-build 目标改写
└── Dockerfile                 # web-builder 阶段改写
```

> 备选方案（不推荐）：抽掉 monorepo 壳做成单包。可减少 pnpm 依赖，但后续无法直接 `git merge` 上游更新，放弃。

### 2.2 路由模式：前端静态路由 + 自定义权限 meta

soybean 支持前端静态路由与后端动态路由两种。GoDDI 的 permissions 由 `/users/me` 返回结构化 `resource+action`，不是菜单路由表，故：

- 采用**前端静态路由**（Elegant Router 文件约定生成）
- 在 soybean 的守卫链（`src/router/guard/`，已核实该目录存在，含 auth/route 等守卫）中**追加一个 `permissionGuard`**，读取自定义 meta：

```ts
// 路由 meta 扩展（在生成路由对象上追加，或 er.config.ts 的 getRouteMeta）
meta: { permission: { resource: 'dns', action: 'read' } }

// guard 逻辑：复用现有 authStore.hasPermission()，
// 失败时提示 noPermission 并重定向 home（与现 guards.ts 行为一致）
```

- 后端动态路由方案留作二期，无必要不启用。

### 2.3 Elegant Router 命名约定（已核验 v0.3.8 文档）

- 动态参数用**文件名方括号**（不是文件夹）：`views/dns/zone-detail/[id].vue` → `/dns/zone-detail/:id`；可选参数 `[[param]]`；多参数 `detail_[id]_[userId].vue`
- 路由 name 自动生成 **PascalCase**（如 `DnsZoneDetailId`），可用 `getRouteName` 自定义
- `(group)` 圆括号文件夹仅作文件组织、不进路径
- 路由 meta 通过 `getRouteMeta`（仅填充缺失属性）或直接改生成物添加
- 菜单/面包屑文案走 `meta.i18nKey` + locale 文件

---

## 3. 目录映射表

### 3.1 基础设施

| 现路径（web/src/） | 目标路径（web-admin/src/） | 改造说明 |
|---|---|---|
| `api/client.ts` | `service/request/goddi.ts`（新文件，整体替换模板 demo 请求层） | **策略修正**：模板请求层基于 `@sa/axios` workspace 包（FlatRequest 泛型封装），与其耦合改造成本高于收益；GoDDI 无 refresh token 实际流程（refreshToken 仅占位），故保留现有原生 axios 拦截器封装整体搬入，模板自带 demo API（含 @sa/axios 实例与 mock 登录）全部删除，登录/登出/用户信息改调 Goddi API。401/code 附加逻辑原样保留 |
| `api/*.ts`（9 个） | `service/api/*.ts` | 仅改 import 路径 |
| `stores/auth.ts` | `store/modules/auth/index.ts`（重写模板版） | sessionStorage/JWT 过期判断/hasPermission 原样保留，接口对齐 soybean 的 `login`/`getUserInfo` 契约 |
| `stores/app.ts` | 合入 soybean `theme`/`app` store | 仅保留 GoDDI 特有状态 |
| `composables/useAuth.ts` | 由 auth store 替代 | 删除 |
| `composables/usePagination.ts` | `hooks/common/*` | 原样搬，或改用 soybean 的 useHookTable |
| `composables/usePermission.ts` | `store/modules/auth` 内 hasPermission 替代 | 删除 |
| `i18n/zh-CN.ts` / `en-US.ts` | `locales/langs/zh-cn/*.ts`、`en-us/*.ts` | 按模块拆分：common/dns/dhcp/ipam/admin/logs/settings |
| `components/ConfirmDialog.vue` | 保留原样 | Naive UI 通用组件，直接搬 |
| `components/DashboardChart.vue`、`RcodeDonut.vue`、`StatsTrendChart.vue` | `components/custom/` | vue-echarts 原样搬（模板已内置 echarts） |

### 3.2 视图路由映射（24 视图 + 登录）

Elegant Router 目录约定：`views/<module>/<page>/index.vue`，动态段用 `[id]` 目录。菜单/面包屑文案走 `meta.i18nKey` + locale 文件。

| 现路由 | 目标目录 | 权限 meta |
|---|---|---|
| `/login`（LoginView.vue） | `views/login/index.vue`（基于模板登录页定制） | 无（公开） |
| `/`（DashboardView.vue） | `views/dashboard/index.vue`（模板 home 改造） | 无 |
| `/dns/zones` | `views/dns/zones/index.vue` | dns:read |
| `/dns/zones/:id` | `views/dns/zone-detail/[id].vue`（文件名方括号，非文件夹） | dns:read |
| `/dns/forwarders` | `views/dns/forwarders/index.vue` | dns:read |
| `/dns/security` | `views/dns/security/index.vue` | dns:read |
| `/dns/cache` | `views/dns/cache/index.vue` | dns:read |
| `/tools/client` | `views/tools/client/index.vue` | dns:read |
| `/dhcp/scopes` | `views/dhcp/scopes/index.vue` | dhcp:read |
| `/dhcp/leases` | `views/dhcp/leases/index.vue` | dhcp:read |
| `/dhcp/reservations` | `views/dhcp/reservations/index.vue` | dhcp:read |
| `/dhcp/options` | `views/dhcp/options/index.vue` | dhcp:read |
| `/ipam/spaces` | `views/ipam/spaces/index.vue` | ipam:read |
| `/ipam/subnets` | `views/ipam/subnets/index.vue` | ipam:read |
| `/ipam/addresses` | `views/ipam/addresses/index.vue` | ipam:read |
| `/admin/users` | `views/admin/users/index.vue` | user:read |
| `/admin/roles` | `views/admin/roles/index.vue` | role:read |
| `/admin/groups` | `views/admin/groups/index.vue` | group:read |
| `/admin/tokens` | `views/admin/tokens/index.vue` | token:read |
| `/admin/sessions` | `views/admin/sessions/index.vue` | user:read |
| `/logs/audit` | `views/logs/audit/index.vue` | audit:read |
| `/logs/dns` | `views/logs/dns/index.vue` | dns:read |
| `/logs/dhcp` | `views/logs/dhcp/index.vue` | dhcp:read |
| `/settings` | `views/settings/system/index.vue` | settings:read |
| `/settings/backup` | `views/settings/backup/index.vue` | backup:read |

> 页面内部改动模式统一：去掉 `PageHeader` → 由布局面包屑接管；表格/表单/Modal 逻辑不动；`usePagination` 的接入点不变。

### 3.3 菜单分组（侧边栏 structure）

```
Dashboard（仪表盘）
DNS          : zones / forwarders / security / cache
Tools        : client
DHCP         : scopes / leases / reservations / options
IPAM         : spaces / subnets / addresses
Administration: users / roles / groups / tokens / sessions
Logs         : audit / dns / dhcp
Settings     : system / backup
```

---

## 4. 构建链改造

### 4.1 Makefile

```make
web-build:
	cd web-admin && pnpm install --frozen-lockfile && pnpm build
	rm -rf web/dist && mkdir -p web/dist
	cp -r web-admin/dist/. web/dist/
```

### 4.2 Dockerfile（web-builder 阶段）

```dockerfile
FROM node:22-alpine AS web-builder
RUN corepack enable && corepack prepare pnpm@latest-10 --activate
WORKDIR /app/web-admin
COPY web-admin/pnpm-workspace.yaml web-admin/pnpm-lock.yaml web-admin/package.json ./
RUN pnpm install --frozen-lockfile
COPY web-admin/ ./
RUN pnpm build
```

最终 stage 的 `COPY --from=web-builder` 目标路径不变（`/app/web/dist/`），Go embed 契约不动。

### 4.3 Vite 配置要点（web-admin，已核验模板 vite.config.ts）

- `outDir` 未显式配置（默认 `dist`），保持不动，由外层脚本拷贝
- **dev proxy 由 .env 驱动**（模板用 `createViteProxy(viteEnv)` 读取 `VITE_HTTP_PROXY` / `VITE_SERVICE_BASE_URL` / `VITE_SERVICE_BASE_URL_*` 等环境变量），不写死在 vite.config —— GoDDI 后端地址按此约定落 `.env`
- `base` 由 `VITE_BASE_URL` 控制：Go embed 场景设为 `/`（GoDDI 后端在根路径服务前端）
- 模板支持 `VITE_ROUTER_HISTORY=hash`；GoDDI 后端已支持 history 模式 SPA 回退，保持 history；若后续发现嵌路径部署问题可切 hash 兜底
- dev 端口 9527 / preview 9725：Playwright baseURL 与 CI 需同步修改

---

## 5. 分阶段实施计划

### Phase 0 — 准备（0.5 天）

| # | 任务 | 产出/验收 |
|---|---|---|
| 0.1 | 建分支 `feat/soybean-admin-migration` | 分支存在 |
| 0.2 | 本地确认 Node ≥ 20.19 + pnpm ≥ 10.5 | `pnpm -v` 通过 |
| 0.3 | 拉取 soybean-admin v2.2.0 到 `web-admin/`，去除 demo 页面（views/about、demo 等） | `pnpm dev` 可启动空白骨架 |
| 0.4 | **vue-router 5 预验证**（风险 R-1）：将 Dashboard 一页原样搬入模板跑通，确认 useRoute/useRouter/守卫写法无 breaking | 页面渲染与跳转正常，差异点记录 |

### Phase 1 — 骨架接入与构建链（0.5 天）

| # | 任务 | 产出/验收 |
|---|---|---|
| 1.1 | 改写 Makefile `web-build`（见 4.1） | `make web-build` 后 `web/dist/index.html` 存在 |
| 1.2 | 改写 Dockerfile web-builder 阶段（见 4.2） | `docker build` 全链路通过 |
| 1.3 | 迁移 vite dev proxy 与 .env | dev 模式可调通后端 `/api` |
| 1.4 | 旧 `web/` 目录清空（保留 .gitignore 策略：dist 入库与否与现状一致） | — |

### Phase 2 — 认证与权限（1 天）

| # | 任务 | 产出/验收 |
|---|---|---|
| 2.1 | 迁移 `api/client.ts` → `service/request/goddi.ts`（原生 axios 封装整搬）；**删除模板 demo 请求层**（@sa/axios 实例、mock API、request/index.ts 内登录刷新逻辑） | 模板无残留 demo API 引用；401/code 附加逻辑行为与现一致 |
| 2.2 | 迁移 `api/*.ts` 9 个模块 | `pnpm typecheck` 通过 |
| 2.3 | 重写 `store/modules/auth`：sessionStorage 三 token、JWT 过期判断、hasPermission/hasAnyPermission 原样搬 | 现有单测（如有）通过 |
| 2.4 | 定制登录页（保留 redirect query、error=network 分支语义） | 登录/登出/过期重登全流程手工验证 |
| 2.5 | 守卫链：保留 soybean auth 守卫 + 追加 permissionGuard（meta.permission → hasPermission，失败提示并回 Dashboard） | 无权限访问 `/admin/users` 被拦截 |

### Phase 3 — 路由与菜单（0.5 天）

| # | 任务 | 产出/验收 |
|---|---|---|
| 3.1 | 按 3.2 映射表建目录与空 index.vue，Elegant Router 自动生成路由与类型 | `src/router/elegant/` 生成物齐全，typecheck 通过 |
| 3.2 | 路由 meta：i18nKey/图标/order/permission 全量配置 | 菜单树与 3.3 分组一致 |
| 3.3 | locale 菜单文案补齐（zh/en） | 中英文菜单完整显示 |

### Phase 4 — 页面批量搬迁（1–1.5 天）

| # | 任务 | 产出/验收 |
|---|---|---|
| 4.1 | 搬 3 个公共组件 + ConfirmDialog + usePagination | typecheck 通过 |
| 4.2 | 按模块搬迁（顺序：DNS → DHCP → IPAM → Tools → Admin → Logs → Settings → Dashboard） | 每模块完成后自测 CRUD |
| 4.3 | Dashboard 三个图表接 vue-echarts（模板已内置 echarts 依赖，确认版本兼容） | 图表渲染正常 |
| 4.4 | 删除各页 PageHeader 调用，检查每页返回/面包屑正常 | — |

### Phase 5 — i18n 拆分与样式归拢（0.5 天）

| # | 任务 | 产出/验收 |
|---|---|---|
| 5.1 | 两大 i18n 文件按模块拆入 `locales/langs/`，key 命名对齐 soybean 约定 | 语言切换全站生效，无 missing key 告警 |
| 5.2 | 自定义样式归拢到 UnoCSS/主题变量；暗色模式逐页过一遍 | 暗色模式无大面积硬编码底色 |

### Phase 6 — 验收与收尾（0.5 天）

| # | 任务 | 产出/验收 |
|---|---|---|
| 6.1 | 更新 Playwright e2e 选择器（新布局类名） | e2e 全绿 |
| 6.2 | `make build` 全链路 + 二进制启动验证 embed UI | 内嵌页面可访问 |
| 6.3 | 删除旧 `web/src`、`web/package.json` 等遗留文件 | 仓库无死代码 |
| 6.4 | 更新 README 构建说明（pnpm 要求、目录说明） | 文档同步 |

---

## 6. 风险与对策

| 风险 | 影响 | 对策 |
|---|---|---|
| **R-1 vue-router 4→5**：模板用 5.0.7，GoDDI 页面代码基于 4.x API | useRoute/useRouter/router-link 等常用 API 兼容性需验证；自定义守卫写法可能有细微差异 | Phase 0 先用 1–2 个代表页面在模板内验证；关注 vue-router 5 changelog breaking changes；守卫全部走模板守卫链，不保留旧 guards.ts |
| **R-2 vue-i18n 9→11**：模板用 11.4.2 | createI18n/useI18n 核心 API 兼容，但 v10/v11 移除了部分废弃能力（如 legacy options 默认值变化）；现有语料 key 结构需整体搬移 | 语料是纯 JSON 结构，风险低；Phase 5 全站切换语言回归；typecheck 会暴露 API 差异 |
| **R-3 TypeScript 5.6→6.0**：模板 tsconfig + TS6 严格度更高 | 现有 24 页面 + api 层代码可能报类型错误 | Phase 2/4 每步跑 `pnpm typecheck`；预计多为宽松类型标注收紧，机械修复 |
| Elegant Router 命名约束导致路由名变化 | e2e/外链失效 | **URL path 全部保持不变**（映射表按原 path 设计，动态路由用 `[id].vue` 生成 `:id` 段）；路由 name 变化无外部影响；e2e 全量回归 |
| 模板请求层（@sa/axios）与现有 axios 封装冲突 | 请求行为不一致 | 已定策略：整体删除模板 demo 请求层，保留原生 axios 封装（见 3.1） |
| monorepo 构建时间增加 | CI 变慢 | Docker layer 缓存 pnpm store；CI 增量 install |
| 模板升级带来的 breaking change | 后续维护成本 | 锁定 v2.2.x；升级时走单独分支 + e2e 回归 |
| pnpm 强制（npm/yarn 不支持） | 贡献者环境 | README + Makefile 统一入口，Docker 内 corepack 固化版本 |
| UnoCSS 学习成本 | 样式迁移摩擦 | Phase 5 允许页面内 scoped CSS 暂留，仅全局样式先归拢 |

---

## 7. 回滚策略

- 整体在独立分支进行，旧 `web/` 在 Phase 6.3 前不删除，主干随时可合回/回滚
- 上线以 tag 为准，Go embed 产物二进制与前端目录一一对应，出问题回退镜像即可

---

## 8. 实施偏差记录（实施完成后补充）

以下为实际实施中与原方案的偏差，均已验证通过（vue-tsc 零错误、生产构建 2.8M、`go build` embed 正常、Playwright e2e 17/17 全部通过）：

1. **动态路由路径变化**：`/dns/zones/:id` 区域详情无法与 `index.vue` 同目录共存（Elegant Router 命名冲突 `dns_zones`），迁移至 `views/dns/zone-detail/[id].vue`，实际路径为 `/dns/zone-detail/:id`；e2e 与视图内跳转已同步更新。
2. **`/settings` 无独立页面**：settings 为纯分组（`settings/system` + `settings/backup`），父级由 transform 自动重定向到第一个子路由；原指向 `/settings` 的链接统一改为 `/settings/system`。
3. **登录后重定向**：auth store 的重定向不再受 tab 清理逻辑（`checkTabClear`）门控——首次登录 `lastLoginUserId` 为空会返回 true 导致跳过 push；现改为无条件 sanitized redirect。
4. **存储前缀去 soybean 化**：`VITE_STORAGE_PREFIX` 由 `SOY_` 改为 `GODDI_`；i18n 语言键为 `GODDI_lang`（JSON 编码）。
5. **主题切换简化**：头部 ThemeSchemaSwitch 从 light→dark→auto 三态循环改为明暗二态切换（企业控制台无 auto 需求）；组件加 `aria-label="Toggle theme"`。
6. **无障碍标注**：为 e2e 与可访问性补充 `aria-label`（Switch language / Toggle navigation / Toggle theme）。
7. **移动端 sider 修复**：AdminLayout 移动端抽屉 `w-0` 工具类在构建产物中覆盖 `.layout-sider` 宽度变量（CSS 级联顺序问题），导致移动端菜单宽度为 0 不可用；`w-0` 改为折叠态条件类。
8. **移动端导航自动收起**：app store 新增路由 fullPath watcher，移动端路由跳转后自动收起 sider（原模板需手动点遮罩）。
9. **e2e 适配**：语言键 `locale`→`GODDI_lang`、登录按钮/语言切换/深色切换/移动导航选择器适配新 UI、路由表 `/dns/client`→`/tools/client`、`/settings`→`/settings/system`、登出走头像下拉+确认弹窗。
10. **e2e 目录迁移**：`e2e/` 从 `web/` 移出为仓库顶层独立包（自带 package.json，@playwright/test ^1.63.0），避免依赖 soybean monorepo。
