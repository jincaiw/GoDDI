<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { useRouter, useRoute } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { DialogPlugin, Link as TLink, MessagePlugin, Space as TSpace, Switch as TSwitch, Tag as TTag } from 'tdesign-vue-next';
import type { FormInstanceFunctions, PrimaryTableCol } from 'tdesign-vue-next';
import RemoteTable from '@/components/RemoteTable.vue';
import { usePermission } from '@/composables/usePermission';
import { ApiError } from '@/service/api/goddi/client';
import {
  createDNSRecord,
  deleteDNSRecord,
  deleteDNSSECKey,
  disableDNSSEC,
  enableDNSSEC,
  exportZoneFile,
  generateDNSSECKey,
  getCatalogMembers,
  getDNSSECStatus,
  getDNSZone,
  getNSEC3Params,
  getZoneDSRecords,
  getZoneHistory,
  getZonePermissions,
  importZoneFile,
  listDNSRecords,
  listDNSZones,
  promoteDNSSECStandbyKeys,
  rotateDNSSECKeys,
  setNSEC3Params,
  setZonePermissions,
  syncSecondaryZone,
  toggleDNSSECKey,
  updateDNSRecord,
  updateDNSZone,
  type CreateDNSRecordRequest,
  type DNSRecord,
  type DNSSECKey,
  type DNSSECStatus,
  type DNSZone,
  type DSInfo,
  type ZoneACL,
  type ZoneChangeEntry,
  type ZoneImportPreview,
  type ZonePermission
} from '@/service/api/goddi/dns';

defineOptions({ name: 'DNSZoneDetailPage' });

const router = useRouter();
const route = useRoute();
const { t } = useI18n();
const perm = usePermission();
const zoneId = route.params.id as string;

function msgError(err: unknown) {
  void MessagePlugin.error(err instanceof Error ? err.message : t('common.failed'));
}

// --- Zone + tabs ---
const zone = ref<DNSZone | null>(null);
const activeTab = ref('records');

// Declared before loadZone(): that function repopulates the ACL editor from
// the zone row as soon as the page loads.
const aclForm = reactive<ZoneACL & { query_access?: string }>({
  query_access: '',
  allow_query: [],
  allow_transfer: [],
  allow_update: [],
  notify: []
});

async function loadZone() {
  try {
    zone.value = await getDNSZone(zoneId);
    const acl = zone.value.acl;
    aclForm.query_access = acl?.query_access ?? '';
    aclForm.allow_query = [...(acl?.allow_query ?? [])];
    aclForm.allow_transfer = [...(acl?.allow_transfer ?? [])];
    aclForm.allow_update = [...(acl?.allow_update ?? [])];
    aclForm.notify = [...(acl?.notify ?? [])];
    await loadCatalogData();
  } catch (err) {
    msgError(err);
  }
}

// --- Records tab ---
const recordsTable = ref<{ reload: () => Promise<void> } | null>(null);
const recordSearch = ref('');
const recordTypeFilter = ref<string | undefined>(undefined);
const showAddRecord = ref(false);
const recordSubmitting = ref(false);
const editingRecord = ref<DNSRecord | null>(null);
const recordFormRef = ref<FormInstanceFunctions | null>(null);

const recordTypeOptions = [
  'A', 'AAAA', 'CNAME', 'DNAME', 'MX', 'NS', 'PTR', 'SOA', 'SRV', 'TXT', 'CAA',
  'TLSA', 'SVCB', 'HTTPS', 'URI', 'SSHFP', 'NAPTR', 'DS', 'DNSKEY', 'HINFO', 'LOC', 'RP', 'SPF', 'APL'
].map(type => ({ label: type, value: type }));

const recordForm = reactive<CreateDNSRecordRequest & { enabled: boolean; expiry_ttl?: number }>({
  name: '',
  type: 'A',
  value: '',
  ttl: 3600,
  priority: undefined,
  expiry_ttl: undefined,
  enabled: true
});

const recordRules = {
  name: [{ required: true, message: t('dns.records.recordName') + ' required', type: 'error' }],
  value: [{ required: true, message: t('dns.records.recordValue') + ' required', type: 'error' }]
};

const recordParams = computed<Record<string, unknown>>(() => {
  const p: Record<string, unknown> = {};
  if (recordSearch.value) p.name = recordSearch.value;
  if (recordTypeFilter.value) p.type = recordTypeFilter.value;
  return p;
});

async function fetchRecords(params: Record<string, unknown>) {
  return listDNSRecords(zoneId, params);
}

