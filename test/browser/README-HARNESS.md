# 浏览器测试规范（防误判）

本项目的浏览器回归套件曾 4 次产出**假缺陷**，原因都在探针而非产品。本文把这些坑固化为规范，`common.cjs` 提供对应辅助函数。

## 1. 必须用辅助函数，不要手写等价逻辑

| 辅助函数 | 用途 | 替代的错误写法 |
| --- | --- | --- |
| `assertAuthenticated(page, ctx)` | 确认页面已登录且应用已挂载 | 直接断言业务元素，结果探针停在 `/login` 上 |
| `failOnly(page, patterns)` | 只中断指定接口 | 用 `**/api/**` 广域拦截，连 `/auth/me` 也断掉 |
| `visibleDialog(page)` | 取可见对话框 | `.t-dialog` 直接取 `.first()`，命中隐藏克隆 |
| `visibleDialogText(page)` | 读可见对话框全文 | 用 `.t-dialog__body`，实际类名是 `t-dialog__body__icon` |
| `remoteTableError(page)` | 读列表错误态 | 用 `.t-alert--theme-error`，该类名**不存在**，实际是 `.remote-table-error` |

## 2. 五个真实踩过的坑

### 坑 1：广域拦截导致「全站错误态为 0」

拦截 `**/api/v1/**` 时把 `/auth/me` 也中断了，页面被重定向到登录页，于是每个页面都「没有错误态」。

**规范**：只拦截目标接口；先 `assertAuthenticated`。

### 坑 2：忽略多地址空间导致「写入失败」误判

IPAM 分配后列表看不到新记录，一度判定写入失败。实际是列表默认返回另一个地址空间（1270 条），新地址在别的空间。

**规范**：多租户/多空间系统的断言必须显式带空间过滤。

### 坑 3：直接写 localStorage 导致「深色模式失效」

写入 `GODDI_themeSettings.themeScheme='dark'` 后刷新，被 `overrideThemeSettings` 覆盖回 light。

**规范**：主题/布局类验证必须走真实 UI 路径（抽屉 → 下拉 → 选择）。

### 坑 4：猜测 CSS 类名导致「对话框正文为空」

用 `.t-dialog__body` 判断，一直返回 `NONE`，误以为修复失败。实际 confirm 模式渲染的是 `.t-dialog__body__icon`。

**规范**：读整段 `innerText` 而不是依赖猜测的子选择器；先用 `visibleDialogText`。

### 坑 5：等待不足导致「错误态未渲染」

中断接口后只等 1600ms，`remoteTableError` 返回 null；等 2500ms 后正常。

**规范**：用带 `waitFor` 的辅助函数，不要用固定 `sleep` 后立即断言。

## 3. 判定缺陷前的检查清单

报告一个缺陷前，逐条确认：

1. 探针确实处于登录态与应用内（`assertAuthenticated` 通过）；
2. 只中断/篡改了目标接口，未波及其他请求；
3. 数据范围（空间/租户/分页）与产品语义一致；
4. 选择器来自真实 DOM 结构，而非猜测；
5. 已等待到稳定状态（用 `waitFor`，不用固定 sleep）；
6. 若为禁用态、装饰性元素，先核对 WCAG 豁免条款。

## 4. 适用边界

这些规范约束的是**测试正确性**，不改变产品行为。产品缺陷仍按原流程修复并留痕。
