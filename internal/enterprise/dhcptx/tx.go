// Package dhcptx contains an isolated DHCP transaction prototype. It supplies
// no network replies, authority, fencing, trusted clock, or replication apply.
package dhcptx

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jincaiw/GoDDI/internal/enterprise/eventlog"
)

var ErrNoAuthority = errors.New("dhcptx: no write authority")
var ErrStopped = errors.New("dhcptx: fixture stopped")
var ErrInvalid = errors.New("dhcptx: invalid transaction")

// Begin and Commit are deliberately closed pending real authorization.
func Begin(context.Context) error  { return ErrNoAuthority }
func Commit(context.Context) error { return ErrNoAuthority }

// command and fixture are inaccessible to other packages. They model a
// previously selected IPv4 address and simulated authority, not allocation.
type command struct {
	Tenant, Space, LeaseID               string
	ClientKey                            []byte
	XID                                  uint32
	Kind                                 string
	Address                              string
	Stage, ServerID, RequestedIP, CIAddr string
	IssuedMS, ExpiresMS                  int64
}
type reply struct {
	Kind, Address       string
	ExpiresMS, Revision int64
	Seq                 uint64
}
type fixture struct {
	mu                      sync.Mutex
	db                      *sql.DB
	cluster, node           eventlog.ID
	tenant, space, serverID string
	head                    eventlog.Position
	lastMS                  int64
	stopped                 bool
	beforeCommit            func() error
	afterCommit             func() error
}

func canonicalUUID(s string) bool {
	u, e := uuid.Parse(s)
	return e == nil && u != uuid.Nil && u.String() == s
}
func (c command) validate() error {
	if !canonicalUUID(c.Tenant) || !canonicalUUID(c.Space) || !canonicalUUID(c.LeaseID) || len(c.ClientKey) == 0 || len(c.ClientKey) > 256 || c.XID == 0 || (c.Kind != "OFFER" && c.Kind != "ACK") || c.IssuedMS <= 0 || c.ExpiresMS <= c.IssuedMS {
		return ErrInvalid
	}
	a, e := netip.ParseAddr(c.Address)
	if e != nil || !a.Is4() || a.String() != c.Address {
		return ErrInvalid
	}
	if c.Kind == "OFFER" {
		if c.Stage != "" || c.ServerID != "" || c.RequestedIP != "" || c.CIAddr != "" {
			return ErrInvalid
		}
	} else {
		switch c.Stage {
		case "SELECTING":
			s, err := netip.ParseAddr(c.ServerID)
			if err != nil || !s.Is4() || s.IsUnspecified() || s.String() != c.ServerID || c.RequestedIP != c.Address || c.CIAddr != "" {
				return ErrInvalid
			}
		case "INIT_REBOOT":
			if c.ServerID != "" || c.RequestedIP != c.Address || c.CIAddr != "" {
				return ErrInvalid
			}
		case "RENEW_REBIND":
			if c.ServerID != "" || c.RequestedIP != "" || c.CIAddr != c.Address {
				return ErrInvalid
			}
		default:
			return ErrInvalid
		}
	}
	return nil
}
func one(ctx context.Context, c *sql.Conn, q string, args ...any) error {
	r, e := c.ExecContext(ctx, q, args...)
	if e != nil {
		return e
	}
	n, e := r.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return ErrInvalid
	}
	return nil
}

