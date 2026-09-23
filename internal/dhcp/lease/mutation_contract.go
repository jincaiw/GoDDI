package lease

import (
	"errors"
	"fmt"
)

// MutationKind names the lease state transitions that must eventually share one
// durable command/event boundary. The contract is migration-only: Manager and
// the default DHCP path remain SQLite-backed until all callers are migrated.
type MutationKind string

const (
	MutationOffer    MutationKind = "discover_offer"
	MutationBind     MutationKind = "request_bind"
	MutationActivate MutationKind = "request_activate"
	MutationRenew    MutationKind = "request_renew"
	MutationRelease  MutationKind = "release"
	MutationDecline  MutationKind = "decline"
	MutationExpire   MutationKind = "expire"
)

// MutationContract describes the observable consequences of one lease
// mutation. It is deliberately declarative so it can be used by shadow checks,
// WAL event builders, and later transaction orchestration without changing the
// current SQLite implementation.
type MutationContract struct {
	Kind            MutationKind
	From            []LeaseStatus
	To              LeaseStatus
	WALOp           string
	DNSAction       DNSMutationAction
	IPAMAction      IPAMMutationAction
	Generation      GenerationRule
	RequiresDurable bool
	Replay          ReplayRule
}

// MutationCommand is the migration-era command envelope. Before and After are
// snapshots, not live pointers: a WAL builder can safely serialize the command
// after the SQLite mutation without observing later caller changes.
type MutationCommand struct {
	Kind   MutationKind
	Before *Lease
	After  *Lease
}

func (c MutationCommand) Validate() (MutationContract, error) {
	contract, err := ValidateTransition(c.Kind, c.Before)
	if err != nil {
		return MutationContract{}, err
	}
	if c.After == nil {
		return MutationContract{}, fmt.Errorf("%w: %s has no resulting lease", ErrInvalidMutation, c.Kind)
	}
	if c.After.Status != contract.To {
		return MutationContract{}, fmt.Errorf("%w: %s result is %s, want %s", ErrInvalidMutationState, c.Kind, c.After.Status, contract.To)
	}
	if c.After.ID == "" {
		return MutationContract{}, fmt.Errorf("%w: result lease id is empty", ErrInvalidMutation)
	}
	if c.Before != nil && c.After.ID != c.Before.ID {
		return MutationContract{}, fmt.Errorf("%w: %s changed lease id from %q to %q", ErrInvalidMutation, c.Kind, c.Before.ID, c.After.ID)
	}
	if err := validateGeneration(contract.Generation, c.Before, c.After); err != nil {
		return MutationContract{}, err
	}
	return contract, nil
}

// WALEvent returns the canonical snapshot event shape for the command. Sequence
// assignment remains the WAL's responsibility; callers must not fabricate Seq.
func (c MutationCommand) WALEvent() (WALEvent, error) {
	contract, err := c.Validate()
	if err != nil {
		return WALEvent{}, err
	}
	value := *c.After
	return WALEvent{
		Version:  currentWALEventVersion,
		Op:       contract.WALOp,
		Mutation: c.Kind,
		Lease:    &value,
		LeaseID:  value.ID,
	}, nil
}

type DNSMutationAction string

const (
	DNSActionNone   DNSMutationAction = "none"
	DNSActionUpsert DNSMutationAction = "upsert"
	DNSActionDelete DNSMutationAction = "delete"
)

type IPAMMutationAction string

const (
	IPAMActionNone    IPAMMutationAction = "none"
	IPAMActionObserve IPAMMutationAction = "observe"
)

type GenerationRule string

const (
	GenerationKeep      GenerationRule = "keep"
	GenerationStart     GenerationRule = "start_at_one"
	GenerationIncrement GenerationRule = "increment"
)

type ReplayRule string

const (
	ReplayUpsert ReplayRule = "upsert_snapshot"
	ReplayRemove ReplayRule = "remove_by_id"
)

var (
	ErrUnknownMutation      = errors.New("lease mutation: unknown kind")
	ErrInvalidMutation      = errors.New("lease mutation: invalid contract")
	ErrInvalidMutationState = errors.New("lease mutation: invalid source state")
)

