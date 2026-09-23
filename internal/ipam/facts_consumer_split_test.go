package ipam

import (
	"database/sql"
	"testing"

	goddiassets "github.com/jasonwa/goddi"
	"github.com/jasonwa/goddi/internal/facts"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

func TestFactsConsumerUsesControlSideInboxWithSeparateProducerDatabase(t *testing.T) {
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

	// The producer and control databases are separate. A producer outbox must
	// not be handed directly to the projection consumer.
	if producer == control {
		t.Fatal("test setup requires separate database handles")
	}
	goose.SetBaseFS(goddiassets.Migrations())
	defer goose.SetBaseFS(nil)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err := goose.Up(control, "."); err != nil {
		t.Fatal(err)
	}
	producerOutbox, err := facts.NewObservationOutbox(producer)
	if err != nil {
		t.Fatal(err)
	}
	linkage := NewLinkage(control)
	if _, err := NewFactsConsumer(linkage, producerOutbox); err == nil {
		t.Fatal("producer outbox was accepted as the control-side inbox")
	}
	controlInbox, err := facts.NewObservationOutbox(control)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewFactsConsumer(linkage, controlInbox); err != nil {
		t.Fatalf("control-side inbox was rejected: %v", err)
	}
}
