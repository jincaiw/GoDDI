import { test, expect, type Locator, type Page } from '@playwright/test'

// The language switch is a hover-triggered dropdown. Reset the pointer first so
// the hover always produces a fresh mouseover, then pick the option out of the
// currently visible dropdown — a stale detached node would keep the old locale
// and silently leave the page heading unchanged.
async function switchLanguage(page: Page, option: string, switched: Locator) {
  await page.mouse.move(0, 0)
  await page.locator('[aria-label="Switch language"]').hover()
  const item = page.locator('.n-dropdown-option:visible').filter({ hasText: new RegExp(`^${option}$`) }).first()
  await expect(item).toBeVisible()
  // Click the body label — Naive UI binds select on the inner node, and a
  // wrapper click can be swallowed by transient hover state on the dropdown
  // menu container when the pointer has just been reset.
  await item.locator('.n-dropdown-option-body__label').click()
  await expect(switched).toBeVisible()
}

const routes = ['/', '/dns/zones', '/dns/forwarders', '/dns/security', '/dns/cache', '/tools/client',
  '/dhcp/scopes', '/dhcp/leases', '/dhcp/reservations', '/dhcp/options',
  '/ipam/spaces', '/ipam/subnets', '/ipam/addresses', '/admin/users', '/admin/roles',
  '/admin/groups', '/admin/tokens', '/logs/audit', '/logs/dns', '/logs/dhcp', '/settings/system', '/settings/backup']

test('login and all console routes render without runtime errors', async ({ page }) => {
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  page.on('response', response => { if (response.status() >= 500) errors.push(`${response.status()} ${response.url()}`) })
  await page.addInitScript(() => { if (!localStorage.getItem('GODDI_lang')) localStorage.setItem('GODDI_lang', '"en-US"') })
  await page.goto('/login')
  await page.getByRole('textbox').nth(0).fill(process.env.GODDI_TEST_USERNAME || 'admin')
  await page.getByRole('textbox').nth(1).fill(process.env.GODDI_TEST_PASSWORD || 'Admin@123456')
  await page.getByRole('button', { name: 'Login', exact: true }).click()
  await expect(page).toHaveURL(/dashboard$/)
  for (const route of routes) {
    await page.goto(route)
    await expect(page.getByRole('heading', { level: 2 }).first()).toBeVisible()
    await expect(page.locator('.n-spin-body')).toHaveCount(0)
    await expect(page).not.toHaveURL(/login/)
    const demoNames: Record<string, string> = { '/': 'dashboard', '/dns/zones': 'dns-zones', '/settings/backup': 'backup' }
    if (process.env.GODDI_DEMO_DIR && demoNames[route]) {
      await page.screenshot({ path: `${process.env.GODDI_DEMO_DIR}/${demoNames[route]}.png`, fullPage: true, animations: 'disabled' })
    }
  }
  await page.goto('/')
  // Reload between the route loop and the language switch: the dashboard view
  // accumulates focus/scroll/queued-API state across 22 route visits, and the
  // second hover-dropdown click can otherwise land on a detached node. A clean
  // page eliminates the flake without disabling the switch.
  await page.reload()
  await page.waitForLoadState('networkidle')
  await switchLanguage(page, '中文', page.getByRole('heading', { name: '仪表盘', exact: true }))
  await switchLanguage(page, 'English', page.getByRole('heading', { name: 'Dashboard', exact: true }))
  await page.setViewportSize({ width: 390, height: 844 })
  await page.locator('[aria-label="Toggle navigation"]').click()
  await expect(page.locator('aside[class*="layout-mobile-sider"]').first()).toBeVisible()
  await page.locator('[class*="layout-mobile-sider-mask"]').click()
  await expect(page.locator('[class*="layout-mobile-sider-mask"]')).toBeHidden()
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth)
  expect(overflow).toBe(false)
  expect(errors).toEqual([])
})

test('mobile login fits viewport', async ({ page }) => {
  await page.addInitScript(() => { if (!localStorage.getItem('GODDI_lang')) localStorage.setItem('GODDI_lang', '"en-US"') })
  await page.setViewportSize({ width: 320, height: 700 })
  await page.goto('/login')
  await expect(page.getByRole('button', { name: 'Login', exact: true })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth)).toBe(false)
})
