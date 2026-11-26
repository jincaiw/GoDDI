// Package snapshotipam validates eight candidate snapshot row shapes and
// primary-key order. The fixed eight-table order replaces prior isolated six-,
// five-, and four-table candidates; these experimental formats are incompatible even when
// all are labeled schema=1. A production protocol must freeze the format and
// define version negotiation and migration; no backward compatibility is implied.
// It does not validate history completeness, provenance, cross-table or runtime
// invariants, or authorize snapshot activation.
package snapshotipam

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/jincaiw/GoDDI/internal/enterprise/snapshotpipeline"
)

var ErrInvalid = errors.New("snapshotipam: invalid row")
var ErrDirty = errors.New("snapshotipam: validator permanently failed")

const maxRowBytes = 2 << 20
const maxTextBytes = 4096

type Row struct {
	Table   string
	Payload []byte
}
type Validator struct {
	callbackMu sync.Mutex
	mu         sync.Mutex
	previous   map[string][]string
	dirty      bool
}

func New() *Validator { return &Validator{previous: make(map[string][]string)} }

var definitions = map[string][]string{
	"ipam_tenants":           {"id", "slug", "display_name", "revision"},
	"ipam_spaces":            {"tenant_id", "id", "slug", "display_name", "revision"},
	"ipam_prefixes":          {"tenant_id", "space_id", "id", "parent_id", "family", "prefix_len", "first_ip", "last_ip", "kind", "usage", "site_ref", "label", "revision"},
	"ipam_pools":             {"tenant_id", "space_id", "id", "subnet_id", "family", "pool_kind", "first_ip", "last_ip", "label", "revision"},
	"ipam_occupancies":       {"tenant_id", "space_id", "id", "subnet_id", "family", "address", "kind", "owner_ref", "expires_at_ms", "created_at_ms", "revision"},
	"ipam_resource_ids":      {"kind", "tenant_id", "space_id", "id"},
	"static_occupancy_audit": {"cluster_id", "epoch", "tenant_id", "space_id", "resource_id", "principal", "action", "request_id", "prior_revision", "new_revision", "result", "occurred_at_ms", "request_digest"},
	"idempotency_receipt":    {"tenant_id", "principal", "method", "path", "idempotency_key", "request_digest", "response", "created_at_unix_ms", "expires_at_unix_ms"},
}

// Active entities precede the ledger so staging triggers can register IDs;
// the staging layer must verify all active IDs and append historical keys.
var order = []string{"ipam_tenants", "ipam_spaces", "ipam_prefixes", "ipam_pools", "ipam_occupancies", "ipam_resource_ids", "static_occupancy_audit", "idempotency_receipt"}

