package snapshotverify

import (
	"crypto/sha256"
	"errors"
	"sync"
	"testing"
)

func fixture() (Manifest, Trust, Limits, [][]byte) {
	chunks := [][]byte{[]byte("hello\n"), []byte("world\n")}
	m := Manifest{Cluster: [16]byte{1}, Leader: [16]byte{2}, Snapshot: [16]byte{3}, Schema: 1, Anchor: Position{Epoch: 1}, Encoding: EncodingNDJSONV1, RetentionUntilMS: 200, TotalBytes: 12, Tables: []TableCount{{Name: "ipam_spaces", Rows: 1}}}
	for i, b := range chunks {
		m.Chunks = append(m.Chunks, ChunkSpec{Index: uint32(i), Size: uint64(len(b)), SHA256: sha256.Sum256(b)})
	}
	trust := Trust{Authenticated: true, Cluster: m.Cluster, Leader: m.Leader, Snapshot: m.Snapshot, Anchor: m.Anchor}
	return m, trust, Limits{MaxBytes: 1024, MaxChunks: 8, MaxRows: 100, Tables: map[string]bool{"ipam_spaces": true}, Schema: 1}, chunks
}
func TestTransportCompleteIsNotRowOrInstallProof(t *testing.T) {
	m, tr, l, parts := fixture()
	v, err := New(m, tr, l, 100)
	if err != nil {
		t.Fatal(err)
	}
	m.Chunks[1].SHA256 = [32]byte{}
	l.Tables["ipam_spaces"] = false
	for i, p := range parts {
		b, err := v.Feed(uint32(i), p, 101+int64(i))
		if err != nil {
			t.Fatal(err)
		}
		b[0] ^= 1
		if b[0] == p[0] {
			t.Fatal("returned bytes alias input")
		}
	}
	result, err := v.VerifyComplete(103)
	if err != nil {
		t.Fatal(err)
	}
	if result.Bytes != 12 || result.Chunks != 2 || result.DeclaredRows != 1 || result.RowsVerified {
		t.Fatalf("bad result %+v", result)
	}
	if _, err := v.VerifyComplete(104); err == nil {
		t.Fatal("second completion accepted")
	}
}
func TestManifestRejectsInvalidTrustAndLimits(t *testing.T) {
	tests := map[string]func(*Manifest, *Trust, *Limits){
		"unauthenticated": func(_ *Manifest, t *Trust, _ *Limits) { t.Authenticated = false },
		"cluster":         func(m *Manifest, _ *Trust, _ *Limits) { m.Cluster[1] = 1 }, "leader": func(m *Manifest, _ *Trust, _ *Limits) { m.Leader[1] = 1 },
		"snapshot": func(m *Manifest, _ *Trust, _ *Limits) { m.Snapshot[1] = 1 }, "anchor": func(m *Manifest, _ *Trust, _ *Limits) { m.Anchor.Seq = 1 },
		"invalid genesis": func(m *Manifest, t *Trust, _ *Limits) { m.Anchor.Hash[0] = 1; t.Anchor = m.Anchor }, "epoch zero": func(m *Manifest, t *Trust, _ *Limits) { m.Anchor.Epoch = 0; t.Anchor = m.Anchor },
		"schema": func(m *Manifest, _ *Trust, _ *Limits) { m.Schema++ }, "encoding": func(m *Manifest, _ *Trust, _ *Limits) { m.Encoding = 2 },
		"expired": func(m *Manifest, _ *Trust, _ *Limits) { m.RetentionUntilMS = 100 }, "total": func(m *Manifest, _ *Trust, _ *Limits) { m.TotalBytes++ },
		"byte limit": func(_ *Manifest, _ *Trust, l *Limits) { l.MaxBytes = 11 }, "chunk limit": func(_ *Manifest, _ *Trust, l *Limits) { l.MaxChunks = 1 },
		"chunk index": func(m *Manifest, _ *Trust, _ *Limits) { m.Chunks[1].Index = 0 }, "zero chunk": func(m *Manifest, _ *Trust, _ *Limits) { m.Chunks[0].Size = 0 },
		"long chunk":    func(m *Manifest, _ *Trust, _ *Limits) { m.Chunks[0].Size = MaxChunkBytes + 1 },
		"unknown table": func(m *Manifest, _ *Trust, _ *Limits) { m.Tables[0].Name = "secret" }, "duplicate table": func(m *Manifest, _ *Trust, l *Limits) {
			l.Tables["other"] = true
			m.Tables = append(m.Tables, m.Tables[0])
		},
		"row limit": func(m *Manifest, _ *Trust, _ *Limits) { m.Tables[0].Rows = 101 }, "hard config": func(_ *Manifest, _ *Trust, l *Limits) { l.MaxChunks = MaxConfiguredChunks + 1 },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			m, tr, l, _ := fixture()
			mutate(&m, &tr, &l)
			if _, err := New(m, tr, l, 100); err == nil {
				t.Fatal("accepted invalid manifest")
			}
		})
	}
}
func TestChunkErrorsPermanentlyDirty(t *testing.T) {
	tests := map[string]func(*Verifier, [][]byte) error{
		"out of order":   func(v *Verifier, b [][]byte) error { _, err := v.Feed(1, b[1], 101); return err },
		"short":          func(v *Verifier, b [][]byte) error { _, err := v.Feed(0, b[0][:2], 101); return err },
		"long":           func(v *Verifier, b [][]byte) error { _, err := v.Feed(0, append(b[0], 0), 101); return err },
		"digest":         func(v *Verifier, b [][]byte) error { b[0][0] ^= 1; _, err := v.Feed(0, b[0], 101); return err },
		"expired":        func(v *Verifier, b [][]byte) error { _, err := v.Feed(0, b[0], 200); return err },
		"clock rollback": func(v *Verifier, b [][]byte) error { _, err := v.Feed(0, b[0], 99); return err },
		"truncated":      func(v *Verifier, b [][]byte) error { _, err := v.VerifyComplete(101); return err },
		"replay": func(v *Verifier, b [][]byte) error {
			if _, err := v.Feed(0, b[0], 101); err != nil {
				return err
			}
			_, err := v.Feed(0, b[0], 102)
			return err
		},
	}
	for name, fail := range tests {
		t.Run(name, func(t *testing.T) {
			m, tr, l, b := fixture()
			v, err := New(m, tr, l, 100)
			if err != nil {
				t.Fatal(err)
			}
			if err = fail(v, b); err == nil {
				t.Fatal("invalid chunk accepted")
			}
			if _, err = v.Feed(0, b[0], 102); !errors.Is(err, ErrDirty) {
				t.Fatalf("not dirty: %v", err)
			}
			if _, err = v.VerifyComplete(102); !errors.Is(err, ErrDirty) {
				t.Fatalf("completion not dirty: %v", err)
			}
		})
	}
}
func TestExpiryBeforeCompletionRejects(t *testing.T) {
	m, tr, l, b := fixture()
	v, err := New(m, tr, l, 100)
	if err != nil {
		t.Fatal(err)
	}
	for i, p := range b {
		if _, err = v.Feed(uint32(i), p, 101); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = v.VerifyComplete(200); err == nil {
		t.Fatal("expired manifest accepted")
	}
}
func TestConcurrentDuplicateFeedsFailClosed(t *testing.T) {
	m, tr, l, b := fixture()
	v, err := New(m, tr, l, 100)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := v.Feed(0, b[0], 101); results <- err }()
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
		t.Fatalf("successes %d", success)
	}
	if _, err := v.Feed(1, b[1], 102); !errors.Is(err, ErrDirty) {
		t.Fatalf("not dirty %v", err)
	}
}
