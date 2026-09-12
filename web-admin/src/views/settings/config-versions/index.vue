<template>
  <div>
    <page-header :title="t('settings.configVersions.title')">
      <n-button @click="refreshAll">{{ t('common.refresh') }}</n-button>
    </page-header>

    <!-- Resource types this instance can publish. -->
    <n-card :title="t('settings.configVersions.resourceTypesTitle')" size="small" class="cv-card">
      <n-alert v-if="typesError" type="warning" :show-icon="true">{{ typesError }}</n-alert>
      <template v-else>
        <n-space v-if="resourceTypes.length">
          <n-tag v-for="rt in resourceTypes" :key="rt" type="info" size="small">{{ resourceTypeLabel(rt) }}</n-tag>
        </n-space>
        <n-text v-else depth="3">{{ t('settings.configVersions.resourceTypesEmpty') }}</n-text>
      </template>
    </n-card>

    <!-- Revision history. -->
    <n-card :title="t('settings.configVersions.revisionsTitle')" size="small" class="cv-card">
      <div class="cv-filter">
        <n-space align="center">
          <n-select
            v-model:value="filterType"
            :options="typeOptions"
            :placeholder="t('settings.configVersions.filterResourceType')"
            clearable
            style="width: 200px;"
            @update:value="handleFilterChange"
          />
          <n-input
            v-model:value="filterId"
            :placeholder="t('settings.configVersions.resourceIdPlaceholder')"
            clearable
            style="width: 300px;"
            @keyup.enter="handleFilterChange"
          />
          <n-button type="primary" @click="handleFilterChange">{{ t('common.search') }}</n-button>
        </n-space>
      </div>

      <n-alert v-if="revisionsError" type="error" :show-icon="true" class="cv-alert">{{ revisionsError }}</n-alert>

      <n-data-table
        :columns="revisionColumns"
        :data="revisions"
        :loading="loadingRevisions"
        :row-key="(row: ConfigRevision) => row.id"
        :row-props="revisionRowProps"
        :pagination="pagination"
        remote
        :bordered="true"
        size="small"
        @update:page="handlePageChange"
        @update:page-size="handlePageSizeChange"
      />
      <n-text v-if="!loadingRevisions && revisions.length === 0 && !revisionsError" depth="3" class="cv-empty">
        {{ t('settings.configVersions.emptyRevisions') }}
      </n-text>
    </n-card>

    <!-- Diff between two revisions. -->
    <n-card :title="t('settings.configVersions.diffTitle')" size="small" class="cv-card">
      <n-text depth="3" class="cv-hint">{{ t('settings.configVersions.diffHint') }}</n-text>
      <n-space align="center" class="cv-filter">
        <n-select
          v-model:value="diffFrom"
          :options="revisionOptions"
          :placeholder="t('settings.configVersions.diffFrom')"
          style="width: 220px;"
        />
        <span>→</span>
        <n-select
          v-model:value="diffTo"
          :options="revisionOptions"
          :placeholder="t('settings.configVersions.diffTo')"
          style="width: 220px;"
        />
        <n-button type="primary" :disabled="diffFrom === null || diffTo === null" :loading="diffLoading" @click="computeDiff">
          {{ t('settings.configVersions.computeDiff') }}
        </n-button>
      </n-space>

      <n-alert v-if="diffError" type="error" :show-icon="true" class="cv-alert">{{ diffError }}</n-alert>

      <template v-if="diffComputed && !diffError">
        <n-alert v-if="fieldChanges.length === 0 && recordDiffRows.length === 0" type="success" :show-icon="true">
          {{ t('settings.configVersions.noChanges') }}
        </n-alert>

        <template v-else>
          <n-divider>{{ t('settings.configVersions.fieldChanges') }}</n-divider>
          <n-data-table
            v-if="fieldChanges.length"
            :columns="fieldChangeColumns"
            :data="fieldChanges"
            :row-key="(row: ConfigFieldChange) => row.field"
            :bordered="true"
            size="small"
          />
          <n-text v-else depth="3">{{ t('settings.configVersions.noFieldChanges') }}</n-text>

          <template v-if="isRecordsType">
            <n-divider>{{ t('settings.configVersions.recordChanges') }}</n-divider>
            <n-text depth="3" class="cv-hint">{{ t('settings.configVersions.recordChangesHint') }}</n-text>
            <n-data-table
              v-if="recordDiffRows.length"
              :columns="recordDiffColumns"
              :data="recordDiffRows"
              :bordered="true"
              size="small"
              :row-key="recordDiffKey"
            />
            <n-text v-else depth="3">{{ t('settings.configVersions.noRecordChanges') }}</n-text>
            <n-text v-if="recordDiffRows.length" depth="3" class="cv-hint">
              {{ t('settings.configVersions.recordDiffSummary', recordDiffCounts) }}
            </n-text>
          </template>
        </template>
      </template>
    </n-card>

    <!-- Dry-run publish. -->
    <n-card :title="t('settings.configVersions.publishTitle')" size="small" class="cv-card">
      <n-text depth="3" class="cv-hint">{{ t('settings.configVersions.publishHint') }}</n-text>
      <n-alert v-if="!isRecordsType" type="info" :show-icon="true" class="cv-alert">
        {{ t('settings.configVersions.publishApiOnly') }}
      </n-alert>
      <template v-else>
        <n-form-item :label="t('settings.configVersions.contentJson')" label-placement="top">
          <n-input v-model:value="contentText" type="textarea" :rows="12" spellcheck="false" />
        </n-form-item>
        <n-alert v-if="publishError" type="error" :show-icon="true" class="cv-alert">{{ publishError }}</n-alert>
        <n-button
          type="primary"
          :disabled="!canWrite || !filterId || filterType !== 'dns_records'"
          :loading="publishLoading"
          @click="runDryRun"
        >
          {{ t('settings.configVersions.dryRun') }}
        </n-button>
        <n-alert v-if="!canWrite" type="warning" :show-icon="true" class="cv-alert">
          {{ t('settings.configVersions.writeRequired') }}
        </n-alert>

        <template v-if="dryRunResult">
          <n-divider>{{ t('settings.configVersions.dryRunResult') }}</n-divider>
          <n-descriptions :column="2" size="small" bordered>
            <n-descriptions-item :label="t('settings.configVersions.dryRunBaseRevision')">
              {{ dryRunResult.base_revision }}
            </n-descriptions-item>
            <n-descriptions-item :label="t('settings.configVersions.dryRunReplayed')">
              {{ dryRunResult.replayed ? t('common.yesOrNo.yes') : t('common.yesOrNo.no') }}
            </n-descriptions-item>
          </n-descriptions>
          <n-data-table
            v-if="dryRunResult.changes.length"
            :columns="fieldChangeColumns"
            :data="dryRunResult.changes"
            :row-key="(row: ConfigFieldChange) => row.field"
            :bordered="true"
            size="small"
            class="cv-table-gap"
          />
          <n-text v-else depth="3">{{ t('settings.configVersions.noChanges') }}</n-text>
        </template>
      </template>
    </n-card>

    <!-- Data-plane release queue. -->
    <n-card :title="t('settings.configVersions.queueTitle')" size="small" class="cv-card">
      <n-space align="center" class="cv-stats">
        <n-statistic :label="t('settings.configVersions.queuePending')" :value="releaseStats.pending" />
        <n-statistic :label="t('settings.configVersions.queueFailed')">
          <n-text :type="releaseStats.failed > 0 ? 'error' : undefined">{{ releaseStats.failed }}</n-text>
        </n-statistic>
        <n-button
          v-if="writeAllowed"
          type="warning"
          size="small"
          :disabled="releaseStats.failed === 0"
          :loading="retrying"
          @click="retryFailed()"
        >
          {{ t('settings.configVersions.retryFailed') }}
        </n-button>
      </n-space>

      <n-alert v-if="queueError" type="error" :show-icon="true" class="cv-alert">{{ queueError }}</n-alert>

      <n-data-table
        :columns="releaseColumns"
        :data="releases"
        :loading="loadingQueue"
        :row-key="(row: ConfigRelease) => String(row.id)"
        :bordered="true"
        size="small"
      />
      <n-text v-if="!loadingQueue && releases.length === 0 && !queueError" depth="3" class="cv-empty">
        {{ t('settings.configVersions.queueEmpty') }}
      </n-text>
    </n-card>

    <!-- Rollback confirmation. -->
    <n-modal
      v-model:show="showRollback"
      preset="card"
      :title="t('settings.configVersions.rollbackTitle')"
      style="width: 520px;"
    >
      <n-descriptions :column="1" size="small" bordered>
        <n-descriptions-item :label="t('settings.configVersions.resourceType')">
          {{ rollbackRow ? resourceTypeLabel(rollbackRow.resource_type) : '' }}
        </n-descriptions-item>
        <n-descriptions-item :label="t('settings.configVersions.resourceId')">
          {{ rollbackRow?.resource_id }}
        </n-descriptions-item>
        <n-descriptions-item :label="t('settings.configVersions.rollbackTarget')">
          #{{ rollbackRow?.revision }}
        </n-descriptions-item>
      </n-descriptions>

      <n-form-item :label="t('settings.configVersions.rollbackExpected')" label-placement="top">
        <n-input-number v-model:value="rollbackExpected" :min="0" style="width: 160px;" />
        <template #feedback>
          <span class="cv-hint">{{ t('settings.configVersions.rollbackExpectedHint') }}</span>
        </template>
      </n-form-item>
      <n-form-item :label="t('settings.configVersions.rollbackNote')" label-placement="top">
        <n-input v-model:value="rollbackNote" />
      </n-form-item>

      <n-alert v-if="rollbackError" type="error" :show-icon="true">{{ rollbackError }}</n-alert>
      <n-alert v-if="rollbackConflict" type="warning" :show-icon="true" class="cv-alert">
        {{ t('settings.configVersions.conflictBody', { message: rollbackConflict }) }}
      </n-alert>

      <template #footer>
        <n-space justify="end">
          <n-button @click="showRollback = false">{{ t('common.cancel') }}</n-button>
          <n-button
            type="warning"
            :loading="rollbackLoading"
            :disabled="rollbackBaselineLoading"
            @click="submitRollback"
          >
            {{ t('settings.configVersions.rollbackConfirm') }}
          </n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { computed, h, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NTag, useMessage } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import { usePermission } from '@/composables/usePermission'
