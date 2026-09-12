#!/usr/bin/env python3
"""Process-level smoke check for the W09 DHCP HA operator actions.

Builds the real binary and drives it the way an operator would: start the
service, run `goddi ha`, read what the service did about it. The unit tests
cover the mechanism; what they cannot see is whether main() wires it up, and
every case here is a line in main() rather than a function:

  1. The read-only report creates nothing. It stats the lease store before it
     opens anything, because opening a SQLite path creates the file -- so a
     report run against a node that has never served would describe a database
     it had just brought into existence.
  2. `goddi ha degrade` is adopted by the process that is already running. A
     permission that only took effect at the next restart would be a permission
     to restart the process, which is the outage it exists to avoid.
  3. A takeover is refused against a primary that is demonstrably answering.
     This is the one refusal with no way past it, and the process under test is
     what makes the primary answer.
  4. A fence and a rejoin round-trip through the store and are visible from a
     second process.
  5. A fenced node neither serves nor mirrors: the two halves of what it would
     otherwise build are absent from its log.
  6. A standby that is taken over starts as a serving primary. This is the case
     the whole work package exists for, and it is decided entirely by main()
     choosing the role in force over the role in the configuration file.
  7. The shortfall a takeover has to be told is the one the report gave. The
     figure is read out of `goddi ha status --json` and handed straight back;
     a neighbouring figure is refused and leaves the role alone, and the figure
     that was reported is accepted and survives the promotion that clears the
     watermarks it came from.

Like the W08 check, this harness runs unprivileged on a machine that already
has a DHCP service, so port 67 cannot be bound and the process logs that and
carries on. Nothing here serves a client; the assertions are about the role the
process decided to run as, which is visible in its log.

Run from the repository root:

    python3 scripts/w09_ha_operator_smoke_check.py

Exit code 0 means every action behaved as advertised.
"""

import json
import os
import shutil
import socket
import subprocess
import sys
import tempfile
import time
from datetime import datetime, timedelta, timezone

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BIN_NAME = "goddi-w09-smoke"

TOKEN = "w09-smoke-shared-token-not-a-real-credential"

CONFIRM_TIMEOUT = "500ms"
HEARTBEAT = "200ms"
PEER_STALE_AFTER = "1s"

# The takeover gate waits three staleness bounds of silence. The script has to
# outwait it, so the two have to agree; the relationship is asserted rather
# than assumed in case_takeover_over_a_dead_primary.
TAKEOVER_QUIET_MULTIPLE = 3
PEER_STALE_SECONDS = 1.0


def free_port():
    """Ask the kernel for a port nothing is using, then let it go."""
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


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
        '  jwt_secret: "w09-smoke-secret-not-a-real-credential-0123456789"',
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


def ha(binary, args, cwd):
    """Run `goddi ha ...` and return (returncode, stdout, stderr)."""
    done = subprocess.run(
        [binary, "ha"] + args, capture_output=True, text=True, cwd=cwd,
        timeout=60,
    )
    return done.returncode, done.stdout, done.stderr


def ha_status(binary, cfg_path, cwd):
    """Run the machine-readable status, or raise with what it said instead."""
    code, out, err = ha(binary, ["status", "-c", cfg_path, "--json"], cwd)
    if code != 0:
        raise AssertionError(f"goddi ha status exited {code}: {out}{err}")
    try:
        return json.loads(out)
    except json.JSONDecodeError as e:
        raise AssertionError(f"goddi ha status did not print JSON ({e}): {out!r}")


