#!/usr/bin/env python3
"""W14-h 真进程演练：管理进程 kill / 重启不影响 DNS 解析与 DHCP 续租。

这条退出条件此前只标注为「代码路径已成立，但尚未在真实双进程 + 控制库停机下演练」。
本脚本把它变成一次真演练：起三个真进程（control / dns / dhcp，各自独立 data_dir，
共用同一个控制库文件），在控制进程被 kill 的窗口里用真 UDP 查询验证 DNS 仍在应答、
验证 DHCP 进程租约库里「续租要读的那一行」仍在本地可读，并在控制进程重启后验证
停机期间排队的变更追了上来、下行也恢复。

它验证什么
  A 基线：控制台 API 建的 zone/A 记录被 dns 进程向下同步到本地副本，真 UDP 查询
    能拿到答案；往 dhcp 进程租约库直接种一条 active 租约能上行跑到控制库。
  B 停机：control.stop() 后 DNS 仍用真 UDP 应答且答案与停机前一致；dnsdata.db 的
    副本没丢；dhcp 进程还活着，租约行仍 active、lease_end 不变；在控制库被真正
    置为「不可写」时新产生的租约只留下待上行标记而到不了控制库。
  C 重启：control 用同一 data_dir/端口重启后，排队的租约与 DHCP→DNS 事件追上控制库；
    DHCP 事件 dirty 标记清空但 append-only 事实行保留，dns 进程消费下行事件并能被真
    UDP 查询解析；readiness 恢复 ok；再新建一条 A 记录，验证普通 DNS 下行也恢复。

它不验证什么（逐条见结尾「本脚本不能声称的」一节，这是本演练最重要的部分之一）
  * 没有一次真实的 DHCP REQUEST/ACK：本机非特权，:67 绑不上，只有「续租要读的那一行
    仍在本地面」这一层证据。
  * 这是单机三进程，不是三台主机的网络分区。实测：单机 kill 管理进程并不会让控制库
    变得不可写（三个角色共用同一个 SQLite 文件，数据面自己一直持有连接）。因此 B 节
    的「排队」是用一把同机写锁（BEGIN IMMEDIATE）把控制库置为不可写来制造的，这把锁
    是「远端/被分区的控制库对数据面不可用」在本环境里能取到的最接近的等价物。
  * 直接写库种租约不是 DHCP 服务端的发放路径（服务端内部的 lease.Manager 才写得对），
    这里只借它的写形状触发与真实发放相同的触发器。

跑法（仓库根目录）：

    python3 scripts/w14h_split_outage_drill.py

退出码 0 表示每一节的断言都成立（并请一并阅读结尾的「不能声称的」）。
"""

import json
import os
import re
import shutil
import socket
import sqlite3
import struct
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BIN_NAME = "goddi-w14h-drill"

ADMIN_USER = "admin"
ADMIN_PASSWORD = "Admin@123456"
JWT_SECRET = "w14h-drill-secret-not-a-real-credential-0123456789"

# 夹具：控制台里建的 zone 与两条 A 记录；两条租约；一个 DHCP 作用域。
ZONE = "drill.w14h.test"
BASE_NAME = "a1.drill.w14h.test"
NEW_NAME = "a2.drill.w14h.test"
BASE_IP = "203.0.113.7"
NEW_IP = "203.0.113.8"
DNS_EVENT_LEASE_ID = "w14h-dns-event"
DNS_EVENT_HOSTNAME = "event-host"
DNS_EVENT_IP = "10.9.0.70"

SCOPE_NAME = "w14h-drill-scope"
SCOPE_SUBNET = "10.9.0.0/24"
SCOPE_START = "10.9.0.10"
SCOPE_END = "10.9.0.200"

LEASE_DIRECT_ID = "w14h-lease-direct"
LEASE_QUEUED_ID = "w14h-lease-queued"
LEASE_DIRECT_IP = "10.9.0.50"
LEASE_QUEUED_IP = "10.9.0.60"

# 产品日志里的真实标记串（从 internal/dataplane/*.go 与 cmd/goddi/main.go 抄来，
# 不是编的）：下行同步、上行推送成功、以及控制库拒绝写入时的那一条。
M_PUSH_OK = "dataplane: pushed lease changes"
M_PUSH_REFUSED = "dataplane: the control database refused a change; it stays queued"
M_SYNC_FAILED = "dataplane: configuration sync failed; the local copy stays in service"
M_DHCP_DEGRADED = "failed to start DHCP server, running in degraded mode"
M_READINESS_DEGRADED = "dataplane: readiness is below ok; the local copy stays in service"
M_READINESS_OK = "dataplane: readiness ok"

# "domain":"dns","revision":3,"rows":2 —— 一条真的把副本落地的同步记录，对应日志里
# 的 "dataplane: configuration loaded from the control database"（首次）或
# "dataplane: configuration updated"（后续）。注意 "local configuration already
# current" 也带 domain 但没有 rows，所以用 rows 区分「真的应用了一次副本」与
# 「发现没有变化」。
DNS_APPLY = re.compile(r'"domain":"dns","revision":(\d+),"rows":(\d+)')
DHCP_APPLY = re.compile(r'"domain":"dhcp","revision":(\d+),"rows":(\d+)')

# 本机导出 http_proxy，Python 默认 opener 不会绕过 127.0.0.1；管理面请求全走直连。
DIRECT = urllib.request.build_opener(urllib.request.ProxyHandler({}))


