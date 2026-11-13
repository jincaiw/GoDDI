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
import glob, json, os, sys, collections
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
faults = {}
for path in sorted(glob.glob(os.path.join(out, "faults-*.jsonl"))):
    for line in open(path):
        line = line.strip()
        if not line:
            continue
        try:
            r = json.loads(line)
        except Exception:
            continue
        faults[r.get("id", "?")] = r.get("verdict")
expected = ["dns", "dns-transfer", "dhcp", "ipam", "faults", "app-faults", "kernel-faults", "config-author", "web"]
missing = [s for s in expected if s not in suites]
tot = collections.Counter()
for name, d in suites.items():
    for k in ("pass", "fail", "not_verified"):
        tot[k] += int(d.get(k, 0))
verdict = "PASS" if tot["fail"] == 0 and not missing else ("FAIL" if tot["fail"] else "NOT_VERIFIED")
report = {
    "level": "Level 1 — Docker H2 software consistency acceptance",
    "timestamp": ts,
    "verdict": verdict,
    "totals": dict(tot),
    "suites": suites,
    "suites_missing": missing,
    "fault_scenarios": faults,
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
print(f"  Level 2 (physical): NOT_VERIFIED — {len(report['level2_physical_not_verified'])} items")
print(f"report written: {path}")
PY
