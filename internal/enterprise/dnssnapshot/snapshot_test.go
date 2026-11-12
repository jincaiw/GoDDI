package dnssnapshot

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"sync"
	"testing"
)

func snapshot(t *testing.T, cluster [16]byte, version uint64, content string) *Snapshot {
	t.Helper()
	data := []byte(content)
	id := Identity{Cluster: cluster, Version: version, Digest: sha256.Sum256(data)}
	id.Release[15] = byte(version)
	s, err := New(id, data, 1024)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func openForTest(t *testing.T, s *Store, id Identity) {
	t.Helper()
	token, err := s.RequestOpen(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Open(token); err != nil {
		t.Fatal(err)
	}
}

func TestImmutableAndLimits(t *testing.T) {
	data := []byte("original")
	id := Identity{Version: 1, Digest: sha256.Sum256(data)}
	s, err := New(id, data, len(data))
	if err != nil {
		t.Fatal(err)
	}
	data[0] = 'X'
	got := s.Bytes()
	if string(got) != "original" {
		t.Fatal(string(got))
	}
	got[0] = 'Y'
	if string(s.Bytes()) != "original" {
		t.Fatal("accessor exposed bytes")
	}
	if _, err = New(id, []byte("different"), 100); !errors.Is(err, ErrDigest) {
		t.Fatalf("digest: %v", err)
	}
	if _, err = New(id, []byte("original"), 7); !errors.Is(err, ErrInvalid) {
		t.Fatalf("limit: %v", err)
	}
	if _, err = New(id, nil, 1); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty: %v", err)
	}
}

func TestPublicationGateAndVersions(t *testing.T) {
	cluster := [16]byte{1}
	st := NewStore(cluster)
	a := snapshot(t, cluster, 1, "one")
	b := snapshot(t, cluster, 2, "two")
	conflict := snapshot(t, cluster, 2, "other")
	if _, err := st.Load(); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	if err := st.Publish(a); !errors.Is(err, ErrNotRequested) {
		t.Fatal(err)
	}
	if err := st.Request(a.Identity()); err != nil {
		t.Fatal(err)
	}
	if err := st.Publish(a); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Load(); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	openForTest(t, st, a.Identity())
	pinned, err := st.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err = st.Request(b.Identity()); err != nil {
		t.Fatal(err)
	}
	if err = st.Request(conflict.Identity()); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if err = st.Publish(b); err != nil {
		t.Fatal(err)
	}
	current, err := st.Load()
	if err != nil || current.Identity() != b.Identity() || !bytes.Equal(pinned.Bytes(), []byte("one")) {
		t.Fatalf("current/pinned: %v", err)
	}
	if err = st.Publish(a); !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
	if err = st.Publish(conflict); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if err = st.Publish(b); err != nil {
		t.Fatal(err)
	}
	st.Close()
	if _, err = st.Load(); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	if _, err = st.RequestOpen(a.Identity()); !errors.Is(err, ErrNotRequested) {
		t.Fatal(err)
	}
}

func TestClosedGateInvalidatesPendingAndRetainsVersion(t *testing.T) {
	cluster := [16]byte{1}
	st := NewStore(cluster)
	a := snapshot(t, cluster, 2, "one")
	conflict := snapshot(t, cluster, 2, "other")
	if err := st.Request(a.Identity()); err != nil {
		t.Fatal(err)
	}
	st.Close()
	if err := st.Publish(a); !errors.Is(err, ErrNotRequested) {
		t.Fatal(err)
	}
	if err := st.Request(conflict.Identity()); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if err := st.Request(a.Identity()); err != nil {
		t.Fatal(err)
	}
	if err := st.Publish(a); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Load(); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
}

func TestConcurrentReadersCannotMutate(t *testing.T) {
	cluster := [16]byte{3}
	st := NewStore(cluster)
	s := snapshot(t, cluster, 1, "immutable")
	if err := st.Request(s.Identity()); err != nil {
		t.Fatal(err)
	}
	if err := st.Publish(s); err != nil {
		t.Fatal(err)
	}
	openForTest(t, st, s.Identity())
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				value, err := st.Load()
				if err != nil {
					t.Error(err)
					return
				}
				copy := value.Bytes()
				copy[0] = 'X'
				if string(value.Bytes()) != "immutable" {
					t.Error("snapshot mutated")
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestConcurrentCloseAndPublish(t *testing.T) {
	for i := 0; i < 100; i++ {
		cluster := [16]byte{1}
		st := NewStore(cluster)
		a := snapshot(t, cluster, 1, "one")
		b := snapshot(t, cluster, 2, "two")
		if err := st.Request(a.Identity()); err != nil {
			t.Fatal(err)
		}
		if err := st.Publish(a); err != nil {
			t.Fatal(err)
		}
		openForTest(t, st, a.Identity())
		if err := st.Request(b.Identity()); err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		var publishErr error
		go func() { defer wg.Done(); <-start; publishErr = st.Publish(b) }()
		go func() { defer wg.Done(); <-start; st.Close() }()
		close(start)
		wg.Wait()
		if publishErr != nil && !errors.Is(publishErr, ErrNotRequested) {
			t.Fatal(publishErr)
		}
		if _, err := st.Load(); !errors.Is(err, ErrClosed) {
			t.Fatalf("gate reopened: %v", err)
		}
	}
}