const recordColumns: PrimaryTableCol[] = [
  { colKey: 'name', title: t('dns.records.recordName'), ellipsis: true },
  {
    colKey: 'type',
    title: t('dns.records.recordType'),
    width: 90,
    cell: (h, { row }) => h(TTag, { size: 'small', variant: 'light' }, () => String((row as DNSRecord).type))
  },
  { colKey: 'value', title: t('dns.records.recordValue'), ellipsis: true },
  { colKey: 'ttl', title: t('dns.zones.ttl'), width: 80 },
  {
    colKey: 'enabled',
    title: t('common.enabled'),
    width: 90,
    cell: (h, { row }) => {
      const r = row as DNSRecord;
      return h(TSwitch, {
        size: 'small',
        value: Boolean(r.enabled),
        disabled: !perm.canWrite('dns'),
        onChange: (value: unknown) => void toggleRecordEnabled(r, Boolean(value))
      });
    }
  },
  {
    colKey: 'op',
    title: t('common.actions'),
    width: 150,
    cell: (h, { row }) => {
      const r = row as DNSRecord;
      return h(TSpace, { size: 'small' }, () => [
        h(TLink, { theme: 'primary', disabled: !perm.canWrite('dns'), onClick: () => editRecord(r) }, () => t('common.edit')),
        h(TLink, { theme: 'danger', disabled: !perm.canDelete('dns'), onClick: () => confirmDeleteRecord(r.id) }, () => t('common.delete'))
      ]);
    }
  }
];

function resetRecordForm() {
  Object.assign(recordForm, {
    name: '',
    type: 'A',
    value: '',
    ttl: 3600,
    priority: undefined,
    expiry_ttl: undefined,
    enabled: true
  });
}

function openCreateRecord() {
  if (!perm.canWrite('dns')) return;
  editingRecord.value = null;
  resetRecordForm();
  showAddRecord.value = true;
}

function editRecord(record: DNSRecord) {
  if (!perm.canWrite('dns')) return;
  editingRecord.value = record;
  Object.assign(recordForm, {
    name: record.name,
    type: record.type,
    value: record.value,
    ttl: record.ttl,
    priority: record.priority,
    enabled: record.enabled,
    expiry_ttl: undefined
  });
  showAddRecord.value = true;
}

async function handleRecordSubmit() {
  if (!perm.canWrite('dns')) return;
  const valid = await recordFormRef.value?.validate();
  if (valid !== true) return;
  recordSubmitting.value = true;
  try {
    if (editingRecord.value) {
      await updateDNSRecord(zoneId, editingRecord.value.id, recordForm);
      void MessagePlugin.success(t('common.updateSuccess'));
    } else {
      await createDNSRecord(zoneId, recordForm);
      void MessagePlugin.success(t('common.createSuccess'));
    }
    showAddRecord.value = false;
    editingRecord.value = null;
    resetRecordForm();
    await recordsTable.value?.reload();
  } catch (err) {
    msgError(err);
  } finally {
    recordSubmitting.value = false;
  }
}

function confirmDeleteRecord(id: string) {
  if (!perm.canDelete('dns')) return;
  const dlg = DialogPlugin.confirm({
    header: t('common.delete'),
    body: t('common.deleteConfirm'),
    theme: 'danger',
    onConfirm: async () => {
      try {
        await deleteDNSRecord(zoneId, id);
        void MessagePlugin.success(t('common.deleteSuccess'));
        await recordsTable.value?.reload();
      } catch (err) {
        msgError(err);
      }
      dlg.hide();
    }
  });
}

async function toggleRecordEnabled(record: DNSRecord, value: boolean) {
  if (!perm.canWrite('dns')) return;
  try {
    await updateDNSRecord(zoneId, record.id, { enabled: value });
    await recordsTable.value?.reload();
  } catch (err) {
    msgError(err);
  }
}

// --- Import / export / sync ---
const showImport = ref(false);
const importFormat = ref<'bind' | 'csv'>('bind');
const importContent = ref('');
const importPreview = ref<ZoneImportPreview | null>(null);
const importPreviewLoading = ref(false);
const importApplying = ref(false);
const importFileInput = ref<HTMLInputElement | null>(null);
const importFormatOptions = [
  { label: 'BIND zone file', value: 'bind' },
  { label: 'CSV', value: 'csv' }
];

async function handleExport() {
  try {
    const response = await exportZoneFile(zoneId);
    const url = window.URL.createObjectURL(new Blob([response.data as BlobPart]));
    const link = document.createElement('a');
    link.href = url;
    link.download = `${zone.value?.name || 'zone'}.txt`;
    link.click();
    window.URL.revokeObjectURL(url);
  } catch (err) {
    msgError(err);
  }
}

async function loadImportFile(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  if (!file) return;
  try {
    importContent.value = await file.text();
    importPreview.value = null;
  } catch (err) {
    msgError(err);
  } finally {
    input.value = '';
  }
}

/** A rejected import carries its preview on the error payload. */
function previewFromError(err: unknown): ZoneImportPreview | null {
  if (!(err instanceof ApiError) || !err.payload || typeof err.payload !== 'object') return null;
  return 'conflicts' in err.payload ? (err.payload as ZoneImportPreview) : null;
}

async function previewImport() {
  if (!perm.canWrite('dns')) return;
  importPreviewLoading.value = true;
  try {
    const response = await importZoneFile(zoneId, importContent.value, importFormat.value, true);
    importPreview.value = (response.data as { data?: ZoneImportPreview })?.data ?? null;
  } catch (err) {
    importPreview.value = previewFromError(err);
    msgError(err);
  } finally {
    importPreviewLoading.value = false;
  }
}