class Checks:
    """逐条断言结果，并按节统计通过数。"""

    def __init__(self):
        self.rows = []
        self.current = "?"
        self.observations = []

    def section(self, name):
        self.current = name
        print(f"\n== {name} ==")

    def check(self, text, ok, detail=""):
        mark = "PASS" if ok else "FAIL"
        line = f"  [{mark}] {text}"
        if detail and not ok:
            line += f"  <- {detail}"
        print(line)
        self.rows.append((self.current, bool(ok), text))
        return bool(ok)

    def observe(self, text):
        """实测到的事实，不是判定：打印出来但不算通过/失败。

        有些结论是「环境长什么样」而不是「产品对不对」，硬判成 PASS/FAIL 会
        逼出一个恒真的断言。这种就如实打印。
        """
        print(f"  [OBS ] {text}")
        self.observations.append(text)

    def summary(self):
        order = []
        counts = {}
        for section, ok, _ in self.rows:
            if section not in counts:
                counts[section] = [0, 0]
                order.append(section)
            counts[section][0 if ok else 1] += 1
        print()
        total_fail = 0
        for section in order:
            passed, failed = counts[section]
            total_fail += failed
            print(f"{section}: {passed}/{passed + failed} 条断言通过")
        return total_fail


def free_port():
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def config_yaml(role, data_dir, control_db, http_port, dns_port, dns_on, dhcp_on):
    # 逐行拼，和 w07_role_smoke_check.py 一样：dedent 与 f-string 的首行续行会
    # 互相干扰，结果是一个静默解析失败的配置文件。
    on = lambda b: "true" if b else "false"
    lines = [
        "server:",
        '  name: "GoDDI"',
        f'  role: "{role}"',
        f'  http_addr: "127.0.0.1:{http_port}"',
        f'  data_dir: "{data_dir}"',
        "database:",
        '  driver: "sqlite"',
        f'  dsn: "{control_db}"',
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
        "dataplane:",
        "  sync_interval_seconds: 1",
        "security:",
        f'  jwt_secret: "{JWT_SECRET}"',
        "  totp_enabled: false",
        "log:",
        '  level: "debug"',
    ]
    return "\n".join(lines) + "\n"


class Node:
    """一个真进程：Popen + 日志文件 + stop()。抄自 w14_capacity_profile.py。"""

    def __init__(self, binary, name, cfg_path, tmp_root, env=None):
        self.name = name
        self.log_path = os.path.join(tmp_root, f"{name}.log")
        self._log = open(self.log_path, "w")
        self.proc = subprocess.Popen(
            [binary, "serve", "-c", cfg_path],
            stdout=self._log, stderr=subprocess.STDOUT, cwd=tmp_root, text=True,
            env=env,
        )

    def log(self):
        self._log.flush()
        try:
            with open(self.log_path, "r") as fh:
                return fh.read()
        except OSError:
            return ""

    def alive(self):
        return self.proc.poll() is None

    def stop(self):
        if self.proc.poll() is None:
            self.proc.terminate()
        try:
            self.proc.wait(timeout=20)
        except subprocess.TimeoutExpired:
            self.proc.kill()
            self.proc.wait()
        try:
            self._log.close()
        except OSError:
            pass


def wait_for(node, marker, timeout=60.0):
    """等一条日志标记；进程自己退出也算失败（返回当前日志）。"""
    deadline = time.time() + timeout
    while time.time() < deadline:
        out = node.log()
        if marker in out:
            return True, out
        if node.proc.poll() is not None:
            return False, out
        time.sleep(0.1)
    return False, node.log()


def wait_until(fn, timeout=30.0, interval=0.3):
    """轮询一个返回 (ok, value) 的函数，直到 ok 或超时。返回 (ok, value)。"""
    deadline = time.time() + timeout
    value = None
    while time.time() < deadline:
        ok, value = fn()
        if ok:
            return True, value
        time.sleep(interval)
    return False, value


def sqlite(db, script):
    """按库自身的耐久设置跑 SQL（w14_capacity_profile.py 的原样搬用）。"""
    done = subprocess.run(
        ["sqlite3", db],
        input="PRAGMA busy_timeout = 5000;\nPRAGMA journal_mode = WAL;\nPRAGMA synchronous = FULL;\n"
              + script,
        capture_output=True, text=True, timeout=120,
    )
    if done.returncode != 0:
        raise AssertionError(f"sqlite3 failed: {done.stderr.strip()}")
    return done.stdout


def write_row(db_path, sql, params=(), timeout=15):
    """带参数绑定地向本地库写一行，使用产品同样的 WAL + synchronous=FULL。"""
    conn = sqlite3.connect(db_path, timeout=timeout, isolation_level=None)
    try:
        conn.execute(f"PRAGMA busy_timeout = {int(timeout * 1000)}")
        conn.execute("PRAGMA journal_mode = WAL")
        conn.execute("PRAGMA synchronous = FULL")
        conn.execute(sql, params)
    finally:
        conn.close()


def query_rows(db_path, sql, params=(), timeout=5):
    """只读一行/几行。用普通连接：WAL 下读者不被写者阻塞（实测确认）。"""
    conn = sqlite3.connect(db_path, timeout=timeout, isolation_level=None)
    try:
        conn.execute(f"PRAGMA busy_timeout = {int(timeout * 1000)}")
        return conn.execute(sql, params).fetchall()
    finally:
        conn.close()


class ControlDBLock:
    """把控制库置为「不可写」：一把同机写锁。

    BEGIN IMMEDIATE 起一个写事务并持有写锁；数据面下一次推送上行会等满它自己的
    busy_timeout（5000ms）后收到 SQLITE_BUSY，于是那行留在待上行队列里。WAL 模式下
    读者仍能读到最后一次已提交的快照，所以「控制库里还没有这一行」这个断言可以在
    锁仍然持有时独立校验。
    """

    def __init__(self, db_path):
        self.db_path = db_path
        self.conn = None

    def acquire(self):
        self.conn = sqlite3.connect(self.db_path, timeout=15, isolation_level=None)
        self.conn.execute("PRAGMA busy_timeout = 15000")
        self.conn.execute("BEGIN IMMEDIATE")

    def release(self):
        if self.conn is None:
            return
        try:
            self.conn.execute("ROLLBACK")
        except sqlite3.Error:
            pass
        self.conn.close()
        self.conn = None


