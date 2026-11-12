package replicaguard

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"golang.org/x/sys/unix"
)

func fixture(t *testing.T) (*Store, Record, string) {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return NewStore(dir), Record{Version: "1", State: "QUARANTINED", ClusterID: "11111111-1111-4111-8111-111111111111", NodeID: "22222222-2222-4222-8222-222222222222", DBGeneration: "generation-1", Reason: "BOOTSTRAP", Epoch: 1, PositionDigest: strings.Repeat("0", 64)}, dir
}
func TestBootstrapRestartAndNoEnable(t *testing.T) {
	s, r, dir := fixture(t)
	if got, err := s.Read(); err == nil || got != (Record{}) {
		t.Fatalf("missing: %+v %v", got, err)
	}
	if err := s.Quarantine(r); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("implicit bootstrap: %v", err)
	}
	active := r
	active.State = "ACTIVE"
	if err := s.Bootstrap(active); !errors.Is(err, ErrInvalid) {
		t.Fatalf("active bootstrap: %v", err)
	}
	if err := s.Bootstrap(r); err != nil {
		t.Fatal(err)
	}
	if err := s.Bootstrap(r); !errors.Is(err, ErrExists) {
		t.Fatalf("overwrite: %v", err)
	}
	other := NewStore(dir)
	got, err := other.Read()
	if err != nil || got != r {
		t.Fatalf("restart: %+v %v", got, err)
	}
	if err = other.Quarantine(r); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Record){func(v *Record) { v.NodeID = "33333333-3333-4333-8333-333333333333" }, func(v *Record) { v.DBGeneration = "other" }, func(v *Record) { v.Reason = "DIVERGENCE" }, func(v *Record) { v.State = "ACTIVE" }} {
		v := r
		mutate(&v)
		if err := other.Quarantine(v); err == nil {
			t.Fatal("changed record accepted")
		}
	}
	got, err = other.Read()
	if err != nil || got != r {
		t.Fatalf("state changed: %+v %v", got, err)
	}
}
func TestCorruptMissingAndUnsafeFiles(t *testing.T) {
	t.Run("corrupt", func(t *testing.T) {
		s, r, dir := fixture(t)
		if err := os.WriteFile(filepath.Join(dir, recordName), []byte(`{"state":"ACTIVE"}`), 0600); err != nil {
			t.Fatal(err)
		}
		if got, err := s.Read(); err == nil || got != (Record{}) {
			t.Fatalf("corrupt: %+v %v", got, err)
		}
		if err := s.Quarantine(r); err == nil {
			t.Fatal("replaced corruption")
		}
	})
	t.Run("record symlink", func(t *testing.T) {
		s, r, dir := fixture(t)
		target := filepath.Join(t.TempDir(), "target")
		if err := os.WriteFile(target, []byte("safe"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(dir, recordName)); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Read(); err == nil {
			t.Fatal("read symlink")
		}
		if err := s.Bootstrap(r); !errors.Is(err, ErrExists) {
			t.Fatalf("replaced symlink: %v", err)
		}
		b, _ := os.ReadFile(target)
		if string(b) != "safe" {
			t.Fatal("target changed")
		}
	})
	t.Run("lock symlink", func(t *testing.T) {
		s, _, dir := fixture(t)
		if err := os.Symlink("missing", filepath.Join(dir, lockName)); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Read(); err == nil {
			t.Fatal("accepted symlink lock")
		}
	})
	t.Run("fifo record", func(t *testing.T) {
		s, _, dir := fixture(t)
		if err := unix.Mkfifo(filepath.Join(dir, recordName), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Read(); err == nil {
			t.Fatal("accepted fifo")
		}
	})
	t.Run("fifo lock", func(t *testing.T) {
		s, _, dir := fixture(t)
		if err := unix.Mkfifo(filepath.Join(dir, lockName), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Read(); err == nil {
			t.Fatal("accepted fifo lock")
		}
	})
	t.Run("hardlink", func(t *testing.T) {
		s, r, dir := fixture(t)
		if err := s.Bootstrap(r); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(filepath.Join(dir, recordName), filepath.Join(dir, "alias")); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Read(); err == nil {
			t.Fatal("accepted hardlink")
		}
	})
	t.Run("file mode", func(t *testing.T) {
		s, r, dir := fixture(t)
		if err := s.Bootstrap(r); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filepath.Join(dir, recordName), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Read(); err == nil {
			t.Fatal("accepted file mode")
		}
	})
	t.Run("directory mode", func(t *testing.T) {
		s, _, dir := fixture(t)
		if err := os.Chmod(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Read(); err == nil {
			t.Fatal("accepted directory mode")
		}
	})
	t.Run("directory symlink", func(t *testing.T) {
		s, _, dir := fixture(t)
		link := filepath.Join(filepath.Dir(dir), "guard-link")
		if err := os.Symlink(dir, link); err != nil {
			t.Fatal(err)
		}
		s.dir = link
		if _, err := s.Read(); err == nil {
			t.Fatal("accepted directory symlink")
		}
	})
}
func TestSyncFailureIsIndeterminateAndReadReturnsZero(t *testing.T) {
	s, r, dir := fixture(t)
	s.syncDir = func(int) error { return errors.New("injected") }
	if err := s.Bootstrap(r); !errors.Is(err, ErrIndeterminate) {
		t.Fatalf("bootstrap %v", err)
	}
	if got, err := s.Read(); !errors.Is(err, ErrIndeterminate) || got != (Record{}) {
		t.Fatalf("read %+v %v", got, err)
	}
	fresh := NewStore(dir)
	if got, err := fresh.Read(); err != nil || got != r {
		t.Fatalf("durability confirmation %+v %v", got, err)
	}
	if err := fresh.Quarantine(r); err != nil {
		t.Fatal(err)
	}
}
func TestTempCollisionAndFailureCleanup(t *testing.T) {
	s, r, dir := fixture(t)
	name := ".replicaguard-conflict.tmp"
	s.tempName = func() (string, error) { return name, nil }
	target := filepath.Join(dir, name)
	if err := os.WriteFile(target, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.Bootstrap(r); err == nil {
		t.Fatal("collision accepted")
	}
	b, err := os.ReadFile(target)
	if err != nil || string(b) != "untouched" {
		t.Fatalf("collision changed %q %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(dir, recordName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("record created %v", err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, recordName), 0700); err != nil {
		t.Fatal(err)
	}
	if err := s.locked(func(fd int) error { return s.writeAt(fd, r) }); err == nil {
		t.Fatal("rename to directory succeeded")
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temp remains %v", err)
	}
}
func TestConcurrentStoresCannotOverwriteMarker(t *testing.T) {
	s, r, dir := fixture(t)
	if err := s.Bootstrap(r); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- NewStore(dir).Quarantine(r) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil && !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			t.Fatal(err)
		}
	}
	got, err := s.Read()
	if err != nil || got != r {
		t.Fatalf("concurrent state %+v %v", got, err)
	}
}
