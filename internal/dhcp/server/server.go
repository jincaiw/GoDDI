package server

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net"
	"runtime/debug"
	"sync"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/jasonwa/goddi/internal/dhcp/lease"
	"github.com/jasonwa/goddi/internal/dhcp/option"
	"github.com/jasonwa/goddi/internal/dhcp/reservation"
	"github.com/jasonwa/goddi/internal/dhcp/scope"

	dhcpinternal "github.com/jasonwa/goddi/internal/dhcp"
)

// Server represents a DHCPv4 server.
type Server struct {
	scopeMgr    *scope.Manager
	leaseMgr    *lease.Manager
	reservMgr   *reservation.Manager
	optionMgr   *option.Manager
	outbox      *dhcpinternal.DNSOutbox
	eventLogger *dhcpinternal.EventLogger

	interfaces []string
	// trustedRelays are the relay-agent addresses this server will serve. Nil
	// means no restriction, which is what a deployment with no relays needs.
	trustedRelays []*net.IPNet
	// relayAllowListBroken records that the operator supplied a relay list
	// that could not be read in full. The restrictive reading is the right
	// one: an allowlist that failed to load must not behave like no allowlist
	// at all. Directly attached clients are unaffected -- this setting has
	// nothing to say about them.
	relayAllowListBroken bool
	listeners            []net.PacketConn
	serverIPs            map[string]net.IP // interface -> server IP

	// leaseObserver is told about every lease state change. It is optional:
	// when it is nil the data plane still works, because an IPAM view that is
	// behind is a reporting problem, not a reason to refuse a client its
	// address.
	leaseObserver LeaseObserver

	// leaseReplicator is the second copy a binding must reach before the
	// client is told it holds an address. Nil means there is no second copy,
	// which is the single-node case and behaves exactly as it did before this
	// hook existed.
	leaseReplicator LeaseReplicator

	// outboxWake is told that a DNS change was just queued, so the store's
	// replication loop can push it without waiting for its next poll. Optional:
	// unset means the entry waits for the poll, which is correct and slower.
	outboxWake func()

	quit chan struct{}
	wg   sync.WaitGroup
}

// Lease actions passed to a LeaseObserver.
const (
	LeaseObservedBind    = "bind"
	LeaseObservedRenew   = "renew"
	LeaseObservedRelease = "release"
	LeaseObservedExpire  = "expire"
	LeaseObservedDecline = "decline"
)

// LeaseObserver is notified when a lease changes state.
//
// The parameters are primitives on purpose. This package must not import the
// IPAM package: the data plane has to keep serving when the control-side code
// is being changed, and B4 separates them into separate processes entirely.
// A concrete implementation satisfies this interface structurally, with no
// shared type and no import in either direction.
type LeaseObserver interface {
	ObserveLease(action, leaseID, scopeID, ip, mac, hostname string) error
}

// SetLeaseObserver installs the observer. It is called once during wiring.
func (s *Server) SetLeaseObserver(o LeaseObserver) { s.leaseObserver = o }

// LeaseReplicator is the second copy a binding has to reach before the client
// is told it holds an address.
//
// It is an interface with two methods, and the server is written so that nil
// means "there is no second copy and there never was", which is the behaviour
// every deployment had before a pair existed. The types are the server's own,
// so the DHCP request path has no dependency on how the second copy is
// implemented -- or whether it is another machine at all.
type LeaseReplicator interface {
	// MayBind reports whether a new binding or a renewal of an existing one
	// may be promised right now.
	MayBind() bool
	// Confirm makes the binding durable on the second copy, or reports why it
	// could not.
	Confirm(ctx context.Context, l *lease.Lease) error
	// Replicate records a lease change without waiting for the second copy.
	// Offers travel this way: a reservation that is lost costs a client one
	// round trip, not an address.
	Replicate(l *lease.Lease)
}

// SetLeaseReplicator installs the second copy. It is called once during wiring.
//
// Nil is the normal case and is not a degraded one: a single node has nothing
// to confirm against, and the request path behaves exactly as it did before
// this hook existed.
func (s *Server) SetLeaseReplicator(r LeaseReplicator) { s.leaseReplicator = r }

