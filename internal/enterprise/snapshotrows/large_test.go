package snapshotrows

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/jasonwa/goddi/internal/enterprise/idempotency"
)

const transportChunk = 1 << 20

func feedChunks(v *Validator, data []byte) error {
	for len(data) > 0 {
		n := min(len(data), transportChunk)
		if err := v.Feed(data[:n]); err != nil {
			return err
		}
		data = data[n:]
	}
	return nil
}
func TestMaxReceiptAcrossTransportChunks(t *testing.T) {
	body := bytes.Repeat([]byte{0, 0xff, 'x'}, (idempotency.MaxBodyBytes+2)/3)[:idempotency.MaxBodyBytes]
	etag := strings.Repeat("e", idempotency.MaxETagBytes)
	response := make([]byte, 8+len(etag)+len(body))
	copy(response, "GIR1")
	binary.BigEndian.PutUint16(response[4:6], 599)
	binary.BigEndian.PutUint16(response[6:8], uint16(len(etag)))
	copy(response[8:], etag)
	copy(response[8+len(etag):], body)
	row := map[string]string{"tenant_id": base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, 16)), "principal": strings.Repeat("p", idempotency.MaxPrincipalBytes), "method": "POST", "path": "/" + strings.Repeat("a", idempotency.MaxPathBytes-1), "idempotency_key": "11111111-1111-4111-8111-111111111111", "request_digest": base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32)), "response": base64.RawURLEncoding.EncodeToString(response), "created_at_unix_ms": "9223372036854775806", "expires_at_unix_ms": "9223372036854775807"}
	data, err := json.Marshal(map[string]any{"table": "idempotency_receipt", "row": row})
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if len(data) <= transportChunk || len(data) > MaxLineBytes {
		t.Fatalf("unexpected length %d", len(data))
	}
	called := 0
	validate := func(raw []byte) error {
		called++
		var got map[string]string
		if e := json.Unmarshal(raw, &got); e != nil {
			return e
		}
		decoded, e := base64.RawURLEncoding.Strict().DecodeString(got["response"])
		if e != nil {
			return e
		}
		if !bytes.Equal(decoded, response) {
			return errors.New("receipt bytes changed")
		}
		return nil
	}
	l := Limits{LineBytes: MaxLineBytes, TotalBytes: uint64(len(data)), Rows: 1, Tables: 1}
	v, err := New([]Table{{Name: "idempotency_receipt", Count: 1, ValidateRow: validate}}, l)
	if err != nil {
		t.Fatal(err)
	}
	if err = feedChunks(v, data); err != nil {
		t.Fatal(err)
	}
	result, err := v.Finish()
	if err != nil || called != 1 || result.Bytes != uint64(len(data)) || !result.AllRowCallbacksPassed {
		t.Fatalf("result=%+v calls=%d err=%v", result, called, err)
	}
	l.TotalBytes--
	v, err = New([]Table{{Name: "idempotency_receipt", Count: 1, ValidateRow: validate}}, l)
	if err != nil {
		t.Fatal(err)
	}
	if err = feedChunks(v, data); err == nil {
		t.Fatal("accepted insufficient total budget")
	}
	if _, err = v.Finish(); !errors.Is(err, ErrDirty) {
		t.Fatalf("dirty: %v", err)
	}
	l.TotalBytes = uint64(len(data))
	l.LineBytes = 1 << 20
	v, err = New([]Table{{Name: "idempotency_receipt", Count: 1}}, l)
	if err != nil {
		t.Fatal(err)
	}
	if err = feedChunks(v, data); err == nil {
		t.Fatal("old explicit 1MiB line limit changed")
	}
}
func TestTwoMiBLineBoundaryAndLF(t *testing.T) {
	prefix := []byte(`{"table":"a","row":{"payload":"`)
	suffix := []byte(`"}}`)
	line := append(bytes.Clone(prefix), bytes.Repeat([]byte{'x'}, MaxLineBytes-len(prefix)-len(suffix))...)
	line = append(line, suffix...)
	l := Limits{LineBytes: MaxLineBytes, TotalBytes: uint64(MaxLineBytes + 1), Rows: 1, Tables: 1}
	v, err := New([]Table{{Name: "a", Count: 1}}, l)
	if err != nil {
		t.Fatal(err)
	}
	if err = feedChunks(v, append(bytes.Clone(line), '\n')); err != nil {
		t.Fatal(err)
	}
	result, err := v.Finish()
	if err != nil || result.Bytes != uint64(MaxLineBytes+1) {
		t.Fatalf("max line %v %v", result, err)
	}
	// One additional data byte is rejected; LF is excluded from the line limit.
	longer := append(bytes.Clone(line[:len(line)-len(suffix)]), 'x')
	longer = append(longer, suffix...)
	l.TotalBytes++
	v, err = New([]Table{{Name: "a", Count: 1}}, l)
	if err != nil {
		t.Fatal(err)
	}
	if err = feedChunks(v, append(longer, '\n')); err == nil {
		t.Fatal("2MiB+1 line accepted")
	}
	l.TotalBytes = MaxLineBytes
	v, err = New([]Table{{Name: "a", Count: 1}}, l)
	if err != nil {
		t.Fatal(err)
	}
	if err = feedChunks(v, line); err != nil {
		t.Fatal(err)
	}
	if err = v.Feed([]byte{'\n'}); err == nil {
		t.Fatal("LF omitted from total budget")
	}
}
func TestExtremeBoundsCannotOverflow(t *testing.T) {
	base := Limits{LineBytes: MaxLineBytes, TotalBytes: MaxTotalBytes, Rows: MaxRows, Tables: MaxTables}
	for _, bad := range []Limits{{LineBytes: math.MaxInt, TotalBytes: 1, Rows: 1, Tables: 1}, {LineBytes: 1, TotalBytes: math.MaxUint64, Rows: 1, Tables: 1}, {LineBytes: 1, TotalBytes: 1, Rows: math.MaxUint64, Tables: 1}, {LineBytes: 1, TotalBytes: 1, Rows: 1, Tables: math.MaxInt}} {
		if _, err := New([]Table{{Name: "a"}}, bad); err == nil {
			t.Fatalf("bounds accepted %+v", bad)
		}
	}
	if _, err := New([]Table{{Name: "a", Count: math.MaxUint64}}, base); err == nil {
		t.Fatal("overflowing row count accepted")
	}
}
