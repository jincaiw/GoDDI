<template>
  <div>
    <page-header :title="t('dhcp.reservations.title')">
      <n-button v-if="perm.canWrite('dhcp')" type="primary" @click="openCreate">{{ t('dhcp.reservations.createReservation') }}</n-button>
    </page-header>

    <n-data-table
      :columns="columns"
      :data="reservations"
      :loading="loading"
      :pagination="pagination"
      :row-key="(row: DHCPReservation) => row.id"
      @update:page="handlePageChange"
      @update:page-size="handlePageSizeChange"
    />

    <n-modal v-if="showModal" v-model:show="showModal" :title="editing ? t('dhcp.reservations.editReservation') : t('dhcp.reservations.createReservation')" preset="card" style="width: 500px;">
      <n-form :model="formData" label-placement="left" label-width="80px">
        <n-form-item :label="t('dhcp.leases.ip')"><n-input v-model:value="formData.ip_address" /></n-form-item>
        <n-form-item :label="t('dhcp.leases.mac')"><n-input v-model:value="formData.mac_address" /></n-form-item>
        <n-form-item :label="t('dhcp.leases.hostname')"><n-input v-model:value="formData.hostname" /></n-form-item>
        <n-form-item :label="t('dhcp.leases.scope')"><n-input v-model:value="formData.scope_id" /></n-form-item>
        <n-form-item :label="t('common.descriptions')"><n-input v-model:value="formData.description" type="textarea" /></n-form-item>
        <n-form-item :label="t('common.enabled')"><n-switch v-model:value="formData.enabled" /></n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showModal = false">{{ t('common.cancel') }}</n-button>
          <n-button type="primary" :loading="submitting" @click="handleSubmit">{{ t('common.save') }}</n-button>
        </n-space>
      </template>
    </n-modal>

    <confirm-dialog :show="showDeleteConfirm" :message="t('common.deleteConfirm')" @confirm="handleDelete" @cancel="showDeleteConfirm = false" />
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, h, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NSwitch, NSpace, useMessage } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import { usePermission } from '@/composables/usePermission'
import { listDHCPReservations, createDHCPReservation, updateDHCPReservation, deleteDHCPReservation, type DHCPReservation, type CreateDHCPReservationRequest } from '@/api/dhcp'

const { t } = useI18n()
const message = useMessage()
const perm = usePermission()

const loading = ref(false)
const submitting = ref(false)
const reservations = ref<DHCPReservation[]>([])
const showModal = ref(false)
const showDeleteConfirm = ref(false)
const deletingId = ref('')
const editing = ref<DHCPReservation | null>(null)

const pagination = reactive({ page: 1, pageSize: 20, itemCount: 0, showSizePicker: true, pageSizes: [10, 20, 50] })
const formData = reactive<CreateDHCPReservationRequest & { description: string }>({ ip_address: '', mac_address: '', hostname: '', scope_id: '', enabled: true, description: '' })

const columns = [
  { title: t('dhcp.leases.ip'), key: 'ip_address' },
  { title: t('dhcp.leases.mac'), key: 'mac_address' },
  { title: t('dhcp.leases.hostname'), key: 'hostname' },
  { title: t('dhcp.leases.scope'), key: 'scope_name' },
  { title: t('common.enabled'), key: 'enabled', width: 80, render: (row: DHCPReservation) => h(NSwitch, { value: row.enabled, disabled: true }) },
  { title: t('common.actions'), key: 'actions', width: 160, render: (row: DHCPReservation) => h(NSpace, null, {
    default: () => [
      h(NButton, { size: 'small', onClick: () => { editing.value = row; Object.assign(formData, row); showModal.value = true } }, { default: () => t('common.edit') }),
      h(NButton, { size: 'small', type: 'error', disabled: !perm.canDelete('dhcp'), onClick: () => { deletingId.value = row.id; showDeleteConfirm.value = true } }, { default: () => t('common.delete') }),
    ],
  }) },
]

async function loadData() {
  loading.value = true
  try {
    const result = await listDHCPReservations({ page: pagination.page, page_size: pagination.pageSize })
    reservations.value = result.data
    pagination.itemCount = result.meta.total
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) } finally { loading.value = false }
}

function handlePageChange(page: number) { pagination.page = page; loadData() }
function handlePageSizeChange(pageSize: number) { pagination.pageSize = pageSize; pagination.page = 1; loadData() }

function openCreate() {
  editing.value = null
  Object.assign(formData, { ip_address: '', mac_address: '', hostname: '', scope_id: '', enabled: true, description: '' })
  showModal.value = true
}

async function handleSubmit() {
  submitting.value = true
  try {
    if (editing.value) { await updateDHCPReservation(editing.value.id, formData); message.success(t('common.updateSuccess')) }
    else { await createDHCPReservation(formData); message.success(t('common.createSuccess')) }
    showModal.value = false; editing.value = null; loadData()
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) } finally { submitting.value = false }
}

async function handleDelete() {
  try { await deleteDHCPReservation(deletingId.value); message.success(t('common.deleteSuccess')); loadData() } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
  showDeleteConfirm.value = false
}

onMounted(loadData)
</script>
