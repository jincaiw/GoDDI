package eventlog

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"math"
	"time"
)

// ErrNoAuthority means this node is not the recorded writer for the cluster,
// or the epoch it holds no longer matches the authority record. A role or
// licence read at startup is never a permanent grant: every protected
// transaction re-checks it (docs/01-architecture.md "每次受保护提交…使用仍有效的所有权代次").
var ErrNoAuthority = errors.New("eventlog: node holds no valid write authority")

// ErrStaleAuthority means the head moved under the caller: something else
// appended between the read and the compare-and-set.
var ErrStaleAuthority = errors.New("eventlog: replica head moved concurrently")

// ErrAlreadyAppended guards the rule that one business transaction appends at
// most one log entry (docs/04-replication-ha.md "持久化结构").
var ErrAlreadyAppended = errors.New("eventlog: this transaction already appended its event")

// Schema is an isolated empty-database POC, not an existing-deployment migration.
// STATIC events carry candidate audit and receipt bodies; replica apply and
// audit export are not implemented. Begin remains closed pending safety gates.
const Schema = `
CREATE TABLE IF NOT EXISTS writer_authority (
	cluster_id             BLOB PRIMARY KEY CHECK (length(cluster_id) = 16),
	holder_id              BLOB NOT NULL CHECK (length(holder_id) = 16),
	epoch                  INTEGER NOT NULL CHECK (epoch >= 1),
	db_generation          TEXT NOT NULL,
	grant_digest           BLOB NOT NULL CHECK (length(grant_digest) = 32),
	granted_at_unix_ms     INTEGER NOT NULL,
	CHECK (cluster_id <> holder_id)
);

CREATE TABLE IF NOT EXISTS replica_head (
	cluster_id            BLOB PRIMARY KEY CHECK (length(cluster_id) = 16),
	epoch                 INTEGER NOT NULL CHECK (epoch >= 1),
	seq                   INTEGER NOT NULL CHECK (seq >= 0),
	hash                  BLOB NOT NULL CHECK (length(hash) = 32),
	schema_version        INTEGER NOT NULL CHECK (schema_version >= 1),
	advanced_at_unix_ms   INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS replica_event (
	cluster_id            BLOB NOT NULL CHECK (length(cluster_id) = 16),
	epoch                 INTEGER NOT NULL CHECK (epoch >= 1),
	seq                   INTEGER NOT NULL CHECK (seq >= 1),
	leader_id             BLOB NOT NULL CHECK (length(leader_id) = 16),
	schema_version        INTEGER NOT NULL CHECK (schema_version >= 1),
	committed_unix_ms     INTEGER NOT NULL,
	prev_hash             BLOB NOT NULL CHECK (length(prev_hash) = 32),
	hash                  BLOB NOT NULL CHECK (length(hash) = 32),
	payload               BLOB NOT NULL
) STRICT;

CREATE UNIQUE INDEX IF NOT EXISTS replica_event_position
	ON replica_event (cluster_id, epoch, seq);

CREATE INDEX IF NOT EXISTS replica_event_order
	ON replica_event (cluster_id, epoch, seq);

-- SQLite REPLACE can delete a conflicting row without running ordinary
-- DELETE triggers. Reject the conflicting insert before REPLACE can do so.
CREATE TRIGGER IF NOT EXISTS replica_event_no_replace
	BEFORE INSERT ON replica_event
	WHEN EXISTS (SELECT 1 FROM replica_event e WHERE e.cluster_id=NEW.cluster_id
	             AND e.epoch=NEW.epoch AND e.seq=NEW.seq)
BEGIN
	SELECT RAISE(ABORT, 'replica_event position already exists');
END;

-- An appended event is immutable. Retention and verified recovery are the only
-- paths allowed to remove history, and they never run as a side effect of an
-- ordinary transaction (docs/04 "插入后不可变").
CREATE TRIGGER IF NOT EXISTS replica_event_no_update
	BEFORE UPDATE ON replica_event
BEGIN
	SELECT RAISE(ABORT, 'replica_event rows are immutable');
END;

CREATE TRIGGER IF NOT EXISTS replica_event_no_delete
	BEFORE DELETE ON replica_event
BEGIN
	SELECT RAISE(ABORT, 'replica_event history needs the retention procedure');
END;

-- The position a peer has confirmed. A checkpoint only moves forward inside an
-- epoch; a late report from an old epoch must not rewind the watermark
-- (docs/04 "确认水位仅单调推进").
CREATE TABLE IF NOT EXISTS replica_checkpoint (
	cluster_id            BLOB NOT NULL CHECK (length(cluster_id) = 16),
	source_id             BLOB NOT NULL CHECK (length(source_id) = 16),
	epoch                 INTEGER NOT NULL CHECK (epoch >= 1),
	seq                   INTEGER NOT NULL CHECK (seq >= 0),
	hash                  BLOB NOT NULL CHECK (length(hash) = 32),
	applied_at_unix_ms    INTEGER NOT NULL,
	PRIMARY KEY (cluster_id, source_id)
);

-- The receipt of a submitted write, stored with the write it describes.
-- Replaying the same key returns the stored response instead of occupying the
-- address a second time (docs/02-data-ipam.md "幂等与身份").
CREATE TABLE IF NOT EXISTS idempotency_receipt (
	tenant_id           BLOB NOT NULL CHECK (length(tenant_id) = 16),
	principal           TEXT NOT NULL,
	method              TEXT NOT NULL,
	path                TEXT NOT NULL,
	idempotency_key     TEXT NOT NULL,
	request_digest      BLOB NOT NULL CHECK (length(request_digest) = 32),
	response            BLOB NOT NULL,
	created_at_unix_ms  INTEGER NOT NULL,
	expires_at_unix_ms  INTEGER NOT NULL,
	PRIMARY KEY (tenant_id, principal, method, path, idempotency_key)
);

-- REPLACE can bypass DELETE triggers. Do not allow any conflicting insert to
-- overwrite an existing receipt, including a retry with a different digest.
CREATE TRIGGER IF NOT EXISTS idempotency_receipt_no_replace
	BEFORE INSERT ON idempotency_receipt
	WHEN EXISTS (SELECT 1 FROM idempotency_receipt r
	             WHERE r.tenant_id=NEW.tenant_id AND r.principal=NEW.principal
	               AND r.method=NEW.method AND r.path=NEW.path
	               AND r.idempotency_key=NEW.idempotency_key)
BEGIN
	SELECT RAISE(ABORT, 'idempotency receipt already exists');
END;

CREATE TRIGGER IF NOT EXISTS idempotency_receipt_immutable
	BEFORE UPDATE ON idempotency_receipt
BEGIN
	SELECT RAISE(ABORT, 'an idempotency receipt is written once, with its transaction');
END;

-- POC local-only audit: retention/export and replication are not implemented.
CREATE TABLE IF NOT EXISTS static_occupancy_audit (
 cluster_id BLOB NOT NULL CHECK(length(cluster_id)=16),
 epoch INTEGER NOT NULL CHECK(epoch>0),
 tenant_id TEXT NOT NULL, space_id TEXT NOT NULL, resource_id TEXT NOT NULL,
 principal TEXT NOT NULL, action TEXT NOT NULL CHECK(action='ipam.occupancy.create'),
 request_id TEXT NOT NULL, prior_revision INTEGER NOT NULL CHECK(prior_revision=0),
 new_revision INTEGER NOT NULL CHECK(new_revision=1), result TEXT NOT NULL CHECK(result='SUCCESS'),
 occurred_at_ms INTEGER NOT NULL CHECK(occurred_at_ms>=0),
 request_digest BLOB NOT NULL CHECK(length(request_digest)=32),
 PRIMARY KEY(tenant_id,space_id,resource_id,new_revision)
) STRICT;
CREATE TRIGGER IF NOT EXISTS static_occupancy_audit_no_replace BEFORE INSERT ON static_occupancy_audit
WHEN EXISTS (SELECT 1 FROM static_occupancy_audit a WHERE a.tenant_id=NEW.tenant_id AND a.space_id=NEW.space_id AND a.resource_id=NEW.resource_id AND a.new_revision=NEW.new_revision)
BEGIN SELECT RAISE(ABORT, 'audit already exists'); END;
CREATE TRIGGER IF NOT EXISTS static_occupancy_audit_no_update BEFORE UPDATE ON static_occupancy_audit
BEGIN SELECT RAISE(ABORT, 'audit immutable'); END;
CREATE TRIGGER IF NOT EXISTS static_occupancy_audit_no_delete BEFORE DELETE ON static_occupancy_audit
BEGIN SELECT RAISE(ABORT, 'audit immutable'); END;
`

