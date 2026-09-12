#!/usr/bin/env python3
"""D1 决策的实测口径：从 DHCP ACK 到「名字可解析」到底多少毫秒。

第八节把 DDNS 生效时间的界写成 2s，依据是三项上界相加——唤醒（≈0）、下行轮询（≤1s）、
消费者排水（≤1s）。上界之和不是测量：它抓不到「每一项都对、拼起来不对」，也说不清真实
数字是贴着界还是在界的一半。这个脚本做两件事：

  1. 跑 `internal/dhcp/server` 里的四库联动用例，把数字打出来（不是上界，是实测）；
  2. 把唤醒（`Server.SetOutboxWake`）撤掉，要求该用例**失败**——否则「实测的是唤醒
     有没有生效」这句话就是空话，数字再好看也只证明了两段轮询各是 1s。

实测结果（本机，2026-09-12）写在脚本末尾的 EXPECTED 注释里，供对照。实测值贴着 2s，
原因不是抖动的运气，而是**同一进程内两个 1s ticker 相位锁定**：消费者的 tick 总是刚好
落在「同步刚写完、下一轮还没到」的缝里，于是每一轮都付满两项之和。这条结论已回写到
第八节，界本身没有被改。

脚本比对的是**最慢**一轮。这一点由 2026-09-12 首次推送 CI 教会：那时最慢一轮报 2.130s，
之后两轮是 1.862s 与 1.994s——差额全在**第一轮的冷启动**里（客户端第一次发 UDP、zone store
第一次查询），与设计无关。用例因此丢掉一轮预热再测；余量也从 0.1s 提到 0.25s。

**这个数字是量化的，不是抖动的**：两个 1s ticker 相位锁定，一轮的代价要么是两次轮询、要么
是三次，落在哪一档由进程启动时的 tick 对齐决定，与机器忙不忙无关。撤掉唤醒实测到过
3.006/2.995/2.997/2.999s（同一进程四轮全在慢档），也见到过同一份代码报 1.994s（快档）。
所以下面「撤掉唤醒必须失败」的门禁在快档下会失效，脚本因此**重试一次**再下结论——两个独立
进程的相位互不相关，两次都落在快档是罕见的。

跑法（仓库根目录）：

    python3 scripts/d1_ddns_latency_check.py

退出码 0 表示测量跑通、数字在允许范围内、且撤掉唤醒后确实失败。
"""

import os
import pathlib
import re
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent

TEST_FILE = ROOT / "internal/dhcp/server/ddns_latency_test.go"
PKG = "./internal/dhcp/server/"
CASE = "TestTheTimeFromAckToResolvableIsWithinTheBound"

# 宿主用例要跑一轮预热加三轮实测、每轮最多等 10s，加上装配与两次完整数据面启动。
MEASURE_TIMEOUT = "300s"

# 撤掉唤醒的那一行。断言它出现且只出现一次——上一次运行被打断会留下已变异的树，
# 那时「用例失败」证明的就不是唤醒，而是残留。
WAKE_CALL = "\ts.SetOutboxWake(dhcpRunner.Wake)"
WAKE_ABSENT = "\t// MUTATION: the wake-up is removed"

# 撤掉唤醒后应当多等一整轮（1s）。若它仍然通过，就说明这个用例量的不是唤醒。
MUTATION_MUST_FAIL = True


def run(cmd):
    return subprocess.run(cmd, cwd=ROOT, capture_output=True, text=True)


def slowest_seconds(output):
    """从用例日志里取「最慢一轮」的秒数，取不到就返回 None。

    grep 型的解析会把「取不到」表现为 0，于是「数字很小」和「根本没量到」变得无法区分。
    """
    match = re.search(r"slowest of \d+ measured rounds: ([0-9.]+)(m?s|µs|us|ns)", output)
    if not match:
        return None
    value = float(match.group(1))
    unit = match.group(2)
    if unit == "s":
        return value
    if unit == "ms":
        return value / 1_000
    if unit in ("µs", "us"):
        return value / 1_000_000
    return value / 1_000_000_000


