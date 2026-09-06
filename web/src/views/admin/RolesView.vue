<template>
  <div>
    <page-header :title="t('admin.roles.title')">
      <n-button v-if="perm.canWrite('role')" type="primary" @click="openCreate">{{ t('admin.roles.createRole') }}</n-button>
    </page-header>

    <n-data-table
      :columns="columns"
      :data="roles"
      :loading="loading"
      remote :pagination="pagination"
      :row-key="(row: Role) => row.id"
      @update:page="handlePageChange"
      @update:page-size="handlePageSizeChange"
    />

    <!-- Create/Edit Role Modal -->
    <n-modal v-if="showModal" v-model:show="showModal" :title="editing ? t('admin.roles.editRole') : t('admin.roles.createRole')" preset="card" style="width: 450px;">
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

    <!-- Assign Permissions Modal -->
    <n-modal v-if="showPermModal" v-model:show="showPermModal" :title="t('admin.roles.assignPermissions')" preset="card" style="width: 550px;">
      <n-checkbox-group v-model:value="selectedPerms">
        <n-space vertical>
          <div v-for="group in permissionGroups" :key="group.resource">
            <n-text strong>{{ t(`perm.resource.${group.resource}`) }}</n-text>
            <n-space style="margin-top: 4px; margin-left: 12px;">
              <n-checkbox v-for="p in group.permissions" :key="p.id" :value="p.id" :label="t(`perm.action.${p.action}`)" />
            </n-space>
          </div>
        </n-space>
      </n-checkbox-group>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showPermModal = false">{{ t('common.cancel') }}</n-button>
          <n-button type="primary" @click="handleAssignPermissions">{{ t('common.save') }}</n-button>
        </n-space>
      </template>
    </n-modal>

    <confirm-dialog :show="showDeleteConfirm" :message="t('common.deleteConfirm')" @confirm="handleDelete" @cancel="showDeleteConfirm = false" />
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, h, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NTag, NSpace, useMessage } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import { usePermission } from '@/composables/usePermission'
import { listRoles, createRole, updateRole, deleteRole, assignRolePermissions, listPermissions, type Role, type CreateRoleRequest, type Permission } from '@/api/user'

const { t } = useI18n()
const message = useMessage()
const perm = usePermission()

const loading = ref(false)
const submitting = ref(false)
const roles = ref<Role[]>([])
const showModal = ref(false)
const showDeleteConfirm = ref(false)
const showPermModal = ref(false)
const deletingId = ref('')
const editing = ref<Role | null>(null)
const allPermissions = ref<Permission[]>([])
const selectedPerms = ref<string[]>([])
const assigningRoleId = ref('')

const pagination = reactive({ page: 1, pageSize: 20, itemCount: 0, showSizePicker: true, pageSizes: [10, 20, 50] })
const formData = reactive<CreateRoleRequest>({ name: '', description: '' })

const permissionGroups = computed(() => {
  const groups: Record<string, Permission[]> = {}
  for (const p of allPermissions.value) {
    if (!groups[p.resource]) groups[p.resource] = []
    groups[p.resource].push(p)
  }
  return Object.entries(groups).map(([resource, permissions]) => ({ resource, permissions }))
})

// Localized helpers -------------------------------------------------------

function roleLabel(name: string): string {
  const key = `admin.roles.builtin.${name}`
  const localized = t(key)
  return localized !== key ? localized : name
}

function roleDesc(name: string, fallback: string): string {
  const key = `admin.roles.builtinDesc.${name}`
  const localized = t(key)
  return localized !== key ? localized : fallback
}

function permTag(p: { resource: string; action: string }): string {
  return `${t(`perm.resource.${p.resource}`)} · ${t(`perm.action.${p.action}`)}`
}

const columns = [
  { title: () => t('common.name'), key: 'name', render: (row: Role) => roleLabel(row.name) },
  { title: () => t('common.description'), key: 'description', ellipsis: { tooltip: true }, render: (row: Role) => roleDesc(row.name, row.description) },
  { title: () => t('admin.roles.isSystem'), key: 'is_system', width: 90, render: (row: Role) => h(NTag, { size: 'small', type: row.is_system ? 'info' : 'default' }, { default: () => row.is_system ? t('common.yes') : t('common.no') }) },
  { title: () => t('admin.roles.permissions'), key: 'permissions', render: (row: Role) => h(NSpace, { size: 'small' }, { default: () => (row.permissions || []).map(p => h(NTag, { size: 'small', bordered: false, type: 'info' }, { default: () => permTag(p) })) }) },
  { title: () => t('common.actions'), key: 'actions', width: 220, render: (row: Role) => h(NSpace, null, {
    default: () => [
      h(NButton, { size: 'small', disabled: row.is_system || !perm.canWrite('role'), onClick: () => { editing.value = row; Object.assign(formData, { name: row.name, description: row.description }); showModal.value = true } }, { default: () => t('common.edit') }),
      h(NButton, { size: 'small', disabled: !perm.canWrite('role'), onClick: () => { assigningRoleId.value = row.id; selectedPerms.value = (row.permissions || []).map(p => p.id); showPermModal.value = true } }, { default: () => t('admin.roles.assignPermissions') }),
      h(NButton, { size: 'small', type: 'error', disabled: row.is_system || !perm.canDelete('role'), onClick: () => { deletingId.value = row.id; showDeleteConfirm.value = true } }, { default: () => t('common.delete') }),
    ],
  }) },
]

async function loadData() {
  loading.value = true
  try {
    const result = await listRoles({ page: pagination.page, page_size: pagination.pageSize })
    roles.value = result.data
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
    if (editing.value) { await updateRole(editing.value.id, formData); message.success(t('common.updateSuccess')) }
    else { await createRole(formData); message.success(t('common.createSuccess')) }
    showModal.value = false; editing.value = null; loadData()
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) } finally { submitting.value = false }
}

async function handleDelete() {
  try { await deleteRole(deletingId.value); message.success(t('common.deleteSuccess')); loadData() } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
  showDeleteConfirm.value = false
}

async function loadPermissions() {
  try { allPermissions.value = (await listPermissions({ page: 1, page_size: 100 })).data } catch { /* ignore */ }
}

async function handleAssignPermissions() {
  try {
    await assignRolePermissions(assigningRoleId.value, selectedPerms.value)
    message.success(t('common.updateSuccess'))
    showPermModal.value = false
    loadData()
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
}

onMounted(() => { loadData(); loadPermissions() })
</script>
