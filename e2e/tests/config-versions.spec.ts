import { test, expect, type Locator, type Page } from '@playwright/test'

// The configuration versioning surface, exercised through the console.
//
// What is checked here that unit tests on either side cannot reach:
//
//   * the page is wired to the publishable resource types a real instance
//     actually registers, and the revision table is not an empty state -- the
//     rows are the ones the create paths wrote, so a broken creation hook
//     shows up here as "no history" rather than as a failing Go test;
//   * a rollback is a *publish*, not an undo: the dialog's baseline number is
//     fetched for the row's own resource, and the result is a new revision on
//     top. Without a browser there is no way to see that the button works when
//     the table is not filtered to a single resource.
//
// Every test here creates the zone it works on, through the API, instead of
// adopting whatever revision happens to be newest in the table. That is not
// tidiness. A revision outlives the resource it describes -- the history is
// deliberately not cascaded -- so on a bare instance the newest dns_zone row
// can belong to a zone some earlier spec created and then deleted, and
// publishing against it answers 404. Correctly, and for a reason that has
// nothing to do with the console. The instance therefore does not need seeding
// for this file, and it runs in whatever environment the rest of the suite
// uses.
//
// One trap worth naming because it decides the shape of these assertions:
// `dns_zone` and `dns_records` share a resource id -- both are keyed by the
// zone. Filtering by resource id alone therefore returns two resource types
// interleaved, and "the first row" is not the newest revision of anything.
// Every lookup below filters by type as well, or matches on the revision
// number rather than on position.

const USERNAME = process.env.GODDI_TEST_USERNAME || 'admin'
const PASSWORD = process.env.GODDI_TEST_PASSWORD || 'Admin@123456'

async function login(page: Page) {
  await page.addInitScript(() => {
    if (!localStorage.getItem('GODDI_lang')) localStorage.setItem('GODDI_lang', '"en-US"')
  })
  await page.goto('/login')
  await page.getByRole('textbox').nth(0).fill(USERNAME)
  await page.getByRole('textbox').nth(1).fill(PASSWORD)
  await page.getByRole('button', { name: 'Login', exact: true }).click()
  await expect(page).toHaveURL(/dashboard$/)
}

/** The page's own credentials, so the test can set up state the UI cannot. */
async function authHeaders(page: Page): Promise<Record<string, string>> {
  const stored = await page.evaluate(() => ({
    token: sessionStorage.getItem('token') || '',
    csrf: sessionStorage.getItem('csrf_token') || '',
  }))
  expect(stored.token, 'the console must have stored a token').not.toBe('')
  return { Authorization: `Bearer ${stored.token}`, 'X-CSRF-Token': stored.csrf }
}

async function apiGet(page: Page, path: string): Promise<Record<string, unknown>> {
  const response = await page.request.get(`/api/v1${path}`, { headers: await authHeaders(page) })
  expect(response.status()).toBe(200)
  return (await response.json()) as Record<string, unknown>
}

async function listRevisions(page: Page, params: Record<string, string | number>) {
  const query = Object.entries(params)
    .map(([k, v]) => `${k}=${encodeURIComponent(String(v))}`)
    .join('&')
  const body = await apiGet(page, `/config/revisions?${query}`)
  return (body.data as Array<Record<string, unknown>>) ?? []
}

async function postConfig(page: Page, path: string, data: unknown): Promise<Record<string, unknown>> {
  const response = await page.request.post(`/api/v1${path}`, { headers: await authHeaders(page), data })
  const body = (await response.json()) as Record<string, unknown>
  expect(body.code, `${path} -> ${JSON.stringify(body).slice(0, 300)}`).toBe(0)
  return (body.data ?? {}) as Record<string, unknown>
}

/**
 * A zone of this test's own, with the revision its creation wrote.
 *
 * The revision number is read back rather than assumed to be 1: the create
 * hook is what writes it, and a create that stopped writing history has to
 * fail here rather than be papered over.
 */
