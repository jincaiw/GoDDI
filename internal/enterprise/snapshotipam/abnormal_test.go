package snapshotipam

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"testing"
)

func TestNilSinkPanicPreservesFailureInSubprocess(t *testing.T) {
	if mode := os.Getenv("SNAPSHOTIPAM_NIL_CHILD"); mode != "" {
		v := New()
		effects := 0
		callbacks := v.Tables(func(Row) error { effects++; panic(nil) })
		raw := row(t, tenant("t1"))
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
				if _, err := v.Validate("ipam_tenants", row(t, tenant("t2"))); !errors.Is(err, ErrDirty) {
					t.Errorf("not dirty during recover: %v", err)
				}
			}()
			_ = callbacks[0].ValidateRow(raw)
			returned = true
		}()
		if returned || !recovered || effects != 1 {
			t.Fatalf("returned=%v recovered=%v effects=%d", returned, recovered, effects)
		}
		if err := callbacks[0].ValidateRow(raw); !errors.Is(err, ErrDirty) || effects != 1 {
			t.Fatalf("retry %v effects=%d", err, effects)
		}
		return
	}
	for _, mode := range []string{"0", "1"} {
		t.Run(mode, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestNilSinkPanicPreservesFailureInSubprocess$")
			cmd.Env = append(os.Environ(), "GODEBUG=panicnil="+mode, "SNAPSHOTIPAM_NIL_CHILD="+mode)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%v\n%s", err, out)
			}
		})
	}
}
func TestSinkGoexitPoisonsWithoutReturning(t *testing.T) {
	v := New()
	effects := 0
	callbacks := v.Tables(func(Row) error { effects++; runtime.Goexit(); return nil })
	raw := row(t, tenant("t1"))
	done := make(chan struct{})
	returned := false
	go func() { defer close(done); _ = callbacks[0].ValidateRow(raw); returned = true }()
	<-done
	if returned || effects != 1 {
		t.Fatalf("returned=%v effects=%d", returned, effects)
	}
	if _, err := v.Validate("ipam_tenants", row(t, tenant("t2"))); !errors.Is(err, ErrDirty) {
		t.Fatal(err)
	}
	if err := callbacks[0].ValidateRow(raw); !errors.Is(err, ErrDirty) || effects != 1 {
		t.Fatalf("retry %v effects=%d", err, effects)
	}
}
