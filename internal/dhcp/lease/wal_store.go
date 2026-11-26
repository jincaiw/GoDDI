package lease

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jincaiw/GoDDI/internal/facts"
)

const leaseTimeLayout = "2006-01-02T15:04:05Z"

// WALLeaseStore is the event-sourced lease store: it keeps the authoritative
// lease state in a MemoryIndex, makes every mutation durable in a WAL before it
// returns (so an ACK cannot outrun durability), and projects the resulting
// snapshot to SQLite asynchronously. When a source and a space resolver are
// configured it also derives the unified fact envelope from the same WAL event
// and enqueues it in the same projection transaction, so the facts chain is fed
// by the WAL rather than by a separate synchronous writer.
//
// It implements the DHCP server's lease store contract with the same observable
// transitions as *Manager, and is an explicit opt-in component.
type WALLeaseStore struct {
	db           *sql.DB
	audit        *Manager
	pipeline     *WALPipeline
	source       string
	resolveSpace func(scopeID, ip string) (string, error)
	allocator    *facts.SequenceAllocator
	outbox       *facts.ObservationOutbox
	factsEnabled bool
	now          func() time.Time
	newID        func() string
}

// WALLeaseStoreConfig configures the event-sourced store.
type WALLeaseStoreConfig struct {
	Path     string // WAL file
	DB       *sql.DB
	Capacity int
	// Source and ResolveSpaceID enable fact emission from the WAL. When either
	// is empty the store only maintains the lease projection.
	Source         string
	ResolveSpaceID func(scopeID, ip string) (string, error)
	// HashChain, when true, stamps emitted facts with the D-03 hash chain.
	HashChain bool
}

// OpenWALLeaseStore opens the WAL, recovers the SQLite projection, loads the
// lease/pool snapshot into the read index, and returns a ready store.
func OpenWALLeaseStore(ctx context.Context, cfg WALLeaseStoreConfig) (*WALLeaseStore, error) {
	if ctx == nil {
		return nil, errors.New("lease wal store: context is nil")
	}
	if cfg.Path == "" {
		return nil, errors.New("lease wal store: WAL path is empty")
	}
	if cfg.DB == nil {
		return nil, errors.New("lease wal store: database is nil")
	}
	if cfg.Capacity <= 0 {
		return nil, fmt.Errorf("lease wal store: capacity must be positive")
	}
	s := &WALLeaseStore{
		db:           cfg.DB,
		audit:        NewManager(cfg.DB),
		source:       strings.TrimSpace(cfg.Source),
		resolveSpace: cfg.ResolveSpaceID,
		now:          func() time.Time { return time.Now().UTC() },
		newID:        func() string { return uuid.New().String() },
	}
	if s.source != "" && s.resolveSpace != nil {
		allocator, err := facts.NewSequenceAllocator(cfg.DB)
		if err != nil {
			return nil, fmt.Errorf("lease wal store: sequence allocator: %w", err)
		}
		var outbox *facts.ObservationOutbox
		if cfg.HashChain {
			outbox, err = facts.NewObservationOutboxStamped(cfg.DB, "dhcp_lease_facts")
		} else {
			outbox, err = facts.NewObservationOutbox(cfg.DB)
		}
		if err != nil {
			return nil, fmt.Errorf("lease wal store: observation outbox: %w", err)
		}
		s.allocator = allocator
		s.outbox = outbox
		s.factsEnabled = true
	}
	pipeline, err := OpenWALPipeline(ctx, WALPipelineConfig{
		Path:     cfg.Path,
		DB:       cfg.DB,
		Capacity: cfg.Capacity,
		NextOp: func(v *Lease) WALEvent {
			event := WALEvent{Version: currentWALEventVersion, Op: WALEventUpsert}
			copy := *v
			event.Lease = &copy
			return event
		},
		Apply: s.applyEvent,
	})
	if err != nil {
		return nil, err
	}
	s.pipeline = pipeline
	leases, pools, err := loadLeaseSnapshot(ctx, cfg.DB)
	if err != nil {
		_ = pipeline.Close(context.Background())
		return nil, err
	}
	if err := pipeline.Index().ReplaceChecked(leases, pools); err != nil {
		_ = pipeline.Close(context.Background())
		return nil, fmt.Errorf("lease wal store: publish snapshot: %w", err)
	}
	return s, nil
}

