#!/usr/bin/env python3
"""Disaster-recovery snapshot smoke check for W12-a.

Builds the real binary and drives `goddi backup` / `goddi restore` against an
instance the binary itself created. The point is to catch what a unit test
cannot see: which files main() actually decides this deployment owns, whether
the stores a running process opens are the ones the archiver reaches for, and
whether a restore puts them back where the running process will look for them.

The round trip is asserted with a marker row rather than with file sizes. A row
is written into the control database while the service is stopped, the archive
is taken, the row is deleted, and an in-place restore has to bring it back. That
one assertion covers three separate claims at once: the snapshot read through
the write-ahead log rather than the main file, the restore replaced the file the
deployment serves from, and the archive's state, not the host's, is what came
back.

What the run against a *stopped* service cannot show is the write-ahead log
invariant, because a cleanly closed connection deletes its own log. So the
restore is also run with a second process attached to the control database,
which is what a deployment looks like when --in-place was run without stopping
the service. The log is asserted to be there before the restore and gone after
it, and the safety copy is asserted to hold the row that only ever existed in
that log -- so the safety copy is shown to have read through it.

Deliberately not claimed here:

  * Nothing is proven about restoring onto a *different* host. The archive is
    read back on the machine that wrote it, so a path that only exists here
    would not be caught. That is the W12-c exercise.
  * The *consequence* of leaving a stale log behind is not observed, only the
    invariant that it is removed. A restored database comes back in rollback
    journal mode, so a leftover log sits inert until something switches the
    file back to WAL -- which the service does at startup, at which point the
    log would be recovered against a database it does not belong to. Reproducing
    that needs a second service start, and the invariant is asserted directly
    instead.
  * The schema-version and product-version gates are driven end to end. A
    plaintext archive is rewritten member by member -- only manifest.json is
    replaced -- so an archive whose manifest claims a schema or a version the
    binary does not know can be handed to the real command. Both directions are
    covered: an archive ahead of this build is refused, on a host that has
    never been migrated, and an archive behind this build is accepted. The
    refusal of the *older* archive was the behaviour that was wrong before
    W12-b, so it is asserted rather than assumed.
  * Stated plainly, the harness cannot test the gate against a *genuinely*
    newer build: it has one binary. What it can do is hand that binary an
    archive that lies about being newer, which exercises the same code path
    from the same input.
  * No concurrent load runs during the backup. The consistency property is
    covered by TestASnapshotIsOneConsistentInstant, which drives it
    deterministically with an open write transaction.
  * The DHCP and DNS listeners are not asserted to work; this harness runs
    unprivileged, so the DHCP listener cannot bind udp4 0.0.0.0:67. Only the
    existence of the data-plane store files is asserted, which is the part the
    snapshot depends on.

Run from the repository root:

    python3 scripts/w12_snapshot_smoke_check.py

Exit code 0 means every case behaved as advertised.
"""

import io
import json
import os
import re
import shutil
import socket
import subprocess
import sys
import tarfile
import tempfile
import time

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BIN_NAME = "goddi-w12-smoke"
JWT_SECRET = "w12-smoke-jwt-secret-not-a-real-credential-0123456789"
KEY_MATERIAL = "w12-smoke-encryption-key-material-0123456789"
MARKER_KEY = "w12-smoke-marker"
MARKER_VALUE = "written-before-the-archive-and-deleted-after-it"
NOISE_KEY = "w12-smoke-noise"
NOISE_VALUE = "written-after-the-archive-was-taken"
MANIFEST_MEMBER = "manifest.json"
SCHEMA_AHEAD = 9999
VERSION_AHEAD = "9.9.9"
SCHEMA_BEHIND = 1

