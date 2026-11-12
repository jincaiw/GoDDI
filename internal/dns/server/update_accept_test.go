package server

import (
	"testing"

	"github.com/miekg/dns"
)

func updateHeader(opcode int, qr bool, qd, an, ns, ar uint16) dns.Header {
	bits := uint16(opcode) << 11
	if qr {
		bits |= 1 << 15
	}
	return dns.Header{Bits: bits, Qdcount: qd, Ancount: an, Nscount: ns, Arcount: ar}
}

func TestMsgAcceptFuncAllowsDynamicUpdates(t *testing.T) {
	// A normal UPDATE: one zone question and one record in the update
	// section (which miekg maps to the Answer section for this opcode).
	h := updateHeader(dns.OpcodeUpdate, false, 1, 1, 0, 0)
	if got := msgAcceptFunc(h); got != dns.MsgAccept {
		t.Fatalf("UPDATE accept action = %v, want MsgAccept (the library default rejects it with NOTIMP)", got)
	}
	// Prerequisites live in the NS section and may contain several RRs.
	h = updateHeader(dns.OpcodeUpdate, false, 1, 2, 3, 1)
	if got := msgAcceptFunc(h); got != dns.MsgAccept {
		t.Fatalf("UPDATE with prerequisites = %v, want MsgAccept", got)
	}
}

func TestMsgAcceptFuncBoundsUpdateSectionCounts(t *testing.T) {
	cases := []struct {
		name           string
		qd, an, ns, ar uint16
	}{
		{"no zone question", 0, 1, 0, 0},
		{"too many answer records", 1, maxUpdateAnswerRecords + 1, 0, 0},
		{"too many authority records", 1, 1, maxUpdateAuthorityRecords + 1, 0},
		{"too many extra records", 1, 1, 0, maxUpdateExtraRecords + 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := updateHeader(dns.OpcodeUpdate, false, tc.qd, tc.an, tc.ns, tc.ar)
			if got := msgAcceptFunc(h); got != dns.MsgReject {
				t.Errorf("accept action = %v, want MsgReject", got)
			}
		})
	}
}

func TestMsgAcceptFuncKeepsLibraryDefaultsForOtherOpcodes(t *testing.T) {
	// QUERY and NOTIFY must behave exactly as the library's default.
	for _, opcode := range []int{dns.OpcodeQuery, dns.OpcodeNotify} {
		h := updateHeader(opcode, false, 1, 0, 0, 0)
		if got, want := msgAcceptFunc(h), dns.DefaultMsgAcceptFunc(h); got != want {
			t.Errorf("opcode %d action = %v, library default = %v", opcode, got, want)
		}
	}
	// An unsupported opcode is still rejected with NOTIMP by the library.
	h := updateHeader(2 /* STATUS */, false, 1, 0, 0, 0)
	if got := msgAcceptFunc(h); got != dns.MsgRejectNotImplemented {
		t.Errorf("unsupported opcode action = %v, want MsgRejectNotImplemented", got)
	}
	// A response is never a request.
	h = updateHeader(dns.OpcodeUpdate, true, 1, 1, 0, 0)
	if got := msgAcceptFunc(h); got != dns.MsgIgnore {
		t.Errorf("response action = %v, want MsgIgnore", got)
	}
}
