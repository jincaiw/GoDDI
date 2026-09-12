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

	// Deliberately no goddi_dns_clients_total. It was registered and never
	// written, so it reported a steady zero -- a number that looks like a
	// measurement of "no clients" rather than the absence of one. A metric
	// nobody writes is worse than a missing metric, because a dashboard cannot
	// tell the two apart.
	DNSResponseDurationSeconds = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "goddi_dns_response_duration_seconds",
		Help:    "DNS query response duration in seconds.",
		Buckets: prometheus.DefBuckets,
	})

	DNSInflightQueries = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "goddi_dns_inflight_queries",
		Help: "Current number of distinct shared DNS cache-miss forwards.",
	})

	DNSInflightRejectedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "goddi_dns_inflight_rejected_total",
		Help: "Total distinct DNS cache-miss forwards that bypassed shared work because its capacity was full.",
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

	// Secondary-zone refresh health. A secondary that cannot reach its primary
	// keeps answering from a copy that is quietly going stale, and RFC 1035
	// §6.3 makes it stop answering once SOA EXPIRE passes -- so "how long since
	// the last successful transfer" is the difference between a zone that is a
	// little behind and one that is about to go dark.
	//
	// Both are gauges read from the zone table rather than counters kept in
	// memory: the failure count in the table is what the sweep increments, and
	// a counter that restarted with the process would reset to zero exactly
	// when an operator restarted the service to fix the problem.
	SecondaryZoneSyncFailures = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "goddi_dns_secondary_zone_sync_failures",
		Help: "Consecutive failed refreshes of a secondary zone; zero after a success.",
	}, []string{"zone"})

	SecondaryZoneLastSyncTimestampSeconds = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "goddi_dns_secondary_zone_last_sync_timestamp_seconds",
		Help: "Unix time of the last successful transfer of a secondary zone; absent if it never succeeded.",
	}, []string{"zone"})

	// Deliberately no goddi_cluster_nodes_total. There is no cluster to count:
	// the DHCP pair is a single-writer replica, and an endpoint enumerating its
	// members would be answering a question the deployment cannot answer -- a
	// node that cannot see its peer cannot count the pair. What the pair does
	// report is whether this node currently has a second copy, which is the
	// fact an operator can act on. See DHCPHARedundant.
	//
	// Both of these are keyed by node and are absent, not zero, on a deployment
	// with HA switched off. Zero reads as "the cluster lost its second copy",
	// and a single node that never had one must not raise that alarm.
	DHCPHARedundant = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "goddi_dhcp_ha_redundant",
		Help: "1 when this DHCP node holds a second copy of the leases it promises, 0 when it does not. Absent when HA is disabled.",
	}, []string{"node_id"})

	DHCPHAPromising = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "goddi_dhcp_ha_promising",
		Help: "1 when this DHCP node may acknowledge a binding or a renewal, 0 when it is withholding them. Absent when HA is disabled.",
	}, []string{"node_id"})

	DHCPRequestsInflight = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "goddi_dhcp_requests_inflight",
		Help: "Current number of DHCP requests being processed by workers.",
	})

	DHCPRequestQueueDepth = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "goddi_dhcp_request_queue_depth",
		Help: "Current number of DHCP requests waiting for a worker.",
	})

	DHCPRequestQueueCapacity = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "goddi_dhcp_request_queue_capacity",
		Help: "Configured capacity of the bounded DHCP request queue.",
	})

	DHCPRequestsDroppedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "goddi_dhcp_requests_dropped_total",
		Help: "DHCP requests dropped because the bounded request queue was full.",
	})

	DHCPRequestTimeoutsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "goddi_dhcp_request_timeouts_total",
		Help: "DHCP request processing attempts that exceeded the server's observation threshold.",
	})

	DHCPSQLBusyTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "goddi_dhcp_sql_busy_total",
		Help: "DHCP request failures caused by SQLite busy or locked errors.",
	})

	DHCPRequestDurationSeconds = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "goddi_dhcp_request_duration_seconds",
		Help:    "DHCP request processing duration in seconds.",
		Buckets: prometheus.DefBuckets,
	})

	// Data-plane readiness and footprint. These are the numbers behind
	// /ready; the probe itself serves the level only, and the reasons and
	// figures live here and on the authenticated detail endpoint.
	DataPlaneReady = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "goddi_dataplane_ready",
		Help: "Data-plane readiness: 1 at ok, 0.5 at degraded, 0 at failing.",
	}, []string{"plane"})

	DataPlanePendingChanges = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "goddi_dataplane_pending_changes",
		Help: "Changes a data plane still owes to another plane or the control database.",
	}, []string{"plane", "queue"})

	DataPlaneStoreBytes = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "goddi_dataplane_store_bytes",
		Help: "Size of a data plane's local files, including its write-ahead log.",
	}, []string{"plane"})

	// DataPlaneRefusedChanges is a number apart from the backlog. A pending
	// change is work that is owed; a refused one is work the control database
	// will not take. They are worth alerting on differently: the first is the
	// system being behind, the second is the system being told no.
	//
	// It carries no queue label because it is read out of memory by the
	// ten-second metrics tick, and a per-queue count would mean six store reads
	// per tick -- which is the thing that keeps the sample off the store. The
	// queue a refusal came from is in the log line that recorded it.
	DataPlaneRefusedChanges = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "goddi_dataplane_refused_changes",
		Help: "Queued changes the control database has refused; they stay queued and are retried with a widening delay.",
	}, []string{"plane"})

	DataPlaneHeldLeases = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "goddi_dataplane_held_leases",
		Help: "Leases a data plane currently holds.",
	}, []string{"plane"})

	DataPlaneQuotaUsageRatio = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "goddi_dataplane_quota_usage_ratio",
		Help: "How much of a data-plane quota is in use (1.0 means exactly at the bound).",
	}, []string{"plane", "bound"})

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
		Help: "Total number of database errors explicitly reported by producers.",
	})

	DBOpenConnections = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "goddi_db_open_connections",
		Help: "Current open database connections.",
	})

	DBInUseConnections = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "goddi_db_in_use_connections",
		Help: "Current database connections in use.",
	})

	DBIdleConnections = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "goddi_db_idle_connections",
		Help: "Current idle database connections.",
	})

	DBMaxOpenConnections = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "goddi_db_max_open_connections",
		Help: "Configured maximum open database connections; 0 means unlimited.",
	})

	DBWaitCountTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "goddi_db_wait_count_total",
		Help: "Total waits for a database connection.",
	})

	DBWaitDurationSecondsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "goddi_db_wait_duration_seconds_total",
		Help: "Total seconds spent waiting for a database connection.",
	})

	BackupJobsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "goddi_backup_jobs_total",
		Help: "Total number of backup jobs.",
	}, []string{"status"})

	// BackupLastSuccessTimestampSeconds is the Unix time of the most recent
	// backup of one type that completed.
	//
	// A vector rather than a single gauge, and not for cosmetic reasons: a
	// plain gauge is exported at zero from the moment it is registered, so "no
	// backup has ever succeeded" would be published as the epoch -- which every
	// "older than a day" expression matches. A vector exports only the label
	// values something has set, so a type with no successful backup has no
	// series at all, and that absence is what the never-backed-up alert is
	// written against.
	//
	// Labelled by type because that is the axis backups are scheduled on: daily
	// incrementals and a weekly full are two questions, and one global
	// timestamp answers neither.
	BackupLastSuccessTimestampSeconds = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "goddi_backup_last_success_timestamp_seconds",
		Help: "Unix time of the most recent successful backup of each type; absent for a type that has never succeeded.",
	}, []string{"type"})

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

	QueryLogCleanupDeletedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "goddi_query_log_cleanup_deleted_total",
		Help: "Total expired query-log rows deleted by retention cleanup.",
	})

	QueryLogCleanupRunsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "goddi_query_log_cleanup_runs_total",
		Help: "Total retention-cleanup runs started.",
	})

	QueryLogCleanupTimeoutsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "goddi_query_log_cleanup_timeouts_total",
		Help: "Total retention cleanup runs stopped by cancellation or deadline.",
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
	CleanupDeleted  int64
	CleanupRuns     int64
	CleanupTimeouts int64
}

