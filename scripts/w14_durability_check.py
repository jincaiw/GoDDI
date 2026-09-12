#!/usr/bin/env python3
"""Process-level durability check for W14-d.

Two things are recorded here, and they are deliberately *not* reported as one
number, because they are not the same claim:

  A. A graceful restart (SIGTERM) does not lose a decision that was already
     committed. This is the ordinary case: a package upgrade, a config change,
     an operator restarting the service.

  B. A crash (SIGKILL, no deferred close, no final checkpoint) does not lose a
     committed decision, and the store comes back self-consistent. The
     write-ahead log is left behind, which is the concrete reason this is a
     crash-consistency result and not a durability one -- see the caveat at the
     end of this file.

  C. Storage refuses the write, and the command says so instead of reporting a
     permission it did not obtain.

What the asserts are about: every case drives the real binary -- `goddi serve`
to own the lease store, `goddi ha` to write to it -- and then reads the result
back through the read-only path (`goddi ha status --json`, which opens the file
with `mode=ro` and runs no migrations), so nothing under test is asked to
describe its own work.

On the timings: the harness runs unprivileged on a machine that already has a
DHCP service, so udp4 0.0.0.0:67 cannot be bound and the process logs that and
carries on. Nothing here serves a client; the lease store is created and written
by the parts of the process that do not need the socket.

Run from the repository root:

    python3 scripts/w14_durability_check.py

Exit code 0 means every case behaved as advertised.
"""

import json
import os
import shutil
import socket
import stat
import subprocess
import sys
import tempfile
import time

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BIN_NAME = "goddi-w14-durability"

TOKEN = "w14-durability-token-not-a-real-credential"

CONFIRM_TIMEOUT = "500ms"
HEARTBEAT = "200ms"
PEER_STALE_AFTER = "1s"

REASON_GRACEFUL = "w14: written before a graceful restart"
REASON_CRASH = "w14: written before the process was killed"

ADOPTED = "an operator has approved running without a second copy"


def free_port():
    """Ask the kernel for a port nothing is using, then let it go."""
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def config_yaml(data_dir, ha_role, listen_port, peer_port):
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
        '  node_id: "w14-durability"',
        f'  role: "{ha_role}"',
        f'  listen_addr: "127.0.0.1:{listen_port}"',
        f'  peer_address: "127.0.0.1:{peer_port}"',
        f'  peer_token: "{TOKEN}"',
        f'  confirm_timeout: "{CONFIRM_TIMEOUT}"',
        f'  heartbeat_interval: "{HEARTBEAT}"',
        f'  peer_stale_after: "{PEER_STALE_AFTER}"',
        "security:",
        '  jwt_secret: "w14-durability-secret-not-a-real-credential-0123456789"',
        "log:",
        '  level: "info"',
    ]
    return "\n".join(lines) + "\n"


class Node:
    """A running binary whose log can be read while it is still up."""

    def __init__(self, binary, name, cfg_path, tmp_root):
        self.name = name
        self.log_path = os.path.join(tmp_root, f"{name}.log")
        self._log = open(self.log_path, "w")
        self.proc = subprocess.Popen(
            [binary, "serve", "-c", cfg_path],
            stdout=self._log, stderr=subprocess.STDOUT,
            cwd=tmp_root, text=True,
        )

    def log(self):
        self._log.flush()
        try:
            with open(self.log_path, "r") as fh:
                return fh.read()
        except OSError:
            return ""

    def stop(self, kill=False):
        if self.proc.poll() is None:
            if kill:
                self.proc.kill()
            else:
                self.proc.terminate()
        try:
            self.proc.wait(timeout=20)
        except subprocess.TimeoutExpired:
            self.proc.kill()
            self.proc.wait()
        self._log.close()


def wait_for(node, marker, timeout=20.0):
    """Wait until a marker appears in a running process's log."""
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
    """Run `goddi ha ...` and return (returncode, stdout+stderr)."""
    done = subprocess.run(
        [binary, "ha"] + args, capture_output=True, text=True, cwd=cwd, timeout=60,
    )
    return done.returncode, done.stdout + done.stderr


def ha_status(binary, cfg_path, cwd):
    """Read the machine-readable status, or raise with what it said instead."""
    code, out = ha(binary, ["status", "-c", cfg_path, "--json"], cwd)
    if code != 0:
        raise AssertionError(f"goddi ha status exited {code}: {out}")
    try:
        return json.loads(out)
    except json.JSONDecodeError as exc:
        raise AssertionError(f"goddi ha status did not print JSON ({exc}): {out!r}")


