package server

import (
	"net"
	"testing"
	"time"
)

func TestRequestAdmissionDropsWhenQueueIsFull(t *testing.T) {
	store := newLeaseStore(t)
	s := New(store.DB, []string{"eth0"}, nil)
	s.SetRequestAdmission(1, 1)

	msg := discoverMsg(t, testMAC(30))
	req := dhcpRequest{
		msg:       msg,
		addr:      &net.UDPAddr{IP: net.ParseIP("192.0.2.50"), Port: 68},
		conn:      nil,
		ifaceName: "eth0",
		serverIP:  net.ParseIP(testServerIP),
	}
	s.admitRequest(req, "eth0")
	s.admitRequest(req, "eth0")

	if got := len(s.requestQueue); got != 1 {
		t.Fatalf("queue depth = %d, want 1 after saturated admission", got)
	}
	if got := cap(s.requestQueue); got != 1 {
		t.Fatalf("queue capacity = %d, want 1", got)
	}
}

func TestRequestAdmissionWorkersExitOnShutdown(t *testing.T) {
	store := newLeaseStore(t)
	s := New(store.DB, []string{"eth0"}, nil)
	s.SetRequestAdmission(2, 2)
	s.startWorkers()
	close(s.quit)
	waitForWorkers(t, &s.workerWG)
}

func waitForWorkers(t *testing.T, wg interface{ Wait() }) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("workers did not exit after shutdown")
	}
}