# ---------------------------------------------------------------------------
# 管理 API（w13_frontend_smoke.py 的鉴权与播种方式）
# ---------------------------------------------------------------------------

def http_get(port, path, timeout=5):
    url = f"http://127.0.0.1:{port}{path}"
    try:
        with DIRECT.open(url, timeout=timeout) as resp:
            return resp.status, resp.read().decode("utf-8", "replace")
    except urllib.error.HTTPError as exc:
        return exc.code, exc.read().decode("utf-8", "replace")
    except Exception as exc:  # connection refused / timeout / reset
        return None, str(exc)


def api(port, method, path, body=None, token=None, csrf=None, timeout=10):
    headers = {}
    data = None
    if body is not None:
        data = json.dumps(body).encode()
        headers["Content-Type"] = "application/json"
    if token:
        headers["Authorization"] = f"Bearer {token}"
    if csrf:
        headers["X-CSRF-Token"] = csrf
    req = urllib.request.Request(f"http://127.0.0.1:{port}{path}", data=data,
                                 headers=headers, method=method)
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


def login(port):
    status, body = api(port, "POST", "/api/v1/auth/login",
                       {"username": ADMIN_USER, "password": ADMIN_PASSWORD})
    if status != 200 or "data" not in body:
        raise AssertionError(f"登录失败：{status} {body}")
    data = body["data"]
    if not data.get("token"):
        raise AssertionError(f"登录响应没有 token：{body}")
    return data["token"], data.get("csrf_token", "")


# ---------------------------------------------------------------------------
# DNS：手拼报文、真 UDP 查询（本机没有 dnspython，也不引第三方依赖）
# ---------------------------------------------------------------------------

def _encode_name(name):
    out = b""
    for label in name.rstrip(".").split("."):
        if label:
            out += struct.pack("!B", len(label)) + label.encode()
    return out + b"\x00"


def _decode_name(data, off):
    labels = []
    jumped = False
    while True:
        length = data[off]
        if length == 0:
            off += 1
            break
        if length & 0xC0 == 0xC0:
            off += 2
            jumped = True
            break
        labels.append(data[off + 1:off + 1 + length].decode("ascii", "replace"))
        off += 1 + length
    return ".".join(labels), off


def dns_query(name, port, timeout=3.0):
    """发一条 A 查询，返回 (rcode, [(name, ip, ttl), ...])。

    解析不出来就抛 AssertionError 并带上原始响应，绝不返回一个空答案冒充成功。
    """
    packet = struct.pack("!HHHHHH", 0x14, 0x0100, 1, 0, 0, 0)
    packet += _encode_name(name) + struct.pack("!HH", 1, 1)

    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    sock.settimeout(timeout)
    try:
        sock.sendto(packet, ("127.0.0.1", port))
        data, _ = sock.recvfrom(4096)
    finally:
        sock.close()

    if len(data) < 12:
        raise AssertionError(f"DNS 响应过短：{data!r}")
    _qid, flags, qd, an, _ns, _ar = struct.unpack("!HHHHHH", data[:12])
    off = 12
    for _ in range(qd):
        _, off = _decode_name(data, off)
        off += 4
    answers = []
    for _ in range(an):
        owner, off = _decode_name(data, off)
        if off + 10 > len(data):
            raise AssertionError(f"DNS 回答被截断：{data.hex()}")
        rtype, _rclass, ttl, rdlen = struct.unpack("!HHIH", data[off:off + 10])
        off += 10
        rdata = data[off:off + rdlen]
        off += rdlen
        if rtype == 1 and rdlen == 4:
            answers.append((owner, socket.inet_ntoa(rdata), ttl))
    return flags & 0x0F, answers


def check_dns_answer(checks, label, name, port, want_ip, timeout=3.0):
    """真 UDP 查询并断言拿到 want_ip。

    查询本身失败（超时/连接被拒/报文解析不了）也要记成一条失败断言，而不是让异常
    冲出 main ——「DNS 停了」正是本演练要能报出来的事，它不能表现成脚本崩溃。
    """
    try:
        rcode, answers = dns_query(name, port, timeout=timeout)
    except (OSError, AssertionError) as exc:
        return checks.check(label, False, f"UDP 查询未得到应答：{exc!r}")
    ips = [ip for _, ip, _ in answers]
    return checks.check(label, rcode == 0 and want_ip in ips, f"rcode={rcode} answers={answers}")


# ---------------------------------------------------------------------------
# 库检查小工具
# ---------------------------------------------------------------------------

def control_has_lease(control_db, lease_id):
    try:
        rows = query_rows(control_db, "SELECT 1 FROM dhcp_leases WHERE id = ?", (lease_id,))
    except sqlite3.Error:
        return False
    return bool(rows)


def local_dirty(leases_db):
    try:
        rows = query_rows(leases_db, "SELECT lease_id FROM dhcp_lease_dirty")
    except sqlite3.Error:
        return None
    return {r[0] for r in rows}


def local_lease_row(leases_db, lease_id):
    try:
        rows = query_rows(
            leases_db,
            "SELECT status, lease_end FROM dhcp_leases WHERE id = ?", (lease_id,))
    except sqlite3.Error:
        return None
    return rows[0] if rows else None


