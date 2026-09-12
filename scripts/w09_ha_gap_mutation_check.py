#!/usr/bin/env python3
"""Mutation check for the shortfall a takeover is decided on.

The shortfall is "the peer's counter minus what this node applied", and the
value an operator has to type into `--accept-gap` is that subtraction. Two
things about it are easy to get wrong in a way that looks right:

  * where it is computed. The subtraction only describes a shortfall on a node
    that is mirroring somebody. Promotion clears the applied watermark, so a
    promoted node that keeps computing it reports the taken-over node's whole
    counter as missing -- permanently, on a node that is serving clients. The
    lazy fix for that is to stop reporting the figure anywhere, which is just
    as wrong in the other direction: a mirror that is behind has exactly that
    number, and it is the number the takeover demands.
  * what survives the promotion. The transaction that promotes also clears the
    watermarks the figure came from, so unless the figure is written down in
    that same transaction the record of what was given up is gone, and only the
    audit trail and the operator's terminal have it.

Each mutation below is a plausible way to have the wrong behaviour, and each is
checked twice where it can be: against the unit test, and against the
process-level case in w09_ha_operator_smoke_check.py, which is the one that
proves the running binary asks for the figure it just reported.

Discipline kept from the earlier rounds:

  * a mutation that cannot compile proves nothing, so every mutation here keeps
    the tree buildable;
  * the replacement target must exist and be unique, or the mutation is reported
    as SKIPPED and counted as a failure;
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

ROLE = "internal/dhcp/ha/role.go"

UNIT = [
    "go", "test", "-v", "./internal/dhcp/ha/",
    "-run", "TestAShortfallMustBeNamedToBeAccepted|TestAShortfallIsOnlyReportedWhereItMeansSomething",
    "-count=1", "-timeout", "300s",
]

PROCESS_CASE = "takeover-accepts-only-its-own-figure"
PROCESS = ["python3", "scripts/w09_ha_operator_smoke_check.py", f"--only={PROCESS_CASE}"]

GATED = '''	// Only a mirror is missing somebody else's changes. See the field's comment
	// for why the subtraction is not the answer anywhere else.
	if role == config.HARoleStandby {
		if st.Gap = st.PeerSeq - st.AppliedSeq; st.Gap < 0 {
			st.Gap = 0
		}
	}
'''

MUTATIONS = [
    {
        "name": "shortfall 不再按角色分：promoted 节点把对端整个计数器报成缺口",
        "file": ROLE,
        "old": GATED,
        "new": '''	// MUTATION: computed for every role.
	if st.Gap = st.PeerSeq - st.AppliedSeq; st.Gap < 0 {
		st.Gap = 0
	}
''',
        "must_fail": "TestAShortfallIsOnlyReportedWhereItMeansSomething",
        "process": True,
    },
    {
        "name": "闸门反过来：只有非镜像才算缺口（于是镜像永远报 0）",
        "file": ROLE,
        "old": GATED,
        "new": '''	// MUTATION: the gate is inverted, so the one node that has a shortfall
	// reports none and every other role reports one.
	if role != config.HARoleStandby {
		if st.Gap = st.PeerSeq - st.AppliedSeq; st.Gap < 0 {
			st.Gap = 0
		}
	}
''',
        "must_fail": "TestAShortfallIsOnlyReportedWhereItMeansSomething",
    },
    {
        "name": "promote 时写下的接受值不是算出来的缺口（写成 0）",
        "file": ROLE,
        "old": "setWatermark(ctx, tx, metaAcceptedGap, out.Gap)",
        "new": "setWatermark(ctx, tx, metaAcceptedGap, 0)",
        "must_fail": "TestAShortfallMustBeNamedToBeAccepted",
        "process": True,
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
    """Return what the named test did, or None if it did not run.

    The trailing " (" is load-bearing. A subtest's line reads
    `--- PASS: Parent/child`, which *contains* `--- PASS: Parent`, so a naive
    substring search reports a parent as passing on the strength of one green
    child while the parent itself failed.
    """
    pattern = r"^--- (PASS|FAIL): " + re.escape(test_name) + r" \("
    match = re.search(pattern, output, re.MULTILINE)
    return match.group(1).lower() if match else None


def apply_mutation(original, mutation):
    """Return the mutated text, or a string explaining why it cannot be made."""
    if original.count(mutation["old"]) != 1:
        return f"SKIPPED-NOT-FOUND 替换目标出现 {original.count(mutation['old'])} 次"
    return original.replace(mutation["old"], mutation["new"], 1)


def main():
    touched = sorted({m["file"] for m in MUTATIONS})
    before = snapshot(touched)

    for mutation in MUTATIONS:
        path = os.path.join(ROOT, mutation["file"])
        with open(path, "r", encoding="utf-8") as handle:
            original = handle.read()

        marker = mutation.get("dirty", mutation["new"])
        if marker in original:
            report(f"DIRTY-TREE {mutation['name']}：{mutation['file']} 里已经含有变异后的文本，"
                   f"上一次运行没有还原干净")
            continue

        mutated = apply_mutation(original, mutation)
        if mutated.startswith("SKIPPED"):
            report(f"{mutated} {mutation['name']}（{mutation['file']}）")
            continue

        try:
            with open(path, "w", encoding="utf-8") as handle:
                handle.write(mutated)

            completed = subprocess.run(
                UNIT, cwd=ROOT,
                stdout=subprocess.PIPE, stderr=subprocess.STDOUT, check=False,
            )
            output = completed.stdout.decode("utf-8", "replace")
            wanted = mutation["must_fail"]

            caught = False
            if "build failed" in output or "[build failed]" in output:
                report(f"FAIL {mutation['name']}：变异后编译不过，什么都验证不了")
                print(f"----- 尾部输出 -----\n{output[-1500:]}")
            elif named_result(output, wanted) == "fail":
                caught = True
                print(f"ok   {mutation['name']}：{wanted} 确实失败了")
            else:
                report(f"FAIL {mutation['name']}：注入缺陷后 {wanted} 没有失败，这条断言是空转的")
                print(f"----- 尾部输出 -----\n{output[-1800:]}")

            if mutation.get("process") and caught:
                # The process-level case has to see the same defect, or the
                # mechanism is covered only where the store is written by hand.
                ran = subprocess.run(
                    PROCESS, cwd=ROOT,
                    stdout=subprocess.PIPE, stderr=subprocess.STDOUT, check=False,
                )
                text = ran.stdout.decode("utf-8", "replace")
                if f"[FAIL] {PROCESS_CASE}" in text and ran.returncode != 0:
                    print(f"ok   {mutation['name']}：进程级用例 {PROCESS_CASE} 也失败了")
                else:
                    report(f"FAIL {mutation['name']}：进程级用例没有抓到，注入后它仍然报通过")
                    print(f"----- 尾部输出 -----\n{text[-1800:]}")
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
    print(f"\n每处变异都被相应的检查捕获（{len(MUTATIONS)}/{len(MUTATIONS)}）")
    return 0


if __name__ == "__main__":
    sys.exit(main())
