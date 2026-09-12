package api

// The console and the API guard the same resources through two mechanisms that
// no compiler can connect: the console reads `meta.permission` out of its route
// table (web-admin/build/plugins/router.ts) and the API mounts
// rbac.RequirePermission on each route. They are edited in different languages,
// in different directories, by different changes -- and a rename on one side
// produces no error anywhere. The console simply hides a page whose holder is
// allowed to use it, or shows a page whose permission no route consults.
//
// These tests read the three declarations from disk and compare them. Comparing
// written text rather than running systems is deliberate: the console route
// table has no Go representation to exercise, and the question is exactly
// whether the two written declarations agree.
//
// Every extraction below is paired with a count of the raw marker it parses.
// An extraction that silently finds nothing would make its comparison pass
// vacuously, which is the failure mode these tests exist to prevent.

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// A permission is the pair, never the resource alone: `ipam:read` and
// `ipam:write` are different grants and a comparison that ignored the action
// would call a read-only console page "covered" by a write-only route.
type permPair struct {
	resource string
	action   string
}

func (p permPair) String() string { return p.resource + ":" + p.action }

func sortedPairs(set map[permPair]bool) []string {
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p.String())
	}
	sort.Strings(out)

	return out
}

// difference returns the members of a that are not in b, sorted, for a message
// that names the offending pairs rather than just their count.
func difference(a, b map[permPair]bool) []string {
	missing := map[permPair]bool{}
	for p := range a {
		if !b[p] {
			missing[p] = true
		}
	}

	return sortedPairs(missing)
}

var (
	// internal/api/router.go: rbac.RequirePermission(rbacMgr, "dns", "read")
	routeRequireRE = regexp.MustCompile(`RequirePermission\(\s*rbacMgr\s*,\s*"([^"]+)"\s*,\s*"([^"]+)"\s*\)`)
	routeMarkerRE  = regexp.MustCompile(`RequirePermission\(`)

	// web-admin/build/plugins/router.ts: { "resource": "dns", "action": "read" }
	consolePairRE     = regexp.MustCompile(`"resource"\s*:\s*"([^"]+)"\s*,\s*"action"\s*:\s*"([^"]+)"`)
	consoleResourceRE = regexp.MustCompile(`"resource"`)
	consoleActionRE   = regexp.MustCompile(`"action"`)

	// internal/rbac/rbac.go: {Resource: "dns", Action: "read"}
	catalogEntryRE = regexp.MustCompile(`\{Resource:\s*"([^"]+)",\s*Action:\s*"([^"]+)"`)
	catalogMarker  = regexp.MustCompile(`\{Resource:`)

	roleNameRE = regexp.MustCompile(`Name:\s*"([^"]+)"`)

	// A quoted role name on a line that also decides something. The word
	// boundary matters: `roleAdmin: 'admin'` is data, `userRole === 'admin'`
	// is a decision.
	roleWordRE     = regexp.MustCompile(`(?i)\broles?\b`)
	roleDecisionRE = regexp.MustCompile(`===|!==|==|!=|\.includes\(|\.indexOf\(|\.some\(|\.filter\(|\.has\(`)
)

const (
	// Floors, not expectations: these are here to fail loudly when an
	// extraction silently stops finding anything. They are deliberately well
	// below today's counts so ordinary growth does not trip them.
	minRoutePermissions   = 20
	minConsolePermissions = 8
	minCatalogPermissions = 20
	minRoleGrants         = 15
	minRoleNames          = 3
	minConsoleFiles       = 50
)

// repoText reads a path relative to this package. A missing file is a failure,
// not a skip: a moved or renamed file must not turn a comparison into a pass.
func repoText(t *testing.T, rel string) string {
	t.Helper()

	raw, err := os.ReadFile(rel)
	if err != nil {
		t.Fatalf("read %s: %v -- this test compares the console's route table with the API's guards and needs both on disk; a file that moved must break this, not silence it", rel, err)
	}

	return string(raw)
}

// consoleText reads a path relative to web-admin. The console is not part of
// the Go module, so a tree without it (a module cache, a docs-only checkout)
// genuinely has nothing to compare and is skipped -- but a web-admin that
// exists with the file missing is a rename, and a rename must fail.
func consoleText(t *testing.T, rel string) string {
	t.Helper()

	web := filepath.Join("..", "..", "web-admin")
	if _, err := os.Stat(web); errors.Is(err, os.ErrNotExist) {
		t.Skipf("%s is not in this tree; there is no console route table to compare against", web)
	}

	p := filepath.Join(web, rel)
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v -- the console is present but the declaration is not where it was expected; a rename must break this, not silence it", p, err)
	}

	return string(raw)
}

