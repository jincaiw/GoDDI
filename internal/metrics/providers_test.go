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
		backupStatsFn = nil
		secondaryZoneStatsFn = nil
	})
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

func TestSampleProvidersPublishesDHCPPoolUtilisation(t *testing.T) {
	withProviders(t)
	dhcpScopeStatsFn = func() []DHCPScopeSample {
		return []DHCPScopeSample{
			{Scope: "scope-a", Held: 3, Ratio: 0.3},
			{Scope: "scope-b", Held: 9, Ratio: 0.9},
		}
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
