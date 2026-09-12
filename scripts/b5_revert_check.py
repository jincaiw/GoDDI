#!/usr/bin/env python3
"""Back-out verification for B5 (W10 security, W11 observability, W12 upgrade/DR).

Each case disables exactly one B5 mechanism, runs the test that is supposed to
depend on it, and requires that test to FAIL. A case that still passes means the
test does not actually pin the behaviour down -- the mechanism could be deleted
tomorrow and the suite would stay green.

Every replacement is asserted to have happened before the test runs. A case that
reports SKIPPED-NOT-FOUND is a failure, not a pass: a script that silently skips
its own edits reports a green result over nothing, which is the exact failure
this file exists to prevent.

W12's own mechanisms are NOT repeated here. They are covered by
scripts/w12_mutation_check.py (six mutations), which reverts the snapshot, gate
and migration-status mechanisms against the smoke scripts. This file covers the
half of B5 that had no back-out coverage: W10 and W11.

Mechanisms this script deliberately does NOT claim to pin, because no test
distinguishes them and inventing a case would just be a green light that means
nothing:

  * "the audit trail is only pruned inside a maintenance window" in the sense of
    the window being *brief*. TestTheMaintenanceWindowIsNarrowAndLeavesItsOwnRecord
    pins that the window is visible and leaves a record; nothing pins duration,
    because a long DELETE is a property of the statement, not of the schema.
  * "the TSIG fingerprint is a safe display value". It is derived from the
    plaintext, so a case would have to argue about entropy rather than about a
    mechanism that can be removed.
  * "GODDI_ALLOW_DEFAULT_SECRET is the right escape hatch". R1 below removes the
    encryption-key half of the production rule; the default-secret half is a
    separate setting with its own test, and reverting both at once would not
    show which one the suite depends on.
  * "the alert thresholds are the right numbers". Nothing can: they are
    operator policy, and docs/prometheus-alerts.yml says so. C10 exercises the
    referability of a name, not its value.
"""

import pathlib
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent

CONFIG = ROOT / "internal/config/config.go"
SECRETBOX = ROOT / "internal/secretbox/secretbox.go"
TSIG_STORE = ROOT / "internal/dns/transfer/tsig_store.go"
AUDIT_MIG = ROOT / "migrations/024_audit_before_after_and_append_only.sql"
LEASE_AUDIT = ROOT / "internal/dhcp/lease/audit.go"
ADMIN_ALLOW = ROOT / "internal/api/middleware/adminallow.go"
RELAY_ALLOW = ROOT / "internal/dhcp/server/server.go"
DNS_HANDLER = ROOT / "internal/dns/server/handler.go"
METRICS = ROOT / "internal/metrics/metrics.go"
ALERT_RULES = ROOT / "docs/prometheus-alerts.yml"

PKG_CONFIG = "./internal/config/"
PKG_SECRETBOX = "./internal/secretbox/"
PKG_TRANSFER = "./internal/dns/transfer/"
PKG_DATABASE = "./internal/database/"
PKG_LEASE = "./internal/dhcp/lease/"
PKG_MIDDLEWARE = "./internal/api/middleware/"
PKG_DHCP_SERVER = "./internal/dhcp/server/"
PKG_METRICS = "./internal/metrics/"