// DHCPScopeSample is one DHCP scope's address utilisation.
//
// It is sampled rather than pushed because the truth lives in the lease store
// and changes on the request path. A gauge that the DHCP path updated directly
// would be a second author for these series, and a restart would reset it to
// zero while the leases were still there.
type DHCPScopeSample struct {
	// Scope is the scope identifier, used as the label.
	Scope string
	// Held is the number of addresses the pool cannot hand out.
	Held int
	// Ratio is Held over the pool size.
	Ratio float64
}

// DHCPHASample is one DHCP node's redundancy, as the node itself reports it.
//
// It carries two booleans rather than a state name because the state name as a
// label would accumulate: a node that has been paused, then primary, then
// paused again leaves three series behind, two of them frozen at a value nobody
// can interpret. The two questions an alert actually asks are answered by the
// booleans, and the state itself is served by /ready and the logs, where a
// superseded reading is replaced rather than retained.
type DHCPHASample struct {
	// NodeID is the label. It is the node's own identity, so that a series
	// cannot be attributed to the wrong half of a pair.
	NodeID string
	// Redundant is true when this node currently holds a second copy of what
	// it promises.
	Redundant bool
	// Promising is true when this node may acknowledge a binding or a renewal.
	// It is false while the node is withholding, which is a different fact
	// from "this node is broken".
	Promising bool
}

