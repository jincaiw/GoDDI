#!/usr/bin/env python3
"""Role wiring smoke check for W07.

Builds the real binary and starts it once per role, then asserts on what the
process actually constructed and bound. The point is to catch the failure mode a
unit test cannot see: a data-plane process that still opens the management API,
or a control-plane process that still opens the DHCP listener.

The W07-b1 markers are the split seen from outside. A process that serves DHCP
must open a lease store of its own and report serving from it; a process that
only serves the console must not open one, and must say that the leases it shows
are a copy. Neither claim is visible from a unit test, because both are decided
by which handle main() hands to which component.

The W07-d part is a second, sharper version of the same idea: while the process
is up, it is asked what its endpoints say. A process that serves the management
API must answer /health 200, must refuse the detail endpoint without a token,
and must answer /ready consistently with its own startup log -- ok or degraded
when every listener came up, failing when one did not. A data-plane-only process
must not accept a connection on that port at all. Registering the probes is a
line in main(), and a unit test cannot see whether it ran.

(The harness runs unprivileged, so a role serving DHCP never binds udp4
0.0.0.0:67 and its readiness is legitimately failing. That is the answer being
cross-checked, not a tolerated failure -- see reconcile_readiness.)

Run from the repository root:

    python3 scripts/w07_role_smoke_check.py

Exit code 0 means every role behaved as advertised.
"""

import os
import shutil
import socket
import subprocess
import sys
import tempfile
import urllib.error
import urllib.request

import time

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BIN_NAME = "goddi-w07-smoke"

# Each case: role, whether it serves the management API, whether dns/dhcp are
# enabled in the config, and the log markers that must and must not appear. The
# logger writes compact JSON, so the markers have no space after the colon.
CASES = [
    {
        "role": "control",
        "http": True,
        "dns": False,
        "dhcp": False,
        "want": ['"role":"control"', '"management_api":true',
                 "HTTP server listening",
                 # The console has no lease store of its own, so it says the
                 # rows it shows are a copy and refuses to change them.
                 "lease view is a replica"],
        "not_want": ["management API not served by this process",
                     "DHCP server initialized",
                     "dns_server: started successfully",
                     "DHCP lease store opened",
                     "DNS data-plane store opened",
                     "dataplane:"],
    },
    {
        "role": "dns",
        "http": False,
        "dns": True,
        "dhcp": True,
        "want": ['"role":"dns"', '"management_api":false',
                 "management API not served by this process",
                 # The resolver serves from the store it owns, not from the
                 # control database. This is the W07-b2 assembly: a marker that
                 # the zone store was opened for this role and no other.
                 "DNS data-plane store opened",
                 "dns_server: started successfully"],
        "not_want": ["HTTP server listening", "DHCP server initialized",
                     "DHCP lease store opened", "lease view is a replica"],
    },
    {
        "role": "dhcp",
        "http": False,
        "dns": True,
        "dhcp": True,
        "want": ['"role":"dhcp"', '"management_api":false',
                 "management API not served by this process",
                 "DHCP server initialized",
                 # The data plane opens the store it owns and says which file
                 # the requests will be served from.
                 "DHCP lease store opened", '"lease_store"', "dataplane:"],
        "not_want": ["HTTP server listening",
                     "dns_server: started successfully",
                     "DNS data-plane store opened",
                     # No console here, so nothing to warn about a replica.
                     "lease view is a replica"],
    },
    {
        "role": "all",
        "http": True,
        "dns": True,
        "dhcp": True,
        "want": ['"role":"all"', '"management_api":true',
                 "HTTP server listening", "DHCP server initialized",
                 "dns_server: started successfully",
                 "DNS data-plane store opened",
                 "DHCP lease store opened", '"lease_store"', "dataplane:"],
        "not_want": ["management API not served by this process",
                     # Single process: the console holds the authoritative
                     # store, so the operator must not be told otherwise.
                     "lease view is a replica"],
    },
]


def config_yaml(role, data_dir, dns_on, dhcp_on, http_port, dns_port):
    # Built line by line on purpose: textwrap.dedent interacts badly with an
    # f-string whose first line follows a backslash continuation, and the
    # result is a config file that silently fails to parse.
    on = lambda b: "true" if b else "false"
    lines = [
        "server:",
        '  name: "GoDDI"',
        f'  role: "{role}"',
        f'  http_addr: "127.0.0.1:{http_port}"',
        f'  data_dir: "{data_dir}"',
        "database:",
        '  driver: "sqlite"',
        f'  dsn: "{data_dir}/goddi.db"',
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
        '  jwt_secret: "w07-smoke-secret-not-a-real-credential-0123456789"',
        "log:",
        '  level: "info"',
    ]
    return "\n".join(lines) + "\n"


def http_get(port, path, timeout=5):
    """GET a path from the process under test.

    Returns (status, body); the status is None when nothing answered at all,
    which for a data-plane role is the expected outcome rather than an error.
    """
    url = f"http://127.0.0.1:{port}{path}"
    try:
        with urllib.request.urlopen(url, timeout=timeout) as resp:
            return resp.status, resp.read().decode("utf-8", "replace")
    except urllib.error.HTTPError as exc:
        return exc.code, exc.read().decode("utf-8", "replace")
    except Exception as exc:  # connection refused, timeout, reset
        return None, str(exc)


def compact(body):
    """Strip whitespace so a JSON body can be matched without guessing it."""
    return "".join(body.split())


