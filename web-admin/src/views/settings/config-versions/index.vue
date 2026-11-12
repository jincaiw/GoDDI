<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { Link as TLink, MessagePlugin, Tag as TTag } from 'tdesign-vue-next';
import type { PrimaryTableCol } from 'tdesign-vue-next';
import RemoteTable from '@/components/RemoteTable.vue';
import { usePermission } from '@/composables/usePermission';
import { ApiError } from '@/service/api/goddi/client';
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
  type ConfigPublishResult
} from '@/service/api/goddi/config';

defineOptions({ name: 'ConfigVersionsPage' });

const { t } = useI18n();
const { canWrite } = usePermission();
const writeAllowed = computed(() => canWrite('settings'));

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : t('common.failed');
}

// --- Resource types ---------------------------------------------------------
const resourceTypes = ref<string[]>([]);
const typesError = ref('');
const typeOptions = computed(() => resourceTypes.value.map(rt => ({ label: resourceTypeLabel(rt), value: rt })));

function resourceTypeLabel(rt: string): string {
  const key = `settings.configVersions.resourceTypes.${rt}`;
  const localized = t(key);
  return localized === key ? rt : localized;
}
function statusLabel(status: string): string {
  const key = `settings.configVersions.statusLabels.${status}`;
  const localized = t(key);
  return localized === key ? status : localized;
}
function statusTheme(status: string): 'success' | 'primary' | 'danger' | 'default' {
  switch (status) {
    case 'applied': return 'success';
    case 'staged': return 'primary';
    case 'failed': return 'danger';
    default: return 'default';
  }
}

async function loadTypes() {
  typesError.value = '';
  try {
    const res = await listConfigTypes();
    resourceTypes.value = res.resource_types ?? [];
  } catch (err: unknown) {
    typesError.value = errorMessage(err);
  }
}

// --- Revisions --------------------------------------------------------------
const filterType = ref<string | undefined>(undefined);
const filterId = ref('');
const revisionsError = ref('');
const revisionTable = ref<{ reload: () => Promise<void> } | null>(null);
const isRecordsType = computed(() => filterType.value === 'dns_records');
const diffComputed = ref(false);
const revisionsCache = ref<ConfigRevision[]>([]);

const revisionParams = computed<Record<string, unknown>>(() => {
  const p: Record<string, unknown> = {};
  if (filterType.value) p.resource_type = filterType.value;
  if (filterId.value.trim()) p.resource_id = filterId.value.trim();
  return p;
});

/** Wraps getList so diff/rollback can cite the revisions shown on screen. */
async function fetchRevisions(params: Record<string, unknown>) {
  revisionsError.value = '';
  try {
    const result = await listConfigRevisions(params);
    revisionsCache.value = result.data as ConfigRevision[];
    prefillContent();
    return result;
  } catch (err) {
    revisionsError.value = errorMessage(err);
    revisionsCache.value = [];
    throw err;
  }
}

function handleFilterChange() {
  diffComputed.value = false;
  void revisionTable.value?.reload();
}

// A failed revision never reached the data plane, so rolling back to it would
// restore a configuration that was never in service.
const revisionColumns: PrimaryTableCol[] = [
  { colKey: 'revision', title: t('settings.configVersions.revision'), width: 90 },
  {
    colKey: 'status',
    title: t('settings.configVersions.status'),
    width: 120,
    cell: (h, { row }) => {
      const r = row as ConfigRevision;
      return h(TTag, { size: 'small', theme: statusTheme(r.status), variant: 'light' }, () => statusLabel(r.status));
    }
  },
  {
    colKey: 'resource_type',
    title: t('settings.configVersions.resourceType'),
    width: 130,
    cell: (_h, { row }) => resourceTypeLabel((row as ConfigRevision).resource_type)
  },
  { colKey: 'resource_id', title: t('settings.configVersions.resourceId'), width: 200, ellipsis: true },
  {
    colKey: 'actor',
    title: t('settings.configVersions.actor'),
    width: 120,
    cell: (_h, { row }) => (row as ConfigRevision).actor || '-'
  },
  {
    colKey: 'note',
    title: t('settings.configVersions.note'),
    ellipsis: true,
    cell: (_h, { row }) => (row as ConfigRevision).note || '-'
  },
  { colKey: 'created_at', title: t('settings.configVersions.createdAt'), width: 170 },
  {
    colKey: 'applied_at',
    title: t('settings.configVersions.appliedAt'),
    width: 170,
    cell: (_h, { row }) => (row as ConfigRevision).applied_at || '-'
  },
  { colKey: 'applied_generation', title: t('settings.configVersions.appliedGeneration'), width: 130 },
  {
    colKey: 'op',
    title: t('common.actions'),
    width: 110,
    cell: (h, { row }) => {
      const r = row as ConfigRevision;
      if (!writeAllowed.value || r.status === 'failed') return '-';
      return h(TLink, { theme: 'warning', onClick: () => openRollback(r) }, () => t('settings.configVersions.rollback'));
    }
  }
];

