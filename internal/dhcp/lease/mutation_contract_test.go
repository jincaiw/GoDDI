package lease

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestContractForCoversEveryLeaseMutation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		kind       MutationKind
		to         LeaseStatus
		walOp      string
		dns        DNSMutationAction
		ipam       IPAMMutationAction
		generation GenerationRule
		durable    bool
	}{
		{MutationOffer, LeaseStatusOffered, WALEventUpsert, DNSActionNone, IPAMActionNone, GenerationKeep, false},
		{MutationActivate, LeaseStatusActive, WALEventUpsert, DNSActionUpsert, IPAMActionObserve, GenerationIncrement, true},
		{MutationRenew, LeaseStatusActive, WALEventUpsert, DNSActionUpsert, IPAMActionObserve, GenerationIncrement, true},
		{MutationRelease, LeaseStatusReleased, WALEventUpsert, DNSActionDelete, IPAMActionObserve, GenerationKeep, true},
		{MutationDecline, LeaseStatusConflict, WALEventUpsert, DNSActionDelete, IPAMActionObserve, GenerationKeep, true},
		{MutationExpire, LeaseStatusExpired, WALEventUpsert, DNSActionDelete, IPAMActionObserve, GenerationKeep, true},
	}

	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			got, err := ContractFor(tc.kind)
			if err != nil {
				t.Fatalf("ContractFor() error = %v", err)
			}
			if got.To != tc.to || got.WALOp != tc.walOp || got.DNSAction != tc.dns ||
				got.IPAMAction != tc.ipam || got.Generation != tc.generation || got.RequiresDurable != tc.durable {
				t.Fatalf("contract = %+v", got)
			}
			if err := got.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestContractForRejectsUnknownMutation(t *testing.T) {
	t.Parallel()

	if _, err := ContractFor(MutationKind("bogus")); !errors.Is(err, ErrUnknownMutation) {
		t.Fatalf("error = %v, want ErrUnknownMutation", err)
	}
}

func TestValidateTransitionRejectsInvalidStateBeforeSideEffects(t *testing.T) {
	t.Parallel()

	active := &Lease{ID: "l1", Status: LeaseStatusActive}
	if _, err := ValidateTransition(MutationActivate, active); !errors.Is(err, ErrInvalidMutationState) {
		t.Fatalf("error = %v, want ErrInvalidMutationState", err)
	}
	if _, err := ValidateTransition(MutationRenew, nil); !errors.Is(err, ErrInvalidMutationState) {
		t.Fatalf("error = %v, want ErrInvalidMutationState", err)
	}
}

func TestValidateTransitionAllowsExpectedStates(t *testing.T) {
	t.Parallel()

	cases := []struct {
		kind   MutationKind
		before *Lease
	}{
		{MutationOffer, nil},
		{MutationActivate, &Lease{ID: "offer", Status: LeaseStatusOffered}},
		{MutationRenew, &Lease{ID: "active", Status: LeaseStatusActive}},
		{MutationRelease, &Lease{ID: "active", Status: LeaseStatusActive}},
		{MutationDecline, &Lease{ID: "active", Status: LeaseStatusActive}},
		{MutationExpire, &Lease{ID: "conflict", Status: LeaseStatusConflict}},
	}

	for _, tc := range cases {
		if _, err := ValidateTransition(tc.kind, tc.before); err != nil {
			t.Errorf("ValidateTransition(%s) error = %v", tc.kind, err)
		}
	}
}

func TestMutationCommandBuildsCanonicalWALShape(t *testing.T) {
	t.Parallel()

	before := &Lease{ID: "l1", Status: LeaseStatusActive, Generation: 1}
	after := &Lease{ID: "l1", Status: LeaseStatusActive, Generation: 2}
	event, err := (MutationCommand{Kind: MutationRenew, Before: before, After: after}).WALEvent()
	if err != nil {
		t.Fatalf("WALEvent() error = %v", err)
	}
	if event.Seq != 0 {
		t.Fatalf("Seq = %d, want unassigned sequence 0", event.Seq)
	}
	if event.Mutation != MutationRenew || event.Op != WALEventUpsert || event.Lease == nil || event.Lease.ID != "l1" {
		t.Fatalf("event = %+v", event)
	}
}

func TestMutationCommandRejectsWrongResultState(t *testing.T) {
	t.Parallel()

	cmd := MutationCommand{
		Kind:   MutationRelease,
		Before: &Lease{ID: "l1", Status: LeaseStatusActive, Generation: 2},
		After:  &Lease{ID: "l1", Status: LeaseStatusActive, Generation: 2},
	}
	if _, err := cmd.WALEvent(); !errors.Is(err, ErrInvalidMutationState) {
		t.Fatalf("error = %v, want ErrInvalidMutationState", err)
	}
}