// observeLease reports a lease change to the observer.
func (s *Server) observeLease(action string, l *lease.Lease) {
	if l == nil {
		return
	}
	s.observeLeaseFields(action, l.ID, l.ScopeID, l.IPAddress, l.MACAddress, l.Hostname)
}

// observeLeaseFields reports a lease change that has no lease row behind it --
// a DECLINE for an address this server never leased, for instance.
//
// A failure is logged and swallowed. The client's reply is already committed
// and must not be withdrawn because a reporting subsystem is unavailable; the
// reconciler replays missed observations from the lease table afterwards.
func (s *Server) observeLeaseFields(action, leaseID, scopeID, ip, mac, hostname string) {
	if s.leaseObserver == nil {
		return
	}
	if err := s.leaseObserver.ObserveLease(action, leaseID, scopeID, ip, mac, hostname); err != nil {
		slog.Warn("DHCP: lease observation failed",
			"action", action, "lease_id", leaseID, "ip", ip, "error", err)
	}
}

// replicateLeaseState records a lease change for the second copy, without
// waiting for it.
//
// This is for the changes that are not promises: an offer, a quarantine, a
// release. They are replicated anyway, because a mirror that is missing them
// converges on a state the primary was never in -- and after a takeover, an
// address the primary had quarantined would be handed out as though nothing
// had happened.
//
// A failure costs only the accuracy of the mirror until the next snapshot,
// which is why this is fire-and-forget: nothing here may delay a reply to a
// client.
func (s *Server) replicateLeaseState(l *lease.Lease) {
	if s.leaseReplicator == nil || l == nil {
		return
	}
	s.leaseReplicator.Replicate(l)
}

// replicateLeaseStateAfterRelease re-reads a lease and records its new state
// for the second copy.
//
// The release path is the one place where the row a caller holds is the row as
// it was before the change: ReleaseLease reports success or failure and nothing
// else. Sending the pre-release row would tell the mirror the address is still
// held, which is the exact opposite of what just happened.
func (s *Server) replicateLeaseStateAfterRelease(id string) {
	if s.leaseReplicator == nil || id == "" {
		return
	}
	l, err := s.leaseMgr.GetLease(id)
	if err != nil || l == nil {
		slog.Warn("DHCP: could not re-read a released lease for the second copy",
			"lease_id", id, "error", err)
		return
	}
	s.leaseReplicator.Replicate(l)
}

// New creates a new DHCP server.
//
// Everything the request path touches -- scopes, leases, reservations, options,
// the DNS outbox and the event log -- is read and written through one database,
// the data plane's own. The client's reply therefore depends on one file the
// data plane owns, and not on a control-plane database that another process is
// free to restart.
//
// What is deliberately absent is the zone store. Turning a binding into a
// record, and withdrawing it again, is work for the plane that serves the name:
// this server records that the update is owed and moves on, and the consumer on
// the other side applies it. A DHCP process with no DNS role therefore holds no
// handle to a zone table it must not write to.
func New(leaseDB *sql.DB, interfaces []string, eventLogger *dhcpinternal.EventLogger) *Server {
	return &Server{
		scopeMgr:    scope.NewManager(leaseDB),
		leaseMgr:    lease.NewManager(leaseDB),
		reservMgr:   reservation.NewManager(leaseDB),
		optionMgr:   option.NewManager(leaseDB),
		outbox:      dhcpinternal.NewDNSOutbox(leaseDB),
		eventLogger: eventLogger,
		interfaces:  interfaces,
		serverIPs:   make(map[string]net.IP),
		quit:        make(chan struct{}),
	}
}

