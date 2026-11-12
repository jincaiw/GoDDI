package rolelog

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func storeFixture(t *testing.T) (*Store, Record, string) {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	r, err := Parse([]byte(valid))
	if err != nil {
		t.Fatal(err)
	}
	r.Phase = "BOOTSTRAP"
	return NewStore(dir), r, dir
}

func TestStoreInitializationRestartAndFencing(t *testing.T) {
	s, r, dir := storeFixture(t)
	if _, err := s.Read(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing record: %v", err)
	}
	if err := s.Save(r); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("save created missing record: %v", err)
	}
	active := r
	active.Phase = "ACTIVE"
	if err := s.Initialize(active); !errors.Is(err, ErrTransition) {
		t.Fatalf("active initialization: %v", err)
	}
	if err := s.Initialize(r); err != nil {
		t.Fatal(err)
	}
	if err := s.Initialize(r); !errors.Is(err, ErrTransition) {
		t.Fatalf("record replaced: %v", err)
	}
	fresh := NewStore(dir)
	got, err := fresh.Read()
	if err != nil || got != r {
		t.Fatalf("restart read: %+v %v", got, err)
	}
	fenced := r
	fenced.Phase = "FENCED"
	if err := fresh.Save(fenced); err != nil {
		t.Fatal(err)
	}
	if err := fresh.Save(r); !errors.Is(err, ErrTransition) {
		t.Fatalf("reopened fenced role: %v", err)
	}
	got, err = NewStore(dir).Read()
	if err != nil || got != fenced {
		t.Fatalf("fence restart: %+v %v", got, err)
	}
}

func TestStoreRejectsRollbackNonceAndIdentityChanges(t *testing.T) {
	s, r, _ := storeFixture(t)
	if err := s.Initialize(r); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*Record){
		"rollback": func(v *Record) { v.Epoch-- }, "advance": func(v *Record) { v.Epoch++ },
		"nonce": func(v *Record) { v.Nonce = "44444444-4444-4444-8444-444444444444" },
		"clone": func(v *Record) { v.DeviceID = "different-device" }, "generation": func(v *Record) { v.DBGeneration = "other" },
		"active": func(v *Record) { v.Phase = "ACTIVE" }, "invalid": func(v *Record) { v.Nonce = "" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			v := r
			change(&v)
			if err := s.Save(v); err == nil {
				t.Fatal("unsafe transition accepted")
			}
		})
	}
	got, err := s.Read()
	if err != nil || got != r {
		t.Fatalf("changed original: %+v %v", got, err)
	}
}

