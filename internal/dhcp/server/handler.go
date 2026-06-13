package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"regexp"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/jasonwa/goddi/internal/dhcp/lease"
	"github.com/jasonwa/goddi/internal/dhcp/option"
	"github.com/jasonwa/goddi/internal/dhcp/scope"

	dhcpinternal "github.com/jasonwa/goddi/internal/dhcp"
)

// fqdnLabelRegex matches a single valid DNS label per RFC 1035 §2.3.4.
// Labels are 1-63 characters of letters, digits, and hyphens, not starting
// or ending with a hyphen.
var fqdnLabelRegex = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)

// maxFQDNLength is the maximum total length of a fully-qualified domain name
// (RFC 1035 §2.3.4 limits a domain name to 255 octets in DNS wire format).
const maxFQDNLength = 253

// validateFQDN returns the FQDN unchanged if it is well-formed, or an empty
// string if the FQDN should be rejected. See RFC 1035 §2.3.4 for label
// length and character rules.
func validateFQDN(fqdn string) string {
	if fqdn == "" || len(fqdn) > maxFQDNLength {
		return ""
	}
	// Strip optional trailing dot.
	fqdn = trimTrailingDot(fqdn)
	if fqdn == "" {
		return ""
	}
	labels := splitLabels(fqdn)
	if len(labels) == 0 {
		return ""
	}
	for _, l := range labels {
		if !fqdnLabelRegex.MatchString(l) {
			return ""
		}
	}
	return fqdn
}

func trimTrailingDot(s string) string {
	if len(s) > 0 && s[len(s)-1] == '.' {
		return s[:len(s)-1]
	}
	return s
}

func splitLabels(fqdn string) []string {
	var labels []string
	start := 0
	for i := 0; i < len(fqdn); i++ {
		if fqdn[i] == '.' {
			labels = append(labels, fqdn[start:i])
			start = i + 1
		}
	}
	labels = append(labels, fqdn[start:])
	return labels
}

// extractHostname returns a validated hostname, preferring Option 81 (Client
// FQDN) over the HostName option. Empty string if neither produces a valid
// hostname.
func extractHostname(msg *dhcpv4.DHCPv4) string {
	if fqdnOpt := msg.Options.Get(dhcpv4.OptionFQDN); fqdnOpt != nil {
		if fqdn := dhcpinternal.HandleClientFQDN(fqdnOpt); fqdn != "" {
			if v := validateFQDN(fqdn); v != "" {
				return v
			}
		}
	}
	return validateFQDN(msg.HostName())
}

