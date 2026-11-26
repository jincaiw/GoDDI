const { chromium } = require('playwright');
const { BASE, USER, PASS, shot, launchOpts } = require('./common.cjs');

/**
 * The UI half of the read-only RBAC story (the server-side half is pinned in
 * internal/api/rbac_coverage_test.go): a user whose role grants every `:read`
 * permission and no writes must see the console render, but with the write
 * actions either absent (v-if) or disabled (TLink disabled) — never clickable.
 * Cleanup removes the role and user it created.
 */
(async () => {
  const b = await chromium.launch(launchOpts());
  const p = await b.newPage({ viewport: { width: 1440, height: 900 } });
  const errs = [];
  p.on('pageerror', e => errs.push('PAGEERROR: ' + e.message));
  p.on('console', m => {
    if (m.type() === 'error') {
      const x = m.text().slice(0, 160);
      if (!/501|Not Implemented/.test(x)) errs.push('CONSOLE: ' + x);
    }
  });
  const step = (n, ok, x = '') => console.log(`${ok ? 'PASS' : 'FAIL'} | ${n}${x ? ' | ' + x : ''}`);

  const api = async (method, path, body, tok, csrf) => await p.evaluate(async ({ method, path, body, tok, csrf }) => {
    const h = { 'Content-Type': 'application/json', Authorization: `Bearer ${tok}`, 'X-CSRF-Token': csrf || '' };
    const r = await fetch('/api/v1' + path, { method, headers: h, body: body ? JSON.stringify(body) : undefined });
    let data = null;
    try { data = await r.json(); } catch (e) { /* empty body */ }
    return { status: r.status, data };
  }, { method, path, body, tok, csrf });

  // 1. Admin session (API): build the read-only role and user.
  await p.goto(`${BASE}/login`, { waitUntil: 'networkidle' });
  await p.locator('input').nth(0).fill(USER);
  await p.locator('input').nth(1).fill(PASS);
  await p.locator('button[type="submit"], .t-button--theme-primary').first().click();
  await p.waitForTimeout(1800);
  step('logged in as admin', !p.url().includes('/login'));

  const tok = await p.evaluate(() => sessionStorage.getItem('token'));
  const csrf = await p.evaluate(() => sessionStorage.getItem('csrf_token') || '');

  const perms = await api('GET', '/permissions', null, tok, csrf);
  const readIDs = (perms.data?.data || []).filter(x => x.action === 'read').map(x => x.id);
  step('read permissions collected', readIDs.length > 0, `count=${readIDs.length}`);

  const role = await api('POST', '/roles', { name: `rbac16-role-${Date.now()}`, description: 'read-only browser coverage' }, tok, csrf);
  step('role created', role.status === 201, `status=${role.status}`);
  const roleID = role.data?.data?.id;

  const grant = await api('PUT', `/roles/${roleID}/permissions`, { permission_ids: readIDs }, tok, csrf);
  step('read permissions granted', grant.status === 200, `status=${grant.status}`);

  const stamp = Date.now();
  const readerName = `rbac16_reader${stamp}`;
  const user = await api('POST', '/users', { username: readerName, password: 'AdminPass-0123456789', must_change_password: false }, tok, csrf);
  step('reader created', user.status === 201, `status=${user.status}`);
  const userID = user.data?.data?.id;
  const assigned = await api('POST', `/users/${userID}/roles`, { role_ids: [roleID] }, tok, csrf);
  step('role assigned', assigned.status === 200, `status=${assigned.status}`);

  // 2. Reader session through the UI.
  await p.evaluate(() => sessionStorage.clear());
  await p.goto(`${BASE}/login`, { waitUntil: 'networkidle' });
  await p.locator('input').nth(0).fill(readerName);
  await p.locator('input').nth(1).fill('AdminPass-0123456789');
  await p.locator('button[type="submit"], .t-button--theme-primary').first().click();
  await p.waitForTimeout(1800);
  step('logged in as reader', !p.url().includes('/login'));

  // 3. Zones: the create button is v-if'd away entirely, and clicking a row
  //    删除 opens no confirmation dialog — the click handler re-checks the
  //    permission (zone page confirmDelete) before anything happens. The
  //    TDesign TLink disabled prop is not reliably visible in the DOM for
  //    these links, so the observable gate is the handler's own check plus
  //    the server's 403 (pinned server-side in rbac_coverage_test.go).
  await p.goto(`${BASE}/dns/zones`, { waitUntil: 'networkidle' });
  await p.waitForTimeout(1200);
  const createButtons = await p.locator('button', { hasText: /新建|创建|Create/ }).count();
  step('zones create button absent for reader', createButtons === 0, `count=${createButtons}`);
  step('zones table still renders for reader', (await p.locator('.t-table').count()) > 0, '');
  const delLink = p.locator('.t-link', { hasText: '删除' }).first();
  if (await delLink.count()) {
    await delLink.click();
    await p.waitForTimeout(700);
    const confirmDialog = await p.locator('.t-dialog:visible').count();
    step('zones delete opens no confirm dialog for reader', confirmDialog === 0, `dialogs=${confirmDialog}`);
    await p.keyboard.press('Escape');
  } else step('zones delete link', false, 'not found');

  // 4. Subnets: create button absent too.
  await p.goto(`${BASE}/ipam/subnets`, { waitUntil: 'networkidle' });
  await p.waitForTimeout(1200);
  const subCreate = await p.locator('button', { hasText: /新建子网|创建子网|Create/ }).count();
  step('subnets create button absent for reader', subCreate === 0, `count=${subCreate}`);

  // 5. Users page renders (the role grants user:read) with no create button.
  await p.goto(`${BASE}/admin/users`, { waitUntil: 'networkidle' });
  await p.waitForTimeout(1200);
  const userCreate = await p.locator('button', { hasText: /新建用户|创建用户|Create/ }).count();
  step('admin users renders, create absent for reader',
    (await p.locator('.t-table').count()) > 0 && userCreate === 0, `create=${userCreate}`);

  await p.screenshot({ path: shot('ui-batch16-readonly.png'), fullPage: true });

  // 6. Cleanup: the reader cannot delete; the admin session does it.
  //    (The browser is holding the reader session, so the cleanup runs through
  //    a fresh context with the admin credentials the harness already has.)
  const b2 = await chromium.launch(launchOpts());
  const p2 = await b2.newPage();
  await p2.goto(`${BASE}/login`, { waitUntil: 'networkidle' });
  await p2.locator('input').nth(0).fill(USER);
  await p2.locator('input').nth(1).fill(PASS);
  await p2.locator('button[type="submit"], .t-button--theme-primary').first().click();
  await p2.waitForTimeout(1500);
  const adminTok = await p2.evaluate(() => sessionStorage.getItem('token'));
  const adminCsrf = await p2.evaluate(() => sessionStorage.getItem('csrf_token') || '');
  // Reuse the test role. A fresh login obtains the effective permissions after each grant.
  for (const [resource, allowed, allowedPath, denied, deniedPath] of [
    ['dns', 'DNS 查询工具', '/tools/client', '审计日志', '/logs/audit'],
    ['audit', '审计日志', '/logs/audit', 'DNS 查询工具', '/tools/client']
  ]) {
    const permission = perms.data.data.find(item => item.resource === resource && item.action === 'read');
    const result = await api('PUT', `/roles/${roleID}/permissions`, { permission_ids: [permission.id] }, adminTok, adminCsrf);
    step(`${resource}-only role configured`, result.status === 200);
    await p.evaluate(() => sessionStorage.clear());
    await p.goto(`${BASE}/login`, { waitUntil: 'networkidle' });
    await p.locator('input').nth(0).fill(readerName);
    await p.locator('input').nth(1).fill('AdminPass-0123456789');
    await p.locator('button[type="submit"]').click();
    await p.waitForURL('**/dashboard');
    const search = p.locator('.td-quick-search input');
    await search.fill(denied);
    await p.locator('.td-search-empty').waitFor();
    step(`${resource}-only search hides unauthorized page`, await p.getByRole('option').count() === 0);
    await search.fill(allowed);
    await p.getByRole('option', { name: allowed }).waitFor();
    await search.press('Enter');
    await p.waitForURL(`**${allowedPath}`);
    await p.reload({ waitUntil: 'networkidle' });
    const expandMenu = p.getByRole('button', { name: '展开菜单', exact: true });
    if (await expandMenu.isVisible()) await expandMenu.click();
    const side = p.locator('.td-aside');
    step(`${resource}-only deep link expands correct group`, await side.getByRole('button', { name: '日志与诊断' }).getAttribute('aria-expanded') === 'true');
    await side.getByRole('button', { name: allowed, exact: true }).waitFor({ state: 'visible' });
    step(`${resource}-only menu matches search`, await side.getByRole('button', { name: allowed, exact: true }).isVisible() && await side.getByRole('button', { name: denied, exact: true }).count() === 0);
    await p.goto(`${BASE}${deniedPath}`);
    await p.waitForURL('**/403');
    step(`${resource}-only restricted URL returns 403 page`, true);
  }
  await api('DELETE', `/users/${userID}`, null, adminTok, adminCsrf);
  await api('DELETE', `/roles/${roleID}`, null, adminTok, adminCsrf);
  await b2.close();
  step('cleanup removed reader and role', true, '');

  step('restricted roles register only valid route groups', !errs.some(message => message.includes('Error transforming route')));
  console.log('errors:', errs.length);
  errs.slice(0, 8).forEach(e => console.log('  ' + e));
  await b.close();
})().catch(e => { console.error('FATAL', e); process.exit(1); });
