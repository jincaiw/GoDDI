# ADR-0008：首发能力范围与 DNS 区域传送安全

- 状态：已接受（2026-09-23）
- 基线：`v0.10.0 / d3ddb16`；实施工作树为独立评审分支
- 依据：用户授权由工程负责人直接确定技术默认值并开始实施
- 相关：ADR-0001、ADR-0003、ADR-0004、ADR-0006、ADR-0007

## 背景

v1.1 的企业级评估以 `v0.5.2` 为代码基线。最新版已经包含后续 DHCP、配置发布、恢复和事件投递工作；其中一部分仍是未提交原型。继续用 v1.1 的差距清单逐条开发会重复建设，也会把原型误认成生产保证。

DNS 区域传送还存在更基础的完整性问题：`dns_zone_changes` 不是所有区域写路径共同维护的事务日志。记录创建、修改、删除、批量操作及动态更新的 serial 和 journal 写入并非始终处于同一事务；部分更新路径不写完整差异。现有 IXFR 组装逻辑也没有输出每个差异集所需的旧 SOA 与新 SOA 边界。用不完整历史产生增量，可能让副本获得表面成功、实际缺记录的区域。

## 决策

### 1. 交付范围先以 IPv4 DHCP 为准

本轮生产化主线以 IPv4 DHCP 为首发网络画像。IPv6 DNS 记录和 IPAM 能力逐项按现有实现与证据声明；IPv6 DHCP、双栈生命周期和 IPv6-only 站点不纳入本轮 GA 承诺，除非后续用户场景明确要求并完成专项验收。

理由：当前评估材料没有足够的真实站点、客户端和 relay 画像，不能为了表面完整同时扩大 DHCP 协议、IPAM 容量和 HA 范围。IPv4 先形成闭环，降低发布面和故障组合数量。

### 2. DHCP 默认采用可证明的单写热备安全边界

沿用 ADR-0003 的决定：primary 是唯一租约作者；未确认旧主已围栏时不自动提升 standby；严格保护模式要求 standby 持久提交后 primary 才发送成功 ACK。对端不可达时默认暂停新的租约承诺和续租延长，不发送错误 NAK；OFFER、RELEASE、DECLINE 按 ADR-0003 的安全语义处理。单副本降级只允许运维显式批准，并持续标记为降级。没有围栏/见证条件时不承诺网络分区期间持续分配。

SQLite/WAL 的本地 durability、standby durable ACK、应用崩溃和真实断电分别验收。进程重启测试不替代目标文件系统及硬件上的掉电证据。

### 3. 暂停 IXFR 增量应答，统一回退到完整 AXFR

在所有区域 mutation 都能将 RR 变更、SOA serial 与完整 journal marker 原子提交之前，非当前 serial 的 IXFR 请求返回完整 AXFR。serial 完全相同时仍可返回单条当前 SOA。AXFR 遇到记录扫描错误、无效/不支持 RR 或缺少 SOA 时必须失败，不得静默跳过后返回看似完整的数据。

这符合 RFC 1995 允许服务端在无法提供增量时返回全区的规则。代价是传输体积和副本追赶时间增加；它比输出无法证明完整的 IXFR 更安全。

恢复 IXFR 的前置条件：

1. 所有区域变更入口共用事务边界，原子提交记录、SOA serial、完整 change set 和单调 journal sequence。
2. 导入、目录区、批处理、管理 API 与 RFC 2136 UPDATE 全部纳入同一语义；遗漏一个写路径就不能打开增量应答。
3. journal 有明确的保留起点/快照基线；客户端版本早于保留起点、遇到 gap、损坏、未知 RR 或未识别 schema 时回退 AXFR。
4. IXFR 编码符合 RFC 1995 的旧 SOA、删除、新 SOA、添加顺序，并遵循 RFC 1982 的 32 位 serial 比较规则。
5. 协议互操作用例覆盖单个和多个差异集、删除/添加/替换、serial 回绕、历史裁剪、并发写入与 journal 损坏。

### 4. HA、容量和一致性承诺由证据决定

不设通用 QPS、切换秒数、事件延迟或 RPO 数字。先固定节点规格、网络拓扑、租约峰值、DNS 请求混合比例、数据盘和客户端矩阵，再按相同配置测量。真实多主机 fencing、掉电、relay、告警联动和长稳未有现场证据时保持未验收状态。

## 后果

