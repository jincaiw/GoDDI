<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import {
  DialogPlugin,
  Link as TLink,
  MessagePlugin,
  Space as TSpace
} from 'tdesign-vue-next';
import type { FormInstanceFunctions, PrimaryTableCol, PageInfo } from 'tdesign-vue-next';
import { usePermission } from '@/composables/usePermission';
import {
  createIPAMSpace,
  deleteIPAMSpace,
  listIPAMSpaces,
  updateIPAMSpace,
  type CreateIPAMSpaceRequest,
  type IPAMSpace
} from '@/service/api/goddi/ipam';

defineOptions({ name: 'IPAMSpacesPage' });

/**
 * TDesign reference page for the staged console migration.
 *
 * This is the template every remaining page will be migrated to. It covers the
 * full pattern with real backend calls: remote-paginated table, create/edit
 * form in a dialog with validation, delete confirmation, RBAC gating, loading
 * and error handling, and i18n. The API layer (`service/api/goddi/client.ts`)
 * and the auth store are reused unchanged, so JWT/CSRF handling, the 401
 * refresh queue and the {code,message,data} envelope stay identical.
 *
 * Single-node scope: this page talks to the local control API only. Multi-node
 * (HA / replication) UI is deliberately deferred; see docs/tdesign-migration.md.
 */

const { t } = useI18n();
const perm = usePermission();

const loading = ref(false);
const submitting = ref(false);
const rows = ref<IPAMSpace[]>([]);
const showDialog = ref(false);
const editing = ref<IPAMSpace | null>(null);

const pagination = reactive({
  current: 1,
  pageSize: 20,
  total: 0,
  showPageSize: true,
  pageSizeOptions: [10, 20, 50]
});

const formData = reactive<CreateIPAMSpaceRequest>({ name: '', description: '' });
const formRef = ref<FormInstanceFunctions | null>(null);

const formRules = {
  name: [{ required: true, message: t('common.name') + ' required', type: 'error' }]
};

const columns: PrimaryTableCol<IPAMSpace>[] = [
  { colKey: 'name', title: t('common.name'), minWidth: 160 },
  { colKey: 'description', title: t('common.description'), ellipsis: true },
  { colKey: 'created_at', title: t('common.createdAt'), width: 180 },
  {
    colKey: 'op',
    title: t('common.actions'),
    width: 180,
    cell: (h, { row }) =>
      h(TSpace, { size: 'small' }, () => [
        h(
          TLink,
          {
            theme: 'primary',
            disabled: !perm.canWrite('ipam'),
            onClick: () => openEdit(row as IPAMSpace)
          },
          () => t('common.edit')
        ),
        h(
          TLink,
          {
            theme: 'danger',
            disabled: !perm.canDelete('ipam'),
            onClick: () => confirmDelete(row as IPAMSpace)
          },
          () => t('common.delete')
        )
      ])
  }
];

function msgError(err: unknown) {
  void MessagePlugin.error(err instanceof Error ? err.message : t('common.failed'));
}

async function loadData() {
  loading.value = true;
  try {
    const result = await listIPAMSpaces({ page: pagination.current, page_size: pagination.pageSize });
    rows.value = result.data;
    pagination.total = result.meta.total;
  } catch (err) {
    msgError(err);
  } finally {
    loading.value = false;
  }
}

function handlePageChange(pageInfo: PageInfo) {
  pagination.current = pageInfo.current;
  pagination.pageSize = pageInfo.pageSize;
  void loadData();
}

function openCreate() {
  if (!perm.canWrite('ipam')) return;
  editing.value = null;
  Object.assign(formData, { name: '', description: '' });
  showDialog.value = true;
}

function openEdit(row: IPAMSpace) {
  if (!perm.canWrite('ipam')) return;
  editing.value = row;
  Object.assign(formData, { name: row.name, description: row.description });
  showDialog.value = true;
}

async function handleSubmit() {
  if (!perm.canWrite('ipam')) return;
  const valid = await formRef.value?.validate();
  if (valid !== true) return;

  submitting.value = true;
  try {
    if (editing.value) {
      await updateIPAMSpace(editing.value.id, formData);
      void MessagePlugin.success(t('common.updateSuccess'));
    } else {
      await createIPAMSpace(formData);
      void MessagePlugin.success(t('common.createSuccess'));
    }
    showDialog.value = false;
    editing.value = null;
    await loadData();
  } catch (err) {
    msgError(err);
  } finally {
    submitting.value = false;
  }
}

function confirmDelete(row: IPAMSpace) {
  if (!perm.canDelete('ipam')) return;
  const dlg = DialogPlugin.confirm({
    header: t('common.delete'),
    body: `${t('common.deleteConfirm')} (${row.name})`,
    theme: 'danger',
    confirmBtn: { theme: 'danger', content: t('common.confirm') },
    onConfirm: async () => {
      try {
        await deleteIPAMSpace(row.id);
        void MessagePlugin.success(t('common.deleteSuccess'));
        await loadData();
      } catch (err) {
        msgError(err);
      }
      dlg.hide();
    }
  });
}

onMounted(loadData);
</script>

<template>
  <div class="td-page">
    <TCard :bordered="false" class="td-page-header">
      <div class="td-page-title">
        <h2>{{ t('ipam.spaces.title') }}</h2>
        <TTag theme="primary" variant="light">TDesign</TTag>
      </div>
      <TButton v-if="perm.canWrite('ipam')" theme="primary" @click="openCreate">
        <template #icon><TIcon name="add" /></template>
        {{ t('ipam.spaces.createSpace') }}
      </TButton>
    </TCard>

    <TCard :bordered="false">
      <TTable
        row-key="id"
        :data="rows"
        :columns="columns"
        :loading="loading"
        :pagination="{
          current: pagination.current,
          pageSize: pagination.pageSize,
          total: pagination.total,
          showPageSize: pagination.showPageSize,
          pageSizeOptions: pagination.pageSizeOptions
        }"
        stripe
        hover
        @page-change="handlePageChange"
      />
    </TCard>

    <TDialog
      v-model:visible="showDialog"
      :header="editing ? t('ipam.spaces.editSpace') : t('ipam.spaces.createSpace')"
      :confirm-btn="{ loading: submitting, content: t('common.save') }"
      :cancel-btn="t('common.cancel')"
      width="480px"
      @confirm="handleSubmit"
    >
      <TForm ref="formRef" :data="formData" :rules="formRules" label-align="top">
        <TFormItem :label="t('common.name')" name="name">
          <TInput v-model="formData.name" :placeholder="t('common.name')" />
        </TFormItem>
        <TFormItem :label="t('common.description')" name="description">
          <TTextarea v-model="formData.description" :placeholder="t('common.description')" :autosize="{ minRows: 3 }" />
        </TFormItem>
      </TForm>
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
}
.td-page-title {
  display: flex;
  align-items: center;
  gap: 12px;
}
.td-page-title h2 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
}
</style>
