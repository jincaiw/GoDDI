package database

import (
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"testing"

	goddiassets "github.com/jasonwa/goddi"
	"github.com/jasonwa/goddi/internal/dataplane"
)

// The rules checked here are ADR-0002. The point of writing them as code
// rather than as prose is that prose cannot refuse a migration.
//
// The checker is deliberately strict about what it does not understand: an
// unrecognised top-level statement, or a block comment it cannot strip, fails
// the check rather than being skipped. A checker that silently ignores what it
// cannot parse is a checker people learn to ignore.

// statementClass is what a statement does to the schema.
type statementClass int

const (
	// classUnrecognised is a statement this checker has no rule for. It is a
	// failure, not a pass: the alternative is a new kind of statement landing
	// in a migration and the rule silently not covering it.
	classUnrecognised statementClass = iota

	// classAdd only adds. An older binary ignores what it does not know.
	classAdd

	// classReplaceDrop removes an object that the same Up recreates.
	classReplaceDrop

	// classBackfill writes data.
	classBackfill

	// classContract removes or renames something an older binary depends on.
	classContract
)

func (c statementClass) String() string {
	switch c {
	case classAdd:
		return "ADD"
	case classReplaceDrop:
		return "REPLACE"
	case classBackfill:
		return "BACKFILL"
	case classContract:
		return "CONTRACT"
	default:
		return "UNRECOGNISED"
	}
}

var (
	addColumnPattern    = regexp.MustCompile(`(?is)^ALTER\s+TABLE\s+([A-Za-z_][A-Za-z0-9_]*)\s+ADD\s+COLUMN\s+([A-Za-z_][A-Za-z0-9_]*)`)
	updateTargetPattern = regexp.MustCompile(`(?is)^UPDATE\s+([A-Za-z_][A-Za-z0-9_]*)\s+SET\s+([A-Za-z_][A-Za-z0-9_]*)`)
	blockCommentPattern = regexp.MustCompile(`/\*`)

	// The goose directives are spelled out because the checker has to find
	// them before anything else can be parsed.
	gooseUpDirective     = "-- +goose Up"
	gooseDownDirective   = "-- +goose Down"
	gooseBlockBegin      = "-- +goose StatementBegin"
	gooseBlockEnd        = "-- +goose StatementEnd"
	gooseDirectiveMarker = "-- +goose"
)

// migrationVerdict is what the checker found in one migration.
type migrationVerdict struct {
	violations []string
	classes    map[statementClass][]string
}