# A second process that writes to the control database and then holds it open.
#
# It exists to make one invariant testable. A cleanly closed connection
# checkpoints its write-ahead log and deletes it, so an ordinary write leaves
# nothing for a restore to trip over -- which is why the log removal is
# invisible in every other scenario. While another connection is attached SQLite
# cannot remove the log, so this is what a deployment looks like when the
# operator ran --in-place without stopping the service, the case the flag exists
# to warn about.
HOLDER_SCRIPT = """
import sqlite3, sys, time
db = sqlite3.connect(sys.argv[1], isolation_level=None)
db.execute("PRAGMA journal_mode = WAL")
db.execute("INSERT OR REPLACE INTO system_settings (key, value) VALUES (?, ?)", (sys.argv[2], sys.argv[3]))
sys.stdout.write("ready\\n")
sys.stdout.flush()
time.sleep(300)
"""

failures = []


def fail(message):
    failures.append(message)
    print(f"  FAIL {message}")


def check(condition, message):
    if condition:
        print(f"  ok   {message}")
    else:
        fail(message)
    return condition


def free_port():
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def config_yaml(role, data_dir, http_port, dns_port, dns_on=True, dhcp_on=True):
    on = lambda b: "true" if b else "false"
    lines = [
        "env: \"development\"",
        "server:",
        '  name: "GoDDI"',
        f'  role: "{role}"',
        f'  http_addr: "127.0.0.1:{http_port}"',
        f'  public_url: "http://127.0.0.1:{http_port}"',
        f'  data_dir: "{data_dir}"',
        "database:",
        '  driver: "sqlite"',
        f'  dsn: "{data_dir}/goddi.db"',
        "dataplane:",
        '  lease_dsn: ""',
        '  zone_dsn: ""',
        "  sync_interval_seconds: 1",
        "dns:",
        f"  enabled: {on(dns_on)}",
        "  listeners:",
        "    udp:",
        f"      enabled: {on(dns_on)}",
        f'      address: "127.0.0.1:{dns_port}"',
        "    tcp:",
        "      enabled: false",
        "dhcp:",
        f"  enabled: {on(dhcp_on)}",
        "  interfaces: []",
        "security:",
        f'  jwt_secret: "{JWT_SECRET}"',
        f'  encryption_key: "{KEY_MATERIAL}"',
        "log:",
        '  level: "info"',
    ]
    return "\n".join(lines) + "\n"


def write_config(path, body):
    with open(path, "w", encoding="utf-8") as handle:
        handle.write(body)


def run(binary, args, cwd, env=None, expect_success=True):
    """Run the binary and return (returncode, stdout + stderr)."""
    process_env = dict(os.environ)
    if env:
        process_env.update(env)
    completed = subprocess.run(
        [binary] + args,
        cwd=cwd,
        env=process_env,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        timeout=180,
        check=False,
    )
    output = completed.stdout.decode("utf-8", "replace")
    if expect_success and completed.returncode != 0:
        fail(f"{' '.join(args)} exited {completed.returncode}:\n{output}")
    if not expect_success and completed.returncode == 0:
        fail(f"{' '.join(args)} succeeded when it should have been refused:\n{output}")
    return completed.returncode, output


def wait_for_file(path, seconds=20):
    deadline = time.time() + seconds
    while time.time() < deadline:
        if os.path.exists(path):
            return True
        time.sleep(0.2)
    return False


def stop(process):
    """Stop a child process. macOS has no `timeout`, so termination is explicit.

    SIGTERM first so the binary runs its own shutdown path and closes its
    databases cleanly; SIGKILL only if it does not go.
    """
    if process.poll() is not None:
        return
    process.terminate()
    try:
        process.wait(timeout=15)
    except subprocess.TimeoutExpired:
        process.kill()
        process.wait(timeout=10)


def sqlite(db_path, statement):
    completed = subprocess.run(
        ["sqlite3", db_path, statement],
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        check=False,
    )
    if completed.returncode != 0:
        fail(f"sqlite3 {statement!r} on {db_path}: {completed.stdout.decode()}")
    return completed.stdout.decode("utf-8", "replace").strip()


