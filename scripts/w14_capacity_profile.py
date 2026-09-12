#!/usr/bin/env python3
"""Capacity and steady-state profile at the sizing target ADR 0001 fixes.

The claim under test is "≤ 20,000 leases per node, dual node, 4C/8 GiB, lease
store in WAL with synchronous=FULL". The acceptance criterion is blunt about
what may be published before this runs: *no capacity numbers*. So this script
does three things and reports them separately, because they answer different
questions.

  A. How long an idle trip to a full store takes, and what it occupies. Two
     real processes are paired; the primary's store is loaded to exactly 20,000
     leases; the read-only path (`goddi ha status --json`) is timed at that size,
     and the bytes on disk are measured.

  B. How long a standby takes to become a full second copy of a primary that
     already holds 20,000 leases. Replication has no incremental catch-up by
     design -- every connection is a snapshot -- so this is the number that
     decides how long a real failover drill's recovery step takes, and whether
     the 32 MiB frame ceiling in the HA contract is adequate.

  C. What the pair does under sustained writes for a couple of minutes with
     synchronous=FULL commits: the achieved rate, whether either process grows
     without bound, and whether the second writer on the same file produces
     SQLITE_BUSY. A second process writing the store is not the product's write
     path, but it is exactly the contention an operator creates with a script or
     a maintenance job, so the effect is worth seeing rather than assuming.

The bulk load goes through the system `sqlite3`, not the product: 20,000
synchronous commits would spend the whole run on setup, and what is being
measured in A and B is the read and the snapshot, not the insert. Section C says
out loud what that costs in fidelity.

Run from the repository root:

    python3 scripts/w14_capacity_profile.py

Exit code 0 means every measurement completed and every bound held.
"""

import json
import os
import re
import shutil
import socket
import sqlite3
import subprocess
import sys
import tempfile
import time

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BIN_NAME = "goddi-w14-capacity"

# ADR 0001, quoted so the report carries the claim it is testing rather than a
# number someone remembered.
TARGET_LEASES = 20000
TARGET_SHAPE = "双节点 4C/8GiB，租约库 WAL + synchronous=FULL"

TOKEN = "w14-capacity-token-not-a-real-credential"

CONFIRM_TIMEOUT = "2s"
HEARTBEAT = "500ms"
PEER_STALE_AFTER = "3s"

SCOPE_ID = "scope-cap"
SCOPE_SUBNET = "10.0.0.0/16"

STEADY_SECONDS = 120
SINGLE_COMMITS = 200
STEADY_BATCH = 500
STEADY_PERIOD = 5.0


def free_port():
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def config_yaml(data_dir, ha_role, listen_port, peer_port):
    # node_id has to differ between the two nodes: a primary refuses a peer that
    # identifies itself as the primary, which is the check that stops a node
    # from pairing with a second copy of itself.
    lines = [
        "server:",
        '  name: "GoDDI"',
        '  role: "dhcp"',
        f'  data_dir: "{data_dir}"',
        "database:",
        '  driver: "sqlite"',
        f'  dsn: "{data_dir}/goddi.db"',
        "dns:",
        "  enabled: false",
        "  listeners:",
        "    udp:",
        "      enabled: false",
        "    tcp:",
        "      enabled: false",
        "dhcp:",
        "  enabled: true",
        "  interfaces: []",
        "dhcp_ha:",
        "  enabled: true",
        f'  node_id: "w14-capacity-{ha_role}"',
        f'  role: "{ha_role}"',
        f'  listen_addr: "127.0.0.1:{listen_port}"',
        f'  peer_address: "127.0.0.1:{peer_port}"',
        f'  peer_token: "{TOKEN}"',
        f'  confirm_timeout: "{CONFIRM_TIMEOUT}"',
        f'  heartbeat_interval: "{HEARTBEAT}"',
        f'  peer_stale_after: "{PEER_STALE_AFTER}"',
        "security:",
        '  jwt_secret: "w14-capacity-secret-not-a-real-credential-0123456789"',
        "log:",
        '  level: "info"',
    ]
    return "\n".join(lines) + "\n"


