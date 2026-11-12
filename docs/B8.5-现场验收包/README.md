# B8.5 现场验收包（Field Acceptance Package）

> 这是**现场可执行的验收包索引**，把 `docs/B8.5-外部验收Runbook.md` 的 A–E 场景落成"现场照做即可"的清单+模板+采集脚本。**仓内证据不替代真机证据**；未按本包在真实环境执行并归档原始证据前，B8.5 A–E 一律 **BLOCKED / 未验收**。

## 0. 使用步骤
1. 准备环境：两台以上真实主机、可控网络隔离、fencing/witness、真实断电与持久盘、真实 DHCP relay/Option 82 设备与客户端、真实 Prometheus + `promtool` + Alertmanager。详见 Runbook §2。
2. 每台机器安装同一发布镜像；记录 `b85_collect_evidence.sh baseline`。
3. 按场景 A→E 逐项执行（Runbook §3–§7 的步骤与通过判据）。
4. 每个场景执行前后各跑一次 `b85_collect_evidence.sh collect <scenario>` 采集原始证据。
5. 在 `RESULTS.md` 逐行登记 `PASS / PARTIAL / FAIL / BLOCKED` 与证据目录，现场负责人签字。

## 1. 场景与判据（摘要，全文以 Runbook 为准）
| 场景 | 内容 | 通过判据（摘要） | 关键原始证据 |
|---|---|---|---|
| A | 真实 DHCP relay / Option 82 / ACK | 真实设备互操作、报文事实正确、租约与 ACK 正确 | pcap（DISCOVER/REQUEST/ACK）、relay 配置、服务日志 |
| B | 跨进程 DHCP→DNS 最终一致 | 绑定后名字可解析、删除后撤销、追平无残留 | 服务日志、DNS 查询记录、DB 快照 |
| C | 真实掉电恢复 | 掉电重启后租约/事实不丢、无重复/错地址 | 掉电时间线、启动日志、DB 前后快照 |
| D | 双主机网络分区与恢复 | 少数侧 fail-closed、不双写、恢复后对账无分叉 | 双向 pcap、两侧日志、水位记录 |
| E | Prometheus/promtool/告警 | 规则 `promtool` 通过、真实告警触发与恢复 | `/metrics` 快照、`promtool` 输出、Alertmanager 时间线 |

## 2. 目录内容
- `evidence-template.txt`：每个场景的登记模板（复制到证据目录 README）。
- `RESULTS.md`：结果登记表（预填 BLOCKED，待现场填写）。
- `scripts/b85_collect_evidence.sh`：采集版本/提交/二进制哈希、配置、`/metrics`、可选 pcap、DB 快照清单。

## 3. 硬性规则
- 不得用单机、回环 UDP、SQLite 写锁、`kill -9`、静态源码检查或单次 `/metrics` curl 冒充真机通过。
- 任何一项缺原始证据即为 `BLOCKED` 或 `PARTIAL`，不得写 `PASS`。
- 独立专家签字后再冻结相关候选（兼容矩阵/快照迁移/UPSERT/`dnswire`）。