// ContractFor returns the single source of truth for migration-era lease
// mutation semantics. A caller must validate the actual prior state before
// applying the transition; no contract permits an implicit fail-open change.
func ContractFor(kind MutationKind) (MutationContract, error) {
	var contract MutationContract
	switch kind {
	case MutationOffer:
		contract = MutationContract{
			Kind: kind, From: []LeaseStatus{LeaseStatusReleased, LeaseStatusExpired},
			To: LeaseStatusOffered, WALOp: WALEventUpsert,
			DNSAction: DNSActionNone, IPAMAction: IPAMActionNone,
			Generation: GenerationKeep, RequiresDurable: false, Replay: ReplayUpsert,
		}
	case MutationBind:
		contract = MutationContract{
			Kind: kind, To: LeaseStatusActive, WALOp: WALEventUpsert,
			DNSAction: DNSActionUpsert, IPAMAction: IPAMActionObserve,
			Generation: GenerationStart, RequiresDurable: true, Replay: ReplayUpsert,
		}
	case MutationActivate:
		contract = MutationContract{
			Kind: kind, From: []LeaseStatus{LeaseStatusOffered},
			To: LeaseStatusActive, WALOp: WALEventUpsert,
			DNSAction: DNSActionUpsert, IPAMAction: IPAMActionObserve,
			Generation: GenerationIncrement, RequiresDurable: true, Replay: ReplayUpsert,
		}
	case MutationRenew:
		contract = MutationContract{
			Kind: kind, From: []LeaseStatus{LeaseStatusActive},
			To: LeaseStatusActive, WALOp: WALEventUpsert,
			DNSAction: DNSActionUpsert, IPAMAction: IPAMActionObserve,
			Generation: GenerationIncrement, RequiresDurable: true, Replay: ReplayUpsert,
		}
	case MutationRelease:
		contract = MutationContract{
			Kind: kind, From: []LeaseStatus{LeaseStatusActive, LeaseStatusOffered},
			To: LeaseStatusReleased, WALOp: WALEventUpsert,
			DNSAction: DNSActionDelete, IPAMAction: IPAMActionObserve,
			Generation: GenerationKeep, RequiresDurable: true, Replay: ReplayUpsert,
		}
	case MutationDecline:
		contract = MutationContract{
			Kind: kind, From: []LeaseStatus{LeaseStatusActive, LeaseStatusOffered},
			To: LeaseStatusConflict, WALOp: WALEventUpsert,
			DNSAction: DNSActionDelete, IPAMAction: IPAMActionObserve,
			Generation: GenerationKeep, RequiresDurable: true, Replay: ReplayUpsert,
		}
	case MutationExpire:
		contract = MutationContract{
			Kind: kind, From: []LeaseStatus{LeaseStatusActive, LeaseStatusOffered, LeaseStatusConflict},
			To: LeaseStatusExpired, WALOp: WALEventUpsert,
			DNSAction: DNSActionDelete, IPAMAction: IPAMActionObserve,
			Generation: GenerationKeep, RequiresDurable: true, Replay: ReplayUpsert,
		}
	default:
		return MutationContract{}, fmt.Errorf("%w %q", ErrUnknownMutation, kind)
	}
	if err := contract.Validate(); err != nil {
		return MutationContract{}, err
	}
	return contract, nil
}

