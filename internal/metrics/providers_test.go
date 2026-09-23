package metrics

// The registry guard proves every metric is named somewhere in the tree. These
// tests prove the sampling publishes: that the ticker calls the provider, that
// what it returns reaches the exposition, and that a value the provider
// describes as absent stays absent instead of becoming a zero.
//
// They read the Prometheus text exposition rather than the collector, because
// that text is what an alert rule is written against. Asserting on the exported
// name and label set catches a metric that publishes under a name no rule
// refers to -- which is a silent failure in exactly the same way as a metric
// nobody writes.

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// expose registers the given collectors on a private registry and returns the
// exposition text. A private registry is used because the package's collectors
// are registered globally only by InitMetrics, which starts a ticker.
func expose(t *testing.T, collectors ...prometheus.Collector) string {
	t.Helper()
	reg := prometheus.NewRegistry()
	for _, c := range collectors {
		reg.MustRegister(c)
	}

	rec := httptest.NewRecorder()
	promhttp.HandlerFor(reg, promhttp.HandlerOpts{}).
		ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if rec.Code != 200 {
		t.Fatalf("exposition returned %d", rec.Code)
	}
	return rec.Body.String()
}

// withProviders restores the package-level provider slots after the test, so a
// provider registered here cannot decide what a later test samples.
func withProviders(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		dhcpScopeStatsFn = nil
		dhcpScopeLabels = make(map[string]struct{})
		backupStatsFn = nil
		secondaryZoneStatsFn = nil
		dataPlaneStatsFn = nil
		factsConsumerStatsFn = nil
		factsProducerStatsFn = nil
		dhcpScopeLabels = make(map[string]struct{})
		DHCPLeasesActive.Reset()
		DHCPScopeUsageRatio.Reset()
		DHCPScopeUtilizationScrapeError.Set(0)
		DHCPScopeUtilizationLastSuccessTimestampSeconds.Reset()
	})
}

func TestSampleProvidersPublishesFactsProducerStatus(t *testing.T) {
	withProviders(t)
	factsProducerStatsFn = func() []FactsProducerSample {
		return []FactsProducerSample{{
			Domain: "ipam", Pending: 3, Failed: 1, HeadSequence: 12,
			FirstOutstandingSequence: 9,
		}}
	}
	sampleProviders()
	text := expose(t, FactsProducerPending, FactsProducerFailed, FactsProducerHeadSequence, FactsProducerFirstOutstandingSequence)
	for _, want := range []string{
		`goddi_facts_producer_pending{domain="ipam"} 3`,
		`goddi_facts_producer_failed{domain="ipam"} 1`,
		`goddi_facts_producer_head_sequence{domain="ipam"} 12`,
		`goddi_facts_producer_first_outstanding_sequence{domain="ipam"} 9`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("exposition does not contain %q:\n%s", want, text)
		}
	}
}

// samples returns only the sample lines of one metric: the ones that carry a
// value. The HELP and TYPE lines name the metric whether or not anything was
// published, so searching the whole text for the name cannot tell "published"
// from "registered and empty".
func samples(text, metric string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, metric+"{") || strings.HasPrefix(line, metric+" ") {
			out = append(out, line)
		}
	}
	return out
}

func TestSampleProvidersPublishesFactsConsumerStatus(t *testing.T) {
	withProviders(t)
	factsConsumerStatsFn = func() []FactsConsumerSample {
		return []FactsConsumerSample{{
			Domain: "ipam", Pending: 2, Failed: 1, Applied: 7, Lag: 3,
			Gap: true, Readiness: "failing",
		}}
	}

	sampleProviders()
	text := expose(t, FactsConsumerPending, FactsConsumerFailed, FactsConsumerAppliedSequence, FactsConsumerLag, FactsConsumerGap, FactsConsumerReadiness)
	for _, want := range []string{
		`goddi_facts_consumer_pending{domain="ipam"} 2`,
		`goddi_facts_consumer_failed{domain="ipam"} 1`,
		`goddi_facts_consumer_applied_sequence{domain="ipam"} 7`,
		`goddi_facts_consumer_lag{domain="ipam"} 3`,
		`goddi_facts_consumer_gap{domain="ipam"} 1`,
		`goddi_facts_consumer_readiness{domain="ipam"} 0`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("exposition does not contain %q:\n%s", want, text)
		}
	}
}

