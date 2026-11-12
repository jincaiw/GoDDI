// Package eventlog implements the frozen Enterprise replication event envelope.
//
// The byte layout is defined by docs/04-replication-ha.md section "事件与日志
// 协议" and is reproducible from contracts/replication-vector.json. Digests are
// always computed over the ORIGINAL payload bytes: a payload must never be
// re-encoded before hashing, because re-encoding is not byte-preserving and
// would silently break the chain.
package eventlog

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

const (
	// digestPrefix is the fixed framing prefix of the digest layout.
	digestPrefix = "goddi.replica.v1"

	// SchemaVersionV1 is the frozen first event schema version.
	SchemaVersionV1 uint32 = 1

	// IDBytes is the width of a cluster or node identifier (UUID bytes).
	IDBytes = 16
	// HashBytes is the width of a chain position digest.
	HashBytes = sha256.Size
	// MaxPayloadBytes is the trial-run per-event payload ceiling (docs/04:44).
	MaxPayloadBytes = 4 << 20
)

// Errors returned by this package. Callers map them to protocol dispositions;
// none of them grants or revokes a role.
var (
	ErrIDLength         = errors.New("eventlog: cluster and leader IDs must be 16 bytes")
	ErrHashLength       = errors.New("eventlog: chain hashes must be 32 bytes")
	ErrPositionRange    = errors.New("eventlog: epoch and seq must fit the signed 63-bit SQLite range")
	ErrSchemaVersion    = errors.New("eventlog: unsupported schema version")
	ErrPayloadSize      = errors.New("eventlog: payload is empty or exceeds the configured limit")
	ErrPayloadNotJSON   = errors.New("eventlog: payload is not a single JSON object")
	ErrPayloadDuplicate = errors.New("eventlog: payload repeats a JSON object key")
	ErrPayloadEncoding  = errors.New("eventlog: payload must be UTF-8 without a byte-order mark")
	ErrUnknownKind      = errors.New("eventlog: payload contains an unknown change kind")
	ErrAnchorHash       = errors.New("eventlog: position (epoch, 0) must carry the all-zero hash")
	ErrChainBreak       = errors.New("eventlog: prev_hash does not match the previous event")
	ErrDigestMismatch   = errors.New("eventlog: payload digest does not match the event hash")
	ErrDiverged         = errors.New("eventlog: the same position holds two different digests")
	ErrEpochMismatch    = errors.New("eventlog: position epoch differs from the chain epoch")
	ErrIdentityMismatch = errors.New("eventlog: event cluster or leader differs from the chain identity")
)

var uuidRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// ID is a cluster or node identifier held as its raw 16 UUID bytes.
type ID [IDBytes]byte

// ParseID accepts a canonical UUID text form.
func ParseID(s string) (ID, error) {
	var id ID
	if !uuidRE.MatchString(s) {
		return id, fmt.Errorf("%w: %q is not a canonical UUID", ErrIDLength, s)
	}
	raw, err := hex.DecodeString(strings.ReplaceAll(s, "-", ""))
	if err != nil || len(raw) != IDBytes {
		return id, fmt.Errorf("%w: %v", ErrIDLength, err)
	}
	copy(id[:], raw)
	return id, nil
}

// IDFromBytes copies a raw identifier, rejecting the wrong width.
func IDFromBytes(b []byte) (ID, error) {
	var id ID
	if len(b) != IDBytes {
		return id, ErrIDLength
	}
	copy(id[:], b)
	return id, nil
}

// Bytes returns a copy so callers cannot mutate an event in place.
func (id ID) Bytes() []byte {
	out := make([]byte, IDBytes)
	copy(out, id[:])
	return out
}

// Value stores the identifier as its raw 16 bytes. Text is not accepted even
// on the way in: a case or zone difference would silently create two keys for
// one cluster, and the schema checks the width.
func (id ID) Value() (driver.Value, error) { return append([]byte{}, id[:]...), nil }

// Scan reads a stored identifier back into its raw form.
func (id *ID) Scan(src any) error {
	b, ok := src.([]byte)
	if !ok {
		return fmt.Errorf("%w: the database returned %T, not 16 raw bytes", ErrIDLength, src)
	}
	got, err := IDFromBytes(b)
	if err != nil {
		return err
	}
	*id = got
	return nil
}

var (
	_ driver.Valuer = ID{}
	_ sql.Scanner   = (*ID)(nil)
)

// String renders the canonical UUID text form.
func (id ID) String() string {
	h := hex.EncodeToString(id[:])
	return strings.Join([]string{h[0:8], h[8:12], h[12:16], h[16:20], h[20:32]}, "-")
}

// Position is the sequence location of an event inside one epoch.
type Position struct {
	Epoch uint64
	Seq   uint64
	Hash  [HashBytes]byte
}

// GenesisHash returns a fresh all-zero genesis digest; callers cannot mutate
// the package's trust anchor through a shared variable.
func GenesisHash() [HashBytes]byte { return [HashBytes]byte{} }

