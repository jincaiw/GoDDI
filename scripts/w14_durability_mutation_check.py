#!/usr/bin/env python3
"""Mutation check for the W14-d durability work.

W14-d adds two things that are supposed to fail when they are wrong: a crash
drill that kills a real writer with SIGKILL and reads the store back, and a
storage-refusal test. Both are only worth something if the assertions they carry
have teeth, so this script breaks one thing at a time and requires the named
case to be the thing that notices.

Four mutations, plus one that must NOT be caught:

  * `dataplane`'s durability pragma drops to `synchronous=0`. The store's own
    read-back test must catch it -- that setting is the only reason an ACK can
    be called honest.
  * `createLeaseWithStatus` turns a storage refusal into a fabricated success.
    The lease manager's refusal test must catch it.
  * The crash drill's writer journals two sequence numbers per commit -- the
    shape of a server that reports bindings it never persisted. The drill's
    "everything committed is on disk" assertion must catch it.
  * `Degrade` swallows the errors from both meta writes, so the command prints a
    permission it never obtained. This one is checked by running
    scripts/w14_durability_check.py itself, because "a command may not report
    success for a permission it did not obtain" is an assertion that only exists
    there -- the package tests never invoke the CLI.

A near-miss worth knowing about: swallowing only the *first* of the two meta
writes is not caught, and should not be. `Degrade` writes the marker and its
reason as two statements, so the second one still fails and the command still
refuses -- the invariant holds. The mutation above has to break both to produce
a false success.

The fourth is the honest one. Mutating the same pragma must leave the SIGKILL
drill **passing**, and that is the point rather than an oversight: SIGKILL ends
a process, not a kernel, so the page cache still holds every byte the writer
handed to write(2). A build with `synchronous=NORMAL` would pass the crash drill
exactly as `FULL` does. This is recorded as a SURVIVED line with the reason, not
counted as a failure -- a check that silently claimed to cover power loss would
be worse than no check.

Discipline kept from the earlier rounds:

  * a mutation that cannot compile proves nothing, so every mutation here keeps
    the tree buildable;
  * the replacement target must exist and be unique, otherwise the mutation is
    reported as SKIPPED and counted as a failure;
  * a file that already contains the mutated text means a previous run was
    interrupted -- reported as DIRTY-TREE and refused;
  * restoring is not trusted: every touched file is compared byte for byte
    against a snapshot taken before the run.

Run from the repository root.
"""

import os
import re
import subprocess
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

STORE_GO = "internal/dataplane/store.go"
LEASE_GO = "internal/dhcp/lease/lease.go"
DRILL_GO = "internal/dhcp/lease/durability_test.go"
ROLE_GO = "internal/dhcp/ha/role.go"

CHECK = [
    "go", "test", "-v",
    "./internal/dataplane/", "./internal/dhcp/lease/",
    "-count=1",
]

# One mutation is checked by running the process-level script instead of the
# package tests, because the assertion it has to break -- "a command may not
# print success for a permission it did not obtain" -- only exists there.
CHECK_SMOKE = [sys.executable, "scripts/w14_durability_check.py"]

MUTATIONS = [
    {
        "name": "租约库的 synchronous 掉到 0（提交不再等待落盘）",
        "file": STORE_GO,
        "old": '{"synchronous", "2"}, // 2 = FULL',
        "new": '{"synchronous", "0"}, // 0 = OFF',
        "must_fail": "TestOpenAppliesTheDurabilitySettingsThatMakeAnAckHonest",
        "must_pass": [
            ("TestEveryCommittedBindingIsOnDiskAfterAKill",
             "SIGKILL 结束的是进程而不是内核，write(2) 写进去的字节仍在页缓存里，"
             "所以崩溃演练看不见耐久设置——这正是「掉电未验证」的那一半"),
        ],
    },
    {
        "name": "写入被存储层拒绝时返回一个编造的租约（客户端被告知拿到了地址）",
        "file": LEASE_GO,
        "old": '''	if err != nil {
		return nil, fmt.Errorf("failed to create lease: %w", err)
	}''',
        "new": '''	if err != nil {
		return &Lease{ID: id, ScopeID: scopeID, IPAddress: ip, MACAddress: mac, Status: status}, nil
	}''',
        "must_fail": "TestAWriteTheStoreRefusesIsReportedAndLeavesNothing",
    },
    {
        "name": "崩溃演练的写入方每提交一次就记两条（报告它没有持久化的绑定）",
        "file": DRILL_GO,
        "old": '\t\tif _, err := journal.WriteString(strconv.Itoa(i) + "\\n"); err != nil {',
        "new": '\t\tif _, err := journal.WriteString(strconv.Itoa(i) + "\\n" + strconv.Itoa(i+1) + "\\n"); err != nil {',
        "must_fail": "TestEveryCommittedBindingIsOnDiskAfterAKill",
    },
    {
        "name": "degrade 把两处 meta 写入的错误都吞掉（命令打印成功，库里的权限却没写成）",
        "file": ROLE_GO,
        "old": '''	if err := setMetaText(o.store.DB, metaDegraded, now.Format(time.RFC3339)); err != nil {
		return err
	}
	if err := setMetaText(o.store.DB, metaDegradedReason, reason); err != nil {
		return err
	}''',
        "new": '''	_ = setMetaText(o.store.DB, metaDegraded, now.Format(time.RFC3339))
	_ = setMetaText(o.store.DB, metaDegradedReason, reason)''',
        "runner": "smoke",
        "must_fail_text": "the command still reported success",
    },
]

