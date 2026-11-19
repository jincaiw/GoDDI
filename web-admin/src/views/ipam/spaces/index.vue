<script setup lang="ts">
import { computed, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import {
  DialogPlugin,
  Link as TLink,
  MessagePlugin,
  Space as TSpace,
  Tag as TTag
} from 'tdesign-vue-next';
import type { FormInstanceFunctions, PrimaryTableCol } from 'tdesign-vue-next';
import { usePermission } from '@/composables/usePermission';
import RemoteTable from '@/components/RemoteTable.vue';
import { lifecycleLabel, lifecycleTheme } from '@/composables/useIpamLifecycle';
import { formatAPITime } from '@/utils/datetime';
import {
  createIPAMSpace,
  deleteIPAMSpace,
  listIPAMSpaces,
  updateIPAMSpace,
  type CreateIPAMSpaceRequest,
  type IPAMSpace
} from '@/service/api/goddi/ipam';

defineOptions({ name: 'IPAMSpacesPage' });

const { t } = useI18n();
const perm = usePermission();

const tableRef = ref<{ reload: () => Promise<void> } | null>(null);
const lifecycleFilter = ref('active');
const params = computed(() => ({ lifecycle_state: lifecycleFilter.value }));
const submitting = ref(false);
const showDialog = ref(false);
const editing = ref<IPAMSpace | null>(null);

const formData = reactive<CreateIPAMSpaceRequest>({ name: '', description: '' });
const formRef = ref<FormInstanceFunctions | null>(null);

const formRules = {
  name: [{ required: true, message: t('common.name') + t('common.required'), type: 'error' }]
};

const columns: PrimaryTableCol[] = [
  { colKey: 'name', title: t('common.name'), width: 140, ellipsis: true },
  { colKey: 'description', title: t('common.description'), ellipsis: true },
  {
    colKey: 'lifecycle_state',
    title: t('common.status'),
    width: 90,
    cell: (h, { row }) =>
      h(
        TTag,
        { theme: lifecycleTheme((row as IPAMSpace).lifecycle_state) },
        () => lifecycleLabel(t, (row as IPAMSpace).lifecycle_state)
      )
  },
  {
    colKey: 'created_at',
    title: t('common.createdAt'),
    width: 150,
    cell: (_h, { row }) => formatAPITime((row as IPAMSpace).created_at)
  },
  {
    colKey: 'op',
    title: t('common.actions'),
    width: 110,
    cell: (h, { row }) =>
      h(TSpace, { size: 'small', wrap: true }, () => [
        h(
          TLink,
          {
            theme: 'primary',
            disabled: !perm.canWrite('ipam') || (row as IPAMSpace).lifecycle_state === 'retired',
            onClick: () => openEdit(row as IPAMSpace)
          },
          () => t('common.edit')
        ),
        h(
          TLink,
          {
            theme: 'danger',
            disabled: !perm.canDelete('ipam') || (row as IPAMSpace).lifecycle_state === 'retired',
            onClick: () => confirmDelete(row as IPAMSpace)
          },
          () => t('ipam.spaces.retire')
        )
      ])
  }
];

function msgError(err: unknown) {
  void MessagePlugin.error(err instanceof Error ? err.message : t('common.failed'));
}

function handleLifecycleChange(value: string) {
  // RemoteTable deep-watches params and resets to page 1 automatically.
  lifecycleFilter.value = value;
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
    await tableRef.value?.reload();
  } catch (err) {
    msgError(err);
  } finally {
    submitting.value = false;
  }
}

function confirmDelete(row: IPAMSpace) {
  if (!perm.canDelete('ipam')) return;
  const dlg = DialogPlugin.confirm({
    header: t('ipam.spaces.retire'),
    body: `${t('ipam.spaces.retireConfirm')}（ID：${row.id}）`,
    theme: 'danger',
    confirmBtn: { theme: 'danger', content: t('common.confirm') },
    onConfirm: async () => {
      try {
        await deleteIPAMSpace(row.id);
        void MessagePlugin.success(t('ipam.spaces.retireSuccess'));
        await tableRef.value?.reload();
      } catch (err) {
        msgError(err);
      }
      dlg.hide();
    }
  });
}

</script>

<template>
  <div class="td-page">
    <TCard :bordered="false" class="td-page-header">
      <h2>{{ t('ipam.spaces.title') }}</h2>
      <TSelect
        :value="lifecycleFilter"
        :options="[
          { label: t('ipam.lifecycleActive'), value: 'active' },
          { label: t('ipam.lifecycleRetired'), value: 'retired' },
          { label: t('ipam.lifecycleAll'), value: 'all' }
        ]"
        style="width: 140px"
        @change="handleLifecycleChange"
      />
      <TButton v-if="perm.canWrite('ipam')" theme="primary" @click="openCreate">
        <template #icon><TIcon name="add" /></template>
        {{ t('ipam.spaces.createSpace') }}
      </TButton>
    </TCard>

    <RemoteTable
      ref="tableRef"
      :columns="columns"
      :fetcher="listIPAMSpaces"
      :params="params"
      :empty-text="t('common.noData')"
    />

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
  padding: 16px 20px;
}
</style>