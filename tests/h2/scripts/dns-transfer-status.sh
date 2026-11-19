#!/usr/bin/env bash
# Compact status helper for the DNS suite: report "covered" only when the
# latest dedicated dns-transfer suite is all PASS.
set -uo pipefail
cd "$(dirname "$0")/.."
f=$(ls -t reports/dns-transfer-summary-*.json 2>/dev/null | head -1)
[ -z "${f:-}" ] && exit 0
python3 - "$f" <<'PY'
import json,sys
d=json.load(open(sys.argv[1]))
print("covered" if d.get("pass",0)>=8 and d.get("fail",0)==0 and d.get("not_verified",0)==0 else "not-covered")
PY
