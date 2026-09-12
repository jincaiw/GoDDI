#!/usr/bin/env python3
"""Disaster-recovery drill for W12-c: recover onto a host that has nothing.

This is the executable form of `docs/runbook/灾备恢复.md`. Each numbered step
below is the same step as the runbook's, in the same order, so a change to one
is visibly a change to the other.

What it rehearses that the W12-a/W12-b smoke check does not:

  * The source instance is **destroyed** before the recovery starts. Every other
    check reads an archive on the machine that wrote it, which cannot show a
    dependency on a path that only exists there.
  * The target lives under a **different absolute path**, with a configuration
    file that has never pointed at the source.
  * The target gets **no `migrate`**. It starts as a host that has never run,
    and the recovery has to be what makes it into an instance.
  * The recovery is asserted to be the archive's own member **byte for byte**, per
    database -- not merely to contain the right row. The comparison is made
    against the member, never against the source's live file: the member came
    out of `VACUUM INTO`, which rewrites the database as a compacted single file
    in rollback-journal mode, so expecting it to equal a live write-ahead-log
    file would fail on a correct recovery. The drill asserts both.
  * The recovered instance is **started and probed**, because "the files are in
    place" and "the service comes up" are different claims and only the second
    one is what an operator needs.

What the drill deliberately does NOT rehearse:

  * **Losing the key material.** The archive is sealed with a key that is not
    inside it, which is the point of the format. Nothing here tests recovering
    that key from wherever the operator kept it, because nothing here can.
  * **The DHCP listener.** The source serves DHCP so its lease store exists to
    be archived; the target does not, because udp4 :67 is privileged and this
    harness is not. A DHCP-enabled target would report `/ready` as failing with
    `dhcp_listener_failed_to_start`, which is true and says nothing about the
    recovery.

Run from the repository root:

    python3 scripts/w12c_restore_drill.py

Exit code 0 means every step of the rehearsal behaved as the runbook says.
"""

import hashlib
import os
import re
import shutil
import socket
import subprocess
import sys
import tempfile
import time

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BIN_NAME = "goddi-w12c-drill"
JWT_SECRET = "w12c-drill-jwt-secret-not-a-real-credential-0123456789"
KEY_MATERIAL = "w12c-drill-encryption-key-material-0123456789"
MARKER_KEY = "w12c-drill-marker"
SNAPSHOT_MAGIC = b"GODDI-SNAP-ENC1"

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


def step(number, title):
    print(f"\n步骤 {number}：{title}")


def free_port():
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def config_yaml(role, data_dir, http_port, dns_port, dhcp_enabled):
    lines = [
        'env: "development"',
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
        "  enabled: true",
        "  listeners:",
        "    udp:",
        "      enabled: true",
        f'      address: "127.0.0.1:{dns_port}"',
        "    tcp:",
        "      enabled: false",
        "dhcp:",
        f"  enabled: {'true' if dhcp_enabled else 'false'}",
        "  interfaces: []",
        "security:",
        f'  jwt_secret: "{JWT_SECRET}"',
        f'  encryption_key: "{KEY_MATERIAL}"',
        "log:",
        '  level: "info"',
    ]
    return "\n".join(lines) + "\n"


def run(binary, args, cwd, expect_success=True):
    completed = subprocess.run(
        [binary] + args,
        cwd=cwd,
        env=dict(os.environ),
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        timeout=300,
        check=False,
    )
    output = completed.stdout.decode("utf-8", "replace")
    if expect_success and completed.returncode != 0:
        fail(f"{' '.join(args)} exited {completed.returncode}:\n{output}")
    return completed.returncode, output


def stop(process):
    if process.poll() is not None:
        return
    process.terminate()
    try:
        process.wait(timeout=20)
    except subprocess.TimeoutExpired:
        process.kill()
        process.wait(timeout=10)


def wait_for_file(path, seconds=30):
    deadline = time.time() + seconds
    while time.time() < deadline:
        if os.path.exists(path):
            return True
        time.sleep(0.2)
    return False


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


def sha256(path):
    digest = hashlib.sha256()
    with open(path, "rb") as handle:
        for block in iter(lambda: handle.read(1 << 20), b""):
            digest.update(block)
    return digest.hexdigest()


def http_status(url):
    """Return (status_code, body). curl is used rather than a client library so
    the drill has no dependencies beyond what the runbook already assumes."""
    completed = subprocess.run(
        ["curl", "-s", "-w", "\n%{http_code}", url],
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        check=False,
    )
    text = completed.stdout.decode("utf-8", "replace")
    body, _, status = text.rpartition("\n")
    return status.strip(), body


def serve(binary, config, cwd, log_path, waiting_for):
    """Start the service and wait until it has done what the caller needs."""
    with open(log_path, "wb") as log:
        process = subprocess.Popen(
            [binary, "serve", "-c", config],
            cwd=cwd,
            stdout=log,
            stderr=subprocess.STDOUT,
        )
    for path in waiting_for:
        if not wait_for_file(path):
            stop(process)
            fail(f"the service did not create {path}")
            return None
    return process


