const { chromium } = require('playwright');
const { BASE, USER, PASS, shot, launchOpts } = require('./common.cjs');
(async()=>{
 const b=await chromium.launch(launchOpts());
 const p=await b.newPage({viewport:{width:1440,height:900}});
 const errs=[]; p.on('pageerror',e=>errs.push('PAGEERROR: '+e.message));
 p.on('console',m=>{if(m.type()==='error'){const x=m.text().slice(0,150);if(!/501|Not Implemented/.test(x))errs.push('CONSOLE: '+x)}});
 const step=(n,ok,x='')=>console.log(`${ok?'PASS':'FAIL'} | ${n}${x?' | '+x:''}`);
 await p.goto(`${BASE}/login`,{waitUntil:'networkidle'});
 await p.locator('input').nth(0).fill(USER); await p.locator('input').nth(1).fill(PASS);
 await p.locator('button[type="submit"], .t-button--theme-primary').first().click(); await p.waitForTimeout(1800);
 step('logged in', !p.url().includes('/login'));

 await p.goto(BASE+'/dns/zones',{waitUntil:'networkidle'}); await p.waitForTimeout(1000);
 const body=await p.locator('body').innerText();
 step('zones renders', body.includes('区域管理') && (await p.locator('.t-table').count())>0 && (await p.locator('.n-data-table, .n-tabs').count())===0, `tables=${await p.locator('.t-table').count()}`);
 step('zones TDesign tabs', (await p.locator('.t-tabs').count())>0, `tabs=${await p.locator('.t-tabs').count()}`);
 step('zones selection column', (await p.locator('.t-table .t-checkbox').count())>0, `checkboxes=${await p.locator('.t-table .t-checkbox').count()}`);

 // create a zone end-to-end
 const create=p.getByRole('button',{name:/新建区域|创建区域/}).first();
 if(await create.count()){
   await create.click(); await p.waitForTimeout(500);
   const uniq='tdzone'+Date.now().toString()+Math.floor(Math.random()*1000)+'.test';
   const ins=p.locator('.t-dialog input:not([readonly])');
   await ins.first().fill(uniq);
   await p.locator('.t-dialog').getByRole('button',{name:/保存/}).first().click();
   await p.waitForTimeout(1800);
   step('zone created end-to-end', (await p.locator('body').innerText()).includes(uniq), `needle=${uniq}`);
   // Self-clean so repeated runs never push the new zone off the first page.
   const cleaned = await p.evaluate(async (name) => {
     const token = sessionStorage.getItem('token');
     const csrf = sessionStorage.getItem('csrf_token');
     const h = { Authorization: `Bearer ${token}`, 'X-CSRF-Token': csrf || '' };
     const list = await (await fetch('/api/v1/dns/zones?page_size=500', { headers: h })).json();
     const row = (list.data || []).find((x) => x.name === name || x.name === name + '.');
     if (!row) return false;
     const res = await fetch(`/api/v1/dns/zones/${row.id}`, { method: 'DELETE', headers: h });
     return res.ok;
   }, uniq);
   step('test zone cleaned up', cleaned === true, '');
 } else step('zone create button',false,'not found');

 // batch delete selection UI
 await p.goto(BASE+'/dns/zones',{waitUntil:'networkidle'}); await p.waitForTimeout(900);
 const firstCb=p.locator('.t-table .t-checkbox').first();
 if(await firstCb.count()){
   await firstCb.click(); await p.waitForTimeout(500);
   step('row selection exposes batch delete', (await p.locator('body').innerText()).includes('批量删除')||(await p.getByRole('button',{name:/批量删除/}).count())>0, '');
 } else step('row selection available',false,'no checkbox');

 await p.screenshot({path: shot('ui-batch8-zones.png'),fullPage:true});
 console.log('errors:',errs.length); errs.slice(0,8).forEach(e=>console.log('  '+e));
 await b.close();
})().catch(e=>{console.error('FATAL',e);process.exit(1)});
