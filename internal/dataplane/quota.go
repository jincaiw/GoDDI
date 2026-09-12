package dataplane

import (
	"fmt"
	"os"
	"strings"
)

// Quota bounds a data plane's local footprint.
//
// A quota here is a signal, never a gate. Refusing to hand out an address
// because a counter crossed a line would take a working DHCP server and turn
// it into an outage, and it would do so at exactly the moment an operator is
// already being paged. So crossing a bound raises the readiness level and
// writes a log line; it never refuses a client, never rejects a write, and
// never stops a push.
//
// The bounds come from ADR 0001's capacity statement (twenty thousand leases
// on a two-node deployment), not from a measurement of what the disk can
// hold. They are set where "something is wrong" is more likely than "this is
// the busy season", because a threshold that fires during normal operation is
// a threshold that gets muted.
type Quota struct {
	// MaxLeases is how many leases the store may hold. Zero disables it.
	MaxLeases int
	// MaxFileBytes is the size the store's files may reach. Zero disables it.
	MaxFileBytes int64
	// MaxBacklog is how many changes a single outbound queue may hold. Zero
	// disables it.
	//
	// A queue that keeps growing while the control database is reachable
	// means the push is not keeping up, which no other signal reports: the
	// pass itself succeeds, so ControlReachable stays true.
	MaxBacklog int
}

// Defaults are not declared here. They live with the configuration that
// resolves them (config.DefaultMaxLeases and friends), because a default in
// two places is a default that disagrees with itself the first time one of
// them is edited.

// Enabled reports whether any bound is configured.
func (q Quota) Enabled() bool {
	return q.MaxLeases > 0 || q.MaxFileBytes > 0 || q.MaxBacklog > 0
}

// Breach is one bound that has been crossed.
type Breach struct {
	// What names the measurement: "leases", "store_bytes", or a queue name
	// for a backlog ("lease_changes", "dns_events", "dhcp_logs", "records",
	// "zone_serials").
	What string
	// Used is the measurement and Limit its bound, in the unit What implies.
	Used  int64
	Limit int64
}

// String renders a breach the way it appears in a log line.
func (b Breach) String() string {
	return fmt.Sprintf("%s=%d/%d", b.What, b.Used, b.Limit)
}

// Check compares a probe view against the bounds.
//
// It returns the breaches in a fixed order so that a probe body and a log
// line are stable across calls. It never fails: a bound it cannot measure
// (a store with no file, a domain this store does not own) is a bound it does
// not report, which is the only safe direction for a signal that cannot be
// acted on.
func (q Quota) Check(s *Store, st Status) []Breach {
	if !q.Enabled() {
		return nil
	}

	var out []Breach
	if q.MaxLeases > 0 && int64(st.HeldLeases) > int64(q.MaxLeases) {
		out = append(out, Breach{What: "leases", Used: int64(st.HeldLeases), Limit: int64(q.MaxLeases)})
	}
	if q.MaxFileBytes > 0 && s != nil {
		if n, err := s.FileSize(); err == nil && n > q.MaxFileBytes {
			out = append(out, Breach{What: "store_bytes", Used: n, Limit: q.MaxFileBytes})
		}
	}
	if q.MaxBacklog > 0 {
		for _, queue := range st.PendingByQueue() {
			if int64(queue.Pending) > int64(q.MaxBacklog) {
				out = append(out, Breach{What: queue.Name, Used: int64(queue.Pending), Limit: int64(q.MaxBacklog)})
			}
		}
	}
	return out
}

// FileSize reports how much disk the store occupies: the database file plus
// its write-ahead log if one is present.
//
// The WAL is included because in WAL mode the main file can lag the log by a
// checkpoint, so measuring only the database would under-report exactly the
// case this bound exists to catch. A store opened in memory has no files and
// reports zero.
func (s *Store) FileSize() (int64, error) {
	if s == nil {
		return 0, nil
	}
	path := dsnPath(s.dsn)
	if path == "" {
		return 0, nil
	}

	var total int64
	for _, p := range []string{path, path + "-wal"} {
		info, err := os.Stat(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			// The database file itself missing is not an error worth
			// failing a probe over -- SQLite recreates it -- but a real
			// stat failure (permissions, I/O) should be visible.
			if p == path {
				return 0, err
			}
			continue
		}
		total += info.Size()
	}
	return total, nil
}

// dsnPath returns the filesystem path a DSN names, or "" when it names
// nothing on disk (an in-memory store).
func dsnPath(dsn string) string {
	dsn = strings.TrimSpace(dsn)
	// Unconditional: TrimPrefix already returns the input unchanged when the
	// prefix is absent, so a HasPrefix guard would only add a branch.
	dsn = strings.TrimPrefix(dsn, "file:")
	// Strip any query string: the driver's parameters are not part of the path.
	if i := strings.Index(dsn, "?"); i >= 0 {
		dsn = dsn[:i]
	}
	if dsn == "" || dsn == ":memory:" {
		return ""
	}
	return dsn
}