def one_decision(binary, tmp_root, name, reason):
    """Bring a node up, commit one operator decision, and return the pieces.

    The node is created and stopped here so the lease store exists before the
    case decides how to end the process.
    """
    data_dir = os.path.join(tmp_root, name, "data")
    os.makedirs(data_dir, exist_ok=True)
    cfg_path = os.path.join(tmp_root, name, "config.yaml")
    with open(cfg_path, "w") as fh:
        fh.write(config_yaml(data_dir, "primary", free_port(), free_port()))

    node = Node(binary, name, cfg_path, tmp_root)
    ok, out = wait_for(node, "DHCP HA enabled")
    if not ok:
        node.stop()
        raise AssertionError(f"the node never reported its HA settings: {out[-600:]!r}")

    before = ha_status(binary, cfg_path, tmp_root)
    if before["state"] != "primary" or before["degraded"]:
        node.stop()
        raise AssertionError(f"a fresh primary reports state={before['state']} "
                             f"degraded={before['degraded']}, want primary/False")

    code, out = ha(binary, ["degrade", "-c", cfg_path, "--confirm", "--reason", reason], tmp_root)
    if code != 0:
        node.stop()
        raise AssertionError(f"goddi ha degrade --confirm exited {code}: {out}")

    # The decision has to reach the *running* service, not only the file, or
    # case A and case B would both be measuring a file rather than the process
    # this script claims to be about.
    adopted, out = wait_for(node, ADOPTED)
    if not adopted:
        node.stop()
        raise AssertionError(f"the running service never adopted the decision: {out[-600:]!r}")

    return node, cfg_path, data_dir


def assert_one_decision(status, where):
    """Three readings of one fact, which must not disagree.

    `degraded`, the derived state name, and `redundancy_known and not redundant`
    are all computed from the same two meta rows. A store that was interrupted
    between writing the marker and writing its reason would still answer the
    first, but a half-written decision showing up as three disagreeing answers
    is exactly what a crash test should refuse to accept.
    """
    problems = []
    if not status["degraded"]:
        problems.append(f"{where}: the permission is gone")
    if status["state"] != "primary-degraded":
        problems.append(f"{where}: state = {status['state']}, want primary-degraded")
    if not status["redundancy_known"] or status["redundant"]:
        problems.append(f"{where}: redundancy_known={status['redundancy_known']} "
                        f"redundant={status['redundant']}, want True/False")
    if not status.get("degraded_reason", "").strip():
        problems.append(f"{where}: the permission has no reason attached")
    return problems


def case_graceful_restart(binary, tmp_root):
    """A. SIGTERM, then bring the service back."""
    try:
        node, cfg_path, _ = one_decision(binary, tmp_root, "graceful", REASON_GRACEFUL)
    except AssertionError as exc:
        return [str(exc)]

    try:
        node.stop()  # SIGTERM: the process gets to run its deferred closes.
    finally:
        pass

    problems = []
    status = ha_status(binary, cfg_path, tmp_root)
    problems += assert_one_decision(status, "after the graceful stop")
    if status.get("degraded_reason") != REASON_GRACEFUL:
        problems.append(f"the reason came back as {status.get('degraded_reason')!r}, "
                        f"want {REASON_GRACEFUL!r}")

    # And the restarted service serves under it rather than merely storing it.
    restarted = Node(binary, "graceful-restarted", cfg_path, tmp_root)
    try:
        found, out = wait_for(restarted, '"state":"primary-degraded"')
        if not found:
            problems.append("the restarted service did not come up degraded; the "
                            f"decision reached the file but not the process: {out[-600:]!r}")
    finally:
        restarted.stop()
    return problems


def case_power_loss(binary, tmp_root):
    """B. SIGKILL -- no deferred close, no checkpoint, no chance to finish."""
    try:
        node, cfg_path, data_dir = one_decision(binary, tmp_root, "crash", REASON_CRASH)
    except AssertionError as exc:
        return [str(exc)]

    node.stop(kill=True)

    problems = []
    wal = os.path.join(data_dir, "leases.db-wal")
    try:
        size = os.path.getsize(wal)
    except OSError as exc:
        return problems + [f"the write-ahead log is gone after the kill ({exc}); the store "
                           f"cannot have recovered the way this case assumes"]
    if size == 0:
        problems.append("the write-ahead log is empty after the kill; the decision can only "
                        "have survived by having been checkpointed before the process died, "
                        "which is not the situation this case is meant to cover")

    # Read back through the read-only path: `mode=ro`, no migrations, nothing
    # under test is allowed to repair the file on the way to describing it.
    status = ha_status(binary, cfg_path, tmp_root)
    problems += assert_one_decision(status, "after the kill")
    if status.get("degraded_reason") != REASON_CRASH:
        problems.append(f"the reason came back as {status.get('degraded_reason')!r}, "
                        f"want {REASON_CRASH!r}")

    # An independent reader, not the product: if the product could read a file
    # the system SQLite calls corrupt, the read-back above would have been the
    # wrong witness.
    claim = subprocess.run(
        ["sqlite3", os.path.join(data_dir, "leases.db"), "PRAGMA integrity_check;"],
        capture_output=True, text=True, timeout=60,
    )
    if claim.returncode != 0:
        problems.append(f"the system sqlite3 could not read the store after the kill: "
                        f"{claim.stderr.strip()!r}")
    elif claim.stdout.strip() != "ok":
        problems.append(f"PRAGMA integrity_check after the kill = {claim.stdout.strip()!r}, want ok")

    # The restarted service still serves under the decision.
    restarted = Node(binary, "crash-restarted", cfg_path, tmp_root)
    try:
        found, out = wait_for(restarted, '"state":"primary-degraded"')
        if not found:
            problems.append("the restarted service did not come up degraded after the "
                            f"kill: {out[-600:]!r}")
    finally:
        restarted.stop()
    return problems


