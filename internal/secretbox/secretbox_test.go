package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

const testKey = "0123456789abcdef0123456789abcdef"

// legacyTOTPSeal reproduces, byte for byte, the sealing that the TOTP store
// shipped with before this package existed. It is deliberately a copy rather
// than a call into production code: its only job is to prove that a secret
// written by the old build still opens under the new one. If this test ever
// has to change, the change is a compatibility break and needs a rekey.
func legacyTOTPSeal(t *testing.T, keyMaterial, plaintext string) string {
	t.Helper()
	sum := sha256.Sum256([]byte("goddi-totp-v1:" + keyMaterial))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		t.Fatalf("legacy cipher: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("legacy aead: %v", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		t.Fatalf("legacy nonce: %v", err)
	}
	sealed := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	return "enc:v1:" + base64.RawStdEncoding.EncodeToString(append(nonce, sealed...))
}

func TestASealedValueOpensBackToItsPlaintext(t *testing.T) {
	s := New(LabelTSIG, testKey)
	const secret = "c2VjcmV0LWtleS1tYXRlcmlhbA=="

	sealed, err := s.Seal(secret)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if !IsSealed(sealed) {
		t.Fatalf("sealed value %q does not carry the prefix", sealed)
	}
	if strings.Contains(sealed, secret) {
		t.Fatalf("sealed value %q still contains its plaintext", sealed)
	}

	opened, err := s.Open(sealed)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if opened != secret {
		t.Fatalf("opened %q, want %q", opened, secret)
	}
}

func TestSealingTheSameSecretTwiceGivesTwoDifferentValues(t *testing.T) {
	s := New(LabelTSIG, testKey)

	first, err := s.Seal("same input")
	if err != nil {
		t.Fatalf("first seal: %v", err)
	}
	second, err := s.Seal("same input")
	if err != nil {
		t.Fatalf("second seal: %v", err)
	}
	if first == second {
		t.Fatal("two seals of the same plaintext are identical, so the nonce is not random")
	}
}

// A label is not decoration: it separates the key spaces, so a value sealed
// for one purpose cannot be replayed into another column that happens to use
// the same key material.
func TestAValueSealedForOneLabelDoesNotOpenUnderAnother(t *testing.T) {
	totpSealed, err := New(LabelTOTP, testKey).Seal("shared-material")
	if err != nil {
		t.Fatalf("seal with TOTP label: %v", err)
	}

	if _, err := New(LabelTSIG, testKey).Open(totpSealed); err == nil {
		t.Fatal("a TOTP-sealed value opened under the TSIG label, so the labels do not separate key spaces")
	}
	if _, err := New(LabelTOTP, testKey).Open(totpSealed); err != nil {
		t.Fatalf("the same value did not open under its own label: %v", err)
	}
}

// The TOTP label plus this derivation must reproduce the pre-secretbox
// format exactly; an upgrade must not require re-enrolling every second
// factor in the field.
func TestTheTOTPLabelStillReadsWhatTheOldBuildWrote(t *testing.T) {
	const seeded = "JBSWY3DPEHPK3PXP"

	written := legacyTOTPSeal(t, testKey, seeded)

	opened, err := New(LabelTOTP, testKey).Open(written)
	if err != nil {
		t.Fatalf("a value written by the previous build did not open: %v", err)
	}
	if opened != seeded {
		t.Fatalf("opened %q, want %q", opened, seeded)
	}
}

func TestADisabledSealerStoresTheSecretAsItFoundIt(t *testing.T) {
	s := New(LabelTSIG, "")
	if s.Enabled() {
		t.Fatal("a sealer built from empty key material reports itself enabled")
	}

	sealed, err := s.Seal("plain")
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if sealed != "plain" {
		t.Fatalf("disabled sealer changed the value to %q", sealed)
	}
	if opened, err := s.Open("plain"); err != nil || opened != "plain" {
		t.Fatalf("disabled sealer open returned (%q, %v)", opened, err)
	}
}

func TestNewRequiredRefusesToRunWithoutKeyMaterial(t *testing.T) {
	if _, err := NewRequired(LabelTSIG, ""); !errors.Is(err, ErrNoKeyMaterial) {
		t.Fatalf("NewRequired with no key material returned %v, want ErrNoKeyMaterial", err)
	}
	s, err := NewRequired(LabelTSIG, testKey)
	if err != nil {
		t.Fatalf("NewRequired with key material: %v", err)
	}
	if !s.Enabled() {
		t.Fatal("NewRequired returned a disabled sealer for non-empty key material")
	}
}