// checkMigration applies the ADR-0002 rules to one migration's text.
func checkMigration(name, body string) migrationVerdict {
	verdict := migrationVerdict{classes: map[statementClass][]string{}}

	// R1: both directions have to be there. A migration that cannot be
	// reversed structurally is a migration whose only rollback is a backup.
	if !strings.Contains(body, gooseUpDirective) {
		verdict.violations = append(verdict.violations, "缺少 "+gooseUpDirective)
	}
	if !strings.Contains(body, gooseDownDirective) {
		verdict.violations = append(verdict.violations, "缺少 "+gooseDownDirective)
	}
	if len(verdict.violations) > 0 {
		return verdict
	}

	// A block comment would need a stripper this checker does not have.
	// Refusing is better than stripping "--" out of text that is inside one.
	if blockCommentPattern.MatchString(body) {
		verdict.violations = append(verdict.violations,
			"含有 /* */ 块注释；本检查器不解析块注释，请改写为 -- 行注释后重新检查")
		return verdict
	}

	up := blockAfter(body, gooseUpDirective, gooseDownDirective)

	// Comments are removed before the body is split into statements, and that
	// order is load-bearing. A semicolon inside a comment -- as in "a trigger
	// body contains one; that is why it is wrapped" -- would otherwise cut the
	// comment in two, and the half after the semicolon carries no "--" to say
	// it is prose, so it is read as SQL. Both false positives this checker
	// produced while it was being written were exactly that.
	stripped, err := stripComments(collapseStatementBlocks(up))
	if err != nil {
		verdict.violations = append(verdict.violations, err.Error())
		return verdict
	}
	statements := splitStatements(stripped)

	// Everything additive is collected first, in its own pass. A migration is
	// free to drop an object before recreating it -- and 017 does exactly that
	// -- so judging a DROP against the CREATEs seen so far would report the
	// replacement idiom as a violation depending on the order the two
	// statements happen to be written in.
	type parsed struct {
		class statementClass
		text  string
	}
	parsedStatements := make([]parsed, 0, len(statements))
	createdKinds := map[string]bool{}
	introducedColumns := map[string]bool{}
	classes := make(map[statementClass][]string, len(statements))
	for _, statement := range statements {
		class, text := classifyStatement(statement)
		parsedStatements = append(parsedStatements, parsed{class: class, text: text})
		classes[class] = append(classes[class], text)
		if class != classAdd {
			continue
		}
		upper := strings.ToUpper(text)
		if strings.Contains(upper, "INDEX") {
			createdKinds["INDEX"] = true
		}
		if strings.Contains(upper, "TRIGGER") {
			createdKinds["TRIGGER"] = true
		}
		if strings.Contains(upper, "VIEW") {
			createdKinds["VIEW"] = true
		}
		if match := addColumnPattern.FindStringSubmatch(text); match != nil {
			introducedColumns[strings.ToLower(match[1])+"."+strings.ToLower(match[2])] = true
		}
	}
	verdict.classes = classes

	for _, entry := range parsedStatements {
		switch entry.class {
		case classUnrecognised:
			verdict.violations = append(verdict.violations,
				"检查器不认识的语句，请先补充规则再提交："+firstLine(entry.text))

		case classContract:
			// R2.
			verdict.violations = append(verdict.violations,
				"Up 里出现破坏性语句（"+firstLine(entry.text)+"）；破坏性动作必须放到后续发布的迁移里")

		case classReplaceDrop:
			kind := droppedKind(entry.text)
			if kind == "" {
				verdict.violations = append(verdict.violations,
					"检查器无法判断这条 DROP 删的是什么："+firstLine(entry.text))
				continue
			}
			if !createdKinds[kind] {
				// R3.
				verdict.violations = append(verdict.violations,
					"Up 里删了 "+kind+" 却没有在同一迁移里重建同类对象（"+firstLine(entry.text)+"）；没有配对的删除就是破坏性动作")
			}

		case classBackfill:
			// R4.
			if violation := backfillViolation(entry.text, introducedColumns); violation != "" {
				verdict.violations = append(verdict.violations, violation)
			}
		}
	}
	return verdict
}

// backfillViolation reports whether a data write touches a shape this same
// migration did not introduce.
//
// Only `UPDATE ... SET <column>` is judged. Inserting or deleting rows does
// not change the schema's shape -- an older binary reading the table simply
// finds a few more or fewer rows -- so it is not what R4 is about. What R4 is
// about is a migration rewriting the contents of a column that already meant
// something to a reader, which is where an upgrade stops being reversible.
//
// Reading old columns is also not judged: the danger expand-contract guards
// against is the new binary *writing* a shape the old binary cannot read, not
// the new binary reading what is already there.
func backfillViolation(text string, columns map[string]bool) string {
	match := updateTargetPattern.FindStringSubmatch(text)
	if match == nil {
		// Reachable only for INSERT, REPLACE and DELETE, which R4 does not
		// judge. A statement the classifier called a backfill that is none of
		// those would be a classifier bug, and classifyStatement is what
		// decides that -- so there is nothing to guess at here.
		return ""
	}
	target := strings.ToLower(match[1]) + "." + strings.ToLower(match[2])
	if !columns[target] {
		return "Up 里的回填写的是本迁移没有引入的列 " + target + "：" + firstLine(text)
	}
	return ""
}

