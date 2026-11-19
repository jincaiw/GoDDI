<script setup lang="ts">
import { namedDeleteBody } from '@/utils/confirm';
import { reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { DialogPlugin, Link as TLink, MessagePlugin, Progress as TProgress, Space as TSpace, Switch as TSwitch } from 'tdesign-vue-next';
import type { FormInstanceFunctions, PrimaryTableCol } from 'tdesign-vue-next';
import RemoteTable from '@/components/RemoteTable.vue';
import { usePermission } from '@/composables/usePermission';
import {
  createDHCPScope,
  deleteDHCPScope,
  listDHCPScopes,
  updateDHCPScope,
  type CreateDHCPScopeRequest,
  type DHCPScope
} from '@/service/api/goddi/dhcp';

defineOptions({ name: 'DHCPScopesPage' });

/** Starter CRUD page: TCard header + shared remote TTable + TDialog/TForm. */
const { t } = useI18n();
const perm = usePermission();

const tableRef = ref<{ reload: () => Promise<void> } | null>(null);
const formRef = ref<FormInstanceFunctions | null>(null);
const showDialog = ref(false);
const submitting = ref(false);
const editing = ref<DHCPScope | null>(null);

// `comment` and not `description`: this is the field the scope manager stores.
const formData = reactive<CreateDHCPScopeRequest>({
  name: '',
  subnet: '',
  start_ip: '',
  end_ip: '',
  lease_time: 86400,
  enabled: true,
  comment: ''
});

const formRules = {
  name: [{ required: true, message: t('common.name') + t('common.required'), type: 'error' }],
  subnet: [{ required: true, message: t('dhcp.scopes.subnet') + t('common.required'), type: 'error' }],
  start_ip: [{ required: true, message: t('dhcp.scopes.startIp') + t('common.required'), type: 'error' }],
  end_ip: [{ required: true, message: t('dhcp.scopes.endIp') + t('common.required'), type: 'error' }]
};

const columns: PrimaryTableCol[] = [
  { colKey: 'name', title: t('common.name') },
  { colKey: 'subnet', title: t('dhcp.scopes.subnet') },
  { colKey: 'start_ip', title: t('dhcp.scopes.startIp'), width: 140 },
  { colKey: 'end_ip', title: t('dhcp.scopes.endIp'), width: 140 },
  { colKey: 'lease_time', title: t('dhcp.scopes.leaseTime'), width: 110 },
  { colKey: 'active_leases', title: t('dhcp.scopes.activeLeases'), width: 110 },
  {
    colKey: 'usage',
    title: t('dhcp.scopes.usage'),
    width: 140,
    cell: (h, { row }) => {
      const r = row as DHCPScope;
      const pct = r.total_addresses > 0 ? Math.round((r.active_leases / r.total_addresses) * 100) : 0;
      const status = pct > 90 ? 'error' : pct > 70 ? 'warning' : 'success';
      return h(TProgress, { percentage: pct, status, theme: 'line', size: 'small', label: true });
    }
  },
  {
    colKey: 'enabled',
    title: t('common.enabled'),
    width: 90,
    cell: (h, { row }) =>
      h(TSwitch, {
        size: 'small',
        value: Boolean((row as DHCPScope).enabled),
        disabled: !perm.canWrite('dhcp'),
        onChange: () => void toggleEnabled(row as DHCPScope)
      })
  },
  {
    colKey: 'op',
    title: t('common.actions'),
    width: 150,
    cell: (h, { row }) => {
      const r = row as DHCPScope;
      return h(TSpace, { size: 'small' }, () => [
        h(TLink, { theme: 'primary', disabled: !perm.canWrite('dhcp'), onClick: () => openEdit(r) }, () => t('common.edit')),
        h(TLink, { theme: 'danger', disabled: !perm.canDelete('dhcp'), onClick: () => confirmDelete(r.id, r.name) }, () => t('common.delete'))
      ]);
    }
  }
];

function msgError(err: unknown) {
  void MessagePlugin.error(err instanceof Error ? err.message : t('common.failed'));
}

function openCreateScope() {
  if (!perm.canWrite('dhcp')) return;
  editing.value = null;
  Object.assign(formData, {
    name: '',
    subnet: '',
    start_ip: '',
    end_ip: '',
    lease_time: 86400,
    enabled: true,
    comment: ''
  });
  showDialog.value = true;
}

function openEdit(row: DHCPScope) {
  if (!perm.canWrite('dhcp')) return;
  editing.value = row;
  Object.assign(formData, {
    name: row.name,
    subnet: row.subnet,
    start_ip: row.start_ip,
    end_ip: row.end_ip,
    lease_time: row.lease_time,
    enabled: row.enabled,
    comment: row.comment
  });
  showDialog.value = true;
}

async function toggleEnabled(scope: DHCPScope) {
  if (!perm.canWrite('dhcp')) return;
  try {
    await updateDHCPScope(scope.id, { enabled: !scope.enabled });
    void MessagePlugin.success(t('common.updateSuccess'));
    await tableRef.value?.reload();
  } catch (err) {
    msgError(err);
  }
}

async function handleSubmit() {
  if (!perm.canWrite('dhcp')) return;
  const valid = await formRef.value?.validate();
  if (valid !== true) return;

  submitting.value = true;
  try {
    if (editing.value) {
      await updateDHCPScope(editing.value.id, formData);
      void MessagePlugin.success(t('common.updateSuccess'));
    } else {
      await createDHCPScope(formData);
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

function confirmDelete(id: string, label?: string) {
  if (!perm.canDelete('dhcp')) return;
  const dlg = DialogPlugin.confirm({
    header: t('common.delete'),
    body: namedDeleteBody(label || id),
    theme: 'danger',
    confirmBtn: { theme: 'danger', content: t('common.confirm') },
    onConfirm: async () => {
      try {
        await deleteDHCPScope(id);
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
        <h2>{{ t('dhcp.scopes.title') }}</h2>
      </div>
      <TButton v-if="perm.canWrite('dhcp')" theme="primary" @click="openCreateScope">
        <template #icon><TIcon name="add" /></template>
        {{ t('dhcp.scopes.createScope') }}
      </TButton>
    </TCard>

    <RemoteTable ref="tableRef" :columns="columns" :fetcher="listDHCPScopes" />

    <TDialog
      v-model:visible="showDialog"
      :header="editing ? t('dhcp.scopes.editScope') : t('dhcp.scopes.createScope')"
      :confirm-btn="{ loading: submitting, content: t('common.save') }"
      :cancel-btn="t('common.cancel')"
      width="560px"
      @confirm="handleSubmit"
    >
      <TForm ref="formRef" :data="formData" :rules="formRules" label-align="top">
        <TFormItem :label="t('common.name')" name="name">
          <TInput v-model="formData.name" :placeholder="t('common.name')" />
        </TFormItem>
        <TFormItem :label="t('dhcp.scopes.subnet')" name="subnet">
          <TInput v-model="formData.subnet" placeholder="192.168.1.0/24" />
        </TFormItem>
        <TFormItem :label="t('dhcp.scopes.startIp')" name="start_ip">
          <TInput v-model="formData.start_ip" />
        </TFormItem>
        <TFormItem :label="t('dhcp.scopes.endIp')" name="end_ip">
          <TInput v-model="formData.end_ip" />
        </TFormItem>
        <TFormItem :label="t('dhcp.scopes.leaseTime')" name="lease_time">
          <TInputNumber v-model="formData.lease_time" :min="60" style="width: 100%" />
        </TFormItem>
        <TFormItem :label="t('common.description')" name="comment">
          <TTextarea v-model="formData.comment" :placeholder="t('common.description')" :autosize="{ minRows: 2, maxRows: 4 }" />
        </TFormItem>
        <TFormItem :label="t('common.enabled')" name="enabled">
          <TSwitch v-model="formData.enabled" />
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
  gap: 16px;
  padding: 16px 20px;
}
</style>