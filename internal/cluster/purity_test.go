package cluster

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestProtocolModelIsAPureContract pins the N7 boundary from the inside: the
// cluster package is an executable protocol model + a reserved 501 boundary,
// not a production coordination implementation. If real transport, storage,
// election, or an HTTP entrypoint creeps in, this fails so the change is
// reviewed against docs/B8.5-外部验收Runbook.md §9 before anyone can read the
// protocol model as production HA.
func TestProtocolModelIsAPureContract(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}

	forbiddenImports := []string{
		`"net"`, `"net/http"`, `"net/rpc"`, `"os/exec"`, `"database/sql"`,
		`"syscall"`, `"google.golang.org/grpc"`,
	}
	// Markers that would mean a real server/coordinator, not a model.
	forbiddenMarkers := []string{"http.Handler", "RegisterRoutes", "grpc.NewServer", "func Serve("}

	sawErrNotImplemented := false
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		text := string(data)
		for _, imp := range forbiddenImports {
			if strings.Contains(text, imp) {
				t.Fatalf("%s imports %s; internal/cluster must stay a pure protocol model (see B8.5 §9)", e.Name(), imp)
			}
		}
		for _, m := range forbiddenMarkers {
			if strings.Contains(text, m) {
				t.Fatalf("%s contains server marker %q; cluster APIs must remain the reserved 501 boundary", e.Name(), m)
			}
		}
		if strings.Contains(text, "ErrNotImplemented = errors.New") {
			sawErrNotImplemented = true
		}
	}
	if !sawErrNotImplemented {
		t.Fatal("internal/cluster no longer defines ErrNotImplemented; the reserved 501 boundary was removed")
	}
}
