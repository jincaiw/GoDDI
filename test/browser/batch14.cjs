const { chromium } = require('playwright');
const { BASE, USER, PASS, shot, launchOpts } = require('./common.cjs');
(async()=>{
 const b=await chromium.launch(launchOpts());
 const p=await b.newPage({viewport:{width:1440,height:900}});
 const errs=[]; p.on('pageerror',e=>errs.push('PAGEERROR: '+e.message));
 p.on('console',m=>{if(m.type()==='error'){const x=m.text().slice(0,160);if(!/501|Not Implemented|404 \(Not Found\)/.test(x))errs.push('CONSOLE: '+x)}});
 const step=(n,ok,x='')=>console.log(`${ok?'PASS':'FAIL'} | ${n}${x?' | '+x:''}`);
 await p.goto(`${BASE}/login`,{waitUntil:'networkidle'});
 await p.locator('input').nth(0).fill(USER); await p.locator('input').nth(1).fill(PASS);
 await p.locator('button[type="submit"], .t-button--theme-primary').first().click(); await p.waitForTimeout(1800);
 step('login works with TDesign-only shell', !p.url().includes('/login'), `url=${p.url()}`);

 // dashboard (last migrated page) + no Naive DOM anywhere
 await p.goto(BASE+'/dashboard',{waitUntil:'networkidle'}); await p.waitForTimeout(1200);
 const body=await p.locator('body').innerText();
 const naive=await p.locator('[class^="n-"], [class*=" n-"]').count();
 step('dashboard renders TDesign charts/stats', (await p.locator('.t-card').count())>=8 && (await p.locator('.t-table').count())>0, `cards=${await p.locator('.t-card').count()} tables=${await p.locator('.t-table').count()}`);
 step('no Naive UI DOM classes', naive===0, `naiveEls=${naive}`);

 // config-versions (last migrated view)
 await p.goto(BASE+'/settings/config-versions',{waitUntil:'networkidle'}); await p.waitForTimeout(1000);
 step('config-versions renders', (await p.locator('.t-table').count())>0 && (await p.locator('[class^="n-"]').count())===0, '');

 // blank-layout error pages still work with TDesign button
 for(const path of ['/403','/404','/500']){
   await p.goto(BASE+path,{waitUntil:'networkidle'}); await p.waitForTimeout(500);
   step(path+' renders TDesign back button', (await p.locator('.t-button').count())>0 && (await p.locator('[class^="n-"]').count())===0, '');
 }

 // no naive-ui in any served bundle
 const html=await p.goto(BASE+'/dashboard',{waitUntil:'networkidle'}).then(()=>p.content());
 step('no naive-ui string in shell', !/naive-ui/.test(html), '');

 await p.goto(BASE+'/dashboard',{waitUntil:'networkidle'}); await p.waitForTimeout(800);
 const dashText=await p.locator('body').innerText();
 step('dashboard renders no raw i18n keys', !/dashboard\.[A-Za-z]/.test(dashText), '');
 await p.screenshot({path: shot('ui-batch14-dashboard.png'),fullPage:true});
 console.log('errors:',errs.length); errs.slice(0,8).forEach(e=>console.log('  '+e));
 await b.close();
})().catch(e=>{console.error('FATAL',e);process.exit(1)});