import { ApiError } from '@/service/api/goddi/client'
import {
  diffConfigRevisions,
  listConfigReleases,
  listConfigRevisions,
  listConfigTypes,
  publishConfig,
  retryConfigReleases,
  rollbackConfig,
  type ConfigFieldChange,
  type ConfigRelease,
  type ConfigRevision,
  type ConfigRevisionStatus,
  type ConfigPublishResult,
} from '@/service/api/goddi/config'

const { t } = useI18n()
const message = useMessage()
const { canWrite } = usePermission()

const writeAllowed = computed(() => canWrite('settings'))

// --- Types -----------------------------------------------------------------

const resourceTypes = ref<string[]>([])
const typesError = ref('')

const typeOptions = computed(() =>
  resourceTypes.value.map(rt => ({ label: resourceTypeLabel(rt), value: rt }))
)

function resourceTypeLabel(rt: string): string {
  const key = `settings.configVersions.resourceTypes.${rt}`
  const localized = t(key)
  return localized === key ? rt : localized
}

function statusLabel(status: string): string {
  const key = `settings.configVersions.statusLabels.${status}`
  const localized = t(key)
  return localized === key ? status : localized
}

function statusTagType(status: ConfigRevisionStatus): 'default' | 'success' | 'info' | 'error' {
  switch (status) {
    case 'applied':
      return 'success'
    case 'staged':
      return 'info'
    case 'failed':
      return 'error'
    default:
      return 'default'
  }
}

