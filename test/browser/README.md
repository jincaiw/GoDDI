# Browser regression harness

24 Playwright/static suites that exercise the TDesign management UI against an
**isolated** GoDDI server (real Chromium headless, real HTTP API, real SQLite).
This is the reproducible form of the browser evidence in
`docs/audit/IMPLEMENTATION-STATUS.md` §1. **Never point it at production** —
the suites create and delete test rows in the configured database.

> **CI**: the `.github/workflows/ci.yml` `browser-regression` job runs these
> suites on every push/PR (23 of the 24 — `h2-report-ref` needs a generated
> `tests/h2/reports/H2-REPORT-*.json` and is local-only) against a freshly
> built, isolated server. `make verify-browser` remains the local entry point.
>
> **Baseline data**: a fresh database has no zone/scope for a few suites to act
> on, so the CI job first runs `node test/browser/seed_ci.mjs` (with
> `GODDI_BASE_URL`/`GODDI_ADMIN_PASS`). It idempotently creates `demo.test`
> (with `www` A record), a `td-demo-scope` DHCP scope with free addresses, and a
> `td-demo-space` IPAM space. Run it the same way when testing a fresh server.
> The debug-client suite also needs DNS enabled on a high port (CI uses 15353).

## Prerequisites

- `node` and Playwright. Install the harness dependency once:
  `cd test/browser && npm install` (this declares `playwright` in
  `test/browser/package.json`; no other dependencies). Alternatively point
  `NODE_PATH` at an existing Playwright installation. If Playwright cannot be
  resolved, `common.cjs` exits with an actionable message instead of a bare
  `MODULE_NOT_FOUND`.
- A Chromium executable (auto-detected from the Playwright cache, or set
  `GODDI_CHROMIUM`).
- A running isolated server: build (`make build`) then start it from the repo
  root with an isolated config, e.g.
  `GODDI_ADMIN_USERNAME=admin GODDI_ADMIN_PASSWORD=... ./dist/goddi serve -c /path/to/isolated.yaml`.
  `web/dist` must be the current build.

## Run

```bash
export GODDI_ADMIN_PASS='...'          # required
export GODDI_BASE_URL='http://127.0.0.1:18080'
export GODDI_TEST_DB='/tmp/goddi-run/node1/goddi.db'
export GODDI_CHROMIUM='/path/to/chrome-headless-shell'   # optional; auto-detected
./test/browser/run.sh
```

Or via the Makefile: `make verify-browser` (uses the same env vars).

> **Build ordering**: `make verify` rebuilds `web-admin/dist` with content-hashed
> filenames but does not sync `web/dist` (the directory the server serves).
> Run `make build` (or `cp -r web-admin/dist/. web/dist/`) after `make verify`
> and before `make verify-browser`; the `docs` guard compares the two trees
> byte-for-byte and will fail on a stale deployed bundle.

Environment variables:

| Variable | Default | Purpose |
|---|---|---|
| `GODDI_BASE_URL` | `http://127.0.0.1:18080` | server under test |
| `GODDI_ADMIN_USER` | `admin` | login user |
| `GODDI_ADMIN_PASS` | (required) | login password |
| `GODDI_CHROMIUM` | auto-detect | explicit Chromium/Chrome executable |
| `GODDI_SHOT_DIR` | `$TMPDIR/goddi-browser-shots` | screenshot destination |
| `GODDI_TEST_DB` | `/tmp/goddi-run/node1/goddi.db` | isolated DB for the reset helper |
| `GODDI_SKIP_RESET` | `0` | set to 1 to skip the pre-run reset |
| `GODDI_SUITES` | all 24 | subset, space-separated |

## Suites

