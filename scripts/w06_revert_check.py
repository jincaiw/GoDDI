#!/usr/bin/env python3
"""Back-out verification for the W06 IPAM-kernel work.

Each case disables exactly one mechanism, runs the test that is supposed to
depend on it, and requires that test to FAIL. A case that still passes means
the test does not actually pin the behaviour down -- the mechanism could be
deleted tomorrow and the suite would stay green.

Every replacement is asserted to have happened before the test runs. An earlier
round of this script silently skipped its replacements and produced five
fabricated "verified" results; the assertion below is what prevents a repeat.
A case that reports SKIPPED-NOT-FOUND is a failure, not a pass.

Each case names the package it belongs to, because W06 spans the IPAM address
kernel, the subnet kernel and the DHCP server's reporting path.
"""

import pathlib
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent

ADDR = ROOT / "internal/ipam/address"
SUBNET = ROOT / "internal/ipam/subnet"
DHCP = ROOT / "internal/dhcp/server"

PKG_ADDR = "./internal/ipam/address/"
PKG_SUBNET = "./internal/ipam/subnet/"
PKG_DHCP = "./internal/dhcp/server/"

CASES = [
    {
        "label": "R1 addresses are stored in whatever spelling the caller used",
        "file": ADDR / "status.go",
        "old": (
            "\tip := net.ParseIP(s)\n"
            "\tif ip == nil {\n"
            '\t\treturn "", fmt.Errorf("%w: %q", ErrInvalidIP, s)\n'
            "\t}\n"
            "\treturn ip.String(), nil"
        ),
        "new": (
            "\tif net.ParseIP(s) == nil {\n"
            '\t\treturn "", fmt.Errorf("%w: %q", ErrInvalidIP, s)\n'
            "\t}\n"
            "\treturn s, nil"
        ),
        "test": "TestNormalizeIP",
        "pkg": PKG_ADDR,
    },
    {
        "label": "R2 a reserved address handed out by DHCP is quietly reassigned",
        "file": ADDR / "status.go",
        "old": "\t\tcase StatusReserved, StatusExcluded, StatusGateway:",
        "new": "\t\tcase StatusExcluded, StatusGateway:",
        "test": "TestPlanLeaseObservation",
        "pkg": PKG_ADDR,
    },
    {
        "label": "R3 DECLINE is recorded as free instead of contested",
        "file": ROOT / "internal/ipam/linkage.go",
        "old": "\t\treturn address.ObservedContested, true",
        "new": "\t\treturn address.ObservedFree, true",
        "test": "TestIPAMLinkage_DeclineMarksTheAddressContested",
        "pkg": PKG_DHCP,
    },
    {
        "label": "R4 an observation no longer moves the allocation status",
        "file": ADDR / "address.go",
        "old": "\tif changed {\n\t\tres, err := tx.Exec(`UPDATE ipam_addresses SET status=?",
        "new": "\tif false && changed {\n\t\tres, err := tx.Exec(`UPDATE ipam_addresses SET status=?",
        "test": "TestIPAMLinkage_AckAllocatesTheAddress",
        "pkg": PKG_DHCP,
    },
    {
        "label": "R5 an observation no longer writes the audit row",
        "file": ADDR / "address.go",
        "old": (
            "\t\tif err := insertHistoryTx(tx, historyEntry{\n"
            "\t\t\tSpaceID: current.SpaceID, SubnetID: current.SubnetID, IPAddress: canonical,\n"
            '\t\t\tAction: "observe", OldStatus: string(current.Status), NewStatus: string(newStatus),\n'
            "\t\t\tActor: obs.Actor, Reason: reason, Source: defaultSource(obs.Source),\n"
            "\t\t}); err != nil {\n"
            "\t\t\treturn false, err\n"
            "\t\t}"
        ),
        "new": "\t\t_ = reason // history intentionally dropped for this check",
        "test": "TestObserveBySpaceIP",
        "pkg": PKG_ADDR,
    },
    {
        "label": "R6 the overlap check ignores which space a subnet belongs to",
        "file": SUBNET / "subnet.go",
        "old": 'rows, err := m.db.Query("SELECT id, cidr FROM ipam_subnets WHERE space_id = ?", spaceID)',
        "new": 'rows, err := m.db.Query("SELECT id, cidr FROM ipam_subnets WHERE ? <> \'\'", spaceID)',
        "test": "TestCreateSubnet_OverlapIsScopedToItsSpace",
        "pkg": PKG_SUBNET,
    },
    {
        "label": "R7 the materialisation size guard is dropped",
        "file": SUBNET / "subnet.go",
        "old": (
            "\thostBits := bits - ones\n"
            "\tif hostBits >= 63 {\n"
            "\t\treturn false\n"
            "\t}\n"
            "\treturn int64(1)<<uint(hostBits) <= autoCreateMaxAddresses"
        ),
        "new": (
            "\thostBits := bits - ones\n"
            "\t_ = hostBits\n"
            "\treturn int64(1)<<uint(hostBits) <= autoCreateMaxAddresses"
        ),
        "test": "TestMaterializationPolicy",
        "pkg": PKG_SUBNET,
    },
    {
        "label": "R8 the DHCP server stops reporting bindings to IPAM",
        "file": DHCP / "server.go",
        "old": "\tif s.leaseObserver == nil {\n\t\treturn\n\t}",
        "new": "\tif s.leaseObserver == nil || true {\n\t\treturn\n\t}",
        "test": "TestIPAMLinkage_ReleaseFreesTheObservationButKeepsTheAllocation",
        "pkg": PKG_DHCP,
    },
    {
        "label": "R9 the expiry sweep stops reporting the leases it swept",
        "file": DHCP / "server.go",
        "old": "\t\ts.observeLease(LeaseObservedExpire, l)\n\t\tif l.Status != lease.LeaseStatusActive",
        "new": "\t\t// observation intentionally dropped\n\t\tif l.Status != lease.LeaseStatusActive",
        "test": "TestIPAMLinkage_ExpirySweepFreesTheObservation",
        "pkg": PKG_DHCP,
    },
    {
        "label": "R10 every status transition is allowed again",
        "file": ADDR / "status.go",
        "old": "func CanTransition(from, to Status) bool {\n\tif from == to {\n\t\treturn false\n\t}",
        "new": (
            "func CanTransition(from, to Status) bool {\n"
            "\tif true {\n"
            "\t\treturn true\n"
            "\t}\n"
            "\tif from == to {\n"
            "\t\treturn false\n"
            "\t}"
        ),
        "test": "TestCanTransition",
        "pkg": PKG_ADDR,
    },
    {
        "label": "R11 a structural address can be released through the allocation path",
        "file": ADDR / "address.go",
        "old": "\tif current.Status.Structural() {\n\t\treturn nil, fmt.Errorf(",
        "new": "\tif false && current.Status.Structural() {\n\t\treturn nil, fmt.Errorf(",
        "test": "TestReleaseAddress_RefusesStructural",
        "pkg": PKG_ADDR,
    },
    {
        "label": "R12 the address-family guard is dropped from the overlap test",
        "file": SUBNET / "subnet.go",
        "old": "\tcase a4 != nil || b4 != nil:\n\t\t// Exactly one of them is IPv4.\n\t\treturn false\n\t}\n",
        "new": "\t}\n",
        "test": "TestCreateSubnet_IPv4AndIPv6DoNotOverlap",
        "pkg": PKG_SUBNET,
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
            ["go", "test", "-count=1", "-timeout", "180s", "-run", case["test"], case["pkg"]],
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

    # Confirm every touched package is green again after restoring the files.
    for pkg in (PKG_ADDR, PKG_SUBNET, PKG_DHCP):
        proc = subprocess.run(
            ["go", "test", "-count=1", "-timeout", "300s", pkg],
            cwd=ROOT, capture_output=True, text=True,
        )
        if proc.returncode != 0:
            print(f"RESTORE-FAILED: {pkg} is not green after restoring the files")
            print(proc.stdout[-3000:], proc.stderr[-3000:])
            return 2
    print("RESTORED: every touched package green after restoring every file", flush=True)

    if failures:
        print(f"\n{len(failures)} case(s) did not verify:", flush=True)
        for f in failures:
            print(" -", f)
        return 1
    print(f"\nAll {len(CASES)} back-out checks verified.", flush=True)
    return 0


if __name__ == "__main__":
    sys.exit(main())
