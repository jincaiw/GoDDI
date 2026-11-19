<script setup lang="ts">
import { namedDeleteBody } from '@/utils/confirm';
import { reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { DialogPlugin, MessagePlugin, Switch as TSwitch, Tag as TTag, Link as TLink } from 'tdesign-vue-next';
import type { FormInstanceFunctions, PrimaryTableCol } from 'tdesign-vue-next';
import RemoteTable from '@/components/RemoteTable.vue';
import { usePermission } from '@/composables/usePermission';
import {
  createClientPolicy,
  deleteClientPolicy,
  listClientPolicies,
  type ClientPolicy
} from '@/service/api/goddi/dns';

defineOptions({ name: 'DNSSecurityPage' });

const { t } = useI18n();
const perm = usePermission();

const tableRef = ref<{ reload: () => Promise<void> } | null>(null);
const formRef = ref<FormInstanceFunctions | null>(null);
const showDialog = ref(false);
const submitting = ref(false);

const policyForm = reactive({
  name: '',
  source_cidr: '',
  action: 'allow',
  block_list_ids: [] as string[],
  allow_rule_ids: [] as string[],
  enabled: true,
  priority: 0
});

const actionValues = ['allow', 'block', 'apply_lists'];
const actionOptions = actionValues.map(value => ({ label: actionLabel(value), value }));
function actionLabel(value: string): string {
  const key = `dns.security.actionValues.${String(value).toLowerCase()}`;
  const out = t(key);
  return out === key ? value : out;
}

const formRules = {
  name: [{ required: true, message: t('common.name') + t('common.required'), type: 'error' }],
  source_cidr: [{ required: true, message: t('dns.security.sourceCidr') + t('common.required'), type: 'error' }]
};

const columns: PrimaryTableCol[] = [
  { colKey: 'name', title: t('common.name') },
  { colKey: 'source_cidr', title: t('dns.security.sourceCidr') },
  {
    colKey: 'action',
    title: t('dns.security.action'),
    cell: (h, { row }) =>
      h(TTag, { size: 'small', theme: (row as ClientPolicy).action === 'allow' ? 'success' : 'danger', variant: 'light' }, () =>
        actionLabel(String((row as ClientPolicy).action))
      )
  },
  { colKey: 'priority', title: t('common.priority'), width: 90 },
  {
    colKey: 'enabled',
    title: t('common.enabled'),
    width: 90,
    cell: (h, { row }) => h(TSwitch, { size: 'small', disabled: true, value: Boolean((row as ClientPolicy).enabled) })
  },
  {
    colKey: 'op',
    title: t('common.actions'),
    width: 100,
    cell: (h, { row }) =>
      h(
        TLink,
        {
          theme: 'danger',
          disabled: !perm.canDelete('dns'),
          onClick: () => confirmDelete((row as ClientPolicy).id, (row as ClientPolicy).name)
        },
        () => t('common.delete')
      )
  }
];

function msgError(err: unknown) {
  void MessagePlugin.error(err instanceof Error ? err.message : t('common.failed'));
}

function openCreate() {
  if (!perm.canWrite('dns')) return;
  Object.assign(policyForm, {
    name: '',
    source_cidr: '',
    action: 'allow',
    block_list_ids: [],
    allow_rule_ids: [],
    enabled: true,
    priority: 0
  });
  showDialog.value = true;
}

async function handleSubmit() {
  if (!perm.canWrite('dns')) return;
  const valid = await formRef.value?.validate();
  if (valid !== true) return;

  submitting.value = true;
  try {
    await createClientPolicy(policyForm);
    void MessagePlugin.success(t('common.createSuccess'));
    showDialog.value = false;
    await tableRef.value?.reload();
  } catch (err) {
    msgError(err);
  } finally {
    submitting.value = false;
  }
}

function confirmDelete(id: string, label?: string) {
  if (!perm.canDelete('dns')) return;
  const dlg = DialogPlugin.confirm({
    header: t('common.delete'),
    body: namedDeleteBody(label || id),
    theme: 'danger',
    onConfirm: async () => {
      try {
        await deleteClientPolicy(id);
        void MessagePlugin.success(t('common.deleteSuccess'));
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
      <div class="td-page-title">
        <h2>{{ t('dns.security.title') }}</h2>
      </div>
      <TButton v-if="perm.canWrite('dns')" theme="primary" @click="openCreate">
        <template #icon><TIcon name="add" /></template>
        {{ t('dns.security.createPolicy') }}
      </TButton>
    </TCard>

    <RemoteTable ref="tableRef" :columns="columns" :fetcher="listClientPolicies" />

    <TDialog
      v-model:visible="showDialog"
      :header="t('dns.security.createPolicy')"
      :confirm-btn="{ loading: submitting, content: t('common.save') }"
      :cancel-btn="t('common.cancel')"
      width="480px"
      @confirm="handleSubmit"
    >
      <TForm ref="formRef" :data="policyForm" :rules="formRules" label-align="top">
        <TFormItem :label="t('common.name')" name="name">
          <TInput v-model="policyForm.name" />
        </TFormItem>
        <TFormItem :label="t('dns.security.sourceCidr')" name="source_cidr">
          <TInput v-model="policyForm.source_cidr" placeholder="192.168.1.0/24" />
        </TFormItem>
        <TFormItem :label="t('dns.security.action')" name="action">
          <TSelect v-model="policyForm.action" :options="actionOptions" />
        </TFormItem>
        <TFormItem :label="t('common.enabled')" name="enabled">
          <TSwitch v-model="policyForm.enabled" />
        </TFormItem>
        <TFormItem :label="t('common.priority')" name="priority">
          <TInputNumber v-model="policyForm.priority" :min="0" />
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