// classifyStatement names the class of one statement.
//
// The class is decided by the statement's leading verb. That is why a goose
// StatementBegin block is collapsed to its first line before this runs: a
// trigger body is full of semicolons and UPDATEs, and classifying its contents
// would report every trigger as a backfill.
func classifyStatement(statement string) (statementClass, string) {
	text := strings.Join(strings.Fields(statement), " ")
	upper := strings.ToUpper(text)
	if text == "" {
		return classUnrecognised, ""
	}

	switch {
	case strings.HasPrefix(upper, "DROP TABLE"),
		strings.HasPrefix(upper, "DROP COLUMN"),
		strings.Contains(upper, "RENAME TO"),
		strings.HasPrefix(upper, "ALTER TABLE") && strings.Contains(upper, " DROP COLUMN"):
		return classContract, text

	case strings.HasPrefix(upper, "CREATE TABLE"),
		strings.HasPrefix(upper, "CREATE INDEX"),
		strings.HasPrefix(upper, "CREATE UNIQUE INDEX"),
		strings.HasPrefix(upper, "CREATE TRIGGER"),
		strings.HasPrefix(upper, "CREATE VIEW"),
		strings.HasPrefix(upper, "ALTER TABLE") && strings.Contains(upper, " ADD COLUMN"):
		return classAdd, text

	case strings.HasPrefix(upper, "DROP INDEX"),
		strings.HasPrefix(upper, "DROP TRIGGER"),
		strings.HasPrefix(upper, "DROP VIEW"):
		return classReplaceDrop, text

	case strings.HasPrefix(upper, "UPDATE"),
		strings.HasPrefix(upper, "INSERT"),
		strings.HasPrefix(upper, "REPLACE INTO"),
		strings.HasPrefix(upper, "DELETE FROM"):
		return classBackfill, text

	default:
		return classUnrecognised, text
	}
}

func droppedKind(text string) string {
	upper := strings.ToUpper(text)
	switch {
	case strings.HasPrefix(upper, "DROP INDEX"):
		return "INDEX"
	case strings.HasPrefix(upper, "DROP TRIGGER"):
		return "TRIGGER"
	case strings.HasPrefix(upper, "DROP VIEW"):
		return "VIEW"
	default:
		return ""
	}
}

// blockAfter returns the text between two directives.
func blockAfter(body, start, end string) string {
	index := strings.Index(body, start)
	if index < 0 {
		return ""
	}
	rest := body[index+len(start):]
	if stop := strings.Index(rest, end); stop >= 0 {
		return rest[:stop]
	}
	return rest
}

// collapseStatementBlocks replaces every goose StatementBegin/End region with
// its own first line of SQL followed by a semicolon.
//
// A statement block exists precisely because its contents contain semicolons
// goose would otherwise split on, and in this codebase that means a trigger
// body containing UPDATE statements. Keeping the whole block would make every
// trigger look like a backfill; keeping only the leading verb keeps the one
// statement it really is.
func collapseStatementBlocks(body string) string {
	lines := strings.Split(body, "\n")
	kept := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != gooseBlockBegin {
			kept = append(kept, lines[i])
			continue
		}
		i++
		for ; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == gooseBlockEnd {
				break
			}
			candidate := strings.TrimSpace(lines[i])
			if candidate == "" || strings.HasPrefix(candidate, gooseDirectiveMarker) || strings.HasPrefix(candidate, "--") {
				continue
			}
			kept = append(kept, candidate+";")
			break
		}
		// Skip to the end of the block; anything inside it is not a
		// statement of its own.
		for ; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == gooseBlockEnd {
				break
			}
		}
	}
	return strings.Join(kept, "\n")
}

// splitStatements splits a migration body on semicolons that are not inside a
// string literal. Its input must already have had its comments removed.
//
// It splits on semicolons only. An earlier version also broke on newlines,
// which looked harmless and was not: a CREATE TABLE written across several
// lines -- which is how every table in this project is written -- was cut into
// fragments, and the classifier then reported the column definitions as
// statements it did not recognise.
func splitStatements(body string) []string {
	var statements []string
	var current strings.Builder
	inQuote := false
	for i := 0; i < len(body); i++ {
		char := body[i]
		if char == '\'' {
			inQuote = !inQuote
			current.WriteByte(char)
			continue
		}
		if char == ';' && !inQuote {
			if strings.TrimSpace(current.String()) != "" {
				statements = append(statements, current.String())
			}
			current.Reset()
			continue
		}
		current.WriteByte(char)
	}
	if strings.TrimSpace(current.String()) != "" {
		statements = append(statements, current.String())
	}
	return statements
}