async function loadTypes() {
  typesError.value = ''
  try {
    const res = await listConfigTypes()
    resourceTypes.value = res.resource_types ?? []
  } catch (err: unknown) {
    typesError.value = errorMessage(err)
  }
}

// --- Revisions -------------------------------------------------------------

const revisions = ref<ConfigRevision[]>([])
const loadingRevisions = ref(false)
const revisionsError = ref('')
const filterType = ref<string | null>(null)
const filterId = ref('')
const pagination = reactive({
  page: 1,
  pageSize: 20,
  itemCount: 0,
  showSizePicker: true,
  pageSizes: [10, 20, 50],
})

const isRecordsType = computed(() => filterType.value === 'dns_records')

async function loadRevisions() {
  loadingRevisions.value = true
  revisionsError.value = ''
  try {
    const params: Record<string, unknown> = { page: pagination.page, page_size: pagination.pageSize }
    if (filterType.value) params.resource_type = filterType.value
    if (filterId.value.trim()) params.resource_id = filterId.value.trim()
    const result = await listConfigRevisions(params)
    revisions.value = result.data
    pagination.itemCount = result.meta.total
    prefillContent()
  } catch (err: unknown) {
    revisions.value = []
    pagination.itemCount = 0
    revisionsError.value = errorMessage(err)
  } finally {
    loadingRevisions.value = false
  }
}

