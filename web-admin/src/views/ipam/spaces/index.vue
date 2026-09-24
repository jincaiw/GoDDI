<script setup lang="ts">
import { ref, reactive, h, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NSpace, useMessage } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import { usePermission } from '@/composables/usePermission'
import { listIPAMSpaces, createIPAMSpace, updateIPAMSpace, deleteIPAMSpace, type IPAMSpace, type CreateIPAMSpaceRequest } from '@/service/api/goddi/ipam'

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
  { title: () => t('common.name'), key: 'name' },
  { title: () => t('common.description'), key: 'description', ellipsis: { tooltip: true } },
  { title: () => t('common.createdAt'), key: 'created_at', width: 160 },
  { title: () => t('common.actions'), key: 'actions', width: 160, render: (row: IPAMSpace) => h(NSpace, null, {
    default: () => [
      h(NButton, { size: 'small', text: true, disabled: !perm.canWrite('ipam'), onClick: () => { if (!perm.canWrite('ipam')) return; editing.value = row; Object.assign(formData, { name: row.name, description: row.description }); showModal.value = true } }, { default: () => t('common.edit') }),
      h(NButton, { size: 'small', text: true, type: 'error', disabled: !perm.canDelete('ipam'), onClick: () => { if (!perm.canDelete('ipam')) return; deletingId.value = row.id; showDeleteConfirm.value = true } }, { default: () => t('common.delete') }),
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
  if (!perm.canWrite('ipam')) return
  editing.value = null
  Object.assign(formData, { name: '', description: '' })
  showModal.value = true
}

async function handleSubmit() {
  if (!perm.canWrite('ipam')) return
  submitting.value = true
  try {
    if (editing.value) { await updateIPAMSpace(editing.value.id, formData); message.success(t('common.updateSuccess')) }
    else { await createIPAMSpace(formData); message.success(t('common.createSuccess')) }
    showModal.value = false; editing.value = null; loadData()
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) } finally { submitting.value = false }
}

async function handleDelete() {
  if (!perm.canDelete('ipam')) return
  try { await deleteIPAMSpace(deletingId.value); message.success(t('common.deleteSuccess')); loadData() } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
  showDeleteConfirm.value = false
}

onMounted(loadData)
</script>

<template>
  <div>
    <PageHeader :title="t('ipam.spaces.title')">
      <NButton v-if="perm.canWrite('ipam')" type="primary" @click="openCreate">{{ t('ipam.spaces.createSpace') }}</NButton>
    </PageHeader>

    <NDataTable
      :columns="columns"
      :data="spaces"
      :loading="loading"
      remote :pagination="pagination"
      :row-key="(row: IPAMSpace) => row.id"
      @update:page="handlePageChange"
      @update:page-size="handlePageSizeChange"
    />

    <NModal v-if="showModal" v-model:show="showModal" :title="editing ? t('ipam.spaces.editSpace') : t('ipam.spaces.createSpace')" preset="card" style="width: 450px;">
      <NForm :model="formData" label-placement="left" label-width="80px">
        <NFormItem :label="t('common.name')"><NInput v-model:value="formData.name" /></NFormItem>
        <NFormItem :label="t('common.descriptions')"><NInput v-model:value="formData.description" type="textarea" /></NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showModal = false">{{ t('common.cancel') }}</NButton>
          <NButton type="primary" :loading="submitting" :disabled="!perm.canWrite('ipam')" @click="handleSubmit">{{ t('common.save') }}</NButton>
        </NSpace>
      </template>
    </NModal>

    <ConfirmDialog :show="showDeleteConfirm" :message="t('common.deleteConfirm')" @confirm="handleDelete" @cancel="showDeleteConfirm = false" />
  </div>
</template>
