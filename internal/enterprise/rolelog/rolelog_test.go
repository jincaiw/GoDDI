package rolelog

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

const valid = `{"version":"1","cluster_id":"11111111-1111-4111-8111-111111111111","node_id":"22222222-2222-4222-8222-222222222222","device_id":"device-1","epoch":"7","phase":"ACTIVE","nonce":"33333333-3333-4333-8333-333333333333","grant_digest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","evidence_digest":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","db_generation":"generation-1"}`

func validView(r Record) AuthorityView {
	return AuthorityView{r.ClusterID, r.NodeID, r.DeviceID, r.Epoch, r.Phase, r.Nonce, r.GrantDigest, r.EvidenceDigest, r.DBGeneration}
}

func TestParseAndCompare(t *testing.T) {
	r, err := Parse([]byte(valid))
	if err != nil {
		t.Fatal(err)
	}
	if r.Epoch != 7 || r.Version != "1" {
		t.Fatalf("unexpected record: %+v", r)
	}
	if err := Compare(r, validView(r)); err != nil {
		t.Fatal(err)
	}
	// Even an ACTIVE record that matches is merely data consistency, not permission.
}

func TestRejectInvalidJSONAndFields(t *testing.T) {
	cases := map[string]string{
		"duplicate":             strings.Replace(valid, `"version":"1"`, `"version":"1","version":"1"`, 1),
		"duplicate escaped key": strings.Replace(valid, `"version":"1"`, `"version":"1","vers\u0069on":"1"`, 1),
		"unknown":               strings.Replace(valid, `"version":"1"`, `"version":"1","other":"x"`, 1),
		"missing":               strings.Replace(valid, `"version":"1",`, "", 1),
		"null":                  strings.Replace(valid, `"epoch":"7"`, `"epoch":null`, 1),
		"number":                strings.Replace(valid, `"epoch":"7"`, `"epoch":7`, 1),
		"object":                strings.Replace(valid, `"epoch":"7"`, `"epoch":{}`, 1),
		"array":                 `[]`, "trailing": valid + ` {}`, "truncated": valid[:len(valid)-1],
		"bad utf8":         strings.Replace(valid, "device-1", string([]byte{0xff}), 1),
		"oversized":        valid + strings.Repeat(" ", MaxRecordBytes),
		"version":          strings.Replace(valid, `"version":"1"`, `"version":"2"`, 1),
		"phase":            strings.Replace(valid, `"ACTIVE"`, `"active"`, 1),
		"uuid":             strings.Replace(valid, "11111111-1111-4111-8111-111111111111", "00000000-0000-0000-0000-000000000000", 1),
		"same identities":  strings.Replace(valid, "22222222-2222-4222-8222-222222222222", "11111111-1111-4111-8111-111111111111", 1),
		"digest":           strings.Replace(valid, strings.Repeat("a", 64), strings.Repeat("A", 64), 1),
		"empty device":     strings.Replace(valid, "device-1", "", 1),
		"empty generation": strings.Replace(valid, "generation-1", "", 1),
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(input)); !errors.Is(err, ErrInvalid) {
				t.Fatalf("got %v", err)
			}
		})
	}
	for _, epoch := range []string{"", "0", "00", "01", "-1", "+1", "1.0", " 1", "9223372036854775808", "99999999999999999999999999"} {
		t.Run(fmt.Sprintf("epoch_%q", epoch), func(t *testing.T) {
			input := strings.Replace(valid, `"epoch":"7"`, `"epoch":"`+epoch+`"`, 1)
			if _, err := Parse([]byte(input)); !errors.Is(err, ErrInvalid) {
				t.Fatalf("got %v", err)
			}
		})
	}
	if _, err := Parse([]byte(strings.Replace(valid, `"epoch":"7"`, `"epoch":"9223372036854775807"`, 1))); err != nil {
		t.Fatal(err)
	}
}

func TestCompareEveryField(t *testing.T) {
	r, err := Parse([]byte(valid))
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(*AuthorityView){
		"cluster":    func(a *AuthorityView) { a.ClusterID = "other" },
		"node":       func(a *AuthorityView) { a.NodeID = "other" },
		"device":     func(a *AuthorityView) { a.DeviceID = "other" },
		"epoch":      func(a *AuthorityView) { a.Epoch++ },
		"phase":      func(a *AuthorityView) { a.Phase = "STANDBY" },
		"nonce":      func(a *AuthorityView) { a.Nonce = "other" },
		"grant":      func(a *AuthorityView) { a.GrantDigest = strings.Repeat("c", 64) },
		"evidence":   func(a *AuthorityView) { a.EvidenceDigest = strings.Repeat("c", 64) },
		"generation": func(a *AuthorityView) { a.DBGeneration = "other" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			a := validView(r)
			mutate(&a)
			if err := Compare(r, a); !errors.Is(err, ErrMismatch) {
				t.Fatalf("got %v", err)
			}
		})
	}
	if err := Compare(Record{}, AuthorityView{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("zero record: %v", err)
	}
}
