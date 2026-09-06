<template>
  <div>
    <page-header :title="t('admin.users.title')">
      <n-button v-if="perm.canWrite('user')" type="primary" @click="openCreate">{{ t('admin.users.createUser') }}</n-button>
    </page-header>

    <n-data-table
      :columns="columns"
      :data="users"
      :loading="loading"
      remote :pagination="pagination"
      :row-key="(row: User) => row.id"
      @update:page="handlePageChange"
      @update:page-size="handlePageSizeChange"
    />

    <!-- Create/Edit User Modal -->
    <n-modal v-if="showModal" v-model:show="showModal" :title="editing ? t('admin.users.editUser') : t('admin.users.createUser')" preset="card" style="width: 500px;">
      <n-form :model="formData" label-placement="left" label-width="100px">
        <n-form-item :label="t('admin.users.username')"><n-input v-model:value="formData.username" :disabled="!!editing" /></n-form-item>
        <n-form-item v-if="!editing" :label="t('auth.password')"><n-input v-model:value="formData.password" type="password" show-password-on="click" :minlength="8" /></n-form-item>
        <n-form-item :label="t('admin.users.email')"><n-input v-model:value="formData.email" /></n-form-item>
        <n-form-item :label="t('admin.users.displayName')"><n-input v-model:value="formData.display_name" /></n-form-item>
        <n-form-item :label="t('common.enabled')"><n-switch v-model:value="formData.enabled" /></n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showModal = false">{{ t('common.cancel') }}</n-button>
          <n-button type="primary" :loading="submitting" @click="handleSubmit">{{ t('common.save') }}</n-button>
        </n-space>
      </template>
    </n-modal>

    <!-- Assign Roles Modal -->
    <n-modal v-if="showRolesModal" v-model:show="showRolesModal" :title="t('admin.users.assignRoles')" preset="card" style="width: 450px;">
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
import { NButton, NSwitch, NSpace, NTag, useMessage } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import { usePermission } from '@/composables/usePermission'
import { listUsers, createUser, updateUser, deleteUser, assignUserRoles, type User, type CreateUserRequest } from '@/api/user'
import { listRoles, type Role } from '@/api/user'

const { t } = useI18n()
const message = useMessage()
const perm = usePermission()

const loading = ref(false)
const submitting = ref(false)
const users = ref<User[]>([])
const showModal = ref(false)
const showDeleteConfirm = ref(false)
const showRolesModal = ref(false)
const deletingId = ref('')
const editing = ref<User | null>(null)
const allRoles = ref<Role[]>([])
const selectedRoles = ref<string[]>([])
const assigningUserId = ref('')

const pagination = reactive({ page: 1, pageSize: 20, itemCount: 0, showSizePicker: true, pageSizes: [10, 20, 50] })
const formData = reactive<CreateUserRequest & { enabled: boolean }>({ username: '', email: '', password: '', display_name: '', enabled: true })

const columns = [
  { title: () => t('admin.users.username'), key: 'username' },
  { title: () => t('admin.users.displayName'), key: 'display_name' },
  { title: () => t('admin.users.email'), key: 'email' },
  { title: () => t('admin.users.roles'), key: 'roles', render: (row: User) => h(NSpace, { size: 'small' }, { default: () => (row.roles || []).map(r => h(NTag, { size: 'small', type: 'info' }, { default: () => r.name })) }) },
  { title: () => t('common.enabled'), key: 'enabled', width: 80, render: (row: User) => h(NSwitch, {
    value: row.enabled,
    disabled: !perm.canWrite('user'),
    onUpdateValue: async (enabled: boolean) => {
      try { await updateUser(row.id, { enabled }); await loadData() }
      catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
    },
  }) },
  { title: () => t('admin.users.totpEnabled'), key: 'totp_enabled', width: 80, render: (row: User) => h(NTag, { size: 'small', type: row.totp_enabled ? 'success' : 'default' }, { default: () => row.totp_enabled ? 'ON' : 'OFF' }) },
  { title: () => t('common.actions'), key: 'actions', width: 220, render: (row: User) => h(NSpace, null, {
    default: () => [
      h(NButton, { size: 'small', text: true, onClick: () => { editing.value = row; Object.assign(formData, { username: row.username, email: row.email, display_name: row.display_name, enabled: row.enabled }); showModal.value = true } }, { default: () => t('common.edit') }),
      h(NButton, { size: 'small', text: true, disabled: !perm.canWrite('user'), onClick: () => { assigningUserId.value = row.id; selectedRoles.value = (row.roles || []).map(r => r.id); showRolesModal.value = true } }, { default: () => t('admin.users.assignRoles') }),
      h(NButton, { size: 'small', text: true, type: 'error', disabled: !perm.canDelete('user'), onClick: () => { deletingId.value = row.id; showDeleteConfirm.value = true } }, { default: () => t('common.delete') }),
    ],
  }) },
]

async function loadData() {
  loading.value = true
  try {
    const result = await listUsers({ page: pagination.page, page_size: pagination.pageSize })
    users.value = result.data
    pagination.itemCount = result.meta.total
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) } finally { loading.value = false }
}

function handlePageChange(page: number) { pagination.page = page; loadData() }
function handlePageSizeChange(pageSize: number) { pagination.pageSize = pageSize; pagination.page = 1; loadData() }

function openCreate() {
  editing.value = null
  Object.assign(formData, { username: '', email: '', password: '', display_name: '', enabled: true })
  showModal.value = true
}

async function handleSubmit() {
  submitting.value = true
  try {
    if (!editing.value && formData.password.length < 8) {
      message.error(t('auth.passwordMinLength'))
      submitting.value = false
      return
    }
    if (editing.value) { await updateUser(editing.value.id, formData); message.success(t('common.updateSuccess')) }
    else { await createUser(formData); message.success(t('common.createSuccess')) }
    showModal.value = false; editing.value = null; loadData()
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) } finally { submitting.value = false }
}

async function handleDelete() {
  try { await deleteUser(deletingId.value); message.success(t('common.deleteSuccess')); loadData() } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
  showDeleteConfirm.value = false
}

async function loadRoles() {
  try {
    const result = await listRoles({ page: 1, page_size: 100 })
    allRoles.value = result.data
  } catch { /* ignore */ }
}

async function handleAssignRoles() {
  try {
    await assignUserRoles(assigningUserId.value, selectedRoles.value)
    message.success(t('common.updateSuccess'))
    showRolesModal.value = false
    loadData()
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
}

onMounted(() => { loadData(); loadRoles() })
</script>