- 现有普通 IXFR 客户将继续获得有效区域内容，但请求不再节省传输带宽；副本追赶可能需要更多网络与时间。
- AXFR 会在底层数据不可解码时明确失败，运维需要修复源数据后重试，不会收到部分区域并误以为同步成功。
- IXFR journal 需作为一个完整写模型重构，不再把 `dns_zone_changes` 中“有一些行”视为覆盖完整历史。
- IPv6 和 HA 的功能声明必须按独立能力证据分级，不从通用 IP 字段或单机模拟推导生产能力。

## 实施与验收

1. 已实施首批安全边界：IXFR 非当前 serial 回退 AXFR；AXFR 完整性校验 fail-closed，records 与 SOA 从同一 SQLite 读快照取得。
2. 已将 RecordManager 单条/批量增删改、CSV/zone-file 导入及过期清理的记录、serial 与 journal 写入纳入同一 SQL 事务；这只是 W01 的局部收敛，不代表所有 zone mutation 已完整记录。
3. catalog 成员 PTR 与 zone catalog 属性的加入、切换、移除及 zone 创建/删除现已在同一数据库事务提交，并推进涉及 catalog zone 的 serial/history；删除 catalog 时同步清除成员关联。启用 CreatePTR 的 A/AAAA 创建会把正向记录和反向 PTR、serial/history 放在同一数据库事务中。管理 API、zone 类型转换、DHCP A/PTR 投影和配置发布现已在各自本地事务中推进 serial；DHCP RRset 变化提交后也会异步发送 primary NOTIFY。跨库传播与完整 history 编码仍未闭合，不得重新启用 IXFR。
4. Secondary 全量刷新校验 AXFR 所有 RR 为 IN 类、第一条和最后一条为 SOA、首尾 TTL/SOA 数据一致以及所有 RR owner 的区域归属；若响应不完整、含存储模型不支持或不能无损表示的 RR（例如 TXT 字符串边界不能重建），或任一记录写入失败，则回滚整次本地替换。显式 TTL=0 会原样保留；此前“跳过失败记录后仍提交”的部分成功路径已关闭。协议与运行态回归仍待执行。
5. 内存 Store 在一个 SQLite 只读事务里读取 zone 元数据与记录；任何行扫描、迭代或记录构建错误会放弃整份新快照，继续使用上一份完整快照。
6. 区域改名会在同一个本地事务里迁移区域内绝对 owner、更新 catalog 成员 PTR，并推进相关区域 serial/history。catalog 成员属性与 PTR 的普通增删/切换也已改为原子提交。
7. DNSSEC 当前的启用开关和 NSEC3 参数尚不改变权威应答，因为签名、DNSKEY/RRSIG/NSEC 发布尚未实现。签名真正接入时，必须把签名 RRset 和 SOA serial 当作同一个发布变更处理。
8. 将当前工作树未提交的 N5/N6 改动逐文件审查后再决定是否进入后续实现，不复制或覆盖未审查工作。
9. 每个实现批次需补回归用例并由后续验证轮执行；未运行的修复不得标成通过。

10. 数据面 DNS 配置同步会在应用事务内比较 primary 区域的 SOA 和可见 RRset；提交后刷新内存 Store，并只通知实际变化的区域。删除后的区域无法由当前配置发送 NOTIFY，secondary 的删除传播仍依赖刷新/EXPIRE 路径验收。
11. 内存合成 SOA TTL 与 AXFR 的默认 TTL 统一，避免查询和区域传送出现不同 TTL。
12. RFC 2136 更新在 RRset、serial 与 history 事务提交后同步重载 Store，并发送 primary NOTIFY；纯 no-op 不触发通知。相关协议与运行态回归仍待执行。
13. 启用 CreatePTR 的 A/AAAA 创建将正向和反向记录连同各自 serial/history 原子提交；无匹配反向区时仍仅创建正向记录。相关失败回滚与幂等回归尚待执行。
14. Store 按最近到期记录安排快照重载，AXFR 使用同一有效期过滤；后台数据库清理与 serial/NOTIFY 最迟在一分钟轮询内推进。权威内存应答可按到期边界移除，但 secondary 的传播延迟及时间边界仍待验收。
15. Store 关闭会取消 debounce 和 expiry 定时器并等待进行中的数据库读取，主服务在关闭数据面数据库前完成此步骤。
16. RFC 2136 TXT 新增与按 RDATA 删除在存储前校验字符段边界可逆；schema 无法无损表示的消息整条拒绝，以免返回成功却改变实际 TXT RDATA。
