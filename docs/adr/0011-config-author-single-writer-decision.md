# ADR-0011：多节点配置发布的产品决策——单作者规则（最小可行部分）

- 状态：**Accepted（2026-11-13）**，部分实施
- 修订/补充：ADR-0005（多节点配置发布协调）
- 决策人：工程（按"合理性评估 + 推荐方案"推进，等待产品确认预算后再升级为完整协调器）

## 决策背景

ADR-0005 定义了完整的多节点配置发布协调（leader 选举、quorum/witness、epoch fencing、
watermark 追赶）。H2 验收暴露的现实后果是：两台节点各自有独立控制库，`node-a` 建的 zone
不会出现在 `node-b`，跨节点 Revision/Hash 一致性无法证明——这是 H2 验收中唯一的
**架构级**缺口（其余缺口为物理/内核环境所限）。

完整协调器需要：第三仲裁节点/见证、真实网络分区负向测试、跨主机身份系统、运维 fencing
流程。在当前无物理第三节点的环境中**无法诚实验证**，因此不能实现后宣称通过。

## 决策

**分两步走，先落地可验证的一半。**

### 第 1 步（本次实施）：单作者规则（single-author rule）

- 新增配置项 `cluster.config_author`（默认 `true`，单节点部署行为不变）。
- 部署时**恰好一个节点**设为 `true`；其余节点为 `false`。
- 非作者节点上，配置写请求（`POST/PUT/PATCH/DELETE` 且路径属于 `/dns/`、`/dhcp/`、`/ipam/`、`/ddi/`）
  由 `middleware.ConfigAuthor` 以 **409** 拒绝，响应体说明原因并指向 `cluster.config_author`。
- 非作者节点的**读请求、`/health`、`/metrics`、DNS/DHCP 数据面完全不受影响**——它仍然正常
  提供解析、租约镜像与状态上报。
- 被拒绝的写**不落库**（验收脚本断言非作者库中无该行）。

这消除了最危险的失效模式：**两个控制台各自发布同一资源的不同 revision**。

### 第 2 步（未实施，需外部条件）：完整协调器

保持 ADR-0005 的全部要求不变。在具备以下条件前，`internal/cluster.ErrNotImplemented`
继续保留，不新增伪造的 leader/quorum 字段：

1. 第三仲裁节点或 witness（或明确接受"双节点不自动选主，仅显式 fencing + operator takeover"）；
2. 真实网络分区负向测试（少数侧必须拒绝发布）；
3. 跨主机身份（mTLS 已有基础，需扩展到控制面）；
4. 运维 fencing/rejoin 流程与审计。

## 后果

- **收益**：跨节点"配置分叉"从架构风险降级为部署配置约束，且可自动化验证
  （`tests/h2/scripts/config-author-acceptance.sh`：PASS=9）。
- **代价/限制**：
  - 非作者节点的控制台**只读**，运维需知道"改配置要去作者节点"；
  - 作者节点故障时，配置变更需人工把 `config_author` 迁移到另一节点并重启（与 DHCP
    `ha takeover` 同样是显式运维动作，符合 DEC-002"不自动提升"精神）；
  - 这**不是**多主协调：没有 leader 任期、没有 epoch fencing、没有 watermark 追赶，
    因此**不能**宣称"多节点配置发布已实现"。
- **对 H2 验收的影响**：跨节点配置一致性从"NOT_VERIFIED（未实现）"变为
  "**由单作者规则保证不产生分叉**（PASS）"，但"双主自动协调"仍为 NOT_VERIFIED。

## 验收证据

- 单元测试：`internal/api/middleware/config_author_test.go`（作者放行、非作者 409、读放行）。
- 端到端：`tests/h2/scripts/config-author-acceptance.sh`
  （作者 201 / 非作者 409 / 原因可读 / 三类读 200 / health 200 / 数据面 standby / 拒绝不落库）。
- 配置：`tests/h2/config/node-a.yaml`（`config_author: true`）、`node-b.yaml`（`false`）。
