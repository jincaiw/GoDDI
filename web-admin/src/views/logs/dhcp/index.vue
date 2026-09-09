<template>
  <div>
    <page-header :title="t('logs.dhcp.title')" />

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
        :row-key="(row: DHCPLog) => row.id"
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
import { listDHCPLogs, type DHCPLog } from '@/service/api/goddi/dhcp'

const { t } = useI18n()
const loading = ref(false)
const error = ref('')
const logs = ref<DHCPLog[]>([])
const pagination = reactive({ page: 1, pageSize: 20, itemCount: 0, showSizePicker: true, pageSizes: [10, 20, 50] })

const columns = [
  { title: () => t('logs.dhcp.eventType'), key: 'event_type', width: 130, render: (row: DHCPLog) => h(NTag, { size: 'small' }, { default: () => row.event_type }) },
  { title: () => t('logs.dhcp.clientMac'), key: 'client_mac', width: 160 },
  { title: () => t('logs.dhcp.clientIp'), key: 'client_ip', width: 140 },
  { title: () => t('logs.dhcp.hostname'), key: 'hostname', width: 180, ellipsis: { tooltip: true } },
  { title: () => t('logs.dhcp.scope'), key: 'scope_name', width: 160, ellipsis: { tooltip: true } },
  { title: () => t('logs.dhcp.message'), key: 'message', minWidth: 240, ellipsis: { tooltip: true } },
  { title: () => t('common.createdAt'), key: 'created_at', width: 180 },
]

async function loadData() {
  loading.value = true
  error.value = ''
  try {
    const result = await listDHCPLogs({ page: pagination.page, page_size: pagination.pageSize })
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
