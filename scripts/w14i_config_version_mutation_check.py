#!/usr/bin/env python3
"""Mutation check for the configuration-version work: records and creations.

Two things can go wrong in this code silently, and both are the kind that looks
right in review:

  * The record set is a whole-set replace, and dns_records is the one table in
    the system with two authors. A replace that does not say "only the rows this
    revision owns" deletes the A record a DHCP binding published, and the
    consequence appears weeks later as an address that is leased and has no
    name. Name normalisation is the second half of the same trap: a publish that
    stores "www" alongside a row the console stored as "www.example.test."
    produces two rows answering one query.
  * The creation is recorded as a revision that was never released on purpose.
    Marking it `applied` claims a data plane confirmed something it was never
    told about, and `applied` is the status an operator reads as "this is
    running".

The API layer is mutated too, in the one place where the choice is the whole
product: a lost concurrency race has to come back as 409, because 400 tells the
console to fix a document that is fine and 500 hides a normal outcome inside
"the server is broken".

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

RECORDS = "internal/configver/adapters_records.go"
CREATION = "internal/configver/creation.go"
HANDLER = "internal/api/handler/config_version.go"

CHECKS = {
    "unit": [
        "go", "test", "-v", "./internal/configver/",
        "-run", "TestTheRecordSetLeavesTheRecordsADataPlaneAuthoredAlone|TestACreationIsTheFirstRevision",
        "-count=1", "-timeout", "300s",
    ],
    "api": [
        "go", "test", "-v", "./internal/api/handler/",
        "-run", "TestAPublishWithAStaleBaselineIsAConflict",
        "-count=1", "-timeout", "300s",
    ],
}

MUTATIONS = [
    {
        "name": "整集替换不区分作者（把数据面推上来的记录一起删掉）",
        "file": RECORDS,
        "check": "unit",
        # The target and the replacement both carry the closing of the Exec, so
        # that neither is a substring of the other: a mutation whose marker is
        # already in the file is reported as a dirty tree every run and proves
        # nothing.
        "old": "DELETE FROM dns_records WHERE zone_id = ? AND authored_locally = 0`, id); err != nil {",
        "new": "DELETE FROM dns_records WHERE zone_id = ?`, id); err != nil {",
        "must_fail": "TestTheRecordSetLeavesTheRecordsADataPlaneAuthoredAlone",
    },
    {
        "name": "写入前不再归一化记录名（同一个查询落到两行上）",
        "file": RECORDS,
        "check": "unit",
        "old": "rowID, id, zone.NormalizeRecordName(rec.Name, zoneName), rec.Type, rec.Value,",
        "new": "rowID, id, rec.Name, rec.Type, rec.Value,",
        "must_fail": "TestTheRecordSetLeavesTheRecordsADataPlaneAuthoredAlone",
    },
    {
        "name": "创建记成 applied（声称数据面确认过它从没被告知的东西）",
        "file": CREATION,
        "check": "unit",
        "old": "\t\tStatus:       StatusPersisted,",
        "new": "\t\tStatus:       StatusApplied,",
        "must_fail": "TestACreationIsTheFirstRevision",
    },
    {
        "name": "并发冲突报成 400（让控制台去改一份没问题的文档）",
        "file": HANDLER,
        "check": "api",
        "old": """	case errors.As(err, &conflict):
		response.Conflict(w, conflict.Error())""",
        "new": """	case errors.As(err, &conflict):
		response.BadRequest(w, conflict.Error())""",
        "must_fail": "TestAPublishWithAStaleBaselineIsAConflict",
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

    The trailing " (" is load-bearing: a subtest's line reads
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
                CHECKS[mutation["check"]], cwd=ROOT,
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
