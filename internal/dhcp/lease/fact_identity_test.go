package lease

import "testing"

func TestNewMutationFactIdentityIsRetryStable(t *testing.T) {
	one, err := NewMutationFactIdentity("node-a", "space-1", MutationActivate, "lease-1", 2)
	if err != nil {
		t.Fatal(err)
	}
	two, err := NewMutationFactIdentity("node-a", "space-1", MutationActivate, "lease-1", 2)
	if err != nil {
		t.Fatal(err)
	}
	if one != two {
		t.Fatalf("identity changed on retry: %+v != %+v", one, two)
	}
	if err := one.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestNewMutationFactIdentityChangesAcrossMutationGeneration(t *testing.T) {
	one, err := NewMutationFactIdentity("node-a", "space-1", MutationRenew, "lease-1", 2)
	if err != nil {
		t.Fatal(err)
	}
	two, err := NewMutationFactIdentity("node-a", "space-1", MutationRenew, "lease-1", 3)
	if err != nil {
		t.Fatal(err)
	}
	if one.EventID == two.EventID {
		t.Fatal("different generations reused event ID")
	}
	three, err := NewMutationFactIdentity("node-a", "space-1", MutationRelease, "lease-1", 2)
	if err != nil {
		t.Fatal(err)
	}
	if one.EventID == three.EventID {
		t.Fatal("different mutations reused event ID")
	}
}

func TestNewMutationFactIdentityRejectsInvalidInputs(t *testing.T) {
	cases := []struct {
		name       string
		source     string
		space      string
		kind       MutationKind
		leaseID    string
		generation int64
	}{
		{"source", "", "space-1", MutationActivate, "lease-1", 1},
		{"space", "node-a", "", MutationActivate, "lease-1", 1},
		{"kind", "node-a", "space-1", MutationKind("unknown"), "lease-1", 1},
		{"lease", "node-a", "space-1", MutationActivate, "", 1},
		{"generation", "node-a", "space-1", MutationActivate, "lease-1", -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewMutationFactIdentity(tc.source, tc.space, tc.kind, tc.leaseID, tc.generation); err == nil {
				t.Fatal("invalid identity unexpectedly accepted")
			}
		})
	}
}
