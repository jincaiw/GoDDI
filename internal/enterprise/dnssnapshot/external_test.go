package dnssnapshot_test

import (
	"crypto/sha256"
	"errors"
	"sync"
	"testing"

	d "github.com/jincaiw/GoDDI/internal/enterprise/dnssnapshot"
)

func setup(t *testing.T) (*d.Store, *d.Snapshot, d.Identity) {
	t.Helper()
	b := []byte("safe")
	id := d.Identity{Version: 1, Digest: sha256.Sum256(b)}
	s, err := d.New(id, b, 100)
	if err != nil {
		t.Fatal(err)
	}
	st := d.NewStore(id.Cluster)
	if err = st.Request(id); err != nil {
		t.Fatal(err)
	}
	if err = st.Publish(s); err != nil {
		t.Fatal(err)
	}
	return st, s, id
}

func TestCallerCannotOverwriteActiveSnapshot(t *testing.T) {
	st, input, id := setup(t)
	token, err := st.RequestOpen(id)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.Open(token); err != nil {
		t.Fatal(err)
	}
	*input = d.Snapshot{}
	got, err := st.Load()
	if err != nil || got.Identity() != id || string(got.Bytes()) != "safe" {
		t.Fatalf("input overwrite: %v %v", got.Identity(), err)
	}
	got = d.Snapshot{}
	next, err := st.Load()
	if err != nil || next.Identity() != id || string(next.Bytes()) != "safe" {
		t.Fatalf("output overwrite: %v %v", next.Identity(), err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			*input = d.Snapshot{}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			x, e := st.Load()
			if e != nil || x.Identity() != id || string(x.Bytes()) != "safe" {
				t.Error("active snapshot changed")
				return
			}
			x = d.Snapshot{}
		}
	}()
	wg.Wait()
}

func TestOpenTokenInvalidation(t *testing.T) {
	st, _, id := setup(t)
	old, err := st.RequestOpen(id)
	if err != nil {
		t.Fatal(err)
	}
	st.Close()
	if err = st.Open(old); !errors.Is(err, d.ErrNotRequested) {
		t.Fatalf("old token: %v", err)
	}
	if _, err = st.Load(); !errors.Is(err, d.ErrClosed) {
		t.Fatal(err)
	}
	fresh, err := st.RequestOpen(id)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.Open(fresh); err != nil {
		t.Fatal(err)
	}
	if _, err = st.Load(); err != nil {
		t.Fatal(err)
	}
	other, _, _ := setup(t)
	if err = other.Open(fresh); !errors.Is(err, d.ErrNotRequested) {
		t.Fatalf("cross-store token: %v", err)
	}
}

func TestConcurrentCloseOpenToken(t *testing.T) {
	for i := 0; i < 100; i++ {
		st, _, id := setup(t)
		token, err := st.RequestOpen(id)
		if err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		var openErr error
		go func() { defer wg.Done(); <-start; openErr = st.Open(token) }()
		go func() { defer wg.Done(); <-start; st.Close() }()
		close(start)
		wg.Wait()
		if openErr != nil && !errors.Is(openErr, d.ErrNotRequested) {
			t.Fatal(openErr)
		}
		if _, err = st.Load(); !errors.Is(err, d.ErrClosed) {
			t.Fatalf("gate not closed: %v", err)
		}
		if err = st.Open(token); !errors.Is(err, d.ErrNotRequested) {
			t.Fatalf("replayed token: %v", err)
		}
	}
}
