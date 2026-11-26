// Idempotent seed for the browser regression harness.
//
// The suites assume a *seeded* isolated instance (a resolvable zone and a
// DHCP scope with free addresses); on a truly empty fresh DB a handful of
// assertions (scope progress, zone row actions, reservation create, DNS
// debug query) have nothing to operate on. This script creates the minimum
// baseline via the public API so CI can gate the suite on a fresh database.
//
// Safe to run repeatedly; only creates what is missing. Never point it at
// production.
const BASE = process.env.GODDI_BASE_URL || 'http://127.0.0.1:18080'
const USER = process.env.GODDI_ADMIN_USER || 'admin'
const PASS = process.env.GODDI_ADMIN_PASS || ''
const log = (...a) => console.log('[seed]', ...a)

const readJSON = async r => ({ status: r.status, body: await r.json().catch(() => null) })

async function main() {
  if (!PASS) throw new Error('GODDI_ADMIN_PASS is required')
  const login = await readJSON(await fetch(`${BASE}/api/v1/auth/login`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: USER, password: PASS })
  }))
  if (login.status !== 200 || !login.body?.data?.token) throw new Error(`login failed: ${login.status}`)
  const sess = login.body.data
  const H = { 'Content-Type': 'application/json', Authorization: `Bearer ${sess.token}`, 'X-CSRF-Token': sess.csrf_token }
  const get = async p => (await fetch(BASE + '/api/v1' + p, { headers: { Authorization: H.Authorization } })).json()

  // 1. A primary zone with a resolvable A record (DNS debug client + zone rows).
  const zones = await get('/dns/zones?page_size=500')
  let zone = (zones.data || []).find(z => String(z.name).replace(/\.$/, '') === 'demo.test')
  if (!zone) {
    const c = await readJSON(await fetch(`${BASE}/api/v1/dns/zones`, {
      method: 'POST', headers: H, body: JSON.stringify({ name: 'demo.test', type: 'primary', enabled: true })
    }))
    log('create zone demo.test:', c.status)
    zone = c.body?.data
  }
  if (zone?.id) {
    const recs = await get(`/dns/zones/${zone.id}/records?page_size=500`)
    const has = (recs.data || []).some(r => String(r.name).replace(/\.$/, '').replace(/\.demo\.test$/, '') === 'www' && r.type === 'A')
    if (!has) {
      const r = await readJSON(await fetch(`${BASE}/api/v1/dns/zones/${zone.id}/records`, {
        method: 'POST', headers: H, body: JSON.stringify({ name: 'www', type: 'A', value: '192.0.2.10', ttl: 300, enabled: true })
      }))
      log('create record www.demo.test:', r.status)
    }
  }

  // 2. An enabled DHCP scope with free addresses (reservation create, progress).
  const scopes = await get('/dhcp/scopes?page_size=100')
  if (!(scopes.data || []).some(s => s.name === 'td-demo-scope')) {
    const c = await readJSON(await fetch(`${BASE}/api/v1/dhcp/scopes`, {
      method: 'POST', headers: H,
      body: JSON.stringify({
        name: 'td-demo-scope', subnet: '10.99.0.0/24',
        start_ip: '10.99.0.10', end_ip: '10.99.0.200',
        subnet_mask: '255.255.255.0', lease_time: 3600, enabled: true
      })
    }))
    log('create dhcp scope td-demo-scope:', c.status, c.body?.message || '')
  }

  // 3. An IPAM space (subnet create page has a selectable space).
  const spaces = await get('/ipam/spaces?page_size=100')
  if (!(spaces.data || []).some(s => s.name === 'td-demo-space')) {
    const c = await readJSON(await fetch(`${BASE}/api/v1/ipam/spaces`, {
      method: 'POST', headers: H, body: JSON.stringify({ name: 'td-demo-space' })
    }))
    log('create ipam space td-demo-space:', c.status)
  }

  log('done')
}

main().catch(e => { console.error('[seed] FAIL', e.message); process.exit(1) })
