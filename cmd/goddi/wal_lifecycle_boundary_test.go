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

func TestDefaultProcessUsesFactsOnlyDNSConsumer(t *testing.T) {
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

	// The legacy DNS outbox queue was retired in round 286. The unified facts
	// chain is now the only DNS producer: the facts consumer must still be
	// assembled, the DNS-side facts relay must still feed it, and no legacy
	// queue consumer or outbox wake may come back.
	if !strings.Contains(source, "dhcpinternal.NewDNSFactsConsumer(dnsLink, dnsInbox)") {
		t.Fatal("default process does not assemble the unified facts DNS consumer")
	}
	if !strings.Contains(source, "facts.NewRelay(dhcpFactsProducerOutbox, dnsInbox, dnsStore.DB)") {
		t.Fatal("default process does not assemble the DNS-side facts relay")
	}
	for _, forbidden := range []string{
		"NewDNSConsumer",
		"WithDNSSink",
		"SetOutboxWake",
		"LegacyDualWrite",
		"DomainDDNS",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("default process still assembles retired legacy DNS path %q", forbidden)
		}
	}
}

func TestDefaultControlProcessRunsFactsConsumerLifecycle(t *testing.T) {
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

	// The assembly evolved (round 273): the data-plane relay copies the lease
	// plane's producer outbox into the control inbox, and HA primaries run the
	// full pipeline (produce + relay + consume) instead of only the writer.
	for _, required := range []string{
		"ipam.NewLinkage(db.DB)",
		"var dhcpFactsProducerOutbox *facts.ObservationOutbox",
		"if controlPlane {",
		"var relay *facts.Relay",
		"facts.NewObservationOutbox(db.DB)",
		"facts.NewRelay(dhcpFactsProducerOutbox, controlInbox, db.DB)",
		"Relay:   relay,",
		"ipam.NewFactsPipeline(ipamLinkage, controlInbox, ipam.FactsPipelineOptions{",
		"ipamFactsPipeline.Start(backgroundCtx)",
		"ipamFactsPipeline.Stop(ctx)",
		"ipamFactsPipeline.Status(ctx)",
		"facts.NewSequenceAllocator(dhcpStore.DB)",
		"facts.NewObservationOutbox(dhcpStore.DB)",
		"if !haEnabled || haRole == config.HARolePrimary {",
		"dhcpSrv.SetLeaseFactsMutation(",
		"dhcpSrv.SetLeaseObserver(ipamLinkage)",
		"ipamLinkage.Reconcile(ipamReconcileLimit)",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("default process lost existing legacy IPAM path %q", required)
		}
	}
}

// TestDefaultProcessDoesNotAssembleWALPipeline keeps the event-sourced write
// path opt-in: the shipped process must keep the SQLite-backed lease manager
// and never construct the WAL pipeline, whose startup recovery and shutdown
// ordering are exercised by tests but not yet wired into production.
func TestDefaultProcessDoesNotAssembleWALPipeline(t *testing.T) {
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
	for _, forbidden := range []string{"OpenWALPipeline", "NewWALPipeline", "lease.NewWALLeaseStore"} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("default process unexpectedly assembles the opt-in WAL path %q", forbidden)
		}
	}
}

// TestDefaultFactsChainAssemblyMatchesGrayReview keeps the reviewed assembly
// (docs/audit/N6-GRAY-ASSEMBLY-REVIEW.md) tied to main.go: the default process
// must assemble the unified facts chain (producer seam, relay, IPAM pipeline,
// DNS facts consumer) and must not silently wire the opt-in WAL lease store or
// durable ACK gate.
func TestDefaultFactsChainAssemblyMatchesGrayReview(t *testing.T) {
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
		"ipam.NewFactsPipeline(",
		"dhcpinternal.NewDNSFactsConsumer(",
		"facts.NewRelay(dhcpFactsProducerOutbox",
		"ipamFactsPipeline.Start(",
		"ipamFactsPipeline.Stop(",
		"SetLeaseFactsMutation(",
		"SetReleaseFactsMutation(",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("default process lost facts-chain assembly marker %q", required)
		}
	}
	for _, forbidden := range []string{"OpenWALPipeline", "NewWALLeaseStore", "SetDurableLeaseGate("} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("default process unexpectedly wires the opt-in WAL path %q", forbidden)
		}
	}
}