function revisionRowClass(params: { row: unknown }): string {
  return (params.row as ConfigRevision).status === 'failed' ? 'cv-row-failed' : '';
}

// --- Diff -------------------------------------------------------------------
const diffFrom = ref<number | undefined>(undefined);
const diffTo = ref<number | undefined>(undefined);
const diffLoading = ref(false);
const diffError = ref('');
const fieldChanges = ref<ConfigFieldChange[]>([]);
const recordDiffRows = ref<RecordDiffRow[]>([]);

const revisionOptions = computed(() =>
  revisionsCache.value.map(r => ({
    label: `#${r.revision} ${statusLabel(r.status)}${r.actor ? ` · ${r.actor}` : ''}`,
    value: r.revision
  }))
);

function renderValue(value: unknown): string {
  if (value === undefined || value === null) return '—';
  if (typeof value === 'string') return value;
  return JSON.stringify(value);
}

const fieldChangeColumns: PrimaryTableCol[] = [
  { colKey: 'field', title: t('settings.configVersions.field'), width: 200 },
  { colKey: 'old', title: t('settings.configVersions.oldValue'), ellipsis: true, cell: (_h, { row }) => renderValue((row as ConfigFieldChange).old) },
  { colKey: 'new', title: t('settings.configVersions.newValue'), ellipsis: true, cell: (_h, { row }) => renderValue((row as ConfigFieldChange).new) }
];

async function computeDiff() {
  if (!filterType.value || !filterId.value.trim() || diffFrom.value === undefined || diffTo.value === undefined) return;
  diffLoading.value = true;
  diffError.value = '';
  diffComputed.value = false;
  try {
    const res = await diffConfigRevisions({
      resource_type: filterType.value,
      resource_id: filterId.value.trim(),
      from: diffFrom.value,
      to: diffTo.value
    });
    fieldChanges.value = res.changes ?? [];
    // For dns_records the server-side diff reports the whole `records` array as
    // one changed field; classify record-level changes from the two revisions.
    if (isRecordsType.value) {
      const fromRev = revisionsCache.value.find(r => r.revision === diffFrom.value);
      const toRev = revisionsCache.value.find(r => r.revision === diffTo.value);
      recordDiffRows.value = diffRecords(fromRev?.content, toRev?.content);
    } else {
      recordDiffRows.value = [];
    }
    diffComputed.value = true;
  } catch (err: unknown) {
    diffError.value = errorMessage(err);
  } finally {
    diffLoading.value = false;
  }
}

// --- dns_records record-level diff -----------------------------------------
// Matching: by `id` when both sides carry one; otherwise by name+type only
// when that key is unique on both sides. Unmatched records are reported as
// removed + added, never silently paired.
interface DnsRecordItem {
  id?: string; name: string; type: string; value: string; ttl: number; enabled: boolean;
  priority?: number; weight?: number; port?: number; flag?: number; tag?: string;
  comment?: string; tags?: string; owner?: string; expires_at?: string;
}
type RecordDiffKind = 'added' | 'removed' | 'changed';
interface RecordDiffRow {
  kind: RecordDiffKind; name: string; type: string; value: string; changedFields?: string[];
}
const RECORD_FIELDS = [
  'name', 'type', 'value', 'ttl', 'enabled', 'priority', 'weight', 'port', 'flag',
  'tag', 'comment', 'tags', 'owner', 'expires_at'
] as const;

