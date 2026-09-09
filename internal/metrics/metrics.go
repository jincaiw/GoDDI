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

	UpstreamQueriesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "goddi_dns_upstream_queries_total",
		Help: "DNS upstream attempts by stable forwarder ID and outcome.",
	}, []string{"upstream_id", "outcome"})

	UpstreamDurationSeconds = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "goddi_dns_upstream_duration_seconds",
		Help:    "DNS upstream attempt duration in seconds by stable forwarder ID.",
		Buckets: prometheus.DefBuckets,
	}, []string{"upstream_id"})

	UpstreamHealthy = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "goddi_dns_upstream_healthy",
		Help: "Current upstream health state (1 healthy, 0 unhealthy).",
	}, []string{"upstream_id"})

	UpstreamConsecutiveFailures = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "goddi_dns_upstream_consecutive_failures",
		Help: "Current consecutive upstream failure count.",
	}, []string{"upstream_id"})

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

	CacheEntries = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "goddi_dns_cache_entries",
		Help: "Current number of DNS cache entries.",
	})

	CacheMaxEntries = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "goddi_dns_cache_max_entries",
		Help: "Configured DNS cache capacity.",
	})

	CacheSizeBytes = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "goddi_dns_cache_size_bytes",
		Help: "Approximate memory used by cached DNS responses.",
	})

	CacheHitsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "goddi_dns_cache_hits_total",
		Help: "Total number of DNS cache hits.",
	})

	CacheMissesTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "goddi_dns_cache_misses_total",
		Help: "Total number of DNS cache misses.",
	})

	QueryLogDroppedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "goddi_query_log_dropped_total",
		Help: "Total number of query-log entries dropped because the log channel was full.",
	})

	QueryLogWrittenTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "goddi_query_log_written_total",
		Help: "Total number of query-log entries durably committed to SQLite.",
	})

	QueryLogWriteFailuresTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "goddi_query_log_write_failures_total",
		Help: "Total query-log writer failures by pipeline stage.",
	}, []string{"stage"})

	QueryLogQueueDepth = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "goddi_query_log_queue_depth",
		Help: "Current number of query-log entries waiting for asynchronous persistence.",
	})

	QueryLogQueueCapacity = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "goddi_query_log_queue_capacity",
		Help: "Configured capacity of the query-log asynchronous queue.",
	})
)

// CacheStatsSample is a point-in-time snapshot of cache statistics
// delivered by the provider registered through RegisterCacheStatsProvider.
type CacheStatsSample struct {
	Entries    int64
	MaxEntries int64
	SizeBytes  int64
	Hits       int64
	Misses     int64
}

// QueryLogStatsSample captures query-log queue pressure and cumulative writer
// outcomes. It lives in metrics to avoid coupling the exporter to DNS types.
type QueryLogStatsSample struct {
	QueueDepth      int
	QueueCapacity   int
	DroppedFull     int64
	Written         int64
	BeginFailures   int64
	PrepareFailures int64
	ExecFailures    int64
	CommitFailures  int64
	CleanupFailures int64
}

var (
	// cacheStatsFn, when non-nil, is sampled by the metrics ticker.
	cacheStatsFn func() CacheStatsSample

	// queryLogStatsFn, when non-nil, is sampled by the metrics ticker.
	queryLogStatsFn func() QueryLogStatsSample

	// last samples used to convert absolute gauges into counter deltas.
	lastCacheHits           int64
	lastCacheMisses         int64
	lastQLogDropped         int64
	lastQLogWritten         int64
	lastQLogBeginFailures   int64
	lastQLogPrepareFailures int64
	lastQLogExecFailures    int64
	lastQLogCommitFailures  int64
	lastQLogCleanupFailures int64
)

// RegisterCacheStatsProvider registers a callback the metrics ticker
// samples every tick to publish cache gauges/counters. Call once after
// the cache is constructed; pass nil to unregister.
func RegisterCacheStatsProvider(fn func() CacheStatsSample) {
	cacheStatsFn = fn
}