func TestSampleProvidersPublishesFactsConsumerStatusIgnoresEmptyDomain(t *testing.T) {
	withProviders(t)
	FactsConsumerPending.DeleteLabelValues("ipam")
	factsConsumerStatsFn = func() []FactsConsumerSample {
		return []FactsConsumerSample{{Pending: 9, Readiness: "ok"}}
	}
	sampleProviders()
	if got := samples(expose(t, FactsConsumerPending), "goddi_facts_consumer_pending"); len(got) != 0 {
		t.Fatalf("empty domain published facts metric: %v", got)
	}
}

func TestSampleProvidersPublishesStartupRecoveryState(t *testing.T) {
	withProviders(t)
	dataPlaneStatsFn = func() []DataPlaneSample {
		return []DataPlaneSample{{
			Plane:             "lease",
			Level:             "failing",
			StartupConfigured: true,
			StartupReady:      false,
			StartupState:      "failed",
			StartupLastSeq:    41,
		}}
	}

	sampleProviders()
	text := expose(t, DataPlaneStartupConfigured, DataPlaneStartupReady, DataPlaneStartupLastSequence, DataPlaneStartupState)
	for _, want := range []string{
		`goddi_dataplane_startup_recovery_configured{plane="lease"} 1`,
		`goddi_dataplane_startup_recovery_ready{plane="lease"} 0`,
		`goddi_dataplane_startup_recovery_last_seq{plane="lease"} 41`,
		`goddi_dataplane_startup_recovery_state{plane="lease",state="failed"} 1`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("exposition does not contain %q:\\n%s", want, text)
		}
	}
	if strings.Contains(text, "secret") || strings.Contains(text, "permission denied") {
		t.Errorf("startup error text leaked into metrics: %s", text)
	}
}

func TestSampleProvidersPublishesDHCPPoolUtilisation(t *testing.T) {
	withProviders(t)
	dhcpScopeStatsFn = func() ([]DHCPScopeSample, bool) {
		return []DHCPScopeSample{
			{Scope: "scope-a", Held: 3, Ratio: 0.3},
			{Scope: "scope-b", Held: 9, Ratio: 0.9},
		}, true
	}

	sampleProviders()

	text := expose(t, DHCPLeasesActive, DHCPScopeUsageRatio)
	for _, want := range []string{
		`goddi_dhcp_leases_active{scope="scope-a"} 3`,
		`goddi_dhcp_scope_usage_ratio{scope="scope-b"} 0.9`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("exposition does not contain %q:\n%s", want, text)
		}
	}
}

func TestSampleProvidersRemovesDisappearedDHCPScope(t *testing.T) {
	withProviders(t)
	current := []DHCPScopeSample{{Scope: "scope-a", Held: 3, Ratio: 0.3}, {Scope: "scope-b", Held: 9, Ratio: 0.9}}
	dhcpScopeStatsFn = func() ([]DHCPScopeSample, bool) { return current, true }

	sampleProviders()
	current = []DHCPScopeSample{{Scope: "scope-a", Held: 1, Ratio: 0.1}}
	sampleProviders()

	text := expose(t, DHCPLeasesActive, DHCPScopeUsageRatio)
	if strings.Contains(text, `scope="scope-b"`) {
		t.Fatalf("disappeared DHCP scope left stale series:\n%s", text)
	}
	if !strings.Contains(text, `goddi_dhcp_leases_active{scope="scope-a"} 1`) {
		t.Fatalf("remaining DHCP scope was not updated:\n%s", text)
	}
}

func TestSampleProvidersKeepsDHCPScopeSeriesAfterFailedSample(t *testing.T) {
	withProviders(t)
	ok := true
	dhcpScopeStatsFn = func() ([]DHCPScopeSample, bool) {
		if ok {
			return []DHCPScopeSample{{Scope: "scope-a", Held: 3, Ratio: 0.3}}, true
		}
		return nil, false
	}

	sampleProviders()
	ok = false
	sampleProviders()

	text := expose(t, DHCPLeasesActive, DHCPScopeUsageRatio)
	if !strings.Contains(text, `goddi_dhcp_leases_active{scope="scope-a"} 3`) {
		t.Fatalf("failed DHCP sample removed the last good series:\n%s", text)
	}
}

