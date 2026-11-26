// Static contract guard: frontend navigation, i18n route keys, generated
// routes, PATHS and the OpenAPI catalogue must stay in sync.
//
// A new view without a translation shows a raw "route.foo" key in the
// sidebar; a nav item without a PATHS entry silently does nothing when
// clicked; a frontend service call to an unregistered path is a runtime 404.
// This runs without a browser (see run.sh).
const fs = require('fs');
const path = require('path');

const root = path.join(__dirname, '..', '..');
const navPath = path.join(root, 'web-admin/src/components/starter/nav.ts');
const headerPath = path.join(root, 'web-admin/src/components/starter/Header.vue');
const layoutPath = path.join(root, 'web-admin/src/layouts/tdesign-layout/index.vue');
const routerMetaPath = path.join(root, 'web-admin/build/plugins/router.ts');
const generatedRoutesPath = path.join(root, 'web-admin/src/router/elegant/routes.ts');
const openapiPath = path.join(root, 'internal/api/openapi.go');
const rbacPath = path.join(root, 'internal/rbac/rbac.go');
const serviceDir = path.join(root, 'web-admin/src/service/api/goddi');
const langs = ['zh-cn.ts', 'en-us.ts'];

const read = p => fs.readFileSync(p, 'utf8');
let failures = 0;
const fail = msg => { console.log('FAIL | ' + msg); failures++; };

const layout = read(layoutPath);
const nav = read(navPath);
const header = read(headerPath);
const meta = read(routerMetaPath);
const generated = read(generatedRoutesPath);

