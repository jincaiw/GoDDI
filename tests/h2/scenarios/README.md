# H2 fault scenarios (mapping)

Driver: `tests/h2/scripts/*` and `make h2-fault-test`. Each scenario must save
state transitions, persisted watermarks, real DHCP packets, logs, hashes and
the executed commands under `tests/h2/reports/<timestamp>/`.

| ID | Scenario | Driver |
|---|---|---|
| H2-001 | Node A SIGKILL | kill-node.sh node-a [--restart] |
| H2-002 | Node B SIGKILL | kill-node.sh node-b [--restart] |
| H2-003 | A/B replication partition | network-partition.sh node-b |
| H2-004 | one-way replication fault | network-partition.sh + tc on fault-injector |
| H2-005 | replication latency/loss | tc netem on fault-injector (Linux CI) |
| H2-006 | replication ack lost | app-level fault injector (Linux CI) |
| H2-007 | crash after commit before ACK | kill-node.sh during DORA (script) |
| H2-008 | crash right after ACK | kill-node.sh after client lease |
| H2-009 | duplicate/out-of-order replication | unit/fault harness |
| H2-010 | lease log gap / hash error | tamper harness (unit) |
| H2-011 | facts watermark mismatch | tamper harness (unit) |
| H2-012 | SQLite write fault | fault injector / disk ro |
| H2-013 | disk full | tmpfs size limit (Linux CI) |
| H2-014 | old primary return / epoch conflict | partition + restart old primary |
| H2-015 | DNS config publish interrupted | stop node-b mid-publish |
| H2-016 | IPAM/DDNS consumer fault | fault injector |
| H2-017 | concurrent clients | parallel client containers |
| H2-018 | manual recovery after primary loss | ha takeover/fence CLI |
| H2-019 | management crash, data plane continues | kill control plane process |
| H2-020 | license expiry protection | NOT_IMPLEMENTED product feature |

Scenarios requiring kernel tc/netem or disk limits run only on Linux CI; on
macOS OrbStack they are reported NOT_VERIFIED, never PASS.
