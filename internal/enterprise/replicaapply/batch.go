package replicaapply

import (
	"bytes"
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"time"

	"github.com/jincaiw/GoDDI/internal/enterprise/eventlog"
	"github.com/jincaiw/GoDDI/internal/enterprise/replicabatch"
)

// Batch remains closed: a validated batch does not establish authority.
func Batch(context.Context, []eventlog.Event) error { return ErrNoAuthority }

// The fixture caps batches at 16 events, below the protocol ceiling of 256.
// replicabatch bounds aggregate payload at 8 MiB; transport must separately
// bound the complete protobuf response. The caller must not mutate the event
// slice or payloads during this call. All replay, including mixed prefixes and
// suffixes, fails closed; this is not a production compatibility claim.
const fixtureMaxBatchEvents = 16

func (a *testApplier) applyBatchFixture(ctx context.Context, anchor eventlog.Position, input []eventlog.Event) (err error) {
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
	if len(input) == 0 || len(input) > fixtureMaxBatchEvents {
		return eventlog.ErrChainBreak
	}
	if anchor.Epoch != 1 {
		return eventlog.ErrEpochMismatch
	}
	batch, verifyErr := replicabatch.Verify(replicabatch.Expected{Cluster: a.cluster, Leader: a.leader, Start: anchor, Schema: eventlog.SchemaVersionV1}, input)
	if verifyErr != nil {
		return verifyErr
	}
	conn, e := a.db.Conn(ctx)
	if e != nil {
		return e
	}
	defer conn.Close()
	for _, c := range []struct{ q, w string }{{"PRAGMA foreign_keys", "1"}, {"PRAGMA journal_mode", "wal"}, {"PRAGMA synchronous", "2"}} {
		var v string
		if e = conn.QueryRowContext(ctx, c.q).Scan(&v); e != nil {
			return e
		}
		if v != c.w {
			return fmt.Errorf("replicaapply: unsafe %s", c.q)
		}
	}
	if _, e = conn.ExecContext(ctx, "BEGIN IMMEDIATE"); e != nil {
		return e
	}
	clean := false
	defer func() {
		if clean {
			return
		}
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if _, e := conn.ExecContext(cleanup, "ROLLBACK"); e != nil {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			err = errors.Join(err, fmt.Errorf("replicaapply: rollback unconfirmed: %w", e))
		}
	}()
	var epoch, seq, schema int64
	var hash []byte
	e = conn.QueryRowContext(ctx, `SELECT epoch,seq,schema_version,CASE WHEN typeof(hash)='blob' AND length(hash)=32 THEN hash END FROM replica_head WHERE cluster_id=?`, a.cluster[:]).Scan(&epoch, &seq, &schema, &hash)
	if e != nil || epoch != 1 || seq < 0 || schema != int64(eventlog.SchemaVersionV1) || len(hash) != 32 {
		return ErrHead
	}
	if seq == 0 && !bytes.Equal(hash, make([]byte, 32)) {
		return ErrHead
	}
	if seq > 0 {
		current, e := a.stored(ctx, conn, epoch, seq)
		if e != nil || !bytes.Equal(current.Position.Hash[:], hash) {
			return ErrHead
		}
	}
	if anchor.Epoch != uint64(epoch) || anchor.Seq != uint64(seq) || !bytes.Equal(anchor.Hash[:], hash) {
		return ErrHead
	}
	for i := 0; i < batch.Len(); i++ {
		b, ok := batch.At(i)
		if !ok {
			return ErrHead
		}
		event := b.Event()
		if e = staticWriter(b)(ctx, conn); e != nil {
			return e
		}
		result, e := conn.ExecContext(ctx, `INSERT INTO replica_event(cluster_id,epoch,seq,leader_id,schema_version,committed_unix_ms,prev_hash,hash,payload) VALUES(?,?,?,?,?,?,?,?,?)`, a.cluster[:], epoch, int64(event.Position.Seq), a.leader[:], event.Schema, event.CommitMs, event.PrevHash[:], event.Position.Hash[:], event.Payload)
		if e != nil {
			return e
		}
		if e = requireOne(result); e != nil {
			return e
		}
		result, e = conn.ExecContext(ctx, `UPDATE replica_head SET seq=?,hash=?,schema_version=?,advanced_at_unix_ms=? WHERE cluster_id=? AND epoch=? AND seq=? AND hash=?`, int64(event.Position.Seq), event.Position.Hash[:], event.Schema, event.CommitMs, a.cluster[:], epoch, seq, hash)
		if e != nil {
			return e
		}
		if e = requireOne(result); e != nil {
			return ErrHead
		}
		seq = int64(event.Position.Seq)
		hash = append(hash[:0], event.Position.Hash[:]...)
	}
	if a.beforeCommit != nil {
		if e = a.beforeCommit(); e != nil {
			return e
		}
	}
	if _, e = conn.ExecContext(ctx, "COMMIT"); e != nil {
		_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		return fmt.Errorf("replicaapply: commit outcome unknown: %w", e)
	}
	clean = true
	if a.afterCommit != nil {
		if e = a.afterCommit(); e != nil {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			return fmt.Errorf("replicaapply: commit outcome unknown: %w", e)
		}
	}
	return nil
}
