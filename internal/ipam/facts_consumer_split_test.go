package ipam

import (
	"database/sql"
	"testing"

	"github.com/jasonwa/goddi/internal/facts"
	_ "modernc.org/sqlite"
)

func TestFactsConsumerRejectsSeparateProducerAndControlDatabases(t *testing.T) {
	producer, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer producer.Close()
	control, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()

	outbox, err := facts.NewObservationOutbox(producer)
	if err != nil {
		t.Fatal(err)
	}
	linkage := NewLinkage(control)
	if _, err := NewFactsConsumer(linkage, outbox); err == nil {
		t.Fatal("split producer/control databases unexpectedly accepted")
	}
}
