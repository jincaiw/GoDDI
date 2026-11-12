// Package ipamresponse creates a deterministic response for the isolated
// specified-address STATIC create path. It does not authorize or commit writes.
package ipamresponse

import (
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jasonwa/goddi/internal/enterprise/idempotency"
	"github.com/jasonwa/goddi/internal/enterprise/ipamaddr"
)

var ErrInvalid = errors.New("ipamresponse: invalid STATIC result")

type staticBody struct {
	ID       string `json:"id"`
	SubnetID string `json:"subnetId"`
	Address  string `json:"address"`
	OwnerRef string `json:"ownerRef"`
	Kind     string `json:"kind"`
	Revision string `json:"revision"`
}

// Static returns the exact HTTP result retained in an idempotency receipt. The
// supplied identifiers and address must describe the row inserted by the same
// transaction; the caller must enforce that binding and authorization.
func Static(id, subnet, address, owner string) (idempotency.Response, error) {
	for _, value := range []string{id, subnet} {
		u, err := uuid.Parse(value)
		if err != nil || u == uuid.Nil || u.String() != value {
			return idempotency.Response{}, ErrInvalid
		}
	}
	a, err := ipamaddr.ParseAddr(address)
	if err != nil || a.String() != address || len(owner) == 0 || len(owner) > 255 {
		return idempotency.Response{}, ErrInvalid
	}
	for i := range owner {
		if owner[i] < 0x20 || owner[i] > 0x7e {
			return idempotency.Response{}, ErrInvalid
		}
	}
	body, err := json.Marshal(staticBody{ID: id, SubnetID: subnet, Address: address, OwnerRef: owner, Kind: "STATIC", Revision: "1"})
	if err != nil {
		return idempotency.Response{}, err
	}
	return idempotency.Response{Status: 201, ETag: `"v1"`, Body: body}, nil
}