// HandleDiscover handles a DHCP DISCOVER message.
// Processing flow:
//  1. Find matching scope by relay agent IP or receiving interface
//  2. Check for existing reservation (MAC match)
//  3. If no reservation, find available IP in scope range
//  4. Ping check for conflict detection (if enabled) - performed BEFORE the
//     lease is written so we can pick a different IP if there is a conflict
//     without leaving a stale lease behind
//  5. Build response with scope options + reservation options
//  6. Create/update lease record
//  7. If dns_updates enabled, create/update DNS A/PTR records (async, rate-limited)
//  8. Log DHCP event
func (s *Server) HandleDiscover(msg *dhcpv4.DHCPv4, ifaceName string, serverIP net.IP) (*dhcpv4.DHCPv4, error) {
	mac := msg.ClientHWAddr.String()

	// Step 1: Find matching scope.
	sc, err := s.findScope(msg, ifaceName)
	if err != nil {
		return nil, fmt.Errorf("no matching scope: %w", err)
	}

	// Step 2: Check for existing reservation.
	var targetIP string
	var reservID string
	reserv, err := s.reservMgr.GetReservationByMAC(mac)
	if err == nil && reserv != nil && reserv.Enabled {
		targetIP = reserv.IPAddress
		reservID = reserv.ID
		slog.Debug("DHCP: found reservation for DISCOVER", "mac", mac, "ip", targetIP)
	}

	// Step 3: If no reservation, find available IP.
	if targetIP == "" {
		targetIP, err = s.leaseMgr.FindAvailableIP(sc.ID)
		if err != nil {
			return nil, fmt.Errorf("no available IP: %w", err)
		}
		slog.Debug("DHCP: found available IP for DISCOVER", "mac", mac, "ip", targetIP)
	}

	// Step 4: Ping check for conflict detection. Performed BEFORE creating
	// the lease so that conflicts are resolved by picking a new IP and we
	// never end up with a half-written lease that has to be rolled back.
	if sc.PingCheckEnabled && reservID == "" {
		if lease.PingCheck(targetIP) {
			slog.Warn("DHCP: IP conflict detected", "ip", targetIP)
			if s.eventLogger != nil {
				_ = s.eventLogger.LogDHCPEvent(dhcpinternal.EventConflict, sc.ID, mac, targetIP, "IP already in use")
			}
			targetIP, err = s.leaseMgr.FindAvailableIP(sc.ID)
			if err != nil {
				return nil, fmt.Errorf("no available IP after conflict: %w", err)
			}
		}
	}

	// Step 5: Build response with options.
	leaseDuration := time.Duration(sc.LeaseTime) * time.Second
	if leaseDuration == 0 {
		leaseDuration = 86400 * time.Second
	}

	// Build option map from database.
	optionMap, err := s.optionMgr.BuildOptionMapForScope(sc.ID, reservID)
	if err != nil {
		slog.Warn("DHCP: failed to build option map", "error", err)
		optionMap = make(map[int]string)
	}

	// Add scope-level defaults if not already in option map.
	if _, ok := optionMap[option.OptionSubnetMask]; !ok && sc.SubnetMask != "" {
		optionMap[option.OptionSubnetMask] = sc.SubnetMask
	}
	if _, ok := optionMap[option.OptionRouter]; !ok && sc.Router != "" {
		optionMap[option.OptionRouter] = sc.Router
	}
	if _, ok := optionMap[option.OptionDNSServers]; !ok && sc.DNSServers != "" {
		optionMap[option.OptionDNSServers] = sc.DNSServers
	}
	if _, ok := optionMap[option.OptionDomainName]; !ok && sc.DomainName != "" {
		optionMap[option.OptionDomainName] = sc.DomainName
	}
	if _, ok := optionMap[option.OptionLeaseTime]; !ok {
		optionMap[option.OptionLeaseTime] = fmt.Sprintf("%d", sc.LeaseTime)
	}
	if _, ok := optionMap[option.OptionNTPServers]; !ok && sc.NTPServers != "" {
		optionMap[option.OptionNTPServers] = sc.NTPServers
	}

	// Get requested options from Parameter Request List.
	var requestedOptions []dhcpv4.OptionCode
	if prl := msg.ParameterRequestList(); prl != nil {
		requestedOptions = prl
	}

	// Build DHCP options.
	dhcpOptions := option.BuildOptions(sc.ID, reservID, requestedOptions, optionMap)

	// Build OFFER response.
	parsedIP := net.ParseIP(targetIP)
	if parsedIP == nil {
		return nil, fmt.Errorf("invalid IP: %s", targetIP)
	}

	resp, err := dhcpv4.NewReplyFromRequest(msg,
		dhcpv4.WithMessageType(dhcpv4.MessageTypeOffer),
		dhcpv4.WithServerIP(serverIP),
		dhcpv4.WithYourIP(parsedIP),
		dhcpv4.WithLeaseTime(uint32(leaseDuration.Seconds())),
	)
	if err != nil {
		return nil, fmt.Errorf("building OFFER: %w", err)
	}

	// Apply options.
	for _, opt := range dhcpOptions {
		resp.Options.Update(opt)
	}

	// Step 6: Create tentative lease.
	hostname := extractHostname(msg)
	if _, err := s.leaseMgr.CreateLease(sc.ID, targetIP, mac, hostname, leaseDuration); err != nil {
		return nil, fmt.Errorf("failed to create lease: %w", err)
	}

	// Step 7: Async DNS update (rate-limited to dnsUpdateConcurrency).
	if sc.DNSUpdates {
		s.scheduleDNSUpdate(&lease.Lease{
			IPAddress:  targetIP,
			MACAddress: mac,
			Hostname:   hostname,
		}, "create")
	}

	// Step 8: Log event.
	if s.eventLogger != nil {
		_ = s.eventLogger.LogDHCPEvent(dhcpinternal.EventOffer, sc.ID, mac, targetIP, "OFFER sent")
	}

	return resp, nil
}

