package snapshotpipeline

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jincaiw/GoDDI/internal/enterprise/snapshotrows"
	"github.com/jincaiw/GoDDI/internal/enterprise/snapshotverify"
)

func fixture() (snapshotverify.Manifest, snapshotverify.Trust, snapshotverify.Limits, snapshotrows.Limits, []SchemaTable, [][]byte) {
	raw := []byte("{\"table\":\"spaces\",\"row\":{\"name\":\"地址\"}}\n{\"table\":\"prefixes\",\"row\":{\"id\":1}}\n")
	// Deliberately split within the first multi-byte UTF-8 character.
	cut := bytes.Index(raw, []byte("地")) + 1
	chunks := [][]byte{append([]byte(nil), raw[:cut]...), append([]byte(nil), raw[cut:]...)}
	m := snapshotverify.Manifest{Cluster: [16]byte{1}, Leader: [16]byte{2}, Snapshot: [16]byte{3}, Schema: 1, Anchor: snapshotverify.Position{Epoch: 1}, Encoding: snapshotverify.EncodingNDJSONV1, RetentionUntilMS: 200, TotalBytes: uint64(len(raw)), Tables: []snapshotverify.TableCount{{Name: "spaces", Rows: 1}, {Name: "prefixes", Rows: 1}, {Name: "empty", Rows: 0}}}
	for i, b := range chunks {
		m.Chunks = append(m.Chunks, snapshotverify.ChunkSpec{Index: uint32(i), Size: uint64(len(b)), SHA256: sha256.Sum256(b)})
	}
	trust := snapshotverify.Trust{Authenticated: true, Cluster: m.Cluster, Leader: m.Leader, Snapshot: m.Snapshot, Anchor: m.Anchor}
	tl := snapshotverify.Limits{MaxBytes: 1024, MaxChunks: 8, MaxRows: 100, Schema: 1}
	rl := snapshotrows.Limits{LineBytes: 1024, TotalBytes: 1024, Rows: 100, Tables: 3}
	schema := []SchemaTable{{Name: "spaces"}, {Name: "prefixes"}, {Name: "empty"}}
	return m, trust, tl, rl, schema, chunks
}
func TestEmptySnapshotAndInconsistentManifests(t *testing.T) {
	m, tr, tl, rl, s, _ := fixture()
	m.Chunks = nil
	m.TotalBytes = 0
	for i := range m.Tables {
		m.Tables[i].Rows = 0
	}
	p, err := New(m, tr, tl, rl, s, 100)
	if err != nil {
		t.Fatal(err)
	}
	out, err := p.Finish(101)
	if err != nil || out.BusinessComplete || !out.Rows.StructureAndCountsValid || out.Rows.Rows != 0 || out.Transport.Chunks != 0 {
		t.Fatalf("empty: %+v %v", out, err)
	}
	if _, err = p.Finish(101); !errors.Is(err, ErrInvalid) {
		t.Fatalf("repeat: %v", err)
	}
	if _, err = p.Finish(101); !errors.Is(err, ErrDirty) {
		t.Fatalf("not dirty: %v", err)
	}
	for _, kind := range []string{"rows", "bytes", "chunk", "empty chunk", "data without rows", "expiry at start", "identity", "table order"} {
		t.Run(kind, func(t *testing.T) {
			bad := m
			bad.Tables = append([]snapshotverify.TableCount(nil), m.Tables...)
			trust := tr
			now := int64(100)
			switch kind {
			case "rows":
				bad.Tables[0].Rows = 1
			case "bytes":
				bad.TotalBytes = 1
			case "chunk":
				bad.Chunks = []snapshotverify.ChunkSpec{{Index: 0, Size: 1}}
			case "empty chunk":
				bad.Chunks = []snapshotverify.ChunkSpec{{Index: 0, Size: 0}}
			case "data without rows":
				bad.Chunks = []snapshotverify.ChunkSpec{{Index: 0, Size: 1}}
				bad.TotalBytes = 1
			case "expiry at start":
				now = 200
			case "identity":
				trust.Authenticated = false
			case "table order":
				bad.Tables[0], bad.Tables[1] = bad.Tables[1], bad.Tables[0]
			}
			if _, err := New(bad, trust, tl, rl, s, now); err == nil {
				t.Fatal("accepted inconsistent manifest")
			}
		})
	}
	p, err = New(m, tr, tl, rl, s, 100)
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Feed(0, nil, 101); err == nil {
		t.Fatal("accepted chunk for empty snapshot")
	}
	if _, err = p.Finish(101); !errors.Is(err, ErrDirty) {
		t.Fatal(err)
	}
	p, err = New(m, tr, tl, rl, s, 100)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Finish(200); err == nil {
		t.Fatal("accepted expired empty snapshot")
	}
	if _, err = p.Finish(200); !errors.Is(err, ErrDirty) {
		t.Fatal(err)
	}
}

