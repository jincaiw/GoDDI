package database

import (
	"database/sql"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// migrationFileName is goose's naming rule: a numeric version, an underscore,
// a name, and the .sql suffix. Filenames are ordered as strings but have to be
// compared as numbers, which is why the version is parsed out rather than the
// directory listing being trusted to already be in order.
var migrationFileName = regexp.MustCompile(`^(\d+)_.+\.sql$`)

// MigrationStatus answers "how far has this migration group been applied, and
// how far can this build carry it".
//
// It exists because an operator about to restore a backup or roll back a
// binary has two questions that the exit status of `migrate` cannot answer:
// is this database at the version I think it is, and can the binary I have
// read it. Both are answerable from the database itself.
type MigrationStatus struct {
	// Set is what this group of migrations is called in reports: "control",
	// "dnsdata", "leases".
	Set string

	// Path is the database file the status was read from. Empty means this
	// deployment has no location for the store.
	Path string

	// Configured is false when the deployment has no location for this store,
	// which is normal for a control-only host's data planes.
	Configured bool

	// FilePresent is false when the database file does not exist yet. The
	// status is then reported without opening anything, because a report that
	// creates the database it was asked about has changed what it measured.
	FilePresent bool

	// TablePresent distinguishes "never migrated" from "migrated and rolled
	// back to nothing". goose creates its version table on the first Up, so a
	// database without one has never been touched by this project.
	TablePresent bool

	// Applied is the highest version whose most recent record says it is
	// applied. It is deliberately not MAX(version_id): rolling a migration
	// back leaves its row behind, and reading the maximum would report
	// rolled-back work as present.
	Applied int64

	// Known is the highest version this build ships. Zero means this build
	// ships nothing for this group, which is itself worth reporting.
	Known int64

	// Pending are the shipped versions that are not applied, in order.
	Pending []int64

	// Withdrawn are applied versions this build does not ship. It is what
	// restoring a newer backup onto an older binary looks like. Not an error
	// in itself -- the database is simply ahead of the binary -- but it is the
	// one signal that says so, and it is not visible any other way.
	Withdrawn []int64

	// Err is a read failure for this one group. The group is still reported,
	// so one unreadable store does not hide the state of the others.
	Err error
}

// UpToDate reports whether this group has nothing to apply and nothing the
// build does not recognise.
func (s MigrationStatus) UpToDate() bool {
	return s.Err == nil && s.FilePresent && s.TablePresent && len(s.Pending) == 0 && len(s.Withdrawn) == 0
}

// Summary is the one-line form used in reports.
func (s MigrationStatus) Summary() string {
	switch {
	case s.Err != nil:
		return fmt.Sprintf("读取失败：%v", s.Err)
	case !s.Configured:
		return "本部署未配置该存储"
	case !s.FilePresent:
		if len(s.Pending) == 0 {
			return "数据库尚未创建（本二进制也没有该库的迁移）"
		}
		return fmt.Sprintf("数据库尚未创建（本二进制自带 %d 个迁移，全部待应用）", len(s.Pending))
	case !s.TablePresent:
		return fmt.Sprintf("数据库已存在但没有迁移记录（本二进制自带 %d 个迁移，全部待应用）", len(s.Pending))
	case len(s.Pending) == 0 && len(s.Withdrawn) == 0:
		return fmt.Sprintf("已应用 %d / 本二进制最高 %d，无待应用", s.Applied, s.Known)
	default:
		parts := []string{fmt.Sprintf("已应用 %d / 本二进制最高 %d", s.Applied, s.Known)}
		if len(s.Pending) > 0 {
			parts = append(parts, "待应用 "+versionList(s.Pending))
		}
		if len(s.Withdrawn) > 0 {
			parts = append(parts, "本二进制不认识 "+versionList(s.Withdrawn))
		}
		return strings.Join(parts, "，")
	}
}

// versionList renders versions as ranges, so a report of twelve consecutive
// pending migrations reads as "13-24" instead of a wall of numbers.
func versionList(versions []int64) string {
	if len(versions) == 0 {
		return "无"
	}
	parts := make([]string, 0, 4)
	start, previous := versions[0], versions[0]
	flush := func(from, to int64) {
		if from == to {
			parts = append(parts, strconv.FormatInt(from, 10))
			return
		}
		parts = append(parts, fmt.Sprintf("%d-%d", from, to))
	}
	for _, version := range versions[1:] {
		if version == previous+1 {
			previous = version
			continue
		}
		flush(start, previous)
		start, previous = version, version
	}
	flush(start, previous)
	return strings.Join(parts, ",")
}

// ShippedMigrations lists the migration versions a filesystem carries, in
// ascending order.
//
// Files that do not follow the naming rule are skipped rather than guessed at:
// goose would refuse them too, and a version invented from a stray filename
// would make this report disagree with what `migrate` actually does.
func ShippedMigrations(fsys fs.FS) ([]int64, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("reading migration directory: %w", err)
	}

	seen := make(map[int64]bool, len(entries))
	versions := make([]int64, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := migrationFileName.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		version, err := strconv.ParseInt(match[1], 10, 64)
		if err != nil {
			continue
		}
		if seen[version] {
			continue
		}
		seen[version] = true
		versions = append(versions, version)
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i] < versions[j] })
	return versions, nil
}