// RegisterQueryLogStatsProvider registers a callback sampled by the metrics
// ticker to publish query-log queue pressure and writer outcome metrics.
func RegisterQueryLogStatsProvider(fn func() QueryLogStatsSample) {
	queryLogStatsFn = fn
}

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
			UpstreamQueriesTotal,
			UpstreamDurationSeconds,
			UpstreamHealthy,
			UpstreamConsecutiveFailures,
			DHCPLeasesActive,
			DHCPScopeUsageRatio,
			ClusterNodesTotal,
			APIRequestsTotal,
			APIRequestDurationSeconds,
			DBErrorsTotal,
			BackupJobsTotal,
			CacheEntries,
			CacheMaxEntries,
			CacheSizeBytes,
			CacheHitsTotal,
			CacheMissesTotal,
			QueryLogDroppedTotal,
			QueryLogWrittenTotal,
			QueryLogWriteFailuresTotal,
			QueryLogQueueDepth,
			QueryLogQueueCapacity,
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
					sampleProviders()
				}
			}
		}()
	})
}

// sampleProviders polls the optional cache / query-log providers and
// publishes their values. Counter-style metrics are converted from
// absolute provider values into deltas so restarts of the underlying
// component do not make the counters jump backwards.
func sampleProviders() {
	if fn := cacheStatsFn; fn != nil {
		s := fn()
		CacheEntries.Set(float64(s.Entries))
		CacheMaxEntries.Set(float64(s.MaxEntries))
		CacheSizeBytes.Set(float64(s.SizeBytes))
		if d := s.Hits - lastCacheHits; d > 0 {
			CacheHitsTotal.Add(float64(d))
		}
		if d := s.Misses - lastCacheMisses; d > 0 {
			CacheMissesTotal.Add(float64(d))
		}
		lastCacheHits = s.Hits
		lastCacheMisses = s.Misses
	}
	if fn := queryLogStatsFn; fn != nil {
		s := fn()
		QueryLogQueueDepth.Set(float64(s.QueueDepth))
		QueryLogQueueCapacity.Set(float64(s.QueueCapacity))
		addQueryLogDelta(QueryLogDroppedTotal, s.DroppedFull, &lastQLogDropped)
		addQueryLogDelta(QueryLogWrittenTotal, s.Written, &lastQLogWritten)
		addQueryLogDelta(QueryLogWriteFailuresTotal.WithLabelValues("begin"), s.BeginFailures, &lastQLogBeginFailures)
		addQueryLogDelta(QueryLogWriteFailuresTotal.WithLabelValues("prepare"), s.PrepareFailures, &lastQLogPrepareFailures)
		addQueryLogDelta(QueryLogWriteFailuresTotal.WithLabelValues("exec"), s.ExecFailures, &lastQLogExecFailures)
		addQueryLogDelta(QueryLogWriteFailuresTotal.WithLabelValues("commit"), s.CommitFailures, &lastQLogCommitFailures)
		addQueryLogDelta(QueryLogWriteFailuresTotal.WithLabelValues("cleanup"), s.CleanupFailures, &lastQLogCleanupFailures)
	}
}

func addQueryLogDelta(counter prometheus.Counter, current int64, previous *int64) {
	if d := current - *previous; d > 0 {
		counter.Add(float64(d))
	}
	*previous = current
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

// RecordUpstreamAttempt records one data-plane upstream attempt. IDs are
// stable configuration identifiers, rather than names or addresses, to avoid
// high-cardinality labels and preserve metric identity across address changes.
func RecordUpstreamAttempt(id, outcome string, duration time.Duration, healthy bool, failures int32) {
	if id == "" {
		return
	}
	UpstreamQueriesTotal.WithLabelValues(id, outcome).Inc()
	UpstreamDurationSeconds.WithLabelValues(id).Observe(duration.Seconds())
	if healthy {
		UpstreamHealthy.WithLabelValues(id).Set(1)
	} else {
		UpstreamHealthy.WithLabelValues(id).Set(0)
	}
	UpstreamConsecutiveFailures.WithLabelValues(id).Set(float64(failures))
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
