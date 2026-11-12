package timewater

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"golang.org/x/sys/unix"
)

func privateDir(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	d := filepath.Join(root, "private")
	if err := os.Mkdir(d, 0700); err != nil {
		t.Fatal(err)
	}
	return d
}
func TestLifecycleAndRestart(t *testing.T) {
	d := privateDir(t)
	s := NewStore(d)
	if _, err := s.Read(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing: %v", err)
	}
	if err := s.Advance(5); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("implicit bootstrap: %v", err)
	}
	if err := s.Bootstrap(10); err != nil {
		t.Fatal(err)
	}
	if err := s.Bootstrap(11); !errors.Is(err, ErrExists) {
		t.Fatal(err)
	}
	if err := s.Advance(9); !errors.Is(err, ErrRollback) {
		t.Fatal(err)
	}
	if err := s.Advance(12); err != nil {
		t.Fatal(err)
	}
	if got, err := NewStore(d).Read(); err != nil || got != 12 {
		t.Fatalf("restart: %d %v", got, err)
	}
}
func TestCorruptAndUnsafeFiles(t *testing.T) {
	for _, raw := range []string{"", "v1\n0\n", "v1\n01\n", "v1\n-1\n", "v1\n12\nextra", string(make([]byte, 65))} {
		d := privateDir(t)
		if err := os.WriteFile(filepath.Join(d, recordName), []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		s := NewStore(d)
		if _, err := s.Read(); err == nil {
			t.Fatalf("accepted %q", raw)
		}
		if err := s.Advance(20); err == nil {
			t.Fatal("advanced corrupt")
		}
	}
	for _, kind := range []string{"symlink", "fifo", "hardlink", "mode", "lockfifo", "parentlink"} {
		t.Run(kind, func(t *testing.T) {
			d := privateDir(t)
			p := filepath.Join(d, recordName)
			switch kind {
			case "symlink":
				if err := os.Symlink("missing", p); err != nil {
					t.Fatal(err)
				}
			case "fifo":
				if err := unix.Mkfifo(p, 0600); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				other := filepath.Join(d, "other")
				if err := os.WriteFile(other, []byte("v1\n1\n"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Link(other, p); err != nil {
					t.Fatal(err)
				}
			case "mode":
				if err := os.WriteFile(p, []byte("v1\n1\n"), 0644); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(p, 0644); err != nil {
					t.Fatal(err)
				}
			case "lockfifo":
				if err := unix.Mkfifo(filepath.Join(d, lockName), 0600); err != nil {
					t.Fatal(err)
				}
			case "parentlink":
				link := filepath.Join(filepath.Dir(d), "link")
				if err := os.Symlink(d, link); err != nil {
					t.Fatal(err)
				}
				d = link
			}
			if _, err := NewStore(d).Read(); err == nil {
				t.Fatal("unsafe state accepted")
			}
		})
	}
}
func TestSyncFailureAndEqualRetry(t *testing.T) {
	d := privateDir(t)
	s := NewStore(d)
	if err := s.Bootstrap(10); err != nil {
		t.Fatal(err)
	}
	s.syncDir = func(int) error { return errors.New("injected") }
	if err := s.Advance(20); !errors.Is(err, ErrIndeterminate) {
		t.Fatalf("advance: %v", err)
	}
	if got, err := NewStore(d).Read(); err != nil || got != 20 {
		t.Fatalf("post rename: %d %v", got, err)
	}
	if err := s.Advance(20); !errors.Is(err, ErrIndeterminate) {
		t.Fatalf("equal must sync: %v", err)
	}
	if got, err := s.Read(); got != 0 || !errors.Is(err, ErrIndeterminate) {
		t.Fatalf("read must not expose an unconfirmed value: %d, %v", got, err)
	}
	s.syncDir = unix.Fsync
	if err := s.Advance(20); err != nil {
		t.Fatal(err)
	}
}
func TestConcurrentProcesses(t *testing.T) {
	if d := os.Getenv("GODDI_TIMEWATER_CHILD"); d != "" {
		n, err := strconv.ParseInt(os.Getenv("GODDI_TIMEWATER_VALUE"), 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		if err := NewStore(d).Advance(n); err != nil && !errors.Is(err, ErrRollback) {
			t.Fatal(err)
		}
		return
	}
	d := privateDir(t)
	if err := NewStore(d).Bootstrap(1); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := 2; i <= 17; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			cmd := exec.Command(os.Args[0], "-test.run=^TestConcurrentProcesses$")
			cmd.Env = append(os.Environ(), "GODDI_TIMEWATER_CHILD="+d, fmt.Sprintf("GODDI_TIMEWATER_VALUE=%d", n))
			out, err := cmd.CombinedOutput()
			if err != nil {
				errs <- fmt.Errorf("%w: %s", err, out)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if n, err := NewStore(d).Read(); err != nil || n != 17 {
		t.Fatalf("watermark %d: %v", n, err)
	}
}
