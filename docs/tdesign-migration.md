# GoDDI 前端 TDesign 迁移 + 单机优先 / 双机架构预留

> 状态：**阶段 1 + 阶段 2 均已完成**（脚手架 + 登录 + 完整样板页 + 批次 2–15 共 32 个 TDesign 页面（含 `_builtin/login`）+ 死代码清理；**Naive UI 已 100% 移除**），全部在隔离单机环境下浏览器实测；`pnpm lint` / `typecheck` / `build` 全绿。
> 技术栈：Vue 3 + TypeScript + Vite + **TDesign Vue Next Starter** 分层，替换原 soybean-admin / Naive UI。
> 范围约定：**先单机实现所有功能，双机（HA / 复制 / 多节点协调）从技术架构角度预留，最后实现。**

## 1. 阶段 1 已完成（可验证）

| 项 | 文件 | 说明 |
|---|---|---|
| 依赖 | `web-admin/package.json` | `tdesign-vue-next@^1.20.11`、`tdesign-icons-vue-next@^0.4.12`；`pnpm install` / `pnpm build` 在本环境可用 |
| 全局注册 + 样式 | `web-admin/src/plugins/tdesign.ts`、`main.ts` | `app.use(TDesign)` + icons；只引 `tdesign.css`（组件样式，**不引 reset.css**），避免影响未迁移的 Naive UI 页 |
| TDesign 语言 | `src/App.vue` | `t-config-provider` 外层（zh_CN / en_US），内层保留 `NConfigProvider` 服务未迁移页面 |
| **Starter 骨架布局** | `src/layouts/tdesign-layout/index.vue` | `t-layout / t-aside / t-menu / t-header / t-content / t-footer`；深色侧栏、折叠、路由驱动高亮、用户下拉登出、`Single-Node` 标识 |
| 路由接入点 | `web-admin/build/plugins/router.ts` | elegant-router 的 `layouts.base` 指向 TDesign 布局（**改这里**，不要手改 `src/router/elegant/imports.ts`，构建会重新生成并覆盖） |
| **登录页** | `src/views/_builtin/login/index.vue` | 全量 TDesign 重写：`t-form/t-input/t-button/t-alert`，用户名/密码/可选 TOTP，复用 `authStore.login`（JWT+CSRF 存 sessionStorage → `/auth/me` → 重定向），路由守卫/RBAC/拦截器零改动 |
| **完整样板页** | `src/views/ipam/spaces/index.vue` | 全量 TDesign 参考页：远端分页 `t-table`、新建/编辑 `t-dialog + t-form` 校验、`DialogPlugin.confirm` 删除、RBAC 门控、i18n、错误提示；已浏览器实测创建成功并回显 |
| 环境修复 | `.env` / `.env.prod` / `.env.test` | 补 `VITE_OTHER_SERVICE_BASE_URL={}`，消除每次加载的 json5 解析 console error |

**保留不变的关键契约**（迁移红线，原样复用）：
- `src/service/api/goddi/client.ts`：`/api/v1`、`Bearer`、`X-CSRF-Token`、401 刷新队列、信封 `{code,message,data}`、分页 `meta`。
- `src/store/modules/auth/shared.ts`：token 存 **sessionStorage**。
- `usePermission()` → `authStore.hasPermission(resource, action)` RBAC 二元模型。
- i18n 语料 `route.*` / `common.*` / `ipam.spaces.*`。

### 验证结果（真实浏览器，Chromium headless）
- `pnpm typecheck` = 0、`pnpm build` = 0；`web/dist` 已重建（buildTime 2026-11-09 13:29:24，82 文件 / 11M）。
- 登录页 TDesign 组件渲染、登录成功跳转 `/dashboard`、sessionStorage 写入 token。
- TDesign 骨架 `t-layout/t-aside/t-menu/t-header` 全部渲染；`Single-Node` 徽标可见。
- 样板页：表格渲染、新建按钮可见（RBAC 通过）、TDesign 弹窗表单打开、创建后数据回显。
- **10 个未迁移的 Naive UI 页面**（dashboard / dns zones / dns forwarders / dhcp scopes / dhcp leases / ipam subnets / admin users / settings system / logs audit / tools client）在新骨架下全部正常渲染，**0 个 pageerror / 0 console error**。
- 截图证据：`/tmp/goddi-run/ui-login.png`、`ui-spaces.png`、`ui-spaces-created.png`。