// applyEvent is the projection applier: lease snapshot + watermark, and, when
// enabled, the fact envelope for this mutation, all in one transaction.
func (s *WALLeaseStore) applyEvent(ctx context.Context, event WALEvent) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("lease wal store: begin projection: %w", err)
	}
	applied, err := ApplyWALEventTx(ctx, tx, event)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	if applied && s.factsEnabled && event.SpaceID != "" && event.Lease != nil && isFactKind(event.Mutation) {
		if err := s.enqueueFact(ctx, tx, event); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("lease wal store: commit projection: %w", err)
	}
	return nil
}

func (s *WALLeaseStore) enqueueFact(ctx context.Context, tx *sql.Tx, event WALEvent) error {
	sequence, err := s.allocator.NextTx(ctx, tx)
	if err != nil {
		return fmt.Errorf("lease wal store: allocate fact sequence: %w", err)
	}
	identity, err := NewMutationFactIdentity(s.source, event.SpaceID, event.Mutation, event.Lease.ID, event.Lease.Generation)
	if err != nil {
		return fmt.Errorf("lease wal store: fact identity: %w", err)
	}
	command := MutationCommand{Kind: event.Mutation, Before: event.Before, After: event.Lease}
	envelope, err := command.FactEnvelope(identity.EventID, s.source, event.SpaceID, sequence, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("lease wal store: fact envelope: %w", err)
	}
	if err := s.outbox.EnqueueTx(ctx, tx, envelope); err != nil {
		return fmt.Errorf("lease wal store: enqueue fact: %w", err)
	}
	return nil
}

func isFactKind(kind MutationKind) bool {
	switch kind {
	case MutationBind, MutationActivate, MutationRenew, MutationRelease, MutationDecline, MutationExpire:
		return true
	default:
		return false
	}
}

// ReloadFromDB re-reads the lease and pool snapshot from SQLite into the read
// index. It is the standby->primary promotion hook: a standby's SQLite lease DB
// is kept current by the HA mirror while the standby never builds a DHCP server
// (standsAside); when this node is promoted and starts serving, it must publish
// an index built from that already-mirrored snapshot before answering. Call it
// only when the store is not serving traffic (promotion/startup).
func (s *WALLeaseStore) ReloadFromDB(ctx context.Context) error {
	if s == nil || s.pipeline == nil {
		return ErrWALPipelineClosed
	}
	leases, pools, err := loadLeaseSnapshot(ctx, s.db)
	if err != nil {
		return fmt.Errorf("lease wal store: reload snapshot: %w", err)
	}
	if err := s.pipeline.Index().ReplaceChecked(leases, pools); err != nil {
		return fmt.Errorf("lease wal store: reload publish: %w", err)
	}
	return nil
}

func (s *WALLeaseStore) Close(ctx context.Context) error { return s.pipeline.Close(ctx) }
func (s *WALLeaseStore) Drain(ctx context.Context) error { return s.pipeline.Drain(ctx) }
func (s *WALLeaseStore) Index() *MemoryIndex             { return s.pipeline.Index() }

// admit records one mutation: resolve the IPAM space when it carries a fact,
// then cross the WAL durable boundary and project.
func (s *WALLeaseStore) admit(ctx context.Context, kind MutationKind, before *Lease, after Lease) (Lease, error) {
	spaceID := ""
	if s.factsEnabled && isFactKind(kind) {
		if s.resolveSpace == nil {
			return Lease{}, errors.New("lease wal store: fact mutation without a space resolver")
		}
		sid, err := s.resolveSpace(after.ScopeID, after.IPAddress)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return Lease{}, fmt.Errorf("lease wal store: resolve space: %w", err)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			spaceID = strings.TrimSpace(sid)
		}
	}
	copyAfter := after
	event := WALEvent{Version: currentWALEventVersion, Op: WALEventUpsert, Mutation: kind, SpaceID: spaceID}
	if before != nil {
		bc := *before
		event.Before = &bc
	}
	event.Lease = &copyAfter
	if err := s.pipeline.admitEvent(ctx, event); err != nil {
		return Lease{}, err
	}
	return copyAfter, nil
}

