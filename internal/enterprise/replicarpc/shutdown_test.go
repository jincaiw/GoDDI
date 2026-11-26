package replicarpc

import (
	"context"
	"sync"
	"testing"
	"time"

	replica "github.com/jincaiw/GoDDI/internal/gen/replica/v1"
)

func TestStopRepeatedAndConcurrentBeforeRequests(t *testing.T) {
	sv, _ := testServer(t, nil, time.Time{})
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := sv.stop(); err != nil {
				t.Errorf("stop: %v", err)
			}
		}()
	}
	finished := make(chan struct{})
	go func() { wg.Wait(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent stops blocked")
	}
	if err := sv.stop(); err != nil {
		t.Fatal(err)
	}
}

func TestStopWithConcurrentClientsDrains(t *testing.T) {
	sv, f := testServer(t, nil, time.Time{})
	c := connect(t, sv, f, &f.client, f.ca)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_, _ = replica.NewReplicationClient(c).Pull(ctx, request())
		}()
	}
	stopped := make(chan struct{})
	go func() { sv.server.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("server stop did not return")
	}
	finished := make(chan struct{})
	go func() { wg.Wait(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("client goroutines did not finish")
	}
}
