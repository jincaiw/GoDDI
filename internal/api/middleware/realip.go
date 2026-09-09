package middleware

import (
	"net"
	"net/http"
	"strings"
)

// TrustedRealIP uses forwarding headers only when the direct TCP peer belongs
// to a configured reverse-proxy network. With no trusted CIDRs it deliberately
// leaves RemoteAddr untouched, which is the safe default for direct access.
func TrustedRealIP(trustedCIDRs []string) func(http.Handler) http.Handler {
	trusted := parseTrustedNetworks(trustedCIDRs)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			peer := peerIP(r.RemoteAddr)
			if peer != nil && isTrustedPeer(peer, trusted) {
				if clientIP := forwardedClientIP(r); clientIP != "" {
					r.RemoteAddr = net.JoinHostPort(clientIP, "0")
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func parseTrustedNetworks(cidrs []string) []*net.IPNet {
	networks := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, network, err := net.ParseCIDR(cidr)
		if err == nil {
			networks = append(networks, network)
		}
	}
	return networks
}

func peerIP(remoteAddr string) net.IP {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	return net.ParseIP(host)
}

func isTrustedPeer(peer net.IP, networks []*net.IPNet) bool {
	for _, network := range networks {
		if network.Contains(peer) {
			return true
		}
	}
	return false
}

func forwardedClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// A correctly configured trusted proxy overwrites XFF. Take its leftmost
		// client value and reject malformed header values instead of preserving
		// attacker-controlled text in RemoteAddr/audit logs.
		if ip := net.ParseIP(strings.TrimSpace(strings.Split(xff, ",")[0])); ip != nil {
			return ip.String()
		}
	}
	if ip := net.ParseIP(strings.TrimSpace(r.Header.Get("X-Real-IP"))); ip != nil {
		return ip.String()
	}
	return ""
}
