const { chromium } = require('playwright');
const { BASE, USER, PASS, shot, launchOpts } = require('./common.cjs');
(async()=>{
 const b=await chromium.launch(launchOpts());
 const p=await b.newPage({viewport:{width:1440,height:900}});
 const errs=[]; p.on('pageerror',e=>errs.push('PAGEERROR: '+e.message));
 p.on('console',m=>{if(m.type()==='error'){const x=m.text().slice(0,150);if(!/501|Not Implemented/.test(x))errs.push('CONSOLE: '+x)}});
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

 const check=async(path,title)=>{
   await p.goto(BASE+path,{waitUntil:'networkidle'}); await p.waitForTimeout(900);
   const body=await p.locator('body').innerText();
   const tbl=await p.locator('.t-table').count();
   const naive=await p.locator('.n-data-table, .n-modal, .n-form-item, .n-drawer').count();
   step(path+' renders TDesign', body.includes(title)&&tbl>0&&naive===0, `tables=${tbl} naive=${naive}`);
 };
 await check('/ipam/addresses','IP 地址');
 await check('/ipam/subnets','子网管理');

 // create a subnet end-to-end via TDesign page
 let createdSubnetName='';
 await p.goto(BASE+'/ipam/subnets',{waitUntil:'networkidle'}); await p.waitForTimeout(700);
 const create=p.getByRole('button',{name:/新建子网|创建子网/}).first();
 if(await create.count()){
   await create.click(); await p.waitForTimeout(500);
   const uniq='tdsub'+Date.now().toString()+Math.floor(Math.random()*1000);
   createdSubnetName=uniq;
   // Pick a /24 that no existing subnet occupies. batch9 seeds a subnet with
   // a random octet in 70..149 and does not delete it, so a random choice here
   // used to collide (500 "CIDR overlaps") roughly half the time.
   let oct='';
   try {
     const list=await api('GET','/ipam/subnets?page_size=500');
     const used=new Set((list.data||[]).map(x=>{const m=/^10\.(\d+)\./.exec(x.cidr||'');return m?Number(m[1]):-1}));
     for(let candidate=200;candidate<240;candidate++){
       if(!used.has(candidate)){oct=String(candidate);break;}
     }
   } catch(e){ /* fall through to the default below */ }
   if(!oct) oct='200';
   const ins=p.locator('.t-dialog input:not([readonly])');
   await ins.nth(0).fill(uniq);
   await ins.nth(1).fill(`10.${oct}.0.0/24`);
   // space select
   await p.locator('.t-dialog .t-select').first().click(); await p.waitForTimeout(350);
   const opt=p.locator('.t-select__list .t-option, .t-select-option').first();
   if(await opt.count()) await opt.click();
   await p.locator('.t-dialog').getByRole('button',{name:/保存/}).first().click();
   await p.waitForTimeout(1800);
   step('subnet created end-to-end', (await p.locator('body').innerText()).includes(uniq), `needle=${uniq}`);
 } else step('subnet create button',false,'not found');

 // TDesign dialogs from the now-TDesign subnets page
 await p.goto(BASE+'/ipam/subnets',{waitUntil:'networkidle'}); await p.waitForTimeout(800);
 for(const [label,pattern] of [['Import',/导入地址/],['PoolWizard',/创建 DHCP 作用域/],['ReverseZone',/生成反向区域/]]){
   const btn=p.locator('.t-link').filter({hasText:pattern}).first();
   if(await btn.count()){
     await btn.click(); await p.waitForTimeout(650);
     step(label+' dialog visible (TDesign)', (await p.locator('.t-dialog:visible').count())>0, `vis=${await p.locator('.t-dialog:visible').count()}`);
     await p.keyboard.press('Escape'); await p.waitForTimeout(250);
   } else step(label+' button',false,'not found');
 }

 // Address detail drawer from TDesign addresses page
 await p.goto(BASE+'/ipam/addresses',{waitUntil:'networkidle'}); await p.waitForTimeout(800);
 const detail=p.locator('.t-link').filter({hasText:/查看详情|详情/}).first();
 if(await detail.count()){
   await detail.click(); await p.waitForTimeout(800);
   step('AddressDetailDrawer visible (TDesign)', (await p.locator('.t-drawer:visible').count())>0 && (await p.locator('.n-drawer').count())===0, '');
 } else step('detail link',false,'not found');

 // Self-clean the subnet this suite created so repeated runs without a reset
 // cannot accumulate CIDRs and so later suites see a clean table.
 try {
   const list=await api('GET','/ipam/subnets?page_size=500');
   const row=createdSubnetName ? (list.data||[]).find(x=>x.name===createdSubnetName) : null;
   if(row) await api('DELETE','/ipam/subnets/'+row.id);
 } catch(e){ /* best effort; the reset helper also removes tdsub% rows */ }

 await p.screenshot({path: shot('ui-batch10-subnets.png'),fullPage:true});
 console.log('errors:',errs.length); errs.slice(0,8).forEach(e=>console.log('  '+e));
 await b.close();
})().catch(e=>{console.error('FATAL',e);process.exit(1)});