def case_status_creates_nothing(binary, tmp_root):
    """The read-only report must not bring the database into existence."""
    failures = []
    data_dir = os.path.join(tmp_root, "never-ran", "data")
    cfg_path = os.path.join(tmp_root, "never-ran", "config.yaml")
    os.makedirs(os.path.dirname(cfg_path), exist_ok=True)
    with open(cfg_path, "w") as fh:
        fh.write(config_yaml("dhcp", data_dir, "primary", free_port(), free_port()))

    code, out, err = ha(binary, ["status", "-c", cfg_path], tmp_root)
    text = out + err
    if code == 0:
        failures.append("goddi ha status succeeded on a node that has never served DHCP")
    if "never opened its DHCP lease store" not in text:
        failures.append(f"the refusal does not explain itself: {text[-400:]!r}")
    # The load-bearing assertion: opening a SQLite path creates the file, so a
    # report that opened first and explained later would leave a database behind
    # and describe it as empty.
    if os.path.exists(data_dir):
        failures.append(f"the report created {data_dir}, which did not exist before it ran")
    return failures


def case_degrade_is_adopted_live(binary, tmp_root):
    """A permission granted from a second process reaches the running one."""
    failures = []
    data_dir = os.path.join(tmp_root, "degrade", "data")
    os.makedirs(data_dir, exist_ok=True)
    cfg_path = os.path.join(tmp_root, "degrade", "config.yaml")
    with open(cfg_path, "w") as fh:
        fh.write(config_yaml("dhcp", data_dir, "primary", free_port(), free_port()))

    node = Node(binary, "degrade", cfg_path, tmp_root)
    try:
        ok, out = wait_for(node, "DHCP HA enabled")
        if not ok:
            return [f"the process never reported the HA settings: {out[-600:]!r}"]
        if '"state":"paused"' not in out:
            failures.append("the primary did not start paused with no mirror")

        status = ha_status(binary, cfg_path, tmp_root)
        if status["role"] != "primary" or status["state"] != "primary":
            failures.append(f"status reports role={status['role']} state={status['state']}, "
                            "want primary/primary")
        # The one reading a snapshot must not invent. Redundancy is the peer
        # link, the link is not in the store, and a report that answered "yes"
        # here would be the answer nobody checked.
        if status["redundancy_known"]:
            failures.append("status claimed to know whether this node is redundant; "
                            "only the running process observes the peer link")
        if status["degraded"]:
            failures.append("status reports a permission nobody granted")

        # Without the confirmation. The gate is in the mechanism, so this is a
        # refusal rather than a flag parser saying no.
        code, out, err = ha(binary, ["degrade", "-c", cfg_path], tmp_root)
        if code == 0:
            failures.append("goddi ha degrade granted a permission without --confirm")
        elif "explicit confirmation" not in (out + err):
            failures.append(f"the refusal does not say what is missing: {(out + err)[-300:]!r}")

        code, out, err = ha(binary, ["degrade", "-c", cfg_path, "--confirm",
                                     "--reason", "w09 smoke"], tmp_root)
        if code != 0:
            failures.append(f"goddi ha degrade --confirm exited {code}: {out}{err}")
        else:
            # The assertion this case exists for.
            found, out = wait_for(node, "an operator has approved running without a second copy")
            if not found:
                failures.append("the running service never adopted the permission: "
                                f"{out[-600:]!r}")

        status = ha_status(binary, cfg_path, tmp_root)
        if status["state"] != "primary-degraded" or not status["degraded"]:
            failures.append(f"status after the permission = {status['state']}, "
                            f"degraded={status['degraded']}, want primary-degraded/true")
        # Here the store does settle it: the absence of a second copy is what
        # the permission says.
        if not status["redundancy_known"] or status["redundant"]:
            failures.append(f"a degraded node reported redundant={status['redundant']} "
                            f"known={status['redundancy_known']}, want false/true")

        code, out, err = ha(binary, ["degrade", "-c", cfg_path, "--confirm", "--undo"], tmp_root)
        if code != 0:
            failures.append(f"goddi ha degrade --undo exited {code}: {out}{err}")
        else:
            found, out = wait_for(node, "the single-copy approval was withdrawn")
            if not found:
                failures.append("withdrawing the permission never reached the running "
                                f"service: {out[-600:]!r}")
        status = ha_status(binary, cfg_path, tmp_root)
        if status["state"] != "primary" or status["redundancy_known"]:
            failures.append(f"status after the withdrawal = {status['state']} "
                            f"(known={status['redundancy_known']}), want primary/unknown")
    finally:
        node.stop()
    return failures


