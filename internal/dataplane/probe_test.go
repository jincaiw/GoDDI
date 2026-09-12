package dataplane

import (
	"context"
	"errors"
	"testing"

	"github.com/jasonwa/goddi/internal/config"
)

// Readiness levels, and why "degraded" is not "down".
//
// The whole point of the data-plane split is that a plane keeps serving its
// local copy while the control plane is away. A probe that calls that state
// "failing" would have an orchestrator restart the one process that is still
// handing out addresses, which is the outage the split removes. So the level
// rules are pinned here rather than left to the wiring.

// TestAnIdlePlaneWithTheControlDatabaseReachableIsOK is the baseline: nothing
// to report, nothing wrong.
func TestAnIdlePlaneWithTheControlDatabaseReachableIsOK(t *testing.T) {
	control, _, rep := newPair(t)
	insertControlScope(t, control, "s1", "lan", "10.0.0.0/24", "10.0.0.10", "10.0.0.20")

	runner := NewRunner(rep, RunnerConfig{Domains: []Domain{DomainDHCP}, PushLeases: true})
	if _, err := rep.Sync(context.Background(), DomainDHCP); err != nil {
		t.Fatalf("sync: %v", err)
	}
	runner.recordSuccess()

	rd := runner.evaluate(runner.Snapshot())
	if rd.Level != LevelOK {
		t.Fatalf("level = %q (reasons %v), want %q: a store that can read its "+
			"replica and reach the control database has nothing to report",
			rd.Level, rd.Reasons, LevelOK)
	}
}

// TestAPlaneServingItsLocalCopyIsDegradedNotFailing is the load-bearing rule.
// The control database is gone; the local copy is in service. That is a
// caveat for an operator and not a reason to take the process out of service.
func TestAPlaneServingItsLocalCopyIsDegradedNotFailing(t *testing.T) {
	_, _, rep := newPair(t)
	if _, err := rep.Sync(context.Background(), DomainDHCP); err != nil {
		t.Fatalf("sync: %v", err)
	}

	runner := NewRunner(rep, RunnerConfig{Domains: []Domain{DomainDHCP}, PushLeases: true})
	runner.recordFailure(errors.New("control database is unreachable"))

	rd := runner.evaluate(runner.Snapshot())
	if rd.Level != LevelDegraded {
		t.Fatalf("level = %q (reasons %v), want %q: the local copy is in "+
			"service, so this is a caveat, not a failure",
			rd.Level, rd.Reasons, LevelDegraded)
	}
	if !contains(rd.Reasons, "control_unreachable") {
		t.Errorf("reasons = %v, want control_unreachable among them", rd.Reasons)
	}
}

// TestAPlaneThatCannotReadItsReplicaStateIsFailing: this one is not a caveat.
// A process that cannot say what it is serving must not be reported as able
// to serve it.
func TestAPlaneThatCannotReadItsReplicaStateIsFailing(t *testing.T) {
	_, store, rep := newPair(t)
	if _, err := rep.Sync(context.Background(), DomainDHCP); err != nil {
		t.Fatalf("sync: %v", err)
	}
	runner := NewRunner(rep, RunnerConfig{Domains: []Domain{DomainDHCP}, PushLeases: true})
	runner.recordSuccess()

	// Close the store: the replica state can no longer be read.
	store.Close()

	rd := runner.evaluate(runner.Snapshot())
	if rd.Level != LevelFailing {
		t.Fatalf("level = %q (reasons %v), want %q", rd.Level, rd.Reasons, LevelFailing)
	}
	if !hasReasonPrefix(rd.Reasons, "replica_state_unreadable:") {
		t.Errorf("reasons = %v, want a replica_state_unreadable entry", rd.Reasons)
	}
}

// TestACrossedQuotaIsDegradedAndNamesTheBound: crossing a bound raises the
// level and says which one, so the reason is actionable without the operator
// having to guess.
func TestACrossedQuotaIsDegradedAndNamesTheBound(t *testing.T) {
	_, store, rep := newPair(t)
	if _, err := rep.Sync(context.Background(), DomainDHCP); err != nil {
		t.Fatalf("sync: %v", err)
	}
	insertLeases(t, store, 3)

	runner := NewRunner(rep, RunnerConfig{
		Domains:    []Domain{DomainDHCP},
		PushLeases: true,
		Quota:      Quota{MaxLeases: 1},
	})
	runner.recordSuccess()

	rd := runner.evaluate(runner.Snapshot())
	if rd.Level != LevelDegraded {
		t.Fatalf("level = %q (reasons %v), want %q: a crossed bound is a signal, "+
			"not a reason to stop serving", rd.Level, rd.Reasons, LevelDegraded)
	}
	if !contains(rd.Reasons, "quota_exceeded") {
		t.Errorf("reasons = %v, want quota_exceeded among them", rd.Reasons)
	}
	if len(rd.Quota) != 1 || rd.Quota[0].What != "leases" {
		t.Errorf("quota = %v, want exactly the leases bound", rd.Quota)
	}
}