def local_dns_a_values(dns_db):
    try:
        rows = query_rows(dns_db, "SELECT value FROM dns_records WHERE type = 'A'")
    except sqlite3.Error:
        return None
    return {r[0] for r in rows}


def pending_dns_events(db_path):
    try:
        rows = query_rows(
            db_path,
            "SELECT lease_id, action, status FROM dhcp_dns_events "
            "WHERE status = 'pending' ORDER BY id",
        )
    except sqlite3.Error:
        return None
    return rows


def all_dns_events(db_path):
    try:
        rows = query_rows(
            db_path,
            "SELECT lease_id, action, status FROM dhcp_dns_events ORDER BY id",
        )
    except sqlite3.Error:
        return None
    return rows


def dirty_dns_event_ids(db_path):
    try:
        rows = query_rows(db_path, "SELECT event_id FROM dhcp_dns_event_dirty")
    except sqlite3.Error:
        return None
    return {r[0] for r in rows}


# ---------------------------------------------------------------------------
# 各节
# ---------------------------------------------------------------------------

def section_baseline(checks, ports, control, dns, dhcp, data):
    """A. 基线：三个进程都在。"""
    checks.section("A. 基线（三进程都在）")

    status, body = http_get(ports["control"], "/health")
    checks.check("control 管理 API /health = 200", status == 200,
                 f"实际 {status} {body!r}")

    try:
        token, csrf = login(ports["control"])
        checks.check("控制台登录拿到 token", True)
    except AssertionError as exc:
        checks.check("控制台登录拿到 token", False, str(exc))
        return None

    status, body = api(ports["control"], "POST", "/api/v1/dhcp/scopes",
                       {"name": SCOPE_NAME, "subnet": SCOPE_SUBNET,
                        "start_ip": SCOPE_START, "end_ip": SCOPE_END, "enabled": True},
                       token, csrf)
    scope_id = body.get("data", {}).get("id") if isinstance(body, dict) else None
    checks.check("控制台建 DHCP 作用域返回 201", status in (200, 201) and bool(scope_id),
                 f"实际 {status} {body}")
    if not scope_id:
        return None

    status, body = api(ports["control"], "POST", "/api/v1/dns/zones",
                       {"name": ZONE, "type": "forward", "enabled": True}, token, csrf)
    zone_id = body.get("data", {}).get("id") if isinstance(body, dict) else None
    checks.check("控制台建 zone 返回 201", status in (200, 201) and bool(zone_id),
                 f"实际 {status} {body}")
    if not zone_id:
        return None

    status, body = api(ports["control"], "POST", f"/api/v1/dns/zones/{zone_id}/records",
                       {"name": BASE_NAME, "type": "A", "value": BASE_IP, "ttl": 120},
                       token, csrf)
    checks.check("控制台建 A 记录返回 201", status in (200, 201),
                 f"实际 {status} {body}")

    # 下行：dns 进程把 zone/记录复制进自己的副本。标记是真的 —— "rows":N 才是
    # 「应用了一次副本」，"local configuration already current" 不带 rows。
    def dns_applied():
        matches = DNS_APPLY.findall(dns.log())
        if not matches:
            return False, None
        rev, rows = matches[-1]
        return int(rows) >= 2, (int(rev), int(rows))

    ok, last = wait_until(dns_applied, timeout=30)
    checks.check("dns 进程日志出现向下同步标记（domain=dns, rows>=2）", ok,
                 f"最后一条同步记录 {last}；日志尾部 {dns.log()[-500:]!r}")

    values = local_dns_a_values(data["dns_db"])
    checks.check("dns 本地副本 dnsdata.db 里已有该 A 记录", values is not None and BASE_IP in values,
                 f"本地 A 值={values}")

    check_dns_answer(checks, "真 UDP 查询基线名拿到正确 A 答案", BASE_NAME,
                     ports["dns"], BASE_IP)
    # DHCP 侧：作用域也被复制过去了（证明多进程各自的下行都在跑）。
    def dhcp_applied():
        matches = DHCP_APPLY.findall(dhcp.log())
        if not matches:
            return False, None
        rev, rows = matches[-1]
        return int(rows) >= 1, (int(rev), int(rows))

    ok, last = wait_until(dhcp_applied, timeout=30)
    checks.check("dhcp 进程日志出现作用域向下同步标记（domain=dhcp, rows>=1）", ok,
                 f"最后一条同步记录 {last}")
    try:
        scopes = query_rows(data["leases_db"], "SELECT id FROM dhcp_scopes WHERE id = ?", (scope_id,))
    except sqlite3.Error:
        scopes = None
    checks.check("dhcp 本地库 leases.db 里已有该作用域", bool(scopes),
                 f"查询结果={scopes}")

    # 上行基线：往 dhcp 进程自己的租约库种一条 active 租约（真实发放写出来的形状），
    # 触发器会把它标成待上行。它应当跑到控制库 —— 这一步同时证明上行在基线是通的。
    write_row(data["leases_db"],
              "INSERT OR REPLACE INTO dhcp_leases (id, scope_id, ip_address, mac_address,"
              " hostname, client_id, lease_start, lease_end, status, last_seen, generation)"
              " VALUES (?, ?, ?, ?, ?, '', ?, ?, 'active', ?, 1)",
              (LEASE_DIRECT_ID, scope_id, LEASE_DIRECT_IP, "02:00:00:00:14:01",
               "w14h-direct", "2026-09-11T00:00:00Z", "2030-01-01T00:00:00Z",
               "2026-09-11T00:00:00Z"))
    dirty = local_dirty(data["leases_db"])
    checks.check("租约写入后本地出现待上行标记", dirty is not None and LEASE_DIRECT_ID in dirty,
                 f"dirty={dirty}")

    ok, _ = wait_until(lambda: (control_has_lease(data["control_db"], LEASE_DIRECT_ID), None),
                       timeout=30)
    checks.check("基线上行推送：租约跑到了控制库", ok,
                 f"控制库中未出现 {LEASE_DIRECT_ID}")

    ok, _ = wait_until(
        lambda: (dirty is not None and LEASE_DIRECT_ID not in (local_dirty(data["leases_db"]) or set()), None),
        timeout=15)
    checks.check("推送后本地待上行标记被清空", ok,
                 f"dirty={local_dirty(data['leases_db'])}")

    # 上行成功的日志标记（debug 级，脚本把 log.level 设成 debug 才能看到）。库里的
    # 行是主证据，这条是产品自己说它推了。
    checks.check("dhcp 日志报告了一次成功的租约上行推送", M_PUSH_OK in dhcp.log(),
                 dhcp.log()[-400:])

    return {"scope_id": scope_id, "zone_id": zone_id, "token": token, "csrf": csrf,
            "base_end": local_lease_row(data["leases_db"], LEASE_DIRECT_ID)}


