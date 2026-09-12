package zone

import "testing"

// The expired set gates authoritative answers, so name handling must be
// case- and dot-insensitive: the sweeper reports the zone name as stored while
// lookups arrive as query names.
func TestStore_ZoneExpiry_MarksAndClears(t *testing.T) {
	s := NewStore(nil) // db nil: expiry bookkeeping is independent of the DB

	if s.IsZoneExpired("example.com.") {
		t.Fatal("zone reported expired before being marked")
	}

	s.MarkZoneExpired("Example.COM")

	if !s.IsZoneExpired("example.com.") {
		t.Error("zone not reported expired as an FQDN after marking without a trailing dot")
	}
	if !s.IsZoneExpired("EXAMPLE.com.") {
		t.Error("expiry check is case-sensitive")
	}
	if s.IsZoneExpired("other.example.com.") {
		t.Error("unrelated zone reported expired")
	}

	got := s.ExpiredZones()
	if len(got) != 1 || got[0] != "example.com." {
		t.Errorf("ExpiredZones() = %v, want [example.com.]", got)
	}

	s.ClearZoneExpired("example.com")

	if s.IsZoneExpired("example.com.") {
		t.Error("zone still reported expired after ClearZoneExpired")
	}
	if len(s.ExpiredZones()) != 0 {
		t.Errorf("ExpiredZones() = %v, want empty", s.ExpiredZones())
	}
}

// The sweep calls MarkZoneExpired on every pass while a zone stays expired, so
// marking must be idempotent and must not grow the set.
func TestStore_ZoneExpiry_MarkIsIdempotent(t *testing.T) {
	s := NewStore(nil)

	for i := 0; i < 5; i++ {
		s.MarkZoneExpired("sec.example.com.")
	}

	if got := s.ExpiredZones(); len(got) != 1 {
		t.Fatalf("ExpiredZones() = %v, want exactly one entry", got)
	}
}

// Clearing a zone that was never marked, or marking with an empty name, must
// be harmless no-ops rather than creating phantom entries.
func TestStore_ZoneExpiry_EmptyAndUnknownNamesAreNoOps(t *testing.T) {
	s := NewStore(nil)

	s.MarkZoneExpired("")
	s.ClearZoneExpired("")
	s.ClearZoneExpired("never-marked.example.com.")

	if got := s.ExpiredZones(); len(got) != 0 {
		t.Errorf("ExpiredZones() = %v, want empty", got)
	}
	if s.IsZoneExpired("") {
		t.Error("empty zone name reported expired")
	}
}
