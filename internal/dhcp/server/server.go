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
	"golang.org/x/sync/semaphore"

	dhcpinternal "github.com/jasonwa/goddi/internal/dhcp"
)

// dnsUpdateConcurrency caps the number of concurrent DNS updates triggered
// asynchronously from DHCP handlers. This protects the database / DNS writers
// from burst DHCP traffic (e.g. a switch reboot powering on many clients at
// once) which could otherwise spawn an unbounded number of goroutines.
const dnsUpdateConcurrency = 10

// Server represents a DHCPv4 server.
type Server struct {
	db          *sql.DB
	scopeMgr    *scope.Manager
	leaseMgr    *lease.Manager
	reservMgr   *reservation.Manager
	optionMgr   *option.Manager
	dnsLink     *dhcpinternal.DNSLink
	eventLogger *dhcpinternal.EventLogger

	interfaces []string
	listeners  []net.PacketConn
	serverIPs  map[string]net.IP // interface -> server IP

	// dnsUpdateSem limits concurrent DNS update goroutines.
	dnsUpdateSem *semaphore.Weighted

	quit chan struct{}
	wg   sync.WaitGroup
}

// New creates a new DHCP server.
func New(db *sql.DB, interfaces []string, eventLogger *dhcpinternal.EventLogger) *Server {
	return &Server{
		db:           db,
		scopeMgr:     scope.NewManager(db),
		leaseMgr:     lease.NewManager(db),
		reservMgr:    reservation.NewManager(db),
		optionMgr:    option.NewManager(db),
		dnsLink:      dhcpinternal.NewDNSLink(db),
		eventLogger:  eventLogger,
		interfaces:   interfaces,
		serverIPs:    make(map[string]net.IP),
		dnsUpdateSem: semaphore.NewWeighted(dnsUpdateConcurrency),
		quit:         make(chan struct{}),
	}
}

// Start starts the DHCP server.
func (s *Server) Start(ctx context.Context) error {
	// Expire old leases on startup.
	if err := s.leaseMgr.ExpireLeases(); err != nil {
		slog.Warn("DHCP server: failed to expire leases on startup", "error", err)
	}

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
			slog.Error("DHCP: HandleRequest error", "mac", mac, "error", err)
			// Send NAK on error.
			nak, nakErr := s.buildNAK(msg, serverIP, err.Error())
			if nakErr == nil {
				s.sendResponse(nak, msg, addr, conn, serverIP)
			}
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
			if err := s.leaseMgr.ExpireLeases(); err != nil {
				slog.Error("DHCP server: failed to expire leases", "error", err)
			}
		case <-s.quit:
			return
		}
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
