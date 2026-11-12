#!/bin/bash
# Local entry point for the two Playwright jobs CI runs
# (.github/workflows/ci.yml "Browser and API regression tests" and
# "Browser regression tests (seeded fixtures)"), so the same suites can be
# driven with one command outside CI.
#
#   scripts/e2e_local.sh          # run both suites
#   scripts/e2e_local.sh bare     # only the plain-instance suite
#   scripts/e2e_local.sh fixture  # only the seeded-fixture suite
#
# Requirements: e2e/node_modules populated (`cd e2e && npm ci`), the repo
# binary (built here), and a Chromium for Playwright (auto-detected, or set
# GODDI_BROWSER_CHANNEL / Playwright's own cache).
set -u
REPO="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO/e2e" || exit 1

MODE="${1:-both}"

if [ ! -d node_modules/@playwright/test ]; then
  echo "e2e: node_modules missing — run: cd e2e && npm ci" >&2
  exit 2
fi

PLAIN_DIR="$(mktemp -d /tmp/goddi-e2e-plain.XXXXXX)"
PLAIN_PORT=18090
PLAIN_DNS=15362
PLAIN_URL="http://127.0.0.1:$PLAIN_PORT"

echo "== e2e: building the binary =="
( cd "$REPO" && go build -mod=readonly -o "$PLAIN_DIR/goddi" ./cmd/goddi ) || exit 1

status=0

# --- plain instance (the bare suite: api.spec + config-versions.spec) -------
if [ "$MODE" = both ] || [ "$MODE" = bare ]; then
  # The shipped config.yaml + GODDI_* env overrides (the same pattern CI uses).
  echo "== e2e: starting the plain instance on $PLAIN_URL =="
  GODDI_SERVER_HTTP_ADDR="127.0.0.1:$PLAIN_PORT" \
  GODDI_SERVER_PUBLIC_URL="$PLAIN_URL" \
  GODDI_SERVER_DATA_DIR="$PLAIN_DIR/data" \
  GODDI_DATABASE_DSN="$PLAIN_DIR/goddi.db" \
  GODDI_SECURITY_JWT_SECRET="e2e-local-secret-not-for-production-123" \
  GODDI_ADMIN_USERNAME=admin \
  GODDI_ADMIN_PASSWORD='Admin@123456' \
  GODDI_DNS_ENABLED=true \
  GODDI_DHCP_LEASE_STORE=sqlite \
  GODDI_DNS_LISTENERS_UDP_ADDR="127.0.0.1:$PLAIN_DNS" \
  GODDI_DNS_LISTENERS_TCP_ADDR="127.0.0.1:$PLAIN_DNS" \
    "$PLAIN_DIR/goddi" serve -c "$REPO/config.yaml" > "$PLAIN_DIR/serve.log" 2>&1 &
  plain_pid=$!
  for _ in $(seq 1 30); do
    curl -fsS "$PLAIN_URL/health" > /dev/null 2>&1 && break
    sleep 0.5
  done
  if ! curl -fsS "$PLAIN_URL/health" > /dev/null; then
    echo "e2e: plain instance failed to start" >&2
    cat "$PLAIN_DIR/serve.log" >&2
    kill "$plain_pid" 2>/dev/null
    exit 1
  fi

  echo "== e2e: bare suite =="
  GODDI_TEST_BASE_URL="$PLAIN_URL" \
  GODDI_TEST_USERNAME=admin \
  GODDI_TEST_PASSWORD='Admin@123456' \
  GODDI_TEST_DNS_PORT=$PLAIN_DNS \
    npx playwright test || status=1

  kill "$plain_pid" 2>/dev/null
  wait "$plain_pid" 2>/dev/null
fi

# --- seeded fixture instance (the fixture suite) ----------------------------
if [ "$MODE" = both ] || [ "$MODE" = fixture ]; then
  echo "== e2e: starting the seeded fixture instance (w13) on 16092 =="
  GODDI_SMOKE_PORT=16092 python3 "$REPO/scripts/w13_frontend_smoke.py" up || exit 1
  fixture_status=0
  GODDI_TEST_BASE_URL="http://127.0.0.1:16092" \
    npx playwright test --config playwright.fixture.config.ts || fixture_status=1
  # Do not swallow the bare suite's failure: keep the worse status.
  [ "$fixture_status" != 0 ] && status=1
  GODDI_SMOKE_PORT=16092 python3 "$REPO/scripts/w13_frontend_smoke.py" down || true
  [ "$status" != 0 ] && echo "e2e: fixture suite failed" >&2
  # status must survive: bare already folded into $status above, fixture into
  # this one — keep the worse of the two.
fi

# Keep the plain instance's dir when a suite failed (the serve.log is the
# most useful artifact), clean it when everything passed.
if [ "$status" = 0 ]; then
  rm -rf "$PLAIN_DIR"
else
  echo "e2e: kept $PLAIN_DIR for debugging (see serve.log)"
fi
echo "== e2e: done, exit=$status =="
exit "$status"
