package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

// DNSSecondaryZoneExempt, when true, lets a non-author node create DNS
// *secondary* zones.
//
// A secondary zone is not an authored configuration: it is a follower that
// mirrors a primary elsewhere, and it is exactly how DNS redundancy is deployed
// on a pair of nodes. The single-author rule (ADR-0011) exists to stop two nodes
// publishing divergent revisions of the same resource; a secondary cannot
// diverge because it only ever copies. The two axes -- "who may author" and
// "who may follow" -- are therefore different, and this exemption keeps the
// rule from blocking the DNS redundancy the deployment needs.
//
// Only the creation of a zone whose body says type=secondary is exempt. Every
// other configuration write on a non-author node stays refused.
var DNSSecondaryZoneExempt = true

// isDNSSecondaryCreate reports whether this request creates a secondary zone.
func isDNSSecondaryCreate(r *http.Request) bool {
	if r.Method != http.MethodPost || !strings.HasSuffix(strings.TrimRight(r.URL.Path, "/"), "/dns/zones") {
		return false
	}
	body, err := readBodyPeek(r)
	if err != nil {
		return false
	}
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(probe.Type), "secondary")
}

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
			if DNSSecondaryZoneExempt && isDNSSecondaryCreate(r) {
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

// readBodyPeek reads the request body and restores it for the handler.
//
// The middleware needs one field out of the body; consuming it without putting
// it back would make the handler see an empty request and fail a write that the
// middleware had just allowed.
func readBodyPeek(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	const maxPeek = 1 << 16
	buf, err := io.ReadAll(io.LimitReader(r.Body, maxPeek))
	r.Body = io.NopCloser(bytes.NewReader(buf))
	return buf, err
}