// Init creates the replication tables atomically. It is currently used only
// for isolated development and tests; it is not an upstream schema migration.
func Init(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, Schema); err != nil {
		return fmt.Errorf("eventlog: create replication schema: %w", err)
	}
	return tx.Commit()
}

// Authority records which node holds the single write right and at which
// epoch. The row alone grants nothing: the node's own role log is the other
// half, and neither travels in a snapshot or a restore.
type Authority struct {
	Cluster    ID
	Holder     ID
	Epoch      uint64
	Generation string
	Grant      [HashBytes]byte
	GrantedAt  time.Time
}

// CurrentAuthority reads the authority record.
func CurrentAuthority(ctx context.Context, db *sql.DB, cluster ID) (Authority, error) {
	if ctx == nil || db == nil {
		return Authority{}, fmt.Errorf("eventlog: missing context or database")
	}
	var (
		a         Authority
		epoch     int64
		grantedAt int64
		grant     []byte
	)
	err := db.QueryRowContext(ctx, `
		SELECT cluster_id, holder_id, epoch, db_generation, grant_digest, granted_at_unix_ms
		FROM writer_authority WHERE cluster_id = ?`, cluster[:]).
		Scan(&a.Cluster, &a.Holder, &epoch, &a.Generation, &grant, &grantedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNoAuthority
	}
	if err != nil {
		return a, err
	}
	a.Epoch = uint64(epoch)
	a.GrantedAt = time.UnixMilli(grantedAt)
	if a.Grant, err = ParseHash(grant); err != nil {
		return a, err
	}
	return a, nil
}