`smoke` (login + spaces), `batch2` (log pages), `batch3` (security/leases/settings),
`batch4` (options/reservations/backup/tokens), `batch5` (sessions/scopes/allowed/groups),
`batch6` (users/roles/cache), `batch7` (forwarders/blocked/tools client),
`batch8` (zones), `batch9` (IPAM dialogs), `batch10` (IPAM pages),
`batch11` (blank-layout 403/404/500), `batch12` (zone detail 5 tabs),
`batch13` (config versions), `batch14` (dashboard + Naive-free check), `batch15` (standalone DNSSEC key-lifecycle page), `batch16` (read-only role: create buttons absent, delete opens no confirm dialog). batch4 also runs option and backup create-e2e steps with self-cleanup, batch3 saves and restores a system setting through the UI (round 205-217 — these pages were render-only before).

## Playwright e2e suites (two instances, one command)

`make verify-e2e` (or `scripts/e2e_local.sh`) reproduces the two Playwright jobs CI runs, locally:

- **bare suite** — `e2e/tests/api.spec.ts` + `e2e/tests/config-versions.spec.ts` against a throwaway plain instance (port 18090, DNS on 15362) that the script builds and tears down;
- **seeded fixture suite** — `e2e/tests/ipam-console.fixture.spec.ts` against `scripts/w13_frontend_smoke.py`'s seeded instance (port 16092).

Requires `cd e2e && npm ci` once. The two stale pre-TDesign specs (`console.spec.ts`, `workflows.spec.ts`) are excluded by `e2e/playwright.config.ts` and carry disposition headers; see their file tops.

`legacy` (the 10 originally-unmigrated pages), `docs` (static guard: `docs/STATUS.md` links/headline counts, config.yaml keys
vs `internal/config` yaml tags, `docs/CONFIG-REFERENCE.md` coverage of optional
keys, all `GODDI_*` env overrides, every env-table config path being composed
of real yaml tags, `web/dist` matching `web-admin/dist` byte-for-byte, the
review dossier's headline counts, unique numbered headings / unchanged
"未验收" statuses in the B8.5 runbook, and all runbook §N cross-references
pointing at real sections; no browser needed),
`i18n` (static guard: DNSSEC hints stay current in both languages, and every literal
`t('...')` key resolves in both locales; no browser needed),
`theme` (static guard: devflow TDesign Blue default and production override, light
side menu, TDesign-token shell, TDesign status colors, global responsive caps and
tokenized spacing/radius/font rules, localized dashboard rcode labels, GoDDI
package identity, and raw-code coverage in `route-audit`; no browser needed),
`h2-report-ref` (static guard: G5 and both H2 acceptance docs cite the newest
`H2-REPORT-<ts>.json`; no browser needed),
`route-audit` (walks every static route plus a real zone detail page; fails on raw i18n keys, ISO timestamps, known raw enums, navigation errors, or page errors),
`contracts` (static guard: nav/ROUTE_META keys have translations in both languages,
navigable nav items have PATHS entries, ROUTE_META keys match generated routes,
ROUTE_META permissions exist in RBAC, frontend service API literals match a documented
OpenAPI path **and their request helper's HTTP method matches a documented operation on
that path**, every management view is exercised by a browser suite, and the
SINGLE-NODE-FIRST reservation holds (NodeModeBadge pinned to single, cluster.ts
issues no requests, no cluster nav group); no browser needed).

## Test-data governance

Every suite that creates a row **deletes it afterwards** and uses a
full-timestamp+random unique suffix. `reset_testdata.py` is a belt-and-braces
clean of test-prefixed rows before a full run; without it, repeated rounds
accumulate rows past page 1 of `RemoteTable` and produce pagination-only FAILs
that have nothing to do with the product. It only deletes rows with known test
prefixes and only from the DB path you pass.

`run.sh` prints one `FAILs=N` line per suite and `TOTAL_FAILS=N`, and exits
non-zero if any suite failed or returned a non-zero exit code.

`reset_testdata.py` refuses a missing/unopenable DB with a clear message and a
non-zero exit instead of a Python traceback; `run.sh` only passes a DB path
when `GODDI_TEST_DB` is set, so the helper's default still applies. Chromium
auto-detection was verified by running the full 19 browser suites with `GODDI_CHROMIUM`
unset (round 130, `TOTAL_FAILS=0`).