// TestTheWorstLevelWins: a process reports one level, so the more severe of
// the planes' answers has to be the one that survives.
func TestTheWorstLevelWins(t *testing.T) {
	_, store, rep := newPair(t)
	if _, err := rep.Sync(context.Background(), DomainDHCP); err != nil {
		t.Fatalf("sync: %v", err)
	}
	insertLeases(t, store, 3)

	runner := NewRunner(rep, RunnerConfig{
		Domains:    []Domain{DomainDHCP},
		PushLeases: true,
		Quota:      Quota{MaxLeases: 1},
	})
	// Both conditions at once: a crossed quota and an unreachable control
	// database. Neither outranks the other, and both are degraded.
	runner.recordFailure(errors.New("gone"))

	rd := runner.evaluate(runner.Snapshot())
	if rd.Level != LevelDegraded {
		t.Fatalf("level = %q, want %q", rd.Level, LevelDegraded)
	}
	if !contains(rd.Reasons, "control_unreachable") || !contains(rd.Reasons, "quota_exceeded") {
		t.Errorf("reasons = %v, want both conditions named", rd.Reasons)
	}
}

// TestTheProbeIsAnsweredBeforeTheFirstPass: a process that has just started
// is not failing, and it is not ok either -- nothing has been checked yet.
func TestTheProbeIsAnsweredBeforeTheFirstPass(t *testing.T) {
	_, _, rep := newPair(t)
	runner := NewRunner(rep, RunnerConfig{Domains: []Domain{DomainDHCP}, PushLeases: true})

	rd := runner.Probe()
	if rd.Level != LevelDegraded {
		t.Fatalf("level = %q, want %q before the first pass", rd.Level, LevelDegraded)
	}
	if !contains(rd.Reasons, "no_pass_yet") {
		t.Errorf("reasons = %v, want no_pass_yet", rd.Reasons)
	}
}

// TestProbingReadsMemoryOnly is what makes it safe to answer /ready without a
// credential: the probe must not take the store's single connection, because
// an unauthenticated caller could otherwise starve the request path it is
// reporting on.
//
// The store is closed after the pass, so a probe that touched it would have
// to report failing. It still reports what the last pass found.
func TestProbingReadsMemoryOnly(t *testing.T) {
	_, store, rep := newPair(t)
	if _, err := rep.Sync(context.Background(), DomainDHCP); err != nil {
		t.Fatalf("sync: %v", err)
	}
	runner := NewRunner(rep, RunnerConfig{Domains: []Domain{DomainDHCP}, PushLeases: true})
	runner.recordSuccess()
	runner.refreshProbe()

	before := runner.Probe()
	if before.Level != LevelOK {
		t.Fatalf("level = %q before closing the store, want %q", before.Level, LevelOK)
	}

	store.Close()

	after := runner.Probe()
	if after.Level != before.Level {
		t.Fatalf("level changed to %q after the store was closed: the probe is "+
			"reading the store instead of the last pass's view", after.Level)
	}
	if after.Reasons != nil {
		t.Errorf("reasons = %v, want none: the cached view had none", after.Reasons)
	}
}

// TestADNSStoreIsNotMeasuredAgainstTheLeaseCeiling: a DNS store syncs the
// scopes but owns no lease, so the lease count it reports is zero by
// construction and the bound must not fire off it.
func TestADNSStoreIsNotMeasuredAgainstTheLeaseCeiling(t *testing.T) {
	control, _, _ := newPair(t)
	insertControlScope(t, control, "s1", "lan", "10.0.0.0/24", "10.0.0.10", "10.0.0.20")

	zoneStore := newStore(t, config.DataPlaneZone)
	rep := NewReplicator(control, zoneStore)
	runner := NewRunner(rep, RunnerConfig{
		Domains:     []Domain{DomainDNS},
		PushRecords: true,
		Quota:       Quota{MaxLeases: 1},
	})
	if _, err := rep.Sync(context.Background(), DomainDNS); err != nil {
		t.Fatalf("sync: %v", err)
	}
	runner.recordSuccess()

	rd := runner.evaluate(runner.Snapshot())
	if rd.Level != LevelOK {
		t.Fatalf("level = %q (reasons %v), want %q: a store that owns no lease "+
			"cannot be over the lease ceiling", rd.Level, rd.Reasons, LevelOK)
	}
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func hasReasonPrefix(haystack []string, prefix string) bool {
	for _, s := range haystack {
		if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}
