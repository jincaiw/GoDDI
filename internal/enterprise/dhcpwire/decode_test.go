package dhcpwire

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
)

func packet(typ byte, opts ...byte) []byte {
	p := make([]byte, 240)
	p[0] = 1
	p[1] = 1
	p[2] = 6
	binary.BigEndian.PutUint32(p[4:8], 42)
	copy(p[28:34], []byte{1, 2, 3, 4, 5, 6})
	copy(p[236:240], []byte{99, 130, 83, 99})
	p = append(p, 53, 1, typ)
	p = append(p, opts...)
	return append(p, 255, 0, 0)
}
func TestRelayAgentClaimsAndIndependentCopies(t *testing.T) {
	payload := []byte{9, 2, 7, 8, 2, 1, 4, 1, 2, 5, 6, 9, 1, 3}
	opts := append([]byte{61, 2, 1, 7, 82, byte(len(payload))}, payload...)
	p := packet(Discover, opts...)
	binary.BigEndian.PutUint16(p[10:12], 0x8001)
	// A direct packet may claim Option 82; ingress must decide trust.
	m, err := Decode(p)
	if err != nil || m.RelayInfo == nil || !m.Broadcast || m.Flags != 0x8001 || !bytes.Equal(m.ClientID, []byte{1, 7}) {
		t.Fatalf("%+v %v", m, err)
	}
	r := m.RelayInfo
	if !r.CircuitPresent || !r.RemotePresent || !bytes.Equal(r.Raw, payload) || !bytes.Equal(r.CircuitID, []byte{5, 6}) || !bytes.Equal(r.RemoteID, []byte{4}) || len(r.Suboptions) != 4 || r.Suboptions[0].Code != 9 || r.Suboptions[3].Code != 9 {
		t.Fatalf("%+v", r)
	}
	p[251] ^= 0xff
	if !bytes.Equal(r.Raw, payload) {
		t.Fatal("raw aliases datagram")
	}
	r.Suboptions[2].Value[0] = 99
	if r.CircuitID[0] != 5 || !bytes.Equal(r.Raw, payload) {
		t.Fatal("suboptions alias other returned data")
	}
	r.CircuitID[1] = 88
	if r.Suboptions[2].Value[1] != 6 {
		t.Fatal("circuit aliases suboption")
	}
}

func TestRelayAgentMalformed(t *testing.T) {
	for _, raw := range [][]byte{nil, {1}, {1, 0}, {1, 2, 9}, {1, 1, 9, 1, 1, 8}, {2, 1, 9, 2, 1, 8}, make([]byte, 256)} {
		if _, err := ParseRelayAgent(raw); !errors.Is(err, ErrMalformed) {
			t.Fatalf("accepted %v: %v", raw, err)
		}
	}
	for _, opts := range [][]byte{{82, 0}, {82, 2, 1, 3}, {82, 3, 1, 1, 9, 82, 3, 2, 1, 7}} {
		if _, err := Decode(packet(Discover, opts...)); !errors.Is(err, ErrMalformed) {
			t.Fatalf("accepted %v: %v", opts, err)
		}
	}
	withoutEnd := packet(Discover, 82, 3, 1, 1, 9)
	withoutEnd = withoutEnd[:len(withoutEnd)-3]
	if _, err := Decode(withoutEnd); !errors.Is(err, ErrMalformed) {
		t.Fatal("missing END accepted")
	}
	r, err := ParseRelayAgent([]byte{99, 1, 7})
	if err != nil || r.CircuitPresent || r.RemotePresent {
		t.Fatalf("unknown: %+v %v", r, err)
	}
}

func TestBroadcastFlagPreservesReservedBits(t *testing.T) {
	plain := packet(Discover)
	binary.BigEndian.PutUint16(plain[10:12], 0x1234)
	broadcast := append([]byte(nil), plain...)
	binary.BigEndian.PutUint16(broadcast[10:12], 0x9234)
	a, err := Decode(plain)
	if err != nil || a.Flags != 0x1234 || a.Broadcast {
		t.Fatalf("plain flags=%#x broadcast=%v err=%v", a.Flags, a.Broadcast, err)
	}
	b, err := Decode(broadcast)
	if err != nil || b.Flags != 0x9234 || !b.Broadcast {
		t.Fatalf("broadcast flags=%#x broadcast=%v err=%v", b.Flags, b.Broadcast, err)
	}
}

