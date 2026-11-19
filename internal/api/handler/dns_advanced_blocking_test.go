package handler

import (
	"testing"
	"time"
)

func TestBlockingUntilValueOmitsZeroTime(t *testing.T) {
	if got := blockingUntilValue(time.Time{}); got != nil {
		t.Fatalf("zero time = %v, want nil", got)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if got := blockingUntilValue(now); got != now {
		t.Fatalf("non-zero time = %v, want %v", got, now)
	}
}
