package dynamic_update

import (
	"database/sql"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/jasonwa/goddi/internal/dns/zone"
	"github.com/miekg/dns"
	_ "modernc.org/sqlite"
)

const (
	testZoneName = "example.com."
	testKeyName  = "ddns-key."
	testSecret   = "c2VjcmV0LXRlc3Qta2V5LW1hdGVyaWFsLTEyMzQ1Ng=="
)

// newUpdateDB mirrors production SQLite pool sizing (one connection, see
// internal/database/database.go). Every query inside an update must therefore
// go through the active transaction: a nested statement on the pool would block
// forever rather than error, which is only observable as a test that never
// returns.
func newUpdateDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	schema := `
	CREATE TABLE dns_zones (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL UNIQUE,
		type TEXT NOT NULL,
		enabled BOOLEAN DEFAULT TRUE,
		dnssec_enabled BOOLEAN DEFAULT FALSE,
		default_ttl INTEGER DEFAULT 3600,
		soa_mname TEXT NOT NULL,
		soa_rname TEXT NOT NULL,
		serial INTEGER NOT NULL,
		refresh INTEGER DEFAULT 3600,
		retry INTEGER DEFAULT 600,
		expire INTEGER DEFAULT 86400,
		minimum INTEGER DEFAULT 300,
		transfer_policy TEXT,
		update_policy TEXT,
		acl TEXT,
		catalog TEXT,
		created_at DATETIME NOT NULL DEFAULT (datetime('now')),
		updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
	);
	CREATE TABLE dns_records (
		id TEXT PRIMARY KEY,
		zone_id TEXT NOT NULL,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		value TEXT NOT NULL,
		ttl INTEGER DEFAULT 300,
		enabled BOOLEAN DEFAULT TRUE,
		-- The handler marks the rows it writes as this plane's, which is what
		-- keeps a configuration sync from withdrawing a name a client was
		-- authorised to add. A fixture without the column would make the write
		-- fail rather than exercise it.
		authored_locally INTEGER NOT NULL DEFAULT 0
	);
	CREATE TABLE dns_zone_changes (
		id TEXT PRIMARY KEY,
		zone_id TEXT NOT NULL,
		serial INTEGER NOT NULL,
		change_type TEXT NOT NULL,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		value TEXT NOT NULL,
		ttl INTEGER DEFAULT 300,
		priority INTEGER,
		weight INTEGER,
		port INTEGER,
		created_at DATETIME NOT NULL DEFAULT (datetime('now'))
	);
	CREATE TABLE dns_dynamic_update_policies (
		id TEXT PRIMARY KEY,
		zone_id TEXT NOT NULL,
		name TEXT NOT NULL,
		pattern TEXT,
		action TEXT NOT NULL,
		tsig_key_name TEXT,
		enabled BOOLEAN DEFAULT TRUE
	);
	-- This mirror has to carry every column the audit writer names, or the
	-- write fails and the failure reads as "auditing is broken" rather than
	-- "the fixture is a column behind". old_value / new_value are the pair
	-- migration 024 adds; TestHandleUpdateFrom_RecordsTheRecordsItReplaced
	-- fails if this table loses them.
	CREATE TABLE audit_logs (
		id TEXT PRIMARY KEY,
		user_id TEXT,
		username TEXT,
		action TEXT NOT NULL,
		resource_type TEXT NOT NULL,
		resource_id TEXT,
		detail TEXT,
		old_value TEXT,
		new_value TEXT,
		source_ip TEXT,
		user_agent TEXT,
		success BOOLEAN DEFAULT TRUE,
		created_at DATETIME NOT NULL DEFAULT (datetime('now'))
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	if _, err := db.Exec(`
		INSERT INTO dns_zones (id, name, type, enabled, soa_mname, soa_rname, serial)
		VALUES ('zone-1', ?, 'primary', 1, 'ns1.example.com.', 'hostmaster.example.com.', 2024010101)`,
		testZoneName); err != nil {
		t.Fatalf("seed zone: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO dns_dynamic_update_policies (id, zone_id, name, pattern, action, tsig_key_name, enabled)
		VALUES ('pol-1', 'zone-1', 'allow-all', '', 'allow', '', 1)`); err != nil {
		t.Fatalf("seed policy: %v", err)
	}

	return db
}

