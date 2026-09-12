package lease

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jasonwa/goddi/internal/config"
	"github.com/jasonwa/goddi/internal/dataplane"
)

// What this file is for, and what it is deliberately not for.
//
// The acceptance criterion is "a lease must be on disk before the client is
// told it has one". At the level of a single call that is
// TestNoAckWhenTheLeaseCannotBeWritten: the reply is withheld when the write
// fails. What neither that test nor any in-process test can see is what the
// file on disk looks like after the process is *gone* without warning -- a
// half-applied insert, a rollback that only ran in memory, or a binding that
// was never committed at all surviving into the next start. Those are only
// visible by killing a real process and reading the file back from another one.
//
// So the writer is a real second OS process (this test binary re-entered with
// GODDI_W14_LEASE_WRITER=1, opening the same dataplane.Open the shipping binary
// calls), and the kill is a real SIGKILL. What is asserted is the shape of the
// store afterwards.
//
// It is NOT a power-loss test, and the difference matters enough to write down.
// SIGKILL ends the process; it does not end the kernel or the storage stack.
// Every byte the writer handed to write(2) is still in the page cache, so a
// build with synchronous=NORMAL would pass this test exactly as a build with
// synchronous=FULL does -- the durability setting is invisible here. The file
// `--wal` existing after the kill is the evidence: the rows were recovered from
// a write-ahead log that was never checkpointed, not from pages that reached
// the device. What FULL buys can only be shown by cutting power to a real
// machine, which this harness cannot do. The setting itself is read back from
// the live connection in TestOpenAppliesTheDurabilitySettingsThatMakeAnAckHonest;
// that a real power cut loses nothing is, and remains, unverified.

const (
	writerEnv  = "GODDI_W14_LEASE_WRITER"
	storeEnv   = "GODDI_W14_LEASE_STORE"
	journalEnv = "GODDI_W14_LEASE_JOURNAL"
)

// writerPool and the address arithmetic below cover 10.0.0.1 .. 10.255.255.254,
// far more than any round of this drill writes, so the writer never reuses an
// address and never has to deal with a rejected duplicate.
const writerPoolSubnet = "10.0.0.0/16"

func TestMain(m *testing.M) {
	// A test binary that re-enters itself is the only way to get a real
	// SIGKILL against the real write path in a `go test` run. The switch is on
	// an environment variable rather than an argument so that a stray flag
	// cannot make the whole package run as a writer.
	if os.Getenv(writerEnv) == "1" {
		os.Exit(runWriterChild())
	}
	os.Exit(m.Run())
}

// runWriterChild is the second process: it opens the real lease store, seeds
// one pool, and then writes bindings as fast as it can until it is killed.
//
// It journals each binding *after* CreateLease has returned, and never syncs
// the journal. That is deliberate. The journal only has to be readable by the
// parent, and a byte handed to write(2) is readable by another process even if
// the writer never syncs and then dies -- the page cache outlives the process.
// Syncing would make the journal as slow as the store and buy nothing, and not
// syncing keeps the writes tight enough that a kill lands in the middle of one.
func runWriterChild() int {
	store, err := dataplane.Open(config.DataPlaneLease, os.Getenv(storeEnv))
	if err != nil {
		fmt.Fprintln(os.Stderr, "writer: opening the lease store:", err)
		return 1
	}
	defer store.Close()

	if _, err := store.Exec(`
		INSERT INTO dhcp_scopes (id, name, subnet, start_ip, end_ip, enabled)
		VALUES ('scope-1', 'durability', ?, '10.0.0.1', '10.255.255.254', 1)`,
		writerPoolSubnet); err != nil {
		fmt.Fprintln(os.Stderr, "writer: seeding the pool:", err)
		return 1
	}

	journal, err := os.OpenFile(os.Getenv(journalEnv), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		fmt.Fprintln(os.Stderr, "writer: opening the journal:", err)
		return 1
	}
	defer journal.Close()

	m := NewManager(store.DB)
	fmt.Println("READY")

	for i := 1; i < 1<<24; i++ {
		ip := fmt.Sprintf("10.%d.%d.%d", (i>>16)&0xff, (i>>8)&0xff, i&0xff)
		mac := fmt.Sprintf("02:00:00:%02x:%02x:%02x", (i>>16)&0xff, (i>>8)&0xff, i&0xff)
		if _, err := m.CreateLease("scope-1", ip, mac, "durability", time.Hour); err != nil {
			fmt.Fprintln(os.Stderr, "writer: creating a lease:", err)
			return 1
		}
		if _, err := journal.WriteString(strconv.Itoa(i) + "\n"); err != nil {
			fmt.Fprintln(os.Stderr, "writer: journalling:", err)
			return 1
		}
	}
	return 0
}

