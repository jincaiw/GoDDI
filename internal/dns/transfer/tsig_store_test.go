package transfer

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jasonwa/goddi/internal/secretbox"
	_ "modernc.org/sqlite"
)

const tsigTestKey = "0123456789abcdef0123456789abcdef"

// newTSIGDB builds the table as migration 023 leaves it.
func newTSIGDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`
		CREATE TABLE dns_tsig_keys (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			algorithm TEXT NOT NULL DEFAULT 'hmac-sha256',
			secret TEXT NOT NULL,
			secret_ciphertext TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT (datetime('now'))
		)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	return db
}

func tsigSealer() *secretbox.Sealer { return secretbox.New(secretbox.LabelTSIG, tsigTestKey) }

// storedTSIGKey reads the row as it actually sits on disk, bypassing the
// accessors that would otherwise hide the columns under test.
func storedTSIGKey(t *testing.T, db *sql.DB, name string) (plaintext, ciphertext string) {
	t.Helper()
	if err := db.QueryRow(`SELECT secret, secret_ciphertext FROM dns_tsig_keys WHERE name = ?`, name).
		Scan(&plaintext, &ciphertext); err != nil {
		t.Fatalf("reading stored key %s: %v", name, err)
	}
	return plaintext, ciphertext
}

func TestACreatedTSIGKeyIsNotStoredInTheClear(t *testing.T) {
	db := newTSIGDB(t)

	created, err := CreateTSIGKeyRecord(db, tsigSealer(), "key-1", "transfer.example.", "hmac-sha256")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Secret == "" {
		t.Fatal("the creation response carried no secret, so the caller has nothing to configure")
	}

	plaintext, ciphertext := storedTSIGKey(t, db, "transfer.example.")
	if plaintext != "" {
		t.Fatalf("the plaintext column still holds %q", plaintext)
	}
	if !secretbox.IsSealed(ciphertext) {
		t.Fatalf("the sealed column holds %q, which is not sealed", ciphertext)
	}
	if strings.Contains(ciphertext, created.Secret) {
		t.Fatal("the sealed column contains its own plaintext")
	}
}

func TestTheStoredTSIGSecretOpensBackToWhatWasHandedOut(t *testing.T) {
	db := newTSIGDB(t)

	created, err := CreateTSIGKeyRecord(db, tsigSealer(), "key-1", "transfer.example.", "hmac-sha256")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	secrets, err := TSIGSecretMap(db, tsigSealer())
	if err != nil {
		t.Fatalf("load secret map: %v", err)
	}
	if got := secrets["transfer.example."]; got != created.Secret {
		t.Fatalf("the loaded secret %q does not match the one handed out", got)
	}
}

// The list endpoint is what any holder of dns:read can call. The secret must
// not be reachable through it — not in a field, and not by marshalling the
// record that carries it.
func TestAListedTSIGKeyNeverCarriesItsSecret(t *testing.T) {
	db := newTSIGDB(t)

	created, err := CreateTSIGKeyRecord(db, tsigSealer(), "key-1", "transfer.example.", "hmac-sha256")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	keys, err := ListTSIGKeys(db, tsigSealer())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("listed %d keys, want 1", len(keys))
	}
	if keys[0].Secret != "" {
		t.Fatalf("the listed record carries the secret %q", keys[0].Secret)
	}

	encoded, err := json.Marshal(keys[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	body := string(encoded)
	if strings.Contains(body, created.Secret) {
		t.Fatalf("the serialised record leaked the secret: %s", body)
	}
	if strings.Contains(body, `"secret"`) {
		t.Fatalf("the serialised record still has a secret field: %s", body)
	}
	if !strings.Contains(body, "secret_fingerprint") {
		t.Fatalf("the serialised record carries no fingerprint: %s", body)
	}
}

func TestTheFingerprintIdentifiesAKeyWithoutRevealingIt(t *testing.T) {
	db := newTSIGDB(t)

	first, err := CreateTSIGKeyRecord(db, tsigSealer(), "key-1", "a.example.", "hmac-sha256")
	if err != nil {
		t.Fatalf("create a: %v", err)
	}
	if _, err := CreateTSIGKeyRecord(db, tsigSealer(), "key-2", "b.example.", "hmac-sha256"); err != nil {
		t.Fatalf("create b: %v", err)
	}

	keys, err := ListTSIGKeys(db, tsigSealer())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	byName := map[string]TSIGKeyRecord{}
	for _, k := range keys {
		byName[k.Name] = k
	}

	if byName["a.example."].SecretFingerprint != first.SecretFingerprint {
		t.Fatal("the fingerprint of a listed key does not match the one from creation")
	}
	if byName["a.example."].SecretFingerprint == byName["b.example."].SecretFingerprint {
		t.Fatal("two different keys share a fingerprint")
	}
	if strings.Contains(byName["a.example."].SecretFingerprint, first.Secret) {
		t.Fatal("the fingerprint contains the secret")
	}
}

// A process with no encryption key material must not quietly write a key it
// cannot protect.
func TestCreatingATSIGKeyRefusesWhenNoKeyMaterialIsConfigured(t *testing.T) {
	db := newTSIGDB(t)

	if _, err := CreateTSIGKeyRecord(db, secretbox.New(secretbox.LabelTSIG, ""), "key-1", "transfer.example.", "hmac-sha256"); err == nil {
		t.Fatal("a TSIG key was stored while no key material was configured")
	}

	var rows int
	if err := db.QueryRow(`SELECT count(*) FROM dns_tsig_keys`).Scan(&rows); err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 0 {
		t.Fatalf("%d rows were written", rows)
	}
}

// Migration 023 adds the sealed column, but rows written before it still hold
// their secret in the clear. This is the backfill.
func TestSealingMovesPreExistingPlaintextRowsAndIsIdempotent(t *testing.T) {
	db := newTSIGDB(t)
	if _, err := db.Exec(
		`INSERT INTO dns_tsig_keys (id, name, algorithm, secret, secret_ciphertext) VALUES (?, ?, ?, ?, '')`,
		"legacy", "legacy.example.", "hmac-sha256", "bGVnYWN5LXNlY3JldA==",
	); err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}

	sealed, err := SealPlaintextTSIGSecrets(db, tsigSealer())
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if sealed != 1 {
		t.Fatalf("sealed %d rows, want 1", sealed)
	}

	plaintext, ciphertext := storedTSIGKey(t, db, "legacy.example.")
	if plaintext != "" {
		t.Fatalf("the plaintext column still holds %q", plaintext)
	}
	opened, err := tsigSealer().Open(ciphertext)
	if err != nil {
		t.Fatalf("the backfilled value does not open: %v", err)
	}
	if opened != "bGVnYWN5LXNlY3JldA==" {
		t.Fatalf("backfilled to %q", opened)
	}

	again, err := SealPlaintextTSIGSecrets(db, tsigSealer())
	if err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if again != 0 {
		t.Fatalf("the second pass resealed %d rows, so the backfill is not idempotent", again)
	}
}

func TestSealingDoesNothingWithoutKeyMaterial(t *testing.T) {
	db := newTSIGDB(t)
	if _, err := db.Exec(
		`INSERT INTO dns_tsig_keys (id, name, algorithm, secret, secret_ciphertext) VALUES ('legacy', 'legacy.example.', 'hmac-sha256', 'bGVnYWN5', '')`,
	); err != nil {
		t.Fatalf("insert: %v", err)
	}

	sealed, err := SealPlaintextTSIGSecrets(db, secretbox.New(secretbox.LabelTSIG, ""))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if sealed != 0 {
		t.Fatalf("sealed %d rows with no key material", sealed)
	}
	if plaintext, _ := storedTSIGKey(t, db, "legacy.example."); plaintext != "bGVnYWN5" {
		t.Fatalf("the row was touched: %q", plaintext)
	}
}

// A key that cannot be opened must fail the load rather than be dropped from
// the map: a map quietly missing a live key REFUSES every transfer signed with
// it, and the operator would have nothing to explain why.
func TestAnUnreadableTSIGKeyFailsTheLoadInsteadOfDisappearing(t *testing.T) {
	db := newTSIGDB(t)
	if _, err := db.Exec(
		`INSERT INTO dns_tsig_keys (id, name, algorithm, secret, secret_ciphertext) VALUES ('k', 'broken.example.', 'hmac-sha256', '', 'enc:v1:bm90LXJlYWxseQ')`,
	); err != nil {
		t.Fatalf("insert: %v", err)
	}

	if _, err := TSIGSecretMap(db, tsigSealer()); err == nil {
		t.Fatal("the load succeeded with an unreadable key, so the key would silently stop signing")
	}
}

// The list is the operator's inventory. A key it cannot decrypt still has to
// appear — saying nothing exists is worse than saying something is unreadable.
func TestAnUnreadableTSIGKeyStillAppearsInTheList(t *testing.T) {
	db := newTSIGDB(t)
	if _, err := db.Exec(
		`INSERT INTO dns_tsig_keys (id, name, algorithm, secret, secret_ciphertext) VALUES ('k', 'broken.example.', 'hmac-sha256', '', 'enc:v1:bm90LXJlYWxseQ')`,
	); err != nil {
		t.Fatalf("insert: %v", err)
	}

	keys, err := ListTSIGKeys(db, tsigSealer())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("listed %d keys, want 1", len(keys))
	}
	if keys[0].SecretFingerprint != "unreadable" {
		t.Fatalf("fingerprint is %q, want the unreadable marker", keys[0].SecretFingerprint)
	}
}

func TestAnEmptyTableLoadsToANilMap(t *testing.T) {
	db := newTSIGDB(t)

	secrets, err := TSIGSecretMap(db, tsigSealer())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if secrets != nil {
		t.Fatalf("an empty table produced a non-nil map: %v", secrets)
	}
}

// The endpoint's own default is "hmac-sha256"; the wire spelling is
// "hmac-sha256.". Both must work, and both must be stored in the same form, or
// the same key would look different depending on how it was created.
func TestCreatingAKeyAcceptsEitherSpellingOfTheAlgorithm(t *testing.T) {
	db := newTSIGDB(t)

	cases := []struct {
		name      string
		algorithm string
	}{
		{"default", ""},
		{"stored form", "hmac-sha256"},
		{"wire form", "hmac-sha256."},
		{"upper case", "HMAC-SHA512."},
	}
	for i, tc := range cases {
		created, err := CreateTSIGKeyRecord(db, tsigSealer(), "id-"+tc.name, "key-"+tc.name+".", tc.algorithm)
		if err != nil {
			t.Fatalf("%s (%q): %v", tc.name, tc.algorithm, err)
		}
		want := "hmac-sha256"
		if i == 3 {
			want = "hmac-sha512"
		}
		if created.Algorithm != want {
			t.Fatalf("%s stored algorithm %q, want %q", tc.name, created.Algorithm, want)
		}
	}
}

func TestCreatingAKeyRefusesABrokenAlgorithm(t *testing.T) {
	db := newTSIGDB(t)

	for _, algorithm := range []string{"hmac-md5", "hmac-sha1", "hmac-sha1.", "nonsense"} {
		if _, err := CreateTSIGKeyRecord(db, tsigSealer(), "id", "key.example.", algorithm); err == nil {
			t.Fatalf("algorithm %q was accepted; MD5 and SHA1 are cryptographically broken", algorithm)
		}
	}

	var rows int
	if err := db.QueryRow(`SELECT count(*) FROM dns_tsig_keys`).Scan(&rows); err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 0 {
		t.Fatalf("%d rows were written for rejected algorithms", rows)
	}
}
