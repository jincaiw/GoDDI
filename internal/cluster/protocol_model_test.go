package cluster

import "testing"

func TestProtocolModelRejectsStaleEpochAndMinority(t *testing.T) {
	model := NewProtocolModel(7, "node-a", 2)
	model.SetVotes(1)
	if _, err := model.Publish(PublishRequest{PublishID: "p-minority", Epoch: 7, Leader: "node-a"}); err == nil {
		t.Fatal("minority partition committed a publish")
	}
	model.SetVotes(2)
	if _, err := model.Publish(PublishRequest{PublishID: "p-stale", Epoch: 6, Leader: "node-a"}); err == nil {
		t.Fatal("stale epoch committed a publish")
	}
}

func TestProtocolModelPublishIDIsIdempotent(t *testing.T) {
	model := NewProtocolModel(3, "node-a", 2)
	model.SetVotes(2)
	first, err := model.Publish(PublishRequest{PublishID: "p-1", Epoch: 3, Leader: "node-a"})
	if err != nil {
		t.Fatalf("first publish: %v", err)
	}
	second, err := model.Publish(PublishRequest{PublishID: "p-1", Epoch: 3, Leader: "node-a"})
	if err != nil {
		t.Fatalf("replay publish: %v", err)
	}
	if first != second || second.Revision != 1 || second.Seq != 1 {
		t.Fatalf("replay = %+v, first = %+v; want one logical revision", second, first)
	}
}

func TestProtocolModelWatermarkGapPauses(t *testing.T) {
	model := NewProtocolModel(1, "node-a", 1)
	if err := model.Apply("node-b", 2); err == nil {
		t.Fatal("watermark gap was accepted")
	}
	if err := model.Apply("node-b", 1); err != nil {
		t.Fatalf("apply first sequence: %v", err)
	}
	if err := model.Apply("node-b", 1); err != nil {
		t.Fatalf("duplicate sequence should be idempotent: %v", err)
	}
	if err := model.Apply("node-b", 3); err == nil {
		t.Fatal("second watermark gap was accepted")
	}
}

func TestProtocolModelFencedNodeNeedsCleanupBeforeRejoin(t *testing.T) {
	model := NewProtocolModel(1, "node-a", 1)
	model.FenceNode("node-b")
	if err := model.RejoinNode("node-b"); err == nil {
		t.Fatal("fenced node rejoined without cleanup")
	}
	model.CleanNode("node-b")
	if err := model.RejoinNode("node-b"); err != nil {
		t.Fatalf("rejoin after cleanup: %v", err)
	}
}

func TestProtocolModelUnknownResultCanBeQueriedWithoutRepublish(t *testing.T) {
	model := NewProtocolModel(2, "node-a", 1)
	model.SetVotes(1)
	first, err := model.Publish(PublishRequest{PublishID: "p-unknown", Epoch: 2, Leader: "node-a"})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if err := model.MarkUnknown("p-unknown"); err != nil {
		t.Fatalf("mark unknown: %v", err)
	}
	queried, ok := model.Query("p-unknown")
	if !ok || queried.Status != PublishUnknown || queried.Revision != first.Revision {
		t.Fatalf("query = %+v, found=%v", queried, ok)
	}
	replayed, err := model.Publish(PublishRequest{PublishID: "p-unknown", Epoch: 2, Leader: "node-a"})
	if err != nil {
		t.Fatalf("query/retry: %v", err)
	}
	if replayed != queried {
		t.Fatalf("retry created or changed logical result: %+v vs %+v", replayed, queried)
	}
}
