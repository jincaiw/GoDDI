<template>
  <div>
    <page-header :title="t('dhcp.options.title')">
      <n-button v-if="perm.canWrite('dhcp')" type="primary" @click="openCreate">{{ t('dhcp.options.createOption') }}</n-button>
    </page-header>

    <n-data-table
      :columns="columns"
      :data="options"
      :loading="loading"
      remote :pagination="pagination"
      :row-key="(row: DHCPOption) => row.id"
      @update:page="handlePageChange"
      @update:page-size="handlePageSizeChange"
    />

    <n-modal v-if="showModal" v-model:show="showModal" :title="editing ? t('dhcp.options.editOption') : t('dhcp.options.createOption')" preset="card" style="width: 500px;">
      <n-form :model="formData" label-placement="left" label-width="100px">
        <n-form-item :label="t('dhcp.options.code')"><n-input-number v-model:value="formData.code" :min="1" :max="254" style="width: 100%;" /></n-form-item>
        <n-form-item :label="t('dhcp.options.optionValue')"><n-input v-model:value="formData.value" /></n-form-item>
        <n-form-item :label="t('common.priority')"><n-select v-model:value="formData.priority" :options="[{ label: 'Global', value: 'global' }, { label: 'Scope', value: 'scope' }, { label: 'Client Class', value: 'client_class' }, { label: 'Reservation', value: 'reservation' }]" clearable /></n-form-item>
        <n-form-item :label="t('dhcp.options.scope')" required><n-select v-model:value="formData.scope_id" :options="scopeOptions" filterable :disabled="!!editing" /></n-form-item>
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
import { NButton, NSpace, useMessage } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import { usePermission } from '@/composables/usePermission'
import { listDHCPScopes } from '@/api/dhcp'
import { listDHCPOptions, createDHCPOption, updateDHCPOption, deleteDHCPOption, type DHCPOption, type CreateDHCPOptionRequest } from '@/api/dhcp'

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
const options = ref<DHCPOption[]>([])
const showModal = ref(false)
const showDeleteConfirm = ref(false)
const deletingId = ref('')
const editing = ref<DHCPOption | null>(null)

const pagination = reactive({ page: 1, pageSize: 20, itemCount: 0, showSizePicker: true, pageSizes: [10, 20, 50] })
const formData = reactive<CreateDHCPOptionRequest>({ code: 1, value: '', priority: 'global', scope_id: '' })

const columns = [
  { title: () => t('dhcp.options.code'), key: 'code', width: 80 },
  { title: () => t('dhcp.options.optionValue'), key: 'value', ellipsis: { tooltip: true } },
  { title: () => t('dhcp.options.scope'), key: 'scope_id' },
  { title: () => t('common.priority'), key: 'priority', width: 100 },
  { title: () => t('common.actions'), key: 'actions', width: 160, render: (row: DHCPOption) => h(NSpace, null, {
    default: () => [
      h(NButton, { size: 'small', onClick: () => { editing.value = row; Object.assign(formData, { code: row.code, value: row.value, priority: row.priority, scope_id: row.scope_id }); showModal.value = true } }, { default: () => t('common.edit') }),
      h(NButton, { size: 'small', type: 'error', disabled: !perm.canDelete('dhcp'), onClick: () => { deletingId.value = row.id; showDeleteConfirm.value = true } }, { default: () => t('common.delete') }),
    ],
  }) },
]

async function loadData() {
  loading.value = true
  try {
    const result = await listDHCPOptions({ page: pagination.page, page_size: pagination.pageSize })
    options.value = result.data
    pagination.itemCount = result.meta.total
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) } finally { loading.value = false }
}

function handlePageChange(page: number) { pagination.page = page; loadData() }
function handlePageSizeChange(pageSize: number) { pagination.pageSize = pageSize; pagination.page = 1; loadData() }

function openCreate() {
  editing.value = null
  Object.assign(formData, { code: 1, value: '', priority: 'global', scope_id: '' })
  showModal.value = true
}

async function handleSubmit() {
  if (!formData.scope_id) { message.error(t('dhcp.options.scopeRequired')); return }
  submitting.value = true
  try {
    if (editing.value) { await updateDHCPOption(editing.value.id, formData); message.success(t('common.updateSuccess')) }
    else { await createDHCPOption(formData); message.success(t('common.createSuccess')) }
    showModal.value = false; editing.value = null; loadData()
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) } finally { submitting.value = false }
}

async function handleDelete() {
  try { await deleteDHCPOption(deletingId.value); message.success(t('common.deleteSuccess')); loadData() } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
  showDeleteConfirm.value = false
}

onMounted(() => { loadData(); loadScopes() })
</script>
