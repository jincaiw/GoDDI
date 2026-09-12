#!/usr/bin/env python3
"""Mutation check for the console/API permission consistency guards (W13-5).

The guards in internal/api/router_permission_consistency_test.go compare three
declarations written in two languages across two directories. A comparison like
that fails in two directions: it can be too loose (it passes while the
declarations disagree) and it can be too tight (it passes while reading
nothing). Both look identical in a green run, so each is injected separately
here.

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

ROUTER_TS = "web-admin/build/plugins/router.ts"
GUARD_INDEX = "web-admin/src/router/guard/index.ts"
GUARD_PERM = "web-admin/src/router/guard/permission.ts"
ROUTER_GO = "internal/api/router.go"
RBAC_GO = "internal/rbac/rbac.go"

API_TEST = ["go", "test", "./internal/api/", "-count=1", "-run"]


def guard(name):
    return API_TEST + [name]


MUTATIONS = [
    # --- too loose: the comparison must notice a disagreement --------------

    {
        "name": "控制台按一个后端从不检查的权限门禁路由",
        "file": ROUTER_TS,
        "old": '                  "resource": "audit",\n                  "action": "read"\n',
        "new": '                  "resource": "audit",\n                  "action": "list"\n',
        "check": guard("TestEveryConsoleRoutePermissionIsCheckedByAnAPIRoute"),
        "must_contain": "audit:list",
    },
    {
        "name": "路由要求一个没人能持有的权限（角色只能从目录里取）",
        "file": ROUTER_GO,
        "old": 'r.With(rbac.RequirePermission(rbacMgr, "settings", "write")).Post("/publish", handler.PublishConfigRevision)',
        "new": 'r.With(rbac.RequirePermission(rbacMgr, "settings", "publish")).Post("/publish", handler.PublishConfigRevision)',
        "check": guard("TestTheAPINeverGatesOnAPermissionNobodyCanHold"),
        "must_contain": "settings:publish",
    },
    {
        "name": "目录里多出一个没有任何路由检查的权限",
        "file": RBAC_GO,
        "old": '\t{Resource: "audit", Action: "read", Description: "Read audit logs"},\n',
        "new": '\t{Resource: "audit", Action: "read", Description: "Read audit logs"},\n'
               '\t{Resource: "widgets", Action: "read", Description: "Read widgets"},\n',
        "check": guard("TestThePermissionCatalogHasNoOrphans"),
        "must_contain": "widgets:read",
    },
    {
        "name": "预置角色授予一个目录里不存在的权限（拼写错误）",
        "file": RBAC_GO,
        "old": '\t\t\t{Resource: "ipam", Action: "delete"},\n\t\t\t{Resource: "settings", Action: "read"},\n'
               '\t\t\t{Resource: "audit", Action: "read"},\n\t\t\t{Resource: "backup", Action: "read"},\n',
        "new": '\t\t\t{Resource: "ipam", Action: "delete"},\n\t\t\t{Resource: "settings", Action: "read"},\n'
               '\t\t\t{Resource: "audit", Action: "read"},\n\t\t\t{Resource: "backupp", Action: "read"},\n',
        "check": guard("TestEveryGrantedRolePermissionIsInTheCatalog"),
        "must_contain": "backupp:read",
    },
    {
        "name": "控制台路由表改挂角色名（打开第二条授权轴）",
        "file": ROUTER_TS,
        "old": '      "ipam_spaces": {\n            "icon": "mdi:earth",\n            "order": 1,\n            "permission": {\n',
        "new": '      "ipam_spaces": {\n            "icon": "mdi:earth",\n            "order": 1,\n'
               '            "roles": ["admin"],\n            "permission": {\n',
        "check": guard("TestTheConsoleRouteTableCarriesNoRoleGate"),
        "must_contain": '"roles"',
    },
    {
        "name": "组件里按角色名判断（第三条授权轴，无声明可查）",
        "file": GUARD_PERM,
        "old": "    const authStore = useAuthStore();\n",
        "new": "    const authStore = useAuthStore();\n"
               "    const isAdmin = authStore.userInfo.roles.includes('admin');\n",
        "check": guard("TestTheConsoleNeverDecidesFromARoleName"),
        "must_contain": "roles.includes('admin')",
    },
    {
        "name": "权限守卫写了但没装上",
        "file": GUARD_INDEX,
        "old": "  createPermissionGuard(router);\n",
        "new": "  // createPermissionGuard(router);\n",
        "check": guard("TestTheConsoleGateIsInstalled"),
        "must_contain": "does not register createPermissionGuard",
    },

    # --- too tight: an extraction that stops reading must not pass ----------

    {
        "name": "控制台权限块换了字段顺序（抽取必须报错而不是少读）",
        "file": ROUTER_TS,
        "old": '      "dns_zones": {\n            "icon": "mdi:dns",\n            "order": 1,\n'
               '            "permission": {\n                  "resource": "dns",\n'
               '                  "action": "read"\n            }\n      },\n',
        "new": '      "dns_zones": {\n            "icon": "mdi:dns",\n            "order": 1,\n'
               '            "permission": {\n                  "action": "read",\n'
               '                  "resource": "dns"\n            }\n      },\n',
        "check": guard("TestEveryConsoleRoutePermissionIsCheckedByAnAPIRoute"),
        "must_contain": "reshaped",
    },
    {
        "name": "路由的权限名抽成了常量（抽取必须报错而不是少读）",
        "file": ROUTER_GO,
        "old": '\t\t\tr.Route("/logs", func(r chi.Router) {\n'
               '\t\t\t\tr.With(rbac.RequirePermission(rbacMgr, "audit", "read")).Get("/audit", h.ListAuditLogs)\n'
               '\t\t\t\tr.With(rbac.RequirePermission(rbacMgr, "audit", "read")).Get("/login", h.ListLoginHistory)\n',
        "new": '\t\t\tr.Route("/logs", func(r chi.Router) {\n'
               '\t\t\t\tconst auditResource = "audit"\n'
               '\t\t\t\tr.With(rbac.RequirePermission(rbacMgr, auditResource, "read")).Get("/audit", h.ListAuditLogs)\n'
               '\t\t\t\tr.With(rbac.RequirePermission(rbacMgr, "audit", "read")).Get("/login", h.ListLoginHistory)\n',
        "check": guard("TestEveryConsoleRoutePermissionIsCheckedByAnAPIRoute"),
        "must_contain": "reshaped",
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
                mutation["check"], cwd=ROOT,
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