def section_outage(checks, ports, control, dns, dhcp, data, state, lock):
    """B. 控制进程停机期间。lock 由 main 创建，在这里获取，由 main 兜底释放。"""
    checks.section("B. 控制进程停机期间")

    control.stop()
    time.sleep(1.5)
    status, body = http_get(ports["control"], "/health")
    checks.check("control 进程确已停止（管理 API 不再应答）", status is None,
                 f"仍返回 {status} {body!r}")
    checks.check("control 进程对象已退出", not control.alive(),
                 f"poll={control.proc.poll()}")

    # 核心主张：DNS 仍在应答，且答案与停机前一致。
    check_dns_answer(checks, "停机期间真 UDP 查询仍拿到正确 A 答案", BASE_NAME,
                     ports["dns"], BASE_IP)

    values = local_dns_a_values(data["dns_db"])
    checks.check("停机期间 dnsdata.db 本地副本未丢该记录", values is not None and BASE_IP in values,
                 f"本地 A 值={values}")

    # DHCP：进程还活着，且「续租要读的那一行」仍 active、lease_end 不变。
    checks.check("停机期间 dhcp 进程仍然活着", dhcp.alive(),
                 f"poll={dhcp.proc.poll()}")
    row = local_lease_row(data["leases_db"], LEASE_DIRECT_ID)
    checks.check("停机期间租约行仍可读且状态为 active", row is not None and row[0] == "active",
                 f"row={row}")
    checks.check("停机期间租约 lease_end 与停机前一致",
                 row is not None and state["base_end"] is not None and row[1] == state["base_end"][1],
                 f"停机前={state['base_end']} 停机后={row}")

    # ---- 本机事实：单机 kill 管理进程并不会让控制库不可写 ----
    # 三个角色共用同一个 SQLite 文件，数据面自己一直持有连接，所以它能在控制进程
    # 死后继续把变更写上去。这是单机三进程的边界，不是三台主机的网络分区。
    write_row(data["leases_db"],
              "INSERT OR REPLACE INTO dhcp_leases (id, scope_id, ip_address, mac_address,"
              " hostname, client_id, lease_start, lease_end, status, last_seen, generation)"
              " VALUES (?, ?, ?, ?, ?, '', ?, ?, 'active', ?, 1)",
              (LEASE_DIRECT_ID + "-2", state["scope_id"], LEASE_DIRECT_IP[:-2] + "80",
               "02:00:00:00:14:02", "w14h-direct2", "2026-09-11T00:00:00Z",
               "2030-01-01T00:00:00Z", "2026-09-11T00:00:00Z"))
    reached, _ = wait_until(
        lambda: (control_has_lease(data["control_db"], LEASE_DIRECT_ID + "-2"), None), timeout=15)
    if reached:
        checks.observe("单机 kill control 后，dhcp 仍把新租约推到了控制库："
                       "同机共享 SQLite 文件时，管理进程停止并不等于控制库不可用")
    else:
        checks.observe("单机 kill control 后新租约 15s 内没到控制库；"
                       "本环境这次没有观察到「共享文件仍可写」")

    # ---- 用写锁制造真正的「控制库不可写」，演练排队 ----
    lock.acquire()
    checks.check("已用 BEGIN IMMEDIATE 把控制库置为不可写", lock.conn is not None)

    # 同一条本地租约同时触发 DHCP→DNS 事件：先写租约，再写 durable outbox
    # intent。两者都由数据面上行，控制库不可写期间必须留在 dhcp 本地库。
    write_row(data["leases_db"],
              "INSERT OR REPLACE INTO dhcp_leases (id, scope_id, ip_address, mac_address,"
              " hostname, client_id, lease_start, lease_end, status, last_seen, generation)"
              " VALUES (?, ?, ?, ?, ?, '', ?, ?, 'active', ?, 1)",
              (LEASE_QUEUED_ID, state["scope_id"], LEASE_QUEUED_IP, "02:00:00:00:14:03",
               "w14h-queued", "2026-09-11T00:00:00Z", "2030-01-01T00:00:00Z",
               "2026-09-11T00:00:00Z"))
    write_row(data["leases_db"],
              "INSERT OR REPLACE INTO dhcp_leases (id, scope_id, ip_address, mac_address,"
              " hostname, client_id, lease_start, lease_end, status, last_seen, generation)"
              " VALUES (?, ?, ?, ?, ?, '', ?, ?, 'active', ?, 1)",
              (DNS_EVENT_LEASE_ID, state["scope_id"], DNS_EVENT_IP, "02:00:00:00:14:04",
               DNS_EVENT_HOSTNAME + "." + ZONE, "2026-09-11T00:00:00Z",
               "2030-01-01T00:00:00Z", "2026-09-11T00:00:00Z"))
    write_row(data["leases_db"],
              "INSERT INTO dhcp_dns_events (lease_id, generation, action, scope_id, ip_address,"
              " mac_address, hostname) VALUES (?, 1, 'create', ?, ?, ?, ?)",
              (DNS_EVENT_LEASE_ID, state["scope_id"], DNS_EVENT_IP,
               "02:00:00:00:14:04", DNS_EVENT_HOSTNAME + "." + ZONE))

    dirty = local_dirty(data["leases_db"])
    checks.check("停机期间产生的租约留下了待上行标记", dirty is not None and LEASE_QUEUED_ID in dirty,
                 f"dirty={dirty}")
    local_events = pending_dns_events(data["leases_db"])
    checks.check("DHCP→DNS 事件在本地 outbox 中待上行",
                 local_events is not None and any(r[0] == DNS_EVENT_LEASE_ID for r in local_events),
                 f"pending events={local_events}")
    checks.check("控制库暂时没有该 DHCP→DNS 事件",
                 not any(r[0] == DNS_EVENT_LEASE_ID for r in (all_dns_events(data["control_db"]) or [])),
                 f"control events={all_dns_events(data['control_db'])}")

    # 等一次推送尝试失败（busy_timeout 5s + 退避），再断言控制库还没有它。
    ok, _ = wait_until(lambda: (M_PUSH_REFUSED in dhcp.log(), None), timeout=25)
    checks.check("dhcp 进程日志记录了控制库拒绝写入、变更留在队列", ok,
                 f"未出现 {M_PUSH_REFUSED!r}")
    degraded_before = dhcp.log().count(M_READINESS_DEGRADED)
    ok, _ = wait_until(lambda: (dhcp.log().count(M_READINESS_DEGRADED) > degraded_before, None), timeout=15)
    checks.check("控制库不可写后 dhcp readiness 进入 degraded", ok,
                 f"degraded 日志数 {dhcp.log().count(M_READINESS_DEGRADED)}，之前 {degraded_before}")
    degraded_count = dhcp.log().count(M_READINESS_DEGRADED)
    time.sleep(2.0)
    checks.check("readiness 保持 degraded 时不重复记录同级别日志",
                 dhcp.log().count(M_READINESS_DEGRADED) == degraded_count,
                 f"前后计数 {degraded_count}/{dhcp.log().count(M_READINESS_DEGRADED)}")

    checks.check("控制库中确实还没有这条排队租约",
                 not control_has_lease(data["control_db"], LEASE_QUEUED_ID),
                 "控制库意外地已经有了它")

    # 写锁只阻塞写，不阻塞读（WAL 下读者不被写者挡住），所以 dns 的下行轮询在停机
    # 期间仍能读到控制库、副本保持完整，日志里不会出现 "configuration sync failed"。
    # 这不是故障，反而是「本地副本继续服务」的另一面；真正的读不可达（网络分区）本
    # 环境模拟不了，如实验证只能靠这把写锁，故只作观察不作断言。
    if M_SYNC_FAILED not in dns.log() and M_SYNC_FAILED not in dhcp.log():
        checks.observe("写锁只阻塞写不阻塞读（WAL）：停机期间下行轮询仍能读控制库，"
                       "因此没有出现 'configuration sync failed'；真正的读分区未模拟")

    checks.observe("边界（设计如此，不是故障）：停机期间新产生的变更要等控制库恢复才送达；"
                   "本节的「排队」是把控制库置为不可写后观察到的，锁在 C 节开头释放")