// SecondaryZoneSample is one secondary zone's refresh health.
//
// HasSynced is separate from the timestamp for the same reason as the backup
// gauge: a zone that has never transferred is not a zone that transferred at
// the epoch, and only one of those two is an alarm.
type SecondaryZoneSample struct {
	// Zone is the zone name, used as the label.
	Zone string
	// Failures is the consecutive failure count, zero after a success.
	Failures int
	// HasSynced is false when the zone has never completed a transfer.
	HasSynced bool
	// LastSync is the completion time of the last successful transfer.
	LastSync time.Time
}

// DBStatsSample represents a portable snapshot of database/sql pool pressure.
type DBStatsSample struct {
	OpenConnections    int
	InUse              int
	Idle               int
	MaxOpenConnections int
	WaitCount          int64
	WaitDuration       time.Duration
}

// BackupStatsSample is the age of the newest successful backup of one type.
//
// Only ever produced for a type that has a successful backup. "Never" is
// expressed by the absence of a sample, because a timestamp cannot carry it: a
// zero would be the epoch, and the epoch is a real instant that every staleness
// expression matches.
type BackupStatsSample struct {
	// Type is the backup type: full, dns, dhcp, ipam, security or config.
	Type string
	// LastSuccess is the completion time of the newest successful backup of
	// this type.
	LastSuccess time.Time
}

// DataPlaneSample is one data plane's probe view, as published to
// Prometheus.
//
// It carries the level as well as the numbers because the level is what an
// alert is written against: the figures say how close a bound is, the level
// says whether anything has already crossed it.
type DataPlaneSample struct {
	// Plane names the store: "lease" or "zone".
	Plane string
	// Level is "ok", "degraded" or "failing".
	Level string
	// Pending is the backlog per outbound queue, by queue name.
	Pending map[string]int
	// RefusedRows are queued changes the control database has declined, across
	// every queue. They are still owed and are being retried with a widening
	// delay.
	RefusedRows int
	// StoreBytes is the footprint of the store's files.
	StoreBytes int64
	// HeldLeases is how many leases the store holds. Zero for a store that
	// owns no leases.
	HeldLeases int
	// QuotaRatio is the fraction of each configured bound in use, by bound
	// name. A value at or above 1 means the bound is crossed.
	QuotaRatio map[string]float64
}

