package lease

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// MutationFactIdentity is the stable identity context for one DHCP lease fact.
// EventID is deterministic for the same source, space, mutation, lease and
// generation, so a retry can reuse it without creating a second fact.
type MutationFactIdentity struct {
	EventID string
	Source  string
	SpaceID string
}

// NewMutationFactIdentity derives a retry-stable event identity. The lease
// generation is part of the key: each accepted binding transition gets a new
// fact, while repeating the same request after a timeout gets the same ID.
func NewMutationFactIdentity(source, spaceID string, kind MutationKind, leaseID string, generation int64) (MutationFactIdentity, error) {
	if strings.TrimSpace(source) == "" || strings.TrimSpace(spaceID) == "" ||
		strings.TrimSpace(leaseID) == "" {
		return MutationFactIdentity{}, errors.New("lease fact identity: source, space ID, and lease ID are required")
	}
	if generation < 0 {
		return MutationFactIdentity{}, errors.New("lease fact identity: generation must be non-negative")
	}
	if _, err := ContractFor(kind); err != nil {
		return MutationFactIdentity{}, err
	}
	key := strings.Join([]string{
		identityPart(source), identityPart(spaceID), identityPart(string(kind)),
		identityPart(leaseID), strconv.FormatInt(generation, 10),
	}, "|")
	sum := sha256.Sum256([]byte(key))
	return MutationFactIdentity{
		EventID: "dhcp-fact-" + hex.EncodeToString(sum[:]),
		Source:  source,
		SpaceID: spaceID,
	}, nil
}

func identityPart(value string) string {
	return fmt.Sprintf("%d:%s", len(value), value)
}

func (i MutationFactIdentity) Validate() error {
	if strings.TrimSpace(i.EventID) == "" || strings.TrimSpace(i.Source) == "" || strings.TrimSpace(i.SpaceID) == "" {
		return errors.New("lease fact identity: event ID, source, and space ID are required")
	}
	return nil
}
