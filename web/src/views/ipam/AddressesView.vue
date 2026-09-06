<template>
  <div>
    <page-header :title="t('ipam.addresses.title')">
      <n-space>
        <n-button v-if="perm.canWrite('ipam')" type="primary" @click="showAllocateModal = true">{{ t('ipam.addresses.allocate') }}</n-button>
        <n-button @click="loadData">{{ t('common.refresh') }}</n-button>
      </n-space>
    </page-header>

    <div class="filter-bar">
      <n-space>
        <n-input v-model:value="searchQuery" :placeholder="t('common.search')" clearable style="width: 240px;" @keyup.enter="loadData" />
        <n-select v-model:value="statusFilter" :options="statusOptions" clearable :placeholder="t('common.status')" style="width: 140px;" @update:value="loadData" />
      </n-space>
    </div>

    <n-data-table
      :columns="columns"
      :data="addresses"
      :loading="loading"
      remote :pagination="pagination"
      :row-key="(row: IPAMAddress) => row.id"
      @update:page="handlePageChange"
      @update:page-size="handlePageSizeChange"
    />

    <!-- Allocate IP Modal -->
    <n-modal v-if="showAllocateModal" v-model:show="showAllocateModal" :title="t('ipam.addresses.allocate')" preset="card" style="width: 450px;">
      <n-form :model="allocateForm" label-placement="left" label-width="80px">
        <n-form-item :label="t('ipam.subnets.title')">
          <n-select v-model:value="allocateForm.subnet_id" :options="subnetOptions" />
        </n-form-item>
        <n-form-item :label="t('ipam.addresses.ip')"><n-input v-model:value="allocateForm.ip_address" placeholder="Auto-assign if empty" /></n-form-item>
        <n-form-item :label="t('ipam.addresses.hostname')"><n-input v-model:value="allocateForm.hostname" /></n-form-item>
        <n-form-item :label="t('ipam.addresses.mac')"><n-input v-model:value="allocateForm.mac_address" /></n-form-item>
        <n-form-item :label="t('ipam.addresses.owner')"><n-input v-model:value="allocateForm.owner" /></n-form-item>
        <n-form-item :label="t('ipam.addresses.device')"><n-input v-model:value="allocateForm.device" /></n-form-item>
        <n-form-item :label="t('ipam.addresses.location')"><n-input v-model:value="allocateForm.location" /></n-form-item>
        <n-form-item :label="t('common.descriptions')"><n-input v-model:value="allocateForm.description" /></n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showAllocateModal = false">{{ t('common.cancel') }}</n-button>
          <n-button type="primary" :loading="allocating" @click="handleAllocate">{{ t('common.save') }}</n-button>
        </n-space>
      </template>
    </n-modal>

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
import { listIPAMAddresses, allocateIP, releaseIP, listIPAMSubnets, type IPAMAddress, type AllocateIPRequest } from '@/api/ipam'

const { t } = useI18n()
const message = useMessage()
const perm = usePermission()

const loading = ref(false)
const allocating = ref(false)
const addresses = ref<IPAMAddress[]>([])
const searchQuery = ref('')
const statusFilter = ref<string | null>(null)
const showAllocateModal = ref(false)
const showReleaseConfirm = ref(false)
const releasingId = ref('')
const subnetOptions = ref<Array<{ label: string; value: string }>>([])

const pagination = reactive({ page: 1, pageSize: 20, itemCount: 0, showSizePicker: true, pageSizes: [10, 20, 50] })

const allocateForm = reactive<AllocateIPRequest>({ subnet_id: '', ip_address: '', hostname: '', mac_address: '', owner: '', device: '', location: '', description: '' })

const statusOptions = [
  { label: t('ipam.addresses.statusUsed'), value: 'used' },
  { label: t('ipam.addresses.statusFree'), value: 'available' },
  { label: t('ipam.addresses.statusReserved'), value: 'reserved' },
  { label: t('ipam.addresses.statusConflict'), value: 'conflict' },
]

function statusTag(status: string) {
  const map: Record<string, 'success' | 'default' | 'warning' | 'error'> = {
    used: 'success', available: 'default', reserved: 'warning', conflict: 'error',
  }
  return map[status] || 'default'
}

const columns = [
  { title: () => t('ipam.addresses.ip'), key: 'ip_address' },
  { title: () => t('common.status'), key: 'status', width: 100, render: (row: IPAMAddress) => h(NTag, { size: 'small', type: statusTag(row.status) }, { default: () => row.status }) },
  { title: () => t('ipam.addresses.hostname'), key: 'hostname' },
  { title: () => t('ipam.addresses.mac'), key: 'mac_address' },
  { title: () => t('common.description'), key: 'description', ellipsis: { tooltip: true } },
  { title: () => t('common.actions'), key: 'actions', width: 100, render: (row: IPAMAddress) => row.status === 'used' ? h(NButton, { size: 'small', text: true, type: 'warning', disabled: !perm.canWrite('ipam'), onClick: () => { releasingId.value = row.id; showReleaseConfirm.value = true } }, { default: () => t('ipam.addresses.release') }) : null },
]

async function loadData() {
  loading.value = true
  try {
    const params: Record<string, unknown> = { page: pagination.page, page_size: pagination.pageSize }
    if (searchQuery.value) params.search = searchQuery.value
    if (statusFilter.value) params.status = statusFilter.value
    const result = await listIPAMAddresses(params)
    addresses.value = result.data
    pagination.itemCount = result.meta.total
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) } finally { loading.value = false }
}

function handlePageChange(page: number) { pagination.page = page; loadData() }
function handlePageSizeChange(pageSize: number) { pagination.pageSize = pageSize; pagination.page = 1; loadData() }

async function loadSubnets() {
  try {
    const result = await listIPAMSubnets({ page: 1, page_size: 100 })
    subnetOptions.value = result.data.map(s => ({ label: `${s.name} (${s.cidr})`, value: s.id }))
  } catch { /* ignore */ }
}

async function handleAllocate() {
  allocating.value = true
  try {
    await allocateIP(allocateForm)
    message.success(t('common.createSuccess'))
    showAllocateModal.value = false
    loadData()
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) } finally { allocating.value = false }
}

async function handleRelease() {
  try { await releaseIP(releasingId.value); message.success(t('common.success')); loadData() } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
  showReleaseConfirm.value = false
}

onMounted(() => { loadData(); loadSubnets() })
</script>
