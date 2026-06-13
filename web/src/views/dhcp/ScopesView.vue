<template>
  <div>
    <page-header :title="t('dhcp.scopes.title')">
      <n-button v-if="perm.canWrite('dhcp')" type="primary" @click="openCreateScope">{{ t('dhcp.scopes.createScope') }}</n-button>
    </page-header>

    <n-data-table
      :columns="columns"
      :data="scopes"
      :loading="loading"
      :pagination="pagination"
      :row-key="(row: DHCPScope) => row.id"
      @update:page="handlePageChange"
      @update:page-size="handlePageSizeChange"
    />

    <!-- Create/Edit Scope Modal -->
    <n-modal v-if="showModal" v-model:show="showModal" preset="card" :title="editingScope ? t('dhcp.scopes.editScope') : t('dhcp.scopes.createScope')" style="width: 550px;">
      <n-form :model="formData" label-placement="left" label-width="100px">
        <n-form-item :label="t('common.name')"><n-input v-model:value="formData.name" /></n-form-item>
        <n-form-item :label="t('dhcp.scopes.subnet')"><n-input v-model:value="formData.subnet" placeholder="192.168.1.0/24" /></n-form-item>
        <n-form-item :label="t('dhcp.scopes.startIp')"><n-input v-model:value="formData.start_ip" /></n-form-item>
        <n-form-item :label="t('dhcp.scopes.endIp')"><n-input v-model:value="formData.end_ip" /></n-form-item>
        <n-form-item :label="t('dhcp.scopes.leaseTime')"><n-input-number v-model:value="formData.lease_time" :min="60" style="width: 100%;" /></n-form-item>
        <n-form-item :label="t('common.description')"><n-input v-model:value="formData.description" type="textarea" /></n-form-item>
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
import { NButton, NSwitch, NSpace, NProgress, useMessage } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import { usePermission } from '@/composables/usePermission'
import { listDHCPScopes, createDHCPScope, updateDHCPScope, deleteDHCPScope, type DHCPScope, type CreateDHCPScopeRequest } from '@/api/dhcp'

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

const formData = reactive<CreateDHCPScopeRequest & { enabled: boolean; description: string }>({
  name: '', subnet: '', start_ip: '', end_ip: '', lease_time: 86400, enabled: true, description: '',
})

const columns = [
  { title: t('common.name'), key: 'name' },
  { title: t('dhcp.scopes.subnet'), key: 'subnet' },
  { title: t('dhcp.scopes.startIp'), key: 'start_ip', width: 130 },
  { title: t('dhcp.scopes.endIp'), key: 'end_ip', width: 130 },
  { title: t('dhcp.scopes.leaseTime'), key: 'lease_time', width: 100 },
  { title: t('dhcp.scopes.activeLeases'), key: 'active_leases', width: 100 },
  {
    title: t('dhcp.scopes.usage'), key: 'usage', width: 120,
    render: (row: DHCPScope) => {
      const pct = row.total_addresses > 0 ? Math.round((row.active_leases / row.total_addresses) * 100) : 0
      return h(NProgress, { type: 'line', percentage: pct, indicatorPlacement: 'inside', status: pct > 90 ? 'error' : pct > 70 ? 'warning' : 'success' })
    },
  },
  { title: t('common.enabled'), key: 'enabled', width: 80, render: (row: DHCPScope) => h(NSwitch, { value: row.enabled, disabled: !perm.canWrite('dhcp'), onUpdateValue: () => toggleEnabled(row) }) },
  { title: t('common.actions'), key: 'actions', width: 160, render: (row: DHCPScope) => h(NSpace, null, {
    default: () => [
      h(NButton, { size: 'small', onClick: () => { editingScope.value = row; Object.assign(formData, row); showModal.value = true } }, { default: () => t('common.edit') }),
      h(NButton, { size: 'small', type: 'error', disabled: !perm.canDelete('dhcp'), onClick: () => { deletingId.value = row.id; showDeleteConfirm.value = true } }, { default: () => t('common.delete') }),
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
  editingScope.value = null
  Object.assign(formData, { name: '', subnet: '', start_ip: '', end_ip: '', lease_time: 86400, enabled: true, description: '' })
  showModal.value = true
}

async function toggleEnabled(scope: DHCPScope) {
  try { await updateDHCPScope(scope.id, { enabled: !scope.enabled }); message.success(t('common.updateSuccess')); loadData() } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
}

async function handleSubmit() {
  submitting.value = true
  try {
    if (editingScope.value) { await updateDHCPScope(editingScope.value.id, formData); message.success(t('common.updateSuccess')) }
    else { await createDHCPScope(formData); message.success(t('common.createSuccess')) }
    showModal.value = false; editingScope.value = null; loadData()
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) } finally { submitting.value = false }
}

async function handleDelete() {
  try { await deleteDHCPScope(deletingId.value); message.success(t('common.deleteSuccess')); loadData() } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
  showDeleteConfirm.value = false
}

onMounted(loadData)
</script>