// Log is one node's authoritative event log.
type Log struct {
	db      *sql.DB
	cluster ID
	node    ID
	// CleanupBudget bounds the rollback confirmation. When even that cannot
	// be confirmed the physical connection is retired rather than reused
	// (docs/02-data-ipam.md "异常").
	CleanupBudget time.Duration
}

// NewLog binds a database handle to this node's identity.
func NewLog(db *sql.DB, cluster, node ID) *Log {
	return &Log{db: db, cluster: cluster, node: node, CleanupBudget: 5 * time.Second}
}

// Cluster and Node expose the identities the log writes under.
func (l *Log) Cluster() ID { return l.cluster }
func (l *Log) Node() ID    { return l.node }

// WriteTx is one transaction on the single write channel. Every statement of a
// protected write runs on it, and no network call belongs inside it.
type WriteTx struct {
	log       *Log
	conn      *sql.Conn
	epoch     uint64
	head      Position
	appended  *Event
	receipt   *receiptState
	static    *StaticOccupancy
	audit     *StaticAudit
	audited   bool
	committed bool
	dirty     bool
}

// Begin refuses protected writes until an independent role authority and
// fencing implementation can prove the node is entitled to write.
func (l *Log) Begin(ctx context.Context) (*WriteTx, error) {
	// A database row alone cannot prove the independent local role log,
	// generation, fencing and current grant required by the HA contract.
	return nil, ErrNoAuthority
}

func (t *WriteTx) readHead(ctx context.Context) error {
	var (
		headEpoch, seq int64
		hash           []byte
	)
	err := t.conn.QueryRowContext(ctx,
		"SELECT epoch, seq, CASE WHEN typeof(hash)='blob' AND length(hash)=32 THEN hash END FROM replica_head WHERE cluster_id = ?", t.log.cluster[:]).
		Scan(&headEpoch, &seq, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: missing authoritative head", ErrChainBreak)
	}
	if err != nil {
		return err
	}
	h, err := ParseHash(hash)
	if err != nil {
		return err
	}
	if uint64(headEpoch) != t.epoch {
		return fmt.Errorf("%w: the head chain is at epoch %s but authority grants %s",
			ErrNoAuthority, DecUint(uint64(headEpoch)), DecUint(t.epoch))
	}
	t.head = Position{Epoch: t.epoch, Seq: uint64(seq), Hash: h}
	return nil
}

// Head is the position this transaction started from.
func (t *WriteTx) Head() Position { return t.head }

// requireOneInserted rejects silent trigger suppression and driver row-count failures.
func requireOneInserted(result sql.Result) error {
	if result == nil {
		return errors.New("insert returned no result")
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("insert row count unavailable: %w", err)
	}
	if n != 1 {
		return fmt.Errorf("insert affected %d rows, expected 1", n)
	}
	return nil
}

