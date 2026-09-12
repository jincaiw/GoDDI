# ADR-0006：安全姿态与能力声明边界

- 状态：**已接受，作为声明与契约基线**（2026-09-12）
- 范围：B8.8
- 基线：已发布 `v0.8.0` / `28ed5aa`

## 决策摘要

### 管理面 TLS

- 内置管理面 TLS 可用，但默认关闭；启用时必须同时提供证书和私钥，启动阶段校验失败即拒绝启动。
- 默认最低 TLS 版本为 TLS 1.2，TLS 1.2 cipher suite 显式列出；可选 mTLS 使用 `RequireAndVerifyClientCert`。
- 证书文件变化时运行时热加载；新证书损坏或不匹配时继续使用旧证书并记录错误，不能中断既有服务。
- 未启用内置 TLS 时，管理面必须位于 HTTPS 反向代理或可信隔离网络之后；文档不得把默认 HTTP 描述为安全传输。
- 真实企业 CA、证书轮换流程、吊销和多主机部署仍需外部验收。

### DNSSEC

- 当前支持密钥生成、DS 计算、实验性启用入口和状态查询。
- 当前不生成或服务 RRSIG、DNSKEY、NSEC/NSEC3；`signed=false`、`experimental=true` 是真实状态。
- `dnssec_enabled` 不能单独解释为“权威区已受 DNSSEC 保护”；生产环境不得以当前入口作为完整签名链路。
- 完整签名引擎、密钥托管、轮换窗口和验证器互操作属于后续独立能力，不纳入本 ADR 的已完成范围。

### 扩展 API

- SSO、Cluster、Apps、管理面 DHCP HA 扩展当前继续返回 501，并由 OpenAPI 501 契约测试钉住。
- `internal/cluster.ErrNotImplemented` 必须保留，直到 B8.7 的真实多节点协议和验收完成。
- DoT、DoH、DoQ 配置入口已有实际 listener 实现，不得再写成“仍为 501”；但是否启用、证书路径和真实客户端互操作仍按部署配置验收。
- 旧 PRD/评估文档中的“预留”属于历史规划语义，若描述当前代码能力，必须区分“已有实现”与“规划项”。

## 证据矩阵

| 能力 | 当前仓内事实 | 当前结论 |
|---|---|---|
| 管理面 TLS | `internal/tlsutil` 支持 TLS 1.2/1.3、mTLS、启动校验、热加载；默认由配置关闭 | 可用但默认关闭，需部署保护 |
| DNSSEC | 有 KSK/ZSK/DS 和状态入口；没有 RRSIG/DNSKEY/NSEC/NSEC3 | experimental/incomplete |
| DoT/DoH/DoQ | `ConfigureDoT`、`ConfigureDoH`、`ConfigureDoQ` 共用真实 listener 配置与热替换路径 | 已有实现，需真实握手验收 |
| SSO | handler 明确 501 | 未实现 |
| Cluster | handler 明确 501；cluster 包返回 `ErrNotImplemented` | 未实现 |
| Apps | handler 明确 501 | 未实现 |
| DHCP HA API 扩展 | handler 明确 501；DHCP 数据面 HA 通过独立机制存在 | 扩展 API 未实现，数据面 HA 不混同 |

## 约束

- 文档、OpenAPI、UI 状态和 HTTP 返回码必须使用同一能力分类：`implemented`、`experimental/incomplete`、`reserved/501`、`external-validation-required`。
- 任何仓内单测、静态路径存在或示例配置不能替代真实证书生命周期、真实 DoT/DoH/DoQ 客户端、真实 DNSSEC validator 或企业身份系统验收。
