package eventlogguard

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"
	"time"
)

var errSimulatedInterrupt = errors.New("simulated sqlite interrupted")

type interruptConnector struct{ started chan struct{} }

func (c *interruptConnector) Connect(context.Context) (driver.Conn, error) {
	return &interruptConn{started: c.started}, nil
}
func (c *interruptConnector) Driver() driver.Driver { return interruptDriver{} }

type interruptDriver struct{}

func (interruptDriver) Open(string) (driver.Conn, error) { return nil, errors.New("unused") }

type interruptConn struct{ started chan struct{} }

func (*interruptConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("unused") }
func (*interruptConn) Close() error                        { return nil }
func (*interruptConn) Begin() (driver.Tx, error)           { return nil, errors.New("unused") }
func (c *interruptConn) BeginTx(ctx context.Context, _ driver.TxOptions) (driver.Tx, error) {
	close(c.started)
	<-ctx.Done()
	return nil, errSimulatedInterrupt
}
func TestInspectBeginInterruptionReturnsUnknown(t *testing.T) {
	connector := &interruptConnector{started: make(chan struct{})}
	db := sql.OpenDB(connector)
	defer db.Close()
	c, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type result struct {
		v   Version
		err error
	}
	out := make(chan result, 1)
	go func() { v, e := Inspect(ctx, c); out <- result{v, e} }()
	select {
	case <-connector.started:
	case <-time.After(5 * time.Second):
		t.Fatal("BEGIN not reached")
	}
	cancel()
	select {
	case r := <-out:
		if r.v != Unknown || !errors.Is(r.err, ErrInvalid) || !errors.Is(r.err, context.Canceled) || !errors.Is(r.err, errSimulatedInterrupt) {
			t.Fatalf("version=%v error=%v", r.v, r.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("BEGIN did not return")
	}
}

func TestVerifyBeginInterruptionPreservesDriverAndContext(t *testing.T) {
	for _, kind := range []string{"cancel", "deadline"} {
		t.Run(kind, func(t *testing.T) {
			connector := &interruptConnector{started: make(chan struct{})}
			db := sql.OpenDB(connector)
			defer db.Close()
			c, err := db.Conn(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			var ctx context.Context
			var cancel context.CancelFunc
			if kind == "cancel" {
				ctx, cancel = context.WithCancel(context.Background())
			} else {
				ctx, cancel = context.WithTimeout(context.Background(), time.Second)
			}
			defer cancel()
			out := make(chan error, 1)
			go func() { out <- Verify(ctx, c) }()
			select {
			case <-connector.started:
			case <-time.After(5 * time.Second):
				t.Fatal("BEGIN not reached")
			}
			if kind == "cancel" {
				cancel()
			}
			select {
			case err = <-out:
			case <-time.After(5 * time.Second):
				t.Fatal("BEGIN did not return")
			}
			want := context.Canceled
			if kind == "deadline" {
				want = context.DeadlineExceeded
			}
			if !errors.Is(err, ErrInvalid) || !errors.Is(err, want) || !errors.Is(err, errSimulatedInterrupt) {
				t.Fatalf("error chains lost: %v", err)
			}
		})
	}
}
