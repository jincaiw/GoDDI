package ipam

// The lists in a response are declared as arrays, so a client is entitled to
// read them as arrays. A nil slice marshals to `null`, and a client that counts
// entries -- which is how the console reads all three of these -- fails on the
// first one.
//
// This is not hypothetical. `AddressView.conflicts` was null for every address
// with nothing to report, which is most of them, and a Vue render reading
// `.length` on it threw: the drawer came up with a title and a blank body, and
// the import dialog closed itself instead of showing the report. Both looked
// like the feature not working rather than like a shape mismatch.
//
// The guard is reflective on purpose. Hand-written assertions name the fields
// someone remembered, and the fields someone remembered here were the ones
// already spelled `[]T{}` -- the two that were broken were the two nobody
// listed.

import (
	"reflect"
	"strings"
	"testing"

	"github.com/jasonwa/goddi/internal/ipam/address"
)

// wireListFloor is how many lists each type carries. It is a floor rather than
// an observation: if a list is added, raise it. If the reflection below stops
// matching -- a renamed type, a tag edit, a field turned into a pointer -- the
// count drops under the floor and this says so, instead of the guard passing
// while watching an empty set.
var wireListFloor = map[string]int{
	"AddressView":   6,
	"ImportReport":  2,
	"DHCPScopePlan": 3,
}

// assertNoNilLists fails for every list that is nil and returns how many it
// looked at.
func assertNoNilLists(t *testing.T, what string, v any) int {
	t.Helper()

	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			t.Fatalf("%s: the builder returned nil", what)
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		t.Fatalf("%s: expected a struct, got %s", what, rv.Kind())
	}

	rt := rv.Type()
	checked := 0
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		if !field.IsExported() {
			continue
		}
		// Only the lists that are always on the wire. A field tagged
		// `omitempty` is absent when empty, and absence is not null: a client
		// reading it as optional is reading it correctly.
		tag, tagged := field.Tag.Lookup("json")
		if !tagged || tag == "-" || strings.Contains(tag, "omitempty") {
			continue
		}
		if rv.Field(i).Kind() != reflect.Slice {
			continue
		}

		checked++
		if rv.Field(i).IsNil() {
			t.Errorf("%s.%s is nil: it marshals to null, and \"no entries\" is not the same answer as \"no such field\"",
				what, field.Name)
		}
	}
	return checked
}

func TestNoResponseBuilderLeavesAListNil(t *testing.T) {
	db := newTestDB(t)
	seedSpaceAndSubnet(t, db, "sp1", "sn1", "192.0.2.0/24")
	// One address and nothing else. Every list on the view is empty here, and
	// empty is the only state in which a nil list can be seen: as soon as
	// something is in it, the slice is non-nil and the bug is invisible.
	seedAddress(t, db, "ad1", "sp1", "sn1", "192.0.2.10", address.StatusAvailable)

	linkage := NewLinkage(db)

	view, err := linkage.ViewAddress("sp1", "192.0.2.10")
	if err != nil {
		t.Fatalf("building the view: %v", err)
	}
	// A file with a header and no rows: no errors and nothing to change, which
	// is the report the dialog renders when a clean file is previewed.
	report, err := NewImportExport(db).PreviewAddressesCSV("sn1", []byte(importHeader+"\n"))
	if err != nil {
		t.Fatalf("previewing an empty file: %v", err)
	}
	plan, err := linkage.PlanDHCPScope("sn1")
	if err != nil {
		t.Fatalf("building the scope plan: %v", err)
	}

	for _, subject := range []struct {
		name  string
		value any
	}{
		{"AddressView", view},
		{"ImportReport", report},
		{"DHCPScopePlan", plan},
	} {
		floor, bounded := wireListFloor[subject.name]
		if !bounded {
			t.Errorf("%s has no floor in wireListFloor: a type added here without one is a type this guard silently skips",
				subject.name)
			continue
		}
		if checked := assertNoNilLists(t, subject.name, subject.value); checked < floor {
			t.Errorf("%s: matched %d list(s), floor is %d -- the guard is looking at fewer fields than the type has",
				subject.name, checked, floor)
		}
	}
}
