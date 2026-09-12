import { test, expect, type Locator, type Page } from '@playwright/test'

// The W13 console surfaces, exercised against a real instance.
//
// Two things here cannot be checked without a browser and a running binary:
//
//   * the import goes file -> text -> JSON -> report -> rendered table. Every
//     step is a place to drop a field, and a unit test on either side would
//     still pass;
//   * the 360 view's value is what it shows when the four subsystems disagree,
//     which is a rendering question.
//
// The instance is started by scripts/w13_frontend_smoke.py up, and this file is
// run against that instance by its own CI step (`.fixture.spec.ts` selects the
// config in playwright.fixture.config.ts). The seed is not optional: the
// import dialog needs a subnet to import into, the sparse case needs a subnet
// that is *not* materialised, and the detail drawer needs an address that
// already has a DNS record. The other specs in this directory assume the
// opposite -- a table they alone have written to -- so the two sets cannot
// share an instance, and the file suffix is what keeps a new spec in the bare
// set by default rather than in no run at all.
//
// One property of the fixture decides what the report says. A subnet at or
// below autoCreateMaxAddresses (1<<16) is materialised: every address already
// has a row, so an import reports updates. A subnet above it stays sparse, so
// an import reports creates. Both are covered, because both are real paths.

const SUBNET_NAME = process.env.GODDI_SMOKE_SUBNET || 'w13-smoke-subnet'
const SUBNET_CIDR = process.env.GODDI_SMOKE_CIDR || '192.0.2.0/24'
const SPARSE_SUBNET_NAME = process.env.GODDI_SMOKE_SPARSE_SUBNET || 'w13-smoke-sparse'
const SPARSE_SUBNET_CIDR = process.env.GODDI_SMOKE_SPARSE_CIDR || '198.50.0.0/15'
const SEED_IP = process.env.GODDI_SMOKE_IP || '192.0.2.10'
const SEED_NAME = process.env.GODDI_SMOKE_NAME || 'seeded.w13smoke.test'

// The column order is positional: the header row is consumed, not validated.
// It has to be the order the exporter writes and the parser reads, which puts
// mac_address before hostname. Written the other way round, the MAC lands in
// the hostname column and the import reports success.
const CSV_HEADER = 'ip_address,status,mac_address,hostname,owner,device,location,description'

/**
 * The addresses this run writes.
 *
 * Drawn per run rather than fixed, because the suite writes and does not clean
 * up, and the instance keeps its database between runs. With fixed addresses a
 * second run would find them already carrying this file's values: the
 * materialised /24 would report Unchanged where the case is about Update, and
 * the sparse subnet would report Unchanged where the case is about Create --
 * a failure about the plan when the subject is really the fixture. The two
 * halves of the /24 draw are kept in disjoint ranges so they cannot collide,
 * and a run's addresses appear in its own output, so a failure stays readable.
 */
function octet(from: number, to: number): number {
  return from + Math.floor(Math.random() * (to - from + 1))
}
const NEW_IP = `192.0.2.${octet(2, 127)}`
const SECOND_IP = `192.0.2.${octet(128, 253)}`
const SPARSE_IP = `198.50.${octet(2, 127)}.${octet(2, 253)}`

const IMPORT_CSV = [
  CSV_HEADER,
  `${NEW_IP},used,aa:bb:cc:dd:ee:20,host-a,ops,dev1,rack1,created by the console smoke test`,
  `${SECOND_IP},used,,host-b,ops,dev2,rack1,created by the console smoke test`,
].join('\n')

const SPARSE_CSV = [CSV_HEADER, `${SPARSE_IP},used,,sparse-host,ops,dev9,rack9,nothing has a row for this yet`].join('\n')

// Two rows claiming the same address. The file contradicts itself, so no
// reading of it can be applied and the whole batch is refused.
const CONTRADICTORY_CSV = ['ip_address,status,hostname', '192.0.2.30,used,dup-a', '192.0.2.30,used,dup-b'].join('\n')

