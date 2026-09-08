package zone

// Benchmarks for the hot authoritative lookup path. They exist to catch
// performance regressions in Store.Lookup (zone map + RRset map access),
// which sits on the critical path of every authoritative answer.

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/database"
	"github.com/miekg/dns"
)

// benchStore builds a migrated sqlite database seeded with one zone and
// recordCount A records, then returns a loaded in-memory Store.
func benchStore(b *testing.B, recordCount int) (*Store, func()) {
	b.Helper()
	dir := b.TempDir()
	dbh, err := database.New(config.DatabaseConfig{
		Driver: "sqlite",
		DSN:    filepath.Join(dir, "bench.db"),
	})
	if err != nil {
		b.Fatal(err)
	}
	if err := dbh.RunMigrations(filepath.Join("..", "..", "..", "migrations")); err != nil {
		dbh.Close()
		b.Fatal(err)
	}

	if _, err := dbh.Exec(`INSERT INTO dns_zones (id,name,type,enabled,default_ttl,soa_mname,soa_rname,serial,refresh,retry,expire,minimum)
		VALUES ('z1','bench.example.com.','primary',1,300,'ns1.bench.example.com.','admin.bench.example.com.',1,3600,600,86400,300)`); err != nil {
		b.Fatal(err)
	}
	tx, err := dbh.Begin()
	if err != nil {
		b.Fatal(err)
	}
	stmt, err := tx.Prepare(`INSERT INTO dns_records (id,zone_id,name,type,value,ttl,enabled) VALUES (?,?,?,?,?,?,1)`)
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < recordCount; i++ {
		name := fmt.Sprintf("host%d.bench.example.com", i)
		if _, err := stmt.Exec(fmt.Sprintf("r%d", i), "z1", name, "A", "192.0.2.1", 300); err != nil {
			b.Fatal(err)
		}
	}
	stmt.Close()
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}

	s := NewStore(dbh.DB)
	return s, func() { dbh.Close() }
}

// BenchmarkStoreLookup_Hit measures lookups that resolve to a real record.
func BenchmarkStoreLookup_Hit(b *testing.B) {
	s, cleanup := benchStore(b, 1000)
	defer cleanup()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		name := fmt.Sprintf("host%d.bench.example.com.", i%1000)
		if _, _, ok := s.Lookup(name, dns.TypeA); !ok {
			b.Fatalf("expected hit for %s", name)
		}
	}
}

// BenchmarkStoreLookup_Miss measures NXDOMAIN lookups (worst case: full
// zone/rrset miss path).
func BenchmarkStoreLookup_Miss(b *testing.B) {
	s, cleanup := benchStore(b, 1000)
	defer cleanup()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Lookup(fmt.Sprintf("missing%d.bench.example.com.", i), dns.TypeA)
	}
}

// BenchmarkStoreLookup_Parallel measures concurrent lookup throughput, which
// is what a production DNS server actually does.
func BenchmarkStoreLookup_Parallel(b *testing.B) {
	s, cleanup := benchStore(b, 1000)
	defer cleanup()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			name := fmt.Sprintf("host%d.bench.example.com.", i%1000)
			s.Lookup(name, dns.TypeA)
			i++
		}
	})
}