// extractPairs pulls permission pairs out of src and checks the caller's
// accounting: markerCount is how many times the raw marker occurs. If the two
// disagree, the source was reshaped in a way the pattern no longer reads, and
// comparing the truncated result would quietly under-check.
func extractPairs(t *testing.T, src, what string, pairRE, markerRE *regexp.Regexp, markerCount int) map[permPair]bool {
	t.Helper()

	m := pairRE.FindAllStringSubmatch(src, -1)
	if len(m) != markerCount {
		t.Fatalf("%s: parsed %d permission pair(s) but found %d occurrence(s) of %s; the declaration was reshaped and this extraction no longer reads all of it",
			what, len(m), markerCount, markerRE.String())
	}

	set := map[permPair]bool{}
	for _, g := range m {
		set[permPair{resource: g[1], action: g[2]}] = true
	}

	return set
}

func routePermissions(t *testing.T) map[permPair]bool {
	t.Helper()

	src := repoText(t, "router.go")
	set := extractPairs(t, src, "internal/api/router.go", routeRequireRE, routeMarkerRE, len(routeMarkerRE.FindAllString(src, -1)))
	if len(set) < minRoutePermissions {
		t.Fatalf("internal/api/router.go declares only %d distinct permission pair(s), below the floor of %d; the extraction is not reading the routes any more", len(set), minRoutePermissions)
	}

	return set
}

func consolePermissions(t *testing.T) map[permPair]bool {
	t.Helper()

	src := consoleText(t, filepath.Join("build", "plugins", "router.ts"))
	resources := len(consoleResourceRE.FindAllString(src, -1))
	actions := len(consoleActionRE.FindAllString(src, -1))
	pairs := len(consolePairRE.FindAllStringSubmatch(src, -1))
	if resources != actions || resources != pairs {
		t.Fatalf("the console route table has %d \"resource\"/%d \"action\" key(s) but only %d adjacent pair(s); the permission block was reshaped and this extraction no longer reads all of it", resources, actions, pairs)
	}

	set := extractPairs(t, src, "web-admin/build/plugins/router.ts", consolePairRE, consoleResourceRE, resources)
	if len(set) < minConsolePermissions {
		t.Fatalf("the console route table declares only %d distinct permission pair(s), below the floor of %d; either the console stopped gating its routes or the extraction stopped reading them", len(set), minConsolePermissions)
	}

	return set
}

// rbacBlock returns the body of a top-level var declaration: everything after
// header -- and after opener, for a declaration whose type is an anonymous
// struct -- up to the next closing brace in column zero. Slicing by brace
// nesting would be fragile; the file is gofmt'd, so a column-zero brace ends
// the declaration and the indented braces inside it do not match.
func rbacBlock(t *testing.T, src, header, opener string) string {
	t.Helper()

	i := strings.Index(src, header)
	if i < 0 {
		t.Fatalf("internal/rbac/rbac.go no longer contains %q; this test reads the permission catalog and the role grants out of that declaration", header)
	}

	rest := src[i+len(header):]
	if opener != "" {
		k := strings.Index(rest, opener)
		if k < 0 {
			t.Fatalf("internal/rbac/rbac.go: %q is not followed by %q; the declaration was reshaped and this extraction no longer reads its body", header, opener)
		}
		rest = rest[k+len(opener):]
	}

	j := strings.Index(rest, "\n}")
	if j < 0 {
		t.Fatalf("internal/rbac/rbac.go: %q has no closing brace in column zero", header)
	}

	return rest[:j]
}

func permissionCatalog(t *testing.T) map[permPair]bool {
	t.Helper()

	block := rbacBlock(t, repoText(t, filepath.Join("..", "rbac", "rbac.go")),
		"var PredefinedPermissions = []Permission{", "")
	set := extractPairs(t, block, "rbac.PredefinedPermissions", catalogEntryRE, catalogMarker, len(catalogMarker.FindAllString(block, -1)))
	if len(set) < minCatalogPermissions {
		t.Fatalf("rbac.PredefinedPermissions declares only %d permission(s), below the floor of %d", len(set), minCatalogPermissions)
	}

	return set
}

