// Full-route UI hygiene audit.
//
// Logs in once, walks every concrete management route, and asserts that no raw
// i18n keys, ISO timestamps, known enum values, or page errors are rendered.
// The dynamic zone-detail route is resolved from a real zone id so the check
// works on a fresh seeded database without a hand-maintained fixture file.
const fs = require('fs');
const path = require('path');
const { chromium } = require('playwright');
const { BASE, USER, PASS, SHOT_DIR, launchOpts } = require('./common.cjs');

const routes = [
  '/dashboard',
  '/admin', '/admin/groups', '/admin/roles', '/admin/sessions', '/admin/tokens', '/admin/users',
  '/dhcp', '/dhcp/ha', '/dhcp/leases', '/dhcp/options', '/dhcp/reservations', '/dhcp/scopes',
  '/dns', '/dns/allowed', '/dns/apps', '/dns/blocked', '/dns/cache', '/dns/dnssec',
  '/dns/forwarders', '/dns/security', '/dns/zones',
  '/ipam', '/ipam/addresses', '/ipam/spaces', '/ipam/subnets', '/ipam/retirement-archive',
  '/logs', '/logs/audit', '/logs/dhcp', '/logs/dns', '/settings'
];

const I18N = /\b((?:common|route|dns|dhcp|ipam|admin|settings|logs|tools|haStatus|sessions|theme|page|form|icon|menu)\.[A-Za-z0-9_.]+)\b/g;
const ISO = /\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z/g;
const ENUM = /(?:^|\n)(active|retired|archived|used|available|reserved|conflict|allow|block|apply_lists|primary|secondary|stub|forward|catalog|standby|monitor|true|false|NXDOMAIN|NODATA|REFUSED|DROP|CUSTOM_IP|noerror|nxdomain|servfail|refused|assign_role|set_permissions|dns_dynamic_update|ddns_record_create|ddns_record_delete|dhcp_lease_bind|dhcp_lease_release|dhcp_lease_decline|dhcp_lease_expire|dhcp_ha_degrade|dhcp_ha_takeover|dhcp_ha_fence|dhcp_ha_rejoin)(?:\n|$)/g;

async function zoneDetailRoute(page) {
  return await page.evaluate(async base => {
    const token = sessionStorage.getItem('token');
    if (!token) return { route: '', createdId: '' };
    const csrf = sessionStorage.getItem('csrf_token') || '';
    const headers = { Authorization: 'Bearer ' + token, 'Content-Type': 'application/json', 'X-CSRF-Token': csrf };
    const list = await fetch(base + '/api/v1/dns/zones?page=1&page_size=1', { headers });
    if (list.ok) {
      const body = await list.json();
      const id0 = body?.data?.[0]?.id;
      if (id0) return { route: '/dns/zone-detail/' + encodeURIComponent(id0), createdId: '' };
    }
    // Fresh database: create a temporary zone so the dynamic route is exercisable.
    const name = 'tdroute' + Date.now().toString() + Math.floor(Math.random() * 1000) + '.test.';
    const created = await fetch(base + '/api/v1/dns/zones', {
      method: 'POST', headers, body: JSON.stringify({ name, type: 'primary', enabled: true })
    });
    if (!created.ok) return { route: '', createdId: '' };
    const cb = await created.json();
    const zid = cb?.data?.id || '';
    return { route: zid ? '/dns/zone-detail/' + encodeURIComponent(zid) : '', createdId: zid };
  }, BASE);
}

(async () => {
  const browser = await chromium.launch(launchOpts());
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
  const pageErrors = [];
  page.on('pageerror', e => pageErrors.push(e.message));

  await page.goto(BASE + '/login', { waitUntil: 'networkidle' });
  await page.locator('input').nth(0).fill(USER);
  await page.locator('input').nth(1).fill(PASS);
  await page.getByRole('button', { name: '登录' }).click();
  await page.waitForFunction(() => !location.pathname.startsWith('/login'), null, { timeout: 15000 });

  const zoneRoute = await zoneDetailRoute(page);
  if (!zoneRoute.route) throw new Error('no DNS zone fixture for /dns/zone-detail/:id');
  const allRoutes = [...routes, zoneRoute.route];
  const out = [];
  let failed = 0;
  for (const route of allRoutes) {
    pageErrors.length = 0;
    let text = '';
    try {
      await page.goto(BASE + route, { waitUntil: 'networkidle', timeout: 15000 });
      await page.waitForTimeout(350);
      text = await page.locator('body').innerText();
    } catch (e) {
      text = 'NAV_ERROR ' + e.message.slice(0, 160);
    }
    const i18n = [...new Set([...text.matchAll(I18N)].map(m => m[1]))];
    const iso = [...new Set([...text.matchAll(ISO)].map(m => m[0]))];
    const enums = [...new Set([...text.matchAll(ENUM)].map(m => m[1]))];
    const errors = [...new Set(pageErrors)];
    const findings = { route, i18n, iso, enums, errors };
    out.push(findings);
    if (i18n.length || iso.length || enums.length || errors.length || text.startsWith('NAV_ERROR')) {
      failed += 1;
      console.log('FAIL ' + route + ' ' + JSON.stringify(findings));
    }
  }

  if (zoneRoute.createdId) {
    await page.evaluate(async ({ base, id }) => {
      const token = sessionStorage.getItem('token');
      const csrf = sessionStorage.getItem('csrf_token') || '';
      await fetch(base + '/api/v1/dns/zones/' + encodeURIComponent(id), {
        method: 'DELETE', headers: { Authorization: 'Bearer ' + token, 'X-CSRF-Token': csrf }
      }).catch(() => {});
    }, { base: BASE, id: zoneRoute.createdId });
  }

  fs.mkdirSync(SHOT_DIR, { recursive: true });
  fs.writeFileSync(path.join(SHOT_DIR, 'route-audit.json'), JSON.stringify(out, null, 2));
  await browser.close();
  if (failed) {
    console.log('errors: ' + failed);
    process.exitCode = 1;
  } else {
    console.log('PASS route-audit routes=' + allRoutes.length + ' raw=0 errors=0');
  }
})().catch(e => {
  console.log('FAIL route-audit FATAL ' + e.message);
  console.log('errors: 1');
  process.exit(1);
});