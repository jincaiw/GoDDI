// Static guard for docs/STATUS.md, the single-page handoff.
//
// That document is the entry point for a reviewer or a field engineer. If its
// cited counts drift from the repository, or a referenced file is moved, the
// handoff quietly becomes wrong -- which is worse than having no summary.
// This runs without a browser (see run.sh).

const fs = require('fs');
const path = require('path');

const root = path.join(__dirname, '..', '..');
const statusPath = path.join(root, 'docs/STATUS.md');
let failures = 0;
const fail = msg => { console.log('FAIL | ' + msg); failures++; };

if (!fs.existsSync(statusPath)) {
  console.log('FAIL | docs/STATUS.md is missing');
  process.exit(1);
}
const status = fs.readFileSync(statusPath, 'utf8');

// 1. Every repo path mentioned in the handoff must still exist.
const links = new Set(
  [...status.matchAll(/`([^`]+?\.(?:md|go|ts|vue|cjs|sh|json|proto|yaml))`/g)].map(m => m[1])
);
for (const link of links) {
  const candidates = [
    path.join(root, link),
    path.join(root, 'docs', link),
    path.join(root, 'docs/audit', link),
    path.join(root, 'test/browser', link)
  ];
  if (!candidates.some(p => fs.existsSync(p))) {
    fail(`docs/STATUS.md references a missing file: ${link}`);
  }
}

// 2. Recompute the headline counts from the repository.
const readDir = p => fs.readdirSync(p, { withFileTypes: true });
const countViews = dir => readDir(dir).flatMap(e => {
  const full = path.join(dir, e.name);
  return e.isDirectory() ? countViews(full) : (e.name.endsWith('.vue') ? [full] : []);
});
const countTestFuncs = dir => readDir(dir).flatMap(e => {
  const full = path.join(dir, e.name);
  if (e.isDirectory()) return e.name === 'testdata' || e.name === 'node_modules' ? [] : countTestFuncs(full);
  if (!e.name.endsWith('_test.go')) return [];
  return [...fs.readFileSync(full, 'utf8').matchAll(/^func (?:Test|Fuzz)\w*/gm)];
});

const enterpriseDir = path.join(root, 'internal/enterprise');
const enterprisePkgs = readDir(enterpriseDir).filter(e => e.isDirectory()).length;
const enterpriseTests = countTestFuncs(enterpriseDir).length;
const openapi = fs.readFileSync(path.join(root, 'internal/api/openapi.go'), 'utf8');
const openapiOps = [...openapi.matchAll(/\{"\/[^"]+", "(?:GET|POST|PUT|DELETE|PATCH)"/g)].length;
const runSh = fs.readFileSync(path.join(__dirname, 'run.sh'), 'utf8');
const defaultSuites = (runSh.match(/GODDI_SUITES:-([^"]+)/) || [])[1];
const suiteNames = defaultSuites ? defaultSuites.trim().split(/\s+/) : [];
const browserChecks = suiteNames.length;
const staticSuites = suiteNames.filter(name => ['docs', 'contracts', 'i18n', 'theme', 'h2-report-ref'].includes(name));
const chromiumSuites = browserChecks - staticSuites.length;


// TDesign management pages: one route per .vue file under views/, plus the
// login view. Elegant-router maps each .vue page file to a route.
function tdesignPages() {
  const viewsDir = path.join(root, 'web-admin/src/views');
  if (!fs.existsSync(viewsDir)) return 0;
  let n = 0;
  const walk = d => {
    for (const e of fs.readdirSync(d, { withFileTypes: true })) {
      if (e.isDirectory()) {
        if (e.name === '_builtin') continue; // login counted separately
        walk(path.join(d, e.name));
      } else if (e.name.endsWith('.vue')) {
        n += 1;
      }
    }
  };
  walk(viewsDir);
  return n + 1; // + _builtin/login
}


// Authority sentinels: enterprise packages that define their own
// ErrNoAuthority (each guards a different protected-write surface).
function authoritySentinels() {
  const entDir = path.join(root, 'internal/enterprise');
  if (!fs.existsSync(entDir)) return 0;
  let n = 0;
  for (const dir of fs.readdirSync(entDir, { withFileTypes: true })) {
    if (!dir.isDirectory()) continue;
    const pkgDir = path.join(entDir, dir.name);
    for (const f of fs.readdirSync(pkgDir)) {
      if (f.endsWith('.go') && !f.endsWith('_test.go')) {
        const src = fs.readFileSync(path.join(pkgDir, f), 'utf8');
        if (src.includes('ErrNoAuthority = errors.New')) { n += 1; break; }
      }
    }
  }
  return n;
}

const expected = [
  { label: 'TDesign pages', needle: `${tdesignPages()} 个 TDesign 页面` },
  { label: 'API operations', needle: `${openapiOps} 个 ` + '`/api/v1` 操作' },
  { label: 'enterprise packages', needle: `${enterprisePkgs} 个 ` + '`internal/enterprise` 包' },
  { label: 'enterprise tests', needle: `${enterpriseTests} 个测试` },
  { label: 'authority sentinels', needle: `${authoritySentinels()} 个 ` + '`ErrNoAuthority` 哨兵' },
  { label: 'verification checks', needle: `${browserChecks} 项检查` },
  { label: 'Chromium suites', needle: `${chromiumSuites} 个真实 Chromium 套件` },
  { label: 'static suites', needle: `${staticSuites.length} 个静态守卫` },
];

for (const e of expected) {
  if (!status.includes(e.needle)) {
    fail(`docs/STATUS.md no longer states the current ${e.label}: ${e.needle}`);
  }
}

// 3. The handoff must keep its explicit non-claims and external-blocker
// section; removing them would let a reader mistake candidates for acceptance.
for (const marker of ['刻意未做的事', '外部阻断', '待独立评审冻结', '不构成']) {
  if (!status.includes(marker)) {
    fail(`docs/STATUS.md lost the "${marker}" boundary statement`);
  }
}

// 4. Config documentation parity. Every yaml tag in internal/config must
// either appear in the sample config.yaml or be explained in
// docs/CONFIG-REFERENCE.md; and every key in config.yaml must have a matching
// yaml tag (the loader uses UnmarshalStrict, so a stale sample key breaks
// startup). This keeps the sample config from drifting unnoticed.
const configDir = path.join(root, 'internal/config');
const configTags = new Set();