func TestMutationCommandEnforcesLeaseIdentityAndGeneration(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cmd  MutationCommand
	}{
		{
			name: "identity changed",
			cmd: MutationCommand{
				Kind:   MutationRenew,
				Before: &Lease{ID: "l1", Status: LeaseStatusActive, Generation: 1},
				After:  &Lease{ID: "l2", Status: LeaseStatusActive, Generation: 2},
			},
		},
		{
			name: "increment skipped",
			cmd: MutationCommand{
				Kind:   MutationRenew,
				Before: &Lease{ID: "l1", Status: LeaseStatusActive, Generation: 1},
				After:  &Lease{ID: "l1", Status: LeaseStatusActive, Generation: 3},
			},
		},
		{
			name: "keep changed",
			cmd: MutationCommand{
				Kind:   MutationRelease,
				Before: &Lease{ID: "l1", Status: LeaseStatusActive, Generation: 2},
				After:  &Lease{ID: "l1", Status: LeaseStatusReleased, Generation: 3},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.cmd.Validate(); !errors.Is(err, ErrInvalidMutation) && !errors.Is(err, ErrInvalidMutationState) {
				t.Fatalf("error = %v, want contract/state validation error", err)
			}
		})
	}
}

func TestValidateGenerationRejectsIncrementOverflow(t *testing.T) {
	t.Parallel()

	before := &Lease{ID: "l1", Status: LeaseStatusActive, Generation: maxLeaseGeneration}
	after := &Lease{ID: "l1", Status: LeaseStatusActive, Generation: maxLeaseGeneration}
	if err := validateGeneration(GenerationIncrement, before, after); !errors.Is(err, ErrInvalidMutationState) {
		t.Fatalf("validateGeneration() error = %v, want overflow rejection", err)
	}
}

func TestMutationCommandAcceptsGenerationRules(t *testing.T) {
	t.Parallel()

	cases := []MutationCommand{
		{Kind: MutationOffer, After: &Lease{ID: "offer", Status: LeaseStatusOffered, Generation: 0}},
		{Kind: MutationActivate, Before: &Lease{ID: "offer", Status: LeaseStatusOffered, Generation: 0}, After: &Lease{ID: "offer", Status: LeaseStatusActive, Generation: 1}},
		{Kind: MutationRenew, Before: &Lease{ID: "active", Status: LeaseStatusActive, Generation: 1}, After: &Lease{ID: "active", Status: LeaseStatusActive, Generation: 2}},
		{Kind: MutationRelease, Before: &Lease{ID: "active", Status: LeaseStatusActive, Generation: 2}, After: &Lease{ID: "active", Status: LeaseStatusReleased, Generation: 2}},
		{Kind: MutationDecline, Before: &Lease{ID: "active", Status: LeaseStatusActive, Generation: 2}, After: &Lease{ID: "active", Status: LeaseStatusConflict, Generation: 2}},
		{Kind: MutationExpire, Before: &Lease{ID: "active", Status: LeaseStatusActive, Generation: 2}, After: &Lease{ID: "active", Status: LeaseStatusExpired, Generation: 2}},
	}
	for _, cmd := range cases {
		if _, err := cmd.Validate(); err != nil {
			t.Errorf("Validate(%s) error = %v", cmd.Kind, err)
		}
	}
}

func TestWALMutationRoundTripPreservesMutationKind(t *testing.T) {
	t.Parallel()

	event := WALEvent{
		Version:  currentWALEventVersion,
		Seq:      1,
		Op:       WALEventUpsert,
		Mutation: MutationRenew,
		Lease:    &Lease{ID: "l1", Status: LeaseStatusActive},
	}
	encoded, err := EncodeWALEvent(event)
	if err != nil {
		t.Fatalf("EncodeWALEvent() error = %v", err)
	}
	decoded, err := decodeWALEvent(encoded)
	if err != nil {
		t.Fatalf("decode event: %v", err)
	}
	if decoded.Mutation != MutationRenew {
		t.Fatalf("mutation = %q, want %q", decoded.Mutation, MutationRenew)
	}
}

func decodeWALEvent(data []byte) (WALEvent, error) {
	var event WALEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return WALEvent{}, err
	}
	if err := validateWALEvent(event); err != nil {
		return WALEvent{}, err
	}
	return event, nil
}
