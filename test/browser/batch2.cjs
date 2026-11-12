const { chromium } = require('playwright');
const { BASE, USER, PASS, shot, launchOpts } = require('./common.cjs');
const pages=[
  {path:'/logs/audit', title:'审计日志'},
  {path:'/logs/dhcp',  title:'DHCP 日志'},
  {path:'/logs/dns',   title:'DNS 查询日志'},
  {path:'/dns/apps',   title:'应用'},
  {path:'/ipam/spaces',title:'地址空间'},
];
(async()=>{
 const b=await chromium.launch(launchOpts());
 const p=await b.newPage({viewport:{width:1440,height:900}});
 const errs=[]; p.on('pageerror',e=>errs.push('PAGEERROR: '+e.message));
 p.on('console',m=>{if(m.type()==='error'){const x=m.text().slice(0,160); if(!/501|Not Implemented/.test(x)) errs.push('CONSOLE: '+x)}});
 await p.goto(`${BASE}/login`,{waitUntil:'networkidle'});
 await p.locator('input').nth(0).fill(USER);
 await p.locator('input').nth(1).fill(PASS);
 await p.locator('button[type="submit"], .t-button--theme-primary').first().click();
 await p.waitForTimeout(1800);
 console.log('logged in:', !p.url().includes('/login'));
 for(const {path,title} of pages){
   const before=errs.length;
   await p.goto(BASE+path,{waitUntil:'networkidle',timeout:30000});
   await p.waitForTimeout(900);
   const body=await p.locator('body').innerText();
   const h2=await p.locator('h2').first().innerText().catch(()=>'');
   const tables=await p.locator('.t-table').count();
   const empties=await p.locator('.t-empty').count();
   const shell=await p.locator('.t-layout__sider').count();
   const naive=await p.locator('.n-data-table, .n-card').count();
   const known501 = path==='/dns/apps' ? (await p.locator('body').innerText()).includes('501') : true;
   const ok=body.includes(title)&&shell>0&&(tables>0||empties>0)&&naive===0&&errs.length===before&&known501;
   console.log(`${ok?'PASS':'FAIL'} | ${path.padEnd(14)} | h2="${h2}" tables=${tables} empty=${empties} naive=${naive} shell=${shell}`);
 }
 await p.goto(BASE+'/logs/audit',{waitUntil:'networkidle'}); await p.waitForTimeout(600);
 await p.screenshot({path: shot('ui-batch2-audit.png'),fullPage:true});
 console.log('errors total:',errs.length); errs.slice(0,6).forEach(e=>console.log('  '+e));
 await b.close();
})().catch(e=>{console.error('FATAL',e);process.exit(1)});
