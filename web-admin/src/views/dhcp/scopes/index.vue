<script setup lang="ts">
import { ref, reactive, h, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NSwitch, NSpace, NProgress, useMessage } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import { usePermission } from '@/composables/usePermission'
import { listDHCPScopes, createDHCPScope, updateDHCPScope, deleteDHCPScope, type DHCPScope, type CreateDHCPScopeRequest } from '@/service/api/goddi/dhcp'

const { t } = useI18n()
const message = useMessage()
const perm = usePermission()

const loading = ref(false)
const submitting = ref(false)
const scopes = ref<DHCPScope[]>([])
const showModal = ref(false)
const showDeleteConfirm = ref(false)
const deletingId = ref('')
const editingScope = ref<DHCPScope | null>(null)

const pagination = reactive({ page: 1, pageSize: 20, itemCount: 0, showSizePicker: true, pageSizes: [10, 20, 50] })

// `comment` and not `description`: this is the field the scope manager stores,
// and the extra key the form used to carry was dropped on the way in -- the UI
// said the description had been saved and the column stayed empty.
const formData = reactive<CreateDHCPScopeRequest>({
  name: '', subnet: '', start_ip: '', end_ip: '', lease_time: 86400, enabled: true, comment: '',
})

const columns = [
  { title: () => t('common.name'), key: 'name' },
  { title: () => t('dhcp.scopes.subnet'), key: 'subnet' },
  { title: () => t('dhcp.scopes.startIp'), key: 'start_ip', width: 130 },
  { title: () => t('dhcp.scopes.endIp'), key: 'end_ip', width: 130 },
  { title: () => t('dhcp.scopes.leaseTime'), key: 'lease_time', width: 100 },
  { title: () => t('dhcp.scopes.activeLeases'), key: 'active_leases', width: 100 },
  {
    title: () => t('dhcp.scopes.usage'), key: 'usage', width: 120,
    render: (row: DHCPScope) => {
      const pct = row.total_addresses > 0 ? Math.round((row.active_leases / row.total_addresses) * 100) : 0
      return h(NProgress, { type: 'line', percentage: pct, indicatorPlacement: 'inside', status: pct > 90 ? 'error' : pct > 70 ? 'warning' : 'success' })
    },
  },
  { title: () => t('common.enabled'), key: 'enabled', width: 80, render: (row: DHCPScope) => h(NSwitch, { value: row.enabled, disabled: !perm.canWrite('dhcp'), onUpdateValue: () => toggleEnabled(row) }) },
  { title: () => t('common.actions'), key: 'actions', width: 160, render: (row: DHCPScope) => h(NSpace, null, {
    default: () => [
      h(NButton, { size: 'small', text: true, disabled: !perm.canWrite('dhcp'), onClick: () => { if (!perm.canWrite('dhcp')) return; editingScope.value = row; Object.assign(formData, row); showModal.value = true } }, { default: () => t('common.edit') }),
      h(NButton, { size: 'small', text: true, type: 'error', disabled: !perm.canDelete('dhcp'), onClick: () => { if (!perm.canDelete('dhcp')) return; deletingId.value = row.id; showDeleteConfirm.value = true } }, { default: () => t('common.delete') }),
    ],
  }) },
]

async function loadData() {
  loading.value = true
  try {
    const result = await listDHCPScopes({ page: pagination.page, page_size: pagination.pageSize })
    scopes.value = result.data
    pagination.itemCount = result.meta.total
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) } finally { loading.value = false }
}

function handlePageChange(page: number) { pagination.page = page; loadData() }
function handlePageSizeChange(pageSize: number) { pagination.pageSize = pageSize; pagination.page = 1; loadData() }

function openCreateScope() {
  if (!perm.canWrite('dhcp')) return
  editingScope.value = null
  Object.assign(formData, { name: '', subnet: '', start_ip: '', end_ip: '', lease_time: 86400, enabled: true, comment: '' })
  showModal.value = true
}

async function toggleEnabled(scope: DHCPScope) {
  if (!perm.canWrite('dhcp')) return
  try { await updateDHCPScope(scope.id, { enabled: !scope.enabled }); message.success(t('common.updateSuccess')); loadData() } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
}

async function handleSubmit() {
  if (!perm.canWrite('dhcp')) return
  submitting.value = true
  try {
    if (editingScope.value) { await updateDHCPScope(editingScope.value.id, formData); message.success(t('common.updateSuccess')) }
    else { await createDHCPScope(formData); message.success(t('common.createSuccess')) }
    showModal.value = false; editingScope.value = null; loadData()
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) } finally { submitting.value = false }
}

async function handleDelete() {
  if (!perm.canDelete('dhcp')) return
  try { await deleteDHCPScope(deletingId.value); message.success(t('common.deleteSuccess')); loadData() } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
  showDeleteConfirm.value = false
}

onMounted(loadData)
</script>

<template>
  <div>
    <PageHeader :title="t('dhcp.scopes.title')">
      <NButton v-if="perm.canWrite('dhcp')" type="primary" @click="openCreateScope">{{ t('dhcp.scopes.createScope') }}</NButton>
    </PageHeader>

    <NDataTable
      :columns="columns"
      :data="scopes"
      :loading="loading"
      remote :pagination="pagination"
      :row-key="(row: DHCPScope) => row.id"
      @update:page="handlePageChange"
      @update:page-size="handlePageSizeChange"
    />

    <!-- Create/Edit Scope Modal -->
    <NModal v-if="showModal" v-model:show="showModal" preset="card" :title="editingScope ? t('dhcp.scopes.editScope') : t('dhcp.scopes.createScope')" style="width: 550px;">
      <NForm :model="formData" label-placement="left" label-width="100px">
        <NFormItem :label="t('common.name')"><NInput v-model:value="formData.name" /></NFormItem>
        <NFormItem :label="t('dhcp.scopes.subnet')"><NInput v-model:value="formData.subnet" placeholder="192.168.1.0/24" /></NFormItem>
        <NFormItem :label="t('dhcp.scopes.startIp')"><NInput v-model:value="formData.start_ip" /></NFormItem>
        <NFormItem :label="t('dhcp.scopes.endIp')"><NInput v-model:value="formData.end_ip" /></NFormItem>
        <NFormItem :label="t('dhcp.scopes.leaseTime')"><NInputNumber v-model:value="formData.lease_time" :min="60" style="width: 100%;" /></NFormItem>
        <NFormItem :label="t('common.description')"><NInput v-model:value="formData.comment" type="textarea" /></NFormItem>
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