def archive_paths(report):
    """Pull the database lines out of a `goddi backup` report.

    The report is the operator-facing summary; parsing it is deliberate, because
    what the operator reads is what has to be true. Only the part between the
    name and the arrow is inspected -- the size column is padded and may contain
    its own spaces, so it is not a field this can split on.
    """
    found = {}
    for line in report.splitlines():
        match = re.match(r"^ {4}(control|dnsdata|leases) +(.*?)←", line)
        if match:
            found[match.group(1)] = "不存在" not in match.group(2)
    return found


def start_holder(db_path):
    """Start the attached writer and return the process once it reports ready."""
    process = subprocess.Popen(
        [sys.executable, "-c", HOLDER_SCRIPT, db_path, NOISE_KEY, NOISE_VALUE],
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
    )
    line = process.stdout.readline().decode("utf-8", "replace").strip()
    if line != "ready":
        stop(process)
        fail(f"the attached writer did not start: {line!r}")
        return None
    return process


def side_files(directory, names):
    """Every write-ahead log, shared-memory file or leftover copy beside a
    database this deployment serves from. Each one is a file a future reader
    could replay against a database it does not belong to."""
    found = []
    for name in names:
        for suffix in ("-wal", "-shm", "-journal", ".restore-new"):
            if os.path.exists(os.path.join(directory, name + suffix)):
                found.append(name + suffix)
    return found


def pending_total(report):
    """The pending count from a `migrate --status` report, or None."""
    match = re.search(r"^待应用合计：(\d+)$", report, re.M)
    return int(match.group(1)) if match else None


def store_summary(report, name):
    """The summary line that follows a store's location line."""
    match = re.search(r"^ {2}" + re.escape(name) + r" {2}.*\n {4}(.*)$", report, re.M)
    return match.group(1) if match else ""


def rewrite_manifest(archive, target, mutate):
    """Copy a *plaintext* archive, replacing only manifest.json.

    Nothing about the manifest is authenticated and nothing in it covers
    itself -- it lists digests for the members beside it -- so an archive whose
    manifest lies is still a valid archive, and every member check still
    passes. That is what makes it possible to hand the real binary an archive
    claiming a schema or a version it does not know: the file is genuine, only
    the claim is not.

    Only plaintext archives can be rewritten this way. An encrypted one is
    sealed in chunks with the manifest inside the ciphertext, which is the
    point of the format, and the harness does not pretend otherwise.
    """
    with tarfile.open(archive, "r:gz") as tar:
        members = []
        for member in tar.getmembers():
            if not member.isfile():
                continue
            handle = tar.extractfile(member)
            members.append((member.name, handle.read() if handle else b""))

    names = [name for name, _ in members]
    if MANIFEST_MEMBER not in names:
        fail(f"{archive} has no {MANIFEST_MEMBER}; the rewrite cannot be done")
        return None

    manifest = json.loads(dict(members)[MANIFEST_MEMBER].decode("utf-8"))
    mutate(manifest)
    replacement = json.dumps(manifest, ensure_ascii=False, indent=2).encode("utf-8")

    with tarfile.open(target, "w:gz") as tar:
        for name, data in members:
            payload = replacement if name == MANIFEST_MEMBER else data
            info = tarfile.TarInfo(name)
            info.size = len(payload)
            info.mode = 0o600
            tar.addfile(info, io.BytesIO(payload))
    return target


