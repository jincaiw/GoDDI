<template>
  <div>
    <page-header :title="t('logs.dhcp.title')">
      <n-button @click="loadData">{{ t('common.refresh') }}</n-button>
    </page-header>

    <n-card style="margin-bottom: 16px;">
      <n-space>
        <n-input v-model:value="filters.client_mac" :placeholder="t('logs.dhcp.clientMac')" clearable style="width: 180px;" @keyup.enter="loadData" />
        <n-input v-model:value="filters.event_type" :placeholder="t('logs.dhcp.eventType')" clearable style="width: 140px;" @keyup.enter="loadData" />
        <n-button type="primary" @click="loadData">{{ t('common.search') }}</n-button>
      </n-space>
    </n-card>

    <n-data-table
      :columns="columns"
      :data="logs"
      :loading="loading"
      :pagination="pagination"
      :row-key="(row: DHCPLog) => row.id"
      @update:page="handlePageChange"
      @update:page-size="handlePageSizeChange"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useMessage } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import { listDHCPLogs, type DHCPLog } from '@/api/dhcp'

const { t } = useI18n()
const message = useMessage()

const loading = ref(false)
const logs = ref<DHCPLog[]>([])
const filters = reactive({ client_mac: '', event_type: '' })

const pagination = reactive({ page: 1, pageSize: 20, itemCount: 0, showSizePicker: true, pageSizes: [10, 20, 50] })

const columns = [
  { title: t('logs.dhcp.eventType'), key: 'event_type', width: 120 },
  { title: t('logs.dhcp.clientMac'), key: 'client_mac', width: 150 },
  { title: t('logs.dhcp.clientIp'), key: 'client_ip', width: 130 },
  { title: t('logs.dhcp.hostname'), key: 'hostname', width: 130 },
  { title: t('logs.dhcp.scope'), key: 'scope_name', width: 130 },
  { title: t('logs.dhcp.message'), key: 'message', ellipsis: { tooltip: true } },
  { title: t('common.createdAt'), key: 'created_at', width: 160 },
]

async function loadData() {
  loading.value = true
  try {
    const params: Record<string, unknown> = { page: pagination.page, page_size: pagination.pageSize }
    if (filters.client_mac) params.client_mac = filters.client_mac
    if (filters.event_type) params.event_type = filters.event_type
    const result = await listDHCPLogs(params)
    logs.value = result.data
    pagination.itemCount = result.meta.total
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) } finally { loading.value = false }
}

function handlePageChange(page: number) { pagination.page = page; loadData() }
function handlePageSizeChange(pageSize: number) { pagination.pageSize = pageSize; pagination.page = 1; loadData() }

onMounted(loadData)
</script>
