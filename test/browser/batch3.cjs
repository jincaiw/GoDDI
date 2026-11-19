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

 // 1. dns/security
 await p.goto(BASE+'/dns/security',{waitUntil:'networkidle'}); await p.waitForTimeout(900);
 let body=await p.locator('body').innerText();
 step('security renders', body.includes('客户端策略') && (await p.locator('.t-table').count())>0 && (await p.locator('.n-data-table').count())===0, `tables=${await p.locator('.t-table').count()}`);
 const create=p.getByRole('button',{name:/新建客户端策略|新建策略|创建策略/}).first();
 const hasCreate=await create.count();
 if(hasCreate){
   await create.click(); await p.waitForTimeout(500);
   const dlgInputs=await p.locator('.t-dialog input').count();
   step('security create dialog (TDesign)', dlgInputs>=2, `inputs=${dlgInputs}`);
   const uniq='tdsec-'+Date.now().toString()+Math.floor(Math.random()*1000);
   await p.locator('.t-dialog input').nth(0).fill(uniq);
   await p.locator('.t-dialog input').nth(1).fill('10.88.0.0/24');
   await p.locator('.t-dialog').getByRole('button',{name:/保存/}).first().click();
   await p.waitForTimeout(1600);
   body=await p.locator('body').innerText();
   step('security policy created end-to-end', body.includes(uniq), `needle=${uniq}`);
   // Self-clean so repeated runs never push the new row off the first page.
   const cleaned = await p.evaluate(async (name) => {
     const token = sessionStorage.getItem('token');
     const csrf = sessionStorage.getItem('csrf_token');
     const h = { Authorization: `Bearer ${token}`, 'X-CSRF-Token': csrf || '' };
     const list = await (await fetch('/api/v1/dns/security/policies?page_size=500', { headers: h })).json();
     const row = (list.data || []).find((x) => x.name === name);
     if (!row) return false;
     const res = await fetch(`/api/v1/dns/security/policies/${row.id}`, { method: 'DELETE', headers: h });
     return res.ok;
   }, uniq);
   step('test policy cleaned up', cleaned === true, '');
   await p.screenshot({path: shot('ui-batch3-security.png'),fullPage:true});
 } else { step('security create button present',false,'RBAC or i18n mismatch'); }

 // 2. dhcp/leases
 await p.goto(BASE+'/dhcp/leases',{waitUntil:'networkidle'}); await p.waitForTimeout(900);
 body=await p.locator('body').innerText();
  const leaseText=await p.locator('.td-page').first().innerText().catch(()=>'');
  step('leases renders', (leaseText.includes('租约')||body.includes('租约')) && (await p.locator('.t-table').count())>0 && (await p.locator('.n-data-table').count())===0);
 const filters=await p.locator('.filter-bar input, .filter-bar .t-select').count();
 step('leases filters (TDesign)', filters>=1, `controls=${filters}`);
 await p.screenshot({path: shot('ui-batch3-leases.png'),fullPage:true});

 // 3. settings/system
 await p.goto(BASE+'/settings/system',{waitUntil:'networkidle'}); await p.waitForTimeout(1000);
 body=await p.locator('body').innerText();
 const rows=await p.locator('.setting-row').count();
 const naive=await p.locator('.n-form-item, .n-switch, .n-input').count();
 step('settings renders', rows>0 && naive===0, `rows=${rows} naive=${naive}`);
 const saveBtns=await p.locator('.setting-row').getByRole('button',{name:/保存/}).count();
 step('settings save buttons (RBAC)', saveBtns>0, `count=${saveBtns}`);
 // Save one setting end-to-end and restore it (round 207: this page was
 // render-only before). dns_cache_min_ttl is an int setting that is safe to
 // wiggle: the cache clamps, nothing durable depends on the exact value.
 const row=p.locator('.setting-row',{hasText:'缓存最小 TTL'}).first();
 if(await row.count()){
   const orig=await p.evaluate(async()=>{
     const token=sessionStorage.getItem('token'), csrf=sessionStorage.getItem('csrf_token');
     const h={Authorization:`Bearer ${token}`,'X-CSRF-Token':csrf||''};
     const j=await (await fetch('/api/v1/settings',{headers:h})).json();
     const s=(j.data||[]).find(x=>x.key==='dns_cache_min_ttl');
     return s?String(s.value):null;
   });
   if(orig!==null){
     const wiggle=orig==='95'?'96':'95';
     await row.locator('input').fill(wiggle);
     await row.getByRole('button',{name:/保存/}).click();
     await p.waitForTimeout(1200);
     const after=await p.evaluate(async()=>{
       const token=sessionStorage.getItem('token'), csrf=sessionStorage.getItem('csrf_token');
       const h={Authorization:`Bearer ${token}`,'X-CSRF-Token':csrf||''};
       const j=await (await fetch('/api/v1/settings',{headers:h})).json();
       const s=(j.data||[]).find(x=>x.key==='dns_cache_min_ttl');
       return s?String(s.value):null;
     });
     step('setting saved end-to-end', after===wiggle, `dns_cache_min_ttl=${orig}->${after}`);
     // Restore the original value through the same UI path.
     await row.locator('input').fill(orig);
     await row.getByRole('button',{name:/保存/}).click();
     await p.waitForTimeout(1200);
     const restored=await p.evaluate(async()=>{
       const token=sessionStorage.getItem('token'), csrf=sessionStorage.getItem('csrf_token');
       const h={Authorization:`Bearer ${token}`,'X-CSRF-Token':csrf||''};
       const j=await (await fetch('/api/v1/settings',{headers:h})).json();
       const s=(j.data||[]).find(x=>x.key==='dns_cache_min_ttl');
       return s?String(s.value):null;
     });
     step('setting restored', restored===orig, `back to ${restored}`);
   } else step('dns_cache_min_ttl found',false,'missing from /settings');
 } else step('settings row locator',false,'缓存最小 TTL row not found');
 await p.screenshot({path: shot('ui-batch3-settings.png'),fullPage:true});

 console.log('errors:',errs.length); errs.slice(0,6).forEach(e=>console.log('  '+e));
 await b.close();
})().catch(e=>{console.error('FATAL',e);process.exit(1)});