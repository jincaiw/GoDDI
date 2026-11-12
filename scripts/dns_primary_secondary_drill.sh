#!/usr/bin/env bash
# DNS 方案A（标准主从）loopback 演练：起真实 primary DNS 服务器（AXFR/TSIG）
# 与真实 secondary（SecondarySync）经 TCP 拉取，用真实 DNS 查询验证复制、变更
# 再同步、以及"停掉 primary 后 secondary 仍应答"的读容灾。无需外部 dig。
set -uo pipefail
cd "$(dirname "$0")/.."
echo "== DNS 方案A 主从演练（loopback, 真实 DNS 服务器 + SecondarySync + DNS 查询）=="
go test -mod=readonly ./internal/dns/transfer/ -run 'TestPrimarySecondary' -count=1 -v
status=$?
echo "===================================================="
if [ $status -eq 0 ]; then echo "DRILL: OK"; else echo "DRILL: FAIL"; fi
exit $status
