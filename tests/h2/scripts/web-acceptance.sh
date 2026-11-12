#!/usr/bin/env bash
# Web console acceptance: run every Playwright suite plus the human-style CRUD
# walkthrough against a running console. Requires an isolated server and the
# harness dependency (test/browser/npm install).
set -uo pipefail
H2DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$H2DIR"
TS=$(date -u +%Y%m%dT%H%M%SZ); OUT=reports; mkdir -p "$OUT"
BASE="${GODDI_BASE_URL:-http://127.0.0.1:18080}"
PASS="${GODDI_ADMIN_PASS:-AdminPass-0123456789}"
export GODDI_BASE_URL="$BASE" GODDI_ADMIN_PASS="$PASS"
cd "$H2DIR/../../test/browser"
total=0; fails=0; errs=0
for f in contracts.cjs i18n.cjs docs.cjs batch2.cjs batch3.cjs batch4.cjs batch5.cjs batch6.cjs \
         batch7.cjs batch8.cjs batch9.cjs batch10.cjs batch11.cjs batch12.cjs batch13.cjs \
         batch14.cjs batch15.cjs batch16.cjs; do
  [ -f "$f" ] || continue
  out=$(node "$f" 2>&1)
  p=$(printf '%s' "$out" | grep -c '^PASS'); fa=$(printf '%s' "$out" | grep -c '^FAIL'); er=$(printf '%s' "$out" | grep -c '^errors: [1-9]')
  total=$((total+p)); fails=$((fails+fa)); errs=$((errs+er))
  printf '%-14s PASS=%-3d FAIL=%d ERRORS=%d\n' "$f" "$p" "$fa" "$er"
  [ "$fa" -gt 0 ] && printf '%s\n' "$out" | grep '^FAIL' | head -3
done
cd "$H2DIR"
W=$(node "$H2DIR/scripts/web-walkthrough.cjs" 2>&1); printf '%s\n' "$W" | tail -9
wp=$(printf '%s' "$W" | grep -c '^PASS'); wf=$(printf '%s' "$W" | grep -c '^FAIL')
total=$((total+wp)); fails=$((fails+wf))
printf '{"suite":"web","timestamp":"%s","pass":%d,"fail":%d,"not_verified":%d,"page_errors":%d}\n' \
  "$TS" "$total" "$fails" "$errs" 0 > "$OUT/web-summary-$TS.json"
echo "----"; echo "WEB PASS=$total FAIL=$fails PAGE_ERRORS=$errs"
[ "$fails" -eq 0 ]
