package forwarder

import "testing"

func TestForwardersOrderedByPriority(t *testing.T) {
	fg := NewForwarderGroup(StrategySequential, 0)
	fg.SetForwarders([]*Forwarder{
		{ID: "last", Priority: 100},
		{ID: "first", Priority: 10},
		{ID: "same-first", Priority: 20},
		{ID: "same-second", Priority: 20},
	})

	got := fg.GetForwarders()
	want := []string{"first", "same-first", "same-second", "last"}
	if len(got) != len(want) {
		t.Fatalf("forwarder count = %d, want %d", len(got), len(want))
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("forwarder at %d = %q, want %q", i, got[i].ID, id)
		}
	}
}

func TestAddForwarderMaintainsPriorityOrder(t *testing.T) {
	fg := NewForwarderGroup(StrategySequential, 0)
	for _, fwd := range []*Forwarder{
		{ID: "high", Address: "8.8.8.8:53", Enabled: true, Priority: 100},
		{ID: "low", Address: "1.1.1.1:53", Enabled: true, Priority: 1},
	} {
		if err := fg.AddForwarder(fwd); err != nil {
			t.Fatalf("AddForwarder(%s): %v", fwd.ID, err)
		}
	}

	got := fg.GetForwarders()
	if got[0].ID != "low" || got[1].ID != "high" {
		t.Fatalf("priorities not ordered: %q, %q", got[0].ID, got[1].ID)
	}
}