// Append adds the single log entry for this transaction. It must be called
// before Commit; a rollback that never reached it consumes no sequence number.
func (t *WriteTx) Append(ctx context.Context, commitMs int64, changes ...Change) (*Event, error) {
	if t.appended != nil {
		t.dirty = true
		return nil, ErrAlreadyAppended
	}
	if t.dirty {
		return nil, sql.ErrTxDone
	}
	if t.receipt != nil && t.receipt.replay {
		t.dirty = true
		return nil, errors.New("eventlog: cannot append during receipt replay")
	}
	if t.static != nil {
		if commitMs != t.static.CreatedAtMS {
			t.dirty = true
			return nil, fmt.Errorf("eventlog: STATIC event time does not match transaction time")
		}
		if err := t.checkStaticChange(changes); err != nil {
			t.dirty = true
			return nil, err
		}
	}
	payload, err := MarshalChanges(changes...)
	if err != nil {
		t.dirty = true
		return nil, err
	}
	next := Position{Epoch: t.epoch, Seq: t.head.Seq + 1}
	event := Event{
		ClusterID: t.log.cluster,
		LeaderID:  t.log.node,
		Position:  next,
		Schema:    SchemaVersionV1,
		CommitMs:  commitMs,
		PrevHash:  t.head.Hash,
		Payload:   payload,
	}
	digest, err := event.Hash()
	if err != nil {
		t.dirty = true
		return nil, err
	}
	event.Position.Hash = digest

	insertResult, err := t.conn.ExecContext(ctx, `
		INSERT INTO replica_event
			(cluster_id, epoch, seq, leader_id, schema_version, committed_unix_ms,
			 prev_hash, hash, payload)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.log.cluster[:], int64(next.Epoch), int64(next.Seq), t.log.node[:], event.Schema,
		commitMs, event.PrevHash[:], digest[:], payload)
	if err != nil {
		t.dirty = true
		return nil, fmt.Errorf("eventlog: append event: %w", err)
	}
	if err := requireOneInserted(insertResult); err != nil {
		t.dirty = true
		return nil, fmt.Errorf("eventlog: append event: %w", err)
	}

	res, err := t.conn.ExecContext(ctx, `
		UPDATE replica_head
		SET epoch = ?, seq = ?, hash = ?, schema_version = ?, advanced_at_unix_ms = ?
		WHERE cluster_id = ? AND epoch = ? AND seq = ? AND hash = ?`,
		int64(next.Epoch), int64(next.Seq), digest[:], SchemaVersionV1, commitMs,
		t.log.cluster[:], int64(t.head.Epoch), int64(t.head.Seq), t.head.Hash[:])
	if err != nil {
		t.dirty = true
		return nil, fmt.Errorf("eventlog: advance head: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		t.dirty = true
		return nil, err
	}
	if n != 1 {
		t.dirty = true
		return nil, ErrStaleAuthority
	}
	t.appended = &event
	t.head = event.Position
	// Append only prepares an event inside the open transaction. Return a copy
	// so caller mutation cannot change the subsequently reported committed event.
	result := event
	result.Payload = append([]byte(nil), event.Payload...)
	return &result, nil
}

// Commit makes the business change and its log entry durable together. An
// ambiguous COMMIT is never retried blind: the caller must resolve it through
// the idempotency receipt on a known-clean connection.
func (t *WriteTx) Commit(ctx context.Context) error {
	if t.dirty || t.appended == nil || (t.receipt != nil && !t.receipt.inserted) || (t.static != nil && (t.receipt == nil || !t.receipt.inserted || !t.audited)) {
		return t.abort(ctx, errors.New("eventlog: transaction is incomplete or invalid"))
	}
	if _, err := t.conn.ExecContext(ctx, "COMMIT"); err != nil {
		return t.abort(ctx, fmt.Errorf("eventlog: commit result is unknown: %w", err))
	}
	t.conn.Close()
	t.dirty = false
	t.committed = true
	return nil
}

// Rollback undoes the transaction. The caller must not treat a failed
// rollback as clean; see abort.
func (t *WriteTx) Rollback(ctx context.Context) error {
	return t.abort(ctx, nil)
}

func (t *WriteTx) abort(ctx context.Context, cause error) error {
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), t.log.CleanupBudget)
	defer cancel()
	_, err := t.conn.ExecContext(rctx, "ROLLBACK")
	if err != nil {
		_ = t.conn.Raw(func(any) error { return driver.ErrBadConn })
	}
	t.close()
	if err != nil {
		return errors.Join(
			fmt.Errorf("eventlog: rollback could not be confirmed; the physical connection was retired and this write channel must be rechecked"),
			err, cause)
	}
	return cause
}

func (t *WriteTx) close() {
	_ = t.conn.Close()
}

// Committed returns a copy only after COMMIT has succeeded. A rollback or an
// unknown commit outcome never exposes an event as confirmed committed.
func (t *WriteTx) Committed() *Event {
	if !t.committed || t.appended == nil {
		return nil
	}
	event := *t.appended
	event.Payload = append([]byte(nil), event.Payload...)
	return &event
}

// Head reports this node's own applied position. It is telemetry, never a
// promotion proof.
func (l *Log) Head(ctx context.Context) (Position, error) {
	if ctx == nil || l == nil || l.db == nil {
		return Position{}, fmt.Errorf("eventlog: missing context, log, or database")
	}
	var (
		epoch, seq int64
		hash       []byte
	)
	err := l.db.QueryRowContext(ctx,
		"SELECT epoch, seq, CASE WHEN typeof(hash)='blob' AND length(hash)=32 THEN hash END FROM replica_head WHERE cluster_id = ?", l.cluster[:]).
		Scan(&epoch, &seq, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return Position{}, fmt.Errorf("%w: missing authoritative head", ErrChainBreak)
	}
	if err != nil {
		return Position{}, err
	}
	h, err := ParseHash(hash)
	if err != nil {
		return Position{}, err
	}
	return Position{Epoch: uint64(epoch), Seq: uint64(seq), Hash: h}, nil
}

// MaxBatchEvents is the transport bound for one Pull response (docs/04:46).
const MaxBatchEvents = 256

// MaxBatchPayloadBytes bounds payload bytes only; protobuf envelope and framing
// are additional and must be budgeted separately by the transport.
const MaxBatchPayloadBytes = 8 << 20

// ErrPayloadBudgetTooSmall indicates that the first event cannot fit without
// skipping it. The error reports the payload bytes needed.
var ErrPayloadBudgetTooSmall = errors.New("eventlog: payload budget too small")

// Read returns a bounded batch using the default payload budget.
func (l *Log) Read(ctx context.Context, after Position, max int) ([]Event, error) {
	return l.ReadWithPayloadBudget(ctx, after, max, MaxBatchPayloadBytes)
}

// ReadWithPayloadBudget limits the sum of payload bytes, not the serialized
// protobuf response. Callers must enforce the wire budget independently.
func (l *Log) ReadWithPayloadBudget(ctx context.Context, after Position, max, budget int) ([]Event, error) {
	return l.readWithPayloadBudget(ctx, after, max, budget, nil, nil)
}

// ReadSnapshot returns a bounded contiguous candidate prefix and the head seen
// in the same SQLite read transaction. The observed head is telemetry only:
// it does not prove authority, peer identity, consent, or continuity between a
// partial prefix and that head. Callers must independently authorize the role
// and enforce the complete protobuf wire budget. No partial result is returned
// on error. A successful empty prefix means the anchor equals the observed head.
func (l *Log) ReadSnapshot(ctx context.Context, after Position, max int) ([]Event, Position, error) {
	return l.readSnapshot(ctx, after, max, nil)
}

// ReadSnapshotTx reads using the caller's existing SQLite transaction, without
// beginning, committing, or rolling it back. The caller must establish and own
// a consistent transaction on the correct database and generation, with
// PRAGMA query_only=ON, no attached databases and no TEMP objects. The caller
// must exclusively control that connection and cannot alter its PRAGMAs during
// this operation; SQL ReadOnly transaction options alone do not enforce this. The passed
// tx selects the database; it is not checked against l.db. The caller must bind
// database identity, generation and cluster independently. The head is telemetry
// from that SQLite view only; it grants no authority and does not prove
// continuity to an unknown tail. No partial results are returned on error.
func (l *Log) ReadSnapshotTx(ctx context.Context, tx *sql.Tx, after Position, max int) (events []Event, head Position, err error) {
	if ctx == nil {
		return nil, Position{}, fmt.Errorf("eventlog: missing context")
	}
	defer func() {
		if canceled := ctx.Err(); canceled != nil {
			events, head = nil, Position{}
			if err == nil {
				err = canceled
			} else {
				err = errors.Join(err, canceled)
			}
		}
	}()
	if l == nil || tx == nil {
		return nil, Position{}, fmt.Errorf("eventlog: missing log or transaction")
	}
	if err := validateReadArgs(after, max, MaxBatchPayloadBytes); err != nil {
		return nil, Position{}, err
	}
	var readOnly int
	if err := tx.QueryRowContext(ctx, `PRAGMA query_only`).Scan(&readOnly); err != nil {
		return nil, Position{}, err
	}
	if readOnly != 1 {
		return nil, Position{}, fmt.Errorf("eventlog: caller transaction requires PRAGMA query_only=ON")
	}
	rows, err := tx.QueryContext(ctx, `PRAGMA database_list`)
	if err != nil {
		return nil, Position{}, err
	}
	for rows.Next() {
		var seq int
		var name, filename string
		if err := rows.Scan(&seq, &name, &filename); err != nil {
			rows.Close()
			return nil, Position{}, err
		}
		if name != "main" && name != "temp" {
			rows.Close()
			return nil, Position{}, fmt.Errorf("eventlog: caller transaction must not have attached databases")
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, Position{}, err
	}
	if err := rows.Close(); err != nil {
		return nil, Position{}, err
	}
	var tempObjects int
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM temp.sqlite_schema)`).Scan(&tempObjects); err != nil {
		return nil, Position{}, err
	}
	if tempObjects != 0 {
		return nil, Position{}, fmt.Errorf("eventlog: caller transaction must not have TEMP objects")
	}
	events, err = l.readWithPayloadBudgetTx(ctx, tx, after, max, MaxBatchPayloadBytes, &head, nil)
	if err != nil {
		return nil, Position{}, err
	}
	return events, head, nil
}

