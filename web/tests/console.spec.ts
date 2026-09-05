import { test, expect } from '@playwright/test'

const routes = ['/', '/dns/zones', '/dns/forwarders', '/dns/security', '/dns/cache', '/dns/client',
  '/dhcp/scopes', '/dhcp/leases', '/dhcp/reservations', '/dhcp/options',
  '/ipam/spaces', '/ipam/subnets', '/ipam/addresses', '/admin/users', '/admin/roles',
  '/admin/groups', '/admin/tokens', '/logs/audit', '/logs/dns', '/logs/dhcp', '/settings', '/settings/backup']

test('login and all console routes render without runtime errors', async ({ page }) => {
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  page.on('response', response => { if (response.status() >= 500) errors.push(`${response.status()} ${response.url()}`) })
  await page.goto('/login')
  await page.getByRole('textbox').nth(0).fill(process.env.GODDI_TEST_USERNAME || 'admin')
  await page.getByRole('textbox').nth(1).fill(process.env.GODDI_TEST_PASSWORD || 'Admin@123456')
  await page.getByRole('button', { name: '登录', exact: true }).click()
  await expect(page).toHaveURL(/\/$/)
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
  await page.getByRole('button', { name: '语言', exact: true }).click()
  await page.getByText('English', { exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Dashboard', exact: true })).toBeVisible()
  await page.setViewportSize({ width: 390, height: 844 })
  await page.getByRole('button', { name: 'Open navigation' }).click()
  await expect(page.locator('.n-drawer')).toBeVisible()
  await page.keyboard.press('Escape')
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth)
  expect(overflow).toBe(false)
  expect(errors).toEqual([])
})

test('mobile login fits viewport', async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 700 })
  await page.goto('/login')
  await expect(page.getByRole('button', { name: '登录', exact: true })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth)).toBe(false)
})
