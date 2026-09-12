#!/usr/bin/env python3
"""Mutation check for the W14-e capacity work.

W14-e measured the allocation path at the documented ceiling (ADR 0001: 20,000
leases per node) and found it cost 310 ms per call, because it issued two
queries for every address already handed out. The rewrite reads the held set
once and walks it for the first gap. That is a performance change to the code
that decides which address a client gets, so it needs both kinds of proof:

  * an *equivalence* proof -- the same address comes back as before, pinned
    against a reference implementation in
    TestTheFirstFreeAddressIsTheOneBruteForceWouldFind;
  * a *shape* proof -- the cost no longer depends on how far into the pool the
    first gap is, pinned by
    TestTheCostOfAllocatingDoesNotDependOnWhereTheGapIs.

Each mutation below breaks one of those and requires the named case to be the
thing that notices.

The first mutation restores the old shape rather than the old text: a probe per
candidate address, with its own query. It replaces a whole contiguous block, so
it is described by its first and last line rather than by an exact string.

Discipline kept from the earlier rounds:

  * a mutation that cannot compile proves nothing, so every mutation here keeps
    the tree buildable -- including dropping the now-unused `sort` import when
    the block that used it is replaced;
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

LEASE_GO = "internal/dhcp/lease/lease.go"

CHECK = [
    "go", "test", "-v", "./internal/dhcp/lease/",
    "-run", "TestTheFirstFreeAddressIsTheOneBruteForceWouldFind"
            "|TestTheCostOfAllocatingDoesNotDependOnWhereTheGapIs",
    "-count=1", "-timeout", "900s",
]

# The observed cost of one probe per candidate address, restored by mutation 1.
# Kept as a hole so the mutation is described by the property it brings back
# rather than by a copy of code that no longer exists.
PROBE_LOOP = '''	// MUTATION: the original shape -- one probe per candidate address.
	statusPlaceholders, statusArgs := heldStatusPlaceholders()
	availabilityQuery := `SELECT CASE WHEN EXISTS(
			SELECT 1 FROM dhcp_leases
			WHERE scope_id = ? AND ip_address = ? AND status IN (` + statusPlaceholders + `)
		) OR EXISTS(
			SELECT 1 FROM dhcp_reservations
			WHERE scope_id = ? AND ip_address = ? AND enabled = 1
		) THEN 1 ELSE 0 END`
	for candidate := from; candidate <= to; candidate++ {
		ipStr := uint64ToIPv4(candidate)
		args := append([]interface{}{scopeID, ipStr}, statusArgs...)
		args = append(args, scopeID, ipStr)
		var takenN int
		if err := m.db.QueryRow(availabilityQuery, args...).Scan(&takenN); err != nil {
			return "", err
		}
		if takenN == 0 {
			return ipStr, nil
		}
	}
	return "", fmt.Errorf("no available IP addresses in scope")
}
'''

MUTATIONS = [
    {
        "name": "分配退回「每个候选地址一次探测」的形状（代价重新取决于池子有多满）",
        "file": LEASE_GO,
        "replace_between": [
            "\t// The held-status placeholder list comes from heldStatuses so a status can",
            "\treturn uint64ToIPv4(next), nil\n}\n",
        ],
        "new": PROBE_LOOP,
        # The block that used it goes away with the block, so the import has to
        # go too or the mutation does not compile and proves nothing.
        "also_drop": [("\t\"net\"\n\t\"sort\"\n", "\t\"net\"\n")],
        "must_fail": "TestTheCostOfAllocatingDoesNotDependOnWhereTheGapIs",
    },
    {
        "name": "走查被抽掉，永远返回范围起点（起点被占用时也照发）",
        "file": LEASE_GO,
        "old": '''	next := from
	for _, v := range taken {
		if v < next || v > to {
			continue
		}
		if v > next {
			break
		}
		next = v + 1
	}''',
        "new": '''	next := from''',
        # The mutated text is the original with the loop cut out, so it *contains*
        # the replacement; what marks a half-restored tree is the line that used
        # to follow the loop being directly behind the assignment.
        "dirty": '''	next := from
	if next > to {''',
        "must_fail": "TestTheFirstFreeAddressIsTheOneBruteForceWouldFind",
    },
    {
        "name": "间隙判断被抽掉，next 一直推到最后一个被占用地址的下一个",
        "file": LEASE_GO,
        "old": '''		if v > next {
			break
		}''',
        "new": '''		if false {
			break
		}''',
        "must_fail": "TestTheFirstFreeAddressIsTheOneBruteForceWouldFind",
    },
    {
        "name": "范围上界不再检查（池子满了也返回范围外的地址）",
        "file": LEASE_GO,
        "old": '''	if next > to {
		return "", fmt.Errorf("no available IP addresses in scope")
	}''',
        "new": '''	if false {
		return "", fmt.Errorf("no available IP addresses in scope")
	}''',
        "must_fail": "TestTheFirstFreeAddressIsTheOneBruteForceWouldFind",
    },
    {
        "name": "预约不再进入被占用集合（把有预约的地址发给了别人）",
        "file": LEASE_GO,
        "old": '''		UNION
		SELECT ip_address FROM dhcp_reservations
			WHERE scope_id = ? AND enabled = 1`''',
        "new": '''		UNION
		SELECT ip_address FROM dhcp_reservations
			WHERE scope_id = ? AND enabled = 1 AND 1 = 0`''',
        "must_fail": "TestTheFirstFreeAddressIsTheOneBruteForceWouldFind",
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
    """Return 'pass' or 'fail' for a top-level test, or None if it did not run.

    The trailing " (" is load-bearing: a subtest prints `--- PASS: Parent/child`,
    which contains `--- PASS: Parent`, so a substring search reports a parent
    that failed as passing whenever any one of its children was green.
    """
    pattern = r"^--- (PASS|FAIL): " + re.escape(test_name) + r" \("
    match = re.search(pattern, output, re.MULTILINE)
    return match.group(1).lower() if match else None


def apply_mutation(original, mutation):
    """Return the mutated text, or a string explaining why it cannot be made."""
    if "replace_between" in mutation:
        start_marker, end_marker = mutation["replace_between"]
        if original.count(start_marker) != 1:
            return f"SKIPPED-NOT-FOUND 起始标记出现 {original.count(start_marker)} 次"
        if original.count(end_marker) != 1:
            return f"SKIPPED-NOT-FOUND 结束标记出现 {original.count(end_marker)} 次"
        start = original.index(start_marker)
        end = original.index(end_marker) + len(end_marker)
        text = original[:start] + mutation["new"] + original[end:]
    else:
        if original.count(mutation["old"]) != 1:
            return f"SKIPPED-NOT-FOUND 替换目标出现 {original.count(mutation['old'])} 次"
        text = original.replace(mutation["old"], mutation["new"])
    for old, new in mutation.get("also_drop", []):
        if old in text:
            text = text.replace(old, new, 1)
            break
    return text


def main():
    touched = sorted({m["file"] for m in MUTATIONS})
    before = snapshot(touched)

    for mutation in MUTATIONS:
        path = os.path.join(ROOT, mutation["file"])
        with open(path, "r", encoding="utf-8") as handle:
            original = handle.read()

        if mutation.get("dirty", mutation["new"]) in original:
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
                CHECK, cwd=ROOT,
                stdout=subprocess.PIPE, stderr=subprocess.STDOUT, check=False,
            )
            output = completed.stdout.decode("utf-8", "replace")
            wanted = mutation["must_fail"]

            if "build failed" in output or "[build failed]" in output:
                report(f"FAIL {mutation['name']}：变异后编译不过，什么都验证不了")
                print(f"----- 尾部输出 -----\n{output[-1500:]}")
            elif named_result(output, wanted) == "fail":
                print(f"ok   {mutation['name']}：{wanted} 确实失败了")
            else:
                report(f"FAIL {mutation['name']}：注入缺陷后 {wanted} 没有失败，这条断言是空转的")
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
    print(f"\n每处变异都被相应的检查捕获（{len(MUTATIONS)}/{len(MUTATIONS)}）")
    return 0


if __name__ == "__main__":
    sys.exit(main())