def main():
    failures = []

    if not TEST_FILE.exists():
        print(f"SKIPPED-NOT-FOUND: {TEST_FILE} 不在树里")
        return 1

    original = TEST_FILE.read_text()

    if WAKE_ABSENT in original:
        print("DIRTY-TREE: 测试文件里已经含有变异后的文本，上一次运行没有还原干净")
        return 1

    occurrences = original.count(WAKE_CALL)
    if occurrences != 1:
        print(f"SKIPPED-NOT-FOUND: 唤醒调用在测试文件里出现 {occurrences} 次，期望恰好 1 次")
        return 1

    # ---- 一次：实测 -------------------------------------------------------
    proc = run(["go", "test", PKG, "-run", CASE, "-count=1", "-v",
                "-timeout", MEASURE_TIMEOUT])
    output = proc.stdout + proc.stderr

    if "build failed" in output or "[build failed]" in output:
        print("FAIL: 用例编译不过，什么都测量不到")
        print(output[-3000:])
        return 1

    seconds = slowest_seconds(output)
    if seconds is None:
        print("FAIL: 没能从输出里读出最慢一轮的秒数——这不算「测得很快」")
        print(output[-3000:])
        return 1

    print(f"实测：最慢一轮 {seconds:.3f}s（界 2s，两段轮询各 1s）")
    if proc.returncode != 0:
        failures.append(f"用例没有通过（最慢一轮 {seconds:.3f}s）")
        print(output[-3000:])

    # 贴着界是本次的结论，不是失败；超过界 + 调度余量才说明多等了一轮。
    # 余量与用例里的 latencySlack 一致（0.25s）。
    if seconds > 2.25:
        failures.append(f"最慢一轮 {seconds:.3f}s 超过了 2s 界加 0.25s 调度余量")

    # ---- 两次：撤掉唤醒，必须失败 -----------------------------------------
    #
    # 重试一次：数字是相位量化的（见文件头），落在快档时撤掉唤醒也报 ≈2s，
    # 那一档下这条门禁失效。两个独立进程的相位互不相关，所以第二次基本不会再撞。
    mutated = original.replace(WAKE_CALL, WAKE_ABSENT, 1)
    try:
        # 撤掉调用点后，`dhcpRunner` 仍被赋值给 world，不会变成未使用变量，
        # 所以这处变异改的是赋值/调用处而不是判断处，能编译。
        TEST_FILE.write_text(mutated)

        attempts = []
        for attempt in (1, 2):
            proc = run(["go", "test", PKG, "-run", CASE, "-count=1", "-v",
                        "-timeout", MEASURE_TIMEOUT])
            output = proc.stdout + proc.stderr

            if "build failed" in output or "[build failed]" in output:
                failures.append("撤掉唤醒后编译不过，这处变异什么都验证不了")
                print(output[-2000:])
                break

            mutated_seconds = slowest_seconds(output)
            attempts.append((proc.returncode, mutated_seconds))

            if proc.returncode != 0 and mutated_seconds is not None:
                print(f"变异（第 {attempt} 次）：撤掉唤醒后最慢一轮 {mutated_seconds:.3f}s，"
                      f"用例失败（{CASE}）")
                if mutated_seconds <= seconds:
                    failures.append(
                        f"撤掉唤醒后最慢一轮反而是 {mutated_seconds:.3f}s（原 {seconds:.3f}s）："
                        f"这个用例量的不是唤醒")
                break

            print(f"变异（第 {attempt} 次）：撤掉唤醒后用例仍通过"
                  f"（最慢一轮 {mutated_seconds}s）——相位落在快档，重试一次")
        else:
            failures.append(
                f"撤掉唤醒后连跑两次都通过（最慢一轮 {[a[1] for a in attempts]}）："
                f"它没有钉住唤醒")
            print(output[-2000:])
    finally:
        TEST_FILE.write_text(original)

    # ---- 还原不靠信任，靠逐字节比对 ---------------------------------------
    if TEST_FILE.read_text() != original:
        failures.append("RESTORE-FAILED: 还原后测试文件与原文不一致")
    else:
        # 包变绿不等于文件还原，两件事都要说。
        proc = run(["go", "test", PKG, "-count=1", "-timeout", "300s"])
        if proc.returncode != 0:
            failures.append(f"RESTORE-FAILED: 还原后 {PKG} 没有全绿")
            print((proc.stdout + proc.stderr)[-3000:])
        else:
            print(f"还原：{TEST_FILE.name} 与原文逐字节一致，{PKG} 全绿")

    if failures:
        print(f"\n{len(failures)} 项问题：")
        for item in failures:
            print(" -", item)
        return 1

    print(f"\n实测 {seconds:.3f}s 在界内；撤掉唤醒后确实失败。")
    return 0


# EXPECTED（本机 10C macOS arm64，2026-09-12；供下次运行对照）
#
#   有唤醒：最慢一轮 ≈ 1.998–2.007s（预热后三轮彼此相差 <10ms）
#   撤掉唤醒：≈ 2.995–3.006s（慢档）或 ≈ 1.994s（快档，见文件头）
#
# 两个数字都是**两次 1s 轮询的下界**，差额只有一档。判据是「超过 2.25s」，
# 也就是说：一档之内是设计，多出一档才是缺陷。


if __name__ == "__main__":
    sys.exit(main())
