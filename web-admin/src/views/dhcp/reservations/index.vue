<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { DialogPlugin, Link as TLink, MessagePlugin, Space as TSpace, Switch as TSwitch } from 'tdesign-vue-next';
import type { FormInstanceFunctions, PrimaryTableCol } from 'tdesign-vue-next';
import RemoteTable from '@/components/RemoteTable.vue';
import { usePermission } from '@/composables/usePermission';
import {
  createDHCPReservation,
  deleteDHCPReservation,
  listDHCPReservations,
  listDHCPScopes,
  updateDHCPReservation,
  type CreateDHCPReservationRequest,
  type DHCPReservation
} from '@/service/api/goddi/dhcp';

defineOptions({ name: 'DHCPReservationsPage' });

const { t } = useI18n();
const perm = usePermission();

const tableRef = ref<{ reload: () => Promise<void> } | null>(null);
const formRef = ref<FormInstanceFunctions | null>(null);
const showDialog = ref(false);
const submitting = ref(false);
const editing = ref<DHCPReservation | null>(null);
const scopeOptions = ref<Array<{ label: string; value: string }>>([]);

const formData = reactive<CreateDHCPReservationRequest & { description: string }>({
  ip_address: '',
  mac_address: '',
  hostname: '',
  scope_id: '',
  enabled: true,
  description: ''
});

const formRules = {
  ip_address: [{ required: true, message: t('dhcp.leases.ip') + ' required', type: 'error' }],
  mac_address: [{ required: true, message: t('dhcp.leases.mac') + ' required', type: 'error' }],
  scope_id: [{ required: true, message: t('dhcp.options.scopeRequired'), type: 'error' }]
};

const columns: PrimaryTableCol[] = [
  { colKey: 'ip_address', title: t('dhcp.leases.ip'), width: 150 },
  { colKey: 'mac_address', title: t('dhcp.leases.mac'), width: 170 },
  { colKey: 'hostname', title: t('dhcp.leases.hostname'), ellipsis: true },
  { colKey: 'scope_name', title: t('dhcp.leases.scope'), ellipsis: true },
  {
    colKey: 'enabled',
    title: t('common.enabled'),
    width: 90,
    cell: (h, { row }) => h(TSwitch, { size: 'small', disabled: true, value: Boolean((row as DHCPReservation).enabled) })
  },
  {
    colKey: 'op',
    title: t('common.actions'),
    width: 160,
    cell: (h, { row }) => {
      const r = row as DHCPReservation;
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
  Object.assign(formData, {
    ip_address: '',
    mac_address: '',
    hostname: '',
    scope_id: '',
    enabled: true,
    description: ''
  });
  showDialog.value = true;
}

function openEdit(row: DHCPReservation) {
  if (!perm.canWrite('dhcp')) return;
  editing.value = row;
  Object.assign(formData, {
    ip_address: row.ip_address,
    mac_address: row.mac_address,
    hostname: row.hostname,
    scope_id: row.scope_id,
    enabled: row.enabled,
    description: row.description
  });
  showDialog.value = true;
}

async function handleSubmit() {
  if (!perm.canWrite('dhcp')) return;
  const valid = await formRef.value?.validate();
  if (valid !== true) return;

  submitting.value = true;
  try {
    if (editing.value) {
      await updateDHCPReservation(editing.value.id, formData);
      void MessagePlugin.success(t('common.updateSuccess'));
    } else {
      await createDHCPReservation(formData);
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
        await deleteDHCPReservation(id);
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
      <h2>{{ t('dhcp.reservations.title') }}</h2>
      <TButton v-if="perm.canWrite('dhcp')" theme="primary" @click="openCreate">
        <template #icon><TIcon name="add" /></template>
        {{ t('dhcp.reservations.createReservation') }}
      </TButton>
    </TCard>

    <RemoteTable ref="tableRef" :columns="columns" :fetcher="listDHCPReservations" />

    <TDialog
      v-model:visible="showDialog"
      :header="editing ? t('dhcp.reservations.editReservation') : t('dhcp.reservations.createReservation')"
      :confirm-btn="{ loading: submitting, content: t('common.save') }"
      :cancel-btn="t('common.cancel')"
      width="520px"
      @confirm="handleSubmit"
    >
      <TForm ref="formRef" :data="formData" :rules="formRules" label-align="top">
        <TFormItem :label="t('dhcp.leases.ip')" name="ip_address">
          <TInput v-model="formData.ip_address" />
        </TFormItem>
        <TFormItem :label="t('dhcp.leases.mac')" name="mac_address">
          <TInput v-model="formData.mac_address" />
        </TFormItem>
        <TFormItem :label="t('dhcp.leases.hostname')" name="hostname">
          <TInput v-model="formData.hostname" />
        </TFormItem>
        <TFormItem :label="t('dhcp.leases.scope')" name="scope_id">
          <TSelect v-model="formData.scope_id" :options="scopeOptions" filterable />
        </TFormItem>
        <TFormItem :label="t('common.descriptions')" name="description">
          <TTextarea v-model="formData.description" :autosize="{ minRows: 2, maxRows: 4 }" />
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
  padding: 16px 20px;
}
.td-page-header h2 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
}
</style>
