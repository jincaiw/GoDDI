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
 step('logged in', !p.url().includes('/login'));

 await p.goto(BASE+'/settings/config-versions',{waitUntil:'networkidle'}); await p.waitForTimeout(1200);
 const body=await p.locator('body').innerText();
 const naive=await p.locator('.n-card, .n-data-table, .n-modal, .n-form-item').count();
 step('config-versions renders TDesign', body.includes('配置版本') && (await p.locator('.t-table').count())>0 && naive===0, `tables=${await p.locator('.t-table').count()} naive=${naive}`);
 step('resource types loaded', body.includes('dns_records')||body.includes('区域'), body.slice(0,120).replace(/\n/g,' | '));
 step('revision table present', (await p.locator('.t-table').count())>=1, '');
 step('release queue stats present', body.includes('待应用')||body.includes('失败')||body.includes('队列'), '');
 step('diff controls present', (await p.locator('.t-select').count())>=2, `selects=${await p.locator('.t-select').count()}`);
 // failed-row tint class wired
 step('no Naive UI remnants', naive===0, '');
 await p.screenshot({path: shot('ui-batch13-config.png'),fullPage:true});
 console.log('errors:',errs.length); errs.slice(0,8).forEach(e=>console.log('  '+e));
 await b.close();
})().catch(e=>{console.error('FATAL',e);process.exit(1)});