// stripComments removes -- line comments, ignoring any that appear inside a
// string literal.
//
// It refuses a line that ends inside an open string literal rather than
// guessing: this scans one line at a time, so a string that continues onto the
// next line would be misread and the comment scan would be wrong from there on.
// A migration using one is not something this checker can read, and saying so
// is the only honest answer.
func stripComments(body string) (string, error) {
	lines := strings.Split(body, "\n")
	kept := make([]string, 0, len(lines))
	for number, line := range lines {
		text, openQuote := lineWithoutComment(line)
		if openQuote {
			return "", fmt.Errorf("第 %d 行以未闭合的字符串字面量结束；本检查器按行去注释，读不了跨行字符串", number+1)
		}
		kept = append(kept, text)
	}
	return strings.Join(kept, "\n"), nil
}

// lineWithoutComment cuts a line at the first "--" that is not inside a string
// literal, and reports whether the line ends inside one.
func lineWithoutComment(line string) (string, bool) {
	inQuote := false
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '\'':
			inQuote = !inQuote
		case '-':
			if !inQuote && i+1 < len(line) && line[i+1] == '-' {
				// The rest of the line is a comment, so whatever quotes it
				// contains are prose and must not affect the quote state.
				return line[:i], false
			}
		}
	}
	return line, inQuote
}

func firstLine(text string) string {
	if index := strings.IndexByte(text, '\n'); index >= 0 {
		return text[:index]
	}
	if len(text) > 120 {
		return text[:120] + "…"
	}
	return text
}

// migrationSetNames lists the .sql files in a migration filesystem, sorted.
func migrationSetNames(fsys fs.FS) ([]string, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names, nil
}

// TestMigrationsFollowExpandContract applies ADR-0002 to every migration this
// project ships, in both migration sets.
func TestMigrationsFollowExpandContract(t *testing.T) {
	sets := []struct {
		name string
		fsys fs.FS
	}{
		{"control", goddiassets.Migrations()},
		{"dataplane", dataplane.Migrations()},
	}

	total := 0
	for _, set := range sets {
		names, err := migrationSetNames(set.fsys)
		if err != nil {
			t.Fatalf("listing %s migrations: %v", set.name, err)
		}
		if len(names) == 0 {
			t.Fatalf("the %s migration set is empty; this check would pass vacuously", set.name)
		}
		for _, name := range names {
			body, err := fs.ReadFile(set.fsys, name)
			if err != nil {
				t.Fatalf("reading %s/%s: %v", set.name, name, err)
			}
			total++
			verdict := checkMigration(name, string(body))
			for _, violation := range verdict.violations {
				t.Errorf("%s/%s：%s", set.name, name, violation)
			}
		}
	}
	t.Logf("检查了 %d 个迁移", total)
}

