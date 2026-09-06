<template>
  <div>
    <page-header :title="t('logs.audit.title')">
      <n-button @click="loadData">{{ t('common.refresh') }}</n-button>
    </page-header>

    <div class="filter-bar">
      <n-space>
        <n-input v-model:value="filters.username" :placeholder="t('logs.audit.user')" clearable style="width: 140px;" @keyup.enter="loadData" />
        <n-input v-model:value="filters.action" :placeholder="t('logs.audit.action')" clearable style="width: 140px;" @keyup.enter="loadData" />
        <n-input v-model:value="filters.resource" :placeholder="t('logs.audit.resource')" clearable style="width: 140px;" @keyup.enter="loadData" />
        <n-button type="primary" @click="loadData">{{ t('common.search') }}</n-button>
      </n-space>
    </div>

    <n-data-table
      :columns="columns"
      :data="logs"
      :loading="loading"
      remote :pagination="pagination"
      :row-key="(row: AuditLog) => row.id"
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
import { listAuditLogs, type AuditLog } from '@/api/logs'

const { t } = useI18n()
const message = useMessage()

const loading = ref(false)
const logs = ref<AuditLog[]>([])
const filters = reactive({ username: '', action: '', resource: '' })

const pagination = reactive({ page: 1, pageSize: 20, itemCount: 0, showSizePicker: true, pageSizes: [10, 20, 50] })

const columns = [
  { title: () => t('logs.audit.user'), key: 'username', width: 120 },
  { title: () => t('logs.audit.action'), key: 'action', width: 120 },
  { title: () => t('logs.audit.resource'), key: 'resource', width: 120 },
  { title: () => t('logs.audit.resourceId'), key: 'resource_id', width: 120 },
  { title: () => t('logs.audit.details'), key: 'details', ellipsis: { tooltip: true } },
  { title: () => t('logs.audit.ip'), key: 'ip', width: 130 },
  { title: () => t('common.createdAt'), key: 'created_at', width: 160 },
]

async function loadData() {
  loading.value = true
  try {
    const params: Record<string, unknown> = { page: pagination.page, page_size: pagination.pageSize }
    if (filters.username) params.username = filters.username
    if (filters.action) params.action = filters.action
    if (filters.resource) params.resource = filters.resource
    const result = await listAuditLogs(params)
    logs.value = result.data
    pagination.itemCount = result.meta.total
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) } finally { loading.value = false }
}

function handlePageChange(page: number) { pagination.page = page; loadData() }
function handlePageSizeChange(pageSize: number) { pagination.pageSize = pageSize; pagination.page = 1; loadData() }

onMounted(loadData)
</script>