// HandleRequest handles a DHCP REQUEST message.
//
// NOTE: This implementation does NOT distinguish between the SELECTING,
// INIT-REBOOT, and RENEWING/REBINDING states. Per RFC 2131:
//   - SELECTING: client sends REQUEST with server identifier and requested IP
//     to accept an OFFER.
//   - INIT-REBOOT: client sends REQUEST with requested IP but no server
//     identifier, to verify a previously assigned lease.
//   - RENEWING/REBINDING: client unicasts/ broadcasts REQUEST with no server
//     identifier and no new requested IP, to extend an existing lease.
//     All three flows are currently treated identically: the requested IP is
//     honored if it is within the scope and not leased to another MAC, and an
//     existing lease for the same MAC is renewed. This is functionally correct
//     for the common case (SELECTING and RENEWING) but does not strictly
//     conform to RFC 2131 §4.4 (e.g. INIT-REBOOT should NAK if the lease is
//     unknown to the server).
func (s *Server) HandleRequest(msg *dhcpv4.DHCPv4, ifaceName string, serverIP net.IP) (*dhcpv4.DHCPv4, error) {
	mac := msg.ClientHWAddr.String()
	requestedIP := msg.RequestedIPAddress()

	// If no requested IP, use client IP.
	if requestedIP == nil || requestedIP.IsUnspecified() {
		requestedIP = msg.ClientIPAddr
	}

	if requestedIP == nil || requestedIP.IsUnspecified() {
		return nil, fmt.Errorf("no IP address requested")
	}

	// Find matching scope.
	sc, err := s.findScope(msg, ifaceName)
	if err != nil {
		return nil, fmt.Errorf("no matching scope: %w", err)
	}

	// Check for reservation.
	var reservID string
	reserv, err := s.reservMgr.GetReservationByMAC(mac)
	if err == nil && reserv != nil && reserv.Enabled {
		reservID = reserv.ID
		// If reservation IP doesn't match requested IP, NAK.
		if reserv.IPAddress != requestedIP.String() {
			return s.buildNAKWithMsg(msg, serverIP,
				fmt.Sprintf("requested IP %s does not match reservation %s", requestedIP, reserv.IPAddress)), nil
		}
	}

	// Check if requested IP is within scope range.
	startIP := net.ParseIP(sc.StartIP)
	endIP := net.ParseIP(sc.EndIP)
	if startIP == nil || endIP == nil {
		return nil, fmt.Errorf("invalid scope IP range")
	}
	requestedIP4 := requestedIP.To4()
	if requestedIP4 == nil {
		return nil, fmt.Errorf("invalid requested IP")
	}
	if !ipInRange(requestedIP, startIP, endIP) {
		return s.buildNAKWithMsg(msg, serverIP,
			fmt.Sprintf("requested IP %s not in scope range", requestedIP)), nil
	}

	// Check for existing lease.
	existingLease, err := s.leaseMgr.GetLeaseByIP(requestedIP.String())
	if err != nil {
		return nil, fmt.Errorf("failed to query lease for IP %s: %w", requestedIP, err)
	}
	if existingLease != nil && existingLease.MACAddress != mac {
		// IP is leased to a different MAC - NAK.
		return s.buildNAKWithMsg(msg, serverIP,
			fmt.Sprintf("IP %s is leased to another client", requestedIP)), nil
	}

	// Create or renew lease.
	leaseDuration := time.Duration(sc.LeaseTime) * time.Second
	if leaseDuration == 0 {
		leaseDuration = 86400 * time.Second
	}

	hostname := extractHostname(msg)
	if existingLease != nil && existingLease.MACAddress == mac {
		// Renew existing lease.
		if _, err := s.leaseMgr.RenewLease(existingLease.ID, leaseDuration); err != nil {
			return nil, fmt.Errorf("failed to renew lease: %w", err)
		}
	} else {
		// Create new lease.
		if _, err := s.leaseMgr.CreateLease(sc.ID, requestedIP.String(), mac, hostname, leaseDuration); err != nil {
			return nil, fmt.Errorf("failed to create lease: %w", err)
		}
	}

	// Build option map.
	optionMap, err := s.optionMgr.BuildOptionMapForScope(sc.ID, reservID)
	if err != nil {
		slog.Warn("DHCP: failed to build option map", "error", err)
		optionMap = make(map[int]string)
	}

	// Add scope-level defaults.
	if _, ok := optionMap[option.OptionSubnetMask]; !ok && sc.SubnetMask != "" {
		optionMap[option.OptionSubnetMask] = sc.SubnetMask
	}
	if _, ok := optionMap[option.OptionRouter]; !ok && sc.Router != "" {
		optionMap[option.OptionRouter] = sc.Router
	}
	if _, ok := optionMap[option.OptionDNSServers]; !ok && sc.DNSServers != "" {
		optionMap[option.OptionDNSServers] = sc.DNSServers
	}
	if _, ok := optionMap[option.OptionDomainName]; !ok && sc.DomainName != "" {
		optionMap[option.OptionDomainName] = sc.DomainName
	}
	if _, ok := optionMap[option.OptionLeaseTime]; !ok {
		optionMap[option.OptionLeaseTime] = fmt.Sprintf("%d", int(leaseDuration.Seconds()))
	}

	// Get requested options.
	var requestedOptions []dhcpv4.OptionCode
	if prl := msg.ParameterRequestList(); prl != nil {
		requestedOptions = prl
	}

	dhcpOptions := option.BuildOptions(sc.ID, reservID, requestedOptions, optionMap)

	// Build OFFER response.
	resp, err := dhcpv4.NewReplyFromRequest(msg,
		dhcpv4.WithMessageType(dhcpv4.MessageTypeAck),
		dhcpv4.WithServerIP(serverIP),
		dhcpv4.WithYourIP(requestedIP),
		dhcpv4.WithLeaseTime(uint32(leaseDuration.Seconds())),
	)
	if err != nil {
		return nil, fmt.Errorf("building ACK: %w", err)
	}

	for _, opt := range dhcpOptions {
		resp.Options.Update(opt)
	}

	// Async DNS update (rate-limited).
	if sc.DNSUpdates {
		s.scheduleDNSUpdate(&lease.Lease{
			IPAddress:  requestedIP.String(),
			MACAddress: mac,
			Hostname:   hostname,
		}, "create")
	}

	// Log event.
	if s.eventLogger != nil {
		_ = s.eventLogger.LogDHCPEvent(dhcpinternal.EventAck, sc.ID, mac, requestedIP.String(), "ACK sent")
	}

	return resp, nil
}

