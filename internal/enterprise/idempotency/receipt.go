// Package idempotency stores retry receipts inside a caller-owned SQL transaction.
// It neither authenticates callers nor grants authority or commits transactions.
package idempotency

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	MaxPrincipalBytes = 256
	MaxPathBytes      = 2048
	MaxETagBytes      = 512
	MaxBodyBytes      = 1 << 20
	maxEncodedBytes   = 8 + MaxETagBytes + MaxBodyBytes
)

var (
	ErrInvalid  = errors.New("idempotency: invalid input")
	ErrConflict = errors.New("idempotency: key reused with different request")
	ErrCorrupt  = errors.New("idempotency: corrupt receipt")
	ErrWrite    = errors.New("idempotency: receipt write unconfirmed")
)

// Scope must be derived after authentication and current authorization.
// Path is the canonical, absolute API path without query or fragment. This
// initial implementation rejects percent escapes to avoid ambiguous spellings.
type Scope struct {
	Tenant                       [16]byte
	Principal, Method, Path, Key string
}

// Response contains exact committed HTTP status, ETag and body bytes. Headers
// outside this contract are not retained; callers must not assume otherwise.
type Response struct {
	Status int
	ETag   string
	Body   []byte
}

type Receipt struct {
	Response                 Response
	CreatedAtMS, ExpiresAtMS int64
}

func validScope(s Scope) error {
	if s.Tenant == ([16]byte{}) || len(s.Principal) == 0 || len(s.Principal) > MaxPrincipalBytes || !utf8.ValidString(s.Principal) || strings.TrimSpace(s.Principal) != s.Principal || strings.ContainsAny(s.Principal, "\x00\r\n") {
		return ErrInvalid
	}
	switch s.Method {
	case "POST", "PUT", "PATCH", "DELETE":
	default:
		return ErrInvalid
	}
	if len(s.Path) == 0 || len(s.Path) > MaxPathBytes || !utf8.ValidString(s.Path) || !strings.HasPrefix(s.Path, "/") || path.Clean(s.Path) != s.Path || strings.ContainsAny(s.Path, "%?#\\\x00\r\n") {
		return ErrInvalid
	}
	u, err := url.ParseRequestURI(s.Path)
	if err != nil || u.Path != s.Path || u.RawQuery != "" {
		return ErrInvalid
	}
	id, err := uuid.Parse(s.Key)
	if err != nil || id == uuid.Nil || id.String() != s.Key {
		return ErrInvalid
	}
	return nil
}

// Backend provides only receipt operations, not arbitrary SQL. Implementations
// must use the same caller-owned transaction for both operations and bound all
// fields returned from storage before allocating them.
type Backend interface {
	ReadReceipt(context.Context, Scope) (Stored, error)
	CreateReceipt(context.Context, Scope, Stored) error
}

// Stored is the bounded on-disk form exchanged with a transaction adapter.
type Stored struct {
	Digest                   []byte
	Response                 []byte
	CreatedAtMS, ExpiresAtMS int64
}

type sqlBackend struct{ tx *sql.Tx }

// Lookup returns found=false only when no receipt exists. A different digest
// yields ErrConflict even after expiry: cleanup and reuse are not automatic.
// The caller must authenticate and authorize before invoking this method.
func Lookup(ctx context.Context, tx *sql.Tx, scope Scope, digest [32]byte) (Receipt, bool, error) {
	if tx == nil {
		return Receipt{}, false, ErrInvalid
	}
	return LookupBackend(ctx, sqlBackend{tx}, scope, digest)
}

// LookupBackend accepts a receipt-only adapter owned by the caller's transaction.
func LookupBackend(ctx context.Context, backend Backend, scope Scope, digest [32]byte) (Receipt, bool, error) {
	if backend == nil {
		return Receipt{}, false, ErrInvalid
	}
	if err := validScope(scope); err != nil {
		return Receipt{}, false, err
	}
	stored, err := backend.ReadReceipt(ctx, scope)
	if errors.Is(err, sql.ErrNoRows) {
		return Receipt{}, false, nil
	}
	if err != nil {
		return Receipt{}, false, err
	}
	if len(stored.Digest) != 32 || len(stored.Response) > maxEncodedBytes || stored.CreatedAtMS <= 0 || stored.ExpiresAtMS <= stored.CreatedAtMS {
		return Receipt{}, false, ErrCorrupt
	}
	if subtle.ConstantTimeCompare(stored.Digest, digest[:]) != 1 {
		return Receipt{}, false, ErrConflict
	}
	response, err := decode(stored.Response)
	if err != nil {
		return Receipt{}, false, err
	}
	return Receipt{Response: response, CreatedAtMS: stored.CreatedAtMS, ExpiresAtMS: stored.ExpiresAtMS}, true, nil
}

// Insert stores a receipt in the caller's transaction, without committing it.
func Insert(ctx context.Context, tx *sql.Tx, scope Scope, digest [32]byte, receipt Receipt) error {
	if tx == nil {
		return ErrInvalid
	}
	return InsertBackend(ctx, sqlBackend{tx}, scope, digest, receipt)
}