// The roles declaration has an anonymous struct type, so the body starts after
// its field list and the literal's opening brace, not right after the header.
const rolesOpener = "\n}{\n"

func roleGrants(t *testing.T) map[permPair]bool {
	t.Helper()

	block := rbacBlock(t, repoText(t, filepath.Join("..", "rbac", "rbac.go")), "var PredefinedRoles = []struct", rolesOpener)
	set := extractPairs(t, block, "rbac.PredefinedRoles", catalogEntryRE, catalogMarker, len(catalogMarker.FindAllString(block, -1)))
	if len(set) < minRoleGrants {
		t.Fatalf("rbac.PredefinedRoles grants only %d distinct permission(s), below the floor of %d; the roles were emptied or the block moved", len(set), minRoleGrants)
	}

	return set
}

// TestEveryConsoleRoutePermissionIsCheckedByAnAPIRoute pins the direction that
// hides pages: the console denies navigation on a permission the API never
// requires. The holder is then refused a page whose every request under it
// would succeed -- a console that under-serves a role the operator believes
// they granted.
func TestEveryConsoleRoutePermissionIsCheckedByAnAPIRoute(t *testing.T) {
	console := consolePermissions(t)
	routes := routePermissions(t)

	if missing := difference(console, routes); len(missing) > 0 {
		t.Errorf("the console gates %v but no route in internal/api/router.go requires %v; hide a page behind a permission the API does not check and the page disappears for holders the API would serve",
			missing, missing)
	}
}

// TestTheAPINeverGatesOnAPermissionNobodyCanHold pins the other direction,
// which is an outage. Routes are guarded against the catalog: permissions are
// granted to roles by id from that catalog, so a gate on a permission outside
// it is a gate no role can ever satisfy -- the route answers 403 to everyone,
// including admin, and looks like a policy rather than a typo.
func TestTheAPINeverGatesOnAPermissionNobodyCanHold(t *testing.T) {
	routes := routePermissions(t)
	catalog := permissionCatalog(t)

	if missing := difference(routes, catalog); len(missing) > 0 {
		t.Errorf("internal/api/router.go requires %v but rbac.PredefinedPermissions does not declare %v; no role can hold an undeclared permission, so every request under those routes is refused for everyone",
			missing, missing)
	}
}

// TestThePermissionCatalogHasNoOrphans pins the mirror of the lockout: a
// permission that exists and can be granted but that no route consults. An
// administrator hands out a grant, the role matrix reads as if access were
// given, and nothing changes. This is the same defect shape as a probe that
// reports healthy because it never checks.
func TestThePermissionCatalogHasNoOrphans(t *testing.T) {
	catalog := permissionCatalog(t)
	routes := routePermissions(t)

	if orphans := difference(catalog, routes); len(orphans) > 0 {
		t.Errorf("rbac.PredefinedPermissions declares %v but no route requires %v; granting an orphan permission changes nothing while reading as though it did",
			orphans, orphans)
	}
}

// TestEveryGrantedRolePermissionIsInTheCatalog keeps the built-in roles honest.
// A role is a list of resource/action pairs written by hand, and a typo there
// has no error path: the role simply grants less than its description says, and
// the operator finds out by being refused.
func TestEveryGrantedRolePermissionIsInTheCatalog(t *testing.T) {
	grants := roleGrants(t)
	catalog := permissionCatalog(t)

	if stray := difference(grants, catalog); len(stray) > 0 {
		t.Errorf("rbac.PredefinedRoles grants %v which rbac.PredefinedPermissions does not declare; a role holding an undeclared permission grants nothing while its description claims otherwise",
			stray)
	}
}

// TestTheConsoleRouteTableCarriesNoRoleGate keeps the console down to one
// authorization axis. The scaffold this console was built on gates routes on
// `meta.roles` as well, and both readers are still live in src/ -- but nothing
// writes `roles` into the route table, so the role axis admits everything today
// and the permission axis is the only one that decides. Adding a `roles` key
// silently switches the second axis on, and the two can disagree: a role that
// holds the permission is refused the page by its name, or the reverse.
func TestTheConsoleRouteTableCarriesNoRoleGate(t *testing.T) {
	src := consoleText(t, filepath.Join("build", "plugins", "router.ts"))

	if !strings.Contains(src, "const ROUTE_META") {
		t.Fatalf("web-admin/build/plugins/router.ts does not define ROUTE_META; this test reads the console's route table out of that declaration and will not pass by finding nothing")
	}

	if strings.Contains(src, `"roles"`) {
		t.Error("the console route table declares \"roles\"; that switches on the scaffold's role-based gate alongside the permission gate, and two gates that can disagree are worse than one that is read")
	}
}

