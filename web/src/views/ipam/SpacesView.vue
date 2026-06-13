<template>
  <div>
    <page-header :title="t('ipam.spaces.title')">
      <n-button v-if="perm.canWrite('ipam')" type="primary" @click="openCreate">{{ t('ipam.spaces.createSpace') }}</n-button>
    </page-header>

    <n-data-table
      :columns="columns"
      :data="spaces"
      :loading="loading"
      :pagination="pagination"
      :row-key="(row: IPAMSpace) => row.id"
      @update:page="handlePageChange"
      @update:page-size="handlePageSizeChange"
    />

    <n-modal v-if="showModal" v-model:show="showModal" :title="editing ? t('ipam.spaces.editSpace') : t('ipam.spaces.createSpace')" preset="card" style="width: 450px;">
      <n-form :model="formData" label-placement="left" label-width="80px">
        <n-form-item :label="t('common.name')"><n-input v-model:value="formData.name" /></n-form-item>
        <n-form-item :label="t('common.descriptions')"><n-input v-model:value="formData.description" type="textarea" /></n-form-item>
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
import { listIPAMSpaces, createIPAMSpace, updateIPAMSpace, deleteIPAMSpace, type IPAMSpace, type CreateIPAMSpaceRequest } from '@/api/ipam'

const { t } = useI18n()
const message = useMessage()
const perm = usePermission()

const loading = ref(false)
const submitting = ref(false)
const spaces = ref<IPAMSpace[]>([])
const showModal = ref(false)
const showDeleteConfirm = ref(false)
const deletingId = ref('')
const editing = ref<IPAMSpace | null>(null)

const pagination = reactive({ page: 1, pageSize: 20, itemCount: 0, showSizePicker: true, pageSizes: [10, 20, 50] })
const formData = reactive<CreateIPAMSpaceRequest>({ name: '', description: '' })

const columns = [
  { title: t('common.name'), key: 'name' },
  { title: t('common.description'), key: 'description', ellipsis: { tooltip: true } },
  { title: t('common.createdAt'), key: 'created_at', width: 160 },
  { title: t('common.actions'), key: 'actions', width: 160, render: (row: IPAMSpace) => h(NSpace, null, {
    default: () => [
      h(NButton, { size: 'small', onClick: () => { editing.value = row; Object.assign(formData, { name: row.name, description: row.description }); showModal.value = true } }, { default: () => t('common.edit') }),
      h(NButton, { size: 'small', type: 'error', disabled: !perm.canDelete('ipam'), onClick: () => { deletingId.value = row.id; showDeleteConfirm.value = true } }, { default: () => t('common.delete') }),
    ],
  }) },
]

async function loadData() {
  loading.value = true
  try {
    const result = await listIPAMSpaces({ page: pagination.page, page_size: pagination.pageSize })
    spaces.value = result.data
    pagination.itemCount = result.meta.total
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) } finally { loading.value = false }
}

function handlePageChange(page: number) { pagination.page = page; loadData() }
function handlePageSizeChange(pageSize: number) { pagination.pageSize = pageSize; pagination.page = 1; loadData() }

function openCreate() {
  editing.value = null
  Object.assign(formData, { name: '', description: '' })
  showModal.value = true
}

async function handleSubmit() {
  submitting.value = true
  try {
    if (editing.value) { await updateIPAMSpace(editing.value.id, formData); message.success(t('common.updateSuccess')) }
    else { await createIPAMSpace(formData); message.success(t('common.createSuccess')) }
    showModal.value = false; editing.value = null; loadData()
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) } finally { submitting.value = false }
}

async function handleDelete() {
  try { await deleteIPAMSpace(deletingId.value); message.success(t('common.deleteSuccess')); loadData() } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
  showDeleteConfirm.value = false
}

onMounted(loadData)
</script>
