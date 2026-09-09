package system

import (
	"sync"
	"testing"
)

func TestNotifyPreservesUpdateOrder(t *testing.T) {
	m := NewManager(nil)
	var mu sync.Mutex
	got := make([]string, 0, 2)
	done := make(chan struct{})
	m.Subscribe(func(key, value string) {
		mu.Lock()
		got = append(got, key+"="+value)
		if len(got) == 2 {
			close(done)
		}
		mu.Unlock()
	})

	m.notify("first", "1")
	m.notify("second", "2")
	<-done

	mu.Lock()
	defer mu.Unlock()
	if got[0] != "first=1" || got[1] != "second=2" {
		t.Fatalf("notification order = %#v, want first then second", got)
	}
}
