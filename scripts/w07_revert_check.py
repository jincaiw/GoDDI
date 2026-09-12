#!/usr/bin/env python3
"""Back-out verification for the W07 data-plane split.

Each case disables exactly one mechanism, runs the test that is supposed to
depend on it, and requires that test to FAIL. A case that still passes means the
test does not actually pin the behaviour down -- the mechanism could be deleted
tomorrow and the suite would stay green.

Every replacement is asserted to have happened, and to have happened exactly
once, before the test runs. An earlier round of this script (W05) silently
skipped its replacements and produced five fabricated "verified" results; the
assertion below is what prevents a repeat. A case that reports
SKIPPED-NOT-FOUND or SKIPPED-NOT-UNIQUE is a failure, not a pass.

Mechanisms this script deliberately does NOT claim to pin, because no test
distinguishes them and inventing a case would just be a green light that means
nothing:

  * the order of "clear the dirty markers" strictly after "commit the push" --
    no test induces a commit failure after a successful upsert, so the exact
    interleaving is unobservable. R15 pins the coarser version of the rule: the
    markers must survive a push that never reached the control database at all;
  * the choice of WAL over the rollback journal -- R2 pins that the setting is
    applied and verified, not that WAL is the better choice;
  * "the request path never writes to the control database". R18 pins that the
    console refuses to release a lease it only holds a copy of, which is the
    property that matters, but a handler could still write a control-side row
    nobody reads. Closing the control database would show it; the tests here do
    not, and a case that merely claims to would be a green light over an unread
    row.
  * "crossing a quota never refuses a client, a write or a push". There is no
    gate to remove: the quota is read by the probe and by nothing else, so a
    case would have to invent the refusal first and then revert it, which
    verifies the invention rather than the product. What is pinned instead is
    the half that could regress silently -- R36 requires a degraded level to
    still answer 200, so a crossed bound can only ever be reported.

The readiness log line used to be listed here as unassertable -- "a
counter-based test would pin the test's own counter rather than the operator's
log". That was true of a counter and false of the record: probe_log_test.go now
captures slog.Record through a real handler, so both halves of the rule are
pinned. R45 requires a repeated level to produce no line at all and R46 requires
the attribute that carries the level not to be named `level`, because a JSON
handler already writes that key for the severity.

What the batch's earlier numbering used to cover, and why the cases are gone:

  * (ex R16) "the reply is served from the lease store rather than the DNS
    store". It is no longer a mechanism that can be reverted: server.New takes
    a lease database and an interface list, and the DNS store is not passed to
    the DHCP server at all, so there is no field left to point at the wrong
    database. That is a stronger guarantee than a test -- the compiler holds it
    -- and the part that is still falsifiable is the process assembly, which
    scripts/w07_role_smoke_check.py asserts by starting the binary.
  * (ex R17) "the DDNS applier reads the binding from the lease store". A Stores
    value is built with Same() in every construction that exists -- main.go
    passes Same(dnsStore.DB) and the tests pass Same(db) -- so swapping the two
    fields cannot change any observable behaviour, and a case that swapped them
    would report a verified revert over a no-op. The rule that replaced it is
    covered by R29: the applier publishes from the event's own snapshot when the
    lease replica has not arrived yet.
  * "the DDNS consumer is woken when an event is queued". It is not, and cannot
    be: the consumer is another process, on another file system, and the only
    way it learns of the entry is that this store's push reaches the control
    database and its own poll reads it. R32 pins the wake-up that does exist --
    the one to this process's own outbox push -- and the consumer's 1s drain is
    a constant no test can make fail by removing it.

R19-R22 cover the record write guard: the console refuses to edit, delete or
batch-delete a record the data plane authored, and still writes the records the
operator authored in the same zone. R23-R31 cover the rest of W07-b2: the
authorship boundary the downward sync draws, the revision counter the data
plane's own writes must not move, the zone serial in both directions, the event
push that must not overwrite a consumer's state, the orphan and missing-record
criteria that have to survive a lagging lease replica, and the supersede rule
itself. R32-R35 cover what was added to keep DDNS inside the exit condition
after the split: the near side's wake-up, the buffer that keeps a signal
arriving before the loop starts, and the poll interval's default. R36-R44 cover
the tiered probe and the resource quotas: liveness that never fails on
degradation, a readiness answer that only a genuine failure makes a 503,
neither an unwatched process nor an unclassifiable level read as healthy, a
probe answered from memory rather than from the store's single connection, what
each condition means (a crossed quota and an unreadable replica), and the
footprint measurement that has to include the write-ahead log. R45-R46 cover the
readiness log line: that it is written when the level changes and not on every
pass, and that the attribute carrying the level does not collide with the
severity key the JSON handler writes.
"""

