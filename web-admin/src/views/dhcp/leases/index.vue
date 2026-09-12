<template>
  <div>
    <page-header :title="t('dhcp.leases.title')">
      <n-button @click="loadData">{{ t('common.refresh') }}</n-button>
    </page-header>

    <div class="filter-bar">
      <n-space>
        <n-input v-model:value="searchQuery" :placeholder="t('common.search') + ' IP/MAC'" clearable style="width: 240px;" @keyup.enter="loadData" />
        <n-select v-model:value="statusFilter" :options="statusOptions" clearable :placeholder="t('common.status')" style="width: 140px;" @update:value="loadData" />
      </n-space>
    </div>

    <n-data-table
      :columns="columns"
      :data="leases"
      :loading="loading"
      remote :pagination="pagination"
      :row-key="(row: DHCPLease) => row.id"
      @update:page="handlePageChange"
      @update:page-size="handlePageSizeChange"
    />

    <confirm-dialog :show="showReleaseConfirm" :message="t('common.deleteConfirm')" @confirm="handleRelease" @cancel="showReleaseConfirm = false" />
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, h, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NTag, NSpace, useMessage } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import { usePermission } from '@/composables/usePermission'
import { listDHCPLeases, deleteDHCPLease, type DHCPLease } from '@/service/api/goddi/dhcp'

const { t } = useI18n()
const message = useMessage()
const perm = usePermission()

const loading = ref(false)
const leases = ref<DHCPLease[]>([])
const searchQuery = ref('')
const statusFilter = ref<string | null>(null)
const showReleaseConfirm = ref(false)
const releasingId = ref('')

const statusOptions = [
  { label: 'Active', value: 'active' },
  { label: 'Offered', value: 'offered' },
  { label: 'Expired', value: 'expired' },
  { label: 'Released', value: 'released' },
  { label: 'Conflict', value: 'conflict' },
]

// Statuses that are not a confirmed binding must not look like one: an offer is
// pending, and a conflict is an address held out of the pool after a DECLINE.
const statusTagType: Record<string, 'success' | 'info' | 'warning' | 'error' | 'default'> = {
  active: 'success',
  offered: 'info',
  conflict: 'warning',
  expired: 'default',
  released: 'default',
}

const pagination = reactive({ page: 1, pageSize: 20, itemCount: 0, showSizePicker: true, pageSizes: [10, 20, 50] })

const columns = [
  { title: () => t('dhcp.leases.ip'), key: 'ip' },
  { title: () => t('dhcp.leases.mac'), key: 'mac' },
  { title: () => t('dhcp.leases.hostname'), key: 'hostname', ellipsis: { tooltip: true } },
  { title: () => t('dhcp.leases.scope'), key: 'scope_name' },
  { title: () => t('common.status'), key: 'status', render: (row: DHCPLease) => h(NTag, { size: 'small', type: statusTagType[row.status] ?? 'default' }, { default: () => row.status }) },
  { title: () => t('dhcp.leases.startTime'), key: 'start_time', width: 160 },
  { title: () => t('dhcp.leases.endTime'), key: 'end_time', width: 160 },
  { title: () => t('common.actions'), key: 'actions', width: 100, render: (row: DHCPLease) => h(NButton, { size: 'small', text: true, type: 'error', disabled: !perm.canDelete('dhcp'), onClick: () => { releasingId.value = row.id; showReleaseConfirm.value = true } }, { default: () => t('dhcp.leases.release') }) },
]

async function loadData() {
  loading.value = true
  try {
    const params: Record<string, unknown> = { page: pagination.page, page_size: pagination.pageSize }
    if (searchQuery.value) params.search = searchQuery.value
    if (statusFilter.value) params.status = statusFilter.value
    const result = await listDHCPLeases(params)
    leases.value = result.data
    pagination.itemCount = result.meta.total
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) } finally { loading.value = false }
}

function handlePageChange(page: number) { pagination.page = page; loadData() }
function handlePageSizeChange(pageSize: number) { pagination.pageSize = pageSize; pagination.page = 1; loadData() }

async function handleRelease() {
  try { await deleteDHCPLease(releasingId.value); message.success(t('common.success')); loadData() } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
  showReleaseConfirm.value = false
}

onMounted(loadData)
</script>