// Event is one replication event envelope.
type Event struct {
	ClusterID ID
	LeaderID  ID
	Position  Position
	Schema    uint32
	CommitMs  int64
	PrevHash  [HashBytes]byte
	Payload   []byte
}

// AnchorHash is the digest recorded at position (epoch, 0). The first epoch
// starts from 32 zero bytes; a rebaselined epoch carries the predecessor's
// frozen final digest, and that binding comes from the takeover record rather
// than from anything this package can observe (docs/04:45).
func AnchorHash(epoch uint64, predecessorFinal [HashBytes]byte) ([HashBytes]byte, error) {
	if epoch == 0 {
		return GenesisHash(), fmt.Errorf("%w: epoch starts at 1", ErrPositionRange)
	}
	if epoch == 1 {
		if predecessorFinal != GenesisHash() {
			return GenesisHash(), fmt.Errorf("%w: epoch 1 anchors on the all-zero hash", ErrAnchorHash)
		}
		return GenesisHash(), nil
	}
	return predecessorFinal, nil
}

// NewAnchor returns the empty head of the first epoch.
func NewAnchor(epoch uint64) (Position, error) {
	if epoch != 1 {
		return Position{}, fmt.Errorf("%w: later epochs require an explicitly verified rebase anchor", ErrAnchorHash)
	}
	return Position{Epoch: 1, Seq: 0, Hash: GenesisHash()}, nil
}

// Hash computes the event digest over the frozen byte layout.
func (e Event) Hash() ([HashBytes]byte, error) {
	var h [HashBytes]byte
	if err := e.validateShape(); err != nil {
		return h, err
	}
	// Check size before copying or decoding so malformed input cannot force an
	// extra unbounded allocation. Callers must not mutate Payload concurrently.
	w := e.Payload
	if err := e.validatePayload(w); err != nil {
		return h, err
	}
	d := sha256.New()
	wd := d
	writeAll(wd, []byte(digestPrefix))
	writeAll(wd, []byte{0})
	writeAll(wd, e.ClusterID[:])
	writeAll(wd, be64(e.Position.Epoch))
	writeAll(wd, be64(e.Position.Seq))
	writeAll(wd, e.LeaderID[:])
	writeAll(wd, be32(e.Schema))
	writeAll(wd, be64(uint64(e.CommitMs)))
	writeAll(wd, e.PrevHash[:])
	writeAll(wd, be32(uint32(len(w))))
	writeAll(wd, w)
	copy(h[:], d.Sum(nil))
	return h, nil
}

func writeAll(w interface{ Write([]byte) (int, error) }, b []byte) {
	// crypto.Hash.Write never fails and the widths below are fixed.
	_, _ = w.Write(b)
}

func (e Event) validateShape() error {
	if e.Position.Epoch == 0 {
		return fmt.Errorf("%w: epoch starts at 1", ErrPositionRange)
	}
	if !inInt64Range(e.Position.Epoch) || !inInt64Range(e.Position.Seq) {
		return ErrPositionRange
	}
	if e.Position.Seq == 0 {
		return fmt.Errorf("%w: position (epoch, 0) is an anchor, not an event", ErrChainBreak)
	}
	if e.Schema != SchemaVersionV1 {
		return fmt.Errorf("%w: %d", ErrSchemaVersion, e.Schema)
	}
	return nil
}

func inInt64Range(v uint64) bool {
	// SQLite rowids and the log's own counters are signed; 2^63-1 is the cap.
	return v <= uint64(1<<63-1)
}

func be64(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
}

func be32(v uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, v)
	return b
}

// Verify checks an event against its recorded digest. Position (epoch, 0) is
// the chain anchor: it carries no payload and no digest of its own, so it is
// validated structurally instead.
func (e Event) Verify() error {
	if e.Position.Seq == 0 {
		if len(e.Payload) != 0 {
			return fmt.Errorf("%w: an anchor carries no payload", ErrAnchorHash)
		}
		if e.Position.Epoch == 0 {
			return fmt.Errorf("%w: epoch starts at 1", ErrPositionRange)
		}
		if e.Position.Epoch == 1 && e.Position.Hash != GenesisHash() {
			return fmt.Errorf("%w: epoch 1 anchors on 32 zero bytes", ErrAnchorHash)
		}
		return nil
	}
	want, err := e.Hash()
	if err != nil {
		return err
	}
	if want != e.Position.Hash {
		return ErrDigestMismatch
	}
	return nil
}

// VerifyChain links an event to its predecessor and requires the same cluster
// and leader within the epoch. The caller must independently authenticate the
// peer and supply trusted identities, including on an anchor: a hash does not
// authenticate either identity.
func VerifyChain(prev, next Event) error {
	if prev.ClusterID != next.ClusterID || prev.LeaderID != next.LeaderID {
		return ErrIdentityMismatch
	}
	if err := prev.Verify(); err != nil {
		return fmt.Errorf("eventlog: previous event is invalid: %w", err)
	}
	if err := next.Verify(); err != nil {
		return err
	}
	if next.Position.Epoch != prev.Position.Epoch {
		return fmt.Errorf("%w: %d then %d", ErrEpochMismatch, prev.Position.Epoch, next.Position.Epoch)
	}
	if next.Position.Seq != prev.Position.Seq+1 {
		return fmt.Errorf("%w: %d follows %d", ErrChainBreak, next.Position.Seq, prev.Position.Seq)
	}
	if next.PrevHash != prev.Position.Hash {
		return fmt.Errorf("%w: prev_hash is not the predecessor digest", ErrChainBreak)
	}
	return nil
}