def store_digests(data_dir):
    """sha256 of every store file present, by store name."""
    digests = {}
    for name, filename in (("control", "goddi.db"), ("dnsdata", "dnsdata.db"), ("leases", "leases.db")):
        path = os.path.join(data_dir, filename)
        if os.path.exists(path):
            digests[name] = sha256(path)
    return digests


def drill(binary, tmp_root):
    src = os.path.join(tmp_root, "source")
    src_data = os.path.join(src, "data")
    vault = os.path.join(tmp_root, "vault")
    dst = os.path.join(tmp_root, "target")
    dst_data = os.path.join(dst, "data")
    for directory in (src_data, vault, dst_data):
        os.makedirs(directory, exist_ok=True)

    src_config = os.path.join(src, "config.yaml")
    dst_config = os.path.join(dst, "config.yaml")
    src_http, src_dns = free_port(), free_port()
    dst_http, dst_dns = free_port(), free_port()
    # The source serves DHCP so that its lease store exists to be archived. The
    # target does not, for a reason that has nothing to do with the recovery:
    # the DHCP listener needs udp4 :67, this harness runs unprivileged, and a
    # DHCP-enabled process here would report /ready as failing with
    # dhcp_listener_failed_to_start -- a true statement about the harness that
    # would make the readiness assertion below meaningless.
    write(src_config, config_yaml("all", src_data, src_http, src_dns, dhcp_enabled=True))
    write(dst_config, config_yaml("all", dst_data, dst_http, dst_dns, dhcp_enabled=False))

    # ---------------------------------------------------------------- 1
    step(1, "把源实例建起来，并让它持有可识别的数据")
    run(binary, ["migrate", "-c", src_config], src)
    process = serve(binary, src_config, src, os.path.join(src, "serve.log"),
                    [os.path.join(src_data, "leases.db"), os.path.join(src_data, "dnsdata.db")])
    if process is None:
        return
    stop(process)
    check(all(os.path.exists(os.path.join(src_data, name))
              for name in ("goddi.db", "dnsdata.db", "leases.db")),
          "the source instance owns all three stores")

    marker = f"drill-{os.urandom(8).hex()}"
    sqlite(os.path.join(src_data, "goddi.db"),
           f"INSERT OR REPLACE INTO system_settings (key, value) VALUES ('{MARKER_KEY}', '{marker}')")
    check(sqlite(os.path.join(src_data, "goddi.db"),
                 f"SELECT value FROM system_settings WHERE key = '{MARKER_KEY}'") == marker,
          "the marker row is in the source control database")

    source_digests = store_digests(src_data)
    check(len(source_digests) == 3, f"three stores were fingerprinted ({sorted(source_digests)})")

    # ------------------------------------------------------------------- 2
    step(2, "取归档，并把它放到源实例之外的地方")
    archive = os.path.join(vault, "goddi-drill.goddi-snap")
    _, backup_report = run(binary, ["backup", "-c", src_config, "-o", archive], src)
    check(os.path.exists(archive), "backup wrote an archive")

    with open(archive, "rb") as handle:
        head = handle.read(len(SNAPSHOT_MAGIC))
    check(head == SNAPSHOT_MAGIC, "the archive is sealed, not plaintext")

    _, verify_report = run(binary, ["restore", "-c", src_config, "-i", archive, "--verify"], src)
    check("校验通过" in verify_report, "the archive verifies before anything is destroyed")

    # The archived schema level is read from the report so it can be compared
    # with what the recovered instance reports afterwards: the manifest's claim
    # and the file it describes have to agree.
    archived_level = re.search(r"^\s+control\s.*schema (\d+)", verify_report, re.M)
    if not check(archived_level is not None, "the report names the archived schema level"):
        return
    archived_level = int(archived_level.group(1))
    print(f"   （归档的控制库 schema 版本为 {archived_level}）")

    # ------------------------------------------------------------------- 3
    step(3, "销毁源实例——这一步之后就没有回头路了")
    shutil.rmtree(src)
    check(not os.path.exists(src), "the source instance is gone")
    check(os.path.exists(archive), "the archive survives, because it is not under the source")

    # ------------------------------------------------------------------- 4
    step(4, "目标主机：确认它确实什么都没有")
    dst_config_digest = sha256(dst_config)
    _, status = run(binary, ["migrate", "-c", dst_config, "--status"], dst)
    check("数据库尚未创建" in status, "the target reports its stores as not created")
    check("待应用合计：0" not in status, "the target has migrations to apply")
    check(not os.path.exists(os.path.join(dst_data, "goddi.db")),
          "the target has no control database")

    # ------------------------------------------------------------------- 5
    step(5, "恢复到目标主机")
    _, restore_report = run(binary, ["restore", "-c", dst_config, "-i", archive, "--in-place", "--yes"], dst)
    check("目标主机上没有任何库，没有需要留存的替换前副本" in restore_report,
          "the recovery notices there was nothing to preserve, rather than failing on a database that never existed")
    for name in ("control", "dnsdata", "leases"):
        check(f"已替换 {name}" in restore_report, f"the {name} store was put in place")
    check("配置文件刻意未应用" in restore_report,
          "the recovery says the archived configuration was not applied")

    # ------------------------------------------------------------------- 6
    step(6, "核对恢复结果：文件、配置、schema 三样都要对")
    check(sha256(dst_config) == dst_config_digest,
          "the target's own configuration was not overwritten by the archived one")

    # Nothing has opened the recovered databases yet, so this is the moment at
    # which they can be compared with what was archived. The comparison is made
    # against the archive's own member -- unpacked separately -- and not against
    # the source's live file.
    #
    # Those two are not the same thing and must not be expected to be: the
    # member was produced by VACUUM INTO, which rewrites the database as a
    # compacted single file in rollback-journal mode, while the live source file
    # was a write-ahead-log database that had been written to. Comparing the
    # recovered file against the live one would fail on a correct recovery.
    unpacked = os.path.join(dst, "unpacked")
    run(binary, ["restore", "-c", dst_config, "-i", archive, "--into", unpacked], dst)
    for name, filename in (("control", "goddi.db"), ("dnsdata", "dnsdata.db"), ("leases", "leases.db")):
        member = os.path.join(unpacked, "db", name + ".db")
        recovered = os.path.join(dst_data, filename)
        if not check(os.path.exists(member), f"the archive holds a {name} member"):
            continue
        check(sha256(recovered) == sha256(member),
              f"the recovered {name} store is the archive's member, byte for byte")
        check(sha256(recovered) != source_digests.get(name),
              f"the recovered {name} store is not the source's live file, which it must not be expected to be")
        check(sqlite(recovered, "PRAGMA integrity_check") == "ok",
              f"the recovered {name} store passes SQLite's integrity check")

    check(not os.path.exists(os.path.join(src, "data")), "the source's data directory is still gone")

    leftovers = [name + suffix
                 for name in ("goddi.db", "dnsdata.db", "leases.db")
                 for suffix in ("-wal", "-shm", "-journal", ".restore-new")
                 if os.path.exists(os.path.join(dst_data, name + suffix))]
    check(not leftovers, f"no log, shared-memory file or leftover copy remains (found {leftovers})")

    _, status = run(binary, ["migrate", "-c", dst_config, "--status"], dst)
    check("无待应用" in status, "the recovered stores need no migrations")
    level = re.search(r"已应用 (\d+) / 本二进制最高 (\d+)", status)
    if check(level is not None, "the target reports its schema level"):
        check(int(level.group(1)) == archived_level,
              f"the recovered level matches the archive's claim ({level.group(1)} == {archived_level})")
    check(sqlite(os.path.join(dst_data, "goddi.db"),
                 f"SELECT value FROM system_settings WHERE key = '{MARKER_KEY}'") == marker,
          "the recovered control database carries the source's marker row")

    # ------------------------------------------------------------------- 7
    step(7, "把恢复出来的实例真正启动起来并探活")
    process = serve(binary, dst_config, dst, os.path.join(dst, "serve.log"), [])
    if process is None:
        return
    try:
        # Both probes are polled to 200 rather than sampled once: the listener
        # accepts connections a moment before the handler chain answers them, so
        # a single request can be refused on a service that is coming up
        # correctly. Nothing here is about that race.
        deadline = time.time() + 30
        ready_code, ready_body, health_code = "", "", ""
        while time.time() < deadline:
            health_code, _ = http_status(f"http://127.0.0.1:{dst_http}/health")
            ready_code, ready_body = http_status(f"http://127.0.0.1:{dst_http}/ready")
            if health_code == "200" and ready_code == "200":
                break
            time.sleep(0.5)
        check(health_code == "200", f"the recovered instance answers its liveness probe (HTTP {health_code})")
        check(ready_code == "200",
              f"the recovered instance reports itself ready (HTTP {ready_code}, body {ready_body.strip()!r})")
    finally:
        stop(process)

    with open(os.path.join(dst, "serve.log"), "rb") as handle:
        log = handle.read().decode("utf-8", "replace")
    check("GoDDI stopped gracefully" in log, "the recovered instance shut down cleanly")
    check("panic" not in log.lower(), "the recovered instance did not panic")
    # The substantive claim behind the probe: the replicated plane came up
    # current from the restored store, rather than being reported ok because
    # nothing was checked.
    check("dataplane: readiness ok" in log,
          "the recovered instance's data plane reported itself current")


def write(path, body):
    with open(path, "w", encoding="utf-8") as handle:
        handle.write(body)


def main():
    for tool in ("sqlite3", "curl"):
        if shutil.which(tool) is None:
            print(f"{tool} is required for this drill")
            return 2

    tmp_root = tempfile.mkdtemp(prefix="goddi-w12c-drill-")
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

        drill(binary, tmp_root)
    finally:
        shutil.rmtree(tmp_root, ignore_errors=True)

    if failures:
        print(f"\n{len(failures)} 项演练步骤未按预期成立")
        return 1
    print("\n演练完成：一台什么都没有的主机变成了可用实例")
    return 0


if __name__ == "__main__":
    sys.exit(main())
