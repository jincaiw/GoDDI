// Static i18n guard, two parts (no browser needed; see run.sh):
//
// 1. The DNSSEC experimental hints must not regress to the pre-signing
//    wording and must keep the parent-DS boundary.
// 2. Every literal t('a.b.c') key used in Vue/TS source must resolve in BOTH
//    locale files. A typo or a removed key renders the raw "a.b.c" in the UI;
//    vue-tsc does not check string literals, so this guard does.
//
// Key resolution uses the dotted key's leaf name against the locale files'
// `leaf:` definitions. That is deliberately looser than a full nested parse:
// it catches typos and removals without duplicating the locale schema here.
const fs = require('fs');
const path = require('path');

const root = path.join(__dirname, '..', '..');
const langsDir = path.join(root, 'web-admin/src/locales/langs');
const langs = ['zh-cn.ts', 'en-us.ts'];
let failures = 0;
const fail = msg => { console.log('FAIL | ' + msg); failures++; };

const localeText = {};
for (const f of langs) {
  localeText[f] = fs.readFileSync(path.join(langsDir, f), 'utf8');
}

// --- Part 1: DNSSEC hint wording ---
const hintKeys = ['dnssecExperimentalHint', 'dnssecNsec3Hint', 'experimentalHint'];
const required = [/父区 DS|parent-side DS|no parent/i];
const stale = [
  '尚未携带 RRSIG',
  '不生成任何 RRSIG',
  '尚未生成签名',
  '待真实签名实现',
  'carry no RRSIG',
  'take effect once real zone signing lands',
  'only flags the zone'
];
for (const f of langs) {
  for (const key of hintKeys) {
    const m = localeText[f].match(new RegExp(`${key}:\\s*'([^']*)'`));
    if (!m) continue;
    const value = m[1];
    if (key !== 'dnssecNsec3Hint' && !required.some(r => r.test(value))) {
      fail(`${f} ${key} lost the parent-DS boundary`);
    }
    for (const s of stale) {
      if (value.includes(s)) fail(`${f} ${key} regressed to stale claim: ${s}`);
    }
  }
}

// --- Part 2: literal t() keys must exist in both languages ---
const collectSources = dir => fs.readdirSync(dir, { withFileTypes: true }).flatMap(e => {
  const full = path.join(dir, e.name);
  if (e.isDirectory()) return collectSources(full);
  return /\.(vue|ts)$/.test(e.name) && !e.name.endsWith('.d.ts') ? [full] : [];
});
const sourceDir = path.join(root, 'web-admin/src');
const usedKeys = new Set();
for (const file of collectSources(sourceDir)) {
  const text = fs.readFileSync(file, 'utf8');
  // Only literal keys. Dynamic keys (template strings / concatenation) are
  // intentionally out of scope; they are reviewed where they are built.
  for (const m of text.matchAll(/(?<![\w$])(?:\$t|t)\(\s*'([^']+)'/g)) {
    usedKeys.add(m[1]);
  }
}
if (usedKeys.size === 0) fail('no literal i18n keys found; the extraction regex broke');

for (const f of langs) {
  const body = localeText[f];
  for (const key of usedKeys) {
    const leaf = key.split('.').pop();
    const def = new RegExp("(?:^|\\s)'?" + leaf.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') + "'?\\s*:", 'm');
    if (!def.test(body)) fail(`${f} is missing i18n key ${key}`);
  }
}

if (failures === 0) {
  console.log(`PASS | ${usedKeys.size} literal i18n keys resolve in both languages; DNSSEC hints are current`);
}
process.exit(failures ? 1 : 0);