// Accept classifies an incoming event against a locally applied head, as
// docs/04-replication-ha.md prescribes: a repeat of the same position with the
// same digest is a no-op, the same position with a different digest diverges
// and must be quarantined, and a jump in the sequence is a gap rather than
// something to skip past. Crossing epochs is never accepted here: a rebase is
// authorised by the takeover record and a signed snapshot, not by a stream.
func Accept(head, incoming Event) (duplicate bool, err error) {
	if head.ClusterID != incoming.ClusterID || head.LeaderID != incoming.LeaderID {
		return false, ErrIdentityMismatch
	}
	if err = incoming.Verify(); err != nil {
		return false, err
	}
	if incoming.Position.Epoch != head.Position.Epoch {
		return false, fmt.Errorf("%w: head (%s,%s), incoming (%s,%s)", ErrEpochMismatch,
			DecUint(head.Position.Epoch), DecUint(head.Position.Seq),
			DecUint(incoming.Position.Epoch), DecUint(incoming.Position.Seq))
	}
	if incoming.Position.Seq == head.Position.Seq {
		if incoming.Position.Hash == head.Position.Hash {
			return true, nil
		}
		return false, fmt.Errorf("%w at (%s,%s)", ErrDiverged,
			DecUint(incoming.Position.Epoch), DecUint(incoming.Position.Seq))
	}
	if incoming.Position.Seq != head.Position.Seq+1 {
		return false, fmt.Errorf("%w: expected %s after (%s,%s), got %s", ErrChainBreak,
			DecUint(head.Position.Seq+1),
			DecUint(head.Position.Epoch), DecUint(head.Position.Seq),
			DecUint(incoming.Position.Seq))
	}
	if err = VerifyChain(head, incoming); err != nil {
		return false, err
	}
	return false, nil
}

type wirePayload struct {
	Changes []Change `json:"changes"`
}

var (
	kindRE  = regexp.MustCompile(`^[a-z0-9_.-]{1,64}$`)
	int64RE = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)
)

func (e Event) validatePayload(w []byte) error {
	_, err := DecodePayload(w)
	return err
}

// hasDuplicateKeys reports a repeated key at any object depth, which encoding
// or decoding silently hides.
func hasDuplicateKeys(raw []byte) bool {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	t, err := dec.Token()
	if err != nil {
		return true
	}
	if delim, ok := t.(json.Delim); !ok || delim != '{' {
		return true
	}
	return scanObjectKeys(dec)
}

func scanObjectKeys(dec *json.Decoder) bool {
	seen := map[string]bool{}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return true
		}
		key, ok := t.(string)
		if !ok {
			return true
		}
		if seen[key] {
			return true
		}
		seen[key] = true
		if dup, err := scanValue(dec); err != nil || dup {
			return true
		}
	}
	if _, err := dec.Token(); err != nil {
		return true
	}
	return false
}

func scanValue(dec *json.Decoder) (bool, error) {
	t, err := dec.Token()
	if err != nil {
		return false, err
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return false, nil
	}
	switch delim {
	case '{':
		return scanObjectKeys(dec), nil
	case '[':
		for dec.More() {
			if dup, err := scanValue(dec); err != nil || dup {
				return true, err
			}
		}
		_, err := dec.Token()
		return false, err
	}
	return false, nil
}

// DecUint renders a 64-bit counter the way the API and JSON payloads carry it:
// a plain decimal string, never a JS number.
func DecUint(v uint64) string { return new(big.Int).SetUint64(v).String() }

// DecInt64 renders a signed 64-bit counter as a decimal string.
func DecInt64(v int64) string { return strconv.FormatInt(v, 10) }

// ParseDecInt64 accepts the decimal-string form emitted by DecInt64 and
// rejects anything that would silently round or overflow.
func ParseDecInt64(s string) (int64, error) {
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return 0, fmt.Errorf("%w: %q is not a decimal integer", ErrPositionRange, s)
	}
	if !n.IsInt64() {
		return 0, fmt.Errorf("%w: %q exceeds int64", ErrPositionRange, s)
	}
	return n.Int64(), nil
}

// ParseDecUint accepts the decimal-string form emitted by DecUint.
func ParseDecUint(s string) (uint64, error) {
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return 0, fmt.Errorf("%w: %q is not a decimal integer", ErrPositionRange, s)
	}
	if !n.IsUint64() || !inInt64Range(n.Uint64()) {
		return 0, fmt.Errorf("%w: %q exceeds the log range", ErrPositionRange, s)
	}
	return n.Uint64(), nil
}