// afterHead is a private deterministic synchronization hook for isolation tests.
func (l *Log) readSnapshot(ctx context.Context, after Position, max int, afterHead func()) ([]Event, Position, error) {
	var head Position
	events, err := l.readWithPayloadBudget(ctx, after, max, MaxBatchPayloadBytes, &head, afterHead)
	if err != nil {
		return nil, Position{}, err
	}
	return events, head, nil
}

func validateReadArgs(after Position, max, budget int) error {
	if max <= 0 || max > MaxBatchEvents {
		return fmt.Errorf("eventlog: batch size %d is outside 1..%d", max, MaxBatchEvents)
	}
	if budget <= 0 || budget > MaxBatchPayloadBytes {
		return fmt.Errorf("eventlog: payload budget %d is outside 1..%d", budget, MaxBatchPayloadBytes)
	}
	if after.Epoch == 0 || after.Epoch > math.MaxInt64 || after.Seq > math.MaxInt64 {
		return fmt.Errorf("%w: invalid anchor", ErrChainBreak)
	}
	return nil
}

func (l *Log) readWithPayloadBudget(ctx context.Context, after Position, max, budget int, observedHead *Position, afterHead func()) (events []Event, err error) {
	if ctx == nil || l == nil || l.db == nil {
		return nil, fmt.Errorf("eventlog: missing context, log, or database")
	}
	if err := validateReadArgs(after, max, budget); err != nil {
		return nil, err
	}
	conn, err := l.db.Conn(ctx)
	if err != nil {
		if canceled := ctx.Err(); canceled != nil {
			err = errors.Join(err, canceled)
		}
		return nil, err
	}
	defer conn.Close()
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		// A driver may have started a transaction before reporting failure;
		// without a Tx handle rollback cannot be confirmed.
		err = errors.Join(err, retireReadConnection(conn))
		if canceled := ctx.Err(); canceled != nil {
			err = errors.Join(err, canceled)
		}
		return nil, err
	}
	var head Position
	var headTarget *Position
	if observedHead != nil {
		headTarget = &head
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			// A failed or uncertain rollback, including ErrTxDone after context
			// cancellation, must not put this physical connection back in use.
			retireErr := retireReadConnection(conn)
			events = nil
			err = errors.Join(err, fmt.Errorf("eventlog: read transaction rollback: %w", rollbackErr), retireErr)
		}
		if canceled := ctx.Err(); canceled != nil {
			events = nil
			err = errors.Join(err, canceled)
		}
		if observedHead != nil {
			if err != nil {
				*observedHead = Position{}
			} else {
				*observedHead = head
			}
		}
	}()
	return l.readWithPayloadBudgetTx(ctx, tx, after, max, budget, headTarget, afterHead)
}