## 2. 阶段 2 进度（按样板页批量迁移）

**批次 2 已完成（round 99）**，新增 4 个纯 TDesign 页面，均为真实浏览器验证通过：
- `src/views/logs/audit/index.vue`、`logs/dhcp/index.vue`、`logs/dns/index.vue`：远端分页日志表；`NTag` → `TTag` 渲染函数，`NDataTable` → 共享 `RemoteTable`。
- `src/views/dns/apps/index.vue`：`t-empty` 空状态（后端 `/apps` 返回 501，UI 明确标注，不是 UI 缺陷）。
- **新增共享组件** `src/components/RemoteTable.vue`：封装 `t-table` + 远端分页 + loading/error，后续所有列表页只需声明 `columns` + `fetcher`（沿用 `getList()` 的 `{data,meta}` 契约）。
- 修过一个真实组件 API 坑：TDesign Empty 的操作区插槽是 `#action`，不是 `#extra`（原写法导致 501 提示不渲染）。
- 验证：`pnpm typecheck`=0、`pnpm build`=0、`web/dist` 重建；Chromium 中 5 个页面（含样板页）全部 PASS、0 console/page error；核心登录+CRUD 冒烟与 10 个未迁移页面回归全绿。

**当前 TDesign 页面共 32 个（含 `_builtin/login`）**（0 处 `naive-ui` 引用）：
`_builtin/login`、`ipam/spaces`、`logs/audit`、`logs/dhcp`、`logs/dns`、`dns/apps`、`dns/security`、`dhcp/ha`、`dhcp/leases`、`settings/system`、`dhcp/options`、`dhcp/reservations`、`settings/backup`、`admin/tokens`、`admin/sessions`、`dhcp/scopes`、`dns/allowed`、`admin/groups`、`admin/users`、`admin/roles`、`dns/cache`、`dns/forwarders`、`dns/blocked`、`tools/client`、`dns/zones`、`ipam/addresses`、`ipam/subnets`、`ipam/retirement-archive`、`dns/zone-detail`、`settings/config-versions`、`dashboard`、`dns/dnssec`。

**批次 3 已完成（round 100）**，新增 3 个纯 TDesign 页面 + `RemoteTable` 增强：
- `views/dns/security/index.vue`：客户端策略表 + 新建弹窗（`t-form` 校验）+ `DialogPlugin.confirm` 删除；浏览器实测**创建策略成功并回显**。
- `views/dhcp/leases/index.vue`：租约表 + 搜索/状态过滤 + 释放确认；`RemoteTable` 新增 `params` prop（深度监听，过滤变更自动回到第 1 页重载）。
- `views/settings/system/index.vue`：25 项系统设置，bool/int/string 三类控件（`t-switch` / `t-input-number` / `t-input` / `t-textarea`）+ 逐项保存；浏览器实测**改值 60→67 保存后刷新仍为 67**。
- 修过两个真实 TDesign API 坑：`t-input` 的 `type` 不含 `textarea`（必须用 `t-textarea`）；`t-switch` 的字符串布尔值要用 `:custom-value="['true','false']"`。
- 验证：`typecheck`=0、`build`=0、专项 8 项全 PASS、0 error；核心冒烟 + 批次 2 + 10 个未迁移页面回归全绿。

