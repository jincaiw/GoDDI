// Human-style CRUD walkthrough against a running console (real Chromium, real
// HTTP, real SQLite). It logs in through the login form, then creates a zone,
// confirms the list UI shows it, adds a record, confirms the zone-detail page
// renders it, and deletes the zone. The console API is used for the writes
// because the other Playwright suites already cover the form interactions; the
// navigation and rendering steps are real page loads.
//
// The detail route is /dns/zone-detail/:id (the list page links there too).
const pathMod = require('path');
// Resolve playwright from the repo's harness install, independent of the cwd
// the script is launched from.
const repoRoot = pathMod.resolve(__dirname, '..', '..', '..');
const { chromium } = require(pathMod.join(repoRoot, 'test', 'browser', 'node_modules', 'playwright'));
const BASE = process.env.GODDI_BASE_URL || 'http://127.0.0.1:18080';
const PASS = process.env.GODDI_ADMIN_PASS;
let pass = 0, fail = 0;
const ok = (m) => { console.log('PASS | ' + m); pass++; };
const bad = (m) => { console.log('FAIL | ' + m); fail++; };

(async () => {
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();
  const errors = [];
  page.on('pageerror', (e) => errors.push(String(e)));
  await page.goto(BASE + '/login', { waitUntil: 'networkidle' });
  await page.fill('input[type="text"], input[name="username"]', 'admin');
  await page.fill('input[type="password"]', PASS);
  await page.click('button[type="submit"]');
  await page.waitForTimeout(2500);
  (await page.locator('text=仪表盘').count()) > 0 ? ok('logged in and dashboard rendered') : bad('login failed');

  // Navigate the zone page through the UI and use the real API for the write
  // (the harness's other suites cover the form interactions).
  // One login, one session: the CSRF token is bound to the token it was issued
  // with, so two independent logins produce a pair that does not match.
  const auth = await page.evaluate(async () => {
    const r = await fetch('/api/v1/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username: 'admin', password: 'AdminPass-0123456789' }) });
    const j = await r.json();
    return { token: j.data.token, csrf: j.data.csrf_token };
  });
  const token = auth.token, csrf = auth.csrf;
  const name = 'webtest' + Date.now() + '.test.';
  const created = await page.evaluate(async ([t, c, n]) => {
    const r = await fetch('/api/v1/dns/zones', { method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + t, 'X-CSRF-Token': c }, body: JSON.stringify({ name: n, type: 'primary' }) });
    return { status: r.status, body: await r.text() };
  }, [token, csrf, name]);
  created.status === 201 ? ok('zone created through the console API (' + name + ')') : bad('zone create status=' + created.status + ' ' + created.body.slice(0, 120));
  const zid = JSON.parse(created.body).data.id;

  await page.goto(BASE + '/dns/zones', { waitUntil: 'networkidle' });
  await page.waitForTimeout(1500);
  const visible = await page.locator('text=' + name).count();
  visible > 0 ? ok('new zone visible in the zone list UI') : bad('zone not visible in the list UI');

  const rec = await page.evaluate(async ([t, c, id]) => {
    const r = await fetch('/api/v1/dns/zones/' + id + '/records', { method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + t, 'X-CSRF-Token': c }, body: JSON.stringify({ name: 'www', type: 'A', ttl: 300, value: '192.0.2.9' }) });
    return r.status;
  }, [token, csrf, zid]);
  rec === 201 ? ok('record added to the new zone') : bad('record create status=' + rec);

  await page.goto(BASE + '/dns/zone-detail/' + zid, { waitUntil: 'networkidle' });
  let seen = 0;
  for (let i = 0; i < 15; i++) {
    seen = await page.locator('text=192.0.2.9').count();
    if (seen > 0) break;
    await page.waitForTimeout(1000);
  }
  seen > 0 ? ok('record visible on the zone detail page') : bad('record not visible on the detail page (url=' + page.url() + ')');

  const del = await page.evaluate(async ([t, c, id]) => {
    const r = await fetch('/api/v1/dns/zones/' + id, { method: 'DELETE', headers: { Authorization: 'Bearer ' + t, 'X-CSRF-Token': c } });
    return r.status;
  }, [token, csrf, zid]);
  (del === 200 || del === 204) ? ok('zone deleted (cleanup)') : bad('zone delete status=' + del);
  errors.length === 0 ? ok('no page errors during the walkthrough') : bad('page errors: ' + errors.slice(0, 2).join('; '));
  await browser.close();
  console.log('----'); console.log('CRUD PASS=' + pass + ' FAIL=' + fail);
  process.exit(fail === 0 ? 0 : 1);
})().catch((e) => { console.error('FAIL | ' + e.message); process.exit(1); });
