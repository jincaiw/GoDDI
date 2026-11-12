#!/usr/bin/env bash
# Performance baseline: run all Go benchmarks and record a timestamped report.
# Non-destructive; no production service is touched.
set -uo pipefail
cd "$(dirname "$0")/.."
OUT="${BENCH_OUT:-/tmp/goddi-bench-$(date +%Y%m%dT%H%M%S).txt}"
echo "== Go benchmarks ($(go version)) ==" | tee "$OUT"
go test -run '^$' -bench . -benchmem ./internal/... 2>&1 | tee -a "$OUT"
echo "report: $OUT"