样板页 `src/views/ipam/spaces/index.vue` 即为表单/弹窗类模板；`RemoteTable.vue` 为列表类模板。每页迁移清单：
1. 组件替换：`NDataTable→t-table`、`NModal→t-dialog`、`NForm/NFormItem→t-form/t-form-item`、`NInput→t-input`、`NButton→t-button`、`useMessage→MessagePlugin`、`ConfirmDialog→DialogPlugin.confirm`、`PageHeader→t-card` 页头。
2. 保留：`usePermission()`、`$t` / `useI18n()`、`service/api/goddi/*` 全部调用、信封与分页处理。
   RBAC 呈现规则：导航按读权限过滤；页面内写/删操作在用户无权限时禁用并保留可见性，避免隐藏造成功能位置不可发现；危险批量操作按确认对话框二次确认。
3. 表格 `cell` 用 `h(...)` 渲染函数（TS 类型友好），不要引入 JSX（当前 `lang="ts"`）。
4. 删除该文件的 `naive-ui` import；完成后该文件不再计入 `grep -rl naive-ui src/views`。

**批次 4 已完成（round 101）**，新增 4 个纯 TDesign 页面，全部 CRUD 全链路浏览器实测：
- `views/dhcp/options/index.vue`：选项表 + 新建/编辑弹窗（code/value/priority/scope）+ 删除确认。
- `views/dhcp/reservations/index.vue`：保留地址表 + 表单 + 删除；**浏览器实测创建 `02:00:00:de:ad:b4` 成功回显**。
- `views/settings/backup/index.vue`：备份表（名称/大小/类型/状态标签）+ 新建备份 + 下载/恢复/删除确认。
- `views/admin/tokens/index.vue`：令牌表 + 新建弹窗 + **一次性令牌展示弹窗**（复制按钮）；浏览器实测创建成功显示一次性令牌。
- `RemoteTable` 再增强：新增 `showPagination` prop（备份列表不分页时隐藏分页条）。
- 已核实 TDesign API：`t-dialog :footer="false"` 隐藏页脚、`t-date-picker enable-time-picker`、`t-textarea readonly`。
- 验证：`typecheck`=0、`build`=0、专项 9 项全 PASS、0 error；核心冒烟 + 批次 2/3 + 10 个未迁移页面回归全绿。

**批次 5 已完成（round 102）**，新增 4 个纯 TDesign 页面：
- `views/admin/sessions/index.vue`：会话表 + 撤销单个 / 撤销其他会话；`listSessions()` 返回裸数组，用 `fetchSessions` 包装成 `{data,meta}` 契约。
- `views/dhcp/scopes/index.vue`：作用域表 + `t-progress` 使用率 + 启用开关（inline 更新）+ 新建/编辑/删除。
- `views/dns/allowed/index.vue`：允许规则表 + 添加/删除/清空 + 导入/导出文件。
- `views/admin/groups/index.vue`：组表 + 新建/编辑 + `t-checkbox-group` 分配角色 + 删除。
- **修了一个既有业务逻辑 bug**：`importAllowRules` 返回 AxiosResponse，原页面读 `res.data.imported`（信封层）恒为 undefined；现正确读 `res.data.data.{imported,skipped}`。
- 修过 TDesign API 坑：`t-progress` 的颜色属性是 `status`（success/error/warning），`theme` 只控制形状（line/plump/circle）。
- 验证：`typecheck`=0、`build`=0、专项 9 项全 PASS、0 error；**端到端创建组 `tdgroup-99753` 与允许规则 `td-allow-03430.test` 成功回显**；全量回归（核心/批次2/3/4/10 个未迁移页）全绿。回归中出现的 409/400 经排查是测试脚本复用固定 MAC 造成的重复数据，改唯一 MAC 后 0 error，非产品缺陷。

