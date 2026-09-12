package lease

import (
	"bytes"
	"errors"
	"testing"
)

func TestRecoverMemoryIndexPublishesBaselineAndWALAtomically(t *testing.T) {
	target := NewMemoryIndex()
	target.Replace([]Lease{{ID: "old", IPAddress: "192.0.2.9", Status: LeaseStatusActive}}, nil)
	baseline := []Lease{{ID: "l1", IPAddress: "192.0.2.10", ScopeID: "scope-1", Status: LeaseStatusActive}}
	var wal bytes.Buffer
	if err := AppendWALEvent(&wal, WALEvent{Version: 1, Seq: 1, Op: WALEventUpsert, Lease: &Lease{ID: "l2", IPAddress: "192.0.2.11", ScopeID: "scope-1", Status: LeaseStatusOffered}}); err != nil {
		t.Fatal(err)
	}
	last, err := RecoverMemoryIndex(target, baseline, []AddressPool{{ScopeID: "scope-1", StartIP: "192.0.2.10", EndIP: "192.0.2.12"}}, bytes.NewReader(wal.Bytes()), 0)
	if err != nil || last != 1 {
		t.Fatalf("recovery = %d, %v", last, err)
	}
	if got, ok := target.Get("l1"); !ok || got.IPAddress != "192.0.2.10" {
		t.Fatalf("baseline lease = %+v, %v", got, ok)
	}
	if got, ok := target.Get("l2"); !ok || got.IPAddress != "192.0.2.11" {
		t.Fatalf("WAL lease = %+v, %v", got, ok)
	}
	if _, ok := target.Get("old"); ok {
		t.Fatal("old index was not atomically replaced")
	}
}

func TestRecoverMemoryIndexLeavesTargetUntouchedOnGap(t *testing.T) {
	target := NewMemoryIndex()
	target.Replace([]Lease{{ID: "old", IPAddress: "192.0.2.9", Status: LeaseStatusActive}}, nil)
	var wal bytes.Buffer
	if err := AppendWALEvent(&wal, WALEvent{Version: 1, Seq: 2, Op: WALEventRemove, LeaseID: "old"}); err != nil {
		t.Fatal(err)
	}
	if _, err := RecoverMemoryIndex(target, nil, nil, bytes.NewReader(wal.Bytes()), 0); !errors.Is(err, ErrWALGap) {
		t.Fatalf("error = %v, want ErrWALGap", err)
	}
	if got, ok := target.Get("old"); !ok || got.IPAddress != "192.0.2.9" {
		t.Fatalf("target changed after failed recovery: %+v, %v", got, ok)
	}
}

func TestWALFileRecoversMemoryIndexFromOpenFile(t *testing.T) {
	path := t.TempDir() + "/leases.wal"
	wal, err := OpenWAL(path)
	if err != nil {
		t.Fatal(err)
	}
	defer wal.Close()
	value := Lease{ID: "l2", IPAddress: "192.0.2.11", ScopeID: "scope-1", Status: LeaseStatusActive}
	if err := wal.Append(WALEvent{Version: 1, Seq: 1, Op: WALEventUpsert, Lease: &value}); err != nil {
		t.Fatal(err)
	}
	if err := wal.Sync(); err != nil {
		t.Fatal(err)
	}
	target := NewMemoryIndex()
	last, err := wal.RecoverMemoryIndex(target, nil, nil, 0)
	if err != nil || last != 1 {
		t.Fatalf("recovery = %d, %v", last, err)
	}
	if got, ok := target.Get("l2"); !ok || got.IPAddress != value.IPAddress {
		t.Fatalf("recovered lease = %+v, %v", got, ok)
	}
}

func TestRecoverMemoryIndexAppliesRemoval(t *testing.T) {
	baseline := []Lease{{ID: "l1", IPAddress: "192.0.2.10", Status: LeaseStatusActive}}
	var wal bytes.Buffer
	if err := AppendWALEvent(&wal, WALEvent{Version: 1, Seq: 1, Op: WALEventRemove, LeaseID: "l1"}); err != nil {
		t.Fatal(err)
	}
	target := NewMemoryIndex()
	if _, err := RecoverMemoryIndex(target, baseline, nil, bytes.NewReader(wal.Bytes()), 0); err != nil {
		t.Fatal(err)
	}
	if _, ok := target.Get("l1"); ok {
		t.Fatal("removed lease survived recovery")
	}
}