function parseRecordItems(content: unknown): DnsRecordItem[] {
  if (!content || typeof content !== 'object') return [];
  const records = (content as { records?: unknown }).records;
  return Array.isArray(records) ? (records as DnsRecordItem[]) : [];
}
function sameField(a: unknown, b: unknown): boolean {
  return JSON.stringify(a ?? null) === JSON.stringify(b ?? null);
}
function recordKey(r: DnsRecordItem): string {
  return `${r.name}\u0000${r.type}`;
}
function groupIndices(items: DnsRecordItem[], matched: boolean[]): Map<string, number[]> {
  const groups = new Map<string, number[]>();
  items.forEach((item, index) => {
    if (matched[index]) return;
    const key = recordKey(item);
    const bucket = groups.get(key);
    if (bucket) bucket.push(index);
    else groups.set(key, [index]);
  });
  return groups;
}
function diffRecords(fromContent: unknown, toContent: unknown): RecordDiffRow[] {
  const from = parseRecordItems(fromContent);
  const to = parseRecordItems(toContent);
  const fromMatched = from.map(() => false);
  const toMatched = to.map(() => false);
  const pairs: Array<[number, number]> = [];

  const toById = new Map<string, number>();
  to.forEach((r, j) => { if (r.id) toById.set(r.id, j); });
  from.forEach((r, i) => {
    if (!r.id) return;
    const j = toById.get(r.id);
    if (j !== undefined && !toMatched[j]) {
      fromMatched[i] = true; toMatched[j] = true; pairs.push([i, j]);
    }
  });

  const fromGroups = groupIndices(from, fromMatched);
  const toGroups = groupIndices(to, toMatched);
  fromGroups.forEach((idxs, key) => {
    const others = toGroups.get(key);
    if (idxs.length === 1 && others && others.length === 1) {
      const i = idxs[0], j = others[0];
      fromMatched[i] = true; toMatched[j] = true; pairs.push([i, j]);
    }
  });

  const rows: RecordDiffRow[] = [];
  pairs.forEach(([i, j]) => {
    const changedFields = RECORD_FIELDS.filter(f => !sameField(from[i][f], to[j][f]));
    if (changedFields.length > 0) {
      rows.push({ kind: 'changed', name: to[j].name, type: to[j].type, value: to[j].value, changedFields });
    }
  });
  from.forEach((r, i) => { if (!fromMatched[i]) rows.push({ kind: 'removed', name: r.name, type: r.type, value: r.value }); });
  to.forEach((r, j) => { if (!toMatched[j]) rows.push({ kind: 'added', name: r.name, type: r.type, value: r.value }); });
  return rows;
}

const recordDiffCounts = computed(() => ({
  added: recordDiffRows.value.filter(r => r.kind === 'added').length,
  removed: recordDiffRows.value.filter(r => r.kind === 'removed').length,
  changed: recordDiffRows.value.filter(r => r.kind === 'changed').length
}));

const recordDiffColumns: PrimaryTableCol[] = [
  {
    colKey: 'kind',
    title: t('settings.configVersions.status'),
    width: 100,
    cell: (h, { row }) => {
      const r = row as RecordDiffRow;
      const theme = r.kind === 'added' ? 'success' : r.kind === 'removed' ? 'danger' : 'warning';
      return h(TTag, { size: 'small', theme, variant: 'light' }, () => t(`settings.configVersions.recordDiffKind.${r.kind}`));
    }
  },
  { colKey: 'name', title: t('settings.configVersions.recordName'), width: 220, ellipsis: true },
  { colKey: 'type', title: t('settings.configVersions.recordType'), width: 90 },
  { colKey: 'value', title: t('settings.configVersions.recordValue'), ellipsis: true },
  {
    colKey: 'changedFields',
    title: t('settings.configVersions.changedFields'),
    width: 260,
    cell: (_h, { row }) => {
      const r = row as RecordDiffRow;
      return r.changedFields && r.changedFields.length ? r.changedFields.join(', ') : '-';
    }
  }
];
function recordDiffKey(row: RecordDiffRow): string {
  return `${row.kind}:${row.name}:${row.type}:${row.value}`;
}

// --- Publish (dry run) ------------------------------------------------------
const contentText = ref('');
const contentTouched = ref(false);
const publishLoading = ref(false);
const publishError = ref('');
const dryRunResult = ref<ConfigPublishResult | null>(null);

