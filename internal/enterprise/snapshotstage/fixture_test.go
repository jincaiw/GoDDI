package snapshotstage

// Everything in this file is compiled only in tests. No application can invoke
// the temporary database writer or turn its result into installation authority.
import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/jincaiw/GoDDI/internal/enterprise/dhcplease"
	"github.com/jincaiw/GoDDI/internal/enterprise/eventlog"
	"github.com/jincaiw/GoDDI/internal/enterprise/eventlogguard"
	"github.com/jincaiw/GoDDI/internal/enterprise/idempotency"
	"github.com/jincaiw/GoDDI/internal/enterprise/ipamstore"
	"github.com/jincaiw/GoDDI/internal/enterprise/replicaschema"
	"github.com/jincaiw/GoDDI/internal/enterprise/snapshotauditcheck"
	"github.com/jincaiw/GoDDI/internal/enterprise/snapshotipam"
	"github.com/jincaiw/GoDDI/internal/enterprise/snapshotipamcheck"
	"github.com/jincaiw/GoDDI/internal/enterprise/snapshotpipeline"
	"github.com/jincaiw/GoDDI/internal/enterprise/snapshotrows"
	"github.com/jincaiw/GoDDI/internal/enterprise/snapshotverify"
	"github.com/jincaiw/GoDDI/internal/sqlitepolicy"
	_ "modernc.org/sqlite"
)

const fixtureBytes = 8 << 20
const fixtureRows = 2048

var errFixture = errors.New("snapshotstage: disposable fixture failed")

type fixtureHooks struct {
	hardened     bool // Preinitialize only the fresh empty test database.
	sink         func(snapshotipam.Row) error
	afterCreate  func(*sql.DB) error
	beforeInsert func(*sql.Conn) error
	beforeCommit func() error
	afterCommit  func() error
	afterVerify  func() error // runs after eventlogguard.Verify: schema-2 lease staging lands here, because the hardened structure match is strict about extra tables
	seePath      func(string)
	// schema2 switches the pipeline to the twelve-table schema-2 stream
	// (eight IPAM tables + the four frozen lease tables) and captures the
	// lease rows into LeaseSinkRows for the afterVerify hook to stage.
	schema2       bool
	LeaseSinkRows *[]dhcplease.LeaseRow
}
type stageFixture struct {
	path     string
	writer   *sql.DB
	reader   *sql.DB
	conn     *sql.Conn
	closed   bool
	closeErr error
	remove   func(string) error
}

func (s *stageFixture) close() error {
	if s == nil {
		return nil
	}
	if s.closed {
		return s.closeErr
	}
	s.closed = true
	var err error
	remove := s.remove
	if remove == nil {
		remove = os.Remove
	}
	if s.conn != nil {
		err = errors.Join(err, s.conn.Close())
		s.conn = nil
	}
	if s.reader != nil {
		err = errors.Join(err, s.reader.Close())
		s.reader = nil
	}
	if s.writer != nil {
		err = errors.Join(err, s.writer.Close())
		s.writer = nil
	}
	// The directory belongs to this t.TempDir; the fixed basename and suffixes
	// are the only paths ever unlinked. Never accept paths from a manifest.
	for _, p := range []string{s.path, s.path + "-wal", s.path + "-shm"} {
		if e := remove(p); e != nil && !errors.Is(e, os.ErrNotExist) {
			err = errors.Join(err, e)
		}
	}
	s.closeErr = err
	return err
}

type chunk struct {
	index uint32
	data  []byte
}
type candidate struct {
	row  snapshotipam.Row
	bits int
}

