package ipamstore

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/jasonwa/goddi/internal/sqlitepolicy"
)

// This is a schema-level test, not an end-to-end allocation test. Independent
// physical SQLite connections compete for the same scoped address; one row
// can commit. The business API, idempotency receipt and event log are not yet
// connected to this schema.
func TestIndependentConnectionsCannotClaimSameAddress(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ipam.db")
	db := sqlitepolicy.OpenWriter(path)
	db.SetMaxOpenConns(2)
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO ipam_tenants(id,slug,display_name) VALUES ('t','tenant','Tenant')`,
		`INSERT INTO ipam_spaces(tenant_id,id,slug,display_name) VALUES ('t','s','space','Space')`,
		`INSERT INTO ipam_prefixes(tenant_id,space_id,id,family,prefix_len,first_ip,last_ip,kind,usage) VALUES ('t','s','net',4,24,x'000000000000000000000000C0000200',x'000000000000000000000000C00002FF','SUBNET','STANDARD')`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	// Reserving both connections together forces database/sql to open two
	// physical handles rather than serialising the test through one handle.
	first, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i, conn := range []*sql.Conn{first, second} {
		wg.Add(1)
		go func(i int, conn *sql.Conn) {
			defer wg.Done()
			<-start
			_, err := conn.ExecContext(ctx, `INSERT INTO ipam_occupancies(tenant_id,space_id,id,subnet_id,family,address,kind,owner_ref,created_at_ms) VALUES ('t','s',?,'net',4,x'000000000000000000000000C000020A','STATIC',?,1)`, fmt.Sprintf("id-%d", i), fmt.Sprintf("owner-%d", i))
			errs <- err
		}(i, conn)
	}
	close(start)
	wg.Wait()
	close(errs)
	wins, conflicts := 0, 0
	for err := range errs {
		switch {
		case err == nil:
			wins++
		case strings.Contains(err.Error(), "IPAM_ADDRESS_OCCUPIED"):
			conflicts++
		default:
			t.Fatalf("unexpected write failure: %v", err)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("expected one winner and one conflict, got %d and %d", wins, conflicts)
	}
	var count int
	if err := first.QueryRowContext(ctx, `SELECT count(*) FROM ipam_occupancies WHERE tenant_id='t' AND space_id='s'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("expected one persisted occupancy, got %d (%v)", count, err)
	}
}