func newUpdateHandler(t *testing.T) (*UpdateHandler, *sql.DB) {
	t.Helper()
	db := newUpdateDB(t)
	// zoneStore/recordMgr stay nil: these tests exercise the update path, and
	// a nil store simply skips the post-commit Reload.
	h := NewUpdateHandler(db, nil, zone.NewZoneManager(db, nil), nil)
	h.SetTSIGSecrets(map[string]string{testKeyName: testSecret})
	return h, db
}

// signedUpdate builds a TSIG-authenticated update message and round-trips it
// through the wire format so the handler sees exactly what a client would send.
func signedUpdate(t *testing.T, build func(m *dns.Msg)) *dns.Msg {
	t.Helper()

	m := new(dns.Msg)
	m.SetUpdate(testZoneName)
	build(m)
	m.SetTsig(testKeyName, dns.HmacSHA256, 300, time.Now().Unix())

	wire, _, err := dns.TsigGenerate(m, testSecret, "", false)
	if err != nil {
		t.Fatalf("sign update: %v", err)
	}
	var parsed dns.Msg
	if err := parsed.Unpack(wire); err != nil {
		t.Fatalf("unpack signed update: %v", err)
	}
	return &parsed
}

func aRR(name, ip string, ttl uint32) *dns.A {
	return &dns.A{
		Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: ttl},
		A:   mustParseIP(ip),
	}
}

func mustParseIP(s string) net.IP {
	ip := net.ParseIP(s)
	if ip == nil {
		panic("invalid test IP: " + s)
	}
	return ip
}

// countRecords returns how many records match the given name/type filter.
func countRecords(t *testing.T, db *sql.DB, name, rtype string) int {
	t.Helper()
	query := "SELECT COUNT(*) FROM dns_records WHERE zone_id = 'zone-1'"
	args := []interface{}{}
	if name != "" {
		query += " AND name = ?"
		args = append(args, name)
	}
	if rtype != "" {
		query += " AND type = ?"
		args = append(args, rtype)
	}
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("count records: %v", err)
	}
	return n
}

func zoneSerial(t *testing.T, db *sql.DB) uint32 {
	t.Helper()
	var s uint32
	if err := db.QueryRow("SELECT serial FROM dns_zones WHERE id = 'zone-1'").Scan(&s); err != nil {
		t.Fatalf("read serial: %v", err)
	}
	return s
}

// ---------------------------------------------------------------------------
// planUpdateOps: CLASS-based classification (RFC 2136 §2.4.2)
// ---------------------------------------------------------------------------

