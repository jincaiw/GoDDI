// Package snapshotrows validates bounded NDJSON structure and declared table
// counts. It does not authenticate sources, install data or grant authority.
package snapshotrows

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"unicode/utf8"
)

// MaxLineBytes bounds a single NDJSON line excluding its terminating LF.
// Callers must explicitly choose LineBytes; transport chunk limits are separate.
const MaxLineBytes = 2 << 20
const MaxTotalBytes uint64 = 1 << 40
const MaxRows uint64 = 1_000_000_000
const MaxTables = 256
const maxDepth = 128

var ErrInvalid = errors.New("snapshotrows: invalid stream or configuration")
var ErrDirty = errors.New("snapshotrows: validator permanently failed")

// ValidateRow sees an independently owned JSON object encoding, which it may
// retain or mutate. It may validate a schema and primary-key order, but its
// success is not authentication. It must not reenter this validator.
type ValidateRow func([]byte) error
type Table struct {
	Name        string
	Count       uint64
	ValidateRow ValidateRow
}
type Limits struct {
	LineBytes        int
	TotalBytes, Rows uint64
	Tables           int
}
type TableResult struct {
	Name string
	Rows uint64
}
type Result struct {
	Bytes, Rows             uint64
	Tables                  []TableResult
	StructureAndCountsValid bool
	AllRowCallbacksPassed   bool
}
type Validator struct {
	mu              sync.Mutex
	tables          []Table
	counts          []uint64
	index           map[string]int
	limits          Limits
	pending         []byte
	total, rows     uint64
	last            int
	dirty, finished bool
}

// New takes the expected tables in their declared order and exact row counts.
// Bounds must be nonzero and cannot exceed the package absolute limits. The
// table list and callback references are copied; callbacks are trusted code,
// never an authentication or authorization hook.
func New(tables []Table, l Limits) (*Validator, error) {
	if l.LineBytes <= 0 || l.LineBytes > MaxLineBytes || l.TotalBytes == 0 || l.TotalBytes > MaxTotalBytes || l.Rows == 0 || l.Rows > MaxRows || l.Tables <= 0 || l.Tables > MaxTables || len(tables) == 0 || len(tables) > l.Tables {
		return nil, ErrInvalid
	}
	v := &Validator{tables: append([]Table(nil), tables...), counts: make([]uint64, len(tables)), index: make(map[string]int, len(tables)), limits: l, last: -1}
	var sum uint64
	for i, t := range tables {
		if len(t.Name) == 0 || len(t.Name) > 256 || !utf8.ValidString(t.Name) || t.Count > l.Rows-sum {
			return nil, ErrInvalid
		}
		if _, ok := v.index[t.Name]; ok {
			return nil, ErrInvalid
		}
		v.index[t.Name] = i
		sum += t.Count
	}
	return v, nil
}

// Feed consumes arbitrary chunks. A line limit excludes its terminating LF;
// the total byte limit includes LF. Inputs must not be mutated during Feed.
// Every error permanently invalidates the validator. A callback panic also
// invalidates it before being rethrown. At most one bounded line is retained.
func (v *Validator) Feed(chunk []byte) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	returned := false
	defer func() {
		// Do not recover: this preserves panic(nil) even with panicnil=1, and
		// also lets runtime.Goexit continue after poisoning the validator.
		if !returned {
			v.dirty = true
		}
	}()
	err := v.feedLocked(chunk)
	returned = true
	return err
}