// PendingMigrationStatus is the state of a store that has no database file
// yet: everything this build ships is pending.
func PendingMigrationStatus(fsys fs.FS, set string) MigrationStatus {
	status := MigrationStatus{Set: set}
	shipped, err := ShippedMigrations(fsys)
	if err != nil {
		status.Err = err
		return status
	}
	if len(shipped) > 0 {
		status.Known = shipped[len(shipped)-1]
	}
	status.Pending = shipped
	return status
}

// MigrationStatusOf reads the applied set for one migration group from an open
// connection.
func MigrationStatusOf(conn *sql.DB, fsys fs.FS, set string) MigrationStatus {
	status := MigrationStatus{Set: set, FilePresent: true}

	shipped, err := ShippedMigrations(fsys)
	if err != nil {
		status.Err = err
		return status
	}
	if len(shipped) > 0 {
		status.Known = shipped[len(shipped)-1]
	}

	applied, present, err := appliedVersions(conn)
	if err != nil {
		status.Err = err
		return status
	}
	status.TablePresent = present
	if !present {
		status.Pending = shipped
		return status
	}

	shippedSet := make(map[int64]bool, len(shipped))
	for _, version := range shipped {
		shippedSet[version] = true
	}
	for _, version := range shipped {
		if applied[version] {
			if version > status.Applied {
				status.Applied = version
			}
			continue
		}
		status.Pending = append(status.Pending, version)
	}
	for version, isApplied := range applied {
		// Version 0 is goose's own bootstrap row, not a migration.
		if !isApplied || version == 0 || shippedSet[version] {
			continue
		}
		status.Withdrawn = append(status.Withdrawn, version)
	}
	sort.Slice(status.Withdrawn, func(i, j int) bool { return status.Withdrawn[i] < status.Withdrawn[j] })
	return status
}

// appliedVersions folds goose's version table into "which versions are applied
// right now".
//
// The table is append-only: applying a migration writes a row with
// is_applied = 1, and rolling one back writes another row for the same version
// with is_applied = 0. So the answer is the *last* record per version, not the
// presence of any record for it.
func appliedVersions(conn *sql.DB) (map[int64]bool, bool, error) {
	rows, err := conn.Query(`SELECT version_id, is_applied FROM goose_db_version ORDER BY id`)
	if err != nil {
		if isMissingMigrationTable(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("reading goose_db_version: %w", err)
	}
	defer rows.Close()

	applied := map[int64]bool{}
	for rows.Next() {
		var version int64
		var raw any
		if err := rows.Scan(&version, &raw); err != nil {
			return nil, false, fmt.Errorf("reading migration record: %w", err)
		}
		state, err := asAppliedFlag(raw)
		if err != nil {
			return nil, false, fmt.Errorf("migration record for version %d: %w", version, err)
		}
		applied[version] = state
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("reading goose_db_version: %w", err)
	}
	return applied, true, nil
}

// isMissingMigrationTable recognises the one "this table is not there"
// condition that means "never migrated" rather than "something is wrong". It
// names the table as well as the error, so a different missing table is
// reported instead of being read as an unmigrated database.
func isMissingMigrationTable(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no such table") && strings.Contains(message, "goose_db_version")
}

// asAppliedFlag converts goose's is_applied column.
//
// The column is declared INTEGER, but the value arrives as whatever the driver
// hands back, so every accepted shape is named and anything else is an error.
// Guessing here would mean a database whose state cannot be read being
// reported as up to date -- the one outcome a status report must never invent.
func asAppliedFlag(raw any) (bool, error) {
	switch value := raw.(type) {
	case bool:
		return value, nil
	case int64:
		return flagFromInt(value)
	case string:
		return flagFromText(value)
	case []byte:
		return flagFromText(string(value))
	default:
		return false, fmt.Errorf("is_applied is %T, which this build does not understand", raw)
	}
}

func flagFromInt(value int64) (bool, error) {
	switch value {
	case 0:
		return false, nil
	case 1:
		return true, nil
	default:
		return false, fmt.Errorf("is_applied is %d, want 0 or 1", value)
	}
}

func flagFromText(value string) (bool, error) {
	switch strings.TrimSpace(value) {
	case "0", "false", "False", "FALSE":
		return false, nil
	case "1", "true", "True", "TRUE":
		return true, nil
	default:
		return false, fmt.Errorf("is_applied is %q, want 0 or 1", value)
	}
}
