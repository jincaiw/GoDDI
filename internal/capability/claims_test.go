package capability

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReadmeCapabilityClaimsMatchReleaseBoundary(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	for _, name := range []string{"README.md", "README.zh-CN.md"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		text := string(data)
		for _, stale := range []string{"v0.1.2", "v0.1.3", "releases/download/v0.1.3"} {
			if strings.Contains(text, stale) {
				t.Errorf("%s still contains stale release claim %q", name, stale)
			}
		}
		for _, required := range []string{
			"v0.8.3",
			"v0.8.2",
			"v0.8.1",
			"DoT/DoH/DoQ",
			"DNSSEC",
			"501 Not Implemented",
			"external-validation-required",
		} {
			if !strings.Contains(text, required) {
				t.Errorf("%s is missing required capability boundary %q", name, required)
			}
		}
		if strings.Contains(text, "DoT/DoH/DoQ configuration") && strings.Contains(text, "reserved APIs") {
			t.Errorf("%s collapses implemented encrypted DNS listeners into the reserved API claim", name)
		}
	}
}
