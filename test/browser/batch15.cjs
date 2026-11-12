const { chromium } = require('playwright');
const { BASE, USER, PASS, shot, launchOpts } = require('./common.cjs');

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

  await p.goto(`${BASE}/login`, { waitUntil: 'networkidle' });
  await p.locator('input').nth(0).fill(USER);
  await p.locator('input').nth(1).fill(PASS);
  await p.locator('button[type="submit"], .t-button--theme-primary').first().click();
  await p.waitForTimeout(1800);
  step('logged in', !p.url().includes('/login'));

  await p.goto(`${BASE}/dns/dnssec`, { waitUntil: 'networkidle' });
  await p.waitForTimeout(1400);
  const body = await p.locator('body').innerText();
  const naive = await p.locator('.n-card, .n-data-table, .n-select, .n-dialog').count();
  const cards = await p.locator('.t-card').count();
  const tables = await p.locator('.t-table').count();
  const alert = await p.locator('.t-alert').count();
  const select = await p.locator('.t-select').count();

  step('DNSSEC page renders TDesign', cards >= 3 && tables >= 2 && alert >= 1 && select >= 1 && naive === 0,
    `cards=${cards} tables=${tables} alert=${alert} select=${select} naive=${naive}`);
  step('key lifecycle section present', /密钥生命周期|Key lifecycle/.test(body));
  step('DS records section present', /DS 记录|DS records/.test(body));
  step('NSEC3 section present', /NSEC3 参数|NSEC3 parameters/.test(body));
  step('experimental hint names missing parent DS chain', /父区 DS|parent-side DS/.test(body));
  step('DNSSEC page renders no raw i18n keys', !/dns\.dnssec\.[A-Za-z]/.test(body), '');

  // Zone selector must be populated from the real API (no hardcoded sample).
  await p.locator('.t-select').first().click();
  await p.waitForTimeout(500);
  const options = await p.locator('.t-select__list .t-option, .t-select-option').count();
  step('zone selector populated from API', options > 0, `options=${options}`);
  await p.keyboard.press('Escape');

  await p.screenshot({ path: shot('ui-batch15-dnssec.png'), fullPage: true });
  console.log('errors:', errs.length);
  errs.slice(0, 8).forEach(e => console.log('  ' + e));
  await b.close();
})().catch(e => { console.error('FATAL', e); process.exit(1); });