// TestEveryCommittedBindingIsOnDiskAfterAKill is the durability property stated
// as something that can fail.
//
// Two directions, and both are needed for it to mean anything:
//
//   - Everything the writer was told it had committed is in the store. A
//     missing one is an acknowledged binding the server has forgotten -- the
//     client holds an address nobody can account for.
//   - The store holds no more bindings than were committed, give or take the
//     one that could have been in flight when the process died. A surplus is
//     the opposite failure and the more insidious one: a row from a
//     transaction that was rolled back, or one that a future power loss would
//     have taken back, would read as a binding the server issued. It is the
//     reason the store runs synchronous=FULL at all.
func TestEveryCommittedBindingIsOnDiskAfterAKill(t *testing.T) {
	// Three volumes, so the kill lands at a different point in the run each
	// time rather than always right after startup.
	for _, committed := range []int{50, 300, 900} {
		t.Run(fmt.Sprintf("killed after %d committed", committed), func(t *testing.T) {
			dir := t.TempDir()
			dsn := filepath.Join(dir, "leases.db")
			journalPath := filepath.Join(dir, "committed.txt")

			cmd := exec.Command(os.Args[0])
			cmd.Env = append(os.Environ(),
				writerEnv+"=1", storeEnv+"="+dsn, journalEnv+"="+journalPath)
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatalf("wiring the writer's stdout: %v", err)
			}
			cmd.Stderr = os.Stderr
			if err := cmd.Start(); err != nil {
				t.Fatalf("starting the writer: %v", err)
			}

			// Wait for the writer to be past its migrations and seeding, then
			// for it to have committed enough to be worth killing. Waiting on
			// the journal rather than on a sleep means a slow machine extends
			// the wait instead of turning this into a test that proves nothing.
			line, err := bufio.NewReader(stdout).ReadString('\n')
			if err != nil || strings.TrimSpace(line) != "READY" {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				t.Fatalf("the writer never reported READY (read %q, err %v)", line, err)
			}
			if err := waitForJournal(journalPath, committed, 60*time.Second); err != nil {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				t.Fatalf("%v", err)
			}

			// SIGKILL, not SIGTERM: no deferred close, no final checkpoint, no
			// chance to finish the statement it is in the middle of.
			if err := cmd.Process.Kill(); err != nil {
				t.Fatalf("killing the writer: %v", err)
			}
			_ = cmd.Wait()

			journalled := readJournal(t, journalPath)
			if len(journalled) < committed {
				t.Fatalf("the journal holds %d committed bindings, expected at least %d; the drill proved nothing",
					len(journalled), committed)
			}

			// The recovery is SQLite's, but that it happens through the
			// product's own opener -- the same pragmas, the same migrations --
			// is this project's business.
			store, err := dataplane.Open(config.DataPlaneLease, dsn)
			if err != nil {
				t.Fatalf("reopening the lease store after the kill: %v", err)
			}
			defer store.Close()

			var claim string
			if err := store.QueryRow(`PRAGMA integrity_check`).Scan(&claim); err != nil {
				t.Fatalf("integrity_check after the kill: %v", err)
			}
			if claim != "ok" {
				t.Errorf("integrity_check after the kill = %q, want ok", claim)
			}

			// The write-ahead log is still there, which is the concrete reason
			// this drill is about crash consistency and not about power loss:
			// the rows came back from a log this process never checkpointed.
			if _, err := os.Stat(dsn + "-wal"); err != nil {
				t.Errorf("the write-ahead log is gone after the kill (%v); the store cannot have recovered the way this test assumes", err)
			}

			var rows int
			if err := store.QueryRow(`SELECT COUNT(*) FROM dhcp_leases`).Scan(&rows); err != nil {
				t.Fatalf("counting the leases left by the killed writer: %v", err)
			}
			if rows < len(journalled) {
				t.Errorf("the store holds %d bindings but %d were committed before the kill; an acknowledged binding did not survive",
					rows, len(journalled))
			}
			if rows > len(journalled)+1 {
				t.Errorf("the store holds %d bindings but only %d were committed (at most one can have been in flight); a binding the server never issued survived the kill",
					rows, len(journalled))
			}

			// Every committed binding, not merely the same number of rows: a
			// store holding the right count of the wrong addresses would pass
			// a count-only check.
			missing := 0
			for _, i := range journalled {
				var status string
				err := store.QueryRow(
					`SELECT status FROM dhcp_leases WHERE scope_id = 'scope-1' AND ip_address = ?`,
					writerIP(i)).Scan(&status)
				if err == sql.ErrNoRows {
					missing++
					continue
				}
				if err != nil {
					t.Fatalf("reading back committed binding %d: %v", i, err)
				}
				if status != string(LeaseStatusActive) {
					t.Errorf("committed binding %d came back as %q, want active", i, status)
				}
			}
			if missing > 0 {
				t.Errorf("%d of %d committed bindings did not come back after the kill", missing, len(journalled))
			}

			t.Logf("killed after %d committed bindings: %d rows, %d recovered from the write-ahead log",
				len(journalled), rows, len(journalled))
		})
	}
}