// InsertBackend never overwrites a receipt; failures must abort the transaction.
func InsertBackend(ctx context.Context, backend Backend, scope Scope, digest [32]byte, receipt Receipt) error {
	if backend == nil {
		return ErrInvalid
	}
	if err := validScope(scope); err != nil {
		return err
	}
	if receipt.CreatedAtMS <= 0 || receipt.ExpiresAtMS <= receipt.CreatedAtMS {
		return ErrInvalid
	}
	encoded, err := encode(receipt.Response)
	if err != nil {
		return err
	}
	return backend.CreateReceipt(ctx, scope, Stored{Digest: digest[:], Response: encoded, CreatedAtMS: receipt.CreatedAtMS, ExpiresAtMS: receipt.ExpiresAtMS})
}

// sqlReader is used only by fixed receipt statements within this package.
type sqlReader interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func (b sqlBackend) ReadReceipt(ctx context.Context, s Scope) (Stored, error) {
	return readStored(ctx, b.tx, s)
}
func (b sqlBackend) CreateReceipt(ctx context.Context, s Scope, r Stored) error {
	return createStored(ctx, b.tx, s, r)
}

// readStored uses fixed bounded SQL.
func readStored(ctx context.Context, tx sqlReader, scope Scope) (Stored, error) {
	var result Stored
	var responseLen, created, expires sql.NullInt64
	err := tx.QueryRowContext(ctx, `SELECT
 CASE WHEN typeof(request_digest) = 'blob' AND length(request_digest) = 32 THEN request_digest ELSE NULL END,
 length(response), CASE WHEN typeof(response) = 'blob' AND length(response) <= ? THEN response ELSE NULL END,
 CASE WHEN typeof(created_at_unix_ms) = 'integer' THEN created_at_unix_ms ELSE NULL END,
 CASE WHEN typeof(expires_at_unix_ms) = 'integer' THEN expires_at_unix_ms ELSE NULL END
 FROM idempotency_receipt WHERE tenant_id=? AND principal=? AND method=? AND path=? AND idempotency_key=?`,
		maxEncodedBytes, scope.Tenant[:], scope.Principal, scope.Method, scope.Path, scope.Key).
		Scan(&result.Digest, &responseLen, &result.Response, &created, &expires)
	if err != nil {
		return Stored{}, err
	}
	if len(result.Digest) != 32 || !responseLen.Valid || responseLen.Int64 > maxEncodedBytes || int64(len(result.Response)) != responseLen.Int64 || result.Response == nil || !created.Valid || !expires.Valid {
		return Stored{}, ErrCorrupt
	}
	result.CreatedAtMS, result.ExpiresAtMS = created.Int64, expires.Int64
	return result, nil
}

// createStored uses a fixed INSERT; duplicate keys never overwrite a receipt.
func createStored(ctx context.Context, tx sqlReader, scope Scope, r Stored) error {
	result, err := tx.ExecContext(ctx, `INSERT INTO idempotency_receipt (tenant_id, principal, method, path, idempotency_key, request_digest, response, created_at_unix_ms, expires_at_unix_ms) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, scope.Tenant[:], scope.Principal, scope.Method, scope.Path, scope.Key, r.Digest, r.Response, r.CreatedAtMS, r.ExpiresAtMS)
	if err != nil {
		return fmt.Errorf("idempotency: insert receipt: %w", err)
	}
	if result == nil {
		return fmt.Errorf("idempotency: insert receipt: %w: missing result", ErrWrite)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("idempotency: insert receipt: %w: rows affected: %w", ErrWrite, err)
	}
	if n != 1 {
		return fmt.Errorf("idempotency: insert receipt: %w: rows affected = %d", ErrWrite, n)
	}
	return nil
}

// Version 1 framing: magic, status (u16 big endian), ETag length (u16), then
// ETag UTF-8 bytes and the unmodified body. Framing is private storage format.
func encode(r Response) ([]byte, error) {
	if r.Status < 100 || r.Status > 599 || len(r.ETag) > MaxETagBytes || len(r.Body) > MaxBodyBytes || !utf8.ValidString(r.ETag) || strings.ContainsAny(r.ETag, "\x00\r\n") {
		return nil, ErrInvalid
	}
	b := make([]byte, 8+len(r.ETag)+len(r.Body))
	copy(b, "GIR1")
	binary.BigEndian.PutUint16(b[4:6], uint16(r.Status))
	binary.BigEndian.PutUint16(b[6:8], uint16(len(r.ETag)))
	copy(b[8:], r.ETag)
	copy(b[8+len(r.ETag):], r.Body)
	return b, nil
}
func decode(b []byte) (Response, error) {
	if len(b) < 8 || string(b[:4]) != "GIR1" {
		return Response{}, ErrCorrupt
	}
	n := int(binary.BigEndian.Uint16(b[6:8]))
	if n > MaxETagBytes || n > len(b)-8 || len(b)-8-n > MaxBodyBytes {
		return Response{}, ErrCorrupt
	}
	r := Response{Status: int(binary.BigEndian.Uint16(b[4:6])), ETag: string(b[8 : 8+n]), Body: append([]byte(nil), b[8+n:]...)}
	if _, err := encode(r); err != nil {
		return Response{}, ErrCorrupt
	}
	return r, nil
}
