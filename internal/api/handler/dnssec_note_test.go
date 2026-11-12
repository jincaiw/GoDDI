package handler

import (
	"strings"
	"testing"
)

// The DNSSEC warning is safety-critical operator copy. It used to claim that
// authoritative answers carried no RRSIG/DNSKEY at all, which became false
// once positive-answer and NSEC/NSEC3 denial signing landed. Pin both halves:
// the capability that now exists, and the boundary that still forbids
// production use. A regression in either direction misleads an operator.
func TestDNSSECExperimentalNoteStatesSignedAnswersAndMissingParentDS(t *testing.T) {
	note := dnssecExperimentalNote
	for _, want := range []string{"已签名", "NSEC/NSEC3", "父区 DS", "实验功能", "生产就绪"} {
		if !strings.Contains(note, want) {
			t.Fatalf("dnssecExperimentalNote %q no longer states %q", note, want)
		}
	}
	for _, stale := range []string{
		"尚未携带 RRSIG",
		"不生成任何 RRSIG",
		"尚未生成签名",
		"待真实签名实现",
		"仅标记区域",
	} {
		if strings.Contains(note, stale) {
			t.Fatalf("dnssecExperimentalNote %q regressed to the stale claim %q", note, stale)
		}
	}
}
