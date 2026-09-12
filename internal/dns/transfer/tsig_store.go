package transfer

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/jasonwa/goddi/internal/secretbox"
)

// TSIGKeyRecord is a stored TSIG key with its metadata (extends the
// bare TSIGKey struct in tsig.go with persistence fields).
//
// Secret is excluded from JSON on purpose. The value is handed to the caller
// exactly once, by CreateTSIGKeyRecord, in a TSIGKeyCreated; everything that
// lists or describes a key reports SecretFingerprint instead, which is enough
// to tell two keys apart and not enough to sign anything.
type TSIGKeyRecord struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Algorithm         string    `json:"algorithm"`
	Secret            string    `json:"-"`
	SecretFingerprint string    `json:"secret_fingerprint"`
	CreatedAt         time.Time `json:"created_at"`
}

// TSIGKeyCreated is the response to the call that generated a key. It is a
// separate type rather than a record with the field filled in, so that the
// plaintext cannot reach a response by way of a shared struct.
type TSIGKeyCreated struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Algorithm         string    `json:"algorithm"`
	Secret            string    `json:"secret"`
	SecretFingerprint string    `json:"secret_fingerprint"`
	CreatedAt         time.Time `json:"created_at"`
}

// ListTSIGKeys returns all stored TSIG keys. Secrets are decrypted only to
// derive the fingerprint, and are never returned.
func ListTSIGKeys(db *sql.DB, sealer *secretbox.Sealer) ([]TSIGKeyRecord, error) {
	if db == nil {
		return nil, fmt.Errorf("listing TSIG keys: no database")
	}
	rows, err := db.Query(`SELECT id, name, algorithm, secret_ciphertext, created_at FROM dns_tsig_keys ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type stored struct {
		record TSIGKeyRecord
		sealed string
	}
	var pending []stored
	for rows.Next() {
		var s stored
		if err := rows.Scan(&s.record.ID, &s.record.Name, &s.record.Algorithm, &s.sealed, &s.record.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning TSIG key: %w", err)
		}
		pending = append(pending, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	keys := make([]TSIGKeyRecord, 0, len(pending))
	for _, s := range pending {
		fingerprint, err := fingerprintStoredSecret(sealer, s.sealed)
		if err != nil {
			// A key we cannot read still has to appear in the list. Dropping it
			// would tell the operator the key does not exist, which is worse
			// than telling them it cannot be read.
			slog.Error("tsig: stored secret could not be opened", "key", s.record.Name, "error", err)
			s.record.SecretFingerprint = "unreadable"
		} else {
			s.record.SecretFingerprint = fingerprint
		}
		keys = append(keys, s.record)
	}
	return keys, nil
}

func fingerprintStoredSecret(sealer *secretbox.Sealer, sealed string) (string, error) {
	if sealed == "" {
		return "", nil
	}
	plaintext, err := sealer.Open(sealed)
	if err != nil {
		return "", err
	}
	return secretbox.Fingerprint(plaintext), nil
}

// CreateTSIGKeyRecord generates and stores a new TSIG key. The returned record
// carries the secret in the clear for this one response; the database holds
// only the sealed form.
func CreateTSIGKeyRecord(db *sql.DB, sealer *secretbox.Sealer, id, name, algorithm string) (*TSIGKeyCreated, error) {
	normalized, err := normalizeTSIGAlgorithm(algorithm)
	if err != nil {
		return nil, err
	}
	algorithm = normalized

	secret, err := CreateTSIGKey(name, algorithm)
	if err != nil {
		return nil, err
	}
	sealed, err := sealTSIGSecret(sealer, secret)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(
		`INSERT INTO dns_tsig_keys (id, name, algorithm, secret, secret_ciphertext, created_at) VALUES (?, ?, ?, '', ?, datetime('now'))`,
		id, name, algorithm, sealed,
	); err != nil {
		return nil, err
	}
	return &TSIGKeyCreated{
		ID:                id,
		Name:              name,
		Algorithm:         algorithm,
		Secret:            secret,
		SecretFingerprint: secretbox.Fingerprint(secret),
		CreatedAt:         time.Now(),
	}, nil
}

// sealTSIGSecret seals a TSIG secret, refusing to store it in the clear.
//
// Unlike TOTP — where an installation that never configured key material has
// seeds on disk that must keep being readable — a TSIG key is only ever
// written by this build. There is no legacy to honour, so a missing key is an
// error rather than a silent downgrade to plaintext.
func sealTSIGSecret(sealer *secretbox.Sealer, secret string) (string, error) {
	if !sealer.Enabled() {
		return "", fmt.Errorf("refusing to store a TSIG secret in the clear: set security.encryption_key (or GODDI_SECURITY_ENCRYPTION_KEY)")
	}
	sealed, err := sealer.Seal(secret)
	if err != nil {
		return "", fmt.Errorf("sealing TSIG secret: %w", err)
	}
	return sealed, nil
}

// DeleteTSIGKey removes a TSIG key by id.
func DeleteTSIGKey(db *sql.DB, id string) error {
	_, err := db.Exec(`DELETE FROM dns_tsig_keys WHERE id = ?`, id)
	return err
}

// TSIGSecretMap loads all stored keys as the name->secret map consumed by
// dns.Server.TsigSecret. With the map set, miekg/dns automatically verifies
// inbound signed requests (RFC 2845) and signs the corresponding responses,
// covering AXFR/IXFR/NOTIFY transports.
//
// A key that cannot be opened is reported as an error rather than skipped.
// Skipping it would leave the server running with a map that silently omits a
// live key: every transfer signed with that key would be REFUSED, and the
// operator would have nothing in the log to explain why.
func TSIGSecretMap(db *sql.DB, sealer *secretbox.Sealer) (map[string]string, error) {
	rows, err := db.Query(`SELECT name, secret_ciphertext FROM dns_tsig_keys ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type stored struct{ name, sealed string }
	var pending []stored
	for rows.Next() {
		var s stored
		if err := rows.Scan(&s.name, &s.sealed); err != nil {
			return nil, fmt.Errorf("scanning TSIG key: %w", err)
		}
		pending = append(pending, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	if len(pending) == 0 {
		return nil, nil
	}
	m := make(map[string]string, len(pending))
	for _, s := range pending {
		secret, err := sealer.Open(s.sealed)
		if err != nil {
			return nil, fmt.Errorf("opening TSIG key %q: %w", s.name, err)
		}
		if secret == "" {
			return nil, fmt.Errorf("TSIG key %q has no stored secret", s.name)
		}
		m[s.name] = secret
	}
	return m, nil
}

// CountPlaintextTSIGSecrets reports how many rows still hold their secret in
// the clear. It exists so that `goddi rekey --dry-run` can describe the whole
// migration, including the part migration 023 leaves to startup.
func CountPlaintextTSIGSecrets(db *sql.DB) (int, error) {
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM dns_tsig_keys WHERE secret <> ''`).Scan(&count); err != nil {
		return 0, fmt.Errorf("counting plaintext TSIG secrets: %w", err)
	}
	return count, nil
}

// SealPlaintextTSIGSecrets moves any secret still stored in the clear into the
// sealed column, and clears the plaintext column.
//
// This is the backfill half of migration 023. It runs at startup because
// SQLite cannot encrypt, and it is idempotent: once a row has
// secret_ciphertext set, and secret empty, it is left alone.
func SealPlaintextTSIGSecrets(db *sql.DB, sealer *secretbox.Sealer) (int, error) {
	if !sealer.Enabled() {
		return 0, nil
	}

	rows, err := db.Query(`SELECT id, name, secret FROM dns_tsig_keys WHERE secret <> ''`)
	if err != nil {
		return 0, err
	}
	type plain struct{ id, name, secret string }
	var pending []plain
	for rows.Next() {
		var p plain
		if err := rows.Scan(&p.id, &p.name, &p.secret); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scanning plaintext TSIG key: %w", err)
		}
		pending = append(pending, p)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	// Cursor closed before the first UPDATE: the control database runs on a
	// single connection, where writing while a cursor is open hangs.
	rows.Close()

	sealed := 0
	for _, p := range pending {
		ciphertext, err := sealer.Seal(p.secret)
		if err != nil {
			return sealed, fmt.Errorf("sealing TSIG key %q: %w", p.name, err)
		}
		if _, err := db.Exec(
			`UPDATE dns_tsig_keys SET secret_ciphertext = ?, secret = '' WHERE id = ?`,
			ciphertext, p.id,
		); err != nil {
			return sealed, fmt.Errorf("sealing TSIG key %q: %w", p.name, err)
		}
		sealed++
	}
	return sealed, nil
}