class Node:
    def __init__(self, binary, name, cfg_path, tmp_root):
        self.name = name
        self.log_path = os.path.join(tmp_root, f"{name}.log")
        self._log = open(self.log_path, "w")
        self.proc = subprocess.Popen(
            [binary, "serve", "-c", cfg_path],
            stdout=self._log, stderr=subprocess.STDOUT, cwd=tmp_root, text=True,
        )

    def log(self):
        self._log.flush()
        try:
            with open(self.log_path, "r") as fh:
                return fh.read()
        except OSError:
            return ""

    def rss_kib(self):
        """Resident set size in KiB, or None once the process is gone."""
        if self.proc.poll() is not None:
            return None
        done = subprocess.run(["ps", "-o", "rss=", "-p", str(self.proc.pid)],
                              capture_output=True, text=True)
        try:
            return int(done.stdout.strip())
        except ValueError:
            return None

    def stop(self):
        if self.proc.poll() is None:
            self.proc.terminate()
        try:
            self.proc.wait(timeout=20)
        except subprocess.TimeoutExpired:
            self.proc.kill()
            self.proc.wait()
        self._log.close()


def wait_for(node, marker, timeout=60.0):
    deadline = time.time() + timeout
    while time.time() < deadline:
        out = node.log()
        if marker in out:
            return True, out
        if node.proc.poll() is not None:
            return False, out
        time.sleep(0.1)
    return False, node.log()


def ha(binary, args, cwd):
    done = subprocess.run([binary, "ha"] + args, capture_output=True, text=True,
                          cwd=cwd, timeout=120)
    return done.returncode, done.stdout + done.stderr


def ha_status(binary, cfg_path, cwd):
    code, out = ha(binary, ["status", "-c", cfg_path, "--json"], cwd)
    if code != 0:
        raise AssertionError(f"goddi ha status exited {code}: {out}")
    return json.loads(out)


def sqlite(db, script):
    """Run SQL with the durability settings the store itself runs with."""
    done = subprocess.run(
        ["sqlite3", db],
        input="PRAGMA busy_timeout = 5000;\nPRAGMA journal_mode = WAL;\nPRAGMA synchronous = FULL;\n"
              + script,
        capture_output=True, text=True, timeout=600,
    )
    if done.returncode != 0:
        raise AssertionError(f"sqlite3 failed: {done.stderr.strip()}")
    return done.stdout


def store_bytes(data_dir):
    total = 0
    for name in ("leases.db", "leases.db-wal", "leases.db-shm"):
        path = os.path.join(data_dir, name)
        if os.path.exists(path):
            total += os.path.getsize(path)
    return total


def read_only_count(db_path):
    """Count the leases from outside the process, read-only.

    Not a query through the product: the point is that the number is produced by
    a different reader than the one under test.
    """
    conn = sqlite3.connect(f"file:{db_path}?mode=ro", uri=True, timeout=30)
    try:
        return conn.execute("SELECT COUNT(*) FROM dhcp_leases").fetchone()[0]
    finally:
        conn.close()


def host_portrait():
    cpu = os.cpu_count()
    mem = subprocess.run(["sysctl", "-n", "hw.memsize"], capture_output=True, text=True)
    try:
        gib = int(mem.stdout.strip()) / (1 << 30)
    except ValueError:
        gib = float("nan")
    return cpu, gib