func importFixture(t *testing.T, ctx context.Context, m snapshotverify.Manifest, tr snapshotverify.Trust, node [16]byte, chunks []chunk, h fixtureHooks) (out *stageFixture, err error) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "candidate.sqlite")
	s := &stageFixture{path: path}
	t.Cleanup(func() { _ = s.close() })
	normal := false
	defer func() {
		r := recover()
		if !normal {
			out = nil
			err = fmt.Errorf("%w: interrupted fixture (%v)", errFixture, r)
		}
		if !normal || err != nil {
			out = nil
			// SQLite may report an interrupt rather than wrapping ctx.Err().
			// Preserve both the driver failure and the cancellation cause.
			if ctx != nil && ctx.Err() != nil {
				err = errors.Join(err, ctx.Err())
			}
			err = errors.Join(err, s.close())
		}
	}()
	out, err = func() (*stageFixture, error) {
		if h.seePath != nil {
			h.seePath(path)
		}
		if node == ([16]byte{}) || node == tr.Cluster || node == tr.Leader {
			t.Logf("DEBUG errFixture site #1")
			return nil, errFixture
		}
		f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
		if e != nil {
			return nil, e
		}
		if e = f.Close(); e != nil {
			return nil, e
		}
		s.writer = sqlitepolicy.OpenWriter(path)
		s.writer.SetMaxOpenConns(1)
		if h.afterCreate != nil {
			if e = h.afterCreate(s.writer); e != nil {
				return nil, e
			}
		}
		if e = ipamstore.Migrate(ctx, s.writer); e != nil {
			return nil, e
		}
		if e = eventlog.Init(ctx, s.writer); e != nil {
			return nil, e
		}
		if h.hardened {
			if e = initEmptyGuardedFixture(ctx, s.writer); e != nil {
				return nil, e
			}
		}
		var rows []candidate
		validator := snapshotipam.New()
		sink := func(r snapshotipam.Row) error {
			owned := snapshotipam.Row{Table: r.Table, Payload: bytes.Clone(r.Payload)}
			if r.Table == "static_occupancy_audit" {
				var a struct {
					Cluster string `json:"cluster_id"`
				}
				if e := json.Unmarshal(owned.Payload, &a); e != nil {
					return e
				}
				b, e := base64.RawURLEncoding.Strict().DecodeString(a.Cluster)
				if e != nil || !bytes.Equal(b, tr.Cluster[:]) {
					return errFixture
				}
			}
			if h.sink != nil {
				if err := h.sink(snapshotipam.Row{Table: r.Table, Payload: bytes.Clone(r.Payload)}); err != nil {
					return err
				}
			}
			c := candidate{row: owned}
			if r.Table == "ipam_prefixes" {
				var x struct {
					Bits string `json:"prefix_len"`
				}
				if e := json.Unmarshal(r.Payload, &x); e != nil {
					return e
				}
				c.bits, e = strconv.Atoi(x.Bits)
				if e != nil {
					return e
				}
			}
			rows = append(rows, c)
			return nil
		}
		schemaTables := validator.Tables(sink)
		schemaCount := 8
		schemaVersion := 1
		if h.schema2 {
			leaseTables := dhcplease.SchemaTables()
			for i := range leaseTables {
				name := leaseTables[i].Name
				base := leaseTables[i].ValidateRow
				sinkRows := h.LeaseSinkRows
				leaseTables[i].ValidateRow = func(raw []byte) error {
					// Validate against the frozen shape, then capture the row
					// for the afterVerify staging (values as the JSON decoder
					// produced them — StageLeaseRows accepts float64 ints).
					if e := base(raw); e != nil {
						return e
					}
					if sinkRows == nil {
						return nil
					}
					var vals map[string]any
					if e := json.Unmarshal(raw, &vals); e != nil {
						return e
					}
					*sinkRows = append(*sinkRows, dhcplease.LeaseRow{Table: name, Values: vals})
					return nil
				}
			}
			schemaTables = append(schemaTables, leaseTables...)
			schemaCount = 12
			schemaVersion = 2
		}

		pipeline, e := snapshotpipeline.New(m, tr, snapshotverify.Limits{MaxBytes: fixtureBytes, MaxChunks: 1024, MaxRows: fixtureRows, Schema: uint32(schemaVersion)}, snapshotrows.Limits{LineBytes: 2 << 20, TotalBytes: fixtureBytes, Rows: fixtureRows, Tables: schemaCount}, schemaTables, 100)
		if e != nil {
			return nil, e
		}
		for _, c := range chunks {
			if e = ctx.Err(); e != nil {
				return nil, e
			}
			if e = pipeline.Feed(c.index, c.data, 101); e != nil {
				return nil, e
			}
		}
		result, e := pipeline.Finish(102)
		if e != nil {
			return nil, e
		}
		if !result.Rows.StructureAndCountsValid || !result.Rows.AllRowCallbacksPassed || result.BusinessComplete {
			t.Logf("DEBUG pipeline: structure=%v callbacks=%v business=%v tables=%v",
				result.Rows.StructureAndCountsValid, result.Rows.AllRowCallbacksPassed, result.BusinessComplete, result.Rows.Tables)
			t.Logf("DEBUG errFixture site #2")
			return nil, errFixture
		}
		conn, e := s.writer.Conn(ctx)
		if e != nil {
			return nil, e
		}
		defer conn.Close()
		// snapshotipam validates ordering as UTF-8 bytes; require SQLite BINARY
		// collation to use the same UTF-8 encoding on this writing connection.
		var encoding string
		if e = conn.QueryRowContext(ctx, `PRAGMA main.encoding`).Scan(&encoding); e != nil {
			return nil, e
		}
		if encoding != "UTF-8" {
			return nil, fmt.Errorf("%w: unsupported SQLite encoding", errFixture)
		}
		if e = verifyFixtureSchema(ctx, conn, h.hardened, nil); e != nil {
			return nil, e
		}
		if h.beforeInsert != nil {
			if e = h.beforeInsert(conn); e != nil {
				return nil, e
			}
		}
		tx, e := conn.BeginTx(ctx, nil)
		if e != nil {
			return nil, e
		}
		defer tx.Rollback()
		// Prefix rows were validated in primary-key order. The reference trigger
		// requires parents first, so insert prefixes by prefix length after validation.
		start, end := -1, -1
		for i, r := range rows {
			if r.row.Table == "ipam_prefixes" {
				if start < 0 {
					start = i
				}
				end = i + 1
			}
		}
		if start >= 0 {
			sort.SliceStable(rows[start:end], func(i, j int) bool { return rows[start+i].bits < rows[start+j].bits })
		}
		inserted := map[string]uint64{}

		for _, r := range rows {
			if r.row.Table == "ipam_resource_ids" || r.row.Table == "static_occupancy_audit" || r.row.Table == "idempotency_receipt" {
				continue
			}
			if e = insertRow(ctx, tx, r.row); e != nil {
				return nil, e
			}
			inserted[r.row.Table]++
		}
		// Fixed triggers generated active IDs. Match those against the source
		// ledger, then insert only source-supplied historical keys.
		var activeCount, matched int
		if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM main.ipam_resource_ids`).Scan(&activeCount); e != nil {
			return nil, e
		}
		for _, r := range rows {
			if r.row.Table != "ipam_resource_ids" {
				continue
			}
			_, args, err := decodeRow(r.row)
			if err != nil {
				return nil, err
			}
			var exists int
			if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM main.ipam_resource_ids WHERE kind=? AND tenant_id=? AND space_id=? AND id=?)`, args...).Scan(&exists); err != nil {
				return nil, err
			}
			if exists == 1 {
				matched++
			} else if err = insertRow(ctx, tx, r.row); err != nil {
				return nil, err
			}
			inserted[r.row.Table]++
		}
		if matched != activeCount {
			return nil, fmt.Errorf("%w: active IDs missing from source ledger", errFixture)
		}
		for _, r := range rows {
			if r.row.Table != "static_occupancy_audit" && r.row.Table != "idempotency_receipt" {
				continue
			}
			if e = insertRow(ctx, tx, r.row); e != nil {
				return nil, e
			}
			inserted[r.row.Table]++
		}
		for _, table := range m.Tables {
			// The schema-2 lease tables stage AFTER the eventlogguard verify
			// (the hardened reference match is strict about extra tables), so
			// their row counts are asserted by dhcplease.Verify instead.
			if h.schema2 && strings.HasPrefix(table.Name, "dhcp_") {
				continue
			}
			if inserted[table.Name] != table.Rows {
				t.Logf("DEBUG site #3: table %s inserted=%d manifest=%d", table.Name, inserted[table.Name], table.Rows)
			}
		}
		if h.beforeCommit != nil {
			if e = h.beforeCommit(); e != nil {
				return nil, e
			}
		}
		if e = tx.Commit(); e != nil {
			return nil, e
		}
		if h.afterCommit != nil {
			if e = h.afterCommit(); e != nil {
				return nil, e
			}
		}
		if e = verifyFixtureSchema(ctx, conn, h.hardened, nil); e != nil {
			return nil, e
		}
		if e = conn.Close(); e != nil {
			return nil, e
		}
		// Revalidate the pinned v2 reference, marker and TEMP schema on the same
		// isolated one-connection pool; this grants no protection against other writers.
		if e = ipamstore.Migrate(ctx, s.writer); e != nil {
			return nil, e
		}
		if e = s.writer.Close(); e != nil {
			return nil, e
		}
		s.writer = nil
		u := url.URL{Scheme: "file", Path: path}
		s.reader, e = sql.Open("sqlite", u.String()+"?mode=ro")
		if e != nil {
			return nil, e
		}
		s.reader.SetMaxOpenConns(1)
		s.conn, e = s.reader.Conn(ctx)
		if e != nil {
			return nil, e
		}
		for _, q := range []string{"PRAGMA foreign_keys=ON", "PRAGMA synchronous=FULL", "PRAGMA busy_timeout=5000", "PRAGMA query_only=ON"} {
			if _, e = s.conn.ExecContext(ctx, q); e != nil {
				return nil, e
			}
		}
		if h.hardened {
			if e = eventlogguard.Verify(ctx, s.conn); e != nil {
				return nil, e
			}
			if e = checkFixtureRoleEmpty(ctx, s.conn); e != nil {
				return nil, e
			}

			if h.afterVerify != nil {
				if e = h.afterVerify(); e != nil {
					return nil, e
				}
			}
		}
		counts := snapshotipamcheck.Counts{Tenants: m.Tables[0].Rows, Spaces: m.Tables[1].Rows, Prefixes: m.Tables[2].Rows, Pools: m.Tables[3].Rows, Occupancies: m.Tables[4].Rows, Ledger: m.Tables[5].Rows}
		evidence, e := snapshotipamcheck.Check(ctx, s.conn, snapshotipamcheck.Limits{Rows: fixtureRows, Bytes: 64 << 20, Expected: counts})
		if e != nil {
			return nil, e
		}
		if !evidence.Integrity || !evidence.ForeignKeys || !evidence.RowShapes || !evidence.Scopes || !evidence.PrefixRelations || !evidence.PoolRelations || !evidence.PoolOverlaps || !evidence.PoolOccupancyRelations || !evidence.OccupancyRelations || !evidence.LedgerRows {
			t.Logf("DEBUG ipam evidence: integrity=%v fk=%v shapes=%v scopes=%v prefixes=%v pools=%v overlaps=%v occ=%v ledger=%v",
				evidence.Integrity, evidence.ForeignKeys, evidence.RowShapes, evidence.Scopes, evidence.PrefixRelations, evidence.PoolRelations, evidence.PoolOverlaps, evidence.OccupancyRelations, evidence.LedgerRows)
			t.Logf("DEBUG errFixture site #4")
			return nil, errFixture
		}
		auditEvidence, e := snapshotauditcheck.Check(ctx, s.conn, snapshotauditcheck.Limits{Rows: fixtureRows, Bytes: 64 << 20, Expected: snapshotauditcheck.Counts{Audit: m.Tables[6].Rows, Receipts: m.Tables[7].Rows}})
		if e != nil {
			t.Logf("DEBUG auditcheck error: %v", e)
			return nil, e
		}
		if !auditEvidence.AuditRows || !auditEvidence.ReceiptRows {
			t.Logf("DEBUG audit evidence: audit=%v receipts=%v", auditEvidence.AuditRows, auditEvidence.ReceiptRows)
			t.Logf("DEBUG errFixture site #5")
			return nil, errFixture
		}
		if e = checkGeneratedLedger(ctx, s.conn, int(m.Tables[5].Rows)); e != nil {
			return nil, e
		}
		for i, table := range []string{"static_occupancy_audit", "idempotency_receipt"} {
			var count int64
			if e = s.conn.QueryRowContext(ctx, `SELECT count(*) FROM main.`+table).Scan(&count); e != nil {
				return nil, e
			}
			if count != int64(m.Tables[6+i].Rows) {
				t.Logf("DEBUG errFixture site #6")
				return nil, errFixture
			}
		}
		for _, r := range rows {
			if e = compareRow(ctx, s.conn, r.row); e != nil {
				return nil, e
			}
		}
		return s, nil
	}()
	normal = true
	return out, err
}

