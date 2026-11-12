# 接管许可候选 v1

本文件定义隔离实现的候选格式，不声称完成 G1 全部字段冻结或跨系统互操作。依据 `/Volumes/My-Data/jason.wa/goddi/docs/04-replication-ha.md` 第 78–94 行：签名校验不是角色授权。调用方仍须独立验证身份、审批职责和授权登记、许可签发与撤销、最高 epoch、候选唯一性、一次性 nonce 持久消费、外部 fencing 实际有效性、可信全局尾部和数据恢复；记录 PROMOTING 并持久化 nonce、DB 世代与新 epoch、读回 fencing、持久启用记录后才考虑开闸。未知尾部保持全局禁止业务写和 DHCP 发包。

候选正文是一个无 BOM 的 UTF-8 JSON 对象，键严格按以下顺序，所有字段均必需且值为非空 ASCII 字符串，字符只允许字母、数字及 `-_.:@`。不支持空格、控制字符、Unicode、转义、数组、数字类型、额外字段、重复键或空白。此严格子集的 JCS 规范化等同于按 ASCII/UTF-16 排序并直接编码字符串；不是通用 JCS 实现。整数必须为无前导零的非负十进制字符串，最大 `9223372036854775807`。

| 字段 | 候选约束 |
|---|---|
| `anchorEpoch`, `anchorSeq`, `anchorHash` | 权威前任最终锚点；epoch 等于 `oldEpoch`，seq 非负，hash 是 32 字节无填充 base64url；(1,0) hash 必须全零；后续 epoch 的 seq=0 可以继承全零锚点，seq>0 在本候选实现中保守要求非零。它只是声明，须与可信冻结证明或批准恢复基线独立核对。 |
| `candidateId`, `clusterId`, `predecessorId` | 非 nil、不同的小写规范 UUID；标识候选、集群及前任。 |
| `fencingDigest`, `planDigest` | 非零 32 字节摘要，无填充规范 base64url；摘要匹配不证明隔离或审批事实。 |
| `kind`, `version` | 分别为 `takeover-permit` 和 `1`。 |
| `newEpoch`, `oldEpoch` | 正 int64，new 严格大于 old；调用方还须检查全局已发行最高值及未决许可。 |
| `nonce` | 非 nil 小写规范 UUID；必须在独立持久角色日志一次性消费，此模块不消费。 |
| `notBeforeMs`, `notAfterMs` | UTC Unix 毫秒，有效区间 `[notBeforeMs, notAfterMs)`；经可信时钟服务得到的完整闭区间 `[lower,upper]` 须落在其内部。若无法证明可信区间则不得调用放行；此处不判定时钟来源或 holdover。 |
| `requester` | 1–256 字节的上述允许字符，表示独立认证主体；不得与任一审批主体相同。 |

分离签名输入为两个按 `keyId` 严格升序且 key ID 不重复的条目，均固定 Ed25519；每个签名为 64 字节无填充规范 base64url。二者签名相同的 `goddi.ha.signature.v1` ASCII 字节、一个 NUL 和正文原始字节。key ID 只能检索本地受信 keyring；各自须具备 `approver` 用途，拥有不同公钥和不同自然人主体，均不同于 requester。此模块接收已解析结构；外层 wire envelope 的重复键、未知字段与大小限制尚需单独冻结和严格校验。拒绝缺失或多余签名，不从输入信任算法。有效期是许可消费时门禁，不是活动主自动过期租约。

签名测试中的黄金正文与由独立 Python cryptography 生成的两条固定签名见 `internal/enterprise/hapermit/permit_test.go`。此候选不实现证据内容模式、恢复清单、身份认证、审计、重放防护或密钥吊销。