/**
 * A row for exactly this address.
 *
 * The address list holds every address in the subnet, so a plain substring
 * match for "192.0.2.10" also matches "192.0.2.100" through "192.0.2.109".
 * The lookahead is what makes these assertions about one address.
 */
function ipRow(page: Page, ip: string): Locator {
  const escaped = ip.replace(/\./g, '\\.')
  return page.locator('tr').filter({ hasText: new RegExp(`(^|[^0-9.])${escaped}(?![0-9])`) })
}

async function login(page: Page) {
  await page.addInitScript(() => {
    if (!localStorage.getItem('GODDI_lang')) localStorage.setItem('GODDI_lang', '"en-US"')
  })
  await page.goto('/login')
  await page.getByRole('textbox').nth(0).fill(process.env.GODDI_TEST_USERNAME || 'admin')
  await page.getByRole('textbox').nth(1).fill(process.env.GODDI_TEST_PASSWORD || 'Admin@123456')
  await page.getByRole('button', { name: 'Login', exact: true }).click()
  await expect(page).toHaveURL(/dashboard$/)
}

/** Open the import dialog for a subnet from the subnets table. */
async function openImportDialog(page: Page, subnetName: string, subnetCidr: string) {
  await page.goto('/ipam/subnets')
  const row = page.locator('tr', { hasText: subnetName })
  await expect(row).toBeVisible()
  await row.getByRole('button', { name: 'Import Addresses', exact: true }).click()

  const modal = page.locator('.n-modal').filter({ hasText: 'Import addresses' })
  await expect(modal).toBeVisible()
  // The entry point names the subnet it will write into: the operator does not
  // pick one again inside the dialog.
  await expect(modal.getByText(`${subnetName} (${subnetCidr})`)).toBeVisible()
  return modal
}

/** Put CSV text into the dialog through the file input, as an operator would. */
async function pickFile(modal: Locator, csv: string, name: string) {
  await modal.locator('input[type="file"]').setInputFiles({
    name,
    mimeType: 'text/csv',
    buffer: Buffer.from(csv, 'utf-8'),
  })
  // The dialog shows the chosen file's name once it has read the text.
  await expect(modal.getByText(name)).toBeVisible()
}

async function searchAddress(page: Page, ip: string) {
  await page.goto('/ipam/addresses')
  await page.getByPlaceholder('Search').first().fill(ip)
  await page.keyboard.press('Enter')
}

/** The credentials the page is holding, so a test can act as a second tab.
 *  Requests made through the browser already carry both; `page.request` does
 *  not, and a mutating call without the CSRF token is refused. */
async function authHeaders(page: Page) {
  return page.evaluate(() => ({
    Authorization: `Bearer ${sessionStorage.getItem('token') || ''}`,
    'X-CSRF-Token': sessionStorage.getItem('csrf_token') || '',
  }))
}

/** Open the pool wizard for a subnet from the subnets table. */
async function openPoolWizard(page: Page, subnetName: string, subnetCidr: string) {
  await page.goto('/ipam/subnets')
  const row = page.locator('tr', { hasText: subnetName })
  await expect(row).toBeVisible()
  await row.getByRole('button', { name: 'Create DHCP Scope', exact: true }).click()

  const modal = page.locator('.n-modal').filter({ hasText: 'Create DHCP scope from a subnet' })
  await expect(modal).toBeVisible()
  await expect(modal.getByText(`${subnetName} (${subnetCidr})`)).toBeVisible()
  return modal
}

