<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { DialogPlugin, Link as TLink, MessagePlugin, Space as TSpace, Tag as TTag } from 'tdesign-vue-next';
import type { FormInstanceFunctions, PrimaryTableCol } from 'tdesign-vue-next';
import RemoteTable from '@/components/RemoteTable.vue';
import { usePermission } from '@/composables/usePermission';
import {
  assignGroupRoles,
  createGroup,
  deleteGroup,
  listGroups,
  listRoles,
  updateGroup,
  type CreateGroupRequest,
  type Group,
  type Role
} from '@/service/api/goddi/user';

defineOptions({ name: 'AdminGroupsPage' });

const { t } = useI18n();
const perm = usePermission();

const tableRef = ref<{ reload: () => Promise<void> } | null>(null);
const formRef = ref<FormInstanceFunctions | null>(null);
const showDialog = ref(false);
const showRoles = ref(false);
const submitting = ref(false);
const editing = ref<Group | null>(null);
const allRoles = ref<Role[]>([]);
const selectedRoles = ref<string[]>([]);
const assigningGroupId = ref('');

const formData = reactive<CreateGroupRequest>({ name: '', description: '' });

const formRules = {
  name: [{ required: true, message: t('common.name') + ' required', type: 'error' }]
};

const columns: PrimaryTableCol[] = [
  { colKey: 'name', title: t('common.name') },
  { colKey: 'description', title: t('common.description'), ellipsis: true },
  {
    colKey: 'roles',
    title: t('admin.users.roles'),
    cell: (h, { row }) =>
      h(
        TSpace,
        { size: 'small' },
        () =>
          ((row as Group).roles || []).map(r =>
            h(TTag, { size: 'small', theme: 'primary', variant: 'light' }, () => r.name)
          )
      )
  },
  {
    colKey: 'op',
    title: t('common.actions'),
    width: 220,
    cell: (h, { row }) => {
      const r = row as Group;
      return h(TSpace, { size: 'small' }, () => [
        h(TLink, { theme: 'primary', disabled: !perm.canWrite('group'), onClick: () => openEdit(r) }, () => t('common.edit')),
        h(
          TLink,
          {
            theme: 'primary',
            disabled: !perm.canWrite('group') || !perm.canRead('role'),
            onClick: () => openRoles(r)
          },
          () => t('admin.groups.assignRoles')
        ),
        h(TLink, { theme: 'danger', disabled: !perm.canDelete('group'), onClick: () => confirmDelete(r.id) }, () => t('common.delete'))
      ]);
    }
  }
];

function msgError(err: unknown) {
  void MessagePlugin.error(err instanceof Error ? err.message : t('common.failed'));
}

function openCreate() {
  if (!perm.canWrite('group')) return;
  editing.value = null;
  Object.assign(formData, { name: '', description: '' });
  showDialog.value = true;
}

function openEdit(row: Group) {
  if (!perm.canWrite('group')) return;
  editing.value = row;
  Object.assign(formData, { name: row.name, description: row.description });
  showDialog.value = true;
}

function openRoles(row: Group) {
  if (!perm.canWrite('group') || !perm.canRead('role')) return;
  assigningGroupId.value = row.id;
  selectedRoles.value = (row.roles || []).map(r => r.id);
  showRoles.value = true;
}

async function handleSubmit() {
  if (!perm.canWrite('group')) return;
  const valid = await formRef.value?.validate();
  if (valid !== true) return;

  submitting.value = true;
  try {
    if (editing.value) {
      await updateGroup(editing.value.id, formData);
      void MessagePlugin.success(t('common.updateSuccess'));
    } else {
      await createGroup(formData);
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
  if (!perm.canDelete('group')) return;
  const dlg = DialogPlugin.confirm({
    header: t('common.delete'),
    body: t('common.deleteConfirm'),
    theme: 'danger',
    onConfirm: async () => {
      try {
        await deleteGroup(id);
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
  if (!perm.canWrite('group') || !perm.canRead('role')) return;
  try {
    await assignGroupRoles(assigningGroupId.value, selectedRoles.value);
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
      <h2>{{ t('admin.groups.title') }}</h2>
      <TButton v-if="perm.canWrite('group')" theme="primary" @click="openCreate">
        <template #icon><TIcon name="add" /></template>
        {{ t('admin.groups.createGroup') }}
      </TButton>
    </TCard>

    <RemoteTable ref="tableRef" :columns="columns" :fetcher="listGroups" />

    <TDialog
      v-model:visible="showDialog"
      :header="editing ? t('admin.groups.editGroup') : t('admin.groups.createGroup')"
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
      v-model:visible="showRoles"
      :header="t('admin.groups.assignRoles')"
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
          :disabled="!perm.canWrite('group') || !perm.canRead('role')"
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