// TestTheContractCheckRejectsEachRuleBreakingMigration is what makes the check
// above worth anything.
//
// A rule that has never been observed to fail is a rule that is not known to
// be enforced. Each case below breaks exactly one rule and has to be refused.
func TestTheContractCheckRejectsEachRuleBreakingMigration(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		wantPhrase string
	}{
		{
			name:       "R1 缺少 Down",
			body:       "-- +goose Up\nCREATE TABLE t (id TEXT);\n",
			wantPhrase: "缺少 -- +goose Down",
		},
		{
			name:       "R1 缺少 Up",
			body:       "-- +goose Down\nDROP TABLE t;\n",
			wantPhrase: "缺少 -- +goose Up",
		},
		{
			name:       "R2 Up 里删列",
			body:       "-- +goose Up\nALTER TABLE t DROP COLUMN legacy;\n\n-- +goose Down\nALTER TABLE t ADD COLUMN legacy TEXT;\n",
			wantPhrase: "破坏性语句",
		},
		{
			name:       "R2 Up 里删表",
			body:       "-- +goose Up\nDROP TABLE old_thing;\n\n-- +goose Down\nCREATE TABLE old_thing (id TEXT);\n",
			wantPhrase: "破坏性语句",
		},
		{
			name:       "R3 删索引却不重建",
			body:       "-- +goose Up\nCREATE TABLE t (id TEXT);\nDROP INDEX idx_old;\n\n-- +goose Down\nDROP TABLE t;\n",
			wantPhrase: "却没有在同一迁移里重建",
		},
		{
			name:       "R4 回填写了本迁移没引入的列",
			body:       "-- +goose Up\nALTER TABLE t ADD COLUMN fresh TEXT NOT NULL DEFAULT '';\nUPDATE t SET legacy = 'x';\n\n-- +goose Down\nALTER TABLE t DROP COLUMN fresh;\n",
			wantPhrase: "本迁移没有引入的列 t.legacy",
		},
		{
			name:       "R4 同一张表上仍按列判定",
			body:       "-- +goose Up\nCREATE TABLE t (id TEXT, fresh TEXT);\nUPDATE t SET legacy = 'x';\n\n-- +goose Down\nDROP TABLE t;\n",
			wantPhrase: "本迁移没有引入的列 t.legacy",
		},
		{
			name:       "R3 删触发器却不重建",
			body:       "-- +goose Up\nCREATE TABLE t (id TEXT);\nDROP TRIGGER IF EXISTS trg_gone;\n\n-- +goose Down\nDROP TABLE t;\n",
			wantPhrase: "删了 TRIGGER 却没有在同一迁移里重建",
		},
		{
			name:       "注释里的分号不得把注释切成两半",
			body:       "-- +goose Up\n-- 一段说明，里面有一个分号; 分号后面的半句没有 -- 前缀\nPRAGMA journal_mode = WAL;\n\n-- +goose Down\nSELECT 1;\n",
			wantPhrase: "不认识的语句",
		},
		{
			name:       "检查器不认识的语句",
			body:       "-- +goose Up\nVACUUM;\n\n-- +goose Down\nSELECT 1;\n",
			wantPhrase: "不认识的语句",
		},
		{
			name:       "块注释无法解析",
			body:       "-- +goose Up\n/* 这块注释本检查器读不了 */\nCREATE TABLE t (id TEXT);\n\n-- +goose Down\nDROP TABLE t;\n",
			wantPhrase: "块注释",
		},
	}

	for _, entry := range cases {
		entry := entry
		t.Run(entry.name, func(t *testing.T) {
			verdict := checkMigration("synthetic.sql", entry.body)
			if len(verdict.violations) == 0 {
				t.Fatalf("这条迁移违反了规则却没有被拒绝；检查器对这个规则是空转的")
			}
			joined := strings.Join(verdict.violations, " ")
			if !strings.Contains(joined, entry.wantPhrase) {
				t.Errorf("拒绝理由与预期不符：\n got: %s\nwant: 含 %q", joined, entry.wantPhrase)
			}
		})
	}
}

// TestTheContractCheckAcceptsTheReplacementIdiom keeps the rules from being so
// strict that the two patterns this codebase relies on start failing.
func TestTheContractCheckAcceptsTheReplacementIdiom(t *testing.T) {
	body := `-- +goose Up
CREATE TABLE dataplane_revision (domain TEXT PRIMARY KEY, revision INTEGER NOT NULL DEFAULT 0);
INSERT OR IGNORE INTO dataplane_revision (domain, revision) VALUES ('dns', 0);

-- +goose StatementBegin
DROP TRIGGER IF EXISTS trg_old;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_old
AFTER INSERT ON dataplane_revision
BEGIN
    UPDATE dataplane_revision SET revision = revision + 1 WHERE domain = 'dns';
END;
-- +goose StatementEnd

DROP INDEX IF EXISTS idx_old;
CREATE UNIQUE INDEX idx_new ON dataplane_revision(domain);

-- +goose Down
DROP TRIGGER IF EXISTS trg_old;
DROP INDEX IF EXISTS idx_new;
DROP TABLE IF EXISTS dataplane_revision;
`
	verdict := checkMigration("replacement.sql", body)
	for _, violation := range verdict.violations {
		t.Errorf("这段合规的迁移被拒绝了：%s", violation)
	}
	// The trigger body's UPDATE must not have been read as a backfill: a
	// backfill on a table this migration did create would pass R4 anyway, so
	// the class itself is asserted.
	if len(verdict.classes[classBackfill]) != 1 {
		t.Errorf("backfill 类应当只有 INSERT 一条，实际 %v", verdict.classes[classBackfill])
	}
	if len(verdict.classes[classReplaceDrop]) != 2 {
		t.Errorf("REPLACE 类应当有两条 DROP（索引与触发器各一），实际 %v", verdict.classes[classReplaceDrop])
	}
}