func TestVectors(t *testing.T) {
	ip := []byte{50, 4, 192, 0, 2, 42}
	sid := []byte{54, 4, 192, 0, 2, 1}
	selectOpts := append(append([]byte{}, ip...), sid...)
	for _, tc := range []struct {
		name string
		typ  byte
		opts []byte
		ci   bool
	}{
		{"discover", Discover, nil, false}, {"discover requested", Discover, ip, false}, {"selecting", Request, selectOpts, false}, {"init reboot", Request, ip, false}, {"renew", Request, nil, true}, {"decline", Decline, selectOpts, false}, {"release", Release, sid, true}, {"inform", Inform, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := packet(tc.typ, tc.opts...)
			if tc.ci {
				copy(p[12:16], []byte{192, 0, 2, 42})
			}
			copy(p[24:28], []byte{198, 51, 100, 1})
			m, e := Decode(p)
			if e != nil || m.Type != tc.typ || m.XID != 42 || m.HardwareHex != "010203040506" || m.GIAddr.String() != "198.51.100.1" {
				t.Fatalf("%+v %v", m, e)
			}
		})
	}
}
func TestClientIdentityAndClone(t *testing.T) {
	p := packet(Discover, 61, 3, 1, 2, 3)
	a, e := Decode(p)
	if e != nil {
		t.Fatal(e)
	}
	q := packet(Discover, 61, 3, 1, 2, 3)
	copy(q[28:34], []byte{9, 9, 9, 9, 9, 9})
	b, e := Decode(q)
	if e != nil || !bytes.Equal(a.ClientKey, b.ClientKey) {
		t.Fatal("client id must precede hardware")
	}
	p[28] = 99
	p[245] = 99
	if a.HardwareAddress[0] != 1 || a.ClientID[0] != 1 || a.ClientKey[1] != 1 {
		t.Fatal("input alias")
	}
	h1, e := Decode(packet(Discover))
	if e != nil {
		t.Fatal(e)
	}
	h := packet(Discover)
	h[1] = 2
	h2, e := Decode(h)
	if e != nil || bytes.Equal(h1.ClientKey, h2.ClientKey) || bytes.Equal(h1.ClientKey, a.ClientKey) {
		t.Fatal("identity namespace collision")
	}
	// Scope is intentionally not part of this key: callers must bind it separately.
}
func TestRejectMalformed(t *testing.T) {
	good := packet(Discover)
	req := []byte{50, 4, 192, 0, 2, 42}
	sid := []byte{54, 4, 192, 0, 2, 1}
	cases := [][]byte{nil, good[:239], packet(Discover, 53, 1, Discover), packet(Discover, 50, 4, 192, 0, 2, 1, 50, 4, 192, 0, 2, 2), packet(Discover, 61, 1, 1), packet(Discover, 61, 2, 1, 2, 61, 2, 1, 2), packet(Discover, 51, 3, 0, 0, 0), packet(Discover, 52, 1, 1), packet(Discover, 82, 2, 1, 0), packet(Request), packet(Request, sid...), packet(Decline, req...), packet(Release, sid...), packet(Inform), packet(2)}
	truncated := packet(Discover)
	truncated = truncated[:len(truncated)-3]
	cases = append(cases, truncated)
	extra := append(packet(Discover), 1)
	cases = append(cases, extra)
	huge := make([]byte, MaxPayload+1)
	copy(huge, good)
	cases = append(cases, huge)
	for _, mutate := range []func([]byte){func(p []byte) { p[0] = 2 }, func(p []byte) { p[1] = 0 }, func(p []byte) { p[2] = 0 }, func(p []byte) { p[2] = 17 }, func(p []byte) { p[2] = 5 }, func(p []byte) { p[3] = 17 }, func(p []byte) { clear(p[4:8]) }, func(p []byte) { p[236] = 0 }} {
		q := append([]byte(nil), good...)
		mutate(q)
		cases = append(cases, q)
	}
	for i, p := range cases {
		if m, e := Decode(p); !errors.Is(e, ErrMalformed) || m.XID != 0 {
			t.Fatalf("case %d accepted %+v %v", i, m, e)
		}
	}
}
func FuzzDecode(f *testing.F) {
	f.Add(packet(Discover))
	f.Add([]byte{})
	f.Add(packet(Request, 50, 4, 192, 0, 2, 1))
	f.Fuzz(func(t *testing.T, p []byte) {
		m, e := Decode(p)
		if e == nil {
			if m.XID == 0 || len(m.ClientKey) == 0 || len(p) > MaxPayload {
				t.Fatalf("invalid success %+v", m)
			}
		}
	})
}