def section_restart(checks, ports, control_node, dns, dhcp, data, state, lock, processes):
    """C. 控制进程重启后。重启出的进程立刻登记进 processes，异常时也停得掉。"""
    checks.section("C. 控制进程重启后")

    lock.release()
    checks.check("已释放控制库写锁", lock.conn is None)
    degraded_count = dhcp.log().count(M_READINESS_DEGRADED)
    ready_count = dhcp.log().count(M_READINESS_OK)

    control = Node(control_node[0], "control", control_node[1], control_node[2],
                   env=control_node[3])
    processes.append(control)  # 先登记再往下走：中途抛异常也必须停得掉
    ok, _ = wait_until(lambda: (http_get(ports["control"], "/health")[0] == 200, None), timeout=45)
    checks.check("control 用同一 data_dir/端口重启后 /health = 200", ok,
                 f"/health={http_get(ports['control'], '/health')}")

    ok, _ = wait_until(
        lambda: (control_has_lease(data["control_db"], LEASE_QUEUED_ID), None), timeout=45)
    checks.check("停机期间排队的租约在重启后追上了控制库", ok,
                 f"控制库中仍没有 {LEASE_QUEUED_ID}")

    ok, _ = wait_until(
        lambda: (any(r[0] == DNS_EVENT_LEASE_ID for r in (all_dns_events(data["control_db"]) or [])), None),
        timeout=45)
    checks.check("DHCP→DNS 事件在控制库恢复后完成上行", ok,
                 f"控制库 events={all_dns_events(data['control_db'])}")

    def queue_empty():
        d = local_dirty(data["leases_db"])
        return d is not None and len(d) == 0, d

    ok, d = wait_until(queue_empty, timeout=30)
    checks.check("本地待上行队列回到 0", ok, f"dirty={d}")

    # DHCP 本地事件是 append-only 事实源：成功上行只清除 dirty marker，事件本身
    # 仍保留为 pending，供 DNS 副本下行后由 DNSConsumer 消费；不能把「本地事件行」
    # 误判成「本地上行队列」，否则会错误要求生产者删除它。
    def event_dirty_cleared():
        rows = query_rows(data["leases_db"],
                          "SELECT COUNT(*) FROM dhcp_dns_event_dirty d "
                          "JOIN dhcp_dns_events e ON e.id = d.event_id "
                          "WHERE e.lease_id = ?", (DNS_EVENT_LEASE_ID,))
        return int(rows[0][0]) == 0, rows

    ok, _ = wait_until(event_dirty_cleared, timeout=30)
    checks.check("DHCP 本地 DNS 事件成功上行后 dirty 标记清空", ok,
                 f"event_dirty={dirty_dns_event_ids(data['leases_db'])}")
    local_event_rows = all_dns_events(data["leases_db"])
    checks.check("DHCP 本地 DNS 事件事实行仍保留且未被生产者删除",
                 local_event_rows is not None and any(
                     r[0] == DNS_EVENT_LEASE_ID and r[2] == "pending" for r in local_event_rows),
                 f"local events={local_event_rows}")

    def dns_event_applied():
        rows = query_rows(data["dns_db"],
                          "SELECT value FROM dns_records WHERE owner = 'dhcp' AND owner_ref = ?",
                          (DNS_EVENT_LEASE_ID,))
        return bool(rows and any(r[0] == DNS_EVENT_IP for r in rows)), rows

    ok, records = wait_until(dns_event_applied, timeout=45)
    checks.check("DNS 副本消费下行事件并写入 DHCP A 记录", ok,
                 f"dns records={records}")
    check_dns_answer(checks, "真 UDP 查询 DHCP→DNS 事件名拿到正确 A 答案",
                     DNS_EVENT_HOSTNAME + "." + ZONE, ports["dns"], DNS_EVENT_IP)

    ok, _ = wait_until(lambda: (dhcp.log().count(M_READINESS_OK) > ready_count, None), timeout=30)
    checks.check("控制库恢复后 dhcp readiness 恢复 ok", ok,
                 f"ok 日志数 {dhcp.log().count(M_READINESS_OK)}，之前 {ready_count}")
    checks.check("readiness 恢复后没有再次产生重复 degraded 日志",
                 dhcp.log().count(M_READINESS_DEGRADED) == degraded_count,
                 f"degraded 日志数 {degraded_count}/{dhcp.log().count(M_READINESS_DEGRADED)}")

    ok, _ = wait_until(
        lambda: (control_has_lease(data["control_db"], LEASE_DIRECT_ID + "-2"), None), timeout=30)
    checks.check("停机期间直接写入的那条租约也在控制库里", ok,
                 f"控制库中没有 {LEASE_DIRECT_ID}-2")

    # 下行恢复：新建一条 A 记录，dns 进程同步到并能被真 UDP 查询解析。
    try:
        token, csrf = login(ports["control"])
        checks.check("重启后控制台仍可登录", True)
    except AssertionError as exc:
        checks.check("重启后控制台仍可登录", False, str(exc))
        return control

    before = len(DNS_APPLY.findall(dns.log()))
    status, body = api(ports["control"], "POST", f"/api/v1/dns/zones/{state['zone_id']}/records",
                       {"name": NEW_NAME, "type": "A", "value": NEW_IP, "ttl": 120},
                       token, csrf)
    checks.check("重启后新建 A 记录返回 201", status in (200, 201),
                 f"实际 {status} {body}")

    def dns_downlinked():
        values = local_dns_a_values(data["dns_db"])
        return values is not None and NEW_IP in values, values

    ok, values = wait_until(dns_downlinked, timeout=30)
    checks.check("新记录被 dns 进程向下同步到本地副本", ok, f"本地 A 值={values}")

    after = len(DNS_APPLY.findall(dns.log()))
    checks.check("dns 日志出现新一次副本应用（application 计数增加）", after > before,
                 f"before={before} after={after}")

    check_dns_answer(checks, "真 UDP 查询新名字拿到正确 A 答案（下行已恢复）", NEW_NAME,
                     ports["dns"], NEW_IP)

    check_dns_answer(checks, "重启后基线名仍能解析", BASE_NAME, ports["dns"], BASE_IP)

    return control