async function createZone(
  page: Page,
): Promise<{ id: string; revision: number; content: Record<string, unknown> }> {
  const created = await postConfig(page, '/dns/zones', {
    name: `configversions-${Date.now()}.example`,
    type: 'primary',
  })
  const id = String(created.id)
  expect(id, 'creating a zone must return its id').not.toBe('')
  const revisions = await listRevisions(page, {
    resource_type: 'dns_zone',
    resource_id: id,
    page: 1,
    page_size: 1,
  })
  expect(revisions.length, 'creating a zone must write a revision').toBeGreaterThan(0)
  return { id, revision: Number(revisions[0].revision), content: (revisions[0].content ?? {}) as Record<string, unknown> }
}

async function openPage(page: Page) {
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  page.on('response', response => {
    if (response.status() >= 500) errors.push(`${response.status()} ${response.url()}`)
  })
  await page.goto('/settings/config-versions')
  await expect(page.getByRole('heading', { level: 2 }).first()).toBeVisible()
  await expect(page.locator('.n-spin-body')).toHaveCount(0)
  return errors
}

const bodyRows = (page: Page) => page.locator('.n-data-table').first().locator('tbody .n-data-table-tr')
const revisionOf = (row: Locator) => row.locator('td').nth(0)

const TYPE_COLUMN = 'td[data-col-key="resource_type"]'
const ID_COLUMN = 'td[data-col-key="resource_id"]'

/**
 * The row for one revision, narrowed by type and by resource id when given.
 *
 * Both narrowings are load-bearing rather than defensive. `dns_zone` and
 * `dns_records` share a resource id, and every resource's history starts at
 * revision 1, so "the first #1 with this type" is as likely to be another
 * resource's as this one's -- and the rollback button acts on the row's
 * resource.
 */
async function findRow(
  page: Page,
  revision: number,
  resourceType?: string,
  resourceId?: string,
): Promise<Locator> {
  const count = await bodyRows(page).count()
  for (let i = 0; i < count; i += 1) {
    const row = bodyRows(page).nth(i)
    const text = (await revisionOf(row).textContent())?.trim()
    if (text !== String(revision)) continue
    if (resourceType) {
      const type = (await row.locator(TYPE_COLUMN).textContent())?.trim()
      if (type !== resourceType) continue
    }
    if (resourceId) {
      const id = (await row.locator(ID_COLUMN).textContent())?.trim()
      if (id !== resourceId) continue
    }
    return row
  }
  throw new Error(`no revision row #${revision}${resourceType ? ` (${resourceType})` : ''} among ${count} rows`)
}

test('the page lists the types this instance publishes and the history it already wrote', async ({ page }) => {
  await login(page)
  const errors = await openPage(page)

  const types = ((await apiGet(page, '/config/types')).data as { resource_types: string[] }).resource_types
  expect(types.length).toBeGreaterThan(0)

  // Real rows, produced by the create hook rather than by this test's UI
  // interaction -- but produced *by this test*, so the check does not depend on
  // what an earlier spec happened to leave behind.
  const zone = await createZone(page)
  await page.reload()

  await expect(bodyRows(page).first()).toBeVisible()
  expect(await bodyRows(page).count()).toBeGreaterThan(0)

  // And the row is findable by the resource that wrote it, not just present.
  await findRow(page, zone.revision, 'DNS Zone', zone.id)

  expect(errors).toEqual([])
})