def case_all(binary, tmp_root):
    print("case: role=all, three stores, in-place restore")
    work = os.path.join(tmp_root, "all")
    data_dir = os.path.join(work, "data")
    os.makedirs(work, exist_ok=True)
    config = os.path.join(work, "config.yaml")
    write_config(config, config_yaml("all", data_dir, free_port(), free_port()))

    _, output = run(binary, ["migrate", "-c", config], work)
    check(os.path.exists(os.path.join(data_dir, "goddi.db")),
          "migrate created the control database")

    # Start the real process so the data-plane stores are created by the code
    # that decides this deployment owns them, not by the harness guessing.
    log_path = os.path.join(work, "serve.log")
    with open(log_path, "wb") as log:
        process = subprocess.Popen(
            [binary, "serve", "-c", config],
            cwd=work,
            stdout=log,
            stderr=subprocess.STDOUT,
        )
    try:
        lease_store = wait_for_file(os.path.join(data_dir, "leases.db"))
        check(lease_store, "a running role=all process created its lease store")
        zone_store = wait_for_file(os.path.join(data_dir, "dnsdata.db"))
        check(zone_store, "a running role=all process created its DNS data store")
    finally:
        stop(process)

    control = os.path.join(data_dir, "goddi.db")
    sqlite(control, f"INSERT OR REPLACE INTO system_settings (key, value) "
                    f"VALUES ('{MARKER_KEY}', '{MARKER_VALUE}')")
    check(sqlite(control, f"SELECT value FROM system_settings WHERE key = '{MARKER_KEY}'") == MARKER_VALUE,
          "the marker row is in the control database")

    snapshot = os.path.join(work, "snap.goddi-snap")
    _, backup_report = run(binary, ["backup", "-c", config, "-o", snapshot], work)
    check(os.path.exists(snapshot), "backup wrote an archive")

    found = archive_paths(backup_report)
    for name in ("control", "dnsdata", "leases"):
        check(found.get(name) is True, f"the archive holds the {name} store")
    check("不存在" not in backup_report,
          "no store was reported absent on a host that has all three")

    # Every archive is refused a second time at the same path: one mistyped
    # path must not turn two backups into one.
    run(binary, ["backup", "-c", config, "-o", snapshot], work, expect_success=False)

    sqlite(control, f"DELETE FROM system_settings WHERE key = '{MARKER_KEY}'")
    check(sqlite(control, f"SELECT value FROM system_settings WHERE key = '{MARKER_KEY}'") == "",
          "the marker row is gone again")

    _, verify_report = run(binary, ["restore", "-c", config, "-i", snapshot, "--verify"], work)
    check("校验通过" in verify_report, "restore --verify accepted the archive")
    check(not any(os.path.exists(os.path.join(work, name)) for name in ("unpacked", "archive")),
          "restore --verify wrote nothing")

    run(binary, ["restore", "-c", config, "-i", snapshot, "--in-place"], work, expect_success=False)
    run(binary, ["restore", "-c", config, "-i", snapshot, "--verify"], work,
        env={"GODDI_SECURITY_ENCRYPTION_KEY": "the-wrong-key-material-entirely"},
        expect_success=False)

    # Attach a writer so a write-ahead log is genuinely there when the restore
    # runs. Without it this whole part of the check would be vacuous: a cleanly
    # closed connection removes its own log.
    holder = start_holder(control)
    try:
        check(os.path.exists(control + "-wal"),
              "a write-ahead log is in place before the restore")

        _, restore_report = run(binary, ["restore", "-c", config, "-i", snapshot, "--in-place", "--yes"], work)
    finally:
        if holder is not None:
            stop(holder)

    for name in ("control", "dnsdata", "leases"):
        check(f"已替换 {name}" in restore_report, f"the in-place restore replaced the {name} store")
    check("配置文件刻意未应用" in restore_report,
          "the in-place restore says the archived configuration was not applied")

    # Asserted before anything else opens the restored databases, because a
    # reader would create or consume exactly the files being looked for.
    leftovers = side_files(data_dir, ("goddi.db", "dnsdata.db", "leases.db"))
    check(not leftovers,
          f"the log, shared-memory file and leftover copy were removed with the databases they belonged to (found {leftovers})")

    check(sqlite(control, f"SELECT value FROM system_settings WHERE key = '{MARKER_KEY}'") == MARKER_VALUE,
          "the restored control database carries the row the archive was taken with")
    check(sqlite(control, f"SELECT value FROM system_settings WHERE key = '{NOISE_KEY}'") == "",
          "the restored control database does not carry the row written after the archive was taken")

    safety = re.search(r"替换前已留存当前数据库副本：(\S+)", restore_report)
    if not check(safety is not None, "the in-place restore reported where the safety copy is"):
        return
    safety_path = safety.group(1)
    check(os.path.exists(safety_path), "the safety copy exists")

    # The safety copy is an archive like any other, so it is read the way an
    # operator would read it rather than by reaching inside it.
    #
    # Members are named for the store, not for the file: the control database is
    # db/control.db whatever the deployment happens to call it on disk. That is
    # what lets a restore be driven from the archive's own index, and it is why
    # this looks for db/control.db and not db/goddi.db.
    safety_unpacked = os.path.join(work, "safety-unpacked")
    run(binary, ["restore", "-c", config, "-i", safety_path, "--into", safety_unpacked], work)
    safety_db = os.path.join(safety_unpacked, "db", "control.db")
    check(sqlite(safety_db, f"SELECT value FROM system_settings WHERE key = '{MARKER_KEY}'") == "",
          "the safety copy holds the state that was replaced, not the state that replaced it")
    check(sqlite(safety_db, f"SELECT value FROM system_settings WHERE key = '{NOISE_KEY}'") == NOISE_VALUE,
          "the safety copy read through the write-ahead log, so it holds the row that was only in the log")

    for name in ("goddi.db", "dnsdata.db", "leases.db"):
        check(os.path.exists(os.path.join(data_dir, name)), f"{name} is present after the restore")