// SetTrustedRelays restricts relayed requests to relay agents inside the given
// networks. An empty list removes the restriction: a deployment with no relays
// has nothing to list, and inventing a default would break every existing one.
//
// This is a relay allowlist, not a client one. A client chooses its own MAC
// address, so a MAC list would exclude honest hardware and stop nobody who had
// decided to lie; the relay's giaddr, by contrast, is set by a device the
// operator owns and reaches the server only if that device sent the packet.
func (s *Server) SetTrustedRelays(cidrs []string) {
	networks := make([]*net.IPNet, 0, len(cidrs))
	broken := false
	for _, cidr := range cidrs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			// Config validation refuses these at startup. If one reaches here
			// anyway, the list it belongs to must not be silently widened: an
			// entry dropped from an allowlist makes the list longer, which is
			// the one direction an access control must never move on its own.
			slog.Error("DHCP: refusing a relay network that cannot be read; relayed requests are refused until it is fixed",
				"entry", cidr, "error", err)
			broken = true
			continue
		}
		networks = append(networks, network)
	}
	s.trustedRelays = networks
	s.relayAllowListBroken = broken
	switch {
	case broken:
		slog.Error("DHCP: the relay allowlist is not in force in full; no relayed request will be served")
	case len(networks) > 0:
		slog.Info("DHCP: serving only these relay agents", "networks", cidrs)
	}
}

// relayIsTrusted reports whether a request may be served.
//
// A request with no giaddr came from a client on a directly attached network,
// and is governed by the interface list rather than by this one. That verdict
// does not depend on the state of the relay allowlist: a typo in a setting
// about relays must not take down the network that has no relays.
func (s *Server) relayIsTrusted(msg *dhcpv4.DHCPv4) bool {
	giaddr := msg.GatewayIPAddr
	if giaddr == nil || giaddr.IsUnspecified() {
		return true
	}
	if s.relayAllowListBroken {
		return false
	}
	if len(s.trustedRelays) == 0 {
		return true
	}
	for _, network := range s.trustedRelays {
		if network.Contains(giaddr) {
			return true
		}
	}
	return false
}

// Start starts the DHCP server.
func (s *Server) Start(ctx context.Context) error {
	// Expire old leases on startup. Bindings that lapsed while the process was
	// down must take their DNS records with them, otherwise a name keeps
	// pointing at an address the pool has already reissued.
	s.sweepExpiredLeases("startup")

	// Determine which interfaces to serve.
	ifaces := s.interfaces
	if len(ifaces) == 0 {
		// Serve on all interfaces.
		allIfaces, err := net.Interfaces()
		if err != nil {
			return fmt.Errorf("failed to list interfaces: %w", err)
		}
		for _, iface := range allIfaces {
			if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback == 0 {
				ifaces = append(ifaces, iface.Name)
			}
		}
	}

	// Bind the DHCP socket exactly once. Binding 0.0.0.0:67 once per
	// interface is impossible (EADDRINUSE from the second bind on) — the
	// single socket receives broadcasts arriving on any interface, and each
	// served interface only contributes its IPv4 address for DHCP option
	// fields (giaddr, sname, etc.).
	conn, err := net.ListenPacket("udp4", "0.0.0.0:67")
	if err != nil {
		return fmt.Errorf("failed to listen on :67: %w", err)
	}
	s.listeners = append(s.listeners, conn)

	// Register per-interface server IPs and start a receive loop per
	// interface over the shared socket. Concurrent ReadFrom on the same
	// UDP socket is safe; each loop uses its interface's server IP when
	// building replies.
	primaryIP := net.IPv4zero
	for _, ifaceName := range ifaces {
		if err := s.registerInterfaceServerIP(ifaceName); err != nil {
			slog.Error("DHCP server: skipping interface", "interface", ifaceName, "error", err)
			continue
		}
		serverIP := s.serverIPs[ifaceName]
		if primaryIP.IsUnspecified() {
			primaryIP = serverIP
		}
		s.wg.Add(1)
		go s.receiveLoop(conn, ifaceName, serverIP)
		slog.Info("DHCP server: serving interface", "interface", ifaceName, "server_ip", serverIP)
	}

	if primaryIP.IsUnspecified() {
		conn.Close()
		return fmt.Errorf("no interfaces available for DHCP listening")
	}

	// Start lease expiry goroutine.
	go s.leaseExpiryLoop()

	slog.Info("DHCP server started", "listeners", len(s.listeners), "interfaces", len(s.serverIPs))
	return nil
}

