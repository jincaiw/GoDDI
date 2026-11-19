#!/usr/bin/env bash
# Run the 24 browser/static regression suites against an already-running isolated
# GoDDI server. Never point this at production: the suites create and delete
# test rows in the configured database.
#
# Required environment:
#   GODDI_ADMIN_PASS   admin password for the isolated server
# Optional:
#   GODDI_BASE_URL     default http://127.0.0.1:18080
#   GODDI_ADMIN_USER   default admin
#   GODDI_CHROMIUM     explicit Chromium executable (auto-detected otherwise)
#   GODDI_SHOT_DIR     screenshot directory
#   GODDI_TEST_DB      isolated DB for reset_testdata.py
#   GODDI_SKIP_RESET=1 skip the pre-run test-data reset
#   GODDI_SUITES       space-separated subset of suite names
set -u
cd "$(dirname "$0")"

if [ -z "${GODDI_ADMIN_PASS:-}" ]; then
  echo "GODDI_ADMIN_PASS is required." >&2
  exit 2
fi

if [ "${GODDI_SKIP_RESET:-0}" != "1" ]; then
  if [ -n "${GODDI_TEST_DB:-}" ]; then
    python3 reset_testdata.py "$GODDI_TEST_DB" || exit 1
  else
    python3 reset_testdata.py || exit 1
  fi
fi

suites="${GODDI_SUITES:-docs contracts i18n theme h2-report-ref keyboard route-audit batch16 smoke batch2 batch3 batch4 batch5 batch6 batch7 batch8 batch9 batch10 batch11 batch12 batch13 batch14 batch15 legacy}"

total=0
failed=0
for s in $suites; do
  out=$(node "$s.cjs" 2>&1)
  code=$?
  n=$(printf '%s\n' "$out" | grep -c 'FAIL')
  total=$((total + n + (code != 0)))
  if [ "$n" -gt 0 ] || [ "$code" -ne 0 ]; then
    failed=1
    printf '%s\n' "$out"
  fi
  printf '%-9s FAILs=%s\n' "$s" "$n"
done

echo "TOTAL_FAILS=$total"
exit "$failed"