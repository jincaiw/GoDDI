const { chromium } = require('playwright');
const { BASE, USER, PASS, shot, launchOpts } = require('./common.cjs');

(async () => {
  const browser = await chromium.launch({
    ...launchOpts(),
    headless: true,
    args: ['--no-sandbox', '--disable-dev-shm-usage']
  });
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
  const errors = [];
  page.on('pageerror', e => errors.push('PAGEERROR: ' + e.message));
  page.on('console', m => { if (m.type() === 'error') errors.push('CONSOLE: ' + m.text().slice(0, 200)); });

  const step = (name, ok, extra = '') =>
    console.log(`${ok ? 'PASS' : 'FAIL'} | ${name}${extra ? ' | ' + extra : ''}`);

  // 1. Login page renders with TDesign
  await page.goto(`${BASE}/login`, { waitUntil: 'networkidle', timeout: 30000 });
  await page.waitForTimeout(1200);
  const title = await page.title();
  const loginText = await page.locator('body').innerText();
  const hasTDesignLogin = await page.locator('.t-form, .t-input, .t-button').count();
  step('login page loads', title.length > 0, `title="${title}" (document.title is i18n by router guard)`);
  step('login page uses TDesign components', hasTDesignLogin > 0, `matched=${hasTDesignLogin}`);
  step('login page has form fields', (await page.locator('input').count()) >= 2, `inputs=${await page.locator('input').count()}`);
  await page.screenshot({ path: shot('ui-login.png'), fullPage: true });

  // 2. Log in with real API
  const inputs = page.locator('input');
  await inputs.nth(0).fill(USER);
  await inputs.nth(1).fill(PASS);
  await page.locator('button[type="submit"], .t-button--theme-primary').first().click();
  await page.waitForTimeout(2500);
  const urlAfter = page.url();
  step('login succeeds (redirect away from /login)', !urlAfter.includes('/login'), `url=${urlAfter}`);
  step('session token stored', await page.evaluate(() => !!sessionStorage.getItem('token')), '');
  await page.screenshot({ path: shot('ui-after-login.png'), fullPage: true });

  // 3. TDesign shell (header/aside/menu) present
  const shell = {
    layout: await page.locator('.t-layout').count(),
    aside: await page.locator('.t-aside, .t-layout__sider').count(),
    menu: await page.locator('.t-menu').count(),
    header: await page.locator('.t-layout__header, .t-header').count()
  };
  step('TDesign shell renders', shell.layout > 0 && shell.menu > 0, JSON.stringify(shell));
  step('Single-Node badge visible', (await page.locator('body').innerText()).includes('Single-Node'), '');

  // 4. Sample page: IPAM Spaces
  await page.goto(`${BASE}/ipam/spaces`, { waitUntil: 'networkidle', timeout: 30000 });
  await page.waitForTimeout(1800);
  const bodyText = await page.locator('body').innerText();
  step('IPAM spaces page renders', bodyText.includes('地址空间'), bodyText.slice(0, 120).replace(/\n/g, ' | '));
  const tableCount = await page.locator('.t-table').count();
  step('TDesign table renders', tableCount > 0, `tables=${tableCount}`);
  await page.screenshot({ path: shot('ui-spaces.png'), fullPage: true });

  // 5. Create a space end-to-end
  const createBtn = page.getByRole('button', { name: /新建地址空间/ }).first();
  const canCreate = await createBtn.count();
  step('create button visible (RBAC ok)', canCreate > 0, `count=${canCreate}`);
  if (canCreate) {
    await createBtn.click();
    await page.waitForTimeout(600);
    const dialogInputs = page.locator('.t-dialog input, .t-dialog textarea');
    const nDialogInputs = await dialogInputs.count();
    step('TDesign dialog with form opens', nDialogInputs >= 1, `inputs=${nDialogInputs}`);
    if (nDialogInputs >= 1) {
      const uniq = 'tdesign-' + Date.now().toString()+Math.floor(Math.random()*1000);
      await dialogInputs.nth(0).fill(uniq);
      if (nDialogInputs >= 2) await dialogInputs.nth(1).fill('created by browser smoke test');
      await page.locator('.t-dialog').getByRole('button', { name: /保存/ }).first().click();
      await page.waitForTimeout(2000);
      const after = await page.locator('body').innerText();
      step('created space appears in table', after.includes(uniq), `needle=${uniq}`);
    }
    await page.screenshot({ path: shot('ui-spaces-created.png'), fullPage: true });
  }

  console.log('--- browser errors (' + errors.length + ') ---');
  errors.slice(0, 12).forEach(e => console.log('  ' + e));
  step('no uncaught page errors', errors.filter(e => e.startsWith('PAGEERROR')).length === 0, '');

  await browser.close();
})().catch(e => { console.error('FATAL', e); process.exit(1); });
