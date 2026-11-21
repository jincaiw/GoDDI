# Enterprise IPAM 占用复制事件 v1

本切片制定首个占用事件 v1，待系统集成及迁移评审；它不是资料包原有示例已冻结的业务 schema。它不启用复制应用、不授予写权限，不替代接收方校验、事务门禁、作用域授权或迁移对账。资料包依据：`资料包(外部，未随仓库提供)/docs/02-data-ipam.md` 第 20–25、34–48、52–73 行及 `资料包(外部，未随仓库提供)/docs/04-replication-ha.md` 第 24–49 行；SQL 参考 `资料包(外部，未随仓库提供)/contracts/001_ipam.sql` 第 165–184 行。

复用事件封套 `changes`、`kind`、`objectId`、`revision`、`body`：`ipam.occupancy.set` 表示已决定的完整占用行，`ipam.occupancy.cleared` 表示删除前完整行的墓碑。二者 body 的 `id` 与封套 `objectId`、body 的 `revision` 与封套 `revision` 必须分别逐字相等。删除 revision 是被删除行的最后修订，不是新建修订；该 ID 永不复用。接收方须核对同租户、空间、ID、子网、地址、创建时间及修订和现有行一致，拒绝缺失、冲突和陈旧墓碑；重放只按已验证日志位置/哈希去重，不能通过重用 ID 覆盖新占用。此处只定义数据与检查要求，未实现数据库应用。

body 全字段必需，禁止未知键、重复键和大小写别名：

- `tenantId`、`spaceId`、`id`、`subnetId`：小写规范 UUID 文本，不接受 nil UUID。
- `family`：JSON 数字 4 或 6；`address`：该地址族的规范文本，IPv6 按 Go `netip` 的压缩小写输出；拒绝 zone、IPv4-mapped IPv6、非规范拼写。存储时依 `ipamaddr` 转成 16 字节网络序，IPv4 高 12 字节为零。
- `kind`：`HOLD`、`STATIC`、`RESERVATION`、`EXCLUDED`；`ownerRef`：1–1024 字节可打印 ASCII（本切片的保守限制，需集成评审）。
- `createdAtMs`：非负 SQLite int64 的规范十进制字符串；`revision`：正 int64 的规范十进制字符串。禁止符号、前导零及溢出。
- `expiresAtMs`：HOLD 时为大于 `createdAtMs` 的非负 int64 规范十进制字符串；其他 kind 必须为 JSON `null`。不在解码时按本机时钟重算到期。

上游须在受控写事务内验证合法状态转换及范围，连同业务变更、事件、审计与幂等收据原子提交。唯一普通就地变化是符合授权和可信时间条件的 HOLD→STATIC，修订恰增一；删除和其他专用工作流需检查运行面引用。接收方还须核验父子范围、地址唯一、活动租约和跨对象不变量；本切片形状检查不能替代这些验证。

黄金 JSON（无末尾换行）：

```json
{"changes":[{"kind":"ipam.occupancy.set","objectId":"33333333-3333-4333-8333-333333333333","revision":"1","body":{"tenantId":"11111111-1111-4111-8111-111111111111","spaceId":"22222222-2222-4222-8222-222222222222","id":"33333333-3333-4333-8333-333333333333","subnetId":"44444444-4444-4444-8444-444444444444","family":4,"address":"192.0.2.10","kind":"STATIC","ownerRef":"host-1","createdAtMs":"100","expiresAtMs":null,"revision":"1"}}]}
```

此 JSON 是编码示例而非整个事件的规范化要求；接受合法 JSON 空白时仍必须对**原始 payload 字节**使用 eventlog 散列，不可解码后重编码。`ipamevent.DecodePayload` 当前只接收本切片的两种 kind；混合领域 batch 需要在后续实现独立分派。生产兼容性、签名与身份、迁移、接收方重放和故障验收尚未完成。