def probe_ports(port, case):
    """Ask the running process what the W07-d endpoints say.

    Two of the decisions this batch made are only visible from outside the
    process, so a unit test cannot hold them: whether the probes were actually
    registered by main(), and whether a data-plane role binds an HTTP socket at
    all. The second is the one that keeps "the console is being restarted" and
    "clients cannot get an address" two different events.

    Returns (failures, observed) where observed is the (status, body) of /ready,
    or None for a role that serves no management API. What that answer should be
    is decided later, against the startup log -- see reconcile_readiness.
    """
    failures = []

    if not case.get("http"):
        try:
            with socket.create_connection(("127.0.0.1", port), timeout=2):
                failures.append(
                    f"a {case['role']} process accepted a connection on port {port}, "
                    "but a data-plane role must not run a management API")
        except OSError:
            pass  # nothing listening, which is the point
        return failures, None

    status, body = http_get(port, "/health")
    if status != 200:
        failures.append(f"GET /health = {status} ({body!r}), want 200: liveness "
                        "answers whether the process runs, not whether it is happy")
    elif '"status":"ok"' not in compact(body):
        failures.append(f"/health body = {body!r}, want status ok")

    observed = http_get(port, "/ready")

    # The unauthenticated probe must serve the level and nothing else; the
    # figures live behind authentication.
    status, body = http_get(port, "/api/v1/system/dataplane")
    if status != 401:
        failures.append(f"GET /api/v1/system/dataplane without a token = {status}, "
                        f"want 401 (body {body!r})")
    return failures, observed


# What a process says when a listener it was told to run never came up.
START_FAILURES = ("failed to start DNS server", "failed to start DHCP server")


def reconcile_readiness(observed, out):
    """Require the readiness answer to agree with the startup log.

    This is deliberately not "the probe must be green". A process that could not
    bind a listener it was told to run is not able to do its job, and the point
    of the tiered probe is that it says so. In this harness that is the normal
    case for a role serving DHCP: binding udp4 0.0.0.0:67 needs privileges a
    test run does not have, so the listener fails and the answer has to be
    failing -- which is also why the marker lists in CASES never asserted on it
    before. Checking the answer against the log is what makes this a real
    assertion either way: a process whose listeners all came up must be ok or
    degraded, and one whose listener did not must be failing.
    """
    if observed is None:
        return []

    status, body = observed
    body = compact(body)
    not_started = [m for m in START_FAILURES if m in out]

    if not_started:
        if status != 503:
            return [f"GET /ready = {status} ({body!r}) while the log says "
                    f"{not_started[0]!r}: a listener that never came up is failing, "
                    "and failing is the only level that answers 503"]
        if '"status":"failing"' not in body:
            return [f"/ready body = {body!r} while the log says {not_started[0]!r}, "
                    "want status failing"]
        return []

    if status != 200:
        return [f"GET /ready = {status} ({body!r}), want 200: every listener came up "
                f"and at least one probe is registered ({out.count('HTTP server')} http)"]
    if '"status":"ok"' not in body and '"status":"degraded"' not in body:
        return [f"/ready body = {body!r}, want ok or degraded"]
    return []


def run_case(binary, case, tmp_root):
    role = case["role"]
    http_port = 18100 + CASES.index(case)
    dns_port = 15300 + CASES.index(case)
    data_dir = os.path.join(tmp_root, role, "data")
    os.makedirs(data_dir, exist_ok=True)
    cfg_path = os.path.join(tmp_root, role, "config.yaml")
    with open(cfg_path, "w") as fh:
        fh.write(config_yaml(role, data_dir, case["dns"], case["dhcp"],
                             http_port, dns_port))

    proc = subprocess.Popen(
        [binary, "serve", "-c", cfg_path],
        stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
        cwd=tmp_root, text=True,
    )
    # Let it get through startup, then ask what its endpoints say while it is
    # still up. Asking after the signal would only measure the shutdown path.
    time.sleep(4)
    failures, observed = probe_ports(http_port, case)
    proc.terminate()
    try:
        out, _ = proc.communicate(timeout=20)
    except subprocess.TimeoutExpired:
        proc.kill()
        out, _ = proc.communicate()

    failures += reconcile_readiness(observed, out)

    for marker in case["want"]:
        if marker not in out:
            failures.append(f"missing: {marker!r}")
    for marker in case["not_want"]:
        if marker in out:
            failures.append(f"must not appear: {marker!r}")

    # A process that died on its own never tested anything.
    if "GoDDI stopped gracefully" not in out and proc.returncode not in (0, -15):
        failures.append(f"process exited with {proc.returncode}")

    return failures, out


def main():
    tmp_root = tempfile.mkdtemp(prefix="goddi-w07-smoke-")
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
        for case in CASES:
            failures, out = run_case(binary, case, tmp_root)
            status = "PASS" if not failures else "FAIL"
            print(f"[{status}] role={case['role']}")
            if failures:
                bad += 1
                for f in failures:
                    print(f"         {f}")
                # Only the lines that decide the outcome: the whole startup log
                # is a few hundred lines and buries the reason.
                for line in out.splitlines():
                    if any(k in line for k in (
                            '"role"', "management_api", "listening",
                            "server initialized", "dns_server: started",
                            "degraded mode", "not served by this process",
                            "lease", "dataplane")):
                        print(f"         | {line[:220]}")

        print()
        if bad:
            print(f"{bad} of {len(CASES)} roles behaved unexpectedly")
            return 1
        print(f"all {len(CASES)} roles started the components they advertise")
        return 0
    finally:
        shutil.rmtree(tmp_root, ignore_errors=True)


if __name__ == "__main__":
    sys.exit(main())
