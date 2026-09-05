import { test, expect } from '@playwright/test'
import { execFileSync } from 'node:child_process'

test('token isolation, CSRF, single use, and DNS backup restoration', async ({ request }) => {
  const login = await request.post('/api/v1/auth/login', { data: { username: process.env.GODDI_TEST_USERNAME || 'admin', password: process.env.GODDI_TEST_PASSWORD || 'Admin@123456' } })
  expect(login.ok()).toBe(true)
  const session = (await login.json()).data
  const headers = { Authorization: `Bearer ${session.token}`, 'X-CSRF-Token': session.csrf_token }
  const forbidden = await request.post('/api/v1/dns/zones', { headers: { Authorization: headers.Authorization }, data: { name: 'csrf.invalid' } })
  expect(forbidden.status()).toBe(401)
  const created = await request.post('/api/v1/tokens', { headers, data: { name: `review-${Date.now()}`, scope: 'dns:read' } })
  expect(created.status()).toBe(201)
  const token = (await created.json()).data
  const tokenHeaders = { Authorization: `Bearer ${token.token}` }
  expect((await request.get('/api/v1/dns/zones', { headers: tokenHeaders })).status()).toBe(200)
  expect((await request.get('/api/v1/users', { headers: tokenHeaders })).status()).toBe(403)
  expect((await request.post('/api/v1/dns/zones', { headers: tokenHeaders, data: { name: 'blocked.invalid' } })).status()).toBe(403)
  expect((await request.post('/api/v1/auth/totp/setup', { headers: tokenHeaders, data: {} })).status()).toBe(403)
  const once = (await (await request.post('/api/v1/tokens', { headers, data: { name: 'once', scope: 'dns:read', is_single_use: true } })).json()).data
  const onceHeaders = { Authorization: `Bearer ${once.token}` }
  expect((await request.get('/api/v1/dns/zones', { headers: onceHeaders })).status()).toBe(200)
  expect((await request.get('/api/v1/dns/zones', { headers: onceHeaders })).status()).toBe(401)
  const zone = (await (await request.post('/api/v1/dns/zones', { headers, data: { name: `backup-${Date.now()}.example`, type: 'primary' } })).json()).data
  expect((await request.post(`/api/v1/dns/zones/${zone.id}/records`, { headers, data: { name: 'restore', type: 'A', value: '192.0.2.42', ttl: 60, enabled: true } })).status()).toBe(201)
  const query = (tcp = false) => execFileSync('dig', ['@127.0.0.1', '-p', process.env.GODDI_TEST_DNS_PORT!, `restore.${zone.name}`, 'A', '+short', '+time=2', '+tries=1', ...(tcp ? ['+tcp'] : [])], { encoding: 'utf8' }).trim()
  if (process.env.GODDI_TEST_DNS_PORT) await expect.poll(() => query()).toBe('192.0.2.42')
  const backupResponse = await request.post('/api/v1/backup', { headers, data: { type: 'dns', description: 'automated restore verification' } })
  expect(backupResponse.status()).toBe(201)
  const backup = (await backupResponse.json()).data
  expect(backup.status).toBe('completed')
  expect((await request.get(`/api/v1/backup/${backup.id}/download`, { headers })).status()).toBe(200)
  expect((await request.delete(`/api/v1/dns/zones/${zone.id}`, { headers })).status()).toBe(200)
  if (process.env.GODDI_TEST_DNS_PORT) await expect.poll(() => query()).not.toBe('192.0.2.42')
  expect((await request.post(`/api/v1/backup/${backup.id}/restore`, { headers, data: {} })).status()).toBe(200)
  expect((await request.get(`/api/v1/dns/zones/${zone.id}`, { headers })).status()).toBe(200)
  if (process.env.GODDI_TEST_DNS_PORT) {
    await expect.poll(() => query()).toBe('192.0.2.42')
    expect(query(true)).toBe('192.0.2.42')
  }
  await request.delete(`/api/v1/dns/zones/${zone.id}`, { headers })
  await request.delete(`/api/v1/backup/${backup.id}`, { headers })
  await request.delete(`/api/v1/tokens/${token.id}`, { headers })
})