func TestNilPanicAndGoexitPermanentlyDirty(t *testing.T) {
	if os.Getenv("PIPELINE_PANIC_CHILD") == "1" {
		m, tr, tl, rl, s, chunks := fixture()
		calls := 0
		s[0].ValidateRow = func([]byte) error { calls++; panic(nil) }
		p, err := New(m, tr, tl, rl, s, 100)
		if err != nil {
			t.Fatal(err)
		}
		if err = p.Feed(0, chunks[0], 101); err != nil {
			t.Fatal(err)
		}
		if err = p.Feed(1, chunks[1], 102); !errors.Is(err, ErrInvalid) {
			t.Fatalf("nil panic accepted: %v", err)
		}
		if err = p.Feed(1, chunks[1], 102); !errors.Is(err, ErrDirty) || calls != 1 {
			t.Fatalf("dirty=%v calls=%d", err, calls)
		}
		if _, err = p.Finish(103); !errors.Is(err, ErrDirty) {
			t.Fatal(err)
		}
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestNilPanicAndGoexitPermanentlyDirty$")
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "GODEBUG=") && !strings.HasPrefix(e, "PIPELINE_PANIC_CHILD=") {
			cmd.Env = append(cmd.Env, e)
		}
	}
	cmd.Env = append(cmd.Env, "GODEBUG=panicnil=1", "PIPELINE_PANIC_CHILD=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("child: %s %v", output, err)
	}
	m, tr, tl, rl, s, chunks := fixture()
	calls := 0
	s[0].ValidateRow = func([]byte) error { calls++; runtime.Goexit(); return nil }
	p, err := New(m, tr, tl, rl, s, 100)
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Feed(0, chunks[0], 101); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { defer close(done); _ = p.Feed(1, chunks[1], 102) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Goexit blocked")
	}
	if err = p.Feed(1, chunks[1], 102); !errors.Is(err, ErrDirty) || calls != 1 {
		t.Fatalf("dirty=%v calls=%d", err, calls)
	}
	if _, err = p.Finish(103); !errors.Is(err, ErrDirty) {
		t.Fatal(err)
	}
}

