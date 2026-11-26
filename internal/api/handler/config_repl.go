package handler

import (
	"crypto/subtle"
	"database/sql"
	"net/http"

	"github.com/jincaiw/GoDDI/internal/configrepl"
	"github.com/jincaiw/GoDDI/internal/dataplane"
)

// ConfigSnapshotHandler serves ADR-0013 configuration replication: it returns
// this node's configuration tables as a JSON snapshot the follower applies.
//
// It is authorised by a shared secret header rather than a session, because the
// follower is a peer process, not a browser: X-Goddi-Replication-Token must
// match the configured token. When the token is empty the endpoint is closed
// (503) rather than open -- an unauthenticated whole-config dump would leak all
// zones and scopes to anyone who can reach the port.
func ConfigSnapshotHandler(controlDB *sql.DB, nodeID, token string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if controlDB == nil {
			http.Error(w, "config replication: no control database", http.StatusServiceUnavailable)
			return
		}
		if token == "" {
			// Fail closed: without a configured token there is no safe way to
			// tell a peer from an intruder.
			http.Error(w, "config replication: not enabled (no token configured)", http.StatusServiceUnavailable)
			return
		}
		got := r.Header.Get("X-Goddi-Replication-Token")
		if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			http.Error(w, "config replication: bad token", http.StatusUnauthorized)
			return
		}
		snap, err := configrepl.ReadSnapshot(r.Context(), controlDB, nodeID, dataplane.ConfigTableSpecs())
		if err != nil {
			http.Error(w, "config replication: "+err.Error(), http.StatusInternalServerError)
			return
		}
		body, err := configrepl.EncodeSnapshotJSON(snap)
		if err != nil {
			http.Error(w, "config replication: encoding snapshot: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}
}
