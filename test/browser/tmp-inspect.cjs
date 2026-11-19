const { chromium } = require("playwright");
const { BASE, USER, PASS, launchOpts } = require("./common.cjs");
(async()=>{
 const b=await chromium.launch(launchOpts());
 const p=await b.newPage({viewport:{width:1440,height:900}});
 await p.goto(`${BASE}/login`,{waitUntil:"networkidle"});
 await p.locator("input").nth(0).fill(USER); await p.locator("input").nth(1).fill(PASS);
 await p.locator("button[type=submit], .t-button--theme-primary").first().click(); await p.waitForTimeout(1600);
 await p.goto(`${BASE}/dashboard`,{waitUntil:"networkidle"}); await p.waitForTimeout(700);
 console.log("MENU", JSON.stringify(await p.locator(".t-menu__item, [role=menuitem]").evaluateAll(els=>els.slice(0,20).map(e=>({tag:e.tagName,txt:e.textContent.trim().slice(0,30),href:e.getAttribute("href"),tabindex:e.getAttribute("tabindex"),cls:e.className})))));
 await p.goto(`${BASE}/dns/zones`,{waitUntil:"networkidle"}); await p.waitForTimeout(700);
 console.log("BUTTONS", JSON.stringify(await p.locator("button, .t-button").evaluateAll(els=>els.slice(0,30).map(e=>({tag:e.tagName,txt:e.textContent.trim().slice(0,30),aria:e.getAttribute("aria-label"),cls:e.className})))));
 await b.close();
})().catch(e=>{console.error(e);process.exit(1)});