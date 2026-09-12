#!/usr/bin/env python3
"""Process-level smoke check for the B5 management surface (W10-c, W11).

Starts the real binary and asks it, from outside, what its management surface
does. Three decisions this batch made cannot be seen from a unit test, because
all three are made in main() and in the router's wiring rather than in a handler:

  * the management allowlist is applied to /api/v1/**, and to /metrics as part of
    the same surface -- so an address outside the list is refused there;
  * /health and /ready are registered ABOVE the allowlist on purpose: they belong
    to the orchestrator, and an orchestrator whose probe is refused restarts the
    process, which would cause the outage the setting was meant to prevent. A
    unit test can call the middleware and see it refuse; only the assembled
    router shows that the probes are not behind it.
  * the allowlist is the OUTER of the two middlewares on /metrics, so a source
    that will be refused regardless gets a 403 and not a 401 -- it does not learn
    from a 401-then-403 difference whether the token it presented was valid.

A fourth claim is asserted here rather than in the unit suite: an allowlist entry
that does not parse is refused at startup, with an error that names the setting.
That is the fail-closed half -- the middleware's own backstop is never reached
with a validated config, and this is what an operator actually meets.

Run from the repository root:

    python3 scripts/b5_surface_smoke_check.py

Exit code 0 means the surface behaved as the code says it does.
"""

import json
import os
import socket
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BIN_NAME = "goddi-b5-surface-smoke"
JWT_SECRET = "b5-smoke-jwt-secret-not-a-real-credential-0123456789"

FORBIDDEN_MESSAGE = "this client is not allowed to reach the management API"

# Every request goes through this opener, which carries no proxy handlers. See
# request() for why a direct connection is the difference between measuring this
# process and measuring whatever proxy the machine happens to export.
DIRECT = urllib.request.build_opener(urllib.request.ProxyHandler({}))

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


def config_yaml(data_dir, http_port, dns_port, allow=None):
    lines = [
        'env: "development"',
        "server:",
        '  name: "GoDDI"',
        '  role: "all"',
        f'  http_addr: "127.0.0.1:{http_port}"',
        f'  public_url: "http://127.0.0.1:{http_port}"',
        f'  data_dir: "{data_dir}"',
        "database:",
        '  driver: "sqlite"',
        f'  dsn: "{data_dir}/goddi.db"',
        "dns:",
        "  enabled: true",
        "  listeners:",
        "    udp:",
        "      enabled: true",
        f'      address: "127.0.0.1:{dns_port}"',
        "    tcp:",
        "      enabled: false",
        "dhcp:",
        "  enabled: false",
        "  interfaces: []",
        "security:",
        f'  jwt_secret: "{JWT_SECRET}"',
    ]
    if allow:
        lines.append("  admin_allow_cidrs:")
        for cidr in allow:
            lines.append(f'    - "{cidr}"')
    lines += [
        "log:",
        '  level: "info"',
    ]
    return "\n".join(lines) + "\n"


def request(port, method, path, body=None, timeout=5):
    """Return (status, body). A status of None means nothing answered.

    The request is made through an opener with no proxy handlers. That is not
    tidiness: this harness runs on machines that export http_proxy, and Python
    does NOT bypass the proxy for 127.0.0.1. Without this, every request goes to
    the proxy instead of to the process under test -- which forwards correctly
    while the process is up, and answers its own 5xx once it is down. A check
    written that way measures the operator's proxy, and reports "the port is
    still serving" about a process that has already exited.
    """
    url = f"http://127.0.0.1:{port}{path}"
    data = None
    headers = {}
    if body is not None:
        data = json.dumps(body).encode()
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with DIRECT.open(req, timeout=timeout) as resp:
            return resp.status, resp.read().decode("utf-8", "replace")
    except urllib.error.HTTPError as exc:
        return exc.code, exc.read().decode("utf-8", "replace")
    except Exception as exc:  # connection refused, timeout, reset
        return None, str(exc)


def listening(port, timeout=2):
    """True when something accepts a TCP connection on the port.

    A raw socket rather than an HTTP request, because "is anything listening"
    is a different question from "what does it say" and the proxy cannot be
    involved in the first one at all.
    """
    try:
        with socket.create_connection(("127.0.0.1", port), timeout=timeout):
            return True
    except OSError:
        return False


def start(binary, config, cwd, log_path):
    with open(log_path, "wb") as log:
        process = subprocess.Popen(
            [binary, "serve", "-c", config],
            cwd=cwd, stdout=log, stderr=subprocess.STDOUT,
        )
    return process


def stop(process):
    if process.poll() is not None:
        return
    process.terminate()
    try:
        process.wait(timeout=20)
    except subprocess.TimeoutExpired:
        process.kill()
        process.wait(timeout=10)


def wait_until_serving(port, seconds=30):
    deadline = time.time() + seconds
    while time.time() < deadline:
        status, _ = request(port, "GET", "/health")
        if status == 200:
            return True
        time.sleep(0.3)
    return False