def case_storage_refuses(binary, tmp_root):
    """C. Two ways for storage to say no, and one invariant across both.

    The invariant is what matters: **a command may report success only if the
    permission is really in the store**. A storage layer that refuses the write
    must produce a refusal, never a printed success with nothing behind it --
    the same rule the DHCP reply path follows for a binding it could not commit.
    """
    problems = []
    data_dir = os.path.join(tmp_root, "storage", "data")
    os.makedirs(data_dir, exist_ok=True)
    cfg_path = os.path.join(tmp_root, "storage", "config.yaml")
    with open(cfg_path, "w") as fh:
        fh.write(config_yaml(data_dir, "primary", free_port(), free_port()))

    # Bring the store into existence the way a node does, then stop.
    node = Node(binary, "storage", cfg_path, tmp_root)
    ok, out = wait_for(node, "DHCP HA enabled")
    node.stop()
    if not ok:
        return [f"the node never reported its HA settings: {out[-600:]!r}"]

    db = os.path.join(data_dir, "leases.db")
    reason = "w14: storage refused this write"

    def attempt():
        return ha(binary, ["degrade", "-c", cfg_path, "--confirm", "--reason", reason], tmp_root)

    def observed(where, code, output, status):
        """The one invariant: reported success <=> the permission is really there."""
        if code == 0 and not status["degraded"]:
            problems.append(f"{where}: the command printed success but the permission is not "
                            f"in the store -- a refusal reported as a success")
        if code != 0 and status["degraded"]:
            problems.append(f"{where}: the command failed but the permission *is* in the "
                            f"store; the error does not describe what happened")

    # C1 -- the filesystem refuses. Every file SQLite needs to write for a WAL
    # commit is made read-only, including the log and its index: making only the
    # database file read-only would leave the WAL writable and prove nothing.
    #
    # The read-back happens after the permissions are restored, and that is not
    # just tidiness: a read-only open of a WAL database still needs write access
    # to recover the log, so `goddi ha status` cannot read the store at all
    # while the files are read-only. That the read fails there is the same fact
    # as the write failing, one layer up.
    targets = [db, db + "-wal", db + "-shm"]
    modes = {path: os.stat(path).st_mode for path in targets if os.path.exists(path)}
    dir_mode = os.stat(data_dir).st_mode
    for path in modes:
        os.chmod(path, 0o444)
    os.chmod(data_dir, 0o555)
    try:
        code, output = attempt()
    finally:
        os.chmod(data_dir, dir_mode)
        for path, mode in modes.items():
            os.chmod(path, mode)

    if code == 0:
        # Not a pass: the injection did not bite, so this sub-case observed
        # nothing. Saying so is the only honest option.
        problems.append("read-only storage: the write went through anyway, so this "
                        "sub-case proved nothing")
    elif not output.strip():
        problems.append("read-only storage: the command failed without saying anything")
    observed("read-only storage", code, output, ha_status(binary, cfg_path, tmp_root))

    # C2 -- the storage layer refuses this table. A BEFORE INSERT trigger on the
    # meta table is what a read-only replica, a full disk with a guard, or a
    # policy engine would look like from inside the process, and it names the
    # layer that said no.
    subprocess.run(
        ["sqlite3", db,
         "CREATE TRIGGER w14_refuse_meta BEFORE INSERT ON dataplane_meta "
         "BEGIN SELECT RAISE(ABORT, 'w14: storage refused this write'); END;"],
        capture_output=True, text=True, timeout=60, check=True,
    )
    try:
        code, output = attempt()
        if code == 0:
            problems.append("the storage layer refusing the table: the command still "
                            "reported success")
        elif "storage refused this write" not in output and "readonly" not in output.lower():
            problems.append(f"the refusal does not carry the storage layer's own words: "
                            f"{output.strip()[-300:]!r}")
        observed("the storage layer refusing the table", code, output,
                 ha_status(binary, cfg_path, tmp_root))

        # And the same command succeeds once storage works again, so the refusal
        # above was the storage layer and not the command being broken.
        subprocess.run(["sqlite3", db, "DROP TRIGGER w14_refuse_meta;"],
                       capture_output=True, text=True, timeout=60, check=True)
        code, output = attempt()
        status = ha_status(binary, cfg_path, tmp_root)
        if code != 0:
            problems.append(f"the command failed after storage recovered: {output.strip()[-300:]!r}")
        elif not status["degraded"]:
            problems.append("the permission is still not in the store after storage recovered")
    finally:
        subprocess.run(["sqlite3", db, "DROP TRIGGER IF EXISTS w14_refuse_meta;"],
                       capture_output=True, text=True, timeout=60, check=False)

    return problems


