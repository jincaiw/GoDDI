<script setup lang="ts">
import { ref, reactive, h, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NTag, NSpace, useMessage } from 'naive-ui'
import PageHeader from '@/components/PageHeader.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import { usePermission } from '@/composables/usePermission'
import { listRoles, createRole, updateRole, deleteRole, setRolePermissions, listPermissions, type Role, type CreateRoleRequest, type Permission } from '@/service/api/goddi/user'

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
      h(NButton, { size: 'small', text: true, disabled: row.is_system || !perm.canWrite('role'), onClick: () => { if (row.is_system || !perm.canWrite('role')) return; editing.value = row; Object.assign(formData, { name: row.name, description: row.description }); showModal.value = true } }, { default: () => t('common.edit') }),
      h(NButton, { size: 'small', text: true, disabled: !perm.canWrite('role'), onClick: () => { if (!perm.canWrite('role')) return; assigningRoleId.value = row.id; selectedPerms.value = (row.permissions || []).map(p => p.id); showPermModal.value = true } }, { default: () => t('admin.roles.assignPermissions') }),
      h(NButton, { size: 'small', text: true, type: 'error', disabled: row.is_system || !perm.canDelete('role'), onClick: () => { if (row.is_system || !perm.canDelete('role')) return; deletingId.value = row.id; showDeleteConfirm.value = true } }, { default: () => t('common.delete') }),
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
  if (!perm.canWrite('role')) return
  editing.value = null
  Object.assign(formData, { name: '', description: '' })
  showModal.value = true
}

async function handleSubmit() {
  if (!perm.canWrite('role')) return
  submitting.value = true
  try {
    if (editing.value) { await updateRole(editing.value.id, formData); message.success(t('common.updateSuccess')) }
    else { await createRole(formData); message.success(t('common.createSuccess')) }
    showModal.value = false; editing.value = null; loadData()
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) } finally { submitting.value = false }
}

async function handleDelete() {
  if (!perm.canDelete('role')) return
  try { await deleteRole(deletingId.value); message.success(t('common.deleteSuccess')); loadData() } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
  showDeleteConfirm.value = false
}

async function loadPermissions() {
  try { allPermissions.value = (await listPermissions({ page: 1, page_size: 100 })).data } catch { /* ignore */ }
}

async function handleAssignPermissions() {
  if (!perm.canWrite('role')) return
  try {
    await setRolePermissions(assigningRoleId.value, selectedPerms.value)
    message.success(t('common.updateSuccess'))
    showPermModal.value = false
    loadData()
  } catch (err: unknown) { message.error(err instanceof Error ? err.message : t('common.failed')) }
}

onMounted(() => { loadData(); loadPermissions() })
</script>

<template>
  <div>
    <PageHeader :title="t('admin.roles.title')">
      <NButton v-if="perm.canWrite('role')" type="primary" @click="openCreate">{{ t('admin.roles.createRole') }}</NButton>
    </PageHeader>

    <NDataTable
      :columns="columns"
      :data="roles"
      :loading="loading"
      remote :pagination="pagination"
      :row-key="(row: Role) => row.id"
      @update:page="handlePageChange"
      @update:page-size="handlePageSizeChange"
    />

    <!-- Create/Edit Role Modal -->
    <NModal v-if="showModal" v-model:show="showModal" :title="editing ? t('admin.roles.editRole') : t('admin.roles.createRole')" preset="card" style="width: 450px;">
      <NForm :model="formData" label-placement="left" label-width="80px">
        <NFormItem :label="t('common.name')"><NInput v-model:value="formData.name" :disabled="!perm.canWrite('role')" /></NFormItem>
        <NFormItem :label="t('common.descriptions')"><NInput v-model:value="formData.description" type="textarea" :disabled="!perm.canWrite('role')" /></NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showModal = false">{{ t('common.cancel') }}</NButton>
          <NButton type="primary" :loading="submitting" :disabled="!perm.canWrite('role')" @click="handleSubmit">{{ t('common.save') }}</NButton>
        </NSpace>
      </template>
    </NModal>

    <!-- Assign Permissions Modal -->
    <NModal v-if="showPermModal" v-model:show="showPermModal" :title="t('admin.roles.assignPermissions')" preset="card" style="width: 550px;">
      <NCheckboxGroup v-model:value="selectedPerms">
        <NSpace vertical>
          <div v-for="group in permissionGroups" :key="group.resource">
            <NText strong>{{ t(`perm.resource.${group.resource}`) }}</NText>
            <NSpace style="margin-top: 4px; margin-left: 12px;">
              <NCheckbox v-for="p in group.permissions" :key="p.id" :value="p.id" :label="t(`perm.action.${p.action}`)" :disabled="!perm.canWrite('role')" />
            </NSpace>
          </div>
        </NSpace>
      </NCheckboxGroup>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showPermModal = false">{{ t('common.cancel') }}</NButton>
          <NButton type="primary" :disabled="!perm.canWrite('role')" @click="handleAssignPermissions">{{ t('common.save') }}</NButton>
        </NSpace>
      </template>
    </NModal>

    <ConfirmDialog :show="showDeleteConfirm" :message="t('common.deleteConfirm')" @confirm="handleDelete" @cancel="showDeleteConfirm = false" />
  </div>
</template>