def case_control_only(binary, tmp_root):
    print("case: role=control, no data-plane stores, unpack only")
    work = os.path.join(tmp_root, "control")
    data_dir = os.path.join(work, "data")
    os.makedirs(work, exist_ok=True)
    config = os.path.join(work, "config.yaml")
    write_config(config, config_yaml("control", data_dir, free_port(), free_port()))

    run(binary, ["migrate", "-c", config], work)
    check(not os.path.exists(os.path.join(data_dir, "leases.db")),
          "a control-only instance has no lease store to archive")

    snapshot = os.path.join(work, "snap.goddi-snap")
    _, report = run(binary, ["backup", "-c", config, "-o", snapshot], work)
    found = archive_paths(report)
    check(found.get("control") is True, "the archive holds the control store")
    for name in ("dnsdata", "leases"):
        check(found.get(name) is False,
              f"the absent {name} store is recorded as absent rather than omitted")

    unpacked = os.path.join(work, "unpacked")
    _, restore_report = run(binary, ["restore", "-c", config, "-i", snapshot, "--into", unpacked], work)
    check("已解包到" in restore_report, "restore --into reported an unpack directory")
    for member in ("db/control.db", "config/config.yaml"):
        check(os.path.exists(os.path.join(unpacked, *member.split("/"))),
              f"the unpacked archive holds {member}")

    run(binary, ["restore", "-c", config, "-i", snapshot, "--into", unpacked], work, expect_success=False)


