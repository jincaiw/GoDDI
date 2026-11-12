package generationpointer

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"golang.org/x/sys/unix"
)

func fixture(t *testing.T) (*store, Record, Expected) {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	r := Record{Version: "1", ClusterID: "11111111-1111-4111-8111-111111111111", NodeID: "22222222-2222-4222-8222-222222222222", OldGeneration: "33333333-3333-4333-8333-333333333333", CurrentGeneration: "33333333-3333-4333-8333-333333333333", TargetGeneration: "44444444-4444-4444-8444-444444444444", Phase: "STAGED", EvidenceDigest: strings.Repeat("a", 64)}
	return newStore(dir), r, Expected{r.ClusterID, r.NodeID, r.OldGeneration, r.TargetGeneration, r.EvidenceDigest}
}
func installed(r Record) Record {
	r.Phase = "INSTALLED"
	r.CurrentGeneration = r.TargetGeneration
	return r
}
func TestPublicEntryPointsRemainClosed(t *testing.T) {
	for _, fn := range []func(context.Context) error{Import, Install, Activate, Switch} {
		if err := fn(context.Background()); !errors.Is(err, ErrNoAuthority) {
			t.Fatal(err)
		}
	}
}
func TestStageAdvanceInspectRestart(t *testing.T) {
	s, r, e := fixture(t)
	if got, err := Inspect(s.dir, e); err == nil || got != (Record{}) {
		t.Fatalf("missing %+v %v", got, err)
	}
	if err := s.stage(r); err != nil {
		t.Fatal(err)
	}
	if got, err := Inspect(s.dir, e); err != nil || got != r {
		t.Fatalf("stage %+v %v", got, err)
	}
	next := installed(r)
	if err := newStore(s.dir).advance(next, e); err != nil {
		t.Fatal(err)
	}
	if got, err := Inspect(s.dir, e); err != nil || got != next {
		t.Fatalf("restart %+v %v", got, err)
	}
	if err := s.advance(next, e); err != nil {
		t.Fatal(err)
	}
	if err := s.stage(r); err == nil {
		t.Fatal("stage overwrote existing")
	}
	if err := s.advance(r, e); err == nil {
		t.Fatal("downgrade")
	}
	bad := next
	bad.EvidenceDigest = strings.Repeat("b", 64)
	if err := s.advance(bad, e); err == nil {
		t.Fatal("changed evidence")
	}
	wrong := e
	wrong.TargetGeneration = "55555555-5555-4555-8555-555555555555"
	if got, err := Inspect(s.dir, wrong); err == nil || got != (Record{}) {
		t.Fatalf("wrong identity %+v %v", got, err)
	}
}
func TestInvalidTransitionAndBadRecords(t *testing.T) {
	t.Run("before stage", func(t *testing.T) {
		s, r, e := fixture(t)
		if err := s.advance(installed(r), e); err == nil {
			t.Fatal("implicit init")
		}
	})
	t.Run("identity mismatch", func(t *testing.T) {
		s, r, e := fixture(t)
		if err := s.stage(r); err != nil {
			t.Fatal(err)
		}
		next := installed(r)
		next.TargetGeneration = "55555555-5555-4555-8555-555555555555"
		next.CurrentGeneration = next.TargetGeneration
		if err := s.advance(next, e); err == nil {
			t.Fatal("new target")
		}
	})
	for name, change := range map[string]func(*Record){"ready": func(r *Record) { r.Phase = "READY" }, "active": func(r *Record) { r.Phase = "ACTIVE" }, "same generation": func(r *Record) { r.TargetGeneration = r.OldGeneration }, "wrong current": func(r *Record) { r.CurrentGeneration = r.TargetGeneration }, "zero digest": func(r *Record) { r.EvidenceDigest = strings.Repeat("0", 64) }, "noncanonical uuid": func(r *Record) { r.ClusterID = "AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA" }, "oversized": func(r *Record) { r.EvidenceDigest = strings.Repeat("a", 4096) }} {
		t.Run(name, func(t *testing.T) {
			s, r, _ := fixture(t)
			change(&r)
			if err := s.stage(r); err == nil {
				t.Fatal("accepted invalid")
			}
		})
	}
}
func TestFailureBeforeAndAfterRename(t *testing.T) {
	for _, kind := range []string{"file sync", "rename before", "rename after", "dir sync"} {
		t.Run(kind, func(t *testing.T) {
			s, r, e := fixture(t)
			if err := s.stage(r); err != nil {
				t.Fatal(err)
			}
			next := installed(r)
			switch kind {
			case "file sync":
				s.syncFile = func(*os.File) error { return errors.New("injected") }
			case "rename before":
				s.rename = func(int, string, int, string) error { return errors.New("injected") }
			case "rename after":
				s.rename = func(a int, b string, c int, d string) error {
					if err := unix.Renameat(a, b, c, d); err != nil {
						return err
					}
					return errors.New("injected")
				}
			case "dir sync":
				s.syncDir = func(int) error { return errors.New("injected") }
			}
			err := s.advance(next, e)
			if err == nil {
				t.Fatal("fault ignored")
			}
			if kind != "file sync" && !errors.Is(err, ErrIndeterminate) {
				t.Fatalf("unconfirmed %v", err)
			}
			got, err := Inspect(s.dir, e)
			if err != nil {
				t.Fatal(err)
			}
			want := r
			if kind == "rename after" || kind == "dir sync" {
				want = next
			}
			if got != want {
				t.Fatalf("got %+v want %+v", got, want)
			}
		})
	}
}
func TestInspectRequiresExistingLockWithoutSideEffects(t *testing.T) {
	s, r, e := fixture(t)
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(s.dir, recordName), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := Inspect(s.dir, e); err == nil || got != (Record{}) {
		t.Fatalf("missing lock: %+v %v", got, err)
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != recordName {
		t.Fatalf("inspect wrote files: %+v", entries)
	}
	if err = os.Chmod(s.dir, 0500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(s.dir, 0700)
	if got, err := Inspect(s.dir, e); err == nil || got != (Record{}) {
		t.Fatalf("read-only directory: %+v %v", got, err)
	}
	entries, err = os.ReadDir(s.dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("inspect wrote files: %+v", entries)
	}
}
func TestExpectedEvidenceBound(t *testing.T) {
	s, r, e := fixture(t)
	if err := s.stage(r); err != nil {
		t.Fatal(err)
	}
	for _, digest := range []string{strings.Repeat("b", 64), strings.Repeat("0", 64), ""} {
		bad := e
		bad.EvidenceDigest = digest
		if got, err := Inspect(s.dir, bad); err == nil || got != (Record{}) {
			t.Fatalf("digest mismatch: %+v %v", got, err)
		}
	}
	if got, err := Inspect(s.dir, e); err != nil || got != r {
		t.Fatalf("valid: %+v %v", got, err)
	}
}
func TestReadSyncFailureReturnsZero(t *testing.T) {
	s, r, e := fixture(t)
	if err := s.stage(r); err != nil {
		t.Fatal(err)
	}
	s.syncDir = func(int) error { return errors.New("injected") }
	if got, err := s.inspect(e); !errors.Is(err, ErrIndeterminate) || got != (Record{}) {
		t.Fatalf("read %+v %v", got, err)
	}
}
func TestUnsafeAndCorruptState(t *testing.T) {
	for _, kind := range []string{"corrupt", "oversized", "record symlink", "lock symlink", "fifo record", "fifo lock", "hardlink", "record mode", "directory mode", "directory symlink"} {
		t.Run(kind, func(t *testing.T) {
			s, r, e := fixture(t)
			record := filepath.Join(s.dir, recordName)
			lock := filepath.Join(s.dir, lockName)
			if kind == "corrupt" || kind == "oversized" || kind == "record symlink" || kind == "fifo record" {
				if err := os.WriteFile(lock, nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			switch kind {
			case "corrupt":
				if err := os.WriteFile(record, []byte(`{"phase":"ACTIVE"}`), 0600); err != nil {
					t.Fatal(err)
				}
			case "oversized":
				if err := os.WriteFile(record, []byte(strings.Repeat("x", maxRecordBytes+1)), 0600); err != nil {
					t.Fatal(err)
				}
			case "record symlink":
				if err := os.Symlink("elsewhere", record); err != nil {
					t.Fatal(err)
				}
			case "lock symlink":
				if err := os.Symlink("elsewhere", lock); err != nil {
					t.Fatal(err)
				}
			case "fifo record":
				if err := unix.Mkfifo(record, 0600); err != nil {
					t.Fatal(err)
				}
			case "fifo lock":
				if err := unix.Mkfifo(lock, 0600); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := s.stage(r); err != nil {
					t.Fatal(err)
				}
				if err := os.Link(record, filepath.Join(s.dir, "alias")); err != nil {
					t.Fatal(err)
				}
			case "record mode":
				if err := s.stage(r); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(record, 0644); err != nil {
					t.Fatal(err)
				}
			case "directory mode":
				if err := os.Chmod(s.dir, 0755); err != nil {
					t.Fatal(err)
				}
			case "directory symlink":
				link := filepath.Join(filepath.Dir(s.dir), "pointer-link")
				if err := os.Symlink(s.dir, link); err != nil {
					t.Fatal(err)
				}
				s.dir = link
			}
			if got, err := Inspect(s.dir, e); err == nil || got != (Record{}) {
				t.Fatalf("unsafe %+v %v", got, err)
			}
		})
	}
}
func TestConcurrentTransitionsAndLock(t *testing.T) {
	s, r, e := fixture(t)
	if err := s.stage(r); err != nil {
		t.Fatal(err)
	}
	next := installed(r)
	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- newStore(s.dir).advance(next, e) }()
	}
	wg.Wait()
	close(errs)
	success := 0
	for err := range errs {
		if err == nil {
			success++
		} else if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			t.Fatal(err)
		}
	}
	if success == 0 {
		t.Fatal("all failed")
	}
	if got, err := Inspect(s.dir, e); err != nil || got != next {
		t.Fatalf("final %+v %v", got, err)
	}
}