function handleFilterChange() {
  pagination.page = 1
  diffComputed.value = false
  loadRevisions()
}

function handlePageChange(page: number) {
  pagination.page = page
  loadRevisions()
}

function handlePageSizeChange(pageSize: number) {
  pagination.pageSize = pageSize
  pagination.page = 1
  loadRevisions()
}

const revisionColumns = [
  { title: () => t('settings.configVersions.revision'), key: 'revision', width: 90 },
  {
    title: () => t('settings.configVersions.status'),
    key: 'status',
    width: 120,
    render: (row: ConfigRevision) =>
      h(NTag, { size: 'small', type: statusTagType(row.status), bordered: false }, { default: () => statusLabel(row.status) }),
  },
  {
    title: () => t('settings.configVersions.resourceType'),
    key: 'resource_type',
    width: 130,
    render: (row: ConfigRevision) => resourceTypeLabel(row.resource_type),
  },
  { title: () => t('settings.configVersions.resourceId'), key: 'resource_id', width: 200, ellipsis: { tooltip: true } },
  { title: () => t('settings.configVersions.actor'), key: 'actor', width: 120, render: (row: ConfigRevision) => row.actor || '-' },
  { title: () => t('settings.configVersions.note'), key: 'note', ellipsis: { tooltip: true }, render: (row: ConfigRevision) => row.note || '-' },
  { title: () => t('settings.configVersions.createdAt'), key: 'created_at', width: 170 },
  { title: () => t('settings.configVersions.appliedAt'), key: 'applied_at', width: 170, render: (row: ConfigRevision) => row.applied_at || '-' },
  { title: () => t('settings.configVersions.appliedGeneration'), key: 'applied_generation', width: 130 },
  {
    title: () => t('common.actions'),
    key: 'actions',
    width: 110,
    render: (row: ConfigRevision) => {
      // A failed revision never reached the data plane, so rolling back to it
      // would restore a configuration that was never in service.
      const restorable = row.status !== 'failed'
      if (!writeAllowed.value || !restorable) return '-'
      return h(
        NButton,
        { size: 'small', text: true, type: 'warning', onClick: () => openRollback(row) },
        { default: () => t('settings.configVersions.rollback') }
      )
    },
  },
]