// registerInterfaceServerIP resolves the interface's primary IPv4 address and
// records it for DHCP option fields.
func (s *Server) registerInterfaceServerIP(ifaceName string) error {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return fmt.Errorf("interface %s not found: %w", ifaceName, err)
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return fmt.Errorf("failed to get addresses for %s: %w", ifaceName, err)
	}

	// Find the IPv4 address for this interface.
	var serverIP net.IP
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipNet.IP.To4()
		if ip != nil {
			serverIP = ip
			break
		}
	}

	if serverIP == nil {
		return fmt.Errorf("no IPv4 address on interface %s", ifaceName)
	}

	s.serverIPs[ifaceName] = serverIP
	return nil
}

// receiveLoop reads DHCP packets from a connection and processes them.
func (s *Server) receiveLoop(conn net.PacketConn, ifaceName string, serverIP net.IP) {
	defer s.wg.Done()

	buf := make([]byte, 1500)

	for {
		select {
		case <-s.quit:
			return
		default:
		}

		conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, addr, err := conn.ReadFrom(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			if s.isStopped() {
				return
			}
			slog.Error("DHCP server: read error", "interface", ifaceName, "error", err)
			continue
		}

		// Parse DHCP packet.
		msg, err := dhcpv4.FromBytes(buf[:n])
		if err != nil {
			slog.Debug("DHCP server: failed to parse packet", "error", err)
			continue
		}

		// Process the message. handleMessage recovers panics internally so
		// one malformed/hostile packet cannot kill the receive loop and
		// silently take DHCP down for the whole interface.
		s.handleMessage(msg, addr, conn, ifaceName, serverIP)
	}
}

// handleMessage processes a DHCP message and sends a response.
func (s *Server) handleMessage(msg *dhcpv4.DHCPv4, addr net.Addr, conn net.PacketConn, ifaceName string, serverIP net.IP) {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("DHCP server: recovered from panic while handling packet",
				"interface", ifaceName, "panic", rec,
				"client", addr.String(), "stack", string(debug.Stack()))
		}
	}()
	msgType := msg.MessageType()

	mac := msg.ClientHWAddr.String()
	clientIP := msg.ClientIPAddr.String()

	// Only a client request is served.
	//
	// RFC 2131 section 3 defines the field rather than mandating a check: every
	// message a client sends to a server carries BOOTREQUEST, and BOOTREPLY
	// travels only the other way. Nothing that legitimately reaches a server
	// port says "I am a reply", so a packet that does is either a forgery or
	// something else's reply that landed on our socket.
	//
	// Answering it would be worse than dropping it, which is what makes this
	// worth a check rather than a shrug: the library derives the reply's opcode
	// from the request's (dhcpv4.WithReply), so anything that is not
	// BOOTREQUEST in comes back out as op=BOOTREQUEST -- a packet no client can
	// accept, and one that reads on the wire as a request.
	if msg.OpCode != dhcpv4.OpcodeBootRequest {
		slog.Warn("DHCP: dropping a message whose opcode is not BOOTREQUEST",
			"opcode", uint8(msg.OpCode), "interface", ifaceName,
			"client", mac, "msg_type", msgType.String())
		return
	}

	// A relayed request is served only for a relay we were told about. The
	// giaddr is the relay's address on the client subnet, so an unexpected one
	// means a device we do not know is forwarding for a network we may have no
	// business handing addresses to -- and it can name any subnet it likes.
	//
	// Silence, not a NAK: the client behind that relay did nothing wrong, and a
	// NAK would tell it to discard a lease that may be perfectly valid on the
	// subnet it actually belongs to.
	if !s.relayIsTrusted(msg) {
		slog.Warn("DHCP: dropping a request from an unlisted relay agent",
			"giaddr", msg.GatewayIPAddr.String(), "interface", ifaceName,
			"client", mac, "msg_type", msgType.String())
		return
	}

	switch msgType {
	case dhcpv4.MessageTypeDiscover:
		resp, err := s.HandleDiscover(msg, ifaceName, serverIP)
		if err != nil {
			slog.Error("DHCP: HandleDiscover error", "mac", mac, "error", err)
			return
		}
		s.sendResponse(resp, msg, addr, conn, serverIP)

	case dhcpv4.MessageTypeRequest:
		resp, err := s.HandleRequest(msg, ifaceName, serverIP)
		if err != nil {
			// An internal failure is not the client's fault. Answering with a
			// NAK would make every client discard a lease that still works —
			// turning a control-plane problem into a network outage. Protocol
			// rejections are already returned as a NAK by HandleRequest, so
			// stay silent here and let the client retry.
			slog.Error("DHCP: HandleRequest error, staying silent", "mac", mac, "error", err)
			return
		}
		if resp == nil {
			// Deliberately no reply: the message was addressed to another
			// server, or the state requires silence (RFC 2131 §4.3.2).
			return
		}
		s.sendResponse(resp, msg, addr, conn, serverIP)

	case dhcpv4.MessageTypeRelease:
		if err := s.HandleRelease(msg); err != nil {
			slog.Error("DHCP: HandleRelease error", "mac", mac, "error", err)
		}

	case dhcpv4.MessageTypeDecline:
		if err := s.HandleDecline(msg); err != nil {
			slog.Error("DHCP: HandleDecline error", "mac", mac, "error", err)
		}

	case dhcpv4.MessageTypeInform:
		resp, err := s.HandleInform(msg, ifaceName, serverIP)
		if err != nil {
			slog.Error("DHCP: HandleInform error", "mac", mac, "error", err)
			return
		}
		s.sendResponse(resp, msg, addr, conn, serverIP)
	}

	// Log the event after processing (scope is determined within each handler).
	if s.eventLogger != nil {
		_ = s.eventLogger.LogDHCPEvent(dhcpinternal.DHCPEventType(msgType.String()), "", mac, clientIP, "")
	}
}

