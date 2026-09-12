#!/usr/bin/env python3
"""Mutation check for the upward-queue defect found in W14.

Every one of the six queues that carry changes up to the control database used
to return on the first row error. Because the markers are cleared only after the
transaction commits, a single row the control database would not accept left
every marker in the batch in place: the next pass read the same batch, stopped
at the same row, and it repeated once a second for as long as the process ran.
The runner reported it as a database error the whole time.

The fix has two halves, and each of them can be broken silently:

  * per-row isolation -- one refused row no longer decides the fate of the rows
    beside it (TestOneRefusedRowDoesNotTakeTheBatchWithIt);
  * refusal bookkeeping -- the refused row is written down and paced rather than
    retried at the poll interval (TestRefusedRowsAreHeldBackAndThenTravel,
    TestASecondRefusalWidensTheDelay).

There is also the mechanism's own reach: the refusal count is summed over a list
of the six marker tables, and the six pushes share one implementation. Both of
those are the kind of thing that stays correct until someone adds a seventh
queue, so they are mutated too.

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

TRANSFER = "internal/dataplane/transfer.go"

CHECK = [
    "go", "test", "-v", "./internal/dataplane/",
    "-run", "|".join([
        "TestOneRefusedRowDoesNotTakeTheBatchWithIt",
        "TestRefusedRowsAreHeldBackAndThenTravel",
        "TestASecondRefusalWidensTheDelay",
        "TestTheRetryDelayWidensAndThenStops",
        "TestEveryUpwardQueueRecordsARefusal",
        "TestEveryMarkerTableIsInTheUpwardQueueList",
    ]),
    "-count=1", "-timeout", "600s",
]

# The loop body as it was before the fix: the first row the control database
# refuses ends the pass, which is what left every marker in the batch behind.
OLD_SHAPE_UPSERT = '''		} else {
			if _, err := upsertStmt.Exec(p.values...); err != nil {
				return res, fmt.Errorf("dataplane: copying lease %s up: %w", p.id, err)
			}
			res.Upserted++
		}
'''

MUTATIONS = [
    {
        "name": "退回旧形状：一行被拒就让整批 pass 结束",
        "file": TRANSFER,
        "old": '''		} else {
			if _, err := upsertStmt.Exec(p.values...); err != nil {
				refused = append(refused, pushFailure{key: p.id, err: err})
				continue
			}
			res.Upserted++
		}
''',
        "new": OLD_SHAPE_UPSERT,
        "must_fail": "TestOneRefusedRowDoesNotTakeTheBatchWithIt",
    },
    {
        "name": "被拒的行不再记账（于是每个 tick 重试同一条语句）",
        "file": TRANSFER,
        "old": '''	if err := r.recordPushFailures("dhcp_lease_dirty", "lease_id", refused); err != nil {
		return res, err
	}
''',
        "new": '''	if len(refused) > 0 {
		// MUTATION: the refusal is not written down.
	}
''',
        "must_fail": "TestRefusedRowsAreHeldBackAndThenTravel",
    },
    {
        "name": "读队列时不再跳过仍在退避期内的行",
        "file": TRANSFER,
        "old": "\t\tWHERE d.next_attempt_at IS NULL OR julianday(d.next_attempt_at) <= julianday('now')\n",
        "new": "\t\tWHERE 1 = 1\n",
        "must_fail": "TestRefusedRowsAreHeldBackAndThenTravel",
    },
    {
        "name": "被拒的行也把标记清掉（把没送到报成送到了）",
        "file": TRANSFER,
        "old": '''			if _, err := upsertStmt.Exec(p.values...); err != nil {
				refused = append(refused, pushFailure{key: p.id, err: err})
				continue
			}
			res.Upserted++
''',
        "new": '''			if _, err := upsertStmt.Exec(p.values...); err != nil {
				// MUTATION: reported as delivered.
				done = append(done, p.id)
				continue
			}
			res.Upserted++
''',
        "must_fail": "TestOneRefusedRowDoesNotTakeTheBatchWithIt",
    },
    {
        "name": "退避阶梯被压成零（拒了立刻重试）",
        "file": TRANSFER,
        "old": '''func pushBackoff(attempts int) time.Duration {
	switch {
	case attempts <= 1:
		return time.Second
''',
        "new": '''func pushBackoff(attempts int) time.Duration {
	switch {
	case attempts <= 1:
		return 0
''',
        "must_fail": "TestTheRetryDelayWidensAndThenStops",
    },
    {
        "name": "拒绝计数漏掉一条队列（那条队列被拒时没有任何信号）",
        "file": TRANSFER,
        # A rename rather than a removal: a deletion leaves no text that exists
        # only after the mutation, so it could never be told apart from an
        # already-dirty tree. The typo is also the realistic shape -- this is
        # what a seventh queue added by hand would look like.
        "old": '\t{"dhcp_log_dirty", "log_id"},\n',
        "new": '\t{"dhcp_logs_dirty", "log_id"},\n',
        "must_fail": "TestEveryMarkerTableIsInTheUpwardQueueList",
    },
    {
        "name": "拒绝计数里的键列名写错（记账会在最坏的时刻失败）",
        "file": TRANSFER,
        "old": '\t{"dns_record_dirty", "record_id"},\n',
        "new": '\t{"dns_record_dirty", "record_key"},\n',
        "must_fail": "TestEveryMarkerTableIsInTheUpwardQueueList",
    },
    {
        "name": "六条队列里有一条没接上（只有租约那条被覆盖）",
        "file": TRANSFER,
        "old": '''		if _, err := stmt.Exec(p.values...); err != nil {
			refused = append(refused, pushFailure{key: p.key, err: err})
			continue
		}
		done = append(done, p.key)
		n++
	}
	if err := tx.Commit(); err != nil {
		return n, fmt.Errorf("dataplane: committing the log push: %w", err)
	}
''',
        "new": '''		if _, err := stmt.Exec(p.values...); err != nil {
			return n, fmt.Errorf("dataplane: copying log entry %s up: %w", p.key, err)
		}
		done = append(done, p.key)
		n++
	}
	if err := tx.Commit(); err != nil {
		return n, fmt.Errorf("dataplane: committing the log push: %w", err)
	}
''',
        "must_fail": "TestEveryUpwardQueueRecordsARefusal",
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

    The trailing " (" is load-bearing, and it was not in the earlier mutation
    scripts because none of their tests had subtests. A subtest's line reads
    `--- PASS: Parent/child`, which *contains* `--- PASS: Parent` -- so a naive
    substring search reports a parent test as passing on the strength of one
    green child, while the parent itself failed. This script's six-way queue
    test has exactly that shape.
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

        # The text that exists only after the mutation. Every mutation here is
        # a replacement, so that marker always exists.
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