// TestAWriteTheStoreRefusesIsReportedAndLeavesNothing is the store-level half of
// "no acknowledgement for a binding that was not committed".
//
// The reply path is covered elsewhere; what this pins is that the failure is
// not swallowed one layer down, and that a refused write leaves no partial row
// for a later restart to adopt as a real binding.
func TestAWriteTheStoreRefusesIsReportedAndLeavesNothing(t *testing.T) {
	store := newLeaseStore(t)
	seedScope(t, store.DB, "scope-1", "durability", writerPoolSubnet, "10.0.0.1", "10.255.255.254")
	m := NewManager(store.DB)

	// Storage-layer injection: the database itself refuses the insert, which is
	// what a read-only filesystem, a full disk or a corrupt page would look
	// like from here.
	if _, err := store.Exec(`CREATE TRIGGER w14_refuse_insert BEFORE INSERT ON dhcp_leases
		BEGIN SELECT RAISE(ABORT, 'simulated storage failure'); END;`); err != nil {
		t.Fatalf("installing the failing trigger: %v", err)
	}

	if _, err := m.CreateLease("scope-1", "10.0.0.5", "02:00:00:00:00:05", "refused", time.Hour); err == nil {
		t.Fatal("CreateLease reported success although the store refused the write")
	}

	var rows int
	if err := store.QueryRow(`SELECT COUNT(*) FROM dhcp_leases`).Scan(&rows); err != nil {
		t.Fatalf("counting after the refused write: %v", err)
	}
	if rows != 0 {
		t.Errorf("a refused write left %d rows behind, want 0", rows)
	}

	// And the same call succeeds once storage works again, so the refusal above
	// is about the store saying no and not about the call being broken.
	if _, err := store.Exec(`DROP TRIGGER w14_refuse_insert`); err != nil {
		t.Fatalf("dropping the failing trigger: %v", err)
	}
	if _, err := m.CreateLease("scope-1", "10.0.0.5", "02:00:00:00:00:05", "accepted", time.Hour); err != nil {
		t.Fatalf("CreateLease after storage recovered: %v", err)
	}
}

// writerIP mirrors the child's address arithmetic. Keeping the two in one place
// is not an option (the child cannot call into the parent's test code through a
// process boundary), so the parent recomputes it and the mismatch would show up
// as every committed binding reading as missing.
func writerIP(i int) string {
	return fmt.Sprintf("10.%d.%d.%d", (i>>16)&0xff, (i>>8)&0xff, i&0xff)
}

// waitForJournal blocks until the journal holds at least want lines, or gives
// up. Counting lines rather than parsing them keeps this usable while the
// writer is still appending.
func waitForJournal(path string, want int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if n := countLines(path); n >= want {
			return nil
		}
		time.Sleep(5 * time.Millisecond)
	}
	return fmt.Errorf("the writer committed only %d bindings in %s, wanted at least %d", countLines(path), timeout, want)
}

func countLines(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n := 0
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}

// readJournal returns the committed sequence numbers, in order.
//
// A trailing line with no newline cannot exist here -- the writer appends the
// newline in the same write as the number -- but if one ever did it would mean
// a committed binding, so it is kept rather than silently dropped.
func readJournal(t *testing.T, path string) []int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the writer's journal: %v", err)
	}
	out := make([]int, 0, 1024)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		n, err := strconv.Atoi(line)
		if err != nil {
			t.Fatalf("the journal holds %q, which is not a sequence number: %v", line, err)
		}
		out = append(out, n)
	}
	return out
}
