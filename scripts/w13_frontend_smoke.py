#!/usr/bin/env python3
"""Start (or stop) a real GoDDI instance for the W13 console smoke test.

Why a separate script rather than ``playwright.config.ts``'s ``webServer``: the
console is embedded in the Go binary, so exercising it through the browser means
building the Go binary, not starting a dev server. The e2e suite in ``e2e/``
already expects an instance at ``GODDI_TEST_BASE_URL``; this is what puts one
there, and it uses the same environment recipe as CI so the surface under test
is the one CI tests.

Usage, from the repository root:

    python3 scripts/w13_frontend_smoke.py up      # build, start, seed, print state
    python3 scripts/w13_frontend_smoke.py down    # stop and clean up

``up`` is idempotent in the sense that it refuses to start a second instance
while one is recorded; ``down`` always removes the state file.

The seeded data matters: the import dialog needs a subnet to import into, and
the detail drawer needs an address that has a DNS record, so that its sections
render something rather than an empty state.
"""

import json
import os
import shutil
import signal
import socket
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
STATE = os.path.join(tempfile.gettempdir(), "goddi-w13-smoke.json")
BIN = os.path.join(tempfile.gettempdir(), "goddi-w13-smoke-server")
LOG = os.path.join(tempfile.gettempdir(), "goddi-w13-smoke.log")

PORT = 16090
BASE = f"http://127.0.0.1:{PORT}"
ADMIN_USER = "admin"
ADMIN_PASSWORD = "Admin@123456"

SPACE_NAME = "w13-smoke-space"
SUBNET_NAME = "w13-smoke-subnet"
SUBNET_CIDR = "192.0.2.0/24"
# A /24 is at or below autoCreateMaxAddresses (1<<16), so the subnet is
# materialised: every address in it already has a row, and an import therefore
# reports updates. The /15 is above the threshold and stays sparse, so an
# import there reports creates -- the only way to see both actions through the
# console, and the reason the fixture carries two subnets.
SPARSE_SUBNET_NAME = "w13-smoke-sparse"
SPARSE_SUBNET_CIDR = "198.50.0.0/15"
ZONE_NAME = "w13smoke.test"
SEED_IP = "192.0.2.10"
SEED_NAME = "seeded.w13smoke.test"

# Every request goes direct. This machine exports http_proxy and Python does not
# bypass it for 127.0.0.1, so a default opener would test the proxy instead.
DIRECT = urllib.request.build_opener(urllib.request.ProxyHandler({}))


def fail(message):
    print(f"FAIL {message}")
    return 1


def request(method, path, body=None, token=None, csrf=None, timeout=10):
    headers = {}
    data = None
    if body is not None:
        data = json.dumps(body).encode()
        headers["Content-Type"] = "application/json"
    if token:
        headers["Authorization"] = f"Bearer {token}"
    if csrf:
        headers["X-CSRF-Token"] = csrf
    req = urllib.request.Request(f"{BASE}{path}", data=data, headers=headers, method=method)
    try:
        with DIRECT.open(req, timeout=timeout) as resp:
            return resp.status, json.loads(resp.read().decode("utf-8", "replace") or "{}")
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode("utf-8", "replace")
        try:
            return exc.code, json.loads(raw or "{}")
        except json.JSONDecodeError:
            return exc.code, {"raw": raw}
    except Exception as exc:
        return None, {"error": str(exc)}


def listening(port, timeout=2):
    try:
        with socket.create_connection(("127.0.0.1", port), timeout=timeout):
            return True
    except OSError:
        return False


def wait_for_health(seconds=45):
    deadline = time.time() + seconds
    while time.time() < deadline:
        status, _ = request("GET", "/health")
        if status == 200:
            return True
        time.sleep(0.3)
    return False