func (s *WALLeaseStore) heldIn(ip string, statuses ...LeaseStatus) (*Lease, bool) {
	l, ok := s.pipeline.Index().HeldByIP(ip)
	if !ok {
		return nil, false
	}
	for _, st := range statuses {
		if l.Status == st {
			return l, true
		}
	}
	return nil, false
}

func (s *WALLeaseStore) scopeHoldsIP(scopeID, ip string) (*Lease, bool) {
	l, ok := s.pipeline.Index().HeldByIP(ip)
	if !ok || l.ScopeID != scopeID || !isHeld(l.Status) {
		return nil, false
	}
	return l, true
}

func (s *WALLeaseStore) CreateLease(scopeID, ip, mac, hostname string, duration time.Duration) (*Lease, error) {
	if held, ok := s.scopeHoldsIP(scopeID, ip); ok && held.MACAddress != mac {
		return nil, fmt.Errorf("failed to create lease: address %s held by lease %s", ip, held.ID)
	}
	now := s.now()
	value := Lease{
		ID: s.newID(), ScopeID: scopeID, IPAddress: ip, MACAddress: mac, Hostname: hostname,
		LeaseStart: now.Format(leaseTimeLayout), LeaseEnd: now.Add(duration).Format(leaseTimeLayout),
		Status: LeaseStatusActive, LastSeen: now.Format(leaseTimeLayout), Generation: 1,
	}
	created, err := s.admit(context.Background(), MutationBind, nil, value)
	if err != nil {
		return nil, fmt.Errorf("failed to create lease: %w", err)
	}
	s.audit.auditBind(nil, &created)
	return &created, nil
}

func (s *WALLeaseStore) ReserveAddress(scopeID, ip, mac, hostname string) (*Lease, error) {
	for _, st := range []LeaseStatus{LeaseStatusActive, LeaseStatusOffered} {
		if held, ok := s.heldIn(ip, st); ok {
			if held.MACAddress != mac {
				return nil, ErrAddressTaken
			}
			if held.Status == LeaseStatusOffered {
				return s.refreshHold(*held)
			}
			return held, nil
		}
	}
	now := s.now()
	value := Lease{
		ID: s.newID(), ScopeID: scopeID, IPAddress: ip, MACAddress: mac, Hostname: hostname,
		LeaseStart: now.Format(leaseTimeLayout), LeaseEnd: now.Add(offerHold).Format(leaseTimeLayout),
		Status: LeaseStatusOffered, LastSeen: now.Format(leaseTimeLayout), Generation: 0,
	}
	created, err := s.admit(context.Background(), MutationOffer, nil, value)
	if err != nil {
		return nil, err
	}
	return &created, nil
}

func (s *WALLeaseStore) refreshHold(current Lease) (*Lease, error) {
	now := s.now()
	updated := current
	updated.LeaseEnd = now.Add(offerHold).Format(leaseTimeLayout)
	updated.LastSeen = now.Format(leaseTimeLayout)
	out, err := s.admit(context.Background(), MutationOffer, &current, updated)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh offer: %w", err)
	}
	return &out, nil
}

