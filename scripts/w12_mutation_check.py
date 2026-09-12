#!/usr/bin/env python3
"""Mutation check for the W12-b and W12-c additions.

Each mutation injects one defect into the code under test and requires the
named check to fail. A check that still passes with the defect in place was
never checking anything -- and that is exactly the failure mode a green run
cannot reveal.

Every mutation asserts its replacement target exists before patching, so a
mutation that silently matched nothing is reported as SKIPPED-NOT-FOUND and
counts as a failure rather than as a pass.
"""

import os
import subprocess
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
PYTHON = sys.executable

MUTATIONS = [
    {
        "name": "回滚记录被读成已应用（用行存在代替 is_applied）",
        "file": "internal/database/migrate_status.go",
        "old": "\t\tapplied[version] = state\n",
        "new": "\t\tapplied[version] = true\n",
        "check": ["go", "test", "./internal/database/", "-count=1", "-run",
                  "TestARolledBackMigrationIsReportedPendingAgain"],
    },
    {
        "name": "migrate --status 直接打开数据库，从而创建它",
        "file": "cmd/goddi/main.go",
        "old": "\tif _, err := os.Stat(path); err != nil {\n\t\tif os.IsNotExist(err) {",
        "new": "\tif _, err := os.Stat(path); err != nil {\n\t\tif false {",
        "check": [PYTHON, "scripts/w12_snapshot_smoke_check.py"],
        "must_contain": "did not create the database it was asked about",
    },
    {
        "name": "恢复前不比 schema（闸门只比产品版本）",
        "file": "cmd/goddi/main.go",
        "old": "\t\tcase entry.SchemaVersion > known:\n",
        "new": "\t\tcase false && entry.SchemaVersion > known:\n",
        "check": [PYTHON, "scripts/w12_snapshot_smoke_check.py"],
        "must_contain": "too new",
    },
    {
        "name": "拒绝理由里产品版本排在 schema 前面",
        "file": "cmd/goddi/main.go",
        "old": "\tcase len(f.SchemaAhead) > 0:\n",
        "new": "\tcase false && len(f.SchemaAhead) > 0:\n",
        "check": ["go", "test", "./cmd/goddi/", "-count=1", "-run",
                  "TestTheSchemaSignalOutranksTheProductVersion"],
    },
    {
        "name": "未知存储被当成可读，而不是拒绝",
        "file": "cmd/goddi/main.go",
        "old": "\t\tif !ok {\n\t\t\tfitness.UnknownStores = append(fitness.UnknownStores, entry.Name)\n\t\t\tcontinue\n\t\t}\n",
        "new": "\t\tif !ok {\n\t\t\tcontinue\n\t\t}\n",
        "check": ["go", "test", "./cmd/goddi/", "-count=1", "-run",
                  "TestTheGateRefusesAStoreThisBinaryCannotPlace"],
    },
    {
        "name": "空主机恢复也强行走「替换前副本」",
        "file": "cmd/goddi/main.go",
        "old": "\tif existing := existingStores(cfg); len(existing) == 0 {\n",
        "new": "\tif existing := snapshotTargets(cfg); false {\n",
        "check": [PYTHON, "scripts/w12c_restore_drill.py"],
        "must_contain": "the recovery notices there was nothing to preserve",
    },
]

failures = []


def report(message):
    failures.append(message)
    print(message)


for mutation in MUTATIONS:
    path = os.path.join(ROOT, mutation["file"])
    with open(path, "r", encoding="utf-8") as handle:
        original = handle.read()

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

        completed = subprocess.run(
            mutation["check"],
            cwd=ROOT,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            check=False,
        )
        output = completed.stdout.decode("utf-8", "replace")
        failed = completed.returncode != 0
        named = mutation.get("must_contain")
        if named is not None:
            failed = failed and (named in output)

        if failed:
            print(f"ok   {mutation['name']}：检查确实失败了")
        else:
            report(f"FAIL {mutation['name']}：注入缺陷后检查仍然通过，这条断言是空转的")
            print(f"----- 尾部输出 -----\n{output[-1500:]}")
    finally:
        with open(path, "w", encoding="utf-8") as handle:
            handle.write(original)

if failures:
    print(f"\n{len(failures)} 项变异未被捕获")
    sys.exit(1)
print("\n每处变异都被相应的检查捕获")
