// Package snapshotipamcheck checks a bounded subset of IPAM invariants in an
// isolated SQLite staging database. It does not authorize installation or roles.
package snapshotipamcheck

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jasonwa/goddi/internal/enterprise/snapshotipam"
)

var ErrInvalid = errors.New("snapshotipamcheck: invalid staging database")

const maxRows = 100000
const maxBytes = 64 << 20

type Counts struct{ Tenants, Spaces, Prefixes, Pools, Occupancies, Ledger uint64 }
type Limits struct {
	Rows, Bytes uint64
	Expected    Counts
}

// Evidence is emitted only if all named checks succeed. It does not cover
// leases, ledger history or active-ID coverage, audit, receipts, HA, production
// schema or readiness. LedgerRows covers only row shapes, order and count.
type Evidence struct {
	Integrity, ForeignKeys, RowShapes, Scopes, PrefixRelations, OccupancyRelations bool
	PoolRelations, PoolOverlaps, PoolOccupancyRelations, LedgerRows                bool
	Counts                                                                         Counts
}
type key2 struct{ tenant, space string }
type key3 struct{ tenant, space, id string }
type prefix struct {
	key          key3
	parent       string
	hasParent    bool
	family, bits int64
	first, last  [16]byte
	kind, usage  string
}
type pool struct {
	key          key3
	subnet, kind string
	family       int64
	first, last  [16]byte
}

type occupancy struct {
	key    key3
	subnet string
	family int64
	addr   [16]byte
}
type field struct {
	name, kind string
	nullable   bool
}
type table struct {
	name, order string
	fields      []field
}

var schemas = []table{
	{"ipam_tenants", "id", []field{{"id", "text", false}, {"slug", "text", false}, {"display_name", "text", false}, {"revision", "integer", false}}},
	{"ipam_spaces", "tenant_id,id", []field{{"tenant_id", "text", false}, {"id", "text", false}, {"slug", "text", false}, {"display_name", "text", false}, {"revision", "integer", false}}},
	{"ipam_prefixes", "tenant_id,space_id,id", []field{{"tenant_id", "text", false}, {"space_id", "text", false}, {"id", "text", false}, {"parent_id", "text", true}, {"family", "integer", false}, {"prefix_len", "integer", false}, {"first_ip", "blob", false}, {"last_ip", "blob", false}, {"kind", "text", false}, {"usage", "text", false}, {"site_ref", "text", true}, {"label", "text", false}, {"revision", "integer", false}}},
	{"ipam_pools", "tenant_id,space_id,id", []field{{"tenant_id", "text", false}, {"space_id", "text", false}, {"id", "text", false}, {"subnet_id", "text", false}, {"family", "integer", false}, {"pool_kind", "text", false}, {"first_ip", "blob", false}, {"last_ip", "blob", false}, {"label", "text", false}, {"revision", "integer", false}}},
	{"ipam_occupancies", "tenant_id,space_id,id", []field{{"tenant_id", "text", false}, {"space_id", "text", false}, {"id", "text", false}, {"subnet_id", "text", false}, {"family", "integer", false}, {"address", "blob", false}, {"kind", "text", false}, {"owner_ref", "text", false}, {"expires_at_ms", "integer", true}, {"created_at_ms", "integer", false}, {"revision", "integer", false}}},
	{"ipam_resource_ids", "kind,tenant_id,space_id,id", []field{{"kind", "text", false}, {"tenant_id", "text", false}, {"space_id", "text", false}, {"id", "text", false}}},
}

