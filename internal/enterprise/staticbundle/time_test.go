package staticbundle

import (
	"bytes"
	"math"
	"strconv"
	"testing"

	"github.com/jincaiw/GoDDI/internal/enterprise/auditwire"
	"github.com/jincaiw/GoDDI/internal/enterprise/eventlog"
	"github.com/jincaiw/GoDDI/internal/enterprise/receiptwire"
)

func TestWindowBoundaryAndOverflow(t *testing.T) {
	for _, tc := range []struct {
		name            string
		created, expiry int64
		valid           bool
	}{
		{"exact", 100, 100 + eventlog.StaticReceiptMinRetentionMS, true},
		{"short", 100, 99 + eventlog.StaticReceiptMinRetentionMS, false},
		{"max exact", math.MaxInt64 - eventlog.StaticReceiptMinRetentionMS, math.MaxInt64, true},
		{"near max short", math.MaxInt64 - eventlog.StaticReceiptMinRetentionMS + 1, math.MaxInt64, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, c := fixture(t)
			c[0].Body = bytes.Replace(c[0].Body, []byte(`"createdAtMs":"100"`), []byte(`"createdAtMs":"`+strconv.FormatInt(tc.created, 10)+`"`), 1)
			a, err := auditwire.Decode(c[1].Body)
			if err != nil {
				t.Fatal(err)
			}
			a.OccurredAtMS = tc.created
			c[1].Body, err = auditwire.Encode(a)
			if err != nil {
				t.Fatal(err)
			}
			r, err := receiptwire.Decode(c[2].Body)
			if err != nil {
				t.Fatal(err)
			}
			r.Receipt.CreatedAtMS = tc.created
			r.Receipt.ExpiresAtMS = tc.expiry
			c[2].Body, err = receiptwire.Encode(r)
			if err != nil {
				t.Fatal(err)
			}
			e.CommitMs = tc.created
			e = rehash(t, e, c)
			_, err = Validate(e)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
		})
	}
}
