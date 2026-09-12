#!/usr/bin/env python3
"""Mutation check for the branch cases added in W14-b.

W14-b's deliverable is an enumeration of key branches, which is only worth
anything if each case fails when the branch is removed. So this script removes
the branch behind each of the four defects W14-b found, one at a time, and
requires the named case to be the thing that notices.

The four, and what removing the branch restores:

  * DeleteScope's guard goes back to `status = 'active'` on its own -- an
    expired-but-unswept row blocks deletion again, and the node that runs no
    data plane cannot delete the scope.
  * DeleteScope's cleanup goes back to the narrower WHERE clause -- a lapsed
    offer and a lapsed quarantine are left pointing at a scope that no longer
    exists.
  * DeleteSpace's subnet count goes back to discarding its error -- a check that
    could not run reads as "this space is empty" and the delete goes ahead.
  * DeleteSpace's guard is inverted -- a space with subnets is deleted, and the
    schema's ON DELETE CASCADE takes the subtree with it.
  * The reservation duplicate-address check is made unable to match -- two MACs
    end up on one address.
  * The reservation input check is loosened from "or" to "and" -- a row with one
    missing field reaches NOT NULL and surfaces as a 500 instead of a 400.
  * The reservation layer is dropped from the option priority ladder -- a client
    with a reservation is told the scope's value, and the reservation silently
    stops meaning anything.
  * The Parameter Request List filter is dropped from BuildOptions -- every
    client is told every option, including the ones it did not ask for.
  * The reverse-root filter is dropped from the reverse zone plan -- the
    console offers in-addr.arpa / ip6.arpa as things to create.
  * CreateScope stops normalising its comment -- an omitted comment and an
    emptied one are stored with different spellings again.

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

SCOPE_GO = "internal/dhcp/scope/scope.go"
SPACE_GO = "internal/ipam/space/space.go"
RESERVATION_GO = "internal/dhcp/reservation/reservation.go"
OPTION_GO = "internal/dhcp/option/option.go"
OPTION_BUILDER_GO = "internal/dhcp/option/builder.go"
SUBNET_GO = "internal/ipam/subnet/subnet.go"

CHECK = [
    "go", "test",
    "./internal/dhcp/scope/", "./internal/dhcp/reservation/", "./internal/dhcp/option/",
    "./internal/ipam/space/", "./internal/ipam/subnet/",
    "-count=1",
]

MUTATIONS = [
    {
        "name": "DeleteScope 的守卫退回只看状态（过期但未清扫的行重新挡住删除）",
        "file": SCOPE_GO,
        "old": "const liveBinding = `scope_id = ? AND status = 'active' AND julianday(lease_end) > julianday('now')`",
        "new": "const liveBinding = `scope_id = ? AND status = 'active'`",
        "must_contain": "TestDeletingIsNotBlockedByABindingThatAlreadyRanOut",
    },
    {
        "name": "DeleteScope 的清理退回只删过期与已释放（滞留的 offer 与隔离期变成孤儿行）",
        "file": SCOPE_GO,
        "old": '\tif _, err := tx.Exec("DELETE FROM dhcp_leases WHERE scope_id = ?", id); err != nil {\n\t\treturn fmt.Errorf("failed to delete leases: %w", err)\n\t}',
        "new": '\tif _, err := tx.Exec("DELETE FROM dhcp_leases WHERE scope_id = ? AND (julianday(lease_end) <= julianday(\'now\') OR status = \'released\')", id); err != nil {\n\t\treturn fmt.Errorf("failed to delete leases: %w", err)\n\t}',
        "must_contain": "TestDeletingTakesTheAssociationsWithIt",
    },
    {
        "name": "DeleteSpace 再次吞掉子网计数查询的错误（读不出来＝空的）",
        "file": SPACE_GO,
        "old": '\tif err := m.db.QueryRow("SELECT COUNT(*) FROM ipam_subnets WHERE space_id = ?", id).Scan(&subnetCount); err != nil {\n\t\treturn fmt.Errorf("failed to check for subnets in space %s: %w", id, err)\n\t}',
        "new": '\t_ = m.db.QueryRow("SELECT COUNT(*) FROM ipam_subnets WHERE space_id = ?", id).Scan(&subnetCount)',
        "must_contain": "TestACheckThatCannotRunIsNotAnEmptyAnswer",
    },
    {
        "name": "DeleteSpace 的守卫反过来判断（带子网的空间被删，级联带走整棵子树）",
        "file": SPACE_GO,
        "old": "\tif subnetCount > 0 {",
        "new": "\tif subnetCount < 0 {",
        "must_contain": "TestASpaceThatStillHasSubnetsIsNotDeleted",
    },
    {
        "name": "预约的地址重复检查改成永不匹配（两个 MAC 占同一地址）",
        "file": RESERVATION_GO,
        "old": "WHERE scope_id = ? AND ip_address = ? AND enabled = 1 LIMIT 1",
        "new": "WHERE scope_id = ? AND ip_address = ? AND enabled = 1 AND 1 = 0 LIMIT 1",
        "must_contain": "TestAnAddressCannotBeReservedTwiceInTheSameScope",
    },
    {
        "name": "预约的必填校验从「或」放宽成「且」（缺一个字段的请求落到 NOT NULL 上）",
        "file": RESERVATION_GO,
        "old": '\tif ip == "" || mac == "" {',
        "new": '\tif ip == "" && mac == "" {',
        "must_contain": "TestMissingFieldsAreRefusedRatherThanStoredBlank",
    },
    {
        "name": "选项优先级阶梯里抽掉预约层（有预约的客户端拿到作用域的值）",
        "file": OPTION_GO,
        "old": '\tif reservationID != "" {\n\t\tresOpts, err := m.GetOptionsByReservation(reservationID)',
        "new": '\tif false && reservationID != "" {\n\t\tresOpts, err := m.GetOptionsByReservation(reservationID)',
        "must_contain": "TestTheMostSpecificLayerWins",
    },
    {
        "name": "BuildOptions 不再按 Parameter Request List 过滤（客户端拿到它没要的选项）",
        "file": OPTION_BUILDER_GO,
        "old": '\t\tif len(requestedSet) > 0 && !requestedSet[uint8(code)] {\n\t\t\tcontinue\n\t\t}',
        "new": '\t\tif false {\n\t\t\tcontinue\n\t\t}',
        "must_contain": "TestTheClientGetsWhatItAskedForAndNothingMore",
    },
    {
        "name": "反向区域计划不再滤掉反向树根（把 in-addr.arpa 也拿出来当可创建的区域）",
        "file": SUBNET_GO,
        "old": '\t\tif c == "in-addr.arpa" || c == "ip6.arpa" {\n\t\t\tcontinue\n\t\t}',
        "new": '\t\tif false {\n\t\t\tcontinue\n\t\t}',
        "must_contain": "TestThePlanOffersTheDelegationsButNotTheRoot",
    },
    {
        "name": "CreateScope 不再归一化 comment（未填与清空又存成两种写法）",
        "file": SCOPE_GO,
        "old": '\t\tdnsUpdates, nullIfEmpty(comment), now, now)',
        "new": '\t\tdnsUpdates, comment, now, now)',
        "must_contain": "TestAScopeRoundTripsAndDefaultsAreExplicit",
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
                print(f"ok   {mutation['name']}：{named} 确实失败了")
            else:
                report(f"FAIL {mutation['name']}：注入缺陷后 {named} 仍然通过（或失败理由不对），"
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
    print(f"\n每处变异都被相应的检查捕获（{len(MUTATIONS)}/{len(MUTATIONS)}）")

    return 0


if __name__ == "__main__":
    sys.exit(main())