def caveats():
    print("""
== 本脚本不能声称的 ==

  * 真实 DHCP REQUEST/ACK 往返未验证。本机非特权，:67 绑不上（日志里是
    "failed to start DHCP server, running in degraded mode"），因此 B 节只验证了
    「续租要读的那一行仍在本地可读、lease_end 未变」——这是本环境里这条主张能取到的
    最强证据，不是一次真的续租往返，更不是一次真的 DISCOVER/OFFER/REQUEST/ACK。
  * 这是单机三进程，不是三台主机的网络分区。三个角色共用同一个 SQLite 控制库文件，
    数据面自己一直持有连接，所以单机 kill 管理进程并不会让控制库不可写（B 节已实测
    并打印）。B 节的「排队」是用一把同机写锁（BEGIN IMMEDIATE）把控制库置为不可写
    制造的，它是「远端/被分区的控制库对数据面不可用」在本环境里最接近的等价物，
    不是真的网络分区。
  * 「排队后能追上」验证的是重试路径与退避（tested 到几十秒），不是长时间停机。控制库
    不可写持续几小时/几天后队列会不会涨破、退避会不会退化，这里没有测。
  * 下行同步的验证靠轮询本地副本 + 真 UDP 查询，覆盖的是 1s 轮询这个默认节奏；把
    sync_interval_seconds 调大后客户端等待变长的量级，这里没有测。
  * 直接写库种租约与直接插入 DHCP→DNS 事件绕过了 DHCP 服务端的 lease.Manager /
    enqueueDNSEvent（唯一合法写者）。它们复用了同样的持久化表和上行/下行链路，但不
    等同于一次真实 DHCP REQUEST/ACK；字段（client_id、generation 等）是脚本手填的。
  * 控制进程重启用的是同一 data_dir 与端口，验证的是进程重启；不是升级/迁移过程、
    也不是控制库文件本身丢失或损坏后的恢复。
""")


