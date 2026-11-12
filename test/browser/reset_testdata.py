#!/usr/bin/env python3
"""Remove test-only rows created by the Playwright suites from an isolated DB.

This is a test-harness helper, never a production operation. Repeated
regression rounds otherwise accumulate rows past the first page of
RemoteTable, which makes "new row visible" assertions flake for reasons that
have nothing to do with the product. Individual suites also self-clean; this
is a belt-and-braces reset before a full-suite run.

Usage:
    python3 reset_testdata.py [DB_PATH]

Default DB_PATH comes from GODDI_TEST_DB, else /tmp/goddi-run/node1/goddi.db.
"""
import os
import sqlite3
import sys

DB = sys.argv[1] if len(sys.argv) > 1 else os.environ.get(
    "GODDI_TEST_DB", "/tmp/goddi-run/node1/goddi.db")

# Each entry is (table, column, LIKE prefix). Only test-prefixed rows go.
RULES = [
    ("dns_zones", "name", "tdzone%"),
    ("dns_zones", "name", "tddetail%"),
    ("dns_zones", "name", "tdsub%"),
    ("dns_zones", "name", "axfr-e2e%"),
    ("dns_zones", "name", "signed-e2e%"),
    ("dns_zones", "name", "nsec-e2e%"),
    ("dns_zones", "name", "nsec3-e2e%"),
    ("dns_zones", "name", "dnssec-%"),
    ("dns_zones", "name", "split-role%"),
    ("dns_zone_transfer", "id", "acl-e2e%"),
    ("dns_client_policies", "name", "tdsec-%"),
    ("groups", "name", "tdgroup-%"),
    ("dns_allow_rules", "pattern", "td-allow-%"),
    ("users", "username", "tduser%"),
    ("roles", "name", "tdrole%"),
    ("ipam_spaces", "name", "tdesign-%"),
    ("ipam_spaces", "name", "td-seed-space-%"),
    ("ipam_subnets", "name", "td-seed-subnet-%"),
    ("ipam_subnets", "name", "tdsub%"),
    ("dns_forwarders", "name", "tdfwd%"),
    ("api_tokens", "name", "tdtoken-%"),
    ("dhcp_reservations", "description", "td-%"),
]


def main():
    if not os.path.isfile(DB):
        print(f"reset: database not found: {DB}", file=sys.stderr)
        print("Set GODDI_TEST_DB (or pass a path) to the isolated DB under test.",
              file=sys.stderr)
        return 1
    try:
        c = sqlite3.connect(DB)
    except sqlite3.Error as exc:
        print(f"reset: cannot open {DB}: {exc}", file=sys.stderr)
        return 1
    total = 0
    for table, column, prefix in RULES:
        try:
            exists = c.execute(
                "SELECT 1 FROM sqlite_master WHERE type='table' AND name=?", (table,)).fetchone()
            if not exists:
                continue
            cols = [r[1] for r in c.execute(f"PRAGMA table_info({table})")]
            if column not in cols:
                continue
            total += c.execute(
                f"DELETE FROM {table} WHERE {column} LIKE ?", (prefix,)).rowcount
        except sqlite3.Error:
            continue
    c.commit()
    c.close()
    print(f"reset: removed {total} accumulated test rows from {DB}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