// buildNAKWithMsg builds a NAK response and logs the rejection reason. Unlike
// returning an error, this lets the caller send a proper NAK to the client
// (with the server identifier option set) instead of a silent failure.
func (s *Server) buildNAKWithMsg(req *dhcpv4.DHCPv4, serverIP net.IP, message string) *dhcpv4.DHCPv4 {
	nak, err := s.buildNAK(req, serverIP, message)
	if err != nil {
		slog.Error("DHCP: failed to build NAK", "error", err)
		return nil
	}
	return nak
}

// scheduleDNSUpdate runs an async DNS update throttled by s.dnsUpdateSem.
// The call returns immediately; the actual DNS work is performed by a
// goroutine. If the semaphore is exhausted the goroutine blocks until a slot
// is available rather than spawning an unbounded number of workers.
func (s *Server) scheduleDNSUpdate(l *lease.Lease, action string) {
	go func() {
		ctx := context.Background()
		if err := s.dnsUpdateSem.Acquire(ctx, 1); err != nil {
			slog.Error("DHCP: failed to acquire DNS update slot", "error", err)
			return
		}
		defer s.dnsUpdateSem.Release(1)
		if err := s.dnsLink.UpdateDNSRecord(l, action); err != nil {
			slog.Error("DHCP: async DNS update failed", "error", err)
		}
	}()
}

// HandleRelease handles a DHCP RELEASE message.
func (s *Server) HandleRelease(msg *dhcpv4.DHCPv4) error {
	mac := msg.ClientHWAddr.String()
	clientIP := msg.ClientIPAddr.String()

	// Find lease by MAC.
	l, err := s.leaseMgr.GetLeaseByMAC(mac)
	if err != nil || l == nil {
		return fmt.Errorf("no active lease for MAC %s", mac)
	}

	// Find scope to check DNSUpdates flag before DNS cleanup.
	sc, err := s.scopeMgr.GetScope(l.ScopeID)
	if err != nil {
		return fmt.Errorf("scope not found for lease: %w", err)
	}

	// Release the lease.
	if err := s.leaseMgr.ReleaseLease(l.ID); err != nil {
		return err
	}

	// Async DNS cleanup - only if scope has DNSUpdates enabled. Rate-limited.
	if sc.DNSUpdates {
		s.scheduleDNSUpdate(l, "delete")
	}

	// Log event.
	if s.eventLogger != nil {
		_ = s.eventLogger.LogDHCPEvent(dhcpinternal.EventRelease, l.ScopeID, mac, clientIP, "lease released")
	}

	return nil
}

// HandleDecline handles a DHCP DECLINE message.
func (s *Server) HandleDecline(msg *dhcpv4.DHCPv4) error {
	mac := msg.ClientHWAddr.String()
	requestedIP := msg.RequestedIPAddress()
	if requestedIP == nil {
		requestedIP = msg.ClientIPAddr
	}

	if requestedIP == nil {
		return fmt.Errorf("no IP in DECLINE")
	}

	// Find and mark the lease as conflict (not released).
	l, err := s.leaseMgr.GetLeaseByIP(requestedIP.String())
	if err != nil || l == nil {
		return fmt.Errorf("no lease for IP %s", requestedIP)
	}

	if err := s.leaseMgr.MarkLeaseConflict(l.ID); err != nil {
		return err
	}

	// Log conflict.
	if s.eventLogger != nil {
		_ = s.eventLogger.LogDHCPEvent(dhcpinternal.EventDecline, l.ScopeID, mac, requestedIP.String(), "client declined IP")
	}

	return nil
}

