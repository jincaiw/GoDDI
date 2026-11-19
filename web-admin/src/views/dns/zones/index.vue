<script setup lang="ts">
import { computed, reactive, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { Link as TLink, MessagePlugin, Space as TSpace, Switch as TSwitch, Tag as TTag } from 'tdesign-vue-next';
import type { FormInstanceFunctions, PrimaryTableCol } from 'tdesign-vue-next';
import RemoteTable from '@/components/RemoteTable.vue';
import { usePersistedRef } from '@/composables/usePersistedRef';
import { usePermission } from '@/composables/usePermission';
import {
  batchDeleteDNSZones,
  cloneDNSZone,
  convertDNSZone,
  createDNSZone,
  deleteDNSZone,
  listDNSZones,
  updateDNSZone,
  type CreateDNSZoneRequest,
  type DNSZone
} from '@/service/api/goddi/dns';

defineOptions({ name: 'DNSZonesPage' });

const router = useRouter();
const { t } = useI18n();
const perm = usePermission();

const tableRef = ref<{ reload: () => Promise<void> } | null>(null);
const formRef = ref<FormInstanceFunctions | null>(null);
const showCreate = ref(false);
const showClone = ref(false);
const showConvert = ref(false);
const submitting = ref(false);
const batchDeleting = ref(false);
const editing = ref<DNSZone | null>(null);
const checkedKeys = ref<Array<string | number>>([]);

const searchQuery = usePersistedRef('dns_zones:search', '');
const filterType = ref<string | undefined>(undefined);
const activeTab = ref<'authoritative' | 'allowed' | 'blocked'>('authoritative');

const cloneSource = ref<DNSZone | null>(null);
const cloneName = ref('');
const convertSource = ref<DNSZone | null>(null);
const convertTarget = ref('primary');

const typeOptions = [
  { label: 'Primary', value: 'primary' },
  { label: 'Secondary', value: 'secondary' },
  { label: 'Stub', value: 'stub' },
  { label: 'Forward', value: 'forward' }
];
const zoneTypeOptions = computed(() => [
  ...typeOptions,
  { label: 'Catalog', value: 'catalog' },
  { label: t('dns.zones.typeAllowed'), value: 'allowed' },
  { label: t('dns.zones.typeBlocked'), value: 'blocked' }
]);
// Special (allowed/blocked) zones cannot be converted; the backend enforces it.
const convertTargetOptions = computed(() => typeOptions.filter(o => o.value !== convertSource.value?.type));

const formData = reactive<CreateDNSZoneRequest & { enabled: boolean }>({
  name: '',
  type: 'primary',
  default_ttl: 3600,
  soa_mname: '',
  soa_rname: '',
  refresh: 3600,
  retry: 600,
  expire: 604800,
  minimum: 86400,
  enabled: true
});
const isSpecialType = computed(() => formData.type === 'allowed' || formData.type === 'blocked');

// A DNS zone name must be a syntactically valid domain (optionally ending in
// a dot). Checking this on the client stops obviously invalid input from
// reaching the API, where the error surfaces as a generic 400.
const ZONE_NAME_PATTERN = /^([a-z0-9_]([a-z0-9_-]{0,61}[a-z0-9_])?\.)+[a-z]{2,63}\.?$/i;

const formRules = {
  name: [
    { required: true, message: t('dns.zones.zoneName') + ' required', type: 'error' },
    {
      validator: (value: string) => ZONE_NAME_PATTERN.test(String(value || '').trim()),
      message: t('dns.zones.zoneNameInvalid'),
      type: 'error'
    }
  ]
};

const params = computed<Record<string, unknown>>(() => {
  const p: Record<string, unknown> = {};
  if (searchQuery.value) p.name = searchQuery.value;
  p.type = activeTab.value === 'authoritative' ? filterType.value || 'authoritative' : activeTab.value;
  return p;
});

function handleTabChange(tab: string | number) {
  activeTab.value = String(tab) as 'authoritative' | 'allowed' | 'blocked';
  filterType.value = undefined;
  checkedKeys.value = [];
}

function msgError(err: unknown) {
  void MessagePlugin.error(err instanceof Error ? err.message : t('common.failed'));
}

// Zone type values are raw backend enums (primary/secondary/...). Resolve them
// through the locale table so the table never renders untranslated enums;
// fall back to the raw value when a new type appears before its translation.
function typeLabel(value: string): string {
  const key = `dns.zones.typeValues.${String(value).toLowerCase()}`;
  const out = t(key);
  return out === key ? value : out;
}

const columns: PrimaryTableCol[] = [
  { colKey: 'name', title: t('dns.zones.zoneName'), minWidth: 180 },
  {
    colKey: 'type',
    title: t('dns.zones.zoneType'),
    width: 110,
    cell: (h, { row }) => {
      const r = row as DNSZone;
      const theme = r.type === 'blocked' ? 'danger' : r.type === 'allowed' || r.type === 'primary' ? 'success' : 'primary';
      return h(TTag, { size: 'small', theme, variant: 'light' }, () => typeLabel(r.type));
    }
  },
  { colKey: 'records_count', title: t('dns.zones.records'), width: 90 },
  {
    colKey: 'dnssec_enabled',
    title: t('dns.zones.dnssec'),
    width: 90,
    cell: (h, { row }) =>
      h(TTag, { size: 'small', theme: (row as DNSZone).dnssec_enabled ? 'success' : 'default', variant: 'light' }, () =>
        (row as DNSZone).dnssec_enabled ? 'ON' : 'OFF'
      )
  },
  {
    colKey: 'enabled',
    title: t('common.enabled'),
    width: 90,
    cell: (h, { row }) => {
      const r = row as DNSZone;
      return h(TSwitch, {
        size: 'small',
        value: Boolean(r.enabled),
        disabled: !perm.canWrite('dns'),
        onChange: (value: unknown) => void toggleEnabled(r, Boolean(value))
      });
    }
  },
  {
    colKey: 'op',
    title: t('common.actions'),
    width: 300,
    cell: (h, { row }) => {
      const r = row as DNSZone;
      const special = r.type === 'allowed' || r.type === 'blocked';
      return h(TSpace, { size: 'small' }, () => [
        ...(special
          ? []
          : [
              h(TLink, { theme: 'primary', onClick: () => void router.push(`/dns/zone-detail/${r.id}`) }, () => t('common.edit')),
              h(TLink, { theme: 'primary', disabled: !perm.canWrite('dns'), onClick: () => openClone(r) }, () => t('dns.zones.clone')),
              h(TLink, { theme: 'primary', disabled: !perm.canWrite('dns'), onClick: () => openConvert(r) }, () => t('dns.zones.convert'))
            ]),
        h(
          TLink,
          { theme: 'danger', disabled: !perm.canDelete('dns'), onClick: () => confirmDelete(r.id) },
          () => t('common.delete')
        )
      ]);
    }
  }
];

function openCreate() {
  if (!perm.canWrite('dns')) return;
  editing.value = null;
  const presetType = activeTab.value === 'allowed' ? 'allowed' : activeTab.value === 'blocked' ? 'blocked' : 'primary';
  Object.assign(formData, {
    name: '',
    type: presetType,
    default_ttl: 3600,
    soa_mname: '',
    soa_rname: '',
    refresh: 3600,
    retry: 600,
    expire: 604800,
    minimum: 86400,
    enabled: true
  });
  showCreate.value = true;
}

function openClone(zone: DNSZone) {
  if (!perm.canWrite('dns')) return;
  cloneSource.value = zone;
  cloneName.value = `${zone.name}-copy`;
  showClone.value = true;
}

function openConvert(zone: DNSZone) {
  if (!perm.canWrite('dns')) return;
  convertSource.value = zone;
  convertTarget.value = zone.type === 'primary' ? 'secondary' : 'primary';
  showConvert.value = true;
}

async function toggleEnabled(zone: DNSZone, value: boolean) {
  if (!perm.canWrite('dns')) return;
  try {
    await updateDNSZone(zone.id, { enabled: value });
    await tableRef.value?.reload();
  } catch (err) {
    msgError(err);
  }
}

async function handleSubmit() {
  if (!perm.canWrite('dns')) return;
  const valid = await formRef.value?.validate();
  if (valid !== true) return;

  submitting.value = true;
  try {
    if (editing.value) {
      await updateDNSZone(editing.value.id, formData);
      void MessagePlugin.success(t('common.updateSuccess'));
    } else {
      await createDNSZone(formData);
      void MessagePlugin.success(t('common.createSuccess'));
    }
    showCreate.value = false;
    editing.value = null;
    await tableRef.value?.reload();
  } catch (err) {
    msgError(err);
  } finally {
    submitting.value = false;
  }
}

async function handleClone() {
  if (!perm.canWrite('dns')) return;
  if (!cloneSource.value || !cloneName.value.trim()) {
    void MessagePlugin.warning(t('dns.zones.newNameRequired'));
    return;
  }
  submitting.value = true;
  try {
    await cloneDNSZone(cloneSource.value.id, cloneName.value.trim());
    void MessagePlugin.success(t('common.createSuccess'));
    showClone.value = false;
    await tableRef.value?.reload();
  } catch (err) {
    msgError(err);
  } finally {
    submitting.value = false;
  }
}

async function handleConvert() {
  if (!perm.canWrite('dns') || !convertSource.value || !convertTarget.value) return;
  submitting.value = true;
  try {
    await convertDNSZone(convertSource.value.id, convertTarget.value);
    void MessagePlugin.success(t('common.updateSuccess'));
    showConvert.value = false;
    await tableRef.value?.reload();
  } catch (err) {
    msgError(err);
  } finally {
    submitting.value = false;
  }
}

// The delete confirmation is a component-level dialog rather than
// DialogPlugin.confirm: the plugin is mounted outside the app context, so its
// body text resolved to an empty string and the confirm button fell back to
// the primary theme instead of danger.
const pendingDeleteId = ref<string | null>(null);
const deleteDialogVisible = computed({
  get: () => pendingDeleteId.value !== null,
  set: (value: boolean) => {
    if (!value) pendingDeleteId.value = null;
  }
});

function confirmDelete(id: string) {
  if (!perm.canDelete('dns')) return;
  pendingDeleteId.value = id;
}

async function handleDeleteConfirm() {
  const id = pendingDeleteId.value;
  if (!id) return;
  try {
    await deleteDNSZone(id);
    void MessagePlugin.success(t('common.deleteSuccess'));
    pendingDeleteId.value = null;
    await tableRef.value?.reload();
  } catch (err) {
    msgError(err);
  }
}

const batchDeleteVisible = ref(false);

function confirmBatchDelete() {
  if (!perm.canDelete('dns') || checkedKeys.value.length === 0) return;
  batchDeleteVisible.value = true;
}

async function handleBatchDeleteConfirm() {
  batchDeleting.value = true;
  try {
    const result = await batchDeleteDNSZones(checkedKeys.value.map(k => String(k)));
    void MessagePlugin.success(t('dns.zones.batchDeleteDone', { n: result.deleted }));
    checkedKeys.value = [];
    batchDeleteVisible.value = false;
    await tableRef.value?.reload();
  } catch (err) {
    msgError(err);
  } finally {
    batchDeleting.value = false;
  }
}
</script>

<template>
  <div class="td-page">
    <TCard :bordered="false" class="td-page-header">
      <h2>{{ t('dns.zones.title') }}</h2>
      <TButton v-if="perm.canWrite('dns')" theme="primary" @click="openCreate">
        <template #icon><TIcon name="add" /></template>
        {{ t('dns.zones.createZone') }}
      </TButton>
    </TCard>

    <TCard :bordered="false" class="filter-bar">
      <TTabs :value="activeTab" @change="handleTabChange">
        <TTabPanel value="authoritative" :label="t('dns.zones.tabAuthoritative')" />
        <TTabPanel value="allowed" :label="t('dns.zones.tabAllowed')" />
        <TTabPanel value="blocked" :label="t('dns.zones.tabBlocked')" />
      </TTabs>
      <TSpace size="small" align="center" class="filter-controls">
        <TInput
          v-model="searchQuery"
          clearable
          :placeholder="t('common.search')"
          style="width: 220px"
          @enter="tableRef?.reload()"
          @clear="tableRef?.reload()"
        >
          <template #prefix-icon><TIcon name="search" /></template>
        </TInput>
        <TSelect
          v-if="activeTab === 'authoritative'"
          v-model="filterType"
          :options="typeOptions"
          clearable
          :placeholder="t('dns.zones.zoneType')"
          style="width: 160px"
        />
        <TButton variant="outline" @click="tableRef?.reload()">{{ t('common.refresh') }}</TButton>
        <template v-if="checkedKeys.length > 0">
          <span class="selected-count">{{ t('dns.zones.selectedCount', { n: checkedKeys.length }) }}</span>
          <TButton
            v-if="perm.canDelete('dns')"
            theme="danger"
            variant="outline"
            :loading="batchDeleting"
            @click="confirmBatchDelete"
          >
            {{ t('dns.zones.batchDelete') }}
          </TButton>
        </template>
      </TSpace>
    </TCard>

    <RemoteTable
      ref="tableRef"
      v-model:selected-keys="checkedKeys"
      :columns="columns"
      :fetcher="listDNSZones"
      :params="params"
      selectable
    />

    <TDialog
      v-model:visible="showCreate"
      :header="editing ? t('dns.zones.editZone') : t('dns.zones.createZone')"
      :confirm-btn="{ loading: submitting, content: t('common.save') }"
      :cancel-btn="t('common.cancel')"
      width="640px"
      @confirm="handleSubmit"
    >
      <TForm ref="formRef" :data="formData" :rules="formRules" label-align="top">
        <TFormItem :label="t('dns.zones.zoneName')" name="name">
          <TInput v-model="formData.name" :disabled="!!editing" />
        </TFormItem>
        <TFormItem :label="t('dns.zones.zoneType')" name="type">
          <TSelect v-model="formData.type" :options="zoneTypeOptions" :disabled="!!editing" />
        </TFormItem>
        <template v-if="!isSpecialType">
          <TFormItem :label="t('dns.zones.ttl')" name="default_ttl">
            <TInputNumber v-model="formData.default_ttl" :min="60" />
          </TFormItem>
          <TFormItem :label="t('dns.zones.primaryNs')" name="soa_mname">
            <TInput v-model="formData.soa_mname" />
          </TFormItem>
          <TFormItem :label="t('dns.zones.adminEmail')" name="soa_rname">
            <TInput v-model="formData.soa_rname" />
          </TFormItem>
          <TFormItem :label="t('dns.zones.refresh')" name="refresh">
            <TInputNumber v-model="formData.refresh" :min="0" />
          </TFormItem>
          <TFormItem :label="t('dns.zones.retry')" name="retry">
            <TInputNumber v-model="formData.retry" :min="0" />
          </TFormItem>
          <TFormItem :label="t('dns.zones.expire')" name="expire">
            <TInputNumber v-model="formData.expire" :min="0" />
          </TFormItem>
          <TFormItem :label="t('dns.zones.minimum')" name="minimum">
            <TInputNumber v-model="formData.minimum" :min="0" />
          </TFormItem>
        </template>
        <TAlert
          v-if="activeTab !== 'authoritative' || isSpecialType"
          theme="info"
          :message="t('dns.zones.specialHintText')"
        />
        <TFormItem :label="t('common.enabled')" name="enabled">
          <TSwitch v-model="formData.enabled" />
        </TFormItem>
      </TForm>
    </TDialog>

    <TDialog
      v-model:visible="showClone"
      :header="t('dns.zones.cloneTitle')"
      :confirm-btn="{ loading: submitting, content: t('common.save') }"
      :cancel-btn="t('common.cancel')"
      width="480px"
      @confirm="handleClone"
    >
      <TForm label-align="top">
        <TFormItem :label="t('dns.zones.zoneName')">
          <TInput :value="cloneSource?.name" disabled />
        </TFormItem>
        <TFormItem :label="t('dns.zones.newName')">
          <TInput v-model="cloneName" :placeholder="cloneSource ? `${cloneSource.name}-copy` : ''" />
        </TFormItem>
      </TForm>
    </TDialog>

    <TDialog
      v-model:visible="showConvert"
      :header="t('dns.zones.convertTitle')"
      :confirm-btn="{ loading: submitting, content: t('common.save') }"
      :cancel-btn="t('common.cancel')"
      width="480px"
      @confirm="handleConvert"
    >
      <TForm label-align="top">
        <TFormItem :label="t('dns.zones.zoneName')">
          <TInput :value="convertSource?.name" disabled />
        </TFormItem>
        <TFormItem :label="t('dns.zones.convertTarget')">
          <TSelect v-model="convertTarget" :options="convertTargetOptions" />
        </TFormItem>
        <TAlert theme="info" :message="t('dns.zones.convertHint')" />
      </TForm>
    </TDialog>

    <TDialog
      v-model:visible="deleteDialogVisible"
      :header="t('common.delete')"
      theme="danger"
      :confirm-btn="{ content: t('common.confirm'), theme: 'danger' }"
      :body="t('common.deleteConfirm')"
      @confirm="handleDeleteConfirm"
    />

    <TDialog
      v-model:visible="batchDeleteVisible"
      :header="t('dns.zones.batchDelete')"
      theme="danger"
      :confirm-btn="{ content: t('common.confirm'), theme: 'danger', loading: batchDeleting }"
      :body="t('dns.zones.batchDeleteConfirm', { n: checkedKeys.length })"
      @confirm="handleBatchDeleteConfirm"
    />
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
.filter-bar :deep(.t-card__body) {
  padding: 12px 16px;
}
.filter-controls {
  margin-top: 12px;
}
.selected-count {
  font-size: 13px;
  color: #6b7280;
}
</style>