# internal/enterprise — 候选/参考实现边界

本目录是**企业级能力的候选/参考实现**，**未接入生产运行路径**。它用于：

- 固化跨系统契约（事件链、快照校验、复制 schema、角色/epoch/fencing 原语）；
- 为评审提供可编译、可测试的参考代码（附带大量单测/Fuzz）；
- 支撑 `contracts/*` 中的候选协议与 `docs/audit/*` 的评审材料。

## 边界声明

- **不是生产 DDL / 迁移 / 验收依据**；其中 `ipamstore` 等自带独立参考库与摘要校验，
  仅用于隔离验证。
- **不得与其“生产侧同名主题”相互替代**：`internal/dhcp` ↔ `enterprise/dhcplease`、
  `internal/ipam` ↔ `enterprise/ipamstore`、`internal/dns` ↔ `enterprise/dnswire/dnsanswer`。
- 删除或迁移本目录任何包前，必须先核对引用方（测试、contracts、audit 文档）并走独立评审。

## 规模（审计快照）

- 约 53 个子包、225 个 Go 文件、数百个测试/Fuzz 入口。
- 长期保留或逐步迁出，需在 `docs/audit/REMEDIATION-TRACKING.md` 记录决策后再执行。
