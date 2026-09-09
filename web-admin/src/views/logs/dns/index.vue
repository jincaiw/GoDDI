<template>
  <div>
    <page-header :title="t('logs.dns.title')" />

    <n-alert v-if="error" type="error" closable style="margin-bottom: 16px;" @close="error = ''">
      {{ error }}
    </n-alert>

    <n-card>
      <n-data-table
        :columns="columns"
        :data="logs"
        :loading="loading"
        remote
        :pagination="pagination"
        :row-key="(row: DNSQueryLog) => row.id"
        @update:page="handlePageChange"
        @update:page-size="handlePageSizeChange"
      />
    </n-card>
  </div>
</template>

<script setup lang="ts">
import { h, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NTag } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import { listDNSQueryLogs, type DNSQueryLog } from '@/service/api/goddi/dns'

const { t } = useI18n()
const loading = ref(false)
const error = ref('')
const logs = ref<DNSQueryLog[]>([])
const pagination = reactive({ page: 1, pageSize: 20, itemCount: 0, showSizePicker: true, pageSizes: [10, 20, 50] })

const columns = [
  { title: () => t('logs.dns.queryName'), key: 'query_name', minWidth: 220, ellipsis: { tooltip: true } },
  { title: () => t('logs.dns.queryType'), key: 'query_type', width: 100 },
  { title: () => t('logs.dns.clientIp'), key: 'client_ip', width: 140 },
  { title: () => t('logs.dns.responseCode'), key: 'response_code', width: 130 },
  { title: () => t('logs.dns.responseTime'), key: 'response_time', width: 110, render: (row: DNSQueryLog) => `${row.response_time} ms` },
  { title: () => t('logs.dns.cached'), key: 'cached', width: 100, render: (row: DNSQueryLog) => h(NTag, { size: 'small', type: row.cached ? 'success' : 'default' }, { default: () => row.cached ? t('common.yes') : t('common.no') }) },
  { title: () => t('logs.dns.blocked'), key: 'blocked', width: 100, render: (row: DNSQueryLog) => h(NTag, { size: 'small', type: row.blocked ? 'error' : 'default' }, { default: () => row.blocked ? t('common.yes') : t('common.no') }) },
  { title: () => t('logs.dns.upstream'), key: 'upstream', width: 160, ellipsis: { tooltip: true } },
  { title: () => t('common.createdAt'), key: 'created_at', width: 180 },
]

async function loadData() {
  loading.value = true
  error.value = ''
  try {
    const result = await listDNSQueryLogs({ page: pagination.page, page_size: pagination.pageSize })
    logs.value = result.data
    pagination.itemCount = result.meta.total
  } catch (err: unknown) {
    error.value = err instanceof Error ? err.message : t('common.failed')
  } finally {
    loading.value = false
  }
}

function handlePageChange(page: number) {
  pagination.page = page
  loadData()
}

function handlePageSizeChange(pageSize: number) {
  pagination.pageSize = pageSize
  pagination.page = 1
  loadData()
}

onMounted(loadData)
</script>