import pathlib
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent

DP = ROOT / "internal/dataplane"
DP_MIG = DP / "migrations/001_dataplane.sql"
SYNC = DP / "sync.go"
TRANSFER = DP / "transfer.go"
RUNNER = DP / "runner.go"
CTRL_MIG = ROOT / "migrations/021_dataplane_revision.sql"
AUTHOR_MIG = ROOT / "migrations/022_record_authorship.sql"

DHCP_SERVER = ROOT / "internal/dhcp/server/server.go"
DNS_LINK = ROOT / "internal/dhcp/dns_link.go"
LEASE_HANDLER = ROOT / "internal/api/handler/dhcp_lease.go"
ZONE_RECORD = ROOT / "internal/dns/zone/record.go"
ZONE_OWNERSHIP = ROOT / "internal/dns/zone/ownership.go"
ROLE_CONFIG = ROOT / "internal/config/role.go"
PROBE = DP / "probe.go"
QUOTA = DP / "quota.go"
HEALTH = ROOT / "internal/api/handler/health.go"

PKG_DP = "./internal/dataplane/"
PKG_DHCP = "./internal/dhcp/"
PKG_DHCP_SERVER = "./internal/dhcp/server/"
PKG_HANDLER = "./internal/api/handler/"
PKG_CONFIG = "./internal/config/"