func (v *Validator) feedLocked(chunk []byte) error {
	if v.dirty {
		return ErrDirty
	}
	if v.finished {
		return v.fail(ErrInvalid)
	}
	if uint64(len(chunk)) > v.limits.TotalBytes-v.total {
		return v.fail(ErrInvalid)
	}
	v.total += uint64(len(chunk))
	for len(chunk) > 0 {
		i := bytes.IndexByte(chunk, '\n')
		length := len(chunk)
		if i >= 0 {
			length = i
		}
		if length > v.limits.LineBytes-len(v.pending) {
			return v.fail(ErrInvalid)
		}
		v.pending = append(v.pending, chunk[:length]...)
		if i < 0 {
			return nil
		}
		if err := v.line(v.pending); err != nil {
			return v.fail(err)
		}
		v.pending = v.pending[:0]
		chunk = chunk[i+1:]
	}
	return nil
}
func (v *Validator) fail(err error) error { v.dirty = true; return err }
func (v *Validator) line(raw []byte) error {
	if !utf8.Valid(raw) || bytes.HasPrefix(raw, []byte{0xef, 0xbb, 0xbf}) || len(raw) == 0 || !validSurrogates(raw) {
		return ErrInvalid
	}
	// Token walk checks duplicate keys recursively before unmarshalling fields.
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := walk(dec, 0); err != nil {
		return ErrInvalid
	}
	if _, err := dec.Token(); err != io.EOF {
		return ErrInvalid
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || len(object) != 2 {
		return ErrInvalid
	}
	tableRaw, ok := object["table"]
	if !ok {
		return ErrInvalid
	}
	row, ok := object["row"]
	if !ok {
		return ErrInvalid
	}
	var name string
	if err := json.Unmarshal(tableRaw, &name); err != nil {
		return ErrInvalid
	}
	row = bytes.TrimSpace(row)
	if len(row) == 0 || row[0] != '{' {
		return ErrInvalid
	}
	i, ok := v.index[name]
	if !ok || i < v.last || v.counts[i] >= v.tables[i].Count || v.rows >= v.limits.Rows {
		return ErrInvalid
	}
	if fn := v.tables[i].ValidateRow; fn != nil {
		if err := fn(append([]byte(nil), row...)); err != nil {
			return fmt.Errorf("%w: row callback: %v", ErrInvalid, err)
		}
	}
	v.counts[i]++
	v.rows++
	v.last = i
	return nil
}

// validSurrogates inspects JSON string escapes before encoding/json can replace
// malformed UTF-16 with U+FFFD. JSON syntax itself is checked by the decoder.
func validSurrogates(raw []byte) bool {
	inString := false
	for i := 0; i < len(raw); i++ {
		if !inString {
			if raw[i] == '"' {
				inString = true
			}
			continue
		}
		if raw[i] == '"' {
			inString = false
			continue
		}
		if raw[i] != '\\' {
			continue
		}
		i++
		if i >= len(raw) {
			return false
		}
		if raw[i] != 'u' {
			continue
		}
		value, ok := hex4(raw[i+1:])
		if !ok {
			return false
		}
		i += 4
		if value >= 0xdc00 && value <= 0xdfff {
			return false
		}
		if value >= 0xd800 && value <= 0xdbff {
			if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
				return false
			}
			low, ok := hex4(raw[i+3:])
			if !ok || low < 0xdc00 || low > 0xdfff {
				return false
			}
			i += 6
		}
	}
	return true
}

func hex4(b []byte) (uint16, bool) {
	if len(b) < 4 {
		return 0, false
	}
	var n uint16
	for _, c := range b[:4] {
		var v byte
		switch {
		case c >= '0' && c <= '9':
			v = c - '0'
		case c >= 'a' && c <= 'f':
			v = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			v = c - 'A' + 10
		default:
			return 0, false
		}
		n = n<<4 | uint16(v)
	}
	return n, true
}

func walk(d *json.Decoder, depth int) error {
	if depth > maxDepth {
		return ErrInvalid
	}
	tok, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]struct{}{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			s, ok := key.(string)
			if !ok {
				return ErrInvalid
			}
			if _, exists := seen[s]; exists {
				return ErrInvalid
			}
			seen[s] = struct{}{}
			if err = walk(d, depth+1); err != nil {
				return err
			}
		}
		end, err := d.Token()
		if err != nil || end != json.Delim('}') {
			return ErrInvalid
		}
		return nil
	case '[':
		for d.More() {
			if err := walk(d, depth+1); err != nil {
				return err
			}
		}
		end, err := d.Token()
		if err != nil || end != json.Delim(']') {
			return ErrInvalid
		}
		return nil
	default:
		return ErrInvalid
	}
}

// Finish requires every line to end with LF and all declared counts to match.
// The result proves only the stated structural checks and callbacks supplied by
// the caller, not database completeness, authenticity, or takeover readiness.
func (v *Validator) Finish() (Result, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.dirty {
		return Result{}, ErrDirty
	}
	if v.finished || len(v.pending) != 0 {
		return Result{}, v.fail(ErrInvalid)
	}
	out := Result{Bytes: v.total, Rows: v.rows, StructureAndCountsValid: true, AllRowCallbacksPassed: true, Tables: make([]TableResult, len(v.tables))}
	for i, t := range v.tables {
		if v.counts[i] != t.Count {
			return Result{}, v.fail(ErrInvalid)
		}
		out.Tables[i] = TableResult{t.Name, v.counts[i]}
		if t.ValidateRow == nil {
			out.AllRowCallbacksPassed = false
		}
	}
	v.finished = true
	return out, nil
}
