package metrics

// Checks on the shipped alert rules.
//
// A rule can be wrong in four ways that all look like working monitoring from
// the outside, because a rule that never fires is indistinguishable from a
// system with nothing wrong:
//
//  1. it names a metric that is not registered -- renamed, removed, or never
//     existed;
//  2. it names a metric that is registered but has no writer, so the series is
//     a constant;
//  3. it is malformed, so Prometheus refuses to load the file and every rule in
//     it is silently absent;
//  4. it is incomplete -- no severity, no summary -- so when it does fire
//     nobody can tell what it means.
//
// The first two are covered against the registry; the last two here. None of
// this proves an expression is *correct*, only that it is well formed and
// referable, which is the part a build can decide.

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const alertRulesPath = "../../docs/prometheus-alerts.yml"

// metricNamePattern finds every metric name in an expression.
//
// The class includes uppercase and the boundary is required, which together are
// what make a typo visible. Narrower -- [a-z0-9_] -- the pattern stops at the
// first capital and the trailing boundary then fails, so a name misspelled with
// a capital letter is not reported as unknown; it is not reported at all, and
// the check silently loses the one case it exists to catch.
var metricNamePattern = regexp.MustCompile(`\bgoddi_[A-Za-z0-9_]+\b`)

type alertRule struct {
	Alert       string            `yaml:"alert"`
	Expr        string            `yaml:"expr"`
	For         string            `yaml:"for"`
	Labels      map[string]string `yaml:"labels"`
	Annotations map[string]string `yaml:"annotations"`
}

type alertGroup struct {
	Name  string      `yaml:"name"`
	Rules []alertRule `yaml:"rules"`
}

type alertFile struct {
	Groups []alertGroup `yaml:"groups"`
}

func loadAlertRules(t *testing.T) alertFile {
	t.Helper()
	raw, err := os.ReadFile(alertRulesPath)
	if err != nil {
		t.Skipf("%s is not in this tree", alertRulesPath)
	}
	var doc alertFile
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("%s does not parse: %v", alertRulesPath, err)
	}
	if len(doc.Groups) == 0 {
		t.Fatalf("%s has no groups; the file would load and alert on nothing", alertRulesPath)
	}
	return doc
}

func eachRule(t *testing.T, fn func(t *testing.T, rule alertRule)) {
	t.Helper()
	for _, g := range loadAlertRules(t).Groups {
		if g.Name == "" {
			t.Errorf("%s has a group without a name", alertRulesPath)
		}
		if len(g.Rules) == 0 {
			t.Errorf("group %s has no rules", g.Name)
		}
		for _, r := range g.Rules {
			t.Run(g.Name+"/"+r.Alert, func(t *testing.T) { fn(t, r) })
		}
	}
}

// TestEveryAlertRuleIsComplete covers the failure that only shows up at three
// in the morning: a rule that fires with no severity (so no routing) or no
// summary (so no explanation).
func TestEveryAlertRuleIsComplete(t *testing.T) {
	eachRule(t, func(t *testing.T, rule alertRule) {
		if strings.TrimSpace(rule.Alert) == "" {
			t.Error("rule has no alert name")
		}
		if strings.TrimSpace(rule.Expr) == "" {
			t.Error("rule has no expression")
		}
		if strings.TrimSpace(rule.For) == "" {
			t.Error("rule has no 'for'; without it a single scrape's outlier fires it")
		}
		switch rule.Labels["severity"] {
		case "critical", "warning", "info":
		default:
			t.Errorf("severity = %q, want critical, warning or info", rule.Labels["severity"])
		}
		if strings.TrimSpace(rule.Annotations["summary"]) == "" {
			t.Error("rule has no summary annotation")
		}
	})
}

// TestAlertNamesAreUnique catches the copy-paste that makes one rule mask
// another: two rules sharing a name means one alert state, and the second
// rule's condition is never visible.
func TestAlertNamesAreUnique(t *testing.T) {
	seen := make(map[string]string)
	for _, g := range loadAlertRules(t).Groups {
		for _, r := range g.Rules {
			if where, ok := seen[r.Alert]; ok {
				t.Errorf("alert %s is declared in both %s and %s", r.Alert, where, g.Name)
			}
			seen[r.Alert] = g.Name
		}
	}
}