function prefillContent() {
  if (contentTouched.value || !isRecordsType.value) return;
  const newest = revisionsCache.value[0];
  if (newest) contentText.value = JSON.stringify(newest.content, null, 2);
}

const currentRevision = computed(() => {
  if (!filterType.value || !filterId.value.trim() || revisionsCache.value.length === 0) return 0;
  return revisionsCache.value.reduce((max, r) => Math.max(max, r.revision), 0);
});

async function runDryRun() {
  publishLoading.value = true;
  publishError.value = '';
  dryRunResult.value = null;
  try {
    let content: unknown;
    try {
      content = JSON.parse(contentText.value || '{}');
    } catch (err: unknown) {
      throw new Error(t('settings.configVersions.invalidJson'), { cause: err });
    }
    dryRunResult.value = await publishConfig({
      resource_type: 'dns_records',
      resource_id: filterId.value.trim(),
      content,
      expected_revision: currentRevision.value,
      dry_run: true
    });
  } catch (err: unknown) {
    publishError.value = errorMessage(err);
  } finally {
    publishLoading.value = false;
  }
}

// --- Rollback ---------------------------------------------------------------
const showRollback = ref(false);
const rollbackRow = ref<ConfigRevision | null>(null);
const rollbackExpected = ref<number>(0);
const rollbackNote = ref('');
const rollbackLoading = ref(false);
const rollbackError = ref('');
const rollbackConflict = ref('');
const rollbackBaselineLoading = ref(false);

/** Fetch the concurrency baseline for the row's own resource, not the
 *  unfiltered table (which usually shows another resource's newest number). */
async function openRollback(row: ConfigRevision) {
  if (!canWrite('settings')) return;
  rollbackRow.value = row;
  rollbackExpected.value = row.revision;
  rollbackNote.value = '';
  rollbackError.value = '';
  rollbackConflict.value = '';
  showRollback.value = true;
  rollbackBaselineLoading.value = true;
  try {
    const result = await listConfigRevisions({
      resource_type: row.resource_type,
      resource_id: row.resource_id,
      page: 1,
      page_size: 1
    });
    const newest = result.data.reduce((max, r) => Math.max(max, r.revision), 0);
    if (newest > 0) rollbackExpected.value = newest;
  } catch {
    // Baseline stays at the row's own revision; a wrong baseline is reported
    // as a conflict rather than applied.
  } finally {
    rollbackBaselineLoading.value = false;
  }
}

async function submitRollback() {
  if (!canWrite('settings')) return;
  const row = rollbackRow.value;
  if (!row) return;
  rollbackLoading.value = true;
  rollbackError.value = '';
  rollbackConflict.value = '';
  try {
    await rollbackConfig({
      resource_type: row.resource_type,
      resource_id: row.resource_id,
      to_revision: row.revision,
      expected_revision: rollbackExpected.value,
      note: rollbackNote.value || undefined
    });
    void MessagePlugin.success(t('settings.configVersions.rollbackSuccess'));
    showRollback.value = false;
    contentTouched.value = false;
    await refreshAll();
  } catch (err: unknown) {
    // A 409 is not bad content; another writer published first.
    if (err instanceof ApiError && err.status === 409) rollbackConflict.value = err.message;
    else rollbackError.value = errorMessage(err);
  } finally {
    rollbackLoading.value = false;
  }
}

// --- Release queue ----------------------------------------------------------
const releases = ref<ConfigRelease[]>([]);
const releaseStats = reactive({ pending: 0, failed: 0 });
const queueError = ref('');
const retrying = ref(false);
const queueTable = ref<{ reload: () => Promise<void> } | null>(null);

async function fetchQueue(_params: Record<string, unknown>) {
  queueError.value = '';
  try {
    const res = await listConfigReleases();
    releases.value = res.releases ?? [];
    releaseStats.pending = res.stats?.pending ?? 0;
    releaseStats.failed = res.stats?.failed ?? 0;
    return { data: releases.value, meta: { total: releases.value.length } };
  } catch (err) {
    releases.value = [];
    releaseStats.pending = 0;
    releaseStats.failed = 0;
    queueError.value = errorMessage(err);
    throw err;
  }
}

