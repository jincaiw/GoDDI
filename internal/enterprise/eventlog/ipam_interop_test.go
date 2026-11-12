package eventlog_test

import (
	"testing"

	"github.com/jasonwa/goddi/internal/enterprise/eventlog"
	"github.com/jasonwa/goddi/internal/enterprise/ipamevent"
)

func TestStaticCandidateInteroperatesWithReplicaDecoder(t *testing.T) {
	o := eventlog.StaticOccupancy{TenantID: "11111111-1111-4111-8111-111111111111", SpaceID: "22222222-2222-4222-8222-222222222222", SubnetID: "33333333-3333-4333-8333-333333333333", ID: "44444444-4444-4444-8444-444444444444", Family: 4, Address: [16]byte{12: 192, 14: 2, 15: 10}, OwnerRef: "asset:one", CreatedAtMS: 1}
	c, err := o.Change()
	if err != nil {
		t.Fatal(err)
	}
	payload, err := eventlog.MarshalChanges(c)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := ipamevent.DecodePayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded) != 1 || decoded[0].Occupancy.TenantID != o.TenantID || decoded[0].Occupancy.SpaceID != o.SpaceID || decoded[0].Occupancy.SubnetID != o.SubnetID || decoded[0].Occupancy.ID != o.ID || decoded[0].Occupancy.Address != "192.0.2.10" || decoded[0].Occupancy.OwnerRef != o.OwnerRef || decoded[0].Occupancy.Kind != "STATIC" || decoded[0].Occupancy.CreatedAtMS != "1" || decoded[0].Occupancy.Revision != "1" {
		t.Fatalf("unexpected event: %+v", decoded)
	}
}