// A failed release must be impossible to scroll past: the row is tinted and
// the status tag is an error, because this is the one state where the
// configuration was recorded but never reached the data plane.
function revisionRowProps(row: ConfigRevision) {
  if (row.status === 'failed') return { style: 'background-color: rgba(208, 48, 80, 0.10);' }
  return {}
}

// --- Diff ------------------------------------------------------------------

const diffFrom = ref<number | null>(null)
const diffTo = ref<number | null>(null)
const diffLoading = ref(false)
const diffError = ref('')
const diffComputed = ref(false)
const fieldChanges = ref<ConfigFieldChange[]>([])
const recordDiffRows = ref<RecordDiffRow[]>([])

const revisionOptions = computed(() =>
  revisions.value.map(r => ({
    label: `#${r.revision} ${statusLabel(r.status)}${r.actor ? ` · ${r.actor}` : ''}`,
    value: r.revision,
  }))
)

const fieldChangeColumns = [
  { title: () => t('settings.configVersions.field'), key: 'field', width: 200 },
  {
    title: () => t('settings.configVersions.oldValue'),
    key: 'old',
    render: (row: ConfigFieldChange) => renderValue(row.old),
  },
  {
    title: () => t('settings.configVersions.newValue'),
    key: 'new',
    render: (row: ConfigFieldChange) => renderValue(row.new),
  },
]

function renderValue(value: unknown): string {
  if (value === undefined || value === null) return '—'
  if (typeof value === 'string') return value
  return JSON.stringify(value)
}

async function computeDiff() {
  if (!filterType.value || !filterId.value.trim() || diffFrom.value === null || diffTo.value === null) return
  diffLoading.value = true
  diffError.value = ''
  diffComputed.value = false
  try {
    const res = await diffConfigRevisions({
      resource_type: filterType.value,
      resource_id: filterId.value.trim(),
      from: diffFrom.value,
      to: diffTo.value,
    })
    fieldChanges.value = res.changes ?? []
    // For dns_records the server-side diff reports the whole `records` array as
    // one changed field, which tells an operator nothing. The record-level
    // classification is computed here from the two revisions' content.
    if (isRecordsType.value) {
      const fromRev = revisions.value.find(r => r.revision === diffFrom.value)
      const toRev = revisions.value.find(r => r.revision === diffTo.value)
      recordDiffRows.value = diffRecords(fromRev?.content, toRev?.content)
    } else {
      recordDiffRows.value = []
    }
    diffComputed.value = true
  } catch (err: unknown) {
    diffError.value = errorMessage(err)
  } finally {
    diffLoading.value = false
  }
}

// --- dns_records record-level diff -----------------------------------------
//
// Matching rule (stated so the output can be trusted or challenged):
//   1. Match by `id` when both sides carry one. `id` is the row identity and
//      is what a rollback restores, so it is the strongest signal.
//   2. Otherwise match by `name` + `type`, but ONLY when that key is unique on
//      both sides. If a key repeats on either side the pairing would be a
//      guess (which of the two www A records is the same one?), so those
//      records are reported as removed + added rather than silently paired.
//   3. A matched pair whose fields differ is `changed`; a matched pair with no
//      differing field is omitted from the table.
// Nothing is ever reported as `changed` unless both sides were actually
// matched; an unclassifiable record shows up as removed + added.

interface DnsRecordItem {
  id?: string
  name: string
  type: string
  value: string
  ttl: number
  enabled: boolean
  priority?: number
  weight?: number
  port?: number
  flag?: number
  tag?: string
  comment?: string
  tags?: string
  owner?: string
  expires_at?: string
}

type RecordDiffKind = 'added' | 'removed' | 'changed'

interface RecordDiffRow {
  kind: RecordDiffKind
  name: string
  type: string
  value: string
  changedFields?: string[]
}

const RECORD_FIELDS = [
  'name',
  'type',
  'value',
  'ttl',
  'enabled',
  'priority',
  'weight',
  'port',
  'flag',
  'tag',
  'comment',
  'tags',
  'owner',
  'expires_at',
] as const

