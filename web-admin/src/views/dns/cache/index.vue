<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { DialogPlugin, Link as TLink, MessagePlugin } from 'tdesign-vue-next';
import type { PrimaryTableCol } from 'tdesign-vue-next';
import RemoteTable from '@/components/RemoteTable.vue';
import { usePermission } from '@/composables/usePermission';
import {
  flushDNSCache,
  flushDNSCacheEntry,
  getDNSCacheStats,
  listCacheEntries,
  type CacheEntry,
  type CacheStats
} from '@/service/api/goddi/dns';

defineOptions({ name: 'DNSCachePage' });

const { t } = useI18n();
const perm = usePermission();

const stats = ref<CacheStats>({
  entries: 0,
  max_entries: 0,
  hits: 0,
  misses: 0,
  hit_rate: 0,
  miss_rate: 0,
  size_bytes: 0
});
const filterQname = ref('');
const tableRef = ref<{ reload: () => Promise<void> } | null>(null);

const params = computed<Record<string, unknown>>(() => (filterQname.value ? { qname: filterQname.value } : {}));

function formatTTL(seconds: number): string {
  if (seconds <= 0) return '-';
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
  return `${Math.floor(seconds / 3600)}h`;
}

function formatTime(value: string): string {
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? value : d.toLocaleString();
}

function msgError(err: unknown) {
  void MessagePlugin.error(err instanceof Error ? err.message : t('common.failed'));
}

const columns: PrimaryTableCol[] = [
  { colKey: 'qname', title: t('dns.cache.colName'), minWidth: 220, ellipsis: true },
  { colKey: 'qtype', title: t('dns.cache.colType'), width: 100 },
  {
    colKey: 'ttl_left',
    title: t('dns.cache.colTTL'),
    width: 100,
    cell: (_h, { row }) => formatTTL((row as CacheEntry).ttl_left)
  },
  { colKey: 'hit_count', title: t('dns.cache.colHits'), width: 90 },
  {
    colKey: 'expires_at',
    title: t('dns.cache.colExpires'),
    width: 190,
    cell: (_h, { row }) => formatTime((row as CacheEntry).expires_at)
  },
  {
    colKey: 'op',
    title: t('common.actions'),
    width: 110,
    cell: (h, { row }) =>
      h(
        TLink,
        { theme: 'warning', disabled: !perm.canDelete('dns'), onClick: () => void handleFlushEntry(row as CacheEntry) },
        () => t('dns.cache.flushEntry')
      )
  }
];

async function loadStats() {
  try {
    stats.value = await getDNSCacheStats();
  } catch (err) {
    msgError(err);
  }
}

async function refreshAll() {
  await Promise.all([loadStats(), tableRef.value?.reload()]);
}

function confirmFlushCache() {
  if (!perm.canDelete('dns')) return;
  const dlg = DialogPlugin.confirm({
    header: t('dns.cache.flushCache'),
    body: t('dns.cache.flushConfirm'),
    theme: 'danger',
    onConfirm: async () => {
      try {
        await flushDNSCache();
        void MessagePlugin.success(t('common.success'));
        await refreshAll();
      } catch (err) {
        msgError(err);
      }
      dlg.hide();
    }
  });
}

async function handleFlushEntry(row: CacheEntry) {
  if (!perm.canDelete('dns')) return;
  try {
    await flushDNSCacheEntry(row.qname, row.qtype);
    void MessagePlugin.success(t('common.success'));
    await refreshAll();
  } catch (err) {
    msgError(err);
  }
}

const statCards = computed(() => [
  { label: t('dns.cache.entries'), value: String(stats.value.entries) },
  { label: t('dns.cache.maxEntries'), value: String(stats.value.max_entries) },
  { label: t('dns.cache.hitRate'), value: `${(stats.value.hit_rate ?? 0).toFixed(1)}%` },
  { label: t('dns.cache.missRate'), value: `${(stats.value.miss_rate ?? 0).toFixed(1)}%` }
]);

onMounted(loadStats);
</script>

<template>
  <div class="td-page">
    <TCard :bordered="false" class="td-page-header">
      <h2>{{ t('dns.cache.title') }}</h2>
      <TButton v-if="perm.canDelete('dns')" theme="danger" variant="outline" @click="confirmFlushCache">
        {{ t('dns.cache.flushCache') }}
      </TButton>
    </TCard>

    <div class="stat-grid">
      <TCard v-for="card in statCards" :key="card.label" :bordered="false" class="stat-card">
        <div class="stat-label">{{ card.label }}</div>
        <div class="stat-value">{{ card.value }}</div>
      </TCard>
    </div>

    <TCard :bordered="false" class="filter-bar">
      <TSpace size="small">
        <TInput
          v-model="filterQname"
          clearable
          size="small"
          :placeholder="t('dns.cache.filterQname')"
          style="width: 220px"
          @enter="tableRef?.reload()"
        />
        <TButton size="small" variant="outline" @click="tableRef?.reload()">
          {{ t('common.search') }}
        </TButton>
        <span class="filter-title">{{ t('dns.cache.entryList') }}</span>
      </TSpace>
    </TCard>

    <RemoteTable ref="tableRef" :columns="columns" :fetcher="listCacheEntries" :params="params" />
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
.stat-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 16px;
}
.stat-card :deep(.t-card__body) {
  padding: 16px 20px;
}
.stat-label {
  font-size: 13px;
  color: #6b7280;
}
.stat-value {
  margin-top: 8px;
  font-size: 24px;
  font-weight: 600;
}
.filter-bar :deep(.t-card__body) {
  padding: 12px 16px;
}
.filter-title {
  margin-left: 8px;
  font-size: 14px;
  font-weight: 600;
}
@media (max-width: 900px) {
  .stat-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
