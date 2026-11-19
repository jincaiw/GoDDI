package configrepl

import (
	"context"
	"strings"
	"testing"
)

// Snapshot reading distinguishes a table absent from this deployment from
// an empty table. An absent table must be omitted, so the follower skips it
// instead of deleting its local copy.
func TestReadSnapshotOmitsAbsentTableButKeepsEmptyTable(t *testing.T) {
	ctx := context.Background()
	src := newDB(t)
	// dhcp_scopes exists but is empty; ipam_spaces is absent.
	if _, err := src.Exec(schemaScopes); err != nil {
		t.Fatal(err)
	}
	snap, err := ReadSnapshot(ctx, src, "node-a", specs())
	if err != nil {
		t.Fatal(err)
	}
	var scopesSeen, spacesSeen bool
	for _, tbl := range snap.Tables {
		if tbl.Name == "dhcp_scopes" {
			scopesSeen = true
			if len(tbl.Rows) != 0 {
				t.Fatal("empty table carried rows")
			}
		}
		if tbl.Name == "ipam_spaces" {
			spacesSeen = true
		}
	}
	if !scopesSeen {
		t.Fatal("empty table missing from snapshot")
	}
	if spacesSeen {
		t.Fatal("absent table must be omitted, not carried as empty")
	}
}

// Large signed INTEGER values must not become float64 on the wire: a
// precision loss would silently change a replicated numeric column.
func TestSnapshotJSONPreservesLargeInteger(t *testing.T) {
	const big = "9007199254740993"
	blob := []byte(`{"version":1,"node_id":"node-a","tables":[{"name":"t","rows":[[` + big + `]]}]}`)
	snap, err := DecodeSnapshotJSON(blob)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := snap.Tables[0].Rows[0][0].(int64)
	if !ok {
		t.Fatalf("decoded type = %T, want int64", snap.Tables[0].Rows[0][0])
	}
	if got != 9007199254740993 {
		t.Fatalf("decoded = %d, want %s", got, big)
	}
	reencoded, err := EncodeSnapshotJSON(snap)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(reencoded, big) {
		t.Fatalf("large integer lost after re-encode: %s", reencoded)
	}
}

// A non-integral number must not be silently rounded during decode. Whether
// an INTEGER column rejects it later is a separate policy.
func TestSnapshotJSONKeepsFractionalNumberDistinct(t *testing.T) {
	blob := []byte(`{"version":1,"node_id":"node-a","tables":[{"name":"t","rows":[[1.5]]}]}`)
	snap, err := DecodeSnapshotJSON(blob)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := snap.Tables[0].Rows[0][0].(float64); !ok || got != 1.5 {
		t.Fatalf("decoded = %#v, want float64(1.5)", snap.Tables[0].Rows[0][0])
	}
}

func contains(b []byte, s string) bool {
	return len(b) >= len(s) && strings.Contains(string(b), s)
}
