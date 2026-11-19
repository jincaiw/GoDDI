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
   await p.goto(BASE+path,{waitUntil:'networkidle'}); await p.waitForTimeout(800);
   const body=await p.locator('body').innerText();
   const tbl=await p.locator('.t-table').count();
   const naive=await p.locator('.n-data-table, .n-modal, .n-form-item').count();
   step(path+' renders', body.includes(title)&&tbl>0&&naive===0, `tables=${tbl} naive=${naive}`);
 };
 await check('/dhcp/options','DHCP 选项');
 await check('/dhcp/reservations','保留地址');
 await check('/settings/backup','备份管理');
 await check('/admin/tokens','API 令牌');

 // create a reservation end-to-end
 await p.goto(BASE+'/dhcp/reservations',{waitUntil:'networkidle'}); await p.waitForTimeout(700);
 const create=p.getByRole('button',{name:/新建保留|创建保留/}).first();
 if(await create.count()){
   await create.click(); await p.waitForTimeout(500);
   const ins=await p.locator('.t-dialog input').count();
   step('reservation dialog (TDesign)', ins>=3, `inputs=${ins}`);
   const suffix=Date.now().toString().slice(-4);
   let ip='';
   const fixture = await p.evaluate(async () => {
     const token=sessionStorage.getItem('token'), csrf=sessionStorage.getItem('csrf_token');
     const h={Authorization:`Bearer ${token}`, 'X-CSRF-Token':csrf||''};
     const scopes=await (await fetch('/api/v1/dhcp/scopes?page_size=100',{headers:h})).json();
     const scope=(scopes.data||[]).find(s=>s.enabled) || (scopes.data||[])[0];
     const res=await (await fetch('/api/v1/dhcp/reservations?page_size=500',{headers:h})).json();
     const leases=await (await fetch('/api/v1/dhcp/leases?page_size=500',{headers:h})).json();
     return {scope, taken:[...(res.data||[]).map(x=>x.ip_address), ...(leases.data||[]).map(x=>x.ip_address)]};
   });
   const scope=fixture.scope;
   if(scope){
     const prefix=String(scope.start_ip||'').split('.').slice(0,3).join('.');
     const start=Number(String(scope.start_ip||'').split('.').pop()||0);
     const end=Number(String(scope.end_ip||'').split('.').pop()||0);
     for(let n=start;n<=end;n++){const c=prefix+'.'+n;if(!fixture.taken.includes(c)){ip=c;break}}
   }
   if(!ip) throw new Error('no free IP in enabled DHCP scope');
   await p.locator('.t-dialog input').nth(0).fill(ip);
   const mac='02:00:00:de:ad:'+suffix.slice(0,2);
   await p.locator('.t-dialog input').nth(1).fill(mac);
   // scope select: click and choose first option
   await p.locator('.t-dialog .t-select').last().click(); await p.waitForTimeout(400);
   const opt=p.locator('.t-select__list .t-option, .t-select-option').first();
   if(await opt.count()) await opt.click();
   await p.locator('.t-dialog').getByRole('button',{name:/保存/}).first().click();
   await p.waitForTimeout(1600);
   const body=await p.locator('body').innerText();
   step('reservation created end-to-end', body.includes(mac), `ip=${ip}`);
   // Self-clean so repeated runs don't exhaust the scope's IP range.
   const cleaned = await p.evaluate(async (macAddr) => {
     const token = sessionStorage.getItem('token');
     const csrf = sessionStorage.getItem('csrf_token');
     const h = { Authorization: `Bearer ${token}`, 'X-CSRF-Token': csrf || '' };
     const list = await (await fetch('/api/v1/dhcp/reservations?page_size=500', { headers: h })).json();
     const row = (list.data || []).find((r) => r.mac_address === macAddr);
     if (!row) return false;
     const res = await fetch(`/api/v1/dhcp/reservations/${row.id}`, { method: 'DELETE', headers: h });
     return res.ok;
   }, mac);
   step('test reservation cleaned up', cleaned === true, '');
 } else step('reservation create button',false,'not found');

 // create an API token end-to-end
 await p.goto(BASE+'/admin/tokens',{waitUntil:'networkidle'}); await p.waitForTimeout(700);
 const tcreate=p.getByRole('button',{name:/新建令牌|创建令牌/}).first();
 if(await tcreate.count()){
   await tcreate.click(); await p.waitForTimeout(500);
   await p.locator('.t-dialog input').first().fill('tdtoken-'+Date.now().toString()+Math.floor(Math.random()*1000));
   await p.locator('.t-dialog').getByRole('button',{name:/保存/}).first().click();
   await p.waitForTimeout(1600);
   const body=await p.locator('body').innerText();
   step('token created (one-time value shown)', body.includes('令牌已创建')||body.includes('请妥善保存'), '');
 } else step('token create button',false,'not found');

 // create a DHCP option end-to-end (round 203 found this page render-only)
 await p.goto(BASE+'/dhcp/options',{waitUntil:'networkidle'}); await p.waitForTimeout(700);
 const ocreate=p.getByRole('button',{name:/新建选项|创建选项|新建/}).first();
 if(await ocreate.count()){
   await ocreate.click(); await p.waitForTimeout(500);
   const ovals=['15','tdopt-'+Date.now().toString().slice(-6)+'.example'];
   await p.locator('.t-dialog input').nth(0).fill(ovals[0]);
   await p.locator('.t-dialog input').nth(1).fill(ovals[1]);
   // 作用域 is a required select; pick its first option or the save fails.
   await p.locator('.t-dialog .t-select').last().click(); await p.waitForTimeout(400);
   const oopt=p.locator('.t-select__list .t-option, .t-select-option').first();
   if(await oopt.count()) await oopt.click(); await p.waitForTimeout(300);
   await p.locator('.t-dialog').getByRole('button',{name:/保存/}).first().click();
   await p.waitForTimeout(1600);
   const obody=await p.locator('body').innerText();
   step('option created end-to-end', obody.includes(ovals[1]), `value=${ovals[1]}`);
   const ocleaned = await p.evaluate(async (val) => {
     const token=sessionStorage.getItem('token'), csrf=sessionStorage.getItem('csrf_token');
     const h={Authorization:`Bearer ${token}`,'X-CSRF-Token':csrf||''};
     const list=await (await fetch('/api/v1/dhcp/options?page_size=500',{headers:h})).json();
     const row=(list.data||[]).find(x=>x.option_value===val||(x.value===val));
     if(!row) return false;
     const res=await fetch(`/api/v1/dhcp/options/${row.id}`,{method:'DELETE',headers:h});
     return res.ok;
   }, ovals[1]);
   step('test option cleaned up', ocleaned===true, '');
 } else step('option create button',false,'not found');

 // create a backup end-to-end (round 203 found this page render-only; the
 // restore path is covered by api.spec and release_flow_test.go). The button
 // creates immediately with an auto-generated `manual-` name — no dialog.
 await p.goto(BASE+'/settings/backup',{waitUntil:'networkidle'}); await p.waitForTimeout(700);
 const before = await p.evaluate(async () => {
   const token=sessionStorage.getItem('token'), csrf=sessionStorage.getItem('csrf_token');
   const h={Authorization:`Bearer ${token}`,'X-CSRF-Token':csrf||''};
   const list=await (await fetch('/api/v1/backup?page_size=200',{headers:h})).json();
   return (list.data||[]).map(x=>x.id);
 });
 const bcreate=p.getByRole('button',{name:/新建备份|创建备份|Create Backup/}).first();
 if(await bcreate.count()){
   await bcreate.click(); await p.waitForTimeout(2500);
   const bbody=await p.locator('body').innerText();
   step('backup created end-to-end', bbody.includes('manual-'), '');
   const bcleaned = await p.evaluate(async (prevIds) => {
     const token=sessionStorage.getItem('token'), csrf=sessionStorage.getItem('csrf_token');
     const h={Authorization:`Bearer ${token}`,'X-CSRF-Token':csrf||''};
     const list=await (await fetch('/api/v1/backup?page_size=200',{headers:h})).json();
     const rows=(list.data||[]).filter(x=>!prevIds.includes(x.id) && String(x.description||'').startsWith('manual-'));
     let ok=rows.length>0;
     for(const row of rows){
       const res=await fetch(`/api/v1/backup/${row.id}`,{method:'DELETE',headers:h});
       ok=ok&&res.ok;
     }
     return ok;
   }, before);
   step('test backup cleaned up', bcleaned===true, '');
 } else step('backup create button',false,'not found');

 await p.waitForTimeout(600);
 await p.screenshot({path: shot('ui-batch4-backup.png'),fullPage:true});
 console.log('errors:' ,errs.length); errs.slice(0,8).forEach(e=>console.log('  '+e));
 await b.close();
})().catch(e=>{console.error('FATAL',e);process.exit(1)});