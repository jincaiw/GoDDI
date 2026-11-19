const { chromium } = require('playwright');
const { BASE, USER, PASS, shot, launchOpts } = require('./common.cjs');
(async()=>{
 const b=await chromium.launch(launchOpts());
 const p=await b.newPage({viewport:{width:1440,height:900}});
 const errs=[]; p.on('pageerror',e=>errs.push('PAGEERROR: '+e.message));
 const step=(n,ok,x='')=>console.log(`${ok?'PASS':'FAIL'} | ${n}${x?' | '+x:''}`);
 await p.goto(`${BASE}/login`,{waitUntil:'networkidle'});
 await p.locator('input').nth(0).focus();
 await p.keyboard.type(USER);
 await p.keyboard.press('Tab');
 await p.keyboard.type(PASS);
 for(let i=0;i<8;i++){
   const active=await p.evaluate(()=>({tag:document.activeElement?.tagName,text:document.activeElement?.textContent?.trim()||''}));
   if(active.tag==='BUTTON' && active.text.includes('登录')) break;
   await p.keyboard.press('Tab');
 }
 await p.keyboard.press('Enter');
 await p.waitForTimeout(1800);
 step('keyboard login', !p.url().includes('/login'), `url=${p.url()}`);
 const labels=await p.locator('[aria-label]').count();
 step('header accessible labels', labels>=4, `labels=${labels}`);
 await p.goto(`${BASE}/dashboard`,{waitUntil:'networkidle'}); await p.waitForTimeout(700);
 const expand=p.getByRole('button',{name:/展开菜单|expand/i}).first(); if(await expand.count()){ await expand.click(); await p.waitForTimeout(400); }
 const zoneNav=p.locator('button.menu-key-target[aria-label="区域管理"]').first();
 await zoneNav.focus(); await p.keyboard.press('Enter'); await p.waitForTimeout(1200);
 step('keyboard navigation', p.url().includes('/dns/zones'), `url=${p.url()}`);
 const create=p.getByRole('button',{name:/新建区域|创建区域/}).first();
 step('create action focusable', await create.count()>0);
 if(await create.count()){
   await create.focus(); await p.keyboard.press('Enter'); await p.waitForTimeout(500);
   const visible=await p.locator('.t-dialog').first().isVisible().catch(()=>false);
   step('keyboard opens dialog', visible);
   const input=p.locator('.t-dialog input:not([readonly])').first();
   if(await input.count()){ await input.focus(); await p.keyboard.type('keyboard-a11y-check'); }
   const typed=await p.locator('.t-dialog input:not([readonly])').first().inputValue().catch(()=>'');
   step('keyboard enters dialog text', typed==='keyboard-a11y-check', `value=${typed}`);
   await p.keyboard.press('Escape'); await p.waitForTimeout(400);
   const closed=await p.locator('.t-dialog').first().isVisible().catch(()=>false);
   step('escape dismisses dialog', !closed);
 }
 const settings=p.getByRole('button',{name:/主题|Theme/}).first();
 if(await settings.count()){
   await settings.focus(); await p.keyboard.press('Enter'); await p.waitForTimeout(500);
   const drawer=await p.locator('.t-drawer').first().isVisible().catch(()=>false);
   step('keyboard opens theme drawer', drawer);
   await p.keyboard.press('Escape');
 } else step('theme button found', false);
 await p.screenshot({path:shot('ui-keyboard-accessibility.png'),fullPage:true});
 console.log('errors:',errs.length); errs.slice(0,5).forEach(e=>console.log('  '+e));
 await b.close();
})().catch(e=>{console.error('FATAL',e);process.exit(1)});