var (
	// cacheStatsFn, when non-nil, is sampled by the metrics ticker.
	cacheStatsFn func() CacheStatsSample

	// queryLogStatsFn, when non-nil, is sampled by the metrics ticker.
	queryLogStatsFn func() QueryLogStatsSample

	// dbStatsFn, when non-nil, is sampled by the metrics ticker.
	dbStatsFn func() DBStatsSample

	// backupStatsFn, when non-nil, is sampled by the metrics ticker.
	backupStatsFn func() []BackupStatsSample

	// dhcpScopeStatsFn, when non-nil, is sampled by the metrics ticker.
	dhcpScopeStatsFn func() []DHCPScopeSample

	// dhcpHAStatsFn, when non-nil, is sampled by the metrics ticker. It
	// returns nothing on a node with HA switched off, which is what keeps the
	// redundancy series absent rather than zero there.
	dhcpHAStatsFn func() []DHCPHASample

	// secondaryZoneStatsFn, when non-nil, is sampled by the metrics ticker.
	secondaryZoneStatsFn func() []SecondaryZoneSample

	// dataPlaneStatsFn, when non-nil, is sampled by the metrics ticker.
	dataPlaneStatsFn func() []DataPlaneSample

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
	lastQLogCleanupDeleted  int64
	lastQLogCleanupRuns     int64
	lastQLogCleanupTimeouts int64
	lastDBWaitCount         int64
	lastDBWaitDuration      time.Duration
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

// RegisterDBStatsProvider registers a database/sql pool snapshot callback.
func RegisterDBStatsProvider(fn func() DBStatsSample) {
	dbStatsFn = fn
}

// RegisterBackupStatsProvider registers a callback sampled by the metrics
// ticker to publish the age of the newest successful backup of each type.
func RegisterBackupStatsProvider(fn func() []BackupStatsSample) {
	backupStatsFn = fn
}

// RegisterDHCPScopeStatsProvider registers a callback sampled by the metrics
// ticker to publish DHCP pool utilisation.
func RegisterDHCPScopeStatsProvider(fn func() []DHCPScopeSample) {
	dhcpScopeStatsFn = fn
}

// RegisterDHCPHAStatsProvider registers a callback sampled by the metrics
// ticker to publish this node's DHCP redundancy.
//
// A provider rather than a gauge the HA code writes: the HA package has more
// than one state in which a node is healthy, and a push from each transition
// would be a series that depends on which transitions happened to be observed.
// Sampling the current state has one author and one meaning.
func RegisterDHCPHAStatsProvider(fn func() []DHCPHASample) {
	dhcpHAStatsFn = fn
}

// RegisterSecondaryZoneStatsProvider registers a callback sampled by the
// metrics ticker to publish secondary-zone refresh health.
func RegisterSecondaryZoneStatsProvider(fn func() []SecondaryZoneSample) {
	secondaryZoneStatsFn = fn
}

// RegisterDataPlaneStatsProvider registers a callback sampled by the metrics
// ticker to publish data-plane readiness and footprint metrics.
func RegisterDataPlaneStatsProvider(fn func() []DataPlaneSample) {
	dataPlaneStatsFn = fn
}

// dataPlaneReadyValue maps a readiness level onto the gauge. Degraded is
// deliberately neither 1 nor 0: it is not healthy, and it is not an outage,
// and an alert that has to treat it as either is an alert that is wrong half
// the time.
func dataPlaneReadyValue(level string) float64 {
	switch level {
	case "ok":
		return 1
	case "degraded":
		return 0.5
	default:
		return 0
	}
}

// boolGauge renders a yes/no fact as the 1 or 0 a gauge can carry. It is named
// rather than inlined so that the two places that publish a boolean cannot end
// up disagreeing about which value means which.
func boolGauge(v bool) float64 {
	if v {
		return 1
	}
	return 0
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
			DNSResponseDurationSeconds,
			DNSInflightQueries,
			DNSInflightRejectedTotal,
			UpstreamQueriesTotal,
			UpstreamDurationSeconds,
			UpstreamHealthy,
			UpstreamConsecutiveFailures,
			DHCPLeasesActive,
			DHCPScopeUsageRatio,
			SecondaryZoneSyncFailures,
			SecondaryZoneLastSyncTimestampSeconds,
			APIRequestsTotal,
			APIRequestDurationSeconds,
			DBErrorsTotal,
			DBOpenConnections,
			DBInUseConnections,
			DBIdleConnections,
			DBMaxOpenConnections,
			DBWaitCountTotal,
			DBWaitDurationSecondsTotal,
			BackupJobsTotal,
			BackupLastSuccessTimestampSeconds,
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
			QueryLogCleanupDeletedTotal,
			QueryLogCleanupRunsTotal,
			QueryLogCleanupTimeoutsTotal,
			DataPlaneReady,
			DataPlanePendingChanges,
			DataPlaneStoreBytes,
			DataPlaneHeldLeases,
			DataPlaneQuotaUsageRatio,
			DataPlaneRefusedChanges,
			DHCPHARedundant,
			DHCPHAPromising,
			DHCPRequestsInflight,
			DHCPRequestQueueDepth,
			DHCPRequestQueueCapacity,
			DHCPRequestsDroppedTotal,
			DHCPRequestTimeoutsTotal,
			DHCPSQLBusyTotal,
			DHCPRequestDurationSeconds,
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
		addQueryLogDelta(QueryLogCleanupDeletedTotal, s.CleanupDeleted, &lastQLogCleanupDeleted)
		addQueryLogDelta(QueryLogCleanupRunsTotal, s.CleanupRuns, &lastQLogCleanupRuns)
		addQueryLogDelta(QueryLogCleanupTimeoutsTotal, s.CleanupTimeouts, &lastQLogCleanupTimeouts)
	}
	if fn := dbStatsFn; fn != nil {
		s := fn()
		DBOpenConnections.Set(float64(s.OpenConnections))
		DBInUseConnections.Set(float64(s.InUse))
		DBIdleConnections.Set(float64(s.Idle))
		DBMaxOpenConnections.Set(float64(s.MaxOpenConnections))
		if d := s.WaitCount - lastDBWaitCount; d > 0 {
			DBWaitCountTotal.Add(float64(d))
		}
		if d := s.WaitDuration - lastDBWaitDuration; d > 0 {
			DBWaitDurationSecondsTotal.Add(d.Seconds())
		}
		lastDBWaitCount = s.WaitCount
		lastDBWaitDuration = s.WaitDuration
	}
	if fn := backupStatsFn; fn != nil {
		for _, s := range fn() {
			BackupLastSuccessTimestampSeconds.WithLabelValues(s.Type).Set(float64(s.LastSuccess.Unix()))
		}
	}
	if fn := dhcpScopeStatsFn; fn != nil {
		for _, s := range fn() {
			DHCPLeasesActive.WithLabelValues(s.Scope).Set(float64(s.Held))
			DHCPScopeUsageRatio.WithLabelValues(s.Scope).Set(s.Ratio)
		}
	}
	if fn := dhcpHAStatsFn; fn != nil {
		for _, s := range fn() {
			DHCPHARedundant.WithLabelValues(s.NodeID).Set(boolGauge(s.Redundant))
			DHCPHAPromising.WithLabelValues(s.NodeID).Set(boolGauge(s.Promising))
		}
	}
	if fn := secondaryZoneStatsFn; fn != nil {
		for _, s := range fn() {
			SecondaryZoneSyncFailures.WithLabelValues(s.Zone).Set(float64(s.Failures))
			if s.HasSynced {
				SecondaryZoneLastSyncTimestampSeconds.WithLabelValues(s.Zone).Set(float64(s.LastSync.Unix()))
			}
		}
	}
	if fn := dataPlaneStatsFn; fn != nil {
		for _, s := range fn() {
			DataPlaneReady.WithLabelValues(s.Plane).Set(dataPlaneReadyValue(s.Level))
			DataPlaneStoreBytes.WithLabelValues(s.Plane).Set(float64(s.StoreBytes))
			DataPlaneHeldLeases.WithLabelValues(s.Plane).Set(float64(s.HeldLeases))
			DataPlaneRefusedChanges.WithLabelValues(s.Plane).Set(float64(s.RefusedRows))
			for queue, n := range s.Pending {
				DataPlanePendingChanges.WithLabelValues(s.Plane, queue).Set(float64(n))
			}
			for bound, ratio := range s.QuotaRatio {
				DataPlaneQuotaUsageRatio.WithLabelValues(s.Plane, bound).Set(ratio)
			}
		}
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

// SetDHCPRequestQueue records the current bounded queue depth and capacity.
func SetDHCPRequestQueue(depth, capacity int) {
	DHCPRequestQueueDepth.Set(float64(depth))
	DHCPRequestQueueCapacity.Set(float64(capacity))
}

// AddDHCPRequestInflight records worker concurrency.
func AddDHCPRequestInflight(delta int) {
	DHCPRequestsInflight.Add(float64(delta))
}

// RecordDHCPRequestDropped records an admission drop.
func RecordDHCPRequestDropped() {
	DHCPRequestsDroppedTotal.Inc()
}

// RecordDHCPRequestTimeout records a slow request.
func RecordDHCPRequestTimeout() {
	DHCPRequestTimeoutsTotal.Inc()
}

// RecordDHCPRequestDuration records one request processing duration.
func RecordDHCPRequestDuration(duration time.Duration) {
	DHCPRequestDurationSeconds.Observe(duration.Seconds())
}

// RecordDHCPSQLBusy records a SQLite lock contention failure.
func RecordDHCPSQLBusy() {
	DHCPSQLBusyTotal.Inc()
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

// SetDNSInflightQueries records the current bounded shared-forwarding work.
func SetDNSInflightQueries(count int) {
	DNSInflightQueries.Set(float64(count))
}

// RecordDNSInflightRejected records a shared-forwarding admission bypass.
func RecordDNSInflightRejected() {
	DNSInflightRejectedTotal.Inc()
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
