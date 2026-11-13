package middleware

import (
	"log/slog"
	"net/http"
	"strings"
)

// ConfigAuthor refuses configuration writes on a node that is not the
// deployment's configuration author.
//
// ADR-0005 requires that only one node may author configuration, so that two
// nodes can never publish divergent revisions of the same resource. The full
// coordinator (leader election, quorum, witness, epoch fencing, watermark
// catch-up) is not implemented; this middleware is the smallest honest part of
// that contract: a deployment-time single-author statement enforced at the API
// boundary, failing closed with a clear reason rather than silently accepting a
// second writer.
//
// It is installed only on mutating configuration routes. Reads, metrics, health
// and the DHCP/DNS data planes are untouched, so a non-author node keeps
// serving traffic and reporting state exactly as before.
// configPaths are the API prefixes whose writes are configuration: the objects a
// second node could otherwise publish a divergent revision of. Account, auth and
// operational endpoints are deliberately not here -- they are per-node state, not
// shared configuration.
var configPaths = []string{"/dns/", "/dhcp/", "/ipam/", "/ddi/"}

// isConfigWrite reports whether this request mutates configuration. Reads
// (GET/HEAD/OPTIONS) always pass, so a non-author node keeps serving the
// console, metrics and the data planes.
func isConfigWrite(r *http.Request) bool {
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return false
	}
	for _, prefix := range configPaths {
		if strings.Contains(r.URL.Path, prefix) {
			return true
		}
	}
	return false
}

func ConfigAuthor(author bool, nodeName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if author {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isConfigWrite(r) {
				next.ServeHTTP(w, r)
				return
			}
			slog.Warn("config write refused: this node is not the configuration author",
				"method", r.Method, "path", r.URL.Path, "node", nodeName,
				"reason", "cluster.config_author=false (ADR-0005 single-author rule)")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"code":409,"message":"此节点不是配置作者，配置写操作已被拒绝（cluster.config_author=false）；请在配置作者节点上执行，或调整部署配置"}`))
		})
	}
}
