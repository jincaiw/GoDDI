package snapshotrows

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"testing"
)

func TestNilPanicPreservesFailureInSubprocess(t *testing.T) {
	if mode := os.Getenv("SNAPSHOTROWS_NIL_CHILD"); mode != "" {
		effects := 0
		v := makeValidator(t, []Table{{Name: "a", Count: 1, ValidateRow: func([]byte) error { effects++; panic(nil) }}})
		returned, recovered := false, false
		func() {
			defer func() {
				x := recover()
				recovered = true
				if mode == "1" && x != nil {
					t.Errorf("nil panic changed: %v", x)
				}
				if mode == "0" && x == nil {
					t.Error("default panic lost")
				}
				if e := v.Feed(nil); !errors.Is(e, ErrDirty) {
					t.Errorf("not dirty during recover: %v", e)
				}
			}()
			_ = v.Feed([]byte("{\"table\":\"a\",\"row\":{}}\n"))
			returned = true
		}()
		if returned || !recovered || effects != 1 {
			t.Fatalf("returned=%v recovered=%v effects=%d", returned, recovered, effects)
		}
		if _, err := v.Finish(); !errors.Is(err, ErrDirty) {
			t.Fatal(err)
		}
		if err := v.Feed([]byte("{\"table\":\"a\",\"row\":{}}\n")); !errors.Is(err, ErrDirty) || effects != 1 {
			t.Fatalf("retry %v effects=%d", err, effects)
		}
		return
	}
	for _, mode := range []string{"0", "1"} {
		t.Run(mode, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestNilPanicPreservesFailureInSubprocess$")
			cmd.Env = append(os.Environ(), "GODEBUG=panicnil="+mode, "SNAPSHOTROWS_NIL_CHILD="+mode)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%v\n%s", err, out)
			}
		})
	}
}

func TestGoexitPoisonsWithoutReturning(t *testing.T) {
	effects := 0
	v := makeValidator(t, []Table{{Name: "a", Count: 1, ValidateRow: func([]byte) error { effects++; runtime.Goexit(); return nil }}})
	done := make(chan struct{})
	returned := false
	go func() { defer close(done); _ = v.Feed([]byte("{\"table\":\"a\",\"row\":{}}\n")); returned = true }()
	<-done
	if returned || effects != 1 {
		t.Fatalf("returned=%v effects=%d", returned, effects)
	}
	if err := v.Feed(nil); !errors.Is(err, ErrDirty) {
		t.Fatal(err)
	}
	if _, err := v.Finish(); !errors.Is(err, ErrDirty) {
		t.Fatal(err)
	}
}
