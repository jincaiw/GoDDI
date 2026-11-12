# STATIC 创建成功审计载荷 v1（候选）

此格式是独立候选，未发布、未注册日志，也未实现接收、复制应用、认证或审计归档。它对应 `docs/08-api-ui.md:13` 和当前 `eventlog.StaticAudit` / `static_occupancy_audit`；主事务仍须绑定业务对象、幂等请求及日志，并保证审计原子提交。成功解码不证明来源、角色、时间、授权、业务一致性或已持久化。安全审计与复制日志职责分别保留。

全部字段必需：`version` 固定字符串 `1`；`clusterId`、`tenantId`、`spaceId`、`resourceId`、`requestId` 是非 nil 小写标准 UUID；`epoch` 是 1 至 SQLite int64 上限的规范十进制字符串；`principal` 是 1–256 字节 UTF-8，首尾不含空白且无 NUL/CR/LF；`action` 固定 `ipam.occupancy.create`；`priorRevision` 和 `newRevision` 分别为字符串 `0` 和 `1`；`result` 固定 `SUCCESS`；`occurredAtMs` 是非负 SQLite int64 的规范十进制 UTC Unix 毫秒字符串；`requestDigest` 是请求摘要的恰好 32 字节、无填充规范 base64url，禁止非规范尾位。摘要须由上游绑定同次认证请求，不能充当签名。

候选规范编码是以下字段顺序的紧凑 UTF-8 JSON，字符串转义按 Go `encoding/json.Marshal` 默认规则（包括 HTML 特殊字符及 U+2028/U+2029），无空白、BOM 或尾随内容；并非通用 JCS。单 body 最多 4096 字节。解码重编码逐字节比较，拒绝重复键、未知键、大小写别名、缺失、null、非规范数字/UUID/base64、非法 UTF-8、无效类型和未知版本。跨语言互操作和编码规则须在冻结前评审。未来封套须受整个事件 payload 的 4 MiB 上限约束，并对原始字节散列。

黄金 JSON（无末尾换行）：

```json
{"version":"1","clusterId":"11111111-1111-4111-8111-111111111111","epoch":"1","tenantId":"22222222-2222-4222-8222-222222222222","spaceId":"33333333-3333-4333-8333-333333333333","resourceId":"44444444-4444-4444-8444-444444444444","principal":"user:1","action":"ipam.occupancy.create","requestId":"55555555-5555-4555-8555-555555555555","priorRevision":"0","newRevision":"1","result":"SUCCESS","occurredAtMs":"100","requestDigest":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
```

主体、请求标识及摘要可能敏感；传输需集群认证加密，日志、快照、备份和审计读取必须按授权保护并脱敏。接收方必须核对 cluster/epoch、租户空间、资源、主体、请求摘要与同事务业务和收据的一致性；重复按事件位置与 hash 处理，不得再次执行外部动作。候选未实现这些检查或复制应用，不能把验证通过视作授权或审计已归档。
