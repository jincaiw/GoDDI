const { chromium } = require('playwright');
const { BASE, USER, PASS, shot, launchOpts } = require('./common.cjs');
(async()=>{
 const b=await chromium.launch(launchOpts());
 const p=await b.newPage({viewport:{width:1440,height:900}});
 const errs=[]; let expectNegative=false;
 p.on('pageerror',e=>errs.push('PAGEERROR: '+e.message));
 p.on('console',m=>{if(m.type()==='error'){const x=m.text().slice(0,150);if(expectNegative && /400 \(Bad Request\)|404 \(Not Found\)|409 \(Conflict\)/.test(x)) return;if(!/501|Not Implemented/.test(x))errs.push('CONSOLE: '+x)}});
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

 // seed space + subnet + address
 const suffix=Date.now().toString()+Math.floor(Math.random()*1000);
 const sp=await api('POST','/ipam/spaces',{name:'td-seed-space-'+suffix});
 const sid=sp.data?.data?.id;
 // Pick a free /24: this suite's seeded subnets are not deleted (an address
 // is allocated in them), so repeated runs without a reset must not collide
 // with an earlier seed or with batch10's subnet.
 let oct='';
 try {
   const list=await api('GET','/ipam/subnets?page_size=500');
   const used=new Set((list.data||[]).map(x=>{const m=/^10\.(\d+)\./.exec(x.cidr||'');return m?Number(m[1]):-1}));
   for(let candidate=70;candidate<150;candidate++){
     if(!used.has(candidate)){oct=String(candidate);break;}
   }
 } catch(e){ /* fall through */ }
 if(!oct) oct=String(70+Math.floor(Math.random()*80));
 const sn=await api('POST','/ipam/subnets',{space_id:sid,name:'td-seed-subnet-'+suffix,cidr:`10.${oct}.0.0/24`});
 const subid=sn.data?.data?.id;
 step('seed data created', !!subid, `space=${sid} subnet=${subid}`);
 if(subid){
   await api('POST','/ipam/addresses/allocate',{subnet_id:subid,owner:'td-seed'});
 }

 // Lifecycle restore and purge are state transitions with real guards;
 // exercise them through the same API the archive UI uses.
 {
   const rs = Date.now().toString() + Math.floor(Math.random()*1000);
   const created = await api('POST','/ipam/spaces',{name:'td-restore-space-'+rs});
   const rid = created.data?.data?.id;
   const retired = rid ? await api('DELETE','/ipam/spaces/'+rid) : null;
   const restored = rid ? await api('POST','/ipam/spaces/'+rid+'/restore',{}) : null;
   const after = rid ? await api('GET','/ipam/spaces/'+rid) : null;
   step('retired space restored through API',
     retired?.status===200 && restored?.status===200 && after?.data?.data?.lifecycle_state==='active');
   expectNegative=true;
   if (rid) await api('POST','/ipam/spaces/'+rid+'/purge',{confirmation:rid});
   await p.waitForTimeout(200); expectNegative=false;
 }
 {
   const ps = Date.now().toString() + Math.floor(Math.random()*1000);
   const created = await api('POST','/ipam/spaces',{name:'td-purge-space-'+ps});
   const pid = created.data?.data?.id;
   if (pid) await api('DELETE','/ipam/spaces/'+pid);
   expectNegative=true;
   const wrong = pid ? await api('POST','/ipam/spaces/'+pid+'/purge',{confirmation:'wrong-id'}) : null;
   const purged = pid ? await api('POST','/ipam/spaces/'+pid+'/purge',{confirmation:pid}) : null;
   const after = pid ? await api('GET','/ipam/spaces/'+pid) : null;
   await p.waitForTimeout(200); expectNegative=false;
   step('retired empty tombstone purged with exact confirmation',
     wrong?.status===400 && purged?.status===200 && after?.status===404);
 }
 // ipam/subnets: 3 TDesign dialogs via row actions
 await p.goto(BASE+'/ipam/subnets',{waitUntil:'networkidle'}); await p.waitForTimeout(1000);
 step('subnets page loads', (await p.locator('body').innerText()).includes('子网管理'), `rows=${await p.locator('.t-table tbody tr').count()}`);
 const rowTexts=await p.locator('.t-table tbody tr').first().innerText().catch(()=>'');
 console.log('  first row:', rowTexts.replace(/\n/g,' | ').slice(0,180));

 const tryBtn=async(label,pattern)=>{
   const btn=p.locator('.t-link').filter({hasText:pattern}).first();
   if(await btn.count()){
     await btn.click(); await p.waitForTimeout(800);
     step(label+' opens as TDesign dialog', (await p.locator('.t-dialog').count())>0 && (await p.locator('.n-modal').count())===0, `tdialog=${await p.locator('.t-dialog').count()}`);
     await p.keyboard.press('Escape'); await p.waitForTimeout(300);
     return true;
   }
   console.log('  INFO | no button matching', String(pattern));
   return false;
 };
 let opened=0;
 if(await tryBtn('ImportAddressesDialog',/导入地址/)) opened++;
 if(await tryBtn('PoolWizardDialog',/创建.*作用域|生成作用域|DHCP 作用域/)) opened++;
 if(await tryBtn('ReverseZoneDialog',/反向/)) opened++;
 step('at least one TDesign IPAM dialog verified', opened>0, `opened=${opened}`);

 // addresses detail drawer
 await p.goto(BASE+'/ipam/addresses',{waitUntil:'networkidle'}); await p.waitForTimeout(900);
 const detail=p.locator('.t-link').filter({hasText:/查看详情|详情/}).first();
 if(await detail.count()){
   await detail.click(); await p.waitForTimeout(1000);
   step('AddressDetailDrawer opens as TDesign', (await p.locator('.t-drawer').count())>0 && (await p.locator('.n-drawer').count())===0, `tdrawer=${await p.locator('.t-drawer').count()}`);
   await p.screenshot({path: shot('ui-batch9-drawer.png'),fullPage:true});
 } else console.log('  INFO | detail button not found; address rows:', await p.locator('.t-table tbody tr').count());

 console.log('errors:',errs.length); errs.slice(0,8).forEach(e=>console.log('  '+e));
 await b.close();
})().catch(e=>{console.error('FATAL',e);process.exit(1)});