CASES = [
    # -- W10-a: credential and key material ---------------------------------
    # R1 makes production accept the fallback it must refuse. Without a
    # dedicated key every sealed value is derived from the JWT secret, so
    # rotating that secret destroys the TOTP seeds and TSIG keys instead of
    # just invalidating sessions.
    {
        "label": "B1 production accepts a deployment with no dedicated encryption key",
        "file": CONFIG,
        "old": "\tif cfg.Security.EncryptionKey == \"\" {\n\t\tif cfg.IsProduction() {",
        "new": "\tif cfg.Security.EncryptionKey == \"\" {\n\t\tif false && cfg.IsProduction() {",
        "test": "TestProductionRefusesToRunWithoutADedicatedEncryptionKey",
        "pkg": PKG_CONFIG,
    },
    # R2 drops the purpose label from the derivation. Two purposes configured
    # with the same material then share one keystream, and a ciphertext can be
    # moved between columns unnoticed -- which is the whole reason the label is
    # an argument to Derive rather than a comment at the call site.
    {
        "label": "B2 the purpose label stops participating in the key derivation",
        "file": SECRETBOX,
        "old": 'sum := sha256.Sum256([]byte(label + ":" + keyMaterial))',
        "new": "sum := sha256.Sum256([]byte(keyMaterial))",
        "test": "TestAValueSealedForOneLabelDoesNotOpenUnderAnother",
        "pkg": PKG_SECRETBOX,
    },
    # R3 downgrades a missing key to plaintext for TSIG, which is the shape the
    # comment says must not exist: a TSIG key is only ever written by this
    # build, so there is no legacy that would excuse storing it in the clear.
    {
        "label": "B3 a TSIG secret is stored in the clear when no key material is configured",
        "file": TSIG_STORE,
        "old": (
            "\tif !sealer.Enabled() {\n"
            "\t\treturn \"\", fmt.Errorf(\"refusing to store a TSIG secret in the clear: "
            "set security.encryption_key (or GODDI_SECURITY_ENCRYPTION_KEY)\")\n"
            "\t}"
        ),
        "new": "\tif !sealer.Enabled() {\n\t\treturn secret, nil\n\t}",
        "test": "TestCreatingATSIGKeyRefusesWhenNoKeyMaterialIsConfigured",
        "pkg": PKG_TRANSFER,
    },
    # -- W10-b: the audit trail ----------------------------------------------
    # R4 makes the append-only trigger a no-op. The property then lives only in
    # the current code rather than in the database, which is the difference the
    # trigger exists to draw.
    {
        "label": "B4 the audit trail can be edited again",
        "file": AUDIT_MIG,
        "old": (
            "    SELECT RAISE(ABORT, 'audit_logs is append-only: "
            "an entry that can be edited is not a record of anything');"
        ),
        "new": "    SELECT 1;",
        "test": "TestMigration024MakesTheAuditTrailUneditable",
        "pkg": PKG_DATABASE,
    },
    # R5 records only the after side. An entry that names the transition but
    # not what it moved between is the exact report the two columns were added
    # to make answerable.
    {
        "label": "B5 a lease audit entry records no before value",
        "file": LEASE_AUDIT,
        "old": "\t\tOldValue:     renderLeaseState(before),\n\t\tNewValue:     renderLeaseState(after),",
        "new": '\t\tOldValue:     "",\n\t\tNewValue:     "",',
        "test": "TestReleaseAndDeclineAreRecordedWithBothSides",
        "pkg": PKG_LEASE,
    },
    # -- W10-c: network-layer hardening --------------------------------------
    # R6 makes the management allowlist ignore an entry that does not parse
    # instead of treating the list as unusable. The list is then judged on the
    # entries that happen to parse -- silently wider than the operator wrote,
    # which is the failure mode the code refuses to have.
    #
    # This is written as "do not set the flag" rather than "do not consult it":
    # dropping `broken` from the condition leaves it assigned and never read,
    # which is a compile error, and a case that breaks the build verifies
    # nothing about the behaviour.
    {
        "label": "B6 an unreadable allowlist entry widens the allowlist instead of refusing",
        "file": ADMIN_ALLOW,
        "old": "\t\t\tbroken = true\n\t\t\tcontinue",
        "new": "\t\t\tbroken = false\n\t\t\tcontinue",
        "test": "TestAnUnparsableEntryRefusesEverything",
        "pkg": PKG_MIDDLEWARE,
    },
    # R7 does the same for the relay list. Relayed requests then start being
    # served even though the operator's list could not be honoured -- and a
    # relay that should have been refused is the worse of the two silent
    # outcomes.
    {
        "label": "B7 an unreadable relay entry stops refusing relayed requests",
        "file": RELAY_ALLOW,
        "old": "\tif s.relayAllowListBroken {\n\t\treturn false\n\t}",
        "new": "\tif false && s.relayAllowListBroken {\n\t\treturn false\n\t}",
        "test": "TestAnUnreadableRelayEntryRefusesRelayedRequestsAndNothingElse",
        "pkg": PKG_DHCP_SERVER,
    },
    # -- W11: observability --------------------------------------------------
    # R8 removes the only writer of a registered metric. The series then
    # exports a steady zero, and a steady zero is indistinguishable from a
    # measurement of nothing happening -- which is how a dead metric survives a
    # dashboard and an alert written against it.
    {
        "label": "B8 a registered metric loses its only writer",
        "file": DNS_HANDLER,
        "old": "\t\t\tmetrics.DNSDroppedTotal.Inc()",
        "new": "\t\t\t// the drop counter is no longer recorded",
        "test": "TestEveryRegisteredMetricHasAWriter",
        "pkg": PKG_METRICS,
    },
    # R9 publishes the epoch for a backup type that has never succeeded. The
    # alert is written as "older than a day", so a zero matches it immediately:
    # "never backed up" and "catastrophically behind" become the same signal,
    # and they need different alerts.
    {
        "label": "B9 a backup type that never succeeded exports the epoch",
        "file": METRICS,
        "old": (
            "\tif fn := backupStatsFn; fn != nil {\n"
            "\t\tfor _, s := range fn() {\n"
            "\t\t\tBackupLastSuccessTimestampSeconds.WithLabelValues(s.Type)"
            ".Set(float64(s.LastSuccess.Unix()))\n"
            "\t\t}\n"
            "\t}"
        ),
        "new": (
            "\tif fn := backupStatsFn; fn != nil {\n"
            "\t\tfor _, s := range fn() {\n"
            "\t\t\tBackupLastSuccessTimestampSeconds.WithLabelValues(s.Type)"
            ".Set(float64(s.LastSuccess.Unix()))\n"
            "\t\t}\n"
            "\t\tBackupLastSuccessTimestampSeconds.WithLabelValues(\"full\").Set(0)\n"
            "\t}"
        ),
        "test": "TestSampleProvidersLeavesAnAgeUnknownUntilThereIsOne",
        "pkg": PKG_METRICS,
    },
    # R10 names a metric in an alert expression that no collector exports.
    # Prometheus loads the rule, the rule never fires, and a rule that never
    # fires cannot be told from a system with nothing wrong.
    {
        "label": "B10 an alert rule names a metric that is not registered",
        "file": ALERT_RULES,
        "old": "        expr: rate(goddi_dns_dropped_total[5m]) > 0",
        "new": "        expr: rate(goddi_dns_dropped_total_typo[5m]) > 0",
        "test": "TestEveryAlertedMetricExists",
        "pkg": PKG_METRICS,
    },
]


