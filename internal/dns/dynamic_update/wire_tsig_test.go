package dynamic_update

import (
	"errors"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// compressedSignedUpdate mimics a real client: the wire is compressed before
// the TSIG MAC is computed. Unpacking and re-packing such a message inside
// the handler produces different bytes, so a re-packed verification fails.
func compressedSignedUpdate(t *testing.T) *dns.Msg {
	t.Helper()
	m := new(dns.Msg)
	m.SetUpdate(testZoneName)
	m.Insert([]dns.RR{aRR("wire.example.com.", "192.0.2.40", 300)})
	m.SetTsig(testKeyName, dns.HmacSHA256, 300, time.Now().Unix())
	m.Compress = true
	wire, _, err := dns.TsigGenerate(m, testSecret, "", false)
	if err != nil {
		t.Fatalf("sign update: %v", err)
	}
	var parsed dns.Msg
	if err := parsed.Unpack(wire); err != nil {
		t.Fatalf("unpack signed update: %v", err)
	}
	return &parsed
}

func TestHandleUpdateFromUsesListenerTSIGStatusForCompressedWire(t *testing.T) {
	h, db := newUpdateHandler(t)
	msg := compressedSignedUpdate(t)

	// A nil status means the listener verified the MAC against the raw wire.
	resp, err := h.HandleUpdateFrom(msg, "192.0.2.1", nil)
	if err != nil {
		t.Fatalf("HandleUpdateFrom: %v", err)
	}
	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}
	if n := countRecords(t, db, "wire.example.com.", "A"); n != 1 {
		t.Fatalf("records after signed update = %d, want 1", n)
	}
}

func TestHandleUpdateFromRefusesWhenListenerTSIGStatusIsAnError(t *testing.T) {
	h, db := newUpdateHandler(t)
	msg := compressedSignedUpdate(t)

	resp, err := h.HandleUpdateFrom(msg, "192.0.2.1", errors.New("dns: bad signature"))
	if err != nil {
		t.Fatalf("HandleUpdateFrom: %v", err)
	}
	if resp.Rcode != dns.RcodeRefused {
		t.Fatalf("rcode = %s, want REFUSED", dns.RcodeToString[resp.Rcode])
	}
	if n := countRecords(t, db, "wire.example.com.", "A"); n != 0 {
		t.Fatalf("a rejected update wrote %d records", n)
	}
}

// Pins the reason the listener status is required: re-packing a compressed
// request cannot reproduce the bytes the client signed.
func TestHandleUpdateFromFallbackRejectsCompressedWire(t *testing.T) {
	h, db := newUpdateHandler(t)
	msg := compressedSignedUpdate(t)

	resp, err := h.HandleUpdateFrom(msg, "192.0.2.1")
	if err != nil {
		t.Fatalf("HandleUpdateFrom: %v", err)
	}
	if resp.Rcode != dns.RcodeRefused {
		t.Fatalf("rcode = %s, want REFUSED (re-packed MAC cannot match)", dns.RcodeToString[resp.Rcode])
	}
	if n := countRecords(t, db, "wire.example.com.", "A"); n != 0 {
		t.Fatalf("a rejected update wrote %d records", n)
	}
}
