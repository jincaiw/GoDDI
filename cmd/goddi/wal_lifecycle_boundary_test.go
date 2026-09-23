package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// This is a migration boundary guard, not proof of production WAL assembly.
// The staged WAL/applier helpers are intentionally not wired into the default
// process until their transaction, readiness, and shutdown contracts are closed.
func TestDefaultProcessLeavesWALProjectionLifecycleExplicitlyUnassembled(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	mainPath := filepath.Join(filepath.Dir(file), "main.go")
	data, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("read %s: %v", mainPath, err)
	}
	source := string(data)

	for _, required := range []string{
		"dhcpSrv.Start(context.Background())",
		"dhcpSrv.Shutdown(ctx)",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("default process lost existing DHCP lifecycle call %q", required)
		}
	}
	for _, forbidden := range []string{
		"lease.NewAsyncApplier(",
		"lease.RecoverSQLiteProjection(",
		"dhcpSrv.SetStartupGate(",
		"lease.OpenWAL(",
		"wal.Sync()",
		"applier.Wait(",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("default process unexpectedly assembled staged WAL lifecycle hook %q", forbidden)
		}
	}

	shutdownMarkers := []string{
		"shutting down HTTP server",
		"shutting down DNS server",
		"shutting down DHCP server",
		"flushing DHCP event logs",
	}
	last := -1
	for _, marker := range shutdownMarkers {
		pos := strings.Index(source, marker)
		if pos < 0 {
			t.Fatalf("shutdown sequence lost marker %q", marker)
		}
		if pos <= last {
			t.Fatalf("shutdown marker %q is out of order", marker)
		}
		last = pos
	}
}

func TestDefaultProcessKeepsLegacyDNSConsumerUntilFactsMigrationIsComplete(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	mainPath := filepath.Join(filepath.Dir(file), "main.go")
	data, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("read %s: %v", mainPath, err)
	}
	source := string(data)

	if !strings.Contains(source, "dhcpinternal.NewDNSConsumer(dnsStore.DB, link)") {
		t.Fatal("default process no longer assembles the legacy DHCP-to-DNS consumer")
	}
	for _, forbidden := range []string{
		"facts.NewWatermarkStore(",
		"NewFactsDNSConsumer(",
		"FactsDNSConsumer",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("default process unexpectedly assembled unified facts DNS migration hook %q", forbidden)
		}
	}
}

func TestDefaultProcessLeavesFactsConsumerLifecycleOptIn(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	mainPath := filepath.Join(filepath.Dir(file), "main.go")
	data, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("read %s: %v", mainPath, err)
	}
	source := string(data)

	for _, forbidden := range []string{
		"ipam.NewFactsPipeline(",
		"FactsPipelineOptions{Enabled: true",
		"factsPipeline.Start(",
		"factsPipeline.Wake(",
		"factsPipeline.Stop(",
		"FactsPipelineLifecycle",
		"ipam.NewFactsConsumer(",
		"ipam.NewFactsConsumerWithOptions(",
		"factsConsumer.Start(",
		"factsConsumer.Wake(",
		"factsConsumer.Stop(",
		"FactsConsumerLifecycle",
		"facts.ObservationOutbox",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("default process unexpectedly assembled opt-in facts consumer lifecycle hook %q", forbidden)
		}
	}

	for _, required := range []string{
		"ipam.NewLinkage(db.DB)",
		"dhcpSrv.SetLeaseObserver(ipamLinkage)",
		"facts.NewSequenceAllocator(dhcpStore.DB)",
		"facts.NewObservationOutbox(dhcpStore.DB)",
		"dhcpSrv.SetLeaseFactsMutation(",
		"ipamLinkage.Reconcile(ipamReconcileLimit)",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("default process lost existing legacy IPAM path %q", required)
		}
	}
}