async function applyImport() {
  if (!perm.canWrite('dns') || !importPreview.value?.valid) return;
  importApplying.value = true;
  try {
    await importZoneFile(zoneId, importContent.value, importFormat.value);
    void MessagePlugin.success(t('dns.zones.importSuccess'));
    showImport.value = false;
    importContent.value = '';
    importPreview.value = null;
    await loadZone();
    await recordsTable.value?.reload();
  } catch (err) {
    importPreview.value = previewFromError(err);
    msgError(err);
  } finally {
    importApplying.value = false;
  }
}

async function handleSync() {
  if (!perm.canWrite('dns')) return;
  try {
    await syncSecondaryZone(zoneId);
    void MessagePlugin.success(t('common.success'));
    await loadZone();
    await recordsTable.value?.reload();
  } catch (err) {
    msgError(err);
  }
}

// --- Options tab: ACL + catalog ---
const aclSaving = ref(false);
const queryAccessOptions = computed(() => [
  { label: t('dns.zones.queryAccessDefault'), value: '' },
  { label: t('dns.zones.queryAccessAllow'), value: 'allow' },
  { label: t('dns.zones.queryAccessDeny'), value: 'deny' },
  { label: t('dns.zones.queryAccessPrivate'), value: 'allow_only_private_networks' }
]);

async function saveACL() {
  if (!perm.canWrite('dns')) return;
  aclSaving.value = true;
  try {
    // Send the ACL unconditionally: empty lists clear restrictions server-side.
    const acl: ZoneACL = {
      query_access: aclForm.query_access || undefined,
      allow_query: aclForm.allow_query,
      allow_transfer: aclForm.allow_transfer,
      allow_update: aclForm.allow_update,
      notify: aclForm.notify
    };
    await updateDNSZone(zoneId, { acl });
    void MessagePlugin.success(t('common.updateSuccess'));
    await loadZone();
  } catch (err) {
    msgError(err);
  } finally {
    aclSaving.value = false;
  }
}

const catalogMembers = ref<string[]>([]);
const catalogZones = ref<DNSZone[]>([]);
const catalogSaving = ref(false);
const catalogForm = reactive<{ catalog: string }>({ catalog: '-' });
const catalogOptions = computed(() => [
  { label: t('dns.zones.catalogNone'), value: '-' },
  ...catalogZones.value.map(z => ({ label: z.name, value: z.name }))
]);

async function loadCatalogData() {
  if (!zone.value) return;
  catalogForm.catalog = zone.value.catalog || '-';
  if (zone.value.type === 'catalog') {
    try {
      catalogMembers.value = (await getCatalogMembers(zoneId)) ?? [];
    } catch {
      catalogMembers.value = [];
    }
    return;
  }
  try {
    catalogZones.value = (await listDNSZones({ page: 1, page_size: 100, type: 'catalog' })).data;
  } catch {
    catalogZones.value = [];
  }
}

async function saveCatalog() {
  if (!perm.canWrite('dns')) return;
  catalogSaving.value = true;
  try {
    zone.value = await updateDNSZone(zoneId, { catalog: catalogForm.catalog });
    catalogForm.catalog = zone.value.catalog || '-';
    void MessagePlugin.success(t('common.success'));
  } catch (err) {
    msgError(err);
  } finally {
    catalogSaving.value = false;
  }
}

async function toggleZoneEnabled(value: unknown) {
  if (!perm.canWrite('dns') || !zone.value) return;
  try {
    await updateDNSZone(zoneId, { enabled: Boolean(value) });
    await loadZone();
  } catch (err) {
    msgError(err);
  }
}

// --- Permissions tab ---
const permissions = ref<ZonePermission[]>([]);
const permSaving = ref(false);
const permLoaded = ref(false);
const permissionColumns: PrimaryTableCol[] = [
  {
    colKey: 'principal_type',
    title: t('dns.zones.permPrincipalType'),
    width: 120,
    cell: (h, { row }) => {
      const r = row as ZonePermission;
      return h(TTag, { size: 'small', theme: r.principal_type === 'user' ? 'primary' : 'warning', variant: 'light' }, () =>
        r.principal_type === 'user' ? t('dns.zones.permPrincipalUser') : t('dns.zones.permPrincipalGroup')
      );
    }
  },
  { colKey: 'principal_id', title: t('dns.zones.permPrincipalId'), ellipsis: true },
  {
    colKey: 'can_view',
    title: t('dns.zones.permView'),
    width: 90,
    cell: (h, { row }) => h(TSwitch, { size: 'small', value: Boolean((row as ZonePermission).can_view), disabled: !perm.canWrite('dns'), onChange: (v: unknown) => ((row as ZonePermission).can_view = Boolean(v)) })
  },
  {
    colKey: 'can_modify',
    title: t('dns.zones.permModify'),
    width: 90,
    cell: (h, { row }) => h(TSwitch, { size: 'small', value: Boolean((row as ZonePermission).can_modify), disabled: !perm.canWrite('dns'), onChange: (v: unknown) => ((row as ZonePermission).can_modify = Boolean(v)) })
  },
  {
    colKey: 'can_delete',
    title: t('dns.zones.permDelete'),
    width: 90,
    cell: (h, { row }) => h(TSwitch, { size: 'small', value: Boolean((row as ZonePermission).can_delete), disabled: !perm.canWrite('dns'), onChange: (v: unknown) => ((row as ZonePermission).can_delete = Boolean(v)) })
  },
  {
    colKey: 'op',
    title: t('common.actions'),
    width: 90,
    cell: (h, { row }) => {
      const r = row as ZonePermission;
      return h(TLink, { theme: 'danger', disabled: !perm.canWrite('dns'), onClick: () => removePermission(r) }, () => t('common.delete'));
    }
  }
];

