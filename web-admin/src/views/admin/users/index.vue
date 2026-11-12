<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { DialogPlugin, Link as TLink, MessagePlugin, Space as TSpace, Switch as TSwitch, Tag as TTag } from 'tdesign-vue-next';
import type { FormInstanceFunctions, PrimaryTableCol } from 'tdesign-vue-next';
import RemoteTable from '@/components/RemoteTable.vue';
import { usePermission } from '@/composables/usePermission';
import {
  assignUserRoles,
  createUser,
  deleteUser,
  listRoles,
  listUsers,
  updateUser,
  type CreateUserRequest,
  type Role,
  type User
} from '@/service/api/goddi/user';

defineOptions({ name: 'AdminUsersPage' });

const { t } = useI18n();
const perm = usePermission();

const tableRef = ref<{ reload: () => Promise<void> } | null>(null);
const formRef = ref<FormInstanceFunctions | null>(null);
const showDialog = ref(false);
const showRoles = ref(false);
const submitting = ref(false);
const editing = ref<User | null>(null);
const allRoles = ref<Role[]>([]);
const selectedRoles = ref<string[]>([]);
const assigningUserId = ref('');

const formData = reactive<CreateUserRequest & { enabled: boolean }>({
  username: '',
  email: '',
  password: '',
  display_name: '',
  enabled: true
});

const formRules = {
  username: [{ required: true, message: t('admin.users.username') + ' required', type: 'error' }]
};

const columns: PrimaryTableCol[] = [
  { colKey: 'username', title: t('admin.users.username') },
  { colKey: 'display_name', title: t('admin.users.displayName') },
  { colKey: 'email', title: t('admin.users.email'), ellipsis: true },
  {
    colKey: 'roles',
    title: t('admin.users.roles'),
    cell: (h, { row }) =>
      h(
        TSpace,
        { size: 'small' },
        () =>
          ((row as User).roles || []).map(r =>
            h(TTag, { size: 'small', theme: 'primary', variant: 'light' }, () => r.name)
          )
      )
  },
  {
    colKey: 'enabled',
    title: t('common.enabled'),
    width: 90,
    cell: (h, { row }) => {
      const r = row as User;
      return h(TSwitch, {
        size: 'small',
        value: Boolean(r.enabled),
        disabled: !perm.canWrite('user'),
        onChange: (value: unknown) => void toggleEnabled(r, Boolean(value))
      });
    }
  },
  {
    colKey: 'totp_enabled',
    title: t('admin.users.totpEnabled'),
    width: 90,
    cell: (h, { row }) =>
      h(TTag, { size: 'small', theme: (row as User).totp_enabled ? 'success' : 'default', variant: 'light' }, () =>
        (row as User).totp_enabled ? 'ON' : 'OFF'
      )
  },
  {
    colKey: 'op',
    title: t('common.actions'),
    width: 220,
    cell: (h, { row }) => {
      const r = row as User;
      return h(TSpace, { size: 'small' }, () => [
        h(TLink, { theme: 'primary', disabled: !perm.canWrite('user'), onClick: () => openEdit(r) }, () => t('common.edit')),
        h(
          TLink,
          {
            theme: 'primary',
            disabled: !perm.canWrite('user') || !perm.canRead('role'),
            onClick: () => openRoles(r)
          },
          () => t('admin.users.assignRoles')
        ),
        h(TLink, { theme: 'danger', disabled: !perm.canDelete('user'), onClick: () => confirmDelete(r.id) }, () => t('common.delete'))
      ]);
    }
  }
];

function msgError(err: unknown) {
  void MessagePlugin.error(err instanceof Error ? err.message : t('common.failed'));
}

function openCreate() {
  if (!perm.canWrite('user')) return;
  editing.value = null;
  Object.assign(formData, { username: '', email: '', password: '', display_name: '', enabled: true });
  showDialog.value = true;
}

function openEdit(row: User) {
  if (!perm.canWrite('user')) return;
  editing.value = row;
  Object.assign(formData, {
    username: row.username,
    email: row.email,
    password: '',
    display_name: row.display_name,
    enabled: row.enabled
  });
  showDialog.value = true;
}

