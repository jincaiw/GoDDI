// Package replicaapply contains an isolated application prototype. It does not
// authenticate peers, authorize a role, perform schema migration or verify a
// production database. The public entry point cannot apply events.
package replicaapply

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jasonwa/goddi/internal/enterprise/eventlog"
)

var ErrNoAuthority = errors.New("replicaapply: no authenticated authority")
var ErrStopped = errors.New("replicaapply: write channel stopped after uncertain result")
var ErrHead = errors.New("replicaapply: missing or invalid trusted head")

// Apply is intentionally unavailable until peer authentication, role binding,
// schema validation, and durable quarantine/recovery are implemented.
func Apply(context.Context, eventlog.Event) error { return ErrNoAuthority }

// testApplier has no exported constructor or production entry point. Its
// trust identities and genesis are injected exclusively by private tests.
type testApplier struct {
	mu                        sync.Mutex
	db                        *sql.DB
	cluster, leader           eventlog.ID
	stopped                   bool
	beforeCommit, afterCommit func() error
}

// run pins one physical connection and never returns an unconfirmed rollback
// to its pool. Commit ambiguity or failed rollback stops this fixture channel;
// no automatic resume or ACK is available.
func (a *testApplier) run(ctx context.Context, event eventlog.Event, write func(context.Context, *sql.Conn) error) (err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.stopped {
		return ErrStopped
	}
	defer func() {
		if err != nil {
			a.stopped = true
		}
	}()
	if event.ClusterID != a.cluster || event.LeaderID != a.leader {
		return eventlog.ErrIdentityMismatch
	}
	if event.Position.Epoch != 1 {
		return eventlog.ErrEpochMismatch
	}
	if err = event.Verify(); err != nil {
		return err
	}
	conn, err := a.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	for _, check := range []struct{ query, want string }{{"PRAGMA foreign_keys", "1"}, {"PRAGMA journal_mode", "wal"}, {"PRAGMA synchronous", "2"}} {
		var got string
		if err = conn.QueryRowContext(ctx, check.query).Scan(&got); err != nil {
			return err
		}
		if got != check.want {
			return fmt.Errorf("replicaapply: unsafe %s", check.query)
		}
	}
	if _, err = conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	clean := false
	defer func() {
		if clean {
			return
		}
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if _, rollbackErr := conn.ExecContext(cleanup, "ROLLBACK"); rollbackErr != nil {
			a.stopped = true
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			err = errors.Join(err, fmt.Errorf("replicaapply: rollback unconfirmed: %w", rollbackErr))
		}
	}()
	var epoch, seq, schema int64
	var hash []byte
	err = conn.QueryRowContext(ctx, `SELECT epoch,seq,schema_version,CASE WHEN typeof(hash)='blob' AND length(hash)=32 THEN hash END FROM replica_head WHERE cluster_id=?`, a.cluster[:]).Scan(&epoch, &seq, &schema, &hash)
	if err != nil || epoch != 1 || seq < 0 || schema != int64(eventlog.SchemaVersionV1) || len(hash) != 32 {
		return ErrHead
	}
	if seq == 0 && string(hash) != string(make([]byte, 32)) {
		return ErrHead
	}
	if seq > 0 {
		current, e := a.stored(ctx, conn, epoch, seq)
		if e != nil || !bytes.Equal(current.Position.Hash[:], hash) {
			return ErrHead
		}
	}
	// This historical no-op checks only a stored event hash; it does not prove
	// current business projections, authority, or retained history continuity.
	if event.Position.Seq <= uint64(seq) {
		prior, e := a.stored(ctx, conn, epoch, int64(event.Position.Seq))
		if e != nil || prior.Position.Hash != event.Position.Hash {
			return eventlog.ErrDiverged
		}
		if _, err = conn.ExecContext(ctx, "ROLLBACK"); err != nil {
			return err
		}
		clean = true
		return nil
	}
	if event.Position.Seq != uint64(seq)+1 || string(event.PrevHash[:]) != string(hash) {
		return eventlog.ErrChainBreak
	}
	if err = write(ctx, conn); err != nil {
		return err
	}
	result, err := conn.ExecContext(ctx, `INSERT INTO replica_event(cluster_id,epoch,seq,leader_id,schema_version,committed_unix_ms,prev_hash,hash,payload) VALUES(?,?,?,?,?,?,?,?,?)`, a.cluster[:], epoch, int64(event.Position.Seq), a.leader[:], event.Schema, event.CommitMs, event.PrevHash[:], event.Position.Hash[:], event.Payload)
	if err != nil {
		return err
	}
	if err = requireOne(result); err != nil {
		return err
	}
	res, err := conn.ExecContext(ctx, `UPDATE replica_head SET seq=?,hash=?,schema_version=?,advanced_at_unix_ms=? WHERE cluster_id=? AND epoch=? AND seq=? AND hash=?`, int64(event.Position.Seq), event.Position.Hash[:], event.Schema, event.CommitMs, a.cluster[:], epoch, seq, hash)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrHead
	}
	if _, err = conn.ExecContext(ctx, "COMMIT"); err != nil {
		a.stopped = true
		return fmt.Errorf("replicaapply: commit outcome unknown: %w", err)
	}
	clean = true
	return nil
}

// requireOne detects ignored or indeterminate writes. It does not replace
// checking the complete schema under the same transaction and authority gate.
func requireOne(result sql.Result) error {
	if result == nil {
		return errors.New("replicaapply: missing write result")
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("replicaapply: write count unavailable: %w", err)
	}
	if n != 1 {
		return fmt.Errorf("replicaapply: write affected %d rows, expected 1", n)
	}
	return nil
}

// stored reconstructs and verifies the original stored event, never trusting a
// digest column alone. Every variable-size field is bounded before Scan.
func (a *testApplier) stored(ctx context.Context, c *sql.Conn, epoch, seq int64) (eventlog.Event, error) {
	var e eventlog.Event
	var leader, previous, hash, payload []byte
	var size sql.NullInt64
	err := c.QueryRowContext(ctx, `SELECT
 CASE WHEN typeof(leader_id)='blob' AND length(leader_id)=16 THEN leader_id END,
 schema_version,committed_unix_ms,
 CASE WHEN typeof(prev_hash)='blob' AND length(prev_hash)=32 THEN prev_hash END,
 CASE WHEN typeof(hash)='blob' AND length(hash)=32 THEN hash END,
 length(payload),CASE WHEN typeof(payload)='blob' AND length(payload)<=? THEN payload END
 FROM replica_event WHERE cluster_id=? AND epoch=? AND seq=?`, eventlog.MaxPayloadBytes, a.cluster[:], epoch, seq).Scan(&leader, &e.Schema, &e.CommitMs, &previous, &hash, &size, &payload)
	if err != nil {
		return e, err
	}
	if len(leader) != 16 || len(previous) != 32 || len(hash) != 32 || !size.Valid || size.Int64 <= 0 || size.Int64 > eventlog.MaxPayloadBytes || int64(len(payload)) != size.Int64 {
		return e, ErrHead
	}
	copy(e.LeaderID[:], leader)
	copy(e.PrevHash[:], previous)
	copy(e.Position.Hash[:], hash)
	e.ClusterID = a.cluster
	e.Position.Epoch = uint64(epoch)
	e.Position.Seq = uint64(seq)
	e.Payload = payload
	if e.LeaderID != a.leader {
		return e, eventlog.ErrIdentityMismatch
	}
	if err = e.Verify(); err != nil {
		return e, err
	}
	return e, nil
}