CASES = [
    # -- durability: the setting that makes an ACK honest -------------------
    # R1-R3 are pinned by the write-then-read-back guard: the store refuses to
    # open when the durability settings it needs are not the ones in force, so
    # the test fails at Open rather than at a later assertion.
    {
        "label": "R1 the lease store opens with synchronous=NORMAL",
        "file": DP / "store.go",
        "old": '\t{"synchronous", "2"},',
        "new": '\t{"synchronous", "1"},',
        "test": "TestOpenAppliesTheDurabilitySettingsThatMakeAnAckHonest",
    },
    {
        "label": "R2 the lease store opens on the rollback journal",
        "file": DP / "store.go",
        "old": '\t{"journal_mode", "wal"},',
        "new": '\t{"journal_mode", "delete"},',
        "test": "TestOpenAppliesTheDurabilitySettingsThatMakeAnAckHonest",
    },
    {
        "label": "R3 the lease store opens with foreign keys off",
        "file": DP / "store.go",
        "old": '\t{"foreign_keys", "1"},',
        "new": '\t{"foreign_keys", "0"},',
        "test": "TestOpenAppliesTheDurabilitySettingsThatMakeAnAckHonest",
    },
    # -- the replica schema -------------------------------------------------
    {
        "label": "R4 a replica table drifts away from the control schema",
        "file": DP_MIG,
        "old": "    domain_name        TEXT,\n    lease_time         INTEGER DEFAULT 86400,",
        "new": "    domain_name_x      TEXT,\n    lease_time         INTEGER DEFAULT 86400,",
        "test": "TestReplicaTablesMatchTheControlSchema",
    },
    {
        "label": "R5 a replica table gains a foreign key",
        "file": DP_MIG,
        "old": (
            "CREATE TABLE IF NOT EXISTS dhcp_reservations (\n"
            "    id          TEXT PRIMARY KEY,\n"
            "    scope_id    TEXT NOT NULL,\n"
        ),
        "new": (
            "CREATE TABLE IF NOT EXISTS dhcp_reservations (\n"
            "    id          TEXT PRIMARY KEY,\n"
            "    scope_id    TEXT NOT NULL REFERENCES dhcp_scopes(id),\n"
        ),
        "test": "TestReplicaTablesCarryNoForeignKeys",
    },
    {
        "label": "R6 a replica foreign key cascades leases away with their scope",
        "file": DP_MIG,
        "old": (
            "CREATE TABLE IF NOT EXISTS dhcp_leases (\n"
            "    id          TEXT PRIMARY KEY,\n"
            "    scope_id    TEXT NOT NULL,\n"
        ),
        "new": (
            "CREATE TABLE IF NOT EXISTS dhcp_leases (\n"
            "    id          TEXT PRIMARY KEY,\n"
            "    scope_id    TEXT NOT NULL REFERENCES dhcp_scopes(id) ON DELETE CASCADE,\n"
        ),
        "test": "TestScopeReplacementKeepsLeases",
    },
    # -- upward replication: what is owed ------------------------------------
    {
        "label": "R7 lease writes stop being marked for replication",
        "file": DP_MIG,
        "old": (
            "CREATE TRIGGER IF NOT EXISTS trg_dp_lease_dirty_insert\n"
            "AFTER INSERT ON dhcp_leases\n"
            "BEGIN\n"
        ),
        "new": (
            "CREATE TRIGGER IF NOT EXISTS trg_dp_lease_dirty_insert\n"
            "AFTER INSERT ON dhcp_leases\n"
            "WHEN 0\n"
            "BEGIN\n"
        ),
        "test": "TestLeaseWritesAreMarkedForReplication",
    },
    {
        "label": "R8 the control revision stops moving when a scope is written",
        "file": CTRL_MIG,
        "old": (
            "CREATE TRIGGER IF NOT EXISTS trg_dp_rev_dhcp_scopes_ins\n"
            "AFTER INSERT ON dhcp_scopes\n"
            "BEGIN\n"
        ),
        "new": (
            "CREATE TRIGGER IF NOT EXISTS trg_dp_rev_dhcp_scopes_ins\n"
            "AFTER INSERT ON dhcp_scopes\n"
            "WHEN 0\n"
            "BEGIN\n"
        ),
        "test": "TestControlRevisionBumpsOnEveryWritePath",
    },
    {
        "label": "R9 lease writes bump the control revision they must not touch",
        "file": CTRL_MIG,
        "old": "INSERT OR IGNORE INTO dataplane_revision (domain, revision) VALUES ('dns', 0);",
        "new": (
            "INSERT OR IGNORE INTO dataplane_revision (domain, revision) VALUES ('dns', 0);\n"
            "\n"
            "-- +goose StatementBegin\n"
            "CREATE TRIGGER IF NOT EXISTS trg_dp_rev_dhcp_leases_ins\n"
            "AFTER INSERT ON dhcp_leases\n"
            "BEGIN\n"
            "    UPDATE dataplane_revision SET revision = revision + 1,"
            " updated_at = datetime('now') WHERE domain = 'dhcp';\n"
            "END;\n"
            "-- +goose StatementEnd\n"
        ),
        "test": "TestControlRevisionDoesNotFollowLeaseWrites",
    },
    # -- downward replication: the rules that protect clients ----------------
    {
        "label": "R10 a scope that still holds leases is deleted anyway",
        "file": DP / "sync.go",
        "old": (
            "\tprotected := map[string]bool{}\n"
            "\tfor _, id := range held {\n"
            "\t\tif !keep[id] {\n"
            "\t\t\tprotected[id] = true\n"
            "\t\t}\n"
            "\t}\n"
            "\treturn protected, nil"
        ),
        "new": (
            "\t_ = held\n"
            "\t_ = keep\n"
            "\treturn map[string]bool{}, nil"
        ),
        "test": "TestSyncRetainsAScopeThatStillHoldsLeases",
    },
    {
        "label": "R11 leases of a removed scope are left behind",
        "file": DP / "sync.go",
        "old": "\tif d == DomainDHCP {\n\t\tdropped, err = dropOrphanLeases(tx)",
        "new": "\tif false && d == DomainDHCP {\n\t\tdropped, err = dropOrphanLeases(tx)",
        "test": "TestSyncDropsLeasesWhoseScopeIsReallyGone",
    },
    # -- the upgrade path ----------------------------------------------------
    {
        "label": "R12 the lease takeover runs again on every start",
        "file": DP / "transfer.go",
        "old": "\tif done {\n\t\treturn 0, nil\n\t}",
        "new": "\tif false && done {\n\t\treturn 0, nil\n\t}",
        "test": "TestTakeOverLeasesMovesTheRowsOnceAndOnlyOnce",
    },
    {
        "label": "R13 the takeover overwrites a store that already owns its leases",
        "file": DP / "transfer.go",
        "old": "\tif local {\n\t\tslog.Info(",
        "new": "\tif false && local {\n\t\tslog.Info(",
        "test": "TestTakeOverLeasesLeavesAnOwnedStoreAlone",
    },
    {
        "label": "R14 a DHCP process starts with nothing to serve",
        "file": DP / "runner.go",
        "old": "\t\t\tif !has {\n",
        "new": "\t\t\tif !has && false {\n",
        "test": "TestPrimeRefusesToStartWithNothingToServe",
    },
    {
        "label": "R15 lease markers are cleared before the control database accepts them",
        "file": DP / "transfer.go",
        "old": "\ttx, err := r.control.Begin()\n\tif err != nil {\n\t\treturn res, fmt.Errorf(\"%w: beginning the lease push: %v\", ErrControlUnavailable, err)",
        "new": (
            "\tpremature := make([]string, 0, len(pending))\n"
            "\tfor _, p := range pending {\n"
            "\t\tpremature = append(premature, p.id)\n"
            "\t}\n"
            "\tif err := r.clearDirty(premature); err != nil {\n"
            "\t\treturn res, err\n"
            "\t}\n"
            "\ttx, err := r.control.Begin()\n"
            "\tif err != nil {\n"
            "\t\treturn res, fmt.Errorf(\"%w: beginning the lease push: %v\", ErrControlUnavailable, err)"
        ),
        "test": "TestPushKeepsItsMarkersWhenTheReplicaCannotBeWritten",
    },
    # -- the console's write guards (W07-b1 leases, W07-b2 records) -----------
    # R18 drops the console's write guard on the lease replica, so a release
    # against the copy is accepted, reports success, and is then overwritten by
    # the next push.
    {
        "label": "R18 the console releases a lease it only holds a copy of",
        "file": LEASE_HANDLER,
        "old": "\tif DHCPServices.LeasesAreReplica {",
        "new": "\tif false && DHCPServices.LeasesAreReplica {",
        "test": "TestTheConsoleRefusesToReleaseALeaseItOnlyHasACopyOf",
        "pkg": PKG_HANDLER,
    },
    # R19-R21 disable one call of the record guard at a time. Each write path
    # has its own call, so a case per path is the only way to show that none of
    # them was left out: a guard added to UpdateRecord and forgotten in
    # BatchDeleteRecords would still look green under a single case.
    {
        "label": "R19 the console edits a record the data plane owns",
        "file": ZONE_RECORD,
        "old": (
            "\tif err := m.assertEditable(id); err != nil {\n"
            "\t\treturn nil, err\n"
            "\t}\n"
            "\n"
            "\t// Verify record exists."
        ),
        "new": (
            "\tif err := error(nil); err != nil {\n"
            "\t\treturn nil, err\n"
            "\t}\n"
            "\n"
            "\t// Verify record exists."
        ),
        "test": "TestTheConsoleRefusesToEditARecordTheDataPlaneOwns",
        "pkg": PKG_HANDLER,
    },
    {
        "label": "R20 the console deletes a record the data plane owns",
        "file": ZONE_RECORD,
        "old": (
            "\tif err := m.assertEditable(id); err != nil {\n"
            "\t\treturn err\n"
            "\t}\n"
            "\n"
            "\t// Get record to know which zone to update serial for."
        ),
        "new": (
            "\tif err := error(nil); err != nil {\n"
            "\t\treturn err\n"
            "\t}\n"
            "\n"
            "\t// Get record to know which zone to update serial for."
        ),
        "test": "TestTheConsoleRefusesToDeleteARecordTheDataPlaneOwns",
        "pkg": PKG_HANDLER,
    },
    {
        "label": "R21 the console batch-deletes a record the data plane owns",
        "file": ZONE_RECORD,
        "old": (
            "\tfor _, id := range ids {\n"
            "\t\tif err := m.assertEditable(id); err != nil {\n"
        ),
        "new": (
            "\tfor _, id := range ids {\n"
            "\t\tif err := error(nil); err != nil {\n"
        ),
        "test": "TestTheConsoleRefusesABatchThatMixesInAPublishedRecord",
        "pkg": PKG_HANDLER,
    },
    # R22 makes the guard refuse every record instead of the ones the data
    # plane authored. This is the half that costs the operator something: a
    # zone-wide refusal would take away the records they do own. Reading the
    # column is what keeps the guard per row.
    {
        "label": "R22 the guard refuses the operator's own records too",
        "file": ZONE_OWNERSHIP,
        "old": "\tif authoredLocally != 0 {",
        "new": "\tif true {",
        "test": "TestTheConsoleEditsARecordOfItsOwn",
        "pkg": PKG_HANDLER,
    },
    # -- the authorship boundary (W07-b2) ------------------------------------
    # R23 replaces records wholesale. The control plane's set then becomes the
    # whole truth, and every record this plane published for a DHCP binding is
    # deleted by the next configuration change anywhere in the system.
    {
        "label": "R23 the downward sync replaces the records this plane authored",
        "file": SYNC,
        "old": '\t{name: "dns_records", key: "id", mode: replaceControlAuthored, columns: []string{',
        "new": '\t{name: "dns_records", key: "id", mode: replaceWhole, columns: []string{',
        "test": "TestADownwardSyncLeavesTheRecordsThisPlaneAuthored",
    },
    # R24 moves the revision counter on the data plane's own record writes. The
    # plane then polls in response to its own writes -- a loop, and the reason
    # the counter is filtered to the rows the control plane authored.
    {
        "label": "R24 the revision counter follows the data plane's own writes",
        "file": AUTHOR_MIG,
        "old": (
            "CREATE TRIGGER trg_dp_rev_dns_records_ins\n"
            "AFTER INSERT ON dns_records\n"
            "WHEN NEW.authored_locally = 0\n"
        ),
        "new": (
            "CREATE TRIGGER trg_dp_rev_dns_records_ins\n"
            "AFTER INSERT ON dns_records\n"
        ),
        "test": "TestOnlyTheControlPlaneWritesMoveTheDnsRevision",
    },
    # R25 drops the serial restore after a downward sync. The zone row is
    # replaced wholesale, so the serial this plane already advertised comes back
    # as the control plane's older value -- and every secondary concludes the
    # zone has not changed.
    {
        "label": "R25 a downward sync overwrites a locally bumped serial",
        "file": SYNC,
        "old": (
            "\tif d == DomainDNS {\n"
            "\t\tif err := reapplyLocalSerials(tx); err != nil {\n"
            "\t\t\treturn false, nil, 0, err\n"
            "\t\t}\n"
            "\t}"
        ),
        "new": (
            "\tif false && d == DomainDNS {\n"
            "\t\tif err := reapplyLocalSerials(tx); err != nil {\n"
            "\t\t\treturn false, nil, 0, err\n"
            "\t\t}\n"
            "\t}"
        ),
        "test": "TestALocallyBumpedSerialSurvivesADownwardSync",
    },
    # R26 makes the serial push unconditional. The bound is replaced by a
    # placeholder that is always true rather than deleted, so the statement
    # keeps its three arguments and the case isolates the comparison instead of
    # breaking the call. A control plane that was down while this store bumped
    # the serial several times comes back holding an older value, and the zone's
    # serial goes backwards past a number it has already published.
    {
        "label": "R26 the serial push can lower the control plane's serial",
        "file": TRANSFER,
        "old": "WHERE id = ? AND serial < ?",
        "new": "WHERE id = ? AND ? IS NOT NULL",
        "test": "TestTheSerialReachesTheControlPlaneAndOnlyEverRises",
    },
    # R27 carries the consumer's own state back up with the payload. The
    # producer cannot observe the consumer, so an event the DNS side already
    # processed is reported to the console as pending again.
    {
        "label": "R27 the event push overwrites another consumer's state",
        "file": TRANSFER,
        "old": "\t\tON CONFLICT(id) DO NOTHING`,",
        "new": "\t\tON CONFLICT(id) DO UPDATE SET status = 'pending'`,",
        "test": "TestTheOutboxPushCannotResetAnotherConsumersState",
    },
    # R28 restores the criterion the split had to abandon: a record is an
    # orphan when its lease row is absent. The lease table here is a replica,
    # and absence means "the update has not arrived yet", not "the binding is
    # gone" -- so a lagging replica withdraws names that are still in use.
    {
        "label": "R28 an orphan is a record whose lease replica has not arrived",
        "file": DNS_LINK,
        "old": (
            "\t\tJOIN dhcp_leases l ON l.id = r.owner_ref\n"
            "\t\tWHERE r.owner = 'dhcp' AND r.owner_ref IS NOT NULL\n"
            "\t\t  AND l.status NOT IN (?, ?)"
        ),
        "new": (
            "\t\tLEFT JOIN dhcp_leases l ON 1 = 0\n"
            "\t\tWHERE r.owner = 'dhcp' AND r.owner_ref IS NOT NULL\n"
            "\t\t  AND (l.status IS NULL OR l.status NOT IN (?, ?))"
        ),
        "test": "TestReconcile_LeavesARecordWhoseLeaseIsNotInTheReplica",
        "pkg": PKG_DHCP,
    },
    # R29 stops counting a record with no expiry as unfinished work. Such a
    # record is the one written from a lagging lease replica, where the lease's
    # end was not visible; nothing else will ever fill the expiry in, so the
    # name outlives the binding that justified it.
    {
        "label": "R29 a record published without an expiry is never completed",
        "file": DNS_LINK,
        "old": "\t\t          AND r.expires_at IS NOT NULL\n",
        "new": "",
        "test": "TestTheOutboxCrossesToTheDNSPlaneAndPublishesWithoutTheLeaseReplica",
        "pkg": PKG_DHCP_SERVER,
    },
    # R30 and R31 disable the two halves of the supersede rule. Without them a
    # replayed create resurrects a name that a later renewal, or a teardown,
    # has already taken away.
    {
        "label": "R30 a create from a superseded generation is published",
        "file": DNS_LINK,
        "old": "\tif replicaGeneration > eventGeneration {\n\t\treturn true\n\t}",
        "new": "\tif false && replicaGeneration > eventGeneration {\n\t\treturn true\n\t}",
        "test": "TestApplyCreate_DropsSupersededGeneration",
        "pkg": PKG_DHCP,
    },
    {
        "label": "R31 a create for a binding that is no longer held is published",
        "file": DNS_LINK,
        "old": "\treturn replicaGeneration == eventGeneration && replicaStatus != string(lease.LeaseStatusActive)",
        "new": "\t_ = replicaStatus\n\treturn false",
        "test": "TestApplyCreate_SkipsUnconfirmedBinding",
        "pkg": PKG_DHCP,
    },
    # -- the DDNS delay (W07-b2 follow-up) -----------------------------------
    # The interval appears twice in the path between a DHCP client being
    # answered and its own name resolving. R32-R35 pin the two ways it is not
    # paid in full: the near side wakes its own loop instead of waiting, and the
    # signal survives arriving before that loop starts.
    {
        "label": "R32 a queued DNS change does not wake the outbox push",
        "file": DHCP_SERVER,
        "old": "\tif err == nil {\n\t\ts.wakeOutbox()\n\t}",
        "new": "\tif false && err == nil {\n\t\ts.wakeOutbox()\n\t}",
        "test": "TestQueueingADNSChangeWakesTheOutboxPush",
        "pkg": PKG_DHCP_SERVER,
    },
    {
        "label": "R33 a wake does not run a pass",
        "file": RUNNER,
        "old": "\tselect {\n\tcase r.wake <- struct{}{}:\n\tdefault:\n\t}",
        "new": "\t_ = r.wake",
        "test": "TestAWokenLoopPushesWithoutWaitingForTheInterval",
        "pkg": PKG_DP,
    },
    {
        "label": "R34 a signal sent before the loop starts is dropped",
        "file": RUNNER,
        "old": "\treturn &Runner{rep: rep, cfg: cfg, wake: make(chan struct{}, 1)}",
        "new": "\treturn &Runner{rep: rep, cfg: cfg, wake: make(chan struct{})}",
        "test": "TestASignalSentBeforeTheLoopStartsIsNotLost",
        "pkg": PKG_DP,
    },
    {
        "label": "R35 the poll interval goes back to five seconds",
        "file": ROLE_CONFIG,
        "old": "const DefaultSyncIntervalSeconds = 1",
        "new": "const DefaultSyncIntervalSeconds = 5",
        "test": "TestTheSyncIntervalDefaultMeetsTheDdnsBound",
        "pkg": PKG_CONFIG,
    },
    # -- readiness: liveness is not readiness ---------------------------------
    # The process used to answer 503 on /health when a subsystem was degraded.
    # An orchestrator restarts an unhealthy container, so that asked for a
    # restart of the one process still serving its local copy -- the outage the
    # split removes. R36-R39 pin the replacement: liveness never fails on
    # degradation, only a genuine failure is a 503, and neither an unwatched
    # process nor an unclassifiable level can be read as healthy.
    {
        "label": "R36 readiness reports degraded as not ready",
        "file": HEALTH,
        "old": "\tif level == LevelFailing {",
        "new": "\tif level != LevelOK {",
        "test": "TestReadinessReportsTheWorstPlane",
        "pkg": PKG_HANDLER,
    },
    {
        "label": "R37 liveness fails when a subsystem is degraded",
        "file": HEALTH,
        "old": '\twriteStatus(w, http.StatusOK, map[string]any{"status": LevelOK})',
        "new": (
            "\tlevel, _ := readiness()\n"
            "\tif level != LevelOK {\n"
            "\t\twriteStatus(w, http.StatusServiceUnavailable, map[string]any{\"status\": level})\n"
            "\t\treturn\n"
            "\t}\n"
            '\twriteStatus(w, http.StatusOK, map[string]any{"status": LevelOK})'
        ),
        "test": "TestLivenessIgnoresDegradation",
        "pkg": PKG_HANDLER,
    },
    {
        "label": "R38 a process with nothing to watch reports healthy",
        "file": HEALTH,
        "old": "\t\treturn LevelFailing, []PlaneStatus{{",
        "new": "\t\treturn LevelOK, []PlaneStatus{{",
        "test": "TestAProcessWithNoProbesIsNotReady",
        "pkg": PKG_HANDLER,
    },
    {
        "label": "R39 an unclassifiable level passes through as itself",
        "file": HEALTH,
        "old": "\t\treturn LevelFailing\n\t}\n}",
        "new": "\t\treturn level\n\t}\n}",
        "test": "TestAnUnclassifiableLevelIsTreatedAsFailing",
        "pkg": PKG_HANDLER,
    },
    # -- readiness: what the probe is allowed to cost -------------------------
    # /ready is unauthenticated by necessity, so it must not take the store's
    # single connection. R40 pins that it answers from the last pass's view.
    {
        "label": "R40 the probe reads the store instead of the last pass",
        "file": PROBE,
        "old": "\treturn r.probe\n}",
        "new": "\treturn r.evaluate(r.Snapshot())\n}",
        "test": "TestProbingReadsMemoryOnly",
        "pkg": PKG_DP,
    },
    {
        "label": "R41 a probe that costs nothing is re-run per request",
        "file": HEALTH,
        "old": "\tif !c.at.IsZero() && time.Since(c.at) < c.interval {",
        "new": "\tif !c.at.IsZero() {",
        "test": "TestCachedProbeReEvaluatesOnceTheIntervalPasses",
        "pkg": PKG_HANDLER,
    },
    # -- readiness: what each condition means ---------------------------------
    {
        "label": "R42 a crossed quota does not raise the level",
        "file": PROBE,
        "old": "\tif breaches := r.cfg.Quota.Check(r.rep.store, st); len(breaches) > 0 {",
        "new": "\tif breaches := []Breach(nil); len(breaches) > 0 {",
        "test": "TestACrossedQuotaIsDegradedAndNamesTheBound",
        "pkg": PKG_DP,
    },
    {
        "label": "R43 a replica that cannot be read is only a caveat",
        "file": PROBE,
        "old": "\t\t\trd.Level = LevelFailing",
        "new": "\t\t\trd.Level = LevelDegraded",
        "test": "TestAPlaneThatCannotReadItsReplicaStateIsFailing",
        "pkg": PKG_DP,
    },
    {
        "label": "R44 the store footprint stops at the database file",
        "file": QUOTA,
        "old": '\tfor _, p := range []string{path, path + "-wal"} {',
        "new": '\tfor _, p := range []string{path} {',
        "test": "TestTheStoreFootprintCountsTheWriteAheadLog",
        "pkg": PKG_DP,
    },
    # -- readiness: the line the operator actually reads ----------------------
    # The line exists so that a change is visible and a steady state is not
    # repeated every pass. Both halves matter: a line per pass is a line nobody
    # reads, and no line at all is a change nobody sees.
    {
        "label": "R45 every pass logs the level, changed or not",
        "file": PROBE,
        "old": "\tchanged := !r.probed || r.probe.Level != rd.Level",
        "new": "\tchanged := true",
        "test": "TestALevelChangeIsLoggedAndARepeatedOneIsNot",
        "pkg": PKG_DP,
    },
    # The attribute cannot be called `level`: slog's JSON handler already emits
    # a `level` key for the severity, and a parser that keeps the last duplicate
    # turns the warning's own severity into the value of the readiness field --
    # so the record stops being filterable as a warning.
    {
        "label": "R46 the readiness attribute shadows the severity key",
        "file": PROBE,
        "old": '\tattrs := []any{"readiness", string(rd.Level),',
        "new": '\tattrs := []any{"level", string(rd.Level),',
        "test": "TestTheReadinessLineDoesNotShadowTheSeverity",
        "pkg": PKG_DP,
    },
]


