<script setup lang="ts">
import { namedDeleteBody } from '@/utils/confirm';
import { computed, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { DialogPlugin, Link as TLink, MessagePlugin, Space as TSpace, Switch as TSwitch, Tag as TTag } from 'tdesign-vue-next';
import type { FormInstanceFunctions, PrimaryTableCol } from 'tdesign-vue-next';
import RemoteTable from '@/components/RemoteTable.vue';
import { usePermission } from '@/composables/usePermission';
import {
  createConditionalForwarder,
  createDNSForwarder,
  deleteConditionalForwarder,
  deleteDNSForwarder,
  listConditionalForwarders,
  listDNSForwarders,
  updateDNSForwarder,
  type ConditionalForwarder,
  type DNSForwarder
} from '@/service/api/goddi/dns';

defineOptions({ name: 'DNSForwardersPage' });

const { t } = useI18n();
const perm = usePermission();

const fwdTable = ref<{ reload: () => Promise<void> } | null>(null);
const condTable = ref<{ reload: () => Promise<void> } | null>(null);
const fwdFormRef = ref<FormInstanceFunctions | null>(null);
const condFormRef = ref<FormInstanceFunctions | null>(null);
const showFwd = ref(false);
const showCond = ref(false);
const submitting = ref(false);
const editing = ref<DNSForwarder | null>(null);
const allForwarders = ref<DNSForwarder[]>([]);
const togglingId = ref<string | null>(null);

const protocolOptions = [
  { label: 'UDP', value: 'udp' },
  { label: 'TCP', value: 'tcp' },
  { label: 'DoT', value: 'dot' },
  { label: 'DoH', value: 'doh' },
  { label: 'DoQ', value: 'doq' }
];

const addressPlaceholders: Record<string, string> = {
  udp: '8.8.8.8:53',
  tcp: '8.8.8.8:53',
  dot: 'dns.quad9.net:853',
  doh: 'https://dns.quad9.net/dns-query',
  doq: 'dns.adguard-dns.com:853'
};

const fwdForm = reactive({ name: '', protocol: 'udp', address: '', enabled: true, priority: 0 });
const condForm = reactive<{ domain: string; forwarder_ids: string[]; enabled: boolean }>({
  domain: '',
  forwarder_ids: [],
  enabled: true
});

const addressPlaceholder = computed(() => addressPlaceholders[fwdForm.protocol] ?? '8.8.8.8:53');
const forwarderOptions = computed(() =>
  allForwarders.value.map(f => ({ label: `${f.name} (${f.address})`, value: f.id }))
);

const fwdRules = {
  name: [{ required: true, message: t('common.name') + t('common.required'), type: 'error' }],
  address: [{ required: true, message: t('dns.forwarders.address') + t('common.required'), type: 'error' }]
};
const condRules = {
  domain: [{ required: true, message: t('dns.forwarders.domain') + t('common.required'), type: 'error' }],
  forwarder_ids: [{ required: true, message: t('dns.forwarders.title') + t('common.required'), type: 'error' }]
};

const fwdColumns: PrimaryTableCol[] = [
  { colKey: 'name', title: t('common.name') },
  {
    colKey: 'protocol',
    title: t('dns.forwarders.protocol'),
    width: 110,
    cell: (h, { row }) => h(TTag, { size: 'small', variant: 'light' }, () => String((row as DNSForwarder).protocol).toUpperCase())
  },
  { colKey: 'address', title: t('dns.forwarders.address'), ellipsis: true },
  { colKey: 'priority', title: t('common.priority'), width: 90 },
  {
    colKey: 'enabled',
    title: t('common.enabled'),
    width: 90,
    cell: (h, { row }) => {
      const r = row as DNSForwarder;
      return h(TSwitch, {
        size: 'small',
        value: Boolean(r.enabled),
        disabled: !perm.canWrite('dns'),
        loading: togglingId.value === r.id,
        onChange: (value: unknown) => void toggleEnabled(r, Boolean(value))
      });
    }
  },
  {
    colKey: 'op',
    title: t('common.actions'),
    width: 150,
    cell: (h, { row }) => {
      const r = row as DNSForwarder;
      return h(TSpace, { size: 'small' }, () => [
        h(TLink, { theme: 'primary', disabled: !perm.canWrite('dns'), onClick: () => openEdit(r) }, () => t('common.edit')),
        h(TLink, { theme: 'danger', disabled: !perm.canDelete('dns'), onClick: () => confirmDeleteFwd(r.id) }, () => t('common.delete'))
      ]);
    }
  }
];

const condColumns: PrimaryTableCol[] = [
  { colKey: 'domain', title: t('dns.forwarders.domain'), ellipsis: true },
  {
    colKey: 'enabled',
    title: t('common.enabled'),
    width: 90,
    cell: (h, { row }) => h(TSwitch, { size: 'small', disabled: true, value: Boolean((row as ConditionalForwarder).enabled) })
  },
  {
    colKey: 'op',
    title: t('common.actions'),
    width: 100,
    cell: (h, { row }) =>
      h(
        TLink,
        { theme: 'danger', disabled: !perm.canDelete('dns'), onClick: () => confirmDeleteCond((row as ConditionalForwarder).id) },
        () => t('common.delete')
      )
  }
];

function msgError(err: unknown) {
  void MessagePlugin.error(err instanceof Error ? err.message : t('common.failed'));
}

/** Wraps getList into RemoteTable's contract and keeps a local copy for the select. */
async function fetchForwarders(params: Record<string, unknown>) {
  const result = await listDNSForwarders(params);
  allForwarders.value = result.data as DNSForwarder[];
  return result;
}

function openCreate() {
  if (!perm.canWrite('dns')) return;
  editing.value = null;
  Object.assign(fwdForm, { name: '', protocol: 'udp', address: '', enabled: true, priority: 0 });
  showFwd.value = true;
}

function openEdit(row: DNSForwarder) {
  if (!perm.canWrite('dns')) return;
  editing.value = row;
  Object.assign(fwdForm, {
    name: row.name,
    protocol: row.protocol,
    address: row.address,
    enabled: row.enabled,
    priority: row.priority
  });
  showFwd.value = true;
}

async function toggleEnabled(row: DNSForwarder, value: boolean) {
  if (!perm.canWrite('dns')) return;
  togglingId.value = row.id;
  try {
    await updateDNSForwarder(row.id, {
      name: row.name,
      protocol: row.protocol,
      address: row.address,
      priority: row.priority,
      enabled: value
    });
    void MessagePlugin.success(t('common.updateSuccess'));
    await fwdTable.value?.reload();
  } catch (err) {
    msgError(err);
  } finally {
    togglingId.value = null;
  }
}

async function handleFwdSubmit() {
  if (!perm.canWrite('dns')) return;
  const valid = await fwdFormRef.value?.validate();
  if (valid !== true) return;

  submitting.value = true;
  try {
    if (editing.value) await updateDNSForwarder(editing.value.id, fwdForm);
    else await createDNSForwarder(fwdForm);
    void MessagePlugin.success(t('common.success'));
    showFwd.value = false;
    await fwdTable.value?.reload();
  } catch (err) {
    msgError(err);
  } finally {
    submitting.value = false;
  }
}

function confirmDeleteFwd(id: string) {
  if (!perm.canDelete('dns')) return;
  const dlg = DialogPlugin.confirm({
    header: t('common.delete'),
    body: namedDeleteBody(id),
    theme: 'danger',
    onConfirm: async () => {
      try {
        await deleteDNSForwarder(id);
        void MessagePlugin.success(t('common.deleteSuccess'));
        await fwdTable.value?.reload();
      } catch (err) {
        msgError(err);
      }
      dlg.hide();
    }
  });
}

function openCond() {
  if (!perm.canWrite('dns')) return;
  Object.assign(condForm, { domain: '', forwarder_ids: [], enabled: true });
  showCond.value = true;
}

async function handleCondSubmit() {
  if (!perm.canWrite('dns')) return;
  const valid = await condFormRef.value?.validate();
  if (valid !== true) return;

  submitting.value = true;
  try {
    await createConditionalForwarder(condForm);
    void MessagePlugin.success(t('common.createSuccess'));
    showCond.value = false;
    await condTable.value?.reload();
  } catch (err) {
    msgError(err);
  } finally {
    submitting.value = false;
  }
}

function confirmDeleteCond(id: string) {
  if (!perm.canDelete('dns')) return;
  const dlg = DialogPlugin.confirm({
    header: t('common.delete'),
    body: namedDeleteBody(id),
    theme: 'danger',
    onConfirm: async () => {
      try {
        await deleteConditionalForwarder(id);
        void MessagePlugin.success(t('common.deleteSuccess'));
        await condTable.value?.reload();
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
        <h2>{{ t('dns.forwarders.title') }}</h2>
      </div>
      <TButton v-if="perm.canWrite('dns')" theme="primary" @click="openCreate">
        <template #icon><TIcon name="add" /></template>
        {{ t('dns.forwarders.createForwarder') }}
      </TButton>
    </TCard>

    <TTabs default-value="forwarders">
      <TTabPanel value="forwarders" :label="t('dns.forwarders.title')">
        <RemoteTable ref="fwdTable" :columns="fwdColumns" :fetcher="fetchForwarders" :show-pagination="false" />
      </TTabPanel>
      <TTabPanel value="conditional" :label="t('dns.forwarders.conditionalForwarders')">
        <TCard :bordered="false" class="inner-actions">
          <TButton v-if="perm.canWrite('dns')" theme="primary" size="small" @click="openCond">
            {{ t('dns.forwarders.createConditional') }}
          </TButton>
        </TCard>
        <RemoteTable ref="condTable" :columns="condColumns" :fetcher="listConditionalForwarders" :show-pagination="false" />
      </TTabPanel>
    </TTabs>

    <TDialog
      v-model:visible="showFwd"
      :header="editing ? t('common.edit') : t('dns.forwarders.createForwarder')"
      :confirm-btn="{ loading: submitting, content: t('common.save') }"
      :cancel-btn="t('common.cancel')"
      width="500px"
      @confirm="handleFwdSubmit"
    >
      <TForm ref="fwdFormRef" :data="fwdForm" :rules="fwdRules" label-align="top">
        <TFormItem :label="t('common.name')" name="name">
          <TInput v-model="fwdForm.name" />
        </TFormItem>
        <TFormItem :label="t('dns.forwarders.protocol')" name="protocol">
          <TSelect v-model="fwdForm.protocol" :options="protocolOptions" />
        </TFormItem>
        <TFormItem :label="t('dns.forwarders.address')" name="address">
          <TInput v-model="fwdForm.address" :placeholder="addressPlaceholder" />
        </TFormItem>
        <TFormItem :label="t('common.enabled')" name="enabled">
          <TSwitch v-model="fwdForm.enabled" />
        </TFormItem>
        <TFormItem :label="t('common.priority')" name="priority">
          <TInputNumber v-model="fwdForm.priority" :min="0" />
        </TFormItem>
      </TForm>
    </TDialog>

    <TDialog
      v-model:visible="showCond"
      :header="t('dns.forwarders.createConditional')"
      :confirm-btn="{ loading: submitting, content: t('common.save') }"
      :cancel-btn="t('common.cancel')"
      width="500px"
      @confirm="handleCondSubmit"
    >
      <TForm ref="condFormRef" :data="condForm" :rules="condRules" label-align="top">
        <TFormItem :label="t('dns.forwarders.domain')" name="domain">
          <TInput v-model="condForm.domain" :placeholder="t('common.queryExample')" />
        </TFormItem>
        <TFormItem :label="t('dns.forwarders.title')" name="forwarder_ids">
          <TSelect v-model="condForm.forwarder_ids" :options="forwarderOptions" multiple filterable :min-collapsed-num="2" />
        </TFormItem>
        <TFormItem :label="t('common.enabled')" name="enabled">
          <TSwitch v-model="condForm.enabled" />
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
.inner-actions :deep(.t-card__body) {
  padding: 12px 16px;
}
</style>