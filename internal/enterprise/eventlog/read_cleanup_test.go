package eventlog

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"sync/atomic"
	"testing"

	"modernc.org/sqlite"
)

var errSimulatedRollback = errors.New("simulated rollback report failure")
var errSimulatedBegin = errors.New("simulated driver interrupted")

type rollbackConnector struct {
	path           string
	opens, closes  atomic.Int32
	fail           atomic.Bool
	beginCancel    context.CancelFunc
	failAfterBegin atomic.Bool
}

func (c *rollbackConnector) Connect(ctx context.Context) (driver.Conn, error) {
	raw, err := (&sqlite.Driver{}).Open(c.path)
	if err != nil {
		return nil, err
	}
	c.opens.Add(1)
	return &rollbackConn{Conn: raw, owner: c}, nil
}
func (c *rollbackConnector) Driver() driver.Driver { return &sqlite.Driver{} }

type rollbackConn struct {
	driver.Conn
	owner *rollbackConnector
}

func (c *rollbackConn) Close() error { c.owner.closes.Add(1); return c.Conn.Close() }
func (c *rollbackConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if c.owner.beginCancel != nil {
		c.owner.beginCancel()
		return nil, errSimulatedBegin
	}
	tx, err := c.Conn.(driver.ConnBeginTx).BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	if c.owner.failAfterBegin.Swap(false) {
		// The driver really started the transaction, then reports failure and
		// withholds its handle. Closing the physical connection must clean it.
		return nil, errSimulatedBegin
	}
	return &rollbackTx{Tx: tx, owner: c.owner}, nil
}
func (c *rollbackConn) QueryContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, q, args)
}
func (c *rollbackConn) ExecContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Result, error) {
	return c.Conn.(driver.ExecerContext).ExecContext(ctx, q, args)
}
func (c *rollbackConn) CheckNamedValue(v *driver.NamedValue) error {
	if n, ok := c.Conn.(driver.NamedValueChecker); ok {
		return n.CheckNamedValue(v)
	}
	return driver.ErrSkip
}

type rollbackTx struct {
	driver.Tx
	owner *rollbackConnector
}

func (t *rollbackTx) Rollback() error {
	err := t.Tx.Rollback()
	if err != nil {
		return err
	}
	if t.owner.fail.Swap(false) {
		return errSimulatedRollback
	}
	return nil
}

func TestRetireClosedConnectionPreservesError(t *testing.T) {
	db, _ := logFixture(t)
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err = conn.Close(); err != nil {
		t.Fatal(err)
	}
	if err = retireReadConnection(conn); !errors.Is(err, sql.ErrConnDone) {
		t.Fatalf("retirement error lost: %v", err)
	}
}

func TestReadBeginDriverErrorAndCancellation(t *testing.T) {
	writer, l := logFixture(t)
	var seq int
	var name, filename string
	if err := writer.QueryRow(`PRAGMA database_list`).Scan(&seq, &name, &filename); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	connector := &rollbackConnector{path: filename, beginCancel: cancel}
	db := sql.OpenDB(connector)
	defer db.Close()
	reader := NewLog(db, l.cluster, l.node)
	events, head, err := reader.ReadSnapshot(ctx, Position{Epoch: 1}, 1)
	if !errors.Is(err, errSimulatedBegin) || !errors.Is(err, context.Canceled) || len(events) != 0 || head != (Position{}) {
		t.Fatalf("events=%v head=%+v err=%v", events, head, err)
	}
}

func TestReadBeginFailureAfterNativeBeginRetiresConnection(t *testing.T) {
	writer, l := logFixture(t)
	first := seedEvent(t, writer, l, 1, GenesisHash())
	var seq int
	var name, filename string
	if err := writer.QueryRow(`PRAGMA database_list`).Scan(&seq, &name, &filename); err != nil {
		t.Fatal(err)
	}
	connector := &rollbackConnector{path: filename}
	connector.failAfterBegin.Store(true)
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(1)
	defer db.Close()
	reader := NewLog(db, l.cluster, l.node)
	events, head, err := reader.ReadSnapshot(context.Background(), Position{Epoch: 1}, 1)
	if !errors.Is(err, errSimulatedBegin) || len(events) != 0 || head != (Position{}) || connector.closes.Load() != 1 {
		t.Fatalf("events=%v head=%+v err=%v closed=%d", events, head, err, connector.closes.Load())
	}
	events, head, err = reader.ReadSnapshot(context.Background(), Position{Epoch: 1}, 1)
	if err != nil || len(events) != 1 || head != first.Position || connector.opens.Load() != 2 {
		t.Fatalf("retry events=%v head=%+v err=%v opens=%d", events, head, err, connector.opens.Load())
	}
}

func TestLegacyReadRejectsNilInputs(t *testing.T) {
	db, l := logFixture(t)
	_ = db
	for _, log := range []*Log{nil, {}, l} {
		if _, err := log.Read(nil, Position{Epoch: 1}, 1); err == nil {
			t.Fatal("nil context accepted")
		}
		if _, _, err := log.ReadSnapshot(nil, Position{Epoch: 1}, 1); err == nil {
			t.Fatal("nil context accepted")
		}
	}
	var missing *Log
	if _, err := missing.Read(context.Background(), Position{Epoch: 1}, 1); err == nil {
		t.Fatal("nil log accepted")
	}
	if _, _, err := missing.ReadSnapshot(context.Background(), Position{Epoch: 1}, 1); err == nil {
		t.Fatal("nil log accepted")
	}
	if _, err := (&Log{}).Read(context.Background(), Position{Epoch: 1}, 1); err == nil {
		t.Fatal("nil database accepted")
	}
}

func TestReadSnapshotRollbackFailureRetiresConnection(t *testing.T) {
	writer, l := logFixture(t)
	first := seedEvent(t, writer, l, 1, GenesisHash())
	var seq int
	var name, filename string
	if err := writer.QueryRow(`PRAGMA database_list`).Scan(&seq, &name, &filename); err != nil {
		t.Fatal(err)
	}
	connector := &rollbackConnector{path: filename}
	connector.fail.Store(true)
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(1)
	defer db.Close()
	reader := NewLog(db, l.cluster, l.node)
	events, head, err := reader.ReadSnapshot(context.Background(), Position{Epoch: 1}, 1)
	if !errors.Is(err, errSimulatedRollback) || len(events) != 0 || head != (Position{}) {
		t.Fatalf("events=%v head=%+v err=%v", events, head, err)
	}
	if connector.closes.Load() != 1 {
		t.Fatalf("physical connection not closed: %d", connector.closes.Load())
	}
	events, head, err = reader.ReadSnapshot(context.Background(), Position{Epoch: 1}, 1)
	if err != nil || len(events) != 1 || head != first.Position || connector.opens.Load() != 2 {
		t.Fatalf("retry %v %+v %v opened=%d", events, head, err, connector.opens.Load())
	}
	connector.fail.Store(true)
	bad := first.Position
	bad.Hash[0] ^= 1
	events, head, err = reader.ReadSnapshot(context.Background(), bad, 1)
	if !errors.Is(err, ErrChainBreak) || !errors.Is(err, errSimulatedRollback) || len(events) != 0 || head != (Position{}) {
		t.Fatalf("joined causes: %v %+v %v", events, head, err)
	}
}
