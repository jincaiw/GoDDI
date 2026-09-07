<template>
  <div>
    <page-header :title="t('dns.cache.title')">
      <n-button v-if="perm.canDelete('dns')" type="error" @click="handleFlushCache">{{ t('dns.cache.flushCache') }}</n-button>
    </page-header>

    <n-grid :cols="4" :x-gap="16" :y-gap="16" responsive="screen" item-responsive>
      <n-gi span="4 m:2 l:1">
        <n-card>
          <n-statistic :label="t('dns.cache.entries')" :value="stats.entries" />
        </n-card>
      </n-gi>
      <n-gi span="4 m:2 l:1">
        <n-card>
          <n-statistic :label="t('dns.cache.maxEntries')" :value="stats.max_entries" />
        </n-card>
      </n-gi>
      <n-gi span="4 m:2 l:1">
        <n-card>
          <n-statistic :label="t('dns.cache.hitRate')">
            <template #default>{{ (stats.hit_rate ?? 0).toFixed(1) }}%</template>
          </n-statistic>
        </n-card>
      </n-gi>
      <n-gi span="4 m:2 l:1">
        <n-card>
          <n-statistic :label="t('dns.cache.missRate')">
            <template #default>{{ (stats.miss_rate ?? 0).toFixed(1) }}%</template>
          </n-statistic>
        </n-card>
      </n-gi>
    </n-grid>

    <n-card :title="t('dns.cache.entryList')" style="margin-top: 16px;">
      <template #header-extra>
        <n-space>
          <n-input v-model:value="filterQname" :placeholder="t('dns.cache.filterQname')" clearable size="small" style="width: 200px;" @keyup.enter="loadEntries" />
          <n-button size="small" @click="loadEntries">{{ t('common.search') }}</n-button>
        </n-space>
      </template>
      <n-data-table
        :columns="entryColumns"
        :data="entries"
        :loading="loadingEntries"
        :bordered="false"
        size="small"
        :pagination="entryPagination"
        remote
      />
    </n-card>

    <confirm-dialog
      :show="showFlushConfirm"
      :message="t('dns.cache.flushConfirm')"
      @confirm="confirmFlush"
      @cancel="showFlushConfirm = false"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, h, ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, useMessage } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import { usePermission } from '@/composables/usePermission'
import { getDNSCacheStats, flushDNSCache, listCacheEntries, flushDNSCacheEntry, type CacheStats, type CacheEntry } from '@/service/api/goddi/dns'

const { t } = useI18n()
const message = useMessage()
const perm = usePermission()

const stats = ref<CacheStats>({ entries: 0, max_entries: 0, hits: 0, misses: 0, hit_rate: 0, miss_rate: 0, size_bytes: 0 })
const showFlushConfirm = ref(false)

const entries = ref<CacheEntry[]>([])
const loadingEntries = ref(false)
const filterQname = ref('')
const entryPage = ref(1)
const entryPageSize = ref(20)
const entryTotal = ref(0)

const entryPagination = computed(() => ({
  page: entryPage.value,
  pageSize: entryPageSize.value,
  itemCount: entryTotal.value,
  onChange: (p: number) => { entryPage.value = p; loadEntries() },
}))

const entryColumns = computed(() => [
  { title: t('dns.cache.colName'), key: 'qname', minWidth: 200, ellipsis: { tooltip: true } },
  { title: t('dns.cache.colType'), key: 'qtype', width: 90 },
  { title: t('dns.cache.colTTL'), key: 'ttl_left', width: 100, render: (row: CacheEntry) => formatTTL(row.ttl_left) },
  { title: t('dns.cache.colHits'), key: 'hit_count', width: 90 },
  { title: t('dns.cache.colExpires'), key: 'expires_at', width: 180, render: (row: CacheEntry) => formatTime(row.expires_at) },
  {
    title: t('common.actions'),
    key: 'actions',
    width: 110,
    render: (row: CacheEntry) =>
      h(
        NButton,
        {
          size: 'tiny',
          type: 'warning',
          disabled: !perm.canDelete('dns'),
          onClick: () => handleFlushEntry(row),
        },
        { default: () => t('dns.cache.flushEntry') },
      ),
  },
])

function formatTTL(seconds: number): string {
  if (seconds <= 0) return '-'
  if (seconds < 60) return `${seconds}s`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`
  return `${Math.floor(seconds / 3600)}h`
}

function formatTime(value: string): string {
  const d = new Date(value)
  return Number.isNaN(d.getTime()) ? value : d.toLocaleString()
}

async function loadStats() {
  try {
    stats.value = await getDNSCacheStats()
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  }
}

async function loadEntries() {
  loadingEntries.value = true
  try {
    const res = await listCacheEntries({
      qname: filterQname.value || undefined,
      page: entryPage.value,
      page_size: entryPageSize.value,
    })
    entries.value = res.data ?? []
    entryTotal.value = res.meta?.total ?? entries.value.length
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  } finally {
    loadingEntries.value = false
  }
}

function handleFlushCache() {
  showFlushConfirm.value = true
}

async function confirmFlush() {
  try {
    await flushDNSCache()
    message.success(t('common.success'))
    loadStats()
    loadEntries()
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  }
  showFlushConfirm.value = false
}

async function handleFlushEntry(row: CacheEntry) {
  try {
    await flushDNSCacheEntry(row.qname, row.qtype)
    message.success(t('common.success'))
    loadStats()
    loadEntries()
  } catch (err: unknown) {
    message.error(err instanceof Error ? err.message : t('common.failed'))
  }
}

onMounted(() => {
  loadStats()
  loadEntries()
})
</script>