func (s *WALLeaseStore) ActivateLease(id string, duration time.Duration) (*Lease, error) {
	current, ok := s.pipeline.Index().Get(id)
	if !ok || (current.Status != LeaseStatusOffered && current.Status != LeaseStatusActive) {
		return nil, fmt.Errorf("lease %s is not in an activatable state", id)
	}
	if current.Generation >= maxLeaseGeneration {
		return nil, fmt.Errorf("%w: activate generation would overflow at %d", ErrInvalidMutationState, current.Generation)
	}
	now := s.now()
	after := *current
	after.LeaseStart = now.Format(leaseTimeLayout)
	after.LeaseEnd = now.Add(duration).Format(leaseTimeLayout)
	after.Status = LeaseStatusActive
	after.LastSeen = now.Format(leaseTimeLayout)
	after.Generation = current.Generation + 1
	activated, err := s.admit(context.Background(), MutationActivate, current, after)
	if err != nil {
		return nil, fmt.Errorf("failed to activate lease: %w", err)
	}
	if current.Status == LeaseStatusOffered {
		s.audit.auditBind(current, &activated)
	}
	return &activated, nil
}

func (s *WALLeaseStore) RenewLease(id string, duration time.Duration) (*Lease, error) {
	current, ok := s.pipeline.Index().Get(id)
	if !ok || current.Status != LeaseStatusActive {
		return nil, fmt.Errorf("lease %s not found or not active", id)
	}
	if current.Generation >= maxLeaseGeneration {
		return nil, fmt.Errorf("%w: renew generation would overflow at %d", ErrInvalidMutationState, current.Generation)
	}
	now := s.now()
	after := *current
	after.LeaseStart = now.Format(leaseTimeLayout)
	after.LeaseEnd = now.Add(duration).Format(leaseTimeLayout)
	after.LastSeen = now.Format(leaseTimeLayout)
	after.Generation = current.Generation + 1
	renewed, err := s.admit(context.Background(), MutationRenew, current, after)
	if err != nil {
		return nil, fmt.Errorf("failed to renew lease: %w", err)
	}
	return &renewed, nil
}

func (s *WALLeaseStore) GetLease(id string) (*Lease, error) {
	l, ok := s.pipeline.Index().Get(id)
	if !ok {
		return nil, fmt.Errorf("lease not found")
	}
	return l, nil
}

func (s *WALLeaseStore) GetLeaseByMAC(mac string) (*Lease, error) {
	l, ok := s.pipeline.Index().ByMAC(mac)
	if !ok || l.Status != LeaseStatusActive {
		return nil, nil
	}
	return l, nil
}

func (s *WALLeaseStore) GetHeldLeaseByIP(ip string) (*Lease, error) {
	if held, ok := s.heldIn(ip, LeaseStatusActive, LeaseStatusOffered); ok {
		return held, nil
	}
	return nil, nil
}

func (s *WALLeaseStore) FindAvailableIP(scopeID string) (string, error) {
	return s.pipeline.Index().FindAvailableIP(scopeID)
}

func (s *WALLeaseStore) ReleaseLease(id string) error {
	current, ok := s.pipeline.Index().Get(id)
	if !ok {
		return nil
	}
	if current.Status != LeaseStatusActive && current.Status != LeaseStatusOffered {
		return nil
	}
	now := s.now()
	after := *current
	after.Status = LeaseStatusReleased
	after.LastSeen = now.Format(leaseTimeLayout)
	released, err := s.admit(context.Background(), MutationRelease, current, after)
	if err != nil {
		return fmt.Errorf("failed to release lease: %w", err)
	}
	s.audit.auditRelease(current, &released)
	return nil
}

func (s *WALLeaseStore) QuarantineIP(scopeID, ip, mac string) (*Lease, error) {
	if scopeID == "" {
		return nil, fmt.Errorf("cannot quarantine %s without a scope", ip)
	}
	now := s.now()
	if held, ok := s.heldIn(ip, LeaseStatusActive, LeaseStatusOffered, LeaseStatusConflict); ok && held.ScopeID == scopeID {
		after := *held
		after.Status = LeaseStatusConflict
		after.LeaseEnd = now.Add(declineQuarantine).Format(leaseTimeLayout)
		after.LastSeen = now.Format(leaseTimeLayout)
		conflicted, err := s.admit(context.Background(), MutationDecline, held, after)
		if err != nil {
			return nil, fmt.Errorf("failed to mark lease as conflict: %w", err)
		}
		s.audit.auditDecline(held, &conflicted)
		return &conflicted, nil
	}
	tombstone := Lease{
		ID: s.newID(), ScopeID: scopeID, IPAddress: ip, MACAddress: mac,
		LeaseStart: now.Format(leaseTimeLayout), LeaseEnd: now.Add(declineQuarantine).Format(leaseTimeLayout),
		Status: LeaseStatusConflict, LastSeen: now.Format(leaseTimeLayout), Generation: 0,
	}
	created, err := s.admit(context.Background(), MutationDecline, nil, tombstone)
	if err != nil {
		return nil, fmt.Errorf("failed to write conflict tombstone for %s: %w", ip, err)
	}
	s.audit.auditDecline(nil, &created)
	return &created, nil
}

