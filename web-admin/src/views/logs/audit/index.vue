<template>
  <div>
    <page-header :title="t('logs.audit.title')" />

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
        :row-key="(row: AuditLog) => row.id"
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
import { listAuditLogs, type AuditLog } from '@/service/api/goddi/logs'

const { t } = useI18n()
const loading = ref(false)
const error = ref('')
const logs = ref<AuditLog[]>([])
const pagination = reactive({ page: 1, pageSize: 20, itemCount: 0, showSizePicker: true, pageSizes: [10, 20, 50] })

const columns = [
  { title: () => t('logs.audit.user'), key: 'username', width: 140 },
  { title: () => t('logs.audit.action'), key: 'action', width: 140, render: (row: AuditLog) => h(NTag, { size: 'small' }, { default: () => row.action }) },
  { title: () => t('logs.audit.resource'), key: 'resource', width: 140 },
  { title: () => t('logs.audit.resourceId'), key: 'resource_id', width: 160, ellipsis: { tooltip: true } },
  { title: () => t('logs.audit.details'), key: 'details', ellipsis: { tooltip: true } },
  { title: () => t('logs.audit.ip'), key: 'ip', width: 140 },
  { title: () => t('common.createdAt'), key: 'created_at', width: 180 },
]

async function loadData() {
  loading.value = true
  error.value = ''
  try {
    const result = await listAuditLogs({ page: pagination.page, page_size: pagination.pageSize })
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
