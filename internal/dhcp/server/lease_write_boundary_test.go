package server

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// The only non-test files allowed to contain a direct write to dhcp_leases.
// Each has a documented role in docs/audit/N5-BYPASS-MATRIX.md; anything else
// is an unaudited bypass of the lease authority/facts seam.
var allowedLeaseWriteFiles = []string{
	"internal/dhcp/lease/lease.go",              // default SQLite authority
	"internal/dhcp/lease/facts_mutation.go",     // facts mutation writer (authority seam)
	"internal/dhcp/lease/facts_expiry_batch.go", // facts expiry sweep
	"internal/dhcp/lease/wal_projection.go",     // opt-in WAL projection
	"internal/dhcp/ha/mirror.go",                // standby replica apply
	"internal/dhcp/scope/scope.go",              // controlled cascade (non-live history only)
	"internal/dataplane/sync.go",                // control<->lease projection copy
	"internal/dataplane/transfer.go",            // control<->lease projection copy
}

func TestLeaseWritesAreConfinedToTheDocumentedOwners(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Join(filepath.Dir(file), "..", "..", "..")

	patterns := []string{"INSERT INTO dhcp_leases", "UPDATE dhcp_leases", "DELETE FROM dhcp_leases"}
	found := map[string]bool{}
	for _, base := range []string{filepath.Join(root, "internal"), filepath.Join(root, "cmd")} {
		err := filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			text := string(data)
			for _, p := range patterns {
				if strings.Contains(text, p) {
					rel, err := filepath.Rel(root, path)
					if err != nil {
						return err
					}
					found[filepath.ToSlash(rel)] = true
					break
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", base, err)
		}
	}

	allowed := map[string]bool{}
	for _, f := range allowedLeaseWriteFiles {
		allowed[f] = true
	}
	var unexpected []string
	for f := range found {
		if !allowed[f] {
			unexpected = append(unexpected, f)
		}
	}
	sort.Strings(unexpected)
	if len(unexpected) > 0 {
		t.Fatalf("undocumented direct dhcp_leases writes in %v; add them to docs/audit/N5-BYPASS-MATRIX.md with owner/transaction/facts semantics and to allowedLeaseWriteFiles", unexpected)
	}
	var missing []string
	for _, f := range allowedLeaseWriteFiles {
		if !found[f] {
			missing = append(missing, f)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("allowlisted files no longer write dhcp_leases (%v); update allowedLeaseWriteFiles and the matrix doc", missing)
	}
}

func TestDocumentedBypassMatrixStaysAccurate(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Join(filepath.Dir(file), "..", "..", "..")
	docPath := filepath.Join(root, "docs", "audit", "N5-BYPASS-MATRIX.md")
	data, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("read bypass matrix: %v", err)
	}
	doc := string(data)
	for _, f := range allowedLeaseWriteFiles {
		if !strings.Contains(doc, filepath.Base(f)) && !strings.Contains(doc, f) {
			t.Fatalf("bypass matrix does not mention %s", f)
		}
	}
	// The controlled scope-delete bypass must stay described as non-live-history
	// only, and the durable-ACK gap must stay declared rather than implied fixed.
	for _, needle := range []string{"受控旁路", "durableGate=nil"} {
		if !strings.Contains(doc, needle) {
			t.Fatalf("bypass matrix lost required statement %q", needle)
		}
	}
}