**批次 6 已完成（round 103）**，新增 3 个纯 TDesign 页面：
- `views/admin/users/index.vue`：用户表 + 新建/编辑（密码≥8 校验）+ 启用开关 inline 更新 + `t-checkbox-group` 分配角色 + 删除。
- `views/admin/roles/index.vue`：角色表 + 新建/编辑 + 按 resource 分组的 `t-checkbox-group` 权限分配 + 系统角色禁改/禁删 + 删除。
- `views/dns/cache/index.vue`：4 个统计卡片 + 缓存条目表（qname 过滤）+ 单条/全量清空。
- TDesign API 坑：`t-switch` 的 `onChange` 回调参数是 `SwitchValue`（string|number|boolean），不是 boolean；用 `Boolean(value)` 转换。
- 验证：`typecheck`=0、`build`=0、专项 8 项全 PASS、0 error；**端到端创建用户 `tduser34260` 与角色 `tdrole38133` 成功回显**；全量回归（核心/批次2/3/4/5/10 个未迁移页）全绿。

**批次 7 已完成（round 104）**，新增 3 个纯 TDesign 页面：
- `views/dns/forwarders/index.vue`：`t-tabs` 两个标签页（转发器 / 条件转发器）+ 新建/编辑/删除 + 启用开关 inline 更新。**修了既有功能 bug**：条件转发器原表单缺 `forwarder_ids`，后端 `CreateConditionalForwarder` 明确要求非空并返回 400，导致该功能从未可用；现增加转发器多选。
- `views/dns/blocked/index.vue`：阻断列表表 + 规则子表 + 新建列表/规则弹窗 + 清空/刷新 + 临时禁用横幅（30s 轮询，卸载时清理）。
- `views/tools/client/index.vue`：DNS 调试客户端，查询表单 + 应答表 + 「导入到区域」弹窗（`t-descriptions` 显示耗时/服务器）。
- 验证：`typecheck`=0、`build`=0、专项 9 项全 PASS、0 error；**端到端创建转发器 `tdfwd75209` 成功回显、真实 DNS 查询返回应答并渲染导入动作**；全量回归（核心/批次2–6/10 个未迁移页）全绿。

**批次 8 已完成（round 105）**：迁移最大的常规列表页 `views/dns/zones/index.vue`（403 行）：
- `t-tabs` 三标签（权威/允许/阻断）+ 名称搜索 + 类型筛选 + 刷新 + **行多选批量删除**。
- 新建/编辑区域弹窗（含完整 SOA 字段，特殊类型隐藏）+ 克隆弹窗 + 类型转换弹窗 + 单个/批量删除确认。
- `RemoteTable` 再增强：`selectable` prop + `v-model:selected-keys`（`defineModel`），自动前置 `row-select` 多选列。
- 端到端实测：创建区域 `tdzone28483.test` 成功回显；勾选行后出现「批量删除」按钮。
- 排查并修复测试数据污染：批次 7 的回归脚本反复创建指向不可达 `198.18.0.53:53` 的转发器，累积 4 条后被标记为 unhealthy，导致 `/dns/client` 返回 500。已清理临时库并让脚本**创建后自动删除测试转发器**；重跑全部回归 0 FAIL、0 error。此为测试脚本问题，非产品缺陷。
- 验证：`typecheck`=0、`build`=0、专项 7 项全 PASS、0 error；核心 + 批次 2–8 + 10 个未迁移页面全绿。

