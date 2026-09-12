package server

import (
	"context"
	"errors"
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
//  1. Find matching scope by relay agent IP, client address or receiving interface
//  2. Check for existing reservation (MAC match)
//  3. If no reservation, reserve an available address as an OFFER
//  4. Build response with scope options + reservation options
//  5. Log DHCP event
//
// The address is recorded with status 'offered', not 'active': no binding
// exists until the client confirms it with a REQUEST (RFC 2131 §4.3.1).
// Writing an active lease here would let a client that never sends REQUEST —
// or a scanner walking the range — hold addresses for the full lease time and
// exhaust the scope.
//
// DNS is not touched from DISCOVER for the same reason: the name must follow
// the confirmed binding, not a proposal.
func (s *Server) HandleDiscover(msg *dhcpv4.DHCPv4, ifaceName string, serverIP net.IP) (*dhcpv4.DHCPv4, error) {
	mac := msg.ClientHWAddr.String()
	hostname := extractHostname(msg)

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

	// Step 3: Reserve an address, or refresh the reservation.
	var offer *lease.Lease
	if reservID != "" {
		offer, err = s.leaseMgr.ReserveAddress(sc.ID, targetIP, mac, hostname)
		if err != nil && !errors.Is(err, lease.ErrAddressTaken) {
			return nil, fmt.Errorf("failed to reserve reserved IP %s: %w", targetIP, err)
		}
		if errors.Is(err, lease.ErrAddressTaken) {
			// The address is administratively reserved to this MAC but held by
			// someone else: that is a real conflict an operator must resolve,
			// not something to paper over by offering a different address.
			return nil, fmt.Errorf("reserved IP %s is held by another client", targetIP)
		}
	} else {
		offer, err = s.reserveAddress(sc, mac, hostname)
		if err != nil {
			return nil, err
		}
		targetIP = offer.IPAddress
		slog.Debug("DHCP: reserved address for DISCOVER", "mac", mac, "ip", targetIP)
	}

	// Tell the second copy an address is now spoken for, without waiting for it.
	// An offer is not a promise -- it expires, and a client that never sees it
	// has lost a round trip rather than an address -- so it does not belong on
	// the path that decides whether a client gets an answer. It is replicated
	// all the same, because a mirror that has never heard of the offer would
	// hand the same address to the next client after a takeover.
	s.replicateLeaseState(offer)

	// Step 4: Build response with options.
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

	// Step 5: Log event.
	if s.eventLogger != nil {
		_ = s.eventLogger.LogDHCPEvent(dhcpinternal.EventOffer, sc.ID, mac, targetIP, "OFFER sent")
	}

	return resp, nil
}

// reserveAddress picks a free address in the scope and reserves it for a
// DISCOVER, retrying a bounded number of times.
//
// FindAvailableIP is a read and the reservation is a separate write, so two
// concurrent DISCOVERs can select the same candidate; the unique index on
// (scope_id, ip_address) rejects the loser, which retries with the next free
// address instead of failing the client outright.
// reserveAddress picks a free address in the scope and writes the offer.
//
// It returns the row it wrote rather than just the address. The row is what
// the second copy needs, and looking it up again afterwards would be a second
// query racing the first client to send its REQUEST.
func (s *Server) reserveAddress(sc *scope.Scope, mac, hostname string) (*lease.Lease, error) {
	const attempts = 5

	var lastErr error
	for i := 0; i < attempts; i++ {
		ip, err := s.leaseMgr.FindAvailableIP(sc.ID)
		if err != nil {
			return nil, err
		}

		// Conflict probe before the reservation is written, so a conflicting
		// address is quarantined instead of being offered and then declined.
		if sc.PingCheckEnabled && lease.PingCheck(ip) {
			slog.Warn("DHCP: IP conflict detected", "ip", ip, "scope", sc.ID)
			if s.eventLogger != nil {
				_ = s.eventLogger.LogDHCPEvent(dhcpinternal.EventConflict, sc.ID, mac, ip, "IP already in use")
			}
			// The address was never offered to a client, so it owns no DNS
			// record; a stale row left by an earlier binding is withdrawn by the
			// reconciler.
			quarantined, err := s.leaseMgr.QuarantineIP(sc.ID, ip, mac)
			if err != nil {
				slog.Error("DHCP: failed to quarantine conflicting IP", "ip", ip, "error", err)
			}
			s.replicateLeaseState(quarantined)
			continue
		}

		offer, err := s.leaseMgr.ReserveAddress(sc.ID, ip, mac, hostname)
		if err != nil {
			lastErr = err
			continue // lost the race for this address; try the next candidate
		}
		return offer, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("every candidate address in scope %s conflicted", sc.ID)
	}
	return nil, fmt.Errorf("could not reserve an address after %d attempts: %w", attempts, lastErr)
}

// requestState classifies a DHCPREQUEST per RFC 2131 §4.3.2. The three states
// demand different server behaviour (ACK / NAK / silence), which the previous
// implementation collapsed into a single "honour the requested address" path.
type requestState int

const (
	// requestSelecting: the client accepts an OFFER from this server, so the
	// message carries our server identifier plus the requested address.
	requestSelecting requestState = iota
	// requestInitReboot: the client revalidates an address it was assigned
	// earlier. No server identifier and no ciaddr, but a requested address.
	requestInitReboot
	// requestRenewing: the client extends an existing binding. No server
	// identifier; ciaddr holds the address already in use.
	requestRenewing
)

// classifyRequest decides how a DHCPREQUEST must be handled and which address it
// concerns. ok is false when the message must be ignored entirely.
func (s *Server) classifyRequest(msg *dhcpv4.DHCPv4) (requestState, net.IP, bool) {
	if sid := msg.ServerIdentifier(); sid != nil && !sid.IsUnspecified() {
		if !s.isOurServerID(sid) {
			// The client chose a different server; answering would violate
			// RFC 2131 §4.3.2 and can leave two servers fighting over one
			// client, each NAKing the other's ACK.
			return requestSelecting, nil, false
		}
		ip := msg.RequestedIPAddress()
		if ip == nil || ip.IsUnspecified() {
			return requestSelecting, nil, false
		}
		return requestSelecting, ip, true
	}

	if msg.ClientIPAddr != nil && !msg.ClientIPAddr.IsUnspecified() {
		return requestRenewing, msg.ClientIPAddr, true
	}

	if ip := msg.RequestedIPAddress(); ip != nil && !ip.IsUnspecified() {
		return requestInitReboot, ip, true
	}

	return requestInitReboot, nil, false
}

// isOurServerID reports whether a DHCP server identifier belongs to this host.
func (s *Server) isOurServerID(ip net.IP) bool {
	for _, own := range s.serverIPs {
		if own != nil && own.Equal(ip) {
			return true
		}
	}
	return false
}

// HandleRequest handles a DHCP REQUEST message.
//
// The three states defined by RFC 2131 §4.3.2 are handled separately:
//
//   - SELECTING  — the client accepts our OFFER: the address must be free or
//     already held for this client, otherwise NAK.
//   - INIT-REBOOT — the client revalidates an earlier assignment: ACK when our
//     binding matches, NAK when it disagrees, and stay silent when we have no
//     record of the client at all, so it can try another server instead of
//     being told its working address is invalid.
//   - RENEWING/REBINDING — the client extends a binding named by ciaddr.
//
// A REQUEST carrying another server's identifier is dropped without a reply.
func (s *Server) HandleRequest(msg *dhcpv4.DHCPv4, ifaceName string, serverIP net.IP) (*dhcpv4.DHCPv4, error) {
	mac := msg.ClientHWAddr.String()

	state, requestedIP, ok := s.classifyRequest(msg)
	if !ok {
		slog.Debug("DHCP: REQUEST not addressed to this server, ignoring",
			"mac", mac, "state", state)
		return nil, nil
	}

	requestedIP4 := requestedIP.To4()
	if requestedIP4 == nil {
		// IPv6 in a DHCPv4 message is malformed; dropping is safer than NAKing
		// a packet we cannot interpret.
		slog.Debug("DHCP: REQUEST with a non-IPv4 address, ignoring", "mac", mac, "ip", requestedIP)
		return nil, nil
	}
	requestedIP = requestedIP4

	// Find matching scope by giaddr, ciaddr, requested address or interface.
	sc, err := s.findScope(msg, ifaceName)
	if err != nil {
		// We are not authoritative for this address. Silence is the only safe
		// answer: a NAK would tell the client to discard a binding that may be
		// perfectly valid on the subnet it actually belongs to.
		slog.Debug("DHCP: no scope for REQUEST, ignoring",
			"mac", mac, "ip", requestedIP, "state", state)
		return nil, nil
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

	// The address must be inside this scope's pool. A renewal of an address
	// inside the subnet but outside the pool (a statically configured host) is
	// allowed: the pool bounds govern allocation, not the confirmation of a
	// binding the client already has.
	if state != requestRenewing {
		startIP := net.ParseIP(sc.StartIP)
		endIP := net.ParseIP(sc.EndIP)
		if startIP == nil || endIP == nil {
			return nil, fmt.Errorf("invalid scope IP range")
		}
		if !ipInRange(requestedIP, startIP, endIP) {
			return s.buildNAKWithMsg(msg, serverIP,
				fmt.Sprintf("requested IP %s not in scope range", requestedIP)), nil
		}
	}

	// Look up whatever currently holds the address: a confirmed binding or an
	// outstanding reservation both make it unavailable to another client.
	held, err := s.leaseMgr.GetHeldLeaseByIP(requestedIP.String())
	if err != nil {
		return nil, fmt.Errorf("failed to query lease for IP %s: %w", requestedIP, err)
	}
	if held != nil && held.Status == lease.LeaseStatusConflict {
		return s.buildNAKWithMsg(msg, serverIP,
			fmt.Sprintf("IP %s is quarantined after a conflict", requestedIP)), nil
	}
	if held != nil && held.MACAddress != mac {
		return s.buildNAKWithMsg(msg, serverIP,
			fmt.Sprintf("IP %s is held by another client", requestedIP)), nil
	}

	// INIT-REBOOT with nothing holding the address: either we never knew this
	// client (stay silent) or its binding points somewhere else (NAK so it
	// restarts discovery).
	if state == requestInitReboot && held == nil {
		own, err := s.leaseMgr.GetLeaseByMAC(mac)
		if err != nil {
			return nil, fmt.Errorf("failed to query lease for MAC %s: %w", mac, err)
		}
		if own == nil {
			slog.Debug("DHCP: INIT-REBOOT for an unknown client, staying silent",
				"mac", mac, "ip", requestedIP)
			return nil, nil
		}
		if own.IPAddress != requestedIP.String() {
			return s.buildNAKWithMsg(msg, serverIP,
				fmt.Sprintf("client %s is bound to %s, not %s", mac, own.IPAddress, requestedIP)), nil
		}
		// own.IPAddress == requestedIP would have been found by
		// GetHeldLeaseByIP, so this is unreachable; stay silent rather than
		// guess.
		return nil, nil
	}

	leaseDuration := time.Duration(sc.LeaseTime) * time.Second
	if leaseDuration == 0 {
		leaseDuration = 86400 * time.Second
	}
	hostname := extractHostname(msg)

	// There is no second copy, and no operator has said this node may run
	// without one. Nothing below this line is done: not the write, not the
	// reply.
	//
	// Not writing is the deliberate part. A binding that was never promised to
	// anybody still occupies a row, and an address held by a client that was
	// never told about it is an address the pool cannot hand out. Staying
	// silent and staying clean are the same decision here, and the alternative
	// -- write, then discover the second copy is gone -- leaves a trail of
	// phantom leases for as long as the pair is broken.
	if s.leaseReplicator != nil && !s.leaseReplicator.MayBind() {
		slog.Warn("DHCP: withholding the ACK; this node has no second copy of the binding",
			"mac", mac, "ip", requestedIP, "state", "paused")
		return nil, nil
	}

	// Commit the binding BEFORE building the ACK: the client treats the ACK as
	// authoritative, so the state it relies on must already be durable.
	var bound *lease.Lease
	observed := LeaseObservedBind
	switch {
	case held == nil:
		bound, err = s.leaseMgr.CreateLease(sc.ID, requestedIP.String(), mac, hostname, leaseDuration)
		if err != nil {
			return nil, fmt.Errorf("failed to create lease: %w", err)
		}
	case held.Status == lease.LeaseStatusOffered:
		// Promote the reservation made by the matching DISCOVER.
		bound, err = s.leaseMgr.ActivateLease(held.ID, leaseDuration)
		if err != nil {
			return nil, fmt.Errorf("failed to activate lease: %w", err)
		}
	default:
		bound, err = s.leaseMgr.RenewLease(held.ID, leaseDuration)
		if err != nil {
			return nil, fmt.Errorf("failed to renew lease: %w", err)
		}
		observed = LeaseObservedRenew
	}

	// The binding is durable here. Before the client is told it holds the
	// address, a second machine has to have it too, or a power loss on this one
	// becomes two clients using one address on the next boot.
	//
	// A failure to confirm is answered with silence, not with a NAK. A NAK says
	// "this address is not usable", and the client acts on that by abandoning
	// it and starting over; the truth is that this server cannot make a promise
	// right now, and the address is fine. Silence leaves the client retrying,
	// and the retry lands on the same address once the second copy is back.
	if s.leaseReplicator != nil {
		if err := s.leaseReplicator.Confirm(context.Background(), bound); err != nil {
			slog.Warn("DHCP: withholding the ACK; the second copy did not confirm",
				"mac", mac, "ip", requestedIP, "lease_id", bound.ID, "error", err)
			return nil, nil
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

	// Build ACK response.
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

	// Queue the DNS publish. The binding is already durable at this point, so a
	// failure to queue must not fail the request: the client is entitled to its
	// ACK, and the reconciler will publish the record on its next pass.
	if sc.DNSUpdates && bound != nil {
		if err := s.enqueueDNSEvent(bound, dhcpinternal.DNSEventCreate); err != nil {
			slog.Error("DHCP: failed to queue DNS update",
				"lease_id", bound.ID, "hostname", hostname, "error", err)
		}
	}

	// Report the binding to IPAM. Like the DNS queue above, this must not fail
	// the request: the client is entitled to its ACK, and the reconciler
	// replays any observation that was missed.
	s.observeLease(observed, bound)

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
	// The row now says "released", and the mirror has to say the same. It is
	// the state the client has just told us it no longer wants; leaving the
	// mirror holding a live binding would keep the address out of the pool on
	// the other node forever.
	s.replicateLeaseStateAfterRelease(l.ID)

	// Queue DNS cleanup - only if scope has DNSUpdates enabled. The event
	// carries the generation read before the release, which is the generation
	// the published record was written at, so a renewal that lands before the
	// consumer runs keeps its record.
	if sc.DNSUpdates {
		if err := s.enqueueDNSEvent(l, dhcpinternal.DNSEventDelete); err != nil {
			slog.Error("DHCP: failed to queue DNS teardown",
				"lease_id", l.ID, "hostname", l.Hostname, "error", err)
		}
	}

	// A RELEASE is the client saying it no longer holds the address, so the
	// IPAM view must stop reporting it as in use. This is independent of the
	// scope's DNSUpdates flag: that flag governs whether a DNS name exists,
	// not whether the address is in use.
	s.observeLease(LeaseObservedRelease, l)

	// Log event.
	if s.eventLogger != nil {
		_ = s.eventLogger.LogDHCPEvent(dhcpinternal.EventRelease, l.ScopeID, mac, clientIP, "lease released")
	}

	return nil
}

// HandleDecline handles a DHCP DECLINE message.
//
// DECLINE means the client found the offered address already in use, so the
// address must be taken out of the pool for a quarantine period. Merely
// relabelling an existing lease was not enough: the availability query only
// looked at 'active' leases, so the declined address was handed to the next
// client immediately, producing a decline loop across the scope.
//
// A DECLINE can also arrive for an address this server has no lease for (a
// statically configured host, or a conflict reported by a peer), so a
// quarantine tombstone is written in that case rather than dropping the report.
func (s *Server) HandleDecline(msg *dhcpv4.DHCPv4) error {
	mac := msg.ClientHWAddr.String()
	requestedIP := msg.RequestedIPAddress()
	if requestedIP == nil || requestedIP.IsUnspecified() {
		requestedIP = msg.ClientIPAddr
	}
	if requestedIP == nil || requestedIP.IsUnspecified() {
		return fmt.Errorf("no IP in DECLINE")
	}

	// The declined address itself identifies the scope.
	scopeID := ""
	if sc, err := s.scopeMgr.FindScopeByIP(requestedIP.String()); err == nil {
		scopeID = sc.ID
	} else {
		// Without a scope we cannot write a tombstone (dhcp_leases.scope_id is
		// a foreign key), so there is nothing safe to record.
		slog.Warn("DHCP: DECLINE for an address outside every scope, not quarantined",
			"mac", mac, "ip", requestedIP)
		return nil
	}

	quarantined, err := s.leaseMgr.QuarantineIP(scopeID, requestedIP.String(), mac)
	if err != nil {
		return err
	}
	// The quarantine has to reach the mirror too. It is a promise in the
	// negative: this address must not be handed out. A mirror that never heard
	// it would offer the very address the client just reported as taken.
	s.replicateLeaseState(quarantined)

	// A declined address must also lose its DNS record: the client has just
	// told us the binding is wrong, so continuing to publish the name would
	// hand out an address something else is already using. Queued rather than
	// written here so it is retried and survives a restart.
	if quarantined != nil && quarantined.Hostname != "" {
		if sc, err := s.scopeMgr.GetScope(quarantined.ScopeID); err == nil && sc.DNSUpdates {
			if err := s.enqueueDNSEvent(quarantined, dhcpinternal.DNSEventDelete); err != nil {
				slog.Error("DHCP: failed to queue DNS teardown for declined address",
					"lease_id", quarantined.ID, "error", err)
			}
		}
	}

	// Report the DECLINE to IPAM using the address from the message, not from
	// the lease: a DECLINE can arrive for an address this server never leased
	// (a statically configured host, or a conflict reported by a peer), and in
	// that case there is no lease row to read the address from.
	s.observeLeaseFields(LeaseObservedDecline, "", scopeID, requestedIP.String(), mac, "")

	// Log conflict.
	if s.eventLogger != nil {
		_ = s.eventLogger.LogDHCPEvent(dhcpinternal.EventDecline, scopeID, mac, requestedIP.String(), "client declined IP")
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