def run_case(case: dict) -> str:
    path = case["file"]
    pkg = case.get("pkg", PKG_METRICS)
    backup = path.read_text()

    # A tree an earlier run left mutated is refused before anything is edited.
    # Without this check, a run killed mid-case turns the NEXT run's results
    # into nonsense: the file no longer holds the original pattern, the case
    # reports SKIPPED-NOT-FOUND, and an unrelated package goes red for a reason
    # that has nothing to do with the case being run. That is not hypothetical:
    # it happened while this script was being written.
    if case["new"] in backup:
        return (f"DIRTY-TREE: {case['label']} (its mutation is already in {path.name}; "
                "a previous run did not restore it -- restore the file and re-run)")

    if case["old"] not in backup:
        return f"SKIPPED-NOT-FOUND: {case['label']} (the pattern is not in {path.name})"

    occurrences = backup.count(case["old"])
    if occurrences != 1:
        return (
            f"SKIPPED-NOT-UNIQUE: {case['label']} "
            f"(the pattern appears {occurrences} times in {path.name})"
        )

    path.write_text(backup.replace(case["old"], case["new"], 1))
    try:
        proc = subprocess.run(
            ["go", "test", "-count=1", "-timeout", "180s", "-run", case["test"], pkg],
            cwd=ROOT, capture_output=True, text=True,
        )
    finally:
        path.write_text(backup)

    output = proc.stdout + proc.stderr
    if "build failed" in output or "cannot use" in output or "undefined:" in output:
        return f"INVALID: {case['label']} broke the build instead of the behaviour"
    if "--- FAIL:" in output or "panic: test timed out" in output:
        return f"OK: {case['label']} -> {case['test']} failed as expected"
    return f"NOT-VERIFIED: {case['label']} -> {case['test']} still passed without the mechanism"


def main() -> int:
    failures = []
    for case in CASES:
        result = run_case(case)
        print(result, flush=True)
        if not result.startswith("OK:"):
            failures.append(result)

    # Confirm every package the cases touched is green again after restoring.
    packages = []
    for case in CASES:
        pkg = case.get("pkg", PKG_METRICS)
        if pkg not in packages:
            packages.append(pkg)
    for pkg in packages:
        proc = subprocess.run(
            ["go", "test", "-count=1", "-timeout", "300s", pkg],
            cwd=ROOT, capture_output=True, text=True,
        )
        if proc.returncode != 0:
            print(f"RESTORE-FAILED: {pkg} is not green after restoring the files")
            print(proc.stdout[-3000:], proc.stderr[-3000:])
            return 2
    print(f"RESTORED: {', '.join(packages)} green after restoring every file", flush=True)

    # And confirm the files themselves are back, not merely that the suites are
    # green. A suite can be green while a file still carries a mutation that
    # this particular batch of tests happens not to exercise.
    dirty = []
    for case in CASES:
        text = case["file"].read_text()
        if case["new"] in text or case["old"] not in text:
            dirty.append(f"{case['file'].name}: {case['label']}")
    if dirty:
        print("LEFTOVER-MUTATION: the tree is not back to its original state:")
        for item in dirty:
            print(" -", item)
        return 2
    print(f"TREE-CLEAN: all {len(CASES)} target files hold their original text again", flush=True)

    if failures:
        print(f"\n{len(failures)} case(s) did not verify:", flush=True)
        for f in failures:
            print(" -", f)
        return 1
    print(f"\nAll {len(CASES)} back-out checks verified.", flush=True)
    return 0


if __name__ == "__main__":
    sys.exit(main())
