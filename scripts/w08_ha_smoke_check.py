#!/usr/bin/env python3
"""Process-level smoke check for the W08 DHCP HA pair.

Builds the real binary and starts it as the HA primary and as the HA standby,
then asserts on what the processes actually did. The four things checked here
are the ones a unit test cannot see, because each is decided by a line in
main() rather than by a function:

  1. A standby builds no DHCP server at all. Its leases are somebody else's and
     it must not answer a client. A unit test cannot tell "the standby refused"
     from "the standby was never started" -- the absence of the marker is the
     assertion.
  2. A primary with no mirror starts paused and says so. Starting in a state
     that permits promises would open a window on every restart in which a
     binding is acknowledged with nowhere to recover it from.
  3. Two real processes form a pair over the direct channel, and the primary
     returns to a non-serving state when the mirror goes away. The package
     tests cover this against real sockets; what they cannot cover is whether
     main() wires the two halves, which ports they end up on, and whether the
     fail-closed transition happens in the running binary.
  4. A configuration with HA switched on and no peer token is refused before
     anything is bound. This is the fail-closed rule of ADR 0003, and the
     message has to name the setting an operator must fix.

Case 3 starts a process that tries to bind udp4 0.0.0.0:67. The harness runs
unprivileged and this machine already has a DHCP service, so that bind fails and
the process logs it and carries on -- the same situation the W07 role smoke
check documents. Nothing here serves a client; the DHCP listener is not what is
under test, and every case below asserts on the HA channel, which lives on a
high port chosen by the script.

Run from the repository root:

    python3 scripts/w08_ha_smoke_check.py

Exit code 0 means both halves behaved as advertised.
"""

import os
import shutil
import socket
import subprocess
import sys
import tempfile
import time

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BIN_NAME = "goddi-w08-smoke"

TOKEN = "w08-smoke-shared-token-not-a-real-credential"

# Timings for the pair under test. They are short so the run is quick, and they
# keep the relationship the configuration validator enforces: a confirmation is
# given up on before the peer is declared stale.
CONFIRM_TIMEOUT = "500ms"
HEARTBEAT = "200ms"
PEER_STALE_AFTER = "1s"


def free_port():
    """Ask the kernel for a port nothing is using, then let it go.

    There is a race between closing this socket and the process under test
    binding the port, which is unavoidable without handing the socket over.
    Nothing else on the machine is competing for a high port in the second
    between the two, and a failure here shows up as a clear "nothing was
    listening" rather than as a wrong result.
    """
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def closed_port():
    """A port nothing is listening on, for the peer address of a lone node."""
    return free_port()


