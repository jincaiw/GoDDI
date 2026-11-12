// Package dnswire implements a bounded candidate snapshot format, not a DNS
// server, authorization decision, signed artifact, or durable activation.
package dnswire

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	MaxBytes   = 1 << 20
	MaxZones   = 128
	MaxRecords = 8192
	MaxTTL     = 1<<31 - 1
)

var ErrInvalid = errors.New("dnswire: invalid candidate snapshot")

// Document is an input/output value. Version is a positive decimal string;
// monotonicity across documents must be checked by the publication layer.
type Document struct {
	Cluster  string `json:"cluster"`
	Node     string `json:"node"`
	Snapshot string `json:"snapshot"`
	Version  string `json:"version"`
	View     string `json:"view"`
	Zones    []Zone `json:"zones"`
}
type Zone struct {
	Name    string   `json:"name"`
	Serial  uint32   `json:"serial"`
	Records []Record `json:"records"`
}
type Record struct {
	Owner string `json:"owner"`
	Type  string `json:"type"`
	RData string `json:"rdata"`
	TTL   uint32 `json:"ttl"`
}

// Snapshot is returned by value and keeps its canonical bytes private.
type Snapshot struct {
	raw    []byte
	digest [32]byte
}

func (s Snapshot) Bytes() []byte    { return bytes.Clone(s.raw) }
func (s Snapshot) Digest() [32]byte { return s.digest }

// Document returns a fresh value with independently allocated record slices.
func (s Snapshot) Document() (Document, error) {
	if len(s.raw) == 0 {
		return Document{}, ErrInvalid
	}
	var d Document
	if err := json.Unmarshal(s.raw, &d); err != nil {
		return Document{}, err
	}
	return d, nil
}

// Build validates, copies and sorts input before producing canonical bytes.
// The caller must not mutate input concurrently with this call.
func Build(input Document) (Snapshot, error) {
	if err := validate(input); err != nil {
		return Snapshot{}, err
	}
	d := input
	d.Zones = make([]Zone, len(input.Zones))
	for i, z := range input.Zones {
		d.Zones[i] = z
		d.Zones[i].Records = append([]Record(nil), z.Records...)
		sort.Slice(d.Zones[i].Records, func(a, b int) bool { return lessRecord(d.Zones[i].Records[a], d.Zones[i].Records[b]) })
	}
	sort.Slice(d.Zones, func(i, j int) bool { return d.Zones[i].Name < d.Zones[j].Name })
	raw, err := json.Marshal(d)
	if err != nil || len(raw) > MaxBytes {
		return Snapshot{}, ErrInvalid
	}
	return Snapshot{raw: raw, digest: sha256.Sum256(raw)}, nil
}

// Decode accepts only the exact canonical bytes output by Build. Re-encoding
// for comparison rejects duplicate keys, unknown fields, reordered fields,
// invalid Unicode replacement, alternate escaping and trailing content. The
// digest is over the original accepted bytes, never a normalized substitute.
func Decode(raw []byte) (Snapshot, error) {
	if len(raw) == 0 || len(raw) > MaxBytes || !utf8.Valid(raw) {
		return Snapshot{}, ErrInvalid
	}
	var d Document
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil {
		return Snapshot{}, fmt.Errorf("%w: JSON", ErrInvalid)
	}
	candidate, err := Build(d)
	if err != nil {
		return Snapshot{}, err
	}
	if !bytes.Equal(raw, candidate.raw) {
		return Snapshot{}, fmt.Errorf("%w: noncanonical JSON", ErrInvalid)
	}
	return candidate, nil
}

func uuidOK(s string) bool {
	u, err := uuid.Parse(s)
	return err == nil && u != uuid.Nil && u.String() == s
}
func validName(s string) bool {
	if s == "." {
		return true
	}
	if len(s) < 2 || len(s) > 254 || s[len(s)-1] != '.' {
		return false
	}
	for _, part := range strings.Split(s[:len(s)-1], ".") {
		if len(part) == 0 || len(part) > 63 {
			return false
		}
		for i := 0; i < len(part); i++ {
			c := part[i]
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
				return false
			}
		}
	}
	return true
}
func inZone(owner, zone string) bool {
	return zone == "." || owner == zone || strings.HasSuffix(owner, "."+zone)
}
func decimal(s string, bits int) bool {
	if len(s) == 0 || len(s) > 20 || len(s) > 1 && s[0] == '0' {
		return false
	}
	for i := range s {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	_, err := strconv.ParseUint(s, 10, bits)
	return err == nil
}
func lessRecord(a, b Record) bool {
	if a.Owner != b.Owner {
		return a.Owner < b.Owner
	}
	if a.Type != b.Type {
		return a.Type < b.Type
	}
	return a.RData < b.RData
}

func validate(d Document) error {
	if !uuidOK(d.Cluster) || !uuidOK(d.Node) || !uuidOK(d.Snapshot) || !decimal(d.Version, 63) || d.Version == "0" || len(d.View) == 0 || len(d.View) > 128 {
		return ErrInvalid
	}
	for i := range d.View {
		c := d.View[i]
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
			return ErrInvalid
		}
	}
	if len(d.Zones) == 0 || len(d.Zones) > MaxZones {
		return ErrInvalid
	}
	seenZones := make(map[string]bool, len(d.Zones))
	total := 0
	for _, z := range d.Zones {
		if !validName(z.Name) || seenZones[z.Name] || len(z.Records) == 0 || len(z.Records) > MaxRecords {
			return ErrInvalid
		}
		seenZones[z.Name] = true
		total += len(z.Records)
		if total > MaxRecords {
			return ErrInvalid
		}
		seen := make(map[Record]bool, len(z.Records))
		ttl := make(map[string]uint32)
		soa, ns := 0, 0
		for _, r := range z.Records {
			if !validName(r.Owner) || !inZone(r.Owner, z.Name) || len(r.Type) > 4 || r.TTL > MaxTTL || len(r.RData) == 0 || len(r.RData) > 1024 {
				return ErrInvalid
			}
			key := r.Owner + "\x00" + r.Type
			if old, ok := ttl[key]; ok && old != r.TTL {
				return ErrInvalid
			}
			ttl[key] = r.TTL
			if seen[r] {
				return ErrInvalid
			}
			seen[r] = true
			switch r.Type {
			case "A", "AAAA":
				ip, err := netip.ParseAddr(r.RData)
				if err != nil || ip.Zone() != "" || ip.Is4In6() || ip.String() != r.RData || (r.Type == "A" && !ip.Is4()) || (r.Type == "AAAA" && !ip.Is6()) {
					return ErrInvalid
				}
			case "NS":
				if !validName(r.RData) {
					return ErrInvalid
				}
				if r.Owner == z.Name {
					ns++
				}
			case "SOA":
				if r.Owner != z.Name {
					return ErrInvalid
				}
				soa++
				fields := strings.Split(r.RData, " ")
				if len(fields) != 7 || !validName(fields[0]) || !validName(fields[1]) {
					return ErrInvalid
				}
				for _, v := range fields[2:] {
					if !decimal(v, 32) {
						return ErrInvalid
					}
				}
				if fields[2] != strconv.FormatUint(uint64(z.Serial), 10) {
					return ErrInvalid
				}
			default:
				return ErrInvalid
			}
		}
		if soa != 1 || ns == 0 {
			return ErrInvalid
		}
	}
	return nil
}