def case_migrate_status(binary, tmp_root):
    """The report an operator reads before deciding to start a restored host.

    Two properties matter and neither is visible in a unit test: the command
    must not create the database it was asked about, and the counts it prints
    must come from the deployment's own store list -- which is decided by role,
    not by the harness.
    """
    print("case: migrate --status and --check")
    work = os.path.join(tmp_root, "status")
    data_dir = os.path.join(work, "data")
    os.makedirs(work, exist_ok=True)
    config = os.path.join(work, "config.yaml")
    write_config(config, config_yaml("all", data_dir, free_port(), free_port()))
    control = os.path.join(data_dir, "goddi.db")

    _, report = run(binary, ["migrate", "-c", config, "--status"], work)
    total = pending_total(report)
    check(total is not None and total > 0,
          f"a host that has never been migrated reports pending migrations (got {total!r})")
    check("数据库尚未创建" in store_summary(report, "control"),
          "the control store is reported as not created rather than as an error")
    check(not os.path.exists(control),
          "migrate --status did not create the database it was asked about")
    run(binary, ["migrate", "-c", config, "--check"], work, expect_success=False)

    run(binary, ["migrate", "-c", config], work)
    check(os.path.exists(control), "migrate created the control database")

    _, report = run(binary, ["migrate", "-c", config, "--status"], work)
    check("无待应用" in store_summary(report, "control"),
          "the control store has nothing pending once migrate has run")
    for name in ("dnsdata", "leases"):
        check("数据库尚未创建" in store_summary(report, name),
              f"the {name} store is reported as not created until a data plane runs")
        check(not os.path.exists(os.path.join(data_dir, name + ".db")),
              f"migrate --status did not create {name}.db")
    # The total is checked against the two stores' own reported counts rather
    # than against a number written here: a literal would have to be updated
    # every time a data-plane migration is added, and the day someone forgets is
    # the day this assertion stops meaning anything.
    per_store = re.search(r"本二进制自带 (\d+) 个迁移", store_summary(report, "dnsdata"))
    check(per_store is not None, "the report says how many migrations the build ships for a store")
    if per_store:
        expected = 2 * int(per_store.group(1))
        check(total is not None and pending_total(report) == expected,
              f"the total counts every store's pending migrations (want {expected}, got {pending_total(report)})")
    run(binary, ["migrate", "-c", config, "--check"], work, expect_success=False)

    log_path = os.path.join(work, "serve.log")
    with open(log_path, "wb") as log:
        process = subprocess.Popen(
            [binary, "serve", "-c", config],
            cwd=work,
            stdout=log,
            stderr=subprocess.STDOUT,
        )
    try:
        check(wait_for_file(os.path.join(data_dir, "leases.db")),
              "a running role=all process created its lease store")
        check(wait_for_file(os.path.join(data_dir, "dnsdata.db")),
              "a running role=all process created its DNS data store")
    finally:
        stop(process)

    _, report = run(binary, ["migrate", "-c", config, "--status"], work)
    check(pending_total(report) == 0,
          f"nothing is pending once every store exists (got {pending_total(report)})")
    for name in ("control", "dnsdata", "leases"):
        check("无待应用" in store_summary(report, name), f"{name} reports nothing pending")

    _, check_report = run(binary, ["migrate", "-c", config, "--check"], work)
    check("每个存储都不缺迁移" in check_report,
          "migrate --check says so out loud when there is nothing to do")