// Validate returns independently owned original row JSON after validating every
// field and strict SQLite BINARY primary-key order within its table. The input
// must not be concurrently modified. An error permanently invalidates v.
func (v *Validator) Validate(table string, raw []byte) (Row, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.dirty {
		return Row{}, ErrDirty
	}
	result, key, err := validate(table, raw)
	if err != nil {
		v.dirty = true
		return Row{}, err
	}
	if old, ok := v.previous[table]; ok && compare(old, key) >= 0 {
		v.dirty = true
		return Row{}, fmt.Errorf("%w: primary key order", ErrInvalid)
	}
	v.previous[table] = append([]string(nil), key...)
	return result, nil
}
func compare(a, b []string) int {
	for i := range a {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

// Tables returns fixed-order pipeline callbacks. A sink is trusted caller code
// and may write only disposable staging; no result means snapshot is ready.
// Callbacks serialize validation and sink together; do not concurrently mix
// direct Validate calls with callbacks, or reenter the validator from a sink.
func (v *Validator) Tables(sink func(Row) error) []snapshotpipeline.SchemaTable {
	tables := make([]snapshotpipeline.SchemaTable, 0, len(order))
	for _, name := range order {
		table := name
		tables = append(tables, snapshotpipeline.SchemaTable{Name: table, ValidateRow: func(raw []byte) (err error) {
			v.callbackMu.Lock()
			defer v.callbackMu.Unlock()
			returned := false
			defer func() {
				// Never recover: nil panics and runtime.Goexit must propagate
				// naturally after marking the validator unusable.
				if !returned {
					v.mu.Lock()
					v.dirty = true
					v.mu.Unlock()
				}
			}()
			err = func() error {
				r, err := v.Validate(table, raw)
				if err != nil {
					return err
				}
				if sink != nil {
					if err = sink(r); err != nil {
						v.mu.Lock()
						v.dirty = true
						v.mu.Unlock()
						return err
					}
				}
				return nil
			}()
			returned = true
			return err
		}})
	}
	return tables
}

func validate(table string, raw []byte) (Row, []string, error) {
	names, ok := definitions[table]
	if !ok || len(raw) == 0 || len(raw) > maxRowBytes || !utf8.Valid(raw) || !validSurrogates(raw) {
		return Row{}, nil, ErrInvalid
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	token, err := dec.Token()
	if err != nil || token != json.Delim('{') {
		return Row{}, nil, ErrInvalid
	}
	fields := make(map[string]json.RawMessage, len(names))
	allowed := make(map[string]bool, len(names))
	for _, n := range names {
		allowed[n] = true
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return Row{}, nil, ErrInvalid
		}
		name, ok := tok.(string)
		if !ok || !allowed[name] {
			return Row{}, nil, ErrInvalid
		}
		if _, exists := fields[name]; exists {
			return Row{}, nil, ErrInvalid
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return Row{}, nil, ErrInvalid
		}
		fields[name] = value
	}
	if token, err = dec.Token(); err != nil || token != json.Delim('}') {
		return Row{}, nil, ErrInvalid
	}
	if _, err = dec.Token(); err != io.EOF || len(fields) != len(names) {
		return Row{}, nil, ErrInvalid
	}
	text := func(name string, nullable, nonempty bool) (string, error) {
		value := fields[name]
		if bytes.Equal(value, []byte("null")) {
			if nullable {
				return "", nil
			}
			return "", ErrInvalid
		}
		var s string
		if err := json.Unmarshal(value, &s); err != nil || !utf8.ValidString(s) || len(s) > maxTextBytes || strings.ContainsRune(s, 0) || (nonempty && s == "") {
			return "", ErrInvalid
		}
		return s, nil
	}
	number := func(name string, nullable bool) (int64, bool, error) {
		value := fields[name]
		if bytes.Equal(value, []byte("null")) {
			if nullable {
				return 0, false, nil
			}
			return 0, false, ErrInvalid
		}
		var s string
		if err := json.Unmarshal(value, &s); err != nil || s == "" || (len(s) > 1 && s[0] == '0') {
			return 0, false, ErrInvalid
		}
		for i := range s {
			if s[i] < '0' || s[i] > '9' {
				return 0, false, ErrInvalid
			}
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0, false, ErrInvalid
		}
		return n, true, nil
	}
	blob := func(name string) ([16]byte, error) {
		var out [16]byte
		s, e := text(name, false, true)
		if e != nil {
			return out, e
		}
		b, e := base64.RawURLEncoding.Strict().DecodeString(s)
		if e != nil || len(b) != 16 || base64.RawURLEncoding.EncodeToString(b) != s {
			return out, ErrInvalid
		}
		copy(out[:], b)
		return out, nil
	}
	if table == "static_occupancy_audit" || table == "idempotency_receipt" {
		var key []string
		var err error
		if table == "static_occupancy_audit" {
			key, err = auditKey(fields)
		} else {
			key, err = receiptKey(fields)
		}
		if err != nil {
			return Row{}, nil, err
		}
		return Row{Table: table, Payload: bytes.Clone(raw)}, key, nil
	}
	if table == "ipam_resource_ids" {
		kind, e := text("kind", false, true)
		if e != nil {
			return Row{}, nil, e
		}
		tenant, e := text("tenant_id", false, false)
		if e != nil {
			return Row{}, nil, e
		}
		space, e := text("space_id", false, false)
		if e != nil {
			return Row{}, nil, e
		}
		id, e := text("id", false, true)
		if e != nil {
			return Row{}, nil, e
		}
		switch kind {
		case "TENANT":
			if tenant != "" || space != "" {
				return Row{}, nil, ErrInvalid
			}
		case "SPACE":
			if tenant == "" || space != "" {
				return Row{}, nil, ErrInvalid
			}
		case "PREFIX", "POOL", "OCCUPANCY":
			if tenant == "" || space == "" {
				return Row{}, nil, ErrInvalid
			}
		default:
			return Row{}, nil, ErrInvalid
		}
		return Row{Table: table, Payload: bytes.Clone(raw)}, []string{kind, tenant, space, id}, nil
	}
	var key []string
	for _, n := range []string{"tenant_id", "space_id", "id"} {
		if _, ok := fields[n]; ok {
			s, e := text(n, false, true)
			if e != nil {
				return Row{}, nil, e
			}
			key = append(key, s)
		}
	}
	rev, _, e := number("revision", false)
	if e != nil || rev <= 0 {
		return Row{}, nil, ErrInvalid
	}
	switch table {
	case "ipam_tenants", "ipam_spaces":
		for _, n := range []string{"slug", "display_name"} {
			if _, e := text(n, false, true); e != nil {
				return Row{}, nil, e
			}
		}
	case "ipam_prefixes":
		parent, e := text("parent_id", true, true)
		if e != nil || (parent != "" && parent == key[len(key)-1]) {
			return Row{}, nil, ErrInvalid
		}
		if _, e = text("site_ref", true, false); e != nil {
			return Row{}, nil, e
		}
		if _, e = text("label", false, false); e != nil {
			return Row{}, nil, e
		}
		family, _, e := number("family", false)
		if e != nil || (family != 4 && family != 6) {
			return Row{}, nil, ErrInvalid
		}
		bits, _, e := number("prefix_len", false)
		max := int64(128)
		if family == 4 {
			max = 32
		}
		if e != nil || bits > max {
			return Row{}, nil, ErrInvalid
		}
		first, e := blob("first_ip")
		if e != nil {
			return Row{}, nil, e
		}
		last, e := blob("last_ip")
		if e != nil || !validPrefix(family, int(bits), first, last) {
			return Row{}, nil, ErrInvalid
		}
		kind, e := text("kind", false, true)
		if e != nil {
			return Row{}, nil, e
		}
		usage, e := text("usage", false, true)
		if e != nil {
			return Row{}, nil, e
		}
		if kind == "CONTAINER" {
			if usage != "AGGREGATE" {
				return Row{}, nil, ErrInvalid
			}
		} else if kind == "SUBNET" {
			if !((usage == "STANDARD" && bits < max-1) || (usage == "P2P" && bits == max-1) || (usage == "HOST" && bits == max)) {
				return Row{}, nil, ErrInvalid
			}
		} else {
			return Row{}, nil, ErrInvalid
		}
	case "ipam_pools":
		if _, e = text("subnet_id", false, true); e != nil {
			return Row{}, nil, e
		}
		if _, e = text("label", false, false); e != nil {
			return Row{}, nil, e
		}
		kind, err := text("pool_kind", false, true)
		if err != nil || (kind != "IPAM" && kind != "DHCP") {
			return Row{}, nil, ErrInvalid
		}
		family, _, err := number("family", false)
		if err != nil || (family != 4 && family != 6) {
			return Row{}, nil, ErrInvalid
		}
		first, err := blob("first_ip")
		if err != nil {
			return Row{}, nil, err
		}
		last, err := blob("last_ip")
		if err != nil {
			return Row{}, nil, err
		}
		if bytes.Compare(first[:], last[:]) > 0 || !validAddr(family, first) || !validAddr(family, last) {
			return Row{}, nil, ErrInvalid
		}
		if family == 6 {
			mappedFirst := [16]byte{10: 0xff, 11: 0xff}
			mappedLast := [16]byte{10: 0xff, 11: 0xff, 12: 0xff, 13: 0xff, 14: 0xff, 15: 0xff}
			if bytes.Compare(first[:], mappedLast[:]) <= 0 && bytes.Compare(last[:], mappedFirst[:]) >= 0 {
				return Row{}, nil, ErrInvalid
			}
		}
	case "ipam_occupancies":
		if _, e = text("subnet_id", false, true); e != nil {
			return Row{}, nil, e
		}
		if _, e = text("owner_ref", false, true); e != nil {
			return Row{}, nil, e
		}
		family, _, e := number("family", false)
		if e != nil || (family != 4 && family != 6) {
			return Row{}, nil, ErrInvalid
		}
		address, e := blob("address")
		if e != nil || !validAddr(family, address) {
			return Row{}, nil, ErrInvalid
		}
		kind, e := text("kind", false, true)
		if e != nil {
			return Row{}, nil, e
		}
		created, _, e := number("created_at_ms", false)
		if e != nil {
			return Row{}, nil, e
		}
		expiry, present, e := number("expires_at_ms", true)
		if e != nil {
			return Row{}, nil, e
		}
		switch kind {
		case "HOLD":
			if !present || expiry <= created {
				return Row{}, nil, ErrInvalid
			}
		case "STATIC", "RESERVATION", "EXCLUDED":
			if present {
				return Row{}, nil, ErrInvalid
			}
		default:
			return Row{}, nil, ErrInvalid
		}
	}
	return Row{Table: table, Payload: append([]byte(nil), raw...)}, key, nil
}
func validAddr(family int64, a [16]byte) bool {
	zero := make([]byte, 12)
	if family == 4 {
		return bytes.Equal(a[:12], zero)
	}
	mapped := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255}
	return !bytes.Equal(a[:12], mapped)
}
func validPrefix(family int64, bits int, first, last [16]byte) bool {
	if family == 4 && (!validAddr(family, first) || !validAddr(family, last)) {
		return false
	}
	if family == 6 && bits >= 96 && !validAddr(family, first) {
		return false
	}
	prefix := bits
	if family == 4 {
		prefix += 96
	}
	for i := 0; i < 128; i++ {
		mask := byte(1 << uint(7-i%8))
		a, b := first[i/8]&mask, last[i/8]&mask
		if i < prefix {
			if a != b {
				return false
			}
		} else if a != 0 || b == 0 {
			return false
		}
	}
	return true
}