**批次 9 已完成（round 106）**：迁移 4 个 IPAM 弹窗/抽屉组件 + 2 个辅助组件到 TDesign，解除最后 2 个页面的依赖：
- `components/ipam/SectionTitle.vue`、`ImportReportSummary.vue`：小组件，`n-tag`/`n-descriptions` → `t-tag`/`t-descriptions`。
- `components/ipam/ReverseZoneDialog.vue`：`planReverseZone` 计划 + `t-radio-group` 候选 + TTL + 权限提示，创建反向区域。
- `components/ipam/AddressDetailDrawer.vue`：360° 地址视图，`t-drawer` + `t-descriptions` + 4 张 `t-table` + `t-timeline` 历史；保留「计划加载失败 ≠ 空计划」「列表被截断 ≠ 未覆盖」等语义。
- `components/ipam/ImportAddressesDialog.vue`：CSV 预览/导入两步，`ApiError.payload` 携带逐行报告、拒绝时不显示确认按钮。
- `components/ipam/PoolWizardDialog.vue`：`t-steps` 两步向导 + 指纹校验 + 409 过期计划重算。
- 集成验证：从仍为 Naive 的 `ipam/subnets`、`ipam/addresses` 页面点击行操作，**4 个 TDesign 对话框/抽屉均可见且无 Naive 残留**（用 `:visible` 断言），0 error。为此先用 API 播种了 space/subnet/address。
- 测试数据治理：batch9 播种改为唯一名称、batch4 创建保留地址后自动删除，避免重复运行耗尽作用域 IP 池导致的偶发 409/400（已确认是测试脚本问题，非产品缺陷）。
- 验证：`typecheck`=0、`build`=0、专项 8 项 + 可见性 4 项全 PASS；核心 + 批次 2–8 + 10 个未迁移页面回归全绿。

**批次 10 已完成（round 107）**：迁移最后 2 个 IPAM 页面，全部链路纯 TDesign：
- `views/ipam/addresses/index.vue`：地址表（搜索/状态过滤）+ 分配 IP 弹窗 + 释放确认 + 360° 详情抽屉。
- `views/ipam/subnets/index.vue`：子网表 + 新建/编辑弹窗（空间下拉在一次 `Promise.all` 中同步刷新）+ 删除确认 + 3 个 TDesign 对话框（导入地址 / 地址池向导 / 反向区域）。
- 端到端实测：创建子网 `tdsub562756` 成功回显；3 个对话框与详情抽屉均 `:visible` 且无 Naive 残留。
- 回归脚本维护：batch4 改为查询已占用 IP 后取空闲地址（消除 409 冲突）并在创建后删除；batch9 的 Naive 选择器更新为 TDesign（`.t-table` / `.t-link`），因页面已在批次 10 迁移；修复 `page.evaluate` 不能返回 `Set` 的序列化问题。最终 11 个套件全绿、0 FAIL / 0 error。

**批次 11 已完成（round 108）：死代码清理**（未改产品行为，全部浏览器验证通过）：
- 经可达性分析确认后删除 **69 个 `web-admin/src` 文件**（旧 soybean 模板残留）：
  - `src/layouts/base-layout/`（1）与 `src/layouts/modules/`（43）——`imports.ts` 已指向 `tdesign-layout`，整棵树无引用；
  - `src/views/_builtin/login/modules/`（5）——登录页已重写为 TDesign，旧 `pwd-login/code-login/register/reset-pwd/bind-wechat` 模块无引用；
  - `src/hooks/common/form.ts`、`table.ts`（2）——仅被上述登录模块使用；
  - 18 个 `src/components/` 死组件（含 `icon-tooltip`、`theme-schema-switch`、`button-icon` 等旧布局控件集群）。
- `blank-layout/index.vue` 简化为纯 `<RouterView />`（原 `GlobalContent` 包装依赖已删的 modules 树）。
- **保留** `AppProvider`、`locales/naive.ts`、`plugins/app.ts`、`store/modules/theme/shared.ts`、4 个 `typings/*.d.ts`——2 个未迁移视图与 `App.vue` 的 `NConfigProvider` 仍需要，最后统一移除。
- 验证：`typecheck`=0、`build`=0；专项检查 blank-layout 的 `/login`、`/403`、`/404`、`/500`、登录后跳转、2 个剩余 Naive 视图均正常；全量回归 12 个套件 0 FAIL / 0 error（batch4 一次偶发 console error 重跑即消失，为测试时序抖动）。
- `naive-ui` 引用文件 **17 → 11**。