def case_takeover_is_refused_against_a_live_primary(binary, tmp_root):
    """The refusal an operator cannot argue their way past."""
    failures = []
    primary_listen = free_port()
    standby_listen = free_port()
    primary_dir = os.path.join(tmp_root, "live", "primary")
    standby_dir = os.path.join(tmp_root, "live", "standby")
    os.makedirs(primary_dir, exist_ok=True)
    os.makedirs(standby_dir, exist_ok=True)

    primary_cfg = os.path.join(primary_dir, "config.yaml")
    with open(primary_cfg, "w") as fh:
        fh.write(config_yaml("dhcp", primary_dir, "primary", primary_listen, standby_listen))
    standby_cfg = os.path.join(standby_dir, "config.yaml")
    with open(standby_cfg, "w") as fh:
        fh.write(config_yaml("dhcp", standby_dir, "standby", standby_listen, primary_listen))

    primary = Node(binary, "live-primary", primary_cfg, tmp_root)
    standby = None
    try:
        ok, out = wait_for(primary, "DHCP HA enabled")
        if not ok:
            return [f"the primary never reported the HA settings: {out[-600:]!r}"]
        standby = Node(binary, "live-standby", standby_cfg, tmp_root)
        ok, out = wait_for(standby, "mirror rebuilt from a snapshot")
        if not ok:
            return [f"the pair never formed: {out[-600:]!r}"]

        status = ha_status(binary, standby_cfg, tmp_root)
        if status["role"] != "standby" or status["state"] != "standby":
            failures.append(f"the standby reports role={status['role']} state={status['state']}")
        if status["redundancy_known"]:
            failures.append("a standby claimed to know whether the pair is redundant; that "
                            "is the primary's fact to report")

        # The fence statement is checked first, so its absence is what comes
        # back even while the primary is alive.
        code, out, err = ha(binary, ["takeover", "-c", standby_cfg, "--confirm"], tmp_root)
        if code == 0:
            failures.append("a takeover was allowed with no statement about the old primary")
        elif "stopped or fenced" not in (out + err):
            failures.append(f"the refusal does not name what is missing: {(out + err)[-300:]!r}")

        code, out, err = ha(binary, ["takeover", "-c", standby_cfg, "--confirm",
                                     "--old-primary-cannot-write"], tmp_root)
        if code == 0:
            failures.append("a takeover of a live primary was allowed")
        elif "still answering" not in (out + err):
            failures.append(f"the refusal does not say why: {(out + err)[-300:]!r}")

        status = ha_status(binary, standby_cfg, tmp_root)
        if status["role"] != "standby":
            failures.append(f"a refused takeover changed the role to {status['role']}")
        if not status["peer_seq_at"]:
            failures.append("status does not record when the primary was last heard from")
    finally:
        if standby is not None:
            standby.stop()
        primary.stop()
    return failures


