package dhcptx

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"strconv"

	"github.com/jasonwa/goddi/internal/enterprise/eventlog"
)

type leaseBody struct {
	Version     string `json:"version"`
	Tenant      string `json:"tenantId"`
	Space       string `json:"spaceId"`
	Lease       string `json:"leaseId"`
	Client      string `json:"clientKeyHex"`
	XID         string `json:"xid"`
	OfferXID    string `json:"offerXid"`
	Kind        string `json:"kind"`
	Address     string `json:"address"`
	Stage       string `json:"stage"`
	ServerID    string `json:"serverId"`
	RequestedIP string `json:"requestedIp"`
	CIAddr      string `json:"ciAddr"`
	Issued      string `json:"issuedMs"`
	Expires     string `json:"expiresMs"`
	Revision    string `json:"revision"`
}

// verifyCurrentLease reconstructs the prior command only from the head event;
// verifyReplay then binds it to the current lease and outbox.
func (f *fixture) verifyCurrentLease(ctx context.Context, conn *sql.Conn, incoming command, oldKind string, expiry, revision, epoch, seq int64) error {
	if seq <= 0 {
		return ErrInvalid
	}
	var payload []byte
	if err := conn.QueryRowContext(ctx, `SELECT CASE WHEN length(payload)<=? THEN payload END FROM replica_event WHERE cluster_id=? AND epoch=? AND seq=?`, eventlog.MaxPayloadBytes, f.cluster[:], epoch, seq).Scan(&payload); err != nil {
		return ErrInvalid
	}
	changes, err := eventlog.DecodePayload(payload)
	if err != nil || len(changes) != 1 {
		return ErrInvalid
	}
	var body leaseBody
	if err := json.Unmarshal(changes[0].Body, &body); err != nil {
		return ErrInvalid
	}
	xid, err := strconv.ParseUint(body.XID, 10, 32)
	if err != nil || xid == 0 {
		return ErrInvalid
	}
	previous := incoming
	previous.Kind = oldKind
	previous.XID = uint32(xid)
	previous.Stage, previous.ServerID, previous.RequestedIP, previous.CIAddr = body.Stage, body.ServerID, body.RequestedIP, body.CIAddr
	if previous.validate() != nil || (previous.Stage == "SELECTING" && previous.ServerID != f.serverID) {
		return ErrInvalid
	}
	return f.verifyReplay(ctx, conn, previous, reply{Kind: oldKind, Address: incoming.Address, ExpiresMS: expiry, Revision: revision, Seq: uint64(seq)}, epoch)
}

// verifyReplay accepts only the current trusted head. Historical replay needs
// independent chain verification and retention policy, which this fixture lacks.
func (f *fixture) verifyReplay(ctx context.Context, conn *sql.Conn, c command, r reply, epoch int64) error {
	var leaseIssued, offerXID int64
	err := conn.QueryRowContext(ctx, `SELECT CASE WHEN typeof(issued_ms)='integer' AND issued_ms>0 THEN issued_ms END, CASE WHEN typeof(offer_xid)='integer' AND offer_xid BETWEEN 1 AND 4294967295 THEN offer_xid END FROM dhcp_lease WHERE tenant=? AND space=? AND lease_id=? AND client_key=? AND address=? AND kind=? AND expires_ms=? AND revision=?`, c.Tenant, c.Space, c.LeaseID, c.ClientKey, c.Address, c.Kind, r.ExpiresMS, r.Revision).Scan(&leaseIssued, &offerXID)
	if err != nil {
		return ErrInvalid
	}
	var leader, prev, hash, payload, outHash, outPayload []byte
	var schema uint32
	var commitMS int64
	err = conn.QueryRowContext(ctx, `SELECT CASE WHEN length(e.leader_id)=16 THEN e.leader_id END,e.schema_version,e.committed_unix_ms,CASE WHEN length(e.prev_hash)=32 THEN e.prev_hash END,CASE WHEN length(e.hash)=32 THEN e.hash END,CASE WHEN length(e.payload)<=? THEN e.payload END,CASE WHEN length(o.hash)=32 THEN o.hash END,CASE WHEN length(o.payload)<=? THEN o.payload END FROM replica_event e JOIN dhcp_outbox o ON e.cluster_id=o.cluster_id AND e.epoch=o.epoch AND e.seq=o.seq WHERE e.cluster_id=? AND e.epoch=? AND e.seq=?`, eventlog.MaxPayloadBytes, eventlog.MaxPayloadBytes, f.cluster[:], epoch, int64(r.Seq)).Scan(&leader, &schema, &commitMS, &prev, &hash, &payload, &outHash, &outPayload)
	if err != nil || len(leader) != 16 || len(prev) != 32 || len(hash) != 32 || len(payload) == 0 || !bytes.Equal(hash, outHash) || !bytes.Equal(payload, outPayload) || !bytes.Equal(hash, f.head.Hash[:]) {
		return ErrInvalid
	}
	ev := eventlog.Event{ClusterID: f.cluster, Position: eventlog.Position{Epoch: uint64(epoch), Seq: r.Seq}, Schema: schema, CommitMs: commitMS, Payload: payload}
	copy(ev.LeaderID[:], leader)
	copy(ev.PrevHash[:], prev)
	copy(ev.Position.Hash[:], hash)
	if ev.LeaderID != f.node || ev.Verify() != nil {
		return ErrInvalid
	}
	changes, err := eventlog.DecodePayload(payload)
	if err != nil || len(changes) != 1 || changes[0].Kind != "dhcp.lease.committed" || changes[0].ObjectID != c.LeaseID || changes[0].Revision != strconv.FormatInt(r.Revision, 10) {
		return ErrInvalid
	}
	var body leaseBody
	if err = json.Unmarshal(changes[0].Body, &body); err != nil {
		return ErrInvalid
	}
	encoded, err := json.Marshal(body)
	if err != nil || !bytes.Equal(encoded, changes[0].Body) {
		return ErrInvalid
	}
	if body.Version != "2" || body.Stage != c.Stage || body.ServerID != c.ServerID || body.RequestedIP != c.RequestedIP || body.CIAddr != c.CIAddr || body.Tenant != c.Tenant || body.Space != c.Space || body.Lease != c.LeaseID || body.Client != hex.EncodeToString(c.ClientKey) || body.XID != strconv.FormatUint(uint64(c.XID), 10) || body.OfferXID != strconv.FormatInt(offerXID, 10) || (c.Kind == "OFFER" && offerXID != int64(c.XID)) || body.Kind != c.Kind || body.Address != c.Address || body.Issued != strconv.FormatInt(commitMS, 10) || commitMS <= 0 || leaseIssued != commitMS || commitMS > c.IssuedMS || body.Expires != strconv.FormatInt(r.ExpiresMS, 10) || body.Revision != strconv.FormatInt(r.Revision, 10) {
		return ErrInvalid
	}
	return nil
}
