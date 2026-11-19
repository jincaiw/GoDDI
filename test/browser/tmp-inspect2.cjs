const { chromium } = require("playwright");
const { BASE, USER, PASS, launchOpts } = require("./common.cjs");
(async()=>{
 const b=await chromium.launch(launchOpts());
 const p=await b.newPage({viewport:{width:1440,height:900}});
 await p.goto(`${BASE}/login`,{waitUntil:"networkidle"});
 await p.locator("input").nth(0).fill(USER); await p.locator("input").nth(1).fill(PASS);
 await p.locator("button[type=submit], .t-button--theme-primary").first().click(); await p.waitForTimeout(1600);
 await p.goto(`${BASE}/dashboard`,{waitUntil:"networkidle"}); await p.waitForTimeout(700);
 console.log(await p.locator(".t-menu__item").filter({hasText:"区域管理"}).first().evaluate(e=>e.outerHTML.slice(0,1200)));
 console.log("focusable", await p.locator(".t-menu a[href], .t-menu [tabindex]").count());
 console.log(await p.locator(".t-menu a[href], .t-menu [tabindex]").evaluateAll(els=>els.slice(0,12).map(e=>e.outerHTML.slice(0,300))));
 await b.close();
})().catch(e=>{console.error(e);process.exit(1)});