#!/usr/bin/env bash
# B8.5 evidence collector. Usage:
#   b85_collect_evidence.sh baseline              # versions/hashes/config
#   b85_collect_evidence.sh collect <scenario>    # metrics + db snapshot + optional pcap note
# Evidence is written under $B85_EVIDENCE_DIR (default ./b85-evidence/<scenario>).
# It does NOT start/stop any service; capture pcaps separately with tcpdump.
set -uo pipefail
MODE="${1:-baseline}"; SCEN="${2:-baseline}"
OUT="${B85_EVIDENCE_DIR:-./b85-evidence}/$SCEN"; mkdir -p "$OUT"
ts(){ date -u +%Y-%m-%dT%H:%M:%SZ; }
case "$MODE" in
  baseline)
    { echo "collected_at_utc: $(ts)"; echo "uname: $(uname -a)"; echo "go_version: $(go version 2>/dev/null || echo n/a)";
      echo "git_commit: $(git -C "$(dirname "$0")/.." rev-parse HEAD 2>/dev/null || echo n/a)";
      for b in goddi /usr/local/bin/goddi ./dist/goddi; do [ -x "$b" ] && { echo "binary: $b sha256=$(sha256sum "$b" 2>/dev/null | awk '{print $1}')"; }; done;
    } > "$OUT/baseline.txt"
    echo "baseline -> $OUT/baseline.txt" ;;
  collect)
    base="${GODDI_MGMT_URL:-http://127.0.0.1:6080}"
    { echo "collected_at_utc: $(ts)"; echo "scenario: $SCEN"; } > "$OUT/collect.txt"
    curl -fsS "$base/metrics" > "$OUT/metrics.prom" 2>/dev/null && echo "metrics saved" >> "$OUT/collect.txt" || echo "metrics: unreachable ($base/metrics)" >> "$OUT/collect.txt"
    curl -fsS "$base/ready" > "$OUT/ready.json" 2>/dev/null || true
    echo "NOTE: capture pcaps with e.g. 'tcpdump -ni <if> -w $OUT/capture.pcap udp port 67 or udp port 68 or udp port 53'" >> "$OUT/collect.txt"
    echo "collected -> $OUT" ;;
  *) echo "usage: $0 baseline | collect <scenario>" >&2; exit 2 ;;
esac