func (s *WALLeaseStore) ExpireLeases() ([]*Lease, error) {
	now := s.now()
	var swept []*Lease
	for _, l := range s.pipeline.Index().Snapshot() {
		if !isHeld(l.Status) {
			continue
		}
		end, err := time.Parse(leaseTimeLayout, l.LeaseEnd)
		if err != nil {
			continue
		}
		if end.After(now) {
			continue
		}
		before := l
		after := l
		after.Status = LeaseStatusExpired
		expired, err := s.admit(context.Background(), MutationExpire, &before, after)
		if err != nil {
			return swept, fmt.Errorf("failed to expire lease %s: %w", l.ID, err)
		}
		s.audit.auditExpire(&before, &expired)
		swept = append(swept, &expired)
	}
	return swept, nil
}

func loadLeaseSnapshot(ctx context.Context, db *sql.DB) ([]Lease, []AddressPool, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, scope_id, ip_address, mac_address, hostname, client_id,
		lease_start, lease_end, status, last_seen, generation FROM dhcp_leases`)
	if err != nil {
		return nil, nil, fmt.Errorf("lease wal store: read leases: %w", err)
	}
	defer rows.Close()
	var leases []Lease
	for rows.Next() {
		var l Lease
		var hostname, clientID sql.NullString
		if err := rows.Scan(&l.ID, &l.ScopeID, &l.IPAddress, &l.MACAddress, &hostname, &clientID,
			&l.LeaseStart, &l.LeaseEnd, &l.Status, &l.LastSeen, &l.Generation); err != nil {
			return nil, nil, fmt.Errorf("lease wal store: scan lease: %w", err)
		}
		l.Hostname = hostname.String
		l.ClientID = clientID.String
		leases = append(leases, l)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	pools, err := loadAddressPools(ctx, db)
	if err != nil {
		return nil, nil, err
	}
	return leases, pools, nil
}

func loadAddressPools(ctx context.Context, db *sql.DB) ([]AddressPool, error) {
	reserved := map[string]map[string]bool{}
	rrows, err := db.QueryContext(ctx, `SELECT scope_id, ip_address FROM dhcp_reservations WHERE enabled = 1`)
	if err != nil {
		return nil, fmt.Errorf("lease wal store: read reservations: %w", err)
	}
	for rrows.Next() {
		var scopeID, ip string
		if err := rrows.Scan(&scopeID, &ip); err != nil {
			rrows.Close()
			return nil, err
		}
		if reserved[scopeID] == nil {
			reserved[scopeID] = map[string]bool{}
		}
		if net.ParseIP(ip).To4() != nil {
			reserved[scopeID][ip] = true
		}
	}
	if err := rrows.Err(); err != nil {
		rrows.Close()
		return nil, err
	}
	rrows.Close()

	srows, err := db.QueryContext(ctx, `SELECT id, start_ip, end_ip FROM dhcp_scopes WHERE enabled = 1`)
	if err != nil {
		return nil, fmt.Errorf("lease wal store: read scopes: %w", err)
	}
	defer srows.Close()
	var pools []AddressPool
	for srows.Next() {
		var id, start, end string
		if err := srows.Scan(&id, &start, &end); err != nil {
			return nil, err
		}
		pools = append(pools, AddressPool{ScopeID: id, StartIP: start, EndIP: end, Reservations: reserved[id]})
	}
	return pools, srows.Err()
}