func TestOpenReturnsUnsealedValuesUnchanged(t *testing.T) {
	s := New(LabelTSIG, testKey)

	got, err := s.Open("not-sealed-at-all")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if got != "not-sealed-at-all" {
		t.Fatalf("open returned %q", got)
	}
}

// OpenSealed is the strict form: a caller that demands sealing must not be
// satisfied by a value an operator left in the clear.
func TestOpenSealedRefusesAPlaintextValue(t *testing.T) {
	s := New(LabelTSIG, testKey)

	if _, err := s.OpenSealed("not-sealed-at-all"); err == nil {
		t.Fatal("OpenSealed accepted an unsealed value")
	}
}

func TestASealedValueCannotBeOpenedWithoutKeyMaterial(t *testing.T) {
	sealed, err := New(LabelTSIG, testKey).Seal("secret")
	if err != nil {
		t.Fatalf("seal: %v", err)
	}

	if _, err := New(LabelTSIG, "").Open(sealed); err == nil {
		t.Fatal("a disabled sealer opened a sealed value instead of refusing")
	}
}

func TestASealedValueDoesNotOpenUnderTheWrongKey(t *testing.T) {
	sealed, err := New(LabelTSIG, testKey).Seal("secret")
	if err != nil {
		t.Fatalf("seal: %v", err)
	}

	if _, err := New(LabelTSIG, "another-key-entirely").Open(sealed); err == nil {
		t.Fatal("a sealed value opened under the wrong key material")
	}
}

func TestFingerprintIsStableAndRevealsNothing(t *testing.T) {
	a := Fingerprint("secret-one")
	b := Fingerprint("secret-one")
	c := Fingerprint("secret-two")

	if a != b {
		t.Fatalf("fingerprint is not stable: %q vs %q", a, b)
	}
	if a == c {
		t.Fatal("two different secrets share a fingerprint")
	}
	if strings.Contains(a, "secret-one") {
		t.Fatalf("fingerprint %q contains the secret", a)
	}
}

// --- Rekey ---

// newRekeyDB returns a control-database-shaped fixture. The single connection
// is the point: the control database runs that way, and it is what turns a
// cursor left open across a write from a bug into a permanent hang.
func newRekeyDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`
		CREATE TABLE secrets (id TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '')`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	return db
}

