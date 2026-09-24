<script setup lang="ts">
import { ref, reactive, h, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NTag, NSpace, useMessage } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import { usePermission } from '@/composables/usePermission'
import { listGroups, createGroup, updateGroup, deleteGroup, assignGroupRoles, type Group, type CreateGroupRequest } from '@/service/api/goddi/user'
import { listRoles, type Role } from '@/service/api/goddi/user'

const { t } = useI18n()
const message = useMessage()
const perm = usePermission()

const loading = ref(false)
const submitting = ref(false)
const groups = ref<Group[]>([])
const showModal = ref(false)
const showDeleteConfirm = ref(false)
const showRolesModal = ref(false)
const deletingId = ref('')
const editing = ref<Group | null>(null)
const allRoles = ref<Role[]>([])
const selectedRoles = ref<string[]>([])
const assigningGroupId = ref('')

const pagination = reactive({ page: 1, pageSize: 20, itemCount: 0, showSizePicker: true, pageSizes: [10, 20, 50] })
const formData = reactive<CreateGroupRequest>({ name: '', description: '' })

const columns = [
  { title: () => t('common.name'), key: 'name' },
  { title: () => t('common.description'), key: 'description', ellipsis: { tooltip: true } },
  { title: () => t('admin.users.roles'), key: 'roles', render: (row: Group) => h(NSpace, { size: 'small' }, { default: () => (row.roles || []).map(r => h(NTag, { size: 'small', type: 'info' }, { default: () => r.name })) }) },
  { title: () => t('common.actions'), key: 'actions', width: 220, render: (row: Group) => h(NSpace, null, {
    default: () => [
      h(NButton, { size: 'small', text: true, disabled: !perm.canWrite('group'), onClick: () => { if (!perm.canWrite('group')) return; editing.value = row; Object.assign(formData, { name: row.name, description: row.description }); showModal.value = true } }, { default: () => t('common.edit') }),
      h(NButton, { size: 'small', text: true, disabled: !perm.canWrite('group') || !perm.canRead('role'), onClick: () => { if (!perm.canWrite('group') || !perm.canRead('role')) return; assigningGroupId.value = row.id; selectedRoles.value = (row.roles || []).map(r => r.id); showRolesModal.value = true } }, { default: () => t('admin.groups.assignRoles') }),
      h(NButton, { size: 'small', text: true, type: 'error', disabled: !perm.canDelete('group'), onClick: () => { if (!perm.canDelete('group')) return; deletingId.value = row.id; showDeleteConfirm.value = true } }, { default: () => t('common.delete') }),
    ],
  }) },
]

async function loadData() {
  loading.value = true
  try {
    const result = await listGroups({ page: pagination.page, page_size: pagination.pageSize })
    groups.value = result.data
    pagination.itemCount = result.meta.total
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) } finally { loading.value = false }
}

function handlePageChange(page: number) { pagination.page = page; loadData() }
function handlePageSizeChange(pageSize: number) { pagination.pageSize = pageSize; pagination.page = 1; loadData() }

function openCreate() {
  if (!perm.canWrite('group')) return
  editing.value = null
  Object.assign(formData, { name: '', description: '' })
  showModal.value = true
}

async function handleSubmit() {
  if (!perm.canWrite('group')) return
  submitting.value = true
  try {
    if (editing.value) { await updateGroup(editing.value.id, formData); message.success(t('common.updateSuccess')) }
    else { await createGroup(formData); message.success(t('common.createSuccess')) }
    showModal.value = false; editing.value = null; loadData()
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) } finally { submitting.value = false }
}

async function handleDelete() {
  if (!perm.canDelete('group')) return
  try { await deleteGroup(deletingId.value); message.success(t('common.deleteSuccess')); loadData() } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
  showDeleteConfirm.value = false
}

async function loadRoles() {
  try { allRoles.value = (await listRoles({ page: 1, page_size: 100 })).data } catch { /* ignore */ }
}

async function handleAssignRoles() {
  if (!perm.canWrite('group') || !perm.canRead('role')) return
  try {
    await assignGroupRoles(assigningGroupId.value, selectedRoles.value)
    message.success(t('common.updateSuccess'))
    showRolesModal.value = false
    loadData()
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
}

onMounted(() => { loadData(); loadRoles() })
</script>

<template>
  <div>
    <PageHeader :title="t('admin.groups.title')">
      <NButton v-if="perm.canWrite('group')" type="primary" @click="openCreate">{{ t('admin.groups.createGroup') }}</NButton>
    </PageHeader>

    <NDataTable
      :columns="columns"
      :data="groups"
      :loading="loading"
      remote :pagination="pagination"
      :row-key="(row: Group) => row.id"
      @update:page="handlePageChange"
      @update:page-size="handlePageSizeChange"
    />

    <NModal v-if="showModal" v-model:show="showModal" :title="editing ? t('admin.groups.editGroup') : t('admin.groups.createGroup')" preset="card" style="width: 450px;">
      <NForm :model="formData" label-placement="left" label-width="80px">
        <NFormItem :label="t('common.name')"><NInput v-model:value="formData.name" :disabled="!perm.canWrite('group')" /></NFormItem>
        <NFormItem :label="t('common.descriptions')"><NInput v-model:value="formData.description" type="textarea" :disabled="!perm.canWrite('group')" /></NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showModal = false">{{ t('common.cancel') }}</NButton>
          <NButton type="primary" :loading="submitting" :disabled="!perm.canWrite('group')" @click="handleSubmit">{{ t('common.save') }}</NButton>
        </NSpace>
      </template>
    </NModal>

    <!-- Assign Roles Modal -->
    <NModal v-if="showRolesModal" v-model:show="showRolesModal" :title="t('admin.groups.assignRoles')" preset="card" style="width: 450px;">
      <NCheckboxGroup v-model:value="selectedRoles">
        <NSpace item-style="display: flex;">
          <NCheckbox v-for="role in allRoles" :key="role.id" :value="role.id" :label="role.name" :disabled="!perm.canWrite('group') || !perm.canRead('role')" />
        </NSpace>
      </NCheckboxGroup>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showRolesModal = false">{{ t('common.cancel') }}</NButton>
          <NButton type="primary" :disabled="!perm.canWrite('group') || !perm.canRead('role')" @click="handleAssignRoles">{{ t('common.save') }}</NButton>
        </NSpace>
      </template>
    </NModal>

    <ConfirmDialog :show="showDeleteConfirm" :message="t('common.deleteConfirm')" @confirm="handleDelete" @cancel="showDeleteConfirm = false" />
  </div>
</template>
