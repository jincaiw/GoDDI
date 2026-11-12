package lease

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jasonwa/goddi/internal/facts"
)

var allMutationKinds = []MutationKind{
	MutationOffer, MutationBind, MutationActivate, MutationRenew,
	MutationRelease, MutationDecline, MutationExpire,
}

func TestEveryMutationKindHasAContract(t *testing.T) {
	for _, kind := range allMutationKinds {
		if _, err := ContractFor(kind); err != nil {
			t.Fatalf("ContractFor(%s) = %v, want a contract", kind, err)
		}
	}
	if _, err := ContractFor(MutationKind("nope")); err == nil {
		t.Fatal("ContractFor accepted an unknown mutation kind")
	}
}

// TestOfferMutationPublishesNoFact documents the deliberate gap in N6: an OFFER
// is not a binding, so the contract maps it to no DNS and no IPAM action and no
// durable requirement. The default ReserveAddress path therefore correctly does
// not emit a fact. If this changes, N6-MUTATION-PREREQUISITES.md must change too.
func TestOfferMutationPublishesNoFact(t *testing.T) {
	contract, err := ContractFor(MutationOffer)
	if err != nil {
		t.Fatal(err)
	}
	if contract.DNSAction != DNSActionNone || contract.IPAMAction != IPAMActionNone {
		t.Fatalf("offer contract has observable actions: dns=%s ipam=%s", contract.DNSAction, contract.IPAMAction)
	}
	if contract.RequiresDurable {
		t.Fatal("offer contract unexpectedly requires durable ACK")
	}
}

func TestMappedMutationsRequireDurable(t *testing.T) {
	for _, kind := range []MutationKind{MutationBind, MutationActivate, MutationRenew, MutationRelease, MutationDecline, MutationExpire} {
		contract, err := ContractFor(kind)
		if err != nil {
			t.Fatal(err)
		}
		if !contract.RequiresDurable {
			t.Fatalf("%s does not require durable ACK", kind)
		}
	}
}

func TestFactIdentityIsRetryStableAndGenerationScoped(t *testing.T) {
	a, err := NewMutationFactIdentity("src", "space", MutationRenew, "lease-1", 2)
	if err != nil {
		t.Fatal(err)
	}
	again, err := NewMutationFactIdentity("src", "space", MutationRenew, "lease-1", 2)
	if err != nil {
		t.Fatal(err)
	}
	if a.EventID != again.EventID {
		t.Fatalf("identity not retry-stable: %s vs %s", a.EventID, again.EventID)
	}
	nextGen, _ := NewMutationFactIdentity("src", "space", MutationRenew, "lease-1", 3)
	if nextGen.EventID == a.EventID {
		t.Fatal("identity did not change with generation")
	}
	otherKind, _ := NewMutationFactIdentity("src", "space", MutationRelease, "lease-1", 2)
	if otherKind.EventID == a.EventID {
		t.Fatal("identity did not change with mutation kind")
	}
	for _, bad := range []struct {
		source, space string
	}{{"", "s"}, {"s", ""}} {
		if _, err := NewMutationFactIdentity(bad.source, bad.space, MutationRenew, "l", 1); err == nil {
			t.Fatalf("identity accepted empty source/space: %+v", bad)
		}
	}
}

func sampleEnvelope(eventID string, seq int64) facts.Envelope {
	return facts.Envelope{
		EventID:        eventID,
		Version:        facts.CurrentEnvelopeVersion,
		Entity:         "dhcp_lease",
		Action:         "renew",
		Generation:     2,
		Sequence:       seq,
		Source:         "src",
		OccurredAt:     time.Now().UTC(),
		PayloadVersion: 1,
		Payload:        json.RawMessage(`{"lease_id":"lease-1"}`),
	}
}

// TestDuplicateFactEnqueueIsIdempotentAcrossRestart proves a replayed fact does
// not create a second row (retry/restart safety) while a conflicting envelope
// under the same id is rejected instead of silently ignored.
func TestDuplicateFactEnqueueIsIdempotentAcrossRestart(t *testing.T) {
	store := newLeaseStore(t)
	outbox, err := facts.NewObservationOutbox(store.DB)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	enqueue := func(env facts.Envelope) error {
		tx, err := store.DB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if err := outbox.EnqueueTx(ctx, tx, env); err != nil {
			_ = tx.Rollback()
			return err
		}
		return tx.Commit()
	}
	env := sampleEnvelope("dup-1", 1)
	if err := enqueue(env); err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	if err := enqueue(env); err != nil {
		t.Fatalf("duplicate enqueue must be idempotent, got %v", err)
	}
	var count int
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM dhcp_ipam_observation_events WHERE event_id = ?`, "dup-1").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("duplicate fact stored %d rows, want 1", count)
	}
	conflict := sampleEnvelope("dup-1", 2)
	if err := enqueue(conflict); err == nil {
		t.Fatal("a conflicting envelope under the same event id was accepted")
	}
}
