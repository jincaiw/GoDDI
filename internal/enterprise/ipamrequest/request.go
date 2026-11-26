// Package ipamrequest normalizes a narrow specified-address STATIC create
// request. It performs neither authentication, authorization, receipt lookup,
// precondition evaluation, nor address allocation.
package ipamrequest

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"

	"github.com/google/uuid"
	"github.com/jincaiw/GoDDI/internal/enterprise/httpcontract"
	"github.com/jincaiw/GoDDI/internal/enterprise/ipamaddr"
)

var ErrInvalid = errors.New("ipamrequest: invalid static create request")

// Request represents only a STATIC allocation with an explicitly supplied
// address, no poolId or holdSeconds. The HTTP adapter must reject unknown,
// duplicate and unsupported body fields or query parameters, including IDs and
// server timestamps; it must not silently drop any result-affecting input.
// TenantID is a request scope, not proof of identity or authorization. IfMatch
// contains every HTTP field line; absent is allowed for this POST. Conditional
// creation is an optional extension: adapters accepting it must atomically check
// it against the intended resource after receipt lookup, or reject it outright.
type Request struct {
	TenantID, SpaceID, SubnetID string
	Address, OwnerRef           string
	IfMatch                     []string
}

// Normalized is the validated semantic input. Address is canonical. IDs use
// canonical lower-case UUID spelling; OwnerRef retains its exact bytes.
type Normalized struct {
	TenantID, SpaceID, SubnetID string
	Address                     netip.Addr
	OwnerRef                    string
	IfMatchRevision             *int64
}

// Normalize returns normalized semantics and a SHA-256 digest. Each frame
// element is a 4-byte unsigned big-endian byte length followed by its value,
// in this fixed order: domain "goddi.ipam.static-create.v1", tenant UUID bytes,
// space UUID bytes, subnet UUID bytes, one-byte family (4/6), 16-byte network
// address, exact owner bytes, then an If-Match presence byte followed (when
// present) by the revision as an unsigned big-endian 8-byte integer. Kind is
// fixed by the domain; the frame never includes generated IDs or timestamps.
// This is not a receipt key: the caller separately scopes receipts by tenant,
// authenticated principal, method, canonical path, and idempotency key.
// The owner restriction to 1..255 printable ASCII bytes is an intentional
// narrower subset of OpenAPI, aligned with the candidate event schema.
func Normalize(r Request) (Normalized, [32]byte, error) {
	var n Normalized
	var zero [32]byte
	ids := make([]uuid.UUID, 3)
	for i, s := range []string{r.TenantID, r.SpaceID, r.SubnetID} {
		id, err := uuid.Parse(s)
		if err != nil || id == uuid.Nil || id.String() != s {
			return n, zero, fmt.Errorf("%w: UUID", ErrInvalid)
		}
		ids[i] = id
	}
	if len(r.Address) > 45 {
		return n, zero, fmt.Errorf("%w: address", ErrInvalid)
	}
	addr, err := ipamaddr.ParseAddr(r.Address)
	if err != nil {
		return n, zero, fmt.Errorf("%w: address", ErrInvalid)
	}
	family, encoded, err := ipamaddr.Encode(addr)
	if err != nil {
		return n, zero, fmt.Errorf("%w: address", ErrInvalid)
	}
	if len(r.OwnerRef) == 0 || len(r.OwnerRef) > 255 {
		return n, zero, fmt.Errorf("%w: owner", ErrInvalid)
	}
	for i := range r.OwnerRef {
		if r.OwnerRef[i] < 0x20 || r.OwnerRef[i] > 0x7e {
			return n, zero, fmt.Errorf("%w: owner", ErrInvalid)
		}
	}
	var revision *int64
	if len(r.IfMatch) > 0 {
		v, e := httpcontract.ParseIfMatch(r.IfMatch)
		if e != nil {
			return n, zero, e
		}
		revision = &v
	}
	n = Normalized{TenantID: r.TenantID, SpaceID: r.SpaceID, SubnetID: r.SubnetID, Address: addr, OwnerRef: r.OwnerRef, IfMatchRevision: revision}
	h := sha256.New()
	frame := func(b []byte) {
		var l [4]byte
		binary.BigEndian.PutUint32(l[:], uint32(len(b)))
		_, _ = h.Write(l[:])
		_, _ = h.Write(b)
	}
	frame([]byte("goddi.ipam.static-create.v1"))
	for _, id := range ids {
		frame(id[:])
	}
	frame([]byte{byte(family)})
	frame(encoded[:])
	frame([]byte(r.OwnerRef))
	if revision == nil {
		frame([]byte{0})
	} else {
		var value [9]byte
		value[0] = 1
		binary.BigEndian.PutUint64(value[1:], uint64(*revision))
		frame(value[:])
	}
	var digest [32]byte
	copy(digest[:], h.Sum(nil))
	return n, digest, nil
}
