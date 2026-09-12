package metrics

// Guards on the metric registry itself.
//
// A metric with no writer is not a harmless placeholder. It is exported at a
// steady zero, and a steady zero is indistinguishable from a measurement of
// nothing happening: a dashboard reading goddi_dns_clients_total could not tell
// "no clients" from "nobody ever wired this up". Both of the metrics this test
// was written against spent their whole life in that state -- registered,
// documented in the PRD, and never once written.
//
// The checks below are source-level because that is where the mistake is made.
// They parse the declarations rather than keeping a list, so adding a metric
// does not mean editing this file -- only wiring it up does.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var (
	// collectorDecl matches `Symbol = prometheus.NewGauge(prometheus.GaugeOpts{ ... Name: "goddi_x" ...`.
	// The non-greedy body keeps one declaration's Name from matching another's.
	//
	// The name class includes uppercase on purpose. Restricted to [a-z0-9_] the
	// pattern stops at the first capital and the trailing boundary then fails,
	// so a metric named with a capital -- or a typo that introduced one -- would
	// be invisible to every check built on this. A name that cannot be seen
	// cannot be checked.
	collectorDecl = regexp.MustCompile(`(?m)^[ \t]*(\w+)[ \t]*=[ \t]*prometheus\.New\w+\(prometheus\.\w+Opts\{[^}]*?Name:[ \t]*"(goddi_[A-Za-z0-9_]+)"`)

	// registerBlock captures the argument list of prometheus.MustRegister(...).
	registerBlock = regexp.MustCompile(`(?s)prometheus\.MustRegister\((.*?)\n\t+\)`)

	// registerEntry matches one symbol per line inside that list.
	registerEntry = regexp.MustCompile(`(?m)^[ \t]*([A-Z]\w+),?[ \t]*$`)
)

// repoRoot is the module root, resolved from this package's directory.
const repoRoot = "../.."

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// declaredMetrics maps each collector symbol to the metric name it exports.
func declaredMetrics(t *testing.T) map[string]string {
	t.Helper()
	src := mustRead(t, "metrics.go")
	out := make(map[string]string)
	for _, m := range collectorDecl.FindAllStringSubmatch(src, -1) {
		out[m[1]] = m[2]
	}
	if len(out) == 0 {
		t.Fatal("no collector declarations were parsed; the pattern no longer matches the source")
	}
	return out
}

func registeredSymbols(t *testing.T) map[string]bool {
	t.Helper()
	src := mustRead(t, "metrics.go")
	block := registerBlock.FindStringSubmatch(src)
	if block == nil {
		t.Fatal("prometheus.MustRegister(...) was not found; the pattern no longer matches the source")
	}
	out := make(map[string]bool)
	for _, m := range registerEntry.FindAllStringSubmatch(block[1], -1) {
		out[m[1]] = true
	}
	if len(out) == 0 {
		t.Fatal("the registration list parsed as empty; the pattern no longer matches the source")
	}
	return out
}

// TestEveryDeclaredMetricIsRegistered is the cheap half: a collector that is
// declared and never registered produces no series at all, so a dashboard or
// alert written against it silently has nothing to read.
func TestEveryDeclaredMetricIsRegistered(t *testing.T) {
	declared := declaredMetrics(t)
	registered := registeredSymbols(t)

	var missing []string
	for symbol := range declared {
		if !registered[symbol] {
			missing = append(missing, symbol)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("declared but never registered: %s", strings.Join(missing, ", "))
	}

	var unknown []string
	for symbol := range registered {
		if _, ok := declared[symbol]; !ok {
			unknown = append(unknown, symbol)
		}
	}
	sort.Strings(unknown)
	if len(unknown) > 0 {
		t.Errorf("registered but not declared here: %s", strings.Join(unknown, ", "))
	}
}

// TestEveryRegisteredMetricHasAWriter is the half that matters.
//
// The rule is a count, not an analysis: a symbol must appear somewhere other
// than its own declaration and its entry in the registration list. That is
// enough to catch the failure being guarded against -- a collector nobody ever
// touches appears exactly twice and no more -- but it is not a proof that the
// writer is correct or reachable. A symbol mentioned only in a comment would
// pass. Writes made by a provider registered from another package show up here
// as references, which is intended: this test asks whether something in the
// tree names the metric, not whether the value is right.
func TestEveryRegisteredMetricHasAWriter(t *testing.T) {
	declared := declaredMetrics(t)
	registered := registeredSymbols(t)

	// Every non-test Go file outside this package, plus this package's own
	// non-test files. Test files are excluded on purpose: a test that reads a
	// metric should not be able to make a dead metric look alive.
	sources := make(map[string]string)
	metricsGo := mustRead(t, "metrics.go")
	for _, dir := range []string{filepath.Join(repoRoot, "internal"), filepath.Join(repoRoot, "cmd")} {
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			if strings.HasSuffix(filepath.ToSlash(path), "/metrics/metrics.go") {
				return nil
			}
			if b, err := os.ReadFile(path); err == nil {
				sources[path] = string(b)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
	if len(sources) < 10 {
		t.Fatalf("only %d source files were collected; the walk is not finding the tree", len(sources))
	}

	// Every reference in this package's own file: the declaration and the
	// registration entry are the two that do not count.
	selfRefs := make(map[string]int)
	for _, m := range regexp.MustCompile(`\b[A-Z]\w+\b`).FindAllString(metricsGo, -1) {
		selfRefs[m]++
	}

	var dead []string
	for symbol, name := range declared {
		if !registered[symbol] {
			continue // reported by the test above
		}
		// Two of the symbol's occurrences in this file are the declaration and
		// the registration. Anything beyond that is a write or a read.
		beyond := selfRefs[symbol] - 2
		for _, src := range sources {
			beyond += len(regexp.MustCompile(`\b`+regexp.QuoteMeta(symbol)+`\b`).FindAllStringIndex(src, -1))
		}
		if beyond <= 0 {
			dead = append(dead, symbol+" ("+name+")")
		}
	}
	sort.Strings(dead)
	if len(dead) > 0 {
		t.Errorf("registered with no writer, so each reports a steady zero: %s", strings.Join(dead, ", "))
	}
}

// TestEveryAlertedMetricExists lives in alert_rules_test.go, with the rest of
// the checks on the shipped rules.
