<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { DialogPlugin, Link as TLink, MessagePlugin, Space as TSpace } from 'tdesign-vue-next';
import type { FormInstanceFunctions, PrimaryTableCol } from 'tdesign-vue-next';
import RemoteTable from '@/components/RemoteTable.vue';
import { usePermission } from '@/composables/usePermission';
import {
  createDHCPOption,
  deleteDHCPOption,
  listDHCPOptions,
  listDHCPScopes,
  updateDHCPOption,
  type CreateDHCPOptionRequest,
  type DHCPOption
} from '@/service/api/goddi/dhcp';

defineOptions({ name: 'DHCPOptionsPage' });

const { t } = useI18n();
const perm = usePermission();

const tableRef = ref<{ reload: () => Promise<void> } | null>(null);
const formRef = ref<FormInstanceFunctions | null>(null);
const showDialog = ref(false);
const submitting = ref(false);
const editing = ref<DHCPOption | null>(null);
const scopeOptions = ref<Array<{ label: string; value: string }>>([]);

const formData = reactive<CreateDHCPOptionRequest>({ code: 1, value: '', priority: 'global', scope_id: '' });

const priorityOptions = [
  { label: 'Global', value: 'global' },
  { label: 'Scope', value: 'scope' },
  { label: 'Client Class', value: 'client_class' },
  { label: 'Reservation', value: 'reservation' }
];

const formRules = {
  value: [{ required: true, message: t('dhcp.options.optionValue') + ' required', type: 'error' }],
  scope_id: [{ required: true, message: t('dhcp.options.scopeRequired'), type: 'error' }]
};

const columns: PrimaryTableCol[] = [
  { colKey: 'code', title: t('dhcp.options.code'), width: 90 },
  { colKey: 'value', title: t('dhcp.options.optionValue'), ellipsis: true },
  { colKey: 'scope_id', title: t('dhcp.options.scope'), ellipsis: true },
  { colKey: 'priority', title: t('common.priority'), width: 120 },
  {
    colKey: 'op',
    title: t('common.actions'),
    width: 160,
    cell: (h, { row }) => {
      const r = row as DHCPOption;
      return h(TSpace, { size: 'small' }, () => [
        h(TLink, { theme: 'primary', disabled: !perm.canWrite('dhcp'), onClick: () => openEdit(r) }, () => t('common.edit')),
        h(TLink, { theme: 'danger', disabled: !perm.canDelete('dhcp'), onClick: () => confirmDelete(r.id) }, () => t('common.delete'))
      ]);
    }
  }
];

function msgError(err: unknown) {
  void MessagePlugin.error(err instanceof Error ? err.message : t('common.failed'));
}

async function loadScopes() {
  try {
    scopeOptions.value = (await listDHCPScopes({ page_size: 100 })).data.map(s => ({
      label: `${s.name} (${s.subnet})`,
      value: s.id
    }));
  } catch (err) {
    msgError(err);
  }
}

function openCreate() {
  if (!perm.canWrite('dhcp')) return;
  editing.value = null;
  Object.assign(formData, { code: 1, value: '', priority: 'global', scope_id: '' });
  showDialog.value = true;
}

function openEdit(row: DHCPOption) {
  if (!perm.canWrite('dhcp')) return;
  editing.value = row;
  Object.assign(formData, { code: row.code, value: row.value, priority: row.priority, scope_id: row.scope_id });
  showDialog.value = true;
}

async function handleSubmit() {
  if (!perm.canWrite('dhcp')) return;
  const valid = await formRef.value?.validate();
  if (valid !== true) return;

  submitting.value = true;
  try {
    if (editing.value) {
      await updateDHCPOption(editing.value.id, formData);
      void MessagePlugin.success(t('common.updateSuccess'));
    } else {
      await createDHCPOption(formData);
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
  if (!perm.canDelete('dhcp')) return;
  const dlg = DialogPlugin.confirm({
    header: t('common.delete'),
    body: t('common.deleteConfirm'),
    theme: 'danger',
    onConfirm: async () => {
      try {
        await deleteDHCPOption(id);
        void MessagePlugin.success(t('common.deleteSuccess'));
        await tableRef.value?.reload();
      } catch (err) {
        msgError(err);
      }
      dlg.hide();
    }
  });
}

onMounted(loadScopes);
</script>

<template>
  <div class="td-page">
    <TCard :bordered="false" class="td-page-header">
      <h2>{{ t('dhcp.options.title') }}</h2>
      <TButton v-if="perm.canWrite('dhcp')" theme="primary" @click="openCreate">
        <template #icon><TIcon name="add" /></template>
        {{ t('dhcp.options.createOption') }}
      </TButton>
    </TCard>

    <RemoteTable ref="tableRef" :columns="columns" :fetcher="listDHCPOptions" />

    <TDialog
      v-model:visible="showDialog"
      :header="editing ? t('dhcp.options.editOption') : t('dhcp.options.createOption')"
      :confirm-btn="{ loading: submitting, content: t('common.save') }"
      :cancel-btn="t('common.cancel')"
      width="520px"
      @confirm="handleSubmit"
    >
      <TForm ref="formRef" :data="formData" :rules="formRules" label-align="top">
        <TFormItem :label="t('dhcp.options.code')" name="code">
          <TInputNumber v-model="formData.code" :min="1" :max="254" style="width: 100%" />
        </TFormItem>
        <TFormItem :label="t('dhcp.options.optionValue')" name="value">
          <TInput v-model="formData.value" />
        </TFormItem>
        <TFormItem :label="t('common.priority')" name="priority">
          <TSelect v-model="formData.priority" :options="priorityOptions" />
        </TFormItem>
        <TFormItem :label="t('dhcp.options.scope')" name="scope_id">
          <TSelect v-model="formData.scope_id" :options="scopeOptions" filterable :disabled="!!editing" />
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
.td-page-header h2 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
}
</style>