def run_case(case: dict) -> str:
    path = case["file"]
    pkg = case.get("pkg", PKG_DP)
    backup = path.read_text()

    # Unique, not merely present. Replacing an ambiguous pattern edits whichever
    # occurrence comes first, which may be an unrelated one, and the case would
    # then report on a mutation that was never made.
    occurrences = backup.count(case["old"])
    if occurrences == 0:
        return f"SKIPPED-NOT-FOUND: {case['label']} (the pattern is not in {path.name})"
    if occurrences > 1:
        return (f"SKIPPED-NOT-UNIQUE: {case['label']} "
                f"(the pattern appears {occurrences} times in {path.name})")

    path.write_text(backup.replace(case["old"], case["new"], 1))
    try:
        proc = subprocess.run(
            ["go", "test", "-count=1", "-timeout", "180s", "-run", case["test"], pkg],
            cwd=ROOT, capture_output=True, text=True,
        )
    finally:
        path.write_text(backup)

    output = proc.stdout + proc.stderr
    if "build failed" in output or "cannot use" in output or "undefined:" in output:
        return f"INVALID: {case['label']} broke the build instead of the behaviour"
    if "--- FAIL:" in output or "panic: test timed out" in output:
        return f"OK: {case['label']} -> {case['test']} failed as expected"
    return f"NOT-VERIFIED: {case['label']} -> {case['test']} still passed without the mechanism"


def main() -> int:
    failures = []
    for case in CASES:
        result = run_case(case)
        print(result, flush=True)
        if not result.startswith("OK:"):
            failures.append(result)

    # Confirm every package the cases touched is green again after restoring.
    packages = []
    for case in CASES:
        pkg = case.get("pkg", PKG_DP)
        if pkg not in packages:
            packages.append(pkg)
    for pkg in packages:
        proc = subprocess.run(
            ["go", "test", "-count=1", "-timeout", "300s", pkg],
            cwd=ROOT, capture_output=True, text=True,
        )
        if proc.returncode != 0:
            print(f"RESTORE-FAILED: {pkg} is not green after restoring the files")
            print(proc.stdout[-3000:], proc.stderr[-3000:])
            return 2
    print(f"RESTORED: {', '.join(packages)} green after restoring every file", flush=True)

    if failures:
        print(f"\n{len(failures)} case(s) did not verify:", flush=True)
        for f in failures:
            print(" -", f)
        return 1
    print(f"\nAll {len(CASES)} back-out checks verified.", flush=True)
    return 0


if __name__ == "__main__":
    sys.exit(main())