// Check uses one dedicated connection and one read transaction. The caller must
// guarantee a reviewed schema, an isolated staging DB, no concurrent use of this
// connection and no other writers/schema changers for the whole call. query_only
// is connection-local, not a database authorization boundary. Expected counts
// must independently come from the verified manifest to detect truncation.
func Check(ctx context.Context, conn *sql.Conn, l Limits) (out Evidence, err error) {
	if conn == nil || l.Rows == 0 || l.Rows > maxRows || l.Bytes == 0 || l.Bytes > maxBytes {
		return out, ErrInvalid
	}
	for _, p := range []string{"PRAGMA query_only", "PRAGMA foreign_keys"} {
		var n int
		if e := conn.QueryRowContext(ctx, p).Scan(&n); e != nil || n != 1 {
			return Evidence{}, ErrInvalid
		}
	}
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Evidence{}, err
	}
	defer func() {
		if e := tx.Rollback(); e != nil {
			out = Evidence{}
			err = errors.Join(err, fmt.Errorf("snapshotipamcheck: transaction cleanup: %w", e))
		}
	}()
	// Pin the main database read view and inspect the connection's schema state
	// inside this same transaction before interpreting any business rows.
	var mainExists, temporaryObjects int
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM main.sqlite_schema LIMIT 1)`).Scan(&mainExists); err != nil {
		return Evidence{}, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sqlite_temp_schema LIMIT 1)`).Scan(&temporaryObjects); err != nil {
		return Evidence{}, err
	}
	if temporaryObjects != 0 {
		return Evidence{}, ErrInvalid
	}
	databases, err := tx.QueryContext(ctx, `PRAGMA database_list`)
	if err != nil {
		return Evidence{}, err
	}
	mainSeen := false
	for databases.Next() {
		var sequence int
		var name, path string
		if err = databases.Scan(&sequence, &name, &path); err != nil {
			_ = databases.Close()
			return Evidence{}, err
		}
		if name == "main" && sequence == 0 && !mainSeen {
			mainSeen = true
			continue
		}
		if name != "temp" || sequence != 1 {
			_ = databases.Close()
			return Evidence{}, ErrInvalid
		}
	}
	if err = databases.Err(); err != nil {
		_ = databases.Close()
		return Evidence{}, err
	}
	if err = databases.Close(); err != nil {
		return Evidence{}, err
	}
	if !mainSeen {
		return Evidence{}, ErrInvalid
	}
	// Go's bytewise string ordering matches SQLite BINARY only for UTF-8.
	var encoding string
	if err = tx.QueryRowContext(ctx, `PRAGMA main.encoding`).Scan(&encoding); err != nil {
		return Evidence{}, err
	}
	if encoding != "UTF-8" {
		return Evidence{}, ErrInvalid
	}
	var integrity string
	if err = tx.QueryRowContext(ctx, "PRAGMA main.integrity_check(1)").Scan(&integrity); err != nil || integrity != "ok" {
		return Evidence{}, ErrInvalid
	}
	fk, err := tx.QueryContext(ctx, "PRAGMA main.foreign_key_check")
	if err != nil {
		return Evidence{}, err
	}
	has := fk.Next()
	fkerr := fk.Err()
	closeErr := fk.Close()
	if has || fkerr != nil || closeErr != nil {
		return Evidence{}, ErrInvalid
	}
	tenants := map[string]bool{}
	spaces := map[key2]bool{}
	prefixes := map[key3]prefix{}
	var occupations []occupancy
	var pools []pool
	var counts [6]uint64
	var rowsTotal, bytesTotal uint64
	validator := snapshotipam.New()
	for index, schema := range schemas {
		// The fixed CASE projections bound every value before database/sql copies it.
		var columns []string
		for _, f := range schema.fields {
			cond := "typeof(" + f.name + ")='" + f.kind + "'"
			if f.kind == "text" {
				cond += " AND length(CAST(" + f.name + " AS BLOB))<=4096"
			}
			if f.kind == "blob" {
				cond += " AND length(" + f.name + ")=16"
			}
			columns = append(columns, "typeof("+f.name+"), CASE WHEN "+cond+" THEN "+f.name+" END")
		}
		query := "SELECT " + strings.Join(columns, ",") + " FROM main." + schema.name + " ORDER BY " + schema.order + " LIMIT ?"
		result, e := tx.QueryContext(ctx, query, int64(l.Rows)+1)
		if e != nil {
			return Evidence{}, e
		}
		for result.Next() {
			rowsTotal++
			counts[index]++
			if rowsTotal > l.Rows {
				result.Close()
				return Evidence{}, ErrInvalid
			}
			dest := make([]any, len(schema.fields)*2)
			ptr := make([]any, len(dest))
			for i := range dest {
				ptr[i] = &dest[i]
			}
			if e = result.Scan(ptr...); e != nil {
				result.Close()
				return Evidence{}, e
			}
			obj := make(map[string]any, len(schema.fields))
			values := make(map[string]any, len(schema.fields))
			for i, f := range schema.fields {
				typ, ok := dest[2*i].(string)
				if !ok {
					result.Close()
					return Evidence{}, ErrInvalid
				}
				value := dest[2*i+1]
				if typ == "null" && f.nullable {
					obj[f.name] = nil
					values[f.name] = nil
					continue
				}
				if typ != f.kind || value == nil {
					result.Close()
					return Evidence{}, ErrInvalid
				}
				switch f.kind {
				case "text":
					s, ok := value.(string)
					if !ok || !utf8.ValidString(s) {
						result.Close()
						return Evidence{}, ErrInvalid
					}
					bytesTotal += uint64(len(s))
					obj[f.name] = s
					values[f.name] = s
				case "integer":
					n, ok := value.(int64)
					if !ok {
						result.Close()
						return Evidence{}, ErrInvalid
					}
					bytesTotal += 8
					obj[f.name] = strconv.FormatInt(n, 10)
					values[f.name] = n
				case "blob":
					b, ok := value.([]byte)
					if !ok || len(b) != 16 {
						result.Close()
						return Evidence{}, ErrInvalid
					}
					bytesTotal += 16
					var a [16]byte
					copy(a[:], b)
					obj[f.name] = base64.RawURLEncoding.EncodeToString(b)
					values[f.name] = a
				}
				if bytesTotal > l.Bytes {
					result.Close()
					return Evidence{}, ErrInvalid
				}
			}
			raw, e := json.Marshal(obj)
			if e != nil {
				result.Close()
				return Evidence{}, e
			}
			if _, e = validator.Validate(schema.name, raw); e != nil {
				result.Close()
				return Evidence{}, e
			}
			switch index {
			case 0:
				tenants[values["id"].(string)] = true
			case 1:
				spaces[key2{values["tenant_id"].(string), values["id"].(string)}] = true
			case 2:
				k := key3{values["tenant_id"].(string), values["space_id"].(string), values["id"].(string)}
				p := prefix{key: k, family: values["family"].(int64), bits: values["prefix_len"].(int64), first: values["first_ip"].([16]byte), last: values["last_ip"].([16]byte), kind: values["kind"].(string), usage: values["usage"].(string)}
				if values["parent_id"] != nil {
					p.parent = values["parent_id"].(string)
					p.hasParent = true
				}
				prefixes[k] = p
			case 3:
				k := key3{values["tenant_id"].(string), values["space_id"].(string), values["id"].(string)}
				pools = append(pools, pool{key: k, subnet: values["subnet_id"].(string), kind: values["pool_kind"].(string), family: values["family"].(int64), first: values["first_ip"].([16]byte), last: values["last_ip"].([16]byte)})
			case 4:
				k := key3{values["tenant_id"].(string), values["space_id"].(string), values["id"].(string)}
				occupations = append(occupations, occupancy{key: k, subnet: values["subnet_id"].(string), family: values["family"].(int64), addr: values["address"].([16]byte)})
			}
		}
		e = result.Err()
		closeErr = result.Close()
		if e != nil {
			return Evidence{}, e
		}
		if closeErr != nil {
			return Evidence{}, closeErr
		}
	}
	got := Counts{Tenants: counts[0], Spaces: counts[1], Prefixes: counts[2], Pools: counts[3], Occupancies: counts[4], Ledger: counts[5]}
	if got != l.Expected {
		return Evidence{}, ErrInvalid
	}
	for k := range spaces {
		if !tenants[k.tenant] {
			return Evidence{}, ErrInvalid
		}
	}
	type siblingKey struct {
		tenant, space, parent string
		hasParent             bool
		family                int64
	}
	groups := map[siblingKey][]prefix{}
	for _, p := range prefixes {
		if !spaces[key2{p.key.tenant, p.key.space}] {
			return Evidence{}, ErrInvalid
		}
		if p.hasParent {
			parent, ok := prefixes[key3{p.key.tenant, p.key.space, p.parent}]
			if !ok || parent.kind != "CONTAINER" || parent.family != p.family || parent.bits >= p.bits || bytes.Compare(parent.first[:], p.first[:]) > 0 || bytes.Compare(parent.last[:], p.last[:]) < 0 {
				return Evidence{}, ErrInvalid
			}
		}
		k := siblingKey{p.key.tenant, p.key.space, p.parent, p.hasParent, p.family}
		groups[k] = append(groups[k], p)
	}
	for _, g := range groups {
		sort.Slice(g, func(i, j int) bool { return bytes.Compare(g[i].first[:], g[j].first[:]) < 0 })
		for i := 1; i < len(g); i++ {
			if bytes.Compare(g[i-1].last[:], g[i].first[:]) >= 0 {
				return Evidence{}, ErrInvalid
			}
		}
	}
	type scopeFamily struct {
		tenant, space string
		family        int64
	}
	poolGroups := make(map[scopeFamily][]pool)
	for _, a := range pools {
		if err := ctx.Err(); err != nil {
			return Evidence{}, err
		}
		p, ok := prefixes[key3{a.key.tenant, a.key.space, a.subnet}]
		if !ok || p.kind != "SUBNET" || p.usage != "STANDARD" || p.family != a.family || bytes.Compare(a.first[:], p.first[:]) <= 0 || bytes.Compare(a.last[:], p.last[:]) > 0 || (a.family == 4 && bytes.Equal(a.last[:], p.last[:])) {
			return Evidence{}, ErrInvalid
		}
		key := scopeFamily{a.key.tenant, a.key.space, a.family}
		poolGroups[key] = append(poolGroups[key], a)
	}
	// Sort once per scope and family; all subsequent point lookups use binary
	// search. Closed intervals touching at an endpoint overlap.
	for _, group := range poolGroups {
		sort.Slice(group, func(i, j int) bool { return bytes.Compare(group[i].first[:], group[j].first[:]) < 0 })
		for i := 1; i < len(group); i++ {
			if bytes.Compare(group[i-1].last[:], group[i].first[:]) >= 0 {
				return Evidence{}, ErrInvalid
			}
		}
	}
	type addrKey struct {
		tenant, space string
		family        int64
		addr          [16]byte
	}
	seen := map[addrKey]bool{}
	for _, a := range occupations {
		if err := ctx.Err(); err != nil {
			return Evidence{}, err
		}
		p, ok := prefixes[key3{a.key.tenant, a.key.space, a.subnet}]
		if !ok || p.kind != "SUBNET" || p.family != a.family || bytes.Compare(a.addr[:], p.first[:]) < 0 || bytes.Compare(a.addr[:], p.last[:]) > 0 {
			return Evidence{}, ErrInvalid
		}
		if p.usage == "STANDARD" && (a.addr == p.first || (a.family == 4 && a.addr == p.last)) {
			return Evidence{}, ErrInvalid
		}
		k := addrKey{a.key.tenant, a.key.space, a.family, a.addr}
		if seen[k] {
			return Evidence{}, ErrInvalid
		}
		seen[k] = true
		group := poolGroups[scopeFamily{a.key.tenant, a.key.space, a.family}]
		index := sort.Search(len(group), func(i int) bool { return bytes.Compare(group[i].first[:], a.addr[:]) > 0 }) - 1
		if index >= 0 && group[index].kind == "DHCP" && bytes.Compare(a.addr[:], group[index].last[:]) <= 0 {
			return Evidence{}, ErrInvalid
		}
	}
	return Evidence{Integrity: true, ForeignKeys: true, RowShapes: true, Scopes: true, PrefixRelations: true, OccupancyRelations: true, PoolRelations: true, PoolOverlaps: true, PoolOccupancyRelations: true, LedgerRows: true, Counts: got}, nil
}
