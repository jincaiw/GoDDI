// dnsprobe sends a real UDP or TCP DNS query from the H2 client container.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/miekg/dns"
)

func main() {
	server := flag.String("server", "10.77.3.4:53", "authoritative server")
	name := flag.String("name", "www.demo.test.", "qname")
	qtype := flag.String("type", "A", "qtype")
	tcp := flag.Bool("tcp", false, "use TCP")
	flag.Parse()
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(*name), dns.StringToType[*qtype])
	c := &dns.Client{Net: "udp", Timeout: 3 * time.Second, DialTimeout: 3 * time.Second}
	if *tcp {
		c.Net = "tcp"
	}
	r, _, err := c.Exchange(m, *server)
	if err != nil {
		fmt.Printf("QUERY_ERROR %v\n", err)
		os.Exit(2)
	}
	answers := []string{}
	for _, rr := range r.Answer {
		answers = append(answers, rr.String())
	}
	fmt.Printf("QUERY rcode=%s answers=%d aa=%v\n", dns.RcodeToString[r.Rcode], len(answers), r.Authoritative)
	for _, a := range answers {
		fmt.Println("ANSWER", a)
	}
	if len(answers) == 0 {
		os.Exit(4)
	}
}
