<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { DialogPlugin, Link as TLink, MessagePlugin, Space as TSpace, Tag as TTag } from 'tdesign-vue-next';
import type { FormInstanceFunctions, PrimaryTableCol } from 'tdesign-vue-next';
import RemoteTable from '@/components/RemoteTable.vue';
import { usePermission } from '@/composables/usePermission';
import {
  createRole,
  deleteRole,
  listPermissions,
  listRoles,
  setRolePermissions,
  updateRole,
  type CreateRoleRequest,
  type Permission,
  type Role
} from '@/service/api/goddi/user';

defineOptions({ name: 'AdminRolesPage' });

const { t } = useI18n();
const perm = usePermission();

const tableRef = ref<{ reload: () => Promise<void> } | null>(null);
const formRef = ref<FormInstanceFunctions | null>(null);
const showDialog = ref(false);
const showPerm = ref(false);
const submitting = ref(false);
const editing = ref<Role | null>(null);
const allPermissions = ref<Permission[]>([]);
const selectedPerms = ref<string[]>([]);
const assigningRoleId = ref('');

const formData = reactive<CreateRoleRequest>({ name: '', description: '' });

const formRules = {
  name: [{ required: true, message: t('common.name') + ' required', type: 'error' }]
};

const permissionGroups = computed(() => {
  const groups: Record<string, Permission[]> = {};
  for (const p of allPermissions.value) {
    if (!groups[p.resource]) groups[p.resource] = [];
    groups[p.resource].push(p);
  }
  return Object.entries(groups).map(([resource, permissions]) => ({ resource, permissions }));
});

function roleLabel(name: string): string {
  const key = `admin.roles.builtin.${name}`;
  const localized = t(key);
  return localized !== key ? localized : name;
}

function roleDesc(name: string, fallback: string): string {
  const key = `admin.roles.builtinDesc.${name}`;
  const localized = t(key);
  return localized !== key ? localized : fallback;
}

function permTag(p: { resource: string; action: string }): string {
  return `${t(`perm.resource.${p.resource}`)} · ${t(`perm.action.${p.action}`)}`;
}

const columns: PrimaryTableCol[] = [
  { colKey: 'name', title: t('common.name'), cell: (_h, { row }) => roleLabel((row as Role).name) },
  {
    colKey: 'description',
    title: t('common.description'),
    ellipsis: true,
    cell: (_h, { row }) => {
      const r = row as Role;
      return roleDesc(r.name, r.description);
    }
  },
  {
    colKey: 'is_system',
    title: t('admin.roles.isSystem'),
    width: 100,
    cell: (h, { row }) =>
      h(TTag, { size: 'small', theme: (row as Role).is_system ? 'primary' : 'default', variant: 'light' }, () =>
        (row as Role).is_system ? t('common.yes') : t('common.no')
      )
  },
  {
    colKey: 'permissions',
    title: t('admin.roles.permissions'),
    cell: (h, { row }) =>
      h(
        TSpace,
        { size: 'small' },
        () =>
          ((row as Role).permissions || []).map(p =>
            h(TTag, { size: 'small', variant: 'light' }, () => permTag(p))
          )
      )
  },
  {
    colKey: 'op',
    title: t('common.actions'),
    width: 220,
    cell: (h, { row }) => {
      const r = row as Role;
      return h(TSpace, { size: 'small' }, () => [
        h(
          TLink,
          { theme: 'primary', disabled: r.is_system || !perm.canWrite('role'), onClick: () => openEdit(r) },
          () => t('common.edit')
        ),
        h(
          TLink,
          { theme: 'primary', disabled: !perm.canWrite('role'), onClick: () => openPerm(r) },
          () => t('admin.roles.assignPermissions')
        ),
        h(
          TLink,
          { theme: 'danger', disabled: r.is_system || !perm.canDelete('role'), onClick: () => confirmDelete(r.id) },
          () => t('common.delete')
        )
      ]);
    }
  }
];

function msgError(err: unknown) {
  void MessagePlugin.error(err instanceof Error ? err.message : t('common.failed'));
}

function openCreate() {
  if (!perm.canWrite('role')) return;
  editing.value = null;
  Object.assign(formData, { name: '', description: '' });
  showDialog.value = true;
}

function openEdit(row: Role) {
  if (row.is_system || !perm.canWrite('role')) return;
  editing.value = row;
  Object.assign(formData, { name: row.name, description: row.description });
  showDialog.value = true;
}

