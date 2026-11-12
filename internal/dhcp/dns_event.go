package dhcp

// DNSEventAction is the kind of change a lease transition owes the DNS side.
type DNSEventAction string

const (
	// DNSEventCreate publishes (or refreshes) the A/PTR records for a binding.
	DNSEventCreate DNSEventAction = "create"
	// DNSEventDelete withdraws them because the binding no longer holds the
	// address.
	DNSEventDelete DNSEventAction = "delete"
)

// DNSEvent is one DNS change owed by a DHCP binding.
//
// The unified facts chain derives this from durable lease facts in
// DNSFactsConsumer; the event carries the lease generation it was decided
// at, so applying it checks that generation against the lease's current value:
// a create replayed after the binding was torn down does not resurrect the
// record, and a delete replayed after a renewal does not remove the live one.
type DNSEvent struct {
	ID         int64
	LeaseID    string
	Generation int64
	Action     DNSEventAction
	ScopeID    string
	IPAddress  string
	MACAddress string
	Hostname   string
	Attempts   int
}