func TestPlanUpdateOps_ClassClassification(t *testing.T) {
	a := aRR("host.example.com.", "192.0.2.10", 300)

	tests := []struct {
		name    string
		build   func(m *dns.Msg)
		wantKnd updateOpKind
		wantRc  int
	}{
		{
			name:    "CLASS IN adds",
			build:   func(m *dns.Msg) { m.Insert([]dns.RR{a}) },
			wantKnd: opAdd,
			wantRc:  dns.RcodeSuccess,
		},
		{
			name:    "CLASS ANY + type ANY deletes the whole name",
			build:   func(m *dns.Msg) { m.RemoveName([]dns.RR{a}) },
			wantKnd: opDeleteName,
			wantRc:  dns.RcodeSuccess,
		},
		{
			name:    "CLASS ANY + concrete type deletes the RRset",
			build:   func(m *dns.Msg) { m.RemoveRRset([]dns.RR{a}) },
			wantKnd: opDeleteRRset,
			wantRc:  dns.RcodeSuccess,
		},
		{
			name:    "CLASS NONE deletes one RR by rdata",
			build:   func(m *dns.Msg) { m.Remove([]dns.RR{a}) },
			wantKnd: opDeleteRR,
			wantRc:  dns.RcodeSuccess,
		},
		{
			name: "CLASS IN with an unsupported type is NOTIMP",
			build: func(m *dns.Msg) {
				m.Insert([]dns.RR{&dns.SRV{
					Hdr:      dns.RR_Header{Name: "_sip._tcp.example.com.", Rrtype: dns.TypeSRV, Class: dns.ClassINET, Ttl: 300},
					Priority: 10, Weight: 60, Port: 5060, Target: "sip.example.com.",
				}})
			},
			wantRc: dns.RcodeNotImplemented,
		},
		{
			name: "CLASS IN with a malformed value is FORMERR",
			build: func(m *dns.Msg) {
				m.Insert([]dns.RR{&dns.A{
					Hdr: dns.RR_Header{Name: "bad.example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
					A:   nil,
				}})
			},
			wantRc: dns.RcodeFormatError,
		},
		{
			name: "an unsupported CLASS is FORMERR",
			build: func(m *dns.Msg) {
				rr := aRR("chaos.example.com.", "192.0.2.11", 300)
				rr.Hdr.Class = dns.ClassCHAOS
				m.Ns = append(m.Ns, rr)
			},
			wantRc: dns.RcodeFormatError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := signedUpdate(t, tt.build)
			ops, rc := planUpdateOps(msg.Ns)
			if rc != tt.wantRc {
				t.Fatalf("rcode = %s, want %s", dns.RcodeToString[rc], dns.RcodeToString[tt.wantRc])
			}
			if tt.wantRc != dns.RcodeSuccess {
				if len(ops) != 0 {
					t.Fatalf("failed validation produced %d ops, want 0", len(ops))
				}
				return
			}
			if len(ops) != 1 {
				t.Fatalf("got %d ops, want 1", len(ops))
			}
			if ops[0].kind != tt.wantKnd {
				t.Errorf("kind = %v, want %v", ops[0].kind, tt.wantKnd)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Atomicity: a failure anywhere must leave the zone untouched
// ---------------------------------------------------------------------------

func TestHandleUpdateFrom_RollsBackOnPartialFailure(t *testing.T) {
	h, db := newUpdateHandler(t)
	before := zoneSerial(t, db)

	// The A record is legal on its own; the CNAME that follows is not, because
	// DNS forbids a CNAME next to another RRset. Per RFC 2136 §3.4.2 the whole
	// update must be discarded, so the A record must not survive.
	msg := signedUpdate(t, func(m *dns.Msg) {
		m.Insert([]dns.RR{aRR("dual.example.com.", "192.0.2.20", 300)})
		m.Insert([]dns.RR{&dns.CNAME{
			Hdr:    dns.RR_Header{Name: "dual.example.com.", Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: 300},
			Target: "target.example.com.",
		}})
	})

	resp, err := h.HandleUpdateFrom(msg, "192.0.2.1")
	if err != nil {
		t.Fatalf("HandleUpdateFrom: %v", err)
	}
	if resp.Rcode != dns.RcodeYXRrset {
		t.Fatalf("rcode = %s, want YXRRSET", dns.RcodeToString[resp.Rcode])
	}
	if n := countRecords(t, db, "dual.example.com.", ""); n != 0 {
		t.Errorf("partial update leaked %d records, want 0 (rollback failed)", n)
	}
	if got := zoneSerial(t, db); got != before {
		t.Errorf("serial changed to %d on a rolled-back update, want %d", got, before)
	}
	if n := countRecords(t, db, "", ""); n != 0 {
		t.Errorf("zone has %d records after a rejected update, want 0", n)
	}
}

func TestHandleUpdateFrom_MalformedUpdateChangesNothing(t *testing.T) {
	h, db := newUpdateHandler(t)
	before := zoneSerial(t, db)

	// Mix a valid add with an SRV the update path cannot encode losslessly.
	// Rejecting at validation time is the only way to avoid writing a record
	// with a zeroed priority, so nothing at all may be written.
	msg := signedUpdate(t, func(m *dns.Msg) {
		m.Insert([]dns.RR{aRR("mixed.example.com.", "192.0.2.21", 300)})
		m.Insert([]dns.RR{&dns.SRV{
			Hdr:      dns.RR_Header{Name: "_sip._tcp.example.com.", Rrtype: dns.TypeSRV, Class: dns.ClassINET, Ttl: 300},
			Priority: 10, Weight: 60, Port: 5060, Target: "sip.example.com.",
		}})
	})

	resp, err := h.HandleUpdateFrom(msg, "192.0.2.1")
	if err != nil {
		t.Fatalf("HandleUpdateFrom: %v", err)
	}
	if resp.Rcode != dns.RcodeNotImplemented {
		t.Fatalf("rcode = %s, want NOTIMP", dns.RcodeToString[resp.Rcode])
	}
	if n := countRecords(t, db, "", ""); n != 0 {
		t.Errorf("rejected update wrote %d records, want 0", n)
	}
	if got := zoneSerial(t, db); got != before {
		t.Errorf("serial changed to %d on a rejected update, want %d", got, before)
	}
}

// ---------------------------------------------------------------------------
// Success path: records, journal and serial move together, exactly once
// ---------------------------------------------------------------------------

func TestHandleUpdateFrom_AppliesAtomicallyAndJournals(t *testing.T) {
	h, db := newUpdateHandler(t)
	before := zoneSerial(t, db)

	msg := signedUpdate(t, func(m *dns.Msg) {
		m.Insert([]dns.RR{
			aRR("web.example.com.", "192.0.2.30", 300),
			aRR("web.example.com.", "192.0.2.31", 300),
		})
	})

	resp, err := h.HandleUpdateFrom(msg, "192.0.2.1")
	if err != nil {
		t.Fatalf("HandleUpdateFrom: %v", err)
	}
	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}
	if n := countRecords(t, db, "web.example.com.", "A"); n != 2 {
		t.Fatalf("got %d A records, want 2", n)
	}

	after := zoneSerial(t, db)
	if after <= before {
		t.Fatalf("serial did not advance: before=%d after=%d", before, after)
	}

	// The IXFR journal must describe exactly the records that were added, and
	// must carry the new serial so a secondary can request the delta.
	rows, err := db.Query(
		"SELECT serial, change_type, name, type, value FROM dns_zone_changes ORDER BY name")
	if err != nil {
		t.Fatalf("query journal: %v", err)
	}
	defer rows.Close()

	var journal []string
	for rows.Next() {
		var serial uint32
		var changeType, name, rtype, value string
		if err := rows.Scan(&serial, &changeType, &name, &rtype, &value); err != nil {
			t.Fatalf("scan journal: %v", err)
		}
		if serial != after {
			t.Errorf("journal row carries serial %d, want %d", serial, after)
		}
		journal = append(journal, fmt.Sprintf("%s %s %s %s", changeType, name, rtype, value))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate journal: %v", err)
	}
	if len(journal) != 2 {
		t.Fatalf("got %d journal rows, want 2: %v", len(journal), journal)
	}
	for _, want := range []string{"add web.example.com. A 192.0.2.30", "add web.example.com. A 192.0.2.31"} {
		found := false
		for _, got := range journal {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("journal missing %q (got %v)", want, journal)
		}
	}

	// Audit must actually be written; the previous statement referenced
	// non-existent columns and failed silently.
	var audits int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM audit_logs WHERE action = 'dns_dynamic_update'").Scan(&audits); err != nil {
		t.Fatalf("count audits: %v", err)
	}
	if audits != 1 {
		t.Errorf("got %d audit rows, want 1", audits)
	}
}

// TestHandleUpdateFrom_RecordsTheRecordsItReplaced pins the before/after pair an
// update leaves behind. Two separate things are being checked: that the writer
// only names columns migration 024 actually creates (the statement before it
// named columns that did not exist, so every dynamic update went un-audited and
// the failure was discarded), and that those columns carry what they claim --
// the records at the touched name either side of the change, not a copy of the
// update message.
func TestHandleUpdateFrom_RecordsTheRecordsItReplaced(t *testing.T) {
	h, db := newUpdateHandler(t)

	// First update: the name does not exist yet, so "before" describes nothing.
	adding := func(m *dns.Msg) {
		m.Insert([]dns.RR{aRR("web.example.com.", "192.0.2.30", 300)})
	}
	if resp, err := h.HandleUpdateFrom(signedUpdate(t, adding), "192.0.2.1"); err != nil || resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("add update: rcode=%v err=%v", resp, err)
	}

	// rowid, not created_at: several updates land in the same second, and an
	// assertion that picks the wrong row is indistinguishable from a broken
	// writer when it fails.
	var oldValue, newValue, username sql.NullString
	if err := db.QueryRow(`
		SELECT old_value, new_value, username FROM audit_logs
		WHERE action = 'dns_dynamic_update' ORDER BY rowid ASC LIMIT 1`).
		Scan(&oldValue, &newValue, &username); err != nil {
		t.Fatalf("read creating audit row: %v", err)
	}
	if oldValue.Valid && oldValue.String != "" {
		t.Errorf("old_value on a creating update = %q, want empty", oldValue.String)
	}
	for _, want := range []string{"web.example.com.", `"A"`, "192.0.2.30"} {
		if !strings.Contains(newValue.String, want) {
			t.Errorf("new_value %q does not mention %q", newValue.String, want)
		}
	}
	// The TSIG key name is the only identity an RFC 2136 client presents.
	// Recording "system" here discards the one fact the entry has.
	if username.String != testKeyName {
		t.Errorf("audit username = %q, want the TSIG key %q", username.String, testKeyName)
	}

	// Second update: withdraw the name (CLASS ANY + type ANY). Now "before"
	// describes the record and "after" describes nothing.
	removing := func(m *dns.Msg) {
		m.RemoveName([]dns.RR{aRR("web.example.com.", "0.0.0.0", 0)})
	}
	if resp, err := h.HandleUpdateFrom(signedUpdate(t, removing), "192.0.2.1"); err != nil || resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("delete update: rcode=%v err=%v", resp, err)
	}
	if n := countRecords(t, db, "web.example.com.", ""); n != 0 {
		t.Fatalf("the name was not withdrawn: %d records remain", n)
	}

	oldValue, newValue, username = sql.NullString{}, sql.NullString{}, sql.NullString{}
	if err := db.QueryRow(`
		SELECT old_value, new_value, username FROM audit_logs
		WHERE action = 'dns_dynamic_update' ORDER BY rowid DESC LIMIT 1`).
		Scan(&oldValue, &newValue, &username); err != nil {
		t.Fatalf("read withdrawing audit row: %v", err)
	}
	if !strings.Contains(oldValue.String, "192.0.2.30") {
		t.Errorf("old_value %q does not describe the record that was withdrawn", oldValue.String)
	}
	if newValue.Valid && newValue.String != "" {
		t.Errorf("new_value after withdrawing a name = %q, want empty", newValue.String)
	}
}

func TestHandleUpdateFrom_RejectsUpdateOutsideZone(t *testing.T) {
	h, db := newUpdateHandler(t)
	before := zoneSerial(t, db)

	msg := signedUpdate(t, func(m *dns.Msg) {
		m.Insert([]dns.RR{aRR("evil.example.com.attacker.net.", "192.0.2.40", 300)})
	})

	resp, err := h.HandleUpdateFrom(msg, "192.0.2.1")
	if err != nil {
		t.Fatalf("HandleUpdateFrom: %v", err)
	}
	if resp.Rcode != dns.RcodeNotZone {
		t.Fatalf("rcode = %s, want NOTZONE", dns.RcodeToString[resp.Rcode])
	}
	if n := countRecords(t, db, "", ""); n != 0 {
		t.Errorf("out-of-zone update wrote %d records, want 0", n)
	}
	if got := zoneSerial(t, db); got != before {
		t.Errorf("serial advanced on a rejected update")
	}
}

// ---------------------------------------------------------------------------
// Prerequisites: CLASS-based, and honest about failure
// ---------------------------------------------------------------------------

func TestPrerequisites_NameNotUsedIsEnforced(t *testing.T) {
	h, db := newUpdateHandler(t)

	// Pre-existing record at the name under test.
	if _, err := db.Exec(`
		INSERT INTO dns_records (id, zone_id, name, type, value, ttl, enabled)
		VALUES ('rec-1', 'zone-1', 'taken.example.com.', 'A', '192.0.2.50', 300, 1)`); err != nil {
		t.Fatalf("seed record: %v", err)
	}

	// "Name is not in use" must be rejected with YXDOMAIN. The previous
	// implementation dispatched on Rrtype and ignored this CLASS NONE + ANY
	// form entirely, so the update below used to succeed and clobber the name.
	msg := signedUpdate(t, func(m *dns.Msg) {
		m.NameNotUsed([]dns.RR{aRR("taken.example.com.", "0.0.0.0", 0)})
		m.Insert([]dns.RR{aRR("taken.example.com.", "192.0.2.51", 300)})
	})

	resp, err := h.HandleUpdateFrom(msg, "192.0.2.1")
	if err != nil {
		t.Fatalf("HandleUpdateFrom: %v", err)
	}
	if resp.Rcode != dns.RcodeYXDomain {
		t.Fatalf("rcode = %s, want YXDOMAIN", dns.RcodeToString[resp.Rcode])
	}
	if n := countRecords(t, db, "taken.example.com.", "A"); n != 1 {
		t.Errorf("got %d records at the name, want the original 1", n)
	}
	var value string
	if err := db.QueryRow(
		"SELECT value FROM dns_records WHERE name = 'taken.example.com.'").Scan(&value); err != nil {
		t.Fatalf("read record: %v", err)
	}
	if value != "192.0.2.50" {
		t.Errorf("record value = %s, want the untouched 192.0.2.50", value)
	}
}

func TestPrerequisites_NameUsedOnMissingNameIsNXDomain(t *testing.T) {
	h, _ := newUpdateHandler(t)

	msg := signedUpdate(t, func(m *dns.Msg) {
		m.NameUsed([]dns.RR{aRR("absent.example.com.", "0.0.0.0", 0)})
	})

	resp, err := h.HandleUpdateFrom(msg, "192.0.2.1")
	if err != nil {
		t.Fatalf("HandleUpdateFrom: %v", err)
	}
	if resp.Rcode != dns.RcodeNameError {
		t.Fatalf("rcode = %s, want NXDOMAIN", dns.RcodeToString[resp.Rcode])
	}
}

func TestPrerequisites_RRsetNotUsedOnExistingRRsetIsYXRrset(t *testing.T) {
	h, db := newUpdateHandler(t)

	if _, err := db.Exec(`
		INSERT INTO dns_records (id, zone_id, name, type, value, ttl, enabled)
		VALUES ('rec-2', 'zone-1', 'exists.example.com.', 'A', '192.0.2.60', 300, 1)`); err != nil {
		t.Fatalf("seed record: %v", err)
	}

	msg := signedUpdate(t, func(m *dns.Msg) {
		m.RRsetNotUsed([]dns.RR{aRR("exists.example.com.", "0.0.0.0", 0)})
	})

	resp, err := h.HandleUpdateFrom(msg, "192.0.2.1")
	if err != nil {
		t.Fatalf("HandleUpdateFrom: %v", err)
	}
	if resp.Rcode != dns.RcodeYXRrset {
		t.Fatalf("rcode = %s, want YXRRSET", dns.RcodeToString[resp.Rcode])
	}
}

func TestPrerequisiteOnlyUpdateDoesNotBumpSerial(t *testing.T) {
	h, db := newUpdateHandler(t)
	before := zoneSerial(t, db)

	// A pure "test and set" probe with no update section: legal, and it must
	// not wake up secondaries with a pointless NOTIFY.
	msg := signedUpdate(t, func(m *dns.Msg) {
		m.NameNotUsed([]dns.RR{aRR("probe.example.com.", "0.0.0.0", 0)})
	})

	resp, err := h.HandleUpdateFrom(msg, "192.0.2.1")
	if err != nil {
		t.Fatalf("HandleUpdateFrom: %v", err)
	}
	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}
	if got := zoneSerial(t, db); got != before {
		t.Errorf("serial advanced to %d for a prerequisite-only update, want %d", got, before)
	}
}