func TestStoreRefusesCorruptAndUnsafeFiles(t *testing.T) {
	t.Run("corrupt", func(t *testing.T) {
		s, r, dir := storeFixture(t)
		if err := os.WriteFile(filepath.Join(dir, recordName), []byte(`{}`), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Read(); !errors.Is(err, ErrInvalid) {
			t.Fatalf("read: %v", err)
		}
		if err := s.Save(r); !errors.Is(err, ErrInvalid) {
			t.Fatalf("overwrite corruption: %v", err)
		}
	})
	t.Run("record-symlink", func(t *testing.T) {
		s, r, dir := storeFixture(t)
		target := filepath.Join(t.TempDir(), "outside")
		if err := os.WriteFile(target, []byte("untouched"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(dir, recordName)); err != nil {
			t.Fatal(err)
		}
		if err := s.Initialize(r); err == nil {
			t.Fatal("initialize replaced symlink")
		}
		if _, err := s.Read(); err == nil {
			t.Fatal("read followed symlink")
		}
		b, _ := os.ReadFile(target)
		if string(b) != "untouched" {
			t.Fatal("outside modified")
		}
	})
	t.Run("lock-symlink", func(t *testing.T) {
		s, _, dir := storeFixture(t)
		if err := os.Symlink(filepath.Join(dir, "missing"), filepath.Join(dir, lockName)); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Read(); err == nil {
			t.Fatal("followed lock symlink")
		}
	})
	t.Run("directory-symlink", func(t *testing.T) {
		s, _, dir := storeFixture(t)
		link := filepath.Join(filepath.Dir(dir), "role-link")
		if err := os.Symlink(dir, link); err != nil {
			t.Fatal(err)
		}
		s.dir = link
		if _, err := s.Read(); err == nil {
			t.Fatal("followed directory symlink")
		}
	})
	t.Run("file-hardlink", func(t *testing.T) {
		s, r, dir := storeFixture(t)
		if err := s.Initialize(r); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(filepath.Join(dir, recordName), filepath.Join(dir, "alias")); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Read(); err == nil {
			t.Fatal("accepted hard-linked record")
		}
	})
	t.Run("permissions", func(t *testing.T) {
		s, _, dir := storeFixture(t)
		if err := os.Chmod(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Read(); err == nil {
			t.Fatal("accepted unsafe directory")
		}
	})
}

func TestTemporaryCollisionLeavesExistingFileAndRecordIntact(t *testing.T) {
	s, r, dir := storeFixture(t)
	if err := s.Initialize(r); err != nil {
		t.Fatal(err)
	}
	name := ".role-collision.tmp"
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("sentinel"), 0600); err != nil {
		t.Fatal(err)
	}
	s.tempName = func() (string, error) { return name, nil }
	next := r
	next.Phase = "FENCED"
	if err := s.Save(next); err == nil {
		t.Fatal("collision accepted")
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "sentinel" {
		t.Fatalf("collision file altered: %q %v", b, err)
	}
	got, err := s.Read()
	if err != nil || got != r {
		t.Fatalf("record changed: %+v %v", got, err)
	}
}

func TestFailedReplacementCleansTemporaryFile(t *testing.T) {
	s, r, dir := storeFixture(t)
	if err := os.Mkdir(filepath.Join(dir, recordName), 0700); err != nil {
		t.Fatal(err)
	}
	name := ".role-failed.tmp"
	s.tempName = func() (string, error) { return name, nil }
	if err := s.withDirectory(func(fd int) error { return s.writeAt(fd, r) }); err == nil {
		t.Fatal("replacement over directory succeeded")
	}
	if _, err := os.Lstat(filepath.Join(dir, name)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary file remains: %v", err)
	}
}

func TestConcurrentFencing(t *testing.T) {
	s, r, _ := storeFixture(t)
	if err := s.Initialize(r); err != nil {
		t.Fatal(err)
	}
	r.Phase = "FENCED"
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- s.Save(r) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestIdenticalRetryConfirmsDirectoryDurability(t *testing.T) {
	s, r, _ := storeFixture(t)
	if err := s.Initialize(r); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("injected directory sync failure")
	calls := 0
	s.syncDir = func(int) error { calls++; return failure }
	r.Phase = "FENCED"
	if err := s.Save(r); !errors.Is(err, ErrIndeterminate) || !errors.Is(err, failure) {
		t.Fatalf("replacement: %v", err)
	}
	if err := s.Save(r); !errors.Is(err, ErrIndeterminate) || !errors.Is(err, failure) {
		t.Fatalf("same-value retry: %v", err)
	}
	if calls != 2 {
		t.Fatalf("sync calls = %d, want 2", calls)
	}
	s.syncDir = func(fd int) error { calls++; return unix.Fsync(fd) }
	if err := s.Save(r); err != nil {
		t.Fatalf("recovered sync: %v", err)
	}
	if calls != 3 {
		t.Fatalf("sync calls = %d, want 3", calls)
	}
	got, err := s.Read()
	if err != nil || got != r {
		t.Fatalf("read after recovery: %+v %v", got, err)
	}
}

func TestReadRejectsFIFOWithoutBlocking(t *testing.T) {
	s, r, dir := storeFixture(t)
	if err := unix.Mkfifo(filepath.Join(dir, recordName), 0600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := s.Read(); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("accepted FIFO")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("reading FIFO blocked")
	}
	done = make(chan error, 1)
	go func() { done <- s.Save(r) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("saved over FIFO")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("saving with FIFO blocked")
	}
}

func TestReadConfirmsOtherStoreReplacementDurability(t *testing.T) {
	writer, record, dir := storeFixture(t)
	if err := writer.Initialize(record); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("directory sync unavailable")
	writer.syncDir = func(int) error { return failure }
	record.Phase = "FENCED"
	if err := writer.Save(record); !errors.Is(err, ErrIndeterminate) {
		t.Fatalf("replacement: %v", err)
	}
	reader := NewStore(dir)
	calls := 0
	reader.syncDir = func(int) error { calls++; return failure }
	got, err := reader.Read()
	if !errors.Is(err, ErrIndeterminate) || !errors.Is(err, failure) || got != (Record{}) {
		t.Fatalf("unconfirmed read: %+v %v", got, err)
	}
	if calls != 1 {
		t.Fatalf("sync calls = %d, want 1", calls)
	}
	reader.syncDir = func(fd int) error { calls++; return unix.Fsync(fd) }
	got, err = reader.Read()
	if err != nil || got != record || calls != 2 {
		t.Fatalf("confirmed read: %+v, calls=%d, err=%v", got, calls, err)
	}
}