def case_fence_and_rejoin(binary, tmp_root):
    """The two role changes round-trip through the store."""
    failures = []
    data_dir = os.path.join(tmp_root, "fence", "data")
    os.makedirs(data_dir, exist_ok=True)
    cfg_path = os.path.join(tmp_root, "fence", "config.yaml")
    with open(cfg_path, "w") as fh:
        fh.write(config_yaml("dhcp", data_dir, "primary", free_port(), free_port()))

    # The store has to exist before any of these can act on it, and the only
    # thing that creates it is the service having run.
    node = Node(binary, "fence-seed", cfg_path, tmp_root)
    ok, out = wait_for(node, "DHCP HA enabled")
    node.stop()
    if not ok:
        return [f"the process never reported the HA settings: {out[-600:]!r}"]

    code, out, err = ha(binary, ["fence", "-c", cfg_path], tmp_root)
    if code == 0:
        failures.append("goddi ha fence took a node out of service without --confirm")
    elif "explicit confirmation" not in (out + err):
        failures.append(f"the refusal does not say what is missing: {(out + err)[-300:]!r}")

    code, out, err = ha(binary, ["fence", "-c", cfg_path, "--confirm",
                                 "--reason", "w09 smoke"], tmp_root)
    if code != 0:
        failures.append(f"goddi ha fence --confirm exited {code}: {out}{err}")
    status = ha_status(binary, cfg_path, tmp_root)
    if status["role"] != "fenced" or status["state"] != "fenced":
        failures.append(f"after the fence status = {status['role']}/{status['state']}, "
                        "want fenced/fenced")
    if status["configured_role"] != "primary":
        failures.append(f"the configuration role was rewritten to {status['configured_role']}; "
                        "the file is the operator's, the store is the record")
    if not status["redundancy_known"] or status["redundant"]:
        failures.append(f"a fenced node reported redundant={status['redundant']} "
                        f"known={status['redundancy_known']}, want false/true")

    # The role takes effect at the next start, and the next start must be a node
    # that serves nothing and mirrors nothing.
    fenced = Node(binary, "fence-fenced", cfg_path, tmp_root)
    try:
        ok, out = wait_for(fenced, "this node is fenced")
        if not ok:
            failures.append(f"the fenced node did not report its state: {out[-600:]!r}")
        for marker in ("DHCP server initialized", "mirror connected to its primary"):
            if marker in out:
                failures.append(f"a fenced node did: {marker!r}")
        if "DHCP HA: this node is a primary" in out:
            failures.append("a fenced node came up as a primary despite the recorded role")
    finally:
        fenced.stop()

    code, out, err = ha(binary, ["rejoin", "-c", cfg_path, "--confirm"], tmp_root)
    if code != 0:
        failures.append(f"goddi ha rejoin --confirm exited {code}: {out}{err}")
    status = ha_status(binary, cfg_path, tmp_root)
    if status["role"] != "standby":
        failures.append(f"after the rejoin the role is {status['role']}, want standby")
    return failures


