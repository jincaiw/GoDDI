package ipamresponse

import (
	"encoding/json"
	"testing"
)

func TestStaticMatchesAllocationContract(t *testing.T) {
	id := "11111111-1111-4111-8111-111111111111"
	subnet := "22222222-2222-4222-8222-222222222222"
	got, err := Static(id, subnet, "192.0.2.1", "owner")
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":"11111111-1111-4111-8111-111111111111","subnetId":"22222222-2222-4222-8222-222222222222","address":"192.0.2.1","ownerRef":"owner","kind":"STATIC","revision":"1"}`
	if got.Status != 201 || got.ETag != `"v1"` || string(got.Body) != want {
		t.Fatalf("unexpected response: %+v %s", got, got.Body)
	}
	var obj map[string]any
	if err := json.Unmarshal(got.Body, &obj); err != nil || len(obj) != 6 {
		t.Fatalf("response fields: %v %v", obj, err)
	}
}

func TestStaticRejectsInvalidFields(t *testing.T) {
	id := "11111111-1111-4111-8111-111111111111"
	for _, c := range []struct{ id, subnet, address, owner string }{
		{"bad", id, "192.0.2.1", "owner"},
		{id, id, "::ffff:192.0.2.1", "owner"},
		{id, id, "2001:0db8::1", "owner"},
		{id, id, "192.0.2.1", ""},
		{id, id, "192.0.2.1", "owner\n"},
	} {
		if _, err := Static(c.id, c.subnet, c.address, c.owner); err == nil {
			t.Fatalf("accepted invalid result: %+v", c)
		}
	}
}
