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
   const naive=await p.locator('.n-data-table, .n-modal, .n-form-item, .n-tabs').count();
   step(path+' renders', body.includes(title)&&tbl>0&&naive===0, `tables=${tbl} naive=${naive}`);
 };
 await check('/dns/forwarders','转发器');
 await check('/dns/blocked','阻断区域');
 await p.goto(BASE+'/tools/client',{waitUntil:'networkidle'}); await p.waitForTimeout(700);
 step('/tools/client renders', (await p.locator('body').innerText()).includes('DNS 查询工具') && (await p.locator('.n-form-item, .n-card').count())===0 && (await p.locator('.t-form').count())>0, `forms=${await p.locator('.t-form').count()}`);

 // forwarders tabs + create forwarder end-to-end
 await p.goto(BASE+'/dns/forwarders',{waitUntil:'networkidle'}); await p.waitForTimeout(700);
 step('forwarders TDesign tabs', (await p.locator('.t-tabs').count())>0, `tabs=${await p.locator('.t-tabs').count()}`);
 const create=p.getByRole('button',{name:/新建转发器|创建转发器/}).first();
 if(await create.count()){
   await create.click(); await p.waitForTimeout(500);
   const uniq='tdfwd'+Date.now().toString()+Math.floor(Math.random()*1000);
   const ins=p.locator('.t-dialog input:not([readonly])');
   await ins.nth(0).fill(uniq);
   await ins.nth(1).fill('198.18.0.53:53');
   await p.locator('.t-dialog').getByRole('button',{name:/保存/}).first().click();
   await p.waitForTimeout(1700);
   step('forwarder created end-to-end', (await p.locator('body').innerText()).includes(uniq), `needle=${uniq}`);
   // Self-clean: delete the test forwarder so repeated runs don't accumulate
   // unreachable upstreams and make the DNS debug client fail.
   const cleaned = await p.evaluate(async (name) => {
     const token = sessionStorage.getItem('token');
     const csrf = sessionStorage.getItem('csrf_token');
     const h = { Authorization: `Bearer ${token}`, 'X-CSRF-Token': csrf || '' };
     const list = await (await fetch('/api/v1/dns/forwarders?page_size=100', { headers: h })).json();
     const row = (list.data || []).find((f) => f.name === name);
     if (!row) return false;
     const res = await fetch(`/api/v1/dns/forwarders/${row.id}`, { method: 'DELETE', headers: h });
     return res.ok;
   }, uniq);
   step('test forwarder cleaned up', cleaned === true, '');
 } else step('forwarder create button',false,'not found');
 await p.screenshot({path: shot('ui-batch7-forwarders.png'),fullPage:true});

 // dns client query end-to-end (real DNS query against local zone)
 await p.goto(BASE+'/tools/client',{waitUntil:'networkidle'}); await p.waitForTimeout(600);
 const qinput=p.locator('.td-page input').first();
 await qinput.fill('www.demo.test');
 await p.getByRole('button',{name:/执行查询|查询/}).first().click();
 await p.waitForTimeout(2000);
 const qbody=await p.locator('body').innerText();
 step('dns client query returns response', qbody.includes('ms')||qbody.includes('NXDOMAIN')||qbody.includes('response'), qbody.slice(-140).replace(/\n/g,' | '));

 // blocked page status bar
 await p.goto(BASE+'/dns/blocked',{waitUntil:'networkidle'}); await p.waitForTimeout(600);
 step('blocked page has disable bar', (await p.locator('body').innerText()).includes('临时关闭')||(await p.locator('.t-select').count())>0, '');

 console.log('errors:',errs.length); errs.slice(0,8).forEach(e=>console.log('  '+e));
 await b.close();
})().catch(e=>{console.error('FATAL',e);process.exit(1)});