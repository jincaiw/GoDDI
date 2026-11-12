package eventlogguard

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"sync/atomic"
	"testing"

	"modernc.org/sqlite"
)

var errSimulatedRollback = errors.New("simulated rollback reporting error")

type rollbackConnector struct {
	path           string
	opened, closed *atomic.Int64
}

func (c rollbackConnector) Connect(context.Context) (driver.Conn, error) {
	base, err := (&sqlite.Driver{}).Open(c.path)
	if err != nil {
		return nil, err
	}
	c.opened.Add(1)
	return &rollbackConn{Conn: base, closed: c.closed}, nil
}
func (rollbackConnector) Driver() driver.Driver { return &sqlite.Driver{} }

type rollbackConn struct {
	driver.Conn
	closed *atomic.Int64
}

func (c *rollbackConn) Close() error { c.closed.Add(1); return c.Conn.Close() }

func (c *rollbackConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	tx, err := c.Conn.(driver.ConnBeginTx).BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return rollbackTx{Tx: tx}, nil
}
func (c *rollbackConn) QueryContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, q, args)
}
func (c *rollbackConn) ExecContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Result, error) {
	return c.Conn.(driver.ExecerContext).ExecContext(ctx, q, args)
}

type rollbackTx struct{ driver.Tx }

func (t rollbackTx) Rollback() error {
	if err := t.Tx.Rollback(); err != nil {
		return err
	}
	return errSimulatedRollback
}
func TestInspectRollbackErrorReturnsUnknown(t *testing.T) {
	db := legacy(t)
	var n int
	var name, path string
	if err := db.QueryRow(`PRAGMA database_list`).Scan(&n, &name, &path); err != nil {
		t.Fatal(err)
	}
	var opened, closed atomic.Int64
	mock := sql.OpenDB(rollbackConnector{path: path, opened: &opened, closed: &closed})
	defer mock.Close()
	c, err := mock.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for _, q := range []string{`PRAGMA foreign_keys=ON`, `PRAGMA busy_timeout=5000`, `PRAGMA synchronous=FULL`, `PRAGMA query_only=ON`} {
		if _, err = c.ExecContext(context.Background(), q); err != nil {
			t.Fatal(err)
		}
	}
	v, err := Inspect(context.Background(), c)
	if v != Unknown || !errors.Is(err, ErrInvalid) || !errors.Is(err, errSimulatedRollback) {
		t.Fatalf("version=%v err=%v", v, err)
	}
	var count int
	if err = c.QueryRowContext(context.Background(), `SELECT 1`).Scan(&count); !errors.Is(err, sql.ErrConnDone) {
		t.Fatalf("old connection reused: %v", err)
	}
	if closed.Load() != 1 || opened.Load() != 1 {
		t.Fatalf("physical connection not closed opened=%d closed=%d", opened.Load(), closed.Load())
	}
	_ = c.Close()
	fresh, e := mock.Conn(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer fresh.Close()
	if opened.Load() != 2 {
		t.Fatalf("pool did not reopen: %d", opened.Load())
	}
	if e = fresh.QueryRowContext(context.Background(), `SELECT count(*) FROM main.sqlite_schema`).Scan(&count); e != nil || count == 0 {
		t.Fatalf("fresh connection query failed %v", e)
	}
}
