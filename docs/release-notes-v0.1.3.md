# Release Notes v0.1.3

发布日期：2026-09-05
基线：v0.1.2（8929d96）→ v0.1.3

本版本为缺陷修复与安全加固版本，无新增功能。源于一次全量代码审查（126 个 Go 源文件 + 52 个前端源文件）、API 集成测试（76 项）与 DNS 协议级实测。

## 关键修复（P0）

- **DHCP 租约过期判定失效**：`lease_end` 以 RFC3339 写入而过期扫描按 SQLite `datetime('now')` 字符串比较，当天到期租约永不回收，地址池可被耗尽。现改用 `julianday()` 比较（同样影响 scope 删除与会话列表）。
- **登录暴力破解防护失效**：限速 key 使用含端口的 `RemoteAddr`，每次新连接 key 都不同，锁定永不触发。现提取纯 IP。
- **后台任务 panic 击穿整个进程**：worker 增加 recover，任务标记 failed。
- **优雅停机顺序错误**：先停 task manager 后停 HTTP，在途请求提交任务会 panic。现先停 HTTP。

## 主要修复（P1，节选）

- DHCP：多网卡监听改为单次绑定共享 socket（原先第二个接口起必然 EADDRINUSE）；保留（reservation）重复 IP 现返回 409、更新越界 IP 返回 400。
- DNS：本地 zone 内不存在的名字现返回权威 NXDOMAIN/NODATA（附 SOA），不再泄漏给上游；API 创建的 zone 的 apex SOA 查询现可应答；RFC 2136 动态更新按 CLASS 分派（原先删除操作全部落入新增路径）；AXFR 主从同步不再丢失 MX Preference / SRV Priority/Weight/Port / CAA Flag；SOA serial 递增加入事务保护；缓存键修复未知类型串答。
- 认证：JWT 校验精确固定 HS256；失败登录计数改为原子 UPSERT（并发不再绕过锁定）；限速内存表空闲条目回收。
- API：未知 `/api/*` 路径返回 JSON 404（原先被 SPA 兜底返回 200 HTML）；CORS 白名单端口精确匹配；批量设置更新补齐值校验；IPAM 删除非空空间返回 409。
- IPAM：子网 CSV 导入校验 CIDR；/31、/32 区间计算修复；广播地址不再入库为可分配。
- 前端：权限不足导航崩溃修复；JWT base64url 解码修复；zone 列表空结果分页修复。

## 验证

- `go build` / `go vet` / `go test -race ./...`（18 包）全部通过
- API 集成测试 76/76 通过；DNS 权威与递归链路 dig 实测通过
- 详见 [测试与修复报告](测试与修复报告-2026-09-05.md)

## 升级说明

- 无数据库迁移；直接替换二进制重启即可。
- 修复后 `ExpireLeases` 会立即回收此前被滞留的到期租约，重启后地址池占用可能明显下降，属预期行为。