func TestSampleProvidersPublishesDHCPUtilizationHealthAfterFailure(t *testing.T) {
	withProviders(t)
	ok := false
	dhcpScopeStatsFn = func() ([]DHCPScopeSample, bool) {
		if !ok {
			return nil, false
		}
		return []DHCPScopeSample{{Scope: "scope-a", Held: 2, Ratio: 0.2}}, true
	}
	secondaryZoneStatsFn = func() []SecondaryZoneSample {
		return []SecondaryZoneSample{{Zone: "zone.example.test", Failures: 2}}
	}

	sampleProviders()
	text := expose(t, DHCPScopeUtilizationScrapeError, DHCPScopeUtilizationLastSuccessTimestampSeconds, SecondaryZoneSyncFailures)
	if !strings.Contains(text, "goddi_dhcp_scope_utilization_scrape_error 1") {
		t.Fatalf("failed DHCP sample did not publish scrape error:\n%s", text)
	}
	if got := samples(text, "goddi_dhcp_scope_utilization_last_success_timestamp_seconds"); len(got) != 0 {
		t.Fatalf("first failed DHCP sample published a success timestamp: %v", got)
	}
	if !strings.Contains(text, `goddi_dns_secondary_zone_sync_failures{zone="zone.example.test"} 2`) {
		t.Fatalf("DHCP provider failure blocked another provider:\n%s", text)
	}
}

func TestSampleProvidersDHCPUtilizationHealthRecoversAndKeepsLastGoodValues(t *testing.T) {
	withProviders(t)
	ok := true
	dhcpScopeStatsFn = func() ([]DHCPScopeSample, bool) {
		if ok {
			return []DHCPScopeSample{{Scope: "scope-a", Held: 3, Ratio: 0.3}}, true
		}
		return nil, false
	}

	sampleProviders()
	first := expose(t, DHCPScopeUtilizationLastSuccessTimestampSeconds)
	firstTimestamp := samples(first, "goddi_dhcp_scope_utilization_last_success_timestamp_seconds")
	if len(firstTimestamp) != 1 {
		t.Fatalf("successful DHCP sample did not publish one timestamp: %v", firstTimestamp)
	}

	ok = false
	sampleProviders()
	failed := expose(t, DHCPLeasesActive, DHCPScopeUtilizationScrapeError, DHCPScopeUtilizationLastSuccessTimestampSeconds)
	if !strings.Contains(failed, `goddi_dhcp_leases_active{scope="scope-a"} 3`) {
		t.Fatalf("failed DHCP sample erased the last good utilization value:\n%s", failed)
	}
	if !strings.Contains(failed, "goddi_dhcp_scope_utilization_scrape_error 1") {
		t.Fatalf("failed DHCP sample did not set scrape error:\n%s", failed)
	}

	ok = true
	sampleProviders()
	recovered := expose(t, DHCPScopeUtilizationScrapeError, DHCPScopeUtilizationLastSuccessTimestampSeconds)
	if !strings.Contains(recovered, "goddi_dhcp_scope_utilization_scrape_error 0") {
		t.Fatalf("successful recovery did not clear scrape error:\n%s", recovered)
	}
	if got := samples(recovered, "goddi_dhcp_scope_utilization_last_success_timestamp_seconds"); len(got) != 1 {
		t.Fatalf("successful recovery lost success timestamp: %v", got)
	}
}

func TestSampleProvidersRemovesAllDHCPScopeSeriesAfterSuccessfulEmptySample(t *testing.T) {
	withProviders(t)
	current := []DHCPScopeSample{{Scope: "scope-a", Held: 3, Ratio: 0.3}}
	dhcpScopeStatsFn = func() ([]DHCPScopeSample, bool) { return current, true }

	sampleProviders()
	current = nil
	sampleProviders()

	text := expose(t, DHCPLeasesActive, DHCPScopeUsageRatio)
	if got := samples(text, "goddi_dhcp_leases_active"); len(got) != 0 {
		t.Fatalf("successful empty DHCP sample left series: %v", got)
	}
}