// A failure other than the requested ErrBadConn means disposal was not
// confirmed; callers must treat the pool as potentially contaminated.
func retireReadConnection(conn *sql.Conn) error {
	err := conn.Raw(func(any) error { return driver.ErrBadConn })
	if err != nil && !errors.Is(err, driver.ErrBadConn) {
		return fmt.Errorf("eventlog: connection retirement failed; pool may be contaminated: %w", err)
	}
	return nil
}

func (l *Log) readWithPayloadBudgetTx(ctx context.Context, tx *sql.Tx, after Position, max, budget int, observedHead *Position, afterHead func()) ([]Event, error) {
	if err := validateReadArgs(after, max, budget); err != nil {
		return nil, err
	}
	var err error
	var headEpoch, headSeq, headSchema, headTime int64
	var headHash []byte
	if observedHead == nil {
		err = tx.QueryRowContext(ctx, `SELECT epoch, seq, CASE WHEN typeof(hash)='blob' AND length(hash)=32 THEN hash END FROM main.replica_head WHERE cluster_id = ?`, l.cluster[:]).Scan(&headEpoch, &headSeq, &headHash)
	} else {
		err = tx.QueryRowContext(ctx, `SELECT CASE WHEN typeof(epoch)='integer' THEN epoch END, CASE WHEN typeof(seq)='integer' THEN seq END, CASE WHEN typeof(hash)='blob' AND length(hash)=32 THEN hash END, CASE WHEN typeof(schema_version)='integer' THEN schema_version END, CASE WHEN typeof(advanced_at_unix_ms)='integer' THEN advanced_at_unix_ms END FROM main.replica_head WHERE cluster_id = ?`, l.cluster[:]).Scan(&headEpoch, &headSeq, &headHash, &headSchema, &headTime)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: missing authoritative head", ErrChainBreak)
	}
	if err != nil {
		return nil, err
	}
	if headEpoch <= 0 || headSeq < 0 || uint64(headEpoch) != after.Epoch || after.Seq > uint64(headSeq) {
		return nil, fmt.Errorf("%w: anchor outside current head", ErrChainBreak)
	}
	var observed Position
	var leader ID
	leaderKnown := false
	checkLeader := func(event Event) error {
		if leaderKnown && leader != event.LeaderID {
			return ErrIdentityMismatch
		}
		leader, leaderKnown = event.LeaderID, true
		return nil
	}
	if observedHead != nil {
		hash, err := ParseHash(headHash)
		if err != nil || headSchema != int64(SchemaVersionV1) || headTime < 0 {
			return nil, fmt.Errorf("%w: invalid observed head", ErrChainBreak)
		}
		observed = Position{Epoch: uint64(headEpoch), Seq: uint64(headSeq), Hash: hash}
		if afterHead != nil {
			afterHead()
		}
		if headSeq == 0 {
			if headEpoch != 1 || hash != GenesisHash() {
				return nil, fmt.Errorf("%w: unverified observed genesis", ErrChainBreak)
			}
		} else {
			last, size, err := l.readEvent(ctx, tx, headEpoch, headSeq, MaxPayloadBytes)
			if err != nil {
				return nil, wrapReadEventError("invalid observed head event", err)
			}
			if size > MaxPayloadBytes {
				return nil, fmt.Errorf("%w: observed head payload is %d bytes", ErrPayloadSize, size)
			}
			if last.Position != observed || int64(last.Schema) != headSchema || last.CommitMs != headTime {
				return nil, fmt.Errorf("%w: observed head event mismatch", ErrChainBreak)
			}
			if err := last.Verify(); err != nil {
				return nil, fmt.Errorf("%w: invalid observed head event: %v", ErrChainBreak, err)
			}
			if err := checkLeader(last); err != nil {
				return nil, err
			}
		}
	}
	if after.Seq == 0 {
		// Only the genesis anchor is independently known. Later epochs need
		// a verified handover anchor, which this store does not yet possess.
		if after.Epoch != 1 || after.Hash != GenesisHash() {
			return nil, fmt.Errorf("%w: unverified anchor", ErrChainBreak)
		}
	} else {
		anchor, size, err := l.readEvent(ctx, tx, headEpoch, int64(after.Seq), MaxPayloadBytes)
		if err != nil {
			return nil, wrapReadEventError("invalid anchor", err)
		}
		if size > MaxPayloadBytes {
			return nil, fmt.Errorf("%w: anchor payload is %d bytes", ErrPayloadSize, size)
		}
		if anchor.Position.Hash != after.Hash {
			return nil, fmt.Errorf("%w: anchor hash mismatch", ErrChainBreak)
		}
		if err := anchor.Verify(); err != nil {
			return nil, fmt.Errorf("eventlog: corrupt anchor: %w", err)
		}
		if err := checkLeader(anchor); err != nil {
			return nil, err
		}
	}
	if after.Seq == uint64(headSeq) {
		h, err := ParseHash(headHash)
		if err != nil || h != after.Hash {
			return nil, fmt.Errorf("%w: head hash mismatch", ErrChainBreak)
		}
	}
	var out []Event
	prev := after.Hash
	remaining := budget
	for len(out) < max && after.Seq+uint64(len(out)) < uint64(headSeq) {
		seq := int64(after.Seq + uint64(len(out)) + 1)
		event, size, err := l.readEvent(ctx, tx, headEpoch, seq, remaining)
		if err != nil {
			return nil, wrapReadEventError(fmt.Sprintf("event %d", seq), err)
		}
		if size > MaxPayloadBytes {
			return nil, fmt.Errorf("%w: event %d payload is %d bytes", ErrPayloadSize, seq, size)
		}
		if size > remaining {
			if len(out) == 0 {
				return nil, fmt.Errorf("%w: need %d payload bytes", ErrPayloadBudgetTooSmall, size)
			}
			break
		}
		if event.PrevHash != prev {
			return nil, fmt.Errorf("%w: event %d has the wrong predecessor", ErrChainBreak, seq)
		}
		if err := event.Verify(); err != nil {
			return nil, fmt.Errorf("eventlog: corrupt event: %w", err)
		}
		if err := checkLeader(event); err != nil {
			return nil, err
		}
		if seq == headSeq {
			head, err := ParseHash(headHash)
			if err != nil || head != event.Position.Hash {
				return nil, fmt.Errorf("%w: head hash mismatch", ErrChainBreak)
			}
		}
		out = append(out, event)
		remaining -= size
		prev = event.Position.Hash
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if observedHead != nil {
		*observedHead = observed
	}
	return out, nil
}

// wrapReadEventError preserves compatibility with ErrChainBreak and the
// original storage error. Neither classification proves peer divergence: a
// caller must distinguish database faults and cancellation before any policy.
func wrapReadEventError(where string, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return fmt.Errorf("%w: %s: %w", ErrChainBreak, where, err)
}

// readEvent asks SQLite to omit the payload before it crosses the driver
// boundary if it exceeds either the remaining budget or the per-event limit.
func (l *Log) readEvent(ctx context.Context, tx *sql.Tx, epoch, seq int64, budget int) (Event, int, error) {
	var event Event
	var leader, prev, hash, payload []byte
	var storedEpoch, storedSeq int64
	var size int64
	err := tx.QueryRowContext(ctx, `SELECT epoch, seq,
		CASE WHEN typeof(leader_id)='blob' AND length(leader_id)=16 THEN leader_id END,
		schema_version, committed_unix_ms,
		CASE WHEN typeof(prev_hash)='blob' AND length(prev_hash)=32 THEN prev_hash END,
		CASE WHEN typeof(hash)='blob' AND length(hash)=32 THEN hash END,
		length(payload), CASE WHEN typeof(payload)='blob' AND length(payload) <= ? AND length(payload) <= ? THEN payload END
		FROM main.replica_event WHERE cluster_id=? AND epoch=? AND seq=?`,
		budget, MaxPayloadBytes, l.cluster[:], epoch, seq).Scan(&storedEpoch, &storedSeq, &leader, &event.Schema, &event.CommitMs, &prev, &hash, &size, &payload)
	if err != nil {
		return Event{}, 0, err
	}
	if storedEpoch != epoch || storedSeq != seq || size < 0 || size > int64(math.MaxInt) {
		return Event{}, 0, ErrChainBreak
	}
	event.ClusterID = l.cluster
	if event.LeaderID, err = IDFromBytes(leader); err != nil {
		return Event{}, 0, err
	}
	if event.PrevHash, err = ParseHash(prev); err != nil {
		return Event{}, 0, err
	}
	if event.Position.Hash, err = ParseHash(hash); err != nil {
		return Event{}, 0, err
	}
	event.Position.Epoch, event.Position.Seq = uint64(epoch), uint64(seq)
	event.Payload = payload
	return event, int(size), nil
}

// RecordApplied refuses unverified peer confirmations. A future implementation
// must authenticate the configured peer and validate its position and hash.
func (l *Log) RecordApplied(ctx context.Context, source ID, pos Position, at time.Time) error {
	// The authenticated configured peer identity is not available here. Until
	// it is, no confirmation can be accepted as a trusted watermark.
	return ErrNoAuthority
}

// Applied reports the last confirmed position of one peer.
func (l *Log) Applied(ctx context.Context, source ID) (Position, error) {
	if ctx == nil || l == nil || l.db == nil {
		return Position{}, fmt.Errorf("eventlog: missing context, log, or database")
	}
	var (
		epoch, seq int64
		hash       []byte
	)
	err := l.db.QueryRowContext(ctx,
		"SELECT epoch, seq, hash FROM replica_checkpoint WHERE cluster_id = ? AND source_id = ?",
		l.cluster[:], source[:]).Scan(&epoch, &seq, &hash)
	if err != nil {
		return Position{}, err
	}
	h, err := ParseHash(hash)
	if err != nil {
		return Position{}, err
	}
	return Position{Epoch: uint64(epoch), Seq: uint64(seq), Hash: h}, nil
}