async function loadPermissions() {
  try {
    permissions.value = (await getZonePermissions(zoneId)) ?? [];
    permLoaded.value = true;
  } catch (err) {
    msgError(err);
  }
}

function addPermission() {
  if (!perm.canWrite('dns')) return;
  permissions.value.push({ principal_type: 'user', principal_id: '', can_view: true, can_modify: false, can_delete: false });
}

function removePermission(row: ZonePermission) {
  if (!perm.canWrite('dns')) return;
  permissions.value = permissions.value.filter(p => !(p.principal_type === row.principal_type && p.principal_id === row.principal_id));
}

async function savePermissions() {
  if (!perm.canWrite('dns')) return;
  if (permissions.value.find(p => !p.principal_id.trim())) {
    void MessagePlugin.warning(t('dns.zones.permPrincipalId'));
    return;
  }
  permSaving.value = true;
  try {
    await setZonePermissions(zoneId, permissions.value);
    void MessagePlugin.success(t('common.updateSuccess'));
    await loadPermissions();
  } catch (err) {
    msgError(err);
  } finally {
    permSaving.value = false;
  }
}

// --- History tab ---
const historyTable = ref<{ reload: () => Promise<void> } | null>(null);
const historyLoaded = ref(false);
async function fetchHistory(params: Record<string, unknown>) {
  const r = await getZoneHistory(zoneId, params);
  historyLoaded.value = true;
  return r;
}
const historyColumns: PrimaryTableCol[] = [
  { colKey: 'serial', title: 'Serial', width: 120 },
  {
    colKey: 'change_type',
    title: 'Action',
    width: 100,
    cell: (h, { row }) =>
      h(TTag, { size: 'small', theme: (row as ZoneChangeEntry).change_type === 'add' ? 'success' : 'danger', variant: 'light' }, () =>
        String((row as ZoneChangeEntry).change_type)
      )
  },
  { colKey: 'name', title: t('dns.records.recordName'), ellipsis: true },
  { colKey: 'type', title: t('dns.records.recordType'), width: 90 },
  { colKey: 'value', title: t('dns.records.recordValue'), ellipsis: true },
  { colKey: 'ttl', title: t('dns.zones.ttl'), width: 80 },
  { colKey: 'created_at', title: 'Time', width: 180 }
];

// --- DNSSEC tab (experimental) ---
const dnssecStatus = ref<DNSSECStatus | null>(null);
const dnssecLoaded = ref(false);
const dsRecords = ref<DSInfo[]>([]);
const nsec3Form = reactive<{ iterations: number; salt: string; optout: boolean }>({ iterations: 0, salt: '', optout: false });
const nsec3Saving = ref(false);
const showGenerateKey = ref(false);
const generateKeyType = ref<'KSK' | 'ZSK'>('KSK');
const generateKeyAlgorithm = ref('ECDSAP256SHA256');
const dnssecAlgorithmOptions = ['ECDSAP256SHA256', 'ECDSAP384SHA384', 'ED25519', 'RSASHA256', 'RSASHA512'].map(a => ({ label: a, value: a }));

const dnssecKeyColumns: PrimaryTableCol[] = [
  { colKey: 'key_tag', title: 'Key Tag', width: 100 },
  { colKey: 'id', title: 'ID', width: 150, ellipsis: true },
  { colKey: 'algorithm', title: 'Algorithm', width: 150 },
  {
    colKey: 'key_type',
    title: 'Type',
    width: 90,
    cell: (h, { row }) => h(TTag, { size: 'small', theme: (row as DNSSECKey).key_type === 'KSK' ? 'warning' : 'primary', variant: 'light' }, () => String((row as DNSSECKey).key_type))
  },
  {
    colKey: 'enabled',
    title: 'Enabled',
    width: 90,
    cell: (h, { row }) => {
      const r = row as DNSSECKey;
      return h(TSwitch, { size: 'small', value: r.enabled ?? true, disabled: !perm.canWrite('dns'), onChange: (v: unknown) => void handleToggleKey(r, Boolean(v)) });
    }
  },
  { colKey: 'created_at', title: 'Created', width: 180 },
  {
    colKey: 'op',
    title: 'Actions',
    width: 90,
    cell: (h, { row }) => h(TLink, { theme: 'danger', disabled: !perm.canDelete('dns'), onClick: () => void handleDeleteKey(row as DNSSECKey) }, () => t('common.delete'))
  }
];