func TestUTF8SplitAndTransportStructureCountOnly(t *testing.T) {
	m, tr, tl, rl, s, chunks := fixture()
	p, err := New(m, tr, tl, rl, s, 100)
	if err != nil {
		t.Fatal(err)
	}
	for i, c := range chunks {
		if err := p.Feed(uint32(i), c, 101); err != nil {
			t.Fatal(err)
		}
	}
	got, err := p.Finish(102)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Rows.StructureAndCountsValid || got.Rows.Rows != 2 || got.Rows.AllRowCallbacksPassed || got.BusinessComplete || got.Transport.RowsVerified || got.Rows.Bytes != m.TotalBytes {
		t.Fatalf("overstated result %+v", got)
	}
}
func TestNewRejectsManifestTableSetAndOrder(t *testing.T) {
	cases := map[string]func(*snapshotverify.Manifest){
		"missing": func(m *snapshotverify.Manifest) { m.Tables = m.Tables[:2] }, "duplicate": func(m *snapshotverify.Manifest) { m.Tables[1].Name = "spaces" }, "unknown": func(m *snapshotverify.Manifest) { m.Tables[1].Name = "unknown" }, "reordered": func(m *snapshotverify.Manifest) { m.Tables[0], m.Tables[1] = m.Tables[1], m.Tables[0] },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			m, tr, tl, rl, s, _ := fixture()
			mutate(&m)
			if _, err := New(m, tr, tl, rl, s, 100); err == nil {
				t.Fatal("invalid table set accepted")
			}
		})
	}
}
func TestUnverifiedChunkNeverReachesCallback(t *testing.T) {
	for _, kind := range []string{"hash", "order"} {
		t.Run(kind, func(t *testing.T) {
			m, tr, tl, rl, s, chunks := fixture()
			calls := 0
			s[0].ValidateRow = func([]byte) error { calls++; return nil }
			p, err := New(m, tr, tl, rl, s, 100)
			if err != nil {
				t.Fatal(err)
			}
			index := uint32(0)
			data := append([]byte(nil), chunks[0]...)
			if kind == "hash" {
				data[0] ^= 1
			} else {
				index = 1
			}
			if err := p.Feed(index, data, 101); err == nil {
				t.Fatal("invalid chunk accepted")
			}
			if calls != 0 {
				t.Fatal("callback saw invalid data")
			}
			if err := p.Feed(0, chunks[0], 102); !errors.Is(err, ErrDirty) {
				t.Fatalf("not dirty: %v", err)
			}
			if _, err := p.Finish(103); !errors.Is(err, ErrDirty) {
				t.Fatalf("finish: %v", err)
			}
		})
	}
}
func TestCallbackFailureMakesStagingDisposable(t *testing.T) {
	m, tr, tl, rl, s, chunks := fixture()
	var staging [][]byte
	s[0].ValidateRow = func(row []byte) error {
		staging = append(staging, append([]byte(nil), row...))
		return errors.New("injected")
	}
	p, err := New(m, tr, tl, rl, s, 100)
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Feed(0, chunks[0], 101); err != nil {
		t.Fatal(err)
	}
	if err = p.Feed(1, chunks[1], 102); err == nil {
		t.Fatal("callback failure accepted")
	}
	if len(staging) != 1 {
		t.Fatalf("expected provisional work: %d", len(staging))
	}
	if _, err = p.Finish(103); !errors.Is(err, ErrDirty) {
		t.Fatalf("finish returned result after failure: %v", err)
	}
	// The caller discards its own provisional staging; the pipeline has no commit API.
	staging = nil
}
func TestCallbackPanicPermanentlyInvalidatesPipeline(t *testing.T) {
	m, tr, tl, rl, s, chunks := fixture()
	s[0].ValidateRow = func([]byte) error { panic("callback failure") }
	p, err := New(m, tr, tl, rl, s, 100)
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Feed(0, chunks[0], 101); err != nil {
		t.Fatal(err)
	}
	if err = p.Feed(1, chunks[1], 102); err == nil {
		t.Fatal("callback panic accepted")
	}
	if _, err = p.Finish(103); !errors.Is(err, ErrDirty) {
		t.Fatalf("not dirty: %v", err)
	}
}

func TestFinishRejectsCountsAndExpiration(t *testing.T) {
	for _, kind := range []string{"count", "expiry", "truncated"} {
		t.Run(kind, func(t *testing.T) {
			m, tr, tl, rl, s, chunks := fixture()
			if kind == "count" {
				m.Tables[0].Rows = 2
			}
			p, err := New(m, tr, tl, rl, s, 100)
			if err != nil {
				t.Fatal(err)
			}
			if kind != "truncated" {
				for i, c := range chunks {
					if err = p.Feed(uint32(i), c, 101); err != nil {
						t.Fatal(err)
					}
				}
			}
			upper := int64(102)
			if kind == "expiry" {
				upper = 200
			}
			if result, err := p.Finish(upper); err == nil || result.BusinessComplete {
				t.Fatalf("finish accepted: %+v %v", result, err)
			}
			if _, err = p.Finish(upper); !errors.Is(err, ErrDirty) {
				t.Fatalf("not permanently dirty: %v", err)
			}
		})
	}
}
func TestConcurrentDuplicateFeedsCannotInterleave(t *testing.T) {
	m, tr, tl, rl, s, chunks := fixture()
	p, err := New(m, tr, tl, rl, s, 100)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- p.Feed(0, chunks[0], 101) }()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("accepted %d copies", success)
	}
	if err := p.Feed(1, chunks[1], 102); !errors.Is(err, ErrDirty) {
		t.Fatalf("pipeline not dirty: %v", err)
	}
}