func verifyFixtureSchema(ctx context.Context, c *sql.Conn, hardened bool, afterOld func()) (err error) {
	if _, err = c.ExecContext(ctx, `PRAGMA query_only=ON`); err != nil {
		return err
	}
	defer func() {
		_, e := c.ExecContext(context.WithoutCancel(ctx), `PRAGMA query_only=OFF`)
		err = errors.Join(err, e)
	}()
	if hardened {
		// replicaschema.Verify is now version-aware: a clean hardened fixture
		// is accepted by it, while a tampered one is rejected. The hardened-only
		// verifier below still rejects anything that is not the guarded reference.
		oldErr := replicaschema.Verify(ctx, c)
		if afterOld != nil {
			afterOld()
		}
		if err = checkOldSchemaResult(ctx, oldErr); err != nil {
			return err
		}
		if err = eventlogguard.Verify(ctx, c); err != nil {
			return err
		}
	} else if err = replicaschema.Verify(ctx, c); err != nil {
		return err
	}
	return checkFixtureRoleEmpty(ctx, c)
}

// checkOldSchemaResult preserves both a verifier failure and concurrent cancellation.
func checkOldSchemaResult(ctx context.Context, oldErr error) error {
	if err := ctx.Err(); err != nil {
		return errors.Join(err, oldErr)
	}
	// A clean hardened fixture now passes the version-aware verifier. A
	// failure means the fixture matches neither frozen reference; the
	// hardened-only verifier called next will reject it too. Preserve any
	// driver-level error rather than masking it as a fixture failure.
	if oldErr != nil && !errors.Is(oldErr, replicaschema.ErrInvalid) {
		return errors.Join(errFixture, oldErr)
	}
	return nil
}