func (c MutationContract) Validate() error {
	if c.Kind == "" || c.To == "" || c.WALOp == "" || c.Replay == "" {
		return ErrInvalidMutation
	}
	if len(c.From) == 0 && c.Kind != MutationBind {
		return ErrInvalidMutation
	}
	for _, status := range c.From {
		if status == "" {
			return fmt.Errorf("%w: source status is empty", ErrInvalidMutation)
		}
	}
	if c.DNSAction != DNSActionNone && c.DNSAction != DNSActionUpsert && c.DNSAction != DNSActionDelete {
		return fmt.Errorf("%w: unsupported dns action %q", ErrInvalidMutation, c.DNSAction)
	}
	if c.IPAMAction != IPAMActionNone && c.IPAMAction != IPAMActionObserve {
		return fmt.Errorf("%w: unsupported ipam action %q", ErrInvalidMutation, c.IPAMAction)
	}
	if c.Generation != GenerationKeep && c.Generation != GenerationStart && c.Generation != GenerationIncrement {
		return fmt.Errorf("%w: unsupported generation rule %q", ErrInvalidMutation, c.Generation)
	}
	if c.WALOp != WALEventUpsert && c.WALOp != WALEventRemove {
		return fmt.Errorf("%w: unsupported wal op %q", ErrInvalidMutation, c.WALOp)
	}
	if c.Replay != ReplayUpsert && c.Replay != ReplayRemove {
		return fmt.Errorf("%w: unsupported replay rule %q", ErrInvalidMutation, c.Replay)
	}
	if c.WALOp == WALEventRemove && c.Replay != ReplayRemove {
		return fmt.Errorf("%w: remove must replay by id", ErrInvalidMutation)
	}
	if c.WALOp == WALEventUpsert && c.Replay != ReplayUpsert {
		return fmt.Errorf("%w: upsert must replay a snapshot", ErrInvalidMutation)
	}
	return nil
}

func (c MutationContract) Allows(status LeaseStatus) bool {
	for _, candidate := range c.From {
		if candidate == status {
			return true
		}
	}
	return false
}

// maxLeaseGeneration is the largest generation that can be incremented without
// wrapping the signed int64 persisted by SQLite and carried by facts/WAL.
const maxLeaseGeneration int64 = 1<<63 - 1

// validateGeneration enforces the generation rule before any WAL, SQLite, DNS,
// or IPAM side effect is attempted.
func validateGeneration(rule GenerationRule, before, after *Lease) error {
	if after == nil || after.Generation < 0 {
		return fmt.Errorf("%w: resulting generation must be non-negative", ErrInvalidMutation)
	}
	if before == nil {
		switch rule {
		case GenerationKeep:
			return nil
		case GenerationStart:
			if after.Generation != 1 {
				return fmt.Errorf("%w: resulting generation=%d, want 1", ErrInvalidMutationState, after.Generation)
			}
			return nil
		default:
			return fmt.Errorf("%w: generation rule %s requires a prior lease", ErrInvalidMutationState, rule)
		}
	}
	if before.Generation < 0 {
		return fmt.Errorf("%w: prior generation must be non-negative", ErrInvalidMutation)
	}
	switch rule {
	case GenerationKeep:
		if after.Generation != before.Generation {
			return fmt.Errorf("%w: resulting generation=%d, want unchanged %d", ErrInvalidMutationState, after.Generation, before.Generation)
		}
	case GenerationStart:
		if after.Generation != 1 {
			return fmt.Errorf("%w: resulting generation=%d, want 1", ErrInvalidMutationState, after.Generation)
		}
	case GenerationIncrement:
		if before.Generation == maxLeaseGeneration {
			return fmt.Errorf("%w: generation would overflow at %d", ErrInvalidMutationState, before.Generation)
		}
		want := before.Generation + 1
		if after.Generation != want {
			return fmt.Errorf("%w: resulting generation=%d, want %d", ErrInvalidMutationState, after.Generation, want)
		}
	default:
		return fmt.Errorf("%w: unknown generation rule %q", ErrInvalidMutation, rule)
	}
	return nil
}

// ValidateTransition rejects an unknown or impossible state transition before
// any WAL, SQLite, DNS, or IPAM side effect is attempted.
func ValidateTransition(kind MutationKind, before *Lease) (MutationContract, error) {
	contract, err := ContractFor(kind)
	if err != nil {
		return MutationContract{}, err
	}
	if before == nil {
		if kind == MutationOffer || kind == MutationBind || kind == MutationDecline {
			return contract, nil
		}
		return MutationContract{}, fmt.Errorf("%w: %s requires an existing lease", ErrInvalidMutationState, kind)
	}
	if !contract.Allows(before.Status) {
		return MutationContract{}, fmt.Errorf("%w: %s cannot transition from %s", ErrInvalidMutationState, kind, before.Status)
	}
	return contract, nil
}