func putSecret(t *testing.T, db *sql.DB, id, value string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO secrets (id, value) VALUES (?, ?)`, id, value); err != nil {
		t.Fatalf("insert %s: %v", id, err)
	}
}

func readSecret(t *testing.T, db *sql.DB, id string) string {
	t.Helper()
	var value string
	if err := db.QueryRow(`SELECT value FROM secrets WHERE id = ?`, id).Scan(&value); err != nil {
		t.Fatalf("read %s: %v", id, err)
	}
	return value
}

var secretsTarget = []Target{{Table: "secrets", IDColumn: "id", ValueColumn: "value", Label: LabelTOTP}}

// The label of the target decides the derivation, so the assertions below
// always reopen with the same label the target named.
func newTestSealer(keyMaterial string) *Sealer { return New(LabelTOTP, keyMaterial) }

func TestRekeyMovesValuesFromTheOldKeyToTheNewOne(t *testing.T) {
	db := newRekeyDB(t)

	sealed, err := newTestSealer("old-key-material").Seal("seed")
	if err != nil {
		t.Fatalf("seal with old key: %v", err)
	}
	putSecret(t, db, "a", sealed)

	outcomes, err := Rekey(db, "old-key-material", "new-key-material", secretsTarget)
	if err != nil {
		t.Fatalf("rekey: %v", err)
	}
	if outcomes[0].Resealed != 1 {
		t.Fatalf("resealed %d, want 1", outcomes[0].Resealed)
	}

	opened, err := newTestSealer("new-key-material").Open(readSecret(t, db, "a"))
	if err != nil {
		t.Fatalf("the rekeyed value does not open under the new key: %v", err)
	}
	if opened != "seed" {
		t.Fatalf("opened %q, want %q", opened, "seed")
	}
}

func TestRekeySealsPlaintextThatPredatesSealing(t *testing.T) {
	db := newRekeyDB(t)
	putSecret(t, db, "a", "written-in-the-clear")

	outcomes, err := Rekey(db, "", testKey, secretsTarget)
	if err != nil {
		t.Fatalf("rekey: %v", err)
	}
	if outcomes[0].SealedPlaintext != 1 {
		t.Fatalf("sealed %d plaintext values, want 1", outcomes[0].SealedPlaintext)
	}

	opened, err := newTestSealer(testKey).Open(readSecret(t, db, "a"))
	if err != nil {
		t.Fatalf("open after sealing: %v", err)
	}
	if opened != "written-in-the-clear" {
		t.Fatalf("opened %q", opened)
	}
}

func TestRekeyIsIdempotent(t *testing.T) {
	db := newRekeyDB(t)
	putSecret(t, db, "a", "clear")

	if _, err := Rekey(db, "", testKey, secretsTarget); err != nil {
		t.Fatalf("first rekey: %v", err)
	}
	afterFirst := readSecret(t, db, "a")

	outcomes, err := Rekey(db, "", testKey, secretsTarget)
	if err != nil {
		t.Fatalf("second rekey: %v", err)
	}
	if outcomes[0].AlreadyCurrent != 1 {
		t.Fatalf("already-current %d, want 1", outcomes[0].AlreadyCurrent)
	}
	if outcomes[0].Changed() {
		t.Fatal("the second pass changed a table that was already current")
	}
	if got := readSecret(t, db, "a"); got != afterFirst {
		t.Fatal("the second pass rewrote an already-current value")
	}
}

func TestRekeyLeavesEmptyValuesAlone(t *testing.T) {
	db := newRekeyDB(t)
	putSecret(t, db, "a", "")

	outcomes, err := Rekey(db, "", testKey, secretsTarget)
	if err != nil {
		t.Fatalf("rekey: %v", err)
	}
	if outcomes[0].Empty != 1 {
		t.Fatalf("empty %d, want 1", outcomes[0].Empty)
	}
	if got := readSecret(t, db, "a"); got != "" {
		t.Fatalf("empty value became %q", got)
	}
}

// A value that opens under neither key is data loss, not a value to skip:
// silently leaving it behind would look like a successful rekey.
func TestRekeyRefusesWhenNeitherKeyOpensAValue(t *testing.T) {
	db := newRekeyDB(t)
	sealed, err := newTestSealer("a-third-key").Seal("orphan")
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	putSecret(t, db, "a", sealed)

	_, err = Rekey(db, testKey, "new-key-material", secretsTarget)
	if err == nil {
		t.Fatal("rekey reported success while a value opened under neither key")
	}
	if !strings.Contains(err.Error(), "does not open with either key") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRekeyRefusesAValueWithNoPreviousKeyAtAll(t *testing.T) {
	db := newRekeyDB(t)
	sealed, err := newTestSealer("a-third-key").Seal("orphan")
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	putSecret(t, db, "a", sealed)

	if _, err := Rekey(db, "", testKey, secretsTarget); err == nil {
		t.Fatal("rekey reported success with no previous key to fall back to")
	}
}

func TestRekeyRefusesEmptyTargetKeyMaterial(t *testing.T) {
	db := newRekeyDB(t)

	if _, err := Rekey(db, "", "", secretsTarget); err == nil {
		t.Fatal("rekey accepted empty target key material")
	}
}

// The rekey list is the inventory of sealed columns. A column added to the
// code but not here would be skipped by every rekey, which is the failure this
// asserts against.
func TestTheRekeyListNamesEverySealedColumn(t *testing.T) {
	targets := ControlDatabaseSecrets()
	if len(targets) < 2 {
		t.Fatalf("the rekey list has %d entries; TOTP seeds and TSIG keys are both sealed", len(targets))
	}

	seen := map[string]bool{}
	for _, target := range targets {
		if target.Table == "" || target.IDColumn == "" || target.ValueColumn == "" {
			t.Fatalf("incomplete target %+v", target)
		}
		if target.Label == "" {
			t.Fatalf("target %s.%s has no label, so its derivation is ambiguous", target.Table, target.ValueColumn)
		}
		key := target.Table + "." + target.ValueColumn
		if seen[key] {
			t.Fatalf("%s is listed twice", key)
		}
		seen[key] = true
	}

	for _, want := range []string{"user_totp_secrets.secret_ciphertext", "dns_tsig_keys.secret_ciphertext"} {
		if !seen[want] {
			t.Fatalf("the rekey list is missing %s", want)
		}
	}
}
