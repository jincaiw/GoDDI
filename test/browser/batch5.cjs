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

 const check=async(path,title)=>{
   await p.goto(BASE+path,{waitUntil:'networkidle'}); await p.waitForTimeout(900);
   const body=await p.locator('body').innerText();
   const tbl=await p.locator('.t-table').count();
   const naive=await p.locator('.n-data-table, .n-modal, .n-form-item, .n-progress').count();
   step(path+' renders', body.includes(title)&&tbl>0&&naive===0, `tables=${tbl} naive=${naive}`);
 };
 await check('/admin/sessions','会话管理');
 await check('/dhcp/scopes','地址池');
 await check('/dns/allowed','允许区域');
 await check('/admin/groups','组管理');

 // progress bar in scopes table (TDesign)
 await p.goto(BASE+'/dhcp/scopes',{waitUntil:'networkidle'}); await p.waitForTimeout(800);
 step('scopes TDesign progress', (await p.locator('.t-progress').count())>0, `progress=${await p.locator('.t-progress').count()}`);

 // create a group end-to-end
 await p.goto(BASE+'/admin/groups',{waitUntil:'networkidle'}); await p.waitForTimeout(700);
 const create=p.getByRole('button',{name:/新建组|创建组/}).first();
 if(await create.count()){
   await create.click(); await p.waitForTimeout(500);
   const uniq='tdgroup-'+Date.now().toString()+Math.floor(Math.random()*1000);
   await p.locator('.t-dialog input').first().fill(uniq);
   await p.locator('.t-dialog').getByRole('button',{name:/保存/}).first().click();
   await p.waitForTimeout(1600);
   step('group created end-to-end', (await p.locator('body').innerText()).includes(uniq), `needle=${uniq}`);
   const gClean = await p.evaluate(async (name) => {
     const h = { Authorization: `Bearer ${sessionStorage.getItem('token')}`, 'X-CSRF-Token': sessionStorage.getItem('csrf_token') || '' };
     const list = await (await fetch('/api/v1/groups?page_size=500', { headers: h })).json();
     const row = (list.data || []).find((x) => x.name === name);
     if (!row) return false;
     return (await fetch(`/api/v1/groups/${row.id}`, { method: 'DELETE', headers: h })).ok;
   }, uniq);
   step('test group cleaned up', gClean === true, '');
 } else step('group create button',false,'not found');

 // add an allow rule end-to-end
 await p.goto(BASE+'/dns/allowed',{waitUntil:'networkidle'}); await p.waitForTimeout(700);
 const add=p.getByRole('button',{name:/添加允许|新增允许|添加规则/}).first();
 if(await add.count()){
   await add.click(); await p.waitForTimeout(500);
   const domain='td-allow-'+Date.now().toString()+Math.floor(Math.random()*1000)+'.test';
   await p.locator('.t-dialog input').first().fill(domain);
   await p.locator('.t-dialog').getByRole('button',{name:/保存/}).first().click();
   await p.waitForTimeout(1600);
   step('allow rule added end-to-end', (await p.locator('body').innerText()).includes(domain), `needle=${domain}`);
   const aClean = await p.evaluate(async (pattern) => {
     const h = { Authorization: `Bearer ${sessionStorage.getItem('token')}`, 'X-CSRF-Token': sessionStorage.getItem('csrf_token') || '' };
     const list = await (await fetch('/api/v1/dns/security/allowlists?page_size=500', { headers: h })).json();
     const row = (list.data || []).find((x) => x.pattern === pattern || x.pattern === pattern + '.');
     if (!row) return false;
     return (await fetch(`/api/v1/dns/security/allowlists/${row.id}`, { method: 'DELETE', headers: h })).ok;
   }, domain);
   step('test allow rule cleaned up', aClean === true, '');
 } else step('allow rule add button',false,'not found');

 await p.screenshot({path: shot('ui-batch5-scopes.png'),fullPage:true});
 console.log('errors:',errs.length); errs.slice(0,8).forEach(e=>console.log('  '+e));

 // HA status page (round 286): real page + read-only API + refresh button.
 await p.goto(BASE+'/dhcp/ha',{waitUntil:'networkidle'}); await p.waitForTimeout(1200);
 const haBody = await p.locator('body').innerText();
 step('/dhcp/ha renders', haBody.includes('HA 状态') || haBody.includes('HA Status'), `chars=${haBody.length}`);
 const haApi = await p.evaluate(async () => {
   const h = { Authorization: `Bearer ${sessionStorage.getItem('token')}` };
   const r = await fetch('/api/v1/dhcp/ha', { headers: h });
   if (!r.ok) return { ok: false, status: r.status };
   const d = await r.json();
   return { ok: true, ha_enabled: d?.data?.ha_enabled };
 });
 step('/dhcp/ha API is read-only 200', haApi.ok === true && typeof haApi.ha_enabled === 'boolean', JSON.stringify(haApi));
 const haRefresh = p.getByRole('button',{name:/刷新|Refresh/}).first();
 if (await haRefresh.count()) {
   await haRefresh.click(); await p.waitForTimeout(900);
   step('/dhcp/ha refresh works', (await p.locator('body').innerText()).length > 30, '');
 }
 await b.close();
})().catch(e=>{console.error('FATAL',e);process.exit(1)});
