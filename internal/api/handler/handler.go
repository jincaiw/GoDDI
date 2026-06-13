package handler

import (
	"encoding/json"
	"net/http"
	"sync"
)

// DegradedState tracks which services are in a degraded state.
var (
	degradedMu   sync.RWMutex
	degradedDNS  bool
	degradedDHCP bool
)

// SetDNSDegraded marks the DNS service as degraded.
func SetDNSDegraded(degraded bool) {
	degradedMu.Lock()
	defer degradedMu.Unlock()
	degradedDNS = degraded
}

// SetDHCPDegraded marks the DHCP service as degraded.
func SetDHCPDegraded(degraded bool) {
	degradedMu.Lock()
	defer degradedMu.Unlock()
	degradedDHCP = degraded
}

// Health handles the health check endpoint.
// It returns only a coarse "ok"/"degraded" status and intentionally does
// not expose internal per-service state (e.g. which subsystem is degraded),
// because the unauthenticated /health endpoint is reachable by any caller
// and should not be a reconnaissance aid.
func Health(w http.ResponseWriter, r *http.Request) {
	degradedMu.RLock()
	anyDegraded := degradedDNS || degradedDHCP
	degradedMu.RUnlock()

	if anyDegraded {
		// Intentionally return a generic status; do not leak which
		// subsystem is degraded or any internal counters.
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "degraded",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
	})
}