test('a rollback is published as a new revision, with the baseline for the row\'s own resource', async ({
  page,
}) => {
  await login(page)
  const errors = await openPage(page)

  // A live zone, created here: the publish below is refused with a 404 unless
  // the row's resource exists.
  const zone = await createZone(page)
  const resourceId = zone.id
  const baseRevision = zone.revision
  const content = zone.content

  // Publish one change so there are two revisions to choose between. The
  // revision number is read back from the response rather than assumed: a
  // publish that silently did nothing must fail here, not two steps later.
  const published = await postConfig(page, '/config/publish', {
    resource_type: 'dns_zone',
    resource_id: resourceId,
    expected_revision: baseRevision,
    content: { ...content, default_ttl: Number(content.default_ttl || 0) + 1 },
    note: 'config-versions e2e setup',
  })
  const secondRevision = Number((published.revision as { revision?: number } | undefined)?.revision)
  expect(Number.isFinite(secondRevision)).toBe(true)
  expect(secondRevision).toBe(baseRevision + 1)

  // Narrow the table to this one resource *type and id*.
  await page.locator('.n-base-selection').first().click()
  // Scoped to the dropdown: "DNS Zone" is also a tag in the type list and a
  // cell in the table, so an unscoped text match is ambiguous.
  await page
    .locator('.n-base-select-option')
    .filter({ hasText: /^DNS Zone$/ })
    .first()
    .click()
  await page.getByPlaceholder('e.g. a zone id').fill(resourceId)
  await page.getByRole('button', { name: 'Search', exact: true }).click()
  await expect(bodyRows(page).first()).toBeVisible()

  const targetRow = await findRow(page, baseRevision, 'DNS Zone', resourceId)
  await targetRow.getByRole('button', { name: 'Rollback', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Roll back to an earlier revision', exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Publish as new revision' }).click()

  await expect(page.getByText('Rollback published')).toBeVisible()

  // The rollback is a publish: a third revision carrying the first revision's
  // content, not a deletion of the second.
  await page.reload()
  await expect(bodyRows(page).first()).toBeVisible()
  await expect(revisionOf(await findRow(page, secondRevision + 1))).toHaveText(String(secondRevision + 1))

  const after = await listRevisions(page, {
    resource_type: 'dns_zone',
    resource_id: resourceId,
    page: 1,
    page_size: 1,
  })
  expect(Number(after[0].revision)).toBe(secondRevision + 1)
  expect(Number((after[0].content as Record<string, unknown>).default_ttl)).toBe(Number(content.default_ttl || 0))

  expect(errors).toEqual([])
})

test('a rollback from the unfiltered table cites the resource\'s own revision, not the row\'s', async ({ page }) => {
  // The regression this pins: the table mixes every resource, so "the newest
  // revision on screen" is usually another resource's number, and a baseline
  // taken from the row being rolled back is stale by construction -- every
  // rollback of anything but that resource's newest revision answered 409 and
  // the button could never succeed. The baseline is fetched per resource now.
  await login(page)
  const errors = await openPage(page)

  // Its own resource, so the unfiltered table holds a row that is
  // unambiguously this test's among the other resources' revision 1s.
  const zone = await createZone(page)
  const resourceId = zone.id
  const baseRevision = zone.revision
  const content = zone.content

  const published = await postConfig(page, '/config/publish', {
    resource_type: 'dns_zone',
    resource_id: resourceId,
    expected_revision: baseRevision,
    content: { ...content, default_ttl: Number(content.default_ttl || 0) + 1 },
    note: 'config-versions e2e unfiltered rollback',
  })
  expect(Number((published.revision as { revision?: number }).revision)).toBe(baseRevision + 1)

  // No filter at all: the row sits among other resources' revisions.
  await page.reload()
  await expect(bodyRows(page).first()).toBeVisible()

  const targetRow = await findRow(page, baseRevision, 'DNS Zone', resourceId)
  await targetRow.getByRole('button', { name: 'Rollback', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Roll back to an earlier revision', exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Publish as new revision' }).click()

  // A stale baseline is surfaced as a conflict instead of applied, so the
  // absence of the conflict banner is the assertion, not the success toast.
  await expect(page.getByText('Rollback published')).toBeVisible()
  await expect(page.getByText(/was modified concurrently/)).toHaveCount(0)

  expect(errors).toEqual([])
})