function openPerm(row: Role) {
  if (!perm.canWrite('role')) return;
  assigningRoleId.value = row.id;
  selectedPerms.value = (row.permissions || []).map(p => p.id);
  showPerm.value = true;
}

async function handleSubmit() {
  if (!perm.canWrite('role')) return;
  const valid = await formRef.value?.validate();
  if (valid !== true) return;

  submitting.value = true;
  try {
    if (editing.value) {
      await updateRole(editing.value.id, formData);
      void MessagePlugin.success(t('common.updateSuccess'));
    } else {
      await createRole(formData);
      void MessagePlugin.success(t('common.createSuccess'));
    }
    showDialog.value = false;
    editing.value = null;
    await tableRef.value?.reload();
  } catch (err) {
    msgError(err);
  } finally {
    submitting.value = false;
  }
}

function confirmDelete(id: string) {
  if (!perm.canDelete('role')) return;
  const dlg = DialogPlugin.confirm({
    header: t('common.delete'),
    body: t('common.deleteConfirm'),
    theme: 'danger',
    onConfirm: async () => {
      try {
        await deleteRole(id);
        void MessagePlugin.success(t('common.deleteSuccess'));
        await tableRef.value?.reload();
      } catch (err) {
        msgError(err);
      }
      dlg.hide();
    }
  });
}

async function handleAssignPermissions() {
  if (!perm.canWrite('role')) return;
  try {
    await setRolePermissions(assigningRoleId.value, selectedPerms.value);
    void MessagePlugin.success(t('common.updateSuccess'));
    showPerm.value = false;
    await tableRef.value?.reload();
  } catch (err) {
    msgError(err);
  }
}

async function loadPermissions() {
  try {
    allPermissions.value = (await listPermissions({ page: 1, page_size: 100 })).data;
  } catch {
    // Permissions are optional for the list; the assign dialog will be empty.
  }
}

onMounted(loadPermissions);
</script>

<template>
  <div class="td-page">
    <TCard :bordered="false" class="td-page-header">
      <h2>{{ t('admin.roles.title') }}</h2>
      <TButton v-if="perm.canWrite('role')" theme="primary" @click="openCreate">
        <template #icon><TIcon name="add" /></template>
        {{ t('admin.roles.createRole') }}
      </TButton>
    </TCard>

    <RemoteTable ref="tableRef" :columns="columns" :fetcher="listRoles" />

    <TDialog
      v-model:visible="showDialog"
      :header="editing ? t('admin.roles.editRole') : t('admin.roles.createRole')"
      :confirm-btn="{ loading: submitting, content: t('common.save') }"
      :cancel-btn="t('common.cancel')"
      width="480px"
      @confirm="handleSubmit"
    >
      <TForm ref="formRef" :data="formData" :rules="formRules" label-align="top">
        <TFormItem :label="t('common.name')" name="name">
          <TInput v-model="formData.name" />
        </TFormItem>
        <TFormItem :label="t('common.descriptions')" name="description">
          <TTextarea v-model="formData.description" :autosize="{ minRows: 2, maxRows: 4 }" />
        </TFormItem>
      </TForm>
    </TDialog>

    <TDialog
      v-model:visible="showPerm"
      :header="t('admin.roles.assignPermissions')"
      :confirm-btn="t('common.save')"
      :cancel-btn="t('common.cancel')"
      width="560px"
      @confirm="handleAssignPermissions"
    >
      <TCheckboxGroup v-model="selectedPerms">
        <div v-for="group in permissionGroups" :key="group.resource" class="perm-group">
          <div class="perm-resource">{{ t(`perm.resource.${group.resource}`) }}</div>
          <div class="perm-actions">
            <TCheckbox
              v-for="p in group.permissions"
              :key="p.id"
              :value="p.id"
              :label="t(`perm.action.${p.action}`)"
              :disabled="!perm.canWrite('role')"
            />
          </div>
        </div>
      </TCheckboxGroup>
    </TDialog>
  </div>
</template>

<style scoped>
.td-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.td-page-header :deep(.t-card__body) {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px 20px;
}
.td-page-header h2 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
}
.perm-group {
  margin-bottom: 16px;
}
.perm-resource {
  font-weight: 600;
  margin-bottom: 8px;
}
.perm-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 20px;
  padding-left: 12px;
}
</style>