// HandleInform handles a DHCP INFORM message.
func (s *Server) HandleInform(msg *dhcpv4.DHCPv4, ifaceName string, serverIP net.IP) (*dhcpv4.DHCPv4, error) {
	mac := msg.ClientHWAddr.String()

	// Find matching scope.
	sc, err := s.findScope(msg, ifaceName)
	if err != nil {
		return nil, fmt.Errorf("no matching scope: %w", err)
	}

	// Build option map.
	optionMap, err := s.optionMgr.BuildOptionMapForScope(sc.ID, "")
	if err != nil {
		slog.Warn("DHCP: failed to build option map for INFORM", "error", err)
		optionMap = make(map[int]string)
	}

	// Add scope-level defaults.
	if _, ok := optionMap[option.OptionSubnetMask]; !ok && sc.SubnetMask != "" {
		optionMap[option.OptionSubnetMask] = sc.SubnetMask
	}
	if _, ok := optionMap[option.OptionRouter]; !ok && sc.Router != "" {
		optionMap[option.OptionRouter] = sc.Router
	}
	if _, ok := optionMap[option.OptionDNSServers]; !ok && sc.DNSServers != "" {
		optionMap[option.OptionDNSServers] = sc.DNSServers
	}
	if _, ok := optionMap[option.OptionDomainName]; !ok && sc.DomainName != "" {
		optionMap[option.OptionDomainName] = sc.DomainName
	}

	// Get requested options.
	var requestedOptions []dhcpv4.OptionCode
	if prl := msg.ParameterRequestList(); prl != nil {
		requestedOptions = prl
	}

	dhcpOptions := option.BuildOptions(sc.ID, "", requestedOptions, optionMap)

	// Build ACK response (INFORM ACK does not include lease time or IP).
	resp, err := dhcpv4.NewReplyFromRequest(msg,
		dhcpv4.WithMessageType(dhcpv4.MessageTypeAck),
		dhcpv4.WithServerIP(serverIP),
	)
	if err != nil {
		return nil, fmt.Errorf("building INFORM ACK: %w", err)
	}

	for _, opt := range dhcpOptions {
		resp.Options.Update(opt)
	}

	// Log event.
	if s.eventLogger != nil {
		_ = s.eventLogger.LogDHCPEvent(dhcpinternal.EventInform, sc.ID, mac, msg.ClientIPAddr.String(), "INFORM ACK sent")
	}

	return resp, nil
}

// findScope finds the matching scope for a DHCP message.
func (s *Server) findScope(msg *dhcpv4.DHCPv4, ifaceName string) (*scope.Scope, error) {
	// Try relay agent IP first.
	if msg.GatewayIPAddr != nil && !msg.GatewayIPAddr.IsUnspecified() {
		sc, err := s.scopeMgr.FindScopeByIP(msg.GatewayIPAddr.String())
		if err == nil {
			return sc, nil
		}
	}

	// Try client IP.
	if msg.ClientIPAddr != nil && !msg.ClientIPAddr.IsUnspecified() {
		sc, err := s.scopeMgr.FindScopeByIP(msg.ClientIPAddr.String())
		if err == nil {
			return sc, nil
		}
	}

	// Try requested IP.
	if reqIP := msg.RequestedIPAddress(); reqIP != nil && !reqIP.IsUnspecified() {
		sc, err := s.scopeMgr.FindScopeByIP(reqIP.String())
		if err == nil {
			return sc, nil
		}
	}

	// Try server interface IP.
	if serverIP, ok := s.serverIPs[ifaceName]; ok {
		sc, err := s.scopeMgr.FindScopeByIP(serverIP.String())
		if err == nil {
			return sc, nil
		}
	}

	return nil, fmt.Errorf("no matching scope found")
}

// ipInRange checks if an IP is within the range [start, end].
// Uses proper numeric comparison by converting IPv4 addresses to their
// 4-byte representation and comparing byte-by-byte.
func ipInRange(ip, start, end net.IP) bool {
	ip4 := ip.To4()
	start4 := start.To4()
	end4 := end.To4()
	if ip4 == nil || start4 == nil || end4 == nil {
		return false
	}
	return bytesCompare(ip4, start4) >= 0 && bytesCompare(ip4, end4) <= 0
}

// bytesCompare compares two net.IP (4-byte) values lexicographically.
// Returns -1 if a < b, 0 if a == b, 1 if a > b.
func bytesCompare(a, b net.IP) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}
