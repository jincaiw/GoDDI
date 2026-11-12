# DEC-008：单管理员审批仅允许"启动重新验证"，不得跳过一致性检查

- 状态：**已确认策略（2026-11-12 正式化）；实现核对 PARTIAL**
- 权威来源：`docs/adr/0003-dhcp-ha-contract.md`（决策 7 接管）、`docs/adr/0010`

## 决策
- 单人（单管理员）审批只能**启动重新验证/接管流程**，**不得**用"批准"直接跳过一致性检查（日志链完整、Lease/Facts 水位、缺口 accept-gap、旧主 fenced）。
- 不一致或缺口未解释时，接管必须被拒绝；不得以审批替代证据。
- 接管与"镜像重新验证"是两个状态机，不得混用（见 DEC-007）。

## 状态/证据（诚实）
- 实现：`goddi ha takeover` 要求 `--confirm`、`--old-primary-cannot-write`、`--accept-gap <缺口>`，缺一拒绝（`internal/dhcp/ha/role.go` `Takeover`）；缺口不为 0 且未 `--accept-gap` 时拒绝（`ErrUnexplainedGap`）。
- **PARTIAL**：未见"单管理员审批=仅启动重新验证"的独立审批实体/工作流；takeover 目前是 CLI 本地动作，未强制"近期 MFA + 理由 + 审计入库 + 证据"全链路（审计为本地 CLI 审计行）。独立"镜像重新验证"状态机未单独实现。
- 后续：把 takeover 接入 RBAC + 近期 MFA + 审批记录 + 证据附件，并拆分"重新验证"与"owner 接管"两个状态机。
