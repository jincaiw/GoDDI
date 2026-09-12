#!/usr/bin/env python3
"""Back-out verification for the W05 configuration-publishing work.

Each case disables exactly one mechanism, runs the test that is supposed to
depend on it, and requires that test to FAIL. A case that still passes means
the test does not actually pin the behaviour down.

Every replacement is asserted to have happened before the test runs. A previous
round of this script silently skipped its replacements and produced five
fabricated "verified" results; the assertion below is what prevents a repeat.
"""

import pathlib
import shutil
import subprocess
import sys

# The script lives in scripts/, but go test must run from the module root.
ROOT = pathlib.Path(__file__).resolve().parent.parent
PKG = "./internal/configver/"

SVCPATH = ROOT / "internal/configver/service.go"
OUTPATH = ROOT / "internal/configver/outbox.go"
DNSPATH = ROOT / "internal/configver/adapters_dns.go"

CASES = [
    {
        "label": "R1 concurrency guard removed",
        "file": SVCPATH,
        "old": "\tif current != req.ExpectedRevision {\n\t\treturn nil, &ConflictError{",
        "new": "\tif false && current != req.ExpectedRevision {\n\t\treturn nil, &ConflictError{",
        "test": "TestPublish_ConcurrentWritersOnlyOneWins",
    },
    {
        "label": "R2 idempotency replay removed",
        "file": SVCPATH,
        "old": "\tif req.IdempotencyKey != \"\" {\n\t\texisting, err := revisionByIdempotencyKey(tx, req.IdempotencyKey)",
        "new": "\tif false && req.IdempotencyKey != \"\" {\n\t\texisting, err := revisionByIdempotencyKey(tx, req.IdempotencyKey)",
        "test": "TestPublish_IdempotentReplayReturnsSameRevision",
    },
    {
        "label": "R3 release event no longer queued",
        "file": SVCPATH,
        "old": "\tif err := insertRelease(tx, rev); err != nil {\n\t\treturn nil, err\n\t}",
        "new": "\t_ = rev // release intentionally not queued for this check",
        "test": "TestDrainOutbox_AppliesAndMarksApplied",
    },
    {
        "label": "R4 dry run stored as a revision again",
        "file": SVCPATH,
        "old": "\tif req.DryRun {\n\t\treturn &PublishResult{Changes: changes, BaseRevision: current, DryRun: true}, nil\n\t}",
        "new": "\tif req.DryRun {\n\t\tprobe := &Revision{ID: uuid.New().String(), ResourceType: req.ResourceType, ResourceID: req.ResourceID, Revision: current + 1, BaseRevision: current, ContentHash: hash, Content: content, Status: StatusPersisted}\n\t\tif err := insertRevision(tx, probe); err != nil {\n\t\t\treturn nil, err\n\t\t}\n\t\tif err := tx.Commit(); err != nil {\n\t\t\treturn nil, err\n\t\t}\n\t\treturn &PublishResult{Revision: probe, Changes: changes, BaseRevision: current, DryRun: true}, nil\n\t}",
        "test": "TestPublish_DryRunStoresNothingAndConsumesNoRevision",
    },
    {
        "label": "R5 resource write outcome ignored on failure",
        "file": OUTPATH,
        "old": "\twritten := resourceApplied || rel.ResourceApplied",
        "new": "\twritten := true // pretend the write always landed",
        "test": "TestDrainOutbox_ApplyFailurePreservesResourceAndFailsRevision",
    },
    {
        "label": "R6 notify failure marks the revision failed anyway",
        "file": OUTPATH,
        "old": "\t\tif !written {\n\t\t\trev, err := scanRevision(tx.QueryRow(revisionSelect+` WHERE id = ?`, rel.RevisionID))",
        "new": "\t\t_ = written\n\t\tif true {\n\t\t\trev, err := scanRevision(tx.QueryRow(revisionSelect+` WHERE id = ?`, rel.RevisionID))",
        "test": "TestDrainOutbox_NotifyFailureKeepsRevisionStaged",
    },
    {
        "label": "R7 DNS release no longer reloads the zone store",
        "file": DNSPATH,
        "old": "\tif a.store == nil {\n\t\treturn nil\n\t}\n\ta.store.ReloadNow()\n\treturn nil",
        "new": "\treturn nil",
        "test": "TestDNSZoneAdapter_ReleaseForcesStoreReload",
    },
    {
        "label": "R8 rollback target usability check removed",
        "file": SVCPATH,
        "old": "\tif !target.Status.restorable() {",
        "new": "\tif false && !target.Status.restorable() {",
        "test": "TestDrainOutbox_ApplyFailurePreservesResourceAndFailsRevision",
    },
    {
        "label": "R9 zone name character check removed",
        "file": DNSPATH,
        "old": "\tif err := validZoneName(c.Name); err != nil {\n\t\treturn err\n\t}",
        "new": "\tif false {\n\t\treturn nil\n\t}",
        "test": "TestDNSZoneAdapter_Validate/invalid_domain",
    },
]


def run_case(case: dict) -> str:
    path = case["file"]
    backup = path.read_text()

    if case["old"] not in backup:
        return f"SKIPPED-NOT-FOUND: {case['label']} (the pattern is not in the file)"

    path.write_text(backup.replace(case["old"], case["new"], 1))
    try:
        proc = subprocess.run(
            ["go", "test", "-count=1", "-timeout", "120s", "-run", case["test"], PKG],
            cwd=ROOT, capture_output=True, text=True,
        )
    finally:
        path.write_text(backup)

    output = proc.stdout + proc.stderr
    if "build failed" in output or "cannot use" in output or "undefined:" in output:
        return f"INVALID: {case['label']} broke the build instead of the behaviour"

    if "--- FAIL:" in output:
        return f"OK: {case['label']} -> {case['test']} failed as expected"
    return f"NOT-VERIFIED: {case['label']} -> {case['test']} still passed without the mechanism"


def main() -> int:
    failures = []
    for case in CASES:
        result = run_case(case)
        print(result, flush=True)
        if not result.startswith("OK:"):
            failures.append(result)

    # Confirm the tree is back to a green state after all the edits.
    proc = subprocess.run(
        ["go", "test", "-count=1", "-timeout", "300s", PKG],
        cwd=ROOT, capture_output=True, text=True,
    )
    if proc.returncode != 0:
        print("RESTORE-FAILED: the package is not green after restoring the files")
        print(proc.stdout[-3000:], proc.stderr[-3000:])
        return 2
    print("RESTORED: full package green after restoring every file", flush=True)

    if failures:
        print(f"\n{len(failures)} case(s) did not verify:", flush=True)
        for f in failures:
            print(" -", f)
        return 1
    print(f"\nAll {len(CASES)} back-out checks verified.", flush=True)
    return 0


if __name__ == "__main__":
    sys.exit(main())