// sendResponse sends a DHCP response packet.
func (s *Server) sendResponse(resp *dhcpv4.DHCPv4, req *dhcpv4.DHCPv4, addr net.Addr, conn net.PacketConn, serverIP net.IP) {
	// Determine destination.
	var dest net.Addr
	if req.GatewayIPAddr != nil && !req.GatewayIPAddr.IsUnspecified() {
		// Relay agent: send to relay agent with port 67.
		dest = &net.UDPAddr{IP: req.GatewayIPAddr, Port: 67}
	} else if !req.ClientIPAddr.IsUnspecified() {
		// Client already has an IP: unicast to it.
		dest = &net.UDPAddr{IP: req.ClientIPAddr, Port: 68}
	} else {
		// Broadcast to client.
		dest = &net.UDPAddr{IP: net.IPv4bcast, Port: 68}
	}

	data := resp.ToBytes()
	if _, err := conn.WriteTo(data, dest); err != nil {
		slog.Error("DHCP server: failed to send response", "dest", dest, "error", err)
	}
}

// buildNAK builds a DHCP NAK response.
func (s *Server) buildNAK(req *dhcpv4.DHCPv4, serverIP net.IP, message string) (*dhcpv4.DHCPv4, error) {
	resp, err := dhcpv4.NewReplyFromRequest(req,
		dhcpv4.WithMessageType(dhcpv4.MessageTypeNak),
		dhcpv4.WithServerIP(serverIP),
	)
	if err != nil {
		return nil, err
	}

	if message != "" && s.eventLogger != nil {
		_ = s.eventLogger.LogDHCPEvent(dhcpinternal.EventNak, "", req.ClientHWAddr.String(), "", message)
	}

	return resp, nil
}

// leaseExpiryLoop periodically expires old leases.
func (s *Server) leaseExpiryLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.sweepExpiredLeases("periodic")
		case <-s.quit:
			return
		}
	}
}