def config_yaml(role, data_dir, ha_role, listen_port, peer_port, token=TOKEN,
                enabled=True):
    on = lambda b: "true" if b else "false"
    lines = [
        "server:",
        '  name: "GoDDI"',
        f'  role: "{role}"',
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
        f"  enabled: {on(enabled)}",
        f'  node_id: "smoke-{ha_role}"',
        f'  role: "{ha_role}"',
        f'  listen_addr: "127.0.0.1:{listen_port}"',
        f'  peer_address: "127.0.0.1:{peer_port}"',
        f'  peer_token: "{token}"',
        f'  confirm_timeout: "{CONFIRM_TIMEOUT}"',
        f'  heartbeat_interval: "{HEARTBEAT}"',
        f'  peer_stale_after: "{PEER_STALE_AFTER}"',
        "security:",
        '  jwt_secret: "w08-smoke-secret-not-a-real-credential-0123456789"',
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


def wait_for(node, marker, timeout=15.0):
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


def listening(port):
    """Whether anything accepts a TCP connection on 127.0.0.1:port.

    A bare socket, not an HTTP client: a refusal here must mean "nothing is
    bound", and a local proxy would answer an HTTP request and make a dead port
    look busy.
    """
    try:
        with socket.create_connection(("127.0.0.1", port), timeout=2):
            return True
    except OSError:
        return False


def case_primary_alone(binary, tmp_root):
    """A primary with no mirror must start paused and say so."""
    failures = []
    data_dir = os.path.join(tmp_root, "primary-alone", "data")
    os.makedirs(data_dir, exist_ok=True)
    listen = free_port()
    cfg_path = os.path.join(tmp_root, "primary-alone", "config.yaml")
    with open(cfg_path, "w") as fh:
        fh.write(config_yaml("dhcp", data_dir, "primary", listen, closed_port()))

    node = Node(binary, "primary-alone", cfg_path, tmp_root)
    try:
        found, out = wait_for(node, "DHCP HA enabled")
        if not found:
            return [f"the process never reported the HA settings: {out[-600:]!r}"]

        for marker in ('"node_id":"smoke-primary"',
                       '"state":"paused"',
                       "this node is a primary"):
            if marker not in out:
                failures.append(f"missing: {marker!r}")

        # The peer channel is up even though the peer is not. A node that
        # silently failed to bind its listen address would look identical in
        # every other signal it emits.
        if not listening(listen):
            failures.append(f"nothing is listening on the HA port {listen}: "
                            "a primary that cannot accept its mirror can never "
                            "become redundant, and it would report paused forever "
                            "without saying why")
        if '"state":"primary"' in out:
            failures.append("the primary reported itself redundant with no mirror")
    finally:
        node.stop()
    return failures


def case_standby_does_not_serve(binary, tmp_root):
    """A standby must build no DHCP server, and must say why."""
    failures = []
    data_dir = os.path.join(tmp_root, "standby", "data")
    os.makedirs(data_dir, exist_ok=True)
    cfg_path = os.path.join(tmp_root, "standby", "config.yaml")
    with open(cfg_path, "w") as fh:
        fh.write(config_yaml("dhcp", data_dir, "standby", free_port(), closed_port()))

    node = Node(binary, "standby", cfg_path, tmp_root)
    try:
        found, out = wait_for(node, "this node is a standby")
        if not found:
            failures.append(f"the standby never reported its role: {out[-600:]!r}")
            return failures

        for marker in ('"state":"standby"',
                       "it will not serve clients",
                       "DHCP lease store opened"):
            if marker not in out:
                failures.append(f"missing: {marker!r}")

        # The load-bearing negative. A standby that built a DHCP server would
        # answer clients from a mirror it does not own, and the console would
        # have two authorities for one address.
        for marker in ("DHCP server initialized", "failed to start DHCP server"):
            if marker in out:
                failures.append(f"must not appear: {marker!r}")
    finally:
        node.stop()
    return failures


def case_pair_forms_and_fails_closed(binary, tmp_root):
    """Two real processes form a pair, and the primary stops promising when the
    mirror goes away."""
    failures = []
    primary_listen = free_port()
    standby_listen = free_port()

    primary_dir = os.path.join(tmp_root, "pair", "primary")
    standby_dir = os.path.join(tmp_root, "pair", "standby")
    os.makedirs(primary_dir, exist_ok=True)
    os.makedirs(standby_dir, exist_ok=True)

    primary_cfg = os.path.join(primary_dir, "config.yaml")
    with open(primary_cfg, "w") as fh:
        fh.write(config_yaml("dhcp", primary_dir, "primary", primary_listen,
                             standby_listen))
    standby_cfg = os.path.join(standby_dir, "config.yaml")
    with open(standby_cfg, "w") as fh:
        fh.write(config_yaml("dhcp", standby_dir, "standby", standby_listen,
                             primary_listen))

    primary = Node(binary, "pair-primary", primary_cfg, tmp_root)
    standby = None
    try:
        ok, out = wait_for(primary, "DHCP HA enabled")
        if not ok:
            return [f"the primary never reported the HA settings: {out[-600:]!r}"]

        standby = Node(binary, "pair-standby", standby_cfg, tmp_root)

        # The mirror dials its primary and is rebuilt from a snapshot. Both
        # halves of the exchange are asserted, because either one alone would
        # pass for a pair that never actually agreed on anything.
        ok, standby_out = wait_for(standby, "mirror rebuilt from a snapshot")
        if not ok:
            failures.append("the standby never rebuilt its mirror from a snapshot")
        for marker in ("mirror connected to its primary", '"applied_seq"'):
            if marker not in standby_out:
                failures.append(f"standby missing: {marker!r}")

        ok, out = wait_for(primary, "sent a snapshot to the mirror")
        if not ok:
            failures.append("the primary never sent a snapshot")
        for marker in ("mirror connected", '"peer_node_id":"smoke-standby"'):
            if marker not in out:
                failures.append(f"primary missing: {marker!r}")

        # The state flips to primary once the mirror has answered. This is the
        # only point at which the running binary would promise an address.
        ok, out = wait_for(primary, '"state":"primary"')
        if not ok:
            failures.append("the primary never reported itself redundant after its "
                            "mirror connected: no binding could ever be acknowledged")

        # And back the other way, which is the fail-closed rule seen from
        # outside: the mirror is killed outright, and the primary has to notice
        # and stop promising on its own.
        standby.stop(kill=True)
        standby = None
        ok, out = wait_for(primary, "the mirror session ended", timeout=15.0)
        if not ok:
            failures.append("the primary never noticed its mirror had gone: a "
                            "binding could be acknowledged with no second copy")
        else:
            # The line announcing the loss must not also claim redundancy. The
            # state is what an operator reads first, and "primary" next to "the
            # mirror session ended" is the exact reading this contract exists to
            # prevent.
            line = out.split("the mirror session ended")[-1].splitlines()[0]
            if '"state":"paused"' not in line:
                failures.append("the primary noticed the mirror had gone but the "
                                f"line still reports it able to promise: {line!r}")
    finally:
        if standby is not None:
            standby.stop()
        primary.stop()
    return failures


def case_refuses_to_start_without_a_peer(binary, tmp_root):
    """HA switched on with no peer token must fail before anything is bound."""
    failures = []
    data_dir = os.path.join(tmp_root, "refuse", "data")
    os.makedirs(data_dir, exist_ok=True)
    cfg_path = os.path.join(tmp_root, "refuse", "config.yaml")
    with open(cfg_path, "w") as fh:
        fh.write(config_yaml("dhcp", data_dir, "primary", free_port(),
                             closed_port(), token=""))

    done = subprocess.run(
        [binary, "serve", "-c", cfg_path],
        capture_output=True, text=True, cwd=tmp_root, timeout=60,
    )
    out = done.stdout + done.stderr
    if done.returncode == 0:
        failures.append("a node with HA on and no peer token started successfully")
    if "dhcp_ha.peer_token" not in out:
        failures.append(f"the refusal does not name the setting: {out[-600:]!r}")
    if "DHCP HA enabled" in out:
        failures.append("the process got as far as logging its HA settings before "
                        "refusing")
    return failures


CASES = [
    ("primary-alone", case_primary_alone,
     "a primary with no mirror starts paused and accepts a mirror"),
    ("standby", case_standby_does_not_serve,
     "a standby mirrors and builds no DHCP server"),
    ("pair", case_pair_forms_and_fails_closed,
     "two processes form a pair and the primary fails closed when it breaks"),
    ("refuse", case_refuses_to_start_without_a_peer,
     "a node with HA on and no peer is refused at startup"),
]


def main():
    tmp_root = tempfile.mkdtemp(prefix="goddi-w08-smoke-")
    binary = os.path.join(tmp_root, BIN_NAME)
    try:
        build = subprocess.run(
            ["go", "build", "-o", binary, "./cmd/goddi"],
            cwd=ROOT, capture_output=True, text=True,
        )
        if build.returncode != 0:
            print("BUILD FAILED")
            print(build.stdout)
            print(build.stderr)
            return 1

        bad = 0
        for name, fn, description in CASES:
            failures = fn(binary, tmp_root)
            status = "PASS" if not failures else "FAIL"
            print(f"[{status}] {name}: {description}")
            if failures:
                bad += 1
                for f in failures:
                    print(f"         {f}")

        print()
        if bad:
            print(f"{bad} of {len(CASES)} HA behaviours were not as advertised")
            return 1
        print(f"all {len(CASES)} HA behaviours held in the running binary")
        return 0
    finally:
        shutil.rmtree(tmp_root, ignore_errors=True)


if __name__ == "__main__":
    sys.exit(main())
