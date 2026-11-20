package lease

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func pipelineBuilder(value *Lease) WALEvent {
	event := WALEvent{Version: currentWALEventVersion, Op: WALEventUpsert}
	copy := *value
	event.Lease = &copy
	return event
}

// TestWALPipelineRecoversBeforeAdmissionAndProjects is the fail-closed startup
// contract: a durable event that was never projected must be applied to SQLite
// by OpenWALPipeline *before* it returns, so admission never begins on a stale
// projection.
func TestWALPipelineRecoversBeforeAdmissionAndProjects(t *testing.T) {
	store := newLeaseStore(t)
	seedScope(t, store.DB, "scope-1", "scope", "192.0.2.0/24", "192.0.2.10", "192.0.2.20")
	path := filepath.Join(t.TempDir(), "leases.wal")

	// Simulate a crash that left a durable-but-unprojected event in the WAL.
	wal, err := OpenWAL(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wal.AppendDurable(WALEvent{Version: currentWALEventVersion, Op: WALEventUpsert, Lease: &Lease{
		ID: "crash-1", ScopeID: "scope-1", IPAddress: "192.0.2.10", MACAddress: "aa:aa",
		Status: LeaseStatusOffered,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := wal.Close(); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	pipeline, err := OpenWALPipeline(ctx, WALPipelineConfig{Path: path, DB: store.DB, Capacity: 8, NextOp: pipelineBuilder})
	if err != nil {
		t.Fatalf("OpenWALPipeline: %v", err)
	}
	defer pipeline.Close(context.Background())

	if got := pipeline.Applied(); got != 1 {
		t.Fatalf("applied after recovery = %d, want 1", got)
	}
	var status string
	if err := store.DB.QueryRow(`SELECT status FROM dhcp_leases WHERE id = ?`, "crash-1").Scan(&status); err != nil {
		t.Fatalf("the durable event was not projected before admission returned: %v", err)
	}
	if status != string(LeaseStatusOffered) {
		t.Fatalf("status = %q, want offered", status)
	}
}

// TestWALPipelineAdmitIsDurableThenProjected proves an admitted lease crosses
// the WAL boundary and reaches the SQLite projection after the ordered drain.
func TestWALPipelineAdmitIsDurableThenProjected(t *testing.T) {
	store := newLeaseStore(t)
	seedScope(t, store.DB, "scope-1", "scope", "192.0.2.0/24", "192.0.2.10", "192.0.2.20")
	path := filepath.Join(t.TempDir(), "leases.wal")
	ctx := context.Background()

	pipeline, err := OpenWALPipeline(ctx, WALPipelineConfig{Path: path, DB: store.DB, Capacity: 8, NextOp: pipelineBuilder})
	if err != nil {
		t.Fatal(err)
	}
	if err := pipeline.Admit(ctx, Lease{ID: "admit-1", ScopeID: "scope-1", IPAddress: "192.0.2.11", MACAddress: "bb:bb", Status: LeaseStatusActive, Generation: 1}); err != nil {
		t.Fatalf("Admit: %v", err)
	}
	if _, ok := pipeline.Index().Get("admit-1"); !ok {
		t.Fatal("the admitted lease is not visible in the read index")
	}
	if err := pipeline.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	var count int
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM dhcp_leases WHERE id = ?`, "admit-1").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("admitted lease projected %d times, want 1", count)
	}
}

// TestWALPipelineCloseDrainsAcceptedQueue pins that accepted work is not lost
// on a graceful close: every admitted event projects before Close returns.
func TestWALPipelineCloseDrainsAcceptedQueue(t *testing.T) {
	store := newLeaseStore(t)
	seedScope(t, store.DB, "scope-1", "scope", "192.0.2.0/24", "192.0.2.10", "192.0.2.30")
	path := filepath.Join(t.TempDir(), "leases.wal")
	ctx := context.Background()

	pipeline, err := OpenWALPipeline(ctx, WALPipelineConfig{Path: path, DB: store.DB, Capacity: 32, NextOp: pipelineBuilder})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		id := "drain-" + string(rune('a'+i))
		if err := pipeline.Admit(ctx, Lease{ID: id, ScopeID: "scope-1", IPAddress: "192.0.2." + itoa(10+i), MACAddress: "cc:" + itoa(10+i), Status: LeaseStatusActive, Generation: 1}); err != nil {
			t.Fatalf("Admit %s: %v", id, err)
		}
	}
	if err := pipeline.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	var count int
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM dhcp_leases`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 10 {
		t.Fatalf("projected %d leases after drain, want 10", count)
	}
	if pipeline.Applied() != 10 {
		t.Fatalf("applied = %d, want 10", pipeline.Applied())
	}
}

// TestWALPipelineRejectsAdmissionAfterClose pins the cutoff: a closed pipeline
// refuses new work rather than accepting it into a dead applier.
func TestWALPipelineRejectsAdmissionAfterClose(t *testing.T) {
	store := newLeaseStore(t)
	seedScope(t, store.DB, "scope-1", "scope", "192.0.2.0/24", "192.0.2.10", "192.0.2.20")
	path := filepath.Join(t.TempDir(), "leases.wal")
	ctx := context.Background()

	pipeline, err := OpenWALPipeline(ctx, WALPipelineConfig{Path: path, DB: store.DB, Capacity: 8, NextOp: pipelineBuilder})
	if err != nil {
		t.Fatal(err)
	}
	if err := pipeline.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := pipeline.Admit(ctx, Lease{ID: "late", ScopeID: "scope-1", IPAddress: "192.0.2.12", MACAddress: "dd:dd", Status: LeaseStatusActive, Generation: 1}); err == nil {
		t.Fatal("Admit succeeded after Close")
	}
	if err := pipeline.Close(ctx); !errors.Is(err, ErrWALLifecycleClosed) {
		t.Fatalf("second Close = %v, want ErrWALLifecycleClosed", err)
	}
}

func TestOpenWALPipelineRejectsBadConfig(t *testing.T) {
	store := newLeaseStore(t)
	ctx := context.Background()
	cases := []WALPipelineConfig{
		{Path: "", DB: store.DB, Capacity: 8, NextOp: pipelineBuilder},
		{Path: filepath.Join(t.TempDir(), "x.wal"), DB: nil, Capacity: 8, NextOp: pipelineBuilder},
		{Path: filepath.Join(t.TempDir(), "x.wal"), DB: store.DB, Capacity: 0, NextOp: pipelineBuilder},
		{Path: filepath.Join(t.TempDir(), "x.wal"), DB: store.DB, Capacity: 8, NextOp: nil},
	}
	for i, cfg := range cases {
		if _, err := OpenWALPipeline(ctx, cfg); !errors.Is(err, ErrWALPipelineConfig) {
			t.Fatalf("case %d: error = %v, want ErrWALPipelineConfig", i, err)
		}
	}
	if _, err := OpenWALPipeline(nil, WALPipelineConfig{Path: "x", DB: store.DB, Capacity: 8, NextOp: pipelineBuilder}); !errors.Is(err, ErrWALPipelineConfig) { //nolint:staticcheck // deliberate nil-context negative test
		t.Fatalf("nil context error = %v, want ErrWALPipelineConfig", err)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [4]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
