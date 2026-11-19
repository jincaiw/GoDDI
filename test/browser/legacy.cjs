const { chromium } = require('playwright');
const { BASE, USER, PASS, shot, launchOpts } = require('./common.cjs');
const paths=['/dashboard','/dns/zones','/dns/forwarders','/dhcp/scopes','/dhcp/leases','/ipam/subnets','/admin/users','/settings/system','/logs/audit','/tools/client'];
(async()=>{
 const b=await chromium.launch(launchOpts());
 const p=await b.newPage({viewport:{width:1440,height:900}});
 const errs=[]; p.on('pageerror',e=>errs.push(e.message));
 await p.goto(`${BASE}/login`,{waitUntil:'networkidle'});
 await p.locator('input').nth(0).fill(USER);
 await p.locator('input').nth(1).fill(PASS);
 await p.locator('button[type="submit"], .t-button--theme-primary').first().click();
 await p.waitForTimeout(1800);
 for(const path of paths){
   const before=errs.length;
   await p.goto(BASE+path,{waitUntil:'networkidle',timeout:30000});
   await p.waitForTimeout(700);
   const txt=(await p.locator('body').innerText()).replace(/\s+/g,' ').slice(0,90);
   const shell=await p.locator('.t-layout__sider').count();
    const hasContent=((await p.locator('.td-page').first().innerText().catch(()=>''))||'').trim().length>20;
   const ok = errs.length===before && shell>0 && hasContent;
   console.log(`${ok?'PASS':'FAIL'} | ${path.padEnd(20)} | shell=${shell} | ${txt}`);
 }
 console.log('pageerrors total:',errs.length); errs.slice(0,5).forEach(e=>console.log('  '+e));
 await b.close();
})().catch(e=>{console.error('FATAL',e);process.exit(1)});