CAVEATS = """\
本脚本不能声称的（与上面的结论同样重要）

  * 真实掉电未验证。kill -9 结束的是进程而不是内核：写入方交给 write(2) 的
    每一个字节仍在页缓存里，恢复靠的是没有被 checkpoint 的 write-ahead log
    （本脚本已经断言它在崩溃后仍在且非空）。所以 B 节是**崩溃一致性**结果，
    不是耐久性结果。`synchronous=FULL` 唯一的机器证据是打开后的读回校验
    （internal/dataplane/store_test.go 的
    TestOpenAppliesTheDurabilitySettingsThatMakeAnAckHonest）；把它降到 0 之后
    本节的演练**照样通过**，这一点由 scripts/w14_durability_mutation_check.py
    作为「预期不被捕获」的变异显式记录。要真正验收「掉电不丢已 ACK 的绑定」，
    需要在真机上切电，本工作树做不到。

  * 客户端的 ACK 未在真实 UDP 会话上验证。本机非特权且 :67 已被占用，DHCP
    监听器绑不上（进程如实记录并继续），所以「租约写不进去就不发 ACK」这条
    只到存储层为止：语义由 internal/dhcp/server/lease_store_test.go 的
    TestNoAckWhenTheLeaseCannotBeWritten 与本工作包的
    TestAWriteTheStoreRefusesIsReportedAndLeavesNothing 钉住，没有一条报文
    在线上被拒绝过。

  * 本脚本写的是算子决策（`goddi ha degrade`）而不是租约。租约写入路径的
    kill -9 演练在 internal/dhcp/lease/durability_test.go，那里由真子进程
    真 SIGKILL 驱动；扫描此处的「已提交的写都还在」结论时，两者要一起读。\
"""


def main():
    binary = os.path.join(tempfile.gettempdir(), BIN_NAME)
    print("构建二进制…")
    build = subprocess.run(["go", "build", "-o", binary, "./cmd/goddi"], cwd=ROOT)
    if build.returncode != 0:
        print("FAIL: go build 失败")
        return 1

    tmp_root = tempfile.mkdtemp(prefix="goddi-w14-durability-")
    failures = []
    try:
        print("\n== A. 进程重启（SIGTERM，有序停机）==")
        for problem in case_graceful_restart(binary, tmp_root):
            failures.append(f"A 进程重启：{problem}")
            print(f"  FAIL {problem}")
        if not any(f.startswith("A ") for f in failures):
            print("  ok   已提交的算子决策与它的理由都活过了有序重启，重启后的服务按它运行")

        print("\n== B. 断电模拟（SIGKILL，与上面的重启分开记录）==")
        for problem in case_power_loss(binary, tmp_root):
            failures.append(f"B 断电：{problem}")
            print(f"  FAIL {problem}")
        if not any(f.startswith("B ") for f in failures):
            print("  ok   库在崩溃后可读且一致，已提交的决策与理由都在，write-ahead log 仍在")

        print("\n== C. 存储层注入（存储说不行时不得报成功）==")
        for problem in case_storage_refuses(binary, tmp_root):
            failures.append(f"C 存储层：{problem}")
            print(f"  FAIL {problem}")
        if not any(f.startswith("C ") for f in failures):
            print("  ok   两种存储拒绝都被如实报了出来，且没有一次「打印成功但库里没有」")
    finally:
        shutil.rmtree(tmp_root, ignore_errors=True)

    print("\n" + CAVEATS)

    if failures:
        print(f"\n{len(failures)} 项问题")
        return 1
    print("A / B / C 三节全部按上表所述成立（并请一并阅读上面的「不能声称的」）")
    return 0


if __name__ == "__main__":
    sys.exit(main())
