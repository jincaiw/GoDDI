#!/usr/bin/env bash
# Aggregate every H2 acceptance summary into one machine-readable report.
# Reads reports/*-summary-*.json (written by the suites) and reports/*.jsonl
# (fault scenarios), then prints and writes reports/H2-REPORT-<ts>.json plus a
# human-readable markdown summary. Missing suites are reported as NOT_RUN, never
# as PASS.
set -uo pipefail
cd "$(dirname "$0")/.."
TS=$(date -u +%Y%m%dT%H%M%SZ); OUT=reports; mkdir -p "$OUT"
python3 - "$OUT" "$TS" <<'PY'
import glob, json, os, re, sys, collections
out, ts = sys.argv[1], sys.argv[2]
suites = {}
for path in sorted(glob.glob(os.path.join(out, "*-summary-*.json"))):
    try:
        d = json.load(open(path))
    except Exception:
        continue
    name = d.get("suite") or os.path.basename(path).split("-summary-")[0]
    cur = suites.get(name)
    # keep the newest summary per suite
    if cur is None or d.get("timestamp", "") >= cur.get("timestamp", ""):
        suites[name] = d
generic_faults = {}
specialist_faults = {}
# The fault driver writes placeholders for scenarios whose proof lives in a
# specialised suite. Read both families explicitly: generic placeholders are
# allowed to describe an unrun scenario, but a specialist verdict for the same
# id is the executed evidence and must win regardless of filename order.
for path in sorted(glob.glob(os.path.join(out, "faults-*.jsonl"))):
    for line in open(path):
        try:
            r = json.loads(line.strip())
        except Exception:
            continue
        generic_faults[r.get("id", "?")] = r.get("verdict")
for path in sorted(glob.glob(os.path.join(out, "app-faults-*.jsonl")) + glob.glob(os.path.join(out, "kernel-faults-*.jsonl"))):
    for line in open(path):
        try:
            r = json.loads(line.strip())
        except Exception:
            continue
        specialist_faults[r.get("id", "?")] = r.get("verdict")
faults = dict(generic_faults)
faults.update(specialist_faults)
# Kernel/app suites also use suffixed child ids such as H2-004a. Promote the
# parent only when all recorded children PASS; a failed child keeps the parent
# FAIL and a mixed set stays NOT_VERIFIED.
children = collections.defaultdict(list)
for fid, verdict in specialist_faults.items():
    m = re.match(r"^(H2-[0-9]+)[a-z]+$", fid)
    if m:
        children[m.group(1)].append(verdict)
for parent, verdicts in children.items():
    if any(v == "FAIL" for v in verdicts):
        faults[parent] = "FAIL"
    elif verdicts and all(v == "PASS" for v in verdicts):
        faults[parent] = "PASS"
expected = ["dns", "dns-transfer", "dhcp", "ipam", "ipam-lifecycle-replication", "split-plane", "faults", "app-faults", "kernel-faults", "config-author", "dns-primary-secondary", "auto-failover", "config-replication", "web"]
missing = [s for s in expected if s not in suites]
tot = collections.Counter()
for name, d in suites.items():
    for k in ("pass", "fail", "not_verified"):
        tot[k] += int(d.get(k, 0))
verdict = "PASS" if tot["fail"] == 0 and not missing else ("FAIL" if tot["fail"] else "NOT_VERIFIED")
# Suite-level NOT_VERIFIED entries are preserved as evidence boundaries, not
# hidden by the aggregate PASS. Emit the reason alongside the counts so a CI
# consumer can distinguish an unimplemented product capability from a failed
# check.
known_boundaries = {
    "config-author": "ADR-0005 full coordinator (leader election, quorum/witness, epoch fencing) is not implemented; ADR-0011 single-author rule is verified",
    "faults": "H2-020 licence-expiry protection is not implemented; legal, invalid and tampered licence samples are required",
}
suite_boundaries = {
    name: note for name, note in known_boundaries.items()
    if name in suites and int(suites[name].get("not_verified", 0)) > 0
}
report = {
    "level": "Level 1 — Docker H2 software consistency acceptance",
    "timestamp": ts,
    "verdict": verdict,
    "totals": dict(tot),
    "suites": suites,
    "suites_missing": missing,
    "fault_scenarios": faults,
    "suite_level_not_verified_boundaries": suite_boundaries,
    "level2_physical_not_verified": [
        "real whole-machine power-loss durability",
        "SSD/eMMC and disk-cache reliability",
        "two independent hosts hardware fault isolation",
        "real NIC/switch/BMC-KVM isolation behaviour",
        "performance, power and long-run stability on target hardware",
    ],
    "level2_note": "Docker H2 cannot prove any Level 2 item; they stay NOT_VERIFIED until physical validation is signed off.",
}
path = os.path.join(out, f"H2-REPORT-{ts}.json")
json.dump(report, open(path, "w"), indent=2, sort_keys=True)
print(f"== H2 acceptance report {ts} ==")
print(f"verdict: {verdict}")
for name in sorted(suites):
    d = suites[name]
    print(f"  {name:14s} PASS={d.get('pass',0):3d} FAIL={d.get('fail',0):2d} NOT_VERIFIED={d.get('not_verified',0):2d}")
if missing:
    print(f"  NOT_RUN: {', '.join(missing)}")
print(f"  totals: PASS={tot['pass']} FAIL={tot['fail']} NOT_VERIFIED={tot['not_verified']}")
fv = collections.Counter(faults.values())
if faults:
    print(f"  fault scenarios: {dict(fv)}")
if suite_boundaries:
    print(f"  suite boundaries: {list(sorted(suite_boundaries))}")
print(f"  Level 2 (physical): NOT_VERIFIED — {len(report['level2_physical_not_verified'])} items")
print(f"report written: {path}")
PY