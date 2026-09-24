<script setup lang="ts">
import { ref, reactive, h, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NSwitch, NSpace, useMessage } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import { usePermission } from '@/composables/usePermission'
import { listDHCPScopes } from '@/service/api/goddi/dhcp'
import { listDHCPReservations, createDHCPReservation, updateDHCPReservation, deleteDHCPReservation, type DHCPReservation, type CreateDHCPReservationRequest } from '@/service/api/goddi/dhcp'

const { t } = useI18n()
const message = useMessage()
const perm = usePermission()
const scopeOptions = ref<Array<{ label: string; value: string }>>([])

async function loadScopes() {
  try { scopeOptions.value = (await listDHCPScopes({ page_size: 100 })).data.map(s => ({ label: `${s.name} (${s.subnet})`, value: s.id })) }
  catch (err) { message.error(err instanceof Error ? err.message : t('common.failed')) }
}

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
  { title: () => t('dhcp.leases.ip'), key: 'ip_address' },
  { title: () => t('dhcp.leases.mac'), key: 'mac_address' },
  { title: () => t('dhcp.leases.hostname'), key: 'hostname' },
  { title: () => t('dhcp.leases.scope'), key: 'scope_name' },
  { title: () => t('common.enabled'), key: 'enabled', width: 80, render: (row: DHCPReservation) => h(NSwitch, { value: row.enabled, disabled: true }) },
  { title: () => t('common.actions'), key: 'actions', width: 160, render: (row: DHCPReservation) => h(NSpace, null, {
    default: () => [
      h(NButton, { size: 'small', text: true, disabled: !perm.canWrite('dhcp'), onClick: () => { if (!perm.canWrite('dhcp')) return; editing.value = row; Object.assign(formData, row); showModal.value = true } }, { default: () => t('common.edit') }),
      h(NButton, { size: 'small', text: true, type: 'error', disabled: !perm.canDelete('dhcp'), onClick: () => { if (!perm.canDelete('dhcp')) return; deletingId.value = row.id; showDeleteConfirm.value = true } }, { default: () => t('common.delete') }),
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
  if (!perm.canWrite('dhcp')) return
  editing.value = null
  Object.assign(formData, { ip_address: '', mac_address: '', hostname: '', scope_id: '', enabled: true, description: '' })
  showModal.value = true
}

async function handleSubmit() {
  if (!perm.canWrite('dhcp')) return
  if (!formData.scope_id) { message.error(t('dhcp.options.scopeRequired')); return }
  submitting.value = true
  try {
    if (editing.value) { await updateDHCPReservation(editing.value.id, formData); message.success(t('common.updateSuccess')) }
    else { await createDHCPReservation(formData); message.success(t('common.createSuccess')) }
    showModal.value = false; editing.value = null; loadData()
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) } finally { submitting.value = false }
}

async function handleDelete() {
  if (!perm.canDelete('dhcp')) return
  try { await deleteDHCPReservation(deletingId.value); message.success(t('common.deleteSuccess')); loadData() } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
  showDeleteConfirm.value = false
}

onMounted(() => { loadData(); loadScopes() })
</script>

<template>
  <div>
    <PageHeader :title="t('dhcp.reservations.title')">
      <NButton v-if="perm.canWrite('dhcp')" type="primary" @click="openCreate">{{ t('dhcp.reservations.createReservation') }}</NButton>
    </PageHeader>

    <NDataTable
      :columns="columns"
      :data="reservations"
      :loading="loading"
      remote :pagination="pagination"
      :row-key="(row: DHCPReservation) => row.id"
      @update:page="handlePageChange"
      @update:page-size="handlePageSizeChange"
    />

    <NModal v-if="showModal" v-model:show="showModal" :title="editing ? t('dhcp.reservations.editReservation') : t('dhcp.reservations.createReservation')" preset="card" style="width: 500px;">
      <NForm :model="formData" label-placement="left" label-width="80px">
        <NFormItem :label="t('dhcp.leases.ip')"><NInput v-model:value="formData.ip_address" /></NFormItem>
        <NFormItem :label="t('dhcp.leases.mac')"><NInput v-model:value="formData.mac_address" /></NFormItem>
        <NFormItem :label="t('dhcp.leases.hostname')"><NInput v-model:value="formData.hostname" /></NFormItem>
        <NFormItem :label="t('dhcp.leases.scope')" required><NSelect v-model:value="formData.scope_id" :options="scopeOptions" filterable /></NFormItem>
        <NFormItem :label="t('common.descriptions')"><NInput v-model:value="formData.description" type="textarea" /></NFormItem>
        <NFormItem :label="t('common.enabled')"><NSwitch v-model:value="formData.enabled" /></NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showModal = false">{{ t('common.cancel') }}</NButton>
          <NButton type="primary" :loading="submitting" :disabled="!perm.canWrite('dhcp')" @click="handleSubmit">{{ t('common.save') }}</NButton>
        </NSpace>
      </template>
    </NModal>

    <ConfirmDialog :show="showDeleteConfirm" :message="t('common.deleteConfirm')" @confirm="handleDelete" @cancel="showDeleteConfirm = false" />
  </div>
</template>