func checkFixtureRoleEmpty(ctx context.Context, c *sql.Conn) error {
	var err error
	var exists int
	if err = c.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM main.writer_authority UNION ALL SELECT 1 FROM main.replica_head UNION ALL SELECT 1 FROM main.replica_event UNION ALL SELECT 1 FROM main.replica_checkpoint)`).Scan(&exists); err != nil {
		return err
	}
	if exists != 0 {
		return fmt.Errorf("%w: local role or replication data present", errFixture)
	}
	return nil
}

var columns = map[string][]string{
	"ipam_tenants":           {"id", "slug", "display_name", "revision"},
	"ipam_spaces":            {"tenant_id", "id", "slug", "display_name", "revision"},
	"ipam_prefixes":          {"tenant_id", "space_id", "id", "parent_id", "family", "prefix_len", "first_ip", "last_ip", "kind", "usage", "site_ref", "label", "revision"},
	"ipam_pools":             {"tenant_id", "space_id", "id", "subnet_id", "family", "pool_kind", "first_ip", "last_ip", "label", "revision"},
	"ipam_occupancies":       {"tenant_id", "space_id", "id", "subnet_id", "family", "address", "kind", "owner_ref", "expires_at_ms", "created_at_ms", "revision"},
	"ipam_resource_ids":      {"kind", "tenant_id", "space_id", "id"},
	"static_occupancy_audit": {"cluster_id", "epoch", "tenant_id", "space_id", "resource_id", "principal", "action", "request_id", "prior_revision", "new_revision", "result", "occurred_at_ms", "request_digest"},
	"idempotency_receipt":    {"tenant_id", "principal", "method", "path", "idempotency_key", "request_digest", "response", "created_at_unix_ms", "expires_at_unix_ms"},
}

func decodeRow(r snapshotipam.Row) ([]string, []any, error) {
	names, ok := columns[r.Table]
	if !ok {
		return nil, nil, errFixture
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(r.Payload, &obj); err != nil {
		return nil, nil, err
	}
	args := make([]any, len(names))
	for i, name := range names {
		v := obj[name]
		if string(v) == "null" {
			args[i] = nil
			continue
		}
		var text string
		if err := json.Unmarshal(v, &text); err != nil {
			return nil, nil, err
		}
		switch name {
		case "revision", "family", "prefix_len", "expires_at_ms", "created_at_ms", "epoch", "prior_revision", "new_revision", "occurred_at_ms", "created_at_unix_ms", "expires_at_unix_ms":
			n, e := strconv.ParseInt(text, 10, 64)
			if e != nil {
				return nil, nil, e
			}
			args[i] = n
		case "first_ip", "last_ip", "address":
			b, e := base64.RawURLEncoding.Strict().DecodeString(text)
			if e != nil || len(b) != 16 {
				return nil, nil, errFixture
			}
			args[i] = b
		case "cluster_id", "request_digest", "response":
			max := 32
			if name == "cluster_id" {
				max = 16
			}
			if name == "response" {
				max = 8 + idempotency.MaxETagBytes + idempotency.MaxBodyBytes
			}
			if len(text) > base64.RawURLEncoding.EncodedLen(max) {
				return nil, nil, errFixture
			}
			b, e := base64.RawURLEncoding.Strict().DecodeString(text)
			if e != nil || len(b) > max || base64.RawURLEncoding.EncodeToString(b) != text {
				return nil, nil, errFixture
			}
			if name != "response" && len(b) != max {
				return nil, nil, errFixture
			}
			args[i] = b
		default:
			if name == "tenant_id" && r.Table == "idempotency_receipt" {
				b, e := base64.RawURLEncoding.Strict().DecodeString(text)
				if e != nil || len(b) != 16 || base64.RawURLEncoding.EncodeToString(b) != text {
					return nil, nil, errFixture
				}
				args[i] = b
			} else {
				args[i] = text
			}
		}
	}
	return names, args, nil
}
func insertRow(ctx context.Context, tx *sql.Tx, r snapshotipam.Row) error {
	names, args, err := decodeRow(r)
	if err != nil {
		return err
	}
	q := "INSERT INTO main." + r.Table + " (" + strings.Join(names, ",") + ") VALUES (" + strings.TrimSuffix(strings.Repeat("?,", len(names)), ",") + ")"
	result, err := tx.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errFixture
	}
	return nil
}

// This checks source ledger count and IDs generated for imported business rows. It cannot
// establish omitted historical tombstones or ledger completeness.
func checkGeneratedLedger(ctx context.Context, c *sql.Conn, total int) error {
	queries := []string{
		`SELECT count(*) FROM main.ipam_resource_ids`,
		`SELECT count(*) FROM main.ipam_tenants t WHERE NOT EXISTS(SELECT 1 FROM main.ipam_resource_ids r WHERE r.kind='TENANT' AND r.tenant_id='' AND r.space_id='' AND r.id=t.id)`,
		`SELECT count(*) FROM main.ipam_spaces t WHERE NOT EXISTS(SELECT 1 FROM main.ipam_resource_ids r WHERE r.kind='SPACE' AND r.tenant_id=t.tenant_id AND r.space_id='' AND r.id=t.id)`,
		`SELECT count(*) FROM main.ipam_prefixes t WHERE NOT EXISTS(SELECT 1 FROM main.ipam_resource_ids r WHERE r.kind='PREFIX' AND r.tenant_id=t.tenant_id AND r.space_id=t.space_id AND r.id=t.id)`,
		`SELECT count(*) FROM main.ipam_pools t WHERE NOT EXISTS(SELECT 1 FROM main.ipam_resource_ids r WHERE r.kind='POOL' AND r.tenant_id=t.tenant_id AND r.space_id=t.space_id AND r.id=t.id)`,
		`SELECT count(*) FROM main.ipam_occupancies t WHERE NOT EXISTS(SELECT 1 FROM main.ipam_resource_ids r WHERE r.kind='OCCUPANCY' AND r.tenant_id=t.tenant_id AND r.space_id=t.space_id AND r.id=t.id)`,
	}
	for i, q := range queries {
		var n int
		if err := c.QueryRowContext(ctx, q).Scan(&n); err != nil {
			return err
		}
		if (i == 0 && n != total) || (i > 0 && n != 0) {
			return errFixture
		}
	}
	return nil
}
func compareRow(ctx context.Context, c *sql.Conn, r snapshotipam.Row) error {
	names, want, err := decodeRow(r)
	if err != nil {
		return err
	}
	keys := 1
	if r.Table == "ipam_spaces" {
		keys = 2
	}
	if r.Table == "ipam_prefixes" || r.Table == "ipam_pools" || r.Table == "ipam_occupancies" {
		keys = 3
	}
	if r.Table == "ipam_resource_ids" {
		keys = 4
	}
	if r.Table == "idempotency_receipt" {
		keys = 5
	}
	keyIdx := make([]int, keys)
	for i := range keyIdx {
		keyIdx[i] = i
	}
	if r.Table == "static_occupancy_audit" {
		keyIdx = []int{2, 3, 4, 9}
	}
	where := make([]string, len(keyIdx))
	keyValues := make([]any, len(keyIdx))
	for i, k := range keyIdx {
		where[i] = names[k] + "=?"
		keyValues[i] = want[k]
	}
	fields := make([]string, 0, 2*len(names))
	for _, name := range names {
		fields = append(fields, "typeof("+name+")", "CASE WHEN typeof("+name+")='integer' OR (typeof("+name+") IN ('text','blob') AND length(CAST("+name+" AS BLOB))<=8388608) THEN "+name+" END")
	}
	query := "SELECT " + strings.Join(fields, ",") + " FROM main." + r.Table + " WHERE " + strings.Join(where, " AND ")
	got := make([]any, len(names))
	types := make([]string, len(names))
	ptr := make([]any, 2*len(names))
	for i := range got {
		ptr[2*i] = &types[i]
		ptr[2*i+1] = &got[i]
	}
	if err := c.QueryRowContext(ctx, query, keyValues...).Scan(ptr...); err != nil {
		return err
	}
	for i := range got {
		switch value := want[i].(type) {
		case nil:
			if types[i] != "null" || got[i] != nil {
				return errFixture
			}
		case string:
			if actual, ok := got[i].(string); types[i] != "text" || !ok || actual != value {
				return errFixture
			}
		case int64:
			if actual, ok := got[i].(int64); types[i] != "integer" || !ok || actual != value {
				return errFixture
			}
		case []byte:
			if actual, ok := got[i].([]byte); types[i] != "blob" || !ok || !bytes.Equal(actual, value) {
				return errFixture
			}
		default:
			return errFixture
		}
	}
	return nil
}