func TestSampleProvidersSkipsEmptyDHCPScopeLabel(t *testing.T) {
	withProviders(t)
	dhcpScopeStatsFn = func() ([]DHCPScopeSample, bool) {
		return []DHCPScopeSample{{Scope: "", Held: 99, Ratio: 0.99}}, true
	}

	sampleProviders()
	text := expose(t, DHCPLeasesActive, DHCPScopeUsageRatio)
	if got := samples(text, "goddi_dhcp_leases_active"); len(got) != 0 {
		t.Fatalf("empty DHCP scope label published a series: %v", got)
	}
}

func TestSampleProvidersPublishesSecondaryZoneHealth(t *testing.T) {
	withProviders(t)
	synced := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	secondaryZoneStatsFn = func() []SecondaryZoneSample {
		return []SecondaryZoneSample{
			{Zone: "ok.example.test", HasSynced: true, LastSync: synced},
			{Zone: "failing.example.test", Failures: 4, HasSynced: true, LastSync: synced.Add(-time.Hour)},
			// Never transferred: the failure count is real and must be
			// published, the timestamp must not become the epoch.
			{Zone: "never.example.test"},
		}
	}

	sampleProviders()

	text := expose(t, SecondaryZoneSyncFailures, SecondaryZoneLastSyncTimestampSeconds)
	if !strings.Contains(text, `goddi_dns_secondary_zone_sync_failures{zone="failing.example.test"} 4`) {
		t.Errorf("the failure count is not published:\n%s", text)
	}

	// %g because that is how the exposition writes a float: a Unix timestamp
	// comes out as 1.7890416e+09, not as a run of digits.
	lastSync := fmt.Sprintf(`goddi_dns_secondary_zone_last_sync_timestamp_seconds{zone="ok.example.test"} %g`,
		float64(synced.Unix()))
	if !strings.Contains(text, lastSync) {
		t.Errorf("exposition does not contain %q:\n%s", lastSync, text)
	}

	// Absence, asserted as absence and only for this metric. A never-synced
	// zone carrying a zero timestamp would read as "last synced in 1970" -- a
	// stale-zone alarm about a zone that has simply never managed a transfer,
	// which needs a different alert.
	for _, line := range samples(text, "goddi_dns_secondary_zone_last_sync_timestamp_seconds") {
		if strings.Contains(line, `zone="never.example.test"`) {
			t.Errorf("a zone that has never transferred published a timestamp: %s", line)
		}
	}
}

// TestSampleProvidersLeavesAnAgeUnknownUntilThereIsOne covers the backup gauge.
// The alert is written as "older than a day", and a zero timestamp matches that
// expression -- which is exactly what a plain gauge would publish from the
// moment it was registered. The type must have no series at all until one of
// its backups succeeds.
func TestSampleProvidersLeavesAnAgeUnknownUntilThereIsOne(t *testing.T) {
	withProviders(t)
	backupStatsFn = func() []BackupStatsSample { return nil }

	sampleProviders()

	if got := samples(expose(t, BackupLastSuccessTimestampSeconds), "goddi_backup_last_success_timestamp_seconds"); len(got) != 0 {
		t.Errorf("the backup age gauge published with no successful backup: %v", got)
	}

	dnsAt := time.Date(2026, 9, 10, 6, 0, 0, 0, time.UTC)
	backupStatsFn = func() []BackupStatsSample {
		return []BackupStatsSample{{Type: "dns", LastSuccess: dnsAt}}
	}

	sampleProviders()

	text := expose(t, BackupLastSuccessTimestampSeconds)
	want := fmt.Sprintf(`goddi_backup_last_success_timestamp_seconds{type="dns"} %g`, float64(dnsAt.Unix()))
	if !strings.Contains(text, want) {
		t.Errorf("exposition does not contain %q:\n%s", want, text)
	}
	// The type that has not been backed up stays absent while another type is
	// present, which is the distinction the whole vector exists for.
	if strings.Contains(text, `type="full"`) {
		t.Errorf("a type with no successful backup was published alongside one that had:\n%s", text)
	}
}