def case_takeover_over_a_dead_primary(binary, tmp_root):
    """A standby that is taken over starts as a primary that serves."""
    failures = []
    primary_listen = free_port()
    standby_listen = free_port()
    primary_dir = os.path.join(tmp_root, "gone", "primary")
    standby_dir = os.path.join(tmp_root, "gone", "standby")
    os.makedirs(primary_dir, exist_ok=True)
    os.makedirs(standby_dir, exist_ok=True)

    primary_cfg = os.path.join(primary_dir, "config.yaml")
    with open(primary_cfg, "w") as fh:
        fh.write(config_yaml("dhcp", primary_dir, "primary", primary_listen, standby_listen))
    standby_cfg = os.path.join(standby_dir, "config.yaml")
    with open(standby_cfg, "w") as fh:
        fh.write(config_yaml("dhcp", standby_dir, "standby", standby_listen, primary_listen))

    primary = Node(binary, "gone-primary", primary_cfg, tmp_root)
    standby = None
    restarted = None
    try:
        ok, out = wait_for(primary, "DHCP HA enabled")
        if not ok:
            return [f"the primary never reported the HA settings: {out[-600:]!r}"]
        standby = Node(binary, "gone-standby", standby_cfg, tmp_root)
        ok, out = wait_for(standby, "mirror rebuilt from a snapshot")
        if not ok:
            return [f"the pair never formed: {out[-600:]!r}"]

        # The primary is killed outright. The standby reconnects forever; what
        # stops is the moment the peer was last heard from, and that is what the
        # quiet window is measured against.
        primary.stop(kill=True)
        ok, out = wait_for(standby, "the mirror lost its primary")
        if not ok:
            failures.append("the standby never noticed its primary had gone")

        # The gate waits three staleness bounds of silence. Waiting less would
        # test the gate's absence rather than its width, so the wait is derived
        # from the constant the mechanism uses.
        time.sleep(TAKEOVER_QUIET_MULTIPLE * PEER_STALE_SECONDS + 1.0)

        code, out, err = ha(binary, ["takeover", "-c", standby_cfg, "--confirm",
                                     "--old-primary-cannot-write"], tmp_root)
        if code != 0:
            return failures + [f"goddi ha takeover exited {code} after the primary died: "
                               f"{out}{err}"]

        status = ha_status(binary, standby_cfg, tmp_root)
        if status["role"] != "primary":
            failures.append(f"after the takeover the role is {status['role']}, want primary")
        if status["state"] != "primary-degraded" or not status["degraded"]:
            failures.append(f"after the takeover the state is {status['state']} "
                            f"(degraded={status['degraded']}); a promotion comes with the "
                            "permission to serve alone, or the node would withhold everything")
        if not status["redundancy_known"] or status["redundant"]:
            failures.append(f"a promoted node reported redundant={status['redundant']} "
                            f"known={status['redundancy_known']}, want false/true")

        # The point of the whole work package: the promoted node now serves.
        restarted = Node(binary, "gone-promoted", standby_cfg, tmp_root)
        ok, out = wait_for(restarted, "DHCP HA: this node is a primary")
        if not ok:
            failures.append(f"the promoted node did not come up as a primary: {out[-600:]!r}")
        else:
            for marker in ('"state":"primary-degraded"', "DHCP server initialized"):
                if marker not in out:
                    failures.append(f"the promoted node's log is missing: {marker!r}")
            if "this node is a standby" in out:
                failures.append("the promoted node came up as a standby: the role in force "
                                "was not read back from the store")
    finally:
        if restarted is not None:
            restarted.stop()
        if standby is not None:
            standby.stop()
        primary.stop()
    return failures


def stage_shortfall(data_dir, applied, peer, heard_ago_seconds):
    """Put a standby's store into the state a partition leaves behind.

    The two watermarks a takeover is decided on are written directly, because
    the state they describe cannot be reached from outside the process: the
    applied watermark advances only when the mirror applies a row, and the
    peer's counter only ever arrives over a link that is up. The case where
    they disagree is the one where a primary got ahead of a link that then
    went away -- and no client traffic here can produce it, because a DHCP
    binding needs a real REQUEST (see the note at the top of this file).

    Everything else in the case is real: the numbers are read back from this
    binary's own report, and the refusal and the promotion are the ones the
    service performs.
    """
    path = os.path.join(data_dir, "leases.db")
    heard = (datetime.now(timezone.utc) - timedelta(seconds=heard_ago_seconds))
    heard = heard.isoformat(timespec="milliseconds").replace("+00:00", "Z")
    writes = {
        "ha_applied_seq": applied,
        "ha_peer_seq": peer,
        "ha_peer_seq_at": heard,
    }
    statement = ";".join(
        "INSERT INTO dataplane_meta (key, value) VALUES ('%s', '%s') "
        "ON CONFLICT(key) DO UPDATE SET value = excluded.value" % (k, v)
        for k, v in writes.items()
    )
    done = subprocess.run(["sqlite3", path, statement], capture_output=True, text=True)
    if done.returncode != 0:
        raise AssertionError(f"staging the shortfall in {path} failed: {done.stderr.strip()}")


