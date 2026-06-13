<template>
  <div>
    <page-header :title="t('admin.groups.title')">
      <n-button v-if="perm.canWrite('role')" type="primary" @click="openCreate">{{ t('admin.groups.createGroup') }}</n-button>
    </page-header>

    <n-data-table
      :columns="columns"
      :data="groups"
      :loading="loading"
      :pagination="pagination"
      :row-key="(row: Group) => row.id"
      @update:page="handlePageChange"
      @update:page-size="handlePageSizeChange"
    />

    <n-modal v-if="showModal" v-model:show="showModal" :title="editing ? t('admin.groups.editGroup') : t('admin.groups.createGroup')" preset="card" style="width: 450px;">
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

    <!-- Assign Roles Modal -->
    <n-modal v-if="showRolesModal" v-model:show="showRolesModal" :title="t('admin.groups.assignRoles')" preset="card" style="width: 450px;">
      <n-checkbox-group v-model:value="selectedRoles">
        <n-space item-style="display: flex;">
          <n-checkbox v-for="role in allRoles" :key="role.id" :value="role.id" :label="role.name" />
        </n-space>
      </n-checkbox-group>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showRolesModal = false">{{ t('common.cancel') }}</n-button>
          <n-button type="primary" @click="handleAssignRoles">{{ t('common.save') }}</n-button>
        </n-space>
      </template>
    </n-modal>

    <confirm-dialog :show="showDeleteConfirm" :message="t('common.deleteConfirm')" @confirm="handleDelete" @cancel="showDeleteConfirm = false" />
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, h, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NTag, NSpace, useMessage } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import { usePermission } from '@/composables/usePermission'
import { listGroups, createGroup, updateGroup, deleteGroup, assignGroupRoles, type Group, type CreateGroupRequest } from '@/api/user'
import { listRoles, type Role } from '@/api/user'

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
  { title: t('common.name'), key: 'name' },
  { title: t('common.description'), key: 'description', ellipsis: { tooltip: true } },
  { title: t('admin.users.roles'), key: 'roles', render: (row: Group) => h(NSpace, { size: 'small' }, { default: () => (row.roles || []).map(r => h(NTag, { size: 'small', type: 'info' }, { default: () => r.name })) }) },
  { title: t('common.actions'), key: 'actions', width: 220, render: (row: Group) => h(NSpace, null, {
    default: () => [
      h(NButton, { size: 'small', onClick: () => { editing.value = row; Object.assign(formData, { name: row.name, description: row.description }); showModal.value = true } }, { default: () => t('common.edit') }),
      h(NButton, { size: 'small', disabled: !perm.canWrite('role'), onClick: () => { assigningGroupId.value = row.id; selectedRoles.value = (row.roles || []).map(r => r.id); showRolesModal.value = true } }, { default: () => t('admin.groups.assignRoles') }),
      h(NButton, { size: 'small', type: 'error', disabled: !perm.canDelete('role'), onClick: () => { deletingId.value = row.id; showDeleteConfirm.value = true } }, { default: () => t('common.delete') }),
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
  editing.value = null
  Object.assign(formData, { name: '', description: '' })
  showModal.value = true
}

async function handleSubmit() {
  submitting.value = true
  try {
    if (editing.value) { await updateGroup(editing.value.id, formData); message.success(t('common.updateSuccess')) }
    else { await createGroup(formData); message.success(t('common.createSuccess')) }
    showModal.value = false; editing.value = null; loadData()
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) } finally { submitting.value = false }
}

async function handleDelete() {
  try { await deleteGroup(deletingId.value); message.success(t('common.deleteSuccess')); loadData() } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
  showDeleteConfirm.value = false
}

async function loadRoles() {
  try { allRoles.value = (await listRoles({ page: 1, page_size: 100 })).data } catch { /* ignore */ }
}

async function handleAssignRoles() {
  try {
    await assignGroupRoles(assigningGroupId.value, selectedRoles.value)
    message.success(t('common.updateSuccess'))
    showRolesModal.value = false
    loadData()
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
}

onMounted(() => { loadData(); loadRoles() })
</script>
