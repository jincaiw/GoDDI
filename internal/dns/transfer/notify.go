package transfer

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/miekg/dns"
)

// notifyACL mirrors the zone ACL JSON stored in dns_zones.acl; only the
// Notify target list is needed for outbound NOTIFY.
type notifyACL struct {
	Notify []string `json:"notify"`
}

// SendNotifyForZone sends a NOTIFY (RFC 1996) for the given primary zone to
// all addresses configured in the zone's ACL notify list. The zone must be a
// primary zone with a notify target list; otherwise this is a no-op. Delivery
// is best-effort: each target gets one retry, failures are logged.
func SendNotifyForZone(db *sql.DB, zoneName string) {
	if db == nil || zoneName == "" {
		return
	}

	if !strings.HasSuffix(zoneName, ".") {
		zoneName += "."
	}
	zoneName = strings.ToLower(zoneName)

	var (
		ztype  string
		aclRaw sql.NullString
		serial uint32
	)
	err := db.QueryRow(
		"SELECT type, acl, serial FROM dns_zones WHERE name = ?", zoneName,
	).Scan(&ztype, &aclRaw, &serial)
	if err != nil {
		if err != sql.ErrNoRows {
			slog.Warn("notify: failed to load zone", "zone", zoneName, "error", err)
		}
		return
	}
	if ztype != "primary" || !aclRaw.Valid || aclRaw.String == "" {
		return
	}

	var acl notifyACL
	if err := json.Unmarshal([]byte(aclRaw.String), &acl); err != nil {
		slog.Warn("notify: failed to parse zone ACL", "zone", zoneName, "error", err)
		return
	}
	if len(acl.Notify) == 0 {
		return
	}

	// Build the NOTIFY message: opcode NOTIFY, SOA in question + answer
	// sections with the zone's current serial (RFC 1996 §3).
	soa := &dns.SOA{
		Hdr: dns.RR_Header{
			Name:   zoneName,
			Rrtype: dns.TypeSOA,
			Class:  dns.ClassINET,
		},
		Serial: serial,
	}
	msg := new(dns.Msg)
	msg.Id = dns.Id()
	msg.Opcode = dns.OpcodeNotify
	msg.Question = []dns.Question{
		{Name: zoneName, Qtype: dns.TypeSOA, Qclass: dns.ClassINET},
	}
	msg.Answer = []dns.RR{soa}

	for _, target := range acl.Notify {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		addr := target
		if _, _, err := net.SplitHostPort(addr); err != nil {
			addr = net.JoinHostPort(addr, "53")
		}
		go sendNotifyOnce(msg, zoneName, addr)
	}
}

// sendNotifyOnce delivers a single NOTIFY with one retry; responses are read
// briefly to let the secondary act, but failures are only logged.
func sendNotifyOnce(msg *dns.Msg, zoneName, addr string) {
	for attempt := 0; attempt < 2; attempt++ {
		client := &dns.Client{Net: "udp", Timeout: 3 * time.Second}
		resp, _, err := client.Exchange(msg, addr)
		if err == nil && resp != nil {
			slog.Info("notify: sent", "zone", zoneName, "target", addr, "rcode", dns.RcodeToString[resp.Rcode])
			return
		}
		if attempt == 0 {
			// Retry once over TCP before giving up (RFC 1996 §5.3).
			tcpClient := &dns.Client{Net: "tcp", Timeout: 5 * time.Second}
			if resp, _, err := tcpClient.Exchange(msg, addr); err == nil && resp != nil {
				slog.Info("notify: sent over TCP", "zone", zoneName, "target", addr)
				return
			}
			return
		}
	}
}

// NotifyTargets returns the configured NOTIFY targets for a zone (used by
// tests and the API layer to validate configuration).
func NotifyTargets(db *sql.DB, zoneName string) ([]string, error) {
	if db == nil || zoneName == "" {
		return nil, fmt.Errorf("invalid arguments")
	}
	if !strings.HasSuffix(zoneName, ".") {
		zoneName += "."
	}
	var aclRaw sql.NullString
	err := db.QueryRow("SELECT acl FROM dns_zones WHERE name = ?", strings.ToLower(zoneName)).Scan(&aclRaw)
	if err != nil {
		return nil, err
	}
	if !aclRaw.Valid || aclRaw.String == "" {
		return nil, nil
	}
	var acl notifyACL
	if err := json.Unmarshal([]byte(aclRaw.String), &acl); err != nil {
		return nil, err
	}
	return acl.Notify, nil
}