def case_open_list(binary, tmp_root):
    """With no allowlist configured, the surface is open and unchanged."""
    print("\n用例 1：未配置白名单时管理面照常开放")
    home = os.path.join(tmp_root, "open")
    data_dir = os.path.join(home, "data")
    os.makedirs(data_dir, exist_ok=True)
    config = os.path.join(home, "config.yaml")
    http_port, dns_port = free_port(), free_port()
    with open(config, "w") as handle:
        handle.write(config_yaml(data_dir, http_port, dns_port))

    process = start(binary, config, home, os.path.join(home, "serve.log"))
    try:
        if not check(wait_until_serving(http_port), "the process came up and answered /health"):
            return
        status, body = request(http_port, "GET", "/health")
        check(status == 200, f"GET /health = {status}, want 200")

        # An empty allowlist is a no-op: the login route must be reachable, not
        # refused. Any answer other than 403 counts; a fresh instance has no
        # administrator, so the route itself decides what it says.
        status, body = request(http_port, "POST", "/api/v1/auth/login",
                               {"username": "nobody", "password": "nothing"})
        check(status not in (403, None),
              f"POST /api/v1/auth/login = {status}, want anything but 403 "
              f"(an empty allowlist changes nothing; body {body.strip()!r})")

        # /metrics is part of the management surface and is authenticated, which
        # is the W11 half: the route exists and is guarded. A 404 would mean it
        # was never registered.
        status, body = request(http_port, "GET", "/metrics")
        check(status == 401,
              f"GET /metrics without a token = {status}, want 401 "
              f"(registered and authenticated; body {body.strip()!r})")
    finally:
        stop(process)


def case_foreign_list(binary, tmp_root):
    """With an allowlist that excludes 127.0.0.1, what is refused and what is not."""
    print("\n用例 2：白名单指向外部网段时，管理面被拒而编排探针不受影响")
    home = os.path.join(tmp_root, "foreign")
    data_dir = os.path.join(home, "data")
    os.makedirs(data_dir, exist_ok=True)
    config = os.path.join(home, "config.yaml")
    http_port, dns_port = free_port(), free_port()
    with open(config, "w") as handle:
        handle.write(config_yaml(data_dir, http_port, dns_port, allow=["10.99.0.0/16"]))

    process = start(binary, config, home, os.path.join(home, "serve.log"))
    try:
        if not check(wait_until_serving(http_port), "the process came up and answered /health"):
            return

        # The probes belong to the orchestrator and are registered above the
        # allowlist. An orchestrator whose probe is refused restarts a process
        # that is serving its clients correctly.
        status, body = request(http_port, "GET", "/health")
        check(status == 200,
              f"GET /health = {status}, want 200: the orchestrator probe is registered "
              f"above the allowlist (body {body.strip()!r})")
        status, body = request(http_port, "GET", "/ready")
        check(status not in (403, None),
              f"GET /ready = {status}, want anything but 403: readiness is a probe too "
              f"(body {body.strip()!r})")

        # The management surface is refused, with the message and not a bare 403.
        status, body = request(http_port, "POST", "/api/v1/auth/login",
                               {"username": "nobody", "password": "nothing"})
        check(status == 403,
              f"POST /api/v1/auth/login = {status}, want 403 (body {body.strip()!r})")
        check(FORBIDDEN_MESSAGE in body,
              "the refusal says why, rather than being a bare 403")

        # The allowlist is the OUTER middleware on /metrics, so an address that
        # will be refused regardless is refused by address -- not told, via a
        # 401, that this endpoint wants a credential.
        status, body = request(http_port, "GET", "/metrics")
        check(status == 403,
              f"GET /metrics from outside the allowlist = {status}, want 403 and not 401: "
              f"the allowlist runs before the credential check (body {body.strip()!r})")
    finally:
        stop(process)


def case_broken_entry(binary, tmp_root):
    """An allowlist entry that does not parse is refused at startup, by name."""
    print("\n用例 3：读不出的白名单条目让进程拒绝启动，并点名该设置")
    home = os.path.join(tmp_root, "broken")
    data_dir = os.path.join(home, "data")
    os.makedirs(data_dir, exist_ok=True)
    config = os.path.join(home, "config.yaml")
    http_port, dns_port = free_port(), free_port()
    # A bare address rather than a network. The configuration refuses it rather
    # than letting the middleware widen the list to everything.
    with open(config, "w") as handle:
        handle.write(config_yaml(data_dir, http_port, dns_port, allow=["10.0.0.5"]))

    completed = subprocess.run(
        [binary, "serve", "-c", config],
        cwd=home, stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
        timeout=60, check=False,
    )
    output = completed.stdout.decode("utf-8", "replace")
    check(completed.returncode != 0, "the process refused to start")
    check("admin_allow_cidrs" in output,
          f"the error names the setting (output was {output.strip()[:200]!r})")
    check(not listening(http_port), "nothing is listening on the management port")


def main():
    tmp_root = tempfile.mkdtemp(prefix="goddi-b5-surface-")
    binary = os.path.join(tmp_root, BIN_NAME)
    print(f"building {BIN_NAME} ...")
    build = subprocess.run(
        ["go", "build", "-o", binary, "./cmd/goddi"],
        cwd=ROOT, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, check=False,
    )
    if build.returncode != 0:
        print(build.stdout.decode("utf-8", "replace"))
        print("build failed")
        return 2

    try:
        case_open_list(binary, tmp_root)
        case_foreign_list(binary, tmp_root)
        case_broken_entry(binary, tmp_root)
    finally:
        import shutil
        shutil.rmtree(tmp_root, ignore_errors=True)

    if failures:
        print(f"\n{len(failures)} 项管理面断言未按预期成立")
        return 1
    print("\n管理面冒烟检查通过：白名单的位置、顺序与失败方式都如代码所述")
    return 0


if __name__ == "__main__":
    sys.exit(main())