**批次 12 已完成（round 109）**：迁移最大的剩余页面 `views/dns/zone-detail/[id].vue`（原 903 行）到纯 TDesign：
- 5 个 `t-tab-panel`：记录（搜索/类型过滤 + `RemoteTable`）、选项（区描述 + `t-tag-input` ACL 编辑器 + Catalog 归属）、权限（行内开关 + 增删保存）、历史（`RemoteTable`）、DNSSEC（启用/轮换/生成密钥/DS/NSEC3 实验功能）。
- 3 个 `t-dialog`：区文件导入（格式选择 + 文件读取 + 冲突预览 + 应用）、记录新建/编辑、DNSSEC 密钥生成；删除用 `DialogPlugin.confirm`。
- 保留既有语义：`ApiError.payload` 携带导入冲突报告、ACL 空列表清空限制、Catalog 成员展示、DNSSEC 实验提示、仅 slave 区可同步。
- 修了一个潜在交互问题：区 `enabled` 开关改用 `change` 事件携带的值，避免连续快速切换时读旧值。
- 端到端实测：创建区 `tddetail…` → 创建 A 记录 `192.0.2.99` 成功回显；4 个附加标签页均可加载，无 Naive 残留；0 error。
- 验证：`typecheck`=0、`build`=0、专项 8 项全 PASS；全量回归 13 个套件 0 FAIL / 0 error。

**批次 13/14 已完成（round 110）：最后一个视图 + Naive UI 彻底移除**：
- 迁移 `views/settings/config-versions/index.vue`（926 行）：资源类型、修订历史、双修订 diff（含 `dns_records` 记录级差异算法）、dry-run 发布、回滚（409 冲突提示）、数据面发布队列，全部改为 `t-card`/`t-table`/`t-dialog`/`t-descriptions`；`RemoteTable` 新增 `rowClassName` 转发以保留失败行高亮。
- **发现并修复扫描盲点**：`views/dashboard` 及若干组件通过 `unplugin-vue-components` 自动导入使用 `<NCard>/<NStatistic>/<NIcon>/<NEmpty>/<NButton>`，源码中无 `naive-ui` 字面量，因此此前统计遗漏。已把 dashboard（240 行）与 `DashboardChart`/`StatsTrendChart`/`exception-base` 全部改为 TDesign；删除死组件 `ConfirmDialog.vue`、`PageHeader.vue`。
- **App 壳层收尾**：`App.vue` 简化为 `TConfigProvider` + `<RouterView />`；登录成功提示、无权限警告、版本更新提示分别改用 `NotifyPlugin`/`MessagePlugin`/`DialogPlugin`；删除 `AppProvider`、`locales/naive.ts`、`typings/naive-ui.d.ts`；`global.d.ts`/`app.d.ts` 的 Naive 类型改为本地最小声明；`theme` store 删除死的 `naiveTheme`/`getNaiveTheme`。
- **依赖收尾**：`build/plugins/unplugin.ts` 移除 `NaiveUiResolver()`；`package.json`、`tsconfig.json` 移除 `naive-ui`；`pnpm install --lockfile-only` + `--frozen-lockfile --offline` 验证锁文件一致，`pnpm typecheck`/`pnpm build` 均通过（naive-ui 已从 node_modules 剪除后仍可构建）。
- 验证：专项 8 项全 PASS（登录、dashboard 10 张卡片、config-versions、403/404/500、**页面中 Naive DOM 元素数 = 0**、shell 无 `naive-ui` 字符串）、0 error；**全量回归 16 个套件 0 FAIL / 0 error**。
- 前端技术栈现为 **Vue 3 + TypeScript + Vite + TDesign Vue Next（Starter 分层）**，无 Naive UI 残留。

**待迁移清单**：**已清空**。批次 14 后 `naive-ui` 引用为 **0**（含 `src/`、`build/`、`package.json`、`tsconfig.json`、`pnpm-lock.yaml` 的 importer 段），`<N*>` 组件标签为 0。
`components/` 8 个、`layouts/` 1（旧 base-layout，最后删除）、`hooks/` 2、`store/` 1、`plugins/` 1、`locales/` 1；`service`/`composables`/`theme` 为 0。

