// Static guard: the gate ledger and acceptance docs must cite the same
// canonical H2 aggregate report as the newest report on disk.
const fs = require('fs');
const path = require('path');
const root = path.join(__dirname, '..', '..');
function findSessionRoot() {
  const ledgerRel = path.join('architecture', 'G5-生产候选门禁清单.md');
  const candidates = [];
  if (process.env.GODDI_SESSION_ROOT) candidates.push(process.env.GODDI_SESSION_ROOT);
  let dir = root;
  for (let i = 0; i < 8; i += 1) {
    candidates.push(dir);
    candidates.push(path.join(dir, 'goddi'));
    dir = path.dirname(dir);
  }
  const found = candidates.find(candidate => fs.existsSync(path.join(candidate, ledgerRel)));
  return found || null;
}
const sessionRoot = findSessionRoot();
let failures = 0;
const fail = msg => { console.log('FAIL | ' + msg); failures++; };
const read = rel => fs.readFileSync(path.join(root, rel), 'utf8');

const reportDir = path.join(root, 'tests/h2/reports');
const reports = fs.readdirSync(reportDir)
  .filter(f => /^H2-REPORT-\d{8}T\d{6}Z\.json$/.test(f))
  .sort();
if (!reports.length) fail('no H2 aggregate report found');
const canonical = reports[reports.length - 1].replace(/\.json$/, '');

const g5 = sessionRoot
  ? fs.readFileSync(path.join(sessionRoot, 'architecture/G5-生产候选门禁清单.md'), 'utf8')
  : null;
const conclusion = read('docs/acceptance/H2_ACCEPTANCE_CONCLUSION.md');
const docker = read('docs/acceptance/H2_DOCKER_ACCEPTANCE.md');
const references = [
  ['H2_ACCEPTANCE_CONCLUSION.md', conclusion],
  ['H2_DOCKER_ACCEPTANCE.md', docker]
];
if (g5) references.unshift(['G5', g5]);
for (const [name, text] of references) {
  if (!text.includes(canonical)) fail(name + ' does not cite canonical ' + canonical);
}
if (failures) {
  console.log('errors: ' + failures);
  process.exit(1);
}
console.log('PASS h2-report-ref canonical=' + canonical + ' g5=' + (g5 ? 'checked' : 'absent'));