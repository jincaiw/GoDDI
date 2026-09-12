#!/usr/bin/env python3
"""Mutation check for the request-level authorization guards (W14-c).

router_permission_consistency_test.go proves the permission *declarations*
agree with each other; it issues no request, so it would stay green if the
middleware were dropped from a route, or if the refusal started naming the
permission the caller lacks. internal/api/authorization_test.go is what covers
those two, and the two are exactly what this script removes:

  * the permission middleware is deleted from a route -- a viewer must then be
    able to do what it has no permission for, and the test says so;
  * the refusal message is made to name the missing permission -- the test's
    "a refusal must not hand the caller a map" assertion has to fire.

Discipline this script keeps, learned from the earlier rounds:

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
import subprocess
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

ROUTER_GO = "internal/api/router.go"
RBAC_MW_GO = "internal/rbac/middleware.go"

GUARD = "TestALowPrivilegeSessionIsStoppedAtTheRouteItCannotUse"
CHECK = ["go", "test", "./internal/api/", "-count=1", "-run", GUARD]

MUTATIONS = [
    {
        "name": "去掉某条路由上的权限中间件（viewer 于是能做它没权限的事）",
        "file": ROUTER_GO,
        "old": 'r.With(rbac.RequirePermission(rbacMgr, "user", "write")).Post("/", h.CreateUser)',
        "new": 'r.Post("/", h.CreateUser)',
        "must_contain": GUARD,
    },
    {
        "name": "拒绝时把缺少的权限名说出来（403 变成一张权限地图）",
        "file": RBAC_MW_GO,
        "old": '\t\t\t\twriteJSONError(w, http.StatusForbidden, 403, "权限不足")\n',
        "new": '\t\t\t\twriteJSONError(w, http.StatusForbidden, 403, "权限不足: 需要 "+resource+":"+action)\n',
        "must_contain": GUARD,
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

            completed = subprocess.run(
                CHECK, cwd=ROOT,
                stdout=subprocess.PIPE, stderr=subprocess.STDOUT, check=False,
            )
            output = completed.stdout.decode("utf-8", "replace")
            named = mutation["must_contain"]
            caught = completed.returncode != 0 and named in output

            if caught:
                print(f"ok   {mutation['name']}：检查确实失败了")
            else:
                report(f"FAIL {mutation['name']}：注入缺陷后检查仍然通过（或失败理由不对），"
                       f"这条断言是空转的")
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
    print("\n每处变异都被相应的检查捕获")

    return 0


if __name__ == "__main__":
    sys.exit(main())