// ---------------------------------------------------------------------------
// Duplicate suppression and deletion semantics
// ---------------------------------------------------------------------------

func TestHandleUpdateFrom_DuplicateAddIsNoOp(t *testing.T) {
	h, db := newUpdateHandler(t)

	add := func(m *dns.Msg) { m.Insert([]dns.RR{aRR("dup.example.com.", "192.0.2.70", 300)}) }

	if resp, err := h.HandleUpdateFrom(signedUpdate(t, add), "192.0.2.1"); err != nil || resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("first update: rcode=%v err=%v", resp, err)
	}
	serialAfterFirst := zoneSerial(t, db)

	// Re-adding the identical RR is a set-membership no-op: it must not create
	// a second copy, and it must not bump the serial.
	resp, err := h.HandleUpdateFrom(signedUpdate(t, add), "192.0.2.1")
	if err != nil {
		t.Fatalf("second update: %v", err)
	}
	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("second update rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}
	if n := countRecords(t, db, "dup.example.com.", "A"); n != 1 {
		t.Errorf("got %d records after a duplicate add, want 1", n)
	}
	if got := zoneSerial(t, db); got != serialAfterFirst {
		t.Errorf("serial advanced on a no-op add: %d -> %d", serialAfterFirst, got)
	}
}

func TestHandleUpdateFrom_DeleteRRsetAndRemoveSpecificRR(t *testing.T) {
	h, db := newUpdateHandler(t)

	seed := func(m *dns.Msg) {
		m.Insert([]dns.RR{
			aRR("pool.example.com.", "192.0.2.80", 300),
			aRR("pool.example.com.", "192.0.2.81", 300),
		})
	}
	if resp, err := h.HandleUpdateFrom(signedUpdate(t, seed), "192.0.2.1"); err != nil || resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("seed update: rcode=%v err=%v", resp, err)
	}

	// Remove exactly one of the two A records.
	removeOne := func(m *dns.Msg) { m.Remove([]dns.RR{aRR("pool.example.com.", "192.0.2.80", 0)}) }
	resp, err := h.HandleUpdateFrom(signedUpdate(t, removeOne), "192.0.2.1")
	if err != nil {
		t.Fatalf("remove update: %v", err)
	}
	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("remove rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}
	if n := countRecords(t, db, "pool.example.com.", "A"); n != 1 {
		t.Fatalf("got %d records after removing one, want 1", n)
	}
	var remaining string
	if err := db.QueryRow("SELECT value FROM dns_records WHERE name = 'pool.example.com.'").Scan(&remaining); err != nil {
		t.Fatalf("read remaining: %v", err)
	}
	if remaining != "192.0.2.81" {
		t.Errorf("remaining record = %s, want 192.0.2.81", remaining)
	}

	// Now delete the whole RRset by type (CLASS ANY + concrete type).
	removeSet := func(m *dns.Msg) { m.RemoveRRset([]dns.RR{aRR("pool.example.com.", "0.0.0.0", 0)}) }
	resp, err = h.HandleUpdateFrom(signedUpdate(t, removeSet), "192.0.2.1")
	if err != nil {
		t.Fatalf("remove RRset: %v", err)
	}
	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("remove RRset rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}
	if n := countRecords(t, db, "pool.example.com.", ""); n != 0 {
		t.Errorf("got %d records after deleting the RRset, want 0", n)
	}
}

// ---------------------------------------------------------------------------
// Structural guard: never block on the single SQLite connection
// ---------------------------------------------------------------------------

// TestHandleUpdateFrom_DoesNotBlockOnSingleConnection turns the classic failure
// mode into a bounded failure. A nested statement on h.db while the update
// transaction holds the pool's only connection does not return an error — it
// waits forever — so without this watchdog a regression would hang the whole
// test binary instead of naming the offending package.
func TestHandleUpdateFrom_DoesNotBlockOnSingleConnection(t *testing.T) {
	h, _ := newUpdateHandler(t)

	msg := signedUpdate(t, func(m *dns.Msg) {
		m.Insert([]dns.RR{
			aRR("watchdog.example.com.", "192.0.2.90", 300),
			aRR("watchdog2.example.com.", "192.0.2.91", 300),
		})
	})

	done := make(chan *dns.Msg, 1)
	go func() {
		resp, err := h.HandleUpdateFrom(msg, "192.0.2.1")
		if err != nil || resp == nil {
			done <- nil
			return
		}
		done <- resp
	}()

	select {
	case resp := <-done:
		if resp == nil {
			t.Fatal("update failed")
		}
		if resp.Rcode != dns.RcodeSuccess {
			t.Fatalf("rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
		}
	case <-time.After(15 * time.Second):
		t.Fatal("update did not return within 15s: a statement is likely running " +
			"on the pool instead of the transaction (single-connection deadlock)")
	}
}