test('the preview reports what the import would do and writes nothing', async ({ page }) => {
  await login(page)
  const modal = await openImportDialog(page, SUBNET_NAME, SUBNET_CIDR)
  await pickFile(modal, IMPORT_CSV, 'addresses.csv')

  await modal.getByRole('button', { name: 'Preview', exact: true }).click()

  // The banner must say this is a preview: a report that reads like a result
  // is the one way a dry run can mislead.
  await expect(modal.getByText('nothing has been written yet')).toBeVisible()

  // The /24 is materialised, so these rows already exist as "available" and the
  // plan has to say update, with the fields it would change.
  const row = modal.locator('tr').filter({ hasText: NEW_IP })
  await expect(row).toContainText('Update')
  await expect(row).toContainText('hostname')

  await expect(modal.getByRole('button', { name: 'Import', exact: true })).toBeEnabled()

  // Nothing was written: the address is still available and still unnamed.
  await searchAddress(page, NEW_IP)
  const listed = ipRow(page, NEW_IP)
  await expect(listed).toHaveCount(1)
  await expect(listed).toContainText('available')
  await expect(listed).not.toContainText('host-a')
})

test('a confirming import applies the plan, and a second preview then says unchanged', async ({ page }) => {
  await login(page)
  let modal = await openImportDialog(page, SUBNET_NAME, SUBNET_CIDR)
  await pickFile(modal, IMPORT_CSV, 'addresses.csv')

  await modal.getByRole('button', { name: 'Preview', exact: true }).click()
  await expect(modal.getByText('nothing has been written yet')).toBeVisible()

  await modal.getByRole('button', { name: 'Import', exact: true }).click()
  await expect(modal.getByText('Import result')).toBeVisible()

  // Applied: the status and the hostname landed.
  await searchAddress(page, NEW_IP)
  const listed = ipRow(page, NEW_IP)
  await expect(listed).toHaveCount(1)
  await expect(listed).toContainText('used')
  await expect(listed).toContainText('host-a')

  // Applying the same file again must be a no-op rather than a rewrite of every
  // row: a report that calls "nothing changed" a change is a report nobody
  // reads twice.
  modal = await openImportDialog(page, SUBNET_NAME, SUBNET_CIDR)
  await pickFile(modal, IMPORT_CSV, 'addresses.csv')
  await modal.getByRole('button', { name: 'Preview', exact: true }).click()

  const row = modal.locator('tr').filter({ hasText: NEW_IP })
  await expect(row).toContainText('Unchanged')
  await expect(row).not.toContainText('Update')
  await expect(modal.getByText('nothing has been written yet')).toBeVisible()
})

test('a sparse subnet reports creates, because nothing has a row there yet', async ({ page }) => {
  await login(page)
  const modal = await openImportDialog(page, SPARSE_SUBNET_NAME, SPARSE_SUBNET_CIDR)
  await pickFile(modal, SPARSE_CSV, 'sparse.csv')

  await modal.getByRole('button', { name: 'Preview', exact: true }).click()
  await expect(modal.getByText('nothing has been written yet')).toBeVisible()

  const row = modal.locator('tr').filter({ hasText: SPARSE_IP })
  await expect(row).toContainText('Create')

  await modal.getByRole('button', { name: 'Import', exact: true }).click()
  await expect(modal.getByText('Import result')).toBeVisible()

  await searchAddress(page, SPARSE_IP)
  const listed = ipRow(page, SPARSE_IP)
  await expect(listed).toHaveCount(1)
  await expect(listed).toContainText('used')
})

test('a file that contradicts itself is refused with the reason, and cannot be applied', async ({ page }) => {
  await login(page)
  const modal = await openImportDialog(page, SUBNET_NAME, SUBNET_CIDR)
  await pickFile(modal, CONTRADICTORY_CSV, 'contradictory.csv')

  await modal.getByRole('button', { name: 'Preview', exact: true }).click()

  // The refusal has to name the line; a bare "import failed" sends the
  // operator back to read the file by eye, which is what the preview is for.
  await expect(modal.getByText('Problems that stop the import')).toBeVisible()
  await expect(modal.getByText(/already appears on line/)).toBeVisible()

  // And the button that would spend a round trip to learn nothing is off.
  await expect(modal.getByRole('button', { name: 'Import', exact: true })).toBeDisabled()
})

