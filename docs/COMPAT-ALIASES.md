# API 兼容别名登记（Compat Aliases）

部分 API 路径是**有意保留的兼容别名**，与主路径指向同一逻辑。新增/删除前请核对本表，
避免误删导致旧客户端失效，或造成同一资源两条写路径语义漂移。

| 兼容别名 | 主路径 | 说明 |
|---|---|---|
| `GET/POST /system/backups`、`.../{id}`、`.../download`、`.../restore` | `/backup`、`/backup/{id}`、`.../download`、`.../restore` | 备份管理兼容路径 |
| `GET /audit-logs` | `/logs/audit` | 审计日志兼容路径 |
| `GET /system/settings`、`PUT /system/settings/{key}` | 设置相关主接口 | 系统设置兼容路径 |
| `POST /ddi/tenants/{tenantId}/ipam/spaces/{spaceId}/allocations` | IPAM 分配主路径 | 企业 STATIC 分配兼容路径（写入受门禁约束） |
| `GET /system/dataplane` | 数据面就绪信息 | 观测兼容路径 |

## 规则

- 兼容别名只做**转发**，不复制业务逻辑；语义以主路径为准。
- OpenAPI `routeDocs`（`internal/api/openapi.go`）必须同时登记别名与主路径，
  由 `TestEveryRegisteredAPIRouteIsDocumentedInOpenAPI` 守护双向一致性。
- 删除别名需先确认无调用方，并在 CHANGELOG 中标注破坏性变更。