failures = []


def report(message):
    failures.append(message)
    print(message)


def snapshot(paths):
    snap = {}
    for rel in paths:
        with open(os.path.join(ROOT, rel), "rb") as handle:
            snap[rel] = handle.read()
    return snap


def named_result(output, test_name):
    """Return 'pass', 'fail' or None for a top-level test in `go test -v` output.

    The trailing " (" is load-bearing. A subtest prints `--- PASS:
    Parent/child`, which *contains* `--- PASS: Parent`: a substring search
    would report the parent as passing on the strength of one green child while
    the parent itself failed. TestEveryCommittedBindingIsOnDiskAfterAKill -- the
    survivor this script asserts stays green under the `synchronous` mutation --
    has subtests, so without the anchor the one claim this script exists to make
    could be produced by output that contradicts it.
    """
    pattern = r"^--- (PASS|FAIL): " + re.escape(test_name) + r" \("
    match = re.search(pattern, output, re.MULTILINE)
    return match.group(1).lower() if match else None


def main():
    touched = sorted({m["file"] for m in MUTATIONS})
    before = snapshot(touched)

    for mutation in MUTATIONS:
        path = os.path.join(ROOT, mutation["file"])
        with open(path, "r", encoding="utf-8") as handle:
            original = handle.read()

        if mutation["new"] in original:
            report(f"DIRTY-TREE {mutation['name']}：{mutation['file']} 里已经含有变异后的文本，"
                   f"上一次运行没有还原干净")
            continue
        if mutation["old"] not in original:
            report(f"SKIPPED-NOT-FOUND {mutation['name']}：在 {mutation['file']} 里找不到替换目标")
            continue
        if original.count(mutation["old"]) != 1:
            report(f"SKIPPED-AMBIGUOUS {mutation['name']}：替换目标在 {mutation['file']} 里出现 "
                   f"{original.count(mutation['old'])} 次")
            continue

        try:
            with open(path, "w", encoding="utf-8") as handle:
                handle.write(original.replace(mutation["old"], mutation["new"]))

            command = CHECK_SMOKE if mutation.get("runner") == "smoke" else CHECK
            completed = subprocess.run(
                command, cwd=ROOT,
                stdout=subprocess.PIPE, stderr=subprocess.STDOUT, check=False,
            )
            output = completed.stdout.decode("utf-8", "replace")

            if mutation.get("runner") == "smoke":
                wanted = mutation["must_fail_text"]
                if wanted in output:
                    print(f"ok   {mutation['name']}：进程级脚本报出了「{wanted}」")
                else:
                    report(f"FAIL {mutation['name']}：注入缺陷后进程级脚本没有报出「{wanted}」，"
                           f"这条断言是空转的")
                    print(f"----- 尾部输出 -----\n{output[-1800:]}")
            else:
                wanted = mutation["must_fail"]
                if named_result(output, wanted) == "fail":
                    print(f"ok   {mutation['name']}：{wanted} 确实失败了")
                else:
                    report(f"FAIL {mutation['name']}：注入缺陷后 {wanted} 没有失败，这条断言是空转的")
                    print(f"----- 尾部输出 -----\n{output[-1800:]}")

            for survivor, reason in mutation.get("must_pass", []):
                if named_result(output, survivor) == "pass":
                    print(f"ok   {survivor} 在同一个变异下仍然通过，符合预期：{reason}")
                else:
                    report(f"FAIL {mutation['name']}：{survivor} 预期在这个变异下仍然通过，"
                           f"实际不是——关于「这个演练覆盖不到什么」的说明已经与事实不符")
                    print(f"----- 尾部输出 -----\n{output[-1800:]}")
        finally:
            with open(path, "w", encoding="utf-8") as handle:
                handle.write(original)
            with open(path, "r", encoding="utf-8") as handle:
                if handle.read() != original:
                    report(f"RESTORE-FAILED {mutation['file']}：还原后内容与原文件不一致")

    after = snapshot(touched)
    for rel in touched:
        if after[rel] != before[rel]:
            report(f"TREE-DIRTY {rel}：本轮结束后与开跑前的快照不一致")
    print("TREE-CLEAN" if all(after[r] == before[r] for r in touched) else "TREE-NOT-CLEAN")

    if failures:
        print(f"\n{len(failures)} 项问题")
        return 1
    print(f"\n每处变异都被相应的检查捕获，且唯一一处「预期不被捕获」的变异确实没被捕获"
          f"（{len(MUTATIONS)}/{len(MUTATIONS)}）")
    return 0


if __name__ == "__main__":
    sys.exit(main())