const navKeys = new Set([...nav.matchAll(/labelKey: 'route\.([^']+)'/g)].map(m => m[1]));
const metaKeys = new Set([...meta.matchAll(/^      "([a-z0-9_-]+)": \{/gm)].map(m => m[1]));
const generatedKeys = new Set([...generated.matchAll(/name: '([a-z0-9_-]+)'/g)].map(m => m[1]));
const pathsBlock = (nav.match(/export const PATHS[^=]*=\s*\{([\s\S]*?)\n\};/) || [])[1] || '';
const pathKeys = new Set([...pathsBlock.matchAll(/^\s+'?([a-zA-Z0-9_-]+)'?:\s*'[^']+'/gm)].map(m => m[1]));

// Group parents are expandable menu sections, not navigable leaves. They are
// listed in `expanded` rather than PATHS.
const groupParents = new Set(['dns', 'dhcp', 'ipam', 'admin', 'logs', 'settings']);

// 1. Every nav key and route-meta key needs a translation in both languages.
for (const lang of langs) {
  const text = read(path.join(root, 'web-admin/src/locales/langs', lang));
  const block = text.match(/  route: \{([\s\S]*?)\n  \},/);
  if (!block) { fail(`${lang} has no route translation block`); continue; }
  const have = new Set([...block[1].matchAll(/'([^']+)':/g)].map(m => m[1]));
  for (const key of new Set([...navKeys, ...metaKeys])) {
    if (!have.has(key)) fail(`${lang} is missing route.${key}`);
  }
}

// 2. Every navigable (non-group) nav key needs a PATHS entry.
for (const key of navKeys) {
  if (!groupParents.has(key) && !pathKeys.has(key)) {
    fail(`nav item ${key} has no PATHS entry (clicking it does nothing)`);
  }
}

// 3. Every route-meta key must match a generated route name. Constant
// routes (login / 403 / 404 / 500) are generated too, so no allowlist needed.
for (const key of metaKeys) {
  if (!generatedKeys.has(key)) fail(`ROUTE_META key ${key} is not a generated route`);
}

// 4. Every frontend API literal must match a documented OpenAPI path AND
// the HTTP method used by its request helper must match an operation on that
// path. Normalise trailing slashes and both {id} and ${id} placeholders.
// Template literals that embed a query-string expression (for example
// `.../import${overwrite ? '?overwrite=true' : ''}`) are covered by the
// path-only check; method matching skips those to avoid a false positive.
const norm = p => p.replace(/\/+$/, '').replace(/\$\{[^}]+\}/g, '{}').replace(/\{[^}]+\}/g, '{}');
const openapi = read(openapiPath);
const documented = new Set(
  [...openapi.matchAll(/\{"(\/[^"]+)", "(?:GET|POST|PUT|DELETE|PATCH)"/g)].map(m => norm(m[1]))
);
const documentedOps = new Set(
  [...openapi.matchAll(/\{"(\/[^"]+)", "(GET|POST|PUT|DELETE|PATCH)"/g)]
    .map(m => `${m[2]} ${norm(m[1])}`)
);
const wrappers = {
  get: 'GET', getList: 'GET', getRaw: 'GET',
  post: 'POST', postRaw: 'POST',
  put: 'PUT', del: 'DELETE', patch: 'PATCH'
};
const prefix = lit => {
  // routeDocs paths are bare (/users, /dns/zones). Strip an explicit
  // /api/v1 prefix when a service literal happens to include one.
  let p = lit;
  if (p.startsWith('/api/')) p = p.slice(4);
  if (p.startsWith('/v1/')) p = p.slice(3);
  return p;
};
// Non-API literals that legitimately appear in the service layer.
const ignored = new Set(['/login', '/v1', '/proxy-default/api/v1', '/', '//']);
for (const file of fs.readdirSync(serviceDir).filter(f => f.endsWith('.ts'))) {
  const text = read(path.join(serviceDir, file));

  // Path coverage for every literal (unchanged).
  for (const m of text.matchAll(/[`'](\/[a-zA-Z0-9_.{}$/+-]+)[`']/g)) {
    let lit = m[1].split('?')[0];
    lit = prefix(lit);
    if (ignored.has(lit)) continue;
    if (!documented.has(norm(lit))) fail(`${file} calls undocumented API path ${lit}`);
  }

  // Method coverage for calls whose literal has no query expression.
  for (const [wrapper, method] of Object.entries(wrappers)) {
    const re = new RegExp('\\b' + wrapper + '\\s*(?:<[^>]*?>)?\\s*\\(\\s*[`\']([^`\']*)[`\']', 'g');
    for (const m of text.matchAll(re)) {
      const lit = m[1];
      if (lit.includes('?')) continue; // query-suffix template: path-only above
      const full = prefix(lit);
      if (ignored.has(full)) continue;
      if (!documentedOps.has(`${method} ${norm(full)}`)) {
        fail(`${file} uses ${method} on ${full}, but no such documented operation`);
      }
    }
  }
}

// 5. Every ROUTE_META permission must exist in the RBAC predefined set.
// An unknown (resource, action) would silently deny the page to everyone.
const predefined = new Set(
  [...read(rbacPath).matchAll(/\{Resource:\s*"([^"]+)",\s*Action:\s*"([^"]+)"/g)]
    .map(m => `${m[1]}:${m[2]}`)
);
for (const block of meta.matchAll(/"permission":\s*\{([^}]*)\}/g)) {
  const r = block[1].match(/"resource":\s*"([^"]+)"/);
  const a = block[1].match(/"action":\s*"([^"]+)"/);
  if (!r || !a) { fail('a ROUTE_META permission block is incomplete'); continue; }
  if (!predefined.has(`${r[1]}:${a[1]}`)) {
    fail(`ROUTE_META permission ${r[1]}:${a[1]} is not a predefined RBAC permission`);
  }
}

// 6. Every management view must be exercised by at least one browser suite.
// A page added without a browser test is how a broken route reaches users.
// Dynamic routes ([id].vue) are checked by their directory prefix instead.
const viewsDir = path.join(root, 'web-admin/src/views');
const walkViews = dir => fs.readdirSync(dir, { withFileTypes: true }).flatMap(e => {
  const full = path.join(dir, e.name);
  if (e.isDirectory()) return walkViews(full);
  return e.name.endsWith('.vue') ? [full] : [];
});
const suiteFiles = fs.readdirSync(__dirname)
  .filter(f => f === 'smoke.cjs' || /^batch\d+\.cjs$/.test(f) || f === 'legacy.cjs' || f === 'route-audit.cjs')
  .map(f => path.join(__dirname, f));
const suiteText = suiteFiles.map(read).join('\n');
for (const view of walkViews(viewsDir)) {
  const rel = view.slice(viewsDir.length + 1).replace(/\\/g, '/');
  if (rel.startsWith('_builtin/')) continue; // /login is asserted separately
  let expected;
  if (/\[[^\]]+\]\.vue$/.test(rel)) {
    expected = '/' + rel.slice(0, rel.lastIndexOf('/'));
  } else {
    expected = '/' + rel.replace(/\/index\.vue$/, '').replace(/\.vue$/, '');
  }
  if (!suiteText.includes(expected)) {
    fail(`view ${rel} (${expected}) has no browser suite`);
  }
}
if (!suiteText.includes('/login')) fail('the login view has no browser suite');

// Visible navigation permissions must match generated route meta. A mismatch
// either hides a reachable page or shows one the guard rejects.
const navSource = read(path.join(root, 'web-admin/src/components/starter/nav.ts'));
const routeSource = read(path.join(root, 'web-admin/src/router/elegant/routes.ts'));
const toolsNavBlock = navSource.slice(navSource.indexOf("key: 'tools_client'"), navSource.indexOf("key: 'tools_client'") + 220);
const toolsRouteBlock = routeSource.slice(routeSource.indexOf("name: 'tools_client'"), routeSource.indexOf("name: 'tools_client'") + 700);
const navPermission = toolsNavBlock.match(/resource: '(\w+)', action: '(\w+)'/);
const routePermission = toolsRouteBlock.match(/resource: '(\w+)'[\s\S]{0,80}?action: '(\w+)'/);
if (!navPermission || !routePermission || navPermission[1] !== routePermission[1] || navPermission[2] !== routePermission[2]) {
  fail('nav/route permission consistency: tools_client NAV and route meta differ');
}
if (!routeSource.includes("activeMenu: 'dns_zones'")) fail('dns_zone-detail must declare activeMenu for shell highlight');

// 7. SINGLE-NODE-FIRST reservation. Until the dual-node architecture is
// implemented (with independently validated role/epoch/fencing and B8.5
// dual-host evidence), the console must not acquire HA state, cluster
// requests, or a cluster nav entry. This is an explicit product decision, not
// an accidental omission, so a future change has to be deliberate.
const badge = read(path.join(root, 'web-admin/src/components/NodeModeBadge.vue'));
if (/withDefaults\([\s\S]*?mode:\s*'single'/.test(badge)) {
  fail('NodeModeBadge hard-codes single-node mode; it must use live HA state');
}
if (!/useNodeMode/.test(badge)) {
  fail('NodeModeBadge no longer uses useNodeMode');
}
if (!/nodeMode\.degraded/.test(badge) || !/case 'single'/.test(badge)) {
  fail('NodeModeBadge lost its fail-closed degraded label');
}
const clusterApi = read(path.join(root, 'web-admin/src/service/api/goddi/cluster.ts'));
if (!/CLUSTER_API_RESERVED\s*=\s*true/.test(clusterApi)) {
  fail('cluster.ts lost its CLUSTER_API_RESERVED marker');
}
if (/\b(get|post|put|del|getList|postRaw|fetch)\s*\(/.test(clusterApi)) {
  fail('cluster.ts now issues a request; dual-node is still reserved');
}
if (/key: 'cluster'/.test(layout)) {
  fail('a cluster nav group was registered during the single-node phase');
}
const badgeUses = [...header.matchAll(/<NodeModeBadge([^>]*)\/>/g)].map(m => m[1]);
if (badgeUses.length === 0) {
  fail('the layout no longer renders NodeModeBadge');
} else if (badgeUses.some(u => /:mode\s*=/.test(u))) {
  fail('NodeModeBadge is bound to dynamic HA state during the single-node phase');
}

if (failures === 0) console.log('PASS | frontend/backend contract parity');
process.exit(failures ? 1 : 0);