function parseRecordItems(content: unknown): DnsRecordItem[] {
  if (!content || typeof content !== 'object') return []
  const records = (content as { records?: unknown }).records
  return Array.isArray(records) ? (records as DnsRecordItem[]) : []
}

function sameField(a: unknown, b: unknown): boolean {
  return JSON.stringify(a ?? null) === JSON.stringify(b ?? null)
}

function recordKey(r: DnsRecordItem): string {
  return `${r.name}\u0000${r.type}`
}

function groupIndices(items: DnsRecordItem[], matched: boolean[]): Map<string, number[]> {
  const groups = new Map<string, number[]>()
  items.forEach((item, index) => {
    if (matched[index]) return
    const key = recordKey(item)
    const bucket = groups.get(key)
    if (bucket) bucket.push(index)
    else groups.set(key, [index])
  })
  return groups
}

function diffRecords(fromContent: unknown, toContent: unknown): RecordDiffRow[] {
  const from = parseRecordItems(fromContent)
  const to = parseRecordItems(toContent)

  const fromMatched = from.map(() => false)
  const toMatched = to.map(() => false)
  const pairs: Array<[number, number]> = []

  // 1) by id on both sides
  const toById = new Map<string, number>()
  to.forEach((r, j) => {
    if (r.id) toById.set(r.id, j)
  })
  from.forEach((r, i) => {
    if (!r.id) return
    const j = toById.get(r.id)
    if (j !== undefined && !toMatched[j]) {
      fromMatched[i] = true
      toMatched[j] = true
      pairs.push([i, j])
    }
  })

  // 2) by name+type, only when unambiguous on both sides
  const fromGroups = groupIndices(from, fromMatched)
  const toGroups = groupIndices(to, toMatched)
  fromGroups.forEach((idxs, key) => {
    const others = toGroups.get(key)
    if (idxs.length === 1 && others && others.length === 1) {
      const i = idxs[0]
      const j = others[0]
      fromMatched[i] = true
      toMatched[j] = true
      pairs.push([i, j])
    }
  })

  const rows: RecordDiffRow[] = []
  pairs.forEach(([i, j]) => {
    const changedFields = RECORD_FIELDS.filter(f => !sameField(from[i][f], to[j][f]))
    if (changedFields.length > 0) {
      rows.push({ kind: 'changed', name: to[j].name, type: to[j].type, value: to[j].value, changedFields })
    }
  })
  from.forEach((r, i) => {
    if (!fromMatched[i]) rows.push({ kind: 'removed', name: r.name, type: r.type, value: r.value })
  })
  to.forEach((r, j) => {
    if (!toMatched[j]) rows.push({ kind: 'added', name: r.name, type: r.type, value: r.value })
  })
  return rows
}

const recordDiffCounts = computed(() => ({
  added: recordDiffRows.value.filter(r => r.kind === 'added').length,
  removed: recordDiffRows.value.filter(r => r.kind === 'removed').length,
  changed: recordDiffRows.value.filter(r => r.kind === 'changed').length,
}))

const recordDiffColumns = [
  {
    title: () => t('settings.configVersions.status'),
    key: 'kind',
    width: 100,
    render: (row: RecordDiffRow) =>
      h(
        NTag,
        {
          size: 'small',
          type: row.kind === 'added' ? 'success' : row.kind === 'removed' ? 'error' : 'warning',
          bordered: false,
        },
        { default: () => t(`settings.configVersions.recordDiffKind.${row.kind}`) }
      ),
  },
  { title: () => t('settings.configVersions.recordName'), key: 'name', width: 220, ellipsis: { tooltip: true } },
  { title: () => t('settings.configVersions.recordType'), key: 'type', width: 90 },
  { title: () => t('settings.configVersions.recordValue'), key: 'value', ellipsis: { tooltip: true } },
  {
    title: () => t('settings.configVersions.changedFields'),
    key: 'changedFields',
    width: 260,
    render: (row: RecordDiffRow) => (row.changedFields && row.changedFields.length ? row.changedFields.join(', ') : '-'),
  },
]