// TestTheConsoleNeverDecidesFromARoleName is the guard against a third axis
// appearing in component code, where no declaration lists it. A decision that
// compares a role to a literal is invisible to every test in this file and to
// the API: the console shows or hides something on a name the backend never
// consulted.
func TestTheConsoleNeverDecidesFromARoleName(t *testing.T) {
	names := roleNames(t)

	root := filepath.Join("..", "..", "web-admin", "src")
	if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
		t.Skipf("%s is not in this tree; there is no console source to scan", root)
	}

	quoted := make([]*regexp.Regexp, 0, len(names))
	for _, n := range names {
		quoted = append(quoted, regexp.MustCompile(`["']`+regexp.QuoteMeta(n)+`["']`))
	}

	scanned := 0
	var offences []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "node_modules" {
				return fs.SkipDir
			}

			return nil
		}
		if ext := filepath.Ext(path); ext != ".ts" && ext != ".vue" {
			return nil
		}

		scanned++
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(raw), "\n") {
			if !roleWordRE.MatchString(line) || !roleDecisionRE.MatchString(line) {
				continue
			}
			for _, re := range quoted {
				if re.MatchString(line) {
					offences = append(offences, fmt.Sprintf("%s:%d: %s", path, i+1, strings.TrimSpace(line)))
					break
				}
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if scanned < minConsoleFiles {
		t.Fatalf("scanned only %d source file(s) under %s, below the floor of %d; a wrong directory would make this check pass by finding nothing", scanned, root, minConsoleFiles)
	}

	for _, o := range offences {
		t.Errorf("the console decides from a role name: %s -- gate on meta.permission instead, so the console and the API read the same grant", o)
	}
}

func roleNames(t *testing.T) []string {
	t.Helper()

	block := rbacBlock(t, repoText(t, filepath.Join("..", "rbac", "rbac.go")), "var PredefinedRoles = []struct", rolesOpener)
	names := map[string]bool{}
	for _, m := range roleNameRE.FindAllStringSubmatch(block, -1) {
		names[m[1]] = true
	}
	if len(names) < minRoleNames {
		t.Fatalf("found %d role name(s) in rbac.PredefinedRoles, below the floor of %d; the scan above would look for nothing", len(names), minRoleNames)
	}

	out := make([]string, 0, len(names))
	for n := range names {
		out = append(out, n)
	}
	sort.Strings(out)

	return out
}

// TestTheConsoleGateIsInstalled closes the loop the other tests open. Every
// assertion above is about what the console declares; none of it takes effect
// unless the guard that reads `meta.permission` is registered. A guard that
// exists but is never installed denies nothing, and the console is open while
// every declaration still reads correctly.
//
// Comments are stripped first. A commented-out registration still contains the
// text of the call, and a plain substring search would read it as installed --
// which is exactly the shape of the defect this test is here for.
func TestTheConsoleGateIsInstalled(t *testing.T) {
	index := codeOnly(consoleText(t, filepath.Join("src", "router", "guard", "index.ts")))
	if !strings.Contains(index, "createPermissionGuard(router)") {
		t.Error("web-admin/src/router/guard/index.ts does not register createPermissionGuard; the route table's meta.permission is then read by nobody and the console has no client-side gate")
	}

	guard := codeOnly(consoleText(t, filepath.Join("src", "router", "guard", "permission.ts")))
	if !strings.Contains(guard, "hasPermission(") {
		t.Error("web-admin/src/router/guard/permission.ts no longer decides through hasPermission; the permission guard is not the permission guard any more")
	}
	if !strings.Contains(guard, "meta.permission") {
		t.Error("web-admin/src/router/guard/permission.ts no longer reads meta.permission; it is deciding on something other than the declaration this file's other tests compare")
	}
}

var blockCommentRE = regexp.MustCompile(`(?s)/\*.*?\*/`)

// codeOnly drops comments. It is deliberately crude -- it is only used to keep
// commented-out code from satisfying a substring search, and over-reporting a
// missing call is the safe direction.
func codeOnly(src string) string {
	src = blockCommentRE.ReplaceAllString(src, "")

	var kept []string
	for _, line := range strings.Split(src, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		kept = append(kept, line)
	}

	return strings.Join(kept, "\n")
}
