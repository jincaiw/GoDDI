package ipamstore

import (
	"database/sql"
	"testing"

	"github.com/jincaiw/GoDDI/internal/enterprise/eventlog"
	_ "modernc.org/sqlite"
)

// TestNewWiresLogAndDB exercises the Store constructor and accessors that are
// otherwise unused by current packages. New/Log/DB sit on the public repository
// surface, so a regression that swapped the bound handle or dropped the log
// identity would otherwise go unnoticed. No schema or write is required.
func TestNewWiresLogAndDB(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	cluster := eventlog.ID{0x01}
	node := eventlog.ID{0x02}
	scope := TenantSpace{TenantID: "tenant-a", SpaceID: "space-a"}

	s := New(db, cluster, node, scope)
	if s == nil {
		t.Fatal("New returned nil Store")
	}
	if got := s.DB(); got != db {
		t.Fatal("DB() did not return the bound handle")
	}
	l := s.Log()
	if l == nil {
		t.Fatal("Log() returned nil")
	}
	if l.Cluster() != cluster || l.Node() != node {
		t.Fatalf("log identity mismatch: cluster=%v node=%v", l.Cluster(), l.Node())
	}
	if s.tenantSpace != scope {
		t.Fatalf("tenant/space scope mismatch: %+v", s.tenantSpace)
	}
}