test('the address detail drawer renders the four subsystems', async ({ page }) => {
  await login(page)
  await searchAddress(page, SEED_IP)

  const row = ipRow(page, SEED_IP)
  await expect(row).toHaveCount(1)
  await row.getByRole('button', { name: 'View detail', exact: true }).click()

  const drawer = page.locator('.n-drawer')
  await expect(drawer).toBeVisible()
  await expect(drawer.getByText(`IP detail · ${SEED_IP}`)).toBeVisible()

  // The DNS half: read from dns_records at the moment the question is asked,
  // so a name that publishes this address must appear.
  await expect(drawer.getByText('DNS records publishing this address')).toBeVisible()
  await expect(drawer.getByText(SEED_NAME)).toBeVisible()
  // The record was configured by hand, so the author tag must not claim a data
  // plane wrote it.
  await expect(drawer.getByText('Console', { exact: true })).toBeVisible()

  // The DHCP half, including the scope relation the backend only computes here.
  await expect(drawer.getByText('DHCP scope coverage')).toBeVisible()
  await expect(drawer.getByText('Change history')).toBeVisible()

  // The space and subnet the address belongs to are named, not implied.
  await expect(drawer.getByText('w13-smoke-space')).toBeVisible()
  await expect(drawer.getByText(`${SUBNET_NAME} (${SUBNET_CIDR})`)).toBeVisible()
})

// An address fenced off after the plan was read. High enough in the /24 to keep
// out of the import cases' way, and released again at the end: an address left
// fenced would change the plan a second run sees.
const FENCED_IP = '192.0.2.250'

test('the pool wizard plans from the subnet, and refuses a plan the world moved out of date', async ({ page }) => {
  await login(page)
  const modal = await openPoolWizard(page, SUBNET_NAME, SUBNET_CIDR)

  // Step 1 is IPAM's answer, not the form's: the range the subnet would hand
  // out, where the gateway came from, and what is fenced off inside it.
  await expect(modal.getByText(/192\.0\.2\.1\s*–\s*192\.0\.2\.254/)).toBeVisible()
  await expect(modal.getByText('Things to know')).toBeVisible()
  await expect(modal.getByText('Fenced-off addresses inside the range')).toBeVisible()
  await expect(modal.getByText('IPAM has fenced off nothing inside this range')).toBeVisible()
  await expect(modal.getByText('Plan fingerprint')).toBeVisible()

  const headers = await authHeaders(page)
  const listed = await page.request.get('/api/v1/ipam/subnets?page_size=100', { headers })
  const subnet = (await listed.json()).data.find((s: { name: string }) => s.name === SUBNET_NAME)
  // Thrown rather than asserted so the narrowing holds: everything below needs
  // the id, and a fixture that is missing is not a failing assertion about the
  // console.
  if (!subnet) throw new Error(`${SUBNET_NAME} is missing from the instance`)

  const scopeName = `smoke-pool-${Date.now()}`
  let createdScopeId = ''
  let reservedId = ''
  try {
    // Fence an address inside the range. This is what another operator's tab
    // does while this one is reading, and it is what the fingerprint is for.
    const reserved = await page.request.post('/api/v1/ipam/addresses/allocate', {
      headers,
      data: { subnet_id: subnet.id, ip_address: FENCED_IP, status: 'reserved', description: 'fenced by the pool wizard smoke test' },
    })
    expect(reserved.ok(), await reserved.text()).toBe(true)
    reservedId = (await reserved.json()).data.id

    await modal.locator('.n-form-item').filter({ hasText: 'Name' }).locator('input').fill(scopeName)
    await modal.getByRole('button', { name: 'Next', exact: true }).click()
    await expect(modal.getByText('This is the scope that will be created')).toBeVisible()
    await expect(modal.getByText('255.255.255.0')).toBeVisible()
    await expect(modal.getByText(scopeName)).toBeVisible()

    await modal.getByRole('button', { name: 'Create scope', exact: true }).click()

    // The server recomputes the plan and refuses. Creating the scope anyway
    // would hand clients the address that was fenced off after approval, which
    // is the exact failure the check exists to prevent.
    await expect(modal.getByText('The plan is out of date')).toBeVisible()
    // And it goes back to the plan rather than offering the button again: the
    // approval is spent.
    await expect(modal.getByRole('button', { name: 'Create scope', exact: true })).toHaveCount(0)

    // Re-planning picks up the fenced address, so the second attempt is an
    // approval of something that is true.
    await modal.getByRole('button', { name: 'Recalculate the plan', exact: true }).click()
    await expect(modal.getByText('The plan is out of date')).toHaveCount(0)
    await expect(modal.getByText(FENCED_IP)).toBeVisible()

    await modal.getByRole('button', { name: 'Next', exact: true }).click()
    await modal.getByRole('button', { name: 'Create scope', exact: true }).click()
    await expect(modal).toHaveCount(0)

    // The scope is on the server, not just in a toast.
    const scopes = await (await page.request.get('/api/v1/dhcp/scopes?page_size=100', { headers })).json()
    const created = scopes.data.find((s: { name: string }) => s.name === scopeName)
    if (!created) throw new Error(`${scopeName} was not created`)
    createdScopeId = created.id
    expect(created.start_ip).toBe('192.0.2.1')
    expect(created.end_ip).toBe('192.0.2.254')
  } finally {
    // The suite runs against an instance it does not own and may be run again,
    // and a leftover scope overlaps the range the next run plans for -- which
    // the plan reports as a conflict, so a second run would be stopped by the
    // litter of the first. The fenced address goes back for the same reason:
    // left reserved, it is in the plan the next run reads.
    if (createdScopeId) await page.request.delete(`/api/v1/dhcp/scopes/${createdScopeId}`, { headers })
    if (reservedId) await page.request.post('/api/v1/ipam/addresses/release', {
      headers,
      data: { id: reservedId, reason: 'pool wizard smoke test cleanup' },
    })
  }
})

