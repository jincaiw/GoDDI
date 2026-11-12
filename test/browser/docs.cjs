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
const browserChecks = defaultSuites ? defaultSuites.trim().split(/\s+/).length : 0;


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
for (const f of fs.readdirSync(configDir).filter(x => x.endsWith('.go') && !x.endsWith('_test.go'))) {
  for (const m of fs.readFileSync(path.join(configDir, f), 'utf8').matchAll(/yaml:"([a-zA-Z0-9_.-]+)"/g)) {
    if (m[1] && m[1] !== '-') configTags.add(m[1]);
  }
}
const sample = fs.readFileSync(path.join(root, 'config.yaml'), 'utf8');
const sampleKeys = new Set();
for (const line of sample.split('\n')) {
  if (line.trim().startsWith('#') || !line.trim()) continue;
  const m = line.match(/^\s*-?\s*([a-zA-Z0-9_.-]+)\s*:/);
  if (m) sampleKeys.add(m[1]);
}
for (const key of sampleKeys) {
  if (!configTags.has(key)) fail(`config.yaml has key "${key}" with no yaml tag (strict loader would reject it)`);
}
const referencePath = path.join(root, 'docs/CONFIG-REFERENCE.md');
const reference = fs.existsSync(referencePath) ? fs.readFileSync(referencePath, 'utf8') : '';
if (!reference) {
  fail('docs/CONFIG-REFERENCE.md is missing');
} else {
  for (const tag of configTags) {
    if (!sampleKeys.has(tag) && !reference.includes(tag)) {
      fail(`config key "${tag}" is neither in config.yaml nor documented in docs/CONFIG-REFERENCE.md`);
    }
  }
}

// 5. Every GODDI_* env override must be documented in the config reference.
// An operator cannot use a knob they cannot discover, and an override that
// silently wins over YAML is exactly the kind that needs to be written down.
const envOverrides = new Set();
for (const f of fs.readdirSync(configDir).filter(x => x.endsWith('.go') && !x.endsWith('_test.go'))) {
  for (const m of fs.readFileSync(path.join(configDir, f), 'utf8')
    .matchAll(/setEnv(?:String|Bool|Int)\("(GODDI_[A-Z0-9_]+)"/g)) {
    envOverrides.add(m[1]);
  }
}
if (envOverrides.size === 0) {
  fail('no GODDI_* env overrides found; the extraction regex broke');
}
for (const env of envOverrides) {
  if (!reference.includes(env)) {
    fail(`env override ${env} is not documented in docs/CONFIG-REFERENCE.md`);
  }
}

// 6. Every config path in the env-var table must be made of real yaml tags.
// The table is generated from Go field names; a camelCase acronym can easily
// produce a plausible-looking but wrong key (for example ExposeOpenAPI ->
// expose_open_api instead of expose_openapi). Operators would set the wrong
// YAML key, so validate every dotted segment here.
const tableRows = [...reference.matchAll(/^\| `(GODDI_[A-Z0-9_]+)` \| .*? \| `([^`]+)` \|$/gm)];
if (tableRows.length === 0) {
  fail('the env-var table in docs/CONFIG-REFERENCE.md is empty or its format changed');
} else {
  for (const m of tableRows) {
    const path = m[2];
    for (const segment of path.split('.')) {
      if (!configTags.has(segment)) {
        fail(`docs/CONFIG-REFERENCE.md maps ${m[1]} to ${path}, but "${segment}" is not a yaml tag`);
      }
    }
  }
}

// 7. Deployment artifact consistency. `make verify` rebuilds
// web-admin/dist but does not sync web/dist; the server serves ./web/dist, so
// a stale deployed bundle would otherwise pass verification unnoticed. When
// web/dist exists it must be byte-for-byte identical to web-admin/dist.
const crypto = require('crypto');
const builtDir = path.join(root, 'web-admin/dist');
const deployedDir = path.join(root, 'web/dist');
const hashTree = dir => {
  const out = {};
  const walk = d => {
    for (const e of fs.readdirSync(d, { withFileTypes: true })) {
      const full = path.join(d, e.name);
      if (e.isDirectory()) walk(full);
      else if (e.isFile()) {
        out[path.relative(dir, full)] = crypto.createHash('sha256').update(fs.readFileSync(full)).digest('hex');
      }
    }
  };
  if (fs.existsSync(dir)) walk(dir);
  return out;
};
if (fs.existsSync(deployedDir)) {
  const built = hashTree(builtDir);
  const deployed = hashTree(deployedDir);
  if (Object.keys(built).length === 0) {
    fail('web/dist exists but web-admin/dist is empty; a fresh build is required');
  }
  for (const [name, hash] of Object.entries(deployed)) {
    if (built[name] !== hash) fail(`deployed asset ${name} differs from the built bundle`);
  }
  for (const name of Object.keys(built)) {
    if (!(name in deployed)) fail(`built asset ${name} is missing from web/dist`);
  }
}

// 8. The review dossier (docs/audit/ENTERPRISE-REVIEW-DOSSIER.md) is the
// second document a reviewer opens after STATUS.md. Its headline counts are
// computed from the same repo facts; pin them so they cannot silently rot.
const dossierPath = path.join(root, 'docs/audit/ENTERPRISE-REVIEW-DOSSIER.md');
if (!fs.existsSync(dossierPath)) {
  fail('docs/audit/ENTERPRISE-REVIEW-DOSSIER.md is missing');
} else {
  const dossier = fs.readFileSync(dossierPath, 'utf8');
  
// Verification make entries: the targets a reviewer runs to reproduce.
function makeVerifyEntries() {
  const mk = fs.readFileSync(path.join(root, 'Makefile'), 'utf8');
  const names = ['verify', 'verify-race', 'verify-browser', 'verify-e2e', 'verify-tidy', 'build'];
  return names.filter(n => new RegExp(`^${n}:`, 'm').test(mk)).length;
}

const dossierClaims = [
    { label: 'enterprise test count', needle: `| Go 测试与 Fuzz/Property 入口 | ${enterpriseTests} |` },
    { label: 'verification entry count', needle: `| 可复现验证入口 | ${makeVerifyEntries()} |` },
    { label: 'browser check count', needle: `浏览器回归共 **${browserChecks} 项检查**` },
    { label: 'production gate count', needle: `## 2. 门禁清单（${authoritySentinels()} 个 ` + '`ErrNoAuthority` 哨兵）' },
  ];
  for (const c of dossierClaims) {
    if (!dossier.includes(c.needle)) fail(`review dossier no longer states the current ${c.label}: ${c.needle}`);
  }
}

