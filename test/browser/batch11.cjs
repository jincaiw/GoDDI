const { chromium } = require('playwright');
const { BASE, USER, PASS, shot, launchOpts } = require('./common.cjs');
(async()=>{
 const b=await chromium.launch(launchOpts());
 const p=await b.newPage({viewport:{width:1440,height:900}});
 const errs=[]; p.on('pageerror',e=>errs.push('PAGEERROR: '+e.message));
 p.on('console',m=>{if(m.type()==='error'){const x=m.text().slice(0,150);if(!/501|Not Implemented|404 \(Not Found\)/.test(x))errs.push('CONSOLE: '+x)}});
 const step=(n,ok,x='')=>console.log(`${ok?'PASS':'FAIL'} | ${n}${x?' | '+x:''}`);
 // blank-layout pages still work after old layout deletion
 for(const path of ['/login','/403','/404','/500']){
   await p.goto(BASE+path,{waitUntil:'networkidle'}); await p.waitForTimeout(600);
   const body=await p.locator('body').innerText();
   step(path+' (blank layout) renders', body.includes('返回首页')||body.trim().length>20, `text=${body.trim().slice(0,20)}`);
 }
 // login still functional
 await p.goto(BASE+'/login',{waitUntil:'networkidle'}); await p.waitForTimeout(500);
 await p.locator('input').nth(0).fill(USER); await p.locator('input').nth(1).fill(PASS);
 await p.locator('button[type="submit"], .t-button--theme-primary').first().click(); await p.waitForTimeout(1600);
 step('login works after cleanup', !p.url().includes('/login'), `url=${p.url()}`);
 // 2 remaining Naive views still work with AppProvider
 for(const path of ['/dns/zone-detail/fake-id','/settings/config-versions']){
   await p.goto(BASE+path,{waitUntil:'networkidle'}); await p.waitForTimeout(800);
   const body=await p.locator('body').innerText();
   step(path+' still renders', body.trim().length>50, `chars=${body.trim().length}`);
 }
 console.log('errors:',errs.length); errs.slice(0,8).forEach(e=>console.log('  '+e));
 await b.close();
})().catch(e=>{console.error('FATAL',e);process.exit(1)});
