package pullresponse

import (
	"bytes"
	"context"
	"sync"
	"testing"

	"github.com/jincaiw/GoDDI/internal/enterprise/eventlog"
)

func TestConcurrentBuildCopiesResults(t *testing.T) {
	db, l, cl, leader := fixture(t)
	c, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	seed(t, c, cl, leader, 1, eventlog.GenesisHash(), 300)
	in := get(t, l, cl, leader, eventlog.Position{Epoch: 1}, 1)
	want, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 25; j++ {
				b, e := Build(in)
				if e != nil || !bytes.Equal(b, want) {
					t.Errorf("build=%v", e)
					return
				}
				b[0] ^= 1
			}
		}()
	}
	wg.Wait()
}
