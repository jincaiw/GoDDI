package metrics

import (
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	// startTime tracks when the application started.
	startTime time.Time

	// once ensures metrics are registered only once.
	once sync.Once

	// stopUptime is closed by Shutdown to terminate the background
	// goroutine that updates the uptime gauge.
	stopUptime chan struct{}
)

// Prometheus metrics.
var (
	UptimeSeconds = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "goddi_uptime_seconds",
		Help: "Number of seconds since the server started.",
	})

	DNSQueriesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "goddi_dns_queries_total",
		Help: "Total number of DNS queries processed.",
	}, []string{"type", "response_code"})

	DNSNoErrorTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "goddi_dns_noerror_total",
		Help: "Total number of DNS queries returning NOERROR.",
	})

	DNSServfailTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "goddi_dns_servfail_total",
		Help: "Total number of DNS queries returning SERVFAIL.",
	})

	DNSNXDomainTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "goddi_dns_nxdomain_total",
		Help: "Total number of DNS queries returning NXDOMAIN.",
	})

	DNSRefusedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "goddi_dns_refused_total",
		Help: "Total number of DNS queries returning REFUSED.",
	})

	DNSAuthoritativeTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "goddi_dns_authoritative_total",
		Help: "Total number of authoritative DNS answers.",
	})

	DNSRecursiveTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "goddi_dns_recursive_total",
		Help: "Total number of recursive DNS queries.",
	})

	DNSCachedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "goddi_dns_cached_total",
		Help: "Total number of DNS cache hits.",
	})

	DNSBlockedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "goddi_dns_blocked_total",
		Help: "Total number of DNS queries blocked by filter.",
	})

	DNSDroppedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "goddi_dns_dropped_total",
		Help: "Total number of DNS queries dropped.",
	})

	DNSClientsTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "goddi_dns_clients_total",
		Help: "Current number of unique DNS clients.",
	})

	DNSResponseDurationSeconds = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "goddi_dns_response_duration_seconds",
		Help:    "DNS query response duration in seconds.",
		Buckets: prometheus.DefBuckets,
	})

	DHCPLeasesActive = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "goddi_dhcp_leases_active",
		Help: "Number of active DHCP leases.",
	}, []string{"scope"})

	DHCPScopeUsageRatio = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "goddi_dhcp_scope_usage_ratio",
		Help: "DHCP scope address usage ratio (0.0-1.0).",
	}, []string{"scope"})

	ClusterNodesTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "goddi_cluster_nodes_total",
		Help: "Number of cluster nodes.",
	})

	APIRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "goddi_api_requests_total",
		Help: "Total number of API requests.",
	}, []string{"method", "path", "status"})

	APIRequestDurationSeconds = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "goddi_api_request_duration_seconds",
		Help:    "API request duration in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path"})

	DBErrorsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "goddi_db_errors_total",
		Help: "Total number of database errors.",
	})

	BackupJobsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "goddi_backup_jobs_total",
		Help: "Total number of backup jobs.",
	}, []string{"status"})
)

// InitMetrics registers all Prometheus metrics and starts the uptime gauge updater.
func InitMetrics() {
	once.Do(func() {
		startTime = time.Now()
		stopUptime = make(chan struct{})

		prometheus.MustRegister(
			UptimeSeconds,
			DNSQueriesTotal,
			DNSNoErrorTotal,
			DNSServfailTotal,
			DNSNXDomainTotal,
			DNSRefusedTotal,
			DNSAuthoritativeTotal,
			DNSRecursiveTotal,
			DNSCachedTotal,
			DNSBlockedTotal,
			DNSDroppedTotal,
			DNSClientsTotal,
			DNSResponseDurationSeconds,
			DHCPLeasesActive,
			DHCPScopeUsageRatio,
			ClusterNodesTotal,
			APIRequestsTotal,
			APIRequestDurationSeconds,
			DBErrorsTotal,
			BackupJobsTotal,
		)

		// Start background goroutine to update uptime gauge. The goroutine
		// exits when Shutdown closes stopUptime.
		go func() {
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-stopUptime:
					return
				case <-ticker.C:
					UptimeSeconds.Set(time.Since(startTime).Seconds())
				}
			}
		}()
	})
}

// Shutdown stops the background goroutine that updates the uptime gauge.
// Safe to call multiple times.
func Shutdown() {
	if stopUptime != nil {
		select {
		case <-stopUptime:
			// already closed
		default:
			close(stopUptime)
		}
	}
}

// RecordDNSQuery records a DNS query metric.
func RecordDNSQuery(qtype, rcode string, duration float64, cached, blocked bool) {
	DNSQueriesTotal.WithLabelValues(qtype, rcode).Inc()
	DNSResponseDurationSeconds.Observe(duration)

	switch rcode {
	case "NOERROR":
		DNSNoErrorTotal.Inc()
	case "SERVFAIL":
		DNSServfailTotal.Inc()
	case "NXDOMAIN":
		DNSNXDomainTotal.Inc()
	case "REFUSED":
		DNSRefusedTotal.Inc()
	}

	if cached {
		DNSCachedTotal.Inc()
	}
	if blocked {
		DNSBlockedTotal.Inc()
	}
}

// RecordDHCPLeaseChange records DHCP lease metrics.
func RecordDHCPLeaseChange(scopeID string, active int, usageRatio float64) {
	DHCPLeasesActive.WithLabelValues(scopeID).Set(float64(active))
	DHCPScopeUsageRatio.WithLabelValues(scopeID).Set(usageRatio)
}

// RecordAPIRequest records an API request metric.
func RecordAPIRequest(method, path string, status int, duration float64) {
	statusStr := "200"
	if status >= 200 && status < 600 {
		statusStr = strconv.Itoa(status)
	}
	APIRequestsTotal.WithLabelValues(method, path, statusStr).Inc()
	APIRequestDurationSeconds.WithLabelValues(method, path).Observe(duration)
}

// RecordDBError records a database error.
func RecordDBError() {
	DBErrorsTotal.Inc()
}

// RecordBackupJob records a backup job metric.
func RecordBackupJob(status string) {
	BackupJobsTotal.WithLabelValues(status).Inc()
}

// GetUptime returns the application uptime duration.
func GetUptime() time.Duration {
	return time.Since(startTime)
}

// GetSystemInfo returns runtime system information.
func GetSystemInfo() map[string]interface{} {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return map[string]interface{}{
		"go_version":   runtime.Version(),
		"goroutines":   runtime.NumGoroutine(),
		"memory_alloc": memStats.Alloc,
		"memory_sys":   memStats.Sys,
		"memory_total": memStats.TotalAlloc,
		"gc_count":     memStats.NumGC,
	}
}
