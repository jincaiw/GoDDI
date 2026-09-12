package secretbox

import (
	"database/sql"
	"fmt"
)

// Target names one stored-secret column and the purpose label it was sealed
// under.
//
// Table and column names are interpolated into SQL, so they must come from
// compiled-in constants, never from configuration or a request.
type Target struct {
	Table       string
	IDColumn    string
	ValueColumn string

	// Label is the purpose the column was sealed under. TOTP seeds and TSIG
	// keys live under different labels, so a single rekey run has to rebuild
	// the derivation per target rather than share one key.
	Label string
}

// RekeyOutcome is what a rekey found and did, per target.
type RekeyOutcome struct {
	Table string

	// Resealed counts values that opened with the previous key and were
	// written again under the current one.
	Resealed int

	// SealedPlaintext counts values that carried no prefix at all — secrets
	// an older build stored in the clear — and have now been sealed.
	SealedPlaintext int

	// AlreadyCurrent counts values the current key already opens.
	AlreadyCurrent int

	// Empty counts rows whose value column is empty.
	Empty int
}

// Changed reports whether the run moved anything at all.
func (o RekeyOutcome) Changed() bool { return o.Resealed > 0 || o.SealedPlaintext > 0 }

// Rekey moves stored secrets from one key material to another.
//
// It exists for the one migration that cannot happen by itself: installing a
// dedicated security.encryption_key over data that was sealed with the JWT
// secret. Without it, setting that key would silently make every stored TOTP
// seed and TSIG key unreadable.
//
// fromKeyMaterial may be empty, in which case only unsealed values are sealed
// and a value the current key cannot open is reported as a failure rather than
// left behind. toKeyMaterial must be non-empty.
//
// Rows are read to completion before any write, because the control database
// runs on a single connection: issuing an UPDATE while a SELECT's cursor is
// still open on the same connection blocks forever rather than erroring.
func Rekey(db *sql.DB, fromKeyMaterial, toKeyMaterial string, targets []Target) ([]RekeyOutcome, error) {
	return walk(db, fromKeyMaterial, toKeyMaterial, targets, true)
}

// RekeyPlan reports what Rekey would do without writing anything. It is the
// same walk with the writes suppressed, so the two cannot drift apart.
func RekeyPlan(db *sql.DB, fromKeyMaterial, toKeyMaterial string, targets []Target) ([]RekeyOutcome, error) {
	return walk(db, fromKeyMaterial, toKeyMaterial, targets, false)
}

func walk(db *sql.DB, fromKeyMaterial, toKeyMaterial string, targets []Target, commit bool) ([]RekeyOutcome, error) {
	if toKeyMaterial == "" {
		return nil, fmt.Errorf("secretbox: refusing to rekey onto empty key material")
	}

	outcomes := make([]RekeyOutcome, 0, len(targets))
	for _, t := range targets {
		to := New(t.Label, toKeyMaterial)
		if !to.Enabled() {
			return outcomes, fmt.Errorf("secretbox: could not initialise the AEAD for label %q", t.Label)
		}
		var from *Sealer
		if fromKeyMaterial != "" {
			from = New(t.Label, fromKeyMaterial)
		}
		outcome, err := rekeyTarget(db, from, to, t, commit)
		if err != nil {
			return outcomes, err
		}
		outcomes = append(outcomes, outcome)
	}
	return outcomes, nil
}

func rekeyTarget(db *sql.DB, from, to *Sealer, t Target, commit bool) (RekeyOutcome, error) {
	outcome := RekeyOutcome{Table: t.Table}

	type row struct {
		id    string
		value string
	}

	rows, err := db.Query(fmt.Sprintf("SELECT %s, %s FROM %s", t.IDColumn, t.ValueColumn, t.Table))
	if err != nil {
		return outcome, fmt.Errorf("reading %s.%s: %w", t.Table, t.ValueColumn, err)
	}
	var pending []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.value); err != nil {
			rows.Close()
			return outcome, fmt.Errorf("scanning %s.%s: %w", t.Table, t.ValueColumn, err)
		}
		pending = append(pending, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return outcome, fmt.Errorf("iterating %s.%s: %w", t.Table, t.ValueColumn, err)
	}
	// Cursor closed before the first UPDATE; see the doc comment.
	rows.Close()

	for _, r := range pending {
		switch {
		case r.value == "":
			outcome.Empty++
			continue

		case !IsSealed(r.value):
			sealed, err := to.Seal(r.value)
			if err != nil {
				return outcome, fmt.Errorf("sealing %s.%s row %s: %w", t.Table, t.ValueColumn, r.id, err)
			}
			if commit {
				if err := writeValue(db, t, r.id, sealed); err != nil {
					return outcome, err
				}
			}
			outcome.SealedPlaintext++

		default:
			if _, err := to.Open(r.value); err == nil {
				outcome.AlreadyCurrent++
				continue
			}
			if !from.Enabled() {
				return outcome, fmt.Errorf("%s.%s row %s is sealed but does not open with the current key, and no previous key was supplied", t.Table, t.ValueColumn, r.id)
			}
			plaintext, err := from.Open(r.value)
			if err != nil {
				return outcome, fmt.Errorf("%s.%s row %s does not open with either key", t.Table, t.ValueColumn, r.id)
			}
			sealed, err := to.Seal(plaintext)
			if err != nil {
				return outcome, fmt.Errorf("resealing %s.%s row %s: %w", t.Table, t.ValueColumn, r.id, err)
			}
			if commit {
				if err := writeValue(db, t, r.id, sealed); err != nil {
					return outcome, err
				}
			}
			outcome.Resealed++
		}
	}

	return outcome, nil
}

func writeValue(db *sql.DB, t Target, id, value string) error {
	if _, err := db.Exec(
		fmt.Sprintf("UPDATE %s SET %s = ? WHERE %s = ?", t.Table, t.ValueColumn, t.IDColumn),
		value, id,
	); err != nil {
		return fmt.Errorf("writing %s.%s row %s: %w", t.Table, t.ValueColumn, id, err)
	}
	return nil
}

// ControlDatabaseSecrets lists the columns a rekey has to walk to cover every
// secret this build seals. It lives next to the rekey machinery so that adding
// a sealed column without adding it here is a visible omission rather than a
// silent one.
func ControlDatabaseSecrets() []Target {
	return []Target{
		{Table: "user_totp_secrets", IDColumn: "user_id", ValueColumn: "secret_ciphertext", Label: LabelTOTP},
		{Table: "dns_tsig_keys", IDColumn: "id", ValueColumn: "secret_ciphertext", Label: LabelTSIG},
	}
}