// TestAlertExpressionsAreWellFormed is a typo check, not a PromQL parser. It
// catches a truncated expression -- an unclosed parenthesis or bracket, a
// stray quote -- which is the shape a hand edit leaves behind. Everything past
// that needs promtool, which is not part of this build.
func TestAlertExpressionsAreWellFormed(t *testing.T) {
	eachRule(t, func(t *testing.T, rule alertRule) {
		var parens, brackets, braces int
		inString := false
		for _, r := range rule.Expr {
			switch {
			case r == '"':
				inString = !inString
			case inString:
			case r == '(':
				parens++
			case r == ')':
				parens--
			case r == '[':
				brackets++
			case r == ']':
				brackets--
			case r == '{':
				braces++
			case r == '}':
				braces--
			}
			if parens < 0 || brackets < 0 || braces < 0 {
				t.Fatalf("unbalanced %q", rule.Expr)
			}
		}
		if inString {
			t.Errorf("unterminated string in %q", rule.Expr)
		}
		if parens != 0 || brackets != 0 || braces != 0 {
			t.Errorf("unbalanced delimiters in %q (parens %d, brackets %d, braces %d)",
				rule.Expr, parens, brackets, braces)
		}
	})
}

// TestEveryAlertedMetricExists is the check that matters most. A rule written
// against a metric that was renamed or removed does not fail to load; it simply
// never fires. That is how a database-error alert sat in this file for a
// release while the counter it watched had no producer at all.
//
// Only the expressions are searched, not the whole document: the file's own
// comments name metrics on purpose, to explain what each rule is for.
func TestEveryAlertedMetricExists(t *testing.T) {
	known := make(map[string]bool)
	registered := registeredSymbols(t)
	for symbol, name := range declaredMetrics(t) {
		// Only metrics that are actually registered count as referable. One
		// that is declared and never registered produces no series at all, so
		// a rule against it is as dead as a rule against a name that does not
		// exist. (The registry test reports that case separately; this check
		// simply refuses to be satisfied by it.)
		if registered[symbol] {
			known[name] = true
		}
	}
	if len(known) == 0 {
		t.Fatal("no registered metrics were found; the check would pass vacuously")
	}

	names := metricNamePattern
	var unknown []string
	eachRule(t, func(t *testing.T, rule alertRule) {
		for _, name := range names.FindAllString(rule.Expr, -1) {
			if !known[name] {
				unknown = append(unknown, name+" (in "+rule.Alert+")")
			}
		}
	})

	if len(unknown) > 0 {
		sort.Strings(unknown)
		t.Errorf("%s alerts on metrics that are not registered: %s",
			alertRulesPath, strings.Join(unknown, ", "))
	}
}

// TestTheKeySignalsAreAlertedOn is a curated list, not a completeness proof.
// Each entry is a signal that was added because its absence had already caused
// a blind spot; the test exists so a later edit that drops a rule has to do so
// deliberately.
func TestTheKeySignalsAreAlertedOn(t *testing.T) {
	alerted := make(map[string]bool)
	eachRule(t, func(t *testing.T, rule alertRule) {
		for _, name := range metricNamePattern.FindAllString(rule.Expr, -1) {
			alerted[name] = true
		}
	})

	for _, name := range []string{
		"goddi_dataplane_ready",                       // the graded readiness level
		"goddi_dataplane_pending_changes",             // a backlog nobody could otherwise see
		"goddi_dhcp_scope_usage_ratio",                // pool exhaustion
		"goddi_dhcp_ha_redundant",                     // a pair that quietly stopped being a pair
		"goddi_dhcp_ha_promising",                     // a node that stopped handing out addresses
		"goddi_db_errors_total",                       // the counter that had no producer
		"goddi_dns_secondary_zone_sync_failures",      // secondary refresh health
		"goddi_backup_last_success_timestamp_seconds", // backup age
		"goddi_dns_servfail_total",                    // resolution quality
	} {
		if !alerted[name] {
			t.Errorf("no rule references %s", name)
		}
	}
}
