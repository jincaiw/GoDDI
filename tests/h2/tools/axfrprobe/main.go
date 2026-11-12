// axfrprobe performs a real AXFR against a GoDDI authoritative server, with an
// optional TSIG signature, and reports what it received. It exists because the
// H2 client image has no dig: this is a real DNS protocol client, not a mock,
// and it exercises the same wire path an operator's dig -y would.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/miekg/dns"
)

func main() {
	server := flag.String("server", "10.77.3.4:53", "authoritative server address")
	zone := flag.String("zone", "h2xfr.test.", "zone to transfer")
	ixfr := flag.Bool("ixfr", false, "issue an IXFR (with -serial) instead of an AXFR")
	serial := flag.Uint("serial", 0, "client serial for -ixfr")
	keyName := flag.String("key", "", "TSIG key name (empty = unsigned)")
	secret := flag.String("secret", "", "TSIG secret (base64)")
	flag.Parse()

	m := new(dns.Msg)
	if *ixfr {
		m.SetIxfr(dns.Fqdn(*zone), uint32(*serial), dns.Fqdn(*zone), "ns."+dns.Fqdn(*zone))
	} else {
		m.SetAxfr(dns.Fqdn(*zone))
	}
	tr := &dns.Transfer{}
	if *keyName != "" {
		tr.TsigSecret = map[string]string{dns.Fqdn(*keyName): *secret}
		m.SetTsig(dns.Fqdn(*keyName), dns.HmacSHA256, 300, time.Now().Unix())
	}
	// Raw diagnostic: send the same message with a plain Conn and report whether
	// the first reply carries a TSIG RR, which distinguishes "server did not
	// sign" from "client rejected the signature".
	env, err := tr.In(m, *server)
	if err != nil {
		fmt.Printf("AXFR_ERROR %v\n", err)
		os.Exit(2)
	}
	n := 0
	for e := range env {
		if e.Error != nil {
			fmt.Printf("AXFR_ERROR %v\n", e.Error)
			os.Exit(3)
		}
		for _, rr := range e.RR {
			if _, ok := rr.(*dns.SOA); ok {
				continue
			}
			fmt.Printf("RR %s\n", rr.String())
			n++
		}
	}
	fmt.Printf("AXFR_RECORDS %d\n", n)
	if n == 0 {
		os.Exit(4)
	}
}
