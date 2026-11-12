package replicabatch

import (
	"fmt"
	"testing"

	"github.com/jasonwa/goddi/internal/enterprise/eventlog"
)

// TestVerifyRejectsAnySingleByteTamper flips every byte of every event's
// payload (bounded), previous hash and stored hash in an otherwise valid batch
// and asserts the batch is rejected. This is the tamper-evidence property: no
// single-byte change to a verified chain may be accepted.
func TestVerifyRejectsAnySingleByteTamper(t *testing.T) {
	expected, events := fixture(t, 2)
	if _, err := Verify(expected, events); err != nil {
		t.Fatalf("baseline batch invalid: %v", err)
	}

	tamper := func(name string, mutate func([]eventlog.Event)) {
		cp := make([]eventlog.Event, len(events))
		for i := range events {
			cp[i] = events[i]
			cp[i].Payload = append([]byte(nil), events[i].Payload...)
		}
		mutate(cp)
		if b, err := Verify(expected, cp); err == nil || b != nil {
			t.Fatalf("%s: tampered batch was accepted", name)
		}
	}

	for ei := range events {
		ei := ei
		limit := len(events[ei].Payload)
		if limit > 48 {
			limit = 48
		}
		for bi := 0; bi < limit; bi++ {
			bi := bi
			tamper(fmt.Sprintf("event%d payload[%d]", ei, bi), func(cp []eventlog.Event) { cp[ei].Payload[bi] ^= 0xff })
		}
		for bi := 0; bi < 32; bi++ {
			bi := bi
			tamper(fmt.Sprintf("event%d prevHash[%d]", ei, bi), func(cp []eventlog.Event) { cp[ei].PrevHash[bi] ^= 0xff })
			tamper(fmt.Sprintf("event%d hash[%d]", ei, bi), func(cp []eventlog.Event) { cp[ei].Position.Hash[bi] ^= 0xff })
		}
	}
}