def case_restore_gates(binary, tmp_root):
    """The gate that decides whether this binary can use an archive.

    The work is done by handing the real binary archives whose manifest lies.
    A plaintext archive is rewritten member by member so only manifest.json
    differs, which means every integrity check still passes and the only thing
    that can refuse the archive is the gate under test.
    """
    print("case: restore refuses the future and accepts the past")
    work = os.path.join(tmp_root, "gates")
    data_dir = os.path.join(work, "data")
    os.makedirs(work, exist_ok=True)
    config = os.path.join(work, "config.yaml")
    write_config(config, config_yaml("control", data_dir, free_port(), free_port()))

    run(binary, ["migrate", "-c", config], work)
    snapshot = os.path.join(work, "snap.goddi-snap")
    run(binary, ["backup", "-c", config, "-o", snapshot, "--plaintext"], work)
    run(binary, ["restore", "-c", config, "-i", snapshot, "--verify"], work)

    # A host that has never been migrated. This is where a restore is prepared,
    # and it is where the comparison this gate replaced checked nothing at all:
    # with no live database to read a version from, it skipped the check.
    fresh = os.path.join(tmp_root, "gates-fresh")
    fresh_data = os.path.join(fresh, "data")
    os.makedirs(fresh, exist_ok=True)
    fresh_config = os.path.join(fresh, "config.yaml")
    write_config(fresh_config, config_yaml("control", fresh_data, free_port(), free_port()))

    def ahead_schema(document):
        for entry in document.get("databases", []):
            entry["schema_version"] = SCHEMA_AHEAD

    ahead = rewrite_manifest(snapshot, os.path.join(work, "ahead-schema.goddi-snap"), ahead_schema)
    if ahead is None:
        return
    check(not os.path.exists(os.path.join(fresh_data, "goddi.db")),
          "the fresh host has no database before the restore is attempted")

    _, verify_report = run(binary, ["restore", "-c", fresh_config, "-i", ahead, "--verify"], fresh)
    check("校验通过" in verify_report,
          "restore --verify still reports on an archive this binary cannot use")
    check(str(SCHEMA_AHEAD) in verify_report,
          "restore --verify names the schema version it found")

    _, refusal = run(binary, ["restore", "-c", fresh_config, "-i", ahead, "--into",
                              os.path.join(fresh, "out")], fresh, expect_success=False)
    check(str(SCHEMA_AHEAD) in refusal,
          "the refusal names the schema version that is too new")
    check("已解包到" in refusal,
          "the refusal says the archive was unpacked for inspection rather than leaving it a mystery")
    check(not os.path.exists(os.path.join(fresh_data, "goddi.db")),
          "the refused unpack left the fresh host without a database")
    run(binary, ["restore", "-c", fresh_config, "-i", ahead, "--in-place", "--yes"],
        fresh, expect_success=False)
    check(not os.path.exists(os.path.join(fresh_data, "goddi.db")),
          "the refused in-place restore did not create a database to replace")

    def ahead_version(document):
        document["version"] = VERSION_AHEAD

    ahead_build = rewrite_manifest(snapshot, os.path.join(work, "ahead-build.goddi-snap"), ahead_version)
    _, refusal = run(binary, ["restore", "-c", fresh_config, "-i", ahead_build, "--in-place", "--yes"],
                     fresh, expect_success=False)
    check(VERSION_AHEAD in refusal,
          f"the refusal names the version that wrote the archive (looked for {VERSION_AHEAD})")

    # The direction that used to be wrong: an archive behind the live database
    # is the ordinary "restore last night's backup" case, and the comparison
    # against the live file that this gate replaced refused it.
    def behind_schema(document):
        for entry in document.get("databases", []):
            entry["schema_version"] = SCHEMA_BEHIND

    behind = rewrite_manifest(snapshot, os.path.join(work, "behind-schema.goddi-snap"), behind_schema)
    if behind is None:
        return

    # The live level is read from the binary's own report rather than written
    # here, so the note the restore prints can be checked against what the
    # database actually says.
    _, status = run(binary, ["migrate", "-c", config, "--status"], work)
    applied = re.search(r"已应用 (\d+) /", store_summary(status, "control"))
    if not check(applied is not None, "the control store reports which schema version it is at"):
        return
    live_level = applied.group(1)
    check(int(live_level) > SCHEMA_BEHIND,
          f"the live control database is ahead of the archive being restored ({live_level} > {SCHEMA_BEHIND})")

    _, restore_report = run(binary, ["restore", "-c", config, "-i", behind, "--in-place", "--yes"], work)
    check("已替换 control" in restore_report,
          "an archive behind the live database is restored rather than refused")
    check(f"本机原为 schema {live_level}，归档为 {SCHEMA_BEHIND}" in restore_report,
          f"the restore reports both schema levels ({live_level} -> {SCHEMA_BEHIND})")
    check("下一步：启动服务" in restore_report,
          "the restore says the next start brings the schema forward")


def main():
    if shutil.which("sqlite3") is None:
        print("sqlite3 is required for this check")
        return 2

    tmp_root = tempfile.mkdtemp(prefix="goddi-w12-smoke-")
    try:
        binary = os.path.join(tmp_root, BIN_NAME)
        print(f"building {BIN_NAME} ...")
        build = subprocess.run(
            ["go", "build", "-o", binary, "./cmd/goddi"],
            cwd=ROOT,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            check=False,
        )
        if build.returncode != 0:
            print(build.stdout.decode("utf-8", "replace"))
            print("build failed")
            return 2

        case_all(binary, tmp_root)
        case_control_only(binary, tmp_root)
        case_migrate_status(binary, tmp_root)
        case_restore_gates(binary, tmp_root)
    finally:
        shutil.rmtree(tmp_root, ignore_errors=True)

    if failures:
        print(f"\n{len(failures)} assertion(s) failed")
        return 1
    print("\nevery snapshot and restore assertion held")
    return 0


if __name__ == "__main__":
    sys.exit(main())
