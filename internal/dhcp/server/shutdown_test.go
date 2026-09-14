package server

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestShutdownReturnsContextErrorWhenWorkerDoesNotDrain(t *testing.T) {
	store := newLeaseStore(t)
	s := New(store.DB, []string{"eth0"}, nil)
	s.SetRequestAdmission(1, 1)

	started := make(chan struct{})
	release := make(chan struct{})
	s.requestHandler = func(dhcpRequest) {
		close(started)
		<-release
	}
	s.startWorkers()
	s.admitRequest(dhcpRequest{
		msg:       discoverMsg(t, testMAC(31)),
		addr:      &net.UDPAddr{IP: net.ParseIP("192.0.2.51"), Port: 68},
		ifaceName: "eth0",
		serverIP:  net.ParseIP(testServerIP),
	}, "eth0")
	<-started

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := s.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown error = %v, want context deadline exceeded", err)
	}

	close(release)
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown after worker drain = %v", err)
	}
}

func TestAdmitRequestDropsAfterShutdownBegins(t *testing.T) {
	store := newLeaseStore(t)
	s := New(store.DB, []string{"eth0"}, nil)
	close(s.quit)

	s.admitRequest(dhcpRequest{
		msg:       discoverMsg(t, testMAC(32)),
		addr:      &net.UDPAddr{IP: net.ParseIP("192.0.2.52"), Port: 68},
		ifaceName: "eth0",
		serverIP:  net.ParseIP(testServerIP),
	}, "eth0")
	if got := len(s.requestQueue); got != 0 {
		t.Fatalf("queue depth after shutdown = %d, want 0", got)
	}
}