const dsColumns: PrimaryTableCol[] = [
  { colKey: 'key_tag', title: 'Key Tag', width: 100 },
  { colKey: 'algorithm', title: 'Algorithm', width: 150 },
  { colKey: 'digest_type', title: 'Digest Type', width: 110 },
  { colKey: 'digest', title: 'Digest', ellipsis: true }
];

async function handleDnssecAction(action: 'enable' | 'disable' | 'rotate') {
  if (!perm.canWrite('dns')) return;
  try {
    if (action === 'enable') await enableDNSSEC(zoneId);
    else if (action === 'disable') await disableDNSSEC(zoneId);
    else await rotateDNSSECKeys(zoneId);
    void MessagePlugin.success(t('common.success'));
    await loadZone();
    dnssecLoaded.value = false;
    await loadDnssec();
  } catch (err) {
    msgError(err);
  }
}

async function loadDsRecords() {
  try {
    dsRecords.value = (await getZoneDSRecords(zoneId)) ?? [];
  } catch (err) {
    msgError(err);
  }
}

async function handleToggleKey(key: DNSSECKey, enabled: boolean) {
  if (!perm.canWrite('dns')) return;
  try {
    await toggleDNSSECKey(zoneId, key.id, enabled);
    key.enabled = enabled;
  } catch (err) {
    msgError(err);
  }
}

async function handleDeleteKey(key: DNSSECKey) {
  if (!perm.canDelete('dns')) return;
  try {
    await deleteDNSSECKey(zoneId, key.id);
    void MessagePlugin.success(t('common.success'));
    dnssecLoaded.value = false;
    await loadDnssec();
  } catch (err) {
    msgError(err);
  }
}

function openGenerateKey() {
  if (!perm.canWrite('dns')) return;
  generateKeyType.value = 'KSK';
  generateKeyAlgorithm.value = 'ECDSAP256SHA256';
  showGenerateKey.value = true;
}

async function handleGenerateKey() {
  if (!perm.canWrite('dns')) return;
  try {
    await generateDNSSECKey(zoneId, generateKeyType.value, generateKeyAlgorithm.value);
    showGenerateKey.value = false;
    void MessagePlugin.success(t('common.success'));
    dnssecLoaded.value = false;
    await loadDnssec();
  } catch (err) {
    msgError(err);
  }
}

async function handlePromoteStandby() {
  if (!perm.canWrite('dns')) return;
  try {
    await promoteDNSSECStandbyKeys(zoneId);
    void MessagePlugin.success(t('common.success'));
    dnssecLoaded.value = false;
    await loadDnssec();
  } catch (err) {
    msgError(err);
  }
}

async function loadNsec3() {
  try {
    const p = await getNSEC3Params(zoneId);
    if (p) {
      nsec3Form.iterations = p.iterations;
      nsec3Form.salt = p.salt;
      nsec3Form.optout = p.optout;
    }
  } catch {
    // NSEC3 params are optional.
  }
}

async function saveNsec3() {
  if (!perm.canWrite('dns')) return;
  nsec3Saving.value = true;
  try {
    await setNSEC3Params(zoneId, { ...nsec3Form });
    void MessagePlugin.success(t('common.success'));
  } catch (err) {
    msgError(err);
  } finally {
    nsec3Saving.value = false;
  }
}

async function loadDnssec() {
  if (dnssecLoaded.value) return;
  try {
    dnssecStatus.value = await getDNSSECStatus(zoneId);
  } catch {
    // DNSSEC is experimental; absence of the endpoint must not break the tab.
  }
  dnssecLoaded.value = true;
  await loadNsec3();
}

function handleTabChange(tab: string | number) {
  const value = String(tab);
  activeTab.value = value;
  if (value === 'permissions' && !permLoaded.value) void loadPermissions();
  else if (value === 'history' && !historyLoaded.value) void historyTable.value?.reload();
  else if (value === 'dnssec' && !dnssecLoaded.value) void loadDnssec();
}

onMounted(() => {
  void loadZone();
});
</script>