def section_a(binary, tmp_root, primary_dir, primary_cfg):
    """Load the ceiling and time the read path against it."""
    numbers = {}
    print("载入 %d 条租约（系统 sqlite3，事务批量）…" % TARGET_LEASES)

    db = os.path.join(primary_dir, "leases.db")

    # The scope goes into both databases, and that is not a detail of the
    # harness. A scope is control-plane configuration the data plane copies
    # down; a lease whose scope exists only in the data plane violates the
    # replica's foreign key when it is pushed up, and the push then fails every
    # second for as long as the row exists. Building the fixture any other way
    # would be measuring a system nobody deploys -- see the finding recorded for
    # W14-e about what a single such row does to the upstream queue.
    scope_row = (f"INSERT OR IGNORE INTO dhcp_scopes (id, name, subnet, start_ip, end_ip, enabled) "
                 f"VALUES ('{SCOPE_ID}', 'capacity', '{SCOPE_SUBNET}', '10.0.0.1', '10.255.255.254', 1);")
    sqlite(os.path.join(primary_dir, "goddi.db"), scope_row)
    sqlite(db, f"""
{scope_row}
BEGIN;
INSERT OR REPLACE INTO dhcp_leases (id, scope_id, ip_address, mac_address, hostname,
    client_id, lease_start, lease_end, status, last_seen, generation)
WITH RECURSIVE seq(i) AS (SELECT 0 UNION ALL SELECT i + 1 FROM seq WHERE i < {TARGET_LEASES - 1})
SELECT 'cap-' || i, '{SCOPE_ID}', '10.' || (i / 256) || '.' || (i % 256),
       printf('02:00:00:%02x:%02x:%02x', (i / 65536) % 256, (i / 256) % 256, i % 256),
       'cap', '', '2026-01-01T00:00:00Z', '2030-01-01T00:00:00Z', 'active',
       '2026-01-01T00:00:00Z', 1
FROM seq;
COMMIT;
""")

    rows = read_only_count(db)
    numbers["rows"] = rows
    if rows != TARGET_LEASES:
        raise AssertionError(f"the store holds {rows} leases, want {TARGET_LEASES}")
    print("  行数（独立只读读取）：%d" % rows)

    numbers["bytes"] = store_bytes(primary_dir)
    print("  库尺寸（db + wal + shm）：%.1f MiB（%.0f 字节/租约）"
          % (numbers["bytes"] / (1 << 20), numbers["bytes"] / rows))

    # The read-only report at the ceiling: this is the path the console, the
    # dependency checks and every operator command go through.
    samples = []
    for _ in range(3):
        started = time.time()
        status = ha_status(binary, primary_cfg, tmp_root)
        samples.append(time.time() - started)
    numbers["status_ms"] = min(samples) * 1000
    print("  goddi ha status 在 %d 条租约上：%.0f ms（三次取最小）" % (rows, numbers["status_ms"]))
    if status.get("seq") is None:
        raise AssertionError("goddi ha status did not report the HA watermarks")

    # What one confirmation costs, which is the shape an ACK has: one row, one
    # commit, synchronous=FULL. Reported separately from C's batched number
    # because they are not the same measurement and averaging them would hide
    # the one an operator feels.
    per_commit = []
    for i in range(TARGET_LEASES, TARGET_LEASES + SINGLE_COMMITS):
        started = time.time()
        conn = sqlite3.connect(db, timeout=30)
        try:
            conn.execute("PRAGMA synchronous = FULL")
            conn.execute("PRAGMA journal_mode = WAL")
            conn.execute(
                "INSERT OR REPLACE INTO dhcp_leases (id, scope_id, ip_address, mac_address,"
                " hostname, client_id, lease_start, lease_end, status, last_seen, generation)"
                " VALUES (?, ?, ?, ?, 'cap', '', '2026-01-01T00:00:00Z', '2030-01-01T00:00:00Z',"
                " 'active', '2026-01-01T00:00:00Z', 1)",
                (f"single-{i}", SCOPE_ID, "10.%d.%d.%d" % (i // 256, i % 256, 0),
                 "02:00:01:%02x:%02x:%02x" % ((i >> 16) & 0xFF, (i >> 8) & 0xFF, i & 0xFF)))
            conn.commit()
        finally:
            conn.close()
        per_commit.append(time.time() - started)

    per_commit.sort()
    numbers["commit_median_ms"] = per_commit[len(per_commit) // 2] * 1000
    numbers["commit_worst_ms"] = per_commit[-1] * 1000
    numbers["commits_per_second"] = 1000.0 / numbers["commit_median_ms"]
    print("  单行同步提交（一次提交＝一次确认的耐久成本）：中位 %.2f ms，最差 %.2f ms，"
          "即 %.0f 次/秒" % (numbers["commit_median_ms"], numbers["commit_worst_ms"],
                            numbers["commits_per_second"]))
    if numbers["commit_worst_ms"] > 1000:
        raise AssertionError(f"one synchronous commit took {numbers['commit_worst_ms']:.0f} ms; "
                             f"that is a client waiting a second for its address")

    # The store is measured after the writes, so the size above describes the
    # load rather than the load plus the log that carried it.
    numbers["bytes_after"] = store_bytes(primary_dir)
    print("  写入后的库尺寸：%.1f MiB" % (numbers["bytes_after"] / (1 << 20)))

    # Everything after this point is about the store as it now stands, not as it
    # stood at the target: the commit-cost measurement above wrote rows of its
    # own, and a later assertion against TARGET_LEASES would be asserting about
    # a store that no longer exists.
    numbers["rows_now"] = TARGET_LEASES + SINGLE_COMMITS
    if read_only_count(db) != numbers["rows_now"]:
        raise AssertionError("the store does not hold what the two measurements above wrote")
    return numbers


def section_b(binary, tmp_root, standby, standby_cfg, primary, expected_rows):
    """Time a standby becoming a full second copy of a 20,000-lease primary."""
    started = time.time()
    ok, out = wait_for(standby, "mirror rebuilt from a snapshot", timeout=300)
    elapsed = time.time() - started

    numbers = {"catchup_s": elapsed}
    if not ok:
        raise AssertionError(f"the standby never rebuilt from a snapshot: {out[-800:]!r}")

    # The primary's own account of what it sent: the contract's frame ceiling is
    # 32 MiB, and whether 20,000 leases fit is the question this answers.
    # Read the primary's log here rather than at the call site: the snapshot is
    # only sent once the standby connects, so a copy taken before that is a copy
    # of a log that does not contain the line being looked for.
    sent = re.findall(r'"msg":"HA: sent a snapshot to the mirror"[^}]*"rows":(\d+)', primary.log())
    if not sent:
        raise AssertionError("the primary never logged sending a snapshot")
    numbers["snapshot_rows"] = int(sent[-1])
    if numbers["snapshot_rows"] != expected_rows:
        raise AssertionError(f"the snapshot carried {numbers['snapshot_rows']} rows, "
                             f"want {expected_rows}")

    # And the independent check: what the standby's own file holds.
    standby_db = os.path.join(os.path.dirname(standby_cfg), "data", "leases.db")
    rows = None
    deadline = time.time() + 60
    while time.time() < deadline:
        try:
            rows = read_only_count(standby_db)
        except sqlite3.Error:
            rows = None
        if rows == expected_rows:
            break
        time.sleep(0.5)
    numbers["standby_rows"] = rows
    if rows != expected_rows:
        raise AssertionError(f"the standby's store holds {rows} leases, want {expected_rows}")

    print("  备机从空到追平 %d 条：%.1f s（快照 %d 行；独立只读读取备机库 %d 行）"
          % (expected_rows, elapsed, numbers["snapshot_rows"], rows))
    return numbers


def section_c(binary, tmp_root, primary, standby, primary_dir, standby_dir, base_rows):
    """Sustained writes for a couple of minutes, with the pair up."""
    numbers = {"batches": 0, "writes": 0, "elapsed": 0.0, "rss": []}
    db = os.path.join(primary_dir, "leases.db")
    next_index = base_rows
    deadline = time.time() + STEADY_SECONDS
    last_sample = 0.0

    while time.time() < deadline:
        time.sleep(STEADY_PERIOD)
        first = next_index
        last = first + STEADY_BATCH - 1
        started = time.time()
        sqlite(db, f"""
BEGIN;
INSERT OR REPLACE INTO dhcp_leases (id, scope_id, ip_address, mac_address, hostname,
    client_id, lease_start, lease_end, status, last_seen, generation)
WITH RECURSIVE seq(i) AS (SELECT {first} UNION ALL SELECT i + 1 FROM seq WHERE i < {last})
SELECT 'cap-' || i, '{SCOPE_ID}', '10.' || (i / 256) || '.' || (i % 256),
       printf('02:00:00:%02x:%02x:%02x', (i / 65536) % 256, (i / 256) % 256, i % 256),
       'cap', '', '2026-01-01T00:00:00Z', '2030-01-01T00:00:00Z', 'active',
       '2026-01-01T00:00:00Z', 1
FROM seq;
COMMIT;
""")
        took = time.time() - started
        next_index = last + 1
        numbers["batches"] += 1
        numbers["writes"] += STEADY_BATCH
        numbers["elapsed"] += took

        if time.time() - last_sample > 20:
            last_sample = time.time()
            numbers["rss"].append((primary.rss_kib(), standby.rss_kib()))

    numbers["rate"] = numbers["writes"] / numbers["elapsed"] if numbers["elapsed"] else 0
    print("  %d 批 × %d 行，共 %d 次同步提交，写入耗时合计 %.1f s（%.0f 行/秒）"
          % (numbers["batches"], STEADY_BATCH, numbers["writes"], numbers["elapsed"], numbers["rate"]))

    for label, node in (("主", primary), ("备", standby)):
        if node.rss_kib() is None:
            raise AssertionError(f"the {label} process is gone after the steady-state run")
    # The whole sample series, not first against last: the first sample lands
    # while the snapshot is still being applied, so it is the highest number of
    # the run and the series falls after it. Reporting only the endpoints would
    # describe that as memory having been released, which is not what happened.
    for label, index in (("主", 0), ("备", 1)):
        series = [sample[index] for sample in numbers["rss"] if sample[index]]
        if series:
            print("  常驻内存 %s（KiB）：%s" % (label, " → ".join(str(v) for v in series)))

    growth = 0
    for index in (0, 1):
        series = [sample[index] for sample in numbers["rss"] if sample[index]]
        if len(series) >= 2:
            growth = max(growth, max(series) - series[0])
    numbers["rss_growth_kib"] = growth
    if growth > 256 * 1024:
        raise AssertionError(f"a process grew by {growth} KiB over {STEADY_SECONDS}s of "
                             f"steady writes; that is a leak until shown otherwise")

    # The lease count is what the writes were for: if the store is not growing by
    # what was written, the rate above is measuring something else.
    rows = read_only_count(db)
    expected = base_rows + numbers["writes"]
    if rows != expected:
        raise AssertionError(f"after the steady-state run the store holds {rows} leases, "
                             f"want {expected}")
    print("  库内行数（独立只读读取）：%d（= 起始 %d + 写入 %d）" % (rows, base_rows, numbers["writes"]))

    # The row count the standby holds is a boundary, not a failure: the HA channel
    # carries what the lease manager hands it, and a second process writing the
    # file is not the manager. Recorded because an operator who writes the store
    # directly -- a script, a repair, a bulk import -- gets rows that the mirror
    # will never see, and nobody should discover that during a failover.
    standby_rows = read_only_count(os.path.join(standby_dir, "leases.db"))
    numbers["standby_rows_after"] = standby_rows
    print("  备机行数（独立只读读取）：%s；跨进程直接写入的 %d 行不在镜像里，"
          "这是刻意的边界而不是故障" % (standby_rows, numbers["writes"]))

    for label, node in (("主", primary), ("备", standby)):
        text = node.log()
        for needle in ("database is locked", "SQLITE_BUSY", "PANIC", "panic:"):
            if needle in text:
                raise AssertionError(f"the {label} process logged {needle!r} during the "
                                     f"steady-state run")
    return numbers


def caveats():
    cpu, gib = host_portrait()
    return """\
本画像不能当作什么（与上面的数字同样重要）

  * 硬件不同。ADR 0001 的目标画像是**双节点 4C/8GiB**，本机是 %d 核 / %.0f GiB；这些数字是
    「在此机器上的形状与量级」，不是 4C/8GiB 上的承诺。要在目标画像上发布容量数字，必须在
    目标画像上跑。
  * 不是 DHCP 客户端压测。本机非特权且 :67 被占用，DHCP 监听器绑不上，所以这里没有一条
    真实客户端的 DISCOVER/REQUEST 被处理过。上面的写入是直接对租约库做的同步提交，它测的是
    存储层在同步提交下的吞吐与争用，不是「每秒能答复多少客户端」。
  * 不是 24 小时长稳。C 节是 %d 秒，足以看到泄漏的形状和 SQLITE_BUSY，不足以声称「不会
    在三天后劣化」。内存判定是增长上界，不是无泄漏证明。
  * 单次运行。所有数字都取自一次运行，没有重复取分布；同一台机器上同时跑别的东西会改变它们。
  * 备机追平只有一个规模点。20,000 条是 ADR 的容量口径，但快照耗时对行数是否线性没有测过，
    所以「40,000 条大约两倍时间」是推断，不是测量。
  * 跨进程直接写库不进镜像（C 节已打印实际行数）。这条是运维注意事项，不是本轮要修的行为：
    租约的唯一合法写者是 `lease.Manager`。
""" % (cpu, gib, STEADY_SECONDS)


def main():
    binary = os.path.join(tempfile.gettempdir(), BIN_NAME)
    print("构建二进制…")
    build = subprocess.run(["go", "build", "-o", binary, "./cmd/goddi"], cwd=ROOT)
    if build.returncode != 0:
        print("FAIL: go build 失败")
        return 1

    cpu, gib = host_portrait()
    print(f"\n== 画像 ==\n  本机：{cpu} 核 / {gib:.0f} GiB 内存")
    print(f"  ADR 0001 的目标：{TARGET_SHAPE}，≤{TARGET_LEASES} 租约")

    tmp_root = tempfile.mkdtemp(prefix="goddi-w14-capacity-")
    failures = []
    try:
        primary_dir = os.path.join(tmp_root, "primary", "data")
        standby_dir = os.path.join(tmp_root, "standby", "data")
        os.makedirs(primary_dir, exist_ok=True)
        os.makedirs(standby_dir, exist_ok=True)
        primary_listen, standby_listen = free_port(), free_port()

        primary_cfg = os.path.join(tmp_root, "primary", "config.yaml")
        with open(primary_cfg, "w") as fh:
            fh.write(config_yaml(primary_dir, "primary", primary_listen, standby_listen))
        standby_cfg = os.path.join(tmp_root, "standby", "config.yaml")
        with open(standby_cfg, "w") as fh:
            fh.write(config_yaml(standby_dir, "standby", standby_listen, primary_listen))

        primary = Node(binary, "primary", primary_cfg, tmp_root)
        standby = None
        try:
            ok, out = wait_for(primary, "DHCP HA enabled")
            if not ok:
                raise AssertionError(f"the primary never reported its HA settings: {out[-600:]!r}")

            print("\n== A. 满库的读取与占用 ==")
            a = section_a(binary, tmp_root, primary_dir, primary_cfg)

            print("\n== B. 备机追平满库（全量快照，无增量追赶）==")
            standby = Node(binary, "standby", standby_cfg, tmp_root)
            b = section_b(binary, tmp_root, standby, standby_cfg, primary, a["rows_now"])

            print("\n== C. 分钟级稳态（同步提交，主备都在）==")
            c = section_c(binary, tmp_root, primary, standby, primary_dir, standby_dir,
                          a["rows_now"])
        except AssertionError as exc:
            failures.append(str(exc))
            print(f"  FAIL {exc}")
        finally:
            if standby is not None:
                standby.stop()
            primary.stop()
    finally:
        shutil.rmtree(tmp_root, ignore_errors=True)

    print("\n" + caveats())

    if failures:
        print(f"\n{len(failures)} 项问题")
        return 1
    print("A / B / C 三节全部完成，且每项上界都成立（并请一并阅读上面的「不能当作什么」）")
    return 0



if __name__ == "__main__":
    sys.exit(main())
