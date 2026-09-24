package dataplane

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/facts"
	"github.com/jasonwa/goddi/internal/ipam"
	"github.com/jasonwa/goddi/internal/ipam/address"
	"github.com/jasonwa/goddi/internal/ipam/space"
	"github.com/jasonwa/goddi/internal/ipam/subnet"
)

func TestDHCPFactRelaysFromLeaseStoreIntoControlIPAMProjection(t *testing.T) {
	producer := newStore(t, config.DataPlaneLease)
	control := newControlDB(t)
	defer control.Close()

	spaceRow, err := space.NewManager(control).CreateSpace("corp", "integration")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := subnet.NewManager(control).CreateSubnet(spaceRow.ID, "test", "192.0.2.0/29", subnet.SubnetOptions{}); err != nil {
		t.Fatal(err)
	}

	producerOutbox, err := facts.NewObservationOutbox(producer.DB)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(ipam.LeaseObservationFact{
		Action: "bind", LeaseID: "lease-e2e", ScopeID: "scope-e2e", SpaceID: spaceRow.ID,
		IP: "192.0.2.2", MAC: "aa:bb:cc:dd:ee:42", Hostname: "host-e2e",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := producerOutbox.Enqueue(context.Background(), facts.Envelope{
		EventID: "fact-e2e", Version: facts.CurrentEnvelopeVersion, Entity: "dhcp_lease", Action: "bind",
		Generation: 1, Sequence: 1, Source: "dhcp-node-e2e", OccurredAt: time.Now().UTC(),
		PayloadVersion: 1, Payload: payload,
	}); err != nil {
		t.Fatal(err)
	}

	replicator := NewReplicator(control, producer)
	if n, err := replicator.PushFacts(context.Background(), 10); err != nil || n != 1 {
		t.Fatalf("PushFacts = %d, %v; want 1, nil", n, err)
	}
	controlInbox, err := facts.NewObservationOutbox(control)
	if err != nil {
		t.Fatal(err)
	}
	consumer, err := ipam.NewFactsConsumer(ipam.NewLinkage(control), controlInbox)
	if err != nil {
		t.Fatal(err)
	}
	applied, err := consumer.ProcessOne(context.Background())
	if err != nil || !applied {
		t.Fatalf("ProcessOne = %v, %v; want true, nil", applied, err)
	}

	projected, err := address.NewManager(control).GetAddressBySpaceIP(spaceRow.ID, "192.0.2.2")
	if err != nil {
		t.Fatal(err)
	}
	if projected == nil || projected.Status != address.StatusDHCP || projected.ObservedState != address.ObservedInUse {
		t.Fatalf("IPAM projection = %+v, want DHCP/in-use", projected)
	}
	var status string
	if err := control.QueryRow(`SELECT status FROM dhcp_ipam_observation_events WHERE event_id='fact-e2e'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "done" {
		t.Fatalf("control inbox status = %q, want done", status)
	}
	if pending, err := replicator.PendingFacts(); err != nil || pending != 0 {
		t.Fatalf("producer pending facts = %d, %v; want 0, nil", pending, err)
	}
}