// sweepExpiredLeases expires lapsed leases and queues the DNS teardown for every
// confirmed binding among them.
//
// The teardown is queued rather than performed here so it is retried on
// failure and survives a restart, exactly like the updates driven by REQUEST.
// Only confirmed bindings are queued: an expired offer or quarantine never
// published a name, so it owns no record to withdraw.
func (s *Server) sweepExpiredLeases(reason string) {
	swept, err := s.leaseMgr.ExpireLeases()
	if err != nil {
		slog.Error("DHCP server: failed to expire leases", "reason", reason, "error", err)
		return
	}
	queued := 0
	for _, l := range swept {
		// Every swept lease stopped being held, whether or not it ever had a
		// name. The observation is what takes the address out of the "in use"
		// column of the IPAM view; the DNS teardown below only applies to
		// confirmed bindings, which are the ones that published one.
		s.observeLease(LeaseObservedExpire, l)

		// The sweep returned the rows as they were before it updated them, so
		// the new status is applied here rather than read back. Hundreds of
		// re-reads per tick would be a second pass over the same table for no
		// new information.
		expired := *l
		expired.Status = lease.LeaseStatusExpired
		s.replicateLeaseState(&expired)

		if l.Status != lease.LeaseStatusActive || l.Hostname == "" {
			continue
		}
		if err := s.enqueueDNSEvent(l, dhcpinternal.DNSEventDelete); err != nil {
			slog.Error("DHCP server: failed to queue DNS teardown for expired lease",
				"lease_id", l.ID, "error", err)
			continue
		}
		queued++
	}
	if queued > 0 {
		slog.Info("DHCP server: queued DNS teardown for expired leases",
			"reason", reason, "count", queued)
	}
}

// enqueueDNSEvent records a DNS change owed by a lease.
//
// The row is written before the DHCP reply is returned, so the intent is
// durable by the time the client is told it may use the address. It is not in
// the same transaction as the lease write — that would require threading a
// transaction through the lease manager, which belongs with the data-plane
// split — so a crash in the gap leaves a binding with no event; the reconciler
// covers exactly that case.
//
// Which wake-up can be sent, and which cannot:
//
//   - The consumer cannot be woken. The entry is applied by the plane that
//     writes the records, and in a split deployment that is another process on
//     another file system. It has no channel to this one, and the only way it
//     learns of the entry is that this store's replication pushes it and the
//     other side pulls it. A channel here would signal a process that cannot
//     see the row yet.
//   - This process's own outbox push can be woken, and is. The row is
//     committed here, in a store this process owns, and the push is a loop this
//     process runs. Without the signal the row waits for the poll interval,
//     which is one of the two intervals between a confirmed binding and a
//     resolvable name; with it, the only wait left on this side is the push
//     itself.
func (s *Server) enqueueDNSEvent(l *lease.Lease, action dhcpinternal.DNSEventAction) error {
	if l == nil {
		return nil
	}
	err := s.outbox.Enqueue(dhcpinternal.DNSEvent{
		LeaseID:    l.ID,
		Generation: l.Generation,
		Action:     action,
		ScopeID:    l.ScopeID,
		IPAddress:  l.IPAddress,
		MACAddress: l.MACAddress,
		Hostname:   l.Hostname,
	})
	if err == nil {
		s.wakeOutbox()
	}
	return err
}

// SetOutboxWake installs the signal sent after a DNS change is queued. It is
// called once during wiring and may be left unset, in which case the entry
// waits for the replication loop's next pass.
//
// It is a callback rather than a channel this package drains, because the
// replication loop belongs to the process that owns the store, not to the
// DHCP server, and the data plane must not have to know which loop it is
// feeding.
func (s *Server) SetOutboxWake(fn func()) { s.outboxWake = fn }

func (s *Server) wakeOutbox() {
	if s.outboxWake != nil {
		s.outboxWake()
	}
}

// isStopped checks if the server has been stopped.
func (s *Server) isStopped() bool {
	select {
	case <-s.quit:
		return true
	default:
		return false
	}
}

// Shutdown gracefully stops the DHCP server.
func (s *Server) Shutdown(ctx context.Context) error {
	slog.Info("DHCP server: shutting down...")
	close(s.quit)

	// Close all listeners.
	for _, conn := range s.listeners {
		conn.Close()
	}

	// Wait for receive loops to finish with a timeout.
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		slog.Info("DHCP server: receive loops exited cleanly")
	case <-time.After(5 * time.Second):
		slog.Warn("DHCP server: shutdown timed out waiting for receive loops")
	case <-ctx.Done():
		slog.Warn("DHCP server: shutdown context cancelled")
	}

	slog.Info("DHCP server: stopped")
	return nil
}
