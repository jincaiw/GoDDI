package transfer

import (
	"database/sql"
	"fmt"
	"time"
)

// TSIGKeyRecord is a stored TSIG key with its metadata (extends the
// bare TSIGKey struct in tsig.go with persistence fields).
type TSIGKeyRecord struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Algorithm string    `json:"algorithm"`
	Secret    string    `json:"secret"`
	CreatedAt time.Time `json:"created_at"`
}

// ListTSIGKeys returns all stored TSIG keys.
func ListTSIGKeys(db *sql.DB) ([]TSIGKeyRecord, error) {
	rows, err := db.Query(`SELECT id, name, algorithm, secret, created_at FROM dns_tsig_keys ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	keys := make([]TSIGKeyRecord, 0, 8)
	for rows.Next() {
		var k TSIGKeyRecord
		if err := rows.Scan(&k.ID, &k.Name, &k.Algorithm, &k.Secret, &k.CreatedAt); err != nil {
			continue
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// CreateTSIGKeyRecord generates and stores a new TSIG key.
func CreateTSIGKeyRecord(db *sql.DB, id, name, algorithm string) (*TSIGKeyRecord, error) {
	if !isAlgorithmAccepted(algorithm) {
		return nil, fmt.Errorf("unsupported TSIG algorithm: %s (use hmac-sha256 or hmac-sha512)", algorithm)
	}
	secret, err := CreateTSIGKey(name, algorithm)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(
		`INSERT INTO dns_tsig_keys (id, name, algorithm, secret, created_at) VALUES (?, ?, ?, ?, datetime('now'))`,
		id, name, algorithm, secret,
	); err != nil {
		return nil, err
	}
	return &TSIGKeyRecord{ID: id, Name: name, Algorithm: algorithm, Secret: secret, CreatedAt: time.Now()}, nil
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
func TSIGSecretMap(db *sql.DB) map[string]string {
	keys, err := ListTSIGKeys(db)
	if err != nil || len(keys) == 0 {
		return nil
	}
	m := make(map[string]string, len(keys))
	for _, k := range keys {
		m[k.Name] = k.Secret
	}
	return m
}
