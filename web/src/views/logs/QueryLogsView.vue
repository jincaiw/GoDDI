<template>
  <div>
    <page-header :title="t('logs.dns.title')">
      <n-button @click="loadData">{{ t('common.refresh') }}</n-button>
    </page-header>

    <n-card style="margin-bottom: 16px;">
      <n-space>
        <n-input v-model:value="filters.query_name" :placeholder="t('logs.dns.queryName')" clearable style="width: 180px;" @keyup.enter="loadData" />
        <n-input v-model:value="filters.client_ip" :placeholder="t('logs.dns.clientIp')" clearable style="width: 140px;" @keyup.enter="loadData" />
        <n-button type="primary" @click="loadData">{{ t('common.search') }}</n-button>
      </n-space>
    </n-card>

    <n-data-table
      :columns="columns"
      :data="logs"
      :loading="loading"
      :pagination="pagination"
      :row-key="(row: DNSQueryLog) => row.id"
      @update:page="handlePageChange"
      @update:page-size="handlePageSizeChange"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, h, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { NTag, useMessage } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import { listDNSQueryLogs, type DNSQueryLog } from '@/api/dns'

const { t } = useI18n()
const message = useMessage()

const loading = ref(false)
const logs = ref<DNSQueryLog[]>([])
const filters = reactive({ query_name: '', client_ip: '' })

const pagination = reactive({ page: 1, pageSize: 20, itemCount: 0, showSizePicker: true, pageSizes: [10, 20, 50] })

const columns = [
  { title: t('logs.dns.queryName'), key: 'query_name', ellipsis: { tooltip: true } },
  { title: t('logs.dns.queryType'), key: 'query_type', width: 80 },
  { title: t('logs.dns.clientIp'), key: 'client_ip', width: 130 },
  { title: t('logs.dns.responseCode'), key: 'response_code', width: 100 },
  { title: t('logs.dns.responseTime'), key: 'response_time', width: 100, render: (row: DNSQueryLog) => `${row.response_time}ms` },
  { title: t('logs.dns.cached'), key: 'cached', width: 80, render: (row: DNSQueryLog) => h(NTag, { size: 'small', type: row.cached ? 'success' : 'default' }, { default: () => row.cached ? 'Yes' : 'No' }) },
  { title: t('logs.dns.blocked'), key: 'blocked', width: 80, render: (row: DNSQueryLog) => h(NTag, { size: 'small', type: row.blocked ? 'error' : 'default' }, { default: () => row.blocked ? 'Yes' : 'No' }) },
  { title: t('logs.dns.upstream'), key: 'upstream', width: 140 },
  { title: t('common.createdAt'), key: 'created_at', width: 160 },
]

async function loadData() {
  loading.value = true
  try {
    const params: Record<string, unknown> = { page: pagination.page, page_size: pagination.pageSize }
    if (filters.query_name) params.query_name = filters.query_name
    if (filters.client_ip) params.client_ip = filters.client_ip
    const result = await listDNSQueryLogs(params)
    logs.value = result.data
    pagination.itemCount = result.meta.total
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) } finally { loading.value = false }
}

function handlePageChange(page: number) { pagination.page = page; loadData() }
function handlePageSizeChange(pageSize: number) { pagination.pageSize = pageSize; pagination.page = 1; loadData() }

onMounted(loadData)
</script>