async function retryFailed(scoped?: ConfigRelease) {
  if (!canWrite('settings')) return;
  retrying.value = true;
  try {
    const res = await retryConfigReleases(
      scoped ? { resource_type: scoped.resource_type, resource_id: scoped.resource_id } : undefined
    );
    void MessagePlugin.success(t('settings.configVersions.retryResult', { count: res.reset }));
    await queueTable.value?.reload();
  } catch (err: unknown) {
    void MessagePlugin.error(errorMessage(err));
  } finally {
    retrying.value = false;
  }
}

const releaseColumns: PrimaryTableCol[] = [
  {
    colKey: 'status',
    title: t('settings.configVersions.status'),
    width: 110,
    cell: (h, { row }) => {
      const r = row as ConfigRelease;
      return h(TTag, { size: 'small', theme: statusTheme(r.status), variant: 'light' }, () => String(r.status));
    }
  },
  {
    colKey: 'resource_type',
    title: t('settings.configVersions.resourceType'),
    width: 130,
    cell: (_h, { row }) => resourceTypeLabel((row as ConfigRelease).resource_type)
  },
  { colKey: 'resource_id', title: t('settings.configVersions.resourceId'), width: 180, ellipsis: true },
  { colKey: 'revision', title: t('settings.configVersions.revision'), width: 90 },
  { colKey: 'attempts', title: t('settings.configVersions.queueAttempts'), width: 90 },
  { colKey: 'applied_generation', title: t('settings.configVersions.appliedGeneration'), width: 130 },
  {
    colKey: 'last_error',
    title: t('settings.configVersions.queueLastError'),
    ellipsis: true,
    cell: (_h, { row }) => (row as ConfigRelease).last_error || '-'
  },
  { colKey: 'created_at', title: t('settings.configVersions.createdAt'), width: 170 },
  {
    colKey: 'op',
    title: t('common.actions'),
    width: 100,
    cell: (h, { row }) => {
      const r = row as ConfigRelease;
      if (!writeAllowed.value || r.status !== 'failed') return '-';
      return h(TLink, { theme: 'warning', onClick: () => void retryFailed(r) }, () => t('settings.configVersions.retry'));
    }
  }
];

async function refreshAll() {
  await Promise.all([loadTypes(), revisionTable.value?.reload(), queueTable.value?.reload()]);
}

onMounted(() => { void refreshAll(); });
</script>