// apply returns a test result only after COMMIT succeeds; it is never a DHCP
// packet and must not be used to transmit one. The client key must not be
// mutated concurrently. Errors permanently stop this in-process fixture.
func (f *fixture) apply(ctx context.Context, input command) (out reply, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stopped {
		return out, ErrStopped
	}
	defer func() {
		if err != nil {
			f.stopped = true
			out = reply{}
		}
	}()
	c := input
	c.ClientKey = append([]byte(nil), input.ClientKey...)
	if err = c.validate(); err != nil {
		return out, err
	}
	if c.Tenant != f.tenant || c.Space != f.space || (c.Stage == "SELECTING" && c.ServerID != f.serverID) {
		return out, ErrInvalid
	}
	// The stable request digest excludes the simulated server clock and expiry.
	raw, err := json.Marshal(struct {
		Tenant, Space, LeaseID                              string
		ClientKey                                           []byte
		XID                                                 uint32
		Kind, Address, Stage, ServerID, RequestedIP, CIAddr string
	}{c.Tenant, c.Space, c.LeaseID, c.ClientKey, c.XID, c.Kind, c.Address, c.Stage, c.ServerID, c.RequestedIP, c.CIAddr})
	if err != nil {
		return out, err
	}
	digest := sha256.Sum256(raw)
	conn, err := f.db.Conn(ctx)
	if err != nil {
		return out, err
	}
	defer conn.Close()
	for _, check := range []struct{ q, w string }{{"PRAGMA foreign_keys", "1"}, {"PRAGMA journal_mode", "wal"}, {"PRAGMA synchronous", "2"}} {
		var got string
		if err = conn.QueryRowContext(ctx, check.q).Scan(&got); err != nil {
			return out, err
		}
		if got != check.w {
			return out, ErrInvalid
		}
	}
	if _, err = conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return out, err
	}
	done := false
	defer func() {
		if done {
			return
		}
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if _, e := conn.ExecContext(cleanup, "ROLLBACK"); e != nil {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			err = errors.Join(err, fmt.Errorf("dhcptx: rollback unknown: %w", e))
		}
	}()
	var epoch, seq, schema int64
	var hash []byte
	err = conn.QueryRowContext(ctx, `SELECT epoch,seq,schema_version,CASE WHEN typeof(hash)='blob' AND length(hash)=32 THEN hash END FROM replica_head WHERE cluster_id=?`, f.cluster[:]).Scan(&epoch, &seq, &schema, &hash)
	if err != nil || epoch <= 0 || seq < 0 || schema != 1 || uint64(epoch) != f.head.Epoch || uint64(seq) != f.head.Seq || !bytes.Equal(hash, f.head.Hash[:]) {
		return out, ErrInvalid
	}
	var headTime int64
	if seq == 0 {
		if epoch != 1 || f.head.Hash != eventlog.GenesisHash() {
			return out, ErrInvalid
		}
	} else {
		var stored eventlog.Event
		var leader, prev, digestBytes, payload []byte
		err = conn.QueryRowContext(ctx, `SELECT leader_id,schema_version,committed_unix_ms,prev_hash,hash,CASE WHEN length(payload)<=? THEN payload END FROM replica_event WHERE cluster_id=? AND epoch=? AND seq=?`, eventlog.MaxPayloadBytes, f.cluster[:], epoch, seq).Scan(&leader, &stored.Schema, &stored.CommitMs, &prev, &digestBytes, &payload)
		if err != nil || len(leader) != 16 || len(prev) != 32 || len(digestBytes) != 32 || len(payload) == 0 {
			return out, ErrInvalid
		}
		copy(stored.LeaderID[:], leader)
		copy(stored.PrevHash[:], prev)
		copy(stored.Position.Hash[:], digestBytes)
		stored.ClusterID = f.cluster
		stored.Position.Epoch = uint64(epoch)
		stored.Position.Seq = uint64(seq)
		stored.Payload = payload
		if stored.LeaderID != f.node || stored.Position.Hash != f.head.Hash || stored.Verify() != nil {
			return out, ErrInvalid
		}
		headTime = stored.CommitMs
	}
	if c.IssuedMS < headTime || c.IssuedMS < f.lastMS {
		return out, ErrInvalid
	}
	var oldDigest []byte
	var replay reply
	err = conn.QueryRowContext(ctx, `SELECT CASE WHEN typeof(digest)='blob' AND length(digest)=32 THEN digest END, CASE WHEN typeof(reply_kind)='text' AND length(CAST(reply_kind AS BLOB))<=5 THEN reply_kind END, CASE WHEN typeof(address)='text' AND length(CAST(address AS BLOB))<=15 THEN address END, CASE WHEN typeof(expires_ms)='integer' AND expires_ms>0 THEN expires_ms END, CASE WHEN typeof(revision)='integer' AND revision>0 THEN revision END, CASE WHEN typeof(seq)='integer' AND seq>0 THEN seq END FROM dhcp_receipt WHERE tenant=? AND space=? AND client_key=? AND xid=? AND kind=?`, c.Tenant, c.Space, c.ClientKey, int64(c.XID), c.Kind).Scan(&oldDigest, &replay.Kind, &replay.Address, &replay.ExpiresMS, &replay.Revision, &replay.Seq)
	if err == nil {
		if !bytes.Equal(oldDigest, digest[:]) || replay.Kind != c.Kind || replay.Address != c.Address || replay.ExpiresMS <= c.IssuedMS || replay.Revision <= 0 || replay.Seq == 0 || replay.Seq != uint64(seq) {
			return out, ErrInvalid
		}
		if err = f.verifyReplay(ctx, conn, c, replay, epoch); err != nil {
			return out, err
		}
		if _, err = conn.ExecContext(ctx, "ROLLBACK"); err != nil {
			return out, err
		}
		done = true
		f.lastMS = c.IssuedMS
		return replay, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	var existingID, oldKind string
	var oldRevision, oldExpiry, oldIssued, offerXID int64
	err = conn.QueryRowContext(ctx, `SELECT lease_id,kind,revision,expires_ms,issued_ms,offer_xid FROM dhcp_lease WHERE tenant=? AND space=? AND client_key=? AND address=?`, c.Tenant, c.Space, c.ClientKey, c.Address).Scan(&existingID, &oldKind, &oldRevision, &oldExpiry, &oldIssued, &offerXID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if c.Kind != "OFFER" {
			return out, ErrInvalid
		}
		oldRevision = 0
	case err != nil:
		return out, err
	default:
		if existingID != c.LeaseID || oldRevision <= 0 || oldRevision == math.MaxInt64 || oldIssued <= 0 || oldExpiry <= oldIssued || c.IssuedMS < oldIssued || offerXID <= 0 || offerXID > math.MaxUint32 {
			return out, ErrInvalid
		}
		// Verify every existing lease against the trusted head before using its
		// expiry or state to permit any transition, including a new OFFER.
		if err = f.verifyCurrentLease(ctx, conn, c, oldKind, oldExpiry, oldRevision, epoch, seq); err != nil {
			return out, err
		}
		if c.Kind == "ACK" && ((oldKind != "OFFER" && oldKind != "ACK") || c.IssuedMS >= oldExpiry) {
			return out, ErrInvalid
		}
		if c.Kind == "ACK" && oldKind == "OFFER" {
			if c.Stage != "SELECTING" || int64(c.XID) != offerXID {
				return out, ErrInvalid
			}
		}
		if c.Kind == "ACK" && oldKind == "ACK" {
			if c.Stage == "SELECTING" {
				return out, ErrInvalid
			}
		}
		if c.Kind == "OFFER" && oldKind == "OFFER" && c.IssuedMS < oldExpiry {
			return out, ErrInvalid
		}
		if c.Kind == "ACK" && oldKind == "ACK" && c.ExpiresMS < oldExpiry {
			return out, ErrInvalid
		}
		if c.Kind == "OFFER" && oldKind == "ACK" && c.IssuedMS < oldExpiry {
			return out, ErrInvalid
		}
	}
	if seq == math.MaxInt64 {
		return out, ErrInvalid
	}
	revision := oldRevision + 1
	if c.Kind == "OFFER" {
		offerXID = int64(c.XID)
	}
	if oldRevision == 0 {
		err = one(ctx, conn, `INSERT INTO dhcp_lease(tenant,space,lease_id,client_key,address,kind,offer_xid,issued_ms,expires_ms,revision) VALUES(?,?,?,?,?,?,?,?,?,?)`, c.Tenant, c.Space, c.LeaseID, c.ClientKey, c.Address, c.Kind, offerXID, c.IssuedMS, c.ExpiresMS, revision)
	} else {
		err = one(ctx, conn, `UPDATE dhcp_lease SET kind=?,offer_xid=?,issued_ms=?,expires_ms=?,revision=? WHERE tenant=? AND space=? AND lease_id=? AND client_key=? AND address=? AND revision=?`, c.Kind, offerXID, c.IssuedMS, c.ExpiresMS, revision, c.Tenant, c.Space, c.LeaseID, c.ClientKey, c.Address, oldRevision)
	}
	if err != nil {
		return out, err
	}
	body, err := json.Marshal(leaseBody{Version: "2", Tenant: c.Tenant, Space: c.Space, Lease: c.LeaseID, Client: hex.EncodeToString(c.ClientKey), XID: strconv.FormatUint(uint64(c.XID), 10), OfferXID: strconv.FormatInt(offerXID, 10), Kind: c.Kind, Address: c.Address, Stage: c.Stage, ServerID: c.ServerID, RequestedIP: c.RequestedIP, CIAddr: c.CIAddr, Issued: strconv.FormatInt(c.IssuedMS, 10), Expires: strconv.FormatInt(c.ExpiresMS, 10), Revision: strconv.FormatInt(revision, 10)})
	if err != nil {
		return out, err
	}
	payload, err := eventlog.MarshalChanges(eventlog.Change{Kind: "dhcp.lease.committed", ObjectID: c.LeaseID, Revision: strconv.FormatInt(revision, 10), Body: body})
	if err != nil {
		return out, err
	}
	ev := eventlog.Event{ClusterID: f.cluster, LeaderID: f.node, Position: eventlog.Position{Epoch: uint64(epoch), Seq: uint64(seq + 1)}, Schema: 1, CommitMs: c.IssuedMS, PrevHash: f.head.Hash, Payload: payload}
	ev.Position.Hash, err = ev.Hash()
	if err != nil {
		return out, err
	}
	if err = one(ctx, conn, `INSERT INTO replica_event(cluster_id,epoch,seq,leader_id,schema_version,committed_unix_ms,prev_hash,hash,payload) VALUES(?,?,?,?,?,?,?,?,?)`, f.cluster[:], epoch, seq+1, f.node[:], 1, c.IssuedMS, ev.PrevHash[:], ev.Position.Hash[:], payload); err != nil {
		return out, err
	}
	if err = one(ctx, conn, `INSERT INTO dhcp_outbox(cluster_id,epoch,seq,hash,payload) VALUES(?,?,?,?,?)`, f.cluster[:], epoch, seq+1, ev.Position.Hash[:], payload); err != nil {
		return out, err
	}
	if err = one(ctx, conn, `INSERT INTO dhcp_receipt(tenant,space,client_key,xid,kind,digest,reply_kind,address,expires_ms,revision,seq) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, c.Tenant, c.Space, c.ClientKey, int64(c.XID), c.Kind, digest[:], c.Kind, c.Address, c.ExpiresMS, revision, seq+1); err != nil {
		return out, err
	}
	if err = one(ctx, conn, `UPDATE replica_head SET seq=?,hash=?,schema_version=1,advanced_at_unix_ms=? WHERE cluster_id=? AND epoch=? AND seq=? AND hash=?`, seq+1, ev.Position.Hash[:], c.IssuedMS, f.cluster[:], epoch, seq, hash); err != nil {
		return out, err
	}
	if f.beforeCommit != nil {
		if err = f.beforeCommit(); err != nil {
			return out, err
		}
	}
	if _, err = conn.ExecContext(ctx, "COMMIT"); err != nil {
		return out, fmt.Errorf("dhcptx: commit outcome unknown: %w", err)
	}
	if f.afterCommit != nil {
		if err = f.afterCommit(); err != nil {
			return out, fmt.Errorf("dhcptx: commit outcome unknown: %w", err)
		}
	}
	done = true
	f.lastMS = c.IssuedMS
	f.head = ev.Position
	return reply{c.Kind, c.Address, c.ExpiresMS, revision, uint64(seq + 1)}, nil
}