function recordDiffKey(row: RecordDiffRow): string {
  return `${row.kind}:${row.name}:${row.type}:${row.value}`
}

// --- Publish (dry run) -----------------------------------------------------

const contentText = ref('')
const contentTouched = ref(false)
const publishLoading = ref(false)
const publishError = ref('')
const dryRunResult = ref<ConfigPublishResult | null>(null)

function prefillContent() {
  if (contentTouched.value || !isRecordsType.value) return
  const newest = revisions.value[0]
  if (newest) {
    contentText.value = JSON.stringify(newest.content, null, 2)
  }
}

async function runDryRun() {
  publishLoading.value = true
  publishError.value = ''
  dryRunResult.value = null
  try {
    let content: unknown
    try {
      content = JSON.parse(contentText.value || '{}')
    } catch (err: unknown) {
      throw new Error(t('settings.configVersions.invalidJson'))
    }
    const res = await publishConfig({
      resource_type: 'dns_records',
      resource_id: filterId.value.trim(),
      content,
      expected_revision: currentRevision.value,
      dry_run: true,
    })
    dryRunResult.value = res
  } catch (err: unknown) {
    publishError.value = errorMessage(err)
  } finally {
    publishLoading.value = false
  }
}

// The newest revision of the currently filtered resource, which is what a
// publish or rollback must cite as its optimistic-concurrency baseline.
const currentRevision = computed(() => {
  if (!filterType.value || !filterId.value.trim() || revisions.value.length === 0) return 0
  return revisions.value.reduce((max, r) => Math.max(max, r.revision), 0)
})

// --- Rollback --------------------------------------------------------------

const showRollback = ref(false)
const rollbackRow = ref<ConfigRevision | null>(null)
const rollbackExpected = ref<number>(0)
const rollbackNote = ref('')
const rollbackLoading = ref(false)
const rollbackError = ref('')
const rollbackConflict = ref('')
const rollbackBaselineLoading = ref(false)

/**
 * Open the rollback dialog for one row.
 *
 * The concurrency baseline is fetched for the row's own resource rather than
 * taken from the table. The unfiltered table mixes every resource, so "the
 * newest revision on screen" is usually another resource's number, and citing
 * the row's own number as its baseline makes every rollback of anything but
 * the newest revision of that resource a guaranteed 409 -- a button that
 * looks available and can never succeed. The baseline is the answer to "what
 * did you think was current", so it is what this resource is at now.
 */
async function openRollback(row: ConfigRevision) {
  rollbackRow.value = row
  rollbackExpected.value = row.revision
  rollbackNote.value = ''
  rollbackError.value = ''
  rollbackConflict.value = ''
  showRollback.value = true
  rollbackBaselineLoading.value = true
  try {
    const result = await listConfigRevisions({
      resource_type: row.resource_type,
      resource_id: row.resource_id,
      page: 1,
      page_size: 1,
    })
    const newest = result.data.reduce((max, r) => Math.max(max, r.revision), 0)
    if (newest > 0) rollbackExpected.value = newest
  } catch {
    // The baseline stays at the row's own revision: the operator can still
    // edit the number by hand, and a wrong baseline is reported as a conflict
    // rather than applied -- which is the failure this dialog is built to show.
  } finally {
    rollbackBaselineLoading.value = false
  }
}

async function submitRollback() {
  const row = rollbackRow.value
  if (!row) return
  rollbackLoading.value = true
  rollbackError.value = ''
  rollbackConflict.value = ''
  try {
    await rollbackConfig({
      resource_type: row.resource_type,
      resource_id: row.resource_id,
      to_revision: row.revision,
      expected_revision: rollbackExpected.value,
      note: rollbackNote.value || undefined,
    })
    message.success(t('settings.configVersions.rollbackSuccess'))
    showRollback.value = false
    contentTouched.value = false
    loadRevisions()
    loadQueue()
  } catch (err: unknown) {
    // A 409 is not "bad content": another writer published first, so the
    // operator must reload and re-apply rather than edit the form.
    if (err instanceof ApiError && err.status === 409) {
      rollbackConflict.value = err.message
    } else {
      rollbackError.value = errorMessage(err)
    }
  } finally {
    rollbackLoading.value = false
  }
}

