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
   const naive=await p.locator('.n-data-table, .n-modal, .n-form-item, .n-statistic').count();
   step(path+' renders', body.includes(title)&&tbl>0&&naive===0, `tables=${tbl} naive=${naive}`);
 };
 await check('/admin/users','用户管理');
 await check('/admin/roles','角色管理');
 await check('/dns/cache','缓存管理');

 // stat cards + cache table
 await p.goto(BASE+'/dns/cache',{waitUntil:'networkidle'}); await p.waitForTimeout(700);
 step('cache stat cards (TDesign)', (await p.locator('.stat-card').count())===4, `cards=${await p.locator('.stat-card').count()}`);
 await p.screenshot({path: shot('ui-batch6-cache.png'),fullPage:true});

 // create a user end-to-end
 await p.goto(BASE+'/admin/users',{waitUntil:'networkidle'}); await p.waitForTimeout(700);
 const create=p.getByRole('button',{name:/新建用户|创建用户/}).first();
 if(await create.count()){
   await create.click(); await p.waitForTimeout(500);
   const uniq='tduser'+Date.now().toString()+Math.floor(Math.random()*1000);
   const ins=p.locator('.t-dialog input');
   await ins.nth(0).fill(uniq);
   await ins.nth(1).fill('Passw0rd!2026');
   await p.locator('.t-dialog').getByRole('button',{name:/保存/}).first().click();
   await p.waitForTimeout(1700);
   step('user created end-to-end', (await p.locator('body').innerText()).includes(uniq), `needle=${uniq}`);
   const uClean = await p.evaluate(async (name) => {
     const h = { Authorization: `Bearer ${sessionStorage.getItem('token')}`, 'X-CSRF-Token': sessionStorage.getItem('csrf_token') || '' };
     const list = await (await fetch('/api/v1/users?page_size=500', { headers: h })).json();
     const row = (list.data || []).find((x) => x.username === name);
     if (!row) return false;
     return (await fetch(`/api/v1/users/${row.id}`, { method: 'DELETE', headers: h })).ok;
   }, uniq);
   step('test user cleaned up', uClean === true, '');
 } else step('user create button',false,'not found');

 // create a role end-to-end
 await p.goto(BASE+'/admin/roles',{waitUntil:'networkidle'}); await p.waitForTimeout(700);
 const rc=p.getByRole('button',{name:/新建角色|创建角色/}).first();
 if(await rc.count()){
   await rc.click(); await p.waitForTimeout(500);
   const rname='tdrole'+Date.now().toString()+Math.floor(Math.random()*1000);
   await p.locator('.t-dialog input').first().fill(rname);
   await p.locator('.t-dialog').getByRole('button',{name:/保存/}).first().click();
   await p.waitForTimeout(1700);
   step('role created end-to-end', (await p.locator('body').innerText()).includes(rname), `needle=${rname}`);
   const rClean = await p.evaluate(async (name) => {
     const h = { Authorization: `Bearer ${sessionStorage.getItem('token')}`, 'X-CSRF-Token': sessionStorage.getItem('csrf_token') || '' };
     const list = await (await fetch('/api/v1/roles?page_size=500', { headers: h })).json();
     const row = (list.data || []).find((x) => x.name === name);
     if (!row) return false;
     return (await fetch(`/api/v1/roles/${row.id}`, { method: 'DELETE', headers: h })).ok;
   }, rname);
   step('test role cleaned up', rClean === true, '');
 } else step('role create button',false,'not found');

 console.log('errors:',errs.length); errs.slice(0,8).forEach(e=>console.log('  '+e));
 await b.close();
})().catch(e=>{console.error('FATAL',e);process.exit(1)});
