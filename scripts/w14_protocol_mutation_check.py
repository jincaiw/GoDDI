#!/usr/bin/env python3
"""Mutation check for the protocol counter-example guards (W14-a).

The v0.6.0 exit criteria require the protocol counter-examples to pass, and for
"malformed packet" the correct behaviour is *nothing at all*. Every assertion
there is negative -- no reply, no lease, no panic, a loop still reading -- and a
negative assertion is the kind that passes for the wrong reason. A server that
dropped everything, or a loop that had already died, would look exactly as
green.

So each defence is removed here, one at a time, and the check has to fail:

  * the parse-error `continue` becomes `return` -- the receive loop stops at the
    first datagram it cannot read;
  * the opcode check is deleted -- the server acts on a message that says it is
    a reply;
  * a missing message type is guessed to be DISCOVER -- the dispatcher invents a
    request out of a packet that carries none;
  * the non-IPv4 guard is deleted -- a REQUEST naming an IPv6 address is
    answered.

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

SERVER_GO = "internal/dhcp/server/server.go"
HANDLER_GO = "internal/dhcp/server/handler.go"

PKG = ["go", "test", "./internal/dhcp/server/", "-count=1", "-run"]


def check(name):
    return PKG + [name]


MUTATIONS = [
    {
        "name": "收包循环在第一个读不懂的数据报上停下（continue → return）",
        "file": SERVER_GO,
        "old": '\t\tmsg, err := dhcpv4.FromBytes(buf[:n])\n'
               '\t\tif err != nil {\n'
               '\t\t\tslog.Debug("DHCP server: failed to parse packet", "error", err)\n'
               '\t\t\tcontinue\n'
               '\t\t}\n',
        "new": '\t\tmsg, err := dhcpv4.FromBytes(buf[:n])\n'
               '\t\tif err != nil {\n'
               '\t\t\tslog.Debug("DHCP server: failed to parse packet", "error", err)\n'
               '\t\t\treturn\n'
               '\t\t}\n',
        "check": check("TestTheReceiveLoopSurvivesDatagramsItCannotParse"),
        "must_contain": "TestTheReceiveLoopSurvivesDatagramsItCannotParse",
    },
    {
        "name": "删掉 op 码校验（服务一个自称是回复的报文）",
        "file": SERVER_GO,
        "old": '\tif msg.OpCode != dhcpv4.OpcodeBootRequest {\n'
               '\t\tslog.Warn("DHCP: dropping a message whose opcode is not BOOTREQUEST",\n'
               '\t\t\t"opcode", uint8(msg.OpCode), "interface", ifaceName,\n'
               '\t\t\t"client", mac, "msg_type", msgType.String())\n'
               '\t\treturn\n'
               '\t}\n\n',
        "new": "",
        "check": check("TestAMessageThatSaysItIsAReplyIsDroppedInSilence"),
        "must_contain": "TestAMessageThatSaysItIsAReplyIsDroppedInSilence",
    },
    {
        "name": "报文没有消息类型时猜成 DISCOVER",
        "file": SERVER_GO,
        "old": '\tmsgType := msg.MessageType()\n\n\tmac := msg.ClientHWAddr.String()\n',
        "new": '\tmsgType := msg.MessageType()\n'
               '\tif msgType == dhcpv4.MessageType(0) {\n'
               '\t\tmsgType = dhcpv4.MessageTypeDiscover\n'
               '\t}\n\n\tmac := msg.ClientHWAddr.String()\n',
        "check": check("TestAMessageWithNoTypeOrAnUnknownTypeLeavesNothingBehind"),
        "must_contain": "TestAMessageWithNoTypeOrAnUnknownTypeLeavesNothingBehind",
    },
    {
        "name": "删掉「地址不是 IPv4 就丢弃」的守卫",
        "file": HANDLER_GO,
        "old": '\trequestedIP4 := requestedIP.To4()\n'
               '\tif requestedIP4 == nil {\n'
               '\t\t// IPv6 in a DHCPv4 message is malformed; dropping is safer than NAKing\n'
               '\t\t// a packet we cannot interpret.\n'
               '\t\tslog.Debug("DHCP: REQUEST with a non-IPv4 address, ignoring", "mac", mac, "ip", requestedIP)\n'
               '\t\treturn nil, nil\n'
               '\t}\n'
               '\trequestedIP = requestedIP4\n',
        "new": '\tif requestedIP4 := requestedIP.To4(); requestedIP4 != nil {\n'
               '\t\trequestedIP = requestedIP4\n'
               '\t}\n',
        "check": check("TestTheNonIPv4GuardRefusesWhatTheWireCannotDeliver"),
        "must_contain": "TestTheNonIPv4GuardRefusesWhatTheWireCannotDeliver",
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

        if mutation["new"] and mutation["new"] in original:
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