function openRoles(row: User) {
  if (!perm.canWrite('user') || !perm.canRead('role')) return;
  assigningUserId.value = row.id;
  selectedRoles.value = (row.roles || []).map(r => r.id);
  showRoles.value = true;
}

async function toggleEnabled(row: User, value: boolean) {
  if (!perm.canWrite('user')) return;
  try {
    await updateUser(row.id, { enabled: value });
    await tableRef.value?.reload();
  } catch (err) {
    msgError(err);
  }
}

async function handleSubmit() {
  if (!perm.canWrite('user')) return;
  const valid = await formRef.value?.validate();
  if (valid !== true) return;
  if (!editing.value && formData.password.length < 8) {
    void MessagePlugin.error(t('auth.passwordMinLength'));
    return;
  }

  submitting.value = true;
  try {
    if (editing.value) {
      await updateUser(editing.value.id, formData);
      void MessagePlugin.success(t('common.updateSuccess'));
    } else {
      await createUser(formData);
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
  if (!perm.canDelete('user')) return;
  const dlg = DialogPlugin.confirm({
    header: t('common.delete'),
    body: t('common.deleteConfirm'),
    theme: 'danger',
    onConfirm: async () => {
      try {
        await deleteUser(id);
        void MessagePlugin.success(t('common.deleteSuccess'));
        await tableRef.value?.reload();
      } catch (err) {
        msgError(err);
      }
      dlg.hide();
    }
  });
}

async function handleAssignRoles() {
  if (!perm.canWrite('user') || !perm.canRead('role')) return;
  try {
    await assignUserRoles(assigningUserId.value, selectedRoles.value);
    void MessagePlugin.success(t('common.updateSuccess'));
    showRoles.value = false;
    await tableRef.value?.reload();
  } catch (err) {
    msgError(err);
  }
}

async function loadRoles() {
  try {
    allRoles.value = (await listRoles({ page: 1, page_size: 100 })).data;
  } catch {
    // Roles are optional for the list; the assign dialog will simply be empty.
  }
}

onMounted(loadRoles);
</script>

<template>
  <div class="td-page">
    <TCard :bordered="false" class="td-page-header">
      <h2>{{ t('admin.users.title') }}</h2>
      <TButton v-if="perm.canWrite('user')" theme="primary" @click="openCreate">
        <template #icon><TIcon name="add" /></template>
        {{ t('admin.users.createUser') }}
      </TButton>
    </TCard>

    <RemoteTable ref="tableRef" :columns="columns" :fetcher="listUsers" />

    <TDialog
      v-model:visible="showDialog"
      :header="editing ? t('admin.users.editUser') : t('admin.users.createUser')"
      :confirm-btn="{ loading: submitting, content: t('common.save') }"
      :cancel-btn="t('common.cancel')"
      width="520px"
      @confirm="handleSubmit"
    >
      <TForm ref="formRef" :data="formData" :rules="formRules" label-align="top">
        <TFormItem :label="t('admin.users.username')" name="username">
          <TInput v-model="formData.username" :disabled="!!editing" />
        </TFormItem>
        <TFormItem v-if="!editing" :label="t('auth.password')" name="password">
          <TInput v-model="formData.password" type="password" />
        </TFormItem>
        <TFormItem :label="t('admin.users.email')" name="email">
          <TInput v-model="formData.email" />
        </TFormItem>
        <TFormItem :label="t('admin.users.displayName')" name="display_name">
          <TInput v-model="formData.display_name" />
        </TFormItem>
        <TFormItem :label="t('common.enabled')" name="enabled">
          <TSwitch v-model="formData.enabled" />
        </TFormItem>
      </TForm>
    </TDialog>

    <TDialog
      v-model:visible="showRoles"
      :header="t('admin.users.assignRoles')"
      :confirm-btn="t('common.save')"
      :cancel-btn="t('common.cancel')"
      width="480px"
      @confirm="handleAssignRoles"
    >
      <TCheckboxGroup v-model="selectedRoles" class="role-list">
        <TCheckbox
          v-for="role in allRoles"
          :key="role.id"
          :value="role.id"
          :label="role.name"
          :disabled="!perm.canWrite('user') || !perm.canRead('role')"
        />
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
.role-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
</style>
