// udpqps is a minimal UDP DNS load generator used to benchmark the GoDDI
// cache data plane. It sends A queries for a fixed qname from N parallel
// workers for a given duration (or count) and reports QPS and error rate.
//
// Usage:
//
//	go run ./scripts/bench/udpqps.go -server 127.0.0.1:53 \
//	    -qname www.example.com. -workers 8 -duration 30s
//
// Typical baselines (macOS arm64, cache enabled, warm cache):
//   - 1 worker,   cache hit:  ~29,000 QPS
//   - 32 workers, cache hit:  ~39,700 QPS
//   - 64 workers, cache hit:  ~38,100 QPS (client-side saturation)
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/miekg/dns"
)

func main() {
	server := flag.String("server", "127.0.0.1:53", "DNS server address (host:port)")
	qname := flag.String("qname", "www.example.com.", "query name to resolve")
	workers := flag.Int("workers", 8, "number of parallel workers")
	duration := flag.Duration("duration", 30*time.Second, "benchmark duration")
	count := flag.Int64("count", 0, "stop after this many queries (0 = use duration)")
	timeout := flag.Duration("timeout", 2*time.Second, "per-query timeout")
	flag.Parse()

	if *workers < 1 {
		*workers = 1
	}

	var sent, ok, failed atomic.Int64
	stop := make(chan struct{})

	// Allow Ctrl-C to terminate early and still print results.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		close(stop)
	}()

	deadline := time.Now().Add(*duration)
	start := time.Now()

	var wg sync.WaitGroup
	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go func(seed int64) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(seed))
			c := &dns.Client{Net: "udp", Timeout: *timeout, UDPSize: 512}
			conn, err := c.Dial(*server)
			if err != nil {
				fmt.Fprintf(os.Stderr, "worker dial: %v\n", err)
				return
			}
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(*timeout))
			for {
				select {
				case <-stop:
					return
				default:
				}
				if *count > 0 && sent.Load() >= *count {
					return
				}
				if *count == 0 && time.Now().After(deadline) {
					return
				}

				// Randomize the case of a few letters so responses with
				// question-case echo do not hit any naive dedup layer.
				name := *qname
				if rng.Intn(4) == 0 {
					b := []byte(name)
					if b[0] >= 'a' && b[0] <= 'z' {
						b[0] -= 32
					}
					name = string(b)
				}

				m := new(dns.Msg)
				m.SetQuestion(name, dns.TypeA)
				m.Id = uint16(rng.Intn(65536))

				sent.Add(1)
				resp, _, err := c.ExchangeWithConn(m, conn)
				if err != nil || resp == nil {
					failed.Add(1)
					continue
				}
				if resp.Rcode != dns.RcodeSuccess && resp.Rcode != dns.RcodeNameError {
					failed.Add(1)
					continue
				}
				ok.Add(1)
			}
		}(int64(i))
	}
	wg.Wait()

	elapsed := time.Since(start).Seconds()
	success := ok.Load()
	fail := failed.Load()
	fmt.Printf("server=%s workers=%d elapsed=%.1fs sent=%d ok=%d failed=%d qps=%.0f success_rate=%.2f%%\n",
		*server, *workers, elapsed, sent.Load(), success, fail,
		float64(success+fail)/elapsed,
		100*float64(success)/float64(success+fail))
}
