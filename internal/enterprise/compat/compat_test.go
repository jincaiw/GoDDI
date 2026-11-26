package compat

import (
	"errors"
	"reflect"
	"testing"

	"github.com/jincaiw/GoDDI/internal/enterprise/eventlog"
	"github.com/jincaiw/GoDDI/internal/enterprise/snapshotverify"
)

func TestCandidateConstantsArePinnedToOwningPackages(t *testing.T) {
	if EventWireSchemaV1 != eventlog.SchemaVersionV1 {
		t.Fatalf("event wire schema = %d, want %d", EventWireSchemaV1, eventlog.SchemaVersionV1)
	}
	if SnapshotEncodingV1 != snapshotverify.EncodingNDJSONV1 {
		t.Fatalf("snapshot encoding = %d, want %d", SnapshotEncodingV1, snapshotverify.EncodingNDJSONV1)
	}
	// Variables (not constants) so vet does not fold the comparison: the
	// premise of the "same number, different axis" rule is that these really
	// are equal today, yet control different objects.
	var wire, enc uint32 = eventlog.SchemaVersionV1, snapshotverify.EncodingNDJSONV1
	if wire != enc {
		t.Fatalf("test premise broken: wire=%d encoding=%d", wire, enc)
	}
}

// The contract's first rule: equal numbers on different axes are not
// interchangeable. Axes() must make that visible and must never expose a
// helper that conflates them.
func TestAxesAreIndependentAndDescriptive(t *testing.T) {
	axes := Axes()
	if len(axes) != 6 {
		t.Fatalf("axes = %d, want 6", len(axes))
	}
	seenName := make(map[string]struct{}, len(axes))
	seenControls := make(map[string]struct{}, len(axes))
	for _, a := range axes {
		if a.Name == "" || a.Current == "" || a.Accepted == "" || a.Controls == "" {
			t.Fatalf("incomplete axis %+v", a)
		}
		if _, dup := seenName[a.Name]; dup {
			t.Fatalf("duplicate axis name %q", a.Name)
		}
		seenName[a.Name] = struct{}{}
		if _, dup := seenControls[a.Controls]; dup {
			t.Fatalf("two axes claim to control %q", a.Controls)
		}
		seenControls[a.Controls] = struct{}{}
	}
	// Several axes legitimately read "1", but they control different objects.
	if axes[0].Controls == axes[1].Controls || axes[1].Controls == axes[2].Controls {
		t.Fatal("an axis lost its distinct scope")
	}
}

func TestNegotiateReturnsSortedIntersection(t *testing.T) {
	got, err := Negotiate([]uint32{3, 1, 2}, []uint32{2, 3, 4})
	if err != nil {
		t.Fatal(err)
	}
	if want := []uint32{2, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("negotiated = %v, want %v", got, want)
	}
}

func TestNegotiateRejectsMalformedOrDisjointSets(t *testing.T) {
	for _, tc := range []struct {
		name              string
		offered, accepted []uint32
		want              error
	}{
		{"empty offered", nil, []uint32{1}, ErrInvalid},
		{"empty accepted", []uint32{1}, nil, ErrInvalid},
		{"zero offered", []uint32{0}, []uint32{1}, ErrInvalid},
		{"zero accepted", []uint32{1}, []uint32{0}, ErrInvalid},
		{"duplicate offered", []uint32{1, 1}, []uint32{1}, ErrInvalid},
		{"duplicate accepted", []uint32{1}, []uint32{1, 1}, ErrInvalid},
		{"no common version", []uint32{2}, []uint32{1}, ErrIncompatible},
		{"disjoint sets", []uint32{5, 6}, []uint32{7, 8}, ErrIncompatible},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Negotiate(tc.offered, tc.accepted)
			if !errors.Is(err, tc.want) || got != nil {
				t.Fatalf("got=%v err=%v, want %v with nil result", got, err, tc.want)
			}
		})
	}
}

func TestNegotiateNeverFallsBackToADefault(t *testing.T) {
	// A peer offering only an unknown version must not be served v1 anyway.
	got, err := Negotiate(acceptedEventWireSchemas, []uint32{99})
	if !errors.Is(err, ErrIncompatible) || got != nil {
		t.Fatalf("unknown peer version silently downgraded: got=%v err=%v", got, err)
	}
}

func TestValidateReadableSchemas(t *testing.T) {
	for _, tc := range []struct {
		name     string
		readable []uint32
		wantErr  error
	}{
		{"v1 only", []uint32{1}, nil},
		{"v1 plus unknown", []uint32{1, 2}, nil},
		{"unknown only", []uint32{2}, ErrIncompatible},
		{"empty", nil, ErrInvalid},
		{"zero", []uint32{0}, ErrInvalid},
		{"duplicate", []uint32{1, 1}, ErrInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateReadableSchemas(tc.readable)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected rejection: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestAcceptedEventWireSchemasReturnsACopy(t *testing.T) {
	a := AcceptedEventWireSchemas()
	if len(a) == 0 {
		t.Fatal("empty acceptance set")
	}
	a[0] = 99
	b := AcceptedEventWireSchemas()
	if b[0] == 99 {
		t.Fatal("caller mutated the shared acceptance set")
	}
}
