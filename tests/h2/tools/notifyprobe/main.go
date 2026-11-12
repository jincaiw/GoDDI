// notifyprobe sends a real DNS NOTIFY (RFC 1996) and reports the answer.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/miekg/dns"
)

func main() {
	server := flag.String("server", "127.0.0.1:53", "authoritative server address")
	zone := flag.String("zone", "h2xfr.test.", "zone to notify about")
	flag.Parse()
	m := new(dns.Msg)
	m.SetNotify(dns.Fqdn(*zone))
	c, err := dns.DialTimeout("udp", *server, 3*time.Second)
	if err != nil {
		fmt.Printf("NOTIFY_ERROR %v\n", err)
		os.Exit(2)
	}
	defer c.Close()
	if err := c.WriteMsg(m); err != nil {
		fmt.Printf("NOTIFY_ERROR %v\n", err)
		os.Exit(3)
	}
	r, err := c.ReadMsg()
	if err != nil {
		fmt.Printf("NOTIFY_ERROR %v\n", err)
		os.Exit(4)
	}
	if r.Rcode != dns.RcodeSuccess {
		fmt.Printf("NOTIFY_ERROR rcode=%d\n", r.Rcode)
		os.Exit(5)
	}
	fmt.Printf("NOTIFY_OK rcode=%d answers=%d\n", r.Rcode, len(r.Answer))
}
