# 预留扩展端点（501）的保留决策记录（round 285）

> 用户指令背景：「不做物理真机验收、独立专家冻结签字」「按建议执行」。
> 原先 7 个 501 端点被描述为「保留至上述证据齐备」；这两类外部证据既已明确不做，
> 「等证据」的前提不再成立，故对 7 个端点逐一作出**产品决策**，把「未完成的待办」
> 收口为「已决策的边界」，避免无限期悬置。

## 1. 决策结果一览

| 端点 | 决策 | 理由 | 重新评估条件 |
|---|---|---|---|
| `GET /dhcp/ha` | **已实施**：round 285 升级为只读状态 API（200） | CLI `goddi ha status` 与 `/ready` 已有完整状态数据；用户明确问过「双机状态怎么查看」；Web 无 HA 状态页；只读实现零产品风险 | — |
| `GET /sso` | 保留 501 | 完整 IdP 集成（OIDC/SAML 配置、登录联邦、密钥轮换、会话迁移）无产品规格；改动落在认证安全面，风险与收益不匹配 | 出现明确的 SSO 产品需求、IdP 选型与验收标准 |
| `PUT /sso` | 保留 501 | 同上（写配置需先有配置模型与权限模型） | 同上 |
| `GET /cluster` | 保留 501 | 多节点集群协调（leader/quorum/transport/配置分发）属 ADR-0005 未实现边界；用户已排除物理多主机与协调证据，容器双节点演练不等于生产协调 | 出现生产级多节点产品规格与现场验收计划 |
| `POST /cluster` | 保留 501 | 同上（节点加入需 fencing/witness 信任域） | 同上 |
| `GET /apps` | 保留 501 | 应用市场运行时（包签名/沙箱/生命周期/权限）无产品规格；前端 `/dns/apps` 已如实渲染「501 Not Implemented」空状态（注释明确为真实产品缺口） | 出现应用市场产品定义、包格式与安全模型 |
| `POST /apps/{id}/install` | 保留 501 | 同上（安装涉及执行任意代码，必须先有信任模型） | 同上 |

**合计：6 个 501 契约有意保留**（原 7 个减去 `/dhcp/ha`）。它们不是"漏做的功能"，而是**明确决定不在本版本提供的产品边界**。

## 2. `/dhcp/ha` 只读 API 的实施边界

- 响应字段：`ha_enabled`、`node_id`、`configured_role`、`role_in_force`、`state`、`redundant`、`promising`，primary 附 `sequence` / `acknowledged_sequence` / `peer_applied_sequence` / `peer_applied_facts_sequence`，standby 附 `applied_sequence` / `peer_last_seen`；HA 未启用时返回 `ha_enabled=false, role=standalone, state=disabled`。
- **只读**：无写方法（`PUT`/`POST`/`DELETE` 均 405/404），不暴露 peer token 或任何密钥，不改变 HA 状态机。
- 守卫：`TestReservedExtensionEndpointsAnswer501` 从 7 项减为 6 项；新增 `TestDHCPHAStatusIsReadOnly`（200 + `ha_enabled` 断言 + 写动词不可用）。
- openapi：`GET /dhcp/ha` 由 `SuccessStatus: 501` 改为 200 + 字段说明；路由权限仍为 `settings:read`。
- 前端未新增页面（属产品决策，本轮不做）；状态也可经 `/ready` 详情与 Prometheus `goddi_dhcp_ha_*` 指标查看。

## 3. 守卫与契约同步清单（改动一处必须同步的全部位置）

1. `internal/api/handler/extension.go` —— stub → 只读 provider。
2. `cmd/goddi/main.go` —— `RegisterDHCPHAStatusProvider` 注册（仅 HA 启用时）。
3. `internal/api/openapi.go` —— 501 → 200、描述更新。
4. `internal/api/rbac_coverage_test.go` —— 守卫表 7→6 + 只读新断言。
5. `docs/STATUS.md` §3 第 5 项 / §5、`docs/audit/ENTERPRISE-REVIEW-DOSSIER.md` §6 第 7 项、`docs/实施计划-v0.9.0-企业级方案复核与生产收敛.md` 主线 4 —— 501 清单与理由改写。
6. 本文件 —— 决策记录。

`TestEveryRegisteredAPIRouteIsDocumentedInOpenAPI` 与 `test/browser/docs.cjs`、`contracts.cjs` 守卫保证上述同步不被遗漏（`cluster.ts` 仍不得发起请求——dual-node 协调依旧保留）。

## 4. 定位

本文档是**决策记录**，不是验收声明、不新增任何权限、不改变任何门禁、不替代任何现场证据。6 个保留端点若要实施，须先有对应的产品规格文档，并按上表重新评估条件启动。