// --- Release queue ---------------------------------------------------------

const releases = ref<ConfigRelease[]>([])
const releaseStats = reactive({ pending: 0, failed: 0 })
const loadingQueue = ref(false)
const queueError = ref('')
const retrying = ref(false)

async function loadQueue() {
  loadingQueue.value = true
  queueError.value = ''
  try {
    const res = await listConfigReleases()
    releases.value = res.releases ?? []
    releaseStats.pending = res.stats?.pending ?? 0
    releaseStats.failed = res.stats?.failed ?? 0
  } catch (err: unknown) {
    releases.value = []
    releaseStats.pending = 0
    releaseStats.failed = 0
    queueError.value = errorMessage(err)
  } finally {
    loadingQueue.value = false
  }
}

async function retryFailed(scoped?: ConfigRelease) {
  retrying.value = true
  try {
    const res = await retryConfigReleases(
      scoped ? { resource_type: scoped.resource_type, resource_id: scoped.resource_id } : undefined
    )
    message.success(t('settings.configVersions.retryResult', { count: res.reset }))
    loadQueue()
  } catch (err: unknown) {
    message.error(errorMessage(err))
  } finally {
    retrying.value = false
  }
}

const releaseColumns = [
  {
    title: () => t('settings.configVersions.status'),
    key: 'status',
    width: 110,
    render: (row: ConfigRelease) =>
      h(
        NTag,
        { size: 'small', type: row.status === 'failed' ? 'error' : row.status === 'applied' ? 'success' : 'info', bordered: false },
        { default: () => row.status }
      ),
  },
  {
    title: () => t('settings.configVersions.resourceType'),
    key: 'resource_type',
    width: 130,
    render: (row: ConfigRelease) => resourceTypeLabel(row.resource_type),
  },
  { title: () => t('settings.configVersions.resourceId'), key: 'resource_id', width: 180, ellipsis: { tooltip: true } },
  { title: () => t('settings.configVersions.revision'), key: 'revision', width: 90 },
  { title: () => t('settings.configVersions.queueAttempts'), key: 'attempts', width: 90 },
  { title: () => t('settings.configVersions.appliedGeneration'), key: 'applied_generation', width: 130 },
  {
    title: () => t('settings.configVersions.queueLastError'),
    key: 'last_error',
    ellipsis: { tooltip: true },
    render: (row: ConfigRelease) => row.last_error || '-',
  },
  { title: () => t('settings.configVersions.createdAt'), key: 'created_at', width: 170 },
  {
    title: () => t('common.actions'),
    key: 'actions',
    width: 100,
    render: (row: ConfigRelease) => {
      if (!writeAllowed.value || row.status !== 'failed') return '-'
      return h(NButton, { size: 'small', text: true, type: 'warning', onClick: () => retryFailed(row) }, { default: () => t('settings.configVersions.retry') })
    },
  },
]

// --- Shared helpers --------------------------------------------------------

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : t('common.failed')
}

function refreshAll() {
  loadTypes()
  loadRevisions()
  loadQueue()
}

onMounted(() => {
  refreshAll()
})
</script>

<style scoped>
.cv-card {
  margin-bottom: 16px;
}
.cv-filter {
  margin-bottom: 12px;
}
.cv-hint {
  display: block;
  font-size: 12px;
  margin-bottom: 8px;
}
.cv-alert {
  margin-bottom: 12px;
}
.cv-empty {
  display: block;
  margin-top: 12px;
}
.cv-stats {
  margin-bottom: 12px;
}
.cv-table-gap {
  margin-top: 12px;
}
</style>