def case_takeover_accepts_only_the_shortfall_it_reported(binary, tmp_root):
    """The number a takeover needs is the one the tool itself reports.

    A takeover is the one action that has to be told how much is being given
    up, and the figure is only correct for the moment it was read: the peer
    keeps counting for as long as it is reachable, and the applied watermark
    keeps moving for as long as the link is up. So the case reads the figure
    out of `goddi ha status --json` and hands that value back, and it checks
    the two things a guess would get wrong -- a neighbouring number is
    refused, and the figure that was reported is the one that is accepted.
    """
    failures = []
    primary_listen = free_port()
    standby_listen = free_port()
    primary_dir = os.path.join(tmp_root, "shortfall", "primary")
    standby_dir = os.path.join(tmp_root, "shortfall", "standby")
    os.makedirs(primary_dir, exist_ok=True)
    os.makedirs(standby_dir, exist_ok=True)

    primary_cfg = os.path.join(primary_dir, "config.yaml")
    with open(primary_cfg, "w") as fh:
        fh.write(config_yaml("dhcp", primary_dir, "primary", primary_listen, standby_listen))
    standby_cfg = os.path.join(standby_dir, "config.yaml")
    with open(standby_cfg, "w") as fh:
        fh.write(config_yaml("dhcp", standby_dir, "standby", standby_listen, primary_listen))

    primary = Node(binary, "shortfall-primary", primary_cfg, tmp_root)
    standby = None
    try:
        ok, out = wait_for(primary, "DHCP HA enabled")
        if not ok:
            return [f"the primary never reported the HA settings: {out[-600:]!r}"]
        standby = Node(binary, "shortfall-standby", standby_cfg, tmp_root)
        ok, out = wait_for(standby, "mirror rebuilt from a snapshot")
        if not ok:
            return [f"the pair never formed: {out[-600:]!r}"]

        # First half: on a live pair the report is populated from the link,
        # not from a file. The counter is zero -- nothing has bound a lease,
        # so nothing has been handed out -- which is what makes the other half
        # of the case necessary rather than redundant.
        live = ha_status(binary, standby_cfg, tmp_root)
        if live["role"] != "standby" or live["state"] != "standby":
            failures.append(f"a mirror of a live primary reports role={live['role']} "
                            f"state={live['state']}, want standby/standby")
        if live["gap"] != 0 or live["applied_seq"] != 0:
            failures.append(f"before any lease was handed out the mirror reports "
                            f"applied={live['applied_seq']} shortfall={live['gap']}, want 0/0")
        young = live["peer_seq_at"]
        if not young:
            failures.append("the mirror has no record of when it last heard its primary, "
                            "so nothing here can be reconciled")
        else:
            heard = datetime.fromisoformat(young.replace("Z", "+00:00"))
            age = (datetime.now(timezone.utc) - heard).total_seconds()
            if age > 10:
                failures.append(f"the mirror last heard its primary {age:.1f}s ago while the "
                                "pair is up: the report is not tracking the link")

        # The pair is taken apart, and the shortfall is staged. The primary is
        # stopped as well, so that what the standby's store says about it is
        # the last thing it ever said.
        standby.stop()
        standby = None
        primary.stop()

        stage_shortfall(standby_dir, applied="7", peer="10", heard_ago_seconds=3600)

        reported = ha_status(binary, standby_cfg, tmp_root)
        if reported["gap"] != 3 or reported["applied_seq"] != 7 or reported["peer_seq"] != 10:
            return failures + [f"the staged shortfall reads back as applied={reported['applied_seq']} "
                               f"peer={reported['peer_seq']} gap={reported['gap']}, want 7/10/3"]
        if reported["accepted_gap"] != 0:
            failures.append(f"a node that was never promoted reports an accepted gap of "
                            f"{reported['accepted_gap']}")

        required = reported["gap"]

        # A number next to the right one is refused. This is the difference
        # between a check and a formality: an operator reading a stale report
        # would type a figure that is plausible and wrong.
        code, out, err = ha(binary, ["takeover", "-c", standby_cfg, "--confirm",
                                     "--old-primary-cannot-write",
                                     "--accept-gap", str(required - 1)], tmp_root)
        if code == 0:
            failures.append(f"a takeover accepted --accept-gap {required - 1} against a "
                            f"shortfall of {required}")
        text = out + err
        if f"--accept-gap {required}" not in text:
            failures.append(f"the refusal does not name the figure to pass: {text[-400:]!r}")
        still = ha_status(binary, standby_cfg, tmp_root)
        if still["role"] != "standby":
            failures.append(f"a refused takeover left the role at {still['role']}: the node "
                            "would serve without having been promoted")

        # And the figure that was reported is accepted. It is passed as the
        # value that came out of the report above rather than as a literal.
        code, out, err = ha(binary, ["takeover", "-c", standby_cfg, "--confirm",
                                     "--old-primary-cannot-write",
                                     "--accept-gap", str(required)], tmp_root)
        if code != 0:
            return failures + [f"the takeover was refused with the figure it reported "
                               f"({required}): {out}{err}"]

        after = ha_status(binary, standby_cfg, tmp_root)
        if after["role"] != "primary":
            failures.append(f"after the takeover the role is {after['role']}, want primary")
        # What was given up survives the promotion, which is the half of the
        # record the promotion itself destroys: it clears the applied
        # watermark, and with it the subtraction that produced the figure.
        if after["accepted_gap"] != required:
            failures.append(f"the promoted node reports an accepted gap of "
                            f"{after['accepted_gap']}, want {required}")
        # And the node stops claiming a shortfall it can no longer have: a
        # primary mirrors nobody, so the taken-over node's counter is not its
        # missing set.
        if after["gap"] != 0:
            failures.append(f"the promoted primary reports a shortfall of {after['gap']} "
                            "against the node it replaced")
        if after["applied_seq"] != 0:
            failures.append(f"the promoted node kept an applied watermark of "
                            f"{after['applied_seq']}; a figure like that satisfies a "
                            "confirmation wait on the spot")
    finally:
        if standby is not None:
            standby.stop()
        primary.stop()
    return failures


