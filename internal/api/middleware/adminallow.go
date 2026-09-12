package middleware

import (
	"log/slog"
	"net"
	"net/http"
)

// AdminAllowList restricts the management surface to a set of source networks.
//
// An empty list is a no-op, which is the default and what a direct deployment
// needs: the console listens on an address only its operator can reach, or it
// sits behind a proxy that already decides who may talk to it. Turning the list
// on is a deployment decision, so nothing here invents a default.
//
// It reads the client from r.RemoteAddr, which is the address TrustedRealIP has
// already rewritten for a configured proxy and left alone otherwise. That makes
// the allowlist agree with the rate limiter and the audit trail about who the
// client is; a second, independent notion of "the client address" is how an
// allowlist ends up guarding the proxy instead of the client.
//
// Fail-closed on an entry that does not parse. Config validation refuses such an
// entry at startup, so this is a backstop rather than a path anyone walks: the
// alternative -- ignoring the entry -- would widen the allowlist silently, and
// refusing everything at least leaves an error line that says why.
func AdminAllowList(cidrs []string) func(http.Handler) http.Handler {
	if len(cidrs) == 0 {
		return func(next http.Handler) http.Handler { return next }
	}

	networks := make([]*net.IPNet, 0, len(cidrs))
	broken := false
	for _, cidr := range cidrs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			slog.Error("management allowlist entry does not parse; refusing every management request until it is fixed",
				"entry", cidr, "error", err)
			broken = true
			continue
		}
		networks = append(networks, network)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			peer := peerIP(r.RemoteAddr)
			if !broken && peer != nil && isTrustedPeer(peer, networks) {
				next.ServeHTTP(w, r)
				return
			}
			// No credential prompt and no detail: a client that is outside the
			// allowlist is outside it whether or not it holds a valid token,
			// and telling it which would be the only information the check
			// produces.
			slog.Warn("management request refused by the allowlist",
				"remote_addr", r.RemoteAddr, "path", r.URL.Path)
			writeForbiddenError(w, "this client is not allowed to reach the management API")
		})
	}
}