<template>
  <div class="td-page">
    <TCard :bordered="false" class="td-page-header">
      <h2>{{ t('settings.configVersions.title') }}</h2>
      <TButton variant="outline" @click="refreshAll">{{ t('common.refresh') }}</TButton>
    </TCard>

    <!-- Resource types this instance can publish. -->
    <TCard :bordered="false" :title="t('settings.configVersions.resourceTypesTitle')" class="cv-card">
      <TAlert v-if="typesError" theme="warning" :message="typesError" />
      <template v-else>
        <TSpace v-if="resourceTypes.length" size="small">
          <TTag v-for="rt in resourceTypes" :key="rt" size="small" theme="primary" variant="light">
            {{ resourceTypeLabel(rt) }}
          </TTag>
        </TSpace>
        <span v-else class="cv-hint">{{ t('settings.configVersions.resourceTypesEmpty') }}</span>
      </template>
    </TCard>

    <!-- Revision history. -->
    <TCard :bordered="false" :title="t('settings.configVersions.revisionsTitle')" class="cv-card">
      <TSpace size="small" align="center" class="cv-filter">
        <TSelect
          v-model="filterType"
          :options="typeOptions"
          :placeholder="t('settings.configVersions.filterResourceType')"
          clearable
          style="width: 200px"
          @change="handleFilterChange"
        />
        <TInput
          v-model="filterId"
          :placeholder="t('settings.configVersions.resourceIdPlaceholder')"
          clearable
          style="width: 300px"
          @enter="handleFilterChange"
        />
        <TButton theme="primary" @click="handleFilterChange">{{ t('common.search') }}</TButton>
      </TSpace>

      <TAlert v-if="revisionsError" theme="error" :message="revisionsError" class="cv-alert" />
      <RemoteTable
        ref="revisionTable"
        :columns="revisionColumns"
        :fetcher="fetchRevisions"
        :params="revisionParams"
        :row-class-name="revisionRowClass"
      />
      <span v-if="revisionsCache.length === 0 && !revisionsError" class="cv-empty">
        {{ t('settings.configVersions.emptyRevisions') }}
      </span>
    </TCard>

    <!-- Diff between two revisions. -->
    <TCard :bordered="false" :title="t('settings.configVersions.diffTitle')" class="cv-card">
      <span class="cv-hint">{{ t('settings.configVersions.diffHint') }}</span>
      <TSpace size="small" align="center" class="cv-filter">
        <TSelect v-model="diffFrom" :options="revisionOptions" :placeholder="t('settings.configVersions.diffFrom')" style="width: 220px" />
        <span>→</span>
        <TSelect v-model="diffTo" :options="revisionOptions" :placeholder="t('settings.configVersions.diffTo')" style="width: 220px" />
        <TButton theme="primary" :disabled="diffFrom === undefined || diffTo === undefined" :loading="diffLoading" @click="computeDiff">
          {{ t('settings.configVersions.computeDiff') }}
        </TButton>
      </TSpace>

      <TAlert v-if="diffError" theme="error" :message="diffError" class="cv-alert" />
      <template v-if="diffComputed && !diffError">
        <TAlert v-if="fieldChanges.length === 0 && recordDiffRows.length === 0" theme="success" :message="t('settings.configVersions.noChanges')" />
        <template v-else>
          <TDivider>{{ t('settings.configVersions.fieldChanges') }}</TDivider>
          <TTable v-if="fieldChanges.length" :columns="fieldChangeColumns" :data="fieldChanges" row-key="field" size="small" bordered />
          <span v-else class="cv-hint">{{ t('settings.configVersions.noFieldChanges') }}</span>

          <template v-if="isRecordsType">
            <TDivider>{{ t('settings.configVersions.recordChanges') }}</TDivider>
            <span class="cv-hint">{{ t('settings.configVersions.recordChangesHint') }}</span>
            <TTable
              v-if="recordDiffRows.length"
              :columns="recordDiffColumns"
              :data="recordDiffRows"
              :row-key="recordDiffKey"
              size="small"
              bordered
            />
            <span v-else class="cv-hint">{{ t('settings.configVersions.noRecordChanges') }}</span>
            <span v-if="recordDiffRows.length" class="cv-hint">
              {{ t('settings.configVersions.recordDiffSummary', recordDiffCounts) }}
            </span>
          </template>
        </template>
      </template>
    </TCard>

    <!-- Dry-run publish. -->
    <TCard :bordered="false" :title="t('settings.configVersions.publishTitle')" class="cv-card">
      <span class="cv-hint">{{ t('settings.configVersions.publishHint') }}</span>
      <TAlert v-if="!isRecordsType" theme="info" :message="t('settings.configVersions.publishApiOnly')" class="cv-alert" />
      <template v-else>
        <TForm label-align="top">
          <TFormItem :label="t('settings.configVersions.contentJson')">
            <TTextarea
              v-model="contentText"
              :autosize="{ minRows: 12, maxRows: 12 }"
              @change="contentTouched = true"
            />
          </TFormItem>
        </TForm>
        <TAlert v-if="publishError" theme="error" :message="publishError" class="cv-alert" />
        <TButton
          theme="primary"
          :disabled="!writeAllowed || !filterId || filterType !== 'dns_records'"
          :loading="publishLoading"
          @click="runDryRun"
        >
          {{ t('settings.configVersions.dryRun') }}
        </TButton>
        <TAlert v-if="!writeAllowed" theme="warning" :message="t('settings.configVersions.writeRequired')" class="cv-alert" />

        <template v-if="dryRunResult">
          <TDivider>{{ t('settings.configVersions.dryRunResult') }}</TDivider>
          <TDescriptions :column="2" size="small" bordered>
            <TDescriptionsItem :label="t('settings.configVersions.dryRunBaseRevision')">
              {{ dryRunResult.base_revision }}
            </TDescriptionsItem>
            <TDescriptionsItem :label="t('settings.configVersions.dryRunReplayed')">
              {{ dryRunResult.replayed ? t('common.yesOrNo.yes') : t('common.yesOrNo.no') }}
            </TDescriptionsItem>
          </TDescriptions>
          <TTable
            v-if="dryRunResult.changes.length"
            :columns="fieldChangeColumns"
            :data="dryRunResult.changes"
            row-key="field"
            size="small"
            bordered
            class="cv-table-gap"
          />
          <span v-else class="cv-hint">{{ t('settings.configVersions.noChanges') }}</span>
        </template>
      </template>
    </TCard>

    <!-- Data-plane release queue. -->
    <TCard :bordered="false" :title="t('settings.configVersions.queueTitle')" class="cv-card">
      <TSpace size="large" align="center" class="cv-stats">
        <div class="cv-stat"><span class="cv-stat-label">{{ t('settings.configVersions.queuePending') }}</span><b>{{ releaseStats.pending }}</b></div>
        <div class="cv-stat"><span class="cv-stat-label">{{ t('settings.configVersions.queueFailed') }}</span><b :class="{ danger: releaseStats.failed > 0 }">{{ releaseStats.failed }}</b></div>
        <TButton
          v-if="writeAllowed"
          theme="warning"
          size="small"
          :disabled="releaseStats.failed === 0"
          :loading="retrying"
          @click="retryFailed()"
        >
          {{ t('settings.configVersions.retryFailed') }}
        </TButton>
      </TSpace>

      <TAlert v-if="queueError" theme="error" :message="queueError" class="cv-alert" />
      <RemoteTable ref="queueTable" :columns="releaseColumns" :fetcher="fetchQueue" :show-pagination="false" />
      <span v-if="releases.length === 0 && !queueError" class="cv-empty">{{ t('settings.configVersions.queueEmpty') }}</span>
    </TCard>

    <!-- Rollback confirmation. -->
    <TDialog
      v-model:visible="showRollback"
      :header="t('settings.configVersions.rollbackTitle')"
      :confirm-btn="{ loading: rollbackLoading, disabled: !writeAllowed || rollbackBaselineLoading, theme: 'warning', content: t('settings.configVersions.rollbackConfirm') }"
      :cancel-btn="t('common.cancel')"
      width="520px"
      @confirm="submitRollback"
    >
      <TDescriptions :column="1" size="small" bordered class="mb-3">
        <TDescriptionsItem :label="t('settings.configVersions.resourceType')">
          {{ rollbackRow ? resourceTypeLabel(rollbackRow.resource_type) : '' }}
        </TDescriptionsItem>
        <TDescriptionsItem :label="t('settings.configVersions.resourceId')">{{ rollbackRow?.resource_id }}</TDescriptionsItem>
        <TDescriptionsItem :label="t('settings.configVersions.rollbackTarget')">#{{ rollbackRow?.revision }}</TDescriptionsItem>
      </TDescriptions>

      <TForm label-align="top">
        <TFormItem :label="t('settings.configVersions.rollbackExpected')" :help="t('settings.configVersions.rollbackExpectedHint')">
          <TInputNumber v-model="rollbackExpected" :min="0" style="width: 160px" />
        </TFormItem>
        <TFormItem :label="t('settings.configVersions.rollbackNote')">
          <TInput v-model="rollbackNote" />
        </TFormItem>
      </TForm>

      <TAlert v-if="rollbackError" theme="error" :message="rollbackError" />
      <TAlert
        v-if="rollbackConflict"
        theme="warning"
        :message="t('settings.configVersions.conflictBody', { message: rollbackConflict })"
        class="cv-alert"
      />
    </TDialog>
  </div>
</template>

<style scoped>
.td-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.td-page-header :deep(.t-card__body) {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px 20px;
}
.td-page-header h2 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
}
.cv-filter {
  margin-bottom: 12px;
}
.cv-hint {
  display: block;
  font-size: 12px;
  color: #9ca3af;
  margin-bottom: 8px;
}
.cv-alert {
  margin-bottom: 12px;
}
.cv-empty {
  display: block;
  margin-top: 12px;
  font-size: 13px;
  color: #9ca3af;
}
.cv-stats {
  margin-bottom: 12px;
}
.cv-stat {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.cv-stat-label {
  font-size: 12px;
  color: #6b7280;
}
.cv-stat b {
  font-size: 22px;
}
.cv-stat .danger {
  color: #d54941;
}
.cv-table-gap {
  margin-top: 12px;
}
.mb-3 {
  margin-bottom: 12px;
}
:deep(.cv-row-failed) {
  background-color: rgba(208, 48, 80, 0.1);
}
</style>