// TestTheContractCheckDoesNotJudgeRowWrites records the boundary R4 draws.
//
// Inserting and deleting rows into a table an earlier migration created is
// accepted. That is deliberate and is the reason 022 may seed the counters 021
// introduced: rows are data, not shape, and an older binary reading the table
// simply finds a few more of them. A checker that refused this would be
// enforcing something nobody decided -- which is how a checker teaches people
// to work around it.
func TestTheContractCheckDoesNotJudgeRowWrites(t *testing.T) {
	body := `-- +goose Up
INSERT OR IGNORE INTO dataplane_revision (domain, revision) VALUES ('ddns', 0);
DELETE FROM dhcp_leases WHERE status = 'expired';

-- +goose Down
DELETE FROM dataplane_revision WHERE domain = 'ddns';
`
	verdict := checkMigration("row-writes.sql", body)
	for _, violation := range verdict.violations {
		t.Errorf("行写入被拒绝了：%s", violation)
	}
	// Two in the Up: the seeded row and the delete. The Down is not parsed by
	// the checker -- its job is to restore a shape, and destroying rows on the
	// way down is what a rollback is for.
	if len(verdict.classes[classBackfill]) != 2 {
		t.Errorf("Up 里的两条行写入都应当归入 BACKFILL 类，实际 %v", verdict.classes[classBackfill])
	}
}

// TestCommentsAreRemovedBeforeStatementsAreSplit pins the order that made two
// real migrations be misread.
//
// A comment sentence containing a semicolon used to be cut in two, and the
// half after the semicolon has no "--" on it to say it is prose -- so it was
// classified as SQL and reported as a statement the checker did not
// recognise. Both 024 and the data plane's 001 did this.
func TestCommentsAreRemovedBeforeStatementsAreSplit(t *testing.T) {
	body := `-- +goose Up
-- 一句话里有分号; 后面的半句不得被当成 SQL
CREATE TABLE t (id TEXT);

-- +goose Down
DROP TABLE t;
`
	verdict := checkMigration("comment-semicolon.sql", body)
	for _, violation := range verdict.violations {
		t.Errorf("注释里的分号造成了误判：%s", violation)
	}
	if len(verdict.classes[classAdd]) != 1 {
		t.Errorf("Up 里应当只解析出一条 ADD 语句，实际 %v", verdict.classes[classAdd])
	}
	if len(verdict.classes[classUnrecognised]) != 0 {
		t.Errorf("注释不应产生语句，实际 %v", verdict.classes[classUnrecognised])
	}
}

// TestAnUnreadableConstructIsRefusedRatherThanSkipped keeps the checker
// fail-closed about the syntax it cannot parse.
func TestAnUnreadableConstructIsRefusedRatherThanSkipped(t *testing.T) {
	body := `-- +goose Up
CREATE TABLE t (
    id TEXT DEFAULT 'a
b'
);

-- +goose Down
DROP TABLE t;
`
	verdict := checkMigration("multiline-string.sql", body)
	if len(verdict.violations) == 0 {
		t.Fatal("跨行字符串没有被拒绝；按行去注释时它是错的，而错得看不出来")
	}
	if !strings.Contains(strings.Join(verdict.violations, " "), "跨行字符串") {
		t.Errorf("拒绝理由应当说明是跨行字符串：%v", verdict.violations)
	}
}