CASES = [
    ("status-creates-nothing", case_status_creates_nothing,
     "the read-only report does not bring the lease store into existence"),
    ("degrade-is-adopted-live", case_degrade_is_adopted_live,
     "a permission granted from a second process reaches the running service"),
    ("takeover-refused-while-live", case_takeover_is_refused_against_a_live_primary,
     "a takeover of a primary that is still answering is refused"),
    ("fence-and-rejoin", case_fence_and_rejoin,
     "a fence stops the node and a rejoin returns it as a mirror"),
    ("takeover-over-a-dead-primary", case_takeover_over_a_dead_primary,
     "a taken-over standby starts as a serving primary"),
    ("takeover-accepts-only-its-own-figure",
     case_takeover_accepts_only_the_shortfall_it_reported,
     "a takeover is refused for any figure but the one the report gave"),
]


def main():
    # One case can be run on its own, which is what the mutation check needs:
    # it drives this script with a defect injected and has to see the case that
    # covers that defect fail.
    only = None
    for arg in sys.argv[1:]:
        if arg.startswith("--only="):
            only = arg.split("=", 1)[1]
    if only is not None and only not in [name for name, _, _ in CASES]:
        print(f"no such case: {only}")
        return 2

    tmp_root = tempfile.mkdtemp(prefix="goddi-w09-smoke-")
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

        selected = [c for c in CASES if only is None or c[0] == only]
        bad = 0
        for name, fn, description in selected:
            failures = fn(binary, tmp_root)
            status = "PASS" if not failures else "FAIL"
            print(f"[{status}] {name}: {description}")
            if failures:
                bad += 1
                for f in failures:
                    print(f"         {f}")

        print()
        if bad:
            print(f"{bad} of {len(selected)} operator behaviours were not as advertised")
            return 1
        print(f"all {len(selected)} operator behaviours held in the running binary")
        return 0
    finally:
        shutil.rmtree(tmp_root, ignore_errors=True)


if __name__ == "__main__":
    sys.exit(main())