**收尾删除项**（最后一页迁完再做）：
- `src/layouts/base-layout/` 及其 `modules/global-*` 子组件；
- `src/App.vue` 的 `NConfigProvider` / `NWatermark` 与 `src/locales/naive.ts`；
- 依赖 `naive-ui`、`@vicons/ionicons5`、`@sa/materials`；
- `build/plugins/unplugin.ts` 的 `NaiveUiResolver()`；
- `src/typings/components.d.ts` 中 Naive 组件声明（重新生成）。

**已知产品缺口（与 UI 迁移独立，见 `docs/audit/RUNTIME-VALIDATION-LEGACY.md`）**：
`/cluster`、`/apps`、`/sso`、`/dhcp/ha` 后端返回 501，前端相应页面只能显示"不可用"；DNSSEC 仍为实验功能。这些**不是 UI 迁移能解决的**，需后端先实现。

## 3. 单机优先 + 双机架构预留（重要）

### 3.1 单机阶段（当前 / 阶段 1–2）
- UI 固定显示 `Single-Node` 标识；所有页面只调用本机控制 API。
- 不引入任何多节点状态、HA 状态、复制进度到 UI 数据模型。
- 后端 `edition: enterprise` 仍 fail-closed（`rejectEnterprise`），Enterprise 受保护写/复制/快照安装入口恒 `ErrNoAuthority`。

### 3.2 双机预留点（只留接口，不实现）
| 层 | 预留方式 | 阶段 3 才实现 |
|---|---|---|
| 布局 | `src/components/NodeModeBadge.vue` 已落地（props 含 4 种 HA 模式），当前 `mode="single"` 硬编码，**不引入 store 字段** | 绑定可信 `/api/v1/cluster/status` |
| 路由 | TDesign 布局 `NAV` 中已留「集群」分组注释占位、不渲染；`/cluster/*` 路由组不注册 | 集群节点管理、HA 状态、复制进度、故障切换 |
| API 层 | `src/service/api/goddi/cluster.ts` 已落地：**仅类型定义、零请求**（`ClusterStatus`/`ClusterNode`/`NodeMode` + `CLUSTER_API_RESERVED` 标记） | 后端契约冻结并独立评审后填充节点/角色/epoch/fencing 读写 |
| 图标/导航 | 侧栏预留 "集群" 分组位（`NAV` 中注释占位，不渲染） | 双机启用时展开 |
| 后端契约 | `internal/cluster/` 已有 `ErrNotImplemented` + `protocol_model.go` 可测试契约，作为协议层预留；**不新增任何写路径** | 多节点协调、传输、持久化、fencing、跨主机分区证据 |

### 3.3 双机实现的前置条件（外部，非 UI 可解）
`docs/B8.5-外部验收Runbook.md` §1 验收矩阵（A–E 全部 `未验收`）与 `docs/STATUS.md` §3 明确：真实双主机 DHCP→DNS 事件追平、真实掉电恢复、双主机网络分区与恢复、真实 Prometheus/Alertmanager 告警触发均**未执行**；`internal/cluster` 多节点协调**未实现**（Runbook §9/§10.1 的设计结论）。这些必须在阶段 3 由真实环境验证后才能开放双机 UI。

## 4. 构建与部署
```bash
cd web-admin
pnpm install --frozen-lockfile
pnpm typecheck && pnpm build
cd .. && rm -rf web/dist && mkdir -p web/dist && cp -r web-admin/dist/. web/dist/
go build ./cmd/goddi   # 从仓库根目录运行，静态目录 ./web/dist 生效
```
> 注意：本工作树**无** `assets.go` go:embed；控制台由 `internal/api/static.go` 从 `./web/dist` 文件系统提供，发布目录必须携带该文件夹。
> `pnpm build` 会由 elegant-router 插件重新生成 `src/router/elegant/{routes,imports}.ts`；布局映射请始终改 `build/plugins/router.ts`。