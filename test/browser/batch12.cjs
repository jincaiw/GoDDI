const { chromium } = require('playwright');
const { BASE, USER, PASS, shot, launchOpts } = require('./common.cjs');
(async()=>{
 const b=await chromium.launch(launchOpts());
 const p=await b.newPage({viewport:{width:1440,height:900}});
 const errs=[]; p.on('pageerror',e=>errs.push('PAGEERROR: '+e.message));
 p.on('console',m=>{if(m.type()==='error'){const x=m.text().slice(0,160);if(!/501|Not Implemented|404 \(Not Found\)/.test(x))errs.push('CONSOLE: '+x)}});
 const step=(n,ok,x='')=>console.log(`${ok?'PASS':'FAIL'} | ${n}${x?' | '+x:''}`);
 const api=async (method,path,body)=>await p.evaluate(async ({method,path,body})=>{
   const token=sessionStorage.getItem('token'), csrf=sessionStorage.getItem('csrf_token');
   const h={'Content-Type':'application/json',Authorization:`Bearer ${token}`,'X-CSRF-Token':csrf||''};
   const r=await fetch('/api/v1'+path,{method,headers:h,body:body?JSON.stringify(body):undefined});
   return {status:r.status, data:await r.json().catch(()=>null)};
 },{method,path,body});
 await p.goto(`${BASE}/login`,{waitUntil:'networkidle'});
 await p.locator('input').nth(0).fill(USER); await p.locator('input').nth(1).fill(PASS);
 await p.locator('button[type="submit"], .t-button--theme-primary').first().click(); await p.waitForTimeout(1800);
 step('logged in', !p.url().includes('/login'));

 // create a zone via API to get a real id
 const z=await api('POST','/dns/zones',{name:'tddetail'+Date.now().toString()+Math.floor(Math.random()*1000)+'.test.',type:'primary'});
 const zid=z.data?.data?.id;
 step('seed zone created', !!zid, `id=${zid}`);

 await p.goto(BASE+`/dns/zone-detail/${zid}`,{waitUntil:'networkidle'}); await p.waitForTimeout(1200);
 const body=await p.locator('body').innerText();
 step('zone-detail renders TDesign', (await p.locator('.t-tabs').count())>0 && (await p.locator('.t-table').count())>0 && (await p.locator('.n-tabs, .n-data-table').count())===0, `tabs=${await p.locator('.t-tabs').count()} tables=${await p.locator('.t-table').count()}`);

 // create a record end-to-end
 const add=p.getByRole('button',{name:/新建记录|创建记录|添加记录/}).first();
 if(await add.count()){
   await add.click(); await p.waitForTimeout(600);
   const uniq='www'+Date.now().toString()+Math.floor(Math.random()*1000);
   const ins=p.locator('.t-dialog:visible input:not([readonly]):not([type=file])');
   await ins.nth(0).fill(uniq);
   await p.locator('.t-dialog:visible textarea').first().fill('192.0.2.99');
   await p.locator('.t-dialog:visible').getByRole('button',{name:/保存/}).first().click();
   await p.waitForTimeout(1800);
   step('record created end-to-end', (await p.locator('body').innerText()).includes('192.0.2.99'), '');
 } else step('add record button',false,'not found');

 // switch tabs: options / permissions / history / dnssec
 for(const [label,pattern] of [['options',/选项/],['permissions',/权限/],['history',/历史/],['dnssec',/DNSSEC/]]){
   const tab=p.locator('.t-tabs__nav-item').filter({hasText:pattern}).first();
   if(await tab.count()){
     await tab.click(); await p.waitForTimeout(800);
     step('tab '+label+' loads', (await p.locator('body').innerText()).length>200, `len=${(await p.locator('body').innerText()).length}`);
   } else step('tab '+label,false,'not found');
 }
 await p.screenshot({path: shot('ui-batch12-detail.png'),fullPage:true});
 console.log('errors:',errs.length); errs.slice(0,8).forEach(e=>console.log('  '+e));
 await b.close();
})().catch(e=>{console.error('FATAL',e);process.exit(1)});