def main():
    tmp_root = tempfile.mkdtemp(prefix="goddi-w14h-drill-")
    binary = os.path.join(tmp_root, BIN_NAME)
    checks = Checks()
    processes = []
    lock_holder = None

    try:
        print("构建二进制…")
        build = subprocess.run(["go", "build", "-o", binary, "./cmd/goddi"], cwd=ROOT)
        if build.returncode != 0:
            print("FAIL: go build 失败")
            return 1

        ports = {"control": free_port(), "dns": free_port(),
                 "control_bogus": free_port(), "dns_bogus": free_port()}

        control_dir = os.path.join(tmp_root, "control", "data")
        dns_dir = os.path.join(tmp_root, "dns", "data")
        dhcp_dir = os.path.join(tmp_root, "dhcp", "data")
        for d in (control_dir, dns_dir, dhcp_dir):
            os.makedirs(d, exist_ok=True)

        control_db = os.path.join(control_dir, "goddi.db")
        data = {
            "control_db": control_db,
            "dns_db": os.path.join(dns_dir, "dnsdata.db"),
            "leases_db": os.path.join(dhcp_dir, "leases.db"),
        }

        control_cfg = os.path.join(tmp_root, "control", "config.yaml")
        with open(control_cfg, "w") as fh:
            fh.write(config_yaml("control", control_dir, control_db,
                                 ports["control"], ports["control_bogus"], False, False))
        dns_cfg = os.path.join(tmp_root, "dns", "config.yaml")
        with open(dns_cfg, "w") as fh:
            fh.write(config_yaml("dns", dns_dir, control_db,
                                 ports["dns_bogus"], ports["dns"], True, False))
        dhcp_cfg = os.path.join(tmp_root, "dhcp", "config.yaml")
        with open(dhcp_cfg, "w") as fh:
            fh.write(config_yaml("dhcp", dhcp_dir, control_db,
                                 ports["dns_bogus"], ports["dns_bogus"], False, True))

        control_env = {**os.environ,
                       "GODDI_ADMIN_USERNAME": ADMIN_USER,
                       "GODDI_ADMIN_PASSWORD": ADMIN_PASSWORD}

        checks.section("0. 三进程启动")
        control = Node(binary, "control", control_cfg, tmp_root, env=control_env)
        processes.append(control)
        control_node = (binary, control_cfg, tmp_root, control_env)

        ok, _ = wait_until(lambda: (http_get(ports["control"], "/health")[0] == 200, None),
                           timeout=45)
        checks.check("control 启动并就绪 (/health=200)", ok)
        ok, _ = wait_for(control, "HTTP server listening", timeout=20)
        checks.check("control 日志报告管理 API 监听", ok, control.log()[-400:])

        dns = Node(binary, "dns", dns_cfg, tmp_root)
        processes.append(dns)
        ok, _ = wait_for(dns, "dns_server: started successfully", timeout=30)
        checks.check("dns 进程启动并报告 DNS listener 就绪", ok, dns.log()[-400:])
        ok, _ = wait_for(dns, "DNS data-plane store opened", timeout=10)
        checks.check("dns 打开了自己的数据面副本存储", ok)

        dhcp = Node(binary, "dhcp", dhcp_cfg, tmp_root)
        processes.append(dhcp)
        ok, _ = wait_for(dhcp, "DHCP lease store opened", timeout=30)
        checks.check("dhcp 打开了自己的租约存储", ok, dhcp.log()[-400:])
        # :67 的结果要等 Start() 之后才落定，所以等一小会儿再看，而不是在读到
        # "DHCP lease store opened" 的那一刻就下结论（那会与 bind 失败日志赛跑）。
        # 非特权下正确的答案是「明确报告绑不上」，但若本机这次绑上了也只是观察。
        failed_bind, _ = wait_for(dhcp, M_DHCP_DEGRADED, timeout=8)
        if failed_bind:
            checks.check("dhcp 明确报告 :67 绑不上并进入 degraded（本环境预期）", True)
        else:
            checks.observe("dhcp 进程 8s 内没有报 :67 失败，本机这次可能绑上了 :67；"
                           "真实 REQUEST/ACK 仍未验证")

        state = section_baseline(checks, ports, control, dns, dhcp, data)
        if state is None:
            raise AssertionError("基线未建立，后续小节无法进行")

        lock_holder = ControlDBLock(data["control_db"])
        section_outage(checks, ports, control, dns, dhcp, data, state, lock_holder)

        section_restart(checks, ports, control_node, dns, dhcp, data, state, lock_holder,
                        processes)

    except AssertionError as exc:
        checks.check("演练整体完成（无中途异常）", False, str(exc))
    finally:
        if lock_holder is not None:
            lock_holder.release()
        for node in reversed(processes):
            try:
                node.stop()
            except Exception:
                pass
        shutil.rmtree(tmp_root, ignore_errors=True)

    caveats()

    failures = checks.summary()
    if failures:
        print(f"\n{failures} 条断言未通过")
        return 1
    print("\n三节全部完成：每一条断言都成立（并请一并阅读上面的「不能声称的」）")
    return 0


if __name__ == "__main__":
    sys.exit(main())