def seed():
    """Log in and create the fixtures the console flow needs.

    Returns the state dict, or raises RuntimeError with what went wrong.
    """
    status, body = request(
        "POST", "/api/v1/auth/login",
        {"username": ADMIN_USER, "password": ADMIN_PASSWORD},
    )
    if status != 200:
        raise RuntimeError(f"login = {status} {body}")
    data = body["data"]
    token, csrf = data["token"], data.get("csrf_token", "")

    status, body = request(
        "POST", "/api/v1/ipam/spaces",
        {"name": SPACE_NAME, "description": "seeded by the W13 console smoke test"},
        token, csrf,
    )
    if status not in (200, 201):
        raise RuntimeError(f"create space = {status} {body}")
    space_id = body["data"]["id"]

    status, body = request(
        "POST", "/api/v1/ipam/subnets",
        {"space_id": space_id, "name": SUBNET_NAME, "cidr": SUBNET_CIDR, "description": "import target"},
        token, csrf,
    )
    if status not in (200, 201):
        raise RuntimeError(f"create subnet = {status} {body}")
    subnet_id = body["data"]["id"]

    status, body = request(
        "POST", "/api/v1/ipam/subnets",
        {"space_id": space_id, "name": SPARSE_SUBNET_NAME, "cidr": SPARSE_SUBNET_CIDR, "description": "sparse import target"},
        token, csrf,
    )
    if status not in (200, 201):
        raise RuntimeError(f"create sparse subnet = {status} {body}")
    sparse_subnet_id = body["data"]["id"]

    # The threshold is a behaviour of the fixture, not an assumption: if a
    # future change materialises the /15 the create path silently stops being
    # exercised, so assert it here rather than let the browser test fail with a
    # confusing "expected Create".
    status, body = request("GET", f"/api/v1/ipam/subnets/{sparse_subnet_id}/stats", token=token)
    if status == 200 and body.get("data", {}).get("materialized"):
        raise RuntimeError(
            f"{SPARSE_SUBNET_CIDR} is now materialised; the create path cannot be exercised through it"
        )

    # One seeded address plus one A record, so the detail drawer has a DNS
    # section and a scope section to render rather than two empty states.
    status, body = request(
        "POST", "/api/v1/ipam/addresses/allocate",
        {"subnet_id": subnet_id, "ip_address": SEED_IP, "hostname": "seeded-host", "description": "seeded"},
        token, csrf,
    )
    if status not in (200, 201):
        raise RuntimeError(f"allocate = {status} {body}")

    status, body = request(
        "POST", "/api/v1/dns/zones",
        {"name": ZONE_NAME, "type": "forward", "enabled": True},
        token, csrf,
    )
    if status not in (200, 201):
        raise RuntimeError(f"create zone = {status} {body}")
    zone_id = body["data"]["id"]

    status, body = request(
        "POST", f"/api/v1/dns/zones/{zone_id}/records",
        {"name": SEED_NAME, "type": "A", "value": SEED_IP, "ttl": 300},
        token, csrf,
    )
    if status not in (200, 201):
        raise RuntimeError(f"create record = {status} {body}")

    return {
        "base_url": BASE,
        "username": ADMIN_USER,
        "password": ADMIN_PASSWORD,
        "space_id": space_id,
        "subnet_id": subnet_id,
        "subnet_label": f"{SUBNET_NAME} ({SUBNET_CIDR})",
        "subnet_cidr": SUBNET_CIDR,
        "sparse_subnet_id": sparse_subnet_id,
        "sparse_subnet_label": f"{SPARSE_SUBNET_NAME} ({SPARSE_SUBNET_CIDR})",
        "seed_ip": SEED_IP,
        "seed_name": SEED_NAME,
        "zone_name": ZONE_NAME,
        "data_dir": STATE_DATA_DIR,
    }


STATE_DATA_DIR = os.path.join(tempfile.gettempdir(), "goddi-w13-smoke-data")


def up():
    if os.path.exists(STATE):
        with open(STATE) as handle:
            previous = json.load(handle)
        if previous.get("pid") and _alive(previous["pid"]):
            print(f"已有实例在跑（pid {previous['pid']}）；先 down 再 up")
            return 1
        os.remove(STATE)

    if listening(PORT):
        return fail(f"端口 {PORT} 已被占用；本脚本要独占它，请先腾出来")

    if os.path.isdir(STATE_DATA_DIR):
        shutil.rmtree(STATE_DATA_DIR)
    os.makedirs(STATE_DATA_DIR, exist_ok=True)

    print("构建二进制…")
    build = subprocess.run(["go", "build", "-o", BIN, "./cmd/goddi"], cwd=ROOT)
    if build.returncode != 0:
        return fail("go build 失败")

    env = {
        **os.environ,
        "GODDI_SERVER_HTTP_ADDR": f"127.0.0.1:{PORT}",
        "GODDI_SERVER_PUBLIC_URL": BASE,
        "GODDI_SERVER_DATA_DIR": STATE_DATA_DIR,
        "GODDI_DATABASE_DSN": os.path.join(STATE_DATA_DIR, "goddi.db"),
        "GODDI_SECURITY_JWT_SECRET": "w13-smoke-secret-not-for-production-0123456789",
        "GODDI_ADMIN_USERNAME": ADMIN_USER,
        "GODDI_ADMIN_PASSWORD": ADMIN_PASSWORD,
        "GODDI_DNS_ENABLED": "false",
        "GODDI_DHCP_ENABLED": "false",
    }
    log = open(LOG, "wb")
    process = subprocess.Popen([BIN, "serve"], cwd=ROOT, env=env, stdout=log, stderr=subprocess.STDOUT)

    if not wait_for_health():
        process.terminate()
        tail = open(LOG, "r", errors="replace").read()[-2000:]
        return fail(f"实例没有在 {PORT} 上就绪；日志尾部：\n{tail}")

    try:
        state = seed()
    except RuntimeError as exc:
        process.terminate()
        tail = open(LOG, "r", errors="replace").read()[-2000:]
        return fail(f"种子数据失败：{exc}\n日志尾部：\n{tail}")

    state["pid"] = process.pid
    with open(STATE, "w") as handle:
        json.dump(state, handle, indent=2, ensure_ascii=False)

    print(json.dumps(state, indent=2, ensure_ascii=False))
    print(f"\n实例已就绪：{BASE}  （pid {process.pid}，日志 {LOG}）")
    return 0


def _alive(pid):
    try:
        os.kill(pid, 0)
    except OSError:
        return False
    return True


def down():
    if not os.path.exists(STATE):
        print("没有记录的实例")
        return 0
    with open(STATE) as handle:
        state = json.load(handle)
    pid = state.get("pid")
    if pid and _alive(pid):
        os.kill(pid, signal.SIGTERM)
        for _ in range(50):
            if not _alive(pid):
                break
            time.sleep(0.2)
        else:
            os.kill(pid, signal.SIGKILL)
    os.remove(STATE)
    print(f"已停止（pid {pid}）")
    return 0


def main():
    command = sys.argv[1] if len(sys.argv) > 1 else ""
    if command == "up":
        return up()
    if command == "down":
        return down()
    print(__doc__)
    return 2


if __name__ == "__main__":
    sys.exit(main())