/** Open the reverse-zone dialog for a subnet from the subnets table. */
async function openReverseZoneDialog(page: Page, subnetName: string, subnetCidr: string) {
  await page.goto('/ipam/subnets')
  const row = page.locator('tr', { hasText: subnetName })
  await expect(row).toBeVisible()
  await row.getByRole('button', { name: 'Generate Reverse Zone', exact: true }).click()

  const modal = page.locator('.n-modal').filter({ hasText: 'Generate Reverse Zone' })
  await expect(modal).toBeVisible()
  await expect(modal.getByText(`${subnetName} (${subnetCidr})`)).toBeVisible()
  return modal
}

test('the reverse zone button creates a zone, and says so when it cannot', async ({ page }) => {
  await login(page)
  const headers = await authHeaders(page)
  const zoneName = '2.0.192.in-addr.arpa'

  // The suite may run twice against one instance, and the second run would then
  // find the zone the first one created -- which is the refusal case below, not
  // the creation case. Clearing it first makes the two runs independent.
  const before = await (await page.request.get('/api/v1/dns/zones?page_size=200', { headers })).json()
  for (const zone of before.data as Array<{ id: string; name: string }>) {
    if (zone.name.startsWith(zoneName)) {
      await page.request.delete(`/api/v1/dns/zones/${zone.id}`, { headers })
    }
  }

  const modal = await openReverseZoneDialog(page, SUBNET_NAME, SUBNET_CIDR)

  // The subnet, not a name the server decided on its own.
  await expect(modal.getByText(SUBNET_CIDR).first()).toBeVisible()

  // Every delegation the addresses could be published in, most specific first.
  // Offering the choice is the point: an operator delegating 192.0.0.0/16
  // publishes the /16.
  //
  // The count is the assertion that matters: four options would mean the root
  // of the reverse tree is on offer too, and creating in-addr.arpa locally
  // takes over reverse resolution for everything -- never the answer to "where
  // does this subnet go".
  const radios = modal.locator('.n-radio')
  await expect(radios).toHaveCount(3)
  await expect(radios.nth(0)).toContainText(zoneName)
  await expect(radios.nth(1)).toContainText('0.192.in-addr.arpa')
  await expect(radios.nth(2)).toContainText('192.in-addr.arpa')
  await expect(modal.getByRole('radio', { name: 'in-addr.arpa', exact: true })).toHaveCount(0)

  // Most specific preselected. Preselected is not decided: the other two are on
  // screen and can be picked instead.
  await expect(radios.nth(0).locator('input')).toBeChecked()
  await expect(modal.getByText('best match')).toBeVisible()

  await modal.getByRole('button', { name: 'Create zone', exact: true }).click()
  await expect(modal).toHaveCount(0)

  // The write is real. This is the assertion the old button failed: it called
  // the compute endpoint, discarded the answer and reported success, so no zone
  // was ever created.
  const after = await (await page.request.get('/api/v1/dns/zones?page_size=200', { headers })).json()
  const created = (after.data as Array<{ id: string; name: string }>).find(z => z.name.startsWith(zoneName))
  expect(created, `${zoneName} was reported as created but is not in the zone list`).toBeTruthy()

  try {
    // And a second attempt is refused rather than reported as success. That is
    // the property separating a real write from a pretend one: a fake success
    // can be repeated forever, a genuine one cannot.
    const again = await openReverseZoneDialog(page, SUBNET_NAME, SUBNET_CIDR)
    await again.getByRole('button', { name: 'Create zone', exact: true }).click()
    await expect(page.getByText(/already exists/)).toBeVisible()
    await expect(again).toBeVisible()
  } finally {
    if (created) await page.request.delete(`/api/v1/dns/zones/${created.id}`, { headers })
  }
})