<template>
  <div class="td-page">
    <TCard :bordered="false" class="td-page-header">
      <div>
        <h2>{{ zone?.name || '' }}</h2>
        <span class="subtitle">{{ t('dns.zones.title') }}</span>
      </div>
      <TSpace size="small">
        <TButton variant="outline" @click="router.push('/dns/zones')">{{ t('common.cancel') }}</TButton>
        <TButton v-if="perm.canWrite('dns')" theme="primary" @click="openCreateRecord">
          <template #icon><TIcon name="add" /></template>
          {{ t('dns.records.createRecord') }}
        </TButton>
        <TButton v-if="perm.canWrite('dns')" variant="outline" @click="showImport = true">{{ t('dns.zones.importZone') }}</TButton>
        <TButton v-if="perm.canRead('dns')" variant="outline" @click="handleExport">{{ t('dns.zones.exportZone') }}</TButton>
        <TButton v-if="perm.canWrite('dns')" variant="outline" :disabled="zone?.type !== 'slave'" @click="handleSync">
          {{ t('dns.zones.syncZone') }}
        </TButton>
      </TSpace>
    </TCard>

    <TCard :bordered="false">
      <TTabs :value="activeTab" @change="handleTabChange">
        <!-- Records -->
        <TTabPanel value="records" :label="t('dns.zones.tabRecords')">
          <TSpace size="small" class="tab-toolbar" align="center">
            <TInput v-model="recordSearch" clearable :placeholder="t('common.search')" style="width: 220px" @enter="recordsTable?.reload()">
              <template #prefix-icon><TIcon name="search" /></template>
            </TInput>
            <TSelect v-model="recordTypeFilter" :options="recordTypeOptions" clearable :placeholder="t('dns.records.recordType')" style="width: 140px" />
            <TButton variant="outline" @click="recordsTable?.reload()">{{ t('common.refresh') }}</TButton>
          </TSpace>
          <RemoteTable ref="recordsTable" :columns="recordColumns" :fetcher="fetchRecords" :params="recordParams" />
        </TTabPanel>

        <!-- Options -->
        <TTabPanel value="options" :label="t('dns.zones.tabOptions')">
          <TDescriptions v-if="zone" :column="4" bordered size="small" class="mb-4">
            <TDescriptionsItem :label="t('dns.zones.zoneType')">{{ zone.type }}</TDescriptionsItem>
            <TDescriptionsItem :label="t('dns.zones.ttl')">{{ zone.default_ttl }}</TDescriptionsItem>
            <TDescriptionsItem :label="t('dns.zones.serial')">{{ zone.serial }}</TDescriptionsItem>
            <TDescriptionsItem :label="t('common.enabled')">
              <TSwitch :value="Boolean(zone.enabled)" :disabled="!perm.canWrite('dns')" @change="toggleZoneEnabled" />
            </TDescriptionsItem>
            <TDescriptionsItem :label="t('dns.zones.primaryNs')">{{ zone.soa_mname || '—' }}</TDescriptionsItem>
            <TDescriptionsItem :label="t('dns.zones.adminEmail')">{{ zone.soa_rname || '—' }}</TDescriptionsItem>
            <TDescriptionsItem :label="t('dns.zones.refresh')">{{ zone.refresh }}</TDescriptionsItem>
            <TDescriptionsItem :label="t('dns.zones.minimum')">{{ zone.minimum }}</TDescriptionsItem>
          </TDescriptions>

          <TCard v-if="zone && zone.type !== 'allowed' && zone.type !== 'blocked'" :bordered="false" :title="t('dns.zones.aclTitle')">
            <TForm label-align="top">
              <TFormItem :label="t('dns.zones.queryAccess')">
                <TSelect v-model="aclForm.query_access" :options="queryAccessOptions" :disabled="!perm.canWrite('dns')" clearable style="width: 320px" />
              </TFormItem>
              <TFormItem :label="t('dns.zones.aclAllowQuery')">
                <TTagInput v-model="aclForm.allow_query" :disabled="!perm.canWrite('dns')" />
              </TFormItem>
              <TFormItem :label="t('dns.zones.aclAllowTransfer')">
                <TTagInput v-model="aclForm.allow_transfer" :disabled="!perm.canWrite('dns')" />
              </TFormItem>
              <TFormItem :label="t('dns.zones.aclAllowUpdate')">
                <TTagInput v-model="aclForm.allow_update" :disabled="!perm.canWrite('dns')" />
              </TFormItem>
              <TFormItem :label="t('dns.zones.aclNotify')">
                <TTagInput v-model="aclForm.notify" :disabled="!perm.canWrite('dns')" />
              </TFormItem>
            </TForm>
            <p class="hint">{{ t('dns.zones.aclHint') }}</p>
            <TButton v-if="perm.canWrite('dns')" theme="primary" size="small" :loading="aclSaving" @click="saveACL">
              {{ t('common.save') }}
            </TButton>
          </TCard>

          <TCard v-if="zone && zone.type === 'catalog'" :bordered="false" :title="t('dns.zones.catalogMembers')" class="mt-4">
            <TSpace v-if="catalogMembers.length" size="small">
              <TTag v-for="m in catalogMembers" :key="m" size="small" theme="primary" variant="light">{{ m }}</TTag>
            </TSpace>
            <TEmpty v-else :description="t('dns.zones.catalogMembersEmpty')" />
          </TCard>

          <TCard
            v-if="zone && !['allowed', 'blocked', 'catalog'].includes(zone.type)"
            :bordered="false"
            :title="t('dns.zones.catalogTitle')"
            class="mt-4"
          >
            <TForm label-align="top">
              <TFormItem :label="t('dns.zones.catalogJoin')">
                <TSelect v-model="catalogForm.catalog" :options="catalogOptions" :disabled="!perm.canWrite('dns')" style="width: 320px" />
              </TFormItem>
            </TForm>
            <p class="hint">{{ t('dns.zones.catalogHint') }}</p>
            <TButton v-if="perm.canWrite('dns')" theme="primary" size="small" :loading="catalogSaving" @click="saveCatalog">
              {{ t('common.save') }}
            </TButton>
          </TCard>
        </TTabPanel>

        <!-- Permissions -->
        <TTabPanel value="permissions" :label="t('dns.zones.tabPermissions')">
          <TAlert theme="info" :message="t('dns.zones.permHint')" class="mb-3" />
          <TSpace size="small" class="tab-toolbar" align="center">
            <TButton v-if="perm.canWrite('dns')" size="small" variant="outline" @click="addPermission">{{ t('dns.zones.permAdd') }}</TButton>
            <TButton v-if="perm.canWrite('dns')" size="small" theme="primary" :loading="permSaving" @click="savePermissions">
              {{ t('common.save') }}
            </TButton>
          </TSpace>
          <TTable :columns="permissionColumns" :data="permissions" row-key="principal_id" stripe hover />
        </TTabPanel>

        <!-- History -->
        <TTabPanel value="history" :label="t('dns.zones.tabHistory')">
          <RemoteTable ref="historyTable" :columns="historyColumns" :fetcher="fetchHistory" :page-size="50" />
        </TTabPanel>

        <!-- DNSSEC (experimental) -->
        <TTabPanel value="dnssec" :label="t('dns.zones.tabDnssec')">
          <TAlert theme="warning" :message="t('dns.zones.dnssecExperimentalHint')" class="mb-3" />
          <TSpace size="small" class="tab-toolbar" wrap>
            <TButton v-if="!zone?.dnssec_enabled && perm.canWrite('dns')" theme="warning" size="small" @click="handleDnssecAction('enable')">
              {{ t('dns.zones.enableDnssec') }}
            </TButton>
            <TButton v-if="zone?.dnssec_enabled && perm.canWrite('dns')" theme="warning" size="small" @click="handleDnssecAction('disable')">
              {{ t('dns.zones.disableDnssec') }}
            </TButton>
            <TButton v-if="zone?.dnssec_enabled && perm.canWrite('dns')" size="small" variant="outline" @click="handleDnssecAction('rotate')">
              {{ t('dns.zones.rotateKeys') }}
            </TButton>
            <TButton v-if="perm.canWrite('dns')" size="small" variant="outline" @click="openGenerateKey">
              {{ t('dns.zones.dnssecGenerateKey') }}
            </TButton>
            <TButton v-if="perm.canWrite('dns')" size="small" variant="outline" @click="handlePromoteStandby">
              {{ t('dns.zones.dnssecPromote') }}
            </TButton>
            <TButton size="small" variant="outline" @click="loadDsRecords">{{ t('dns.zones.dnssecShowDs') }}</TButton>
          </TSpace>
          <TTable v-if="dnssecStatus?.keys?.length" :columns="dnssecKeyColumns" :data="dnssecStatus.keys" row-key="id" stripe hover />
          <TCard v-if="dsRecords.length" :bordered="false" :title="t('dns.zones.dnssecDsTitle')" class="mt-4">
            <TTable :columns="dsColumns" :data="dsRecords" row-key="key_tag" stripe hover />
          </TCard>
          <TCard :bordered="false" :title="t('dns.zones.dnssecNsec3Title')" class="mt-4">
            <TForm layout="inline" label-align="top">
              <TFormItem :label="t('dns.zones.dnssecNsec3Iterations')">
                <TInputNumber v-model="nsec3Form.iterations" :min="0" :max="100" :disabled="!perm.canWrite('dns')" />
              </TFormItem>
              <TFormItem :label="t('dns.zones.dnssecNsec3Salt')">
                <TInput v-model="nsec3Form.salt" placeholder="hex" :disabled="!perm.canWrite('dns')" style="width: 220px" />
              </TFormItem>
              <TFormItem :label="t('dns.zones.dnssecNsec3Optout')">
                <TSwitch v-model="nsec3Form.optout" :disabled="!perm.canWrite('dns')" />
              </TFormItem>
            </TForm>
            <p class="hint">{{ t('dns.zones.dnssecNsec3Hint') }}</p>
            <TButton v-if="perm.canWrite('dns')" theme="primary" size="small" :loading="nsec3Saving" @click="saveNsec3">
              {{ t('common.save') }}
            </TButton>
          </TCard>
        </TTabPanel>
      </TTabs>
    </TCard>

    <!-- Import dialog -->
    <TDialog v-model:visible="showImport" :header="t('dns.zones.importZone')" width="760px" :footer="false">
      <TAlert theme="info" :message="t('dns.zones.importPreviewHint')" class="mb-3" />
      <TSpace size="small" align="center">
        <TSelect v-model="importFormat" :options="importFormatOptions" style="width: 180px" @change="importPreview = null" />
        <TButton variant="outline" size="small" @click="importFileInput?.click()">{{ t('ipam.import.chooseFile') }}</TButton>
        <input
          ref="importFileInput"
          type="file"
          :accept="importFormat === 'csv' ? '.csv,text/csv' : '.zone,.bind,.txt,text/plain'"
          class="hidden-file"
          @change="loadImportFile"
        />
      </TSpace>
      <TTextarea
        v-model="importContent"
        class="mt-3"
        :autosize="{ minRows: 10, maxRows: 12 }"
        :placeholder="t('dns.zones.importContentPlaceholder')"
        @change="importPreview = null"
      />
      <TAlert
        v-if="importPreview"
        class="mt-3"
        :theme="importPreview.valid ? 'success' : 'warning'"
        :title="importPreview.valid ? t('dns.zones.importPreviewValid') : t('dns.zones.importPreviewConflicts')"
      >
        <div>{{ t('dns.zones.importRecordCount', { count: importPreview.record_count }) }}</div>
        <div v-if="Object.keys(importPreview.record_types || {}).length">
          {{ Object.entries(importPreview.record_types).map(([type, count]) => `${type}: ${count}`).join(' · ') }}
        </div>
        <ul v-if="importPreview.conflicts?.length" class="conflict-list">
          <li v-for="(conflict, index) in importPreview.conflicts" :key="`${conflict.row}-${index}`">
            {{ t('dns.zones.importConflictRow', { row: conflict.row ?? conflict.record, owner: conflict.owner, type: conflict.type, message: conflict.message }) }}
          </li>
        </ul>
      </TAlert>
      <div class="dialog-actions">
        <TButton variant="outline" @click="showImport = false">{{ t('common.cancel') }}</TButton>
        <TButton
          variant="outline"
          :loading="importPreviewLoading"
          :disabled="!perm.canWrite('dns') || !importContent.trim()"
          @click="previewImport"
        >
          {{ t('dns.zones.importPreview') }}
        </TButton>
        <TButton
          theme="primary"
          :loading="importApplying"
          :disabled="!perm.canWrite('dns') || !importPreview?.valid"
          @click="applyImport"
        >
          {{ t('dns.zones.importApply') }}
        </TButton>
      </div>
    </TDialog>

    <!-- Add/Edit record dialog -->
    <TDialog
      v-model:visible="showAddRecord"
      :header="editingRecord ? t('dns.records.editRecord') : t('dns.records.createRecord')"
      :confirm-btn="{ loading: recordSubmitting, content: t('common.save') }"
      :cancel-btn="t('common.cancel')"
      width="520px"
      @confirm="handleRecordSubmit"
    >
      <TForm ref="recordFormRef" :data="recordForm" :rules="recordRules" label-align="top">
        <TFormItem :label="t('dns.records.recordName')" name="name">
          <TInput v-model="recordForm.name" />
        </TFormItem>
        <TFormItem :label="t('dns.records.recordType')" name="type">
          <TSelect v-model="recordForm.type" :options="recordTypeOptions" :disabled="!!editingRecord" />
        </TFormItem>
        <TFormItem :label="t('dns.records.recordValue')" name="value">
          <TTextarea v-model="recordForm.value" :autosize="{ minRows: 3, maxRows: 5 }" />
        </TFormItem>
        <TFormItem :label="t('dns.zones.ttl')">
          <TInputNumber v-model="recordForm.ttl" :min="0" />
        </TFormItem>
        <TFormItem :label="t('common.priority')">
          <TInputNumber v-model="recordForm.priority" :min="0" :max="65535" />
        </TFormItem>
        <TFormItem label="expiry_ttl">
          <TInputNumber v-model="recordForm.expiry_ttl" :min="0" clearable placeholder="seconds / 0 = never" />
        </TFormItem>
        <TFormItem :label="t('common.enabled')">
          <TSwitch v-model="recordForm.enabled" />
        </TFormItem>
      </TForm>
    </TDialog>

    <!-- Generate DNSSEC key dialog -->
    <TDialog
      v-model:visible="showGenerateKey"
      :header="t('dns.zones.dnssecGenerateKey')"
      :confirm-btn="t('common.save')"
      :cancel-btn="t('common.cancel')"
      width="440px"
      @confirm="handleGenerateKey"
    >
      <TForm label-align="top">
        <TFormItem label="Key Type">
          <TSelect v-model="generateKeyType" :options="[{ label: 'KSK', value: 'KSK' }, { label: 'ZSK', value: 'ZSK' }]" />
        </TFormItem>
        <TFormItem label="Algorithm">
          <TSelect v-model="generateKeyAlgorithm" :options="dnssecAlgorithmOptions" />
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
.td-page-header h2 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
}
.subtitle {
  font-size: 12px;
  color: #9ca3af;
}
.tab-toolbar {
  margin-bottom: 12px;
}
.mb-4 {
  margin-bottom: 16px;
}
.mb-3 {
  margin-bottom: 12px;
}
.mt-3 {
  margin-top: 12px;
}
.mt-4 {
  margin-top: 16px;
}
.hint {
  margin: 8px 0 12px;
  font-size: 12px;
  color: #9ca3af;
}
.hidden-file {
  display: none;
}
.conflict-list {
  margin: 0;
  padding-left: 18px;
}
.dialog-actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  margin-top: 16px;
}
</style>