// 9. Numbered top-level headings must be unique in the field-facing docs
// (copy-paste section numbering is easy to introduce), and the B8.5 runbook
// must keep all five external items marked as not accepted. A field engineer
// reading a duplicated "## 11" or a silently upgraded status would be misled.
const numberedDocs = [
  'docs/STATUS.md',
  'docs/CONFIG-REFERENCE.md',
  'docs/audit/ENTERPRISE-REVIEW-DOSSIER.md',
  'docs/audit/IMPLEMENTATION-STATUS.md',
  'docs/B8.5-外部验收Runbook.md'
];
for (const rel of numberedDocs) {
  const full = path.join(root, rel);
  if (!fs.existsSync(full)) { fail(`${rel} is missing`); continue; }
  const seen = new Set();
  for (const m of fs.readFileSync(full, 'utf8').matchAll(/^## (\d+)\./gm)) {
    if (seen.has(m[1])) fail(`${rel} has a duplicate "## ${m[1]}." heading`);
    seen.add(m[1]);
  }
}
const runbookPath = path.join(root, 'docs/B8.5-外部验收Runbook.md');
if (fs.existsSync(runbookPath)) {
  const runbook = fs.readFileSync(runbookPath, 'utf8');
  const notAccepted = (runbook.match(/\| 未验收 \|/g) || []).length;
  if (notAccepted < 5) {
    fail(`B8.5 runbook has only ${notAccepted} "未验收" rows; external items must not be upgraded without evidence`);
  }
  // Derive the expected check count from run.sh's default suite list so the
  // guard can never go stale when a suite is added (round 224: the hardcoded
  // "19 项检查" failed to catch the batch16 drift to 20).
  const runSh = path.join(root, 'test/browser/run.sh');
  let expectedChecks = 0;
  if (fs.existsSync(runSh)) {
    const m = fs.readFileSync(runSh, 'utf8').match(/suites="\$\{GODDI_SUITES:-([^}]*)\}"/);
    if (m) expectedChecks = m[1].trim().split(/\s+/).length;
  }
  if (expectedChecks > 0 && !runbook.includes(`${expectedChecks} 项检查`)) {
    fail(`B8.5 runbook no longer states the current ${expectedChecks}-check verification matrix`);
  }
}

// 10. Cross-references to the B8.5 runbook must point at real numbered
// sections. A stale "§11" that now means something else sends a field
// engineer to the wrong instructions.
const runbookForRefs = fs.readFileSync(runbookPath, 'utf8');
const runbookNumbers = new Set(
  [...runbookForRefs.matchAll(/^## (\d+)\./gm)].map(m => m[1])
);
const collectMd = dir => fs.readdirSync(dir, { withFileTypes: true }).flatMap(e => {
  const full = path.join(dir, e.name);
  if (e.isDirectory()) return e.name === 'node_modules' ? [] : collectMd(full);
  return e.name.endsWith('.md') ? [full] : [];
});
for (const md of collectMd(path.join(root, 'docs')).concat(collectMd(path.join(root, 'contracts')))) {
  const text = fs.readFileSync(md, 'utf8');
  for (const m of text.matchAll(/B8\.5-外部验收Runbook\.md`?\s*§(\d+)(?:\.(\d+))?/g)) {
    if (!runbookNumbers.has(m[1])) {
      fail(`${path.relative(root, md)} references B8.5 runbook §${m[1]}${m[2] ? '.' + m[2] : ''}, which does not exist`);
    }
  }
}

if (failures === 0) console.log('PASS | handoff, config docs, dossier, runbook and deployed bundle are consistent with the repository');
process.exit(failures ? 1 : 0);