test('a DHCP scope description survives the form', async ({ page }) => {
  await login(page)
  const headers = await authHeaders(page)
  // Outside every seeded subnet, so the scope cannot overlap anything another
  // case plans for, and deleted at the end for the same reason.
  const scopeName = `smoke-desc-${Date.now()}`
  const comment = 'written by the console smoke test'

  let createdId = ''
  try {
    await page.goto('/dhcp/scopes')
    await page.getByRole('button', { name: 'Create Scope', exact: true }).click()

    const modal = page.locator('.n-modal').filter({ hasText: 'Create Scope' })
    await expect(modal).toBeVisible()
    await modal.locator('.n-form-item').filter({ hasText: 'Name' }).locator('input').fill(scopeName)
    await modal.locator('.n-form-item').filter({ hasText: 'Subnet' }).locator('input').fill('203.0.113.0/24')
    await modal.locator('.n-form-item').filter({ hasText: 'Start IP' }).locator('input').fill('203.0.113.10')
    await modal.locator('.n-form-item').filter({ hasText: 'End IP' }).locator('input').fill('203.0.113.20')
    await modal.locator('.n-form-item').filter({ hasText: 'Description' }).locator('textarea').fill(comment)
    await modal.getByRole('button', { name: 'Save', exact: true }).click()
    await expect(modal).toHaveCount(0)

    // Stored under the name the scope manager has, which is `comment`. The form
    // used to send `description`, an unknown key the backend dropped: the toast
    // said saved and the column stayed empty.
    const scopes = await (await page.request.get('/api/v1/dhcp/scopes?page_size=200', { headers })).json()
    const created = (scopes.data as Array<{ id: string; name: string; comment: string }>)
      .find(s => s.name === scopeName)
    if (!created) throw new Error(`${scopeName} was not created`)
    createdId = created.id
    expect(created.comment).toBe(comment)

    // And it comes back into the form, which is the other half of the same key
    // mismatch: reading `description` always produced an empty box.
    const row = page.locator('tr', { hasText: scopeName })
    await expect(row).toBeVisible()
    await row.getByRole('button', { name: 'Edit', exact: true }).click()
    const edit = page.locator('.n-modal').filter({ hasText: 'Edit Scope' })
    await expect(edit.locator('.n-form-item').filter({ hasText: 'Description' }).locator('textarea')).toHaveValue(comment)
  } finally {
    if (createdId) await page.request.delete(`/api/v1/dhcp/scopes/${createdId}`, { headers })
  }
})
