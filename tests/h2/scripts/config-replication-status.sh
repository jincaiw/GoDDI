#!/usr/bin/env bash
# Compact status helper for H2-015b: report "covered" only when the latest
# config-replication suite shows both DNS propagation checks passing.
set -uo pipefail
cd "$(dirname "$0")/.."
f=$(ls -t reports/config-replication-summary-*.json 2>/dev/null | head -1)
[ -z "${f:-}" ] && exit 0
python3 - "$f" <<'PY'
import json,sys
d=json.load(open(sys.argv[1]))
print("covered" if d.get("pass",0)>=14 and d.get("fail",0)==0 and d.get("not_verified",0)==0 else "not-covered")
PY
